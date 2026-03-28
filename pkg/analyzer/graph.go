package analyzer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// kindColors maps resource kinds to DOT graph colors.
var kindColors = map[string]string{
	"Deployment":  "#4A90D9",
	"StatefulSet": "#7B68EE",
	"DaemonSet":   "#9370DB",
	"Service":     "#50C878",
	"Ingress":     "#FFD700",
	"ConfigMap":   "#87CEEB",
	"Secret":      "#FF6B6B",
	"Job":         "#FFA500",
	"CronJob":     "#FF8C00",
	"PersistentVolumeClaim": "#DDA0DD",
	"ServiceAccount":        "#98FB98",
	"Role":                  "#F0E68C",
	"ClusterRole":           "#F0E68C",
	"RoleBinding":           "#FAFAD2",
	"ClusterRoleBinding":    "#FAFAD2",
	"HorizontalPodAutoscaler": "#FF69B4",
	"PodDisruptionBudget":     "#DEB887",
	"NetworkPolicy":           "#CD853F",
}

// edgeStyles maps relationship types to DOT edge styles.
var edgeStyles = map[types.RelationshipType]string{
	types.RelationLabelSelector:    "solid",
	types.RelationNameReference:    "dashed",
	types.RelationVolumeMount:      "dotted",
	types.RelationEnvFrom:          "dotted",
	types.RelationEnvValueFrom:     "dotted",
	types.RelationAnnotation:       "dashed",
	types.RelationServiceAccount:   "bold",
	types.RelationOwnerReference:   "bold",
	types.RelationRoleBinding:      "dashed",
	types.RelationClusterRoleBinding: "dashed",
	types.RelationPVC:              "dotted",
	types.RelationGatewayRoute:     "solid",
	types.RelationScaleTarget:      "bold",
	types.RelationCustomDependency: "solid",
}

// GenerateDOTGraph produces a Graphviz DOT format string from a ResourceGraph.
// Nodes are colored by resource kind, edges are styled by relationship type.
// This is pure text output — no graphviz library dependency (ADR-019).
func GenerateDOTGraph(graph *types.ResourceGraph) string {
	if graph == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("digraph resources {\n")
	b.WriteString("  rankdir=LR;\n")
	b.WriteString("  node [shape=box, style=filled, fontname=\"Helvetica\"];\n")
	b.WriteString("  edge [fontname=\"Helvetica\", fontsize=10];\n")
	b.WriteString("\n")

	// Collect and sort resource keys for deterministic output
	keys := make([]types.ResourceKey, 0, len(graph.Resources))
	for k := range graph.Resources {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].String() < keys[j].String()
	})

	// Emit nodes
	for _, key := range keys {
		nodeID := sanitizeDOTID(key.String())
		color := kindColors[key.GVK.Kind]
		if color == "" {
			color = "#C0C0C0" // default gray for unknown kinds
		}

		label := fmt.Sprintf("%s\\n%s", key.GVK.Kind, key.Name)
		if key.Namespace != "" {
			label += fmt.Sprintf("\\n(%s)", key.Namespace)
		}

		b.WriteString(fmt.Sprintf("  %q [label=%q, fillcolor=%q];\n", nodeID, label, color))
	}

	b.WriteString("\n")

	// Emit edges (sorted for determinism)
	sortedRels := make([]types.Relationship, len(graph.Relationships))
	copy(sortedRels, graph.Relationships)
	sort.Slice(sortedRels, func(i, j int) bool {
		if sortedRels[i].From.String() != sortedRels[j].From.String() {
			return sortedRels[i].From.String() < sortedRels[j].From.String()
		}
		return sortedRels[i].To.String() < sortedRels[j].To.String()
	})

	for _, rel := range sortedRels {
		fromID := sanitizeDOTID(rel.From.String())
		toID := sanitizeDOTID(rel.To.String())
		style := edgeStyles[rel.Type]
		if style == "" {
			style = "solid"
		}

		edgeLabel := string(rel.Type)
		b.WriteString(fmt.Sprintf("  %q -> %q [label=%q, style=%s];\n", fromID, toID, edgeLabel, style))
	}

	b.WriteString("}\n")
	return b.String()
}

// sanitizeDOTID converts a resource key string to a valid DOT node identifier.
func sanitizeDOTID(s string) string {
	replacer := strings.NewReplacer("/", "_", ".", "_", "-", "_", " ", "_")
	return replacer.Replace(s)
}

