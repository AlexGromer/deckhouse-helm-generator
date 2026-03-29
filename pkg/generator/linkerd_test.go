package generator

// ============================================================
// Test Plan: Linkerd Service Mesh Generator (Task 5.8.3)
// ============================================================
//
// | #  | Test Name                                              | Category    | Input                                        | Expected Output                                                |
// |----|--------------------------------------------------------|-------------|----------------------------------------------|----------------------------------------------------------------|
// |  1 | TestInjectLinkerdAnnotations_AnnotationAdded           | happy       | chart with Deployment, InjectAnnotation=true | linkerd.io/inject=enabled present in template                  |
// |  2 | TestGenerateLinkerdConfig_ServiceProfileTimeout        | happy       | ServiceProfiles=true, DefaultTimeout="5s"    | ServiceProfile YAML contains timeout "5s"                      |
// |  3 | TestGenerateLinkerdConfig_ServiceProfileRetries        | happy       | ServiceProfiles=true, DefaultRetries=3       | ServiceProfile YAML contains retries config                    |
// |  4 | TestGenerateLinkerdConfig_TrafficSplitMultiVersion     | happy       | TrafficSplit=true, 2 Deployments same name   | TrafficSplit YAML generated                                    |
// |  5 | TestInjectLinkerdAnnotations_NilChart                  | error       | nil chart                                    | (nil, 0), no panic                                             |
// |  6 | TestGenerateLinkerdConfig_EmptyGraph                   | edge        | empty graph, all options true                | Templates map empty, no panic                                  |
// |  7 | TestInjectLinkerdAnnotations_CopyOnWrite               | happy       | chart with templates                         | original.Templates unchanged after inject                      |
// |  8 | TestGenerateLinkerdConfig_NOTESTxtContainsUsage        | happy       | any non-empty result                         | NOTESTxt mentions "linkerd" usage instructions                 |
// |  9 | TestGenerateLinkerdConfig_NoServiceProfileWhenDisabled | happy       | ServiceProfiles=false                        | no ServiceProfile in Templates                                 |
// | 10 | TestGenerateLinkerdConfig_NoTrafficSplitWhenDisabled   | happy       | TrafficSplit=false                           | no TrafficSplit in Templates                                   |
// | 11 | TestInjectLinkerdAnnotations_NoAnnotationWhenDisabled  | happy       | InjectAnnotation=false                       | linkerd.io/inject NOT present in any template                  |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── 1: inject annotation present when InjectAnnotation=true ──────────────────

func TestInjectLinkerdAnnotations_AnnotationAdded(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})
	opts := LinkerdOptions{
		InjectAnnotation: true,
		ServiceProfiles:  false,
		TrafficSplit:     false,
		DefaultTimeout:   "5s",
		DefaultRetries:   2,
	}

	newChart, count := InjectLinkerdAnnotations(chart, opts)

	if count == 0 {
		t.Error("expected count > 0 when injecting Linkerd annotation")
	}
	found := false
	for _, content := range newChart.Templates {
		if strings.Contains(content, "linkerd.io/inject") && strings.Contains(content, "enabled") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'linkerd.io/inject: enabled' in at least one template")
	}
}

// ── 2: ServiceProfile YAML contains configured timeout ───────────────────────

func TestGenerateLinkerdConfig_ServiceProfileTimeout(t *testing.T) {
	svc := makeProcessedResource("Service", "myapp", "default", nil)
	graph := buildGraph([]*types.ProcessedResource{svc}, nil)

	opts := LinkerdOptions{
		InjectAnnotation: false,
		ServiceProfiles:  true,
		TrafficSplit:     false,
		DefaultTimeout:   "5s",
		DefaultRetries:   0,
	}

	result := GenerateLinkerdConfig(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil LinkerdResult")
	}
	if len(result.Templates) == 0 {
		t.Fatal("expected at least one template for ServiceProfile")
	}
	for _, yaml := range result.Templates {
		if strings.Contains(yaml, "ServiceProfile") {
			if !strings.Contains(yaml, "5s") {
				t.Errorf("ServiceProfile YAML must contain timeout '5s':\n%s", yaml)
			}
			return
		}
	}
	t.Error("no ServiceProfile template found in result")
}

// ── 3: ServiceProfile YAML contains retries configuration ────────────────────

func TestGenerateLinkerdConfig_ServiceProfileRetries(t *testing.T) {
	svc := makeProcessedResource("Service", "retries-svc", "default", nil)
	graph := buildGraph([]*types.ProcessedResource{svc}, nil)

	opts := LinkerdOptions{
		InjectAnnotation: false,
		ServiceProfiles:  true,
		TrafficSplit:     false,
		DefaultTimeout:   "10s",
		DefaultRetries:   3,
	}

	result := GenerateLinkerdConfig(graph, opts)

	for _, yaml := range result.Templates {
		if strings.Contains(yaml, "ServiceProfile") {
			if !strings.Contains(yaml, "retries") && !strings.Contains(yaml, "isRetryable") {
				t.Errorf("ServiceProfile YAML must contain retries config (DefaultRetries=3):\n%s", yaml)
			}
			return
		}
	}
	t.Error("no ServiceProfile template found in result")
}

// ── 4: TrafficSplit generated for multi-version services ─────────────────────

