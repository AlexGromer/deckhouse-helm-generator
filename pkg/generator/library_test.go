package generator

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/helm"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Subtask 1: LibraryGenerator implements Generator interface
// ============================================================

func TestLibraryGenerator_ImplementsInterface(t *testing.T) {
	var _ Generator = (*LibraryGenerator)(nil)
}

func TestLibraryGenerator_Mode(t *testing.T) {
	gen := NewLibraryGenerator()
	if gen.Mode() != types.OutputModeLibrary {
		t.Errorf("expected mode %s, got %s", types.OutputModeLibrary, gen.Mode())
	}
}

// ============================================================
// Subtask 2: Library Chart.yaml
// ============================================================

func TestLibraryGenerator_ChartYAML_Type(t *testing.T) {
	// Expected: type: library in Chart.yaml
	deploy := makeProcessedResourceWithValues("Deployment", "app", "default",
		map[string]string{"app.kubernetes.io/name": "app"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Find library chart
	var libChart *types.GeneratedChart
	for _, c := range charts {
		if strings.Contains(c.ChartYAML, "type: library") {
			libChart = c
			break
		}
	}
	if libChart == nil {
		t.Fatal("no chart with type: library found")
	}
}

func TestLibraryGenerator_ChartYAML_Fields(t *testing.T) {
	// Expected: apiVersion: v2, name: "library", version present
	deploy := makeProcessedResourceWithValues("Deployment", "app", "default",
		map[string]string{"app.kubernetes.io/name": "app"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	var libChart *types.GeneratedChart
	for _, c := range charts {
		if strings.Contains(c.ChartYAML, "type: library") {
			libChart = c
			break
		}
	}
	if libChart == nil {
		t.Fatal("library chart not found")
	}

	if !strings.Contains(libChart.ChartYAML, "apiVersion: v2") {
		t.Error("Chart.yaml missing apiVersion: v2")
	}
	if !strings.Contains(libChart.ChartYAML, "version:") {
		t.Error("Chart.yaml missing version field")
	}
}

// ============================================================
// Subtask 3: Library chart holds the shared helpers
// ============================================================

// libraryHelperNames are the helpers every wrapper template may rely on.
var libraryHelperNames = []string{
	"library.name",
	"library.fullname",
	"library.chart",
	"library.labels",
	"library.selectorLabels",
	"library.serviceAccountName",
	"library.imagePullSecrets",
	"library.image",
}

func TestLibraryGenerator_Helpers_SharedDefines(t *testing.T) {
	deploy := makeLibraryModeResource("Deployment", "frontend", "frontend", "deployment",
		map[string]interface{}{"replicas": int64(1)})

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, libraryTestOptions())
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	libChart := findLibraryChart(charts)
	if libChart == nil {
		t.Fatal("library chart not found")
	}
	if libChart.Name != "library" {
		t.Errorf("library chart name = %q, want %q", libChart.Name, "library")
	}

	for _, name := range libraryHelperNames {
		define := `define "` + name + `"`
		if n := strings.Count(libChart.Helpers, define); n != 1 {
			t.Errorf("library _helpers.tpl defines %q %d times, want exactly 1", name, n)
		}
	}

	// The helpers are the standard generated ones, named after the library.
	if libChart.Helpers != helm.GenerateHelpers("library") {
		t.Error("library helpers differ from helm.GenerateHelpers(\"library\")")
	}
}

func TestLibraryGenerator_NoKindTemplates(t *testing.T) {
	// The library carries only helpers: the resource manifests live in the
	// wrapper charts, so there are no per-kind named templates.
	resources := []*types.ProcessedResource{
		makeLibraryModeResource("Deployment", "frontend", "frontend", "deployment",
			map[string]interface{}{"replicas": int64(1)}),
		makeLibraryModeResource("Service", "frontend", "frontend", "service",
			map[string]interface{}{"type": "ClusterIP"}),
		makeLibraryModeResource("StatefulSet", "db", "db", "statefulset",
			map[string]interface{}{"replicas": int64(1)}),
	}
	graph := buildGraph(resources, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, libraryTestOptions())
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	libChart := findLibraryChart(charts)
	if libChart == nil {
		t.Fatal("library chart not found")
	}

	if len(libChart.Templates) != 0 {
		t.Errorf("library chart should have no templates, got %d: %v",
			len(libChart.Templates), templatePaths(libChart.Templates))
	}
	for _, kind := range []string{"deployment", "service", "statefulset", "configmap", "ingress"} {
		if strings.Contains(libChart.Helpers, `define "library.`+kind+`"`) {
			t.Errorf("library helpers should not define a kind template library.%s", kind)
		}
	}
	if strings.Contains(libChart.Helpers, "apiVersion:") || strings.Contains(libChart.Helpers, "kind:") {
		t.Error("library helpers should not render Kubernetes manifests")
	}
}

// ============================================================
// Subtask 4: Wrapper templates are the processor templates
// ============================================================

func TestLibraryGenerator_WrapperTemplate_KeepsProcessorContent(t *testing.T) {
	deploy := makeLibraryModeResource("Deployment", "frontend", "frontend", "deployment",
		map[string]interface{}{
			"replicas": int64(3),
			"image":    map[string]interface{}{"repository": "nginx", "tag": "1.25"},
		})

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, libraryTestOptions())
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	wrapper := findChartByName(charts, "frontend")
	if wrapper == nil {
		t.Fatalf("wrapper chart frontend not found, got %v", chartNamesOf(charts))
	}

	content, ok := wrapper.Templates[deploy.TemplatePath]
	if !ok {
		t.Fatalf("wrapper missing template %s, got %v", deploy.TemplatePath, templatePaths(wrapper.Templates))
	}

	// Values paths are flattened: .Values.services.frontend -> .Values.
	if strings.Contains(content, ".Values.services.") {
		t.Errorf("wrapper template still references nested services values:\n%s", content)
	}
	for _, want := range []string{
		"{{- $svc := .Values -}}",
		"kind: Deployment",
		"replicas: {{ $svc.deployment.replicas }}",
		"image: {{ $svc.deployment.image.repository }}:{{ $svc.deployment.image.tag }}",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("wrapper template missing %q:\n%s", want, content)
		}
	}

	// The values the template reads are present in the wrapper values.yaml.
	values := parseValuesYAML(t, wrapper.ValuesYAML)
	dep, ok := values["deployment"].(map[string]interface{})
	if !ok {
		t.Fatalf("wrapper values missing deployment section:\n%s", wrapper.ValuesYAML)
	}
	if dep["replicas"] != float64(3) {
		t.Errorf("deployment.replicas = %v, want 3", dep["replicas"])
	}
	img, _ := dep["image"].(map[string]interface{})
	if img["repository"] != "nginx" || img["tag"] != "1.25" {
		t.Errorf("deployment.image = %v, want nginx:1.25", dep["image"])
	}
}

// ============================================================
// Subtask 9: Edge cases
// ============================================================

func TestLibraryGenerator_Edge_EmptyGraph(t *testing.T) {
	// Input: Empty resource graph
	// Expected: Library chart with all named templates (they're generic)
	graph := buildGraph(nil, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Should still generate library chart with generic templates
	if len(charts) == 0 {
		t.Fatal("expected at least 1 chart (library) even for empty graph")
	}

	libChart := findLibraryChart(charts)
	if libChart == nil {
		t.Fatal("library chart not found for empty graph")
	}
}

func TestLibraryGenerator_Edge_SingleResourceType(t *testing.T) {
	// Input: Only Deployments
	// Expected: the library chart is generic — identical whatever the input.
	deploy := makeLibraryModeResource("Deployment", "app", "app", "deployment",
		map[string]interface{}{"replicas": int64(1)})

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(),
		buildGraph([]*types.ProcessedResource{deploy}, nil), libraryTestOptions())
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	emptyCharts, err := gen.Generate(context.Background(), buildGraph(nil, nil), libraryTestOptions())
	if err != nil {
		t.Fatalf("Generate (empty graph) returned error: %v", err)
	}

	libChart := findLibraryChart(charts)
	emptyLib := findLibraryChart(emptyCharts)
	if libChart == nil || emptyLib == nil {
		t.Fatal("library chart not found")
	}

	if libChart.ChartYAML != emptyLib.ChartYAML {
		t.Error("library Chart.yaml should not depend on the input resources")
	}
	if libChart.Helpers != emptyLib.Helpers {
		t.Error("library helpers should not depend on the input resources")
	}
	if len(libChart.Templates) != 0 || len(emptyLib.Templates) != 0 {
		t.Error("library chart should have no templates")
	}
}

func TestLibraryGenerator_CancelledContext(t *testing.T) {
	graph := buildGraph(nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	gen := NewLibraryGenerator()
	_, err := gen.Generate(ctx, graph, Options{ChartVersion: "0.1.0"})
	if err == nil {
		t.Error("expected error with cancelled context, got nil")
	}
}

// ============================================================
// Helpers
// ============================================================

func findLibraryChart(charts []*types.GeneratedChart) *types.GeneratedChart {
	for _, c := range charts {
		if strings.Contains(c.ChartYAML, "type: library") {
			return c
		}
	}
	return nil
}

// libraryTestOptions mirrors the pipeline: processors render templates for the
// source chart "src", whose helper references the generator must rewrite.
func libraryTestOptions() Options {
	return Options{ChartName: "src", ChartVersion: "0.1.0"}
}

// makeLibraryModeResource builds a ProcessedResource shaped like processor
// output: a template under services.<svc> that uses the source chart helpers.
func makeLibraryModeResource(kind, name, svc, valuesKey string, values map[string]interface{}) *types.ProcessedResource {
	r := makeProcessedResource(kind, name, "default", map[string]string{"app.kubernetes.io/name": svc})
	r.ServiceName = svc
	r.Values = values
	r.ValuesPath = "services." + svc + "." + valuesKey
	r.TemplatePath = "templates/" + svc + "-" + strings.ToLower(kind) + ".yaml"

	var body string
	switch kind {
	case "Deployment", "StatefulSet":
		body = `spec:
  replicas: {{ $svc.` + valuesKey + `.replicas }}
  selector:
    matchLabels:
      {{- include "src.selectorLabels" $ | nindent 6 }}
  template:
    spec:
      containers:
        - name: ` + name + `
          image: {{ $svc.` + valuesKey + `.image.repository }}:{{ $svc.` + valuesKey + `.image.tag }}
`
	case "Service":
		body = `spec:
  type: {{ $svc.` + valuesKey + `.type }}
  selector:
    {{- include "src.selectorLabels" $ | nindent 4 }}
`
	default:
		body = `data:
  {{- toYaml $svc.` + valuesKey + `.data | nindent 2 }}
`
	}

	r.TemplateContent = `{{- $svc := .Values.services.` + svc + ` -}}
{{- if $svc.enabled }}
apiVersion: ` + gvkForKind(kind).GroupVersion().String() + `
kind: ` + kind + `
metadata:
  name: {{ include "src.fullname" $ }}-` + name + `
  labels:
    {{- include "src.labels" $ | nindent 4 }}
` + body + `{{- end }}
`
	return r
}

// parseValuesYAML decodes a generated values.yaml.
func parseValuesYAML(t *testing.T, valuesYAML string) map[string]interface{} {
	t.Helper()
	values := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(valuesYAML), &values); err != nil {
		t.Fatalf("invalid values.yaml: %v\n%s", err, valuesYAML)
	}
	return values
}

// includedHelpers returns the helper names referenced via include/template.
func includedHelpers(content string) []string {
	var names []string
	for _, m := range helperRefRe.FindAllStringSubmatch(content, -1) {
		names = append(names, m[1])
	}
	return names
}

var helperRefRe = regexp.MustCompile(`(?:include|template) "([^"]+)"`)

func templatePaths(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func chartNamesOf(charts []*types.GeneratedChart) []string {
	names := make([]string, len(charts))
	for i, c := range charts {
		names[i] = c.Name
	}
	return names
}
