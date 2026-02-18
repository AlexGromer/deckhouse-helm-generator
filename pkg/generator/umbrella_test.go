package generator

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Subtask 1: UmbrellaGenerator implements Generator interface
// ============================================================

func TestUmbrellaGenerator_ImplementsInterface(t *testing.T) {
	var _ Generator = (*UmbrellaGenerator)(nil)
}

func TestUmbrellaGenerator_Mode(t *testing.T) {
	gen := NewUmbrellaGenerator()
	if gen.Mode() != types.OutputModeUmbrella {
		t.Errorf("expected mode %q, got %q", types.OutputModeUmbrella, gen.Mode())
	}
}

// ============================================================
// Subtask 2: Parent Chart.yaml structure
// ============================================================

func TestUmbrellaGenerator_ParentChartYAML_Dependencies(t *testing.T) {
	// Input: 3 groups (frontend, backend, database)
	resources := []*types.ProcessedResource{
		makeProcessedResourceWithValues("Deployment", "frontend", "default",
			map[string]string{"app.kubernetes.io/name": "frontend"},
			map[string]interface{}{"replicaCount": int64(2)}, "# fe"),
		makeProcessedResourceWithValues("Deployment", "backend", "default",
			map[string]string{"app.kubernetes.io/name": "backend"},
			map[string]interface{}{"replicaCount": int64(3)}, "# be"),
		makeProcessedResourceWithValues("Deployment", "database", "default",
			map[string]string{"app.kubernetes.io/name": "database"},
			map[string]interface{}{"replicaCount": int64(1)}, "# db"),
	}
	graph := buildGraph(resources, nil)

	gen := NewUmbrellaGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "1.0.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	parent := findParentChart(charts)
	if parent == nil {
		t.Fatal("parent umbrella chart not found")
	}

	// Parent Chart.yaml must have dependencies listing all 3 subcharts
	if !strings.Contains(parent.ChartYAML, "dependencies:") {
		t.Error("parent Chart.yaml missing 'dependencies:' section")
	}

	// Should list 3 dependencies
	depCount := strings.Count(parent.ChartYAML, "- name:")
	if depCount != 3 {
		t.Errorf("expected 3 dependencies in parent Chart.yaml, got %d\nChart.yaml:\n%s", depCount, parent.ChartYAML)
	}

	// Each dependency must have condition field
	if !strings.Contains(parent.ChartYAML, "condition:") {
		t.Error("parent Chart.yaml dependencies missing 'condition:' field")
	}
}

func TestUmbrellaGenerator_ParentChartYAML_RequiredFields(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeProcessedResourceWithValues("Deployment", "app", "default",
			map[string]string{"app.kubernetes.io/name": "app"},
			map[string]interface{}{}, "# app"),
	}
	graph := buildGraph(resources, nil)

	gen := NewUmbrellaGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{
		ChartVersion: "2.0.0",
		ChartName:    "myumbrella",
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	parent := findParentChart(charts)
	if parent == nil {
		t.Fatal("parent chart not found")
	}

	if !strings.Contains(parent.ChartYAML, "apiVersion: v2") {
		t.Error("parent Chart.yaml missing 'apiVersion: v2'")
	}
	if !strings.Contains(parent.ChartYAML, "version: 2.0.0") {
		t.Error("parent Chart.yaml missing version 2.0.0")
	}
	// Parent chart is type: application (not library)
	if strings.Contains(parent.ChartYAML, "type: library") {
		t.Error("parent Chart.yaml should not have 'type: library'")
	}
}

// ============================================================
// Subtask 3: Subchart directories
// ============================================================

func TestUmbrellaGenerator_SubchartDirs_Structure(t *testing.T) {
	// Input: 3 groups → expect 4 total charts (1 parent + 3 subcharts)
	resources := []*types.ProcessedResource{
		makeProcessedResourceWithValues("Deployment", "frontend", "default",
			map[string]string{"app.kubernetes.io/name": "frontend"},
			nil, "# fe"),
		makeProcessedResourceWithValues("Deployment", "backend", "default",
			map[string]string{"app.kubernetes.io/name": "backend"},
			nil, "# be"),
		makeProcessedResourceWithValues("Deployment", "database", "default",
			map[string]string{"app.kubernetes.io/name": "database"},
			nil, "# db"),
	}
	graph := buildGraph(resources, nil)

	gen := NewUmbrellaGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "1.0.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// 1 parent + 3 subcharts = 4 total
	if len(charts) != 4 {
		t.Fatalf("expected 4 charts (1 parent + 3 subcharts), got %d: %v",
			len(charts), chartNamesList(charts))
	}

	parent := findParentChart(charts)
	if parent == nil {
		t.Fatal("parent chart not found")
	}

	// Subcharts must be embedded within the parent (path includes "charts/")
	subcharts := findSubcharts(charts)
	if len(subcharts) != 3 {
		t.Fatalf("expected 3 subcharts, got %d", len(subcharts))
	}
	for _, sub := range subcharts {
		if !strings.Contains(sub.Path, "charts/") && !strings.HasPrefix(sub.Name, "charts/") {
			t.Errorf("subchart %q path does not indicate it is in charts/ subdirectory (path=%q)", sub.Name, sub.Path)
		}
	}
}

