package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================
// Task 3.2: Helm Lint Validation
// Validates that all generated charts pass `helm lint`.
// ============================================================

// TestLint_SimpleChart validates that a simple chart (Deployment + Service)
// passes helm lint with no errors.
func TestLint_SimpleChart(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: webapp
  labels:
    app: webapp
spec:
  replicas: 2
  selector:
    matchLabels:
      app: webapp
  template:
    metadata:
      labels:
        app: webapp
    spec:
      containers:
        - name: webapp
          image: webapp:1.0
          ports:
            - containerPort: 8080
`)

	h.WriteInput("service.yaml", `
apiVersion: v1
kind: Service
metadata:
  name: webapp
  labels:
    app: webapp
spec:
  type: ClusterIP
  selector:
    app: webapp
  ports:
    - port: 80
      targetPort: 8080
`)

	h.GenerateChart("simple-lint-chart")
	h.AssertLintSuccess()

	// Also test strict mode
	result := h.Lint(WithLintStrict())
	if !result.Success() {
		t.Errorf("Strict lint failed: %s\n%s", result.Stderr, result.Stdout)
	}
}

// TestLint_FullStackChart validates that a full-stack chart (frontend + backend)
// passes helm lint with no errors.
func TestLint_FullStackChart(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("frontend.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  labels:
    app: frontend
spec:
  replicas: 2
  selector:
    matchLabels:
      app: frontend
  template:
    metadata:
      labels:
        app: frontend
    spec:
      containers:
        - name: nginx
          image: nginx:1.25
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: frontend
  labels:
    app: frontend
spec:
  type: ClusterIP
  selector:
    app: frontend
  ports:
    - port: 80
      targetPort: 80
`)

	h.WriteInput("backend.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: backend
  labels:
    app: backend
spec:
  replicas: 3
  selector:
    matchLabels:
      app: backend
  template:
    metadata:
      labels:
        app: backend
    spec:
      containers:
        - name: api
          image: api:2.0
          ports:
            - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: backend
  labels:
    app: backend
spec:
  type: ClusterIP
  selector:
    app: backend
  ports:
    - port: 8080
      targetPort: 8080
`)

	h.GenerateChart("fullstack-lint-chart")
	h.AssertLintSuccess()
}

// TestLint_DeckhouseChart validates that a chart with Deckhouse CRDs
// passes helm lint with no errors.
func TestLint_DeckhouseChart(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("module-config.yaml", `
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: monitoring
  labels:
    app: monitoring
spec:
  enabled: true
  version: 1
  settings:
    grafana:
      enabled: true
    prometheus:
      retentionDays: 30
`)

	h.GenerateChart("deckhouse-lint-chart")
	h.AssertLintSuccess()
}

// TestLint_WithValuesOverrides validates that charts accept custom values
// without lint errors.
func TestLint_WithValuesOverrides(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: configapp
  labels:
    app: configapp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: configapp
  template:
    metadata:
      labels:
        app: configapp
    spec:
      containers:
        - name: configapp
          image: configapp:1.0
          ports:
            - containerPort: 8080
`)

	h.WriteInput("service.yaml", `
apiVersion: v1
kind: Service
metadata:
  name: configapp
  labels:
    app: configapp
spec:
  type: ClusterIP
  selector:
    app: configapp
  ports:
    - port: 80
      targetPort: 8080
`)

	h.GenerateChart("values-lint-chart")

	// Lint with custom values that override replicas
	valuesFile := h.WriteCustomValues(`
services:
  configapp:
    enabled: true
    deployment:
      replicas: 5
`)

	result := h.Lint(WithLintValues(valuesFile))
	if !result.Success() {
		t.Fatalf("Lint with custom values failed: %s\n%s", result.Stderr, result.Stdout)
	}

	// Also lint with empty values override (should still pass)
	emptyValues := h.WriteCustomValues(`{}`)
	result = h.Lint(WithLintValues(emptyValues))
	if !result.Success() {
		t.Fatalf("Lint with empty values failed: %s\n%s", result.Stderr, result.Stdout)
	}
}

// TestLint_ErrorDetection validates that helm lint correctly detects errors
// in broken charts.
func TestLint_ErrorDetection(t *testing.T) {
	h := NewE2ETestHarness(t)

	// First generate a valid chart
	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: broken
  labels:
    app: broken
spec:
  replicas: 1
  selector:
    matchLabels:
      app: broken
  template:
    metadata:
      labels:
        app: broken
    spec:
      containers:
        - name: broken
          image: broken:1.0
`)

	h.GenerateChart("broken-lint-chart")

	t.Run("InvalidChartYAML", func(t *testing.T) {
		// Corrupt the Chart.yaml to make lint fail
		chartYAML := filepath.Join(h.ChartDir, "Chart.yaml")
		if err := os.WriteFile(chartYAML, []byte("invalid: yaml: [broken"), 0644); err != nil {
			t.Fatalf("Failed to corrupt Chart.yaml: %v", err)
		}

		result := h.Lint()
		if result.Success() {
			t.Error("Expected lint to fail on invalid Chart.yaml")
		}
		if !strings.Contains(result.Stderr+result.Stdout, "Error") &&
			!strings.Contains(result.Stderr+result.Stdout, "error") {
			t.Errorf("Expected error in lint output, got: %s %s", result.Stdout, result.Stderr)
		}
	})

	t.Run("BrokenTemplate", func(t *testing.T) {
		// Regenerate a valid chart first
		h2 := NewE2ETestHarness(t)
		h2.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: broken2
  labels:
    app: broken2
spec:
  replicas: 1
  selector:
    matchLabels:
      app: broken2
  template:
    metadata:
      labels:
        app: broken2
    spec:
      containers:
        - name: broken2
          image: broken2:1.0
`)
		h2.GenerateChart("broken-template-chart")

		// Inject broken template syntax
		templatesDir := filepath.Join(h2.ChartDir, "templates")
		brokenTpl := filepath.Join(templatesDir, "broken.yaml")
		if err := os.WriteFile(brokenTpl, []byte("{{ .Values.nonexistent | invalid_func }}"), 0644); err != nil {
			t.Fatalf("Failed to write broken template: %v", err)
		}

		result := h2.Lint()
		if result.Success() {
			t.Error("Expected lint to fail on broken template")
		}
	})
}
