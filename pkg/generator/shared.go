package generator

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// podOwnerKinds are the kinds with a pod template (Argo Rollout included):
// they are workloads, never move, and their references decide which kinds an
// unreferenced object may belong to (see SplitShared).
var podOwnerKinds = map[string]bool{
	"Deployment": true, "StatefulSet": true, "DaemonSet": true,
	"Job": true, "CronJob": true, "Pod": true, "ReplicaSet": true,
	"Rollout": true,
}

// unreferencedMovable are the kinds that move to the shared chart when no
// workload references them. Other kinds join them when a workload references
// an object of that kind (see SplitShared).
var unreferencedMovable = map[string]bool{
	"ConfigMap": true, "Secret": true, "ServiceAccount": true, "PersistentVolumeClaim": true,
}

// errSharedChartMode is the error of the modes that have no shared chart.
var errSharedChartMode = errors.New("--shared-chart applies to --mode separate and --mode library only")

// SharedObject is an object that moves to the shared chart.
type SharedObject struct {
	Resource *types.ProcessedResource
	// UsedBy are the names of the service groups whose workloads reference the
	// object, sorted. Empty: no workload references it.
	UsedBy []string
}

// Describe is "Kind namespace/name (used by a, b)" or "... (not referenced by
// any workload)".
func (o SharedObject) Describe() string {
	obj := o.Resource.Original.Object
	name := obj.GetName()
	if ns := obj.GetNamespace(); ns != "" {
		name = ns + "/" + name
	}
	use := "not referenced by any resource"
	if len(o.UsedBy) > 0 {
		use = "used by " + strings.Join(o.UsedBy, ", ")
	}
	return fmt.Sprintf("%s %s (%s)", o.Resource.Original.GVK.Kind, name, use)
}

// SharedSplit is the result of SplitShared.
type SharedSplit struct {
	// Shared are the objects that move to the shared chart, sorted by key.
	Shared []SharedObject
	// Groups are the service groups without them; groups left without any
	// object are dropped. The input groups are not changed.
	Groups []*ServiceGroup
}

// SplitShared decides which objects of the service groups belong in a chart
// of their own. It follows the references of the workloads (their
// Dependencies: envFrom, env, volumes, ServiceAccount, imagePullSecrets):
//
//   - an object referenced by resources of two or more groups moves;
//   - an object of a kind a pod can reference (ConfigMap, Secret,
//     ServiceAccount, PersistentVolumeClaim, or any kind a workload
//     references) that no workload references moves;
//   - an object referenced by resources of exactly one group (a workload, an
//     Ingress, a RoleBinding, ...) stays in the group it is in.
//
// CustomResourceDefinitions (they go to crds/), objects without a template and
// workloads themselves never move. The result does not depend on map order.
func SplitShared(groups []*ServiceGroup) *SharedSplit {
	type placed struct {
		resource *types.ProcessedResource
		group    string
	}
	index := map[types.ResourceKey]placed{}
	for _, g := range groups {
		for _, r := range g.Resources {
			if movable(r) {
				index[r.Original.ResourceKey()] = placed{r, g.Name}
			}
		}
	}
	// Every template renders into .Release.Namespace, so the namespace of the
	// input takes no part in the match; the version does not either (the
	// RoleBinding processor writes its ServiceAccount keys without one). A
	// reference finds the object by its key, else by group, kind and name when
	// exactly one object has them, else when exactly one of them has its
	// namespace.
	bare := func(k types.ResourceKey) types.ResourceKey {
		k.GVK.Version = ""
		k.Namespace = ""
		return k
	}
	byName := map[types.ResourceKey][]types.ResourceKey{}
	for key := range index {
		byName[bare(key)] = append(byName[bare(key)], key)
	}
	resolve := func(dep types.ResourceKey) (types.ResourceKey, bool) {
		if _, ok := index[dep]; ok {
			return dep, true
		}
		candidates := byName[bare(dep)]
		if len(candidates) > 1 {
			var sameNS []types.ResourceKey
			for _, c := range candidates {
				if c.Namespace == dep.Namespace {
					sameNS = append(sameNS, c)
				}
			}
			candidates = sameNS
		}
		if len(candidates) == 1 {
			return candidates[0], true
		}
		return dep, false
	}

	// Any object of a group may reference another one: an Ingress its TLS
	// Secret, a RoleBinding its ServiceAccount. Only the workloads decide which
	// kinds count as referenced (an unreferenced Role stays put).
	usedBy := map[types.ResourceKey]map[string]bool{}
	referencedKinds := map[string]bool{}
	for _, g := range groups {
		for _, r := range g.Resources {
			if r.Original == nil {
				continue
			}
			self := r.Original.ResourceKey()
			workload := podOwnerKinds[r.Original.GVK.Kind]
			for _, dep := range r.Dependencies {
				key, ok := resolve(dep)
				if !ok || key == self || podOwnerKinds[key.GVK.Kind] {
					continue
				}
				if usedBy[key] == nil {
					usedBy[key] = map[string]bool{}
				}
				usedBy[key][g.Name] = true
				if workload {
					referencedKinds[key.GVK.Kind] = true
				}
			}
		}
	}

	moved := map[*types.ProcessedResource]bool{}
	var shared []SharedObject
	for key, p := range index {
		groupsOf := usedBy[key]
		switch {
		case len(groupsOf) >= 2:
		case len(groupsOf) == 0 && (unreferencedMovable[key.GVK.Kind] || referencedKinds[key.GVK.Kind]):
		default:
			continue
		}
		users := make([]string, 0, len(groupsOf))
		for name := range groupsOf {
			users = append(users, name)
		}
		sort.Strings(users)
		moved[p.resource] = true
		shared = append(shared, SharedObject{Resource: p.resource, UsedBy: users})
	}
	sort.Slice(shared, func(i, j int) bool {
		return shared[i].Resource.Original.ResourceKey().Less(shared[j].Resource.Original.ResourceKey())
	})

	split := &SharedSplit{Shared: shared}
	for _, g := range groups {
		kept := make([]*types.ProcessedResource, 0, len(g.Resources))
		for _, r := range g.Resources {
			if !moved[r] {
				kept = append(kept, r)
			}
		}
		if len(kept) == 0 && len(g.Resources) > 0 {
			continue
		}
		clone := *g
		clone.Resources = kept
		split.Groups = append(split.Groups, &clone)
	}
	return split
}

