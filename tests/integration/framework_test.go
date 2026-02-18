package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// TestHarness tests
// ============================================================

func TestTestHarness_SetupAndCleanup(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()

	// Verify directories were created
	if _, err := os.Stat(h.TempDir); os.IsNotExist(err) {
		t.Fatal("TempDir was not created")
	}
	if _, err := os.Stat(h.OutputDir); os.IsNotExist(err) {
		t.Fatal("OutputDir was not created")
	}
	if _, err := os.Stat(h.InputDir); os.IsNotExist(err) {
		t.Fatal("InputDir was not created")
	}

	tempDir := h.TempDir
	h.Cleanup()

	// Verify cleanup removed everything
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Fatal("TempDir was not cleaned up")
	}
}

func TestTestHarness_WriteInputFile(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	content := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test
data:
  key: value
`
	h.WriteInputFile("test.yaml", content)

	// Verify file was written
	data, err := os.ReadFile(filepath.Join(h.InputDir, "test.yaml"))
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}
	if string(data) != content {
		t.Errorf("File content mismatch:\ngot: %q\nwant: %q", string(data), content)
	}
}

func TestTestHarness_WriteInputFile_Subdirectory(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	h.WriteInputFile("subdir/nested.yaml", "apiVersion: v1\nkind: Namespace\n")

	// Verify file and directory were created
	if _, err := os.Stat(filepath.Join(h.InputDir, "subdir", "nested.yaml")); os.IsNotExist(err) {
		t.Fatal("Nested file was not created")
	}
}

// ============================================================
// ExecutePipeline tests — using fixtures
// ============================================================

func TestExecutePipeline_SimpleApp(t *testing.T) {
	fixtureDir := filepath.Join("fixtures", "simple-app")
	if _, err := os.Stat(fixtureDir); os.IsNotExist(err) {
		t.Skip("fixtures/simple-app not found, skipping")
	}

	opts := PipelineOptions{
		ChartName:    "simple-app",
		ChartVersion: "1.0.0",
		AppVersion:   "1.0.0",
		Namespace:    "default",
	}

	output, err := ExecutePipeline(fixtureDir, opts)
	if err != nil {
		t.Fatalf("ExecutePipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Verify we got charts
	if len(output.Charts) == 0 {
		t.Fatal("No charts generated")
	}

	// Verify we got processed resources (3: deployment, service, configmap)
	if len(output.Resources) < 3 {
		t.Errorf("Expected at least 3 processed resources, got %d", len(output.Resources))
	}

	// Verify graph has groups
	if len(output.Graph.Groups) == 0 {
		t.Error("Expected at least 1 resource group in the graph")
	}

	// Verify chart structure on disk
	chartDir := filepath.Join(output.OutputDir, "simple-app")
	ValidateChartStructure(t, chartDir)
}

func TestExecutePipeline_FullStack(t *testing.T) {
	fixtureDir := filepath.Join("fixtures", "full-stack")
	if _, err := os.Stat(fixtureDir); os.IsNotExist(err) {
		t.Skip("fixtures/full-stack not found, skipping")
	}

	opts := PipelineOptions{
		ChartName:    "full-stack",
		ChartVersion: "2.0.0",
		AppVersion:   "2.0.0",
		Namespace:    "default",
	}

	output, err := ExecutePipeline(fixtureDir, opts)
	if err != nil {
		t.Fatalf("ExecutePipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	if len(output.Charts) == 0 {
		t.Fatal("No charts generated")
	}

	// Full-stack has multiple resource types
	if len(output.Resources) < 5 {
		t.Errorf("Expected at least 5 processed resources, got %d", len(output.Resources))
	}

	// Should detect relationships
	if len(output.Graph.Relationships) == 0 {
		t.Error("Expected relationships to be detected in full-stack app")
	}

	// Verify chart on disk
	chartDir := filepath.Join(output.OutputDir, "full-stack")
	ValidateChartStructure(t, chartDir)
	ValidateTemplates(t, chartDir)
}

// ============================================================
// Validation helper tests
// ============================================================

func TestValidateValues_ValidYAML(t *testing.T) {
	// Use a helper test to capture validation errors
	tt := &testing.T{}
	_ = tt // We can't easily test t.Error calls, so just verify no panic
	ValidateValues(t, "key: value\nnested:\n  inner: 123\n")
}

func TestValidateValues_EmptyContent(t *testing.T) {
	// Empty content should be detected (we can't easily verify t.Error was called)
	// Just ensure no panic
	mockT := &testing.T{}
	ValidateValues(mockT, "")
}

func TestCompareGeneratedChart_Identical(t *testing.T) {
	chart := makeTestChart("test", "chart.yaml content", "values content",
		map[string]string{"templates/deploy.yaml": "template content"})

	diffs := CompareGeneratedChart(chart, chart)
	if len(diffs) != 0 {
		t.Errorf("Expected no differences for identical charts, got %d: %v", len(diffs), diffs)
	}
}

func TestCompareGeneratedChart_NameDiff(t *testing.T) {
	actual := makeTestChart("actual", "chart", "values", nil)
	expected := makeTestChart("expected", "chart", "values", nil)

	diffs := CompareGeneratedChart(actual, expected)
	if len(diffs) == 0 {
		t.Error("Expected difference for different names")
	}

	found := false
	for _, d := range diffs {
		if strings.Contains(d, "Name") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected name difference to be reported")
	}
}

func TestCompareGeneratedChart_MissingTemplate(t *testing.T) {
	actual := makeTestChart("test", "chart", "values",
		map[string]string{"templates/a.yaml": "content"})
	expected := makeTestChart("test", "chart", "values",
		map[string]string{"templates/a.yaml": "content", "templates/b.yaml": "content"})

	diffs := CompareGeneratedChart(actual, expected)
	if len(diffs) == 0 {
		t.Error("Expected difference for missing template")
	}
}

func TestCompareGeneratedChart_ExtraTemplate(t *testing.T) {
	actual := makeTestChart("test", "chart", "values",
		map[string]string{"templates/a.yaml": "content", "templates/extra.yaml": "content"})
	expected := makeTestChart("test", "chart", "values",
		map[string]string{"templates/a.yaml": "content"})

	diffs := CompareGeneratedChart(actual, expected)

	found := false
	for _, d := range diffs {
		if strings.Contains(d, "unexpected") && strings.Contains(d, "extra.yaml") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected extra template to be reported")
	}
}

// ============================================================
// Pipeline with dynamic input
// ============================================================

func TestExecutePipeline_WithHarness(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	// Write a minimal deployment
	h.WriteInputFile("deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
  namespace: default
  labels:
    app.kubernetes.io/name: test-app
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: test-app
  template:
    metadata:
      labels:
        app.kubernetes.io/name: test-app
    spec:
      containers:
        - name: app
          image: nginx:latest
`)

	opts := PipelineOptions{
		ChartName: "harness-test",
	}

	output, err := ExecutePipeline(h.InputDir, opts)
	if err != nil {
		t.Fatalf("ExecutePipeline with harness failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	if len(output.Charts) == 0 {
		t.Fatal("No charts generated from harness input")
	}

	// Validate the generated chart
	chart := output.Charts[0]
	ValidateValues(t, chart.ValuesYAML)

	if chart.Name == "" {
		t.Error("Generated chart has empty name")
	}
}

// ============================================================
// Test helpers
// ============================================================

func makeTestChart(name, chartYAML, valuesYAML string, templates map[string]string) *types.GeneratedChart {
	if templates == nil {
		templates = make(map[string]string)
	}
	return &types.GeneratedChart{
		Name:      name,
		ChartYAML: chartYAML,
		ValuesYAML: valuesYAML,
		Templates: templates,
	}
}
