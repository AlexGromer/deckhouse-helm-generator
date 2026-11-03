package generator

import (
	"sort"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// inChartNames returns the names of the objects the chart renders, by Kind.
// The helpers of every chart (also of each chart of a separate, umbrella or
// library layout) get the names of the whole graph: global.namePrefix is put
// in front of such a name wherever it is referenced, whichever chart the
// referenced object ends up in. Objects that are not templates (CRDs go to
// crds/) and kinds whose name is fixed (processor.Prefixable) are left out.
func inChartNames(graph *types.ResourceGraph) map[string][]string {
	if graph == nil {
		return nil
	}
	return inChartOf(graphResources(graph))
}

// graphResources returns every resource of the graph once: those of the graph,
// of its groups and the orphans.
func graphResources(graph *types.ResourceGraph) []*types.ProcessedResource {
	seen := map[*types.ProcessedResource]bool{}
	var all []*types.ProcessedResource
	add := func(r *types.ProcessedResource) {
		if r != nil && !seen[r] {
			seen[r] = true
			all = append(all, r)
		}
	}
	for _, r := range graph.Resources {
		add(r)
	}
	for _, g := range graph.Groups {
		for _, r := range g.Resources {
			add(r)
		}
	}
	for _, r := range graph.Orphans {
		add(r)
	}
	return all
}

// inChartOf returns the names of the objects among resources that a chart
// rendering them lists for global.namePrefix, by Kind (see inChartNames).
func inChartOf(all []*types.ProcessedResource) map[string][]string {
	sets := map[string]map[string]bool{}
	for _, r := range all {
		if r.Original == nil || r.Original.Object == nil || isCRD(r) || r.TemplatePath == "" || r.TemplateContent == "" {
			continue
		}
		kind, name := r.Original.Object.GetKind(), r.Original.Object.GetName()
		if name == "" || !processor.Prefixable(kind) {
			continue
		}
		if sets[kind] == nil {
			sets[kind] = map[string]bool{}
		}
		sets[kind][name] = true
	}
	out := make(map[string][]string, len(sets))
	for kind, names := range sets {
		list := make([]string, 0, len(names))
		for n := range names {
			list = append(list, n)
		}
		sort.Strings(list)
		out[kind] = list
	}
	return out
}
