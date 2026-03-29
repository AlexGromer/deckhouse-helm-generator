package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// SecretExampleKey represents a single key in a secret example.
type SecretExampleKey struct {
	Key              string
	Name             string
	Description      string
	Required         bool
	Example          string
	PlaceholderValue string
}

// keyName returns the effective name for this key entry (Name takes precedence over Key).
func (k SecretExampleKey) keyName() string {
	if k.Name != "" {
		return k.Name
	}
	return k.Key
}

// SecretExampleEntry holds the example information for one secret.
type SecretExampleEntry struct {
	Name      string
	Namespace string
	Type      string
	Keys      []SecretExampleKey
}

// SecretsExampleOptions configures secret example generation.
type SecretsExampleOptions struct {
	IncludeComments      bool
	Format               string // "dotenv", "yaml", "shell"
	IncludeGitignoreHint bool
	IncludeDescriptions  bool
	GroupByNamespace     bool
}

// DetectSecretsForExample scans the resource graph for Secrets and returns
// SecretExampleEntry entries suitable for generating example files.
// Excludes kubernetes.io/service-account-token type secrets.
func DetectSecretsForExample(graph *types.ResourceGraph) []SecretExampleEntry {
	var result []SecretExampleEntry
	if graph == nil {
		return result
	}
	for _, r := range graph.Resources {
		if r.Original.GVK.Kind != "Secret" {
			continue
		}
		obj := r.Original.Object
		// Exclude service account token secrets.
		secretType, _, _ := unstructuredStringMap(obj.Object, "type")
		if secretType[""] == "kubernetes.io/service-account-token" {
			continue
		}
		// Also check type as a direct string field.
		if typeVal, ok := obj.Object["type"]; ok {
			if typeStr, ok := typeVal.(string); ok && typeStr == "kubernetes.io/service-account-token" {
				continue
			}
		}

		entry := SecretExampleEntry{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		}
		// Inspect data/stringData keys.
		data, _, _ := unstructuredStringMap(obj.Object, "data")
		for k := range data {
			entry.Keys = append(entry.Keys, SecretExampleKey{
				Name:             k,
				Key:              k,
				Required:         true,
				PlaceholderValue: placeholderFor(k),
				Description:      describeKey(k),
			})
		}
		stringData, _, _ := unstructuredStringMap(obj.Object, "stringData")
		for k := range stringData {
			entry.Keys = append(entry.Keys, SecretExampleKey{
				Name:             k,
				Key:              k,
				Required:         true,
				PlaceholderValue: placeholderFor(k),
				Description:      describeKey(k),
			})
		}
		// If no keys found, add a synthetic placeholder key.
		if len(entry.Keys) == 0 {
			entry.Keys = append(entry.Keys, SecretExampleKey{
				Name:             "SECRET_VALUE",
				Key:              "SECRET_VALUE",
				Required:         true,
				PlaceholderValue: "<your-secret-value>",
				Description:      "Secret value (no keys detected; add your key names here)",
			})
		}
		result = append(result, entry)
	}
	return result
}

// GenerateSecretsExample generates an example secrets file (dotenv or YAML) from entries.
func GenerateSecretsExample(entries []SecretExampleEntry, opts SecretsExampleOptions) string {
	if len(entries) == 0 {
		return ""
	}
	format := opts.Format
	if format == "" {
		format = "dotenv"
	}

	var sb strings.Builder

	// Always write a header.
	sb.WriteString("# Secrets example file — fill in real values before use\n")
	sb.WriteString("# DO NOT commit this file with real secret values.\n")

	if opts.IncludeGitignoreHint {
		sb.WriteString("#\n")
		sb.WriteString("# Hint: add this file to .gitignore to avoid accidental commits:\n")
		sb.WriteString("#   echo '.env' >> .gitignore\n")
		sb.WriteString("#   echo '.env.example' >> .gitignore\n")
	}

	sb.WriteString("\n")

	if opts.GroupByNamespace {
		// Group entries by namespace.
		nsOrder := []string{}
		byNS := map[string][]SecretExampleEntry{}
		for _, entry := range entries {
			ns := entry.Namespace
			if ns == "" {
				ns = "default"
			}
			if _, seen := byNS[ns]; !seen {
				nsOrder = append(nsOrder, ns)
			}
			byNS[ns] = append(byNS[ns], entry)
		}
		for _, ns := range nsOrder {
			sb.WriteString(fmt.Sprintf("# === Namespace: %s ===\n", ns))
			for _, entry := range byNS[ns] {
				writeSecretsExampleEntry(&sb, entry, format, opts)
			}
		}
	} else {
		for _, entry := range entries {
			writeSecretsExampleEntry(&sb, entry, format, opts)
		}
	}

	return sb.String()
}

