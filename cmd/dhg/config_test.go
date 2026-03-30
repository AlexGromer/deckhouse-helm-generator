package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// writeTempYAML writes content to a temporary file and returns its path.
func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, ".dhg.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTempYAML: %v", err)
	}
	return p
}

// ── Test 1: LoadConfig — valid YAML file ──────────────────────────────────────

func TestLoadConfig_ValidYAML(t *testing.T) {
	yaml := `
outputDir: ./charts
chartName: my-app
mode: universal
namespace: production
includeTests: true
includeSchema: false
secretStrategy: env
templateDir: ./templates
plugins:
  - /usr/local/bin/dhg-plugin-a
  - /usr/local/bin/dhg-plugin-b
`
	path := writeTempYAML(t, yaml)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil DHGConfig")
	}

	if cfg.OutputDir != "./charts" {
		t.Errorf("OutputDir = %q, want %q", cfg.OutputDir, "./charts")
	}
	if cfg.ChartName != "my-app" {
		t.Errorf("ChartName = %q, want %q", cfg.ChartName, "my-app")
	}
	if cfg.Mode != "universal" {
		t.Errorf("Mode = %q, want %q", cfg.Mode, "universal")
	}
	if cfg.Namespace != "production" {
		t.Errorf("Namespace = %q, want %q", cfg.Namespace, "production")
	}
	if !cfg.IncludeTests {
		t.Error("IncludeTests should be true")
	}
	if cfg.IncludeSchema {
		t.Error("IncludeSchema should be false")
	}
	if cfg.SecretStrategy != "env" {
		t.Errorf("SecretStrategy = %q, want %q", cfg.SecretStrategy, "env")
	}
}

// ── Test 2: LoadConfig — missing file returns error ───────────────────────────

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/.dhg.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// ── Test 3: MergeConfigWithFlags — flags override config fields ───────────────

func TestMergeConfigWithFlags_FlagsOverrideConfig(t *testing.T) {
	cfg := &DHGConfig{
		OutputDir: "./old-charts",
		ChartName: "old-name",
		Mode:      "separate",
		Namespace: "default",
	}
	flags := map[string]interface{}{
		"outputDir": "./new-charts",
		"chartName": "new-name",
	}

	merged := MergeConfigWithFlags(cfg, flags)
	if merged == nil {
		t.Fatal("expected non-nil result")
	}
	if merged.OutputDir != "./new-charts" {
		t.Errorf("OutputDir = %q, want %q", merged.OutputDir, "./new-charts")
	}
	if merged.ChartName != "new-name" {
		t.Errorf("ChartName = %q, want %q", merged.ChartName, "new-name")
	}
	// Non-overridden fields must retain config values.
	if merged.Mode != "separate" {
		t.Errorf("Mode = %q, want %q (should not be overridden)", merged.Mode, "separate")
	}
	if merged.Namespace != "default" {
		t.Errorf("Namespace = %q, want %q (should not be overridden)", merged.Namespace, "default")
	}
}

// ── Test 4: LoadConfig — empty YAML produces zero-value struct ────────────────

func TestLoadConfig_EmptyYAML(t *testing.T) {
	path := writeTempYAML(t, "")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error for empty config: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil DHGConfig even for empty file")
	}
	// All string fields should be empty / zero.
	if cfg.OutputDir != "" {
		t.Errorf("OutputDir = %q, want empty string", cfg.OutputDir)
	}
	if cfg.ChartName != "" {
		t.Errorf("ChartName = %q, want empty string", cfg.ChartName)
	}
}

// ── Test 5: LoadConfig — all fields populated ────────────────────────────────

