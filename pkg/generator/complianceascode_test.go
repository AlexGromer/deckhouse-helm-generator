package generator

// ============================================================
// Test Plan: Compliance-as-Code Generator (Task 5.5.3)
// ============================================================
//
// | #  | Test Name                                              | Category    | Input                                                    | Expected Output                                                    |
// |----|--------------------------------------------------------|-------------|----------------------------------------------------------|--------------------------------------------------------------------|
// |  1 | TestGenerateCompliancePolicies_CISStandard             | happy       | graph with Deployment, standard=["cis"]                  | result has Policies map with at least 1 entry containing "cis"     |
// |  2 | TestGenerateCompliancePolicies_PSSStandard             | happy       | graph with Deployment, standard=["pss"]                  | result Policies contain PSS-related content (runAsNonRoot etc.)    |
// |  3 | TestGenerateCompliancePolicies_LabelsStandard          | happy       | graph with Deployment, standard=["labels"]               | result Policies contain label requirement policy                   |
// |  4 | TestGenerateCompliancePolicies_ResourcesStandard       | happy       | graph with Deployment, standard=["resources"]            | result Policies contain resource limits requirement policy         |
// |  5 | TestGenerateCompliancePolicies_CombinedStandards       | happy       | graph with Deployment, all 4 standards                   | result Policies count >= 4                                         |
// |  6 | TestGenerateCompliancePolicies_EmptyGraph              | edge        | empty graph, standards=["cis"]                           | result non-nil, Policies may be empty, no panic                    |
// |  7 | TestGenerateCompliancePolicies_KyvernoEngine           | happy       | graph with Deployment, engine="kyverno"                  | Policies contain "kyverno.io" in at least one policy               |
// |  8 | TestGenerateCompliancePolicies_OPAEngine               | happy       | graph with Deployment, engine="opa"                      | Policies contain "gatekeeper.sh" in at least one policy            |
// |  9 | TestGenerateCompliancePolicies_ViolationsDetected      | happy       | graph with Deployment lacking security context           | result Violations non-empty                                        |
// | 10 | TestGenerateCompliancePolicies_SeverityFilter          | boundary    | opts with Severity="high"                                | result Violations contain only high-severity entries               |
// | 11 | TestInjectCompliancePolicies_AddsToExternalFiles       | happy       | chart + ComplianceResult with 2 Policies                 | returned chart ExternalFiles len >= 2                              |
// | 12 | TestInjectCompliancePolicies_Idempotent                | integration | inject twice with same result                            | second inject count == 0 (already present)                         |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func makeDeploymentGraph() *types.ResourceGraph {
	deploy := makeProcessedResource("Deployment", "test-app", "default", nil)
	return buildGraph([]*types.ProcessedResource{deploy}, nil)
}

func makeDeploymentNoSecCtxGraph() *types.ResourceGraph {
	// Deployment without any securityContext set
	deploy := makeProcessedResource("Deployment", "insecure-app", "default", nil)
	g := types.NewResourceGraph()
	g.AddResource(deploy)
	return g
}

// ── 1: CIS standard generates policies ───────────────────────────────────────

func TestGenerateCompliancePolicies_CISStandard(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := ComplianceOptions{
		Engine:    "kyverno",
		Standards: []string{"cis"},
		Severity:  "medium",
	}

	result := GenerateCompliancePolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil ComplianceResult")
	}
	if len(result.Policies) == 0 {
		t.Error("expected at least 1 policy for cis standard")
	}
	// At least one policy should be CIS-related
	found := false
	for name := range result.Policies {
		if strings.Contains(strings.ToLower(name), "cis") ||
			strings.Contains(strings.ToLower(name), "privileged") ||
			strings.Contains(strings.ToLower(name), "security") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected at least one cis-related policy, got keys: %v", policyKeys(result.Policies))
	}
}

// ── 2: PSS standard generates pod security policies ──────────────────────────

func TestGenerateCompliancePolicies_PSSStandard(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := ComplianceOptions{
		Engine:    "kyverno",
		Standards: []string{"pss"},
		Severity:  "high",
	}

	result := GenerateCompliancePolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil ComplianceResult")
	}
	if len(result.Policies) == 0 {
		t.Error("expected policies for pss standard")
	}
	// PSS policies must reference pod-security concepts
	content := allPolicyContent(result.Policies)
	if !strings.Contains(content, "runAsNonRoot") &&
		!strings.Contains(content, "privileged") &&
		!strings.Contains(content, "securityContext") &&
		!strings.Contains(content, "pss") {
		t.Errorf("pss policies must reference pod security concepts, got content snippet: %.200s", content)
	}
}