func TestUmbrellaGenerator_SubchartDirs_ChartContent(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeProcessedResourceWithValues("Deployment", "myapp", "default",
			map[string]string{"app.kubernetes.io/name": "myapp"},
			map[string]interface{}{"replicaCount": int64(1)}, "# app"),
		makeProcessedResourceWithValues("Service", "myapp-svc", "default",
			map[string]string{"app.kubernetes.io/name": "myapp"},
			map[string]interface{}{"port": int64(80)}, "# svc"),
	}
	graph := buildGraph(resources, nil)

	gen := NewUmbrellaGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "1.0.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	subcharts := findSubcharts(charts)
	for _, sub := range subcharts {
		if sub.ChartYAML == "" {
			t.Errorf("subchart %s has empty Chart.yaml", sub.Name)
		}
		if sub.ValuesYAML == "" {
			t.Errorf("subchart %s has empty values.yaml", sub.Name)
		}
		if len(sub.Templates) == 0 {
			t.Errorf("subchart %s has no templates", sub.Name)
		}
	}
}

// ============================================================
// Subtask 4: Cascading values (parent -> subchart)
// ============================================================

func TestUmbrellaGenerator_CascadingValues_PerSubchart(t *testing.T) {
	// Expected: Parent values with frontend: {replicaCount: 3}, backend: {replicaCount: 2}
	resources := []*types.ProcessedResource{
		makeProcessedResourceWithValues("Deployment", "frontend", "default",
			map[string]string{"app.kubernetes.io/name": "frontend"},
			map[string]interface{}{"replicaCount": int64(3)}, "# fe"),
		makeProcessedResourceWithValues("Deployment", "backend", "default",
			map[string]string{"app.kubernetes.io/name": "backend"},
			map[string]interface{}{"replicaCount": int64(2)}, "# be"),
	}
	graph := buildGraph(resources, nil)

	gen := NewUmbrellaGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "1.0.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	parent := findParentChart(charts)
	if parent == nil {
		t.Fatal("parent chart not found")
	}

	// Parent values must have per-subchart sections
	if !strings.Contains(parent.ValuesYAML, "frontend:") {
		t.Error("parent values.yaml missing 'frontend:' section")
	}
	if !strings.Contains(parent.ValuesYAML, "backend:") {
		t.Error("parent values.yaml missing 'backend:' section")
	}
}

func TestUmbrellaGenerator_CascadingValues_Override(t *testing.T) {
	// Parent values must contain the subchart values for override capability
	resources := []*types.ProcessedResource{
		makeProcessedResourceWithValues("Deployment", "app", "default",
			map[string]string{"app.kubernetes.io/name": "app"},
			map[string]interface{}{"replicaCount": int64(3), "image": map[string]interface{}{"repository": "myapp", "tag": "latest"}}, "# app"),
	}
	graph := buildGraph(resources, nil)

	gen := NewUmbrellaGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "1.0.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	parent := findParentChart(charts)
	if parent == nil {
		t.Fatal("parent chart not found")
	}

	// Parent values should contain the subchart name as a top-level key
	if !strings.Contains(parent.ValuesYAML, "app:") {
		t.Error("parent values.yaml missing 'app:' subchart section")
	}
}

// ============================================================
// Subtask 5: Subchart Chart.yaml (child)
// ============================================================

func TestUmbrellaGenerator_SubchartChartYAML(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeProcessedResourceWithValues("Deployment", "frontend", "default",
			map[string]string{"app.kubernetes.io/name": "frontend"},
			nil, "# fe"),
	}
	graph := buildGraph(resources, nil)

	gen := NewUmbrellaGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "1.0.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	subcharts := findSubcharts(charts)
	for _, sub := range subcharts {
		// Subchart must have type: application (not library)
		if strings.Contains(sub.ChartYAML, "type: library") {
			t.Errorf("subchart %s should not be type: library", sub.Name)
		}
		// Subchart Chart.yaml must have apiVersion: v2
		if !strings.Contains(sub.ChartYAML, "apiVersion: v2") {
			t.Errorf("subchart %s Chart.yaml missing apiVersion: v2", sub.Name)
		}
	}
}

// ============================================================
// Subtask 6: Subchart templates correctness
// ============================================================

