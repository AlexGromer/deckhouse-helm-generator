package generator

// ============================================================
// Test Plan: SLO-based Alerting Generator (Task 5.9.6)
// ============================================================
//
// | #  | Test Name                                               | Category | Input                                               | Expected Output                                                           |
// |----|---------------------------------------------------------|----------|-----------------------------------------------------|---------------------------------------------------------------------------|
// |  1 | TestGenerateSLOConfig_AvailabilitySLO99_9               | happy    | AvailabilitySLO=99.9                                | ServiceLevels contain "99.9" availability target                          |
// |  2 | TestGenerateSLOConfig_LatencySLO                        | happy    | LatencySLO=95.0, LatencyThresholdMs=200             | ServiceLevels contain latency target and "200" threshold                  |
// |  3 | TestGenerateSLOConfig_BurnRateAlertsGenerated           | happy    | default opts, non-empty graph                       | BurnRateAlerts is non-empty map                                            |
// |  4 | TestGenerateSLOConfig_ErrorBudgetCalculation            | happy    | AvailabilitySLO=99.9                                | ErrorBudget non-empty and contains budget-related content                 |
// |  5 | TestGenerateSLOConfig_CustomBurnRateWindows             | happy    | BurnRateWindows=["1m","5m","30m"]                   | BurnRateAlerts reference all specified windows                            |
// |  6 | TestGenerateSLOConfig_EmptyGraph                        | edge     | empty graph                                         | non-nil SLOResult, ServiceLevels may be empty                             |
// |  7 | TestInjectSLOAlerts_NilChart                            | error    | nil chart, valid result                             | returns (nil, 0) without panic                                            |
// |  8 | TestInjectSLOAlerts_CopyOnWrite                         | happy    | non-nil chart                                       | original chart.Templates unchanged after inject                           |
// |  9 | TestInjectSLOAlerts_Idempotent                          | happy    | inject twice with same result                       | second inject returns count=0, templates unchanged                        |
// | 10 | TestGenerateSLOConfig_NOTESTxt                          | happy    | non-empty graph, valid opts                         | NOTESTxt non-empty and mentions SLO or alerting                           |
// | 11 | TestGenerateSLOConfig_ServiceLevelCRDContainsSpec       | happy    | non-empty graph                                     | ServiceLevels YAML contains "spec:" field                                 |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// defaultSLOOpts returns a baseline SLOOptions for reuse in tests.
func defaultSLOOpts() SLOOptions {
	return SLOOptions{
		AvailabilitySLO:    99.9,
		LatencySLO:         95.0,
		LatencyThresholdMs: 200,
		BurnRateWindows:    []string{"5m", "30m", "1h", "6h"},
	}
}

// buildSLOGraph creates a resource graph with a single Deployment.
func buildSLOGraph(name, namespace string) *types.ResourceGraph {
	r := makeProcessedResource("Deployment", name, namespace, map[string]string{"app": name})
	return buildGraph([]*types.ProcessedResource{r}, nil)
}

// allSLOServiceLevelContent flattens all ServiceLevels values for assertions.
func allSLOServiceLevelContent(result *SLOResult) string {
	if result == nil {
		return ""
	}
	var sb strings.Builder
	for _, v := range result.ServiceLevels {
		sb.WriteString(v)
		sb.WriteString("\n")
	}
	return sb.String()
}

// allSLOBurnRateContent flattens all BurnRateAlerts values for assertions.
func allSLOBurnRateContent(result *SLOResult) string {
	if result == nil {
		return ""
	}
	var sb strings.Builder
	for _, v := range result.BurnRateAlerts {
		sb.WriteString(v)
		sb.WriteString("\n")
	}
	return sb.String()
}

// ── Test 1: 99.9% availability SLO reflected in ServiceLevels ────────────────

