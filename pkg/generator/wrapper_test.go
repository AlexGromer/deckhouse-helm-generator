package generator

import (
	"context"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Subtask 1: Wrapper Chart.yaml with library dependency
// ============================================================

func TestWrapperChart_ChartYAML_LibraryDependency(t *testing.T) {
	// Expected: dependencies: [{name: library, version: "0.1.0", repository: "file://../library"}]
	deploy := makeProcessedResourceWithValues("Deployment", "frontend", "default",
		map[string]string{"app.kubernetes.io/name": "frontend"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Find wrapper chart (not the library)
	var wrapper *types.GeneratedChart
	for _, c := range charts {
		if !strings.Contains(c.ChartYAML, "type: library") {
			wrapper = c
			break
		}
	}
	if wrapper == nil {
		t.Fatal("wrapper chart not found")
	}

	if !strings.Contains(wrapper.ChartYAML, "name: library") {
		t.Error("wrapper Chart.yaml missing library dependency name")
	}
	if !strings.Contains(wrapper.ChartYAML, "repository: file://../library") {
		t.Error("wrapper Chart.yaml missing file:// repository for library")
	}
}

func TestWrapperChart_ChartYAML_ApplicationType(t *testing.T) {
	// Expected: type: application (not library)
	deploy := makeProcessedResourceWithValues("Deployment", "frontend", "default",
		map[string]string{"app.kubernetes.io/name": "frontend"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	var wrapper *types.GeneratedChart
	for _, c := range charts {
		if !strings.Contains(c.ChartYAML, "type: library") {
			wrapper = c
			break
		}
	}
	if wrapper == nil {
		t.Fatal("wrapper chart not found")
	}

	if !strings.Contains(wrapper.ChartYAML, "type: application") {
		t.Error("wrapper Chart.yaml should have type: application")
	}
}

// ============================================================
// Subtask 2: Wrapper templates use the library helpers
// ============================================================

// assertOnlyLibraryHelpers checks that content references at least one helper
// and that every helper it references is a library helper.
func assertOnlyLibraryHelpers(t *testing.T, path, content string, forbiddenPrefixes ...string) {
	t.Helper()
	refs := includedHelpers(content)
	if len(refs) == 0 {
		t.Errorf("template %s references no helpers:\n%s", path, content)
	}
	for _, ref := range refs {
		if !strings.HasPrefix(ref, "library.") {
			t.Errorf("template %s references non-library helper %q", path, ref)
		}
	}
	for _, prefix := range forbiddenPrefixes {
		if strings.Contains(content, `include "`+prefix+`.`) || strings.Contains(content, `template "`+prefix+`.`) {
			t.Errorf("template %s still references %s.* helpers", path, prefix)
		}
	}
}

func TestWrapperChart_Template_DeploymentUsesLibraryHelpers(t *testing.T) {
	deploy := makeLibraryModeResource("Deployment", "frontend", "frontend", "deployment",
		map[string]interface{}{"replicas": int64(1)})

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

	// The template keeps the processor's path, not a per-kind file name.
	content, ok := wrapper.Templates["templates/frontend-deployment.yaml"]
	if !ok {
		t.Fatalf("wrapper missing templates/frontend-deployment.yaml, got %v", templatePaths(wrapper.Templates))
	}
	if _, ok := wrapper.Templates["templates/deployment.yaml"]; ok {
		t.Error("wrapper should not have a per-kind templates/deployment.yaml")
	}

	assertOnlyLibraryHelpers(t, "templates/frontend-deployment.yaml", content, "src", "frontend")
	for _, want := range []string{
		`include "library.fullname" $`,
		`include "library.labels" $`,
		`include "library.selectorLabels" $`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("deployment template missing %q:\n%s", want, content)
		}
	}
}

func TestWrapperChart_Template_ServiceUsesLibraryHelpers(t *testing.T) {
	svc := makeLibraryModeResource("Service", "frontend", "frontend", "service",
		map[string]interface{}{"type": "ClusterIP"})

	graph := buildGraph([]*types.ProcessedResource{svc}, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, libraryTestOptions())
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	wrapper := findChartByName(charts, "frontend")
	if wrapper == nil {
		t.Fatalf("wrapper chart frontend not found, got %v", chartNamesOf(charts))
	}

	content, ok := wrapper.Templates["templates/frontend-service.yaml"]
	if !ok {
		t.Fatalf("wrapper missing templates/frontend-service.yaml, got %v", templatePaths(wrapper.Templates))
	}
	assertOnlyLibraryHelpers(t, "templates/frontend-service.yaml", content, "src", "frontend")
	if !strings.Contains(content, `include "library.selectorLabels" $`) {
		t.Errorf("service selector should use library.selectorLabels:\n%s", content)
	}
	if !strings.Contains(content, "type: {{ $svc.service.type }}") {
		t.Errorf("service template should read flat values:\n%s", content)
	}
}

// ============================================================
// Subtask 3: Wrapper values are flat
// ============================================================

func TestWrapperChart_Values_FlatStructure(t *testing.T) {
	deploy := makeProcessedResourceWithValues("Deployment", "frontend", "default",
		map[string]string{"app.kubernetes.io/name": "frontend"},
		map[string]interface{}{"replicaCount": int64(3)}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	var wrapper *types.GeneratedChart
	for _, c := range charts {
		if !strings.Contains(c.ChartYAML, "type: library") {
			wrapper = c
			break
		}
	}
	if wrapper == nil {
		t.Fatal("wrapper chart not found")
	}

	if strings.Contains(wrapper.ValuesYAML, "frontend:") && strings.Contains(wrapper.ValuesYAML, "services:") {
		t.Error("wrapper values should be flat, not nested under service name")
	}
}

func TestWrapperChart_Values_AllFields(t *testing.T) {
	// Input: Service with Deployment + Service + Ingress
	// Expected: Values contain replicaCount, image, service, ingress sections
	resources := []*types.ProcessedResource{
		makeProcessedResourceWithValues("Deployment", "app", "default",
			map[string]string{"app.kubernetes.io/name": "app"},
			map[string]interface{}{"replicaCount": int64(2), "image": map[string]interface{}{"repository": "nginx", "tag": "latest"}}, "# deploy"),
		makeProcessedResourceWithValues("Service", "app-svc", "default",
			map[string]string{"app.kubernetes.io/name": "app"},
			map[string]interface{}{"type": "ClusterIP", "port": int64(80)}, "# svc"),
		makeProcessedResourceWithValues("Ingress", "app-ing", "default",
			map[string]string{"app.kubernetes.io/name": "app"},
			map[string]interface{}{"host": "app.example.com"}, "# ing"),
	}

	graph := buildGraph(resources, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	var wrapper *types.GeneratedChart
	for _, c := range charts {
		if !strings.Contains(c.ChartYAML, "type: library") {
			wrapper = c
			break
		}
	}
	if wrapper == nil {
		t.Fatal("wrapper chart not found")
	}

	// Values should contain fields from all resource types
	if !strings.Contains(wrapper.ValuesYAML, "replicaCount") {
		t.Error("wrapper values missing replicaCount from Deployment")
	}
	if !strings.Contains(wrapper.ValuesYAML, "image") {
		t.Error("wrapper values missing image from Deployment")
	}
}

func TestWrapperChart_Values_EnabledAndFlatPaths(t *testing.T) {
	// Processor templates are guarded by `if $svc.enabled` and read
	// $.Values.global, so the flat wrapper values must provide both, and the
	// services.<svc>. prefix of each ValuesPath is stripped.
	resources := []*types.ProcessedResource{
		makeLibraryModeResource("Deployment", "frontend", "frontend", "deployment",
			map[string]interface{}{"replicas": int64(2)}),
		makeLibraryModeResource("Service", "frontend", "frontend", "service",
			map[string]interface{}{"type": "ClusterIP"}),
	}
	graph := buildGraph(resources, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, libraryTestOptions())
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	wrapper := findChartByName(charts, "frontend")
	if wrapper == nil {
		t.Fatalf("wrapper chart frontend not found, got %v", chartNamesOf(charts))
	}

	values := parseValuesYAML(t, wrapper.ValuesYAML)
	if values["enabled"] != true {
		t.Errorf("wrapper values enabled = %v, want true", values["enabled"])
	}
	if g, ok := values["global"].(map[string]interface{}); !ok || len(g) != 0 {
		t.Errorf("wrapper values global = %v, want empty map", values["global"])
	}
	if _, ok := values["services"]; ok {
		t.Error("wrapper values should not be nested under services")
	}
	if dep, _ := values["deployment"].(map[string]interface{}); dep["replicas"] != float64(2) {
		t.Errorf("deployment.replicas = %v, want 2", values["deployment"])
	}
	if svc, _ := values["service"].(map[string]interface{}); svc["type"] != "ClusterIP" {
		t.Errorf("service.type = %v, want ClusterIP", values["service"])
	}
}

// ============================================================
// Subtask 4: Multiple wrapper generation
// ============================================================

func TestWrapperChart_MultipleWrappers(t *testing.T) {
	// Input: 3 services (frontend, backend, database)
	// Expected: 1 library chart + 3 wrapper charts
	resources := []*types.ProcessedResource{
		makeProcessedResourceWithValues("Deployment", "frontend", "default",
			map[string]string{"app.kubernetes.io/name": "frontend"},
			map[string]interface{}{"replicaCount": 1}, "# deploy"),
		makeProcessedResourceWithValues("Deployment", "backend", "default",
			map[string]string{"app.kubernetes.io/name": "backend"},
			map[string]interface{}{"replicaCount": 2}, "# deploy"),
		makeProcessedResourceWithValues("Deployment", "database", "default",
			map[string]string{"app.kubernetes.io/name": "database"},
			map[string]interface{}{"replicaCount": 1}, "# deploy"),
	}

	graph := buildGraph(resources, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// Expected: 1 library + 3 wrappers = 4 charts
	if len(charts) != 4 {
		t.Fatalf("expected 4 charts (1 library + 3 wrappers), got %d", len(charts))
	}

	libraryCount := 0
	wrapperCount := 0
	for _, c := range charts {
		if strings.Contains(c.ChartYAML, "type: library") {
			libraryCount++
		} else {
			wrapperCount++
		}
	}
	if libraryCount != 1 {
		t.Errorf("expected 1 library chart, got %d", libraryCount)
	}
	if wrapperCount != 3 {
		t.Errorf("expected 3 wrapper charts, got %d", wrapperCount)
	}
}

func TestWrapperChart_MultipleWrappers_EachUsesLibrary(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeLibraryModeResource("Deployment", "frontend", "frontend", "deployment",
			map[string]interface{}{"replicas": int64(1)}),
		makeLibraryModeResource("Deployment", "backend", "backend", "deployment",
			map[string]interface{}{"replicas": int64(2)}),
		makeLibraryModeResource("StatefulSet", "database", "database", "statefulset",
			map[string]interface{}{"replicas": int64(1)}),
	}
	graph := buildGraph(resources, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, libraryTestOptions())
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	for _, name := range []string{"frontend", "backend", "database"} {
		wrapper := findChartByName(charts, name)
		if wrapper == nil {
			t.Errorf("wrapper chart %s not found, got %v", name, chartNamesOf(charts))
			continue
		}
		if !strings.Contains(wrapper.ChartYAML, "name: "+name) {
			t.Errorf("wrapper %s Chart.yaml has wrong name:\n%s", name, wrapper.ChartYAML)
		}
		if !strings.Contains(wrapper.ChartYAML, "repository: file://../library") {
			t.Errorf("wrapper %s does not depend on file://../library", name)
		}
		if wrapper.Helpers != "" {
			t.Errorf("wrapper %s should not carry its own _helpers.tpl", name)
		}
		if len(wrapper.Templates) != 1 {
			t.Errorf("wrapper %s should hold only its own templates, got %v", name, templatePaths(wrapper.Templates))
		}
		for path, content := range wrapper.Templates {
			// Neither the source chart's nor any group's helpers may remain.
			assertOnlyLibraryHelpers(t, path, content, "src", "frontend", "backend", "database")
		}
	}
}

// ============================================================
// Subtask 5: Wrapper helpers come from the library
// ============================================================

func TestWrapperChart_Helpers_ProvidedByLibrary(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeLibraryModeResource("Deployment", "frontend", "frontend", "deployment",
			map[string]interface{}{"replicas": int64(1)}),
		makeLibraryModeResource("Service", "frontend", "frontend", "service",
			map[string]interface{}{"type": "ClusterIP"}),
	}
	graph := buildGraph(resources, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, libraryTestOptions())
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	libChart := findLibraryChart(charts)
	wrapper := findChartByName(charts, "frontend")
	if libChart == nil || wrapper == nil {
		t.Fatalf("expected library and frontend charts, got %v", chartNamesOf(charts))
	}

	// No own _helpers.tpl: fullname, labels, ... are the library's.
	if wrapper.Helpers != "" {
		t.Errorf("wrapper should have no _helpers.tpl, got:\n%s", wrapper.Helpers)
	}
	if !strings.Contains(libChart.Helpers, `define "library.fullname"`) {
		t.Error("library chart should define library.fullname")
	}

	// Every helper a wrapper template (or NOTES.txt) references must resolve
	// in the library.
	contents := map[string]string{"NOTES.txt": wrapper.Notes}
	for path, content := range wrapper.Templates {
		contents[path] = content
	}
	for path, content := range contents {
		for _, ref := range includedHelpers(content) {
			if !strings.Contains(libChart.Helpers, `define "`+ref+`"`) {
				t.Errorf("%s references %q, which the library does not define", path, ref)
			}
		}
	}
	if strings.Contains(wrapper.Notes, `"frontend.`) {
		t.Error("wrapper NOTES.txt still references frontend.* helpers")
	}
}

// ============================================================
// Subtask 6: Edge cases
// ============================================================

func TestWrapperChart_Edge_SingleWrapper(t *testing.T) {
	// Input: 1 service
	// Expected: 1 library chart + 1 wrapper chart
	deploy := makeProcessedResourceWithValues("Deployment", "solo", "default",
		map[string]string{"app.kubernetes.io/name": "solo"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if len(charts) != 2 {
		t.Fatalf("expected 2 charts (1 library + 1 wrapper), got %d", len(charts))
	}
}

func TestWrapperChart_Edge_WrapperWithManyResources(t *testing.T) {
	// Input: Service with 3 resource types
	// Expected: Wrapper has the 3 processor templates, each using library helpers
	resources := []*types.ProcessedResource{
		makeLibraryModeResource("Deployment", "app", "app", "deployment",
			map[string]interface{}{"replicas": int64(1)}),
		makeLibraryModeResource("Service", "app", "app", "service",
			map[string]interface{}{"type": "ClusterIP"}),
		makeLibraryModeResource("ConfigMap", "app-config", "app", "configMaps.appConfig",
			map[string]interface{}{"enabled": true, "data": map[string]interface{}{"key": "value"}}),
	}

	graph := buildGraph(resources, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, libraryTestOptions())
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	wrapper := findChartByName(charts, "app")
	if wrapper == nil {
		t.Fatalf("wrapper chart app not found, got %v", chartNamesOf(charts))
	}

	if len(wrapper.Templates) != len(resources) {
		t.Errorf("expected %d templates in wrapper, got %v", len(resources), templatePaths(wrapper.Templates))
	}
	for _, r := range resources {
		content, ok := wrapper.Templates[r.TemplatePath]
		if !ok {
			t.Errorf("wrapper missing template %s", r.TemplatePath)
			continue
		}
		if !strings.Contains(content, "kind: "+r.Original.GVK.Kind) {
			t.Errorf("template %s should render a %s", r.TemplatePath, r.Original.GVK.Kind)
		}
		assertOnlyLibraryHelpers(t, r.TemplatePath, content, "src", "app")
	}

	// The nested ConfigMap values path is kept below the stripped prefix.
	values := parseValuesYAML(t, wrapper.ValuesYAML)
	cms, _ := values["configMaps"].(map[string]interface{})
	if _, ok := cms["appConfig"]; !ok {
		t.Errorf("wrapper values missing configMaps.appConfig:\n%s", wrapper.ValuesYAML)
	}
}
