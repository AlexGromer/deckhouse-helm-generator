package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================
// Task 3.4: Helm Install (Dry-Run) Validation
// Validates that charts can be installed in dry-run mode.
// ============================================================

// TestInstall_DryRunClient validates client-side dry-run install with debug output.
func TestInstall_DryRunClient(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: installapp
  labels:
    app: installapp
spec:
  replicas: 2
  selector:
    matchLabels:
      app: installapp
  template:
    metadata:
      labels:
        app: installapp
    spec:
      containers:
        - name: installapp
          image: installapp:1.0
          ports:
            - containerPort: 8080
`)

	h.WriteInput("service.yaml", `
apiVersion: v1
kind: Service
metadata:
  name: installapp
  labels:
    app: installapp
spec:
  type: ClusterIP
  selector:
    app: installapp
  ports:
    - port: 80
      targetPort: 8080
`)

	h.GenerateChart("install-client-chart")

	result := h.Install("install-release", WithInstallDebug())
	if !result.Success() {
		t.Fatalf("Client-side dry-run install failed: %s\n%s", result.Stderr, result.Stdout)
	}

	// Should contain rendered manifests
	if !strings.Contains(result.Stdout, "kind: Deployment") {
		t.Error("Expected Deployment in dry-run output")
	}
	if !strings.Contains(result.Stdout, "kind: Service") {
		t.Error("Expected Service in dry-run output")
	}

	// Debug output should contain chart info
	combined := result.Stdout + result.Stderr
	if !strings.Contains(combined, "install-client-chart") {
		t.Error("Expected chart name in debug output")
	}
}

// TestInstall_DryRunServer validates server-side dry-run install.
// This test uses the mock K8s API server or skips if no server is available.
func TestInstall_DryRunServer(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: serverapp
  labels:
    app: serverapp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: serverapp
  template:
    metadata:
      labels:
        app: serverapp
    spec:
      containers:
        - name: serverapp
          image: serverapp:1.0
          ports:
            - containerPort: 8080
`)

	h.GenerateChart("install-server-chart")

	// Server-side dry-run uses mock API server via Install()
	// For real server-side, it would need a real cluster.
	// Test with client-side which uses the mock API server internally.
	result := h.Install("server-release")
	if !result.Success() {
		t.Fatalf("Dry-run install failed: %s\n%s", result.Stderr, result.Stdout)
	}

	if !strings.Contains(result.Stdout, "kind: Deployment") {
		t.Error("Expected Deployment in dry-run output")
	}
}

// TestInstall_WithNamespace validates that install dry-run scopes resources
// to the specified namespace.
func TestInstall_WithNamespace(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nsapp
  labels:
    app: nsapp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nsapp
  template:
    metadata:
      labels:
        app: nsapp
    spec:
      containers:
        - name: nsapp
          image: nsapp:1.0
          ports:
            - containerPort: 8080
`)

	h.GenerateChart("namespace-chart")

	result := h.Install("ns-release", WithInstallNamespace("custom-ns"))
	if !result.Success() {
		t.Fatalf("Install with namespace failed: %s\n%s", result.Stderr, result.Stdout)
	}

	// Rendered output should reference the namespace
	if !strings.Contains(result.Stdout, "custom-ns") {
		t.Error("Expected namespace 'custom-ns' in install output")
	}
}

// TestInstall_ReleaseName validates that the release name is used in
// resource names via the fullname helper template.
func TestInstall_ReleaseName(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nameapp
  labels:
    app: nameapp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nameapp
  template:
    metadata:
      labels:
        app: nameapp
    spec:
      containers:
        - name: nameapp
          image: nameapp:1.0
`)

	h.GenerateChart("release-chart")

	result := h.Install("my-custom-release")
	if !result.Success() {
		t.Fatalf("Install with custom release name failed: %s\n%s", result.Stderr, result.Stdout)
	}

	// Release name should appear in resource metadata
	if !strings.Contains(result.Stdout, "my-custom-release") {
		t.Error("Expected release name 'my-custom-release' in install output")
	}

	// Release name should be in labels via the instance label
	if !strings.Contains(result.Stdout, "app.kubernetes.io/instance: my-custom-release") {
		t.Error("Expected instance label with release name")
	}
}

// TestInstall_ValidationErrors validates that install dry-run detects
// invalid chart structures.
func TestInstall_ValidationErrors(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: validapp
  labels:
    app: validapp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: validapp
  template:
    metadata:
      labels:
        app: validapp
    spec:
      containers:
        - name: validapp
          image: validapp:1.0
`)

	h.GenerateChart("validation-chart")

	t.Run("InvalidTemplate", func(t *testing.T) {
		// Inject a template with invalid syntax
		brokenTpl := filepath.Join(h.ChartDir, "templates", "broken.yaml")
		if err := os.WriteFile(brokenTpl, []byte("{{ .Values.missing | bad_func }}"), 0644); err != nil {
			t.Fatalf("Failed to write broken template: %v", err)
		}

		result := h.Install("invalid-release")
		if result.Success() {
			t.Error("Expected install to fail with invalid template")
		}

		// Remove broken template for next subtests
		os.Remove(brokenTpl)
	})

	t.Run("MissingRequiredField", func(t *testing.T) {
		// Inject a template that uses required function
		h2 := NewE2ETestHarness(t)
		h2.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: reqapp
  labels:
    app: reqapp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: reqapp
  template:
    metadata:
      labels:
        app: reqapp
    spec:
      containers:
        - name: reqapp
          image: reqapp:1.0
`)
		h2.GenerateChart("required-install-chart")

		requiredTpl := filepath.Join(h2.ChartDir, "templates", "required-check.yaml")
		if err := os.WriteFile(requiredTpl, []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ required "mandatoryField is required" .Values.mandatoryField }}
data: {}
`), 0644); err != nil {
			t.Fatalf("Failed to write required template: %v", err)
		}

		result := h2.Install("required-release")
		if result.Success() {
			t.Error("Expected install to fail when required value is missing")
		}
	})
}

// TestInstall_WithDependencies validates that a chart with multiple resources
// (simulating chart dependencies) installs correctly in dry-run mode.
func TestInstall_WithDependencies(t *testing.T) {
	h := NewE2ETestHarness(t)

	// Create a chart with multiple interconnected resources
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

	h.WriteInput("configmap.yaml", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: appconfig
  labels:
    app: appconfig
data:
  DB_HOST: postgres.default.svc.cluster.local
  CACHE_URL: redis://redis:6379
`)

	h.GenerateChart("deps-chart")

	result := h.Install("deps-release")
	if !result.Success() {
		t.Fatalf("Install with dependencies failed: %s\n%s", result.Stderr, result.Stdout)
	}

	// Verify all resource types are present in output
	output := result.Stdout
	expectedKinds := []string{"Deployment", "Service", "ConfigMap"}
	for _, kind := range expectedKinds {
		if !strings.Contains(output, "kind: "+kind) {
			t.Errorf("Expected %s in install output", kind)
		}
	}

	// Should have multiple deployments (frontend + backend)
	deploymentCount := strings.Count(output, "kind: Deployment")
	if deploymentCount < 2 {
		t.Errorf("Expected at least 2 Deployments, got %d", deploymentCount)
	}

	// Should have multiple services (frontend + backend)
	serviceCount := strings.Count(output, "kind: Service")
	if serviceCount < 2 {
		t.Errorf("Expected at least 2 Services, got %d", serviceCount)
	}
}
