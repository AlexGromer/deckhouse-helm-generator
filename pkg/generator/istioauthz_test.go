package generator

// ============================================================
// Test Plan: Istio AuthorizationPolicy Generator (Task 5.8.7)
// ============================================================
//
// | #  | Test Name                                               | Category    | Input                                            | Expected Output                                                          |
// |----|---------------------------------------------------------|-------------|--------------------------------------------------|--------------------------------------------------------------------------|
// |  1 | TestGenerateIstioAuthzPolicy_DenyByDefault              | happy       | DenyByDefault=true, no rules                     | AuthorizationPolicy with action DENY and empty selector (namespace-wide) |
// |  2 | TestGenerateIstioAuthzPolicy_AllowRulePerPath           | happy       | Rule with Paths=["/api/*"], Action="ALLOW"        | AuthorizationPolicy YAML contains "/api/*" path                          |
// |  3 | TestGenerateIstioAuthzPolicy_AllowRulePerMethod         | happy       | Rule with Methods=["GET","POST"]                  | YAML contains "GET" and "POST"                                           |
// |  4 | TestGenerateIstioAuthzPolicy_SourcePrincipal            | happy       | Rule with SourcePrincipals=["cluster.local/..."]  | YAML contains principal string                                           |
// |  5 | TestGenerateIstioAuthzPolicy_MultipleRules              | happy       | 3 rules with different paths                      | YAML contains all 3 paths                                                |
// |  6 | TestGenerateIstioAuthzPolicy_EmptyNamespace             | edge        | Namespace=""                                      | returns nil or empty map, no panic                                       |
// |  7 | TestGenerateIstioAuthzPolicy_DenyAction                 | happy       | Rule with Action="DENY"                           | AuthorizationPolicy YAML contains action: DENY                           |
// |  8 | TestGenerateIstioAuthzPolicy_CombinedPolicies           | happy       | DenyByDefault=true + ALLOW rules                  | YAML contains both DENY default and ALLOW rule policies                  |
// |  9 | TestGenerateIstioAuthzPolicy_ValidAPIVersion            | happy       | valid opts                                        | YAML contains security.istio.io apiVersion and AuthorizationPolicy kind  |
// | 10 | TestGenerateIstioAuthzPolicy_NamespaceInYAML            | happy       | Namespace="production"                            | YAML references namespace "production"                                   |

import (
	"strings"
	"testing"
)

// ── 1: DenyByDefault produces a deny-all AuthorizationPolicy ─────────────────

func TestGenerateIstioAuthzPolicy_DenyByDefault(t *testing.T) {
	opts := AuthzPolicyOptions{
		Namespace:     "default",
		Rules:         nil,
		DenyByDefault: true,
	}

	templates := GenerateIstioAuthzPolicy(opts)

	if len(templates) == 0 {
		t.Fatal("expected at least one AuthorizationPolicy template")
	}
	all := joinTemplates(templates)
	if !strings.Contains(all, "AuthorizationPolicy") {
		t.Errorf("expected kind 'AuthorizationPolicy' in YAML:\n%s", all)
	}
	if !strings.Contains(all, "DENY") {
		t.Errorf("expected action DENY in deny-by-default policy:\n%s", all)
	}
}

// ── 2: ALLOW rule contains configured path ───────────────────────────────────

func TestGenerateIstioAuthzPolicy_AllowRulePerPath(t *testing.T) {
	opts := AuthzPolicyOptions{
		Namespace: "default",
		Rules: []AuthzRule{
			{
				Paths:   []string{"/api/*"},
				Methods: []string{"GET"},
				Action:  "ALLOW",
			},
		},
		DenyByDefault: false,
	}

	templates := GenerateIstioAuthzPolicy(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, "/api/*") {
		t.Errorf("expected path '/api/*' in AuthorizationPolicy YAML:\n%s", all)
	}
}

// ── 3: ALLOW rule contains configured HTTP methods ───────────────────────────

func TestGenerateIstioAuthzPolicy_AllowRulePerMethod(t *testing.T) {
	opts := AuthzPolicyOptions{
		Namespace: "default",
		Rules: []AuthzRule{
			{
				Paths:   []string{"/"},
				Methods: []string{"GET", "POST"},
				Action:  "ALLOW",
			},
		},
		DenyByDefault: false,
	}

	templates := GenerateIstioAuthzPolicy(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, "GET") {
		t.Errorf("expected method 'GET' in YAML:\n%s", all)
	}
	if !strings.Contains(all, "POST") {
		t.Errorf("expected method 'POST' in YAML:\n%s", all)
	}
}

// ── 4: SourcePrincipals appears in generated policy ──────────────────────────

func TestGenerateIstioAuthzPolicy_SourcePrincipal(t *testing.T) {
	principal := "cluster.local/ns/default/sa/frontend"
	opts := AuthzPolicyOptions{
		Namespace: "default",
		Rules: []AuthzRule{
			{
				Paths:            []string{"/health"},
				Methods:          []string{"GET"},
				SourcePrincipals: []string{principal},
				Action:           "ALLOW",
			},
		},
		DenyByDefault: false,
	}

	templates := GenerateIstioAuthzPolicy(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, principal) {
		t.Errorf("expected principal %q in AuthorizationPolicy YAML:\n%s", principal, all)
	}
}

