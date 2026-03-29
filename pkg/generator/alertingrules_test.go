package generator

// ============================================================
// Test Plan: Basic PrometheusRule Alerting Rules Generator (Task 5.9.3)
// ============================================================
//
// | #  | Test Name                                                     | Category    | Input                                               | Expected Output                                                         |
// |----|---------------------------------------------------------------|-------------|-----------------------------------------------------|-------------------------------------------------------------------------|
// |  1 | TestGenerateBasicAlerts_AllFiveRulesPresent                   | happy       | non-empty graph, default opts                       | Rules map contains all 5 rule keys                                      |
// |  2 | TestGenerateBasicAlerts_SeverityLabels                       | happy       | opts.Severity="critical"                            | each rule content contains severity="critical"                          |
// |  3 | TestGenerateBasicAlerts_RunbookURLPlaceholder                | happy       | opts.RunbookURLBase="https://runbooks.example.com"  | each rule content contains runbook_url with RunbookURLBase prefix       |
// |  4 | TestGenerateBasicAlerts_ForDuration                          | happy       | default opts                                        | rule content contains "for:" duration field                             |
// |  5 | TestGenerateBasicAlerts_NamespaceInRules                     | happy       | opts.Namespace="production"                         | rule content references "production" namespace                          |
// |  6 | TestGenerateBasicAlerts_EmptyGraph                           | edge        | empty graph                                         | returns non-nil AlertingResult, RuleCount>=5 (generic rules always)     |
// |  7 | TestInjectAlertingRules_CopyOnWrite                          | happy       | non-nil chart                                       | original chart.Templates unchanged after inject                         |
// |  8 | TestInjectAlertingRules_Idempotent                           | happy       | inject twice same result                            | template content unchanged on second inject, count 0 on second call     |
// |  9 | TestGenerateBasicAlerts_NOTESTxtSummary                      | happy       | non-empty result                                    | NOTESTxt field non-empty and mentions alerting/rules                    |
// | 10 | TestInjectAlertingRules_AddsTemplateToChart                  | happy       | chart with no alert templates                       | newChart.Templates contains at least one new template entry             |
// | 11 | TestGenerateBasicAlerts_RuleCountMatchesRulesMap             | happy       | default graph                                       | result.RuleCount == len(result.Rules)                                   |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Helpers
// ============================================================

// makeDeploymentForAlerts creates a minimal ProcessedResource representing a
// Deployment to populate the resource graph used in alerting rule tests.
func makeDeploymentForAlerts(name, namespace string) *types.ProcessedResource {
	r := makeProcessedResource("Deployment", name, namespace, map[string]string{"app": name})
	return r
}

// buildAlertingGraph creates a graph with a single Deployment workload.
func buildAlertingGraph(workloadName, namespace string) *types.ResourceGraph {
	r := makeDeploymentForAlerts(workloadName, namespace)
	return buildGraph([]*types.ProcessedResource{r}, nil)
}

// defaultAlertingOpts returns AlertingOptions with safe defaults for tests.
func defaultAlertingOpts() AlertingOptions {
	return AlertingOptions{
		Namespace:      "default",
		Severity:       "warning",
		RunbookURLBase: "https://runbooks.example.com",
	}
}

// requiredAlertRuleNames lists the five alerting rules the generator must produce.
var requiredAlertRuleNames = []string{
	"CrashLooping",
	"OOMKilled",
	"PodNotReady",
	"HighRestartCount",
	"ContainerWaiting",
}

// ============================================================
// Test 1: All 5 required rules present in result.Rules
// ============================================================

func TestGenerateBasicAlerts_AllFiveRulesPresent(t *testing.T) {
	graph := buildAlertingGraph("myapp", "default")
	opts := defaultAlertingOpts()

	result := GenerateBasicAlerts(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil AlertingResult")
	}

	for _, ruleName := range requiredAlertRuleNames {
		if _, ok := result.Rules[ruleName]; !ok {
			t.Errorf("expected rule %q to be present in result.Rules, got keys: %v",
				ruleName, alertRuleKeys(result.Rules))
		}
	}
}

// ============================================================
// Test 2: Severity label present in each rule
// ============================================================

func TestGenerateBasicAlerts_SeverityLabels(t *testing.T) {
	graph := buildAlertingGraph("myapp", "default")
	opts := defaultAlertingOpts()
	opts.Severity = "critical"

	result := GenerateBasicAlerts(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil AlertingResult")
	}

	for ruleName, ruleContent := range result.Rules {
		if !strings.Contains(ruleContent, "critical") {
			t.Errorf("rule %q does not contain severity 'critical': %s", ruleName, ruleContent)
		}
	}
}

// ============================================================
// Test 3: RunbookURL placeholder uses RunbookURLBase
// ============================================================

func TestGenerateBasicAlerts_RunbookURLPlaceholder(t *testing.T) {
	graph := buildAlertingGraph("myapp", "default")
	opts := defaultAlertingOpts()
	opts.RunbookURLBase = "https://runbooks.example.com"

	result := GenerateBasicAlerts(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil AlertingResult")
	}

	for ruleName, ruleContent := range result.Rules {
		if !strings.Contains(ruleContent, "https://runbooks.example.com") {
			t.Errorf("rule %q does not contain runbook URL base: %s", ruleName, ruleContent)
		}
	}
}

// ============================================================
// Test 4: Rules contain "for:" duration field
// ============================================================

func TestGenerateBasicAlerts_ForDuration(t *testing.T) {
	graph := buildAlertingGraph("myapp", "default")
	opts := defaultAlertingOpts()

	result := GenerateBasicAlerts(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil AlertingResult")
	}

	for ruleName, ruleContent := range result.Rules {
		if !strings.Contains(ruleContent, "for:") && !strings.Contains(ruleContent, "for ") {
			t.Errorf("rule %q is missing a 'for:' duration field: %s", ruleName, ruleContent)
		}
	}
}

