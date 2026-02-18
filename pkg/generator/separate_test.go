package generator

import (
	"context"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Test helpers for SeparateGenerator
// ============================================================

// makeProcessedResourceWithValues creates a ProcessedResource with template content and values.
func makeProcessedResourceWithValues(kind, name, namespace string, labels map[string]string, values map[string]interface{}, templateContent string) *types.ProcessedResource {
	r := makeProcessedResource(kind, name, namespace, labels)
	r.Values = values
	r.TemplateContent = templateContent
	r.TemplatePath = "templates/" + strings.ToLower(kind) + ".yaml"
	return r
}

// findChartByName finds a generated chart by name.
func findChartByName(charts []*types.GeneratedChart, name string) *types.GeneratedChart {
	for _, c := range charts {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// ============================================================
// Subtask 1: SeparateGenerator implements Generator interface
// ============================================================

func TestSeparateGenerator_ImplementsInterface(t *testing.T) {
	// Compile-time check that SeparateGenerator implements Generator
	var _ Generator = (*SeparateGenerator)(nil)
}

func TestSeparateGenerator_Mode(t *testing.T) {
	gen := NewSeparateGenerator()
	if gen.Mode() != types.OutputModeSeparate {
		t.Errorf("expected mode %s, got %s", types.OutputModeSeparate, gen.Mode())
	}
}

// ============================================================
// Subtask 2: Single service chart generation
// ============================================================

func TestSeparateGenerator_SingleGroup_ChartStructure(t *testing.T) {
	// Input: 1 group "myapp" (Deployment + Service)
	// Expected: 1 GeneratedChart with Chart.yaml, values.yaml, templates/deployment.yaml, templates/service.yaml
	deploy := makeProcessedResourceWithValues("Deployment", "myapp-deploy", "default",
		map[string]string{"app.kubernetes.io/name": "myapp"},
		map[string]interface{}{"replicaCount": 1, "image": map[string]interface{}{"repository": "nginx", "tag": "1.21"}},
		"# deployment template")
	svc := makeProcessedResourceWithValues("Service", "myapp-svc", "default",
		map[string]string{"app.kubernetes.io/name": "myapp"},
		map[string]interface{}{"type": "ClusterIP", "ports": []interface{}{map[string]interface{}{"port": 80}}},
		"# service template")

	graph := buildGraph([]*types.ProcessedResource{deploy, svc}, nil)

	gen := NewSeparateGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{
		ChartName:    "test",
		ChartVersion: "0.1.0",
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if len(charts) != 1 {
		t.Fatalf("expected 1 chart, got %d", len(charts))
	}

	chart := charts[0]
	if chart.ChartYAML == "" {
		t.Error("Chart.yaml is empty")
	}
	if chart.ValuesYAML == "" {
		t.Error("values.yaml is empty")
	}
	if len(chart.Templates) == 0 {
		t.Error("no templates generated")
	}
}

func TestSeparateGenerator_SingleGroup_ChartName(t *testing.T) {
	// Input: Group named "frontend"
	// Expected: Chart.yaml name: frontend
	deploy := makeProcessedResourceWithValues("Deployment", "frontend-deploy", "default",
		map[string]string{"app.kubernetes.io/name": "frontend"},
		map[string]interface{}{"replicaCount": 1},
		"# deployment template")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewSeparateGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{
		ChartName:    "test",
		ChartVersion: "0.1.0",
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if len(charts) != 1 {
		t.Fatalf("expected 1 chart, got %d", len(charts))
	}

	if charts[0].Name != "frontend" {
		t.Errorf("expected chart name 'frontend', got '%s'", charts[0].Name)
	}

	if !strings.Contains(charts[0].ChartYAML, "name: frontend") {
		t.Errorf("Chart.yaml does not contain 'name: frontend':\n%s", charts[0].ChartYAML)
	}
}

// ============================================================
// Subtask 3: Multiple service charts
// ============================================================

func TestSeparateGenerator_MultipleGroups_ChartCount(t *testing.T) {
	// Input: 3 groups (frontend, backend, database)
	// Expected: 3 GeneratedCharts
	resources := []*types.ProcessedResource{
		makeProcessedResourceWithValues("Deployment", "frontend-deploy", "default",
			map[string]string{"app.kubernetes.io/name": "frontend"},
			map[string]interface{}{"replicaCount": 2}, "# deploy"),
		makeProcessedResourceWithValues("Service", "frontend-svc", "default",
			map[string]string{"app.kubernetes.io/name": "frontend"},
			map[string]interface{}{"type": "ClusterIP"}, "# svc"),
		makeProcessedResourceWithValues("Ingress", "frontend-ingress", "default",
			map[string]string{"app.kubernetes.io/name": "frontend"},
			map[string]interface{}{"host": "example.com"}, "# ingress"),

		makeProcessedResourceWithValues("Deployment", "backend-deploy", "default",
			map[string]string{"app.kubernetes.io/name": "backend"},
			map[string]interface{}{"replicaCount": 3}, "# deploy"),
		makeProcessedResourceWithValues("Service", "backend-svc", "default",
			map[string]string{"app.kubernetes.io/name": "backend"},
			map[string]interface{}{"type": "ClusterIP"}, "# svc"),
		makeProcessedResourceWithValues("ConfigMap", "backend-config", "default",
			map[string]string{"app.kubernetes.io/name": "backend"},
			map[string]interface{}{"data": "config"}, "# cm"),

		makeProcessedResourceWithValues("StatefulSet", "database-sts", "default",
			map[string]string{"app.kubernetes.io/name": "database"},
			map[string]interface{}{"replicaCount": 1}, "# sts"),
		makeProcessedResourceWithValues("Service", "database-svc", "default",
			map[string]string{"app.kubernetes.io/name": "database"},
			map[string]interface{}{"type": "ClusterIP"}, "# svc"),
		makeProcessedResourceWithValues("PersistentVolumeClaim", "database-pvc", "default",
			map[string]string{"app.kubernetes.io/name": "database"},
			map[string]interface{}{"size": "10Gi"}, "# pvc"),
	}

	graph := buildGraph(resources, nil)

	gen := NewSeparateGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if len(charts) != 3 {
		t.Fatalf("expected 3 charts, got %d", len(charts))
	}
}

func TestSeparateGenerator_MultipleGroups_TemplateIsolation(t *testing.T) {
	// Each chart should only contain templates for its own resources
	resources := []*types.ProcessedResource{
		makeProcessedResourceWithValues("Deployment", "frontend-deploy", "default",
			map[string]string{"app.kubernetes.io/name": "frontend"},
			map[string]interface{}{"replicaCount": 2}, "# frontend deploy"),
		makeProcessedResourceWithValues("Service", "frontend-svc", "default",
			map[string]string{"app.kubernetes.io/name": "frontend"},
			map[string]interface{}{"type": "ClusterIP"}, "# frontend svc"),

		makeProcessedResourceWithValues("Deployment", "backend-deploy", "default",
			map[string]string{"app.kubernetes.io/name": "backend"},
			map[string]interface{}{"replicaCount": 3}, "# backend deploy"),
		makeProcessedResourceWithValues("Service", "backend-svc", "default",
			map[string]string{"app.kubernetes.io/name": "backend"},
			map[string]interface{}{"type": "ClusterIP"}, "# backend svc"),
	}

	graph := buildGraph(resources, nil)

	gen := NewSeparateGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if len(charts) != 2 {
		t.Fatalf("expected 2 charts, got %d", len(charts))
	}

	frontend := findChartByName(charts, "frontend")
	backend := findChartByName(charts, "backend")

	if frontend == nil {
		t.Fatal("frontend chart not found")
	}
	if backend == nil {
		t.Fatal("backend chart not found")
	}

	// Frontend templates should not contain backend content
	for _, content := range frontend.Templates {
		if strings.Contains(content, "backend") {
			t.Error("frontend chart contains backend template content")
		}
	}

	// Backend templates should not contain frontend content
	for _, content := range backend.Templates {
		if strings.Contains(content, "frontend") {
			t.Error("backend chart contains frontend template content")
		}
	}
}

// ============================================================
// Subtask 4: Chart.yaml per service
// ============================================================

func TestSeparateGenerator_ChartYAML_RequiredFields(t *testing.T) {
	// Expected: apiVersion: v2, name, version: "0.1.0", type: application
	deploy := makeProcessedResourceWithValues("Deployment", "myapp", "default",
		map[string]string{"app.kubernetes.io/name": "myapp"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewSeparateGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	chartYAML := charts[0].ChartYAML
	if !strings.Contains(chartYAML, "apiVersion: v2") {
		t.Error("Chart.yaml missing apiVersion: v2")
	}
	if !strings.Contains(chartYAML, "name: myapp") {
		t.Error("Chart.yaml missing name: myapp")
	}
	if !strings.Contains(chartYAML, "version: 0.1.0") {
		t.Error("Chart.yaml missing version: 0.1.0")
	}
	if !strings.Contains(chartYAML, "type: application") {
		t.Error("Chart.yaml missing type: application")
	}
}

func TestSeparateGenerator_ChartYAML_Description(t *testing.T) {
	// Expected: Description auto-generated from service name
	deploy := makeProcessedResourceWithValues("Deployment", "frontend", "default",
		map[string]string{"app.kubernetes.io/name": "frontend"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewSeparateGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if !strings.Contains(charts[0].ChartYAML, "frontend") {
		t.Error("Chart.yaml description does not contain service name")
	}
}

// ============================================================
// Subtask 5: Values scoped to service (flat, no service prefix)
// ============================================================

func TestSeparateGenerator_Values_FlatStructure(t *testing.T) {
	// Input: Group with Deployment (replicas:3, image:nginx:1.21)
	// Expected: values["replicaCount"] == 3, NOT values["frontend"]["replicaCount"]
	deploy := makeProcessedResourceWithValues("Deployment", "frontend-deploy", "default",
		map[string]string{"app.kubernetes.io/name": "frontend"},
		map[string]interface{}{"replicaCount": int64(3), "image": map[string]interface{}{"repository": "nginx", "tag": "1.21"}},
		"# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewSeparateGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	valuesYAML := charts[0].ValuesYAML
	// Should have replicaCount at top level (or under deployment key) - NOT under "frontend."
	if strings.Contains(valuesYAML, "frontend:") {
		t.Error("values.yaml contains service name prefix 'frontend:' — should be flat")
	}
	// Should NOT have services section
	if strings.Contains(valuesYAML, "services:") {
		t.Error("values.yaml contains 'services:' section — should be flat for separate mode")
	}
}

func TestSeparateGenerator_Values_NoServiceNesting(t *testing.T) {
	// Expected: No service-name-prefixed keys in values
	resources := []*types.ProcessedResource{
		makeProcessedResourceWithValues("Deployment", "backend-deploy", "default",
			map[string]string{"app.kubernetes.io/name": "backend"},
			map[string]interface{}{"replicaCount": int64(2)}, "# deploy"),
		makeProcessedResourceWithValues("Service", "backend-svc", "default",
			map[string]string{"app.kubernetes.io/name": "backend"},
			map[string]interface{}{"type": "ClusterIP"}, "# svc"),
	}

	graph := buildGraph(resources, nil)

	gen := NewSeparateGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	chart := findChartByName(charts, "backend")
	if chart == nil {
		t.Fatal("backend chart not found")
	}

	// Values should not contain services section or backend nesting
	if strings.Contains(chart.ValuesYAML, "services:") {
		t.Error("values.yaml contains 'services:' — separate mode should use flat values")
	}
}

// ============================================================
// Subtask 6: _helpers.tpl per service
// ============================================================

func TestSeparateGenerator_Helpers_Fullname(t *testing.T) {
	// Expected: _helpers.tpl contains chart-specific fullname template
	deploy := makeProcessedResourceWithValues("Deployment", "myapp", "default",
		map[string]string{"app.kubernetes.io/name": "myapp"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewSeparateGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	helpers := charts[0].Helpers
	if !strings.Contains(helpers, `define "myapp.fullname"`) {
		t.Errorf("_helpers.tpl missing fullname template for 'myapp':\n%s", helpers)
	}
}

func TestSeparateGenerator_Helpers_Labels(t *testing.T) {
	// Expected: _helpers.tpl contains labels template
	deploy := makeProcessedResourceWithValues("Deployment", "myapp", "default",
		map[string]string{"app.kubernetes.io/name": "myapp"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewSeparateGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	helpers := charts[0].Helpers
	if !strings.Contains(helpers, `define "myapp.labels"`) {
		t.Errorf("_helpers.tpl missing labels template for 'myapp':\n%s", helpers)
	}
}

// ============================================================
// Subtask 7: Template content correctness
// ============================================================

func TestSeparateGenerator_Templates_DeploymentContent(t *testing.T) {
	// Input: Group with Deployment
	// Expected: Generated template references .Values.replicaCount, .Values.image
	deploy := makeProcessedResourceWithValues("Deployment", "myapp-deploy", "default",
		map[string]string{"app.kubernetes.io/name": "myapp"},
		map[string]interface{}{"replicaCount": 1, "image": map[string]interface{}{"repository": "nginx", "tag": "latest"}},
		"# deployment template content")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewSeparateGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if len(charts[0].Templates) == 0 {
		t.Fatal("no templates generated")
	}

	// Check that at least one template exists
	hasTemplate := false
	for path := range charts[0].Templates {
		if strings.Contains(path, "deployment") {
			hasTemplate = true
			break
		}
	}
	if !hasTemplate {
		t.Error("no deployment template found in chart templates")
	}
}

func TestSeparateGenerator_Templates_ServiceContent(t *testing.T) {
	// Input: Group with Service
	// Expected: Generated template exists for service
	svc := makeProcessedResourceWithValues("Service", "myapp-svc", "default",
		map[string]string{"app.kubernetes.io/name": "myapp"},
		map[string]interface{}{"type": "ClusterIP", "ports": []interface{}{map[string]interface{}{"port": 80}}},
		"# service template content")

	graph := buildGraph([]*types.ProcessedResource{svc}, nil)

	gen := NewSeparateGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	hasTemplate := false
	for path := range charts[0].Templates {
		if strings.Contains(path, "service") {
			hasTemplate = true
			break
		}
	}
	if !hasTemplate {
		t.Error("no service template found in chart templates")
	}
}

// ============================================================
// Subtask 8: Edge cases
// ============================================================

func TestSeparateGenerator_Edge_EmptyGraph(t *testing.T) {
	// Input: Empty resource graph
	// Expected: 0 charts, no error
	graph := buildGraph(nil, nil)

	gen := NewSeparateGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if len(charts) != 0 {
		t.Errorf("expected 0 charts for empty graph, got %d", len(charts))
	}
}

func TestSeparateGenerator_Edge_SingleResourceGroup(t *testing.T) {
	// Input: 1 group with 1 ConfigMap
	// Expected: 1 chart with 1 template
	cm := makeProcessedResourceWithValues("ConfigMap", "app-config", "default",
		map[string]string{"app.kubernetes.io/name": "config"},
		map[string]interface{}{"data": "value"},
		"# configmap template")

	graph := buildGraph([]*types.ProcessedResource{cm}, nil)

	gen := NewSeparateGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if len(charts) != 1 {
		t.Fatalf("expected 1 chart, got %d", len(charts))
	}

	if len(charts[0].Templates) < 1 {
		t.Error("expected at least 1 template")
	}
}
