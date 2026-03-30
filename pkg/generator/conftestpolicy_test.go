package generator

// ============================================================
// Test Plan: Conftest Policy Generator (Task 6.0.4)
// ============================================================
//
// | #  | Test Name                                                    | Category    | Input                                              | Expected Output                                                            |
// |----|--------------------------------------------------------------|-------------|-----------------------------------------------------|---------------------------------------------------------------------------|
// |  1 | TestGenerateConftestPolicies_DenyPrivileged                  | happy       | Policies=["deny-privileged"]                        | PolicyFiles contains policy/deny-privileged.rego with deny rule           |
// |  2 | TestGenerateConftestPolicies_RequireLabels                   | happy       | Policies=["require-labels"]                         | PolicyFiles contains policy/require-labels.rego referencing labels        |
// |  3 | TestGenerateConftestPolicies_RequireResources                | happy       | Policies=["require-resources"]                      | PolicyFiles contains policy/require-resources.rego referencing resources  |
// |  4 | TestGenerateConftestPolicies_RequireProbes                   | happy       | Policies=["require-probes"]                         | PolicyFiles contains policy/require-probes.rego referencing probes        |
// |  5 | TestGenerateConftestPolicies_DenyLatestTag                   | happy       | Policies=["deny-latest-tag"]                        | PolicyFiles contains policy/deny-latest-tag.rego referencing :latest      |
// |  6 | TestGenerateConftestPolicies_AllPoliciesCombined             | happy       | all 5 policies                                      | PolicyFiles has 5 entries, Commands non-empty                             |
// |  7 | TestGenerateConftestPolicies_CustomNamespace                 | happy       | Namespace="mynamespace"                             | each .rego file contains the custom namespace declaration                 |
// |  8 | TestGenerateConftestPolicies_EmptyPoliciesDefaultsToAll      | edge        | Policies=[]                                         | result non-nil, all 5 default policies generated                          |
// |  9 | TestInjectConftestPolicies_NilChart                          | edge        | chart=nil                                           | returns nil, 0, no panic                                                  |
// | 10 | TestGenerateConftestPolicies_NOTESTxt                        | happy       | valid graph + all policies                          | NOTESTxt non-empty, contains "conftest"                                   |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newConftestChart(name string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name: name,
		Templates: map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: " + name + "\n",
		},
		ExternalFiles: []types.ExternalFileInfo{},
	}
}

var allConftestPolicies = []string{
	"deny-privileged",
	"require-labels",
	"require-resources",
	"require-probes",
	"deny-latest-tag",
}

// ── 1: deny-privileged policy ─────────────────────────────────────────────────

func TestGenerateConftestPolicies_DenyPrivileged(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := ConftestOptions{
		PolicyDir: "policy",
		Namespace: "main",
		Policies:  []string{"deny-privileged"},
	}

	result := GenerateConftestPolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestResult")
	}
	if len(result.PolicyFiles) == 0 {
		t.Fatal("expected at least one policy file for deny-privileged")
	}

	key := "policy/deny-privileged.rego"
	content, ok := result.PolicyFiles[key]
	if !ok {
		t.Fatalf("expected %q in PolicyFiles, got keys: %v", key, policyFileKeys(result.PolicyFiles))
	}
	if !strings.Contains(content, "deny") {
		t.Errorf("deny-privileged.rego must contain a 'deny' rule, got: %.300s", content)
	}
	if !strings.Contains(strings.ToLower(content), "privileged") {
		t.Errorf("deny-privileged.rego must reference 'privileged', got: %.300s", content)
	}
}

// ── 2: require-labels policy ──────────────────────────────────────────────────

func TestGenerateConftestPolicies_RequireLabels(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := ConftestOptions{
		PolicyDir: "policy",
		Namespace: "main",
		Policies:  []string{"require-labels"},
	}

	result := GenerateConftestPolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestResult")
	}

	key := "policy/require-labels.rego"
	content, ok := result.PolicyFiles[key]
	if !ok {
		t.Fatalf("expected %q in PolicyFiles, got keys: %v", key, policyFileKeys(result.PolicyFiles))
	}
	if !strings.Contains(content, "deny") {
		t.Errorf("require-labels.rego must contain a 'deny' rule, got: %.300s", content)
	}
	if !strings.Contains(strings.ToLower(content), "label") {
		t.Errorf("require-labels.rego must reference labels, got: %.300s", content)
	}
}

// ── 3: require-resources policy ───────────────────────────────────────────────

func TestGenerateConftestPolicies_RequireResources(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := ConftestOptions{
		PolicyDir: "policy",
		Namespace: "main",
		Policies:  []string{"require-resources"},
	}

	result := GenerateConftestPolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestResult")
	}

	key := "policy/require-resources.rego"
	content, ok := result.PolicyFiles[key]
	if !ok {
		t.Fatalf("expected %q in PolicyFiles, got keys: %v", key, policyFileKeys(result.PolicyFiles))
	}
	if !strings.Contains(content, "deny") {
		t.Errorf("require-resources.rego must contain a 'deny' rule, got: %.300s", content)
	}
	lower := strings.ToLower(content)
	if !strings.Contains(lower, "resource") && !strings.Contains(lower, "limit") && !strings.Contains(lower, "request") {
		t.Errorf("require-resources.rego must reference resource limits/requests, got: %.300s", content)
	}
}

// ── 4: require-probes policy ──────────────────────────────────────────────────

