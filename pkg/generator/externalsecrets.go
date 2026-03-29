package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ESOBackend identifies the external secrets backend type.
type ESOBackend string

const (
	ESOBackendAWS   ESOBackend = "aws"
	ESOBackendVault ESOBackend = "vault"
	ESOBackendGCP   ESOBackend = "gcp"
	ESOBackendAzure ESOBackend = "azure"
)

// ESOOptions configures external secrets operator generation.
type ESOOptions struct {
	Backend              ESOBackend
	SecretStoreRef       string
	Namespace            string
	SecretStoreNamespace string
	Region               string
	AWSRegion            string
	VaultAddress         string
	VaultMount           string
	VaultRole            string
	RefreshInterval      string
}

// ESOSecretRef holds a detected secret reference.
type ESOSecretRef struct {
	Name      string
	Namespace string
	Keys      []string
}

// ESOSecret defines an ExternalSecret to be generated.
type ESOSecret struct {
	Name            string
	Namespace       string
	SecretStoreRef  string
	RemotePath      string
	Keys            []string
	RefreshInterval string
}

// DetectESOSecrets inspects the resource graph for Secrets that could be managed by ESO.
// Returns a slice of ESOSecretRef entries, one per detected Secret.
func DetectESOSecrets(graph *types.ResourceGraph) []ESOSecretRef {
	var result []ESOSecretRef
	if graph == nil {
		return result
	}
	for _, r := range graph.Resources {
		if r.Original.GVK.Kind != "Secret" {
			continue
		}
		obj := r.Original.Object
		entry := ESOSecretRef{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}
		// Collect keys from data field.
		if data, ok := obj.Object["data"]; ok {
			if dataMap, ok := data.(map[string]interface{}); ok {
				for k := range dataMap {
					entry.Keys = append(entry.Keys, k)
				}
			}
		}
		result = append(result, entry)
	}
	return result
}

// GenerateSecretStore generates SecretStore or ClusterSecretStore YAML manifests.
// Returns a slice of manifest strings.
func GenerateSecretStore(opts ESOOptions) []string {
	storeName := opts.SecretStoreRef
	if storeName == "" {
		storeName = "default-secret-store"
	}
	region := opts.AWSRegion
	if region == "" {
		region = opts.Region
	}
	if region == "" {
		region = "us-east-1"
	}

	refreshInterval := opts.RefreshInterval
	if refreshInterval == "" {
		refreshInterval = "1h"
	}

	var providerBlock strings.Builder
	switch opts.Backend {
	case ESOBackendAWS:
		providerBlock.WriteString(fmt.Sprintf("  aws:\n    service: SecretsManager\n    region: %s\n", region))
	case ESOBackendVault:
		addr := opts.VaultAddress
		if addr == "" {
			addr = "http://vault:8200"
		}
		mount := opts.VaultMount
		if mount == "" {
			mount = "secret"
		}
		role := opts.VaultRole
		if role == "" {
			role = "eso-role"
		}
		providerBlock.WriteString(fmt.Sprintf(
			"  vault:\n    server: %s\n    path: %s\n    version: v2\n    auth:\n      kubernetes:\n        mountPath: kubernetes\n        role: %s\n",
			addr, mount, role))
	case ESOBackendGCP:
		providerBlock.WriteString("  gcpsm:\n    projectID: my-project\n")
	case ESOBackendAzure:
		providerBlock.WriteString("  azurekv:\n    vaultUrl: https://my-vault.vault.azure.net\n")
	default:
		providerBlock.WriteString("  # unsupported backend\n")
	}

	ns := opts.SecretStoreNamespace
	if ns == "" {
		ns = opts.Namespace
	}
	isCluster := ns == ""
	kind := "SecretStore"
	if isCluster {
		kind = "ClusterSecretStore"
	}

	var sb strings.Builder
	sb.WriteString("apiVersion: external-secrets.io/v1beta1\n")
	sb.WriteString(fmt.Sprintf("kind: %s\n", kind))
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s\n", storeName))
	if !isCluster {
		sb.WriteString(fmt.Sprintf("  namespace: %s\n", ns))
	}
	sb.WriteString(fmt.Sprintf("  refreshInterval: %s\n", refreshInterval))
	sb.WriteString("spec:\n  provider:\n")
	sb.WriteString(providerBlock.String())

	return []string{sb.String()}
}

