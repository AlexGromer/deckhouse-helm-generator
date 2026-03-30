package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newKubeconformChart(templates map[string]string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name:      "myapp",
		Path:      "/charts/myapp",
		ChartYAML: "apiVersion: v2\nname: myapp\nversion: 0.1.0\n",
		Templates: templates,
	}
}

// ── Test 1: config file is generated ─────────────────────────────────────────

func TestGenerateKubeconformConfig_ConfigFilePresent(t *testing.T) {
	chart := newKubeconformChart(map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\n",
	})
	opts := KubeconformOptions{KubernetesVersion: "1.29.0"}

	result := GenerateKubeconformConfig(chart, opts)

	if result == nil {
		t.Fatal("expected non-nil KubeconformResult")
	}
	if len(result.ConfigFiles) == 0 {
		t.Fatal("expected at least one config file in ConfigFiles")
	}
	if _, ok := result.ConfigFiles[".kubeconform.yaml"]; !ok {
		t.Error("expected .kubeconform.yaml to be present in ConfigFiles")
	}
}

// ── Test 2: Kubernetes version appears in config ──────────────────────────────

func TestGenerateKubeconformConfig_K8sVersionInConfig(t *testing.T) {
	chart := newKubeconformChart(map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\n",
	})
	opts := KubeconformOptions{KubernetesVersion: "1.29.0"}

	result := GenerateKubeconformConfig(chart, opts)

	config, ok := result.ConfigFiles[".kubeconform.yaml"]
	if !ok {
		t.Fatal("missing .kubeconform.yaml")
	}
	if !strings.Contains(config, "1.29.0") {
		t.Errorf("expected kubernetes version 1.29.0 in config, got:\n%s", config)
	}
}

// ── Test 3: strict mode is reflected in config ────────────────────────────────

func TestGenerateKubeconformConfig_StrictMode(t *testing.T) {
	chart := newKubeconformChart(map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\n",
	})
	opts := KubeconformOptions{KubernetesVersion: "1.29.0", Strict: true}

	result := GenerateKubeconformConfig(chart, opts)

	config := result.ConfigFiles[".kubeconform.yaml"]
	if !strings.Contains(config, "strict") {
		t.Errorf("expected 'strict' flag in config when Strict=true, got:\n%s", config)
	}
}

// ── Test 4: ignore-missing-schemas appears when set ───────────────────────────

func TestGenerateKubeconformConfig_IgnoreMissingSchemas(t *testing.T) {
	chart := newKubeconformChart(map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\n",
	})
	opts := KubeconformOptions{
		KubernetesVersion:    "1.29.0",
		IgnoreMissingSchemas: true,
	}

	result := GenerateKubeconformConfig(chart, opts)

	config := result.ConfigFiles[".kubeconform.yaml"]
	if !strings.Contains(config, "ignore-missing-schemas") {
		t.Errorf("expected 'ignore-missing-schemas' in config, got:\n%s", config)
	}
}

// ── Test 5: CRD schema URLs are written to config ─────────────────────────────

func TestGenerateKubeconformConfig_CRDSchemaURLs(t *testing.T) {
	chart := newKubeconformChart(map[string]string{
		"templates/module.yaml": "apiVersion: deckhouse.io/v1\nkind: Module\n",
	})
	opts := KubeconformOptions{
		KubernetesVersion: "1.29.0",
		CRDSchemaURLs: map[string]string{
			"deckhouse.io": "https://schemas.deckhouse.io/{{.Group}}/{{.Version}}/{{.Kind}}.json",
		},
	}

	result := GenerateKubeconformConfig(chart, opts)

	config := result.ConfigFiles[".kubeconform.yaml"]
	if !strings.Contains(config, "schemas.deckhouse.io") {
		t.Errorf("expected CRD schema URL in config, got:\n%s", config)
	}
}

// ── Test 6: skip kinds are written to config ──────────────────────────────────

func TestGenerateKubeconformConfig_SkipKinds(t *testing.T) {
	chart := newKubeconformChart(map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\n",
	})
	opts := KubeconformOptions{
		KubernetesVersion: "1.29.0",
		SkipKinds:         []string{"CustomResourceDefinition", "Module"},
	}

	result := GenerateKubeconformConfig(chart, opts)

	config := result.ConfigFiles[".kubeconform.yaml"]
	if !strings.Contains(config, "CustomResourceDefinition") {
		t.Errorf("expected 'CustomResourceDefinition' in skip-kinds config, got:\n%s", config)
	}
	if !strings.Contains(config, "Module") {
		t.Errorf("expected 'Module' in skip-kinds config, got:\n%s", config)
	}
}

// ── Test 7: commands contain template paths ───────────────────────────────────

func TestGenerateKubeconformConfig_CommandsContainTemplatePaths(t *testing.T) {
	chart := newKubeconformChart(map[string]string{
		"templates/deployment.yaml":  "apiVersion: apps/v1\nkind: Deployment\n",
		"templates/statefulset.yaml": "apiVersion: apps/v1\nkind: StatefulSet\n",
	})
	opts := KubeconformOptions{KubernetesVersion: "1.29.0"}

	result := GenerateKubeconformConfig(chart, opts)

	if len(result.Commands) == 0 {
		t.Fatal("expected at least one kubeconform command")
	}
	allCommands := strings.Join(result.Commands, "\n")
	if !strings.Contains(allCommands, "kubeconform") {
		t.Errorf("expected commands to contain 'kubeconform', got:\n%s", allCommands)
	}
	if !strings.Contains(allCommands, "templates/") {
		t.Errorf("expected commands to reference template paths, got:\n%s", allCommands)
	}
}

// ── Test 8: nil chart returns nil result without panic ────────────────────────

func TestGenerateKubeconformConfig_NilChart(t *testing.T) {
	opts := KubeconformOptions{KubernetesVersion: "1.29.0"}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("GenerateKubeconformConfig panicked on nil chart: %v", r)
		}
	}()

	result := GenerateKubeconformConfig(nil, opts)
	if result != nil {
		t.Error("expected nil result for nil chart input")
	}
}

// ── Test 9: InjectKubeconformConfig copy-on-write ─────────────────────────────

func TestInjectKubeconformConfig_CopyOnWrite(t *testing.T) {
	original := newKubeconformChart(map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\n",
	})
	opts := KubeconformOptions{KubernetesVersion: "1.29.0"}
	kubeResult := GenerateKubeconformConfig(original, opts)

	updated, _ := InjectKubeconformConfig(original, kubeResult)

	if updated == original {
		t.Error("InjectKubeconformConfig must return a new chart pointer (copy-on-write)")
	}
	// Original must not be mutated
	if _, ok := original.Templates[".kubeconform.yaml"]; ok {
		t.Error("original chart must not be modified by InjectKubeconformConfig")
	}
}

// ── Test 10: NOTESTxt contains kubeconform instructions ──────────────────────

func TestGenerateKubeconformConfig_NOTESTxtPresent(t *testing.T) {
	chart := newKubeconformChart(map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\n",
	})
	opts := KubeconformOptions{KubernetesVersion: "1.29.0"}

	result := GenerateKubeconformConfig(chart, opts)

	if result.NOTESTxt == "" {
		t.Error("expected non-empty NOTESTxt with usage instructions")
	}
	if !strings.Contains(result.NOTESTxt, "kubeconform") {
		t.Errorf("expected NOTESTxt to mention kubeconform, got:\n%s", result.NOTESTxt)
	}
}
