package generator

// ============================================================
// Test Plan: Prometheus Recording Rules Generator (Task 5.9.7)
// ============================================================
//
// | #  | Test Name                                              | Category | Input                                              | Expected Output                                                              |
// |----|--------------------------------------------------------|----------|----------------------------------------------------|------------------------------------------------------------------------------|
// |  1 | TestGenerateRecordingRules_P50P95P99Default             | happy    | default quantiles [0.5, 0.95, 0.99]                | Rules contain p50, p95, p99 histogram_quantile expressions                   |
// |  2 | TestGenerateRecordingRules_NamespaceInRules             | happy    | Namespace="production"                             | Rules content references "production"                                        |
// |  3 | TestGenerateRecordingRules_CustomQuantiles              | happy    | Quantiles=[0.5, 0.75, 0.99]                        | Rules contain entries for 0.5, 0.75 and 0.99                                 |
// |  4 | TestGenerateRecordingRules_AggregationInterval          | happy    | AggregationInterval="5m"                           | Rules content references "5m" interval                                       |
// |  5 | TestGenerateRecordingRules_EmptyGraph                   | edge     | empty graph                                        | non-nil RecordingRulesResult with RuleCount=0                                |
// |  6 | TestInjectRecordingRules_NilChart                       | error    | nil chart, valid result                            | returns (nil, 0) without panic                                               |
// |  7 | TestInjectRecordingRules_CopyOnWrite                    | happy    | non-nil chart                                      | original chart.Templates unchanged after inject                              |
// |  8 | TestInjectRecordingRules_Idempotent                     | happy    | inject twice with same result                      | second inject returns count=0, templates unchanged                           |
// |  9 | TestGenerateRecordingRules_RuleCountMatchesRules        | happy    | non-empty graph, default opts                      | result.RuleCount == len(result.Rules)                                        |
// | 10 | TestGenerateRecordingRules_NOTESTxt                     | happy    | non-empty graph, valid opts                        | NOTESTxt non-empty and mentions recording rules or prometheus                |

