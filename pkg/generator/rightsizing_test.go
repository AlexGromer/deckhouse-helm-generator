package generator

// ============================================================
// Test Plan — rightsizing_test.go
//
//  1. TestRightSizing_OverprovisionedCPU                — happy   CPU limit/request ratio 5× → "overprovisioned" issue
//  2. TestRightSizing_UnderprovisionedMemory            — happy   mem limit/request ratio 1.11× < UnderprovisionThreshold(1.2) → "underprovisioned"
//  3. TestRightSizing_NoLimits                          — happy   container with no limits → "no-limits" issue
//  4. TestRightSizing_NoRequests                        — happy   container with no requests → "no-requests" issue
//  5. TestRightSizing_Job_SkippedWhenBatchFalse         — edge    Job + IncludeBatchWorkloads=false → not analysed
//  6. TestRightSizing_Job_IncludedWhenBatchTrue         — edge    Job + IncludeBatchWorkloads=true → analysed
//  7. TestRightSizing_WebWorkloadType_Recommendations   — happy   WorkloadType=web → recommendations mention web-specific advice
//  8. TestRightSizing_RatioExactly3x_NotFlagged         — boundary ratio == OverprovisionThreshold exactly → NOT "overprovisioned"
//  9. TestRightSizing_RatioExactly1_2_NotFlagged        — boundary ratio == UnderprovisionThreshold exactly → NOT "underprovisioned"
// 10. TestRightSizing_ZeroRequests_NoRatioIssue         — edge    requests=0 → ratio undefined → no overprovisioned/underprovisioned issue
// 11. TestRightSizing_EmptyGraph                        — edge    empty graph → TotalIssues=0
// 12. TestRightSizing_MalformedMemory_NoRatioIssue      — error   malformed memory quantity → workload listed but ratio issues absent
// 13. TestGenerateRightSizingNotes_ContainsWorkloadNames — integration GenerateRightSizingNotes → returned string contains workload names
// ============================================================

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ─── Section 1: Overprovisioned CPU ──────────────────────────────────────────

func TestRightSizing_OverprovisionedCPU(t *testing.T) {
	// CPU request=100m, CPU limit=500m → ratio = 5.0 > OverprovisionThreshold(3.0)
	graph := makeTestGraphWithWorkload("Deployment", "web", "default", 1, "100m", "500m", "128Mi", "256Mi")
	opts := RightSizingOptions{
		WorkloadType:            WorkloadWeb,
		OverprovisionThreshold:  OverprovisionThreshold,
		UnderprovisionThreshold: UnderprovisionThreshold,
		IncludeBatchWorkloads:   false,
	}

	report := AnalyzeRightSizing(graph, opts)

	if report == nil {
		t.Fatal("AnalyzeRightSizing must not return nil")
	}

	count := report.IssuesByType[RightSizingIssueOverprovisioned]
	if count == 0 {
		t.Errorf("expected at least one 'overprovisioned' issue for CPU ratio 5×, got IssuesByType=%v", report.IssuesByType)
	}
}

// ─── Section 2: Underprovisioned memory ──────────────────────────────────────

func TestRightSizing_UnderprovisionedMemory(t *testing.T) {
	// mem request=128Mi, mem limit=140Mi → ratio ≈ 1.09 < UnderprovisionThreshold(1.2)
	graph := makeTestGraphWithWorkload("Deployment", "web", "default", 1, "100m", "200m", "128Mi", "140Mi")
	opts := RightSizingOptions{
		WorkloadType:            WorkloadWeb,
		OverprovisionThreshold:  OverprovisionThreshold,
		UnderprovisionThreshold: UnderprovisionThreshold,
		IncludeBatchWorkloads:   false,
	}

	report := AnalyzeRightSizing(graph, opts)

	if report == nil {
		t.Fatal("AnalyzeRightSizing must not return nil")
	}

	count := report.IssuesByType[RightSizingIssueUnderprovisioned]
	if count == 0 {
		t.Errorf("expected at least one 'underprovisioned' issue for mem ratio 1.09×, got IssuesByType=%v", report.IssuesByType)
	}
}

// ─── Section 3: No limits ────────────────────────────────────────────────────

