package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
	"sigs.k8s.io/yaml"
)

// GenerateMultiTenantOverlay adds multi-tenancy support to a generated chart.
// It adds tenant loop templates and per-tenant isolation resources.
// If tenantCount is 0, returns the chart unchanged.
func GenerateMultiTenantOverlay(chart *types.GeneratedChart, tenantCount int) *types.GeneratedChart {
	if tenantCount <= 0 {
		return chart
	}

	// Build tenant entries for values.yaml
	tenants := make([]interface{}, 0, tenantCount)
	for i := 1; i <= tenantCount; i++ {
		tenants = append(tenants, map[string]interface{}{
			"name":      fmt.Sprintf("tenant-%d", i),
			"namespace": fmt.Sprintf("%s-tenant-%d", chart.Name, i),
			"resources": map[string]interface{}{
				"cpu":    "1",
				"memory": "2Gi",
			},
		})
	}

	// Parse existing values and add tenants section
	var existingValues map[string]interface{}
	if err := yaml.Unmarshal([]byte(chart.ValuesYAML), &existingValues); err != nil {
		existingValues = make(map[string]interface{})
	}
	existingValues["tenants"] = tenants

	newValuesBytes, err := yaml.Marshal(existingValues)
	if err != nil {
		return chart
	}

	// Copy existing templates
	templates := make(map[string]string, len(chart.Templates)+4)
	for k, v := range chart.Templates {
		templates[k] = v
	}

	// Add tenant templates
	templates["templates/tenant-namespaces.yaml"] = generateTenantNamespaceTemplate(chart.Name)
	templates["templates/tenant-resourcequotas.yaml"] = generateTenantResourceQuotaTemplate(chart.Name)
	templates["templates/tenant-limitranges.yaml"] = generateTenantLimitRangeTemplate(chart.Name)
	templates["templates/tenant-networkpolicies.yaml"] = generateTenantNetworkPolicyTemplate(chart.Name)

	return &types.GeneratedChart{
		Name:          chart.Name,
		Path:          chart.Path,
		ChartYAML:     chart.ChartYAML,
		ValuesYAML:    string(newValuesBytes),
		Templates:     templates,
		Helpers:       chart.Helpers,
		Notes:         chart.Notes,
		ValuesSchema:  chart.ValuesSchema,
		ExternalFiles: chart.ExternalFiles,
	}
}

func generateTenantNamespaceTemplate(chartName string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("{{- /* Tenant Namespaces for %s */ -}}\n", chartName))
	sb.WriteString("{{- range .Values.tenants }}\n")
	sb.WriteString("---\n")
	sb.WriteString("apiVersion: v1\n")
	sb.WriteString("kind: Namespace\n")
	sb.WriteString("metadata:\n")
	sb.WriteString("  name: {{ .namespace }}\n")
	sb.WriteString("  labels:\n")
	sb.WriteString("    tenant: {{ .name }}\n")
	sb.WriteString(fmt.Sprintf("    app.kubernetes.io/managed-by: %s\n", chartName))
	sb.WriteString("{{- end }}\n")
	return sb.String()
}

func generateTenantResourceQuotaTemplate(chartName string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("{{- /* Tenant ResourceQuotas for %s */ -}}\n", chartName))
	sb.WriteString("{{- range .Values.tenants }}\n")
	sb.WriteString("---\n")
	sb.WriteString("apiVersion: v1\n")
	sb.WriteString("kind: ResourceQuota\n")
	sb.WriteString("metadata:\n")
	sb.WriteString("  name: {{ .name }}-quota\n")
	sb.WriteString("  namespace: {{ .namespace }}\n")
	sb.WriteString("spec:\n")
	sb.WriteString("  hard:\n")
	sb.WriteString("    requests.cpu: {{ .resources.cpu }}\n")
	sb.WriteString("    requests.memory: {{ .resources.memory }}\n")
	sb.WriteString("    limits.cpu: {{ .resources.cpu }}\n")
	sb.WriteString("    limits.memory: {{ .resources.memory }}\n")
	sb.WriteString("{{- end }}\n")
	return sb.String()
}

func generateTenantLimitRangeTemplate(chartName string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("{{- /* Tenant LimitRanges for %s */ -}}\n", chartName))
	sb.WriteString("{{- range .Values.tenants }}\n")
	sb.WriteString("---\n")
	sb.WriteString("apiVersion: v1\n")
	sb.WriteString("kind: LimitRange\n")
	sb.WriteString("metadata:\n")
	sb.WriteString("  name: {{ .name }}-limits\n")
	sb.WriteString("  namespace: {{ .namespace }}\n")
	sb.WriteString("spec:\n")
	sb.WriteString("  limits:\n")
	sb.WriteString("    - type: Container\n")
	sb.WriteString("      default:\n")
	sb.WriteString("        cpu: {{ .resources.cpu }}\n")
	sb.WriteString("        memory: {{ .resources.memory }}\n")
	sb.WriteString("      defaultRequest:\n")
	sb.WriteString("        cpu: \"100m\"\n")
	sb.WriteString("        memory: \"128Mi\"\n")
	sb.WriteString("{{- end }}\n")
	return sb.String()
}

func generateTenantNetworkPolicyTemplate(chartName string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("{{- /* Tenant NetworkPolicies for %s — deny cross-tenant traffic */ -}}\n", chartName))
	sb.WriteString("{{- range .Values.tenants }}\n")
	sb.WriteString("---\n")
	sb.WriteString("apiVersion: networking.k8s.io/v1\n")
	sb.WriteString("kind: NetworkPolicy\n")
	sb.WriteString("metadata:\n")
	sb.WriteString("  name: {{ .name }}-isolation\n")
	sb.WriteString("  namespace: {{ .namespace }}\n")
	sb.WriteString("spec:\n")
	sb.WriteString("  podSelector: {}\n")
	sb.WriteString("  policyTypes:\n")
	sb.WriteString("    - Ingress\n")
	sb.WriteString("    - Egress\n")
	sb.WriteString("  ingress:\n")
	sb.WriteString("    - from:\n")
	sb.WriteString("        - namespaceSelector:\n")
	sb.WriteString("            matchLabels:\n")
	sb.WriteString("              tenant: {{ .name }}\n")
	sb.WriteString("  egress:\n")
	sb.WriteString("    - to:\n")
	sb.WriteString("        - namespaceSelector:\n")
	sb.WriteString("            matchLabels:\n")
	sb.WriteString("              tenant: {{ .name }}\n")
	sb.WriteString("    - to:\n")
	sb.WriteString("        - namespaceSelector:\n")
	sb.WriteString("            matchLabels:\n")
	sb.WriteString("              kubernetes.io/metadata.name: kube-system\n")
	sb.WriteString("      ports:\n")
	sb.WriteString("        - port: 53\n")
	sb.WriteString("          protocol: UDP\n")
	sb.WriteString("{{- end }}\n")
	return sb.String()
}
