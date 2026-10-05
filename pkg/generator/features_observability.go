package generator

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// Service mesh, observability and delivery features (`dhg generate --with ...`).
//
// Every feature here follows the same rules so that they compose with each
// other and with features from other files:
//   - all added templates are guarded by a top-level values toggle
//     (`<key>.enabled`) appended with appendTopLevelValues;
//   - added resources reuse the wrapper of the generated template they are
//     derived from (`$svc := ...`, `if $svc.enabled`, `with $svc.<kind>`), so
//     they follow the service's enabled flag and work in every output mode;
//   - templates that cannot be recognised (e.g. rewritten by another feature)
//     are left untouched instead of being broken.
func init() {
	RegisterFeature(Feature{
		Name:        "reloader",
		Description: "Stakater Reloader: restart Deployments/StatefulSets/DaemonSets when the ConfigMaps or Secrets they use change",
		Params: map[string]string{
			"all-workloads": "false",
		},
		Apply: applyReloaderFeature,
	})
	RegisterFeature(Feature{
		Name:        "linkerd",
		Description: "Linkerd: add the proxy injection annotation to Deployment/StatefulSet/DaemonSet pods",
		Params: map[string]string{
			"inject": "enabled",
		},
		Apply: applyLinkerdFeature,
	})
	RegisterFeature(Feature{
		Name:        "istio",
		Description: "Istio: VirtualService, DestinationRule, PeerAuthentication and (values-driven) AuthorizationPolicy per Service",
		Params: map[string]string{
			"timeout":              "30s",
			"retries":              "3",
			"per-try-timeout":      "10s",
			"mtls-mode":            "STRICT",
			"lb-policy":            "",
			"hash-header":          "",
			"outlier-5xx":          "5",
			"max-connections":      "0",
			"max-pending-requests": "0",
		},
		Apply: applyIstioFeature,
	})
	RegisterFeature(Feature{
		Name:        "otel",
		Description: "OpenTelemetry Operator: Instrumentation resource plus auto-instrumentation annotations for workloads with a detected language",
		Params: map[string]string{
			"endpoint":       "http://otel-collector:4317",
			"language":       "",
			"sampler":        "parentbased_traceidratio",
			"sampling-ratio": "1",
			"propagators":    "tracecontext,baggage",
		},
		Apply: applyOTelFeature,
	})
	RegisterFeature(Feature{
		Name:        "prometheus-rules",
		Description: "PrometheusRule with workload alerts (crash loops, OOM kills, restarts, not ready) and optional SLO burn-rate alerts",
		Params: map[string]string{
			"severity":            "warning",
			"runbook-url":         "",
			"slo-availability":    "",
			"slo-requests-metric": "http_requests_total",
			"slo-errors-selector": `code=~"5.."`,
		},
		Apply: applyPrometheusRulesFeature,
	})
	RegisterFeature(Feature{
		Name:        "argo-rollouts",
		Description: "Argo Rollouts: canary Rollout referencing each Deployment (workloadRef), steps configurable in values",
		Params: map[string]string{
			"steps":      "20:1m,50:2m",
			"scale-down": "progressively",
		},
		Apply: applyArgoRolloutsFeature,
	})
}

// podWorkloadKinds are the workload kinds whose pod template is long-running
// and can be annotated for meshes, reloaders and instrumentation.
var podWorkloadKinds = map[string]bool{"Deployment": true, "StatefulSet": true, "DaemonSet": true}

// resourceTemplate is a generated single-resource template split into the
// wrapper that opens it, the YAML document, and the matching closing lines.
type resourceTemplate struct {
	path   string
	prefix []string // control lines before apiVersion ($svc := ..., if, with)
	body   []string // the YAML document
	suffix []string // the {{- end }} lines that close prefix
	kind   string
	name   string // metadata.name expression, verbatim
}