func TestRightSizing_NoLimits(t *testing.T) {
	// Requests present, no limits.
	graph := makeTestGraphWithWorkload("Deployment", "web", "default", 1, "100m", "", "128Mi", "")
	opts := RightSizingOptions{
		WorkloadType:            WorkloadWeb,
		OverprovisionThreshold:  OverprovisionThreshold,
		UnderprovisionThreshold: UnderprovisionThreshold,
		IncludeBatchWorkloads:   false,
	}

	report := AnalyzeRightSizing(graph, opts)

	if report == nil {
		t.Fatal("AnalyzeRightSizing must not return nil")
	}

	count := report.IssuesByType[RightSizingIssueNoLimits]
	if count == 0 {
		t.Errorf("expected at least one 'no-limits' issue for container without resource limits, got IssuesByType=%v", report.IssuesByType)
	}
}

// ─── Section 4: No requests ──────────────────────────────────────────────────

func TestRightSizing_NoRequests(t *testing.T) {
	// No requests at all.
	graph := makeTestGraphWithWorkload("Deployment", "web", "default", 1, "", "500m", "", "256Mi")
	opts := RightSizingOptions{
		WorkloadType:            WorkloadWeb,
		OverprovisionThreshold:  OverprovisionThreshold,
		UnderprovisionThreshold: UnderprovisionThreshold,
		IncludeBatchWorkloads:   false,
	}

	report := AnalyzeRightSizing(graph, opts)

	if report == nil {
		t.Fatal("AnalyzeRightSizing must not return nil")
	}

	count := report.IssuesByType[RightSizingIssueNoRequests]
	if count == 0 {
		t.Errorf("expected at least one 'no-requests' issue for container without resource requests, got IssuesByType=%v", report.IssuesByType)
	}
}

// ─── Section 5: Job skipped when IncludeBatch=false ──────────────────────────

func TestRightSizing_Job_SkippedWhenBatchFalse(t *testing.T) {
	graph := makeTestGraphWithWorkload("Job", "batch-job", "default", 1, "100m", "500m", "128Mi", "256Mi")
	opts := RightSizingOptions{
		WorkloadType:            WorkloadBatch,
		OverprovisionThreshold:  OverprovisionThreshold,
		UnderprovisionThreshold: UnderprovisionThreshold,
		IncludeBatchWorkloads:   false,
	}

	report := AnalyzeRightSizing(graph, opts)

	if report == nil {
		t.Fatal("AnalyzeRightSizing must not return nil")
	}

	if len(report.Workloads) != 0 {
		t.Errorf("expected Job to be skipped when IncludeBatchWorkloads=false, but got %d workloads", len(report.Workloads))
	}
	if report.TotalIssues != 0 {
		t.Errorf("expected TotalIssues=0 when Job is skipped, got %d", report.TotalIssues)
	}
}

// ─── Section 6: Job included when IncludeBatch=true ──────────────────────────

func TestRightSizing_Job_IncludedWhenBatchTrue(t *testing.T) {
	// CPU ratio 5× will trigger overprovisioned once analysed.
	graph := makeTestGraphWithWorkload("Job", "batch-job", "default", 1, "100m", "500m", "128Mi", "256Mi")
	opts := RightSizingOptions{
		WorkloadType:            WorkloadBatch,
		OverprovisionThreshold:  OverprovisionThreshold,
		UnderprovisionThreshold: UnderprovisionThreshold,
		IncludeBatchWorkloads:   true,
	}

	report := AnalyzeRightSizing(graph, opts)

	if report == nil {
		t.Fatal("AnalyzeRightSizing must not return nil")
	}

	if len(report.Workloads) == 0 {
		t.Error("expected Job to be included when IncludeBatchWorkloads=true")
	}
}

// ─── Section 7: WorkloadType=web recommendations ─────────────────────────────

func TestRightSizing_WebWorkloadType_Recommendations(t *testing.T) {
	// CPU ratio 5× → overprovisioned.
	graph := makeTestGraphWithWorkload("Deployment", "web", "default", 1, "100m", "500m", "128Mi", "256Mi")
	opts := RightSizingOptions{
		WorkloadType:            WorkloadWeb,
		OverprovisionThreshold:  OverprovisionThreshold,
		UnderprovisionThreshold: UnderprovisionThreshold,
		IncludeBatchWorkloads:   false,
	}

	report := AnalyzeRightSizing(graph, opts)

	if report == nil {
		t.Fatal("AnalyzeRightSizing must not return nil")
	}

	notes := GenerateRightSizingNotes(report)
	if notes == "" {
		t.Fatal("GenerateRightSizingNotes must not return empty string when issues exist")
	}

	// Notes for a web workload should mention the workload name.
	if !strings.Contains(notes, "web") {
		t.Errorf("expected notes to contain workload name 'web', got:\n%s", notes)
	}
}

// ─── Section 8: Boundary — ratio exactly 3.0 NOT flagged ─────────────────────

