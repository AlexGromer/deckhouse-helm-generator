package generator

// ============================================================
// Test Plan: Advanced OTEL Auto-Instrumentation Generator (Task 5.9.4)
// ============================================================
//
// | #  | Test Name                                              | Category | Input                                              | Expected Output                                                              |
// |----|--------------------------------------------------------|----------|----------------------------------------------------|------------------------------------------------------------------------------|
// |  1 | TestGenerateAdvancedOTEL_ParentBasedSampling           | happy    | SamplingStrategy="parent-based", SamplingRate=0.5  | SamplingConfig contains "parent-based" and "0.5"                             |
// |  2 | TestGenerateAdvancedOTEL_RatioSampling                 | happy    | SamplingStrategy="ratio", SamplingRate=0.1         | SamplingConfig contains "ratio" and "0.1"                                    |
// |  3 | TestGenerateAdvancedOTEL_ResourceAttributesInCRD       | happy    | ResourceAttributes={"service.name":"api"}          | Instrumentation YAML contains "service.name" and "api"                       |
// |  4 | TestGenerateAdvancedOTEL_PropagatorsW3C                | happy    | Propagators=["tracecontext","baggage"]             | Instrumentations contain "tracecontext" and "baggage"                        |
// |  5 | TestGenerateAdvancedOTEL_PropagatorsB3                 | happy    | Propagators=["b3","b3multi"]                       | Instrumentations contain "b3"                                                |
// |  6 | TestGenerateAdvancedOTEL_AlwaysOnSampling              | happy    | SamplingStrategy="always-on"                       | SamplingConfig contains "always-on" or "AlwaysOn"                            |
// |  7 | TestGenerateAdvancedOTEL_AlwaysOffSampling             | happy    | SamplingStrategy="always-off"                      | SamplingConfig contains "always-off" or "AlwaysOff"                          |
// |  8 | TestGenerateAdvancedOTEL_EmptyGraph                    | edge     | empty graph                                        | non-nil result, Instrumentations is empty map                                |
// |  9 | TestGenerateAdvancedOTEL_CombinedConfig                | happy    | all opts set, graph with java+python workloads     | two Instrumentations produced, SamplingConfig set, ResourceAttrs in YAML     |
// | 10 | TestGenerateAdvancedOTEL_NOTESTxt                      | happy    | non-empty graph                                    | NOTESTxt is non-empty and mentions OTEL or instrumentation                   |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// makeWorkloadForAdvancedOTEL builds a Deployment resource with the given image.
func makeWorkloadForAdvancedOTEL(name, image string) *types.ProcessedResource {
	return makeDeploymentWithImage(name, image)
}

// defaultAdvancedOTELOpts returns a baseline AdvancedOTELOptions for reuse.
func defaultAdvancedOTELOpts() AdvancedOTELOptions {
	return AdvancedOTELOptions{
		SamplingStrategy: "parent-based",
		SamplingRate:     1.0,
		ResourceAttributes: map[string]string{
			"service.namespace": "default",
		},
		Propagators: []string{"tracecontext", "baggage"},
	}
}

// buildAdvancedOTELGraph creates a resource graph with a single java-image workload.
func buildAdvancedOTELGraph(name, image string) *types.ResourceGraph {
	r := makeWorkloadForAdvancedOTEL(name, image)
	return buildGraph([]*types.ProcessedResource{r}, nil)
}

// ── Test 1: parent-based sampling strategy ───────────────────────────────────

func TestGenerateAdvancedOTEL_ParentBasedSampling(t *testing.T) {
	graph := buildAdvancedOTELGraph("api", "openjdk:17")
	opts := AdvancedOTELOptions{
		SamplingStrategy:   "parent-based",
		SamplingRate:       0.5,
		ResourceAttributes: map[string]string{},
		Propagators:        []string{"tracecontext"},
	}

	result := GenerateAdvancedOTEL(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil AdvancedOTELResult")
	}
	foundStrategy := false
	foundRate := false
	for _, v := range result.SamplingConfig {
		if strings.Contains(v, "parent-based") || strings.Contains(strings.ToLower(v), "parentbased") {
			foundStrategy = true
		}
		if strings.Contains(v, "0.5") {
			foundRate = true
		}
	}
	if !foundStrategy {
		t.Errorf("SamplingConfig must reference 'parent-based' strategy, got: %v", result.SamplingConfig)
	}
	if !foundRate {
		t.Errorf("SamplingConfig must reference sampling rate '0.5', got: %v", result.SamplingConfig)
	}
}

