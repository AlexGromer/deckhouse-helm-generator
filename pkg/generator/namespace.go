package generator

import (
	"fmt"
	"strings"
)

// NamespaceOpts configures namespace resource generation.
type NamespaceOpts struct {
	ResourceQuota bool
	LimitRange    bool
	NetworkPolicy bool
}

// GenerateNamespaceResources generates namespace-level governance templates.
// Returns map of template path → template content.
func GenerateNamespaceResources(groups []*ServiceGroup, opts NamespaceOpts) map[string]string {
	if len(groups) == 0 {
		return make(map[string]string)
	}

	result := make(map[string]string)

	for _, group := range groups {
		if opts.ResourceQuota {
			path := fmt.Sprintf("templates/%s-resourcequota.yaml", group.Name)
			result[path] = GenerateResourceQuotaTemplate(group)
		}
		if opts.LimitRange {
			path := fmt.Sprintf("templates/%s-limitrange.yaml", group.Name)
			result[path] = GenerateLimitRangeTemplate(group)
		}
		if opts.NetworkPolicy {
			path := fmt.Sprintf("templates/%s-networkpolicy-default.yaml", group.Name)
			result[path] = GenerateNetworkPolicyTemplate(group)
		}
	}

	return result
}

// GenerateResourceQuotaTemplate generates a ResourceQuota template from aggregated resources.
func GenerateResourceQuotaTemplate(group *ServiceGroup) string {
	cpuReq, memReq, cpuLim, memLim := aggregateResources(group)

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
	sb.WriteString(fmt.Sprintf("  name: {{ include \"%s.fullname\" . }}-%s-quota\n", group.Name, group.Name))
	sb.WriteString("  namespace: {{ .Release.Namespace }}\n")
	sb.WriteString("  labels:\n")
	sb.WriteString(fmt.Sprintf("    {{- include \"%s.labels\" . | nindent 4 }}\n", group.Name))
	sb.WriteString("spec:\n")
	sb.WriteString("  hard:\n")
	sb.WriteString(fmt.Sprintf("    requests.cpu: \"%s\"\n", cpuReq))
	sb.WriteString(fmt.Sprintf("    requests.memory: \"%s\"\n", memReq))
	sb.WriteString(fmt.Sprintf("    limits.cpu: \"%s\"\n", cpuLim))
	sb.WriteString(fmt.Sprintf("    limits.memory: \"%s\"\n", memLim))
	sb.WriteString("{{- end }}\n")

	return sb.String()
}

// GenerateLimitRangeTemplate generates a LimitRange template with defaults from workload analysis.
func GenerateLimitRangeTemplate(group *ServiceGroup) string {
	cpuReq, memReq, cpuLim, memLim := aggregateResources(group)

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
	sb.WriteString(fmt.Sprintf("  name: {{ include \"%s.fullname\" . }}-%s-limits\n", group.Name, group.Name))
	sb.WriteString("  namespace: {{ .Release.Namespace }}\n")
	sb.WriteString("spec:\n")
	sb.WriteString("  limits:\n")
	sb.WriteString("    - type: Container\n")
	sb.WriteString("      default:\n")
	sb.WriteString(fmt.Sprintf("        cpu: \"%s\"\n", cpuLim))
	sb.WriteString(fmt.Sprintf("        memory: \"%s\"\n", memLim))
	sb.WriteString("      defaultRequest:\n")
	sb.WriteString(fmt.Sprintf("        cpu: \"%s\"\n", cpuReq))
	sb.WriteString(fmt.Sprintf("        memory: \"%s\"\n", memReq))
	sb.WriteString("{{- end }}\n")

	return sb.String()
}

// GenerateNetworkPolicyTemplate generates a default deny-all + allow same-namespace NetworkPolicy.
func GenerateNetworkPolicyTemplate(group *ServiceGroup) string {
	var sb strings.Builder

	sb.WriteString("{{- if .Values.namespace.networkPolicy.enabled }}\n")
	sb.WriteString("apiVersion: networking.k8s.io/v1\n")
	sb.WriteString("kind: NetworkPolicy\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: {{ include \"%s.fullname\" . }}-%s-default\n", group.Name, group.Name))
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

// aggregateResources extracts CPU/memory requests/limits from all workloads in a group.
func aggregateResources(group *ServiceGroup) (cpuReq, memReq, cpuLim, memLim string) {
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