// writeSecretsExampleEntry writes a single secret's keys to the string builder.
func writeSecretsExampleEntry(sb *strings.Builder, entry SecretExampleEntry, format string, opts SecretsExampleOptions) {
	sb.WriteString(fmt.Sprintf("# Secret: %s/%s\n", entry.Namespace, entry.Name))
	for _, k := range entry.Keys {
		name := k.keyName()
		placeholder := k.PlaceholderValue
		if placeholder == "" {
			placeholder = placeholderFor(name)
		}
		if opts.IncludeDescriptions && k.Description != "" {
			sb.WriteString(fmt.Sprintf("# %s\n", k.Description))
		}
		switch format {
		case "yaml":
			sb.WriteString(fmt.Sprintf("%s: \"%s\"\n", name, placeholder))
		case "shell":
			sb.WriteString(fmt.Sprintf("export %s=\"%s\"\n", strings.ToUpper(name), placeholder))
		default: // dotenv
			sb.WriteString(fmt.Sprintf("%s=%s\n", strings.ToUpper(name), placeholder))
		}
	}
	sb.WriteString("\n")
}

// describeKey returns a human-readable description for a secret key.
func describeKey(keyName string) string {
	lower := strings.ToLower(keyName)
	switch {
	case lower == "tls.crt" || lower == "tls-crt" || strings.Contains(lower, "certificate"):
		return "TLS certificate (PEM format)"
	case lower == "tls.key" || lower == "tls-key":
		return "TLS private key (PEM format)"
	case strings.Contains(lower, "password") || strings.Contains(lower, "passwd"):
		return "Password or secret passphrase"
	case strings.Contains(lower, "token"):
		return "Authentication token or API key"
	case strings.Contains(lower, "user") || strings.Contains(lower, "username"):
		return "Username or account name"
	case strings.Contains(lower, "host"):
		return "Hostname or connection address"
	case strings.Contains(lower, "url") || strings.Contains(lower, "dsn"):
		return "Connection URL or DSN"
	case strings.Contains(lower, "key") || strings.Contains(lower, "api"):
		return "API key or access credential"
	default:
		return fmt.Sprintf("Secret value for '%s'", keyName)
	}
}

// placeholderFor returns a placeholder value for a secret key name.
func placeholderFor(keyName string) string {
	lower := strings.ToLower(keyName)
	switch {
	case lower == "tls.crt" || lower == "tls-crt" || strings.Contains(lower, "certificate"):
		return "<base64-encoded-PEM-certificate>"
	case lower == "tls.key" || lower == "tls-key":
		return "<base64-encoded-PEM-key>"
	case strings.Contains(lower, "password") || strings.Contains(lower, "passwd"):
		return "<your-password>"
	case strings.Contains(lower, "token"):
		return "<your-token>"
	case strings.Contains(lower, "user") || strings.Contains(lower, "username"):
		return "<your-username>"
	default:
		return "<" + strings.ToLower(keyName) + ">"
	}
}

// InjectSecretsExample injects a generated secrets example file into the chart's ExternalFiles.
func InjectSecretsExample(chart *types.GeneratedChart, graph *types.ResourceGraph, opts SecretsExampleOptions) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}
	entries := DetectSecretsForExample(graph)
	content := GenerateSecretsExample(entries, opts)
	result := copyChartTemplatesWithExternalFiles(chart)
	if content == "" {
		return result, 0
	}
	filename := ".env.example"
	if opts.Format == "yaml" {
		filename = "secrets-example.yaml"
	}
	result.ExternalFiles = append(result.ExternalFiles, types.ExternalFileInfo{
		Path:    filename,
		Content: content,
	})
	return result, 1
}

// copyChartTemplatesWithExternalFiles returns a shallow copy of the chart with cloned
// Templates and ExternalFiles slices.
func copyChartTemplatesWithExternalFiles(chart *types.GeneratedChart) *types.GeneratedChart {
	templates := make(map[string]string, len(chart.Templates))
	for k, v := range chart.Templates {
		templates[k] = v
	}
	externalFiles := make([]types.ExternalFileInfo, len(chart.ExternalFiles))
	copy(externalFiles, chart.ExternalFiles)
	return &types.GeneratedChart{
		Name:          chart.Name,
		Path:          chart.Path,
		ChartYAML:     chart.ChartYAML,
		ValuesYAML:    chart.ValuesYAML,
		Templates:     templates,
		Helpers:       chart.Helpers,
		Notes:         chart.Notes,
		ValuesSchema:  chart.ValuesSchema,
		ExternalFiles: externalFiles,
	}
}
