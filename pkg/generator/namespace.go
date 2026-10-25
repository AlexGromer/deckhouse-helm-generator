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

// Fallback quota for a group without pod-creating workloads (e.g. only
// ConfigMaps and Services): nothing in the input says what the namespace
// needs, but the template is still emitted so that the
// namespace.resourceQuota toggle behaves the same for every group. These
// values are not derived from the input; the template says so in a comment.
const (
	quotaFallbackCPURequest    = "1"
	quotaFallbackMemoryRequest = "1Gi"
	quotaFallbackCPULimit      = "2"
	quotaFallbackMemoryLimit   = "2Gi"
)

// GenerateResourceQuotaTemplate generates a ResourceQuota template sized for
// all workloads of the group: requests.* and limits.* are the sum, over the
// workloads, of the effective pod requests/limits (app containers summed,
// init containers by the Kubernetes max rule, unset values filled with the
// group's LimitRange defaults) times the peak number of pods (replicas or
// HPA maxReplicas, plus a Deployment's rollout surge). See quota.go.
//
// A ResourceQuota without scopes counts every pod of the namespace; when
// several groups (or releases) share a namespace, each quota must be raised
// to cover all of them. The template states this in a comment.
func GenerateResourceQuotaTemplate(chartName string, group *ServiceGroup) string {
	cpuReq, memReq := quotaFallbackCPURequest, quotaFallbackMemoryRequest
	cpuLim, memLim := quotaFallbackCPULimit, quotaFallbackMemoryLimit
	totals, ok := groupQuotaTotals(group)
	if ok {
		cpuReq, memReq = qString(totals.requests["cpu"]), qString(totals.requests["memory"])
		cpuLim, memLim = qString(totals.limits["cpu"]), qString(totals.limits["memory"])
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
	// The explanation goes inside the document: features only recognise
	// templates whose lines before apiVersion are template actions.
	if ok {
		sb.WriteString("  # Sized by dhg for the workloads of this group (peak pods x effective pod resources):\n")
		daemonSet := false
		for _, note := range totals.notes {
			sb.WriteString("  #   " + note + "\n")
			daemonSet = daemonSet || strings.HasPrefix(note, "DaemonSet ")
		}
		sb.WriteString("  # The quota counts every pod of the namespace: raise it when other workloads share the namespace.\n")
		if daemonSet {
			sb.WriteString("  # DaemonSets are counted for one node: multiply their share by the number of nodes.\n")
		}
	} else {
		sb.WriteString("  # No workload in this group: placeholder values, not derived from the input.\n")
	}
	sb.WriteString("  hard:\n")
	fmt.Fprintf(&sb, "    requests.cpu: \"%s\"\n", cpuReq)
	fmt.Fprintf(&sb, "    requests.memory: \"%s\"\n", memReq)
	fmt.Fprintf(&sb, "    limits.cpu: \"%s\"\n", cpuLim)
	fmt.Fprintf(&sb, "    limits.memory: \"%s\"\n", memLim)
	sb.WriteString("{{- end }}\n")

	return sb.String()
}

// GenerateLimitRangeTemplate generates a LimitRange template whose
// per-container defaults (applied to containers that declare no requests or
// limits) are the largest values declared by the group's containers; see
// groupLimitDefaults for why the maximum is used.
func GenerateLimitRangeTemplate(chartName string, group *ServiceGroup) string {
	d := groupLimitDefaults(groupWorkloads(group))
	cpuReq, memReq := qString(d.request["cpu"]), qString(d.request["memory"])
	cpuLim, memLim := qString(d.limit["cpu"]), qString(d.limit["memory"])

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
