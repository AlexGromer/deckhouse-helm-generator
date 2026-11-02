package generator

import (
	"fmt"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/helm"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// GenerateDeckhouseModule transforms a standard Helm chart into a Deckhouse module structure.
// It adds OpenAPI schemas and images/ and hooks/ directories. The deckhouse_lib_helm
// dependency is added only when a template or helper includes a helm_lib_* helper:
// the dependency makes helm lint/template fail until "helm dependency update" has
// run, which is pointless for a chart that does not use the library. In that case
// the NOTES say to run it.
// When values is nil, the config schema is derived from the chart's values.yaml.
func GenerateDeckhouseModule(chart *types.GeneratedChart, values map[string]interface{}) *types.GeneratedChart {
	result := *chart

	if values == nil {
		_ = yaml.Unmarshal([]byte(chart.ValuesYAML), &values)
	}

	if usesHelmLib(chart) {
		result.ChartYAML = injectHelmLibDep(chart.ChartYAML)
		result.Notes = addDependencyUpdateNote(chart.Notes)
	}

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

var (
	// templateComment matches a Go-template comment, {{/* ... */}} with optional trim markers.
	templateComment = regexp.MustCompile(`(?s)\{\{-?\s*/\*.*?\*/\s*-?\}\}`)
	// helmLibCall matches an include/template call of a helm_lib_* helper.
	helmLibCall = regexp.MustCompile(`\b(?:include|template)\s+"helm_lib_[A-Za-z0-9_]+"`)
)

// usesHelmLib reports whether a template or the helpers include a helm_lib_* helper.
// Mentions inside template comments do not count.
func usesHelmLib(chart *types.GeneratedChart) bool {
	uses := func(content string) bool {
		return helmLibCall.MatchString(templateComment.ReplaceAllString(content, ""))
	}
	if uses(chart.Helpers) {
		return true
	}
	for _, content := range chart.Templates {
		if uses(content) {
			return true
		}
	}
	return false
}

func injectHelmLibDep(chartYAML string) string {
	for _, d := range declaredDependencies(chartYAML) {
		if d.Name == helmLib.Name {
			return chartYAML
		}
	}
	return addChartDependencies(chartYAML, []helm.Dependency{helmLib})
}

// dependencyUpdateNote tells how to fetch the library chart the module depends on.
const dependencyUpdateNote = "This chart depends on deckhouse_lib_helm: run \"helm dependency update\" in the chart directory\n" +
	"before helm lint, helm template or helm install.\n"

func addDependencyUpdateNote(notes string) string {
	if strings.Contains(notes, "helm dependency update") {
		return notes
	}
	if notes != "" && !strings.HasSuffix(notes, "\n") {
		notes += "\n"
	}
	return notes + dependencyUpdateNote
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
