package generator

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// buildTestBinary compiles a tiny Go program from src into a temp directory and
// returns the path to the resulting binary. The binary is built with `go build`
// which is always available inside `go test`.
func buildTestBinary(t *testing.T, src string) string {
	t.Helper()

	dir := t.TempDir()
	srcFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(srcFile, []byte(src), 0o644); err != nil {
		t.Fatalf("plugin_test: write helper source: %v", err)
	}

	binPath := filepath.Join(dir, "helper")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}

	cmd := exec.Command("go", "build", "-o", binPath, srcFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("go build output:\n%s", out)
		t.Fatalf("plugin_test: build helper binary: %v", err)
	}
	return binPath
}

// validExtractedResource returns a minimal *types.ExtractedResource.
func validExtractedResource() *types.ExtractedResource {
	obj := &unstructured.Unstructured{}
	obj.SetName("test-deploy")
	obj.SetNamespace("default")
	obj.SetKind("Deployment")
	obj.SetAPIVersion("apps/v1")
	return &types.ExtractedResource{
		Object:     obj,
		Source:     types.SourceFile,
		SourcePath: "manifests/deploy.yaml",
		GVK: schema.GroupVersionKind{
			Group:   "apps",
			Version: "v1",
			Kind:    "Deployment",
		},
	}
}

// echoProcessorSrc reads ExtractedResource JSON from stdin, writes back a
// minimal ProcessedResource JSON to stdout — simulates a well-behaved plugin.
const echoProcessorSrc = `package main

import (
	"encoding/json"
	"os"
)

type ProcessedResource struct {
	ServiceName     string                 ` + "`" + `json:"serviceName"` + "`" + `
	TemplatePath    string                 ` + "`" + `json:"templatePath"` + "`" + `
	TemplateContent string                 ` + "`" + `json:"templateContent"` + "`" + `
	ValuesPath      string                 ` + "`" + `json:"valuesPath"` + "`" + `
	Values          map[string]interface{} ` + "`" + `json:"values"` + "`" + `
}

func main() {
	var in map[string]interface{}
	_ = json.NewDecoder(os.Stdin).Decode(&in)
	out := ProcessedResource{
		ServiceName:     "echo-service",
		TemplatePath:    "templates/echo.yaml",
		TemplateContent: "# generated",
		ValuesPath:      "echo",
		Values:          map[string]interface{}{"enabled": true},
	}
	_ = json.NewEncoder(os.Stdout).Encode(out)
}
`

// badJSONProcessorSrc outputs invalid JSON — simulates a broken plugin.
const badJSONProcessorSrc = `package main

import (
	"encoding/json"
	"os"
)

func main() {
	var in map[string]interface{}
	_ = json.NewDecoder(os.Stdin).Decode(&in)
	os.Stdout.WriteString("NOT_VALID_JSON!!!")
}
`

// slowProcessorSrc sleeps 30 s — simulates a hung plugin for timeout tests.
const slowProcessorSrc = `package main

import (
	"encoding/json"
	"os"
	"time"
)

func main() {
	var in map[string]interface{}
	_ = json.NewDecoder(os.Stdin).Decode(&in)
	time.Sleep(30 * time.Second)
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"serviceName": "slow"})
}
`

// ── Test 1: RunExternalProcessor — happy path ─────────────────────────────────

func TestRunExternalProcessor_HappyPath(t *testing.T) {
	bin := buildTestBinary(t, echoProcessorSrc)
	input := validExtractedResource()

	result, err := RunExternalProcessor(bin, input, 10*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil ProcessedResource")
	}
	if result.ServiceName != "echo-service" {
		t.Errorf("ServiceName = %q, want %q", result.ServiceName, "echo-service")
	}
	if result.TemplatePath != "templates/echo.yaml" {
		t.Errorf("TemplatePath = %q, want %q", result.TemplatePath, "templates/echo.yaml")
	}
}

// ── Test 2: RunExternalProcessor — nil input returns error ────────────────────

func TestRunExternalProcessor_NilInput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no /bin/true on Windows")
	}
	// Any real executable works here; we expect an early error before exec.
	_, err := RunExternalProcessor("/bin/true", nil, 5*time.Second)
	if err == nil {
		t.Fatal("expected error for nil input, got nil")
	}
}

// ── Test 3: RunExternalProcessor — plugin binary not found ───────────────────

func TestRunExternalProcessor_PluginNotFound(t *testing.T) {
	_, err := RunExternalProcessor("/nonexistent/plugin-binary-xyz", validExtractedResource(), 5*time.Second)
	if err == nil {
		t.Fatal("expected error for missing executable, got nil")
	}
}

// ── Test 4: RunExternalProcessor — invalid JSON response from plugin ──────────

