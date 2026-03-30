package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// KubeconformOptions configures kubeconform validation.
type KubeconformOptions struct {
	// KubernetesVersion is the target Kubernetes version for schema validation.
	KubernetesVersion string
	// Strict enables strict mode (reject unknown fields).
	Strict bool
	// IgnoreMissingSchemas skips validation of resources without a schema.
	IgnoreMissingSchemas bool
	// SchemaLocations is a list of additional schema location URLs.
	SchemaLocations []string
	// SkipKinds is a list of resource kinds to skip during validation.
	SkipKinds []string
	// CRDSchemaURLs maps group names to CRD schema URL templates.
	CRDSchemaURLs map[string]string
}

// KubeconformResult contains the generated kubeconform configuration and commands.
type KubeconformResult struct {
	// ConfigFiles is a map of filename to content (e.g., ".kubeconform.yaml").
	ConfigFiles map[string]string
	// Commands is a list of shell commands to run kubeconform validation.
	Commands []string
	// NOTESTxt contains usage instructions for developers.
	NOTESTxt string
}

// GenerateKubeconformConfig generates a .kubeconform.yaml config file and
// validation commands for the given chart and options.
// Returns nil if chart is nil.
func GenerateKubeconformConfig(chart *types.GeneratedChart, opts KubeconformOptions) *KubeconformResult {
	if chart == nil {
		return nil
	}

	version := opts.KubernetesVersion
	if version == "" {
		version = "1.29.0"
	}

	var sb strings.Builder
	sb.WriteString("# kubeconform configuration\n")
	sb.WriteString(fmt.Sprintf("kubernetes-version: %q\n", version))

	if opts.Strict {
		sb.WriteString("strict: true\n")
	}
	if opts.IgnoreMissingSchemas {
		sb.WriteString("ignore-missing-schemas: true\n")
	}

	if len(opts.SkipKinds) > 0 {
		sb.WriteString("skip-kinds:\n")
		for _, k := range opts.SkipKinds {
			sb.WriteString(fmt.Sprintf("  - %s\n", k))
		}
	}

	if len(opts.SchemaLocations) > 0 {
		sb.WriteString("schema-locations:\n")
		for _, loc := range opts.SchemaLocations {
			sb.WriteString(fmt.Sprintf("  - %q\n", loc))
		}
	}

	if len(opts.CRDSchemaURLs) > 0 {
		sb.WriteString("schema-locations:\n")
		for _, url := range opts.CRDSchemaURLs {
			sb.WriteString(fmt.Sprintf("  - %q\n", url))
		}
	}

	configContent := sb.String()

	// Collect template paths
	var templatePaths []string
	for path := range chart.Templates {
		if strings.HasPrefix(path, "templates/") && strings.HasSuffix(path, ".yaml") {
			templatePaths = append(templatePaths, path)
		}
	}

	// Build commands
	var commands []string
	if len(templatePaths) > 0 {
		cmdArgs := []string{"kubeconform", fmt.Sprintf("-kubernetes-version=%s", version)}
		if opts.Strict {
			cmdArgs = append(cmdArgs, "-strict")
		}
		if opts.IgnoreMissingSchemas {
			cmdArgs = append(cmdArgs, "-ignore-missing-schemas")
		}
		for _, k := range opts.SkipKinds {
			cmdArgs = append(cmdArgs, fmt.Sprintf("-skip=%s", k))
		}
		for _, url := range opts.CRDSchemaURLs {
			cmdArgs = append(cmdArgs, fmt.Sprintf("-schema-location=%s", url))
		}
		cmdArgs = append(cmdArgs, "templates/")
		commands = append(commands, strings.Join(cmdArgs, " "))
	} else {
		commands = append(commands, fmt.Sprintf("kubeconform -kubernetes-version=%s templates/", version))
	}

	notesTxt := fmt.Sprintf(`kubeconform validation for chart %q
---
Configuration: .kubeconform.yaml
Kubernetes version: %s

To validate chart templates, run:
  %s

Install kubeconform:
  go install github.com/yannh/kubeconform/cmd/kubeconform@latest
`, chart.Name, version, strings.Join(commands, "\n  "))

	return &KubeconformResult{
		ConfigFiles: map[string]string{
			".kubeconform.yaml": configContent,
		},
		Commands: commands,
		NOTESTxt: notesTxt,
	}
}

// InjectKubeconformConfig injects kubeconform config files into the chart using
// copy-on-write semantics. Returns the new chart and nil error on success.
// Returns nil, err if kubeResult is nil.
func InjectKubeconformConfig(chart *types.GeneratedChart, kubeResult *KubeconformResult) (*types.GeneratedChart, error) {
	if chart == nil {
		return nil, fmt.Errorf("chart is nil")
	}
	if kubeResult == nil {
		return nil, fmt.Errorf("kubeconform result is nil")
	}

	newChart := copyChartTemplates(chart)

	for filename, content := range kubeResult.ConfigFiles {
		newChart.Templates[filename] = content
	}

	return newChart, nil
}