func TestUmbrellaGenerator_SubchartTemplates_Content(t *testing.T) {
	// Input: Frontend group (Deployment+Service)
	resources := []*types.ProcessedResource{
		makeProcessedResourceWithValues("Deployment", "frontend", "default",
			map[string]string{"app.kubernetes.io/name": "frontend"},
			map[string]interface{}{"replicaCount": int64(2)}, "# deploy"),
		makeProcessedResourceWithValues("Service", "frontend-svc", "default",
			map[string]string{"app.kubernetes.io/name": "frontend"},
			map[string]interface{}{"port": int64(80)}, "# svc"),
	}
	graph := buildGraph(resources, nil)

	gen := NewUmbrellaGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "1.0.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	subcharts := findSubcharts(charts)
	if len(subcharts) != 1 {
		t.Fatalf("expected 1 subchart, got %d", len(subcharts))
	}
	sub := subcharts[0]

	// Subchart must have templates for deployment and service
	hasDeployTemplate := false
	hasSvcTemplate := false
	for path := range sub.Templates {
		if strings.Contains(path, "deployment") {
			hasDeployTemplate = true
		}
		if strings.Contains(path, "service") {
			hasSvcTemplate = true
		}
	}
	if !hasDeployTemplate {
		t.Error("subchart missing deployment template")
	}
	if !hasSvcTemplate {
		t.Error("subchart missing service template")
	}
}

// ============================================================
// Subtask 7: Global values in umbrella
// ============================================================

func TestUmbrellaGenerator_GlobalValues(t *testing.T) {
	// Input: 3 services with common image registry label
	resources := []*types.ProcessedResource{
		makeProcessedResourceWithValues("Deployment", "frontend", "default",
			map[string]string{"app.kubernetes.io/name": "frontend"},
			map[string]interface{}{"image": map[string]interface{}{"repository": "registry.example.com/frontend"}}, "# fe"),
		makeProcessedResourceWithValues("Deployment", "backend", "default",
			map[string]string{"app.kubernetes.io/name": "backend"},
			map[string]interface{}{"image": map[string]interface{}{"repository": "registry.example.com/backend"}}, "# be"),
	}
	graph := buildGraph(resources, nil)

	gen := NewUmbrellaGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "1.0.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	parent := findParentChart(charts)
	if parent == nil {
		t.Fatal("parent chart not found")
	}

	// Parent values must have global: section
	if !strings.Contains(parent.ValuesYAML, "global:") {
		t.Error("parent values.yaml missing 'global:' section")
	}
}

// ============================================================
// Subtask 8: Edge cases
// ============================================================

func TestUmbrellaGenerator_Edge_SingleSubchart(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeProcessedResourceWithValues("Deployment", "solo", "default",
			map[string]string{"app.kubernetes.io/name": "solo"},
			nil, "# solo"),
	}
	graph := buildGraph(resources, nil)

	gen := NewUmbrellaGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "1.0.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	// 1 parent + 1 subchart = 2 total
	if len(charts) != 2 {
		t.Fatalf("expected 2 charts (parent + 1 subchart), got %d", len(charts))
	}

	parent := findParentChart(charts)
	if parent == nil {
		t.Fatal("parent chart not found")
	}

	subcharts := findSubcharts(charts)
	if len(subcharts) != 1 {
		t.Fatalf("expected 1 subchart, got %d", len(subcharts))
	}
}

func TestUmbrellaGenerator_Edge_EmptyGraph(t *testing.T) {
	graph := buildGraph(nil, nil)

	gen := NewUmbrellaGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "1.0.0"})
	// Empty graph: either error or 1 empty parent chart (both are acceptable)
	if err == nil && len(charts) == 0 {
		// OK — no charts generated for empty input
		return
	}
	if err == nil {
		// OK — parent chart with no subcharts is also acceptable
		parent := findParentChart(charts)
		if parent == nil {
			t.Error("expected at least a parent chart for empty graph")
		}
	}
	// err != nil is also acceptable for empty graph
}

// ============================================================
// Helpers
// ============================================================

// findParentChart finds the umbrella parent chart (has dependencies, no "charts/" in path).
func findParentChart(charts []*types.GeneratedChart) *types.GeneratedChart {
	for _, c := range charts {
		if !strings.Contains(c.Path, "charts/") && !strings.HasPrefix(c.Name, "charts/") {
			return c
		}
	}
	// Fallback: find chart with dependencies in Chart.yaml
	for _, c := range charts {
		if strings.Contains(c.ChartYAML, "dependencies:") {
			return c
		}
	}
	return nil
}

// findSubcharts returns all non-parent charts (subcharts in charts/ subdirectory).
func findSubcharts(charts []*types.GeneratedChart) []*types.GeneratedChart {
	var subs []*types.GeneratedChart
	parent := findParentChart(charts)
	for _, c := range charts {
		if parent != nil && c == parent {
			continue
		}
		subs = append(subs, c)
	}
	return subs
}

func chartNamesList(charts []*types.GeneratedChart) []string {
	names := make([]string, len(charts))
	for i, c := range charts {
		names[i] = fmt.Sprintf("%s(path=%s)", c.Name, c.Path)
	}
	return names
}
