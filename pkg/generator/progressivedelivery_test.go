package generator

// ============================================================
// Test Plan: Progressive Delivery Generator (Task 5.5.4)
// ============================================================
//
// | #  | Test Name                                                        | Category    | Input                                      | Expected Output                                                     |
// |----|------------------------------------------------------------------|-------------|--------------------------------------------|---------------------------------------------------------------------|
// |  1 | TestSuggestProgressiveDelivery_DeploymentReplicas3               | happy       | Deployment replicas=3                      | Rollout YAML generated for the Deployment                           |
// |  2 | TestSuggestProgressiveDelivery_DeploymentReplicas1Skipped        | edge        | Deployment replicas=1, MinReplicas=2       | Deployment NOT in Candidates                                        |
// |  3 | TestSuggestProgressiveDelivery_DefaultCanarySteps                | happy       | no custom steps                            | Rollout contains weights 20, 50, 100                                |
// |  4 | TestSuggestProgressiveDelivery_CustomCanarySteps                 | happy       | custom steps {10, "2m"}, {90, ""}          | Rollout YAML reflects custom step weights                           |
// |  5 | TestSuggestProgressiveDelivery_AnalysisTemplateIncluded          | happy       | IncludeAnalysisTemplate=true               | AnalysisTemplates map is non-empty                                  |
// |  6 | TestSuggestProgressiveDelivery_AnalysisTemplateExcluded          | happy       | IncludeAnalysisTemplate=false              | AnalysisTemplates map is empty                                      |
// |  7 | TestSuggestProgressiveDelivery_EmptyGraph                        | edge        | empty resource graph                       | Candidates empty, Rollouts empty                                    |
// |  8 | TestSuggestProgressiveDelivery_MultipleDeployments               | happy       | 3 Deployments (2 eligible, 1 too small)    | 2 Rollouts generated, 1 skipped                                     |
// |  9 | TestSuggestProgressiveDelivery_NOTESTxtContainsCandidates        | happy       | Deployment replicas=3                      | NOTESTxt contains Deployment name                                   |
// | 10 | TestInjectProgressiveDelivery_AddsRolloutTemplates               | happy       | chart + result with Rollout YAML           | chart.Templates gains rollout template key                          |
// | 11 | TestInjectProgressiveDelivery_CopyOnWrite                        | happy       | original chart                             | original.Templates unchanged after inject                           |
// | 12 | TestInjectProgressiveDelivery_Idempotent                         | happy       | inject twice same result                   | second inject count=0, no duplicate keys                            |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// makeDeploymentWithReplicas creates a Deployment ProcessedResource with the given replica count.
func makeDeploymentWithReplicas(name, namespace string, replicas int64) *types.ProcessedResource {
	r := makeProcessedResource("Deployment", name, namespace, nil)
	r.Original.Object.Object["spec"] = map[string]interface{}{
		"replicas": replicas,
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"app": name},
		},
		"template": map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{"app": name},
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"image": name + ":latest",
					},
				},
			},
		},
	}
	return r
}

// ── 1: Deployment with replicas=3 → Rollout generated ────────────────────────

func TestSuggestProgressiveDelivery_DeploymentReplicas3(t *testing.T) {
	deploy := makeDeploymentWithReplicas("myapp", "default", 3)
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	opts := ProgressiveDeliveryOptions{
		MinReplicas:             2,
		IncludeAnalysisTemplate: false,
	}

	result := SuggestProgressiveDelivery(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil ProgressiveDeliveryResult")
	}
	if len(result.Rollouts) == 0 {
		t.Fatal("expected at least 1 Rollout generated for Deployment with replicas=3")
	}
	found := false
	for _, yaml := range result.Rollouts {
		if strings.Contains(yaml, "myapp") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Rollout YAML to reference 'myapp', got rollouts: %v", result.Rollouts)
	}
}

// ── 2: Deployment with replicas=1 → skipped ──────────────────────────────────

func TestSuggestProgressiveDelivery_DeploymentReplicas1Skipped(t *testing.T) {
	deploy := makeDeploymentWithReplicas("tiny-app", "default", 1)
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	opts := ProgressiveDeliveryOptions{
		MinReplicas:             2,
		IncludeAnalysisTemplate: false,
	}

	result := SuggestProgressiveDelivery(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil result even when no candidates")
	}
	for _, c := range result.Candidates {
		if strings.Contains(c, "tiny-app") {
			t.Errorf("tiny-app (replicas=1) should NOT appear in Candidates, got: %v", result.Candidates)
		}
	}
	if _, ok := result.Rollouts["tiny-app"]; ok {
		t.Errorf("tiny-app should NOT have a generated Rollout")
	}
}

