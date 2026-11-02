package generator

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func makeTestChart(name string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name:       name,
		ChartYAML:  "apiVersion: v2\nname: " + name + "\nversion: 0.1.0\n",
		ValuesYAML: "# Default values\nglobal: {}\n",
		Templates: map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\n",
		},
		Helpers: "{{- define \"" + name + ".labels\" -}}app: " + name + "{{- end -}}\n",
	}
}

// makeHelmLibChart is a chart whose template really includes a helm_lib helper.
func makeHelmLibChart(name string) *types.GeneratedChart {
	chart := makeTestChart(name)
	chart.Templates = map[string]string{
		"templates/deployment.yaml": "metadata:\n  labels:\n    {{- include \"helm_lib_module_labels\" (list . (dict \"app\" \"x\")) | nindent 4 }}\n",
	}
	return chart
}

func TestModuleScaffold_ChartYAML_HasHelmLibDep(t *testing.T) {
	result := GenerateDeckhouseModule(makeHelmLibChart("mymodule"), map[string]interface{}{"enabled": true})

	if !strings.Contains(result.ChartYAML, "name: deckhouse_lib_helm") {
		t.Error("Expected deckhouse_lib_helm dependency in Chart.yaml when a template includes helm_lib_*")
	}
}

// S2-app-F4: the dependency made helm lint/template fail until "helm dependency
// build" although no template used helm_lib_*.
func TestModuleScaffold_ChartYAML_NoDepWhenHelmLibUnused(t *testing.T) {
	chart := makeTestChart("mymodule")
	result := GenerateDeckhouseModule(chart, map[string]interface{}{"enabled": true})

	if strings.Contains(result.ChartYAML, "deckhouse_lib_helm") || strings.Contains(result.ChartYAML, "dependencies:") {
		t.Errorf("Chart.yaml must not declare deckhouse_lib_helm when no template uses helm_lib_*:\n%s", result.ChartYAML)
	}
	if result.ChartYAML != chart.ChartYAML {
		t.Errorf("Chart.yaml changed without a dependency to add:\n%s", result.ChartYAML)
	}
	if result.Notes != chart.Notes {
		t.Errorf("Notes changed without a dependency to add: %q", result.Notes)
	}
}

func TestModuleScaffold_HelmLibInCommentOnly_NoDep(t *testing.T) {
	chart := makeTestChart("mymodule")
	chart.Templates["templates/deployment.yaml"] = "{{- /* note: {{ include \"helm_lib_module_labels\" . }} */ -}}\nkind: Deployment\n"
	result := GenerateDeckhouseModule(chart, nil)

	if strings.Contains(result.ChartYAML, "deckhouse_lib_helm") {
		t.Errorf("a helm_lib_* mention in a template comment is not a use:\n%s", result.ChartYAML)
	}
}

func TestModuleScaffold_HelmLibInHelpers_HasDep(t *testing.T) {
	chart := makeTestChart("mymodule")
	chart.Helpers = "{{- define \"mymodule.labels\" -}}{{ include \"helm_lib_module_labels\" (list .) }}{{- end -}}\n"
	result := GenerateDeckhouseModule(chart, nil)

	if !strings.Contains(result.ChartYAML, "name: deckhouse_lib_helm") {
		t.Errorf("a helm_lib_* include in _helpers.tpl needs the dependency:\n%s", result.ChartYAML)
	}
}

// With the dependency, Chart.yaml cannot be linted or rendered before
// "helm dependency update": the user is told so in NOTES.
func TestModuleScaffold_NotesMentionDependencyUpdate(t *testing.T) {
	chart := makeHelmLibChart("mymodule")
	chart.Notes = "Installed {{ .Chart.Name }}.\n"
	result := GenerateDeckhouseModule(chart, nil)

	if !strings.HasPrefix(result.Notes, chart.Notes) {
		t.Errorf("existing NOTES must be kept:\n%s", result.Notes)
	}
	if !strings.Contains(result.Notes, "helm dependency update") {
		t.Errorf("NOTES must mention 'helm dependency update':\n%s", result.Notes)
	}
	if again := GenerateDeckhouseModule(result, nil); again.Notes != result.Notes {
		t.Errorf("second application changed NOTES:\n%s", again.Notes)
	}
}

func TestModuleScaffold_DoesNotMutateInput(t *testing.T) {
	chart := makeHelmLibChart("mymodule")
	chartYAML, templates := chart.ChartYAML, chart.Templates["templates/deployment.yaml"]
	GenerateDeckhouseModule(chart, nil)
	if chart.ChartYAML != chartYAML || chart.Templates["templates/deployment.yaml"] != templates {
		t.Error("input chart was modified")
	}
}

