// Package helm provides utilities for generating Helm chart components.
package helm

import (
	"fmt"
	"strings"
)

// ChartMetadata contains metadata for a Helm chart.
type ChartMetadata struct {
	Name        string
	Version     string
	APIVersion  string
	AppVersion  string
	Description string
	Type        string
	Keywords    []string
	Home        string
	Sources     []string
	Maintainers []Maintainer
	Icon        string
	KubeVersion string
	Dependencies []Dependency
}

// Maintainer represents a chart maintainer.
type Maintainer struct {
	Name  string
	Email string
	URL   string
}

// Dependency represents a chart dependency.
type Dependency struct {
	Name       string
	Version    string
	Repository string
	Condition  string
	Tags       []string
	Enabled    bool
	Alias      string
}

// GenerateChartYAML generates the Chart.yaml content.
func GenerateChartYAML(meta ChartMetadata) string {
	var sb strings.Builder

	// API version
	apiVersion := meta.APIVersion
	if apiVersion == "" {
		apiVersion = "v2"
	}
	sb.WriteString(fmt.Sprintf("apiVersion: %s\n", apiVersion))

	// Name
	sb.WriteString(fmt.Sprintf("name: %s\n", meta.Name))

	// Description
	description := meta.Description
	if description == "" {
		description = fmt.Sprintf("A Helm chart for %s", meta.Name)
	}
	sb.WriteString(fmt.Sprintf("description: %s\n", description))

	// Type
	chartType := meta.Type
	if chartType == "" {
		chartType = "application"
	}
	sb.WriteString(fmt.Sprintf("type: %s\n", chartType))

	// Version
	version := meta.Version
	if version == "" {
		version = "0.1.0"
	}
	sb.WriteString(fmt.Sprintf("version: %s\n", version))

	// AppVersion
	appVersion := meta.AppVersion
	if appVersion == "" {
		appVersion = "1.0.0"
	}
	sb.WriteString(fmt.Sprintf("appVersion: %s\n", appVersion))

	// Keywords
	if len(meta.Keywords) > 0 {
		sb.WriteString("keywords:\n")
		for _, kw := range meta.Keywords {
			sb.WriteString(fmt.Sprintf("  - %s\n", kw))
		}
	}

	// Home
	if meta.Home != "" {
		sb.WriteString(fmt.Sprintf("home: %s\n", meta.Home))
	}

	// Sources
	if len(meta.Sources) > 0 {
		sb.WriteString("sources:\n")
		for _, src := range meta.Sources {
			sb.WriteString(fmt.Sprintf("  - %s\n", src))
		}
	}

	// Maintainers
	if len(meta.Maintainers) > 0 {
		sb.WriteString("maintainers:\n")
		for _, m := range meta.Maintainers {
			sb.WriteString(fmt.Sprintf("  - name: %s\n", m.Name))
			if m.Email != "" {
				sb.WriteString(fmt.Sprintf("    email: %s\n", m.Email))
			}
			if m.URL != "" {
				sb.WriteString(fmt.Sprintf("    url: %s\n", m.URL))
			}
		}
	}

	// Icon
	if meta.Icon != "" {
		sb.WriteString(fmt.Sprintf("icon: %s\n", meta.Icon))
	}

	// KubeVersion
	if meta.KubeVersion != "" {
		sb.WriteString(fmt.Sprintf("kubeVersion: %s\n", meta.KubeVersion))
	}

	// Dependencies
	if len(meta.Dependencies) > 0 {
		sb.WriteString("dependencies:\n")
		for _, dep := range meta.Dependencies {
			sb.WriteString(fmt.Sprintf("  - name: %s\n", dep.Name))
			sb.WriteString(fmt.Sprintf("    version: %s\n", dep.Version))
			sb.WriteString(fmt.Sprintf("    repository: %s\n", dep.Repository))
			if dep.Condition != "" {
				sb.WriteString(fmt.Sprintf("    condition: %s\n", dep.Condition))
			}
			if len(dep.Tags) > 0 {
				sb.WriteString("    tags:\n")
				for _, tag := range dep.Tags {
					sb.WriteString(fmt.Sprintf("      - %s\n", tag))
				}
			}
			if dep.Alias != "" {
				sb.WriteString(fmt.Sprintf("    alias: %s\n", dep.Alias))
			}
		}
	}

	return sb.String()
}

// GenerateNOTES generates the NOTES.txt content.
func GenerateNOTES(chartName string, services []string) string {
	var sb strings.Builder

	sb.WriteString("===================================================================\n")
	sb.WriteString(fmt.Sprintf("  %s has been installed successfully!\n", chartName))
	sb.WriteString("===================================================================\n\n")

	sb.WriteString("To verify the deployment, run:\n\n")
	sb.WriteString(fmt.Sprintf("  kubectl get all -l app.kubernetes.io/instance={{ .Release.Name }} -n {{ .Release.Namespace }}\n\n"))

	if len(services) > 0 {
		sb.WriteString("Installed services:\n\n")
		for _, svc := range services {
			sb.WriteString(fmt.Sprintf("  - %s\n", svc))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("To customize the installation, edit the values.yaml file and upgrade:\n\n")
	sb.WriteString(fmt.Sprintf("  helm upgrade {{ .Release.Name }} ./%s -n {{ .Release.Namespace }}\n\n", chartName))

	sb.WriteString("For more information, see the chart README.md\n")

	return sb.String()
}

// GenerateREADME generates a basic README.md for the chart.
func GenerateREADME(meta ChartMetadata, services []string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s\n\n", meta.Name))

	description := meta.Description
	if description == "" {
		description = fmt.Sprintf("A Helm chart for %s", meta.Name)
	}
	sb.WriteString(fmt.Sprintf("%s\n\n", description))

	sb.WriteString("## Installation\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString(fmt.Sprintf("helm install my-release ./%s\n", meta.Name))
	sb.WriteString("```\n\n")

	sb.WriteString("## Configuration\n\n")
	sb.WriteString("The following table lists the configurable parameters:\n\n")
	sb.WriteString("| Parameter | Description | Default |\n")
	sb.WriteString("|-----------|-------------|----------|\n")
	sb.WriteString("| `global.imageRegistry` | Global Docker image registry | `\"\"` |\n")
	sb.WriteString("| `global.imagePullSecrets` | Global image pull secrets | `[]` |\n\n")

	if len(services) > 0 {
		sb.WriteString("## Services\n\n")
		sb.WriteString("This chart includes the following services:\n\n")
		for _, svc := range services {
			sb.WriteString(fmt.Sprintf("- **%s**: Enabled by default\n", svc))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Uninstalling\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("helm uninstall my-release\n")
	sb.WriteString("```\n\n")

	sb.WriteString("## Generated by\n\n")
	sb.WriteString("This chart was generated by [Deckhouse Helm Generator](https://github.com/deckhouse/deckhouse-helm-generator)\n")

	return sb.String()
}
