package generator

import (
	"fmt"
	"sort"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// GroupingStrategy indicates how a service group was formed.
type GroupingStrategy string

const (
	// GroupByLabel indicates grouping by app.kubernetes.io/name label.
	GroupByLabel GroupingStrategy = "label"

	// GroupByRelationship indicates grouping by connected components in the relationship graph.
	GroupByRelationship GroupingStrategy = "relationship"

	// GroupByNamespace indicates grouping by namespace when no labels or relationships exist.
	GroupByNamespace GroupingStrategy = "namespace"

	// GroupByIndividual indicates a standalone resource that could not be grouped.
	GroupByIndividual GroupingStrategy = "individual"
)

// ServiceGroup represents a logical group of resources for chart generation.
type ServiceGroup struct {
	// Name is the group/service name.
	Name string

	// Resources contains all resources in this group.
	Resources []*types.ProcessedResource

	// Namespace is the primary namespace for this group.
	Namespace string

	// Strategy indicates how this group was formed.
	Strategy GroupingStrategy
}

// GroupingResult contains the result of resource grouping.
type GroupingResult struct {
	// Groups contains all formed service groups.
	Groups []*ServiceGroup
}

// labelPriority defines the order in which labels are checked for grouping.
// Earlier entries take precedence.
var labelPriority = []string{
	"app.kubernetes.io/name",
	"app.kubernetes.io/instance",
	"app",
	"name",
}

// GroupResources groups resources from a resource graph into logical service groups.
// Strategy priority: label > relationship > namespace > individual.
func GroupResources(graph *types.ResourceGraph) (*GroupingResult, error) {
	if len(graph.Resources) == 0 {
		return &GroupingResult{Groups: make([]*ServiceGroup, 0)}, nil
	}

	grouped := make(map[types.ResourceKey]bool)
	groupsByName := make(map[string]*ServiceGroup)

	// Walk resources in key order: group names and contents must not depend
	// on map iteration order.
	keys := graph.SortedKeys()

	// Pass 1: Group by standard labels (highest priority).
	for _, key := range keys {
		resource := graph.Resources[key]
		appName := extractAppLabel(resource)
		if appName == "" {
			continue
		}
		grouped[key] = true
		if g, ok := groupsByName[appName]; ok {
			g.Resources = append(g.Resources, resource)
		} else {
			groupsByName[appName] = &ServiceGroup{
				Name:      appName,
				Resources: []*types.ProcessedResource{resource},
				Namespace: resource.Original.Object.GetNamespace(),
				Strategy:  GroupByLabel,
			}
		}
	}

	// Pass 2: Group ungrouped resources by relationship connected components.
	var ungrouped []types.ResourceKey
	for _, key := range keys {
		if !grouped[key] {
			ungrouped = append(ungrouped, key)
		}
	}

	if len(ungrouped) > 0 && len(graph.Relationships) > 0 {
		// Build adjacency list (undirected) for ungrouped resources.
		adj := make(map[types.ResourceKey][]types.ResourceKey)
		for _, rel := range graph.Relationships {
			adj[rel.From] = append(adj[rel.From], rel.To)
			adj[rel.To] = append(adj[rel.To], rel.From)
		}
		for k := range adj {
			sort.Slice(adj[k], func(i, j int) bool { return adj[k][i].Less(adj[k][j]) })
		}

		// BFS to find connected components among ALL resources connected by relationships.
		visited := make(map[types.ResourceKey]bool)
		for _, key := range ungrouped {
			if visited[key] {
				continue
			}
			_, ok := adj[key]
			if !ok {
				continue
			}

			// BFS from this ungrouped resource.
			component := make([]*types.ProcessedResource, 0)
			queue := []types.ResourceKey{key}
			visited[key] = true

			for len(queue) > 0 {
				current := queue[0]
				queue = queue[1:]

				if r, ok := graph.Resources[current]; ok {
					component = append(component, r)
				}

				for _, neighbor := range adj[current] {
					if !visited[neighbor] {
						visited[neighbor] = true
						queue = append(queue, neighbor)
					}
				}
			}

			if len(component) > 0 {
				// Name the group from the first workload resource, or first resource name.
				name := nameForComponent(component)
				// Mark all component resources as grouped.
				for _, r := range component {
					rKey := r.Original.ResourceKey()
					grouped[rKey] = true
				}
				// Merge with existing label-based group if any component member was already grouped.
				// Check if any component resource belongs to an existing group.
				existingGroupName := ""
				for _, r := range component {
					rKey := r.Original.ResourceKey()
					for gName, g := range groupsByName {
						for _, gr := range g.Resources {
							if gr.Original.ResourceKey() == rKey {
								existingGroupName = gName
								break
							}
						}
						if existingGroupName != "" {
							break
						}
					}
					if existingGroupName != "" {
						break
					}
				}

				if existingGroupName != "" {
					// Add non-grouped resources from component to existing group.
					existing := groupsByName[existingGroupName]
					existingKeys := make(map[types.ResourceKey]bool)
					for _, r := range existing.Resources {
						existingKeys[r.Original.ResourceKey()] = true
					}
					for _, r := range component {
						rKey := r.Original.ResourceKey()
						if !existingKeys[rKey] {
							existing.Resources = append(existing.Resources, r)
						}
					}
				} else {
					name = uniqueGroupName(groupsByName, name)
					groupsByName[name] = &ServiceGroup{
						Name:      name,
						Resources: component,
						Namespace: component[0].Original.Object.GetNamespace(),
						Strategy:  GroupByRelationship,
					}
				}
			}
		}
	}

	// Pass 3: Group remaining ungrouped resources by namespace.
	nsByNamespace := make(map[string][]*types.ProcessedResource)
	for _, key := range keys {
		resource := graph.Resources[key]
		if !grouped[key] {
			ns := resource.Original.Object.GetNamespace()
			nsByNamespace[ns] = append(nsByNamespace[ns], resource)
			grouped[key] = true
		}
	}

	namespaces := make([]string, 0, len(nsByNamespace))
	for ns := range nsByNamespace {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)
	for _, ns := range namespaces {
		resources := nsByNamespace[ns]
		name := ns
		if name == "" {
			// Use first resource name if no namespace.
			name = resources[0].Original.Object.GetName()
		}
		strategy := GroupByNamespace
		if name == resources[0].Original.Object.GetName() && ns == "" {
			strategy = GroupByIndividual
		}
		name = uniqueGroupName(groupsByName, name)
		groupsByName[name] = &ServiceGroup{
			Name:      name,
			Resources: resources,
			Namespace: ns,
			Strategy:  strategy,
		}
	}

	// Collect all groups into result.
	result := &GroupingResult{
		Groups: make([]*ServiceGroup, 0, len(groupsByName)),
	}
	for _, g := range groupsByName {
		result.Groups = append(result.Groups, g)
	}
	sort.Slice(result.Groups, func(i, j int) bool { return result.Groups[i].Name < result.Groups[j].Name })

	return result, nil
}

// uniqueGroupName returns name, or name-2, name-3, … when a group already
// has it: a second group of the same name would replace the first and its
// resources would be lost.
func uniqueGroupName(groups map[string]*ServiceGroup, name string) string {
	candidate := name
	for i := 2; groups[candidate] != nil; i++ {
		candidate = fmt.Sprintf("%s-%d", name, i)
	}
	return candidate
}

// extractAppLabel extracts the application name from standard Kubernetes labels.
// Checks labels in priority order: app.kubernetes.io/name > app.kubernetes.io/instance > app > name.
func extractAppLabel(resource *types.ProcessedResource) string {
	labels := resource.Original.Object.GetLabels()
	if labels == nil {
		return ""
	}
	for _, labelKey := range labelPriority {
		if val, ok := labels[labelKey]; ok && val != "" {
			return val
		}
	}
	return ""
}

// nameForComponent determines a name for a connected component group.
// Prefers workload resource names (Deployment, StatefulSet, DaemonSet).
func nameForComponent(resources []*types.ProcessedResource) string {
	workloadKinds := map[string]bool{
		"Deployment":  true,
		"StatefulSet": true,
		"DaemonSet":   true,
	}

	for _, r := range resources {
		if workloadKinds[r.Original.GVK.Kind] {
			return r.Original.Object.GetName()
		}
	}

	// Fallback: use first resource name.
	if len(resources) > 0 {
		return resources[0].Original.Object.GetName()
	}
	return "unnamed"
}