func TestGenerateLinkerdConfig_TrafficSplitMultiVersion(t *testing.T) {
	// Two Deployments simulating stable and canary versions of the same service.
	stable := makeProcessedResource("Deployment", "webapp-stable", "default", map[string]string{"app": "webapp", "version": "stable"})
	canary := makeProcessedResource("Deployment", "webapp-canary", "default", map[string]string{"app": "webapp", "version": "canary"})
	svc := makeProcessedResource("Service", "webapp", "default", map[string]string{"app": "webapp"})
	graph := buildGraph([]*types.ProcessedResource{stable, canary, svc}, nil)

	opts := LinkerdOptions{
		InjectAnnotation: false,
		ServiceProfiles:  false,
		TrafficSplit:     true,
		DefaultTimeout:   "",
		DefaultRetries:   0,
	}

	result := GenerateLinkerdConfig(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil LinkerdResult")
	}
	found := false
	for _, yaml := range result.Templates {
		if strings.Contains(yaml, "TrafficSplit") || strings.Contains(yaml, "trafficsplit") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected TrafficSplit template in result templates, got keys: %v", keysOf(result.Templates))
	}
}

// ── 5: nil chart returns nil, 0 ───────────────────────────────────────────────

func TestInjectLinkerdAnnotations_NilChart(t *testing.T) {
	opts := LinkerdOptions{
		InjectAnnotation: true,
		ServiceProfiles:  false,
		TrafficSplit:     false,
	}

	newChart, count := InjectLinkerdAnnotations(nil, opts)

	if newChart != nil {
		t.Errorf("expected nil chart for nil input, got %+v", newChart)
	}
	if count != 0 {
		t.Errorf("expected count=0 for nil chart, got %d", count)
	}
}

// ── 6: empty graph returns empty result without panic ─────────────────────────

func TestGenerateLinkerdConfig_EmptyGraph(t *testing.T) {
	graph := types.NewResourceGraph()

	opts := LinkerdOptions{
		InjectAnnotation: true,
		ServiceProfiles:  true,
		TrafficSplit:     true,
		DefaultTimeout:   "5s",
		DefaultRetries:   2,
	}

	result := GenerateLinkerdConfig(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil result even for empty graph")
	}
	if len(result.Templates) != 0 {
		t.Errorf("expected 0 templates for empty graph, got %d", len(result.Templates))
	}
}

// ── 7: InjectLinkerdAnnotations is copy-on-write ─────────────────────────────

func TestInjectLinkerdAnnotations_CopyOnWrite(t *testing.T) {
	originalContent := testDeploymentTemplate
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": originalContent,
	})
	opts := LinkerdOptions{
		InjectAnnotation: true,
	}

	newChart, _ := InjectLinkerdAnnotations(chart, opts)

	if newChart == chart {
		t.Error("InjectLinkerdAnnotations must return a new chart (copy-on-write)")
	}
	if chart.Templates["templates/deployment.yaml"] != originalContent {
		t.Error("original chart template must not be modified by InjectLinkerdAnnotations")
	}
}

// ── 8: NOTESTxt mentions linkerd usage ───────────────────────────────────────

func TestGenerateLinkerdConfig_NOTESTxtContainsUsage(t *testing.T) {
	svc := makeProcessedResource("Service", "info-svc", "default", nil)
	graph := buildGraph([]*types.ProcessedResource{svc}, nil)

	opts := LinkerdOptions{
		InjectAnnotation: true,
		ServiceProfiles:  true,
		TrafficSplit:     false,
		DefaultTimeout:   "5s",
		DefaultRetries:   1,
	}

	result := GenerateLinkerdConfig(graph, opts)

	lower := strings.ToLower(result.NOTESTxt)
	if !strings.Contains(lower, "linkerd") {
		t.Errorf("NOTESTxt should contain 'linkerd' usage instructions, got:\n%s", result.NOTESTxt)
	}
}

// ── 9: ServiceProfiles=false → no ServiceProfile template ────────────────────

func TestGenerateLinkerdConfig_NoServiceProfileWhenDisabled(t *testing.T) {
	svc := makeProcessedResource("Service", "nosvc-profile", "default", nil)
	graph := buildGraph([]*types.ProcessedResource{svc}, nil)

	opts := LinkerdOptions{
		InjectAnnotation: false,
		ServiceProfiles:  false,
		TrafficSplit:     false,
	}

	result := GenerateLinkerdConfig(graph, opts)

	for k, v := range result.Templates {
		if strings.Contains(k, "serviceprofile") || strings.Contains(v, "ServiceProfile") {
			t.Errorf("ServiceProfile must not be generated when ServiceProfiles=false, but found key=%s", k)
		}
	}
}

// ── 10: TrafficSplit=false → no TrafficSplit template ────────────────────────

func TestGenerateLinkerdConfig_NoTrafficSplitWhenDisabled(t *testing.T) {
	deploy := makeProcessedResource("Deployment", "no-split", "default", nil)
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	opts := LinkerdOptions{
		InjectAnnotation: false,
		ServiceProfiles:  false,
		TrafficSplit:     false,
	}

	result := GenerateLinkerdConfig(graph, opts)

	for k, v := range result.Templates {
		if strings.Contains(k, "trafficsplit") || strings.Contains(v, "TrafficSplit") {
			t.Errorf("TrafficSplit must not be generated when TrafficSplit=false, but found key=%s", k)
		}
	}
}

// ── 11: InjectAnnotation=false → annotation absent ───────────────────────────

func TestInjectLinkerdAnnotations_NoAnnotationWhenDisabled(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})
	opts := LinkerdOptions{
		InjectAnnotation: false,
	}

	newChart, count := InjectLinkerdAnnotations(chart, opts)

	if count != 0 {
		t.Errorf("expected count=0 when InjectAnnotation=false, got %d", count)
	}
	for _, content := range newChart.Templates {
		if strings.Contains(content, "linkerd.io/inject") {
			t.Errorf("linkerd.io/inject must not appear when InjectAnnotation=false")
		}
	}
}
