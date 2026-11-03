package helm

import (
	"encoding/json"
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
	out := GenerateNOTES("myapp", []string{"frontend", "backend"}, NOTESContext{})
	for _, want := range []string{"myapp", "frontend", "backend", "kubectl get all"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in NOTES", want)
		}
	}
}

func TestGenerateNOTES_NoServices(t *testing.T) {
	out := GenerateNOTES("myapp", nil, NOTESContext{})
	if strings.Contains(out, "Installed services") {
		t.Error("should not show services section when empty")
	}
}

func TestGenerateNOTES_WithLoadBalancer(t *testing.T) {
	out := GenerateNOTES("myapp", []string{"web"}, NOTESContext{
		ServiceTypes: []string{"LoadBalancer"},
	})
	if !strings.Contains(out, "LoadBalancer IP") {
		t.Error("NOTES should contain LoadBalancer IP retrieval section")
	}
	if !strings.Contains(out, "jsonpath") {
		t.Error("NOTES should contain jsonpath command for LB IP")
	}
	// Should NOT have port-forward when LB is present
	if strings.Contains(out, "port-forward") {
		t.Error("NOTES should not suggest port-forward when LoadBalancer is available")
	}
}

func TestGenerateNOTES_WithIngress(t *testing.T) {
	out := GenerateNOTES("myapp", []string{"web"}, NOTESContext{
		HasIngress: true,
	})
	if !strings.Contains(out, "application URL") {
		t.Error("NOTES should contain Ingress URL section")
	}
	if !strings.Contains(out, "get ingress") {
		t.Error("NOTES should contain kubectl get ingress command")
	}
	// Should NOT have port-forward when Ingress is present
	if strings.Contains(out, "port-forward") {
		t.Error("NOTES should not suggest port-forward when Ingress is available")
	}
}

func TestGenerateNOTES_DefaultPortForward(t *testing.T) {
	out := GenerateNOTES("myapp", []string{"web"}, NOTESContext{
		ServiceTypes: []string{"ClusterIP"},
	})
	if !strings.Contains(out, "port-forward") {
		t.Error("NOTES should contain port-forward section for ClusterIP services")
	}
	if !strings.Contains(out, "svc/myapp") {
		t.Error("NOTES port-forward should reference the chart service name")
	}
	// Should NOT have LB or Ingress sections
	if strings.Contains(out, "LoadBalancer IP") {
		t.Error("NOTES should not contain LB section for ClusterIP-only services")
	}
}

func TestGenerateNOTES_WithAuth(t *testing.T) {
	out := GenerateNOTES("myapp", []string{"web"}, NOTESContext{
		HasAuth: true,
	})
	if !strings.Contains(out, "Authentication is enabled") {
		t.Error("NOTES should contain auth section when HasAuth is true")
	}
	if !strings.Contains(out, "get secret") {
		t.Error("NOTES should contain secret check command when auth is enabled")
	}
}

func TestGenerateREADME(t *testing.T) {
	out := GenerateREADME(ChartMetadata{Name: "myapp"}, map[string]interface{}{
		"services":  map[string]interface{}{"web": map[string]interface{}{"enabled": true, "deployment": map[string]interface{}{"replicas": 2}}},
		"global":    map[string]interface{}{"imagePullSecrets": []interface{}{}},
		"podLabels": map[string]interface{}{"app.kubernetes.io/name": "web"},
	})
	for _, want := range []string{"# myapp", "helm install", "helm uninstall",
		"| `services.web.enabled` | `true` |", "| `services.web.deployment.replicas` | `2` |",
		"| `global.imagePullSecrets` | `[]` |", "`podLabels.app\\.kubernetes\\.io/name`"} {
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

func TestValuesBuilder_BuildFlat_PathComments(t *testing.T) {
	b := NewValuesBuilder()
	b.SetValue("image.repository", "nginx")
	b.SetValue("image.tag", "latest")
	b.SetValue("service.type", "ClusterIP")
	b.SetValue("service.port", 8080)

	out, err := b.BuildFlat()
	if err != nil {
		t.Fatalf("BuildFlat() error: %v", err)
	}

	// Check that leaf values get path annotations.
	if !strings.Contains(out, "# image.repository") {
		t.Errorf("expected '# image.repository' path comment, got:\n%s", out)
	}
	if !strings.Contains(out, "# image.tag") {
		t.Errorf("expected '# image.tag' path comment, got:\n%s", out)
	}
	if !strings.Contains(out, "# service.type") {
		t.Errorf("expected '# service.type' path comment, got:\n%s", out)
	}
	if !strings.Contains(out, "# service.port") {
		t.Errorf("expected '# service.port' path comment, got:\n%s", out)
	}

	// Parent keys should NOT have path comments on the same line.
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "image:") && strings.Contains(trimmed, "#") {
			t.Errorf("parent key 'image:' should not have inline comment, got: %s", line)
		}
	}
}

