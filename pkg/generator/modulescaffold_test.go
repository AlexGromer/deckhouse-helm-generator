package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func makeTestChart(name string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name:      name,
		ChartYAML: "apiVersion: v2\nname: " + name + "\nversion: 0.1.0\n",
		ValuesYAML: "# Default values\nglobal: {}\n",
		Templates: map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\n",
		},
		Helpers: "{{- define \"" + name + ".labels\" -}}app: " + name + "{{- end -}}\n",
	}
}

func TestModuleScaffold_ChartYAML_HasHelmLibDep(t *testing.T) {
	chart := makeTestChart("mymodule")
	values := map[string]interface{}{"enabled": true}

	result := GenerateDeckhouseModule(chart, values)

	if !strings.Contains(result.ChartYAML, "helm_lib") {
		t.Error("Expected helm_lib dependency in Chart.yaml")
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

func TestModuleScaffold_Templates_HelmLib(t *testing.T) {
	chart := makeTestChart("mymodule")
	result := GenerateDeckhouseModule(chart, map[string]interface{}{})

	found := false
	for _, content := range result.Templates {
		if strings.Contains(content, "helm_lib_module_labels") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected helm_lib_module_labels in at least one template")
	}
}

func TestModuleScaffold_Templates_ModuleImage(t *testing.T) {
	chart := makeTestChart("mymodule")
	result := GenerateDeckhouseModule(chart, map[string]interface{}{})

	found := false
	for _, content := range result.Templates {
		if strings.Contains(content, "helm_lib_module_image") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected helm_lib_module_image in at least one template")
	}
}

func TestModuleScaffold_DefaultDisabled(t *testing.T) {
	opts := Options{}
	if opts.DeckhouseModule {
		t.Error("Expected DeckhouseModule to default to false")
	}
}
