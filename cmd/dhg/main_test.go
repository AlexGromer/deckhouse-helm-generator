package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// executeCmd runs the root command with the given arguments, capturing cobra's
// output writer.  Commands that use fmt.Printf directly (like newVersionCmd)
// write to os.Stdout and are NOT captured here — use captureStdout for those.
func executeCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()

	root := newRootCmd()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	err := root.ExecuteContext(context.Background())
	return buf.String(), err
}

// captureStdout redirects os.Stdout to a pipe, calls fn, then returns what was
// written.  Use this for commands that call fmt.Printf instead of
// cmd.OutOrStdout().
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = orig

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy from pipe: %v", err)
	}
	r.Close()

	return buf.String()
}

// ── TestNewRootCmd ────────────────────────────────────────────────────────────

func TestNewRootCmd(t *testing.T) {
	cmd := newRootCmd()

	if cmd.Use != "dhg" {
		t.Errorf("expected Use %q, got %q", "dhg", cmd.Use)
	}

	if cmd.Short != "Deckhouse Helm Generator" {
		t.Errorf("expected Short %q, got %q", "Deckhouse Helm Generator", cmd.Short)
	}

	subNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subNames[sub.Use] = true
	}

	for _, expected := range []string{"generate", "analyze", "version"} {
		if !subNames[expected] {
			t.Errorf("expected subcommand %q to be registered", expected)
		}
	}

	got := len(cmd.Commands())
	if got != 3 {
		t.Errorf("expected 3 subcommands, got %d", got)
	}
}

func TestNewRootCmd_Version(t *testing.T) {
	cmd := newRootCmd()

	if !strings.Contains(cmd.Version, "dev") {
		t.Errorf("expected Version to contain %q, got %q", "dev", cmd.Version)
	}

	if !strings.Contains(cmd.Version, "unknown") {
		t.Errorf("expected Version to contain %q, got %q", "unknown", cmd.Version)
	}
}

// ── TestNewGenerateCmd ────────────────────────────────────────────────────────

func TestNewGenerateCmd(t *testing.T) {
	cmd := newGenerateCmd()

	if cmd.Use != "generate" {
		t.Errorf("expected Use %q, got %q", "generate", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("expected Short to be non-empty")
	}
}

func TestNewGenerateCmd_Flags(t *testing.T) {
	cmd := newGenerateCmd()

	expectedFlags := []string{
		"file",
		"output",
		"chart-name",
		"chart-version",
		"app-version",
		"mode",
		"source",
		"namespace",
		"namespaces",
		"selector",
		"include-kinds",
		"exclude-kinds",
		"recursive",
		"kubeconfig",
		"context",
		"include-tests",
		"include-readme",
		"include-schema",
		"verbose",
		"env-values",
		"deckhouse-module",
	}

	for _, name := range expectedFlags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag %q to be registered on generate command", name)
		}
	}
}

func TestNewGenerateCmd_ChartNameIsRequired(t *testing.T) {
	// cobra stores the "required" annotation under the key
	// "cobra_annotation_bash_comp_one_required_flag" with value ["true"].
	// If that key is absent fall back to an execution-based check.
	cmd := newGenerateCmd()

	flag := cmd.Flags().Lookup("chart-name")
	if flag == nil {
		t.Fatal("flag chart-name not found")
	}

	const cobraRequiredKey = "cobra_annotation_bash_comp_one_required_flag"
	if ann, ok := flag.Annotations[cobraRequiredKey]; ok {
		if len(ann) == 0 || ann[0] != "true" {
			t.Errorf("expected chart-name required annotation to be [true], got %v", ann)
		}
		return
	}

	// Annotation key not present under that name; verify via execution.
	root := newRootCmd()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"generate", "--file", "/tmp"})
	err := root.ExecuteContext(context.Background())
	if err == nil {
		t.Error("expected error when --chart-name is missing, got nil")
	}
}

// ── TestNewAnalyzeCmd ─────────────────────────────────────────────────────────

func TestNewAnalyzeCmd(t *testing.T) {
	cmd := newAnalyzeCmd()

	if cmd.Use != "analyze" {
		t.Errorf("expected Use %q, got %q", "analyze", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("expected Short to be non-empty")
	}
}

func TestNewAnalyzeCmd_FileFlag(t *testing.T) {
	cmd := newAnalyzeCmd()

	flag := cmd.Flags().Lookup("file")
	if flag == nil {
		t.Fatal("expected 'file' flag to be registered on analyze command")
	}
}

func TestNewAnalyzeCmd_AdditionalFlags(t *testing.T) {
	cmd := newAnalyzeCmd()

	expectedFlags := []string{
		"file",
		"output-format",
		"output",
		"summary",
		"color",
		"verbose",
		"namespace",
		"namespaces",
		"include-kinds",
		"exclude-kinds",
		"recursive",
	}

	for _, name := range expectedFlags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag %q to be registered on analyze command", name)
		}
	}
}

// ── TestNewVersionCmd ─────────────────────────────────────────────────────────

func TestNewVersionCmd(t *testing.T) {
	cmd := newVersionCmd()

	if cmd.Use != "version" {
		t.Errorf("expected Use %q, got %q", "version", cmd.Use)
	}

	if cmd.Short == "" {
		t.Error("expected Short to be non-empty")
	}
}

// ── TestVersionCmd_Execute ────────────────────────────────────────────────────

// TestVersionCmd_Execute verifies that running the version subcommand prints
// the default version string.  newVersionCmd uses fmt.Printf (writing to
// os.Stdout directly) rather than cobra's OutOrStdout(), so we capture real
// stdout via an OS pipe.
func TestVersionCmd_Execute(t *testing.T) {
	var execErr error

	out := captureStdout(t, func() {
		root := newRootCmd()
		root.SetArgs([]string{"version"})
		execErr = root.ExecuteContext(context.Background())
	})

	if execErr != nil {
		t.Fatalf("unexpected error executing version command: %v", execErr)
	}

	if !strings.Contains(out, "dev") {
		t.Errorf("expected version output to contain %q, got: %q", "dev", out)
	}
}

// ── TestGenerateCmd_MissingChartName ──────────────────────────────────────────

func TestGenerateCmd_MissingChartName(t *testing.T) {
	_, err := executeCmd(t, "generate", "--file", "/tmp")
	if err == nil {
		t.Error("expected error when --chart-name is missing, got nil")
	}
}

// ── TestRootCmd_Help ──────────────────────────────────────────────────────────

func TestRootCmd_Help(t *testing.T) {
	out, err := executeCmd(t, "--help")
	if err != nil {
		t.Fatalf("unexpected error executing --help: %v", err)
	}

	if !strings.Contains(out, "dhg") {
		t.Errorf("expected help output to contain %q, got: %q", "dhg", out)
	}

	for _, sub := range []string{"generate", "analyze", "version"} {
		if !strings.Contains(out, sub) {
			t.Errorf("expected help output to mention subcommand %q", sub)
		}
	}
}

// ── TestAnalyzeCmd_MissingFileFlag ────────────────────────────────────────────

func TestAnalyzeCmd_MissingFileFlag(t *testing.T) {
	_, err := executeCmd(t, "analyze")
	if err == nil {
		t.Error("expected error when --file flag is missing from analyze command, got nil")
	}
}