// GenerateExternalSecrets generates ExternalSecret manifests for the given secrets.
func GenerateExternalSecrets(secrets []ESOSecret, opts ESOOptions) []string {
	var results []string
	storeRef := opts.SecretStoreRef
	if storeRef == "" {
		storeRef = "default-secret-store"
	}
	refreshInterval := opts.RefreshInterval
	if refreshInterval == "" {
		refreshInterval = "1h"
	}
	for _, secret := range secrets {
		ref := secret.SecretStoreRef
		if ref == "" {
			ref = storeRef
		}
		ri := secret.RefreshInterval
		if ri == "" {
			ri = refreshInterval
		}
		var sb strings.Builder
		sb.WriteString("apiVersion: external-secrets.io/v1beta1\n")
		sb.WriteString("kind: ExternalSecret\n")
		sb.WriteString("metadata:\n")
		sb.WriteString(fmt.Sprintf("  name: %s\n", secret.Name))
		if secret.Namespace != "" {
			sb.WriteString(fmt.Sprintf("  namespace: %s\n", secret.Namespace))
		}
		sb.WriteString("spec:\n")
		sb.WriteString(fmt.Sprintf("  refreshInterval: %s\n", ri))
		sb.WriteString("  secretStoreRef:\n")
		sb.WriteString(fmt.Sprintf("    name: %s\n", ref))
		sb.WriteString("    kind: SecretStore\n")
		sb.WriteString("  target:\n")
		sb.WriteString(fmt.Sprintf("    name: %s\n", secret.Name))
		if len(secret.Keys) > 0 {
			sb.WriteString("  data:\n")
			remotePath := secret.RemotePath
			if remotePath == "" {
				remotePath = secret.Name
			}
			for _, k := range secret.Keys {
				sb.WriteString(fmt.Sprintf("  - secretKey: %s\n", k))
				sb.WriteString("    remoteRef:\n")
				sb.WriteString(fmt.Sprintf("      key: %s/%s\n", remotePath, k))
			}
		}
		results = append(results, sb.String())
	}
	return results
}

// BuildESOValuesFragment returns a Helm values map fragment for ESO configuration.
func BuildESOValuesFragment(secrets []ESOSecret, opts ESOOptions) map[string]interface{} {
	secretNames := make([]string, 0, len(secrets))
	for _, s := range secrets {
		secretNames = append(secretNames, s.Name)
	}
	ri := opts.RefreshInterval
	if ri == "" {
		ri = "1h"
	}
	return map[string]interface{}{
		"externalSecrets": map[string]interface{}{
			"enabled":         true,
			"backend":         string(opts.Backend),
			"secretStoreRef":  opts.SecretStoreRef,
			"namespace":       opts.Namespace,
			"refreshInterval": ri,
			"secrets":         secretNames,
		},
	}
}

// InjectESO injects ExternalSecret manifests derived from a resource graph into a chart.
func InjectESO(chart *types.GeneratedChart, graph *types.ResourceGraph, opts ESOOptions) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}
	result := copyChartTemplatesWithExternalFiles(chart)
	if graph == nil {
		return result, 0
	}

	// Detect secrets from graph.
	refs := DetectESOSecrets(graph)
	secrets := make([]ESOSecret, 0, len(refs))
	for _, ref := range refs {
		secrets = append(secrets, ESOSecret{
			Name:      ref.Name,
			Namespace: ref.Namespace,
			Keys:      ref.Keys,
		})
	}

	manifests := GenerateExternalSecrets(secrets, opts)
	count := 0
	for i, manifest := range manifests {
		path := fmt.Sprintf("templates/external-secret-%d.yaml", i)
		result.Templates[path] = manifest
		count++
	}
	return result, count
}
