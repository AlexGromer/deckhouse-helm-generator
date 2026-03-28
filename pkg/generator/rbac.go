package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// GenerateRBACTemplates creates least-privilege RBAC templates (ServiceAccount, Role, RoleBinding)
// for each chart that contains Deployment or StatefulSet workloads.
// Returns map of template path to template content.
func GenerateRBACTemplates(chart *types.GeneratedChart) map[string]string {
	if !chartHasWorkloads(chart) {
		return make(map[string]string)
	}

	result := make(map[string]string)
	name := chart.Name

	result[fmt.Sprintf("templates/%s-serviceaccount.yaml", name)] = generateServiceAccount(name)
	result[fmt.Sprintf("templates/%s-role.yaml", name)] = generateRole(name)
	result[fmt.Sprintf("templates/%s-rolebinding.yaml", name)] = generateRoleBinding(name)

	return result
}

// chartHasWorkloads checks whether the chart contains Deployment or StatefulSet templates.
func chartHasWorkloads(chart *types.GeneratedChart) bool {
	for _, content := range chart.Templates {
		if strings.Contains(content, "kind: Deployment") || strings.Contains(content, "kind: StatefulSet") {
			return true
		}
	}
	return false
}

// generateServiceAccount builds a ServiceAccount template with automountServiceAccountToken: false.
func generateServiceAccount(name string) string {
	var sb strings.Builder
	sb.WriteString("{{- if .Values.rbac.create }}\n")
	sb.WriteString("apiVersion: v1\n")
	sb.WriteString("kind: ServiceAccount\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: {{ include \"%s.fullname\" . }}-sa\n", name))
	sb.WriteString("  namespace: {{ .Release.Namespace }}\n")
	sb.WriteString("  labels:\n")
	sb.WriteString(fmt.Sprintf("    {{- include \"%s.labels\" . | nindent 4 }}\n", name))
	sb.WriteString("automountServiceAccountToken: false\n")
	sb.WriteString("{{- end }}\n")
	return sb.String()
}

// generateRole builds a Role template with least-privilege permissions (get, list, watch).
func generateRole(name string) string {
	var sb strings.Builder
	sb.WriteString("{{- if .Values.rbac.create }}\n")
	sb.WriteString("apiVersion: rbac.authorization.k8s.io/v1\n")
	sb.WriteString("kind: Role\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: {{ include \"%s.fullname\" . }}-role\n", name))
	sb.WriteString("  namespace: {{ .Release.Namespace }}\n")
	sb.WriteString("  labels:\n")
	sb.WriteString(fmt.Sprintf("    {{- include \"%s.labels\" . | nindent 4 }}\n", name))
	sb.WriteString("rules:\n")
	sb.WriteString("  - apiGroups: [\"\"]\n")
	sb.WriteString("    resources: [\"configmaps\", \"secrets\", \"pods\"]\n")
	sb.WriteString("    verbs: [\"get\", \"list\", \"watch\"]\n")
	sb.WriteString("{{- end }}\n")
	return sb.String()
}

// generateRoleBinding builds a RoleBinding template linking the ServiceAccount to the Role.
func generateRoleBinding(name string) string {
	var sb strings.Builder
	sb.WriteString("{{- if .Values.rbac.create }}\n")
	sb.WriteString("apiVersion: rbac.authorization.k8s.io/v1\n")
	sb.WriteString("kind: RoleBinding\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: {{ include \"%s.fullname\" . }}-rolebinding\n", name))
	sb.WriteString("  namespace: {{ .Release.Namespace }}\n")
	sb.WriteString("  labels:\n")
	sb.WriteString(fmt.Sprintf("    {{- include \"%s.labels\" . | nindent 4 }}\n", name))
	sb.WriteString("roleRef:\n")
	sb.WriteString("  apiGroup: rbac.authorization.k8s.io\n")
	sb.WriteString("  kind: Role\n")
	sb.WriteString(fmt.Sprintf("  name: {{ include \"%s.fullname\" . }}-role\n", name))
	sb.WriteString("subjects:\n")
	sb.WriteString("  - kind: ServiceAccount\n")
	sb.WriteString(fmt.Sprintf("    name: {{ include \"%s.fullname\" . }}-sa\n", name))
	sb.WriteString("    namespace: {{ .Release.Namespace }}\n")
	sb.WriteString("{{- end }}\n")
	return sb.String()
}
