package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ============================================================
// Task 3.1: E2E Framework meta-tests
// Validates the E2E test framework itself works correctly.
// ============================================================

// TestE2E_HelmClientCreation verifies that the Helm client can be created.
func TestE2E_HelmClientCreation(t *testing.T) {
	client, err := NewHelmClient()
	if err != nil {
		t.Skipf("Helm not available: %v", err)
	}

	if client.BinaryPath == "" {
		t.Fatal("Expected non-empty binary path")
	}

	// Verify helm is executable
	result := client.Version()
	if !result.Success() {
		t.Fatalf("helm version failed: %v", result.Err)
	}
	if !strings.Contains(result.Stdout, "v3") {
		t.Errorf("Expected helm v3, got: %s", result.Stdout)
	}
}

// TestE2E_HarnessSetup verifies the test harness creates the right directory structure.
func TestE2E_HarnessSetup(t *testing.T) {
	h := NewE2ETestHarness(t)

	// Verify directories exist
	for _, dir := range []string{h.TempDir, h.InputDir, h.OutputDir} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("Directory %s does not exist: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", dir)
		}
	}

	// Verify helm client is available
	if h.Helm == nil {
		t.Fatal("Expected non-nil Helm client")
	}
}

// TestE2E_GenerateAndLint verifies the full generate → lint pipeline.
func TestE2E_GenerateAndLint(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: testapp
  labels:
    app: testapp
spec:
  replicas: 1
  selector:
    matchLabels:
      app: testapp
  template:
    metadata:
      labels:
        app: testapp
    spec:
      containers:
        - name: testapp
          image: testapp:1.0
          ports:
            - containerPort: 8080
`)

	h.GenerateChart("e2e-test-chart")

	// Verify chart was generated
	if h.Chart == nil {
		t.Fatal("Expected non-nil chart")
	}
	if h.ChartDir == "" {
		t.Fatal("Expected non-empty chart dir")
	}

	// Verify chart files exist on disk
	required := []string{"Chart.yaml", "values.yaml", "templates"}
	for _, f := range required {
		path := filepath.Join(h.ChartDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected %s to exist in chart dir", f)
		}
	}

	// Lint should pass
	h.AssertLintSuccess()
}

// TestE2E_GenerateAndTemplate verifies generate → template pipeline.
func TestE2E_GenerateAndTemplate(t *testing.T) {
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
          image: myapp:latest
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

	h.GenerateChart("e2e-app")

	// Template rendering should succeed
	rendered := h.AssertTemplateSuccess("test-release")

	if rendered == "" {
		t.Fatal("Expected non-empty rendered output")
	}

	// Should contain expected resources
	if !strings.Contains(rendered, "kind: Deployment") {
		t.Error("Expected Deployment in rendered output")
	}
	if !strings.Contains(rendered, "kind: Service") {
		t.Error("Expected Service in rendered output")
	}
}

// TestE2E_InstallDryRunClient verifies install --dry-run=client works.
func TestE2E_InstallDryRunClient(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dryrun
  labels:
    app: dryrun
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dryrun
  template:
    metadata:
      labels:
        app: dryrun
    spec:
      containers:
        - name: dryrun
          image: dryrun:1.0
          ports:
            - containerPort: 8080
`)

	h.GenerateChart("dryrun-chart")

	result := h.Install("dryrun-release")
	if !result.Success() {
		t.Fatalf("helm install --dry-run failed: %s\n%s", result.Stderr, result.Stdout)
	}

	// Output should contain the manifests
	if !strings.Contains(result.Stdout, "kind: Deployment") {
		t.Error("Expected Deployment in dry-run output")
	}
}

// TestE2E_CustomValues verifies that custom values are applied.
func TestE2E_CustomValues(t *testing.T) {
	h := NewE2ETestHarness(t)

	h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: configurable
  labels:
    app: configurable
spec:
  replicas: 1
  selector:
    matchLabels:
      app: configurable
  template:
    metadata:
      labels:
        app: configurable
    spec:
      containers:
        - name: app
          image: app:1.0
          ports:
            - containerPort: 8080
`)

	h.GenerateChart("custom-values-chart")

	// Verify lint with custom values
	valuesFile := h.WriteCustomValues(`
services:
  configurable:
    enabled: true
    deployment:
      replicas: 5
`)

	result := h.Lint(WithLintValues(valuesFile))
	if !result.Success() {
		t.Fatalf("helm lint with custom values failed: %s", result.Stderr)
	}

	// Verify template with custom values
	rendered := h.AssertTemplateSuccess("custom-release", WithTemplateValues(valuesFile))
	if rendered == "" {
		t.Fatal("Expected non-empty rendered output")
	}
}

// TestE2E_Isolation verifies that tests don't share state.
func TestE2E_Isolation(t *testing.T) {
	// Create two harnesses and verify they have different temp dirs
	h1 := NewE2ETestHarness(t)
	h2 := NewE2ETestHarness(t)

	if h1.TempDir == h2.TempDir {
		t.Error("Two harnesses should have different temp dirs")
	}
	if h1.InputDir == h2.InputDir {
		t.Error("Two harnesses should have different input dirs")
	}
	if h1.OutputDir == h2.OutputDir {
		t.Error("Two harnesses should have different output dirs")
	}

	// Write to one shouldn't affect the other
	h1.WriteInput("test.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n")

	path := filepath.Join(h2.InputDir, "test.yaml")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("File in h1 should not appear in h2")
	}
}

// TestE2E_FixturesExist verifies that fixture files are present.
func TestE2E_FixturesExist(t *testing.T) {
	fixtureBase := filepath.Join("fixtures", "charts")

	fixtures := []struct {
		name  string
		files []string
	}{
		{"simple-app", []string{"deployment.yaml", "service.yaml"}},
		{"fullstack", []string{"frontend.yaml", "backend.yaml"}},
		{"deckhouse", []string{"module-config.yaml"}},
	}

	for _, fix := range fixtures {
		fixDir := filepath.Join(fixtureBase, fix.name)
		if _, err := os.Stat(fixDir); os.IsNotExist(err) {
			t.Errorf("Fixture directory %s does not exist", fixDir)
			continue
		}

		for _, file := range fix.files {
			path := filepath.Join(fixDir, file)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("Fixture file %s does not exist", path)
			}
		}
	}
}

// TestE2E_HelmResultAPI verifies the HelmResult helper methods.
func TestE2E_HelmResultAPI(t *testing.T) {
	t.Run("SuccessResult", func(t *testing.T) {
		r := &HelmResult{ExitCode: 0, Stdout: "OK", Stderr: ""}
		if !r.Success() {
			t.Error("Expected success")
		}
		if r.ContainsError("ERROR") {
			t.Error("Should not contain ERROR")
		}
	})

	t.Run("FailureResult", func(t *testing.T) {
		r := &HelmResult{
			ExitCode: 1,
			Stdout:   "",
			Stderr:   "Error: something went wrong",
			Err:      os.ErrNotExist,
		}
		if r.Success() {
			t.Error("Expected failure")
		}
		if !r.ContainsError("something went wrong") {
			t.Error("Should contain error message")
		}
	})
}
