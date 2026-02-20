package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Registry Tests
// ============================================================

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.generators == nil {
		t.Fatal("generators map not initialized")
	}
}

func TestRegistry_Register_And_Get(t *testing.T) {
	r := NewRegistry()
	gen := NewUniversalGenerator()
	r.Register(gen)

	got, err := r.Get(types.OutputModeUniversal)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.Mode() != types.OutputModeUniversal {
		t.Errorf("expected mode %s, got %s", types.OutputModeUniversal, got.Mode())
	}
}

func TestRegistry_Get_NotRegistered(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get(types.OutputModeUniversal)
	if err == nil {
		t.Fatal("expected error for unregistered mode")
	}
	if !strings.Contains(err.Error(), "no generator registered") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRegistry_Register_Overwrite(t *testing.T) {
	r := NewRegistry()
	gen1 := NewUniversalGenerator()
	gen2 := NewUniversalGenerator()
	r.Register(gen1)
	r.Register(gen2)

	got, err := r.Get(types.OutputModeUniversal)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	// Should return the last registered generator
	if got == nil {
		t.Fatal("Get returned nil")
	}
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()
	if r == nil {
		t.Fatal("DefaultRegistry returned nil")
	}

	modes := []types.OutputMode{
		types.OutputModeUniversal,
		types.OutputModeSeparate,
		types.OutputModeLibrary,
		types.OutputModeUmbrella,
	}

	for _, mode := range modes {
		gen, err := r.Get(mode)
		if err != nil {
			t.Errorf("mode %s not registered: %v", mode, err)
			continue
		}
		if gen.Mode() != mode {
			t.Errorf("expected mode %s, got %s", mode, gen.Mode())
		}
	}
}

// ============================================================
// WriteChart Tests
// ============================================================

func TestWriteChart_BasicStructure(t *testing.T) {
	tmpDir := t.TempDir()

	chart := &types.GeneratedChart{
		Name:       "test-chart",
		ChartYAML:  "apiVersion: v2\nname: test-chart\n",
		ValuesYAML: "replicaCount: 1\n",
		Templates: map[string]string{
			"templates/deployment.yaml": "# deployment",
		},
		Helpers: "# helpers",
		Notes:   "# notes",
	}

	err := WriteChart(chart, tmpDir)
	if err != nil {
		t.Fatalf("WriteChart returned error: %v", err)
	}

	// Verify files
	chartDir := filepath.Join(tmpDir, "test-chart")

	files := []string{
		filepath.Join(chartDir, "Chart.yaml"),
		filepath.Join(chartDir, "values.yaml"),
		filepath.Join(chartDir, "templates", "deployment.yaml"),
		filepath.Join(chartDir, "templates", "_helpers.tpl"),
		filepath.Join(chartDir, "templates", "NOTES.txt"),
		filepath.Join(chartDir, ".helmignore"),
	}

	for _, f := range files {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", f)
		}
	}
}