func TestModuleScaffold_ChartYAML_APIVersion(t *testing.T) {
	chart := makeTestChart("mymodule")
	values := map[string]interface{}{}

	result := GenerateDeckhouseModule(chart, values)

	if !strings.Contains(result.ChartYAML, "apiVersion: v2") {
		t.Error("Expected apiVersion: v2 in Chart.yaml")
	}
}

func TestModuleScaffold_OpenAPI_ConfigValues(t *testing.T) {
	chart := makeTestChart("mymodule")
	values := map[string]interface{}{"enabled": true}

	result := GenerateDeckhouseModule(chart, values)

	found := false
	for _, ef := range result.ExternalFiles {
		if ef.Path == "openapi/config-values.yaml" {
			found = true
			if ef.Content == "" {
				t.Error("Expected non-empty config-values.yaml")
			}
			break
		}
	}
	if !found {
		t.Error("Expected openapi/config-values.yaml in ExternalFiles")
	}
}

func TestModuleScaffold_OpenAPI_Values(t *testing.T) {
	chart := makeTestChart("mymodule")
	values := map[string]interface{}{}

	result := GenerateDeckhouseModule(chart, values)

	found := false
	for _, ef := range result.ExternalFiles {
		if ef.Path == "openapi/values.yaml" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected openapi/values.yaml in ExternalFiles")
	}
}

func TestModuleScaffold_OpenAPI_Structure(t *testing.T) {
	chart := makeTestChart("mymodule")
	values := map[string]interface{}{"logLevel": "info"}

	result := GenerateDeckhouseModule(chart, values)

	for _, ef := range result.ExternalFiles {
		if ef.Path == "openapi/config-values.yaml" {
			if !strings.Contains(ef.Content, "type: object") {
				t.Error("Expected 'type: object' in OpenAPI schema")
			}
			if !strings.Contains(ef.Content, "properties:") {
				t.Error("Expected 'properties:' in OpenAPI schema")
			}
			return
		}
	}
	t.Error("openapi/config-values.yaml not found")
}

func TestModuleScaffold_ImagesDir(t *testing.T) {
	chart := makeTestChart("mymodule")
	result := GenerateDeckhouseModule(chart, map[string]interface{}{})

	found := false
	for _, ef := range result.ExternalFiles {
		if ef.Path == "images/README.md" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected images/README.md in ExternalFiles")
	}
}

func TestModuleScaffold_HooksDir(t *testing.T) {
	chart := makeTestChart("mymodule")
	result := GenerateDeckhouseModule(chart, map[string]interface{}{})

	found := false
	for _, ef := range result.ExternalFiles {
		if ef.Path == "hooks/README.md" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected hooks/README.md in ExternalFiles")
	}
}

// The templates are passed through: advertising helpers of a library chart that is
// not a dependency would only mislead.
func TestModuleScaffold_Templates_Unchanged(t *testing.T) {
	chart := makeTestChart("mymodule")
	result := GenerateDeckhouseModule(chart, map[string]interface{}{})

	for path, content := range chart.Templates {
		if result.Templates[path] != content {
			t.Errorf("template %s changed:\n%s", path, result.Templates[path])
		}
	}
}

func TestModuleScaffold_DefaultDisabled(t *testing.T) {
	opts := Options{}
	if opts.DeckhouseModule {
		t.Error("Expected DeckhouseModule to default to false")
	}
}

func TestGenerateDeckhouseModule_MergesIntoExistingDependencies(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:      "app",
		ChartYAML: "apiVersion: v2\nname: app\nversion: 0.1.0\ndependencies:\n  - name: redis\n    version: 1.0.0\n    repository: https://example.com\n",
		Templates: map[string]string{"templates/a.yaml": "{{ include \"helm_lib_module_labels\" . }}"},
	}
	result := GenerateDeckhouseModule(chart, nil)

	if n := strings.Count(result.ChartYAML, "dependencies:"); n != 1 {
		t.Fatalf("dependencies key appears %d times:\n%s", n, result.ChartYAML)
	}
	names := map[string]bool{}
	for _, d := range declaredDependencies(result.ChartYAML) {
		names[d.Name] = true
	}
	if !names["redis"] || !names["deckhouse_lib_helm"] {
		t.Errorf("dependencies = %v, want redis and deckhouse_lib_helm", names)
	}
	if again := GenerateDeckhouseModule(result, nil); again.ChartYAML != result.ChartYAML {
		t.Errorf("second application changed Chart.yaml:\n%s", again.ChartYAML)
	}
}
