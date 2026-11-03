package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --known-keys keeps the variables a platform provides out of the chart;
// both flags belong to --source source.
func TestGenerateCmd_KnownKeys(t *testing.T) {
	known := filepath.Join(t.TempDir(), "known.txt")
	if err := os.WriteFile(known, []byte("# from the platform\nDB_URL\nvault:DB_PASSWORD\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	project := "../../pkg/synth/testdata/orders"

	out := t.TempDir()
	if _, err := executeCmd(t, "generate", "-s", "source", "-f", project, "--known-keys", known, "-o", out, "--chart-name", "orders"); err != nil {
		t.Fatalf("generate: %v", err)
	}
	var chart strings.Builder
	_ = filepath.WalkDir(filepath.Join(out, "orders"), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := os.ReadFile(path)
			chart.WriteString(string(data))
		}
		return nil
	})
	if strings.Contains(chart.String(), "DB_URL") || strings.Contains(chart.String(), "DB_PASSWORD") {
		t.Error("DB_URL and DB_PASSWORD are known keys and must not be variables of the chart")
	}
	if !strings.Contains(chart.String(), "DB_LOGIN") {
		t.Error("DB_LOGIN is not known and must stay a variable")
	}
	synthesis, err := os.ReadFile(filepath.Join(out, "SYNTHESIS.md"))
	if err != nil || !strings.Contains(string(synthesis), "1 variable comes from vault (DB_PASSWORD)") ||
		!strings.Contains(string(synthesis), "1 variable comes from known.txt (DB_URL)") {
		t.Errorf("SYNTHESIS.md lacks the notes about known keys: %v\n%s", err, synthesis)
	}

	for _, args := range [][]string{
		{"generate", "-f", exampleFullStack, "--known-keys", known},
		{"generate", "-f", exampleFullStack, "--known-keys-from-cluster"},
	} {
		_, err := executeCmd(t, append(args, "-o", t.TempDir(), "--chart-name", "x")...)
		if err == nil || !strings.Contains(err.Error(), "--source source") {
			t.Errorf("%v with a file source: %v", args[3], err)
		}
	}
	if _, err := executeCmd(t, "generate", "-s", "source", "-f", project, "--known-keys-from-cluster", "-o", t.TempDir(), "--chart-name", "x"); err == nil ||
		!strings.Contains(err.Error(), "--synth-profile") {
		t.Errorf("--known-keys-from-cluster without a profile: %v", err)
	}
	if _, err := executeCmd(t, "generate", "-s", "source", "-f", project, "--known-keys", filepath.Join(t.TempDir(), "none.txt"), "-o", t.TempDir(), "--chart-name", "x"); err == nil {
		t.Error("a missing known keys file must fail")
	}
}

func TestApplyConfigFileKnownKeys(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "dhg.yaml")
	if err := os.WriteFile(cfg, []byte("source: source\nknown-keys: ./known.txt\nknown-keys-from-cluster: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newGenerateCmd()
	if err := applyConfigFile(cmd, cfg); err != nil {
		t.Fatal(err)
	}
	if got := cmd.Flags().Lookup("known-keys").Value.String(); got != "./known.txt" {
		t.Errorf("known-keys = %q", got)
	}
	if got := cmd.Flags().Lookup("known-keys-from-cluster").Value.String(); got != "true" {
		t.Errorf("known-keys-from-cluster = %q", got)
	}
}
