package generator

// ============================================================
// Test Plan: OpenTelemetry Instrumentation Generator (Task 5.9.1)
// ============================================================
//
// | #  | Test Name                                                  | Category    | Input                                       | Expected Output                                                      |
// |----|------------------------------------------------------------|-------------|---------------------------------------------|----------------------------------------------------------------------|
// |  1 | TestGenerateOTELInstrumentation_JavaDetection              | happy       | Deployment image contains "java"            | DetectedLanguages["<name>"]="java"                                   |
// |  2 | TestGenerateOTELInstrumentation_SpringDetection            | happy       | Deployment image contains "spring"          | DetectedLanguages["<name>"]="java"                                   |
// |  3 | TestGenerateOTELInstrumentation_PythonDetection            | happy       | Deployment image contains "python"          | DetectedLanguages["<name>"]="python"                                 |
// |  4 | TestGenerateOTELInstrumentation_NodejsDetection            | happy       | Deployment image contains "node"            | DetectedLanguages["<name>"]="nodejs"                                 |
// |  5 | TestGenerateOTELInstrumentation_GoDetection                | happy       | Deployment image contains "golang"/"go"     | DetectedLanguages["<name>"]="go"                                     |
// |  6 | TestGenerateOTELInstrumentation_DotnetDetection            | happy       | Deployment image contains "dotnet"/"aspnet" | DetectedLanguages["<name>"]="dotnet"                                 |
// |  7 | TestGenerateOTELInstrumentation_UnknownLanguage             | edge        | Deployment image "nginx:latest"             | DetectedLanguages["<name>"] absent or "unknown"                      |
// |  8 | TestGenerateOTELInstrumentation_CustomExporterEndpoint     | happy       | ExporterEndpoint="http://otelcol:4317"      | Instrumentation YAML contains the endpoint                          |
// |  9 | TestGenerateOTELInstrumentation_SamplingRate               | happy       | SamplingRate=0.25                           | Instrumentation YAML contains "0.25"                                |
// | 10 | TestGenerateOTELInstrumentation_PropagatorW3CAndB3         | happy       | Propagators=["tracecontext","b3"]           | Instrumentation YAML contains both propagators                      |
// | 11 | TestInjectOTELInstrumentation_NilChart                     | error       | nil chart, valid result                     | (nil, 0), no panic                                                   |
// | 12 | TestGenerateOTELInstrumentation_EmptyGraph                 | edge        | empty graph                                 | Instrumentations empty, DetectedLanguages empty                      |
// | 13 | TestInjectOTELInstrumentation_CopyOnWrite                  | happy       | chart + result                              | original.Templates unchanged after inject                            |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// makeDeploymentWithImageName wraps makeDeploymentWithImage for clearer OTEL tests.
func makeOTELDeployment(name, image string) *types.ProcessedResource {
	return makeDeploymentWithImage(name, image)
}

// defaultOTELOpts returns a minimal valid OTELOptions.
func defaultOTELOpts() OTELOptions {
	return OTELOptions{
		ExporterEndpoint: "http://otel-collector:4317",
		ExporterProtocol: "grpc",
		Propagators:      []string{"tracecontext", "baggage"},
		SamplingRate:     1.0,
	}
}

// ── 1: Java detection by image name ──────────────────────────────────────────

func TestGenerateOTELInstrumentation_JavaDetection(t *testing.T) {
	deploy := makeOTELDeployment("java-app", "openjdk:17-slim")
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result := GenerateOTELInstrumentation(graph, defaultOTELOpts())

	if result == nil {
		t.Fatal("expected non-nil OTELResult")
	}
	lang, ok := result.DetectedLanguages["java-app"]
	if !ok {
		t.Fatalf("expected language detected for 'java-app', DetectedLanguages=%v", result.DetectedLanguages)
	}
	if lang != "java" {
		t.Errorf("expected language 'java' for openjdk image, got %q", lang)
	}
}

// ── 2: Spring image → java ───────────────────────────────────────────────────

func TestGenerateOTELInstrumentation_SpringDetection(t *testing.T) {
	deploy := makeOTELDeployment("spring-app", "springboot:2.7")
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result := GenerateOTELInstrumentation(graph, defaultOTELOpts())

	lang, ok := result.DetectedLanguages["spring-app"]
	if !ok {
		t.Fatalf("expected language detected for 'spring-app', DetectedLanguages=%v", result.DetectedLanguages)
	}
	if lang != "java" {
		t.Errorf("expected language 'java' for spring image, got %q", lang)
	}
}

