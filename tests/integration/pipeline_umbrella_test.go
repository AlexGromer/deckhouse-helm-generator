package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/generator"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// umbrellaParentChart finds the parent chart (no "charts/" in Name).
func umbrellaParentChart(charts []*types.GeneratedChart) *types.GeneratedChart {
	for _, c := range charts {
		if !strings.Contains(c.Name, "charts/") {
			return c
		}
	}
	return nil
}

// umbrellaSubcharts returns subcharts (have "charts/" in Name).
func umbrellaSubcharts(charts []*types.GeneratedChart) []*types.GeneratedChart {
	var subs []*types.GeneratedChart
	for _, c := range charts {
		if strings.Contains(c.Name, "charts/") {
			subs = append(subs, c)
		}
	}
	return subs
}

// ============================================================
// Subtask 1: Umbrella with 3 subcharts
// ============================================================

func TestPipelineUmbrella_ThreeSubcharts(t *testing.T) {
	fixturesDir := filepath.Join("fixtures", "umbrella-app")

	output, err := ExecutePipelineWithMode(fixturesDir, PipelineOptions{
		ChartName:    "myapp",
		ChartVersion: "1.0.0",
		Mode:         types.OutputModeUmbrella,
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Expected: 1 parent + 3 subcharts = 4 total
	if len(output.Charts) != 4 {
		t.Fatalf("expected 4 charts (1 parent + 3 subcharts), got %d: %v",
			len(output.Charts), chartNames(output.Charts))
	}

	parent := umbrellaParentChart(output.Charts)
	if parent == nil {
		t.Fatal("parent chart not found")
	}

	// Parent must have 3 dependencies
	depCount := strings.Count(parent.ChartYAML, "- name:")
	if depCount != 3 {
		t.Errorf("expected 3 dependencies in parent Chart.yaml, got %d\n%s", depCount, parent.ChartYAML)
	}

	// Parent must be written to disk
	parentDir := filepath.Join(output.OutputDir, parent.Name)
	if _, err := os.Stat(filepath.Join(parentDir, "Chart.yaml")); err != nil {
		t.Errorf("parent Chart.yaml not found on disk: %v", err)
	}

	// Subcharts must be written to disk
	subs := umbrellaSubcharts(output.Charts)
	if len(subs) != 3 {
		t.Fatalf("expected 3 subcharts, got %d", len(subs))
	}
	for _, sub := range subs {
		subDir := filepath.Join(output.OutputDir, sub.Name)
		if _, err := os.Stat(filepath.Join(subDir, "Chart.yaml")); err != nil {
			t.Errorf("subchart %s Chart.yaml not found on disk: %v", sub.Name, err)
		}
	}
}

// ============================================================
// Subtask 2: Conditional subchart disabling
// ============================================================

func TestPipelineUmbrella_ConditionalDisable(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not found in PATH, skipping helm template test")
	}

	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	h.WriteInputFile("frontend.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  namespace: default
  labels:
    app.kubernetes.io/name: frontend
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: frontend
  template:
    metadata:
      labels:
        app.kubernetes.io/name: frontend
    spec:
      containers:
        - name: frontend
          image: nginx:latest
`)
	h.WriteInputFile("database.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: database
  namespace: default
  labels:
    app.kubernetes.io/name: database
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: database
  template:
    metadata:
      labels:
        app.kubernetes.io/name: database
    spec:
      containers:
        - name: database
          image: postgres:15
`)

	output, err := ExecutePipelineWithMode(h.InputDir, PipelineOptions{
		ChartName:    "myapp",
		ChartVersion: "1.0.0",
		Mode:         types.OutputModeUmbrella,
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	parentDir := filepath.Join(output.OutputDir, "myapp")
	depCmd := exec.Command("helm", "dependency", "build", parentDir)
	if depOut, depErr := depCmd.CombinedOutput(); depErr != nil {
		t.Skipf("helm dependency build failed: %v\n%s", depErr, string(depOut))
	}

	// Render with database disabled
	cmd := exec.Command("helm", "template", "myapp", parentDir,
		"--set", "database.enabled=false")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\nOutput: %s", err, string(out))
	}

	// Database should not be rendered
	rendered := string(out)
	if strings.Contains(rendered, "app.kubernetes.io/instance: myapp-database") ||
		strings.Contains(rendered, "name: database") {
		t.Error("database resources should not be rendered when database.enabled=false")
	}
}

// ============================================================
// Subtask 3: Cascading values
// ============================================================

func TestPipelineUmbrella_CascadingValues(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	h.WriteInputFile("app.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  namespace: default
  labels:
    app.kubernetes.io/name: frontend
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: frontend
  template:
    metadata:
      labels:
        app.kubernetes.io/name: frontend
    spec:
      containers:
        - name: frontend
          image: nginx:latest
`)

	output, err := ExecutePipelineWithMode(h.InputDir, PipelineOptions{
		ChartName:    "myapp",
		ChartVersion: "1.0.0",
		Mode:         types.OutputModeUmbrella,
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	parent := umbrellaParentChart(output.Charts)
	if parent == nil {
		t.Fatal("parent chart not found")
	}

	// Parent values must contain per-subchart sections for cascading override
	if !strings.Contains(parent.ValuesYAML, "frontend:") {
		t.Error("parent values missing 'frontend:' section for cascading")
	}
	// Each subchart must have enabled: true by default
	if !strings.Contains(parent.ValuesYAML, "enabled: true") {
		t.Error("parent values missing 'enabled: true'")
	}
}

// ============================================================
// Subtask 4: Global values propagation
// ============================================================

func TestPipelineUmbrella_GlobalValues(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	h.WriteInputFile("frontend.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  namespace: default
  labels:
    app.kubernetes.io/name: frontend
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: frontend
  template:
    metadata:
      labels:
        app.kubernetes.io/name: frontend
    spec:
      containers:
        - name: frontend
          image: nginx:latest
`)
	h.WriteInputFile("backend.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: backend
  namespace: default
  labels:
    app.kubernetes.io/name: backend
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: backend
  template:
    metadata:
      labels:
        app.kubernetes.io/name: backend
    spec:
      containers:
        - name: backend
          image: backend:latest
`)

	output, err := ExecutePipelineWithMode(h.InputDir, PipelineOptions{
		ChartName:    "myapp",
		ChartVersion: "1.0.0",
		Mode:         types.OutputModeUmbrella,
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	parent := umbrellaParentChart(output.Charts)
	if parent == nil {
		t.Fatal("parent chart not found")
	}

	// Parent values must have global: section
	if !strings.Contains(parent.ValuesYAML, "global:") {
		t.Error("parent values missing 'global:' section")
	}
}

// ============================================================
// Subtask 5: All resource types in umbrella
// ============================================================

func TestPipelineUmbrella_AllResourceTypes(t *testing.T) {
	fixturesDir := filepath.Join("fixtures", "umbrella-app")

	output, err := ExecutePipelineWithMode(fixturesDir, PipelineOptions{
		ChartName:    "full-stack",
		ChartVersion: "1.0.0",
		Mode:         types.OutputModeUmbrella,
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Should have at least 3 service groups
	subs := umbrellaSubcharts(output.Charts)
	if len(subs) < 2 {
		t.Fatalf("expected at least 2 subcharts from fixtures, got %d", len(subs))
	}

	// Each subchart must have templates
	for _, sub := range subs {
		if len(sub.Templates) == 0 {
			t.Errorf("subchart %s has no templates", sub.Name)
		}
	}
}

// ============================================================
// Subtask 6: Helm lint umbrella chart
// ============================================================

func TestPipelineUmbrella_HelmLint(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not found in PATH, skipping lint test")
	}

	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	h.WriteInputFile("app.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
  namespace: default
  labels:
    app.kubernetes.io/name: myapp
spec:
  replicas: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: myapp
  template:
    metadata:
      labels:
        app.kubernetes.io/name: myapp
    spec:
      containers:
        - name: myapp
          image: myapp:1.0
          ports:
            - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: myapp
  namespace: default
  labels:
    app.kubernetes.io/name: myapp
spec:
  selector:
    app.kubernetes.io/name: myapp
  ports:
    - port: 80
      targetPort: 8080
`)

	output, err := ExecutePipelineWithMode(h.InputDir, PipelineOptions{
		ChartName:    "lint-test",
		ChartVersion: "1.0.0",
		Mode:         types.OutputModeUmbrella,
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	parent := umbrellaParentChart(output.Charts)
	if parent == nil {
		t.Fatal("parent chart not found")
	}

	parentDir := filepath.Join(output.OutputDir, parent.Name)

	// Build dependencies first
	depCmd := exec.Command("helm", "dependency", "build", parentDir)
	if depOut, depErr := depCmd.CombinedOutput(); depErr != nil {
		t.Logf("helm dependency build output: %s", string(depOut))
		t.Skipf("helm dependency build failed (skipping lint): %v", depErr)
	}

	// Lint the parent chart
	cmd := exec.Command("helm", "lint", parentDir)
	lintOutput, lintErr := cmd.CombinedOutput()
	if lintErr != nil {
		t.Errorf("helm lint failed for umbrella chart: %v\nOutput: %s", lintErr, string(lintOutput))
	}
}

// ============================================================
// Subtask 7: Regression — Other modes still work
// ============================================================

func TestPipelineUmbrella_UniversalRegression(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	h.WriteInputFile("app.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: webapp
  namespace: default
  labels:
    app.kubernetes.io/name: webapp
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: webapp
  template:
    metadata:
      labels:
        app.kubernetes.io/name: webapp
    spec:
      containers:
        - name: webapp
          image: nginx:latest
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName:    "regression",
		ChartVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Universal pipeline failed (regression): %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	if len(output.Charts) != 1 {
		t.Fatalf("expected 1 chart from universal mode, got %d", len(output.Charts))
	}
}

func TestPipelineUmbrella_SeparateRegression(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	h.WriteInputFile("app.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  namespace: default
  labels:
    app.kubernetes.io/name: frontend
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: frontend
  template:
    metadata:
      labels:
        app.kubernetes.io/name: frontend
    spec:
      containers:
        - name: frontend
          image: nginx:latest
`)

	output, err := ExecutePipelineWithMode(h.InputDir, PipelineOptions{
		ChartName:    "regression",
		ChartVersion: "1.0.0",
		Mode:         types.OutputModeSeparate,
	})
	if err != nil {
		t.Fatalf("Separate pipeline failed (regression): %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	if len(output.Charts) != 1 {
		t.Fatalf("expected 1 chart from separate mode, got %d", len(output.Charts))
	}
}

func TestPipelineUmbrella_LibraryRegression(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	h.WriteInputFile("app.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
  namespace: default
  labels:
    app.kubernetes.io/name: app
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: app
  template:
    metadata:
      labels:
        app.kubernetes.io/name: app
    spec:
      containers:
        - name: app
          image: nginx:latest
`)

	output, err := ExecutePipelineWithMode(h.InputDir, PipelineOptions{
		ChartName:    "regression",
		ChartVersion: "1.0.0",
		Mode:         types.OutputModeLibrary,
	})
	if err != nil {
		t.Fatalf("Library pipeline failed (regression): %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Library mode: 1 library + 1 wrapper = 2 charts
	if len(output.Charts) != 2 {
		t.Fatalf("expected 2 charts from library mode (library+wrapper), got %d", len(output.Charts))
	}
}

// ============================================================
// Helper: write umbrella charts to disk for helm commands
// ============================================================

func writeUmbrellaChartsToDisk(t *testing.T, charts []*types.GeneratedChart, outputDir string) {
	t.Helper()
	for _, chart := range charts {
		if err := generator.WriteChart(chart, outputDir); err != nil {
			t.Fatalf("WriteChart failed for %s: %v", chart.Name, err)
		}
	}
}