// ── 3: Default canary steps contain 20 / 50 / 100 ───────────────────────────

func TestSuggestProgressiveDelivery_DefaultCanarySteps(t *testing.T) {
	deploy := makeDeploymentWithReplicas("canary-app", "default", 3)
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	opts := ProgressiveDeliveryOptions{
		MinReplicas:             2,
		IncludeAnalysisTemplate: false,
		// CanarySteps intentionally empty → defaults should be used
	}

	result := SuggestProgressiveDelivery(graph, opts)

	if len(result.Rollouts) == 0 {
		t.Fatal("expected Rollout for canary-app")
	}
	for _, yaml := range result.Rollouts {
		for _, pct := range []string{"20", "50", "100"} {
			if !strings.Contains(yaml, pct) {
				t.Errorf("default canary Rollout YAML missing weight %s%%:\n%s", pct, yaml)
			}
		}
	}
}

// ── 4: Custom canary steps are reflected in Rollout YAML ─────────────────────

func TestSuggestProgressiveDelivery_CustomCanarySteps(t *testing.T) {
	deploy := makeDeploymentWithReplicas("custom-app", "default", 4)
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	opts := ProgressiveDeliveryOptions{
		MinReplicas: 2,
		CanarySteps: []CanaryStep{
			{Weight: 10, Pause: "2m"},
			{Weight: 90, Pause: ""},
		},
		IncludeAnalysisTemplate: false,
	}

	result := SuggestProgressiveDelivery(graph, opts)

	if len(result.Rollouts) == 0 {
		t.Fatal("expected Rollout for custom-app")
	}
	for _, yaml := range result.Rollouts {
		if !strings.Contains(yaml, "10") {
			t.Errorf("Rollout YAML missing weight 10:\n%s", yaml)
		}
		if !strings.Contains(yaml, "90") {
			t.Errorf("Rollout YAML missing weight 90:\n%s", yaml)
		}
	}
}

// ── 5: AnalysisTemplate included when requested ───────────────────────────────

func TestSuggestProgressiveDelivery_AnalysisTemplateIncluded(t *testing.T) {
	deploy := makeDeploymentWithReplicas("analyzed-app", "default", 3)
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	opts := ProgressiveDeliveryOptions{
		MinReplicas:             2,
		IncludeAnalysisTemplate: true,
	}

	result := SuggestProgressiveDelivery(graph, opts)

	if len(result.AnalysisTemplates) == 0 {
		t.Error("expected non-empty AnalysisTemplates when IncludeAnalysisTemplate=true")
	}
	for _, yaml := range result.AnalysisTemplates {
		if !strings.Contains(yaml, "AnalysisTemplate") {
			t.Errorf("AnalysisTemplate YAML missing kind 'AnalysisTemplate':\n%s", yaml)
		}
	}
}

// ── 6: AnalysisTemplate excluded when not requested ──────────────────────────

func TestSuggestProgressiveDelivery_AnalysisTemplateExcluded(t *testing.T) {
	deploy := makeDeploymentWithReplicas("plain-app", "default", 3)
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	opts := ProgressiveDeliveryOptions{
		MinReplicas:             2,
		IncludeAnalysisTemplate: false,
	}

	result := SuggestProgressiveDelivery(graph, opts)

	if len(result.AnalysisTemplates) != 0 {
		t.Errorf("expected empty AnalysisTemplates when IncludeAnalysisTemplate=false, got: %v", result.AnalysisTemplates)
	}
}

// ── 7: Empty graph → empty result ────────────────────────────────────────────

func TestSuggestProgressiveDelivery_EmptyGraph(t *testing.T) {
	graph := types.NewResourceGraph()

	opts := ProgressiveDeliveryOptions{
		MinReplicas:             2,
		IncludeAnalysisTemplate: true,
	}

	result := SuggestProgressiveDelivery(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil result for empty graph")
	}
	if len(result.Candidates) != 0 {
		t.Errorf("expected 0 candidates for empty graph, got %d", len(result.Candidates))
	}
	if len(result.Rollouts) != 0 {
		t.Errorf("expected 0 rollouts for empty graph, got %d", len(result.Rollouts))
	}
}

// ── 8: Multiple Deployments — 2 eligible, 1 too small ────────────────────────