// ── Test 2: ratio sampling strategy ─────────────────────────────────────────

func TestGenerateAdvancedOTEL_RatioSampling(t *testing.T) {
	graph := buildAdvancedOTELGraph("api", "openjdk:17")
	opts := AdvancedOTELOptions{
		SamplingStrategy:   "ratio",
		SamplingRate:       0.1,
		ResourceAttributes: map[string]string{},
		Propagators:        []string{"tracecontext"},
	}

	result := GenerateAdvancedOTEL(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil AdvancedOTELResult")
	}
	foundStrategy := false
	foundRate := false
	for _, v := range result.SamplingConfig {
		if strings.Contains(strings.ToLower(v), "ratio") || strings.Contains(strings.ToLower(v), "traceidratio") {
			foundStrategy = true
		}
		if strings.Contains(v, "0.1") {
			foundRate = true
		}
	}
	if !foundStrategy {
		t.Errorf("SamplingConfig must reference 'ratio' strategy, got: %v", result.SamplingConfig)
	}
	if !foundRate {
		t.Errorf("SamplingConfig must reference sampling rate '0.1', got: %v", result.SamplingConfig)
	}
}

// ── Test 3: resource attributes appear in Instrumentation CRD ────────────────

func TestGenerateAdvancedOTEL_ResourceAttributesInCRD(t *testing.T) {
	graph := buildAdvancedOTELGraph("api-svc", "openjdk:17")
	opts := AdvancedOTELOptions{
		SamplingStrategy: "parent-based",
		SamplingRate:     1.0,
		ResourceAttributes: map[string]string{
			"service.name":      "api-svc",
			"deployment.env":    "production",
		},
		Propagators: []string{"tracecontext"},
	}

	result := GenerateAdvancedOTEL(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil AdvancedOTELResult")
	}
	for workload, yaml := range result.Instrumentations {
		if !strings.Contains(yaml, "service.name") {
			t.Errorf("Instrumentation YAML for %q must contain resource attribute key 'service.name':\n%s", workload, yaml)
		}
		if !strings.Contains(yaml, "api-svc") {
			t.Errorf("Instrumentation YAML for %q must contain resource attribute value 'api-svc':\n%s", workload, yaml)
		}
	}
}

// ── Test 4: W3C propagators (tracecontext + baggage) ─────────────────────────

func TestGenerateAdvancedOTEL_PropagatorsW3C(t *testing.T) {
	graph := buildAdvancedOTELGraph("svc", "node:20")
	opts := AdvancedOTELOptions{
		SamplingStrategy:   "parent-based",
		SamplingRate:       1.0,
		ResourceAttributes: map[string]string{},
		Propagators:        []string{"tracecontext", "baggage"},
	}

	result := GenerateAdvancedOTEL(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil AdvancedOTELResult")
	}
	if len(result.Instrumentations) == 0 {
		t.Fatal("expected at least one Instrumentation YAML for node workload")
	}
	for workload, yaml := range result.Instrumentations {
		if !strings.Contains(yaml, "tracecontext") {
			t.Errorf("Instrumentation YAML for %q must contain propagator 'tracecontext':\n%s", workload, yaml)
		}
		if !strings.Contains(yaml, "baggage") {
			t.Errorf("Instrumentation YAML for %q must contain propagator 'baggage':\n%s", workload, yaml)
		}
	}
}

// ── Test 5: B3 propagators ───────────────────────────────────────────────────

func TestGenerateAdvancedOTEL_PropagatorsB3(t *testing.T) {
	graph := buildAdvancedOTELGraph("svc", "python:3.11")
	opts := AdvancedOTELOptions{
		SamplingStrategy:   "parent-based",
		SamplingRate:       1.0,
		ResourceAttributes: map[string]string{},
		Propagators:        []string{"b3", "b3multi"},
	}

	result := GenerateAdvancedOTEL(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil AdvancedOTELResult")
	}
	if len(result.Instrumentations) == 0 {
		t.Fatal("expected at least one Instrumentation YAML for python workload")
	}
	for workload, yaml := range result.Instrumentations {
		if !strings.Contains(yaml, "b3") {
			t.Errorf("Instrumentation YAML for %q must contain propagator 'b3':\n%s", workload, yaml)
		}
	}
}

// ── Test 6: always-on sampling ───────────────────────────────────────────────

