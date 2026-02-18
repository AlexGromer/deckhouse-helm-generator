package generator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// threeServiceGraph builds a graph with frontend, backend, database deployments.
func threeServiceGraph() *types.ResourceGraph {
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
	return buildGraph(resources, nil)
}

// ============================================================
// Subtask 1: Condition field in parent Chart.yaml dependencies
// ============================================================

func TestConditionalSubcharts_ConditionField(t *testing.T) {
	graph := threeServiceGraph()

	gen := NewUmbrellaGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{
		ChartVersion: "1.0.0",
		ChartName:    "myapp",
	})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	parent := findParentChart(charts)
	if parent == nil {
		t.Fatal("parent chart not found")
	}

	// Check that each subchart dependency has condition: <name>.enabled
	for _, name := range []string{"frontend", "backend", "database"} {
		expected := fmt.Sprintf("condition: %s.enabled", name)
		if !strings.Contains(parent.ChartYAML, expected) {
			t.Errorf("parent Chart.yaml missing %q\nChart.yaml:\n%s", expected, parent.ChartYAML)
		}
	}
}

// ============================================================
// Subtask 2: Default enabled values
// ============================================================

func TestConditionalSubcharts_DefaultEnabled(t *testing.T) {
	graph := threeServiceGraph()

	gen := NewUmbrellaGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{
		ChartVersion: "1.0.0",
		ChartName:    "myapp",
	})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	parent := findParentChart(charts)
	if parent == nil {
		t.Fatal("parent chart not found")
	}

	// Each subchart must have enabled: true in parent values
	for _, name := range []string{"frontend", "backend", "database"} {
		// Check that the name section exists
		if !strings.Contains(parent.ValuesYAML, name+":") {
			t.Errorf("parent values missing '%s:' section", name)
		}
		// Check enabled: true is somewhere in the values (under the subchart section)
		if !strings.Contains(parent.ValuesYAML, "enabled: true") {
			t.Errorf("parent values missing 'enabled: true' for %s", name)
		}
	}
}

// ============================================================
// Subtask 3: Subchart disabling via helm template
// ============================================================

func TestConditionalSubcharts_DisableSubchart(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not found in PATH, skipping helm template test")
	}

	graph := threeServiceGraph()
	gen := NewUmbrellaGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{
		ChartVersion: "1.0.0",
		ChartName:    "myapp",
	})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	outputDir, err := os.MkdirTemp("", "dhg-cond-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(outputDir)

	// Write all charts to disk
	for _, chart := range charts {
		if err := WriteChart(chart, outputDir); err != nil {
			t.Fatalf("WriteChart failed for %s: %v", chart.Name, err)
		}
	}

	parentDir := filepath.Join(outputDir, "myapp")

	// Run helm dependency build
	depCmd := exec.Command("helm", "dependency", "build", parentDir)
	if depOut, depErr := depCmd.CombinedOutput(); depErr != nil {
		t.Skipf("helm dependency build failed (skipping): %v\n%s", depErr, string(depOut))
	}

	// Render with database disabled
	cmd := exec.Command("helm", "template", "myapp", parentDir,
		"--set", "database.enabled=false")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\nOutput: %s", err, string(out))
	}

	rendered := string(out)
	// Database resources must NOT appear when disabled
	if strings.Contains(rendered, "name: database") {
		t.Error("database resources should not be rendered when database.enabled=false")
	}
}

// ============================================================
// Subtask 4: Isolated toggle (other subcharts unaffected)
// ============================================================

func TestConditionalSubcharts_IsolatedToggle(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not found in PATH, skipping helm template test")
	}

	graph := threeServiceGraph()
	gen := NewUmbrellaGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{
		ChartVersion: "1.0.0",
		ChartName:    "myapp",
	})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	outputDir, err := os.MkdirTemp("", "dhg-cond-iso-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(outputDir)

	for _, chart := range charts {
		if err := WriteChart(chart, outputDir); err != nil {
			t.Fatalf("WriteChart failed: %v", err)
		}
	}

	parentDir := filepath.Join(outputDir, "myapp")

	depCmd := exec.Command("helm", "dependency", "build", parentDir)
	if depOut, depErr := depCmd.CombinedOutput(); depErr != nil {
		t.Skipf("helm dependency build failed: %v\n%s", depErr, string(depOut))
	}

	// Disable database, frontend and backend should still render
	cmd := exec.Command("helm", "template", "myapp", parentDir,
		"--set", "database.enabled=false")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\nOutput: %s", err, string(out))
	}

	rendered := string(out)
	// Frontend resources must still be present
	if !strings.Contains(rendered, "frontend") {
		t.Error("frontend resources should still be rendered when only database is disabled")
	}
	// Backend resources must still be present
	if !strings.Contains(rendered, "backend") {
		t.Error("backend resources should still be rendered when only database is disabled")
	}
}

// ============================================================
// Subtask 5: All subcharts disabled
// ============================================================

func TestConditionalSubcharts_AllDisabled(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not found in PATH, skipping helm template test")
	}

	graph := threeServiceGraph()
	gen := NewUmbrellaGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{
		ChartVersion: "1.0.0",
		ChartName:    "myapp",
	})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	outputDir, err := os.MkdirTemp("", "dhg-cond-all-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(outputDir)

	for _, chart := range charts {
		if err := WriteChart(chart, outputDir); err != nil {
			t.Fatalf("WriteChart failed: %v", err)
		}
	}

	parentDir := filepath.Join(outputDir, "myapp")

	depCmd := exec.Command("helm", "dependency", "build", parentDir)
	if depOut, depErr := depCmd.CombinedOutput(); depErr != nil {
		t.Skipf("helm dependency build failed: %v\n%s", depErr, string(depOut))
	}

	// Disable all subcharts
	cmd := exec.Command("helm", "template", "myapp", parentDir,
		"--set", "frontend.enabled=false",
		"--set", "backend.enabled=false",
		"--set", "database.enabled=false")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\nOutput: %s", err, string(out))
	}

	// With all subcharts disabled, no K8s resource manifests should be rendered
	rendered := string(out)
	// Rendered output should be empty or only contain whitespace/separators
	lines := strings.Split(strings.TrimSpace(rendered), "\n")
	hasResource := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "apiVersion:") || strings.HasPrefix(line, "kind:") {
			hasResource = true
			break
		}
	}
	if hasResource {
		t.Errorf("expected no resources when all subcharts disabled, but got:\n%s", rendered)
	}
}
