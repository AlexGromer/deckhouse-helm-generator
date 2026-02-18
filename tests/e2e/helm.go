// Package e2e provides an end-to-end test framework for validating generated
// Helm charts using the real Helm CLI and optional Kubernetes cluster integration.
package e2e

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// HelmClient wraps the Helm CLI for E2E testing.
type HelmClient struct {
	// BinaryPath is the path to the helm binary.
	BinaryPath string

	// Env contains additional environment variables passed to helm commands.
	Env []string

	// KubeConfig is the path to a kubeconfig file (optional, for cluster tests).
	KubeConfig string
}

// NewHelmClient creates a new HelmClient, locating the helm binary.
// Returns an error if helm is not found.
func NewHelmClient() (*HelmClient, error) {
	path := findHelmBinary()
	if path == "" {
		return nil, fmt.Errorf("helm binary not found in PATH or common locations")
	}
	return &HelmClient{BinaryPath: path}, nil
}

// HelmResult contains the result of a Helm command execution.
type HelmResult struct {
	// Stdout is the captured standard output.
	Stdout string

	// Stderr is the captured standard error.
	Stderr string

	// ExitCode is the process exit code.
	ExitCode int

	// Err is the Go error (non-nil if command failed to start or returned non-zero exit).
	Err error
}

// Success returns true if the command exited with code 0.
func (r *HelmResult) Success() bool {
	return r.ExitCode == 0 && r.Err == nil
}

// runHelm executes a helm command and captures all output.
func (c *HelmClient) runHelm(args ...string) *HelmResult {
	cmd := exec.Command(c.BinaryPath, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Merge environment
	cmd.Env = os.Environ()
	if c.KubeConfig != "" {
		cmd.Env = append(cmd.Env, "KUBECONFIG="+c.KubeConfig)
	}
	cmd.Env = append(cmd.Env, c.Env...)

	err := cmd.Run()
	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		exitCode = -1
	}

	return &HelmResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Err:      err,
	}
}

// Lint runs `helm lint` on the given chart directory.
func (c *HelmClient) Lint(chartDir string, opts ...LintOption) *HelmResult {
	args := []string{"lint", chartDir}

	cfg := &lintConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.valuesFile != "" {
		args = append(args, "--values", cfg.valuesFile)
	}
	if cfg.strict {
		args = append(args, "--strict")
	}

	return c.runHelm(args...)
}

// Template runs `helm template` on the given chart directory.
func (c *HelmClient) Template(releaseName, chartDir string, opts ...TemplateOption) *HelmResult {
	args := []string{"template", releaseName, chartDir}

	cfg := &templateConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.valuesFile != "" {
		args = append(args, "--values", cfg.valuesFile)
	}
	if cfg.namespace != "" {
		args = append(args, "--namespace", cfg.namespace)
	}
	for _, set := range cfg.setValues {
		args = append(args, "--set", set)
	}
	if cfg.showOnly != "" {
		args = append(args, "--show-only", cfg.showOnly)
	}
	if cfg.debug {
		args = append(args, "--debug")
	}

	return c.runHelm(args...)
}

// Install runs `helm install --dry-run` on the given chart directory.
func (c *HelmClient) Install(releaseName, chartDir string, opts ...InstallOption) *HelmResult {
	args := []string{"install", releaseName, chartDir, "--dry-run"}

	cfg := &installConfig{dryRun: "client"}
	for _, opt := range opts {
		opt(cfg)
	}

	// Replace the default --dry-run with the configured mode
	args[len(args)-1] = "--dry-run=" + cfg.dryRun

	// For client-side dry-run, disable OpenAPI validation since there is
	// no real API server to fetch schemas from.
	if cfg.dryRun == "client" {
		args = append(args, "--disable-openapi-validation")
	}

	if cfg.valuesFile != "" {
		args = append(args, "--values", cfg.valuesFile)
	}
	if cfg.namespace != "" {
		args = append(args, "--namespace", cfg.namespace)
	}
	if cfg.createNamespace {
		args = append(args, "--create-namespace")
	}
	for _, set := range cfg.setValues {
		args = append(args, "--set", set)
	}
	if cfg.debug {
		args = append(args, "--debug")
	}

	return c.runHelm(args...)
}