func TestValuesBuilder_BuildFlat_VsBuild(t *testing.T) {
	b := NewValuesBuilder()
	b.SetValue("app.name", "test")
	b.SetValue("app.version", "1.0")

	nested, err := b.Build()
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	flat, err := b.BuildFlat()
	if err != nil {
		t.Fatalf("BuildFlat() error: %v", err)
	}

	// Both should contain the YAML structure.
	if !strings.Contains(nested, "app:") {
		t.Error("Build should contain app key")
	}
	if !strings.Contains(flat, "app:") {
		t.Error("BuildFlat should contain app key")
	}

	// Only flat should have path comments.
	if strings.Contains(nested, "# app.name") {
		t.Error("Build() should NOT have inline path comments")
	}
	if !strings.Contains(flat, "# app.name") {
		t.Error("BuildFlat() should have inline path comments")
	}
}

func TestInferValuesSchema(t *testing.T) {
	out := InferValuesSchema(map[string]interface{}{
		"global":   map[string]interface{}{"imageRegistry": "", "imagePullSecrets": []interface{}{}},
		"services": map[string]interface{}{"web": map[string]interface{}{"enabled": true, "replicas": 2, "cpu": "100m", "extra": nil}},
	})
	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(out), &schema); err != nil {
		t.Fatalf("schema is not JSON: %v\n%s", err, out)
	}
	if schema["$schema"] != "http://json-schema.org/draft-07/schema#" || schema["type"] != "object" {
		t.Errorf("unexpected root: %v", schema)
	}
	web := schema["properties"].(map[string]interface{})["services"].(map[string]interface{})["properties"].(map[string]interface{})["web"].(map[string]interface{})["properties"].(map[string]interface{})
	if web["enabled"].(map[string]interface{})["type"] != "boolean" {
		t.Error("booleans must stay booleans")
	}
	for _, scalar := range []string{"replicas", "cpu"} {
		if types := web[scalar].(map[string]interface{})["type"]; len(types.([]interface{})) != 2 {
			t.Errorf("%s must accept string or number, got %v", scalar, types)
		}
	}
	if len(web["extra"].(map[string]interface{})) != 0 {
		t.Error("null defaults must accept anything")
	}
	pull := schema["properties"].(map[string]interface{})["global"].(map[string]interface{})["properties"].(map[string]interface{})["imagePullSecrets"].(map[string]interface{})
	if pull["type"] != "array" {
		t.Error("lists must stay lists")
	}
}

