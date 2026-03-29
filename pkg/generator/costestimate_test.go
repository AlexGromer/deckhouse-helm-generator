package generator

// ============================================================
// Test Plan — costestimate_test.go
//
//  1. TestCostEstimate_Deployment_AWS                   — happy   Deployment 1 replica, CPU+mem requests, AWS provider → CPUCost/MemoryCost > 0
//  2. TestCostEstimate_StatefulSet_3Replicas            — happy   StatefulSet 3 replicas → Replicas=3, TotalCost = 3× single-replica cost
//  3. TestCostEstimate_Job_Parallelism4                 — happy   Job with parallelism=4 → treated as 4 replicas, cost > 0
//  4. TestCostEstimate_CronJob_Default                  — happy   CronJob → Replicas=1 (default), cost > 0
//  5. TestCostEstimate_ProviderPrices_AWS_GCP_Azure     — happy   3 providers produce different prices (overrides respected)
//  6. TestCostEstimate_IncludeStorage_PVC               — happy   IncludeStorage=true + PVC annotation → StorageCost > 0
//  7. TestCostEstimate_NoRequests_FallbackWarning       — edge    container without resource requests → warning generated, cost uses defaults
//  8. TestCostEstimate_ParseCPU_Integer                 — unit    "2" → 2000 millicores
//  9. TestCostEstimate_ParseMemory_GiB                  — unit    "1Gi" → 1024 MiB
// 10. TestCostEstimate_EmptyGraph_ZeroReport            — edge    empty graph → GrandTotal == 0.0
// 11. TestCostEstimate_EmptyRegion_UsesDefault          — edge    empty Region → report Region is non-empty (default applied)
// 12. TestCostEstimate_MalformedQuantity_Warning        — error   "garbage" CPU quantity → workload-level warning emitted
// 13. TestInjectCostNotes_Idempotent                    — integration second inject does not duplicate NOTES content
// ============================================================

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ─── Helpers ────────────────────────────────────────────────────────────────

func makeTestChartCost() *types.GeneratedChart {
	return &types.GeneratedChart{
		Name:       "test-chart",
		Path:       "/tmp/test-chart",
		ChartYAML:  "apiVersion: v2\nname: test-chart\nversion: 0.1.0",
		ValuesYAML: "replicaCount: 1",
		Templates: map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: test-app\nspec:\n  replicas: 3\n  template:\n    spec:\n      containers:\n      - name: app\n        image: app:latest\n        resources:\n          requests:\n            cpu: 100m\n            memory: 128Mi\n          limits:\n            cpu: 500m\n            memory: 256Mi",
		},
	}
}

// makeTestGraphWithWorkload builds a ResourceGraph with one workload ProcessedResource.
// cpuReq/cpuLim/memReq/memLim use Kubernetes quantity strings ("100m", "1Gi", "" for unset).
func makeTestGraphWithWorkload(kind, name, namespace string, replicas int, cpuReq, cpuLim, memReq, memLim string) *types.ResourceGraph {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": gvkForKind(kind).GroupVersion().String(),
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
		},
	}

	resources := map[string]interface{}{}
	requests := map[string]interface{}{}
	limits := map[string]interface{}{}

	if cpuReq != "" {
		requests["cpu"] = cpuReq
	}
	if memReq != "" {
		requests["memory"] = memReq
	}
	if cpuLim != "" {
		limits["cpu"] = cpuLim
	}
	if memLim != "" {
		limits["memory"] = memLim
	}
	if len(requests) > 0 {
		resources["requests"] = requests
	}
	if len(limits) > 0 {
		resources["limits"] = limits
	}

	containers := []interface{}{
		map[string]interface{}{
			"name":      "app",
			"image":     name + ":latest",
			"resources": resources,
		},
	}

	var specBody map[string]interface{}
	switch kind {
	case "CronJob":
		specBody = map[string]interface{}{
			"jobTemplate": map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": containers,
						},
					},
				},
			},
		}
	case "Job":
		specBody = map[string]interface{}{
			"parallelism": int64(replicas),
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": containers,
				},
			},
		}
	default:
		specBody = map[string]interface{}{
			"replicas": int64(replicas),
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": containers,
				},
			},
		}
	}
	obj.Object["spec"] = specBody

	r := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    gvkForKind(kind),
		},
		ServiceName: name,
		Values:      make(map[string]interface{}),
	}

	graph := types.NewResourceGraph()
	graph.AddResource(r)
	return graph
}

// ─── Section 1: Basic cost computation ───────────────────────────────────────

