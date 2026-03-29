package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// VaultCSIOptions configures Vault CSI Secret Provider Class generation.
type VaultCSIOptions struct {
	VaultAddress         string
	VaultRole            string
	KubernetesMount      string
	Namespace            string
	Provider             string
	KVVersion            int
	MountPath            string
	EnableRotation       bool
	RotationPollInterval string
	NodePublishSecretRef string
}

// VaultCSIObjectSpec defines a secret object to be synced via CSI.
type VaultCSIObjectSpec struct {
	ObjectName  string
	ObjectType  string
	SecretPath  string
	ObjectAlias string
	SecretKey   string
}

// VaultCSIEntry holds a CSI entry candidate.
type VaultCSIEntry struct {
	Name      string
	Namespace string
	WorkloadName       string
	ServiceAccountName string
	Objects            []VaultCSIObjectSpec
}

// DetectVaultCSICandidates scans the resource graph for Secrets suitable for Vault CSI.
// It uses relationship data to associate secrets with their workload consumers.
func DetectVaultCSICandidates(graph *types.ResourceGraph, opts VaultCSIOptions) []VaultCSIEntry {
	var result []VaultCSIEntry
	if graph == nil {
		return result
	}
	for _, r := range graph.Resources {
		if r.Original.GVK.Kind != "Secret" {
			continue
		}
		obj := r.Original.Object
		entry := VaultCSIEntry{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}
		// Look for workloads that reference this secret via volume mount.
		secretKey := r.Original.ResourceKey()
		for _, rel := range graph.GetRelationshipsTo(secretKey) {
			if rel.Type == types.RelationVolumeMount {
				if workload, ok := graph.GetResourceByKey(rel.From); ok {
					entry.WorkloadName = workload.Original.Object.GetName()
					break
				}
			}
		}
		result = append(result, entry)
	}
	return result
}

// GenerateSecretProviderClasses generates SecretProviderClass YAML manifests.
func GenerateSecretProviderClasses(entries []VaultCSIEntry, opts VaultCSIOptions) []string {
	var results []string
	vaultAddr := opts.VaultAddress
	if vaultAddr == "" {
		vaultAddr = "http://vault:8200"
	}
	role := opts.VaultRole
	if role == "" {
		role = "default"
	}
	for _, entry := range entries {
		ns := entry.Namespace
		if ns == "" {
			ns = opts.Namespace
		}

		var objectsBlock strings.Builder
		for _, obj := range entry.Objects {
			objectsBlock.WriteString(fmt.Sprintf("    - objectName: %s\n      secretPath: %s\n      objectType: %s\n",
				obj.ObjectName, obj.SecretPath, obj.ObjectType))
		}

		manifest := fmt.Sprintf(`apiVersion: secrets-store.csi.x-k8s.io/v1
kind: SecretProviderClass
metadata:
  name: %s
  namespace: %s
spec:
  provider: vault
  parameters:
    vaultAddress: %s
    roleName: %s
    objects: |
%s`, entry.Name, ns, vaultAddr, role, objectsBlock.String())
		results = append(results, manifest)
	}
	return results
}

// GenerateVaultCSIVolumePatch generates a YAML snippet for adding a CSI volume and
// volumeMount to a workload for the given VaultCSIEntry and options.
// KVVersion=2 injects /data/ into the secret path; KVVersion=1 uses the path as-is.
func GenerateVaultCSIVolumePatch(entry VaultCSIEntry, opts VaultCSIOptions) string {
	mountPath := opts.MountPath
	if mountPath == "" {
		mountPath = "/mnt/secrets-store"
	}

	var objectsBlock strings.Builder
	for _, obj := range entry.Objects {
		secretPath := obj.SecretPath
		// KV v2: inject /data/ segment after the mount prefix
		if opts.KVVersion == 2 && !strings.Contains(secretPath, "/data/") {
			parts := strings.SplitN(secretPath, "/", 2)
			if len(parts) == 2 {
				secretPath = parts[0] + "/data/" + parts[1]
			}
		}
		objectsBlock.WriteString(fmt.Sprintf("      - objectName: %s\n        secretPath: %s\n        secretKey: %s\n",
			obj.ObjectName, secretPath, obj.SecretKey))
	}

	var sb strings.Builder
	// Build the csi.volumeAttributes block. "objects" is a multi-line string value
	// inside volumeAttributes, as required by the Vault CSI Provider spec.
	sb.WriteString(fmt.Sprintf("volumes:\n  - name: vault-secrets\n    csi:\n      driver: secrets-store.csi.k8s.io\n      readOnly: true\n      volumeAttributes:\n        secretProviderClass: %s\n        objects: |\n%s",
		entry.Name, objectsBlock.String()))

	// nodePublishSecretRef is a sibling of volumeAttributes inside csi:
	if opts.NodePublishSecretRef != "" {
		sb.WriteString(fmt.Sprintf("      nodePublishSecretRef:\n        name: %s\n", opts.NodePublishSecretRef))
	}

	// rotationPollInterval is a key inside csi.volumeAttributes, not a top-level field.
	if opts.EnableRotation && opts.RotationPollInterval != "" {
		sb.WriteString(fmt.Sprintf("        rotationPollInterval: %s\n", opts.RotationPollInterval))
	}

	sb.WriteString(fmt.Sprintf("volumeMounts:\n  - name: vault-secrets\n    mountPath: %s\n    readOnly: true\n", mountPath))

	return sb.String()
}

// InjectVaultCSI injects SecretProviderClass manifests into a chart.
func InjectVaultCSI(chart *types.GeneratedChart, graph *types.ResourceGraph, opts VaultCSIOptions) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}
	entries := DetectVaultCSICandidates(graph, opts)
	result := copyChartTemplatesWithExternalFiles(chart)
	manifests := GenerateSecretProviderClasses(entries, opts)
	count := 0
	for i, manifest := range manifests {
		path := fmt.Sprintf("templates/vault-csi-%d.yaml", i)
		result.Templates[path] = manifest
		count++
	}
	return result, count
}
