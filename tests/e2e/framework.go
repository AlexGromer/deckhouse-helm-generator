package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/analyzer"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/analyzer/detector"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/extractor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/generator"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor/k8s"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// E2ETestHarness manages the lifecycle of an E2E test, including temporary
// directories, chart generation, and Helm CLI interaction.
type E2ETestHarness struct {
	T *testing.T

	// TempDir is the root temporary directory for this test.
	TempDir string

	// InputDir holds input YAML manifests.
	InputDir string

	// OutputDir holds generated chart output.
	OutputDir string

	// ChartDir is the path to the generated chart on disk.
	ChartDir string

	// HelmClient is the Helm CLI wrapper.
	Helm *HelmClient

	// K8s is the Kubernetes client (optional).
	K8s *KubernetesClient

	// Chart is the generated chart metadata.
	Chart *types.GeneratedChart
}

// NewE2ETestHarness creates a new E2E test harness.
// It sets up temporary directories and locates the Helm binary.
func NewE2ETestHarness(t *testing.T) *E2ETestHarness {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "dhg-e2e-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	inputDir := filepath.Join(tmpDir, "input")
	outputDir := filepath.Join(tmpDir, "output")

	for _, dir := range []string{inputDir, outputDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
	}

	helm, err := NewHelmClient()
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Skipf("Helm not available: %v", err)
	}

	h := &E2ETestHarness{
		T:         t,
		TempDir:   tmpDir,
		InputDir:  inputDir,
		OutputDir: outputDir,
		Helm:      helm,
	}

	t.Cleanup(h.Cleanup)
	return h
}

// Cleanup removes all temporary directories.
func (h *E2ETestHarness) Cleanup() {
	if h.TempDir != "" {
		os.RemoveAll(h.TempDir)
	}
}

// WriteInput writes a YAML manifest to the input directory.
func (h *E2ETestHarness) WriteInput(filename, content string) {
	h.T.Helper()
	path := filepath.Join(h.InputDir, filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		h.T.Fatalf("Failed to create dir for %s: %v", filename, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		h.T.Fatalf("Failed to write input %s: %v", filename, err)
	}
}

// GenerateChart runs the full pipeline and generates the chart to disk.
func (h *E2ETestHarness) GenerateChart(chartName string) {
	h.T.Helper()
	h.GenerateChartWithOptions(chartName, "0.1.0", "latest")
}

// GenerateChartWithOptions runs the pipeline with custom chart metadata.
func (h *E2ETestHarness) GenerateChartWithOptions(chartName, chartVersion, appVersion string) {
	h.T.Helper()
	ctx := context.Background()

	// ── Extract ──
	fileExtractor := extractor.NewFileExtractor()
	extractOpts := extractor.Options{
		Paths:     []string{h.InputDir},
		Recursive: true,
	}

	resourceCh, errCh := fileExtractor.Extract(ctx, extractOpts)
	var resources []*types.ExtractedResource
	for {
		select {
		case res, ok := <-resourceCh:
			if !ok {
				resourceCh = nil
			} else {
				resources = append(resources, res)
			}
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
			} else {
				h.T.Fatalf("Extraction error: %v", err)
			}
		}
		if resourceCh == nil && errCh == nil {
			break
		}
	}

	if len(resources) == 0 {
		h.T.Fatal("No resources extracted from input")
	}

	// ── Process ──
	registry := processor.NewRegistry()
	k8s.RegisterAll(registry)

	procCtx := processor.Context{
		Ctx:        ctx,
		ChartName:  chartName,
		OutputMode: types.OutputModeUniversal,
	}

	var processed []*types.ProcessedResource
	for _, res := range resources {
		result, err := registry.Process(procCtx, res.Object)
		if err != nil {
			h.T.Fatalf("Processing error: %v", err)
		}
		if result == nil {
			continue
		}
		processed = append(processed, &types.ProcessedResource{
			Original:        res,
			ServiceName:     result.ServiceName,
			TemplatePath:    result.TemplatePath,
			TemplateContent: result.TemplateContent,
			ValuesPath:      result.ValuesPath,
			Values:          result.Values,
			Dependencies:    result.Dependencies,
		})
	}

	// ── Analyze ──
	a := analyzer.NewDefaultAnalyzer()
	detector.RegisterAll(a)
	graph, err := a.Analyze(ctx, processed)
	if err != nil {
		h.T.Fatalf("Analysis error: %v", err)
	}

	// ── Generate ──
	genOpts := generator.Options{
		OutputDir:    h.OutputDir,
		ChartName:    chartName,
		ChartVersion: chartVersion,
		AppVersion:   appVersion,
		Mode:         types.OutputModeUniversal,
	}

	gen := generator.NewUniversalGenerator()
	charts, err := gen.Generate(ctx, graph, genOpts)
	if err != nil {
		h.T.Fatalf("Generation error: %v", err)
	}

	if len(charts) == 0 {
		h.T.Fatal("No charts generated")
	}

	h.Chart = charts[0]

	// Write to disk
	if err := generator.WriteChart(h.Chart, h.OutputDir); err != nil {
		h.T.Fatalf("Failed to write chart: %v", err)
	}

	h.ChartDir = filepath.Join(h.OutputDir, h.Chart.Name)
}

