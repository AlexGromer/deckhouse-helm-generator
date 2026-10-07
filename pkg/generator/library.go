package generator

import (
	"context"
	"fmt"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/helm"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// LibraryGenerator generates a Helm library chart holding the shared helpers
// (names, labels, selector labels, image, ...) plus one application chart per
// service group whose templates use those shared helpers instead of carrying
// their own _helpers.tpl — the pattern of common library charts such as
// bitnami/common.
type LibraryGenerator struct {
	BaseGenerator
}

// NewLibraryGenerator creates a new LibraryGenerator.
func NewLibraryGenerator() *LibraryGenerator {
	return &LibraryGenerator{
		BaseGenerator: NewBaseGenerator(types.OutputModeLibrary),
	}
}

// Generate creates a library chart and wrapper charts from the resource graph.
func (g *LibraryGenerator) Generate(ctx context.Context, graph *types.ResourceGraph, opts Options) ([]*types.GeneratedChart, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	charts := make([]*types.GeneratedChart, 0)

	// Always generate the library chart with the shared helpers.
	libChart := g.generateLibraryChart(opts)
	charts = append(charts, libChart)

	// Group resources and generate wrapper charts.
	groupResult, err := GroupResources(graph)
	if err != nil {
		return nil, fmt.Errorf("failed to group resources: %w", err)
	}

	for _, group := range groupResult.Groups {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		wrapper, err := g.generateWrapperChart(group, libChart.Name, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to generate chart for group %s: %w", group.Name, err)
		}
		charts = append(charts, wrapper)
	}

	return charts, nil
}

// generateLibraryChart creates the library chart with the shared helpers.
func (g *LibraryGenerator) generateLibraryChart(opts Options) *types.GeneratedChart {
	chartName := "library"

	chartMeta := helm.ChartMetadata{
		Name:        chartName,
		Version:     opts.ChartVersion,
		AppVersion:  opts.AppVersion,
		Description: "Library chart with helpers shared by the service charts",
		APIVersion:  "v2",
		Type:        "library",
		Keywords:    []string{"kubernetes", "library", "deckhouse"},
	}

	return &types.GeneratedChart{
		Name:       chartName,
		Path:       opts.OutputDir,
		ChartYAML:  helm.GenerateChartYAML(chartMeta),
		ValuesYAML: "# Library charts do not have values.yaml\n# Values are provided by wrapper charts\n",
		Templates:  map[string]string{},
		Helpers:    helm.GenerateHelpers(chartName),
	}
}

// generateWrapperChart creates the application chart for a service group.
// It is a separate-mode chart whose helper references point at the library.
func (g *LibraryGenerator) generateWrapperChart(group *ServiceGroup, libraryName string, opts Options) (*types.GeneratedChart, error) {
	sep := &SeparateGenerator{}
	chart, err := sep.generateChartForGroup(group, opts, opts.ChartName)
	if err != nil {
		return nil, err
	}

	chart.ChartYAML = helm.GenerateChartYAML(helm.ChartMetadata{
		Name:        group.Name,
		Version:     opts.ChartVersion,
		AppVersion:  opts.AppVersion,
		Description: fmt.Sprintf("Helm chart for %s (uses the %s library chart)", group.Name, libraryName),
		APIVersion:  "v2",
		Type:        "application",
		Keywords:    []string{"kubernetes", "deckhouse"},
		Dependencies: []helm.Dependency{
			{
				Name:       libraryName,
				Version:    opts.ChartVersion,
				Repository: fmt.Sprintf("file://../%s", libraryName),
			},
		},
	})
	for path, content := range chart.Templates {
		chart.Templates[path] = rewriteHelperReferences(content, group.Name, libraryName)
	}
	chart.Notes = rewriteHelperReferences(chart.Notes, group.Name, libraryName)
	chart.Helpers = ""
	return chart, nil
}
