package e2e

import (
	"os"
	"strings"
	"testing"
	"time"
)

// ============================================================
// Task 3.3: Helm Template Validation
// Validates that all generated charts render correctly with `helm template`.
// ============================================================

// TestTemplate_DefaultValues validates template rendering with default values.
func TestTemplate_DefaultValues(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
  labels:
    app: myapp
spec:
  replicas: 2
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
        - name: myapp
          image: myapp:1.0
          ports:
            - containerPort: 8080
`)

	h.WriteInput("service.yaml", `
apiVersion: v1
kind: Service
metadata:
  name: myapp
  labels:
    app: myapp
spec:
  type: ClusterIP
  selector:
    app: myapp
  ports:
    - port: 80
      targetPort: 8080
`)

	h.GenerateChart("template-default")
	rendered := h.AssertTemplateSuccess("test-release")

	// Verify all expected resources are present
	if !strings.Contains(rendered, "kind: Deployment") {
		t.Error("Expected Deployment in rendered output")
	}
	if !strings.Contains(rendered, "kind: Service") {
		t.Error("Expected Service in rendered output")
	}

	// Verify rendered output is valid YAML (no template errors)
	if strings.Contains(rendered, "<no value>") {
		t.Error("Found '<no value>' in rendered output — indicates missing template value")
	}
}

// TestTemplate_CustomValues validates template rendering with custom values.
func TestTemplate_CustomValues(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: customapp
  labels:
    app: customapp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: customapp
  template:
    metadata:
      labels:
        app: customapp
    spec:
      containers:
        - name: customapp
          image: customapp:1.0
          ports:
            - containerPort: 8080
`)

	h.GenerateChart("template-custom")

	valuesFile := h.WriteCustomValues(`
services:
  customapp:
    enabled: true
    deployment:
      replicas: 10
`)

	rendered := h.AssertTemplateSuccess("custom-release", WithTemplateValues(valuesFile))

	if rendered == "" {
		t.Fatal("Expected non-empty rendered output with custom values")
	}

	// Rendered output should contain the deployment
	if !strings.Contains(rendered, "kind: Deployment") {
		t.Error("Expected Deployment in rendered output with custom values")
	}
}

// TestTemplate_ConditionalBlocks validates that conditional template blocks
// (e.g., enabled/disabled services) render correctly.
func TestTemplate_ConditionalBlocks(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: conditional
  labels:
    app: conditional
spec:
  replicas: 1
  selector:
    matchLabels:
      app: conditional
  template:
    metadata:
      labels:
        app: conditional
    spec:
      containers:
        - name: conditional
          image: conditional:1.0
`)

	h.WriteInput("service.yaml", `
apiVersion: v1
kind: Service
metadata:
  name: conditional
  labels:
    app: conditional
spec:
  type: ClusterIP
  selector:
    app: conditional
  ports:
    - port: 80
      targetPort: 8080
`)

	h.GenerateChart("template-conditional")

	t.Run("ServiceEnabled", func(t *testing.T) {
		valuesFile := h.WriteCustomValues(`
services:
  conditional:
    enabled: true
`)
		rendered := h.AssertTemplateSuccess("enabled-release", WithTemplateValues(valuesFile))
		if !strings.Contains(rendered, "kind: Deployment") {
			t.Error("Expected Deployment when service is enabled")
		}
	})

	t.Run("ServiceDisabled", func(t *testing.T) {
		valuesFile := h.WriteCustomValues(`
services:
  conditional:
    enabled: false
`)
		// Template should still render (it's a valid template operation)
		result := h.Template("disabled-release", WithTemplateValues(valuesFile))
		if !result.Success() {
			t.Fatalf("Template rendering failed: %s", result.Stderr)
		}
	})
}

// TestTemplate_HelperUsage validates that _helpers.tpl templates are used
// and rendered correctly in main templates.
func TestTemplate_HelperUsage(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: helperapp
  labels:
    app: helperapp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: helperapp
  template:
    metadata:
      labels:
        app: helperapp
    spec:
      containers:
        - name: helperapp
          image: helperapp:1.0
`)

	h.GenerateChart("helper-chart")
	rendered := h.AssertTemplateSuccess("helper-release")

	// Verify that helper templates are resolved (no template definition syntax left)
	if strings.Contains(rendered, "{{- define ") {
		t.Error("Found unresolved template definitions in rendered output")
	}

	// Verify fullname helper is applied — release name should appear in resource names
	if !strings.Contains(rendered, "helper-release") {
		t.Error("Expected release name 'helper-release' in rendered output (from fullname helper)")
	}

	// Verify chart labels are present (from labels helper)
	if !strings.Contains(rendered, "helm.sh/chart:") {
		t.Error("Expected 'helm.sh/chart' label from labels helper")
	}
	if !strings.Contains(rendered, "app.kubernetes.io/managed-by: Helm") {
		t.Error("Expected 'app.kubernetes.io/managed-by: Helm' label")
	}
}