// RequireGenerated ensures the chart has been generated.
func (h *E2ETestHarness) RequireGenerated() {
	h.T.Helper()
	if h.ChartDir == "" || h.Chart == nil {
		h.T.Fatal("Chart not generated; call GenerateChart first")
	}
}

// Lint runs helm lint on the generated chart.
func (h *E2ETestHarness) Lint(opts ...LintOption) *HelmResult {
	h.T.Helper()
	h.RequireGenerated()
	return h.Helm.Lint(h.ChartDir, opts...)
}

// Template runs helm template on the generated chart.
func (h *E2ETestHarness) Template(releaseName string, opts ...TemplateOption) *HelmResult {
	h.T.Helper()
	h.RequireGenerated()
	return h.Helm.Template(releaseName, h.ChartDir, opts...)
}

// Install runs helm install --dry-run on the generated chart.
// For client-side dry-run, it starts a mock K8s API server to satisfy
// Helm v3.20+ reachability checks (which occur even with --dry-run=client).
func (h *E2ETestHarness) Install(releaseName string, opts ...InstallOption) *HelmResult {
	h.T.Helper()
	h.RequireGenerated()

	// Check if any option requests server-side dry-run
	cfg := &installConfig{dryRun: "client"}
	for _, opt := range opts {
		opt(cfg)
	}

	// For client-side dry-run, start a mock API server so Helm's
	// IsReachable() check passes without a real cluster.
	if cfg.dryRun == "client" {
		mockServer, kubeconfig := startMockAPIServer(h.T, h.TempDir)
		defer mockServer.Close()

		// Create a helm client with the mock kubeconfig
		helmWithMock := &HelmClient{
			BinaryPath: h.Helm.BinaryPath,
			KubeConfig: kubeconfig,
			Env:        h.Helm.Env,
		}
		return helmWithMock.Install(releaseName, h.ChartDir, opts...)
	}

	return h.Helm.Install(releaseName, h.ChartDir, opts...)
}

// WriteCustomValues writes a custom values file and returns its path.
func (h *E2ETestHarness) WriteCustomValues(content string) string {
	h.T.Helper()
	return WriteValuesFile(h.T, h.TempDir, content)
}

// AssertLintSuccess asserts that helm lint passes.
func (h *E2ETestHarness) AssertLintSuccess(opts ...LintOption) {
	h.T.Helper()
	result := h.Lint(opts...)
	if !result.Success() {
		h.T.Fatalf("helm lint failed: %s\n%s", result.Stderr, result.Stdout)
	}
}

// AssertTemplateSuccess asserts that helm template succeeds.
func (h *E2ETestHarness) AssertTemplateSuccess(releaseName string, opts ...TemplateOption) string {
	h.T.Helper()
	result := h.Template(releaseName, opts...)
	if !result.Success() {
		h.T.Fatalf("helm template failed: %s\n%s", result.Stderr, result.Stdout)
	}
	return result.Stdout
}

// ChartPath returns the path to a file inside the chart directory.
func (h *E2ETestHarness) ChartPath(parts ...string) string {
	h.RequireGenerated()
	args := append([]string{h.ChartDir}, parts...)
	return filepath.Join(args...)
}

// GenerateFixtureChart generates a chart and copies it to the fixtures directory.
// Returns the path to the fixture chart.
func GenerateFixtureChart(t *testing.T, inputs map[string]string, chartName string) string {
	t.Helper()

	h := NewE2ETestHarness(t)

	for name, content := range inputs {
		h.WriteInput(name, content)
	}

	h.GenerateChart(chartName)

	// Copy to a separate directory so it outlives the harness
	fixtureDir, err := os.MkdirTemp("", "dhg-fixture-*")
	if err != nil {
		t.Fatalf("Failed to create fixture dir: %v", err)
	}

	destDir := filepath.Join(fixtureDir, chartName)
	if err := copyDir(h.ChartDir, destDir); err != nil {
		t.Fatalf("Failed to copy chart to fixture: %v", err)
	}

	return destDir
}

