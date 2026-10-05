package generator

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// Generated workloads keep the identity of the input: their pods carry the
// labels from the input and spec.selector is the input's selector (rendered
// from values). Post-processors that need to select the pods of one workload
// (PodDisruptionBudgets, NetworkPolicies, ...) therefore copy the workload's
// own spec.selector instead of assuming labels of their own.

// workloadPDBTemplate returns a PodDisruptionBudget template for the workload
// template wl: it reuses the workload's wrapper (so it follows the workload's
// values and enabled switch) and its spec.selector, so it selects exactly the
// workload's pods. maxUnavailable: 1 keeps node drains possible at any replica
// count. condition, when not empty, additionally guards the template. It
// returns false when the template shape is not recognised.
func workloadPDBTemplate(chartName string, wl *resourceTemplate, nameSuffix, condition string) (string, bool) {
	selector := wl.selector()
	if len(selector) < 2 || !balancedControl(selector[1:]) {
		return "", false
	}
	body := []string{
		"apiVersion: policy/v1",
		"kind: PodDisruptionBudget",
		"metadata:",
		"  name: " + suffixedName(wl.name, "-"+nameSuffix),
		"  namespace: {{ $.Release.Namespace }}",
		"  labels:",
		fmt.Sprintf(`    {{- include "%s.labels" $ | nindent 4 }}`, chartName),
		"spec:",
		"  maxUnavailable: 1",
	}
	body = append(body, selector...)
	if condition == "" {
		var out []string
		out = append(out, wl.prefix...)
		out = append(out, body...)
		out = append(out, wl.suffix...)
		return strings.Join(out, "\n") + "\n", true
	}
	return wl.wrap(condition, body), true
}

// suffixedName appends suffix to a metadata.name expression of a generated
// template: a plain or templated name gets the suffix appended, a quoted
// name is re-quoted with it.
func suffixedName(name, suffix string) string {
	if strings.HasPrefix(name, `"`) {
		if s, err := strconv.Unquote(name); err == nil {
			return strconv.Quote(s + suffix)
		}
	}
	return name + suffix
}

// hasPDBForWorkload reports whether a PodDisruptionBudget of the chart
// selects the pods of the workload template wl (a second PDB on the same pods
// blocks eviction). Selectors and pod labels are resolved through values.yaml
// for values-driven templates; literal selectors are read from the template.
func hasPDBForWorkload(chart *types.GeneratedChart, wl *resourceTemplate) bool {
	values := map[string]interface{}{}
	_ = yaml.Unmarshal([]byte(chart.ValuesYAML), &values)
	podLabels, ok := workloadPodLabels(wl, values)
	if !ok {
		return false
	}
	for _, pdb := range resourceTemplates(chart, isKind("PodDisruptionBudget")) {
		if sel, ok := templateSelector(pdb, values); ok && labelSelectorMatches(sel, podLabels) {
			return true
		}
	}
	return false
}

// workloadPodLabels returns the pod labels a workload template renders from
// the input (its podLabels values, or a literal labels map).
func workloadPodLabels(wl *resourceTemplate, values map[string]interface{}) (map[string]string, bool) {
	if scope, ok := templateScope(wl.prefix, values); ok {
		if labels, ok := scope["podLabels"].(map[string]interface{}); ok {
			return stringMap(labels), true
		}
	}
	tmpl := wl.child(wl.topLevel("spec:"), "  template:")
	meta := wl.child(tmpl, "    metadata:")
	block := wl.block(wl.child(meta, "      labels:"))
	if len(block) < 2 {
		return nil, false
	}
	parsed := map[string]interface{}{}
	if strings.Contains(strings.Join(block, "\n"), "{{") || yaml.Unmarshal([]byte(strings.Join(reindent(block, -6), "\n")), &parsed) != nil {
		return nil, false
	}
	labels, ok := parsed["labels"].(map[string]interface{})
	return stringMap(labels), ok
}

// templateSelector returns the spec.selector a template renders: the values
// it is rendered from, or the literal YAML.
func templateSelector(rt *resourceTemplate, values map[string]interface{}) (map[string]interface{}, bool) {
	// The key may be guarded (`{{- with .selector }}`): the guard lines sit
	// next to it, the block is the same.
	sel := rt.selector()
	if len(sel) < 2 {
		return nil, false
	}
	text := strings.TrimSpace(strings.Join(sel[1:], "\n"))
	if strings.HasPrefix(text, "{{- toYaml .selector |") || strings.HasPrefix(text, "{{- toYaml . |") {
		scope, ok := templateScope(rt.prefix, values)
		if !ok {
			return nil, false
		}
		m, ok := scope["selector"].(map[string]interface{})
		return m, ok
	}
	if strings.Contains(text, "{{") {
		return nil, false
	}
	parsed := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(strings.Join(reindent(sel, -2), "\n")), &parsed); err != nil {
		return nil, false
	}
	m, ok := parsed["selector"].(map[string]interface{})
	return m, ok
}

var (
	reScopeVar  = regexp.MustCompile(`^\{\{-?\s*\$(\w+)\s*:=\s*\.Values((?:\.[A-Za-z_][A-Za-z0-9_]*)*)\s*-?\}\}$`)
	reScopeWith = regexp.MustCompile(`^\{\{-?\s*with\s+\$(\w+)((?:\.[A-Za-z_][A-Za-z0-9_]*)+)\s*-?\}\}$`)
)