// Version returns the helm version string.
func (c *HelmClient) Version() *HelmResult {
	return c.runHelm("version", "--short")
}

// ============================================================
// Option types for Lint, Template, Install
// ============================================================

type lintConfig struct {
	valuesFile string
	strict     bool
}

// LintOption configures a lint invocation.
type LintOption func(*lintConfig)

// WithLintValues specifies a values file for lint.
func WithLintValues(path string) LintOption {
	return func(c *lintConfig) { c.valuesFile = path }
}

// WithLintStrict enables strict mode.
func WithLintStrict() LintOption {
	return func(c *lintConfig) { c.strict = true }
}

type templateConfig struct {
	valuesFile string
	namespace  string
	setValues  []string
	showOnly   string
	debug      bool
}

// TemplateOption configures a template invocation.
type TemplateOption func(*templateConfig)

// WithTemplateValues specifies a values file for template.
func WithTemplateValues(path string) TemplateOption {
	return func(c *templateConfig) { c.valuesFile = path }
}

// WithTemplateNamespace specifies the namespace for template.
func WithTemplateNamespace(ns string) TemplateOption {
	return func(c *templateConfig) { c.namespace = ns }
}

// WithTemplateSet adds a --set value.
func WithTemplateSet(kv string) TemplateOption {
	return func(c *templateConfig) { c.setValues = append(c.setValues, kv) }
}

// WithTemplateShowOnly filters to a single template.
func WithTemplateShowOnly(path string) TemplateOption {
	return func(c *templateConfig) { c.showOnly = path }
}

// WithTemplateDebug enables debug output.
func WithTemplateDebug() TemplateOption {
	return func(c *templateConfig) { c.debug = true }
}

type installConfig struct {
	valuesFile      string
	namespace       string
	createNamespace bool
	setValues       []string
	debug           bool
	dryRun          string // "client" or "server"
}

// InstallOption configures an install invocation.
type InstallOption func(*installConfig)

// WithInstallValues specifies a values file.
func WithInstallValues(path string) InstallOption {
	return func(c *installConfig) { c.valuesFile = path }
}

// WithInstallNamespace specifies the namespace.
func WithInstallNamespace(ns string) InstallOption {
	return func(c *installConfig) { c.namespace = ns }
}

// WithInstallCreateNamespace creates the namespace if it doesn't exist.
func WithInstallCreateNamespace() InstallOption {
	return func(c *installConfig) { c.createNamespace = true }
}

// WithInstallSet adds a --set value.
func WithInstallSet(kv string) InstallOption {
	return func(c *installConfig) { c.setValues = append(c.setValues, kv) }
}

// WithInstallDebug enables debug output.
func WithInstallDebug() InstallOption {
	return func(c *installConfig) { c.debug = true }
}

// WithInstallDryRunServer uses server-side dry-run (requires K8s cluster).
func WithInstallDryRunServer() InstallOption {
	return func(c *installConfig) { c.dryRun = "server" }
}

// ============================================================
// Helm binary discovery
// ============================================================

func findHelmBinary() string {
	// Check PATH first
	if path, err := exec.LookPath("helm"); err == nil {
		return path
	}

	// Check common locations
	locations := []string{
		filepath.Join(os.Getenv("HOME"), ".local", "bin", "helm"),
		"/usr/local/bin/helm",
		"/usr/bin/helm",
		"/snap/bin/helm",
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}

	return ""
}

// RequireHelm skips the test if helm is not available.
func RequireHelm(t *testing.T) *HelmClient {
	t.Helper()
	client, err := NewHelmClient()
	if err != nil {
		t.Skipf("Skipping: %v", err)
	}
	return client
}

// WriteValuesFile writes a values override YAML file to a temporary location.
func WriteValuesFile(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "custom-values.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write values file: %v", err)
	}
	return path
}

// ContainsError checks if the result contains a specific error keyword.
func (r *HelmResult) ContainsError(keyword string) bool {
	combined := r.Stdout + r.Stderr
	return strings.Contains(combined, keyword)
}