func TestWriteChart_ChartYAMLContent(t *testing.T) {
	tmpDir := t.TempDir()

	chart := &types.GeneratedChart{
		Name:       "my-app",
		ChartYAML:  "apiVersion: v2\nname: my-app\nversion: 1.0.0\n",
		ValuesYAML: "enabled: true\n",
		Templates: map[string]string{
			"templates/deploy.yaml": "# deploy",
		},
	}

	if err := WriteChart(chart, tmpDir); err != nil {
		t.Fatalf("WriteChart returned error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, "my-app", "Chart.yaml"))
	if err != nil {
		t.Fatalf("failed to read Chart.yaml: %v", err)
	}

	if string(content) != chart.ChartYAML {
		t.Errorf("Chart.yaml content mismatch:\ngot: %s\nwant: %s", content, chart.ChartYAML)
	}
}

func TestWriteChart_WithSchema(t *testing.T) {
	tmpDir := t.TempDir()

	chart := &types.GeneratedChart{
		Name:         "schema-chart",
		ChartYAML:    "apiVersion: v2\n",
		ValuesYAML:   "enabled: true\n",
		Templates:    map[string]string{"templates/x.yaml": "# x"},
		ValuesSchema: `{"type": "object"}`,
	}

	if err := WriteChart(chart, tmpDir); err != nil {
		t.Fatalf("WriteChart returned error: %v", err)
	}

	schemaPath := filepath.Join(tmpDir, "schema-chart", "values.schema.json")
	content, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("failed to read values.schema.json: %v", err)
	}
	if string(content) != chart.ValuesSchema {
		t.Error("values.schema.json content mismatch")
	}
}

func TestWriteChart_WithExternalFiles(t *testing.T) {
	tmpDir := t.TempDir()

	chart := &types.GeneratedChart{
		Name:       "ext-chart",
		ChartYAML:  "apiVersion: v2\n",
		ValuesYAML: "ok: true\n",
		Templates:  map[string]string{"templates/x.yaml": "# x"},
		ExternalFiles: []types.ExternalFileInfo{
			{Path: "files/config.yaml", Content: "key: value"},
		},
	}

	if err := WriteChart(chart, tmpDir); err != nil {
		t.Fatalf("WriteChart returned error: %v", err)
	}

	extPath := filepath.Join(tmpDir, "ext-chart", "files", "config.yaml")
	content, err := os.ReadFile(extPath)
	if err != nil {
		t.Fatalf("failed to read external file: %v", err)
	}
	if string(content) != "key: value" {
		t.Error("external file content mismatch")
	}
}

func TestWriteChart_ExternalFile_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	chart := &types.GeneratedChart{
		Name:       "traversal-chart",
		ChartYAML:  "apiVersion: v2\n",
		ValuesYAML: "ok: true\n",
		Templates:  map[string]string{"templates/x.yaml": "# x"},
		ExternalFiles: []types.ExternalFileInfo{
			{Path: "../../../etc/passwd", Content: "malicious"},
		},
	}

	err := WriteChart(chart, tmpDir)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "outside chart directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWriteChart_NoHelpers(t *testing.T) {
	tmpDir := t.TempDir()

	chart := &types.GeneratedChart{
		Name:       "no-helpers",
		ChartYAML:  "apiVersion: v2\n",
		ValuesYAML: "ok: true\n",
		Templates:  map[string]string{"templates/x.yaml": "# x"},
		Helpers:    "", // Empty helpers
	}

	if err := WriteChart(chart, tmpDir); err != nil {
		t.Fatalf("WriteChart returned error: %v", err)
	}

	helpersPath := filepath.Join(tmpDir, "no-helpers", "templates", "_helpers.tpl")
	if _, err := os.Stat(helpersPath); !os.IsNotExist(err) {
		t.Error("_helpers.tpl should not be created when Helpers is empty")
	}
}

func TestWriteChart_NoNotes(t *testing.T) {
	tmpDir := t.TempDir()

	chart := &types.GeneratedChart{
		Name:       "no-notes",
		ChartYAML:  "apiVersion: v2\n",
		ValuesYAML: "ok: true\n",
		Templates:  map[string]string{"templates/x.yaml": "# x"},
		Notes:      "", // Empty notes
	}

	if err := WriteChart(chart, tmpDir); err != nil {
		t.Fatalf("WriteChart returned error: %v", err)
	}

	notesPath := filepath.Join(tmpDir, "no-notes", "templates", "NOTES.txt")
	if _, err := os.Stat(notesPath); !os.IsNotExist(err) {
		t.Error("NOTES.txt should not be created when Notes is empty")
	}
}

func TestWriteChart_NestedTemplateDirs(t *testing.T) {
	tmpDir := t.TempDir()

	chart := &types.GeneratedChart{
		Name:       "nested",
		ChartYAML:  "apiVersion: v2\n",
		ValuesYAML: "ok: true\n",
		Templates: map[string]string{
			"templates/sub/deployment.yaml": "# deploy",
			"templates/sub/service.yaml":    "# svc",
		},
	}

	if err := WriteChart(chart, tmpDir); err != nil {
		t.Fatalf("WriteChart returned error: %v", err)
	}

	subDir := filepath.Join(tmpDir, "nested", "templates", "sub")
	if _, err := os.Stat(subDir); os.IsNotExist(err) {
		t.Error("nested template directory was not created")
	}
}

