package generator

import (
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/helm"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// GenerateDeckhouseModule transforms a standard Helm chart into a Deckhouse module structure.
// It adds the deckhouse_lib_helm dependency, OpenAPI schemas, images/ and hooks/ directories,
// and injects helm_lib helpers into templates.
// When values is nil, the config schema is derived from the chart's values.yaml.
func GenerateDeckhouseModule(chart *types.GeneratedChart, values map[string]interface{}) *types.GeneratedChart {
	result := *chart

	if values == nil {
		_ = yaml.Unmarshal([]byte(chart.ValuesYAML), &values)
	}

	// Modify Chart.yaml to add the deckhouse_lib_helm dependency
	result.ChartYAML = injectHelmLibDep(chart.ChartYAML)

	// Inject helm_lib includes into templates
	result.Templates = injectHelmLibIncludes(chart.Templates)

	// Generate external files
	result.ExternalFiles = append(append([]types.ExternalFileInfo{}, chart.ExternalFiles...),
		generateModuleExternalFiles(chart.Name, values)...)

	return &result
}

// helmLib is the Deckhouse library chart providing the helm_lib_* helpers
// (chart name as published in https://deckhouse.github.io/lib-helm).
var helmLib = helm.Dependency{
	Name:       "deckhouse_lib_helm",
	Version:    "~1",
	Repository: "https://deckhouse.github.io/lib-helm",
}

func injectHelmLibDep(chartYAML string) string {
	for _, d := range declaredDependencies(chartYAML) {
		if d.Name == helmLib.Name {
			return chartYAML
		}
	}
	return addChartDependencies(chartYAML, []helm.Dependency{helmLib})
}

func injectHelmLibIncludes(templates map[string]string) map[string]string {
	result := make(map[string]string, len(templates))

	for path, content := range templates {
		if !strings.Contains(content, "helm_lib_module_labels") {
			content = addHelmLibComment(content)
		}
		result[path] = content
	}

	return result
}

func addHelmLibComment(content string) string {
	header := `{{- /* Deckhouse module: use helm_lib helpers */ -}}
{{- /* {{ include "helm_lib_module_labels" . }} for labels */ -}}
{{- /* {{ include "helm_lib_module_image" (list . "imageName") }} for images */ -}}
`
	return header + content
}

func generateModuleExternalFiles(chartName string, values map[string]interface{}) []types.ExternalFileInfo {
	files := make([]types.ExternalFileInfo, 0, 5)

	// openapi/config-values.yaml — public config schema
	configSchema := GenerateOpenAPISchema(values)
	files = append(files, types.ExternalFileInfo{
		Path:    "openapi/config-values.yaml",
		Content: configSchema,
	})

	// openapi/values.yaml — internal values schema
	internalSchema := "type: object\nadditionalProperties: true\nproperties:\n  internal:\n    type: object\n"
	files = append(files, types.ExternalFileInfo{
		Path:    "openapi/values.yaml",
		Content: internalSchema,
	})

	// images/README.md
	files = append(files, types.ExternalFileInfo{
		Path:    "images/README.md",
		Content: fmt.Sprintf("# Images for %s\n\nPlace Dockerfile directories here.\n", chartName),
	})

	// hooks/README.md
	files = append(files, types.ExternalFileInfo{
		Path:    "hooks/README.md",
		Content: fmt.Sprintf("# Hooks for %s\n\nPlace Go or Shell hooks here.\n", chartName),
	})

	return files
}
