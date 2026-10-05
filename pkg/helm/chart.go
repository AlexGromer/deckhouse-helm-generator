// Package helm provides utilities for generating Helm chart components.
package helm

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ChartMetadata contains metadata for a Helm chart.
type ChartMetadata struct {
	Name         string
	Version      string
	APIVersion   string
	AppVersion   string
	Description  string
	Type         string
	Keywords     []string
	Home         string
	Sources      []string
	Maintainers  []Maintainer
	Icon         string
	KubeVersion  string
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
	fmt.Fprintf(&sb, "apiVersion: %s\n", apiVersion)

	// Name
	fmt.Fprintf(&sb, "name: %s\n", meta.Name)

	// Description
	description := meta.Description
	if description == "" {
		description = fmt.Sprintf("A Helm chart for %s", meta.Name)
	}
	fmt.Fprintf(&sb, "description: %s\n", description)

	// Type
	chartType := meta.Type
	if chartType == "" {
		chartType = "application"
	}
	fmt.Fprintf(&sb, "type: %s\n", chartType)

	// Version
	version := meta.Version
	if version == "" {
		version = "0.1.0"
	}
	fmt.Fprintf(&sb, "version: %s\n", version)

	// AppVersion
	appVersion := meta.AppVersion
	if appVersion == "" {
		appVersion = "1.0.0"
	}
	fmt.Fprintf(&sb, "appVersion: %s\n", appVersion)

	// Keywords
	if len(meta.Keywords) > 0 {
		sb.WriteString("keywords:\n")
		for _, kw := range meta.Keywords {
			fmt.Fprintf(&sb, "  - %s\n", kw)
		}
	}

	// Home
	if meta.Home != "" {
		fmt.Fprintf(&sb, "home: %s\n", meta.Home)
	}

	// Sources
	if len(meta.Sources) > 0 {
		sb.WriteString("sources:\n")
		for _, src := range meta.Sources {
			fmt.Fprintf(&sb, "  - %s\n", src)
		}
	}

	// Maintainers
	if len(meta.Maintainers) > 0 {
		sb.WriteString("maintainers:\n")
		for _, m := range meta.Maintainers {
			fmt.Fprintf(&sb, "  - name: %s\n", m.Name)
			if m.Email != "" {
				fmt.Fprintf(&sb, "    email: %s\n", m.Email)
			}
			if m.URL != "" {
				fmt.Fprintf(&sb, "    url: %s\n", m.URL)
			}
		}
	}

	// Icon
	if meta.Icon != "" {
		fmt.Fprintf(&sb, "icon: %s\n", meta.Icon)
	}

	// KubeVersion
	if meta.KubeVersion != "" {
		fmt.Fprintf(&sb, "kubeVersion: %s\n", meta.KubeVersion)
	}

	// Dependencies
	if len(meta.Dependencies) > 0 {
		sb.WriteString("dependencies:\n")
		for _, dep := range meta.Dependencies {
			fmt.Fprintf(&sb, "  - name: %s\n", dep.Name)
			fmt.Fprintf(&sb, "    version: %s\n", dep.Version)
			fmt.Fprintf(&sb, "    repository: %s\n", dep.Repository)
			if dep.Condition != "" {
				fmt.Fprintf(&sb, "    condition: %s\n", dep.Condition)
			}
			if len(dep.Tags) > 0 {
				sb.WriteString("    tags:\n")
				for _, tag := range dep.Tags {
					fmt.Fprintf(&sb, "      - %s\n", tag)
				}
			}
			if dep.Alias != "" {
				fmt.Fprintf(&sb, "    alias: %s\n", dep.Alias)
			}
		}
	}

	return sb.String()
}

// NOTESContext provides additional context for dynamic NOTES.txt generation.
type NOTESContext struct {
	ServiceTypes []string // Kubernetes Service types present (e.g., "LoadBalancer", "ClusterIP")
	HasIngress   bool     // Whether the chart includes Ingress resources
	HasAuth      bool     // Whether the chart includes authentication configuration
}