func TestGenerateAdvancedOTEL_AlwaysOnSampling(t *testing.T) {
	graph := buildAdvancedOTELGraph("api", "openjdk:17")
	opts := AdvancedOTELOptions{
		SamplingStrategy:   "always-on",
		SamplingRate:       1.0,
		ResourceAttributes: map[string]string{},
		Propagators:        []string{"tracecontext"},
	}

	result := GenerateAdvancedOTEL(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil AdvancedOTELResult")
	}
	found := false
	for _, v := range result.SamplingConfig {
		lower := strings.ToLower(v)
		if strings.Contains(lower, "always-on") || strings.Contains(lower, "alwayson") || strings.Contains(lower, "always_on") {
			found = true
		}
	}
	if !found {
		t.Errorf("SamplingConfig must reference 'always-on' strategy, got: %v", result.SamplingConfig)
	}
}

// ── Test 7: always-off sampling ─────────────────────────────────────────────

func TestGenerateAdvancedOTEL_AlwaysOffSampling(t *testing.T) {
	graph := buildAdvancedOTELGraph("api", "openjdk:17")
	opts := AdvancedOTELOptions{
		SamplingStrategy:   "always-off",
		SamplingRate:       0.0,
		ResourceAttributes: map[string]string{},
		Propagators:        []string{"tracecontext"},
	}

	result := GenerateAdvancedOTEL(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil AdvancedOTELResult")
	}
	found := false
	for _, v := range result.SamplingConfig {
		lower := strings.ToLower(v)
		if strings.Contains(lower, "always-off") || strings.Contains(lower, "alwaysoff") || strings.Contains(lower, "always_off") {
			found = true
		}
	}
	if !found {
		t.Errorf("SamplingConfig must reference 'always-off' strategy, got: %v", result.SamplingConfig)
	}
}

// ── Test 8: empty graph produces empty Instrumentations ─────────────────────

func TestGenerateAdvancedOTEL_EmptyGraph(t *testing.T) {
	graph := types.NewResourceGraph()
	opts := defaultAdvancedOTELOpts()

	result := GenerateAdvancedOTEL(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil AdvancedOTELResult for empty graph")
	}
	if result.Instrumentations == nil {
		t.Fatal("Instrumentations map must be non-nil even for empty graph")
	}
	if len(result.Instrumentations) != 0 {
		t.Errorf("expected 0 Instrumentations for empty graph, got %d", len(result.Instrumentations))
	}
}

// ── Test 9: combined config — multiple workloads, all options set ─────────────

func TestGenerateAdvancedOTEL_CombinedConfig(t *testing.T) {
	java := makeWorkloadForAdvancedOTEL("java-api", "openjdk:17")
	py := makeWorkloadForAdvancedOTEL("py-worker", "python:3.11")
	graph := buildGraph([]*types.ProcessedResource{java, py}, nil)

	opts := AdvancedOTELOptions{
		SamplingStrategy: "ratio",
		SamplingRate:     0.25,
		ResourceAttributes: map[string]string{
			"team":        "platform",
			"environment": "staging",
		},
		Propagators: []string{"tracecontext", "b3"},
	}

	result := GenerateAdvancedOTEL(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil AdvancedOTELResult")
	}
	if len(result.Instrumentations) != 2 {
		t.Errorf("expected 2 Instrumentations (java-api + py-worker), got %d", len(result.Instrumentations))
	}
	if len(result.SamplingConfig) == 0 {
		t.Error("expected SamplingConfig to be populated for ratio strategy")
	}
	for workload, yaml := range result.Instrumentations {
		if !strings.Contains(yaml, "team") && !strings.Contains(yaml, "platform") &&
			!strings.Contains(yaml, "environment") && !strings.Contains(yaml, "staging") {
			t.Errorf("Instrumentation YAML for %q missing resource attributes:\n%s", workload, yaml)
		}
	}
}

// ── Test 10: NOTESTxt non-empty and mentions OTEL/instrumentation ─────────────

func TestGenerateAdvancedOTEL_NOTESTxt(t *testing.T) {
	graph := buildAdvancedOTELGraph("api", "openjdk:17")
	opts := defaultAdvancedOTELOpts()

	result := GenerateAdvancedOTEL(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil AdvancedOTELResult")
	}
	if result.NOTESTxt == "" {
		t.Error("expected NOTESTxt to be non-empty")
	}
	lower := strings.ToLower(result.NOTESTxt)
	if !strings.Contains(lower, "otel") && !strings.Contains(lower, "opentelemetry") &&
		!strings.Contains(lower, "instrument") && !strings.Contains(lower, "sampling") {
		t.Errorf("NOTESTxt must mention OTEL/instrumentation context, got: %s", result.NOTESTxt)
	}
}
