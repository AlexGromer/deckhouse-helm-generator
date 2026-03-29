package generator

// ============================================================
// Test Plan: Policy-as-Code Orchestration Layer (Task 5.5.5)
// ============================================================
//
// | #  | Test Name                                              | Category    | Input                                                         | Expected Output                                                         |
// |----|--------------------------------------------------------|-------------|---------------------------------------------------------------|-------------------------------------------------------------------------|
// |  1 | TestGeneratePolicyAsCode_DualOutput                    | happy       | OutputFormats=["kyverno","opa"], all 5 policy types           | KyvernoPolicies and OPAPolicies both non-empty                          |
// |  2 | TestGeneratePolicyAsCode_KyvernoOnly                   | happy       | OutputFormats=["kyverno"], all 5 policy types                 | KyvernoPolicies non-empty, OPAPolicies empty                            |
// |  3 | TestGeneratePolicyAsCode_OPAOnly                       | happy       | OutputFormats=["opa"], all 5 policy types                     | OPAPolicies non-empty, KyvernoPolicies empty                            |
// |  4 | TestGeneratePolicyAsCode_RequireLabels                 | happy       | PolicyTypes=["require-labels"]                                | at least 1 policy referencing label requirements                        |
// |  5 | TestGeneratePolicyAsCode_RequireResources              | happy       | PolicyTypes=["require-resources"]                             | at least 1 policy referencing resource limits                           |
// |  6 | TestGeneratePolicyAsCode_DisallowPrivileged            | happy       | PolicyTypes=["disallow-privileged"]                           | at least 1 policy referencing privileged containers                     |
// |  7 | TestGeneratePolicyAsCode_RestrictRegistries            | happy       | PolicyTypes=["restrict-registries"]                           | at least 1 policy referencing image registries                          |
// |  8 | TestGeneratePolicyAsCode_RequireProbes                 | happy       | PolicyTypes=["require-probes"]                                | at least 1 policy referencing readiness/liveness probes                 |
// |  9 | TestGeneratePolicyAsCode_EmptyGraph                    | edge        | empty graph, all formats & types                              | result non-nil, Policies may be empty, no panic                         |
// | 10 | TestGeneratePolicyAsCode_SummaryContainsCounts         | happy       | dual output, 5 policy types                                   | Summary contains kyverno and opa counts as numbers                      |

