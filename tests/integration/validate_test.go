package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	sigsyaml "sigs.k8s.io/yaml"
)

// ============================================================
// Task 2.5: Generator Output Validation
// Validates generated chart structure, syntax, and Helm compatibility.
// ============================================================

// generateSimpleChart is a helper that produces a chart from a basic
// Deployment+Service input. Many validation tests reuse this.
func generateSimpleChart(t *testing.T) (chartDir string, cleanup func()) {
	t.Helper()

	h := NewTestHarness(t)
	h.Setup()

	h.WriteInputFile("deployment.yaml", `
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

	h.WriteInputFile("service.yaml", `
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

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName:    "test-app",
		ChartVersion: "1.2.3",
		AppVersion:   "2.0.0",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}

	if len(output.Charts) == 0 {
		t.Fatal("Expected at least 1 generated chart")
	}

	dir := filepath.Join(output.OutputDir, output.Charts[0].Name)

	return dir, func() {
		h.Cleanup()
		os.RemoveAll(output.OutputDir)
	}
}

// ============================================================
// Subtask 1: Chart.yaml structure validation
// ============================================================

func TestValidate_ChartYAMLStructure(t *testing.T) {
	chartDir, cleanup := generateSimpleChart(t)
	defer cleanup()

	chartPath := filepath.Join(chartDir, "Chart.yaml")
	data, err := os.ReadFile(chartPath)
	if err != nil {
		t.Fatalf("Cannot read Chart.yaml: %v", err)
	}

	var chartYAML map[string]interface{}
	if err := sigsyaml.Unmarshal(data, &chartYAML); err != nil {
		t.Fatalf("Chart.yaml is not valid YAML: %v", err)
	}

	// Required fields must be present
	requiredFields := []string{"apiVersion", "name", "version"}
	for _, field := range requiredFields {
		if _, ok := chartYAML[field]; !ok {
			t.Errorf("Required field '%s' missing from Chart.yaml", field)
		}
	}

	// apiVersion must be v2
	if apiVersion, ok := chartYAML["apiVersion"].(string); ok {
		if apiVersion != "v2" {
			t.Errorf("Expected apiVersion 'v2', got '%s'", apiVersion)
		}
	} else {
		t.Error("apiVersion is not a string")
	}

	// name must match chart name
	if name, ok := chartYAML["name"].(string); ok {
		if name != "test-app" {
			t.Errorf("Expected name 'test-app', got '%s'", name)
		}
	} else {
		t.Error("name is not a string")
	}

	// version must be valid semver
	if version, ok := chartYAML["version"].(string); ok {
		semverRegex := regexp.MustCompile(`^\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?(\+[a-zA-Z0-9.]+)?$`)
		if !semverRegex.MatchString(version) {
			t.Errorf("Version '%s' is not valid semver", version)
		}
		if version != "1.2.3" {
			t.Errorf("Expected version '1.2.3', got '%s'", version)
		}
	} else {
		t.Error("version is not a string")
	}

	// appVersion should be present
	if appVersion, ok := chartYAML["appVersion"].(string); ok {
		if appVersion != "2.0.0" {
			t.Errorf("Expected appVersion '2.0.0', got '%s'", appVersion)
		}
	}

	// type should be 'application'
	if chartType, ok := chartYAML["type"].(string); ok {
		if chartType != "application" {
			t.Errorf("Expected type 'application', got '%s'", chartType)
		}
	}

	// description should be non-empty
	if desc, ok := chartYAML["description"].(string); ok {
		if desc == "" {
			t.Error("Expected non-empty description")
		}
	}
}

// ============================================================
// Subtask 2: values.yaml syntax validation
// ============================================================