// startMockAPIServer starts a minimal HTTP server emulating a K8s API server.
// It responds to /version, /api, /apis, and common resource discovery paths.
// This satisfies Helm v3.20+ IsReachable() and resource mapping checks for
// --dry-run=client mode. Returns the server and path to a kubeconfig.
func startMockAPIServer(t *testing.T, tmpDir string) (*http.Server, string) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start mock API server: %v", err)
	}

	mux := http.NewServeMux()

	// /version — K8s version info
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"major": "1", "minor": "28", "gitVersion": "v1.28.0",
		})
	})

	// /api — core API versions
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"APIVersions","versions":["v1"]}`))
	})

	// /api/v1 — core v1 resources
	mux.HandleFunc("/api/v1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"v1","resources":[` +
			`{"name":"configmaps","namespaced":true,"kind":"ConfigMap","verbs":["create","delete","get","list","patch","update","watch"]},` +
			`{"name":"secrets","namespaced":true,"kind":"Secret","verbs":["create","delete","get","list","patch","update","watch"]},` +
			`{"name":"services","namespaced":true,"kind":"Service","verbs":["create","delete","get","list","patch","update","watch"]},` +
			`{"name":"persistentvolumeclaims","namespaced":true,"kind":"PersistentVolumeClaim","verbs":["create","delete","get","list","patch","update","watch"]},` +
			`{"name":"namespaces","namespaced":false,"kind":"Namespace","verbs":["create","delete","get","list","patch","update","watch"]}` +
			`]}`))
	})

	// /apis — API group list
	mux.HandleFunc("/apis", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"APIGroupList","apiVersion":"v1","groups":[` +
			`{"name":"apps","versions":[{"groupVersion":"apps/v1","version":"v1"}],"preferredVersion":{"groupVersion":"apps/v1","version":"v1"}},` +
			`{"name":"batch","versions":[{"groupVersion":"batch/v1","version":"v1"}],"preferredVersion":{"groupVersion":"batch/v1","version":"v1"}},` +
			`{"name":"networking.k8s.io","versions":[{"groupVersion":"networking.k8s.io/v1","version":"v1"}],"preferredVersion":{"groupVersion":"networking.k8s.io/v1","version":"v1"}},` +
			`{"name":"rbac.authorization.k8s.io","versions":[{"groupVersion":"rbac.authorization.k8s.io/v1","version":"v1"}],"preferredVersion":{"groupVersion":"rbac.authorization.k8s.io/v1","version":"v1"}}` +
			`]}`))
	})

	// /apis/apps/v1 — apps resources (Deployment, StatefulSet, DaemonSet)
	mux.HandleFunc("/apis/apps/v1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"apps/v1","resources":[` +
			`{"name":"deployments","namespaced":true,"kind":"Deployment","verbs":["create","delete","get","list","patch","update","watch"]},` +
			`{"name":"statefulsets","namespaced":true,"kind":"StatefulSet","verbs":["create","delete","get","list","patch","update","watch"]},` +
			`{"name":"daemonsets","namespaced":true,"kind":"DaemonSet","verbs":["create","delete","get","list","patch","update","watch"]}` +
			`]}`))
	})

	// /apis/batch/v1 — batch resources (Job, CronJob)
	mux.HandleFunc("/apis/batch/v1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"batch/v1","resources":[` +
			`{"name":"jobs","namespaced":true,"kind":"Job","verbs":["create","delete","get","list","patch","update","watch"]},` +
			`{"name":"cronjobs","namespaced":true,"kind":"CronJob","verbs":["create","delete","get","list","patch","update","watch"]}` +
			`]}`))
	})

	// /apis/networking.k8s.io/v1 — networking resources (Ingress)
	mux.HandleFunc("/apis/networking.k8s.io/v1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"networking.k8s.io/v1","resources":[` +
			`{"name":"ingresses","namespaced":true,"kind":"Ingress","verbs":["create","delete","get","list","patch","update","watch"]}` +
			`]}`))
	})

	// /apis/rbac.authorization.k8s.io/v1 — RBAC resources
	mux.HandleFunc("/apis/rbac.authorization.k8s.io/v1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"rbac.authorization.k8s.io/v1","resources":[` +
			`{"name":"clusterroles","namespaced":false,"kind":"ClusterRole","verbs":["create","delete","get","list","patch","update","watch"]},` +
			`{"name":"clusterrolebindings","namespaced":false,"kind":"ClusterRoleBinding","verbs":["create","delete","get","list","patch","update","watch"]},` +
			`{"name":"roles","namespaced":true,"kind":"Role","verbs":["create","delete","get","list","patch","update","watch"]},` +
			`{"name":"rolebindings","namespaced":true,"kind":"RoleBinding","verbs":["create","delete","get","list","patch","update","watch"]}` +
			`]}`))
	})

	// Fallback: return 404 for specific resource lookups (namespaces/...),
	// and empty resource list for API discovery paths.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		// Specific resource lookups (contain "namespaces/") should return 404
		if strings.Contains(path, "namespaces/") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`))
			return
		}
		_, _ = w.Write([]byte(`{"kind":"APIResourceList","apiVersion":"v1","resources":[]}`))
	})

	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()

	addr := listener.Addr().String()
	kubeconfigContent := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
  - cluster:
      server: http://%s
    name: mock
contexts:
  - context:
      cluster: mock
      user: mock
    name: mock
current-context: mock
users:
  - name: mock
    user:
      token: mock
`, addr)

	kubeconfigPath := filepath.Join(tmpDir, "mock-kubeconfig.yaml")
	if err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0600); err != nil {
		t.Fatalf("Failed to write mock kubeconfig: %v", err)
	}

	return server, kubeconfigPath
}

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		return os.WriteFile(destPath, data, 0644)
	})
}