import (
	"fmt"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

var allPolicyTypes = []string{
	"require-labels",
	"require-resources",
	"disallow-privileged",
	"restrict-registries",
	"require-probes",
}

// ── 1: Dual output — both kyverno and opa maps populated ─────────────────────

func TestGeneratePolicyAsCode_DualOutput(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := PolicyAsCodeOptions{
		OutputFormats: []string{"kyverno", "opa"},
		PolicyTypes:   allPolicyTypes,
	}

	result := GeneratePolicyAsCode(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil PolicyAsCodeResult")
	}
	if len(result.KyvernoPolicies) == 0 {
		t.Error("expected non-empty KyvernoPolicies for dual output")
	}
	if len(result.OPAPolicies) == 0 {
		t.Error("expected non-empty OPAPolicies for dual output")
	}
}

// ── 2: Kyverno-only output ────────────────────────────────────────────────────

func TestGeneratePolicyAsCode_KyvernoOnly(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := PolicyAsCodeOptions{
		OutputFormats: []string{"kyverno"},
		PolicyTypes:   allPolicyTypes,
	}

	result := GeneratePolicyAsCode(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil PolicyAsCodeResult")
	}
	if len(result.KyvernoPolicies) == 0 {
		t.Error("expected non-empty KyvernoPolicies for kyverno-only")
	}
	if len(result.OPAPolicies) != 0 {
		t.Errorf("expected empty OPAPolicies for kyverno-only, got %d entries", len(result.OPAPolicies))
	}
	// All kyverno content must reference kyverno.io
	for name, content := range result.KyvernoPolicies {
		if !strings.Contains(content, "kyverno.io") {
			t.Errorf("kyverno policy %q must contain 'kyverno.io', got: %.200s", name, content)
		}
	}
}

// ── 3: OPA-only output ────────────────────────────────────────────────────────

func TestGeneratePolicyAsCode_OPAOnly(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := PolicyAsCodeOptions{
		OutputFormats: []string{"opa"},
		PolicyTypes:   allPolicyTypes,
	}

	result := GeneratePolicyAsCode(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil PolicyAsCodeResult")
	}
	if len(result.OPAPolicies) == 0 {
		t.Error("expected non-empty OPAPolicies for opa-only")
	}
	if len(result.KyvernoPolicies) != 0 {
		t.Errorf("expected empty KyvernoPolicies for opa-only, got %d entries", len(result.KyvernoPolicies))
	}
	// All OPA content must reference gatekeeper
	for name, content := range result.OPAPolicies {
		if !strings.Contains(content, "gatekeeper.sh") {
			t.Errorf("opa policy %q must contain 'gatekeeper.sh', got: %.200s", name, content)
		}
	}
}

// ── 4: require-labels policy type ────────────────────────────────────────────

func TestGeneratePolicyAsCode_RequireLabels(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := PolicyAsCodeOptions{
		OutputFormats: []string{"kyverno"},
		PolicyTypes:   []string{"require-labels"},
	}

	result := GeneratePolicyAsCode(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil PolicyAsCodeResult")
	}
	found := false
	for _, content := range result.KyvernoPolicies {
		if strings.Contains(content, "app.kubernetes.io") || strings.Contains(content, "label") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected at least one require-labels policy referencing label requirements")
	}
}

// ── 5: require-resources policy type ─────────────────────────────────────────

func TestGeneratePolicyAsCode_RequireResources(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := PolicyAsCodeOptions{
		OutputFormats: []string{"kyverno"},
		PolicyTypes:   []string{"require-resources"},
	}

	result := GeneratePolicyAsCode(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil PolicyAsCodeResult")
	}
	found := false
	for _, content := range result.KyvernoPolicies {
		if strings.Contains(content, "resource") || strings.Contains(content, "limits") || strings.Contains(content, "requests") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected at least one require-resources policy referencing resource limits")
	}
}

// ── 6: disallow-privileged policy type ───────────────────────────────────────

func TestGeneratePolicyAsCode_DisallowPrivileged(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := PolicyAsCodeOptions{
		OutputFormats: []string{"kyverno"},
		PolicyTypes:   []string{"disallow-privileged"},
	}

	result := GeneratePolicyAsCode(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil PolicyAsCodeResult")
	}
	found := false
	for _, content := range result.KyvernoPolicies {
		if strings.Contains(content, "privileged") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected at least one disallow-privileged policy referencing privileged containers")
	}
}

// ── 7: restrict-registries policy type ───────────────────────────────────────

func TestGeneratePolicyAsCode_RestrictRegistries(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := PolicyAsCodeOptions{
		OutputFormats: []string{"kyverno"},
		PolicyTypes:   []string{"restrict-registries"},
	}

	result := GeneratePolicyAsCode(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil PolicyAsCodeResult")
	}
	found := false
	for _, content := range result.KyvernoPolicies {
		if strings.Contains(content, "registr") || strings.Contains(content, "image") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected at least one restrict-registries policy referencing image registries")
	}
}

// ── 8: require-probes policy type ────────────────────────────────────────────

func TestGeneratePolicyAsCode_RequireProbes(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := PolicyAsCodeOptions{
		OutputFormats: []string{"kyverno"},
		PolicyTypes:   []string{"require-probes"},
	}

	result := GeneratePolicyAsCode(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil PolicyAsCodeResult")
	}
	found := false
	for _, content := range result.KyvernoPolicies {
		if strings.Contains(content, "Probe") || strings.Contains(content, "probe") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected at least one require-probes policy referencing readiness/liveness probes")
	}
}

// ── 9: Empty graph → no panic, result non-nil ─────────────────────────────────

func TestGeneratePolicyAsCode_EmptyGraph(t *testing.T) {
	graph := types.NewResourceGraph()
	opts := PolicyAsCodeOptions{
		OutputFormats: []string{"kyverno", "opa"},
		PolicyTypes:   allPolicyTypes,
	}

	// Must not panic
	result := GeneratePolicyAsCode(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil PolicyAsCodeResult even for empty graph")
	}
	if result.KyvernoPolicies == nil {
		t.Error("KyvernoPolicies map must be initialized (not nil) even for empty graph")
	}
	if result.OPAPolicies == nil {
		t.Error("OPAPolicies map must be initialized (not nil) even for empty graph")
	}
}

// ── 10: Summary contains policy counts ───────────────────────────────────────

func TestGeneratePolicyAsCode_SummaryContainsCounts(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := PolicyAsCodeOptions{
		OutputFormats: []string{"kyverno", "opa"},
		PolicyTypes:   allPolicyTypes,
	}

	result := GeneratePolicyAsCode(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil PolicyAsCodeResult")
	}
	if result.Summary == "" {
		t.Fatal("Summary must not be empty")
	}
	// Summary must contain the counts as integers in string form
	kyvernoCount := len(result.KyvernoPolicies)
	opaCount := len(result.OPAPolicies)
	if kyvernoCount > 0 && !strings.Contains(result.Summary, fmt.Sprintf("%d", kyvernoCount)) {
		t.Errorf("Summary must contain kyverno policy count %d, got: %s", kyvernoCount, result.Summary)
	}
	if opaCount > 0 && !strings.Contains(result.Summary, fmt.Sprintf("%d", opaCount)) {
		t.Errorf("Summary must contain opa policy count %d, got: %s", opaCount, result.Summary)
	}
}
