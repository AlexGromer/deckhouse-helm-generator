package synth

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// DefaultInferShare is the share of Deployments that must agree on a
// property for InferProfile to put it into the profile.
const DefaultInferShare = 0.9

// InferOptions configures InferProfile.
type InferOptions struct {
	// MinShare is the share of the Deployments (0 < MinShare <= 1) that must
	// have a property, with the same normalized value, for it to become part
	// of the profile.
	MinShare float64
	// Source describes the input in the header of the profile ("cluster,
	// namespace shop"). It must not hold anything that changes between runs
	// over the same input.
	Source string
	// KeepValues writes the stand's values of the environment-specific
	// variables into the profile's vars; otherwise the profile has them empty
	// and InferResult.Values has them.
	KeepValues bool
}

// InferResult is what InferProfile found.
type InferResult struct {
	// Profile is the platform profile (YAML) with a header comment.
	Profile []byte
	// Values is a YAML file with the stand's values of the variables the
	// profile left empty; nil when there are none or KeepValues is set.
	Values []byte
	// Notes are observations the user should look at: properties skipped
	// because the Deployments disagree, definitions that had to be picked.
	Notes []string
	// Deployments is the number of Deployments read.
	Deployments int
}

// inferredProfile is the profile InferProfile writes. Its keys are those of
// Profile; keys that are mappings of free shape are maps, so that the
// output does not depend on how Profile spells them.
type inferredProfile struct {
	Vars             map[string]string        `json:"vars,omitempty"`
	EnvFrom          []map[string]string      `json:"envFrom,omitempty"`
	InitContainers   []map[string]interface{} `json:"initContainers,omitempty"`
	PodLabels        map[string]string        `json:"podLabels,omitempty"`
	DeploymentLabels map[string]string        `json:"deploymentLabels,omitempty"`
	Defaults         map[string]interface{}   `json:"defaults,omitempty"`
	Naming           map[string]string        `json:"naming,omitempty"`
	Selector         map[string]string        `json:"selector,omitempty"`
	Service          map[string]interface{}   `json:"service,omitempty"`
	Strategy         map[string]interface{}   `json:"strategy,omitempty"`
	Volumes          []map[string]interface{} `json:"volumes,omitempty"`
	VolumeMounts     []map[string]interface{} `json:"volumeMounts,omitempty"`
	Container        map[string]interface{}   `json:"container,omitempty"`
	Pod              map[string]interface{}   `json:"pod,omitempty"`
}

// InferProfile reads the Deployments (and the Services that select them) of
// objs and writes the platform profile they share: what dhg profile infer
// does. Everything is deterministic: the result does not depend on the
// order of objs.
func InferProfile(objs []*unstructured.Unstructured, opts InferOptions) (*InferResult, error) {
	if !(opts.MinShare > 0 && opts.MinShare <= 1) {
		return nil, fmt.Errorf("min-share is %v, want a number above 0 and up to 1", opts.MinShare)
	}
	var deps []*inferDeployment
	var services []*unstructured.Unstructured
	for _, o := range objs {
		switch o.GetKind() {
		case "Deployment":
			deps = append(deps, newInferDeployment(o))
		case "Service":
			services = append(services, o)
		}
	}
	if len(deps) == 0 {
		return nil, fmt.Errorf("no Deployments in the input: the profile is inferred from Deployments")
	}
	sort.SliceStable(deps, func(i, j int) bool {
		return deps[i].ns+"/"+deps[i].name < deps[j].ns+"/"+deps[j].name
	})
	sort.SliceStable(services, func(i, j int) bool {
		return services[i].GetNamespace()+"/"+services[i].GetName() < services[j].GetNamespace()+"/"+services[j].GetName()
	})

	in := &inferrer{deps: deps, need: inferNeed(opts.MinShare, len(deps)), opts: opts}
	in.prefix = inferPrefix(deps, in.need)
	if in.prefix == "" {
		if p, n := inferFrequentPrefix(deps); p != "" {
			in.prefix = p
			in.notef("vars.prefix %q is shared by %d of %d Deployments (min-share needs %d); the others keep their own names", p, n, len(deps), in.need)
		}
	}
	if in.prefix != "" {
		for _, d := range deps {
			if d.ns != "" && strings.Contains(in.prefix, d.ns) {
				in.notef("vars.prefix %q holds the namespace %q: it names the environment, set vars.prefix for each stand", in.prefix, d.ns)
				break
			}
		}
	}
	for _, d := range deps {
		d.norm = inferNormalizer{prefix: in.prefix, svc: d.svc}
		d.services = inferSelecting(services, d)
	}

	prof := inferredProfile{}
	prof.InitContainers = in.initContainers()
	prof.EnvFrom = in.envFrom()
	prof.Selector = in.selector()
	prof.PodLabels = in.labels("podLabels", func(d *inferDeployment) map[string]string { return d.podLabels }, prof.Selector)
	prof.DeploymentLabels = in.labels("deploymentLabels", func(d *inferDeployment) map[string]string { return d.labels }, prof.Selector)
	prof.Volumes, prof.VolumeMounts = in.volumes(prof.InitContainers)
	prof.Container = in.patch("container", []string{"imagePullPolicy", "lifecycle", "securityContext"}, func(d *inferDeployment) map[string]interface{} { return d.main })
	prof.Pod = in.patch("pod", []string{"imagePullSecrets", "nodeSelector", "securityContext"}, func(d *inferDeployment) map[string]interface{} { return d.spec })
	prof.Defaults = in.defaults()
	prof.Naming = in.naming()
	prof.Service = in.service()
	prof.Strategy = in.strategy()

	// Environment-specific values become variables.
	x := &inferVars{values: map[string]string{}, redacted: map[string]bool{}}
	for i := range prof.InitContainers {
		prof.InitContainers[i] = x.walk(prof.InitContainers[i], inferVarCtx{}).(map[string]interface{})
	}
	for i := range prof.Volumes {
		prof.Volumes[i] = x.walk(prof.Volumes[i], inferVarCtx{}).(map[string]interface{})
	}
	for i := range prof.VolumeMounts {
		prof.VolumeMounts[i] = x.walk(prof.VolumeMounts[i], inferVarCtx{}).(map[string]interface{})
	}
	if prof.Container != nil {
		prof.Container = x.walk(prof.Container, inferVarCtx{}).(map[string]interface{})
	}
	if prof.Pod != nil {
		prof.Pod = x.walk(prof.Pod, inferVarCtx{}).(map[string]interface{})
	}

	if in.prefix != "" || len(x.values) > 0 {
		prof.Vars = map[string]string{}
	}
	if in.prefix != "" {
		prof.Vars["prefix"] = in.prefix
	}
	for k, v := range x.values {
		if opts.KeepValues {
			prof.Vars[k] = v
		} else {
			prof.Vars[k] = ""
		}
	}

	share := strconv.FormatFloat(opts.MinShare, 'f', -1, 64)
	source := "min-share: " + share
	if opts.Source != "" {
		source = "source: " + opts.Source + ", " + source
	}
	body, err := yaml.Marshal(prof)
	if err != nil {
		return nil, err
	}
	header := "# Generated by dhg profile infer (" + source + ").\n" +
		"# This is the most frequent shape of the Deployments read, not a specification: review it before use.\n"
	if len(x.values) > 0 && !opts.KeepValues {
		header += "# Variables with an empty value are environment-specific; the stand's values are in the values file (--values-out).\n"
	}
	res := &InferResult{Profile: append([]byte(header), body...), Deployments: len(deps)}
	if len(x.values) > 0 && !opts.KeepValues {
		values, err := yaml.Marshal(struct {
			Vars map[string]string `json:"vars"`
		}{x.values})
		if err != nil {
			return nil, err
		}
		res.Values = append([]byte("# Generated by dhg profile infer ("+source+").\n"+
			"# The stand's values of the variables of the profile: put them into \"vars\" of the profile used for this stand.\n"), values...)
	}
	for place := range x.redacted {
		in.notef("credentials in URL of %s were removed from the profile and are not written to the values file; give them to the chart through a Secret", place)
	}
	sort.Strings(in.notes)
	res.Notes = in.notes
	return res, nil
}

