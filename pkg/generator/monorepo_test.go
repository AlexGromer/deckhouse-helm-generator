package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
	"sigs.k8s.io/yaml"
)

// Ensure types import is used even if a test only constructs charts via makeChart.
var _ *types.GeneratedChart

// ============================================================
// Test 1: Single chart — layout has 1 chart, RootDir set to chartName
// ============================================================

func TestMonorepo_SingleChart_ValidLayout(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	layout, err := GenerateMonorepoLayout([]*types.GeneratedChart{chart}, "myproject")
	if err != nil {
		t.Fatalf("GenerateMonorepoLayout returned unexpected error: %v", err)
	}

	if layout == nil {
		t.Fatal("expected non-nil MonorepoLayout, got nil")
	}

	if len(layout.Charts) != 1 {
		t.Errorf("expected 1 chart in layout, got %d", len(layout.Charts))
	}

	if layout.RootDir != "myproject" {
		t.Errorf("expected RootDir to be 'myproject', got %q", layout.RootDir)
	}
}

// ============================================================
// Test 2: Three charts — layout.Charts has 3 entries
// ============================================================

func TestMonorepo_ThreeCharts_AllPresent(t *testing.T) {
	charts := []*types.GeneratedChart{
		makeChart("frontend", map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: frontend\n",
		}),
		makeChart("backend", map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: backend\n",
		}),
		makeChart("worker", map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: worker\n",
		}),
	}

	layout, err := GenerateMonorepoLayout(charts, "myproject")
	if err != nil {
		t.Fatalf("GenerateMonorepoLayout returned unexpected error: %v", err)
	}

	if len(layout.Charts) != 3 {
		t.Errorf("expected 3 charts in layout, got %d", len(layout.Charts))
	}
}

// ============================================================
// Test 3: Makefile contains all required targets
// ============================================================

func TestMonorepo_Makefile_HasAllTargets(t *testing.T) {
	charts := []*types.GeneratedChart{
		makeChart("myapp", map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
		}),
	}

	layout, err := GenerateMonorepoLayout(charts, "myproject")
	if err != nil {
		t.Fatalf("GenerateMonorepoLayout returned unexpected error: %v", err)
	}

	requiredTargets := []string{
		"lint-all",
		"test-all",
		"package-all",
		"template-all",
		"deps-all",
	}

	for _, target := range requiredTargets {
		if !strings.Contains(layout.Makefile, target) {
			t.Errorf("Makefile missing required target %q", target)
		}
	}
}

// ============================================================
// Test 4: Makefile references all chart names
// ============================================================

func TestMonorepo_Makefile_ReferencesChartNames(t *testing.T) {
	charts := []*types.GeneratedChart{
		makeChart("frontend", map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: frontend\n",
		}),
		makeChart("backend", map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: backend\n",
		}),
	}

	layout, err := GenerateMonorepoLayout(charts, "myproject")
	if err != nil {
		t.Fatalf("GenerateMonorepoLayout returned unexpected error: %v", err)
	}

	if !strings.Contains(layout.Makefile, "frontend") {
		t.Error("Makefile must reference chart name 'frontend'")
	}
	if !strings.Contains(layout.Makefile, "backend") {
		t.Error("Makefile must reference chart name 'backend'")
	}
}

// ============================================================
// Test 5: CTConfig is valid YAML with chart-dirs containing "charts/"
// ============================================================

func TestMonorepo_CTConfig_ValidYAML(t *testing.T) {
	charts := []*types.GeneratedChart{
		makeChart("myapp", map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
		}),
	}

	layout, err := GenerateMonorepoLayout(charts, "myproject")
	if err != nil {
		t.Fatalf("GenerateMonorepoLayout returned unexpected error: %v", err)
	}

	if layout.CTConfig == "" {
		t.Fatal("expected non-empty CTConfig, got empty string")
	}

	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(layout.CTConfig), &parsed); err != nil {
		t.Fatalf("CTConfig is not valid YAML: %v\ncontent:\n%s", err, layout.CTConfig)
	}

	chartDirs, ok := parsed["chart-dirs"]
	if !ok {
		t.Fatal("CTConfig YAML missing required key 'chart-dirs'")
	}

	dirsList, ok := chartDirs.([]interface{})
	if !ok {
		t.Fatalf("CTConfig 'chart-dirs' must be a list, got %T", chartDirs)
	}

	found := false
	for _, d := range dirsList {
		s, ok := d.(string)
		if !ok {
			continue
		}
		if strings.Contains(s, "charts/") || s == "charts" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("CTConfig 'chart-dirs' must contain an entry with 'charts/', got: %v", dirsList)
	}
}

