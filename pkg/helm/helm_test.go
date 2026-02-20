package helm

import (
	"strings"
	"testing"
)

// ── GenerateChartYAML ─────────────────────────────────────────────────────────

func TestGenerateChartYAML_Defaults(t *testing.T) {
	out := GenerateChartYAML(ChartMetadata{Name: "myapp"})

	for _, want := range []string{
		"apiVersion: v2",
		"name: myapp",
		"type: application",
		"version: 0.1.0",
		"appVersion: 1.0.0",
		"description: A Helm chart for myapp",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestGenerateChartYAML_Full(t *testing.T) {
	out := GenerateChartYAML(ChartMetadata{
		Name:        "full-chart",
		Version:     "2.0.0",
		APIVersion:  "v2",
		AppVersion:  "3.0.0",
		Description: "Full chart",
		Type:        "library",
		Keywords:    []string{"k8s", "helm"},
		Home:        "https://example.com",
		Sources:     []string{"https://github.com/example"},
		Maintainers: []Maintainer{{Name: "Dev", Email: "dev@example.com", URL: "https://dev.example.com"}},
		Icon:        "https://example.com/icon.png",
		KubeVersion: ">=1.22",
		Dependencies: []Dependency{{
			Name:       "redis",
			Version:    "17.0.0",
			Repository: "https://charts.bitnami.com/bitnami",
			Condition:  "redis.enabled",
			Tags:       []string{"cache"},
			Alias:      "redis-cache",
		}},
	})

	for _, want := range []string{
		"type: library",
		"version: 2.0.0",
		"appVersion: 3.0.0",
		"home: https://example.com",
		"kubeVersion: >=1.22",
		"icon: https://example.com/icon.png",
		"- k8s",
		"- name: Dev",
		"email: dev@example.com",
		"url: https://dev.example.com",
		"- name: redis",
		"condition: redis.enabled",
		"alias: redis-cache",
		"- cache",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in full output", want)
		}
	}
}

func TestGenerateNOTES(t *testing.T) {
	out := GenerateNOTES("myapp", []string{"frontend", "backend"})
	for _, want := range []string{"myapp", "frontend", "backend", "kubectl get all"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in NOTES", want)
		}
	}
}

func TestGenerateNOTES_NoServices(t *testing.T) {
	out := GenerateNOTES("myapp", nil)
	if strings.Contains(out, "Installed services") {
		t.Error("should not show services section when empty")
	}
}

func TestGenerateREADME(t *testing.T) {
	out := GenerateREADME(ChartMetadata{Name: "myapp"}, []string{"web"})
	for _, want := range []string{"# myapp", "helm install", "web", "helm uninstall"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in README", want)
		}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func TestGenerateHelpers(t *testing.T) {
	out := GenerateHelpers("myapp")
	for _, want := range []string{
		`define "myapp.name"`,
		`define "myapp.fullname"`,
		`define "myapp.chart"`,
		`define "myapp.labels"`,
		`define "myapp.selectorLabels"`,
		`define "myapp.serviceAccountName"`,
		`define "myapp.imagePullSecrets"`,
		`define "myapp.image"`,
		`define "myapp.isDeckhouseAvailable"`,
		`define "myapp.annotations"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in helpers", want)
		}
	}
}

func TestGenerateHelmIgnore(t *testing.T) {
	out := GenerateHelmIgnore()
	if !strings.Contains(out, ".git/") || !strings.Contains(out, ".DS_Store") {
		t.Error("helmignore missing expected patterns")
	}
}

func TestGenerateValuesYAMLComment(t *testing.T) {
	out := GenerateValuesYAMLComment("myapp")
	if !strings.Contains(out, "myapp") {
		t.Error("values comment missing chart name")
	}
}

// ── ValuesBuilder ─────────────────────────────────────────────────────────────

func TestNewValuesBuilder(t *testing.T) {
	b := NewValuesBuilder()
	m := b.BuildMap()
	if len(m) != 0 {
		t.Errorf("new builder should be empty, got %d keys", len(m))
	}
}

func TestValuesBuilder_SetGlobal(t *testing.T) {
	b := NewValuesBuilder()
	b.SetGlobal("imageRegistry", "docker.io")

	m := b.BuildMap()
	global, ok := m["global"].(map[string]interface{})
	if !ok {
		t.Fatal("global not a map")
	}
	if global["imageRegistry"] != "docker.io" {
		t.Errorf("imageRegistry = %v; want docker.io", global["imageRegistry"])
	}
}

func TestValuesBuilder_AddService(t *testing.T) {
	b := NewValuesBuilder()
	b.AddService("web", map[string]interface{}{"replicas": 3})

	m := b.BuildMap()
	services := m["services"].(map[string]interface{})
	web := services["web"].(map[string]interface{})

	if web["enabled"] != true {
		t.Error("enabled flag not added automatically")
	}
	if web["replicas"] != 3 {
		t.Errorf("replicas = %v; want 3", web["replicas"])
	}
}

func TestValuesBuilder_SetValue(t *testing.T) {
	b := NewValuesBuilder()
	b.SetValue("a.b.c", "deep")

	val, ok := b.GetValue("a.b.c")
	if !ok || val != "deep" {
		t.Errorf("GetValue(a.b.c) = %v, %v; want deep, true", val, ok)
	}
}

func TestValuesBuilder_GetValue_NotFound(t *testing.T) {
	b := NewValuesBuilder()
	_, ok := b.GetValue("missing.path")
	if ok {
		t.Error("GetValue should return false for missing path")
	}
}

func TestValuesBuilder_MergeValues(t *testing.T) {
	b := NewValuesBuilder()
	b.SetValue("a.x", "original")
	b.MergeValues(map[string]interface{}{
		"a": map[string]interface{}{"y": "merged"},
		"b": "new",
	})

	m := b.BuildMap()
	a := m["a"].(map[string]interface{})
	if a["x"] != "original" {
		t.Error("merge should preserve existing keys")
	}
	if a["y"] != "merged" {
		t.Error("merge should add new keys")
	}
	if m["b"] != "new" {
		t.Error("merge should add top-level keys")
	}
}

func TestValuesBuilder_Build(t *testing.T) {
	b := NewValuesBuilder()
	b.AddService("web", map[string]interface{}{"replicas": 2})

	out, err := b.Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if !strings.Contains(out, "global:") {
		t.Error("Build should add default globals")
	}
	if !strings.Contains(out, "services:") {
		t.Error("Build output should contain services")
	}
}

func TestFormatValuesForService(t *testing.T) {
	out := FormatValuesForService("web", map[string]interface{}{"replicas": 3})
	if out["enabled"] != true {
		t.Error("FormatValuesForService should ensure enabled flag")
	}
	if out["replicas"] != 3 {
		t.Error("FormatValuesForService should preserve values")
	}
}

func TestFormatValuesForService_ExistingEnabled(t *testing.T) {
	out := FormatValuesForService("web", map[string]interface{}{"enabled": false})
	if out["enabled"] != false {
		t.Error("should preserve explicit enabled=false")
	}
}

func TestGenerateValuesSchema(t *testing.T) {
	out := GenerateValuesSchema([]string{"web", "api"})
	if !strings.Contains(out, "web") || !strings.Contains(out, "api") {
		t.Error("schema should contain service names")
	}
	if !strings.Contains(out, "imageRegistry") {
		t.Error("schema should contain global properties")
	}
}
