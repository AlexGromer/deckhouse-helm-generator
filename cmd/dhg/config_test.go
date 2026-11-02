package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "dhg.yaml")
	if err := os.WriteFile(cfg, []byte("chart-name: from-config\nmode: separate\nwith: [flux, policies]\ninclude-schema: true\nspot-grace-period: 30\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newGenerateCmd()
	if err := cmd.ParseFlags([]string{"--mode", "umbrella"}); err != nil {
		t.Fatal(err)
	}
	if err := applyConfigFile(cmd, cfg); err != nil {
		t.Fatal(err)
	}
	get := func(name string) string { return cmd.Flags().Lookup(name).Value.String() }

	if get("chart-name") != "from-config" {
		t.Errorf("chart-name = %q", get("chart-name"))
	}
	if get("mode") != "umbrella" {
		t.Errorf("command-line flag must win over the config file, mode = %q", get("mode"))
	}
	if get("with") != "[flux,policies]" || get("include-schema") != "true" || get("spot-grace-period") != "30" {
		t.Errorf("with=%q include-schema=%q spot-grace-period=%q", get("with"), get("include-schema"), get("spot-grace-period"))
	}
}

func TestApplyConfigFile_Errors(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"unknown key":     "chartname: typo\n",
		"bad yaml":        "mode: [unclosed\n",
		"list for scalar": "mode: [a, b]\n",
	} {
		cfg := filepath.Join(dir, strings.ReplaceAll(name, " ", "-")+".yaml")
		if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := applyConfigFile(newGenerateCmd(), cfg); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if err := applyConfigFile(newGenerateCmd(), filepath.Join(dir, "missing.yaml")); err == nil {
		t.Error("an explicit missing config file must be an error")
	}
}

func TestApplyConfigFile_DefaultIsOptional(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := applyConfigFile(newGenerateCmd(), ""); err != nil {
		t.Errorf("missing .dhg.yaml must be fine: %v", err)
	}
	if err := os.WriteFile(defaultConfigFile, []byte("chart-name: auto\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newGenerateCmd()
	if err := applyConfigFile(cmd, ""); err != nil || cmd.Flags().Lookup("chart-name").Value.String() != "auto" {
		t.Errorf(".dhg.yaml not loaded: %v", err)
	}
}

// libraryRefChartYAML runs `dhg generate` on a small manifest set with the
// given extra arguments and returns the Chart.yaml of the frontend chart.
func libraryRefChartYAML(t *testing.T, cfgContent string, extra ...string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	manifests := filepath.Join(dir, "m")
	if err := os.MkdirAll(manifests, 0o755); err != nil {
		t.Fatal(err)
	}
	app := "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: frontend\n  labels:\n    app.kubernetes.io/name: frontend\nspec:\n  replicas: 1\n  selector:\n    matchLabels:\n      app.kubernetes.io/name: frontend\n  template:\n    metadata:\n      labels:\n        app.kubernetes.io/name: frontend\n    spec:\n      containers:\n        - name: web\n          image: nginx:1.25\n"
	if err := os.WriteFile(filepath.Join(manifests, "app.yaml"), []byte(app), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "dhg.yaml")
	if err := os.WriteFile(cfg, []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	args := append([]string{"generate", "--config", cfg, "-f", manifests, "-o", out, "--chart-name", "shop"}, extra...)
	if _, err := executeCmd(t, args...); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(out, "frontend", "Chart.yaml"))
	if err != nil {
		t.Fatalf("frontend chart not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "frontend", "charts", "library")); err == nil {
		t.Error("the library must not be vendored when library-ref is set")
	}
	return string(data), nil
}

func TestGenerate_LibraryRefFromConfigFile(t *testing.T) {
	cfg := "mode: library\nlibrary-ref: oci://registry.example.com/platform/platform-lib\nlibrary-version: \"1.4.2\"\n"
	chartYAML, err := libraryRefChartYAML(t, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"name: platform-lib", "version: 1.4.2", "repository: oci://registry.example.com/platform"} {
		if !strings.Contains(chartYAML, want) {
			t.Errorf("Chart.yaml missing %q:\n%s", want, chartYAML)
		}
	}
}

func TestGenerate_LibraryRefFlagOverridesConfigFile(t *testing.T) {
	cfg := "mode: library\nlibrary-ref: oci://registry.example.com/platform/platform-lib\nlibrary-version: \"1.4.2\"\n"
	chartYAML, err := libraryRefChartYAML(t, cfg,
		"--library-ref", "oci://registry.other.example/base/common", "--library-version", "^2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"name: common", "version: ^2.0.0", "repository: oci://registry.other.example/base"} {
		if !strings.Contains(chartYAML, want) {
			t.Errorf("Chart.yaml missing %q:\n%s", want, chartYAML)
		}
	}
	if strings.Contains(chartYAML, "platform-lib") || strings.Contains(chartYAML, "1.4.2") {
		t.Errorf("config values must be overridden by the flags:\n%s", chartYAML)
	}
}

func TestGenerate_LibraryRefErrors(t *testing.T) {
	cases := map[string]struct {
		cfg     string
		wantErr string
	}{
		"version missing": {"mode: library\nlibrary-ref: oci://registry.example.com/p/lib\n", "--library-version"},
		"version alone":   {"mode: library\nlibrary-version: \"1.0.0\"\n", "--library-ref"},
		"wrong mode":      {"mode: separate\nlibrary-ref: oci://registry.example.com/p/lib\nlibrary-version: \"1.0.0\"\n", "--mode library"},
		"not oci":         {"mode: library\nlibrary-ref: https://example.com/lib\nlibrary-version: \"1.0.0\"\n", "oci://"},
	}
	for name, tc := range cases {
		_, err := libraryRefChartYAML(t, tc.cfg)
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: error = %v, want one containing %q", name, err, tc.wantErr)
		}
	}
}

// synth-profile comes from .dhg.yaml like any flag.
func TestApplyConfigFileSynthProfile(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "dhg.yaml")
	if err := os.WriteFile(cfg, []byte("source: source\nsynth-profile: ./platform.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newGenerateCmd()
	if err := applyConfigFile(cmd, cfg); err != nil {
		t.Fatal(err)
	}
	if got := cmd.Flags().Lookup("synth-profile").Value.String(); got != "./platform.yaml" {
		t.Errorf("synth-profile = %q", got)
	}
}