// ============================================================
// ValidateChart Tests
// ============================================================

func TestValidateChart_Valid(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:       "valid",
		ChartYAML:  "apiVersion: v2\n",
		ValuesYAML: "ok: true\n",
		Templates:  map[string]string{"templates/x.yaml": "# x"},
	}

	if err := ValidateChart(chart); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateChart_EmptyName(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:       "",
		ChartYAML:  "apiVersion: v2\n",
		ValuesYAML: "ok: true\n",
		Templates:  map[string]string{"templates/x.yaml": "# x"},
	}

	err := ValidateChart(chart)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "chart name is empty") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateChart_EmptyChartYAML(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:       "test",
		ChartYAML:  "",
		ValuesYAML: "ok: true\n",
		Templates:  map[string]string{"templates/x.yaml": "# x"},
	}

	err := ValidateChart(chart)
	if err == nil {
		t.Fatal("expected error for empty Chart.yaml")
	}
	if !strings.Contains(err.Error(), "Chart.yaml is empty") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateChart_EmptyValuesYAML(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:       "test",
		ChartYAML:  "apiVersion: v2\n",
		ValuesYAML: "",
		Templates:  map[string]string{"templates/x.yaml": "# x"},
	}

	err := ValidateChart(chart)
	if err == nil {
		t.Fatal("expected error for empty values.yaml")
	}
	if !strings.Contains(err.Error(), "values.yaml is empty") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateChart_NoTemplates(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:       "test",
		ChartYAML:  "apiVersion: v2\n",
		ValuesYAML: "ok: true\n",
		Templates:  map[string]string{},
	}

	err := ValidateChart(chart)
	if err == nil {
		t.Fatal("expected error for no templates")
	}
	if !strings.Contains(err.Error(), "no templates generated") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ============================================================
// GetServiceNames Tests
// ============================================================

func TestGetServiceNames_Empty(t *testing.T) {
	graph := types.NewResourceGraph()
	names := GetServiceNames(graph)
	if len(names) != 0 {
		t.Errorf("expected 0 names, got %d", len(names))
	}
}

func TestGetServiceNames_Multiple(t *testing.T) {
	graph := types.NewResourceGraph()
	graph.Groups = append(graph.Groups,
		&types.ResourceGroup{Name: "zulu"},
		&types.ResourceGroup{Name: "alpha"},
		&types.ResourceGroup{Name: "mike"},
	)

	names := GetServiceNames(graph)
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}
	// Should be sorted
	if names[0] != "alpha" || names[1] != "mike" || names[2] != "zulu" {
		t.Errorf("names not sorted: %v", names)
	}
}

// ============================================================
// UniversalGenerator Tests
// ============================================================

func TestNewUniversalGenerator(t *testing.T) {
	gen := NewUniversalGenerator()
	if gen == nil {
		t.Fatal("NewUniversalGenerator returned nil")
	}
	if gen.Mode() != types.OutputModeUniversal {
		t.Errorf("expected mode %s, got %s", types.OutputModeUniversal, gen.Mode())
	}
}

func TestUniversalGenerator_Generate_Basic(t *testing.T) {
	deploy := makeProcessedResourceWithValues("Deployment", "myapp", "default",
		map[string]string{"app.kubernetes.io/name": "myapp"},
		map[string]interface{}{"replicaCount": 1},
		"# deployment template")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)
	graph.Groups = []*types.ResourceGroup{
		{
			Name:      "myapp",
			Resources: []*types.ProcessedResource{deploy},
		},
	}

	gen := NewUniversalGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{
		ChartName:    "test-chart",
		ChartVersion: "1.0.0",
		AppVersion:   "1.0.0",
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if len(charts) != 1 {
		t.Fatalf("expected 1 chart, got %d", len(charts))
	}

	chart := charts[0]
	if chart.Name != "test-chart" {
		t.Errorf("expected chart name 'test-chart', got '%s'", chart.Name)
	}
	if chart.ChartYAML == "" {
		t.Error("ChartYAML is empty")
	}
	if chart.ValuesYAML == "" {
		t.Error("ValuesYAML is empty")
	}
	if chart.Helpers == "" {
		t.Error("Helpers is empty")
	}
	if chart.Notes == "" {
		t.Error("Notes is empty")
	}
}

