package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// findChartByTypeLibrary returns the library chart from output.
func findChartByTypeLibrary(charts []*types.GeneratedChart) *types.GeneratedChart {
	for _, c := range charts {
		if strings.Contains(c.ChartYAML, "type: library") {
			return c
		}
	}
	return nil
}

// wrapperCharts returns all non-library charts.
func wrapperCharts(charts []*types.GeneratedChart) []*types.GeneratedChart {
	var wrappers []*types.GeneratedChart
	for _, c := range charts {
		if !strings.Contains(c.ChartYAML, "type: library") {
			wrappers = append(wrappers, c)
		}
	}
	return wrappers
}

// ============================================================
// Subtask 1: Library + 2 wrappers
// ============================================================

func TestPipelineLibrary_TwoServices(t *testing.T) {
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
  replicas: 2
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
          image: nginx:1.25
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: frontend
  namespace: default
  labels:
    app.kubernetes.io/name: frontend
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/name: frontend
  ports:
    - port: 80
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: frontend
  namespace: default
  labels:
    app.kubernetes.io/name: frontend
spec:
  rules:
    - host: frontend.example.com
`)

	h.WriteInputFile("backend.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: backend
  namespace: default
  labels:
    app.kubernetes.io/name: backend
spec:
  replicas: 3
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
          image: myapp/backend:2.0
          ports:
            - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: backend
  namespace: default
  labels:
    app.kubernetes.io/name: backend
spec:
  selector:
    app.kubernetes.io/name: backend
  ports:
    - port: 8080
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: backend-config
  namespace: default
  labels:
    app.kubernetes.io/name: backend
data:
  APP_ENV: production
`)

	output, err := ExecutePipelineWithMode(h.InputDir, PipelineOptions{
		ChartName:    "myapp",
		ChartVersion: "1.0.0",
		Mode:         types.OutputModeLibrary,
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Expected: 1 library + 2 wrapper charts = 3 total
	if len(output.Charts) != 3 {
		t.Fatalf("expected 3 charts (1 library + 2 wrappers), got %d: %v",
			len(output.Charts), chartNames(output.Charts))
	}

	libChart := findChartByTypeLibrary(output.Charts)
	if libChart == nil {
		t.Fatal("library chart not found in output")
	}

	// Library Chart.yaml must have type: library
	if !strings.Contains(libChart.ChartYAML, "type: library") {
		t.Error("library chart Chart.yaml does not have 'type: library'")
	}

	// Both wrapper charts must exist on disk
	wrappers := wrapperCharts(output.Charts)
	if len(wrappers) != 2 {
		t.Fatalf("expected 2 wrapper charts, got %d", len(wrappers))
	}
	for _, w := range wrappers {
		chartDir := filepath.Join(output.OutputDir, w.Name)
		ValidateChartStructure(t, chartDir)
	}
}

// ============================================================
// Subtask 2: Wrapper template calls library include
// ============================================================

func TestPipelineLibrary_WrapperCallsLibrary(t *testing.T) {
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
          image: myapp:latest
---
apiVersion: v1
kind: Service
metadata:
  name: app
  namespace: default
  labels:
    app.kubernetes.io/name: app
spec:
  selector:
    app.kubernetes.io/name: app
  ports:
    - port: 80
`)

	output, err := ExecutePipelineWithMode(h.InputDir, PipelineOptions{
		ChartName:    "app",
		ChartVersion: "1.0.0",
		Mode:         types.OutputModeLibrary,
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	libChart := findChartByTypeLibrary(output.Charts)
	if libChart == nil {
		t.Fatal("library chart not found")
	}

	wrappers := wrapperCharts(output.Charts)
	if len(wrappers) == 0 {
		t.Fatal("no wrapper charts generated")
	}

	for _, wrapper := range wrappers {
		if len(wrapper.Templates) == 0 {
			t.Errorf("wrapper chart %s has no templates", wrapper.Name)
		}
		// No own _helpers.tpl: the wrapper relies on the library helpers.
		if wrapper.Helpers != "" {
			t.Errorf("wrapper chart %s should not have its own _helpers.tpl", wrapper.Name)
		}
		for path, content := range wrapper.Templates {
			refs := helperRefs(content)
			if len(refs) == 0 {
				t.Errorf("wrapper template %s in chart %s uses no library helper\ncontent: %s",
					path, wrapper.Name, content)
			}
			for _, ref := range refs {
				// Neither the source chart's ("app.") nor the group's own helpers.
				if !strings.HasPrefix(ref, "library.") {
					t.Errorf("wrapper template %s in chart %s references non-library helper %q",
						path, wrapper.Name, ref)
				}
				if !strings.Contains(libChart.Helpers, `define "`+ref+`"`) {
					t.Errorf("wrapper template %s in chart %s references %q, not defined in the library",
						path, wrapper.Name, ref)
				}
			}
			// Templates carry the real manifests with flat values.
			if !strings.Contains(content, "apiVersion:") || !strings.Contains(content, "kind:") {
				t.Errorf("wrapper template %s in chart %s should render a Kubernetes resource",
					path, wrapper.Name)
			}
			if strings.Contains(content, ".Values.services.") {
				t.Errorf("wrapper template %s in chart %s still reads nested .Values.services values",
					path, wrapper.Name)
			}
		}
	}
}

// ============================================================
// Subtask 3: DRY verification — no duplication
// ============================================================

func TestPipelineLibrary_DRYVerification(t *testing.T) {
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
		ChartName:    "dry-test",
		ChartVersion: "1.0.0",
		Mode:         types.OutputModeLibrary,
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	libChart := findChartByTypeLibrary(output.Charts)
	if libChart == nil {
		t.Fatal("library chart not found")
	}

	// Shared helpers must be defined EXACTLY ONCE across all generated charts,
	// and only in the library.
	var allContent strings.Builder
	for _, c := range output.Charts {
		allContent.WriteString(c.Helpers)
		for _, tmpl := range c.Templates {
			allContent.WriteString(tmpl)
		}
	}
	all := allContent.String()
	for _, helper := range []string{"library.name", "library.fullname", "library.labels", "library.selectorLabels", "library.chart"} {
		define := `define "` + helper + `"`
		if n := strings.Count(libChart.Helpers, define); n != 1 {
			t.Errorf("define %q found %d times in library helpers (expected exactly 1)", helper, n)
		}
		if n := strings.Count(all, define); n != 1 {
			t.Errorf("define %q found %d times across all charts (expected exactly 1)", helper, n)
		}
	}

	wrappers := wrapperCharts(output.Charts)
	if len(wrappers) != 2 {
		t.Fatalf("expected 2 wrapper charts, got %v", chartNames(output.Charts))
	}

	// Wrappers define nothing themselves: no helpers, no named templates.
	for _, wrapper := range wrappers {
		if wrapper.Helpers != "" {
			t.Errorf("wrapper chart %s duplicates helpers in its own _helpers.tpl", wrapper.Name)
		}
		for path, content := range wrapper.Templates {
			if strings.Contains(content, `define "`) {
				t.Errorf("wrapper template %s in chart %s contains a define (should be include-only)",
					path, wrapper.Name)
			}
			for _, w := range wrappers {
				if strings.Contains(content, `include "`+w.Name+`.`) {
					t.Errorf("wrapper template %s in chart %s references %s.* helpers instead of the library",
						path, wrapper.Name, w.Name)
				}
			}
		}
	}

	// Nothing in the generated output should require a per-chart _helpers.tpl.
	for _, wrapper := range wrappers {
		if _, err := os.Stat(filepath.Join(output.OutputDir, wrapper.Name, "templates", "_helpers.tpl")); err == nil {
			t.Errorf("wrapper chart %s has a _helpers.tpl on disk", wrapper.Name)
		}
	}
	if _, err := os.Stat(filepath.Join(output.OutputDir, libChart.Name, "templates", "_helpers.tpl")); err != nil {
		t.Errorf("library chart _helpers.tpl not written to disk: %v", err)
	}
}