// inferNeed is the number of Deployments out of n that make share.
func inferNeed(share float64, n int) int {
	need := int(math.Ceil(share*float64(n) - 1e-9))
	if need < 1 {
		need = 1
	}
	return need
}

// inferDeployment is a Deployment as inference sees it.
type inferDeployment struct {
	obj    *unstructured.Unstructured
	ns     string
	name   string
	svc    string
	norm   inferNormalizer
	labels map[string]string
	// podLabels are the labels of the pod template, matchLabels the
	// selector's.
	podLabels   map[string]string
	matchLabels map[string]string
	// spec is the pod spec, main the application's container.
	spec     map[string]interface{}
	main     map[string]interface{}
	services []*unstructured.Unstructured
}

func newInferDeployment(o *unstructured.Unstructured) *inferDeployment {
	d := &inferDeployment{obj: o, ns: o.GetNamespace(), name: o.GetName(), labels: o.GetLabels()}
	d.podLabels = inferStringMap(o.Object, "spec", "template", "metadata", "labels")
	d.matchLabels = inferStringMap(o.Object, "spec", "selector", "matchLabels")
	d.spec = inferMap(o.Object, "spec", "template", "spec")
	for _, l := range []map[string]string{d.labels, d.podLabels} {
		for _, key := range []string{"app", "app.kubernetes.io/name"} {
			if d.svc == "" {
				d.svc = l[key]
			}
		}
	}
	if d.svc == "" {
		d.svc = d.name
	}
	containers, _ := d.spec["containers"].([]interface{})
	for _, raw := range containers {
		c, _ := raw.(map[string]interface{})
		if c == nil {
			continue
		}
		name, _ := c["name"].(string)
		if d.main == nil {
			d.main = c
		}
		if name == d.svc || name == d.name {
			d.main = c
			break
		}
	}
	if d.main == nil {
		d.main = map[string]interface{}{}
	}
	return d
}

func inferMap(obj map[string]interface{}, fields ...string) map[string]interface{} {
	v, _, _ := unstructured.NestedFieldNoCopy(obj, fields...)
	m, _ := v.(map[string]interface{})
	return m
}

