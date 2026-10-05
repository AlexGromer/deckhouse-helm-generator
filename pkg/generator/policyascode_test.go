package generator

import (
	"regexp"
	"strings"
	"testing"
)

// secAssertBalanced checks that every block action of a Helm template is
// closed (helm lint catches this too, but only in the golden suite).
func secAssertBalanced(t *testing.T, name, tpl string) {
	t.Helper()
	open := regexp.MustCompile(`\{\{-?\s*(if|range|with|define)\b`).FindAllString(tpl, -1)
	end := regexp.MustCompile(`\{\{-?\s*end\s*-?\}\}`).FindAllString(tpl, -1)
	if len(open) != len(end) {
		t.Errorf("%s: %d block actions but %d ends", name, len(open), len(end))
	}
}

func TestPolicyCatalogue(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range policyCatalogue {
		if seen[r.ID] || seen[r.ValuesKey] {
			t.Errorf("duplicate rule %s/%s", r.ID, r.ValuesKey)
		}
		seen[r.ID], seen[r.ValuesKey] = true, true
		if r.Rego == "" || r.Title == "" || r.Severity == "" {
			t.Errorf("rule %s is incomplete", r.ID)
		}
		if r.ID != "restrict-registries" && !strings.HasPrefix(r.Kyverno, "    - name: ") {
			t.Errorf("rule %s: Kyverno rules must be a list indented under spec.rules", r.ID)
		}
		// Helm would try to evaluate Kyverno variables: they must not appear.
		if strings.Contains(r.Kyverno, "{{") {
			t.Errorf("rule %s: Kyverno rule contains template braces", r.ID)
		}
	}
	for _, id := range defaultPolicyRules {
		if _, ok := lookupPolicyRule(id); !ok {
			t.Errorf("default rule %s not in catalogue", id)
		}
	}
}

func TestParsePolicyOptions(t *testing.T) {
	fc := FeatureContext{Params: map[string]string{
		"engine": "kyverno, conftest", "rules": "require-probes", "action": "enforce", "registries": "ghcr.io/acme",
	}}
	opts, err := parsePolicyOptions(fc)
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Kyverno || !opts.Conftest || opts.Action != "Enforce" {
		t.Errorf("opts = %+v", opts)
	}
	if len(opts.Rules) != 2 || opts.Rules[0].ID != "require-probes" || opts.Rules[1].ID != "restrict-registries" {
		t.Errorf("rules = %+v (registries must enable restrict-registries)", opts.Rules)
	}
}

func TestKyvernoPolicyTemplate(t *testing.T) {
	h := secChartHelpers{fullnameTpl: "app.fullname", labelsTpl: "app.labels"}
	tpl := kyvernoPolicyTemplate(h, policyCatalogue)
	secAssertBalanced(t, "admission-policies.yaml", tpl)
	if !strings.Contains(tpl, "{{- if and $ap.rules.restrictRegistries $ap.allowedRegistries }}") {
		t.Error("restrict-registries must be guarded by a non-empty allowedRegistries")
	}
	if !strings.Contains(tpl, `{{- $patterns = append $patterns (printf "%s/*" (trimSuffix "/" .)) }}`) {
		t.Error("registry patterns must be built from values")
	}
}

func TestConftestPolicyFiles(t *testing.T) {
	rule, _ := lookupPolicyRule("restrict-registries")
	files := conftestPolicyFiles([]policyRule{rule}, nil)
	if _, ok := files["policy/restrict_registries.rego"]; ok {
		t.Error("restrict-registries without registries would reject every image")
	}
	files = conftestPolicyFiles([]policyRule{rule}, []string{"ghcr.io/acme", "quay.io"})
	if !strings.Contains(files["policy/restrict_registries.rego"], `dhg_allowed_registries := ["ghcr.io/acme", "quay.io"]`) {
		t.Errorf("registries not embedded:\n%s", files["policy/restrict_registries.rego"])
	}
	if !strings.Contains(files["policy/dhg_workloads.rego"], `dhg_pod_spec := input.spec.jobTemplate.spec.template.spec if input.kind == "CronJob"`) {
		t.Error("shared workload helpers missing")
	}
}

func TestSecurityTemplatesBalanced(t *testing.T) {
	h := secChartHelpers{fullnameTpl: "app.fullname", labelsTpl: "app.labels"}
	secAssertBalanced(t, "external-secrets.yaml", externalSecretsTemplate(h, "external-secrets.io/v1"))
	secAssertBalanced(t, "istio-egress.yaml", istioEgressTemplate(h, "networking.istio.io/v1"))
	secAssertBalanced(t, "_vault-agent.tpl", vaultAgentHelperTemplate("app.vaultAgent.podAnnotations"))
}