var (
	reOpener    = regexp.MustCompile(`^\s*\{\{-?\s*(if|with|range)\b`)
	reEnd       = regexp.MustCompile(`^\s*\{\{-?\s*end\s*-?\}\}\s*$`)
	reWith      = regexp.MustCompile(`^\s*\{\{-?\s*with\s.*\}\}\s*$`)
	reNindent   = regexp.MustCompile(`\b(n?indent) (\d+)`)
	reFullname  = regexp.MustCompile(`include "([^"]+)\.fullname"`)
	reDefFullnm = regexp.MustCompile(`define "([^"]+)\.fullname"`)
	reIncludeNm = regexp.MustCompile(`^\{\{ include "([^"]+)" \$ \}\}([A-Za-z0-9.-]*)$`)
	reLiteralNm = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)
)

// parseResourceTemplate recognises templates produced by the generator: an
// optional wrapper of control lines, exactly one YAML document, and the
// closing lines of the wrapper. It returns false for anything else.
func parseResourceTemplate(path, content string) (*resourceTemplate, bool) {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "apiVersion:") {
			start = i
			break
		}
		if strings.TrimSpace(l) != "" && !strings.HasPrefix(l, "{{") {
			return nil, false
		}
	}
	if start < 0 {
		return nil, false
	}
	open := 0
	for _, l := range lines[:start] {
		if reOpener.MatchString(l) {
			open++
		} else if reEnd.MatchString(l) {
			open--
		}
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	var suffix []string
	for n := 0; n < open; n++ {
		if end <= start || !reEnd.MatchString(lines[end-1]) || strings.HasPrefix(lines[end-1], " ") {
			return nil, false
		}
		suffix = append([]string{lines[end-1]}, suffix...)
		end--
	}
	rt := &resourceTemplate{
		path:   path,
		prefix: append([]string(nil), lines[:start]...),
		body:   append([]string(nil), lines[start:end]...),
		suffix: suffix,
	}
	kinds := 0
	for _, l := range rt.body {
		if l == "---" {
			return nil, false
		}
		if strings.HasPrefix(l, "kind: ") {
			kinds++
			rt.kind = strings.TrimSpace(strings.TrimPrefix(l, "kind: "))
		}
	}
	if kinds != 1 {
		return nil, false
	}
	if meta := rt.topLevel("metadata:"); meta >= 0 {
		for _, l := range rt.body[meta+1 : blockEnd(rt.body, meta)] {
			if strings.HasPrefix(l, "  name: ") {
				rt.name = strings.TrimSpace(strings.TrimPrefix(l, "  name: "))
				break
			}
		}
	}
	if rt.name == "" {
		return nil, false
	}
	return rt, true
}

// render reassembles the template.
func (rt *resourceTemplate) render() string {
	all := append(append(append([]string(nil), rt.prefix...), rt.body...), rt.suffix...)
	return strings.Join(all, "\n") + "\n"
}

// wrap returns body wrapped in the same prefix/suffix as rt, additionally
// guarded by condition (a template expression such as `$.Values.x.enabled`).
func (rt *resourceTemplate) wrap(condition string, body []string) string {
	var out []string
	out = append(out, "{{- if "+condition+" }}")
	out = append(out, rt.prefix...)
	out = append(out, body...)
	out = append(out, rt.suffix...)
	out = append(out, "{{- end }}")
	return strings.Join(out, "\n") + "\n"
}

// topLevel returns the index of an unindented key line (e.g. "spec:") in the body.
func (rt *resourceTemplate) topLevel(key string) int {
	for i, l := range rt.body {
		if l == key {
			return i
		}
	}
	return -1
}

// child returns the index of line `key` inside the block opened at parent, or -1.
func (rt *resourceTemplate) child(parent int, key string) int {
	if parent < 0 {
		return -1
	}
	for i := parent + 1; i < blockEnd(rt.body, parent); i++ {
		if rt.body[i] == key {
			return i
		}
	}
	return -1
}

