package generator

import (
	"fmt"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"path/filepath"
	"strings"
)

// ParsePBCMapping parses --pbc values in the format "<pbc>=<svc>[,<svc>...]".
func ParsePBCMapping(specs []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, spec := range specs {
		parts := strings.SplitN(spec, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid PBC mapping format: %q (expected <pbc>=<svc1>,<svc2>...)", spec)
		}
		pbc := strings.TrimSpace(parts[0])
		if pbc == "" {
			return nil, fmt.Errorf("empty PBC name in mapping: %q", spec)
		}
		svcs := strings.Split(parts[1], ",")
		for _, svc := range svcs {
			svc = strings.TrimSpace(svc)
			if svc == "" {
				continue
			}
			if existing, ok := result[svc]; ok && existing != pbc {
				return nil, fmt.Errorf("service %q is mapped to multiple PBCs: %q and %q", svc, existing, pbc)
			}
			result[svc] = pbc
		}
	}
	return result, nil
}

// ResolveTopology determines the PBC for each service group based on a 5-priority system.
func ResolveTopology(graph *types.ResourceGraph, explicitMap map[string]string) *types.ServiceTopology {
	topology := &types.ServiceTopology{
		ServicePBC:       make(map[string]string),
		PBCToServices:    make(map[string][]string),
		PlatformServices: make([]string, 0),
	}

	inboundCounts := make(map[string]int)
	if graph != nil {
		for _, conn := range graph.Connections {
			if conn.To.Resource != nil {
				if toRes, ok := graph.Resources[*conn.To.Resource]; ok {
					inboundCounts[toRes.ServiceName]++
				}
			}
		}
	}

	for _, group := range graph.Groups {
		pbc := resolvePBCForGroup(group, explicitMap)

		isDBorBroker := strings.Contains(group.Name, "db") || strings.Contains(group.Name, "postgres") ||
			strings.Contains(group.Name, "kafka") || strings.Contains(group.Name, "redis")

		if isDBorBroker && inboundCounts[group.Name] > 1 {
			pbc = "platform"
			fmt.Printf("Note: Service %q acts as a shared database or broker. Assigned to 'platform' PBC. This is an anti-pattern.\n", group.Name)
		}

		topology.ServicePBC[group.Name] = pbc
		if pbc == "platform" {
			topology.PlatformServices = append(topology.PlatformServices, group.Name)
		} else {
			topology.PBCToServices[pbc] = append(topology.PBCToServices[pbc], group.Name)
		}
	}

	return topology
}

func resolvePBCForGroup(group *types.ResourceGroup, explicitMap map[string]string) string {
	// Priority 1: explicit mapping
	if pbc, ok := explicitMap[group.Name]; ok {
		return pbc
	}

	// Priority 2: dhg.deckhouse.io/pbc label/annotation
	// Priority 3: app.kubernetes.io/part-of label
	var dhgPBCs []string
	var partOfPBCs []string

	for _, res := range group.Resources {
		labels := res.Original.Object.GetLabels()
		annotations := res.Original.Object.GetAnnotations()

		if pbc, ok := labels["dhg.deckhouse.io/pbc"]; ok {
			dhgPBCs = append(dhgPBCs, pbc)
		} else if pbc, ok := annotations["dhg.deckhouse.io/pbc"]; ok {
			dhgPBCs = append(dhgPBCs, pbc)
		}

		if pbc, ok := labels["app.kubernetes.io/part-of"]; ok {
			partOfPBCs = append(partOfPBCs, pbc)
		}
	}

	if len(dhgPBCs) > 0 {
		return mostFrequent(dhgPBCs)
	}

	if len(partOfPBCs) >= 2 && countDistinct(partOfPBCs) >= 2 {
		return mostFrequent(partOfPBCs)
	}

	// Priority 4: metadata.namespace
	var namespaces []string
	for _, res := range group.Resources {
		ns := res.Original.Object.GetNamespace()
		if ns != "" {
			namespaces = append(namespaces, ns)
		}
	}
	if len(namespaces) >= 2 && countDistinct(namespaces) >= 2 {
		return mostFrequent(namespaces)
	}

	// Priority 5: Default/fallback or common prefix heuristic
	parts := strings.Split(group.Name, "-")
	if len(parts) > 1 {
		return parts[0]
	}

	return "default"
}

func mostFrequent(items []string) string {
	counts := make(map[string]int)
	maxCount := 0
	mostFreq := ""
	for _, item := range items {
		counts[item]++
		if counts[item] > maxCount {
			maxCount = counts[item]
			mostFreq = item
		}
	}
	return mostFreq
}

func countDistinct(items []string) int {
	distinct := make(map[string]bool)
	for _, item := range items {
		distinct[item] = true
	}
	return len(distinct)
}

// GeneratePBCUmbrellaChart handles umbrella generation for PBCs (`charts/<pbc>/charts/<service>/`).
func GeneratePBCUmbrellaChart(topology *types.ServiceTopology, baseDir string) map[string]string {
	results := make(map[string]string)

	// Create charts/<pbc>/Chart.yaml files
	for pbc, services := range topology.PBCToServices {
		pbcDir := filepath.Join(baseDir, "charts", pbc)
		chartYaml := fmt.Sprintf("apiVersion: v2\nname: %s\ndescription: PBC umbrella for %s\ntype: application\nversion: 0.1.0\ndependencies:\n", pbc, pbc)

		for _, svc := range services {
			chartYaml += fmt.Sprintf("- name: %s\n  version: 0.1.0\n  condition: %s.enabled\n", svc, svc)
		}

		results[filepath.Join(pbcDir, "Chart.yaml")] = chartYaml
	}
	return results
}

// GenerateSeparateLayout helper function for separate layout mapping to `<output>/<pbc>/<service>/`.
func GenerateSeparateLayout(topology *types.ServiceTopology, baseDir string) map[string]string {
	results := make(map[string]string)
	for pbc, services := range topology.PBCToServices {
		for _, svc := range services {
			path := filepath.Join(baseDir, pbc, svc, "Chart.yaml")
			results[path] = fmt.Sprintf("apiVersion: v2\nname: %s\ndescription: Service %s for PBC %s\nversion: 0.1.0\n", svc, svc, pbc)
		}
	}
	return results
}

// GenerateLibraryLayout helper function for library layout by PBC.
func GenerateLibraryLayout(topology *types.ServiceTopology, baseDir string) map[string]string {
	results := make(map[string]string)
	for pbc, services := range topology.PBCToServices {
		for _, svc := range services {
			path := filepath.Join(baseDir, pbc, svc, "Chart.yaml")
			results[path] = fmt.Sprintf("apiVersion: v2\nname: %s\ntype: library\ndescription: Library Service %s for PBC %s\nversion: 0.1.0\n", svc, svc, pbc)
		}
	}
	return results
}