func inferStringMap(obj map[string]interface{}, fields ...string) map[string]string {
	out := map[string]string{}
	for k, v := range inferMap(obj, fields...) {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

// inferSelecting returns the Services of d's namespace whose selector
// selects d's pods, by name.
func inferSelecting(services []*unstructured.Unstructured, d *inferDeployment) []*unstructured.Unstructured {
	var out []*unstructured.Unstructured
	for _, s := range services {
		if s.GetNamespace() != d.ns {
			continue
		}
		sel := inferStringMap(s.Object, "spec", "selector")
		if len(sel) == 0 {
			continue
		}
		match := true
		for k, v := range sel {
			if d.podLabels[k] != v {
				match = false
			}
		}
		if match {
			out = append(out, s)
		}
	}
	return out
}

// inferPrefix returns the longest prefix p such that at least need of the
// Deployments are named p + their service name; "" when there is none.
func inferPrefix(deps []*inferDeployment, need int) string {
	candidates := map[string]bool{}
	for _, d := range deps {
		if d.svc != "" && len(d.name) > len(d.svc) && strings.HasSuffix(d.name, d.svc) {
			candidates[strings.TrimSuffix(d.name, d.svc)] = true
		}
	}
	best := ""
	for p := range candidates {
		count := 0
		for _, d := range deps {
			if d.name == p+d.svc {
				count++
			}
		}
		if count >= need && (len(p) > len(best) || len(p) == len(best) && p < best) {
			best = p
		}
	}
	return best
}

// inferFrequentPrefix returns the most frequent prefix p of the Deployments
// named p + their service name, when more than half of the Deployments have
// it (the longest, then the first by name, among equals); "" when none.
func inferFrequentPrefix(deps []*inferDeployment) (string, int) {
	count := map[string]int{}
	for _, d := range deps {
		if d.svc != "" && len(d.name) > len(d.svc) && strings.HasSuffix(d.name, d.svc) {
			count[strings.TrimSuffix(d.name, d.svc)]++
		}
	}
	best, bestN := "", 0
	for p, n := range count {
		if n*2 <= len(deps) {
			continue
		}
		if n > bestN || n == bestN && (len(p) > len(best) || len(p) == len(best) && p < best) {
			best, bestN = p, n
		}
	}
	return best, bestN
}

// inferNormalizer turns what is particular to one Deployment (its service
// name and the common prefix) into the templates of a profile.
type inferNormalizer struct {
	prefix string
	svc    string
}

const (
	inferPhName       = ""
	inferPhPrefixName = ""
	inferPhPrefix     = ""
)

// str replaces prefix+service, then the prefix, then the service by their
// templates. A token is replaced only between non-alphanumeric characters,
// so "preorders" is not "pre" + the service "orders".
func (n inferNormalizer) str(s string) string {
	if n.prefix != "" && n.svc != "" {
		s = inferReplaceToken(s, n.prefix+n.svc, inferPhPrefixName)
	}
	if n.prefix != "" {
		s = inferReplaceToken(s, n.prefix, inferPhPrefix)
	}
	if n.svc != "" {
		s = inferReplaceToken(s, n.svc, inferPhName)
	}
	s = strings.ReplaceAll(s, "{{", `{{ "{{" }}`)
	s = strings.ReplaceAll(s, inferPhPrefixName, "{{ .Vars.prefix }}{{ .Name }}")
	s = strings.ReplaceAll(s, inferPhPrefix, "{{ .Vars.prefix }}")
	return strings.ReplaceAll(s, inferPhName, "{{ .Name }}")
}

// value normalizes every string in v and returns a copy.
func (n inferNormalizer) value(v interface{}) interface{} {
	switch t := v.(type) {
	case string:
		return n.str(t)
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, x := range t {
			out[k] = n.value(x)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, x := range t {
			out[i] = n.value(x)
		}
		return out
	}
	return v
}

func inferAlnum(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func inferReplaceToken(s, token, repl string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, token)
		if i < 0 {
			break
		}
		end := i + len(token)
		leftOK := i == 0 || !inferAlnum(s[i-1])
		rightOK := !inferAlnum(token[len(token)-1]) || end == len(s) || !inferAlnum(s[end])
		if leftOK && rightOK {
			b.WriteString(s[:i])
			b.WriteString(repl)
		} else {
			b.WriteString(s[:end])
		}
		s = s[end:]
	}
	b.WriteString(s)
	return b.String()
}

// inferTally counts values by their JSON form.
type inferTally struct {
	count map[string]int
	val   map[string]interface{}
	total int
}

func inferKey(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (t *inferTally) add(v interface{}) {
	if t.count == nil {
		t.count, t.val = map[string]int{}, map[string]interface{}{}
	}
	k := inferKey(v)
	t.count[k]++
	t.val[k] = v
	t.total++
}

// best returns the most frequent value and its count; ties go to the
// smallest JSON form.
func (t *inferTally) best() (interface{}, int) {
	bestKey, n := "", 0
	for k, c := range t.count {
		if c > n || c == n && k < bestKey {
			bestKey, n = k, c
		}
	}
	return t.val[bestKey], n
}

// inferrer holds the state of one inference.
type inferrer struct {
	deps   []*inferDeployment
	need   int
	opts   InferOptions
	prefix string
	notes  []string
}

func (in *inferrer) notef(format string, args ...interface{}) {
	in.notes = append(in.notes, fmt.Sprintf(format, args...))
}

// agreed returns the most frequent of vals when at least need Deployments
// share it. When enough Deployments have the property but disagree, it
// says so in a note; when too few have it, the property is just not common.
func (in *inferrer) agreed(what string, vals []interface{}) (interface{}, bool) {
	if len(vals) < in.need {
		return nil, false
	}
	var t inferTally
	for _, v := range vals {
		t.add(v)
	}
	v, n := t.best()
	if n < in.need {
		in.notef("%s differs between the Deployments (the most frequent value is shared by %d of %d, min-share needs %d of %d); skipped",
			what, n, len(in.deps), in.need, len(in.deps))
		return nil, false
	}
	return v, true
}

// mostFrequent returns the most frequent of vals when at least need
// Deployments have the property at all; if the most frequent value is
// shared by fewer than need, it is used and a note says so.
func (in *inferrer) mostFrequent(what string, vals []interface{}) (interface{}, bool) {
	if len(vals) < in.need {
		return nil, false
	}
	var t inferTally
	for _, v := range vals {
		t.add(v)
	}
	v, n := t.best()
	if n < in.need {
		in.notef("%s differs between the Deployments; the most frequent value, shared by %d of %d, is used", what, n, len(in.deps))
	}
	return v, true
}

// inferCleanContainer drops from a container the fields the API server
// fills in and the empty ones; they say nothing about the platform.
func inferCleanContainer(c map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(c))
	for k, v := range c {
		switch t := v.(type) {
		case map[string]interface{}:
			if len(t) == 0 {
				continue
			}
		case []interface{}:
			if len(t) == 0 {
				continue
			}
		case string:
			if k == "terminationMessagePath" && t == "/dev/termination-log" || k == "terminationMessagePolicy" && t == "File" {
				continue
			}
		}
		out[k] = v
	}
	return out
}

func (in *inferrer) initContainers() []map[string]interface{} {
	type entry struct {
		specs, images inferTally
		present       int
		first         int
	}
	byName := map[string]*entry{}
	for _, d := range in.deps {
		list, _ := d.spec["initContainers"].([]interface{})
		seen := map[string]bool{}
		for i, raw := range list {
			c, _ := d.norm.value(raw).(map[string]interface{})
			name, _ := c["name"].(string)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			e := byName[name]
			if e == nil {
				e = &entry{first: i}
				byName[name] = e
			}
			if i < e.first {
				e.first = i
			}
			e.present++
			image, _ := c["image"].(string)
			e.images.add(image)
			spec := inferCleanContainer(c)
			delete(spec, "image")
			e.specs.add(spec)
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := byName[names[i]], byName[names[j]]
		if a.first != b.first {
			return a.first < b.first
		}
		return names[i] < names[j]
	})
	var out []map[string]interface{}
	for _, name := range names {
		e := byName[name]
		if e.present < in.need {
			in.notef("initContainers.%s is in %d of %d Deployments (min-share needs %d); skipped", name, e.present, len(in.deps), in.need)
			continue
		}
		spec, n := e.specs.best()
		if n < e.present {
			in.notef("initContainers.%s is defined differently in some Deployments; the most frequent definition, shared by %d of %d, is used", name, n, e.present)
		}
		image, _ := e.images.best()
		c := map[string]interface{}{}
		for k, v := range spec.(map[string]interface{}) {
			c[k] = v
		}
		c["image"] = image
		out = append(out, c)
	}
	return out
}

func (in *inferrer) envFrom() []map[string]string {
	type entry struct {
		kind, name string
		present    int
		first      int
	}
	byKey := map[string]*entry{}
	own := "{{ .Name }}-env" // the ConfigMap and Secret dhg builds itself
	for _, d := range in.deps {
		list, _ := d.main["envFrom"].([]interface{})
		seen := map[string]bool{}
		for i, raw := range list {
			m, _ := d.norm.value(raw).(map[string]interface{})
			for _, kind := range []string{"configMapRef", "secretRef"} {
				ref, _ := m[kind].(map[string]interface{})
				name, _ := ref["name"].(string)
				if name == "" || name == own {
					continue
				}
				key := kind + ":" + name
				if seen[key] {
					continue
				}
				seen[key] = true
				e := byKey[key]
				if e == nil {
					e = &entry{kind: kind, name: name, first: i}
					byKey[key] = e
				}
				if i < e.first {
					e.first = i
				}
				e.present++
			}
		}
	}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := byKey[keys[i]], byKey[keys[j]]
		if a.first != b.first {
			return a.first < b.first
		}
		return keys[i] < keys[j]
	})
	var out []map[string]string
	for _, k := range keys {
		if e := byKey[k]; e.present >= in.need {
			out = append(out, map[string]string{e.kind: e.name})
		}
	}
	return out
}

