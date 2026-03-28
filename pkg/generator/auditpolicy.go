package generator

import "strings"

// GenerateAuditPolicy returns a Kubernetes Audit Policy YAML manifest.
// NOTE: This is a reference manifest only. The cluster administrator must
// apply it manually to the kube-apiserver configuration.
func GenerateAuditPolicy() string {
	var b strings.Builder

	b.WriteString("# Kubernetes Audit Policy\n")
	b.WriteString("# Reference manifest — cluster admin must apply manually\n")
	b.WriteString("# via kube-apiserver --audit-policy-file flag.\n")
	b.WriteString("apiVersion: audit.k8s.io/v1\n")
	b.WriteString("kind: Policy\n")
	b.WriteString("rules:\n")

	// Rule: full request+response logging for Secret access
	b.WriteString("  # Log full request and response for Secret access\n")
	b.WriteString("  - level: RequestResponse\n")
	b.WriteString("    resources:\n")
	b.WriteString("      - group: \"\"\n")
	b.WriteString("        resources: [\"secrets\"]\n")

	// Rule: request logging for RBAC changes
	b.WriteString("  # Log requests for RBAC changes\n")
	b.WriteString("  - level: Request\n")
	b.WriteString("    resources:\n")
	b.WriteString("      - group: rbac.authorization.k8s.io\n")
	b.WriteString("        resources: [\"roles\", \"rolebindings\", \"clusterroles\", \"clusterrolebindings\"]\n")

	// Rule: request logging for pod exec/attach
	b.WriteString("  # Log requests for pod exec and attach\n")
	b.WriteString("  - level: Request\n")
	b.WriteString("    resources:\n")
	b.WriteString("      - group: \"\"\n")
	b.WriteString("        resources: [\"pods/exec\", \"pods/attach\"]\n")

	// Default: metadata level for everything else
	b.WriteString("  # Default: metadata level for all other requests\n")
	b.WriteString("  - level: Metadata\n")

	return b.String()
}