// DetectCircularDependencies performs DFS on the ResourceGraph to find cycles.
// Returns an error with the cycle path if a cycle is found, nil otherwise.
func DetectCircularDependencies(graph *types.ResourceGraph) error {
	if graph == nil || len(graph.Resources) == 0 {
		return nil
	}

	const (
		white = 0 // unvisited
		gray  = 1 // in current DFS path
		black = 2 // fully explored
	)

	color := make(map[types.ResourceKey]int)
	parent := make(map[types.ResourceKey]types.ResourceKey)

	var dfs func(node types.ResourceKey) error
	dfs = func(node types.ResourceKey) error {
		color[node] = gray

		for _, rel := range graph.GetRelationshipsFrom(node) {
			neighbor := rel.To
			// Only consider neighbors that exist in the graph
			if _, exists := graph.Resources[neighbor]; !exists {
				continue
			}

			if color[neighbor] == gray {
				// Cycle detected — reconstruct the path
				cycle := reconstructCycle(parent, node, neighbor)
				return fmt.Errorf("circular dependency detected: %s", cycle)
			}

			if color[neighbor] == white {
				parent[neighbor] = node
				if err := dfs(neighbor); err != nil {
					return err
				}
			}
		}

		color[node] = black
		return nil
	}

	// Sort keys for deterministic cycle detection
	keys := make([]types.ResourceKey, 0, len(graph.Resources))
	for k := range graph.Resources {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].String() < keys[j].String()
	})

	for _, key := range keys {
		if color[key] == white {
			if err := dfs(key); err != nil {
				return err
			}
		}
	}

	return nil
}

// reconstructCycle builds a human-readable cycle path string.
func reconstructCycle(parent map[types.ResourceKey]types.ResourceKey, from, to types.ResourceKey) string {
	path := []string{to.String()}
	current := from
	for current != to {
		path = append([]string{current.String()}, path...)
		p, ok := parent[current]
		if !ok {
			break
		}
		current = p
	}
	path = append([]string{to.String()}, path...)
	return strings.Join(path, " -> ")
}

// DecompositionRecommendation contains a suggestion for splitting a chart.
type DecompositionRecommendation struct {
	// SuggestedGroups lists the recommended resource groupings.
	SuggestedGroups []RecommendedGroup

	// CouplingScore is the ratio of inter-group edges to intra-group edges.
	// Lower is better (more cohesive groups).
	CouplingScore float64

	// Reason explains why decomposition is recommended.
	Reason string
}

// RecommendedGroup represents a suggested group of resources.
type RecommendedGroup struct {
	// Name is the suggested group/chart name.
	Name string

	// Resources lists the resource keys in this group.
	Resources []types.ResourceKey
}

// AnalyzeDecomposition analyzes graph coupling and cohesion,
// suggesting split points for separate chart generation.
func AnalyzeDecomposition(graph *types.ResourceGraph) *DecompositionRecommendation {
	if graph == nil || len(graph.Groups) <= 1 {
		return &DecompositionRecommendation{
			CouplingScore: 0,
			Reason:        "single group or empty graph, no decomposition needed",
		}
	}

	// Build resource -> group mapping
	resourceToGroup := make(map[types.ResourceKey]string)
	for _, group := range graph.Groups {
		for _, r := range group.Resources {
			resourceToGroup[r.Original.ResourceKey()] = group.Name
		}
	}

	// Count intra-group and inter-group edges
	var intraEdges, interEdges int
	for _, rel := range graph.Relationships {
		fromGroup := resourceToGroup[rel.From]
		toGroup := resourceToGroup[rel.To]

		if fromGroup == "" || toGroup == "" {
			continue
		}

		if fromGroup == toGroup {
			intraEdges++
		} else {
			interEdges++
		}
	}

	// Calculate coupling score: inter / (inter + intra)
	totalEdges := intraEdges + interEdges
	var couplingScore float64
	if totalEdges > 0 {
		couplingScore = float64(interEdges) / float64(totalEdges)
	}

	// Build recommended groups from existing groups
	groups := make([]RecommendedGroup, 0, len(graph.Groups))
	for _, g := range graph.Groups {
		keys := make([]types.ResourceKey, 0, len(g.Resources))
		for _, r := range g.Resources {
			keys = append(keys, r.Original.ResourceKey())
		}
		groups = append(groups, RecommendedGroup{
			Name:      g.Name,
			Resources: keys,
		})
	}

	var reason string
	switch {
	case couplingScore > 0.5:
		reason = fmt.Sprintf("high coupling (%.1f%%) between groups — consider merging tightly coupled groups or restructuring dependencies", couplingScore*100)
	case couplingScore > 0.2:
		reason = fmt.Sprintf("moderate coupling (%.1f%%) — current grouping is reasonable but could benefit from interface abstraction", couplingScore*100)
	case len(graph.Groups) > 5:
		reason = fmt.Sprintf("many groups (%d) with low coupling (%.1f%%) — good candidates for separate charts", len(graph.Groups), couplingScore*100)
	default:
		reason = fmt.Sprintf("low coupling (%.1f%%) with %d groups — current structure is well-decomposed", couplingScore*100, len(graph.Groups))
	}

	return &DecompositionRecommendation{
		SuggestedGroups: groups,
		CouplingScore:   couplingScore,
		Reason:          reason,
	}
}
