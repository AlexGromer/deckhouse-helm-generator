package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// SecretFinding represents a detected hardcoded secret in a chart.
type SecretFinding struct {
	// Source is the file or section where the secret was found.
	Source string
	// Kind is the Kubernetes resource kind (e.g., "Secret") or "values" for values.yaml.
	Kind string
	// Key is the specific key that contains a secret value.
	Key string
	// Name is the resource name (for template secrets).
	Name string
}

// secretKeyPatterns are values.yaml key names that typically contain secrets.
var secretKeyPatterns = []string{"password", "secret", "token", "key"}

// DetectHardcodedSecrets scans chart templates and values for hardcoded secrets.
// It detects:
//   - kind: Secret resources with stringData or data sections
//   - values.yaml keys matching sensitive patterns (password, secret, token, key)
func DetectHardcodedSecrets(chart *types.GeneratedChart) []SecretFinding {
	var findings []SecretFinding

	// Scan templates for Secret resources
	for path, content := range chart.Templates {
		if !strings.Contains(content, "kind: Secret") {
			continue
		}

		name := extractSecretName(content)
		if strings.Contains(content, "stringData:") || strings.Contains(content, "data:") {
			findings = append(findings, SecretFinding{
				Source: path,
				Kind:   "Secret",
				Key:    "stringData/data",
				Name:   name,
			})
		}
	}

	// Scan values.yaml for sensitive keys
	findings = append(findings, scanValuesForSecrets(chart.ValuesYAML)...)

	return findings
}

// scanValuesForSecrets checks values.yaml content for keys matching secret patterns.
func scanValuesForSecrets(valuesYAML string) []SecretFinding {
	var findings []SecretFinding

	for _, line := range strings.Split(valuesYAML, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Skip empty values and references
		if value == "" || value == "{}" || value == "[]" || strings.HasPrefix(value, "{{") {
			continue
		}

		keyLower := strings.ToLower(key)
		for _, pattern := range secretKeyPatterns {
			if strings.Contains(keyLower, pattern) {
				findings = append(findings, SecretFinding{
					Source: "values.yaml",
					Kind:   "values",
					Key:    key,
				})
				break
			}
		}
	}

	return findings
}

// extractSecretName extracts the metadata.name from a Secret template.
func extractSecretName(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "name:") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "name:"))
		}
	}
	return "unknown"
}

// ConvertToExternalSecrets replaces Secret templates with ExternalSecret resources
// using the external-secrets-operator format.
// Supported providers: vault, aws, gcp, azure.
func ConvertToExternalSecrets(chart *types.GeneratedChart, provider string) *types.GeneratedChart {
	result := &types.GeneratedChart{
		Name:          chart.Name,
		ChartYAML:     chart.ChartYAML,
		ValuesYAML:    chart.ValuesYAML,
		Helpers:       chart.Helpers,
		Notes:         chart.Notes,
		ValuesSchema:  chart.ValuesSchema,
		ExternalFiles: chart.ExternalFiles,
		Templates:     make(map[string]string),
	}

	for path, content := range chart.Templates {
		if !strings.Contains(content, "kind: Secret") {
			result.Templates[path] = content
			continue
		}

		name := extractSecretName(content)
		keys := extractSecretKeys(content)
		esPath := fmt.Sprintf("templates/%s-externalsecret.yaml", name)
		result.Templates[esPath] = generateExternalSecret(name, provider, keys)
	}

	return result
}

// extractSecretKeys extracts data keys from a Secret template.
func extractSecretKeys(content string) []string {
	var keys []string
	inData := false

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)

		if trimmed == "stringData:" || trimmed == "data:" {
			inData = true
			continue
		}

		// End of data section
		if inData && !strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "\t") && trimmed != "" {
			inData = false
			continue
		}

		if inData && strings.Contains(trimmed, ":") {
			key := strings.TrimSpace(strings.SplitN(trimmed, ":", 2)[0])
			if key != "" {
				keys = append(keys, key)
			}
		}
	}

	return keys
}

// providerStoreRef returns the SecretStoreRef kind and name based on provider.
func providerStoreRef(provider string) (kind, storeName string) {
	switch provider {
	case "vault":
		return "ClusterSecretStore", "vault-backend"
	case "aws":
		return "ClusterSecretStore", "aws-secrets-manager"
	case "gcp":
		return "ClusterSecretStore", "gcp-secret-manager"
	case "azure":
		return "ClusterSecretStore", "azure-key-vault"
	default:
		return "ClusterSecretStore", provider + "-backend"
	}
}

// generateExternalSecret creates an ExternalSecret YAML template for the given provider.
func generateExternalSecret(name, provider string, keys []string) string {
	storeKind, storeName := providerStoreRef(provider)

	var sb strings.Builder
	sb.WriteString("apiVersion: external-secrets.io/v1beta1\n")
	sb.WriteString("kind: ExternalSecret\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s\n", name))
	sb.WriteString("  namespace: {{ .Release.Namespace }}\n")
	sb.WriteString("spec:\n")
	sb.WriteString("  refreshInterval: 1h\n")
	sb.WriteString("  secretStoreRef:\n")
	sb.WriteString(fmt.Sprintf("    kind: %s\n", storeKind))
	sb.WriteString(fmt.Sprintf("    name: %s\n", storeName))
	sb.WriteString("  target:\n")
	sb.WriteString(fmt.Sprintf("    name: %s\n", name))
	sb.WriteString("    creationPolicy: Owner\n")

	if len(keys) > 0 {
		sb.WriteString("  data:\n")
		for _, key := range keys {
			sb.WriteString(fmt.Sprintf("    - secretKey: %s\n", key))
			sb.WriteString("      remoteRef:\n")
			sb.WriteString(fmt.Sprintf("        key: %s/%s\n", name, key))
		}
	}

	return sb.String()
}
