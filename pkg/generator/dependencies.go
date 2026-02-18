package generator

import (
	"fmt"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/helm"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ChartDependencyMap maps chart name to its list of helm dependencies.
type ChartDependencyMap map[string][]helm.Dependency

// DetectCrossChartDeps analyzes relationships between service groups and returns
// a map of chart dependencies. It also validates there are no circular dependencies.
// chartVersion specifies the version for generated dependency entries.
func DetectCrossChartDeps(groups []*ServiceGroup, graph *types.ResourceGraph, chartVersion string) (ChartDependencyMap, error) {
	if len(groups) <= 1 {
		return make(ChartDependencyMap), nil
	}

	// Build resource key -> group name mapping.
	resourceToGroup := make(map[types.ResourceKey]string)
	for _, group := range groups {
		for _, r := range group.Resources {
			resourceToGroup[r.Original.ResourceKey()] = group.Name
		}
	}

	// Find cross-group relationships.
	// crossDeps[A] = set of group names that A depends on.
	crossDeps := make(map[string]map[string]bool)
	for _, rel := range graph.Relationships {
		fromGroup, fromOK := resourceToGroup[rel.From]
		toGroup, toOK := resourceToGroup[rel.To]

		if !fromOK || !toOK {
			continue
		}

		// Only record cross-chart dependencies (skip intra-chart).
		if fromGroup == toGroup {
			continue
		}

		if crossDeps[fromGroup] == nil {
			crossDeps[fromGroup] = make(map[string]bool)
		}
		crossDeps[fromGroup][toGroup] = true
	}

	// Check for circular dependencies using DFS.
	if err := detectCircular(crossDeps); err != nil {
		return nil, err
	}

	// Convert to helm.Dependency format.
	result := make(ChartDependencyMap)
	for chartName, depNames := range crossDeps {
		deps := make([]helm.Dependency, 0, len(depNames))
		for depName := range depNames {
			deps = append(deps, helm.Dependency{
				Name:       depName,
				Version:    chartVersion,
				Repository: fmt.Sprintf("file://../%s", depName),
				Condition:  fmt.Sprintf("%s.enabled", depName),
			})
		}
		result[chartName] = deps
	}

	return result, nil
}

// detectCircular checks for circular dependencies using DFS.
func detectCircular(deps map[string]map[string]bool) error {
	const (
		white = 0 // unvisited
		gray  = 1 // in progress
		black = 2 // finished
	)

	color := make(map[string]int)

	var dfs func(node string) error
	dfs = func(node string) error {
		color[node] = gray
		for neighbor := range deps[node] {
			if color[neighbor] == gray {
				// Back edge found — circular dependency.
				return fmt.Errorf("circular dependency detected: %s -> %s", node, neighbor)
			}
			if color[neighbor] == white {
				if err := dfs(neighbor); err != nil {
					return err
				}
			}
		}
		color[node] = black
		return nil
	}

	// Start DFS from each unvisited node.
	for node := range deps {
		if color[node] == white {
			if err := dfs(node); err != nil {
				return err
			}
		}
	}

	return nil
}