// ============================================================
// Test 6: CTConfig has chart-dirs key pointing to charts/
// ============================================================

func TestMonorepo_CTConfig_HasChartDirs(t *testing.T) {
	charts := []*types.GeneratedChart{
		makeChart("myapp", map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
		}),
	}

	layout, err := GenerateMonorepoLayout(charts, "myproject")
	if err != nil {
		t.Fatalf("GenerateMonorepoLayout returned unexpected error: %v", err)
	}

	// Raw string check as a secondary guard independent of YAML parsing
	if !strings.Contains(layout.CTConfig, "chart-dirs") {
		t.Error("CTConfig must contain the key 'chart-dirs'")
	}
	if !strings.Contains(layout.CTConfig, "charts") {
		t.Error("CTConfig 'chart-dirs' must reference the 'charts' directory")
	}
}

// ============================================================
// Test 7: HelmIgnore field is non-empty
// ============================================================

func TestMonorepo_HelmIgnore_Present(t *testing.T) {
	charts := []*types.GeneratedChart{
		makeChart("myapp", map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
		}),
	}

	layout, err := GenerateMonorepoLayout(charts, "myproject")
	if err != nil {
		t.Fatalf("GenerateMonorepoLayout returned unexpected error: %v", err)
	}

	if strings.TrimSpace(layout.HelmIgnore) == "" {
		t.Error("expected non-empty HelmIgnore field in MonorepoLayout")
	}
}

// ============================================================
// Test 8: Empty charts slice returns an error
// ============================================================

func TestMonorepo_ChartsSliceIsCopy(t *testing.T) {
	original := []*types.GeneratedChart{
		makeChart("frontend", map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: frontend\n",
		}),
		makeChart("backend", map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: backend\n",
		}),
	}

	layout, err := GenerateMonorepoLayout(original, "myproject")
	if err != nil {
		t.Fatalf("GenerateMonorepoLayout returned unexpected error: %v", err)
	}

	// Mutate the returned layout's Charts slice.
	layout.Charts[0] = nil

	// Original slice must be unchanged.
	if original[0] == nil {
		t.Error("mutating layout.Charts affected the original slice — Charts must be a defensive copy")
	}
}

func TestMonorepo_EmptyCharts_ReturnsError(t *testing.T) {
	layout, err := GenerateMonorepoLayout([]*types.GeneratedChart{}, "myproject")
	if err == nil {
		t.Error("expected error when charts slice is empty, got nil")
	}
	if layout != nil {
		t.Errorf("expected nil layout on error, got non-nil: %+v", layout)
	}
}

// ============================================================
// Test 9: Chart name with spaces is sanitized in Makefile (lowercase, no spaces)
// ============================================================

func TestMonorepo_ChartNamesSanitized(t *testing.T) {
	chartNames := []string{"My App"}
	makefile := generateMonorepoMakefile(chartNames)

	// The raw unsanitized form must NOT appear as a make target variable
	if strings.Contains(makefile, "My App") {
		t.Error("Makefile must not contain unsanitized chart name 'My App' (with capital and space)")
	}

	// The sanitized form must appear — lowercase, no spaces
	if !strings.Contains(makefile, "my-app") && !strings.Contains(makefile, "myapp") {
		t.Error("Makefile must contain sanitized chart name (lowercase, no spaces) derived from 'My App'")
	}
}

// ============================================================
// Test: sanitizeChartName strips semicolons (injection vector)
// ============================================================

