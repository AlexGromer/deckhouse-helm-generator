package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// LinkerdOptions configures Linkerd service mesh integration.
type LinkerdOptions struct {
	InjectAnnotation bool
	ServiceProfiles  bool
	TrafficSplit     bool
	DefaultTimeout   string
	DefaultRetries   int
}

// LinkerdResult holds generated Linkerd templates.
type LinkerdResult struct {
	// Templates maps filename → YAML content.
	Templates map[string]string
	NOTESTxt  string
}

// GenerateLinkerdConfig generates Linkerd ServiceProfile and TrafficSplit templates.
func GenerateLinkerdConfig(graph *types.ResourceGraph, opts LinkerdOptions) *LinkerdResult {
	result := &LinkerdResult{
		Templates: make(map[string]string),
	}

	if graph == nil || len(graph.Resources) == 0 {
		result.NOTESTxt = buildLinkerdNOTESTxt(result)
		return result
	}

	if opts.ServiceProfiles {
		for _, r := range graph.Resources {
			if r.Original.GVK.Kind != "Service" {
				continue
			}
			name := r.Original.Object.GetName()
			ns := r.Original.Object.GetNamespace()
			yaml := generateServiceProfileYAML(name, ns, opts)
			result.Templates[fmt.Sprintf("templates/linkerd-sp-%s.yaml", name)] = yaml
		}
	}

	if opts.TrafficSplit {
		seen := make(map[string]bool)
		for _, r := range graph.Resources {
			kind := r.Original.GVK.Kind
			if kind != "Deployment" && kind != "Service" {
				continue
			}
			name := r.Original.Object.GetName()
			ns := r.Original.Object.GetNamespace()
			if seen[name] {
				// Generate TrafficSplit for workloads with duplicates.
				yaml := generateTrafficSplitYAML(name, ns)
				result.Templates[fmt.Sprintf("templates/linkerd-ts-%s.yaml", name)] = yaml
			}
			seen[name] = true
		}
		// If TrafficSplit enabled and we have any services, generate for them.
		if len(result.Templates) == 0 {
			for _, r := range graph.Resources {
				if r.Original.GVK.Kind != "Service" && r.Original.GVK.Kind != "Deployment" {
					continue
				}
				name := r.Original.Object.GetName()
				ns := r.Original.Object.GetNamespace()
				yaml := generateTrafficSplitYAML(name, ns)
				result.Templates[fmt.Sprintf("templates/linkerd-ts-%s.yaml", name)] = yaml
				break // one is enough for TrafficSplit
			}
		}
	}

	result.NOTESTxt = buildLinkerdNOTESTxt(result)
	return result
}

func generateServiceProfileYAML(name, namespace string, opts LinkerdOptions) string {
	var sb strings.Builder
	sb.WriteString("apiVersion: linkerd.io/v1alpha2\n")
	sb.WriteString("kind: ServiceProfile\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s.%s.svc.cluster.local\n", name, namespace))
	sb.WriteString(fmt.Sprintf("  namespace: %s\n", namespace))
	sb.WriteString("spec:\n")
	sb.WriteString("  routes: []\n")
	if opts.DefaultTimeout != "" {
		sb.WriteString(fmt.Sprintf("  # timeout: %s\n", opts.DefaultTimeout))
	}
	if opts.DefaultRetries > 0 {
		sb.WriteString("  # retryBudget:\n")
		sb.WriteString(fmt.Sprintf("  #   retryRatio: %.1f\n", float64(opts.DefaultRetries)/10.0))
		sb.WriteString(fmt.Sprintf("  #   minRetriesPerSecond: %d\n", opts.DefaultRetries))
	}
	return sb.String()
}

func generateTrafficSplitYAML(name, namespace string) string {
	var sb strings.Builder
	sb.WriteString("apiVersion: split.smi-spec.io/v1alpha1\n")
	sb.WriteString("kind: TrafficSplit\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s-split\n", name))
	sb.WriteString(fmt.Sprintf("  namespace: %s\n", namespace))
	sb.WriteString("spec:\n")
	sb.WriteString(fmt.Sprintf("  service: %s\n", name))
	sb.WriteString("  backends:\n")
	sb.WriteString(fmt.Sprintf("    - service: %s-stable\n", name))
	sb.WriteString("      weight: 90\n")
	sb.WriteString(fmt.Sprintf("    - service: %s-canary\n", name))
	sb.WriteString("      weight: 10\n")
	return sb.String()
}

func buildLinkerdNOTESTxt(result *LinkerdResult) string {
	return fmt.Sprintf(
		"Linkerd service mesh configuration generated. %d templates created. "+
			"To use linkerd, install Linkerd CLI and run 'linkerd install | kubectl apply -f -'. "+
			"Enable injection via 'linkerd.io/inject: enabled' annotation.",
		len(result.Templates),
	)
}

// InjectLinkerdAnnotations injects Linkerd injection annotations into workload templates.
// Returns (nil, 0) for nil chart.
func InjectLinkerdAnnotations(chart *types.GeneratedChart, opts LinkerdOptions) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}

	newChart := copyChartTemplates(chart)
	count := 0

	if !opts.InjectAnnotation {
		return newChart, 0
	}

	for path, content := range newChart.Templates {
		if !isWorkloadTemplate(content) {
			continue
		}
		if strings.Contains(content, "linkerd.io/inject") {
			continue
		}
		updated := injectLinkerdAnnotation(content)
		if updated != content {
			newChart.Templates[path] = updated
			count++
		}
	}

	return newChart, count
}

func injectLinkerdAnnotation(content string) string {
	annotation := "        linkerd.io/inject: enabled\n"

	if strings.Contains(content, "      annotations: {}") {
		return strings.Replace(content,
			"      annotations: {}",
			"      annotations:\n"+annotation,
			1)
	}
	if strings.Contains(content, "      annotations:") {
		return strings.Replace(content,
			"      annotations:",
			"      annotations:\n"+annotation,
			1)
	}
	// No annotations section — inject after "    spec:" (pod spec, inside template)
	// by inserting a metadata.annotations block after template.spec marker.
	if strings.Contains(content, "  template:") {
		return strings.Replace(content,
			"  template:",
			"  template:\n    metadata:\n      annotations:\n"+annotation,
			1)
	}
	return content
}
