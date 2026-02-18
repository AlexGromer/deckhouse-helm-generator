package generator

import (
	"context"
	"fmt"

	"sigs.k8s.io/yaml"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/helm"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// UmbrellaGenerator generates a parent umbrella chart containing all service groups as subcharts.
type UmbrellaGenerator struct {
	BaseGenerator
}

// NewUmbrellaGenerator creates a new UmbrellaGenerator.
func NewUmbrellaGenerator() *UmbrellaGenerator {
	return &UmbrellaGenerator{
		BaseGenerator: NewBaseGenerator(types.OutputModeUmbrella),
	}
}

// Generate creates a parent umbrella chart with subcharts in charts/ subdirectory.
func (g *UmbrellaGenerator) Generate(ctx context.Context, graph *types.ResourceGraph, opts Options) ([]*types.GeneratedChart, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	groupResult, err := GroupResources(graph)
	if err != nil {
		return nil, fmt.Errorf("failed to group resources: %w", err)
	}

	if len(groupResult.Groups) == 0 {
		return []*types.GeneratedChart{}, nil
	}

	parentName := opts.ChartName
	if parentName == "" {
		parentName = "umbrella"
	}

	charts := make([]*types.GeneratedChart, 0, 1+len(groupResult.Groups))
	deps := make([]helm.Dependency, 0, len(groupResult.Groups))

	sep := &SeparateGenerator{}
	parentValues := make(map[string]interface{})

	for _, group := range groupResult.Groups {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Generate subchart using SeparateGenerator logic.
		subOpts := opts
		subOpts.ChartName = group.Name
		subchart, err := sep.generateChartForGroup(group, subOpts)
		if err != nil {
			return nil, fmt.Errorf("generating subchart for %s: %w", group.Name, err)
		}

		// Place subchart inside parent's charts/ directory.
		subchart.Name = fmt.Sprintf("%s/charts/%s", parentName, group.Name)
		subchart.Path = fmt.Sprintf("%s/charts/", parentName)

		charts = append(charts, subchart)

		// Add dependency for parent Chart.yaml.
		deps = append(deps, helm.Dependency{
			Name:      group.Name,
			Version:   opts.ChartVersion,
			Condition: fmt.Sprintf("%s.enabled", group.Name),
		})

		// Collect flat values for parent values.yaml (with enabled flag).
		flatVals := sep.buildFlatValues(group)
		flatVals["enabled"] = true
		parentValues[group.Name] = flatVals
	}

	// Extract shared global values from all groups.
	globalVals := ExtractGlobalValues(groupResult.Groups)

	// Generate parent chart.
	parentChart := g.generateParentChart(parentName, deps, parentValues, globalVals, opts)

	return append([]*types.GeneratedChart{parentChart}, charts...), nil
}

// generateParentChart creates the umbrella parent chart metadata and values.
func (g *UmbrellaGenerator) generateParentChart(
	chartName string,
	deps []helm.Dependency,
	subValues map[string]interface{},
	globalVals map[string]interface{},
	opts Options,
) *types.GeneratedChart {
	chartMeta := helm.ChartMetadata{
		Name:         chartName,
		Version:      opts.ChartVersion,
		AppVersion:   opts.AppVersion,
		Description:  fmt.Sprintf("Umbrella chart for %s", chartName),
		APIVersion:   "v2",
		Dependencies: deps,
	}

	// Build parent values: global + per-subchart sections.
	allValues := make(map[string]interface{})

	if len(globalVals) > 0 {
		allValues["global"] = globalVals
	} else {
		allValues["global"] = map[string]interface{}{
			"imageRegistry": "",
		}
	}

	for name, vals := range subValues {
		allValues[name] = vals
	}

	valuesBytes, _ := yaml.Marshal(allValues)
	valuesYAML := "# Umbrella chart — override subchart values per-service here\n" + string(valuesBytes)

	return &types.GeneratedChart{
		Name:       chartName,
		Path:       opts.OutputDir,
		ChartYAML:  helm.GenerateChartYAML(chartMeta),
		ValuesYAML: valuesYAML,
		Templates:  map[string]string{},
		Helpers:    helm.GenerateHelpers(chartName),
	}
}