func TestLoadConfig_AllFieldsPopulated(t *testing.T) {
	yaml := `
outputDir: /charts
chartName: full-app
mode: library
namespace: kube-system
includeTests: true
includeSchema: true
secretStrategy: vault
templateDir: /custom/templates
plugins:
  - /opt/plugins/plugin-x
`
	path := writeTempYAML(t, yaml)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []struct {
		field string
		got   interface{}
		want  interface{}
	}{
		{"OutputDir", cfg.OutputDir, "/charts"},
		{"ChartName", cfg.ChartName, "full-app"},
		{"Mode", cfg.Mode, "library"},
		{"Namespace", cfg.Namespace, "kube-system"},
		{"IncludeTests", cfg.IncludeTests, true},
		{"IncludeSchema", cfg.IncludeSchema, true},
		{"SecretStrategy", cfg.SecretStrategy, "vault"},
		{"TemplateDir", cfg.TemplateDir, "/custom/templates"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.field, c.got, c.want)
		}
	}
	if len(cfg.Plugins) != 1 || cfg.Plugins[0] != "/opt/plugins/plugin-x" {
		t.Errorf("Plugins = %v, want [\"/opt/plugins/plugin-x\"]", cfg.Plugins)
	}
}

// ── Test 6: LoadConfig — invalid YAML returns error ──────────────────────────

func TestLoadConfig_InvalidYAML(t *testing.T) {
	// Use YAML with a tab character (invalid in YAML) to force a parse error.
	invalidYAML := "outputDir: valid\n\tinvalidKey: [broken"
	path := writeTempYAML(t, invalidYAML)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected parse error for invalid YAML, got nil")
	}
}

// ── Test 7: MergeConfigWithFlags — nil flags returns original config ──────────

func TestMergeConfigWithFlags_NilFlags(t *testing.T) {
	cfg := &DHGConfig{
		OutputDir: "./charts",
		ChartName: "my-app",
		Mode:      "universal",
	}

	merged := MergeConfigWithFlags(cfg, nil)
	if merged == nil {
		t.Fatal("expected non-nil result")
	}
	if merged.OutputDir != cfg.OutputDir {
		t.Errorf("OutputDir = %q, want %q", merged.OutputDir, cfg.OutputDir)
	}
	if merged.ChartName != cfg.ChartName {
		t.Errorf("ChartName = %q, want %q", merged.ChartName, cfg.ChartName)
	}
	if merged.Mode != cfg.Mode {
		t.Errorf("Mode = %q, want %q", merged.Mode, cfg.Mode)
	}
}

// ── Test 8: MergeConfigWithFlags — partial config + flags ────────────────────

func TestMergeConfigWithFlags_PartialConfigAndFlags(t *testing.T) {
	cfg := &DHGConfig{
		OutputDir: "./charts",
		// ChartName intentionally empty — should come from flags.
	}
	flags := map[string]interface{}{
		"chartName": "from-flag",
		"namespace": "staging",
	}

	merged := MergeConfigWithFlags(cfg, flags)
	if merged.ChartName != "from-flag" {
		t.Errorf("ChartName = %q, want %q", merged.ChartName, "from-flag")
	}
	if merged.Namespace != "staging" {
		t.Errorf("Namespace = %q, want %q", merged.Namespace, "staging")
	}
	if merged.OutputDir != "./charts" {
		t.Errorf("OutputDir = %q, want %q (config value preserved)", merged.OutputDir, "./charts")
	}
}

// ── Test 9: LoadConfig — TemplateDir field is loaded ─────────────────────────

func TestLoadConfig_TemplateDirField(t *testing.T) {
	yaml := "templateDir: /my/custom/templates\n"
	path := writeTempYAML(t, yaml)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TemplateDir != "/my/custom/templates" {
		t.Errorf("TemplateDir = %q, want %q", cfg.TemplateDir, "/my/custom/templates")
	}
}

// ── Test 10: LoadConfig — Plugins list parsed correctly ───────────────────────

func TestLoadConfig_PluginsList(t *testing.T) {
	yaml := `
plugins:
  - /usr/local/bin/plugin-one
  - /usr/local/bin/plugin-two
  - /usr/local/bin/plugin-three
`
	path := writeTempYAML(t, yaml)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Plugins) != 3 {
		t.Fatalf("expected 3 plugins, got %d", len(cfg.Plugins))
	}

	expected := []string{
		"/usr/local/bin/plugin-one",
		"/usr/local/bin/plugin-two",
		"/usr/local/bin/plugin-three",
	}
	for i, want := range expected {
		if cfg.Plugins[i] != want {
			t.Errorf("Plugins[%d] = %q, want %q", i, cfg.Plugins[i], want)
		}
	}
}