func TestUniversalGenerator_Generate_WithSchema(t *testing.T) {
	deploy := makeProcessedResourceWithValues("Deployment", "myapp", "default",
		map[string]string{"app.kubernetes.io/name": "myapp"},
		map[string]interface{}{"replicaCount": 1},
		"# deploy")

	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)
	graph.Groups = []*types.ResourceGroup{
		{Name: "myapp", Resources: []*types.ProcessedResource{deploy}},
	}

	gen := NewUniversalGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{
		ChartName:     "test",
		ChartVersion:  "1.0.0",
		IncludeSchema: true,
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if charts[0].ValuesSchema == "" {
		t.Error("ValuesSchema should be populated when IncludeSchema is true")
	}
}

func TestUniversalGenerator_Generate_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	graph := types.NewResourceGraph()
	gen := NewUniversalGenerator()
	_, err := gen.Generate(ctx, graph, Options{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// ============================================================
// buildServiceConfig Tests
// ============================================================

func TestBuildServiceConfig_SingleDeployment(t *testing.T) {
	deploy := makeProcessedResourceWithValues("Deployment", "myapp", "default",
		nil, map[string]interface{}{"replicaCount": 1}, "# deploy")

	group := &types.ResourceGroup{
		Name:      "myapp",
		Resources: []*types.ProcessedResource{deploy},
	}

	gen := NewUniversalGenerator()
	config := gen.buildServiceConfig(group)

	if enabled, ok := config["enabled"]; !ok || enabled != true {
		t.Error("expected enabled: true in config")
	}
	if _, ok := config["deployment"]; !ok {
		t.Error("expected 'deployment' key for single Deployment")
	}
}

func TestBuildServiceConfig_MultipleDeployments(t *testing.T) {
	deploy1 := makeProcessedResourceWithValues("Deployment", "web", "default",
		nil, map[string]interface{}{"replicaCount": 1}, "# web")
	deploy2 := makeProcessedResourceWithValues("Deployment", "worker", "default",
		nil, map[string]interface{}{"replicaCount": 2}, "# worker")

	group := &types.ResourceGroup{
		Name:      "myapp",
		Resources: []*types.ProcessedResource{deploy1, deploy2},
	}

	gen := NewUniversalGenerator()
	config := gen.buildServiceConfig(group)

	// Multiple deployments should use pluralized key with named sub-keys
	if _, ok := config["deployments"]; !ok {
		t.Error("expected 'deployments' key for multiple Deployments")
	}
}

func TestBuildServiceConfig_ConfigMap(t *testing.T) {
	cm := makeProcessedResourceWithValues("ConfigMap", "app-config", "default",
		nil, map[string]interface{}{"data": "value"}, "# cm")

	group := &types.ResourceGroup{
		Name:      "myapp",
		Resources: []*types.ProcessedResource{cm},
	}

	gen := NewUniversalGenerator()
	config := gen.buildServiceConfig(group)

	// ConfigMaps always use nested structure
	if _, ok := config["configMaps"]; !ok {
		t.Error("expected 'configMaps' key for ConfigMap (always nested)")
	}
}

func TestBuildServiceConfig_Secret(t *testing.T) {
	secret := makeProcessedResourceWithValues("Secret", "db-creds", "default",
		nil, map[string]interface{}{"type": "Opaque"}, "# secret")

	group := &types.ResourceGroup{
		Name:      "myapp",
		Resources: []*types.ProcessedResource{secret},
	}

	gen := NewUniversalGenerator()
	config := gen.buildServiceConfig(group)

	// Secrets always use nested structure
	if _, ok := config["secrets"]; !ok {
		t.Error("expected 'secrets' key for Secret (always nested)")
	}
}

// ============================================================
// kindToValuesKey Tests
// ============================================================

func TestKindToValuesKey(t *testing.T) {
	tests := []struct {
		kind     string
		expected string
	}{
		{"Deployment", "deployment"},
		{"StatefulSet", "statefulSet"},
		{"DaemonSet", "daemonSet"},
		{"Service", "service"},
		{"ConfigMap", "configMap"},
		{"PersistentVolumeClaim", "pvc"},
		{"Ingress", "ingress"},
		{"", "resource"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			result := kindToValuesKey(tt.kind)
			if result != tt.expected {
				t.Errorf("kindToValuesKey(%q) = %q, want %q", tt.kind, result, tt.expected)
			}
		})
	}
}

// ============================================================
// pluralizeKind Tests
// ============================================================

func TestPluralizeKind(t *testing.T) {
	tests := []struct {
		kind     string
		expected string
	}{
		{"Ingress", "ingresses"},
		{"Service", "services"},
		{"ConfigMap", "configMaps"},
		{"Secret", "secrets"},
		{"ServiceAccount", "serviceAccounts"},
		{"Deployment", "deployments"},
		{"StatefulSet", "statefulSets"},
		{"DaemonSet", "daemonSets"},
		{"PersistentVolumeClaim", "persistentVolumeClaims"},
		{"Role", "roles"},
		{"RoleBinding", "roleBindings"},
		{"ClusterRole", "clusterRoles"},
		{"ClusterRoleBinding", "clusterRoleBindings"},
		{"CustomResource", "CustomResources"}, // default: add 's'
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			result := pluralizeKind(tt.kind)
			if result != tt.expected {
				t.Errorf("pluralizeKind(%q) = %q, want %q", tt.kind, result, tt.expected)
			}
		})
	}
}