// templateScope resolves the values map that `.` refers to inside the wrapper
// of a generated template ({{- $svc := .Values.services.web -}} ...
// {{- with $svc.deployment }}).
func templateScope(prefix []string, values map[string]interface{}) (map[string]interface{}, bool) {
	vars := map[string][]string{}
	var scope []string
	found := false
	for _, l := range prefix {
		l = strings.TrimSpace(l)
		if m := reScopeVar.FindStringSubmatch(l); m != nil {
			vars[m[1]] = splitPath(m[2])
			continue
		}
		if m := reScopeWith.FindStringSubmatch(l); m != nil {
			base, ok := vars[m[1]]
			if !ok {
				return nil, false
			}
			scope = append(append([]string(nil), base...), splitPath(m[2])...)
			found = true
		}
	}
	if !found {
		return nil, false
	}
	var cur interface{} = values
	for _, key := range scope {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		cur = m[key]
	}
	m, ok := cur.(map[string]interface{})
	return m, ok
}

func splitPath(p string) []string {
	p = strings.TrimPrefix(p, ".")
	if p == "" {
		return nil
	}
	return strings.Split(p, ".")
}

func stringMap(m map[string]interface{}) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = fmt.Sprint(v)
	}
	return out
}

// labelSelectorMatches reports whether a (non-empty) Kubernetes label
// selector selects a pod with the given labels.
func labelSelectorMatches(selector map[string]interface{}, labels map[string]string) bool {
	matchLabels, _ := selector["matchLabels"].(map[string]interface{})
	exprs, _ := selector["matchExpressions"].([]interface{})
	if len(matchLabels) == 0 && len(exprs) == 0 {
		return false
	}
	for k, v := range matchLabels {
		if got, ok := labels[k]; !ok || got != fmt.Sprint(v) {
			return false
		}
	}
	for _, e := range exprs {
		expr, _ := e.(map[string]interface{})
		key, _ := expr["key"].(string)
		op, _ := expr["operator"].(string)
		vals, _ := expr["values"].([]interface{})
		got, has := labels[key]
		in := false
		for _, v := range vals {
			if fmt.Sprint(v) == got {
				in = true
			}
		}
		switch op {
		case "In":
			if !has || !in {
				return false
			}
		case "NotIn":
			if has && in {
				return false
			}
		case "Exists":
			if !has {
				return false
			}
		case "DoesNotExist":
			if has {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// workloadPodSelector returns the label selector of an input workload's pods:
// the processed selector (spec.selector of the input, as the chart renders
// it), else the pod template labels. Jobs and CronJobs are selected by their
// pod labels. It returns nil when the pods cannot be selected.
func workloadPodSelector(r *types.ProcessedResource) map[string]interface{} {
	if r == nil || r.Original == nil || r.Original.Object == nil {
		return nil
	}
	if sel, ok := r.Values["selector"].(map[string]interface{}); ok && len(sel) > 0 {
		return sel
	}
	obj := r.Original.Object.Object
	kind := r.Original.Object.GetKind()
	templatePath := []string{"spec", "template"}
	if kind == "CronJob" {
		templatePath = []string{"spec", "jobTemplate", "spec", "template"}
	}
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Rollout":
		if sel := nestedMap(obj, "spec", "selector"); len(sel) > 0 {
			return sel
		}
	}
	labels := nestedMap(obj, append(templatePath, "metadata", "labels")...)
	if len(labels) == 0 {
		if labels, ok := r.Values["podLabels"].(map[string]interface{}); ok && len(labels) > 0 {
			return map[string]interface{}{"matchLabels": labels}
		}
		switch kind {
		case "Deployment", "StatefulSet", "DaemonSet":
			// The processors give such a workload app=<name>.
			return map[string]interface{}{"matchLabels": map[string]interface{}{"app": r.Original.Object.GetName()}}
		}
		return nil
	}
	return map[string]interface{}{"matchLabels": labels}
}

// selectorYAML renders a label selector as YAML lines indented by indent.
func selectorYAML(selector map[string]interface{}, indent int) []string {
	out, err := yaml.Marshal(selector)
	if err != nil {
		return nil
	}
	pad := strings.Repeat(" ", indent)
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		lines = append(lines, pad+escapeDelims(l))
	}
	return lines
}

// delimEscaper keeps literal {{ and }} of static content from being
// interpreted by Helm.
var delimEscaper = strings.NewReplacer("{{", `{{"{{"}}`, "}}", `{{"}}"}}`)

func escapeDelims(s string) string { return delimEscaper.Replace(s) }

// sortedWorkloads returns the pod-creating resources of a group in a stable
// order.
func sortedWorkloads(group *ServiceGroup) []*types.ProcessedResource {
	var out []*types.ProcessedResource
	for _, r := range group.Resources {
		if r == nil || r.Original == nil || r.Original.Object == nil {
			continue
		}
		switch r.Original.GVK.Kind {
		case "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob", "Rollout":
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Original.ResourceKey().String() < out[j].Original.ResourceKey().String()
	})
	return out
}