func TestSuggestProgressiveDelivery_MultipleDeployments(t *testing.T) {
	big1 := makeDeploymentWithReplicas("svc-a", "default", 3)
	big2 := makeDeploymentWithReplicas("svc-b", "default", 5)
	small := makeDeploymentWithReplicas("svc-tiny", "default", 1)
	graph := buildGraph([]*types.ProcessedResource{big1, big2, small}, nil)

	opts := ProgressiveDeliveryOptions{
		MinReplicas:             2,
		IncludeAnalysisTemplate: false,
	}

	result := SuggestProgressiveDelivery(graph, opts)

	if len(result.Rollouts) != 2 {
		t.Errorf("expected 2 Rollouts (svc-a + svc-b), got %d: %v", len(result.Rollouts), result.Rollouts)
	}
	if _, ok := result.Rollouts["svc-tiny"]; ok {
		t.Error("svc-tiny (replicas=1) must not have a Rollout")
	}
}

// ── 9: NOTESTxt contains candidate names ─────────────────────────────────────

func TestSuggestProgressiveDelivery_NOTESTxtContainsCandidates(t *testing.T) {
	deploy := makeDeploymentWithReplicas("notes-app", "default", 4)
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	opts := ProgressiveDeliveryOptions{
		MinReplicas:             2,
		IncludeAnalysisTemplate: false,
	}

	result := SuggestProgressiveDelivery(graph, opts)

	if !strings.Contains(result.NOTESTxt, "notes-app") {
		t.Errorf("NOTESTxt should contain candidate 'notes-app', got:\n%s", result.NOTESTxt)
	}
}

// ── 10: InjectProgressiveDelivery adds rollout template keys ─────────────────

func TestInjectProgressiveDelivery_AddsRolloutTemplates(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})
	result := &ProgressiveDeliveryResult{
		Rollouts: map[string]string{
			"test-app": "apiVersion: argoproj.io/v1alpha1\nkind: Rollout\nmetadata:\n  name: test-app\n",
		},
		AnalysisTemplates: map[string]string{},
		Candidates:        []string{"test-app"},
	}

	newChart, count := InjectProgressiveDelivery(chart, result)

	if count == 0 {
		t.Error("expected count > 0 after InjectProgressiveDelivery")
	}
	found := false
	for k, v := range newChart.Templates {
		if strings.Contains(k, "rollout") && strings.Contains(v, "Rollout") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a rollout template in chart.Templates, got keys: %v", keysOf(newChart.Templates))
	}
}

// ── 11: InjectProgressiveDelivery is copy-on-write ───────────────────────────

func TestInjectProgressiveDelivery_CopyOnWrite(t *testing.T) {
	origTemplates := map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	}
	chart := newTestChart(origTemplates)
	result := &ProgressiveDeliveryResult{
		Rollouts: map[string]string{
			"test-app": "apiVersion: argoproj.io/v1alpha1\nkind: Rollout\nmetadata:\n  name: test-app\n",
		},
		AnalysisTemplates: map[string]string{},
		Candidates:        []string{"test-app"},
	}

	newChart, _ := InjectProgressiveDelivery(chart, result)

	if newChart == chart {
		t.Error("InjectProgressiveDelivery must return a new chart (copy-on-write)")
	}
	if len(chart.Templates) != 1 {
		t.Errorf("original chart.Templates modified: expected 1 key, got %d", len(chart.Templates))
	}
	if chart.Templates["templates/deployment.yaml"] != testDeploymentTemplate {
		t.Error("original deployment template must not be modified")
	}
}

// ── 12: InjectProgressiveDelivery is idempotent ──────────────────────────────

func TestInjectProgressiveDelivery_Idempotent(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})
	result := &ProgressiveDeliveryResult{
		Rollouts: map[string]string{
			"test-app": "apiVersion: argoproj.io/v1alpha1\nkind: Rollout\nmetadata:\n  name: test-app\n",
		},
		AnalysisTemplates: map[string]string{},
		Candidates:        []string{"test-app"},
	}

	firstChart, firstCount := InjectProgressiveDelivery(chart, result)
	if firstCount == 0 {
		t.Errorf("expected first inject to add templates, got count=%d", firstCount)
	}
	_, secondCount := InjectProgressiveDelivery(firstChart, result)

	if secondCount != 0 {
		t.Errorf("second inject should be idempotent (count=0), got count=%d", secondCount)
	}
	// Template count must not grow on second inject.
	keysBefore := len(firstChart.Templates)
	secondChart, _ := InjectProgressiveDelivery(firstChart, result)
	if len(secondChart.Templates) != keysBefore {
		t.Errorf("idempotent inject must not add duplicate templates: before=%d after=%d", keysBefore, len(secondChart.Templates))
	}
}

// ── local helper ─────────────────────────────────────────────────────────────

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