// ── 3: Labels standard generates label requirement policies ──────────────────

func TestGenerateCompliancePolicies_LabelsStandard(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := ComplianceOptions{
		Engine:    "kyverno",
		Standards: []string{"labels"},
		Severity:  "low",
	}

	result := GenerateCompliancePolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil ComplianceResult")
	}
	if len(result.Policies) == 0 {
		t.Error("expected policies for labels standard")
	}
	content := allPolicyContent(result.Policies)
	if !strings.Contains(content, "app.kubernetes.io") && !strings.Contains(content, "label") {
		t.Errorf("labels policies must reference label requirements, got: %.200s", content)
	}
}

// ── 4: Resources standard generates resource limits policies ─────────────────

func TestGenerateCompliancePolicies_ResourcesStandard(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := ComplianceOptions{
		Engine:    "kyverno",
		Standards: []string{"resources"},
		Severity:  "medium",
	}

	result := GenerateCompliancePolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil ComplianceResult")
	}
	if len(result.Policies) == 0 {
		t.Error("expected policies for resources standard")
	}
	content := allPolicyContent(result.Policies)
	if !strings.Contains(content, "resource") && !strings.Contains(content, "limits") && !strings.Contains(content, "requests") {
		t.Errorf("resources policies must reference resource limits, got: %.200s", content)
	}
}

// ── 5: Combined standards → at least 4 policies total ────────────────────────

func TestGenerateCompliancePolicies_CombinedStandards(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := ComplianceOptions{
		Engine:    "kyverno",
		Standards: []string{"cis", "pss", "labels", "resources"},
		Severity:  "low",
	}

	result := GenerateCompliancePolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil ComplianceResult")
	}
	if len(result.Policies) < 4 {
		t.Errorf("expected >= 4 policies for all 4 combined standards, got %d", len(result.Policies))
	}
}

// ── 6: Empty graph → no panic, result non-nil ─────────────────────────────────

func TestGenerateCompliancePolicies_EmptyGraph(t *testing.T) {
	graph := types.NewResourceGraph()
	opts := ComplianceOptions{
		Engine:    "kyverno",
		Standards: []string{"cis"},
		Severity:  "medium",
	}

	// Must not panic
	result := GenerateCompliancePolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil ComplianceResult even for empty graph")
	}
	// Policies map must be initialized (may be empty)
	if result.Policies == nil {
		t.Error("Policies map must not be nil even for empty graph")
	}
}

// ── 7: Kyverno engine → policies use kyverno.io apiVersion ───────────────────

func TestGenerateCompliancePolicies_KyvernoEngine(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := ComplianceOptions{
		Engine:    "kyverno",
		Standards: []string{"labels"},
		Severity:  "low",
	}

	result := GenerateCompliancePolicies(graph, opts)

	if result == nil || len(result.Policies) == 0 {
		t.Fatal("expected non-empty policies for kyverno engine")
	}
	content := allPolicyContent(result.Policies)
	if !strings.Contains(content, "kyverno.io") {
		t.Errorf("kyverno engine policies must reference 'kyverno.io', got: %.300s", content)
	}
}

// ── 8: OPA engine → policies use gatekeeper.sh apiVersion ────────────────────

func TestGenerateCompliancePolicies_OPAEngine(t *testing.T) {
	graph := makeDeploymentGraph()
	opts := ComplianceOptions{
		Engine:    "opa",
		Standards: []string{"labels"},
		Severity:  "low",
	}

	result := GenerateCompliancePolicies(graph, opts)

	if result == nil || len(result.Policies) == 0 {
		t.Fatal("expected non-empty policies for opa engine")
	}
	content := allPolicyContent(result.Policies)
	if !strings.Contains(content, "gatekeeper.sh") {
		t.Errorf("opa engine policies must reference 'gatekeeper.sh', got: %.300s", content)
	}
}

// ── 9: Violations detected for insecure workload ─────────────────────────────

