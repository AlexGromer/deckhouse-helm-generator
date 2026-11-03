package generator

import (
	"errors"
	"fmt"
	"regexp"
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
	// Notes are the observations about --shared-keep and --shared-exclude
	// entries (no such object, nothing to keep, where a kept object stays),
	// sorted.
	Notes []string

	// relocated is true when a kept object changed its group.
	relocated bool
}

// SharedOverrides are the exceptions to the rule of SplitShared; each entry
// is "Kind/name" (the Kind is case-insensitive).
type SharedOverrides struct {
	// Keep objects stay with a service group even when the rule moves them.
	// One referenced by several groups stays with the first of them in sorted
	// order.
	Keep []string
	// Exclude objects are never moved: they stay where grouping put them.
	Exclude []string
}

// sharedRef is a parsed "Kind/name" entry; kind is lowercase.
type sharedRef struct{ kind, name string }

var sharedRefKind = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

func parseSharedRef(entry string) (sharedRef, error) {
	kind, name, ok := strings.Cut(strings.TrimSpace(entry), "/")
	if !ok || !sharedRefKind.MatchString(kind) || name == "" || strings.ContainsAny(name, "/ \t") {
		return sharedRef{}, fmt.Errorf("%q is not Kind/name (e.g. ConfigMap/common)", entry)
	}
	return sharedRef{kind: strings.ToLower(kind), name: name}, nil
}

func (r sharedRef) String() string { return r.kind + "/" + r.name }

// ValidateSharedOverrides checks the entries of --shared-keep and
// --shared-exclude: each is Kind/name, and no object is in both lists.
func ValidateSharedOverrides(keep, exclude []string) error {
	parse := func(flag string, entries []string) (map[sharedRef]bool, error) {
		set := map[sharedRef]bool{}
		for _, e := range entries {
			ref, err := parseSharedRef(e)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", flag, err)
			}
			set[ref] = true
		}
		return set, nil
	}
	keepSet, err := parse("--shared-keep", keep)
	if err != nil {
		return err
	}
	excludeSet, err := parse("--shared-exclude", exclude)
	if err != nil {
		return err
	}
	var both []string
	for ref := range keepSet {
		if excludeSet[ref] {
			both = append(both, ref.String())
		}
	}
	if len(both) > 0 {
		sort.Strings(both)
		return fmt.Errorf("%s is in both --shared-keep and --shared-exclude", strings.Join(both, ", "))
	}
	return nil
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
	return SplitSharedWith(groups, SharedOverrides{})
}