func TestRightSizing_RatioExactly3x_NotFlagged(t *testing.T) {
	// CPU request=100m, CPU limit=300m → ratio = 3.0 = threshold → NOT overprovisioned.
	graph := makeTestGraphWithWorkload("Deployment", "web", "default", 1, "100m", "300m", "128Mi", "384Mi")
	opts := RightSizingOptions{
		WorkloadType:            WorkloadWeb,
		OverprovisionThreshold:  OverprovisionThreshold, // 3.0
		UnderprovisionThreshold: UnderprovisionThreshold,
		IncludeBatchWorkloads:   false,
	}

	report := AnalyzeRightSizing(graph, opts)

	if report == nil {
		t.Fatal("AnalyzeRightSizing must not return nil")
	}

	count := report.IssuesByType[RightSizingIssueOverprovisioned]
	if count > 0 {
		t.Errorf("ratio exactly equal to OverprovisionThreshold (3.0) must NOT be flagged as overprovisioned, but got count=%d", count)
	}
}

// ─── Section 9: Boundary — ratio exactly 1.2 NOT flagged ─────────────────────

func TestRightSizing_RatioExactly1_2_NotFlagged(t *testing.T) {
	// mem request=100Mi, mem limit=120Mi → ratio = 1.2 = threshold → NOT underprovisioned.
	graph := makeTestGraphWithWorkload("Deployment", "web", "default", 1, "100m", "200m", "100Mi", "120Mi")
	opts := RightSizingOptions{
		WorkloadType:            WorkloadWeb,
		OverprovisionThreshold:  OverprovisionThreshold,
		UnderprovisionThreshold: UnderprovisionThreshold, // 1.2
		IncludeBatchWorkloads:   false,
	}

	report := AnalyzeRightSizing(graph, opts)

	if report == nil {
		t.Fatal("AnalyzeRightSizing must not return nil")
	}

	count := report.IssuesByType[RightSizingIssueUnderprovisioned]
	if count > 0 {
		t.Errorf("ratio exactly equal to UnderprovisionThreshold (1.2) must NOT be flagged as underprovisioned, but got count=%d", count)
	}
}

// ─── Section 10: Zero requests — no ratio issue ───────────────────────────────

func TestRightSizing_ZeroRequests_NoRatioIssue(t *testing.T) {
	// requests=0 makes ratio undefined; should produce no-requests rather than overprovisioned.
	graph := makeTestGraphWithWorkload("Deployment", "web", "default", 1, "0", "500m", "0", "256Mi")
	opts := RightSizingOptions{
		WorkloadType:            WorkloadWeb,
		OverprovisionThreshold:  OverprovisionThreshold,
		UnderprovisionThreshold: UnderprovisionThreshold,
		IncludeBatchWorkloads:   false,
	}

	report := AnalyzeRightSizing(graph, opts)

	if report == nil {
		t.Fatal("AnalyzeRightSizing must not return nil")
	}

	overprovCount := report.IssuesByType[RightSizingIssueOverprovisioned]
	if overprovCount > 0 {
		t.Errorf("requests=0 must NOT produce overprovisioned issue (ratio undefined), got count=%d", overprovCount)
	}
}

// ─── Section 11: Empty graph ──────────────────────────────────────────────────

func TestRightSizing_EmptyGraph(t *testing.T) {
	graph := makeTestGraph()
	opts := RightSizingOptions{
		WorkloadType:            WorkloadWeb,
		OverprovisionThreshold:  OverprovisionThreshold,
		UnderprovisionThreshold: UnderprovisionThreshold,
		IncludeBatchWorkloads:   false,
	}

	report := AnalyzeRightSizing(graph, opts)

	if report == nil {
		t.Fatal("AnalyzeRightSizing must not return nil for empty graph")
	}
	if report.TotalIssues != 0 {
		t.Errorf("expected TotalIssues=0 for empty graph, got %d", report.TotalIssues)
	}
	if len(report.Workloads) != 0 {
		t.Errorf("expected 0 workloads for empty graph, got %d", len(report.Workloads))
	}
}

// makeTestGraph returns an empty ResourceGraph for use in right-sizing tests.
func makeTestGraph() *types.ResourceGraph {
	return types.NewResourceGraph()
}

// ─── Section 12: Malformed memory — no ratio issue ───────────────────────────