func TestValidate_ValuesYAMLSyntax(t *testing.T) {
	chartDir, cleanup := generateSimpleChart(t)
	defer cleanup()

	valuesPath := filepath.Join(chartDir, "values.yaml")
	data, err := os.ReadFile(valuesPath)
	if err != nil {
		t.Fatalf("Cannot read values.yaml: %v", err)
	}

	content := string(data)

	// Must be non-empty
	if strings.TrimSpace(content) == "" {
		t.Fatal("values.yaml is empty")
	}

	// Valid YAML syntax
	var valuesMap map[string]interface{}
	if err := sigsyaml.Unmarshal(data, &valuesMap); err != nil {
		t.Fatalf("values.yaml is not valid YAML: %v", err)
	}

	// No tab characters (YAML should use spaces)
	if strings.Contains(content, "\t") {
		t.Error("values.yaml contains tab characters, should use spaces only")
	}

	// Must have 'services' section
	if _, ok := valuesMap["services"]; !ok {
		t.Error("Expected 'services' section in values.yaml")
	}

	// Must have 'global' section
	if _, ok := valuesMap["global"]; !ok {
		t.Error("Expected 'global' section in values.yaml")
	}

	// Services should contain our service
	services, ok := valuesMap["services"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected services to be a map")
	}
	if len(services) == 0 {
		t.Error("Expected at least one service in values")
	}

	// Verify no duplicate keys by re-parsing and checking structure
	// (sigs.k8s.io/yaml handles this implicitly — last value wins)
	ValidateValues(t, content)
}

// ============================================================
// Subtask 3: Template rendering validation
// ============================================================

func TestValidate_TemplateRendering(t *testing.T) {
	chartDir, cleanup := generateSimpleChart(t)
	defer cleanup()

	// Find helm
	helmPath := findHelm(t)
	if helmPath == "" {
		t.Skip("helm not found, skipping template rendering test")
	}

	// helm template should succeed with default values
	cmd := exec.Command(helmPath, "template", "test-release", chartDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\nOutput:\n%s", err, string(output))
	}

	rendered := string(output)

	// Output should be non-empty
	if strings.TrimSpace(rendered) == "" {
		t.Fatal("helm template produced empty output")
	}

	// Output should be valid YAML (may contain multiple documents)
	docs := strings.Split(rendered, "---")
	validDocs := 0
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		var parsed interface{}
		if err := sigsyaml.Unmarshal([]byte(doc), &parsed); err != nil {
			t.Errorf("Template rendered invalid YAML document: %v\nDocument:\n%s", err, doc)
		} else {
			validDocs++
		}
	}

	if validDocs == 0 {
		t.Error("No valid YAML documents in helm template output")
	}

	// Should contain expected resource types
	if !strings.Contains(rendered, "kind: Deployment") {
		t.Error("Expected Deployment in rendered output")
	}
	if !strings.Contains(rendered, "kind: Service") {
		t.Error("Expected Service in rendered output")
	}
}

// ============================================================
// Subtask 4: _helpers.tpl validation
// ============================================================

func TestValidate_HelpersTpl(t *testing.T) {
	chartDir, cleanup := generateSimpleChart(t)
	defer cleanup()

	helpersPath := filepath.Join(chartDir, "templates", "_helpers.tpl")
	data, err := os.ReadFile(helpersPath)
	if err != nil {
		t.Fatalf("Cannot read _helpers.tpl: %v", err)
	}

	helpers := string(data)

	if helpers == "" {
		t.Fatal("_helpers.tpl is empty")
	}

	// Standard helpers must be present
	requiredHelpers := []struct {
		name    string
		pattern string
	}{
		{"fullname", `define "test-app.fullname"`},
		{"name", `define "test-app.name"`},
		{"labels", `define "test-app.labels"`},
		{"selectorLabels", `define "test-app.selectorLabels"`},
		{"chart", `define "test-app.chart"`},
		{"serviceAccountName", `define "test-app.serviceAccountName"`},
		{"imagePullSecrets", `define "test-app.imagePullSecrets"`},
	}

	for _, helper := range requiredHelpers {
		if !strings.Contains(helpers, helper.pattern) {
			t.Errorf("Expected helper '%s' (pattern: %s) in _helpers.tpl", helper.name, helper.pattern)
		}
	}

	// Helpers should be properly closed (matching define/end pairs)
	defineCount := strings.Count(helpers, "{{- define")
	endCount := strings.Count(helpers, "{{- end")
	if defineCount == 0 {
		t.Error("No template definitions found in _helpers.tpl")
	}
	// Each define needs at least one end
	if endCount < defineCount {
		t.Errorf("Mismatched define/end: %d defines, %d ends", defineCount, endCount)
	}

	// Labels helper should include standard K8s labels
	if !strings.Contains(helpers, "app.kubernetes.io/name") {
		t.Error("Labels helper should include app.kubernetes.io/name")
	}
	if !strings.Contains(helpers, "app.kubernetes.io/instance") {
		t.Error("Labels helper should include app.kubernetes.io/instance")
	}
	if !strings.Contains(helpers, "app.kubernetes.io/managed-by") {
		t.Error("Labels helper should include app.kubernetes.io/managed-by")
	}
	if !strings.Contains(helpers, "helm.sh/chart") {
		t.Error("Labels helper should include helm.sh/chart")
	}
}