import (
	"fmt"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// defaultRecordingRulesOpts returns a baseline RecordingRulesOptions for reuse.
func defaultRecordingRulesOpts() RecordingRulesOptions {
	return RecordingRulesOptions{
		Namespace:           "default",
		Quantiles:           []float64{0.5, 0.95, 0.99},
		AggregationInterval: "5m",
	}
}

// buildRecordingRulesGraph creates a resource graph with one Deployment.
func buildRecordingRulesGraph(name, namespace string) *types.ResourceGraph {
	r := makeProcessedResource("Deployment", name, namespace, map[string]string{"app": name})
	return buildGraph([]*types.ProcessedResource{r}, nil)
}

// allRecordingRulesContent flattens all Rules values into one string.
func allRecordingRulesContent(result *RecordingRulesResult) string {
	if result == nil {
		return ""
	}
	var sb strings.Builder
	for _, v := range result.Rules {
		sb.WriteString(v)
		sb.WriteString("\n")
	}
	return sb.String()
}

// ── Test 1: default quantiles p50/p95/p99 using histogram_quantile ───────────

func TestGenerateRecordingRules_P50P95P99Default(t *testing.T) {
	graph := buildRecordingRulesGraph("myapp", "default")
	opts := defaultRecordingRulesOpts()

	result := GenerateRecordingRules(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil RecordingRulesResult")
	}
	if len(result.Rules) == 0 {
		t.Fatal("expected Rules to be populated for non-empty graph with default quantiles")
	}
	content := allRecordingRulesContent(result)

	for _, quantile := range []string{"0.5", "0.95", "0.99"} {
		if !strings.Contains(content, quantile) {
			t.Errorf("Rules must contain quantile %q for histogram_quantile, got:\n%s", quantile, content)
		}
	}
	if !strings.Contains(content, "histogram_quantile") {
		t.Errorf("Rules must use 'histogram_quantile' PromQL function, got:\n%s", content)
	}
}

// ── Test 2: namespace appears in generated rules ──────────────────────────────

func TestGenerateRecordingRules_NamespaceInRules(t *testing.T) {
	graph := buildRecordingRulesGraph("myapp", "production")
	opts := RecordingRulesOptions{
		Namespace:           "production",
		Quantiles:           []float64{0.5, 0.95, 0.99},
		AggregationInterval: "5m",
	}

	result := GenerateRecordingRules(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil RecordingRulesResult")
	}
	content := allRecordingRulesContent(result)
	if !strings.Contains(content, "production") {
		t.Errorf("Rules must reference namespace 'production', got:\n%s", content)
	}
}

// ── Test 3: custom quantiles (0.5, 0.75, 0.99) all appear in rules ────────────

func TestGenerateRecordingRules_CustomQuantiles(t *testing.T) {
	graph := buildRecordingRulesGraph("myapp", "default")
	opts := RecordingRulesOptions{
		Namespace:           "default",
		Quantiles:           []float64{0.5, 0.75, 0.99},
		AggregationInterval: "5m",
	}

	result := GenerateRecordingRules(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil RecordingRulesResult")
	}
	content := allRecordingRulesContent(result)
	for _, q := range []float64{0.5, 0.75, 0.99} {
		qStr := fmt.Sprintf("%g", q)
		if !strings.Contains(content, qStr) {
			t.Errorf("Rules must contain custom quantile %q, got:\n%s", qStr, content)
		}
	}
}

// ── Test 4: aggregation interval appears in rules ────────────────────────────

func TestGenerateRecordingRules_AggregationInterval(t *testing.T) {
	graph := buildRecordingRulesGraph("myapp", "default")
	opts := RecordingRulesOptions{
		Namespace:           "default",
		Quantiles:           []float64{0.5, 0.99},
		AggregationInterval: "10m",
	}

	result := GenerateRecordingRules(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil RecordingRulesResult")
	}
	content := allRecordingRulesContent(result)
	if !strings.Contains(content, "10m") {
		t.Errorf("Rules must reference aggregation interval '10m', got:\n%s", content)
	}
}

// ── Test 5: empty graph returns non-nil result with RuleCount=0 ───────────────

func TestGenerateRecordingRules_EmptyGraph(t *testing.T) {
	graph := types.NewResourceGraph()
	opts := defaultRecordingRulesOpts()

	result := GenerateRecordingRules(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil RecordingRulesResult for empty graph")
	}
	if result.Rules == nil {
		t.Error("Rules map must be non-nil even for empty graph")
	}
	if result.RuleCount != 0 {
		t.Errorf("expected RuleCount=0 for empty graph, got %d", result.RuleCount)
	}
}

// ── Test 6: nil chart returns (nil, 0) ───────────────────────────────────────

func TestInjectRecordingRules_NilChart(t *testing.T) {
	graph := buildRecordingRulesGraph("myapp", "default")
	rResult := GenerateRecordingRules(graph, defaultRecordingRulesOpts())

	newChart, count := InjectRecordingRules(nil, rResult)

	if newChart != nil {
		t.Errorf("expected nil chart for nil input, got %+v", newChart)
	}
	if count != 0 {
		t.Errorf("expected count=0 for nil chart, got %d", count)
	}
}

// ── Test 7: InjectRecordingRules is copy-on-write ─────────────────────────────

func TestInjectRecordingRules_CopyOnWrite(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})
	originalCount := len(chart.Templates)

	graph := buildRecordingRulesGraph("myapp", "default")
	rResult := GenerateRecordingRules(graph, defaultRecordingRulesOpts())

	newChart, _ := InjectRecordingRules(chart, rResult)

	if newChart == nil {
		t.Fatal("expected non-nil new chart")
	}
	if newChart == chart {
		t.Error("InjectRecordingRules must return a new chart (copy-on-write)")
	}
	if len(chart.Templates) != originalCount {
		t.Errorf("original chart.Templates mutated: had %d templates, now %d",
			originalCount, len(chart.Templates))
	}
	for path := range chart.Templates {
		if strings.Contains(path, "recording") || strings.Contains(path, "prometheusrule") {
			t.Errorf("original chart gained recording rules template %q — copy-on-write violated", path)
		}
	}
}

// ── Test 8: InjectRecordingRules idempotent ───────────────────────────────────

func TestInjectRecordingRules_Idempotent(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})

	graph := buildRecordingRulesGraph("myapp", "default")
	rResult := GenerateRecordingRules(graph, defaultRecordingRulesOpts())

	firstChart, firstCount := InjectRecordingRules(chart, rResult)
	if firstChart == nil {
		t.Fatal("first inject returned nil chart")
	}
	if firstCount == 0 {
		t.Error("expected firstCount > 0 on first recording rules inject")
	}

	secondChart, secondCount := InjectRecordingRules(firstChart, rResult)
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

// ── Test 9: RuleCount == len(Rules) ──────────────────────────────────────────

func TestGenerateRecordingRules_RuleCountMatchesRules(t *testing.T) {
	graph := buildRecordingRulesGraph("myapp", "default")
	opts := defaultRecordingRulesOpts()

	result := GenerateRecordingRules(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil RecordingRulesResult")
	}
	if result.RuleCount != len(result.Rules) {
		t.Errorf("RuleCount=%d does not match len(Rules)=%d", result.RuleCount, len(result.Rules))
	}
}

// ── Test 10: NOTESTxt non-empty and mentions recording rules / prometheus ─────

func TestGenerateRecordingRules_NOTESTxt(t *testing.T) {
	graph := buildRecordingRulesGraph("myapp", "default")
	opts := defaultRecordingRulesOpts()

	result := GenerateRecordingRules(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil RecordingRulesResult")
	}
	if result.NOTESTxt == "" {
		t.Error("expected NOTESTxt to be non-empty")
	}
	lower := strings.ToLower(result.NOTESTxt)
	if !strings.Contains(lower, "record") && !strings.Contains(lower, "prometheus") &&
		!strings.Contains(lower, "rule") && !strings.Contains(lower, "sli") {
		t.Errorf("NOTESTxt must mention recording rules/prometheus/SLI context, got: %s", result.NOTESTxt)
	}
}
