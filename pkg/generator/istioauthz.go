package generator

import (
	"fmt"
	"strings"
)

// AuthzPolicyOptions configures Istio AuthorizationPolicy generation.
type AuthzPolicyOptions struct {
	// Namespace is the Kubernetes namespace for the policy.
	Namespace string
	// Rules is the list of authorization rules.
	Rules []AuthzRule
	// DenyByDefault generates an additional catch-all DENY policy when true.
	DenyByDefault bool
}

// AuthzRule describes a single authorization rule.
type AuthzRule struct {
	// Paths lists HTTP request paths to match (e.g. "/api/*").
	Paths []string
	// Methods lists HTTP methods to match (e.g. "GET", "POST").
	Methods []string
	// SourcePrincipals lists SPIFFE principals allowed as source.
	SourcePrincipals []string
	// Action is either "ALLOW" or "DENY".
	Action string
}

// GenerateIstioAuthzPolicy generates AuthorizationPolicy templates.
// Returns nil if Namespace is empty.
func GenerateIstioAuthzPolicy(opts AuthzPolicyOptions) map[string]string {
	if opts.Namespace == "" {
		return nil
	}

	templates := make(map[string]string)

	// Deny-by-default policy
	if opts.DenyByDefault {
		yaml := buildDenyAllAuthzPolicyYAML(opts.Namespace)
		key := fmt.Sprintf("templates/istio-authz-deny-all-%s.yaml", opts.Namespace)
		templates[key] = yaml
	}

	// Per-rule policies
	for i, rule := range opts.Rules {
		yaml := buildAuthzRulePolicyYAML(opts.Namespace, rule, i)
		key := fmt.Sprintf("templates/istio-authz-rule-%s-%d.yaml", opts.Namespace, i)
		templates[key] = yaml
	}

	return templates
}

// buildDenyAllAuthzPolicyYAML builds a namespace-wide DENY-all AuthorizationPolicy.
func buildDenyAllAuthzPolicyYAML(namespace string) string {
	return fmt.Sprintf(`apiVersion: security.istio.io/v1beta1
kind: AuthorizationPolicy
metadata:
  name: deny-all
  namespace: %s
spec:
  action: DENY
`, namespace)
}

// buildAuthzRulePolicyYAML builds an AuthorizationPolicy for a single rule.
func buildAuthzRulePolicyYAML(namespace string, rule AuthzRule, idx int) string {
	action := rule.Action
	if action == "" {
		action = "ALLOW"
	}

	var sb strings.Builder
	sb.WriteString("apiVersion: security.istio.io/v1beta1\n")
	sb.WriteString("kind: AuthorizationPolicy\n")
	sb.WriteString("metadata:\n")
	fmt.Fprintf(&sb, "  name: authz-rule-%d\n", idx)
	fmt.Fprintf(&sb, "  namespace: %s\n", namespace)
	sb.WriteString("spec:\n")
	fmt.Fprintf(&sb, "  action: %s\n", action)
	sb.WriteString("  rules:\n  - to:\n    - operation:\n")

	if len(rule.Paths) > 0 {
		sb.WriteString("        paths:\n")
		for _, p := range rule.Paths {
			fmt.Fprintf(&sb, "        - %s\n", p)
		}
	}

	if len(rule.Methods) > 0 {
		sb.WriteString("        methods:\n")
		for _, m := range rule.Methods {
			fmt.Fprintf(&sb, "        - %s\n", m)
		}
	}

	if len(rule.SourcePrincipals) > 0 {
		sb.WriteString("    from:\n    - source:\n        principals:\n")
		for _, p := range rule.SourcePrincipals {
			fmt.Fprintf(&sb, "        - %s\n", p)
		}
	}

	return sb.String()
}