func TestGenerateCompliancePolicies_ViolationsDetected(t *testing.T) {
	// Deployment without security context — should trigger violations
	graph := makeDeploymentNoSecCtxGraph()
	opts := ComplianceOptions{
		Engine:    "kyverno",
		Standards: []string{"pss", "resources"},
		Severity:  "low",
	}

	result := GenerateCompliancePolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Violations) == 0 {
		t.Error("expected violations for insecure deployment missing securityContext and resources")
	}
	// Each violation must have required fields
	for i, v := range result.Violations {
		if v.Resource == "" {
			t.Errorf("violation[%d]: Resource must not be empty", i)
		}
		if v.Standard == "" {
			t.Errorf("violation[%d]: Standard must not be empty", i)
		}
		if v.Severity == "" {
			t.Errorf("violation[%d]: Severity must not be empty", i)
		}
	}
}

// ── 10: Severity filter — only high-severity violations returned ──────────────

func TestGenerateCompliancePolicies_SeverityFilter(t *testing.T) {
	graph := makeDeploymentNoSecCtxGraph()
	opts := ComplianceOptions{
		Engine:    "kyverno",
		Standards: []string{"cis", "pss", "labels", "resources"},
		Severity:  "high",
	}

	result := GenerateCompliancePolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// All reported violations must match or exceed the requested severity
	for i, v := range result.Violations {
		if strings.ToLower(v.Severity) != "high" && strings.ToLower(v.Severity) != "critical" {
			t.Errorf("violation[%d]: expected severity high/critical when filter=high, got %q", i, v.Severity)
		}
	}
}

// ── 11: InjectCompliancePolicies adds policies to ExternalFiles ──────────────

func TestInjectCompliancePolicies_AddsToExternalFiles(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:          "test-chart",
		Templates:     map[string]string{},
		ExternalFiles: []types.ExternalFileInfo{},
	}
	compResult := &ComplianceResult{
		Policies: map[string]string{
			"compliance/require-labels.yaml":    "apiVersion: kyverno.io/v1\nkind: ClusterPolicy\n",
			"compliance/deny-privileged.yaml":   "apiVersion: kyverno.io/v1\nkind: ClusterPolicy\n",
		},
		Violations: nil,
	}

	updated, count := InjectCompliancePolicies(chart, compResult)

	if updated == nil {
		t.Fatal("expected non-nil updated chart")
	}
	if count != 2 {
		t.Errorf("expected count=2 (2 new policies), got %d", count)
	}
	if len(updated.ExternalFiles) < 2 {
		t.Errorf("expected at least 2 ExternalFiles after injection, got %d", len(updated.ExternalFiles))
	}
	// Verify content was injected
	found := 0
	for _, f := range updated.ExternalFiles {
		if strings.HasPrefix(f.Path, "compliance/") {
			found++
		}
	}
	if found < 2 {
		t.Errorf("expected 2 compliance ExternalFiles, found %d", found)
	}
}

// ── 12: InjectCompliancePolicies idempotent — second call returns count=0 ────

func TestInjectCompliancePolicies_Idempotent(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:          "test-chart",
		Templates:     map[string]string{},
		ExternalFiles: []types.ExternalFileInfo{},
	}
	compResult := &ComplianceResult{
		Policies: map[string]string{
			"compliance/require-labels.yaml": "apiVersion: kyverno.io/v1\nkind: ClusterPolicy\n",
		},
	}

	first, count1 := InjectCompliancePolicies(chart, compResult)
	if count1 == 0 {
		t.Fatal("first inject must return count > 0")
	}

	// Second inject on already-updated chart
	second, count2 := InjectCompliancePolicies(first, compResult)

	if second == nil {
		t.Fatal("expected non-nil result on second inject")
	}
	if count2 != 0 {
		t.Errorf("idempotent: second inject should return count=0, got %d", count2)
	}
	// ExternalFiles length must not change
	if len(second.ExternalFiles) != len(first.ExternalFiles) {
		t.Errorf("idempotent: ExternalFiles count changed from %d to %d on second inject",
			len(first.ExternalFiles), len(second.ExternalFiles))
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func policyKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func allPolicyContent(policies map[string]string) string {
	var sb strings.Builder
	for _, v := range policies {
		sb.WriteString(v)
		sb.WriteByte('\n')
	}
	return sb.String()
}
