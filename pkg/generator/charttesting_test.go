package generator

// charttesting_test.go — TDD tests for Task 6.0.7
// Tests for GenerateChartTestingConfig and InjectChartTestingConfig.
// These tests define the contract; the implementation must satisfy them.

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Test 1: GenerateChartTestingConfig produces ct.yaml in ConfigFiles
// ============================================================

func TestGenerateChartTestingConfig_CTYamlGenerated(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	opts := ChartTestingOptions{
		RemoteRepo:     "https://charts.example.com",
		ChartDirs:      []string{"charts/"},
		ExcludedCharts: nil,
		HelmExtraArgs:  "",
		Upgrade:        false,
	}

	result := GenerateChartTestingConfig(chart, opts)
	if result == nil {
		t.Fatal("expected non-nil ChartTestingResult, got nil")
	}

	if len(result.ConfigFiles) == 0 {
		t.Fatal("expected ConfigFiles to be non-empty")
	}

	// ct.yaml must be present under the well-known key.
	ctYaml, ok := result.ConfigFiles["ct.yaml"]
	if !ok {
		t.Fatal("expected ConfigFiles to contain 'ct.yaml' key")
	}
	if strings.TrimSpace(ctYaml) == "" {
		t.Error("ct.yaml content must not be empty")
	}
}

// ============================================================
// Test 2: ct.yaml contains chart-dirs from options
// ============================================================

func TestGenerateChartTestingConfig_ChartDirsInConfig(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	opts := ChartTestingOptions{
		ChartDirs: []string{"charts/", "extra-charts/"},
	}

	result := GenerateChartTestingConfig(chart, opts)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	ctYaml := result.ConfigFiles["ct.yaml"]
	if !strings.Contains(ctYaml, "chart-dirs") {
		t.Error("ct.yaml must contain the 'chart-dirs' key")
	}
	if !strings.Contains(ctYaml, "charts/") {
		t.Error("ct.yaml must reference 'charts/' from ChartDirs option")
	}
	if !strings.Contains(ctYaml, "extra-charts/") {
		t.Error("ct.yaml must reference 'extra-charts/' from ChartDirs option")
	}
}

// ============================================================
// Test 3: ct.yaml contains excluded-charts when set
// ============================================================

func TestGenerateChartTestingConfig_ExcludedCharts(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	opts := ChartTestingOptions{
		ChartDirs:      []string{"charts/"},
		ExcludedCharts: []string{"bad-chart", "legacy-chart"},
	}

	result := GenerateChartTestingConfig(chart, opts)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	ctYaml := result.ConfigFiles["ct.yaml"]
	if !strings.Contains(ctYaml, "excluded-charts") {
		t.Error("ct.yaml must contain 'excluded-charts' key when ExcludedCharts is non-empty")
	}
	if !strings.Contains(ctYaml, "bad-chart") {
		t.Error("ct.yaml must list 'bad-chart' in excluded-charts")
	}
	if !strings.Contains(ctYaml, "legacy-chart") {
		t.Error("ct.yaml must list 'legacy-chart' in excluded-charts")
	}
}

// ============================================================
// Test 4: ct.yaml contains helm-extra-args when HelmExtraArgs is set
// ============================================================

func TestGenerateChartTestingConfig_HelmExtraArgs(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	opts := ChartTestingOptions{
		ChartDirs:     []string{"charts/"},
		HelmExtraArgs: "--timeout 300s --atomic",
	}

	result := GenerateChartTestingConfig(chart, opts)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	ctYaml := result.ConfigFiles["ct.yaml"]
	if !strings.Contains(ctYaml, "helm-extra-args") {
		t.Error("ct.yaml must contain 'helm-extra-args' key when HelmExtraArgs is non-empty")
	}
	if !strings.Contains(ctYaml, "--timeout 300s") {
		t.Error("ct.yaml must include the provided HelmExtraArgs value '--timeout 300s'")
	}
}

// ============================================================
// Test 5: ct.yaml reflects Upgrade=true flag
// ============================================================

