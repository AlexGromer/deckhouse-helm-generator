package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// executeCmd runs the root command with the given arguments, capturing cobra's
// output writer.
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

	for _, expected := range []string{"generate", "analyze", "validate", "diff <dir1> <dir2>", "version"} {
		if !subNames[expected] {
			t.Errorf("expected subcommand %q to be registered", expected)
		}
	}

	got := len(cmd.Commands())
	if got != 7 {
		t.Errorf("expected 7 subcommands (generate, analyze, validate, diff, version, fix, migrate), got %d", got)
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
		"template-style",
		"values-flat",
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
// the default version string via cobra's OutOrStdout().
func TestVersionCmd_Execute(t *testing.T) {
	out, err := executeCmd(t, "version")
	if err != nil {
		t.Fatalf("unexpected error executing version command: %v", err)
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

	for _, sub := range []string{"generate", "analyze", "validate", "diff", "version"} {
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

// ── TestNewRootCmd_HasAllCommands ─────────────────────────────────────────────

func TestNewRootCmd_HasAllCommands(t *testing.T) {
	cmd := newRootCmd()

	expectedCmds := []string{"generate", "analyze", "validate", "diff", "version"}
	for _, name := range expectedCmds {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected command %q not found in root command", name)
		}
	}
}

// ── TestValidateCmd ───────────────────────────────────────────────────────────

func TestValidateCmd_MissingChart(t *testing.T) {
	tmpDir := t.TempDir()

	err := runValidate(context.Background(), validateOptions{
		paths:   []string{tmpDir},
		verbose: false,
	})

	if err == nil {
		t.Error("Expected error for empty directory")
	}
}

func TestValidateCmd_ValidChart(t *testing.T) {
	tmpDir := t.TempDir()

	// Create minimal valid chart
	chartYAML := "apiVersion: v2\nname: test-chart\nversion: 0.1.0\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "Chart.yaml"), []byte(chartYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "values.yaml"), []byte("key: value\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "templates"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "templates", "test.yaml"), []byte("{{ .Values.key }}"), 0644); err != nil {
		t.Fatal(err)
	}

	err := runValidate(context.Background(), validateOptions{
		paths:   []string{tmpDir},
		verbose: true,
	})

	if err != nil {
		t.Errorf("Expected no error for valid chart, got: %v", err)
	}
}

// ── TestDiffCmd ───────────────────────────────────────────────────────────────

func TestDiffCmd_IdenticalDirs(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	content := "test: value\n"
	if err := os.WriteFile(filepath.Join(dir1, "values.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "values.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	err := runDiff(context.Background(), diffOptions{
		dir1:  dir1,
		dir2:  dir2,
		color: false,
	})

	if err != nil {
		t.Errorf("Expected no error for identical dirs, got: %v", err)
	}
}

func TestDiffCmd_NonexistentDir(t *testing.T) {
	err := runDiff(context.Background(), diffOptions{
		dir1:  "/nonexistent/path1",
		dir2:  "/nonexistent/path2",
		color: false,
	})

	if err == nil {
		t.Error("Expected error for nonexistent directory")
	}
}

// ── TestGenerateCmd_HasDryRunFlag ─────────────────────────────────────────────

// ── TestGenerateCmd_CloudProviderValidation ───────────────────────────────────

func TestGenerateCmd_UnknownCloudProviderReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := executeCmd(t,
		"generate",
		"--file", tmpDir,
		"--chart-name", "test",
		"--cloud-provider", "unknown",
		"--dry-run",
	)
	if err == nil {
		t.Fatal("expected error for unknown cloud provider, got nil")
	}
	if !strings.Contains(err.Error(), "unknown cloud provider") {
		t.Errorf("expected error to mention 'unknown cloud provider', got: %v", err)
	}
}

func TestGenerateCmd_AWSCloudProviderWithSpot(t *testing.T) {
	// Create a minimal YAML manifest so extraction succeeds
	tmpDir := t.TempDir()
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deploy
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
      - name: app
        image: nginx:latest
`
	if err := os.WriteFile(filepath.Join(tmpDir, "deploy.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := executeCmd(t,
		"generate",
		"--file", tmpDir,
		"--chart-name", "test",
		"--cloud-provider", "aws",
		"--spot",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("expected no error for --cloud-provider aws --spot, got: %v", err)
	}
}

func TestGenerateCmd_GCPCloudProviderAccepted(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := executeCmd(t,
		"generate",
		"--file", tmpDir,
		"--chart-name", "test",
		"--cloud-provider", "gcp",
		"--dry-run",
	)
	if err != nil {
		if strings.Contains(err.Error(), "unknown cloud provider") {
			t.Fatalf("gcp should be accepted as cloud provider, got: %v", err)
		}
	}
}

func TestGenerateCmd_AzureCloudProviderAccepted(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := executeCmd(t,
		"generate",
		"--file", tmpDir,
		"--chart-name", "test",
		"--cloud-provider", "azure",
		"--dry-run",
	)
	if err != nil {
		if strings.Contains(err.Error(), "unknown cloud provider") {
			t.Fatalf("azure should be accepted as cloud provider, got: %v", err)
		}
	}
}

func TestGenerateCmd_SpotDefaultsToAWS(t *testing.T) {
	tmpDir := t.TempDir()
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deploy
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
      - name: app
        image: nginx:latest
`
	if err := os.WriteFile(filepath.Join(tmpDir, "deploy.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	// --spot without --cloud-provider should default to AWS (no error)
	_, err := executeCmd(t,
		"generate",
		"--file", tmpDir,
		"--chart-name", "test",
		"--spot",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("expected no error for --spot without --cloud-provider, got: %v", err)
	}
}

func TestGenerateCmd_HasDryRunFlag(t *testing.T) {
	cmd := newGenerateCmd()
	flag := cmd.Flags().Lookup("dry-run")
	if flag == nil {
		t.Error("Expected --dry-run flag on generate command")
	}
}

// ── TestNamespaceResources_SkipsDefaultNPWhenAutoNPExists ────────────────────

// TestNamespaceResources_SkipsDefaultNPWhenAutoNPExists verifies that when both
// namespace-resources and auto-NetworkPolicy generators produce output for the
// same group, the broad default NP (from namespace.go) is excluded in favor of
// the fine-grained NP (from networkpolicy.go).
func TestNamespaceResources_SkipsDefaultNPWhenAutoNPExists(t *testing.T) {
	// Simulate namespace-resources templates (broad default NP)
	nsTemplates := map[string]string{
		"templates/frontend-resourcequota.yaml":          "kind: ResourceQuota",
		"templates/frontend-limitrange.yaml":             "kind: LimitRange",
		"templates/frontend-networkpolicy-default.yaml":  "kind: NetworkPolicy\npodSelector: {}",
		"templates/backend-resourcequota.yaml":           "kind: ResourceQuota",
		"templates/backend-limitrange.yaml":              "kind: LimitRange",
		"templates/backend-networkpolicy-default.yaml":   "kind: NetworkPolicy\npodSelector: {}",
	}

	// Simulate auto-NP templates (fine-grained per-service) — only for "frontend"
	autoNP := map[string]string{
		"templates/frontend-networkpolicy.yaml": "kind: NetworkPolicy\ningress:\n- from: [{podSelector: {matchLabels: {app: api}}}]",
	}

	// Build autoNPGroups set (same logic as main.go)
	autoNPGroups := make(map[string]struct{}, len(autoNP))
	for path := range autoNP {
		name := strings.TrimPrefix(path, "templates/")
		name = strings.TrimSuffix(name, "-networkpolicy.yaml")
		if name != path {
			autoNPGroups[name] = struct{}{}
		}
	}

	// Existing chart templates
	chartTemplates := map[string]string{
		"templates/deployment.yaml": "kind: Deployment",
	}

	// Merge with filtering (same logic as main.go)
	templates := make(map[string]string, len(chartTemplates)+len(nsTemplates)+len(autoNP))
	for k, v := range chartTemplates {
		templates[k] = v
	}
	for path, content := range nsTemplates {
		if strings.HasSuffix(path, "-networkpolicy-default.yaml") {
			groupName := strings.TrimPrefix(path, "templates/")
			groupName = strings.TrimSuffix(groupName, "-networkpolicy-default.yaml")
			if _, has := autoNPGroups[groupName]; has {
				continue
			}
		}
		templates[path] = content
	}
	for path, content := range autoNP {
		templates[path] = content
	}

	// Verify: frontend default NP should be excluded (fine-grained NP takes precedence)
	if _, exists := templates["templates/frontend-networkpolicy-default.yaml"]; exists {
		t.Error("frontend-networkpolicy-default.yaml should be excluded when fine-grained auto-NP exists")
	}

	// Verify: frontend fine-grained NP should be present
	if _, exists := templates["templates/frontend-networkpolicy.yaml"]; !exists {
		t.Error("frontend-networkpolicy.yaml (fine-grained) should be present")
	}

	// Verify: backend default NP should be kept (no auto-NP for backend)
	if _, exists := templates["templates/backend-networkpolicy-default.yaml"]; !exists {
		t.Error("backend-networkpolicy-default.yaml should be kept when no auto-NP exists for that group")
	}

	// Verify: other namespace templates are not affected
	if _, exists := templates["templates/frontend-resourcequota.yaml"]; !exists {
		t.Error("frontend-resourcequota.yaml should not be affected by NP filtering")
	}
	if _, exists := templates["templates/frontend-limitrange.yaml"]; !exists {
		t.Error("frontend-limitrange.yaml should not be affected by NP filtering")
	}
	if _, exists := templates["templates/backend-resourcequota.yaml"]; !exists {
		t.Error("backend-resourcequota.yaml should not be affected by NP filtering")
	}

	// Verify: original chart templates preserved
	if _, exists := templates["templates/deployment.yaml"]; !exists {
		t.Error("original chart templates should be preserved")
	}

	// Verify total count: 1 (deployment) + 2 (frontend quota+limit) + 3 (backend quota+limit+defaultNP) + 1 (frontend auto-NP) = 7
	expected := 7
	if len(templates) != expected {
		t.Errorf("expected %d templates total, got %d", expected, len(templates))
		for k := range templates {
			t.Logf("  template: %s", k)
		}
	}
}

// ── TestGenerateCmd_TemplateStyleFlag ────────────────────────────────────────

func TestGenerateCmd_TemplateStyleFlag(t *testing.T) {
	tmpDir := t.TempDir()
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deploy
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
      - name: app
        image: nginx:latest
`
	if err := os.WriteFile(filepath.Join(tmpDir, "deploy.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	// Default value ("standard") should be accepted
	_, err := executeCmd(t,
		"generate",
		"--file", tmpDir,
		"--chart-name", "test",
		"--template-style", "standard",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("expected no error for --template-style standard, got: %v", err)
	}

	// "helm" should be accepted
	_, err = executeCmd(t,
		"generate",
		"--file", tmpDir,
		"--chart-name", "test",
		"--template-style", "helm",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("expected no error for --template-style helm, got: %v", err)
	}

	// Invalid value should return error
	_, err = executeCmd(t,
		"generate",
		"--file", tmpDir,
		"--chart-name", "test",
		"--template-style", "invalid",
		"--dry-run",
	)
	if err == nil {
		t.Fatal("expected error for --template-style invalid, got nil")
	}
	if !strings.Contains(err.Error(), "unknown template style") {
		t.Errorf("expected error to mention 'unknown template style', got: %v", err)
	}
}

// ── TestGenerateCmd_ValuesFlatFlag ───────────────────────────────────────────

func TestGenerateCmd_ValuesFlatFlag(t *testing.T) {
	tmpDir := t.TempDir()
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deploy
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
      - name: app
        image: nginx:latest
`
	if err := os.WriteFile(filepath.Join(tmpDir, "deploy.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	// --values-flat should be accepted without error
	_, err := executeCmd(t,
		"generate",
		"--file", tmpDir,
		"--chart-name", "test",
		"--values-flat",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("expected no error for --values-flat, got: %v", err)
	}

	// Without --values-flat should also work (default behavior)
	_, err = executeCmd(t,
		"generate",
		"--file", tmpDir,
		"--chart-name", "test",
		"--dry-run",
	)
	if err != nil {
		t.Fatalf("expected no error without --values-flat, got: %v", err)
	}
}

// ── TestGenerateCmd_MonorepoKustomizeConflict ─────────────────────────────────

func TestGenerateCmd_MonorepoKustomizeConflict(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := executeCmd(t,
		"generate",
		"--file", tmpDir,
		"--chart-name", "test",
		"--monorepo",
		"--kustomize",
		"--dry-run",
	)
	if err == nil {
		t.Fatal("expected error when both --monorepo and --kustomize are set, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected error to mention 'mutually exclusive', got: %v", err)
	}
}

// ── TestGenerateCmd_PostRendererFlag ──────────────────────────────────────────

func TestGenerateCmd_PostRendererFlag(t *testing.T) {
	cmd := newGenerateCmd()
	flag := cmd.Flags().Lookup("post-renderer")
	if flag == nil {
		t.Fatal("Expected --post-renderer flag on generate command")
	}
	if flag.DefValue != "false" {
		t.Errorf("Expected default value 'false', got '%s'", flag.DefValue)
	}
}