func TestCostEstimate_Deployment_AWS(t *testing.T) {
	t.Helper()
	graph := makeTestGraphWithWorkload("Deployment", "web", "default", 1, "500m", "1", "256Mi", "512Mi")
	opts := CostEstimateOptions{
		Provider: CloudProviderAWS,
		Region:   "us-east-1",
		Unit:     CostUnitMonthly,
	}

	report := GenerateCostEstimate(graph, opts)

	if report == nil {
		t.Fatal("GenerateCostEstimate must not return nil")
	}
	if report.Provider != CloudProviderAWS {
		t.Errorf("expected Provider=%q, got %q", CloudProviderAWS, report.Provider)
	}
	if len(report.Workloads) == 0 {
		t.Fatal("expected at least one workload estimate")
	}
	w := report.Workloads[0]
	if w.CPUCost <= 0 {
		t.Errorf("expected CPUCost > 0 for 500m request, got %f", w.CPUCost)
	}
	if w.MemoryCost <= 0 {
		t.Errorf("expected MemoryCost > 0 for 256Mi request, got %f", w.MemoryCost)
	}
	if report.GrandTotal <= 0 {
		t.Errorf("expected GrandTotal > 0, got %f", report.GrandTotal)
	}
}

func TestCostEstimate_StatefulSet_3Replicas(t *testing.T) {
	graph := makeTestGraphWithWorkload("StatefulSet", "db", "default", 3, "250m", "500m", "512Mi", "1Gi")
	opts := CostEstimateOptions{
		Provider: CloudProviderAWS,
		Region:   "us-east-1",
		Unit:     CostUnitMonthly,
	}

	report := GenerateCostEstimate(graph, opts)

	if report == nil {
		t.Fatal("GenerateCostEstimate must not return nil")
	}
	if len(report.Workloads) == 0 {
		t.Fatal("expected at least one workload estimate")
	}
	w := report.Workloads[0]
	if w.Replicas != 3 {
		t.Errorf("expected Replicas=3, got %d", w.Replicas)
	}

	// Single-replica cost × 3 should equal TotalCost (within floating-point epsilon).
	singleGraph := makeTestGraphWithWorkload("StatefulSet", "db", "default", 1, "250m", "500m", "512Mi", "1Gi")
	singleReport := GenerateCostEstimate(singleGraph, opts)
	if len(singleReport.Workloads) == 0 {
		t.Fatal("expected workload for single-replica graph")
	}
	singleTotal := singleReport.Workloads[0].TotalCost
	expected3x := singleTotal * 3
	const eps = 0.001
	if diff(w.TotalCost, expected3x) > eps {
		t.Errorf("expected 3-replica TotalCost ≈ %f (3×%f), got %f", expected3x, singleTotal, w.TotalCost)
	}
}

// diff returns absolute difference of two float64 values.
func diff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

func TestCostEstimate_Job_Parallelism4(t *testing.T) {
	graph := makeTestGraphWithWorkload("Job", "batch-job", "default", 4, "100m", "200m", "128Mi", "256Mi")
	opts := CostEstimateOptions{
		Provider: CloudProviderAWS,
		Region:   "us-east-1",
		Unit:     CostUnitHourly,
	}

	report := GenerateCostEstimate(graph, opts)

	if report == nil {
		t.Fatal("GenerateCostEstimate must not return nil")
	}
	if report.GrandTotal <= 0 {
		t.Errorf("expected GrandTotal > 0 for Job with parallelism=4, got %f", report.GrandTotal)
	}
}

func TestCostEstimate_CronJob_Default(t *testing.T) {
	graph := makeTestGraphWithWorkload("CronJob", "nightly", "default", 1, "100m", "200m", "128Mi", "256Mi")
	opts := CostEstimateOptions{
		Provider: CloudProviderAWS,
		Region:   "us-east-1",
		Unit:     CostUnitHourly,
	}

	report := GenerateCostEstimate(graph, opts)

	if report == nil {
		t.Fatal("GenerateCostEstimate must not return nil")
	}
	if len(report.Workloads) == 0 {
		t.Fatal("expected at least one workload estimate for CronJob")
	}
	w := report.Workloads[0]
	if w.Replicas != 1 {
		t.Errorf("expected CronJob default Replicas=1, got %d", w.Replicas)
	}
}

// ─── Section 2: Provider price differentiation ───────────────────────────────