// SplitSharedWith is SplitShared with the exceptions of ov. Entries that do
// not parse or name no object are reported in SharedSplit.Notes and
// otherwise ignored (ValidateSharedOverrides rejects the former up front).
func SplitSharedWith(groups []*ServiceGroup, ov SharedOverrides) *SharedSplit {
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

	keep, exclude, notes := sharedOverrideSets(groups, ov)
	hasWorkload := map[string]bool{}
	for _, g := range groups {
		for _, r := range g.Resources {
			if r != nil && r.Original != nil && podOwnerKinds[r.Original.GVK.Kind] {
				hasWorkload[g.Name] = true
			}
		}
	}

	moved := map[*types.ProcessedResource]bool{}
	relocate := map[*types.ProcessedResource]string{}
	var shared []SharedObject
	for key, p := range index {
		ref := refOf(p.resource)
		if exclude[ref] {
			continue
		}
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
		if keep[ref] {
			if len(users) == 0 {
				tail := ""
				if !hasWorkload[p.group] {
					tail = ", which has no workload"
				}
				notes = append(notes, fmt.Sprintf("--shared-keep: %s/%s is referenced by no workload; it stays in chart %s%s",
					p.resource.Original.GVK.Kind, p.resource.Original.Object.GetName(), p.group, tail))
			}
			if len(users) >= 2 {
				if p.group != users[0] {
					relocate[p.resource] = users[0]
				}
				notes = append(notes, fmt.Sprintf("--shared-keep: %s/%s stays in chart %s, the first (in sorted order) of the charts that reference it (%s); the others depend on it, install %s first",
					p.resource.Original.GVK.Kind, p.resource.Original.Object.GetName(), users[0], strings.Join(users, ", "), users[0]))
			}
			continue
		}
		moved[p.resource] = true
		shared = append(shared, SharedObject{Resource: p.resource, UsedBy: users})
	}
	sort.Slice(shared, func(i, j int) bool {
		return shared[i].Resource.Original.ResourceKey().Less(shared[j].Resource.Original.ResourceKey())
	})
	sort.Strings(notes)

	arrivals := map[string][]*types.ProcessedResource{}
	for r, owner := range relocate {
		arrivals[owner] = append(arrivals[owner], r)
	}
	for _, rs := range arrivals {
		sort.Slice(rs, func(i, j int) bool {
			return rs[i].Original.ResourceKey().Less(rs[j].Original.ResourceKey())
		})
	}

	split := &SharedSplit{Shared: shared, Notes: notes, relocated: len(relocate) > 0}
	for _, g := range groups {
		kept := make([]*types.ProcessedResource, 0, len(g.Resources))
		for _, r := range g.Resources {
			if _, away := relocate[r]; !moved[r] && !away {
				kept = append(kept, r)
			}
		}
		kept = append(kept, arrivals[g.Name]...)
		if len(kept) == 0 && len(g.Resources) > 0 {
			continue
		}
		clone := *g
		clone.Resources = kept
		split.Groups = append(split.Groups, &clone)
	}
	return split
}

// refOf is the Kind/name entry that names r.
func refOf(r *types.ProcessedResource) sharedRef {
	return sharedRef{kind: strings.ToLower(r.Original.GVK.Kind), name: r.Original.Object.GetName()}
}