// ============================================================
// Subtask 5: NOTES.txt validation
// ============================================================

func TestValidate_NOTESTxt(t *testing.T) {
	chartDir, cleanup := generateSimpleChart(t)
	defer cleanup()

	notesPath := filepath.Join(chartDir, "templates", "NOTES.txt")
	data, err := os.ReadFile(notesPath)
	if err != nil {
		t.Fatalf("Cannot read NOTES.txt: %v", err)
	}

	notes := string(data)

	if strings.TrimSpace(notes) == "" {
		t.Fatal("NOTES.txt is empty")
	}

	// Should contain useful post-install instructions
	if !strings.Contains(notes, "installed") {
		t.Error("NOTES.txt should mention installation status")
	}

	// Should reference kubectl for verification
	if !strings.Contains(notes, "kubectl") {
		t.Error("NOTES.txt should include kubectl verification command")
	}

	// Should reference the release name
	if !strings.Contains(notes, ".Release.Name") {
		t.Error("NOTES.txt should reference .Release.Name")
	}

	// Should reference the namespace
	if !strings.Contains(notes, ".Release.Namespace") {
		t.Error("NOTES.txt should reference .Release.Namespace")
	}

	// Should mention helm upgrade for customization
	if !strings.Contains(notes, "helm upgrade") {
		t.Error("NOTES.txt should mention helm upgrade for customization")
	}
}

// ============================================================
// Subtask 6: Chart dependencies validation
// ============================================================

func TestValidate_ChartDependencies(t *testing.T) {
	chartDir, cleanup := generateSimpleChart(t)
	defer cleanup()

	// Read Chart.yaml to check for dependencies
	chartPath := filepath.Join(chartDir, "Chart.yaml")
	data, err := os.ReadFile(chartPath)
	if err != nil {
		t.Fatalf("Cannot read Chart.yaml: %v", err)
	}

	var chartYAML map[string]interface{}
	if err := sigsyaml.Unmarshal(data, &chartYAML); err != nil {
		t.Fatalf("Chart.yaml is not valid YAML: %v", err)
	}

	// If dependencies are declared, Chart.lock should exist
	if deps, ok := chartYAML["dependencies"]; ok && deps != nil {
		depsList, ok := deps.([]interface{})
		if ok && len(depsList) > 0 {
			lockPath := filepath.Join(chartDir, "Chart.lock")
			if _, err := os.Stat(lockPath); os.IsNotExist(err) {
				t.Error("Chart.yaml has dependencies but Chart.lock is missing")
			}
		}
	}

	// For charts without dependencies, verify no stale Chart.lock exists
	if _, ok := chartYAML["dependencies"]; !ok {
		lockPath := filepath.Join(chartDir, "Chart.lock")
		if _, err := os.Stat(lockPath); err == nil {
			t.Error("Chart.lock exists but no dependencies declared in Chart.yaml")
		}
	}

	// Verify chart apiVersion is v2 (required for dependencies)
	apiVersion, _ := chartYAML["apiVersion"].(string)
	if apiVersion != "v2" {
		t.Errorf("apiVersion should be 'v2' for dependency support, got '%s'", apiVersion)
	}
}

// ============================================================
// Subtask 7: Kubernetes API version validation
// ============================================================