func TestValuesBuilder_Build(t *testing.T) {
	b := NewValuesBuilder()
	b.SetValue("services.web", map[string]interface{}{"replicas": 2})

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

func TestValuesBuilder_BuildFlat(t *testing.T) {
	b := NewValuesBuilder()
	b.SetGlobal("imageRegistry", "registry.example.com")
	b.SetValue("services.web", map[string]interface{}{
		"replicas": 2,
		"deployment": map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"name": "app",
					"image": map[string]interface{}{
						"repository": "nginx",
						"tag":        "1.21",
					},
				},
			},
		},
	})

	out, err := b.BuildFlat()
	if err != nil {
		t.Fatalf("BuildFlat() error: %v", err)
	}

	// Should still contain the nested structure.
	if !strings.Contains(out, "global:") {
		t.Error("BuildFlat should contain global section")
	}
	if !strings.Contains(out, "services:") {
		t.Error("BuildFlat should contain services section")
	}

	// Leaf values should have inline dot-notation path comments.
	if !strings.Contains(out, "# global.imageRegistry") {
		t.Errorf("BuildFlat should add inline path comment for global.imageRegistry, got:\n%s", out)
	}
}

func TestGenerateChartYAML_OmitsEmptyRepository(t *testing.T) {
	out := GenerateChartYAML(ChartMetadata{
		Name:         "parent",
		Dependencies: []Dependency{{Name: "child", Version: "1.0.0"}},
	})
	if strings.Contains(out, "repository:") {
		t.Errorf("empty repository must be omitted:\n%s", out)
	}
	if !strings.Contains(out, "- name: child") {
		t.Errorf("dependency missing:\n%s", out)
	}
}

func TestGenerateREADME_InstallHasNamespace(t *testing.T) {
	out := GenerateREADME(ChartMetadata{Name: "myapp"}, nil)
	if !strings.Contains(out, "helm install my-release ./myapp --namespace <namespace>") {
		t.Errorf("install hint without --namespace:\n%s", out)
	}
}

func TestInferValuesSchema_ContainersAreObjectsByName(t *testing.T) {
	out := InferValuesSchema(map[string]interface{}{
		"services": map[string]interface{}{"web": map[string]interface{}{"deployment": map[string]interface{}{
			"containers": map[string]interface{}{
				"app": map[string]interface{}{"image": map[string]interface{}{"repository": "nginx", "tag": "1"}},
			},
			"containerOrder": []interface{}{"app"},
			"initContainers": map[string]interface{}{
				"migrate": map[string]interface{}{"image": map[string]interface{}{"repository": "nginx"}},
			},
			"initContainerOrder": []interface{}{"migrate"},
		}}},
	})
	var schema map[string]interface{}
	if err := json.Unmarshal([]byte(out), &schema); err != nil {
		t.Fatalf("schema is not JSON: %v\n%s", err, out)
	}
	dep := schema["properties"].(map[string]interface{})["services"].(map[string]interface{})["properties"].(map[string]interface{})["web"].(map[string]interface{})["properties"].(map[string]interface{})["deployment"].(map[string]interface{})["properties"].(map[string]interface{})
	for _, key := range []string{"containers", "initContainers"} {
		c := dep[key].(map[string]interface{})
		if c["type"] != "object" {
			t.Errorf("%s must be an object, got %v", key, c["type"])
		}
		extra, ok := c["additionalProperties"].(map[string]interface{})
		if !ok || extra["type"] != "object" {
			t.Errorf("%s must allow more containers as objects (additionalProperties), got %v", key, c["additionalProperties"])
		}
		if _, ok := c["properties"].(map[string]interface{}); !ok {
			t.Errorf("%s must keep the properties of its default containers", key)
		}
	}
	for _, key := range []string{"containerOrder", "initContainerOrder"} {
		o := dep[key].(map[string]interface{})
		items, _ := o["items"].(map[string]interface{})
		if o["type"] != "array" || items["type"] != "string" {
			t.Errorf("%s must be a list of strings, got %v", key, o)
		}
	}
}