// sharedOverrideSets parses ov against the objects of groups. Notes report
// entries that name no object, or an object that never moves.
func sharedOverrideSets(groups []*ServiceGroup, ov SharedOverrides) (keep, exclude map[sharedRef]bool, notes []string) {
	present := map[sharedRef]bool{}
	immovable := map[sharedRef]bool{}
	for _, g := range groups {
		for _, r := range g.Resources {
			if r == nil || r.Original == nil || r.Original.Object == nil {
				continue
			}
			ref := refOf(r)
			present[ref] = true
			if !movable(r) {
				immovable[ref] = true
			}
		}
	}
	parse := func(flag string, entries []string) map[sharedRef]bool {
		set := map[sharedRef]bool{}
		for _, e := range entries {
			ref, err := parseSharedRef(e)
			switch {
			case err != nil:
				notes = append(notes, fmt.Sprintf("%s: %v, ignored", flag, err))
			case !present[ref]:
				notes = append(notes, fmt.Sprintf("%s: no object %s in the input, ignored", flag, e))
			case immovable[ref]:
				notes = append(notes, fmt.Sprintf("%s: %s is a workload, a CRD or has no template of its own: it is never moved, the entry has no effect", flag, e))
			default:
				set[ref] = true
			}
		}
		return set
	}
	return parse("--shared-keep", ov.Keep), parse("--shared-exclude", ov.Exclude), notes
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
		if len(opts.SharedKeep) > 0 || len(opts.SharedExclude) > 0 {
			return nil, errors.New("--shared-keep and --shared-exclude need --shared-chart")
		}
		return plan, nil
	}
	if err := ValidateSharedOverrides(opts.SharedKeep, opts.SharedExclude); err != nil {
		return nil, err
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

	split := SplitSharedWith(groups, SharedOverrides{Keep: opts.SharedKeep, Exclude: opts.SharedExclude})
	note := func(text string) {
		if opts.OnSharedNote != nil {
			opts.OnSharedNote(text)
		}
	}
	for _, n := range split.Notes {
		note(n)
	}
	if opts.OnShared != nil {
		opts.OnShared(name, split.Shared)
	}
	if split.relocated {
		plan.groups = split.Groups
	}
	if len(split.Shared) == 0 {
		return plan, nil
	}

	movedSet := map[*types.ProcessedResource]bool{}
	resources := make([]*types.ProcessedResource, 0, len(split.Shared))
	for _, o := range split.Shared {
		movedSet[o.Resource] = true
		kept, problem := withKeepPolicy(o.Resource)
		if problem != "" {
			note(problem)
		}
		resources = append(resources, kept)
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

// keepPolicyKey is the annotation that makes Helm leave an object in place on
// helm uninstall (and when it drops out of a release).
const keepPolicyKey = "helm.sh/resource-policy"

// annotationsFromValues is how processor templates render metadata.annotations
// from the values of the object.
var annotationsFromValues = regexp.MustCompile(`(?m)^[ \t]*\{\{-?\s*with\s+[^}\n]*\.annotations\s*-?\}\}[ \t]*\n  annotations:[ \t]*\n`)

// withKeepPolicy returns a copy of r that renders helm.sh/resource-policy:
// keep, so that helm uninstall of the release of the shared chart never
// deletes the object. An annotation of that name that the object has already
// keeps its value. The problem is non-empty when the template has a shape the
// annotation cannot be added to safely; r is then returned unchanged.
func withKeepPolicy(r *types.ProcessedResource) (*types.ProcessedResource, string) {
	clone := *r
	label := r.Original.GVK.Kind + "/" + r.Original.Object.GetName()
	tmpl := r.TemplateContent

	switch {
	case annotationsFromValues.MatchString(tmpl):
		// The annotations come from the values of the object: put the
		// annotation there, where `helm template --set` can see it.
		annotations := map[string]interface{}{}
		switch existing := r.Values["annotations"].(type) {
		case nil:
		case map[string]string:
			for k, v := range existing {
				annotations[k] = v
			}
		case map[string]interface{}:
			for k, v := range existing {
				annotations[k] = v
			}
		default:
			return r, fmt.Sprintf("%s: annotations in values have an unexpected type, %s is not added; add %s: keep by hand", label, keepPolicyKey, keepPolicyKey)
		}
		if _, ok := annotations[keepPolicyKey]; !ok {
			annotations[keepPolicyKey] = "keep"
		}
		values := make(map[string]interface{}, len(r.Values)+1)
		for k, v := range r.Values {
			values[k] = v
		}
		values["annotations"] = annotations
		clone.Values = values
	case strings.Contains(tmpl, keepPolicyKey):
		// The template renders it itself.
	case !strings.Contains(tmpl, "\n  annotations:"):
		// No annotations at all: add the block as the first field of metadata.
		const head = "\nmetadata:\n"
		i := strings.Index(tmpl, head)
		if i < 0 {
			return r, fmt.Sprintf("%s: the template has no metadata block, %s is not added; add %s: keep by hand", label, keepPolicyKey, keepPolicyKey)
		}
		at := i + len(head)
		clone.TemplateContent = tmpl[:at] + "  annotations:\n    " + keepPolicyKey + ": keep\n" + tmpl[at:]
	default:
		// Literal annotations (a plugin's, for instance) or ones under a
		// condition: adding a line to a block that may not render could
		// silently do nothing, so say it.
		lines := strings.Split(tmpl, "\n")
		for i, l := range lines {
			if l != "  annotations:" {
				continue
			}
			if i > 0 && strings.HasPrefix(strings.TrimSpace(lines[i-1]), "{{") {
				break
			}
			if i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "{{") {
				break
			}
			lines = append(lines[:i+1], append([]string{"    " + keepPolicyKey + ": keep"}, lines[i+1:]...)...)
			clone.TemplateContent = strings.Join(lines, "\n")
			return &clone, ""
		}
		return r, fmt.Sprintf("%s: the annotations of its template are conditional, %s is not added; add %s: keep by hand", label, keepPolicyKey, keepPolicyKey)
	}
	return &clone, ""
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
