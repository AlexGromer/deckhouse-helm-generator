package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// SealedSecretScope controls the scope of a SealedSecret.
type SealedSecretScope string

const (
	SealedSecretScopeStrict        SealedSecretScope = "strict"
	SealedSecretScopeNamespaceWide SealedSecretScope = "namespace-wide"
	SealedSecretScopeClusterWide   SealedSecretScope = "cluster-wide"
)

// SealedSecretEntry defines a single secret to be sealed.
type SealedSecretEntry struct {
	Name        string
	Namespace   string
	Scope       SealedSecretScope
	Data        map[string]string
	Keys        []string
	KubesealCmd string
}

// SealedSecretOptions configures sealed secrets generation.
type SealedSecretOptions struct {
	Scope               SealedSecretScope
	CertificateRef      string
	Namespace           string
	ControllerNamespace string
	ControllerName      string
	CertURL             string
}

// DetectSealedSecretCandidates scans the resource graph for plain Secrets
// that are candidates for sealing. Returns a slice of SealedSecretEntry.
// Excludes kubernetes.io/service-account-token type secrets.
func DetectSealedSecretCandidates(graph *types.ResourceGraph) []SealedSecretEntry {
	var result []SealedSecretEntry
	if graph == nil {
		return result
	}
	for _, r := range graph.Resources {
		if r.Original.GVK.Kind != "Secret" {
			continue
		}
		obj := r.Original.Object
		// Exclude service account token secrets.
		secretType, _, _ := unstructuredString(obj.Object, "type")
		if secretType == "kubernetes.io/service-account-token" {
			continue
		}
		entry := SealedSecretEntry{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
			Scope:     SealedSecretScopeStrict,
		}
		// Collect keys.
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

// GenerateSealedSecrets generates SealedSecret YAML manifests for the given entries.
func GenerateSealedSecrets(entries []SealedSecretEntry, opts SealedSecretOptions) []string {
	var results []string
	scope := opts.Scope
	if scope == "" {
		scope = SealedSecretScopeStrict
	}
	for _, entry := range entries {
		entryScope := entry.Scope
		if entryScope == "" {
			entryScope = scope
		}
		ns := entry.Namespace
		if ns == "" {
			ns = opts.Namespace
		}

		var sb strings.Builder
		sb.WriteString("apiVersion: bitnami.com/v1alpha1\n")
		sb.WriteString("kind: SealedSecret\n")
		sb.WriteString("metadata:\n")
		sb.WriteString(fmt.Sprintf("  name: %s\n", entry.Name))
		if ns != "" {
			sb.WriteString(fmt.Sprintf("  namespace: %s\n", ns))
		}
		// Add scope annotations only for non-strict scopes.
		switch entryScope {
		case SealedSecretScopeNamespaceWide:
			sb.WriteString("  annotations:\n")
			sb.WriteString("    sealedsecrets.bitnami.com/namespace-wide: \"true\"\n")
		case SealedSecretScopeClusterWide:
			sb.WriteString("  annotations:\n")
			sb.WriteString("    sealedsecrets.bitnami.com/cluster-wide: \"true\"\n")
		}
		sb.WriteString("spec:\n")
		sb.WriteString("  encryptedData: {}\n")
		results = append(results, sb.String())
	}
	return results
}

// InjectSealedSecrets injects SealedSecret manifests into a chart's templates.
func InjectSealedSecrets(chart *types.GeneratedChart, graph *types.ResourceGraph, opts SealedSecretOptions) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}
	entries := DetectSealedSecretCandidates(graph)
	result := copyChartTemplates(chart)
	manifests := GenerateSealedSecrets(entries, opts)
	count := 0
	for i, manifest := range manifests {
		path := fmt.Sprintf("templates/sealed-secret-%d.yaml", i)
		result.Templates[path] = manifest
		count++
	}
	// Append notes.
	notes := GenerateSealedSecretsNotes(entries, opts)
	if notes != "" {
		var notesBuilder strings.Builder
		if result.Notes != "" {
			notesBuilder.WriteString(result.Notes)
			if !strings.HasSuffix(result.Notes, "\n") {
				notesBuilder.WriteString("\n")
			}
			notesBuilder.WriteString("\n")
		}
		notesBuilder.WriteString(notes)
		result.Notes = notesBuilder.String()
	}
	return result, count
}

// BuildKubesealCommands generates kubeseal CLI commands for encrypting each entry.
// Returns a copy of the entries with the KubesealCmd field populated.
func BuildKubesealCommands(entries []SealedSecretEntry, opts SealedSecretOptions) []SealedSecretEntry {
	certURL := opts.CertURL
	ctrlNS := opts.ControllerNamespace
	if ctrlNS == "" {
		ctrlNS = "kube-system"
	}
	ctrlName := opts.ControllerName
	if ctrlName == "" {
		ctrlName = "sealed-secrets-controller"
	}

	result := make([]SealedSecretEntry, len(entries))
	for i, entry := range entries {
		cmd := fmt.Sprintf("kubeseal --controller-namespace %s --controller-name %s", ctrlNS, ctrlName)
		if certURL != "" {
			cmd += " --cert " + certURL
		}
		cmd += fmt.Sprintf(" --namespace %s --name %s", entry.Namespace, entry.Name)
		entry.KubesealCmd = cmd
		result[i] = entry
	}
	return result
}

// GenerateSealedSecretsNotes generates human-readable notes for sealed secrets.
func GenerateSealedSecretsNotes(entries []SealedSecretEntry, opts SealedSecretOptions) string {
	var sb strings.Builder
	sb.WriteString("## Sealed Secrets\n\n")
	if len(entries) == 0 {
		sb.WriteString("No secrets detected.\n")
		return sb.String()
	}
	sb.WriteString(fmt.Sprintf("Total: %d secret(s) to seal\n\n", len(entries)))
	for i, entry := range entries {
		cmd := entry.KubesealCmd
		if cmd == "" {
			// Build a default command if not already set.
			withCmd := BuildKubesealCommands([]SealedSecretEntry{entry}, opts)
			cmd = withCmd[0].KubesealCmd
		}
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, cmd))
	}
	return sb.String()
}

// unstructuredString reads a string field from an unstructured object map.
// Returns (value, found, error).
func unstructuredString(obj map[string]interface{}, field string) (string, bool, error) {
	v, ok := obj[field]
	if !ok {
		return "", false, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", true, fmt.Errorf("field %q is not a string", field)
	}
	return s, true, nil
}