// block returns the lines of the block opened at line i (including line i).
func (rt *resourceTemplate) block(i int) []string {
	if i < 0 {
		return nil
	}
	return append([]string(nil), rt.body[i:blockEnd(rt.body, i)]...)
}

// labels returns metadata.labels (key line included), or nil.
func (rt *resourceTemplate) labels() []string {
	return rt.block(rt.child(rt.topLevel("metadata:"), "  labels:"))
}

// selector returns spec.selector (key line included), or nil.
func (rt *resourceTemplate) selector() []string {
	return rt.block(rt.child(rt.topLevel("spec:"), "  selector:"))
}

// blockEnd returns the index after the last line belonging to the block
// opened at line i: the first following non-blank line (YAML or template
// control) whose indentation is not deeper than line i.
func blockEnd(lines []string, i int) int {
	ind := indentOf(lines[i])
	j := i + 1
	for ; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "" {
			continue
		}
		if indentOf(lines[j]) <= ind {
			break
		}
	}
	return j
}

func indentOf(l string) int { return len(l) - len(strings.TrimLeft(l, " ")) }

// reindent shifts lines by delta spaces and adjusts `nindent N`/`indent N`
// arguments accordingly, so copied template blocks keep rendering correctly.
func reindent(lines []string, delta int) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			out = append(out, l)
			continue
		}
		l = reNindent.ReplaceAllStringFunc(l, func(m string) string {
			parts := strings.SplitN(m, " ", 2)
			n, _ := strconv.Atoi(parts[1])
			return fmt.Sprintf("%s %d", parts[0], n+delta)
		})
		if delta >= 0 {
			l = strings.Repeat(" ", delta) + l
		} else {
			l = l[min(-delta, indentOf(l)):]
		}
		out = append(out, l)
	}
	return out
}

// balancedControl reports whether the control lines among lines open and
// close the same number of blocks.
func balancedControl(lines []string) bool {
	depth := 0
	for _, l := range lines {
		if reOpener.MatchString(l) {
			depth++
		} else if reEnd.MatchString(l) {
			depth--
		}
		if depth < 0 {
			return false
		}
	}
	return depth == 0
}