func TestCostEstimate_ProviderPrices_AWS_GCP_Azure(t *testing.T) {
	cases := []struct {
		provider CloudProvider
		region   CostRegion
	}{
		{CloudProviderAWS, "us-east-1"},
		{CloudProviderGCP, "us-central1"},
		{CloudProviderAzure, "eastus"},
	}

	totals := map[CloudProvider]float64{}
	for _, tc := range cases {
		graph := makeTestGraphWithWorkload("Deployment", "app", "default", 1, "500m", "1", "256Mi", "512Mi")
		opts := CostEstimateOptions{
			Provider: tc.provider,
			Region:   tc.region,
			Unit:     CostUnitMonthly,
		}
		report := GenerateCostEstimate(graph, opts)
		if report == nil {
			t.Fatalf("GenerateCostEstimate returned nil for provider=%s", tc.provider)
		}
		totals[tc.provider] = report.GrandTotal
	}

	// At least two providers must differ (providers have distinct pricing).
	awsGcpSame := totals[CloudProviderAWS] == totals[CloudProviderGCP]
	awsAzureSame := totals[CloudProviderAWS] == totals[CloudProviderAzure]
	gcpAzureSame := totals[CloudProviderGCP] == totals[CloudProviderAzure]
	if awsGcpSame && awsAzureSame && gcpAzureSame {
		t.Errorf("all 3 providers returned identical GrandTotal=%f; expected provider-specific pricing", totals[CloudProviderAWS])
	}
}

// ─── Section 3: Storage cost ──────────────────────────────────────────────────

func TestCostEstimate_IncludeStorage_PVC(t *testing.T) {
	graph := makeTestGraphWithWorkload("StatefulSet", "db", "default", 1, "250m", "500m", "256Mi", "512Mi")

	// Add a PVC resource to the graph with storage annotation.
	pvcObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "PersistentVolumeClaim",
			"metadata": map[string]interface{}{
				"name":      "db-data",
				"namespace": "default",
				"annotations": map[string]interface{}{
					"dhg.deckhouse.io/storage-gi": "10",
				},
			},
			"spec": map[string]interface{}{
				"accessModes": []interface{}{"ReadWriteOnce"},
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"storage": "10Gi",
					},
				},
			},
		},
	}
	pvcResource := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: pvcObj,
			GVK:    gvkForKind("PersistentVolumeClaim"),
		},
		ServiceName: "db",
		Values:      make(map[string]interface{}),
	}
	graph.AddResource(pvcResource)

	opts := CostEstimateOptions{
		Provider:       CloudProviderAWS,
		Region:         "us-east-1",
		Unit:           CostUnitMonthly,
		IncludeStorage: true,
	}

	report := GenerateCostEstimate(graph, opts)

	if report == nil {
		t.Fatal("GenerateCostEstimate must not return nil")
	}
	if report.TotalStorageCost <= 0 {
		t.Errorf("expected TotalStorageCost > 0 with IncludeStorage=true and a 10Gi PVC, got %f", report.TotalStorageCost)
	}
}

// ─── Section 4: Edge — no requests ───────────────────────────────────────────

func TestCostEstimate_NoRequests_FallbackWarning(t *testing.T) {
	// Container with no resource requests.
	graph := makeTestGraphWithWorkload("Deployment", "app", "default", 1, "", "", "", "")
	opts := CostEstimateOptions{
		Provider: CloudProviderAWS,
		Region:   "us-east-1",
		Unit:     CostUnitMonthly,
	}

	report := GenerateCostEstimate(graph, opts)

	if report == nil {
		t.Fatal("GenerateCostEstimate must not return nil")
	}
	if len(report.Workloads) == 0 {
		t.Fatal("expected at least one workload estimate even with no requests")
	}

	// Implementation must emit a warning for missing resource requests.
	w := report.Workloads[0]
	if len(w.Warnings) == 0 {
		t.Error("expected at least one warning for container with no resource requests")
	}
	foundWarning := false
	for _, warn := range w.Warnings {
		if strings.Contains(strings.ToLower(warn), "request") || strings.Contains(strings.ToLower(warn), "default") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Errorf("expected warning mentioning 'request' or 'default', got warnings: %v", w.Warnings)
	}
}

// ─── Section 5: parseResourceQuantity unit tests ─────────────────────────────

func TestCostEstimate_ParseCPU_Integer(t *testing.T) {
	millis, err := parseResourceQuantity("2", true)
	if err != nil {
		t.Fatalf("parseResourceQuantity(\"2\", isCPU=true) returned error: %v", err)
	}
	if millis != 2000 {
		t.Errorf("expected 2000 millicores for CPU quantity \"2\", got %d", millis)
	}
}

func TestCostEstimate_ParseMemory_GiB(t *testing.T) {
	mib, err := parseResourceQuantity("1Gi", false)
	if err != nil {
		t.Fatalf("parseResourceQuantity(\"1Gi\", isCPU=false) returned error: %v", err)
	}
	if mib != 1024 {
		t.Errorf("expected 1024 MiB for memory quantity \"1Gi\", got %d", mib)
	}
}