func TestGenerateSLOConfig_AvailabilitySLO99_9(t *testing.T) {
	graph := buildSLOGraph("myapp", "default")
	opts := SLOOptions{
		AvailabilitySLO:    99.9,
		LatencySLO:         95.0,
		LatencyThresholdMs: 200,
		BurnRateWindows:    []string{"5m", "30m", "1h", "6h"},
	}

	result := GenerateSLOConfig(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil SLOResult")
	}
	if len(result.ServiceLevels) == 0 {
		t.Fatal("expected ServiceLevels to be populated for non-empty graph")
	}
	content := allSLOServiceLevelContent(result)
	if !strings.Contains(content, "99.9") {
		t.Errorf("ServiceLevels must contain availability SLO '99.9', got:\n%s", content)
	}
}

// ── Test 2: latency SLO and threshold in ServiceLevels ───────────────────────

func TestGenerateSLOConfig_LatencySLO(t *testing.T) {
	graph := buildSLOGraph("myapp", "default")
	opts := SLOOptions{
		AvailabilitySLO:    99.5,
		LatencySLO:         95.0,
		LatencyThresholdMs: 500,
		BurnRateWindows:    []string{"5m", "30m", "1h", "6h"},
	}

	result := GenerateSLOConfig(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil SLOResult")
	}
	content := allSLOServiceLevelContent(result)
	if !strings.Contains(content, "95") {
		t.Errorf("ServiceLevels must contain latency SLO '95', got:\n%s", content)
	}
	if !strings.Contains(content, "500") {
		t.Errorf("ServiceLevels must contain latency threshold '500' ms, got:\n%s", content)
	}
}

// ── Test 3: BurnRateAlerts populated for non-empty graph ─────────────────────

func TestGenerateSLOConfig_BurnRateAlertsGenerated(t *testing.T) {
	graph := buildSLOGraph("myapp", "default")
	opts := defaultSLOOpts()

	result := GenerateSLOConfig(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil SLOResult")
	}
	if len(result.BurnRateAlerts) == 0 {
		t.Error("expected BurnRateAlerts to be non-empty for non-empty graph with valid SLO opts")
	}
}

// ── Test 4: error budget is calculated and non-empty ─────────────────────────

func TestGenerateSLOConfig_ErrorBudgetCalculation(t *testing.T) {
	graph := buildSLOGraph("myapp", "default")
	opts := SLOOptions{
		AvailabilitySLO:    99.9,
		LatencySLO:         95.0,
		LatencyThresholdMs: 200,
		BurnRateWindows:    []string{"5m", "30m", "1h", "6h"},
	}

	result := GenerateSLOConfig(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil SLOResult")
	}
	if result.ErrorBudget == "" {
		t.Error("expected ErrorBudget to be non-empty for availability SLO 99.9%")
	}
	// Error budget for 99.9% availability = 0.1%, which may appear in various forms.
	lc := strings.ToLower(result.ErrorBudget)
	if !strings.Contains(lc, "budget") && !strings.Contains(lc, "error") && !strings.Contains(result.ErrorBudget, "0.1") {
		t.Errorf("ErrorBudget must describe error budget context, got: %s", result.ErrorBudget)
	}
}

// ── Test 5: custom burn-rate windows appear in BurnRateAlerts ─────────────────

func TestGenerateSLOConfig_CustomBurnRateWindows(t *testing.T) {
	graph := buildSLOGraph("myapp", "default")
	opts := SLOOptions{
		AvailabilitySLO:    99.9,
		LatencySLO:         95.0,
		LatencyThresholdMs: 200,
		BurnRateWindows:    []string{"1m", "5m", "30m"},
	}

	result := GenerateSLOConfig(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil SLOResult")
	}
	content := allSLOBurnRateContent(result)
	for _, window := range []string{"1m", "5m", "30m"} {
		if !strings.Contains(content, window) {
			t.Errorf("BurnRateAlerts must reference custom window %q, got:\n%s", window, content)
		}
	}
}

// ── Test 6: empty graph returns non-nil SLOResult ────────────────────────────

func TestGenerateSLOConfig_EmptyGraph(t *testing.T) {
	graph := types.NewResourceGraph()
	opts := defaultSLOOpts()

	result := GenerateSLOConfig(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil SLOResult even for empty graph")
	}
	if result.ServiceLevels == nil {
		t.Error("ServiceLevels map must be non-nil even for empty graph")
	}
	if result.BurnRateAlerts == nil {
		t.Error("BurnRateAlerts map must be non-nil even for empty graph")
	}
}

