package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Subtask 1: Simple 2-service separation
// ============================================================

func TestPipelineSeparate_TwoServices(t *testing.T) {
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
    - name: http
      port: 80
      targetPort: 80
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
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: frontend
                port:
                  number: 80
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
  type: ClusterIP
  selector:
    app.kubernetes.io/name: backend
  ports:
    - name: http
      port: 8080
      targetPort: 8080
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
  LOG_LEVEL: info
`)

	output, err := ExecutePipelineWithMode(h.InputDir, PipelineOptions{
		ChartName:    "myapp",
		ChartVersion: "1.0.0",
		Mode:         types.OutputModeSeparate,
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// Expected: 2 charts
	if len(output.Charts) != 2 {
		t.Fatalf("expected 2 charts, got %d", len(output.Charts))
	}

	// Each chart should have Chart.yaml, values.yaml, templates/
	for _, chart := range output.Charts {
		chartDir := filepath.Join(output.OutputDir, chart.Name)
		ValidateChartStructure(t, chartDir)
	}
}

// ============================================================
// Subtask 2: 3-tier app with dependencies
// ============================================================

func TestPipelineSeparate_ThreeTierApp(t *testing.T) {
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
  selector:
    app.kubernetes.io/name: frontend
  ports:
    - port: 80
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
`)

	h.WriteInputFile("database.yaml", `apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: database
  namespace: default
  labels:
    app.kubernetes.io/name: database
spec:
  replicas: 1
  serviceName: database-headless
  selector:
    matchLabels:
      app.kubernetes.io/name: database
  template:
    metadata:
      labels:
        app.kubernetes.io/name: database
    spec:
      containers:
        - name: postgres
          image: postgres:15
          ports:
            - containerPort: 5432
---
apiVersion: v1
kind: Service
metadata:
  name: database
  namespace: default
  labels:
    app.kubernetes.io/name: database
spec:
  selector:
    app.kubernetes.io/name: database
  ports:
    - port: 5432
`)

	output, err := ExecutePipelineWithMode(h.InputDir, PipelineOptions{
		ChartName:    "three-tier",
		ChartVersion: "1.0.0",
		Mode:         types.OutputModeSeparate,
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	if len(output.Charts) != 3 {
		t.Fatalf("expected 3 charts for 3-tier app, got %d", len(output.Charts))
	}

	// Verify each chart directory is valid
	for _, chart := range output.Charts {
		chartDir := filepath.Join(output.OutputDir, chart.Name)
		ValidateChartStructure(t, chartDir)
	}
}

// ============================================================
// Subtask 5: Single-service input (degenerate case)
// ============================================================

func TestPipelineSeparate_SingleService(t *testing.T) {
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
  replicas: 1
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
          image: myapp:latest
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
`)

	output, err := ExecutePipelineWithMode(h.InputDir, PipelineOptions{
		ChartName:    "single",
		ChartVersion: "1.0.0",
		Mode:         types.OutputModeSeparate,
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	if len(output.Charts) != 1 {
		t.Fatalf("expected 1 chart for single service, got %d", len(output.Charts))
	}

	chartDir := filepath.Join(output.OutputDir, output.Charts[0].Name)
	ValidateChartStructure(t, chartDir)

	// Values should not have services section
	if strings.Contains(output.Charts[0].ValuesYAML, "services:") {
		t.Error("single-service separate chart should not have 'services:' section")
	}
}

// ============================================================
// Subtask 7: Helm lint all generated charts
// ============================================================

func TestPipelineSeparate_HelmLint(t *testing.T) {
	// Skip if helm not available
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not found in PATH, skipping lint test")
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
          image: nginx:1.25
---
apiVersion: v1
kind: Service
metadata:
  name: frontend
  namespace: default
  labels:
    app.kubernetes.io/name: frontend
spec:
  selector:
    app.kubernetes.io/name: frontend
  ports:
    - port: 80
`)

	output, err := ExecutePipelineWithMode(h.InputDir, PipelineOptions{
		ChartName:    "lint-test",
		ChartVersion: "1.0.0",
		Mode:         types.OutputModeSeparate,
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	for _, chart := range output.Charts {
		chartDir := filepath.Join(output.OutputDir, chart.Name)
		cmd := exec.Command("helm", "lint", chartDir)
		lintOutput, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("helm lint failed for chart %s: %v\nOutput: %s", chart.Name, err, string(lintOutput))
		}
	}
}

// ============================================================
// Subtask 8: Regression — Universal mode still works
// ============================================================

func TestPipelineSeparate_UniversalRegression(t *testing.T) {
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
  replicas: 2
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
          image: nginx:1.25
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

	// Run through universal mode (original pipeline)
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

	chart := output.Charts[0]
	if chart.ChartYAML == "" {
		t.Error("Chart.yaml is empty in universal mode")
	}
	if chart.ValuesYAML == "" {
		t.Error("values.yaml is empty in universal mode")
	}
	if len(chart.Templates) == 0 {
		t.Error("no templates in universal mode")
	}
}