// ─── Section 6: Empty graph ───────────────────────────────────────────────────

func TestCostEstimate_EmptyGraph_ZeroReport(t *testing.T) {
	graph := types.NewResourceGraph()
	opts := CostEstimateOptions{
		Provider: CloudProviderAWS,
		Region:   "us-east-1",
		Unit:     CostUnitMonthly,
	}

	report := GenerateCostEstimate(graph, opts)

	if report == nil {
		t.Fatal("GenerateCostEstimate must not return nil for empty graph")
	}
	if report.GrandTotal != 0.0 {
		t.Errorf("expected GrandTotal=0.0 for empty graph, got %f", report.GrandTotal)
	}
	if len(report.Workloads) != 0 {
		t.Errorf("expected no workload estimates for empty graph, got %d", len(report.Workloads))
	}
}

// ─── Section 7: Empty region → default applied ───────────────────────────────

func TestCostEstimate_EmptyRegion_UsesDefault(t *testing.T) {
	graph := makeTestGraphWithWorkload("Deployment", "app", "default", 1, "100m", "200m", "128Mi", "256Mi")
	opts := CostEstimateOptions{
		Provider: CloudProviderAWS,
		Region:   "", // empty — implementation must substitute a default
		Unit:     CostUnitMonthly,
	}

	report := GenerateCostEstimate(graph, opts)

	if report == nil {
		t.Fatal("GenerateCostEstimate must not return nil when Region is empty")
	}
	if string(report.Region) == "" {
		t.Error("expected report.Region to be non-empty (implementation must apply default region)")
	}
}

// ─── Section 8: Malformed quantity ───────────────────────────────────────────

func TestCostEstimate_MalformedQuantity_Warning(t *testing.T) {
	// Inject a resource with a non-parseable CPU quantity string.
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "bad-app",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"replicas": int64(1),
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "app",
								"image": "app:latest",
								"resources": map[string]interface{}{
									"requests": map[string]interface{}{
										"cpu":    "not-a-number",
										"memory": "128Mi",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	r := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    gvkForKind("Deployment"),
		},
		ServiceName: "bad-app",
		Values:      make(map[string]interface{}),
	}
	graph := types.NewResourceGraph()
	graph.AddResource(r)

	opts := CostEstimateOptions{
		Provider: CloudProviderAWS,
		Region:   "us-east-1",
		Unit:     CostUnitMonthly,
	}

	report := GenerateCostEstimate(graph, opts)

	if report == nil {
		t.Fatal("GenerateCostEstimate must not return nil for malformed quantity")
	}
	// Implementation must not panic and must emit at least one warning.
	if len(report.Workloads) == 0 {
		t.Fatal("expected workload entry even for malformed quantity")
	}
	w := report.Workloads[0]
	if len(w.Warnings) == 0 {
		t.Error("expected at least one warning for malformed CPU quantity \"not-a-number\"")
	}
}

// ─── Section 9: InjectCostNotes — idempotency ────────────────────────────────

func TestInjectCostNotes_Idempotent(t *testing.T) {
	graph := makeTestGraphWithWorkload("Deployment", "web", "default", 2, "200m", "500m", "128Mi", "256Mi")
	opts := CostEstimateOptions{
		Provider: CloudProviderAWS,
		Region:   "us-east-1",
		Unit:     CostUnitMonthly,
	}
	report := GenerateCostEstimate(graph, opts)
	if report == nil {
		t.Fatal("GenerateCostEstimate must not return nil")
	}

	chart := makeTestChartCost()

	result1, injected1 := InjectCostNotes(chart, report)
	if result1 == nil {
		t.Fatal("InjectCostNotes returned nil on first call")
	}
	if !injected1 {
		t.Error("expected injected=true on first InjectCostNotes call")
	}

	// Second injection on already-injected chart must not duplicate content.
	result2, injected2 := InjectCostNotes(result1, report)
	if result2 == nil {
		t.Fatal("InjectCostNotes returned nil on second call")
	}
	_ = injected2 // second call may return false — that is acceptable

	// Notes content must not be duplicated.
	if result1.Notes == "" {
		t.Fatal("expected Notes to be non-empty after first inject")
	}
	count1 := strings.Count(result1.Notes, "Cost Estimate")
	count2 := strings.Count(result2.Notes, "Cost Estimate")
	if count2 > count1 {
		t.Errorf("second InjectCostNotes duplicated content: count before=%d, after=%d", count1, count2)
	}
}