// ── 3: Python detection ───────────────────────────────────────────────────────

func TestGenerateOTELInstrumentation_PythonDetection(t *testing.T) {
	deploy := makeOTELDeployment("py-app", "python:3.11-slim")
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result := GenerateOTELInstrumentation(graph, defaultOTELOpts())

	lang, ok := result.DetectedLanguages["py-app"]
	if !ok {
		t.Fatalf("expected language detected for 'py-app', DetectedLanguages=%v", result.DetectedLanguages)
	}
	if lang != "python" {
		t.Errorf("expected language 'python' for python image, got %q", lang)
	}
}

// ── 4: Node.js detection ─────────────────────────────────────────────────────

func TestGenerateOTELInstrumentation_NodejsDetection(t *testing.T) {
	deploy := makeOTELDeployment("node-app", "node:20-alpine")
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result := GenerateOTELInstrumentation(graph, defaultOTELOpts())

	lang, ok := result.DetectedLanguages["node-app"]
	if !ok {
		t.Fatalf("expected language detected for 'node-app', DetectedLanguages=%v", result.DetectedLanguages)
	}
	if lang != "nodejs" {
		t.Errorf("expected language 'nodejs' for node image, got %q", lang)
	}
}

// ── 5: Go detection ───────────────────────────────────────────────────────────

func TestGenerateOTELInstrumentation_GoDetection(t *testing.T) {
	deploy := makeOTELDeployment("go-svc", "golang:1.22-alpine")
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result := GenerateOTELInstrumentation(graph, defaultOTELOpts())

	lang, ok := result.DetectedLanguages["go-svc"]
	if !ok {
		t.Fatalf("expected language detected for 'go-svc', DetectedLanguages=%v", result.DetectedLanguages)
	}
	if lang != "go" {
		t.Errorf("expected language 'go' for golang image, got %q", lang)
	}
}

// ── 6: .NET detection ────────────────────────────────────────────────────────

func TestGenerateOTELInstrumentation_DotnetDetection(t *testing.T) {
	deploy := makeOTELDeployment("dotnet-api", "mcr.microsoft.com/dotnet/aspnet:8.0")
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result := GenerateOTELInstrumentation(graph, defaultOTELOpts())

	lang, ok := result.DetectedLanguages["dotnet-api"]
	if !ok {
		t.Fatalf("expected language detected for 'dotnet-api', DetectedLanguages=%v", result.DetectedLanguages)
	}
	if lang != "dotnet" {
		t.Errorf("expected language 'dotnet' for aspnet image, got %q", lang)
	}
}

// ── 7: Unknown language — nginx image ────────────────────────────────────────

func TestGenerateOTELInstrumentation_UnknownLanguage(t *testing.T) {
	deploy := makeOTELDeployment("proxy", "nginx:1.25")
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result := GenerateOTELInstrumentation(graph, defaultOTELOpts())

	lang, exists := result.DetectedLanguages["proxy"]
	if exists && lang != "unknown" && lang != "" {
		t.Errorf("nginx image should produce unknown/absent language, got %q", lang)
	}
	// No Instrumentation should be generated for unknown language.
	if _, ok := result.Instrumentations["proxy"]; ok {
		t.Error("no Instrumentation should be generated for unknown-language workload 'proxy'")
	}
}

// ── 8: Custom exporter endpoint reflected in Instrumentation YAML ────────────

func TestGenerateOTELInstrumentation_CustomExporterEndpoint(t *testing.T) {
	deploy := makeOTELDeployment("java-traced", "openjdk:17")
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	opts := OTELOptions{
		ExporterEndpoint: "http://custom-otelcol.monitoring:4317",
		ExporterProtocol: "grpc",
		Propagators:      []string{"tracecontext"},
		SamplingRate:     1.0,
	}

	result := GenerateOTELInstrumentation(graph, opts)

	if len(result.Instrumentations) == 0 {
		t.Fatal("expected at least one Instrumentation YAML for java-traced")
	}
	for _, yaml := range result.Instrumentations {
		if !strings.Contains(yaml, "http://custom-otelcol.monitoring:4317") {
			t.Errorf("Instrumentation YAML must contain custom exporter endpoint:\n%s", yaml)
		}
	}
}