// ============================================================
// Test 5: Namespace appears in rule content
// ============================================================

func TestGenerateBasicAlerts_NamespaceInRules(t *testing.T) {
	graph := buildAlertingGraph("myapp", "production")
	opts := defaultAlertingOpts()
	opts.Namespace = "production"

	result := GenerateBasicAlerts(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil AlertingResult")
	}

	found := false
	for _, ruleContent := range result.Rules {
		if strings.Contains(ruleContent, "production") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected at least one rule to reference namespace 'production'")
	}
}

// ============================================================
// Test 6: Empty graph → non-nil result with all 5 generic rules
// ============================================================

func TestGenerateBasicAlerts_EmptyGraph(t *testing.T) {
	emptyGraph := types.NewResourceGraph()
	opts := defaultAlertingOpts()

	result := GenerateBasicAlerts(emptyGraph, opts)

	if result == nil {
		t.Fatal("expected non-nil AlertingResult for empty graph")
	}
	if result.RuleCount < 5 {
		t.Errorf("expected RuleCount >= 5 for generic rules even with empty graph, got %d", result.RuleCount)
	}
	for _, ruleName := range requiredAlertRuleNames {
		if _, ok := result.Rules[ruleName]; !ok {
			t.Errorf("expected generic rule %q even with empty graph", ruleName)
		}
	}
}

// ============================================================
// Test 7: InjectAlertingRules copy-on-write
// ============================================================

func TestInjectAlertingRules_CopyOnWrite(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})
	originalTemplateCount := len(chart.Templates)

	graph := buildAlertingGraph("myapp", "default")
	result := GenerateBasicAlerts(graph, defaultAlertingOpts())

	newChart, _ := InjectAlertingRules(chart, result)

	if newChart == nil {
		t.Fatal("expected non-nil new chart")
	}
	// Original must still have the same number of templates
	if len(chart.Templates) != originalTemplateCount {
		t.Errorf("original chart.Templates mutated: had %d templates, now %d",
			originalTemplateCount, len(chart.Templates))
	}
	// Original must not have gained alerting template
	for path := range chart.Templates {
		if strings.Contains(path, "alert") || strings.Contains(path, "prometheusrule") {
			t.Errorf("original chart gained alerting template %q — copy-on-write violated", path)
		}
	}
}

// ============================================================
// Test 8: InjectAlertingRules idempotent — second call returns count=0
// ============================================================

func TestInjectAlertingRules_Idempotent(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})

	graph := buildAlertingGraph("myapp", "default")
	alertResult := GenerateBasicAlerts(graph, defaultAlertingOpts())

	firstChart, firstCount := InjectAlertingRules(chart, alertResult)
	if firstChart == nil {
		t.Fatal("first inject returned nil chart")
	}
	if firstCount == 0 {
		t.Error("expected firstCount > 0 on first inject")
	}

	secondChart, secondCount := InjectAlertingRules(firstChart, alertResult)
	if secondChart == nil {
		t.Fatal("second inject returned nil chart")
	}
	if secondCount != 0 {
		t.Errorf("expected secondCount=0 on idempotent inject, got %d", secondCount)
	}

	// Template content must be identical after second inject
	for path, content := range secondChart.Templates {
		if firstContent, ok := firstChart.Templates[path]; ok {
			if content != firstContent {
				t.Errorf("template %q changed on second inject — idempotency violated", path)
			}
		}
	}
}

// ============================================================
// Test 9: NOTESTxt is non-empty and mentions alerting/rules
// ============================================================

func TestGenerateBasicAlerts_NOTESTxtSummary(t *testing.T) {
	graph := buildAlertingGraph("myapp", "default")
	opts := defaultAlertingOpts()

	result := GenerateBasicAlerts(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil AlertingResult")
	}
	if result.NOTESTxt == "" {
		t.Error("expected NOTESTxt to be non-empty")
	}
	// The summary should mention alerting or PrometheusRule context
	lowered := strings.ToLower(result.NOTESTxt)
	if !strings.Contains(lowered, "alert") && !strings.Contains(lowered, "rule") && !strings.Contains(lowered, "prometheus") {
		t.Errorf("NOTESTxt should mention alerting/rules context, got: %s", result.NOTESTxt)
	}
}

// ============================================================
// Test 10: InjectAlertingRules adds template to chart
// ============================================================

func TestInjectAlertingRules_AddsTemplateToChart(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})

	graph := buildAlertingGraph("myapp", "default")
	alertResult := GenerateBasicAlerts(graph, defaultAlertingOpts())

	newChart, count := InjectAlertingRules(chart, alertResult)

	if newChart == nil {
		t.Fatal("expected non-nil chart")
	}
	if count == 0 {
		t.Error("expected count > 0 after injecting alert rules")
	}
	if len(newChart.Templates) <= len(chart.Templates) {
		t.Errorf("expected new chart to have more templates than original: original=%d, new=%d",
			len(chart.Templates), len(newChart.Templates))
	}
}

// ============================================================
// Test 11: RuleCount == len(Rules)
// ============================================================

func TestGenerateBasicAlerts_RuleCountMatchesRulesMap(t *testing.T) {
	graph := buildAlertingGraph("myapp", "default")
	opts := defaultAlertingOpts()

	result := GenerateBasicAlerts(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil AlertingResult")
	}
	if result.RuleCount != len(result.Rules) {
		t.Errorf("RuleCount=%d does not match len(Rules)=%d", result.RuleCount, len(result.Rules))
	}
}

// ============================================================
// Utility
// ============================================================

// alertRuleKeys returns the keys of a string map for diagnostic messages in alerting tests.
func alertRuleKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