// GenerateNOTES generates the NOTES.txt content with context-aware sections.
func GenerateNOTES(chartName string, services []string, ctx NOTESContext) string {
	var sb strings.Builder

	sb.WriteString("===================================================================\n")
	fmt.Fprintf(&sb, "  %s has been installed successfully!\n", chartName)
	sb.WriteString("===================================================================\n\n")

	sb.WriteString("To verify the deployment, run:\n\n")
	sb.WriteString("  kubectl get all -l app.kubernetes.io/instance={{ .Release.Name }} -n {{ .Release.Namespace }}\n\n")

	if len(services) > 0 {
		sb.WriteString("Installed services:\n\n")
		for _, svc := range services {
			fmt.Fprintf(&sb, "  - %s\n", svc)
		}
		sb.WriteString("\n")
	}

	// Dynamic section: LoadBalancer IP retrieval
	for _, st := range ctx.ServiceTypes {
		if strings.EqualFold(st, "LoadBalancer") {
			sb.WriteString("Get the LoadBalancer IP:\n\n")
			sb.WriteString("  kubectl get svc -l app.kubernetes.io/instance={{ .Release.Name }} -n {{ .Release.Namespace }} -o jsonpath='{.items[?(@.spec.type==\"LoadBalancer\")].status.loadBalancer.ingress[0].ip}'\n\n")
			break
		}
	}

	// Dynamic section: Ingress URL
	if ctx.HasIngress {
		sb.WriteString("Get the application URL:\n\n")
		sb.WriteString("  kubectl get ingress -l app.kubernetes.io/instance={{ .Release.Name }} -n {{ .Release.Namespace }}\n\n")
	}

	// Dynamic section: port-forward for ClusterIP (default access method when no LB/Ingress)
	hasLB := false
	for _, st := range ctx.ServiceTypes {
		if strings.EqualFold(st, "LoadBalancer") {
			hasLB = true
			break
		}
	}
	if !hasLB && !ctx.HasIngress {
		sb.WriteString("Access the application via port-forward:\n\n")
		fmt.Fprintf(&sb, "  kubectl port-forward svc/%s 8080:80 -n {{ .Release.Namespace }}\n\n", chartName)
	}

	// Dynamic section: Auth configuration notice
	if ctx.HasAuth {
		sb.WriteString("Authentication is enabled. Ensure auth secrets are configured:\n\n")
		sb.WriteString("  kubectl get secret -l app.kubernetes.io/instance={{ .Release.Name }} -n {{ .Release.Namespace }}\n\n")
	}

	sb.WriteString("To customize the installation, edit the values.yaml file and upgrade:\n\n")
	fmt.Fprintf(&sb, "  helm upgrade {{ .Release.Name }} ./%s -n {{ .Release.Namespace }}\n\n", chartName)

	sb.WriteString("For more information, see the chart README.md\n")

	return sb.String()
}

// GenerateREADME generates the chart's README.md with a parameter table
// built from its default values (one row per leaf, dot-notation paths usable
// with --set).
func GenerateREADME(meta ChartMetadata, values map[string]interface{}) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# %s\n\n", meta.Name)

	description := meta.Description
	if description == "" {
		description = fmt.Sprintf("A Helm chart for %s", meta.Name)
	}
	fmt.Fprintf(&sb, "%s\n\n", description)

	sb.WriteString("## Installation\n\n")
	sb.WriteString("```bash\n")
	fmt.Fprintf(&sb, "helm install my-release ./%s\n", meta.Name)
	sb.WriteString("```\n\n")

	sb.WriteString("## Parameters\n\n")
	rows := flattenValues("", values, nil)
	if len(rows) == 0 {
		sb.WriteString("This chart has no configurable parameters.\n\n")
	} else {
		sb.WriteString("| Parameter | Default |\n")
		sb.WriteString("|-----------|---------|\n")
		for _, r := range rows {
			fmt.Fprintf(&sb, "| `%s` | `%s` |\n", r[0], r[1])
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Uninstalling\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("helm uninstall my-release\n")
	sb.WriteString("```\n\n")

	sb.WriteString("## Generated by\n\n")
	sb.WriteString("This chart was generated by [Deckhouse Helm Generator](https://github.com/AlexGromer/deckhouse-helm-generator)\n")

	return sb.String()
}

// flattenValues lists leaf values as (path, JSON default) rows, sorted by
// path. Lists and empty maps are leaves; long defaults are shortened.
func flattenValues(prefix string, v interface{}, rows [][2]string) [][2]string {
	if m, ok := v.(map[string]interface{}); ok && len(m) > 0 {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			// --set syntax: dots inside a key are escaped.
			path := strings.ReplaceAll(k, ".", `\.`)
			if prefix != "" {
				path = prefix + "." + path
			}
			rows = flattenValues(path, m[k], rows)
		}
		return rows
	}
	if prefix == "" {
		return rows
	}
	data, err := json.Marshal(v)
	def := string(data)
	if err != nil {
		def = fmt.Sprint(v)
	}
	if len(def) > 60 {
		def = def[:57] + "..."
	}
	def = strings.NewReplacer("|", "\\|", "`", "'").Replace(def)
	return append(rows, [2]string{prefix, def})
}