func TestGenerateHelpers_NamePrefix(t *testing.T) {
	out := GenerateHelpersWithRefs("myapp", map[string][]string{
		"Service":   {"web", "api"},
		"ConfigMap": {"cfg", `odd"name`},
	})
	for _, want := range []string{
		`define "myapp.objname"`,
		`define "myapp.ref"`,
		`define "myapp.refs"`,
		`define "myapp.refwalk"`,
		`define "myapp.namePrefix"`,
		// Kinds and names sorted, names JSON-quoted.
		"ConfigMap: [\"cfg\",\"odd\\\"name\"]\nService: [\"api\",\"web\"]\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("helpers lack %q", want)
		}
	}

	// Without objects the set is an empty map; GenerateHelpers is the same.
	empty := GenerateHelpers("myapp")
	if !strings.Contains(empty, "define \"myapp.inChart\" -}}\n{}\n{{- end }}") {
		t.Errorf("empty set is not an empty map:\n%s", empty)
	}
	if empty != GenerateHelpersWithRefs("myapp", nil) {
		t.Error("GenerateHelpers differs from GenerateHelpersWithRefs(nil)")
	}
}

func TestGenerateLibraryHelpers_IndependentOfInput(t *testing.T) {
	lib := GenerateLibraryHelpers("platform-lib")
	if strings.Contains(lib, "define \"platform-lib.inChart\"") {
		t.Error("the library defines the object names; the service chart must")
	}
	if !strings.Contains(lib, "include (printf \"%s.inChart\" $root.Chart.Name) $root") {
		t.Error("the library ref helper does not read the names from the calling chart")
	}
	for _, name := range []string{"name", "fullname", "labels", "namePrefix", "objname", "ref", "refone", "refwalk", "refs"} {
		if n := strings.Count(lib, "define \"platform-lib."+name+"\""); n != 1 {
			t.Errorf("define platform-lib.%s count = %d, want 1", name, n)
		}
	}

	in := GenerateInChartTemplate("web", map[string][]string{"Service": {"web"}, "ConfigMap": {"web-cfg", "a"}})
	want := "{{- define \"web.inChart\" -}}\nConfigMap: [\"a\",\"web-cfg\"]\nService: [\"web\"]\n{{- end }}\n"
	if !strings.Contains(in, want) {
		t.Errorf("GenerateInChartTemplate =\n%s\nwant to contain\n%s", in, want)
	}
}

func TestGenerateNOTES_RolloutStatusAndDriftCheck(t *testing.T) {
	out := GenerateNOTES("myapp", []string{"web"}, NOTESContext{
		Deployments:  []string{"web", "worker"},
		HasWorkloads: true,
	})
	for _, want := range []string{
		`kubectl -n {{ .Release.Namespace }} rollout status deploy/{{ include "myapp.objname" (list $ "web") }} --timeout=30m`,
		`kubectl -n {{ .Release.Namespace }} rollout status deploy/{{ include "myapp.objname" (list $ "worker") }} --timeout=30m`,
		`helm get manifest {{ .Release.Name }} -n {{ .Release.Namespace }} | kubectl -n {{ .Release.Namespace }} diff -f -`,
		"helm --wait may not wait",
		"exit code 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("NOTES lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Recreate") {
		t.Errorf("helm --wait waits for the new pods of a Recreate Deployment, NOTES must not list it among the cases that return early:\n%s", out)
	}
	if strings.Index(out, "rollout status deploy/") > strings.Index(out, "helm get manifest") {
		t.Error("rollout status comes before the drift check")
	}
}

func TestGenerateNOTES_DriftCheckWithoutDeployments(t *testing.T) {
	out := GenerateNOTES("myapp", []string{"db"}, NOTESContext{HasWorkloads: true})
	if strings.Contains(out, "rollout status") {
		t.Error("no Deployment, no rollout status")
	}
	if !strings.Contains(out, "helm get manifest") {
		t.Error("a chart with workloads gets the drift check")
	}
}

func TestGenerateNOTES_NoWorkloadsNoCommands(t *testing.T) {
	out := GenerateNOTES("myapp", []string{"cfg"}, NOTESContext{})
	for _, bad := range []string{"rollout status", "helm get manifest"} {
		if strings.Contains(out, bad) {
			t.Errorf("a chart without workloads must not suggest %q", bad)
		}
	}
}