func TestGenerateChartTestingConfig_UpgradeFlag(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	opts := ChartTestingOptions{
		ChartDirs: []string{"charts/"},
		Upgrade:   true,
	}

	result := GenerateChartTestingConfig(chart, opts)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	ctYaml := result.ConfigFiles["ct.yaml"]
	// upgrade flag must appear in ct.yaml (e.g., "upgrade: true" or "--upgrade")
	if !strings.Contains(ctYaml, "upgrade") {
		t.Error("ct.yaml must reference 'upgrade' when Upgrade=true")
	}
}

// ============================================================
// Test 6: CISteps contains "ct lint"
// ============================================================

func TestGenerateChartTestingConfig_CIStepsContainCtLint(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	opts := ChartTestingOptions{
		ChartDirs: []string{"charts/"},
	}

	result := GenerateChartTestingConfig(chart, opts)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result.CISteps) == 0 {
		t.Fatal("CISteps must be non-empty")
	}

	foundLint := false
	for _, step := range result.CISteps {
		if strings.Contains(step, "ct lint") {
			foundLint = true
			break
		}
	}
	if !foundLint {
		t.Errorf("CISteps must contain a step with 'ct lint', got: %v", result.CISteps)
	}
}

// ============================================================
// Test 7: CISteps contains "ct install"
// ============================================================

func TestGenerateChartTestingConfig_CIStepsContainCtInstall(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	opts := ChartTestingOptions{
		ChartDirs: []string{"charts/"},
	}

	result := GenerateChartTestingConfig(chart, opts)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	foundInstall := false
	for _, step := range result.CISteps {
		if strings.Contains(step, "ct install") {
			foundInstall = true
			break
		}
	}
	if !foundInstall {
		t.Errorf("CISteps must contain a step with 'ct install', got: %v", result.CISteps)
	}
}

// ============================================================
// Test 8: GenerateChartTestingConfig with nil chart returns nil
// ============================================================

func TestGenerateChartTestingConfig_NilChart(t *testing.T) {
	opts := ChartTestingOptions{
		ChartDirs: []string{"charts/"},
	}

	result := GenerateChartTestingConfig(nil, opts)
	if result != nil {
		t.Errorf("expected nil for nil chart, got non-nil result: %+v", result)
	}
}

// ============================================================
// Test 9: InjectChartTestingConfig follows copy-on-write — original chart unchanged
// ============================================================

func TestInjectChartTestingConfig_CopyOnWrite(t *testing.T) {
	original := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})
	// Record the original ExternalFiles length for comparison.
	originalExtLen := len(original.ExternalFiles)

	opts := ChartTestingOptions{
		ChartDirs: []string{"charts/"},
	}
	ctResult := GenerateChartTestingConfig(original, opts)
	if ctResult == nil {
		t.Fatal("GenerateChartTestingConfig returned nil, cannot test InjectChartTestingConfig")
	}

	updated, injectedCount := InjectChartTestingConfig(original, ctResult)

	// The returned chart must be a different pointer (copy-on-write).
	if updated == original {
		t.Error("InjectChartTestingConfig must return a new chart (copy-on-write), not the original pointer")
	}

	// At least one file must have been injected.
	if injectedCount < 1 {
		t.Errorf("expected at least 1 file injected, got %d", injectedCount)
	}

	// The original chart's ExternalFiles must not have grown.
	if len(original.ExternalFiles) != originalExtLen {
		t.Errorf("original chart.ExternalFiles was mutated: before=%d, after=%d",
			originalExtLen, len(original.ExternalFiles))
	}

	// The updated chart must contain the injected files.
	if len(updated.ExternalFiles) <= originalExtLen {
		t.Errorf("updated chart must have more ExternalFiles than original: original=%d, updated=%d",
			originalExtLen, len(updated.ExternalFiles))
	}
}

// ============================================================
// Test 10: ChartTestingResult.NOTESTxt is non-empty
// ============================================================

func TestGenerateChartTestingConfig_NOTESTxt(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	opts := ChartTestingOptions{
		ChartDirs: []string{"charts/"},
		RemoteRepo: "https://charts.example.com",
	}

	result := GenerateChartTestingConfig(chart, opts)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if strings.TrimSpace(result.NOTESTxt) == "" {
		t.Error("ChartTestingResult.NOTESTxt must be non-empty — it should document how to run ct commands")
	}
}

// ============================================================
// Ensure types import is used.
// ============================================================
var _ *types.GeneratedChart