// ============================================================
// Subtask 4: Full-stack with all resource types
// ============================================================

func TestPipelineLibrary_AllResourceTypes(t *testing.T) {
	fixturesDir := filepath.Join("fixtures", "library-app")

	output, err := ExecutePipelineWithMode(fixturesDir, PipelineOptions{
		ChartName:    "library-app",
		ChartVersion: "1.0.0",
		Mode:         types.OutputModeLibrary,
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	libChart := findChartByTypeLibrary(output.Charts)
	if libChart == nil {
		t.Fatal("library chart not found")
	}

	// The library is helpers-only: no per-kind templates.
	if len(libChart.Templates) != 0 {
		t.Errorf("library chart should have no templates, got %d", len(libChart.Templates))
	}
	if !strings.Contains(libChart.Helpers, `define "library.fullname"`) {
		t.Error("library chart helpers missing library.fullname")
	}

	wrappers := wrapperCharts(output.Charts)
	for _, name := range []string{"frontend", "backend", "database"} {
		found := false
		for _, w := range wrappers {
			if w.Name == name {
				found = true
			}
		}
		if !found {
			t.Errorf("wrapper chart %s not generated, got %v", name, chartNames(output.Charts))
		}
	}

	// Every resource kind of the fixture is rendered by some wrapper template.
	kindsRendered := map[string]bool{}
	for _, wrapper := range wrappers {
		if len(wrapper.Templates) == 0 {
			t.Errorf("wrapper chart %s has no templates", wrapper.Name)
		}
		if !strings.Contains(wrapper.ChartYAML, "repository: file://../library") {
			t.Errorf("wrapper chart %s does not depend on file://../library", wrapper.Name)
		}
		if !strings.Contains(wrapper.ValuesYAML, "\nenabled: true\n") {
			t.Errorf("wrapper chart %s values.yaml should enable the chart with a top-level enabled: true", wrapper.Name)
		}
		for path, content := range wrapper.Templates {
			if !strings.HasPrefix(path, "templates/") {
				t.Errorf("wrapper chart %s template %s is outside templates/", wrapper.Name, path)
			}
			for _, kind := range []string{"Deployment", "StatefulSet", "Service", "Ingress", "ConfigMap"} {
				if strings.Contains(content, "\nkind: "+kind+"\n") {
					kindsRendered[kind] = true
				}
			}
			for _, ref := range helperRefs(content) {
				if !strings.HasPrefix(ref, "library.") {
					t.Errorf("wrapper chart %s template %s references non-library helper %q",
						wrapper.Name, path, ref)
				}
			}
		}
	}
	for _, kind := range []string{"Deployment", "StatefulSet", "Service", "Ingress", "ConfigMap"} {
		if !kindsRendered[kind] {
			t.Errorf("no wrapper template renders a %s", kind)
		}
	}
}

// ============================================================
// Subtask 5: Helm lint all charts
// ============================================================

func TestPipelineLibrary_HelmLint(t *testing.T) {
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
		Mode:         types.OutputModeLibrary,
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	for _, chart := range output.Charts {
		chartDir := filepath.Join(output.OutputDir, chart.Name)

		if strings.Contains(chart.ChartYAML, "type: library") {
			// Lint library chart directly
			cmd := exec.Command("helm", "lint", chartDir)
			lintOutput, lintErr := cmd.CombinedOutput()
			if lintErr != nil {
				t.Errorf("helm lint failed for library chart %s: %v\nOutput: %s",
					chart.Name, lintErr, string(lintOutput))
			}
		} else {
			// For wrapper charts: dependency build first, then lint
			depCmd := exec.Command("helm", "dependency", "build", chartDir)
			depOut, depErr := depCmd.CombinedOutput()
			if depErr != nil {
				t.Logf("helm dependency build failed for wrapper %s (skipping lint): %v\nOutput: %s",
					chart.Name, depErr, string(depOut))
				continue
			}
			cmd := exec.Command("helm", "lint", chartDir)
			lintOutput, lintErr := cmd.CombinedOutput()
			if lintErr != nil {
				t.Errorf("helm lint failed for wrapper chart %s: %v\nOutput: %s",
					chart.Name, lintErr, string(lintOutput))
			}
		}
	}
}

// ============================================================
// Subtask 6: Regression — Universal and Separate modes
// ============================================================

func TestPipelineLibrary_UniversalRegression(t *testing.T) {
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
---
apiVersion: v1
kind: Service
metadata:
  name: webapp
  namespace: default
  labels:
    app.kubernetes.io/name: webapp
spec:
  selector:
    app.kubernetes.io/name: webapp
  ports:
    - port: 80
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
	if output.Charts[0].ChartYAML == "" || len(output.Charts[0].Templates) == 0 {
		t.Error("universal mode chart is incomplete")
	}
}

func TestPipelineLibrary_SeparateRegression(t *testing.T) {
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
		ChartName:    "regression-separate",
		ChartVersion: "1.0.0",
		Mode:         types.OutputModeSeparate,
	})
	if err != nil {
		t.Fatalf("Separate pipeline failed (regression): %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	if len(output.Charts) != 2 {
		t.Fatalf("expected 2 charts from separate mode, got %d", len(output.Charts))
	}
}

// ============================================================
// Helpers
// ============================================================

// helperRefs returns the helper names referenced via include/template.
func helperRefs(content string) []string {
	var names []string
	for _, m := range helperRefPattern.FindAllStringSubmatch(content, -1) {
		names = append(names, m[1])
	}
	return names
}

var helperRefPattern = regexp.MustCompile(`(?:include|template) "([^"]+)"`)

func chartNames(charts []*types.GeneratedChart) []string {
	names := make([]string, len(charts))
	for i, c := range charts {
		names[i] = c.Name
	}
	return names
}