func TestValidate_KubernetesAPIVersions(t *testing.T) {
	chartDir, cleanup := generateSimpleChart(t)
	defer cleanup()

	helmPath := findHelm(t)
	if helmPath == "" {
		t.Skip("helm not found, skipping API version test")
	}

	// Render templates
	cmd := exec.Command(helmPath, "template", "test-release", chartDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\nOutput:\n%s", err, string(output))
	}

	rendered := string(output)

	// Deprecated API versions that should NOT appear
	deprecatedAPIs := []struct {
		deprecated  string
		replacement string
	}{
		{"extensions/v1beta1", "apps/v1 or networking.k8s.io/v1"},
		{"apps/v1beta1", "apps/v1"},
		{"apps/v1beta2", "apps/v1"},
		{"networking.k8s.io/v1beta1", "networking.k8s.io/v1"},
		{"policy/v1beta1", "policy/v1"},
		{"rbac.authorization.k8s.io/v1beta1", "rbac.authorization.k8s.io/v1"},
	}

	for _, api := range deprecatedAPIs {
		if strings.Contains(rendered, "apiVersion: "+api.deprecated) {
			t.Errorf("Deprecated API version found: '%s' (use %s instead)",
				api.deprecated, api.replacement)
		}
	}

	// Expected API versions
	docs := strings.Split(rendered, "---")
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		var resource map[string]interface{}
		if err := sigsyaml.Unmarshal([]byte(doc), &resource); err != nil {
			continue
		}

		apiVersion, _ := resource["apiVersion"].(string)
		kind, _ := resource["kind"].(string)

		// Validate specific API versions per kind
		switch kind {
		case "Deployment", "StatefulSet", "DaemonSet":
			if apiVersion != "apps/v1" {
				t.Errorf("%s should use apiVersion 'apps/v1', got '%s'", kind, apiVersion)
			}
		case "Service", "ConfigMap", "Secret":
			if apiVersion != "v1" {
				t.Errorf("%s should use apiVersion 'v1', got '%s'", kind, apiVersion)
			}
		case "Ingress":
			if apiVersion != "networking.k8s.io/v1" {
				t.Errorf("Ingress should use apiVersion 'networking.k8s.io/v1', got '%s'", apiVersion)
			}
		}
	}
}

// ============================================================
// Subtask 8: Label consistency validation
// ============================================================

func TestValidate_LabelConsistency(t *testing.T) {
	chartDir, cleanup := generateSimpleChart(t)
	defer cleanup()

	helmPath := findHelm(t)
	if helmPath == "" {
		t.Skip("helm not found, skipping label consistency test")
	}

	// Render templates
	cmd := exec.Command(helmPath, "template", "test-release", chartDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\nOutput:\n%s", err, string(output))
	}

	rendered := string(output)

	// Parse all resources
	docs := strings.Split(rendered, "---")
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		var resource map[string]interface{}
		if err := sigsyaml.Unmarshal([]byte(doc), &resource); err != nil {
			continue
		}

		kind, _ := resource["kind"].(string)
		if kind == "" {
			continue
		}

		// Get metadata.labels
		metadata, _ := resource["metadata"].(map[string]interface{})
		if metadata == nil {
			t.Errorf("%s resource is missing metadata", kind)
			continue
		}

		labels, _ := metadata["labels"].(map[string]interface{})
		if labels == nil {
			t.Errorf("%s resource is missing labels", kind)
			continue
		}

		// All resources should have standard Kubernetes labels
		requiredLabels := []string{
			"helm.sh/chart",
			"app.kubernetes.io/managed-by",
		}

		for _, label := range requiredLabels {
			if _, ok := labels[label]; !ok {
				t.Errorf("%s is missing label '%s'", kind, label)
			}
		}

		// managed-by should be Helm
		if managedBy, ok := labels["app.kubernetes.io/managed-by"].(string); ok {
			if managedBy != "Helm" {
				t.Errorf("%s: app.kubernetes.io/managed-by should be 'Helm', got '%s'", kind, managedBy)
			}
		}
	}
}

// ============================================================
// Subtask 9: Security validation
// ============================================================

