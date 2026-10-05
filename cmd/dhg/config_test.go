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
	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(t.TempDir()); err != nil {
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