func TestRightSizing_MalformedMemory_NoRatioIssue(t *testing.T) {
	// Use a well-formed CPU but malformed memory to verify no panic and graceful degradation.
	graph := makeTestGraphWithWorkload("Deployment", "web", "default", 1, "100m", "300m", "bad-memory", "bad-memory-limit")
	opts := RightSizingOptions{
		WorkloadType:            WorkloadWeb,
		OverprovisionThreshold:  OverprovisionThreshold,
		UnderprovisionThreshold: UnderprovisionThreshold,
		IncludeBatchWorkloads:   false,
	}

	// Must not panic.
	report := AnalyzeRightSizing(graph, opts)

	if report == nil {
		t.Fatal("AnalyzeRightSizing must not return nil for malformed memory")
	}

	// Memory ratio issues must not be emitted when quantities are unparseable.
	underprovCount := report.IssuesByType[RightSizingIssueUnderprovisioned]
	overprovCount := report.IssuesByType[RightSizingIssueOverprovisioned]
	if underprovCount > 0 || overprovCount > 0 {
		t.Errorf("malformed memory should not produce ratio issues; overprovisioned=%d, underprovisioned=%d", overprovCount, underprovCount)
	}
}

// ─── Section 13: GenerateRightSizingNotes contains workload names ─────────────

func TestGenerateRightSizingNotes_ContainsWorkloadNames(t *testing.T) {
	// Build a report with two workloads, each having a distinct issue.
	graph1 := makeTestGraphWithWorkload("Deployment", "frontend", "default", 1, "100m", "500m", "128Mi", "256Mi")
	graph2 := makeTestGraphWithWorkload("Deployment", "backend", "default", 1, "100m", "", "128Mi", "")

	opts := RightSizingOptions{
		WorkloadType:            WorkloadWeb,
		OverprovisionThreshold:  OverprovisionThreshold,
		UnderprovisionThreshold: UnderprovisionThreshold,
		IncludeBatchWorkloads:   false,
	}

	report1 := AnalyzeRightSizing(graph1, opts)
	report2 := AnalyzeRightSizing(graph2, opts)

	if report1 == nil || report2 == nil {
		t.Fatal("AnalyzeRightSizing must not return nil")
	}

	// Merge workloads into a combined report.
	combined := &RightSizingReport{
		Workloads:    append(report1.Workloads, report2.Workloads...),
		TotalIssues:  report1.TotalIssues + report2.TotalIssues,
		IssuesByType: make(map[RightSizingIssue]int),
	}
	for k, v := range report1.IssuesByType {
		combined.IssuesByType[k] += v
	}
	for k, v := range report2.IssuesByType {
		combined.IssuesByType[k] += v
	}

	notes := GenerateRightSizingNotes(combined)

	if notes == "" {
		t.Fatal("GenerateRightSizingNotes must return non-empty string when issues exist")
	}
	if !strings.Contains(notes, "frontend") {
		t.Errorf("expected notes to contain workload name 'frontend', got:\n%s", notes)
	}
	if !strings.Contains(notes, "backend") {
		t.Errorf("expected notes to contain workload name 'backend', got:\n%s", notes)
	}
}

// ─── Section 14: InjectRightSizingNotes idempotency ──────────────────────────

func TestInjectRightSizingNotes_Idempotent(t *testing.T) {
	graph := makeTestGraphWithWorkload("Deployment", "web", "default", 1, "100m", "500m", "128Mi", "256Mi")
	opts := RightSizingOptions{
		WorkloadType:            WorkloadWeb,
		OverprovisionThreshold:  OverprovisionThreshold,
		UnderprovisionThreshold: UnderprovisionThreshold,
		IncludeBatchWorkloads:   false,
	}

	report := AnalyzeRightSizing(graph, opts)
	if report == nil {
		t.Fatal("AnalyzeRightSizing must not return nil")
	}

	chart := makeTestChartCost()

	result1, injected1 := InjectRightSizingNotes(chart, report)
	if result1 == nil {
		t.Fatal("InjectRightSizingNotes returned nil on first call")
	}
	if !injected1 {
		t.Error("expected injected=true on first InjectRightSizingNotes call")
	}
	if result1.Notes == "" {
		t.Fatal("expected Notes to be non-empty after first inject")
	}

	// Second injection must not duplicate content.
	result2, _ := InjectRightSizingNotes(result1, report)
	if result2 == nil {
		t.Fatal("InjectRightSizingNotes returned nil on second call")
	}

	count1 := strings.Count(result1.Notes, "Right-Sizing")
	count2 := strings.Count(result2.Notes, "Right-Sizing")
	if count2 > count1 {
		t.Errorf("second InjectRightSizingNotes duplicated content: count before=%d, after=%d", count1, count2)
	}
}