func TestRunExternalProcessor_InvalidJSONResponse(t *testing.T) {
	bin := buildTestBinary(t, badJSONProcessorSrc)
	input := validExtractedResource()

	_, err := RunExternalProcessor(bin, input, 10*time.Second)
	if err == nil {
		t.Fatal("expected JSON parse error, got nil")
	}
	// The error must not be nil — that is the full contract here.
	_ = err
}

// ── Test 5: RunExternalProcessor — timeout kills hung plugin ──────────────────

func TestRunExternalProcessor_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow-process timeout test in -short mode")
	}
	bin := buildTestBinary(t, slowProcessorSrc)
	input := validExtractedResource()

	start := time.Now()
	_, err := RunExternalProcessor(bin, input, 300*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	// Sanity: the call must have returned in well under the 30-s sleep.
	if elapsed > 5*time.Second {
		t.Errorf("timeout not enforced — elapsed %v, expected <5s", elapsed)
	}
}

// ── Test 6: DiscoverPlugins — finds executables in a directory ────────────────

func TestDiscoverPlugins_FindsExecutables(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable permission model not applicable on Windows")
	}
	dir := t.TempDir()

	for _, name := range []string{"dhg-plugin-a", "dhg-plugin-b"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\necho '{}'"), 0o755); err != nil {
			t.Fatalf("create plugin file %s: %v", name, err)
		}
	}

	plugins, err := DiscoverPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 2 {
		t.Errorf("expected 2 plugins, got %d", len(plugins))
	}
}

// ── Test 7: DiscoverPlugins — empty directory ─────────────────────────────────

func TestDiscoverPlugins_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	plugins, err := DiscoverPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error for empty dir: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

// ── Test 8: DiscoverPlugins — non-executable files are skipped ───────────────

func TestDiscoverPlugins_NonExecutableFilesSkipped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission model not applicable on Windows")
	}
	dir := t.TempDir()

	// Executable plugin.
	execPath := filepath.Join(dir, "dhg-plugin-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho '{}'"), 0o755); err != nil {
		t.Fatalf("create executable file: %v", err)
	}

	// Regular file (0644) — must be skipped.
	noExecPath := filepath.Join(dir, "dhg-plugin-noexec")
	if err := os.WriteFile(noExecPath, []byte("not executable"), 0o644); err != nil {
		t.Fatalf("create non-executable file: %v", err)
	}

	plugins, err := DiscoverPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 1 {
		t.Errorf("expected 1 plugin (executable only), got %d", len(plugins))
	}
	if len(plugins) > 0 && plugins[0].Name != "dhg-plugin-exec" {
		t.Errorf("expected plugin name %q, got %q", "dhg-plugin-exec", plugins[0].Name)
	}
}

// ── Test 9: DiscoverPlugins — non-existent directory returns error ────────────

func TestDiscoverPlugins_NonExistentDir(t *testing.T) {
	_, err := DiscoverPlugins("/nonexistent/plugin/directory/xyz-404")
	if err == nil {
		t.Fatal("expected error for non-existent directory, got nil")
	}
}

// ── Test 10: DiscoverPlugins — multiple plugins have Name and Path populated ──

func TestDiscoverPlugins_MultiplePluginsPopulateInfo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable permission model not applicable on Windows")
	}
	dir := t.TempDir()

	names := []string{"alpha", "beta", "gamma"}
	for _, n := range names {
		p := filepath.Join(dir, "dhg-plugin-"+n)
		if err := os.WriteFile(p, []byte("#!/bin/sh\necho '{}'"), 0o755); err != nil {
			t.Fatalf("create plugin file: %v", err)
		}
	}

	plugins, err := DiscoverPlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 3 {
		t.Errorf("expected 3 plugins, got %d", len(plugins))
	}

	for i, pi := range plugins {
		if pi.Name == "" {
			t.Errorf("plugin[%d].Name is empty", i)
		}
		if pi.Path == "" {
			t.Errorf("plugin[%d].Path is empty", i)
		}
		if filepath.Dir(pi.Path) != dir {
			t.Errorf("plugin[%d].Path = %q is not inside temp dir %q", i, pi.Path, dir)
		}
	}

	// PluginInfo type must carry Supports and Version fields (may be zero-value).
	var _ PluginInfo = plugins[0] // compile-time structural check
	_ = plugins[0].Supports
	_ = plugins[0].Version

	// Verify json round-trip of PluginInfo is possible (no unexported fields block it).
	data, err := json.Marshal(plugins[0])
	if err != nil {
		t.Errorf("PluginInfo must be JSON-serialisable: %v", err)
	}
	var roundTrip PluginInfo
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Errorf("PluginInfo must be JSON-deserialisable: %v", err)
	}
}