// resourceTemplates returns the recognised templates of chart whose kind is
// accepted by match, sorted by path.
func resourceTemplates(chart *types.GeneratedChart, match func(kind string) bool) []*resourceTemplate {
	paths := make([]string, 0, len(chart.Templates))
	for p := range chart.Templates {
		if strings.HasSuffix(p, ".yaml") && !strings.HasPrefix(baseName(p), "_") {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	var out []*resourceTemplate
	for _, p := range paths {
		rt, ok := parseResourceTemplate(p, chart.Templates[p])
		if ok && match(rt.kind) {
			out = append(out, rt)
		}
	}
	return out
}

func isKind(kinds ...string) func(string) bool {
	return func(k string) bool {
		for _, want := range kinds {
			if k == want {
				return true
			}
		}
		return false
	}
}

func baseName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// featureTemplatePath derives the path of a template a feature adds next to
// the template src, e.g. ("istio", "templates/web-service.yaml") ->
// "templates/istio-web-service.yaml".
func featureTemplatePath(feature, src string) string {
	dir := ""
	if i := strings.LastIndex(src, "/"); i >= 0 {
		dir = src[:i+1]
	}
	return dir + feature + "-" + baseName(src)
}

// chartHelperPrefix returns the name prefix of the chart's helpers (the X in
// `include "X.fullname"`), or "" if the chart has none.
func chartHelperPrefix(chart *types.GeneratedChart) string {
	paths := make([]string, 0, len(chart.Templates))
	for p := range chart.Templates {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		if m := reFullname.FindStringSubmatch(chart.Templates[p]); m != nil {
			return m[1]
		}
	}
	if m := reDefFullnm.FindStringSubmatch(chart.Helpers); m != nil {
		return m[1]
	}
	return ""
}

// chartHasHelper reports whether the chart defines the named helper template.
func chartHasHelper(chart *types.GeneratedChart, name string) bool {
	def := `define "` + name + `"`
	if strings.Contains(chart.Helpers, def) {
		return true
	}
	for p, c := range chart.Templates {
		if strings.HasPrefix(baseName(p), "_") && strings.Contains(c, def) {
			return true
		}
	}
	return false
}

// graphResourceFor returns the input resource a chart template was generated
// from, or nil when it cannot be determined.
func graphResourceFor(graph *types.ResourceGraph, rt *resourceTemplate) *types.ProcessedResource {
	if graph == nil {
		return nil
	}
	var match []*types.ProcessedResource
	for _, r := range graph.Resources {
		if r != nil && r.Original != nil && r.Original.Object != nil &&
			r.TemplatePath == rt.path && r.Original.GVK.Kind == rt.kind {
			match = append(match, r)
		}
	}
	if len(match) == 0 {
		return nil
	}
	sort.Slice(match, func(i, j int) bool {
		return match[i].Original.ResourceKey().String() < match[j].Original.ResourceKey().String()
	})
	return match[0]
}

// annotationLines builds annotation entries guarded by condition. Each entry
// is rendered at indent; values are quoted literally.
func annotationLines(condition string, indent int, annotations [][2]string) []string {
	pad := strings.Repeat(" ", indent)
	out := []string{pad + "{{- if " + condition + " }}"}
	for _, a := range annotations {
		out = append(out, fmt.Sprintf("%s%s: %s", pad, a[0], a[1]))
	}
	return append(out, pad+"{{- end }}")
}

// injectAnnotations adds entries to the annotations of the metadata block
// opened at line meta of rt.body. entries(indent) returns the lines to add,
// indented for the annotations map and guarded by their own condition; cond
// is that condition. The `annotations:` key is rendered only when one of its
// contributors is active, using a guard of the form
//
//	{{- if or (cond2) (cond1) (.podAnnotations) }}
//	annotations:
//	  ...entries of each contributor...
//	{{- end }}
//
// Features of this file extend that guard. The generator's own conditional
// block
//
//	{{- with .podAnnotations }}
//	annotations:
//	  {{- toYaml . | nindent 8 }}
//	{{- end }}
//
// is converted to that form, and entries are appended to an unconditional
// `annotations:` key. Any other structure is left untouched and false is
// returned.
func (rt *resourceTemplate) injectAnnotations(meta int, cond string, entries func(indent int) []string) bool {
	if meta < 0 {
		return false
	}
	body := rt.body
	childPad := strings.Repeat(" ", indentOf(body[meta])+2)
	entryLines := entries(len(childPad) + 2)
	end := blockEnd(body, meta)
	guardPrefix := childPad + "{{- if or ("
	guard := func(conds ...string) string {
		return guardPrefix + strings.Join(conds, ") (") + ") }}"
	}

	at := -1
	for i := meta + 1; i < end; i++ {
		if body[i] == childPad+"annotations:" || body[i] == childPad+"annotations: {}" {
			at = i
			break
		}
	}
	var out []string
	if at < 0 {
		out = append(out, body[:end]...)
		out = append(out, guard(cond), childPad+"annotations:")
		out = append(out, entryLines...)
		out = append(out, childPad+"{{- end }}")
		rt.body = append(out, body[end:]...)
		return true
	}
	if body[at] == childPad+"annotations: {}" {
		out = append(out, body[:at]...)
		out = append(out, childPad+"annotations:")
		out = append(out, entryLines...)
		rt.body = append(out, body[at+1:]...)
		return true
	}

	prev := at - 1
	for prev > meta && strings.TrimSpace(body[prev]) == "" {
		prev--
	}
	annEnd := blockEnd(body, at)
	switch {
	case prev <= meta || !reOpener.MatchString(body[prev]):
		// Unconditional key: append.
		out = append(out, body[:at+1]...)
		out = append(out, entryLines...)
		out = append(out, body[at+1:]...)
	case strings.HasPrefix(body[prev], guardPrefix) && strings.HasSuffix(body[prev], ") }}"):
		// Guard written by another feature of this file: extend it.
		out = append(out, body[:prev]...)
		out = append(out, guardPrefix+cond+") ("+strings.TrimPrefix(body[prev], guardPrefix))
		out = append(out, body[at])
		out = append(out, entryLines...)
		out = append(out, body[at+1:]...)
	case reWith.MatchString(body[prev]) && indentOf(body[prev]) == len(childPad):
		if annEnd >= len(body) || body[annEnd] != childPad+"{{- end }}" {
			return false
		}
		withExpr := strings.TrimSpace(body[prev])
		withExpr = strings.TrimSuffix(strings.TrimPrefix(withExpr, "{{- with "), " }}")
		if strings.Contains(withExpr, "{{") || strings.Contains(withExpr, "}}") {
			return false
		}
		out = append(out, body[:prev]...)
		out = append(out, guard(cond, withExpr), childPad+"annotations:")
		out = append(out, entryLines...)
		out = append(out, "  "+body[prev])
		out = append(out, body[at+1:annEnd]...)
		out = append(out, "  "+body[annEnd])
		out = append(out, body[annEnd:]...) // the original end now closes the guard
	default:
		return false // a conditional key we do not understand
	}
	rt.body = out
	return true
}

// podTemplateMetadata returns the index of spec.template.metadata of a
// Deployment/StatefulSet/DaemonSet body, inserting an empty metadata key when
// the pod template has none. It returns -1 if there is no pod template.
func (rt *resourceTemplate) podTemplateMetadata() int {
	tmpl := rt.child(rt.topLevel("spec:"), "  template:")
	if tmpl < 0 {
		return -1
	}
	if meta := rt.child(tmpl, "    metadata:"); meta >= 0 {
		return meta
	}
	body := append([]string(nil), rt.body[:tmpl+1]...)
	body = append(body, "    metadata:")
	rt.body = append(body, rt.body[tmpl+1:]...)
	return tmpl + 1
}

// addFeatureValues appends the feature's values block to the chart.
func addFeatureValues(chart *types.GeneratedChart, key string, value interface{}) error {
	values, err := appendTopLevelValues(chart.ValuesYAML, key, value)
	if err != nil {
		return err
	}
	chart.ValuesYAML = values
	return nil
}

// addTemplate adds a new template, refusing to overwrite an existing one.
func addTemplate(chart *types.GeneratedChart, path, content string) error {
	if _, exists := chart.Templates[path]; exists {
		return fmt.Errorf("template %s already exists", path)
	}
	chart.Templates[path] = content
	return nil
}

// nameExpressions converts metadata.name expressions of generated templates
// into Helm expressions that evaluate to the names (e.g.
// `(printf "%s-web" (include "app.fullname" $))`). Unknown forms are skipped.
func nameExpressions(templates []*resourceTemplate) []string {
	var out []string
	seen := map[string]bool{}
	for _, rt := range templates {
		var expr string
		if m := reIncludeNm.FindStringSubmatch(rt.name); m != nil {
			expr = fmt.Sprintf(`(printf "%%s%s" (include %q $))`, m[2], m[1])
		} else if reLiteralNm.MatchString(rt.name) {
			expr = strconv.Quote(rt.name)
		} else if name, err := strconv.Unquote(rt.name); err == nil && strings.HasPrefix(rt.name, `"`) {
			expr = strconv.Quote(name) // a name YAML needs quoted, e.g. "123"
		} else {
			continue
		}
		if !seen[expr] {
			seen[expr] = true
			out = append(out, expr)
		}
	}
	return out
}
