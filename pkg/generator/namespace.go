package generator

import (
	"fmt"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// NamespaceOpts configures namespace resource generation.
type NamespaceOpts struct {
	ResourceQuota bool
	LimitRange    bool
	NetworkPolicy bool
}

// GenerateNamespaceResources generates namespace-level governance templates.
// Returns map of template path -> template content.
// chartName is the name of the chart the templates are added to (helper prefix).
func GenerateNamespaceResources(chartName string, groups []*ServiceGroup, opts NamespaceOpts) map[string]string {
	if len(groups) == 0 {
		return make(map[string]string)
	}

	result := make(map[string]string)

	for _, group := range groups {
		if group == nil {
			continue
		}
		if opts.ResourceQuota {
			path := fmt.Sprintf("templates/%s-resourcequota.yaml", group.Name)
			result[path] = GenerateResourceQuotaTemplate(chartName, group)
		}
		if opts.LimitRange {
			path := fmt.Sprintf("templates/%s-limitrange.yaml", group.Name)
			result[path] = GenerateLimitRangeTemplate(chartName, group)
		}
		if opts.NetworkPolicy {
			path := fmt.Sprintf("templates/%s-networkpolicy-default.yaml", group.Name)
			result[path] = GenerateNetworkPolicyTemplate(chartName, group)
		}
	}

	return result
}

// GenerateResourceQuotaTemplate generates a ResourceQuota template from aggregated resources.
func GenerateResourceQuotaTemplate(chartName string, group *ServiceGroup) string {
	cpuReq, memReq, cpuLim, memLim := extractFirstResourceValues(group)

	if cpuReq == "" {
		cpuReq = "1"
	}
	if memReq == "" {
		memReq = "1Gi"
	}
	if cpuLim == "" {
		cpuLim = "2"
	}
	if memLim == "" {
		memLim = "2Gi"
	}

	var sb strings.Builder
	sb.WriteString("{{- if .Values.namespace.resourceQuota.enabled }}\n")
	sb.WriteString("apiVersion: v1\n")
	sb.WriteString("kind: ResourceQuota\n")
	sb.WriteString("metadata:\n")
	fmt.Fprintf(&sb, "  name: {{ include \"%s.fullname\" . }}-%s-quota\n", chartName, processor.ResourceNameSuffix(group.Name))
	sb.WriteString("  namespace: {{ .Release.Namespace }}\n")
	sb.WriteString("  labels:\n")
	fmt.Fprintf(&sb, "    {{- include \"%s.labels\" . | nindent 4 }}\n", chartName)
	sb.WriteString("spec:\n")
	sb.WriteString("  hard:\n")
	fmt.Fprintf(&sb, "    requests.cpu: \"%s\"\n", cpuReq)
	fmt.Fprintf(&sb, "    requests.memory: \"%s\"\n", memReq)
	fmt.Fprintf(&sb, "    limits.cpu: \"%s\"\n", cpuLim)
	fmt.Fprintf(&sb, "    limits.memory: \"%s\"\n", memLim)
	sb.WriteString("{{- end }}\n")

	return sb.String()
}

// GenerateLimitRangeTemplate generates a LimitRange template with defaults from workload analysis.
func GenerateLimitRangeTemplate(chartName string, group *ServiceGroup) string {
	cpuReq, memReq, cpuLim, memLim := extractFirstResourceValues(group)

	if cpuReq == "" {
		cpuReq = "100m"
	}
	if memReq == "" {
		memReq = "128Mi"
	}
	if cpuLim == "" {
		cpuLim = "500m"
	}
	if memLim == "" {
		memLim = "512Mi"
	}

	var sb strings.Builder
	sb.WriteString("{{- if .Values.namespace.limitRange.enabled }}\n")
	sb.WriteString("apiVersion: v1\n")
	sb.WriteString("kind: LimitRange\n")
	sb.WriteString("metadata:\n")
	fmt.Fprintf(&sb, "  name: {{ include \"%s.fullname\" . }}-%s-limits\n", chartName, processor.ResourceNameSuffix(group.Name))
	sb.WriteString("  namespace: {{ .Release.Namespace }}\n")
	sb.WriteString("  labels:\n")
	fmt.Fprintf(&sb, "    {{- include \"%s.labels\" . | nindent 4 }}\n", chartName)
	sb.WriteString("spec:\n")
	sb.WriteString("  limits:\n")
	sb.WriteString("    - type: Container\n")
	sb.WriteString("      default:\n")
	fmt.Fprintf(&sb, "        cpu: \"%s\"\n", cpuLim)
	fmt.Fprintf(&sb, "        memory: \"%s\"\n", memLim)
	sb.WriteString("      defaultRequest:\n")
	fmt.Fprintf(&sb, "        cpu: \"%s\"\n", cpuReq)
	fmt.Fprintf(&sb, "        memory: \"%s\"\n", memReq)
	sb.WriteString("{{- end }}\n")

	return sb.String()
}

// GenerateNetworkPolicyTemplate generates a default deny-all + allow same-namespace NetworkPolicy.
func GenerateNetworkPolicyTemplate(chartName string, group *ServiceGroup) string {
	var sb strings.Builder

	sb.WriteString("{{- if .Values.namespace.networkPolicy.enabled }}\n")
	sb.WriteString("apiVersion: networking.k8s.io/v1\n")
	sb.WriteString("kind: NetworkPolicy\n")
	sb.WriteString("metadata:\n")
	fmt.Fprintf(&sb, "  name: {{ include \"%s.fullname\" . }}-%s-default\n", chartName, processor.ResourceNameSuffix(group.Name))
	sb.WriteString("  namespace: {{ .Release.Namespace }}\n")
	sb.WriteString("spec:\n")
	sb.WriteString("  podSelector: {}\n")
	sb.WriteString("  policyTypes:\n")
	sb.WriteString("    - Ingress\n")
	sb.WriteString("    - Egress\n")
	sb.WriteString("  ingress:\n")
	sb.WriteString("    - from:\n")
	sb.WriteString("        - namespaceSelector:\n")
	sb.WriteString("            matchLabels:\n")
	sb.WriteString("              kubernetes.io/metadata.name: {{ .Release.Namespace }}\n")
	sb.WriteString("  egress:\n")
	sb.WriteString("    # Allow DNS resolution\n")
	sb.WriteString("    - to:\n")
	sb.WriteString("        - namespaceSelector:\n")
	sb.WriteString("            matchLabels:\n")
	sb.WriteString("              kubernetes.io/metadata.name: kube-system\n")
	sb.WriteString("      ports:\n")
	sb.WriteString("        - port: 53\n")
	sb.WriteString("          protocol: UDP\n")
	sb.WriteString("        - port: 53\n")
	sb.WriteString("          protocol: TCP\n")
	sb.WriteString("    # Allow same-namespace egress\n")
	sb.WriteString("    - to:\n")
	sb.WriteString("        - namespaceSelector:\n")
	sb.WriteString("            matchLabels:\n")
	sb.WriteString("              kubernetes.io/metadata.name: {{ .Release.Namespace }}\n")
	sb.WriteString("{{- end }}\n")

	return sb.String()
}

// TODO(HC-4): Implement proper aggregation using k8s.io/apimachinery/pkg/api/resource.Quantity

// extractFirstResourceValues returns the first non-empty resource values found in the group's workloads.
// For accurate quota calculation with multiple workloads, implement proper aggregation.
func extractFirstResourceValues(group *ServiceGroup) (cpuReq, memReq, cpuLim, memLim string) {
	for _, r := range group.Resources {
		resources, ok := r.Values["resources"].(map[string]interface{})
		if !ok {
			continue
		}
		if req, ok := resources["requests"].(map[string]interface{}); ok {
			if v, ok := req["cpu"].(string); ok && cpuReq == "" {
				cpuReq = v
			}
			if v, ok := req["memory"].(string); ok && memReq == "" {
				memReq = v
			}
		}
		if lim, ok := resources["limits"].(map[string]interface{}); ok {
			if v, ok := lim["cpu"].(string); ok && cpuLim == "" {
				cpuLim = v
			}
			if v, ok := lim["memory"].(string); ok && memLim == "" {
				memLim = v
			}
		}
	}
	return
}

// ApplyNamespaceResources adds namespace governance templates (ResourceQuota,
// LimitRange, default NetworkPolicy) and relationship-derived NetworkPolicies
// to a chart, together with the namespace.* values that toggle them.
//
// A chart generated for one service group (separate/library/umbrella modes)
// gets the resources of that group only; a universal chart gets all groups.
// Umbrella parent charts are left unchanged (their subcharts carry the
// resources).
func ApplyNamespaceResources(chart *types.GeneratedChart, graph *types.ResourceGraph, groups []*ServiceGroup, opts NamespaceOpts) (*types.GeneratedChart, error) {
	if len(chart.Templates) == 0 && strings.Contains(chart.ChartYAML, "\ndependencies:") {
		return chart, nil
	}
	name := chartNameOf(chart)
	selected := groups
	for _, g := range groups {
		if g != nil && g.Name == name {
			selected = []*ServiceGroup{g}
			break
		}
	}

	out := cloneChart(chart)
	nsTemplates := GenerateNamespaceResources(name, selected, opts)
	var autoNP map[string]string
	if opts.NetworkPolicy {
		autoNP = GenerateAutoNetworkPolicies(name, graph, selected)
	}
	for path, content := range nsTemplates {
		// A group with a fine-grained policy does not also get the broad
		// default policy, so the two never conflict.
		if strings.HasSuffix(path, "-networkpolicy-default.yaml") {
			fine := strings.TrimSuffix(path, "-default.yaml") + ".yaml"
			if _, ok := autoNP[fine]; ok {
				continue
			}
		}
		out.Templates[path] = content
	}
	for path, content := range autoNP {
		out.Templates[path] = content
	}

	values, err := appendTopLevelValues(out.ValuesYAML, "namespace", map[string]interface{}{
		"resourceQuota": map[string]interface{}{"enabled": opts.ResourceQuota},
		"limitRange":    map[string]interface{}{"enabled": opts.LimitRange},
		"networkPolicy": map[string]interface{}{"enabled": opts.NetworkPolicy},
	})
	if err != nil {
		return nil, err
	}
	out.ValuesYAML = values
	return out, nil
}

// chartNameOf returns the name declared in Chart.yaml (chart.Name may be a
// path such as "parent/charts/sub" for umbrella subcharts).
func chartNameOf(chart *types.GeneratedChart) string {
	for _, line := range strings.Split(chart.ChartYAML, "\n") {
		if strings.HasPrefix(line, "name:") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "name:")), `"'`)
		}
	}
	return chart.Name
}