// ── 5: multiple rules — all paths present ────────────────────────────────────

func TestGenerateIstioAuthzPolicy_MultipleRules(t *testing.T) {
	opts := AuthzPolicyOptions{
		Namespace: "default",
		Rules: []AuthzRule{
			{Paths: []string{"/api/v1/*"}, Methods: []string{"GET"}, Action: "ALLOW"},
			{Paths: []string{"/api/v2/*"}, Methods: []string{"POST"}, Action: "ALLOW"},
			{Paths: []string{"/internal/*"}, Methods: []string{"GET"}, Action: "DENY"},
		},
		DenyByDefault: false,
	}

	templates := GenerateIstioAuthzPolicy(opts)

	all := joinTemplates(templates)
	for _, path := range []string{"/api/v1/*", "/api/v2/*", "/internal/*"} {
		if !strings.Contains(all, path) {
			t.Errorf("expected path %q in YAML:\n%s", path, all)
		}
	}
}

// ── 6: empty Namespace returns nil/empty, no panic ───────────────────────────

func TestGenerateIstioAuthzPolicy_EmptyNamespace(t *testing.T) {
	opts := AuthzPolicyOptions{
		Namespace:     "",
		Rules:         nil,
		DenyByDefault: true,
	}

	// Must not panic.
	templates := GenerateIstioAuthzPolicy(opts)

	if len(templates) > 0 {
		t.Logf("GenerateIstioAuthzPolicy with empty Namespace returned %d templates (implementation choice)", len(templates))
	}
}

// ── 7: DENY action rule in YAML ───────────────────────────────────────────────

func TestGenerateIstioAuthzPolicy_DenyAction(t *testing.T) {
	opts := AuthzPolicyOptions{
		Namespace: "default",
		Rules: []AuthzRule{
			{
				Paths:   []string{"/admin/*"},
				Methods: []string{"DELETE"},
				Action:  "DENY",
			},
		},
		DenyByDefault: false,
	}

	templates := GenerateIstioAuthzPolicy(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, "DENY") {
		t.Errorf("expected action DENY in YAML for DENY rule:\n%s", all)
	}
	if !strings.Contains(all, "/admin/*") {
		t.Errorf("expected path '/admin/*' in YAML:\n%s", all)
	}
}

// ── 8: DenyByDefault + ALLOW rules produce both policy types ─────────────────

func TestGenerateIstioAuthzPolicy_CombinedPolicies(t *testing.T) {
	opts := AuthzPolicyOptions{
		Namespace: "production",
		Rules: []AuthzRule{
			{
				Paths:            []string{"/api/*"},
				Methods:          []string{"GET", "POST"},
				SourcePrincipals: []string{"cluster.local/ns/default/sa/api-client"},
				Action:           "ALLOW",
			},
		},
		DenyByDefault: true,
	}

	templates := GenerateIstioAuthzPolicy(opts)

	if len(templates) < 2 {
		t.Errorf("expected at least 2 templates for DenyByDefault + ALLOW rule, got %d", len(templates))
	}
	all := joinTemplates(templates)
	if !strings.Contains(all, "DENY") {
		t.Errorf("expected DENY policy in combined output:\n%s", all)
	}
	if !strings.Contains(all, "ALLOW") {
		t.Errorf("expected ALLOW policy in combined output:\n%s", all)
	}
}

// ── 9: generated YAML has valid Istio security apiVersion ────────────────────

func TestGenerateIstioAuthzPolicy_ValidAPIVersion(t *testing.T) {
	opts := AuthzPolicyOptions{
		Namespace: "default",
		Rules: []AuthzRule{
			{Paths: []string{"/*"}, Methods: []string{"GET"}, Action: "ALLOW"},
		},
		DenyByDefault: false,
	}

	templates := GenerateIstioAuthzPolicy(opts)

	if len(templates) == 0 {
		t.Fatal("expected at least one template")
	}
	all := joinTemplates(templates)
	if !strings.Contains(all, "security.istio.io") {
		t.Errorf("expected apiVersion 'security.istio.io' in YAML:\n%s", all)
	}
	if !strings.Contains(all, "AuthorizationPolicy") {
		t.Errorf("expected kind 'AuthorizationPolicy' in YAML:\n%s", all)
	}
}

// ── 10: Namespace appears in generated YAML ──────────────────────────────────

func TestGenerateIstioAuthzPolicy_NamespaceInYAML(t *testing.T) {
	opts := AuthzPolicyOptions{
		Namespace: "production",
		Rules: []AuthzRule{
			{Paths: []string{"/health"}, Methods: []string{"GET"}, Action: "ALLOW"},
		},
		DenyByDefault: false,
	}

	templates := GenerateIstioAuthzPolicy(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, "production") {
		t.Errorf("expected namespace 'production' in AuthorizationPolicy YAML:\n%s", all)
	}
}