// movable reports whether r can be written to a shared chart: a template of
// its own, and not a workload, a CRD or an object without a name.
func movable(r *types.ProcessedResource) bool {
	if r == nil || r.Original == nil || r.Original.Object == nil || isCRD(r) {
		return false
	}
	return r.TemplatePath != "" && r.TemplateContent != "" && !podOwnerKinds[r.Original.GVK.Kind]
}

// sharedPlan is what a generator that writes a chart per service group needs
// from the shared chart option.
type sharedPlan struct {
	// groups are the service groups to write, without the shared objects.
	groups []*ServiceGroup
	// inChart lists the object names of the service charts for
	// global.namePrefix. The shared objects are not in it: references to them
	// are literal, whatever the prefix of a service chart is.
	inChart map[string][]string
	// chart is the shared chart; nil when nothing is shared.
	chart *types.GeneratedChart
}

// planShared splits groups for opts.SharedChart. reserved are chart names the
// shared chart may not take besides the names of the groups. Without the
// option it returns the groups and the object names of the whole graph.
func (g *SeparateGenerator) planShared(graph *types.ResourceGraph, groups []*ServiceGroup, opts Options, reserved ...string) (*sharedPlan, error) {
	plan := &sharedPlan{groups: groups, inChart: inChartNames(graph)}
	name := opts.SharedChart
	if name == "" {
		return plan, nil
	}
	if !libraryChartName.MatchString(name) {
		return nil, fmt.Errorf("--shared-chart %q is not a chart name (lowercase letters, digits, '-', '.' and '_')", name)
	}
	for _, r := range reserved {
		if r == name {
			return nil, fmt.Errorf("--shared-chart %q is the name of the library chart; name the shared chart differently", name)
		}
	}
	for _, grp := range groups {
		if grp.Name == name {
			return nil, fmt.Errorf("--shared-chart %q is also the name of a service chart; name the shared chart differently", name)
		}
	}

	split := SplitShared(groups)
	if opts.OnShared != nil {
		opts.OnShared(name, split.Shared)
	}
	if len(split.Shared) == 0 {
		return plan, nil
	}

	movedSet := map[*types.ProcessedResource]bool{}
	resources := make([]*types.ProcessedResource, 0, len(split.Shared))
	for _, o := range split.Shared {
		movedSet[o.Resource] = true
		resources = append(resources, o.Resource)
	}
	var remaining []*types.ProcessedResource
	for _, r := range graphResources(graph) {
		if !movedSet[r] {
			remaining = append(remaining, r)
		}
	}
	plan.groups = split.Groups
	plan.inChart = inChartOf(remaining)

	group := &ServiceGroup{Name: name, Resources: resources, Strategy: GroupByIndividual}
	chart, err := g.generateChartForGroup(group, opts, opts.ChartName, inChartOf(resources))
	if err != nil {
		return nil, fmt.Errorf("failed to generate the shared chart %s: %w", name, err)
	}
	if err := refuseNamePrefix(chart, name); err != nil {
		return nil, err
	}
	plan.chart = chart
	return plan, nil
}

// namePrefixRead is how the helpers of a chart read global.namePrefix.
const namePrefixRead = "{{- (default (dict) .Values.global).namePrefix | default \"\" -}}\n"

// refuseNamePrefix makes the shared chart fail on a non-empty
// global.namePrefix. The service charts refer to the shared objects by their
// names in the input, so a prefix on the shared chart would break every such
// reference without an error.
func refuseNamePrefix(chart *types.GeneratedChart, name string) error {
	if strings.Count(chart.Helpers, namePrefixRead) != 1 {
		return fmt.Errorf("the shared chart %s: the helper of global.namePrefix has an unexpected shape", name)
	}
	guard := "{{- $prefix := (default (dict) .Values.global).namePrefix | default \"\" -}}\n" +
		"{{- if $prefix }}{{ fail \"global.namePrefix must stay empty in the shared chart " + name +
		": the service charts refer to its objects by the names of the input\" }}{{ end -}}\n" +
		"{{- $prefix -}}\n"
	chart.Helpers = strings.Replace(chart.Helpers, namePrefixRead, guard, 1)
	return nil
}
