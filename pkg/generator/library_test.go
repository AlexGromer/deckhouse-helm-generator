package generator

import (
	"context"
	"strings"
	"testing"

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
// Subtask 3: Named deployment template
// ============================================================

func TestLibraryGenerator_NamedTemplate_Deployment(t *testing.T) {
	deploy := makeProcessedResourceWithValues("Deployment", "app", "default",
		map[string]string{"app.kubernetes.io/name": "app"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	libChart := findLibraryChart(charts)
	if libChart == nil {
		t.Fatal("library chart not found")
	}

	// Check for named deployment template
	found := false
	for _, content := range libChart.Templates {
		if strings.Contains(content, `define "library.deployment"`) {
			found = true
			break
		}
	}
	if !found {
		t.Error("library chart missing named template 'library.deployment'")
	}
}

// ============================================================
// Subtask 4: Named service template
// ============================================================

func TestLibraryGenerator_NamedTemplate_Service(t *testing.T) {
	deploy := makeProcessedResourceWithValues("Deployment", "app", "default",
		map[string]string{"app.kubernetes.io/name": "app"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	libChart := findLibraryChart(charts)
	if libChart == nil {
		t.Fatal("library chart not found")
	}

	found := false
	for _, content := range libChart.Templates {
		if strings.Contains(content, `define "library.service"`) {
			found = true
			break
		}
	}
	if !found {
		t.Error("library chart missing named template 'library.service'")
	}
}

// ============================================================
// Subtask 5: Named statefulset template
// ============================================================

func TestLibraryGenerator_NamedTemplate_StatefulSet(t *testing.T) {
	deploy := makeProcessedResourceWithValues("Deployment", "app", "default",
		map[string]string{"app.kubernetes.io/name": "app"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	libChart := findLibraryChart(charts)
	if libChart == nil {
		t.Fatal("library chart not found")
	}

	found := false
	for _, content := range libChart.Templates {
		if strings.Contains(content, `define "library.statefulset"`) {
			found = true
			break
		}
	}
	if !found {
		t.Error("library chart missing named template 'library.statefulset'")
	}
}

// ============================================================
// Subtask 6: Named templates for all remaining resource types
// ============================================================

func TestLibraryGenerator_NamedTemplate_AllTypes(t *testing.T) {
	deploy := makeProcessedResourceWithValues("Deployment", "app", "default",
		map[string]string{"app.kubernetes.io/name": "app"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	libChart := findLibraryChart(charts)
	if libChart == nil {
		t.Fatal("library chart not found")
	}

	// All template content concatenated for searching
	var allTemplateContent strings.Builder
	for _, content := range libChart.Templates {
		allTemplateContent.WriteString(content)
	}
	all := allTemplateContent.String()

	expectedTemplates := []string{
		"library.deployment",
		"library.statefulset",
		"library.daemonset",
		"library.service",
		"library.ingress",
		"library.configmap",
		"library.secret",
		"library.pvc",
		"library.hpa",
		"library.pdb",
		"library.networkpolicy",
		"library.cronjob",
		"library.job",
		"library.serviceaccount",
		"library.role",
		"library.clusterrole",
		"library.rolebinding",
		"library.clusterrolebinding",
	}

	for _, tmpl := range expectedTemplates {
		if !strings.Contains(all, `define "`+tmpl+`"`) {
			t.Errorf("library chart missing named template '%s'", tmpl)
		}
	}
}

// ============================================================
// Subtask 7: Template parameterization via dict pattern
// ============================================================

func TestLibraryGenerator_Templates_DictPattern(t *testing.T) {
	deploy := makeProcessedResourceWithValues("Deployment", "app", "default",
		map[string]string{"app.kubernetes.io/name": "app"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	libChart := findLibraryChart(charts)
	if libChart == nil {
		t.Fatal("library chart not found")
	}

	// Check that templates use context/values pattern
	var allContent strings.Builder
	for _, content := range libChart.Templates {
		allContent.WriteString(content)
	}
	all := allContent.String()

	// Templates should reference .context or .values for parameterization
	if !strings.Contains(all, ".values") && !strings.Contains(all, ".Values") {
		t.Error("library templates should reference .values or .Values for parameterization")
	}
}

// ============================================================
// Subtask 8: Template content for deployment
// ============================================================

func TestLibraryGenerator_TemplateContent_Deployment_Replicas(t *testing.T) {
	deploy := makeProcessedResourceWithValues("Deployment", "app", "default",
		map[string]string{"app.kubernetes.io/name": "app"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	libChart := findLibraryChart(charts)
	if libChart == nil {
		t.Fatal("library chart not found")
	}

	// Find deployment template content
	deployContent := ""
	for _, content := range libChart.Templates {
		if strings.Contains(content, `define "library.deployment"`) {
			deployContent = content
			break
		}
	}

	if deployContent == "" {
		t.Fatal("deployment template not found in library chart")
	}

	if !strings.Contains(deployContent, "replicaCount") && !strings.Contains(deployContent, "replicas") {
		t.Error("deployment template should reference replicaCount or replicas")
	}
}

func TestLibraryGenerator_TemplateContent_Deployment_Image(t *testing.T) {
	deploy := makeProcessedResourceWithValues("Deployment", "app", "default",
		map[string]string{"app.kubernetes.io/name": "app"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	libChart := findLibraryChart(charts)
	if libChart == nil {
		t.Fatal("library chart not found")
	}

	deployContent := ""
	for _, content := range libChart.Templates {
		if strings.Contains(content, `define "library.deployment"`) {
			deployContent = content
			break
		}
	}

	if deployContent == "" {
		t.Fatal("deployment template not found")
	}

	if !strings.Contains(deployContent, "image") {
		t.Error("deployment template should reference image")
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
	// Expected: Still generates all named templates (library is generic)
	deploy := makeProcessedResourceWithValues("Deployment", "app", "default",
		map[string]string{"app.kubernetes.io/name": "app"},
		map[string]interface{}{"replicaCount": 1}, "# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	libChart := findLibraryChart(charts)
	if libChart == nil {
		t.Fatal("library chart not found")
	}

	// Should have templates for all types, not just Deployment
	if len(libChart.Templates) < 5 {
		t.Errorf("expected multiple template files in library chart, got %d", len(libChart.Templates))
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