// ── 9: Sampling rate in Instrumentation YAML ─────────────────────────────────

func TestGenerateOTELInstrumentation_SamplingRate(t *testing.T) {
	deploy := makeOTELDeployment("sampled-svc", "python:3.11")
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	opts := OTELOptions{
		ExporterEndpoint: "http://otel-collector:4317",
		ExporterProtocol: "grpc",
		Propagators:      []string{"tracecontext"},
		SamplingRate:     0.25,
	}

	result := GenerateOTELInstrumentation(graph, opts)

	if len(result.Instrumentations) == 0 {
		t.Fatal("expected Instrumentation YAML for sampled-svc (python)")
	}
	for _, yaml := range result.Instrumentations {
		if !strings.Contains(yaml, "0.25") {
			t.Errorf("Instrumentation YAML must contain sampling rate '0.25':\n%s", yaml)
		}
	}
}

// ── 10: W3C + B3 propagators both present ────────────────────────────────────

func TestGenerateOTELInstrumentation_PropagatorW3CAndB3(t *testing.T) {
	deploy := makeOTELDeployment("traced-node", "node:18")
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	opts := OTELOptions{
		ExporterEndpoint: "http://otel-collector:4317",
		ExporterProtocol: "grpc",
		Propagators:      []string{"tracecontext", "b3"},
		SamplingRate:     1.0,
	}

	result := GenerateOTELInstrumentation(graph, opts)

	if len(result.Instrumentations) == 0 {
		t.Fatal("expected Instrumentation YAML for traced-node (nodejs)")
	}
	for _, yaml := range result.Instrumentations {
		if !strings.Contains(yaml, "tracecontext") {
			t.Errorf("Instrumentation YAML missing propagator 'tracecontext':\n%s", yaml)
		}
		if !strings.Contains(yaml, "b3") {
			t.Errorf("Instrumentation YAML missing propagator 'b3':\n%s", yaml)
		}
	}
}

// ── 11: nil chart returns nil, 0 ─────────────────────────────────────────────

func TestInjectOTELInstrumentation_NilChart(t *testing.T) {
	result := &OTELResult{
		Instrumentations: map[string]string{
			"java-app": "apiVersion: opentelemetry.io/v1alpha1\nkind: Instrumentation\n",
		},
		DetectedLanguages: map[string]string{"java-app": "java"},
		NOTESTxt:          "otel notes",
	}

	newChart, count := InjectOTELInstrumentation(nil, result)

	if newChart != nil {
		t.Errorf("expected nil for nil chart input, got %+v", newChart)
	}
	if count != 0 {
		t.Errorf("expected count=0 for nil chart, got %d", count)
	}
}

// ── 12: empty graph → empty result ───────────────────────────────────────────

func TestGenerateOTELInstrumentation_EmptyGraph(t *testing.T) {
	graph := types.NewResourceGraph()

	result := GenerateOTELInstrumentation(graph, defaultOTELOpts())

	if result == nil {
		t.Fatal("expected non-nil OTELResult for empty graph")
	}
	if len(result.Instrumentations) != 0 {
		t.Errorf("expected 0 Instrumentations for empty graph, got %d", len(result.Instrumentations))
	}
	if len(result.DetectedLanguages) != 0 {
		t.Errorf("expected 0 DetectedLanguages for empty graph, got %d", len(result.DetectedLanguages))
	}
}

// ── 13: InjectOTELInstrumentation is copy-on-write ───────────────────────────

func TestInjectOTELInstrumentation_CopyOnWrite(t *testing.T) {
	originalContent := testDeploymentTemplate
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": originalContent,
	})
	otelResult := &OTELResult{
		Instrumentations: map[string]string{
			"test-app": "apiVersion: opentelemetry.io/v1alpha1\nkind: Instrumentation\nmetadata:\n  name: test-app\n",
		},
		DetectedLanguages: map[string]string{"test-app": "java"},
		NOTESTxt:          "otel instructions",
	}

	newChart, _ := InjectOTELInstrumentation(chart, otelResult)

	if newChart == chart {
		t.Error("InjectOTELInstrumentation must return a new chart (copy-on-write)")
	}
	if chart.Templates["templates/deployment.yaml"] != originalContent {
		t.Error("original chart template must not be modified by InjectOTELInstrumentation")
	}
	if len(chart.Templates) != 1 {
		t.Errorf("original chart must still have 1 template, got %d", len(chart.Templates))
	}
}