// ── Test 7: nil chart returns (nil, 0) ───────────────────────────────────────

func TestInjectSLOAlerts_NilChart(t *testing.T) {
	graph := buildSLOGraph("myapp", "default")
	sloResult := GenerateSLOConfig(graph, defaultSLOOpts())

	newChart, count := InjectSLOAlerts(nil, sloResult)

	if newChart != nil {
		t.Errorf("expected nil chart for nil input, got %+v", newChart)
	}
	if count != 0 {
		t.Errorf("expected count=0 for nil chart, got %d", count)
	}
}

// ── Test 8: InjectSLOAlerts is copy-on-write ──────────────────────────────────

func TestInjectSLOAlerts_CopyOnWrite(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})
	originalCount := len(chart.Templates)

	graph := buildSLOGraph("myapp", "default")
	sloResult := GenerateSLOConfig(graph, defaultSLOOpts())

	newChart, _ := InjectSLOAlerts(chart, sloResult)

	if newChart == nil {
		t.Fatal("expected non-nil new chart")
	}
	if newChart == chart {
		t.Error("InjectSLOAlerts must return a new chart (copy-on-write)")
	}
	if len(chart.Templates) != originalCount {
		t.Errorf("original chart.Templates mutated: had %d templates, now %d",
			originalCount, len(chart.Templates))
	}
	for path := range chart.Templates {
		if strings.Contains(path, "slo") || strings.Contains(path, "prometheusservicelevel") {
			t.Errorf("original chart gained SLO template %q — copy-on-write violated", path)
		}
	}
}

// ── Test 9: InjectSLOAlerts idempotent ───────────────────────────────────────

func TestInjectSLOAlerts_Idempotent(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})

	graph := buildSLOGraph("myapp", "default")
	sloResult := GenerateSLOConfig(graph, defaultSLOOpts())

	firstChart, firstCount := InjectSLOAlerts(chart, sloResult)
	if firstChart == nil {
		t.Fatal("first inject returned nil chart")
	}
	if firstCount == 0 {
		t.Error("expected firstCount > 0 on first SLO inject")
	}

	secondChart, secondCount := InjectSLOAlerts(firstChart, sloResult)
	if secondChart == nil {
		t.Fatal("second inject returned nil chart")
	}
	if secondCount != 0 {
		t.Errorf("expected secondCount=0 on idempotent inject, got %d", secondCount)
	}
	for path, content := range secondChart.Templates {
		if firstContent, ok := firstChart.Templates[path]; ok {
			if content != firstContent {
				t.Errorf("template %q changed on second inject — idempotency violated", path)
			}
		}
	}
}

// ── Test 10: NOTESTxt non-empty and mentions SLO ─────────────────────────────

func TestGenerateSLOConfig_NOTESTxt(t *testing.T) {
	graph := buildSLOGraph("myapp", "default")
	opts := defaultSLOOpts()

	result := GenerateSLOConfig(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil SLOResult")
	}
	if result.NOTESTxt == "" {
		t.Error("expected NOTESTxt to be non-empty")
	}
	lower := strings.ToLower(result.NOTESTxt)
	if !strings.Contains(lower, "slo") && !strings.Contains(lower, "alert") &&
		!strings.Contains(lower, "burn") && !strings.Contains(lower, "budget") {
		t.Errorf("NOTESTxt must mention SLO/alerting/burn rate context, got: %s", result.NOTESTxt)
	}
}

// ── Test 11: ServiceLevel CRD YAML contains "spec:" ──────────────────────────

func TestGenerateSLOConfig_ServiceLevelCRDContainsSpec(t *testing.T) {
	graph := buildSLOGraph("myapp", "default")
	opts := defaultSLOOpts()

	result := GenerateSLOConfig(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil SLOResult")
	}
	if len(result.ServiceLevels) == 0 {
		t.Fatal("expected ServiceLevels to be populated for non-empty graph")
	}
	for name, yaml := range result.ServiceLevels {
		if !strings.Contains(yaml, "spec:") {
			t.Errorf("ServiceLevel CRD YAML for %q must contain 'spec:' field:\n%s", name, yaml)
		}
	}
}