// TestTemplate_VariableSubstitution validates that all template variables
// are substituted and no raw {{ }} expressions remain in the output.
func TestTemplate_VariableSubstitution(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vartest
  labels:
    app: vartest
spec:
  replicas: 3
  selector:
    matchLabels:
      app: vartest
  template:
    metadata:
      labels:
        app: vartest
    spec:
      containers:
        - name: vartest
          image: vartest:2.0
          ports:
            - containerPort: 8080
`)

	h.GenerateChart("var-chart")
	rendered := h.AssertTemplateSuccess("var-release")

	// Check no unresolved template expressions remain
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		// Skip lines that are comments (# ...) as they may contain template docs
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(line, "{{") && strings.Contains(line, "}}") {
			t.Errorf("Line %d contains unresolved template expression: %s", i+1, line)
		}
	}

	// Verify actual values are present
	if !strings.Contains(rendered, "kind: Deployment") {
		t.Error("Expected resolved Deployment kind")
	}
}

// TestTemplate_MultiDocumentYAML validates that rendered output contains
// proper YAML document separators between resources.
func TestTemplate_MultiDocumentYAML(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: multi
  labels:
    app: multi
spec:
  replicas: 1
  selector:
    matchLabels:
      app: multi
  template:
    metadata:
      labels:
        app: multi
    spec:
      containers:
        - name: multi
          image: multi:1.0
          ports:
            - containerPort: 8080
`)

	h.WriteInput("service.yaml", `
apiVersion: v1
kind: Service
metadata:
  name: multi
  labels:
    app: multi
spec:
  type: ClusterIP
  selector:
    app: multi
  ports:
    - port: 80
      targetPort: 8080
`)

	h.GenerateChart("multi-chart")
	rendered := h.AssertTemplateSuccess("multi-release")

	// Count YAML document separators
	docs := strings.Split(rendered, "---")

	// Count documents that contain actual K8s resource definitions
	var nonEmpty int
	for _, doc := range docs {
		trimmed := strings.TrimSpace(doc)
		if trimmed != "" && strings.Contains(trimmed, "kind:") {
			nonEmpty++
		}
	}

	if nonEmpty < 2 {
		t.Errorf("Expected at least 2 YAML documents (Deployment + Service), got %d", nonEmpty)
	}

	// Verify document separator exists
	if !strings.Contains(rendered, "---") {
		t.Error("Expected YAML document separators (---) in multi-resource output")
	}
}

// TestTemplate_RenderingPerformance validates that template rendering
// completes within acceptable time limits.
func TestTemplate_RenderingPerformance(t *testing.T) {
	h := NewE2ETestHarness(t)

	// Generate a chart with multiple resources to test performance
	for i := 0; i < 10; i++ {
		h.WriteInput(
			strings.Replace("deploy-N.yaml", "N", strings.Repeat("a", i+1), 1),
			strings.Replace(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: perfN
  labels:
    app: perfN
spec:
  replicas: 1
  selector:
    matchLabels:
      app: perfN
  template:
    metadata:
      labels:
        app: perfN
    spec:
      containers:
        - name: perfN
          image: perfN:1.0
          ports:
            - containerPort: 8080
`, "perfN", "perf"+strings.Repeat("a", i+1), -1),
		)
	}

	h.GenerateChart("perf-chart")

	start := time.Now()
	rendered := h.AssertTemplateSuccess("perf-release")
	elapsed := time.Since(start)

	// Should complete in under 2 seconds
	if elapsed > 2*time.Second {
		t.Errorf("Template rendering took %v, expected < 2s", elapsed)
	}

	if rendered == "" {
		t.Error("Expected non-empty rendered output")
	}
}

// TestTemplate_ErrorHandling validates that helm template fails gracefully
// with clear error messages for invalid templates.
func TestTemplate_ErrorHandling(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: errtest
  labels:
    app: errtest
spec:
  replicas: 1
  selector:
    matchLabels:
      app: errtest
  template:
    metadata:
      labels:
        app: errtest
    spec:
      containers:
        - name: errtest
          image: errtest:1.0
`)

	h.GenerateChart("error-chart")

	t.Run("InvalidTemplateSyntax", func(t *testing.T) {
		// Inject a broken template
		brokenTplPath := h.ChartPath("templates", "broken.yaml")
		if err := writeFile(brokenTplPath, "{{ .Values.missing | undefined_function }}"); err != nil {
			t.Fatalf("Failed to write broken template: %v", err)
		}

		result := h.Template("error-release")
		if result.Success() {
			t.Error("Expected template rendering to fail with invalid syntax")
		}

		combined := result.Stdout + result.Stderr
		if !strings.Contains(strings.ToLower(combined), "error") {
			t.Errorf("Expected error message in output, got: %s", combined)
		}
	})

	t.Run("MissingRequiredValue", func(t *testing.T) {
		// Create a template that requires a value via 'required'
		h2 := NewE2ETestHarness(t)
		h2.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: reqtest
  labels:
    app: reqtest
spec:
  replicas: 1
  selector:
    matchLabels:
      app: reqtest
  template:
    metadata:
      labels:
        app: reqtest
    spec:
      containers:
        - name: reqtest
          image: reqtest:1.0
`)
		h2.GenerateChart("required-chart")

		// Inject template with required function
		requiredTpl := h2.ChartPath("templates", "required-test.yaml")
		if err := writeFile(requiredTpl, `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ required "myField is required" .Values.myField }}
data: {}
`); err != nil {
			t.Fatalf("Failed to write required template: %v", err)
		}

		result := h2.Template("required-release")
		if result.Success() {
			t.Error("Expected template to fail when required value is missing")
		}
	})
}

// writeFile is a helper that writes content to a file.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