// inferSkipLabels are labels that belong to the controller, to Helm or to
// dhg itself, not to the platform.
var inferSkipLabels = map[string]bool{
	"pod-template-hash": true, "controller-revision-hash": true, "helm.sh/chart": true,
	"app.kubernetes.io/managed-by": true, "app.kubernetes.io/version": true, "app.kubernetes.io/instance": true,
	"app.kubernetes.io/name": true, "release": true, "heritage": true,
}

func (in *inferrer) selector() map[string]string {
	var keys []string
	seen := map[string]bool{}
	for _, d := range in.deps {
		for k := range d.matchLabels {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)
	out := map[string]string{}
	for _, k := range keys {
		var vals []interface{}
		for _, d := range in.deps {
			if v, ok := d.matchLabels[k]; ok {
				vals = append(vals, d.norm.str(v))
			}
		}
		if len(vals) < len(in.deps) {
			in.notef("selector.%s is not in the selector of every Deployment; skipped", k)
			continue
		}
		if v, ok := in.agreed("selector."+k, vals); ok {
			out[k] = v.(string)
		}
	}
	if len(out) == 0 || len(out) == 1 && out["app.kubernetes.io/name"] == "{{ .Name }}" {
		return nil
	}
	return out
}

// labels returns the labels (of field) that at least need Deployments have
// with the same normalized value.
func (in *inferrer) labels(field string, get func(*inferDeployment) map[string]string, selector map[string]string) map[string]string {
	seen := map[string]bool{}
	var keys []string
	for _, d := range in.deps {
		for k := range get(d) {
			if !seen[k] && !inferSkipLabels[k] && !strings.HasPrefix(k, "batch.kubernetes.io/") {
				if _, inSelector := selector[k]; !inSelector {
					seen[k] = true
					keys = append(keys, k)
				}
			}
		}
	}
	sort.Strings(keys)
	out := map[string]string{}
	for _, k := range keys {
		var vals []interface{}
		for _, d := range in.deps {
			if v, ok := get(d)[k]; ok {
				vals = append(vals, d.norm.str(v))
			}
		}
		if v, ok := in.agreed(field+"."+k, vals); ok {
			out[k] = v.(string)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// volumes returns the volumes and the volume mounts of the main container
// that are common; a volume is kept when a kept mount or a kept init
// container mounts it.
func (in *inferrer) volumes(inits []map[string]interface{}) ([]map[string]interface{}, []map[string]interface{}) {
	type mount struct {
		m          map[string]interface{}
		present    int
		first      int
		key        string
		volumeName string
	}
	mounts := map[string]*mount{}
	for _, d := range in.deps {
		list, _ := d.main["volumeMounts"].([]interface{})
		seen := map[string]bool{}
		for i, raw := range list {
			m, _ := d.norm.value(raw).(map[string]interface{})
			key := inferKey(m)
			if m == nil || seen[key] {
				continue
			}
			seen[key] = true
			e := mounts[key]
			if e == nil {
				name, _ := m["name"].(string)
				e = &mount{m: m, first: i, key: key, volumeName: name}
				mounts[key] = e
			}
			if i < e.first {
				e.first = i
			}
			e.present++
		}
	}
	var kept []*mount
	for _, e := range mounts {
		if e.present >= in.need {
			kept = append(kept, e)
		}
	}
	sort.Slice(kept, func(i, j int) bool {
		if kept[i].first != kept[j].first {
			return kept[i].first < kept[j].first
		}
		return kept[i].key < kept[j].key
	})
	used := map[string]bool{}
	var outMounts []map[string]interface{}
	for _, e := range kept {
		outMounts = append(outMounts, e.m)
		used[e.volumeName] = true
	}
	for _, c := range inits {
		list, _ := c["volumeMounts"].([]interface{})
		for _, raw := range list {
			if m, _ := raw.(map[string]interface{}); m != nil {
				if name, _ := m["name"].(string); name != "" {
					used[name] = true
				}
			}
		}
	}

	type volume struct {
		specs   inferTally
		present int
	}
	byName := map[string]*volume{}
	for _, d := range in.deps {
		list, _ := d.spec["volumes"].([]interface{})
		seen := map[string]bool{}
		for _, raw := range list {
			v, _ := d.norm.value(raw).(map[string]interface{})
			name, _ := v["name"].(string)
			if name == "" || seen[name] || !used[name] {
				continue
			}
			seen[name] = true
			e := byName[name]
			if e == nil {
				e = &volume{}
				byName[name] = e
			}
			e.present++
			e.specs.add(v)
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	var outVolumes []map[string]interface{}
	for _, name := range names {
		e := byName[name]
		if e.present < in.need {
			continue
		}
		spec, n := e.specs.best()
		if n < e.present {
			in.notef("volumes.%s is defined differently in some Deployments; the most frequent definition, shared by %d of %d, is used", name, n, e.present)
		}
		outVolumes = append(outVolumes, spec.(map[string]interface{}))
	}
	return outVolumes, outMounts
}

// patch returns the fields of the container or the pod spec that at least
// need Deployments have with the same normalized, non-empty value.
func (in *inferrer) patch(field string, keys []string, get func(*inferDeployment) map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, key := range keys {
		var vals []interface{}
		for _, d := range in.deps {
			v, ok := get(d)[key]
			if !ok || inferEmpty(v) {
				continue
			}
			vals = append(vals, d.norm.value(v))
		}
		if v, ok := in.agreed(field+"."+key, vals); ok {
			out[key] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func inferEmpty(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case map[string]interface{}:
		return len(t) == 0
	case []interface{}:
		return len(t) == 0
	}
	return false
}

func inferInt(v interface{}) (int64, bool) {
	switch t := v.(type) {
	case int64:
		return t, true
	case int:
		return int64(t), true
	case int32:
		return int64(t), true
	case float64:
		return int64(t), t == math.Trunc(t)
	}
	return 0, false
}

func (in *inferrer) defaults() map[string]interface{} {
	out := map[string]interface{}{}

	var res []interface{}
	for _, d := range in.deps {
		r := map[string]interface{}{}
		for _, k := range []string{"requests", "limits"} {
			if m, _ := d.main["resources"].(map[string]interface{}); m != nil {
				if q, _ := m[k].(map[string]interface{}); len(q) > 0 {
					r[k] = q
				}
			}
		}
		if len(r) > 0 {
			res = append(res, r)
		}
	}
	if v, ok := in.mostFrequent("defaults.resources", res); ok {
		out["resources"] = v
	}

	probes := map[string]interface{}{}
	for _, kind := range []struct{ key, field string }{
		{"startup", "startupProbe"}, {"liveness", "livenessProbe"}, {"readiness", "readinessProbe"},
	} {
		var vals []interface{}
		for _, d := range in.deps {
			if p := in.probe(d, kind.field); p != nil {
				vals = append(vals, p)
			}
		}
		if v, ok := in.mostFrequent("defaults.probes."+kind.key, vals); ok {
			probes[kind.key] = v
		}
	}
	if len(probes) > 0 {
		out["probes"] = probes
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// containerPorts returns the main container's ports by name and the first
// port number.
func (d *inferDeployment) containerPorts() (named map[string]int64, numbers map[int64]bool, first int64) {
	named, numbers = map[string]int64{}, map[int64]bool{}
	list, _ := d.main["ports"].([]interface{})
	for i, raw := range list {
		p, _ := raw.(map[string]interface{})
		n, ok := inferInt(p["containerPort"])
		if !ok {
			continue
		}
		if i == 0 || first == 0 {
			first = n
		}
		numbers[n] = true
		if name, _ := p["name"].(string); name != "" {
			named[name] = n
		}
	}
	return named, numbers, first
}

// probe converts the container's probe (field) to the profile's form; nil
// when the container has none or the profile cannot say it.
func (in *inferrer) probe(d *inferDeployment, field string) map[string]interface{} {
	raw, _ := d.main[field].(map[string]interface{})
	if raw == nil {
		return nil
	}
	named, _, first := d.containerPorts()
	out := map[string]interface{}{}
	port := func(handler map[string]interface{}) {
		var n int64
		switch p := handler["port"].(type) {
		case string:
			n = named[p]
		default:
			n, _ = inferInt(p)
		}
		if n != 0 && n != first {
			out["port"] = n
		}
	}
	switch {
	case raw["httpGet"] != nil:
		h, _ := raw["httpGet"].(map[string]interface{})
		path, _ := h["path"].(string)
		if path == "" {
			path = "/"
		}
		out["path"] = d.norm.str(path)
		port(h)
		if scheme, _ := h["scheme"].(string); scheme == "HTTPS" || h["httpHeaders"] != nil {
			in.notef("%s of %s uses HTTPS or headers, which a profile cannot say; the probe is plain HTTP in the profile", field, d.name)
		}
	case raw["tcpSocket"] != nil:
		h, _ := raw["tcpSocket"].(map[string]interface{})
		out["tcp"] = true
		port(h)
	case raw["exec"] != nil:
		h, _ := raw["exec"].(map[string]interface{})
		cmd, _ := d.norm.value(h["command"]).([]interface{})
		if len(cmd) == 0 {
			return nil
		}
		out["exec"] = cmd
	default:
		in.notef("%s of %s is neither HTTP, TCP nor exec (gRPC?); a profile cannot say it, skipped", field, d.name)
		return nil
	}
	for _, k := range []string{"periodSeconds", "timeoutSeconds", "failureThreshold", "successThreshold", "initialDelaySeconds"} {
		if n, ok := inferInt(raw[k]); ok && n != 0 {
			out[k] = n
		}
	}
	return out
}

func (in *inferrer) naming() map[string]string {
	out := map[string]string{}
	for _, f := range []struct {
		key  string
		name func(*inferDeployment) (string, bool)
	}{
		{"deployment", func(d *inferDeployment) (string, bool) { return d.name, true }},
		{"container", func(d *inferDeployment) (string, bool) {
			n, _ := d.main["name"].(string)
			return n, n != ""
		}},
		{"service", func(d *inferDeployment) (string, bool) {
			if s := d.firstService(); s != nil {
				return s.GetName(), true
			}
			return "", false
		}},
	} {
		var vals []interface{}
		for _, d := range in.deps {
			if name, ok := f.name(d); ok {
				vals = append(vals, d.norm.str(name))
			}
		}
		if v, ok := in.mostFrequent("naming."+f.key, vals); ok && v.(string) != "{{ .Name }}" {
			out[f.key] = v.(string)
		}
	}
	multi := 0
	for _, d := range in.deps {
		if len(d.services) > 1 {
			multi++
		}
	}
	if multi > 0 {
		in.notef("%d of %d Deployments are selected by more than one Service; the first by name is used for naming.service and service", multi, len(in.deps))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (d *inferDeployment) firstService() *unstructured.Unstructured {
	if len(d.services) == 0 {
		return nil
	}
	return d.services[0]
}

// service infers the Service type and the ports the application does not
// have.
func (in *inferrer) service() map[string]interface{} {
	out := map[string]interface{}{}
	var types, extras []interface{}
	shifted := map[string]int{}
	example := map[string]string{}
	for _, d := range in.deps {
		s := d.firstService()
		if s == nil {
			continue
		}
		for _, sh := range inferShiftedPorts(d, s) {
			shifted[sh]++
			if example[sh] == "" {
				example[sh] = s.GetName()
			}
		}
		if t, _, _ := unstructured.NestedString(s.Object, "spec", "type"); t != "" {
			types = append(types, t)
		}
		if extra := inferExtraPorts(d, s); len(extra) > 0 {
			extras = append(extras, extra)
		}
	}
	for sh, n := range shifted {
		in.notef("Service port %s: the application's port serves it, so the chart exposes the Service on the application's port, not on the number the stand has (%d of %d Deployments, for example %s); clients that use the stand's number break: add the port to service.extraPorts if they must keep it", sh, n, len(in.deps), example[sh])
	}
	if v, ok := in.mostFrequent("service.type", types); ok && v != "ClusterIP" {
		out["type"] = v
	}
	if v, ok := in.mostFrequent("service.extraPorts", extras); ok {
		out["extraPorts"] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// inferShiftedPorts returns, as "80 -> 8080", the ports of s that a port of
// d's main container serves under another number: the profile drops them as
// served.
func inferShiftedPorts(d *inferDeployment, s *unstructured.Unstructured) []string {
	named, numbers, _ := d.containerPorts()
	list, _, _ := unstructured.NestedSlice(s.Object, "spec", "ports")
	var out []string
	for _, raw := range list {
		p, _ := raw.(map[string]interface{})
		port, ok := inferInt(p["port"])
		if !ok {
			continue
		}
		var target int64
		switch t := p["targetPort"].(type) {
		case string:
			n, ok := named[t]
			if !ok {
				continue
			}
			target = n
		case nil:
			continue
		default:
			target, _ = inferInt(t)
			if !numbers[target] {
				continue
			}
		}
		if target != port {
			out = append(out, strconv.FormatInt(port, 10)+" -> "+strconv.FormatInt(target, 10))
		}
	}
	return out
}

// inferExtraPorts returns the ports of s that no port of d's main container
// serves.
func inferExtraPorts(d *inferDeployment, s *unstructured.Unstructured) []interface{} {
	named, numbers, _ := d.containerPorts()
	list, _, _ := unstructured.NestedSlice(s.Object, "spec", "ports")
	type extra struct {
		port     int64
		protocol string
		m        map[string]interface{}
	}
	var found []extra
	for _, raw := range list {
		p, _ := raw.(map[string]interface{})
		port, ok := inferInt(p["port"])
		if !ok {
			continue
		}
		var served bool
		var target interface{} = port
		switch t := p["targetPort"].(type) {
		case string:
			_, served = named[t]
			target = t
		case nil:
			served = numbers[port]
		default:
			n, _ := inferInt(t)
			served = numbers[n]
			target = n
		}
		if served {
			continue
		}
		name, _ := p["name"].(string)
		if name == "" {
			name = "port-" + strconv.FormatInt(port, 10)
		}
		m := map[string]interface{}{"name": name, "port": port}
		if target != interface{}(port) {
			m["targetPort"] = target
		}
		protocol, _ := p["protocol"].(string)
		if protocol != "" && protocol != "TCP" {
			m["protocol"] = protocol
		}
		found = append(found, extra{port: port, protocol: protocol, m: m})
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].port != found[j].port {
			return found[i].port < found[j].port
		}
		return found[i].protocol < found[j].protocol
	})
	out := make([]interface{}, len(found))
	for i, e := range found {
		out[i] = e.m
	}
	return out
}

func (in *inferrer) strategy() map[string]interface{} {
	def := map[string]interface{}{
		"type":          "RollingUpdate",
		"rollingUpdate": map[string]interface{}{"maxSurge": "25%", "maxUnavailable": "25%"},
	}
	var vals []interface{}
	for _, d := range in.deps {
		s := inferMap(d.obj.Object, "spec", "strategy")
		if s == nil {
			continue
		}
		typ, _ := s["type"].(string)
		switch typ {
		case "Recreate":
			vals = append(vals, map[string]interface{}{"type": "Recreate"})
		case "", "RollingUpdate":
			ru, _ := s["rollingUpdate"].(map[string]interface{})
			surge, unavailable := interface{}("25%"), interface{}("25%")
			if v, ok := ru["maxSurge"]; ok {
				surge = v
			}
			if v, ok := ru["maxUnavailable"]; ok {
				unavailable = v
			}
			vals = append(vals, map[string]interface{}{
				"type":          "RollingUpdate",
				"rollingUpdate": map[string]interface{}{"maxSurge": surge, "maxUnavailable": unavailable},
			})
		}
	}
	v, ok := in.mostFrequent("strategy", vals)
	if !ok || inferKey(v) == inferKey(def) {
		return nil
	}
	return v.(map[string]interface{})
}

// inferVars turns environment-specific strings of the profile into
// variables.
type inferVars struct {
	values map[string]string
	// redacted holds the places whose URL had credentials in it.
	redacted map[string]bool
}

// inferVarCtx is where the walk is: the name of the nearest named list
// element (the container), the keys since then, and the name of the
// environment variable whose value this is.
type inferVarCtx struct {
	owner   string
	words   []string
	envName string
	key     string
}

// inferSkipKeys are fields whose strings are names, paths and kinds, not
// addresses of an environment.
var inferSkipKeys = map[string]bool{
	"name": true, "image": true, "mountPath": true, "subPath": true, "path": true, "key": true,
	"secretName": true, "claimName": true, "apiVersion": true, "kind": true, "fieldPath": true,
}

// label names the place of the walk for a note: the owner and the
// environment variable, or the keys.
func (c inferVarCtx) label() string {
	parts := []string{}
	if c.owner != "" {
		parts = append(parts, c.owner)
	}
	if c.envName != "" {
		parts = append(parts, c.envName)
	} else {
		parts = append(parts, c.words...)
	}
	return strings.Join(parts, " ")
}

func (x *inferVars) walk(v interface{}, c inferVarCtx) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		name, hasName := t["name"].(string)
		_, hasValue := t["value"]
		if c.owner == "" && hasName {
			c.owner = name
		}
		out := make(map[string]interface{}, len(t))
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := c
			child.key = k
			if hasName && hasValue && k == "value" {
				child.envName = name
			} else {
				child.words = append(append([]string(nil), c.words...), k)
			}
			out[k] = x.walk(t[k], child)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, el := range t {
			child := c
			if m, ok := el.(map[string]interface{}); ok {
				if name, _ := m["name"].(string); name != "" {
					out[i] = x.walk(el, child)
					continue
				}
			}
			child.words = append(append([]string(nil), c.words...), strconv.Itoa(i))
			out[i] = x.walk(el, child)
		}
		return out
	case string:
		if inferSkipKeys[c.key] {
			return t
		}
		if red, ok := inferRedactURL(t); ok {
			x.redacted[c.label()] = true
			t = red
		}
		if !strings.Contains(t, "{{") {
			if head, val, ok := inferEnvSpecific(t); ok {
				return head + x.ref(c, "", val)
			}
			return x.words(t, c)
		}
		if head, host, tail, ok := inferTemplatedHost(t); ok {
			return head + x.ref(c, "Host", host) + tail
		}
	}
	return v
}

// inferWordRe is a word of a command line.
var inferWordRe = regexp.MustCompile(`\S+`)

// words makes variables of the IPv4 addresses and URLs that stand as words
// of a command (a shell script of an init container: "until nc -z 10.1.2.3
// 5432"), quotes and ";" around the word kept. Bare host names are not
// recognised here: in a script they are mostly file names.
func (x *inferVars) words(s string, c inferVarCtx) string {
	if !strings.ContainsAny(s, " \t\n") {
		return s
	}
	const edge = "\"';&|()"
	return inferWordRe.ReplaceAllStringFunc(s, func(w string) string {
		core := strings.TrimLeft(w, edge)
		lead := w[:len(w)-len(core)]
		core = strings.TrimRight(core, edge)
		trail := w[len(lead)+len(core):]
		if red, ok := inferRedactURL(core); ok {
			x.redacted[c.label()] = true
			core = red
		}
		head, val, ok := inferEnvSpecific(core)
		if !ok || !inferIPv4.MatchString(val) && !inferURL.MatchString(val) {
			return w
		}
		return lead + head + x.ref(c, "", val) + trail
	})
}

// ref records value as a variable named after the place it was found and
// returns the reference to it.
func (x *inferVars) ref(c inferVarCtx, suffix, value string) string {
	var cands []string
	if c.envName != "" {
		cands = []string{inferCamel(c.envName) + suffix, inferCamel(c.owner, c.envName) + suffix}
	} else {
		last := c.words
		if len(last) > 2 {
			last = last[len(last)-2:]
		}
		cands = []string{inferCamel(append([]string{c.owner}, last...)...) + suffix, inferCamel(append([]string{c.owner}, c.words...)...) + suffix}
	}
	for i := 0; ; i++ {
		var key string
		if i < len(cands) {
			key = cands[i]
		} else {
			key = cands[len(cands)-1] + strconv.Itoa(i-len(cands)+2)
		}
		if key == "prefix" {
			continue
		}
		have, taken := x.values[key]
		if !taken {
			x.values[key] = value
			return "{{ .Vars." + key + " }}"
		}
		if have == value {
			return "{{ .Vars." + key + " }}"
		}
	}
}

var inferWord = regexp.MustCompile(`[A-Za-z0-9]+`)

// inferCamel joins words into a template identifier: "DB_HOST" is "dbHost",
// "wait-for-db", "args" is "waitForDbArgs".
func inferCamel(parts ...string) string {
	var b strings.Builder
	first := true
	for _, p := range parts {
		for _, w := range inferWord.FindAllString(p, -1) {
			if w == strings.ToUpper(w) {
				w = strings.ToLower(w)
			}
			if first {
				b.WriteString(strings.ToLower(w[:1]) + w[1:])
				first = false
			} else {
				b.WriteString(strings.ToUpper(w[:1]) + w[1:])
			}
		}
	}
	s := b.String()
	if s == "" {
		return "value"
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = "v" + s
	}
	return s
}

var (
	inferIPv4     = regexp.MustCompile(`^(?:(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\.){3}(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])(?::[0-9]{1,5})?$`)
	inferHostname = regexp.MustCompile(`^(?:[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?\.)+([A-Za-z][A-Za-z0-9-]*)(?::[0-9]{1,5})?$`)
	// inferURL matches URLs with a host: jdbc:<db>:[<sub>:]//host, jdbc:<db>:thin:@host, http(s)://host.
	inferURL    = regexp.MustCompile(`^((?:jdbc:[A-Za-z0-9]+:(?:[A-Za-z0-9]+:)?(?://|@)|https?://))((?:[^/@\s]*@)?)([^/\s:?;]+)`)
	inferAssign = regexp.MustCompile(`^(-{0,2}[A-Za-z][A-Za-z0-9_.-]*=)(.+)$`)
)

// inferFileExtensions are the last labels of "hostnames" that are file names.
var inferFileExtensions = map[string]bool{
	"yaml": true, "yml": true, "json": true, "sh": true, "jar": true, "war": true, "py": true, "js": true, "conf": true,
	"cfg": true, "ini": true, "properties": true, "xml": true, "txt": true, "log": true, "toml": true, "sql": true,
	"crt": true, "pem": true, "key": true, "so": true, "tar": true, "gz": true, "zip": true, "html": true, "md": true,
	"tpl": true, "env": true, "csv": true, "jks": true, "p12": true, "lock": true, "pid": true, "sock": true,
}

func inferLocalHost(h string) bool {
	h = strings.ToLower(h)
	return h == "localhost" || strings.HasPrefix(h, "127.") || h == "0.0.0.0"
}

// inferIsAddress reports whether s is an IPv4 address or a dotted host name,
// each with an optional port.
func inferIsAddress(s string) bool {
	if inferIPv4.MatchString(s) {
		return !inferLocalHost(s)
	}
	m := inferHostname.FindStringSubmatch(s)
	return m != nil && !inferFileExtensions[strings.ToLower(m[1])] && !inferLocalHost(s)
}

// inferEnvSpecific reports whether s, which holds no template, is an address
// of an environment: an IPv4 address, a dotted host name or a jdbc/http URL
// with a host; or "name=" followed by one. head is the part before the
// address ("--host=") and value the address.
func inferEnvSpecific(s string) (head, value string, ok bool) {
	check := func(s string) bool {
		if s == "" || strings.Contains(s, "{{") {
			return false
		}
		if m := inferURL.FindStringSubmatch(s); m != nil {
			return !inferLocalHost(m[3])
		}
		return inferIsAddress(s)
	}
	if check(s) {
		return "", s, true
	}
	if m := inferAssign.FindStringSubmatch(s); m != nil && check(m[2]) {
		return m[1], m[2], true
	}
	return "", "", false
}

// inferTemplatedHost finds, in a URL that holds a template (a database URL
// with the service's name in it), a literal host to make a variable of:
// head + host + tail is s.
func inferTemplatedHost(s string) (head, host, tail string, ok bool) {
	start := ""
	rest := s
	if m := inferAssign.FindStringSubmatch(s); m != nil && !strings.HasPrefix(s, "jdbc:") && !strings.HasPrefix(s, "http") {
		start, rest = m[1], m[2]
	}
	m := inferURL.FindStringSubmatch(rest)
	if m == nil || strings.Contains(m[3], "{{") || strings.Contains(m[2], "{{") || inferLocalHost(m[3]) {
		return "", "", "", false
	}
	return start + m[1] + m[2], m[3], rest[len(m[0]):], true
}

// inferRedactURL removes the userinfo (user:password@) of the URL in s, or
// of the URL after "name=", and reports whether there was any. A userinfo
// that is a template is not a secret and stays.
func inferRedactURL(s string) (string, bool) {
	start, rest := "", s
	if m := inferAssign.FindStringSubmatch(s); m != nil && !strings.HasPrefix(s, "jdbc:") && !strings.HasPrefix(s, "http") {
		start, rest = m[1], m[2]
	}
	m := inferURL.FindStringSubmatch(rest)
	if m == nil || m[2] == "" || strings.Contains(m[2], "{{") {
		return s, false
	}
	return start + m[1] + rest[len(m[1])+len(m[2]):], true
}
