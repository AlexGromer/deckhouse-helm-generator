package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	sigsyaml "sigs.k8s.io/yaml"
)

// ============================================================
// Subtask 1: Single Deployment + Service
// ============================================================

func TestPipeline_SingleDeploymentAndService(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	h.WriteInputFile("deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: webapp
  namespace: default
  labels:
    app.kubernetes.io/name: webapp
spec:
  replicas: 3
  selector:
    matchLabels:
      app.kubernetes.io/name: webapp
  template:
    metadata:
      labels:
        app.kubernetes.io/name: webapp
    spec:
      containers:
        - name: web
          image: nginx:1.25
          ports:
            - containerPort: 80
          resources:
            limits:
              cpu: "500m"
              memory: "256Mi"
            requests:
              cpu: "100m"
              memory: "128Mi"
`)

	h.WriteInputFile("service.yaml", `apiVersion: v1
kind: Service
metadata:
  name: webapp
  namespace: default
  labels:
    app.kubernetes.io/name: webapp
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/name: webapp
  ports:
    - name: http
      port: 80
      targetPort: 80
      protocol: TCP
`)

	opts := PipelineOptions{
		ChartName:    "myapp",
		ChartVersion: "1.0.0",
		AppVersion:   "1.0.0",
		Namespace:    "default",
	}

	output, err := ExecutePipeline(h.InputDir, opts)
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// ── 1 chart generated ──
	if len(output.Charts) != 1 {
		t.Fatalf("Expected 1 chart, got %d", len(output.Charts))
	}
	chart := output.Charts[0]
	if chart.Name != "myapp" {
		t.Errorf("Chart name: got %q, want %q", chart.Name, "myapp")
	}

	// ── 2 resources processed ──
	if len(output.Resources) != 2 {
		t.Errorf("Expected 2 resources, got %d", len(output.Resources))
	}

	// ── Correct template paths exist ──
	if _, ok := chart.Templates["templates/webapp-deployment.yaml"]; !ok {
		t.Error("Missing deployment template")
	}
	if _, ok := chart.Templates["templates/webapp-service.yaml"]; !ok {
		t.Error("Missing service template")
	}

	// ── 1 group with both resources ──
	if len(output.Graph.Groups) != 1 {
		t.Fatalf("Expected 1 group, got %d", len(output.Graph.Groups))
	}
	group := output.Graph.Groups[0]
	if group.Name != "webapp" {
		t.Errorf("Group name: got %q, want %q", group.Name, "webapp")
	}
	if len(group.Resources) != 2 {
		t.Errorf("Group resources: got %d, want 2", len(group.Resources))
	}

	// ── Service → Deployment relationship ──
	if len(output.Graph.Relationships) < 1 {
		t.Fatal("Expected at least 1 relationship")
	}
	found := false
	for _, rel := range output.Graph.Relationships {
		if rel.From.GVK.Kind == "Service" && rel.To.GVK.Kind == "Deployment" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected Service → Deployment relationship")
	}

	// ── Values contain expected data ──
	var values map[string]interface{}
	if err := sigsyaml.Unmarshal([]byte(chart.ValuesYAML), &values); err != nil {
		t.Fatalf("Failed to parse values.yaml: %v", err)
	}

	services, ok := values["services"].(map[string]interface{})
	if !ok {
		t.Fatal("values.yaml missing 'services' key")
	}
	webapp, ok := services["webapp"].(map[string]interface{})
	if !ok {
		t.Fatal("values.yaml missing 'services.webapp' key")
	}

	// Replicas extracted
	if _, hasReplicas := findNestedKey(webapp, "replicas"); !hasReplicas {
		t.Error("Replicas not found in values")
	}

	// Container image extracted
	if _, hasContainers := findNestedKey(webapp, "containers"); !hasContainers {
		t.Error("Containers not found in values")
	}

	// Service port extracted
	if _, hasPorts := findNestedKey(webapp, "ports"); !hasPorts {
		t.Error("Service ports not found in values")
	}

	// ── Chart structure on disk ──
	chartDir := filepath.Join(output.OutputDir, "myapp")
	ValidateChartStructure(t, chartDir)
}

// ============================================================
// Subtask 2: Deployment + ConfigMap
// ============================================================

func TestPipeline_DeploymentWithConfigMap(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	h.WriteInputFile("deployment.yaml", `apiVersion: apps/v1
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
        - name: app
          image: myapp:latest
          envFrom:
            - configMapRef:
                name: webapp
`)

	h.WriteInputFile("configmap.yaml", `apiVersion: v1
kind: ConfigMap
metadata:
  name: webapp
  namespace: default
  labels:
    app.kubernetes.io/name: webapp
data:
  APP_ENV: production
  LOG_LEVEL: info
`)

	opts := PipelineOptions{
		ChartName: "cmtest",
	}

	output, err := ExecutePipeline(h.InputDir, opts)
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// ── 2+ resources ──
	if len(output.Resources) < 2 {
		t.Errorf("Expected at least 2 resources, got %d", len(output.Resources))
	}

	// ── ConfigMap values present ──
	var values map[string]interface{}
	if err := sigsyaml.Unmarshal([]byte(output.Charts[0].ValuesYAML), &values); err != nil {
		t.Fatalf("Failed to parse values.yaml: %v", err)
	}

	services, ok := values["services"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing 'services' key in values")
	}
	webapp, ok := services["webapp"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing 'services.webapp' key in values")
	}

	// configMaps should exist in values (ConfigMaps always nested by name)
	configMaps, ok := webapp["configMaps"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing 'configMaps' key in service values")
	}
	if len(configMaps) == 0 {
		t.Error("configMaps section is empty")
	}

	// ── ConfigMap template exists ──
	hasConfigMapTemplate := false
	for path := range output.Charts[0].Templates {
		if strings.Contains(path, "configmap") {
			hasConfigMapTemplate = true
			break
		}
	}
	if !hasConfigMapTemplate {
		t.Error("No configmap template found in chart")
	}

	// ── Relationships detected ──
	hasConfigMapRel := false
	for _, rel := range output.Graph.Relationships {
		if rel.To.GVK.Kind == "ConfigMap" || rel.From.GVK.Kind == "ConfigMap" {
			hasConfigMapRel = true
			break
		}
	}
	// Note: envFrom creates a dependency at process time, and detectors also find it
	if !hasConfigMapRel && !hasDeploymentDep(output, "ConfigMap") {
		t.Log("No ConfigMap relationship in graph (may be detected as dependency instead)")
	}
}

// ============================================================
// Subtask 3: Deployment + Secret
// ============================================================

func TestPipeline_DeploymentWithSecret(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	h.WriteInputFile("deployment.yaml", `apiVersion: apps/v1
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
        - name: app
          image: myapp:latest
          env:
            - name: DB_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: webapp
                  key: password
`)

	h.WriteInputFile("secret.yaml", `apiVersion: v1
kind: Secret
metadata:
  name: webapp
  namespace: default
  labels:
    app.kubernetes.io/name: webapp
type: Opaque
data:
  password: cGFzc3dvcmQ=
`)

	opts := PipelineOptions{
		ChartName: "secrettest",
	}

	output, err := ExecutePipeline(h.InputDir, opts)
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	// ── 2+ resources ──
	if len(output.Resources) < 2 {
		t.Errorf("Expected at least 2 resources, got %d", len(output.Resources))
	}

	// ── Secret values present ──
	var values map[string]interface{}
	if err := sigsyaml.Unmarshal([]byte(output.Charts[0].ValuesYAML), &values); err != nil {
		t.Fatalf("Failed to parse values.yaml: %v", err)
	}

	services, ok := values["services"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing 'services' key")
	}
	webapp, ok := services["webapp"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing 'services.webapp' key")
	}

	// secrets should exist in values (Secrets always nested by name)
	secrets, ok := webapp["secrets"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing 'secrets' key in service values")
	}
	if len(secrets) == 0 {
		t.Error("secrets section is empty")
	}

	// ── Secret template exists ──
	hasSecretTemplate := false
	for path := range output.Charts[0].Templates {
		if strings.Contains(path, "secret") {
			hasSecretTemplate = true
			break
		}
	}
	if !hasSecretTemplate {
		t.Error("No secret template found in chart")
	}
}

// ============================================================
// Subtask 4: Values.yaml correctness
// ============================================================

func TestPipeline_ValuesYAMLCorrectness(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	h.WriteInputFile("deployment.yaml", `apiVersion: apps/v1
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
        - name: web
          image: nginx:1.25
          ports:
            - containerPort: 8080
`)

	h.WriteInputFile("service.yaml", `apiVersion: v1
kind: Service
metadata:
  name: webapp
  namespace: default
  labels:
    app.kubernetes.io/name: webapp
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/name: webapp
  ports:
    - name: http
      port: 80
      targetPort: 8080
`)

	h.WriteInputFile("configmap.yaml", `apiVersion: v1
kind: ConfigMap
metadata:
  name: appconfig
  namespace: default
  labels:
    app.kubernetes.io/name: webapp
data:
  LOG_LEVEL: debug
`)

	opts := PipelineOptions{
		ChartName:    "valtest",
		ChartVersion: "2.0.0",
		AppVersion:   "3.0.0",
	}

	output, err := ExecutePipeline(h.InputDir, opts)
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	chart := output.Charts[0]

	// ── Valid YAML ──
	ValidateValues(t, chart.ValuesYAML)

	var values map[string]interface{}
	if err := sigsyaml.Unmarshal([]byte(chart.ValuesYAML), &values); err != nil {
		t.Fatalf("Failed to parse values: %v", err)
	}

	// ── Has global section ──
	global, ok := values["global"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing 'global' section in values")
	}
	if _, ok := global["imageRegistry"]; !ok {
		t.Error("global.imageRegistry missing")
	}

	// ── Has services section ──
	services, ok := values["services"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing 'services' section in values")
	}
	if len(services) == 0 {
		t.Error("services section is empty")
	}

	webapp, ok := services["webapp"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing 'services.webapp' section")
	}

	// ── enabled is bool ──
	enabled, ok := webapp["enabled"]
	if !ok {
		t.Error("Missing 'enabled' key in service")
	} else if _, isBool := enabled.(bool); !isBool {
		t.Errorf("enabled should be bool, got %T", enabled)
	}

	// ── Check types: replicas should be numeric ──
	replicasVal, hasReplicas := findNestedKey(webapp, "replicas")
	if !hasReplicas {
		t.Error("replicas not found in values")
	} else {
		switch replicasVal.(type) {
		case int, int64, float64:
			// OK
		default:
			t.Errorf("replicas should be numeric, got %T", replicasVal)
		}
	}

	// ── Container image is map with repository+tag ──
	containersVal, hasContainers := findNestedKey(webapp, "containers")
	if !hasContainers {
		t.Error("containers not found in values")
	} else if containers, ok := containersVal.([]interface{}); ok && len(containers) > 0 {
		container, ok := containers[0].(map[string]interface{})
		if !ok {
			t.Error("container should be a map")
		} else {
			imageVal, ok := container["image"].(map[string]interface{})
			if !ok {
				t.Error("container.image should be a map")
			} else {
				if _, ok := imageVal["repository"]; !ok {
					t.Error("container.image.repository missing")
				}
				if _, ok := imageVal["tag"]; !ok {
					t.Error("container.image.tag missing")
				}
			}
		}
	}

	// ── Port is numeric ──
	portsVal, hasPorts := findNestedKey(webapp, "ports")
	if !hasPorts {
		t.Error("ports not found in values")
	} else if ports, ok := portsVal.([]interface{}); ok && len(ports) > 0 {
		port, ok := ports[0].(map[string]interface{})
		if !ok {
			t.Error("port should be a map")
		} else {
			portNum := port["port"]
			switch portNum.(type) {
			case int, int64, float64:
				// OK
			default:
				t.Errorf("port.port should be numeric, got %T", portNum)
			}
		}
	}

	// ── Chart.yaml correctness ──
	var chartMeta map[string]interface{}
	if err := sigsyaml.Unmarshal([]byte(chart.ChartYAML), &chartMeta); err != nil {
		t.Fatalf("Invalid Chart.yaml: %v", err)
	}
	if chartMeta["name"] != "valtest" {
		t.Errorf("Chart.yaml name: got %v, want valtest", chartMeta["name"])
	}
	if chartMeta["version"] != "2.0.0" {
		t.Errorf("Chart.yaml version: got %v, want 2.0.0", chartMeta["version"])
	}
	if chartMeta["appVersion"] != "3.0.0" {
		t.Errorf("Chart.yaml appVersion: got %v, want 3.0.0", chartMeta["appVersion"])
	}
}

// ============================================================
// Subtask 5: Helm lint validation
// ============================================================

func TestPipeline_HelmLint(t *testing.T) {
	helmPath := findHelm(t)

	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	h.WriteInputFile("deployment.yaml", `apiVersion: apps/v1
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
        - name: web
          image: nginx:latest
          ports:
            - containerPort: 80
`)

	opts := PipelineOptions{
		ChartName:    "linttest",
		ChartVersion: "0.1.0",
		AppVersion:   "1.0.0",
	}

	output, err := ExecutePipeline(h.InputDir, opts)
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	chartDir := filepath.Join(output.OutputDir, "linttest")

	cmd := exec.Command(helmPath, "lint", chartDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("helm lint failed (exit: %v):\n%s", err, string(out))
	} else {
		t.Logf("helm lint output:\n%s", string(out))
	}

	// No errors in output
	if strings.Contains(strings.ToLower(string(out)), "[error]") {
		t.Errorf("helm lint produced errors:\n%s", string(out))
	}
}

// ============================================================
// Subtask 6: Helm template validation
// ============================================================

func TestPipeline_HelmTemplate(t *testing.T) {
	helmPath := findHelm(t)

	h := NewTestHarness(t)
	h.Setup()
	defer h.Cleanup()

	h.WriteInputFile("deployment.yaml", `apiVersion: apps/v1
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
        - name: web
          image: nginx:1.25
          ports:
            - containerPort: 80
`)

	h.WriteInputFile("service.yaml", `apiVersion: v1
kind: Service
metadata:
  name: webapp
  namespace: default
  labels:
    app.kubernetes.io/name: webapp
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/name: webapp
  ports:
    - name: http
      port: 80
      targetPort: 80
`)

	opts := PipelineOptions{
		ChartName:    "tmpltest",
		ChartVersion: "0.1.0",
		AppVersion:   "1.0.0",
	}

	output, err := ExecutePipeline(h.InputDir, opts)
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	chartDir := filepath.Join(output.OutputDir, "tmpltest")

	cmd := exec.Command(helmPath, "template", "test-release", chartDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("helm template failed (exit: %v):\n%s", err, string(out))
	}

	// Output is non-empty
	rendered := strings.TrimSpace(string(out))
	if rendered == "" {
		t.Error("helm template produced empty output")
	}

	// Output is valid YAML (each document separated by ---)
	docs := strings.Split(rendered, "---")
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" || strings.HasPrefix(doc, "#") {
			continue
		}
		var parsed interface{}
		if err := sigsyaml.Unmarshal([]byte(doc), &parsed); err != nil {
			t.Errorf("helm template output contains invalid YAML:\n%s\nerror: %v", doc, err)
		}
	}

	t.Logf("helm template output (%d bytes):\n%s", len(out), string(out))
}

// ============================================================
// Test helpers
// ============================================================

// findHelm locates the helm binary or skips the test.
func findHelm(t *testing.T) string {
	t.Helper()

	// Check common locations
	candidates := []string{
		"helm",
		filepath.Join(os.Getenv("HOME"), ".local", "bin", "helm"),
		"/usr/local/bin/helm",
	}

	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}

	t.Skip("helm not found in PATH, skipping helm validation test")
	return ""
}

// findNestedKey searches recursively for a key in a nested map.
func findNestedKey(m map[string]interface{}, key string) (interface{}, bool) {
	if val, ok := m[key]; ok {
		return val, true
	}
	for _, v := range m {
		if nested, ok := v.(map[string]interface{}); ok {
			if val, found := findNestedKey(nested, key); found {
				return val, true
			}
		}
	}
	return nil, false
}

// hasDeploymentDep checks if any Deployment resource has a dependency on the given kind.
func hasDeploymentDep(output *ChartOutput, kind string) bool {
	for _, r := range output.Resources {
		if r.Original.GVK.Kind == "Deployment" {
			for _, dep := range r.Dependencies {
				if dep.GVK.Kind == kind {
					return true
				}
			}
		}
	}
	return false
}