func TestValidate_Security(t *testing.T) {
	chartDir, cleanup := generateSimpleChart(t)
	defer cleanup()

	// Check templates for hardcoded secrets
	templatesDir := filepath.Join(chartDir, "templates")
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		t.Fatalf("Cannot read templates directory: %v", err)
	}

	secretPatterns := []struct {
		pattern string
		desc    string
	}{
		{`password:\s*["']?[a-zA-Z0-9]+["']?`, "hardcoded password"},
		{`apiKey:\s*["']?[a-zA-Z0-9]{20,}["']?`, "hardcoded API key"},
		{`secret:\s*["']?[a-zA-Z0-9]{20,}["']?`, "hardcoded secret value"},
		{`token:\s*["']?[a-zA-Z0-9]{20,}["']?`, "hardcoded token"},
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(templatesDir, entry.Name()))
		if err != nil {
			t.Errorf("Cannot read template %s: %v", entry.Name(), err)
			continue
		}

		content := string(data)

		for _, sp := range secretPatterns {
			re := regexp.MustCompile(sp.pattern)
			if re.MatchString(content) {
				// Filter out template references ({{ .Values.xxx }})
				matches := re.FindAllString(content, -1)
				for _, match := range matches {
					if !strings.Contains(match, "{{") && !strings.Contains(match, ".Values") {
						t.Errorf("Potential %s found in template %s: %s",
							sp.desc, entry.Name(), match)
					}
				}
			}
		}
	}

	// Check values.yaml for sensitive defaults
	valuesPath := filepath.Join(chartDir, "values.yaml")
	valuesData, err := os.ReadFile(valuesPath)
	if err != nil {
		t.Fatalf("Cannot read values.yaml: %v", err)
	}

	valuesContent := string(valuesData)

	// Should not have plaintext passwords in default values
	sensitiveKeys := []string{
		"password: ", "apiKey: ", "secretKey: ", "privateKey: ",
	}
	for _, key := range sensitiveKeys {
		if strings.Contains(valuesContent, key) {
			// Check if it's just a placeholder or empty
			lines := strings.Split(valuesContent, "\n")
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.Contains(trimmed, key) {
					// Allow empty values or "changeme" style placeholders
					value := strings.TrimPrefix(trimmed, key)
					value = strings.TrimSpace(value)
					if value != "" && value != `""` && value != "''" &&
						value != "changeme" && !strings.HasPrefix(value, "#") {
						t.Logf("Warning: potential sensitive default for '%s' in values.yaml: %s",
							key, trimmed)
					}
				}
			}
		}
	}
}

// ============================================================
// Subtask 10: Helm lint (automated)
// ============================================================

func TestValidate_HelmLint(t *testing.T) {
	helmPath := findHelm(t)
	if helmPath == "" {
		t.Skip("helm not found, skipping helm lint test")
	}

	// Test with simple chart
	t.Run("SimpleChart", func(t *testing.T) {
		chartDir, cleanup := generateSimpleChart(t)
		defer cleanup()

		cmd := exec.Command(helmPath, "lint", chartDir)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("helm lint failed: %v\nOutput:\n%s", err, string(output))
		}

		outputStr := string(output)
		if strings.Contains(outputStr, "ERROR") {
			t.Errorf("helm lint reported errors:\n%s", outputStr)
		}
	})

	// Test with full-stack chart (from Task 2.3 scenario)
	t.Run("FullStackChart", func(t *testing.T) {
		h := NewTestHarness(t)
		h.Setup()
		t.Cleanup(h.Cleanup)

		h.WriteInputFile("frontend.yaml", `
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
`)
		h.WriteInputFile("backend.yaml", `
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
`)
		h.WriteInputFile("frontend-svc.yaml", `
apiVersion: v1
kind: Service
metadata:
  name: frontend-svc
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
		h.WriteInputFile("backend-svc.yaml", `
apiVersion: v1
kind: Service
metadata:
  name: backend-svc
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

		output, err := ExecutePipeline(h.InputDir, PipelineOptions{
			ChartName: "fullstack-app",
		})
		if err != nil {
			t.Fatalf("Pipeline failed: %v", err)
		}
		defer os.RemoveAll(output.OutputDir)

		if len(output.Charts) == 0 {
			t.Fatal("Expected charts")
		}

		fullstackDir := filepath.Join(output.OutputDir, output.Charts[0].Name)
		cmd := exec.Command(helmPath, "lint", fullstackDir)
		lintOutput, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("helm lint failed for full-stack chart: %v\nOutput:\n%s", err, string(lintOutput))
		}
	})

	// Test with Deckhouse CRD chart (from Task 2.4 scenario)
	t.Run("DeckhouseCRDChart", func(t *testing.T) {
		h := NewTestHarness(t)
		h.Setup()
		t.Cleanup(h.Cleanup)

		h.WriteInputFile("module-config.yaml", `
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: test-module
  labels:
    app: test-module
spec:
  enabled: true
  version: 1
  settings:
    replicas: 3
`)

		output, err := ExecutePipeline(h.InputDir, PipelineOptions{
			ChartName: "deckhouse-chart",
		})
		if err != nil {
			t.Fatalf("Pipeline failed: %v", err)
		}
		defer os.RemoveAll(output.OutputDir)

		if len(output.Charts) == 0 {
			t.Fatal("Expected charts")
		}

		deckhouseDir := filepath.Join(output.OutputDir, output.Charts[0].Name)
		cmd := exec.Command(helmPath, "lint", deckhouseDir)
		lintOutput, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("helm lint failed for Deckhouse chart: %v\nOutput:\n%s", err, string(lintOutput))
		}
	})
}