// ============================================================
// sanitizeName Tests
// ============================================================

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "myapp", "myapp"},
		{"hyphen", "my-app", "myApp"},
		{"underscore", "my_app", "myApp"},
		{"dot", "my.app", "myApp"},
		{"uppercase-first", "MyApp", "myApp"},
		{"mixed", "my-app_config.v2", "myAppConfigV2"},
		{"empty", "", "config"},
		{"special-chars-only", "---", "config"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// ============================================================
// nameForComponent Tests
// ============================================================

func TestNameForComponent_Deployment(t *testing.T) {
	deploy := makeProcessedResource("Deployment", "my-web", "default", nil)
	svc := makeProcessedResource("Service", "my-web-svc", "default", nil)

	name := nameForComponent([]*types.ProcessedResource{svc, deploy})
	if name != "my-web" {
		t.Errorf("expected 'my-web' (Deployment name), got '%s'", name)
	}
}

func TestNameForComponent_StatefulSet(t *testing.T) {
	sts := makeProcessedResource("StatefulSet", "my-db", "default", nil)
	svc := makeProcessedResource("Service", "my-db-svc", "default", nil)

	name := nameForComponent([]*types.ProcessedResource{svc, sts})
	if name != "my-db" {
		t.Errorf("expected 'my-db' (StatefulSet name), got '%s'", name)
	}
}

func TestNameForComponent_DaemonSet(t *testing.T) {
	ds := makeProcessedResource("DaemonSet", "log-agent", "default", nil)
	name := nameForComponent([]*types.ProcessedResource{ds})
	if name != "log-agent" {
		t.Errorf("expected 'log-agent', got '%s'", name)
	}
}

func TestNameForComponent_NoWorkload(t *testing.T) {
	svc := makeProcessedResource("Service", "orphan-svc", "default", nil)
	cm := makeProcessedResource("ConfigMap", "orphan-config", "default", nil)

	name := nameForComponent([]*types.ProcessedResource{svc, cm})
	// Should fallback to first resource name
	if name != "orphan-svc" {
		t.Errorf("expected 'orphan-svc' (first resource), got '%s'", name)
	}
}

func TestNameForComponent_Empty(t *testing.T) {
	name := nameForComponent([]*types.ProcessedResource{})
	if name != "unnamed" {
		t.Errorf("expected 'unnamed' for empty resources, got '%s'", name)
	}
}

// ============================================================
// rewriteTemplateForSeparateMode Tests
// ============================================================

func TestRewriteTemplateForSeparateMode_BasicRewrite(t *testing.T) {
	content := `{{- $svc := .Values.services.frontend }}
replicas: {{ $svc.replicaCount }}`

	result := rewriteTemplateForSeparateMode(content, "frontend")

	expected := `{{- $svc := .Values }}
replicas: {{ $svc.replicaCount }}`

	if result != expected {
		t.Errorf("rewrite mismatch:\ngot:\n%s\nwant:\n%s", result, expected)
	}
}

func TestRewriteTemplateForSeparateMode_EmptyServiceName(t *testing.T) {
	content := "some content"
	result := rewriteTemplateForSeparateMode(content, "")
	if result != content {
		t.Error("content should be unchanged for empty service name")
	}
}

func TestRewriteTemplateForSeparateMode_NoMatch(t *testing.T) {
	content := "replicas: {{ .Values.replicaCount }}"
	result := rewriteTemplateForSeparateMode(content, "frontend")
	if result != content {
		t.Error("content should be unchanged when no pattern matches")
	}
}

// ============================================================
// inferType Tests
// ============================================================

func TestInferType(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"string", "hello", "string"},
		{"bool", true, "boolean"},
		{"int", 42, "integer"},
		{"int32", int32(42), "integer"},
		{"int64", int64(42), "integer"},
		{"float32", float32(3.14), "number"},
		{"float64", 3.14, "number"},
		{"nil", nil, "string"},                   // default
		{"slice", []string{"a"}, "string"},       // default
		{"map", map[string]string{}, "string"},   // default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inferType(tt.input)
			if result != tt.expected {
				t.Errorf("inferType(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// ============================================================
// OpenAPI Schema additional tests for coverage
// ============================================================

func TestGenerateOpenAPISchema_EmptyArray(t *testing.T) {
	result := GenerateOpenAPISchema(map[string]interface{}{
		"tags": []interface{}{},
	})

	if !strings.Contains(result, "type: array") {
		t.Error("Expected 'type: array'")
	}
	// Empty array should default to string items
	if !strings.Contains(result, "type: string") {
		t.Error("Expected items type: string for empty array")
	}
}

func TestGenerateOpenAPISchema_ArrayWithInts(t *testing.T) {
	result := GenerateOpenAPISchema(map[string]interface{}{
		"ports": []interface{}{int64(80), int64(443)},
	})

	if !strings.Contains(result, "type: array") {
		t.Error("Expected 'type: array'")
	}
	if !strings.Contains(result, "type: integer") {
		t.Error("Expected items type: integer")
	}
}

func TestGenerateOpenAPISchema_FloatField(t *testing.T) {
	result := GenerateOpenAPISchema(map[string]interface{}{
		"ratio": 0.75,
	})

	if !strings.Contains(result, "type: number") {
		t.Error("Expected 'type: number' for float field")
	}
}

func TestGenerateOpenAPISchema_DeeplyNested(t *testing.T) {
	result := GenerateOpenAPISchema(map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"value": "deep",
			},
		},
	})

	objectCount := strings.Count(result, "type: object")
	if objectCount < 3 {
		t.Errorf("Expected at least 3 'type: object' for deeply nested, got %d", objectCount)
	}
}

// ============================================================
// BaseGenerator Tests
// ============================================================

func TestBaseGenerator_Mode(t *testing.T) {
	bg := NewBaseGenerator(types.OutputModeSeparate)
	if bg.Mode() != types.OutputModeSeparate {
		t.Errorf("expected mode %s, got %s", types.OutputModeSeparate, bg.Mode())
	}
}