func TestGenerateConftestPolicies_RequireProbes(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := ConftestOptions{
		PolicyDir: "policy",
		Namespace: "main",
		Policies:  []string{"require-probes"},
	}

	result := GenerateConftestPolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestResult")
	}

	key := "policy/require-probes.rego"
	content, ok := result.PolicyFiles[key]
	if !ok {
		t.Fatalf("expected %q in PolicyFiles, got keys: %v", key, policyFileKeys(result.PolicyFiles))
	}
	if !strings.Contains(content, "deny") {
		t.Errorf("require-probes.rego must contain a 'deny' rule, got: %.300s", content)
	}
	lower := strings.ToLower(content)
	if !strings.Contains(lower, "probe") && !strings.Contains(lower, "readiness") && !strings.Contains(lower, "liveness") {
		t.Errorf("require-probes.rego must reference readiness/liveness probes, got: %.300s", content)
	}
}

// ── 5: deny-latest-tag policy ─────────────────────────────────────────────────

func TestGenerateConftestPolicies_DenyLatestTag(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := ConftestOptions{
		PolicyDir: "policy",
		Namespace: "main",
		Policies:  []string{"deny-latest-tag"},
	}

	result := GenerateConftestPolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestResult")
	}

	key := "policy/deny-latest-tag.rego"
	content, ok := result.PolicyFiles[key]
	if !ok {
		t.Fatalf("expected %q in PolicyFiles, got keys: %v", key, policyFileKeys(result.PolicyFiles))
	}
	if !strings.Contains(content, "deny") {
		t.Errorf("deny-latest-tag.rego must contain a 'deny' rule, got: %.300s", content)
	}
	if !strings.Contains(content, "latest") {
		t.Errorf("deny-latest-tag.rego must reference ':latest' tag, got: %.300s", content)
	}
}

// ── 6: All 5 policies combined ────────────────────────────────────────────────

func TestGenerateConftestPolicies_AllPoliciesCombined(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := ConftestOptions{
		PolicyDir: "policy",
		Namespace: "main",
		Policies:  allConftestPolicies,
	}

	result := GenerateConftestPolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestResult")
	}
	if len(result.PolicyFiles) != 5 {
		t.Errorf("expected 5 PolicyFiles for all 5 policies, got %d: %v",
			len(result.PolicyFiles), policyFileKeys(result.PolicyFiles))
	}
	if len(result.Commands) == 0 {
		t.Error("expected at least one conftest command in Commands")
	}
	// Commands must reference conftest and the policy directory
	cmdAll := strings.Join(result.Commands, "\n")
	if !strings.Contains(cmdAll, "conftest") {
		t.Errorf("Commands must contain 'conftest', got: %s", cmdAll)
	}
	if !strings.Contains(cmdAll, "policy") {
		t.Errorf("Commands must reference policy directory, got: %s", cmdAll)
	}
	if !strings.Contains(cmdAll, "templates/") {
		t.Errorf("Commands must reference templates path, got: %s", cmdAll)
	}
}

// ── 7: Custom namespace is reflected in each .rego file ──────────────────────

func TestGenerateConftestPolicies_CustomNamespace(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := ConftestOptions{
		PolicyDir: "policy",
		Namespace: "mynamespace",
		Policies:  allConftestPolicies,
	}

	result := GenerateConftestPolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestResult")
	}
	for path, content := range result.PolicyFiles {
		if !strings.Contains(content, "mynamespace") {
			t.Errorf("policy file %q must contain custom namespace 'mynamespace', got: %.200s", path, content)
		}
	}
}

// ── 8: Empty Policies slice defaults to all 5 built-in policies ──────────────

func TestGenerateConftestPolicies_EmptyPoliciesDefaultsToAll(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := ConftestOptions{
		PolicyDir: "policy",
		Namespace: "main",
		Policies:  []string{}, // empty → should default to all
	}

	result := GenerateConftestPolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestResult for empty Policies")
	}
	if len(result.PolicyFiles) == 0 {
		t.Error("empty Policies slice must default to all built-in policies (non-empty PolicyFiles)")
	}
	if len(result.PolicyFiles) < 5 {
		t.Errorf("expected all 5 default policies, got %d: %v",
			len(result.PolicyFiles), policyFileKeys(result.PolicyFiles))
	}
}

// ── 9: nil chart input to InjectConftestPolicies returns nil ─────────────────

func TestInjectConftestPolicies_NilChart(t *testing.T) {
	result := &ConftestResult{
		PolicyFiles: map[string]string{
			"policy/deny-privileged.rego": "package main\ndeny[msg] { msg := \"no privileged\" }",
		},
		Commands: []string{"conftest test --policy ./policy templates/"},
	}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("InjectConftestPolicies panicked on nil chart: %v", r)
		}
	}()

	updated, count := InjectConftestPolicies(nil, result)
	if updated != nil {
		t.Error("expected nil chart returned for nil input")
	}
	if count != 0 {
		t.Errorf("expected count=0 for nil chart, got %d", count)
	}
}

// ── 10: NOTESTxt is non-empty and references conftest ────────────────────────

func TestGenerateConftestPolicies_NOTESTxt(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := ConftestOptions{
		PolicyDir: "policy",
		Namespace: "main",
		Policies:  allConftestPolicies,
	}

	result := GenerateConftestPolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil ConftestResult")
	}
	if result.NOTESTxt == "" {
		t.Error("expected non-empty NOTESTxt with conftest usage instructions")
	}
	if !strings.Contains(result.NOTESTxt, "conftest") {
		t.Errorf("NOTESTxt must mention 'conftest', got: %s", result.NOTESTxt)
	}
}

// ── helper: collect PolicyFiles keys ─────────────────────────────────────────

func policyFileKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