func TestSanitizeChartName_StripsSemicolon(t *testing.T) {
	got := sanitizeChartName("my;chart")
	if got != "mychart" {
		t.Errorf("expected %q, got %q", "mychart", got)
	}
}

// ============================================================
// Test: sanitizeChartName strips $() command substitution
// ============================================================

func TestSanitizeChartName_StripsDollarParen(t *testing.T) {
	got := sanitizeChartName("my$(cmd)chart")
	if got != "mycmdchart" {
		t.Errorf("expected %q, got %q", "mycmdchart", got)
	}
}

// ============================================================
// Test: sanitizeChartName strips tabs
// ============================================================

func TestSanitizeChartName_StripsTabs(t *testing.T) {
	got := sanitizeChartName("my\tchart")
	if got != "mychart" {
		t.Errorf("expected %q, got %q", "mychart", got)
	}
}

// ============================================================
// Test: sanitizeChartName preserves valid names
// ============================================================

func TestSanitizeChartName_PreservesValid(t *testing.T) {
	got := sanitizeChartName("my-chart-123")
	if got != "my-chart-123" {
		t.Errorf("expected %q, got %q", "my-chart-123", got)
	}
}

// ============================================================
// Test 10: RootDir matches chartName argument
// ============================================================

func TestMonorepo_RootDirMatchesChartName(t *testing.T) {
	charts := []*types.GeneratedChart{
		makeChart("myapp", map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
		}),
	}

	layout, err := GenerateMonorepoLayout(charts, "myproject")
	if err != nil {
		t.Fatalf("GenerateMonorepoLayout returned unexpected error: %v", err)
	}

	if layout.RootDir != "myproject" {
		t.Errorf("expected RootDir == 'myproject', got %q", layout.RootDir)
	}
}

// ============================================================
// Test HS-4a: CI workflow output is safe after HS-2 sanitization fix
// ============================================================

func TestMonorepoCIWorkflow_NoInjection(t *testing.T) {
	// Chart name containing shell injection characters
	chart := makeChart("my;echo pwned", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: test\n",
	})

	layout, err := GenerateMonorepoLayout([]*types.GeneratedChart{chart}, "injtest")
	if err != nil {
		t.Fatalf("GenerateMonorepoLayout returned unexpected error: %v", err)
	}

	// The CI workflow must NOT contain the raw injection payload
	if strings.Contains(layout.CIWorkflow, ";") {
		t.Errorf("CI workflow contains semicolon — injection not sanitized:\n%s", layout.CIWorkflow)
	}
	if strings.Contains(layout.CIWorkflow, "echo pwned") {
		t.Errorf("CI workflow contains raw injection payload 'echo pwned':\n%s", layout.CIWorkflow)
	}
	if strings.Contains(layout.CIWorkflow, "my;echo") {
		t.Errorf("CI workflow contains unsanitized chart name:\n%s", layout.CIWorkflow)
	}

	// The sanitized name should be present instead
	if !strings.Contains(layout.CIWorkflow, "myecho") {
		t.Errorf("CI workflow missing sanitized chart name 'myecho':\n%s", layout.CIWorkflow)
	}
}

// ============================================================
// Test HS-4b: CI workflow produces valid helm lint line for normal chart
// ============================================================

func TestMonorepoCIWorkflow_ValidOutput(t *testing.T) {
	chart := makeChart("my-service", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: my-service\n",
	})

	layout, err := GenerateMonorepoLayout([]*types.GeneratedChart{chart}, "proj")
	if err != nil {
		t.Fatalf("GenerateMonorepoLayout returned unexpected error: %v", err)
	}

	expectedLine := "run: helm lint charts/my-service"
	if !strings.Contains(layout.CIWorkflow, expectedLine) {
		t.Errorf("CI workflow missing expected line %q:\n%s", expectedLine, layout.CIWorkflow)
	}

	expectedName := "name: lint-my-service"
	if !strings.Contains(layout.CIWorkflow, expectedName) {
		t.Errorf("CI workflow missing expected step name %q:\n%s", expectedName, layout.CIWorkflow)
	}
}
