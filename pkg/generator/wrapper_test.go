package generator

import (
	"context"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
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
// Subtask 2: Wrapper template calls library include
// ============================================================

func TestWrapperChart_Template_DeploymentInclude(t *testing.T) {
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

	// Check for include of library.deployment
	found := false
	for _, content := range wrapper.Templates {
		if strings.Contains(content, `include "library.deployment"`) {
			found = true
			break
		}
	}
	if !found {
		t.Error("wrapper template should include library.deployment")
	}
}

func TestWrapperChart_Template_ServiceInclude(t *testing.T) {
	svc := makeProcessedResourceWithValues("Service", "frontend-svc", "default",
		map[string]string{"app.kubernetes.io/name": "frontend"},
		map[string]interface{}{"type": "ClusterIP"}, "# svc")

	graph := buildGraph([]*types.ProcessedResource{svc}, nil)

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

	found := false
	for _, content := range wrapper.Templates {
		if strings.Contains(content, `include "library.service"`) {
			found = true
			break
		}
	}
	if !found {
		t.Error("wrapper template should include library.service")
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

// ============================================================
// Subtask 5: Wrapper _helpers.tpl
// ============================================================

func TestWrapperChart_Helpers_Fullname(t *testing.T) {
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

	if !strings.Contains(wrapper.Helpers, "fullname") {
		t.Error("wrapper _helpers.tpl should contain fullname template")
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
	// Expected: Wrapper has 3 template files, each calling library include
	resources := []*types.ProcessedResource{
		makeProcessedResourceWithValues("Deployment", "app", "default",
			map[string]string{"app.kubernetes.io/name": "app"},
			map[string]interface{}{"replicaCount": 1}, "# deploy"),
		makeProcessedResourceWithValues("Service", "app-svc", "default",
			map[string]string{"app.kubernetes.io/name": "app"},
			map[string]interface{}{"type": "ClusterIP"}, "# svc"),
		makeProcessedResourceWithValues("ConfigMap", "app-config", "default",
			map[string]string{"app.kubernetes.io/name": "app"},
			map[string]interface{}{"data": "config"}, "# cm"),
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

	if len(wrapper.Templates) < 3 {
		t.Errorf("expected at least 3 templates in wrapper, got %d", len(wrapper.Templates))
	}

	// Each template should call library include
	for path, content := range wrapper.Templates {
		if !strings.Contains(content, `include "library.`) {
			t.Errorf("wrapper template %s should call library include", path)
		}
	}
}
