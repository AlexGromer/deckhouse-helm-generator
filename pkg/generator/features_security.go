package generator

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// Security, secrets and policy features (`dhg generate --with ...`).
//
// Every feature here only adds its own templates (under a unique path), its
// own top-level values key (guarded by an `enabled` toggle) and, for
// non-template output, files outside templates/. The only edit to generated
// templates is the single pod-annotations hook used by vault-agent.

func init() {
	RegisterFeature(Feature{
		Name:        "policies",
		Description: "Ship workload security policies: Kyverno Policy resources (engine=kyverno) and/or conftest Rego files in policy/ (engine=conftest)",
		Params: map[string]string{
			"engine":     "kyverno",
			"rules":      strings.Join(defaultPolicyRules, ","),
			"action":     "Audit",
			"registries": "",
		},
		Apply: applyPoliciesFeature,
	})
	RegisterFeature(Feature{
		Name:        "external-secrets",
		Description: "Create External Secrets Operator ExternalSecrets for every Secret the workloads use, read from an existing (Cluster)SecretStore",
		Params: map[string]string{
			"store":           "secret-store",
			"store-kind":      "ClusterSecretStore",
			"refresh":         "1h",
			"key-prefix":      "",
			"api-version":     "external-secrets.io/v1",
			"creation-policy": "Owner",
		},
		Apply: applyExternalSecretsFeature,
	})
	RegisterFeature(Feature{
		Name:        "vault-agent",
		Description: "Annotate Deployments/StatefulSets/DaemonSets that use Secrets for HashiCorp Vault Agent injection (secrets rendered to /vault/secrets/<name>)",
		Params: map[string]string{
			"role":              "",
			"auth-path":         "",
			"path-prefix":       "secret/data",
			"kv-version":        "2",
			"format":            "env",
			"pre-populate-only": "false",
		},
		Apply: applyVaultAgentFeature,
	})
	RegisterFeature(Feature{
		Name:        "istio-egress",
		Description: "Generate Istio ServiceEntries for external hosts the workloads call (detected from URL-like env vars, plus hosts=...)",
		Params: map[string]string{
			"hosts":       "",
			"detect":      "true",
			"export-to":   ".",
			"api-version": "networking.istio.io/v1",
		},
		Apply: applyIstioEgressFeature,
	})
}

// ---------------------------------------------------------------------------
// Helpers shared by the security features.
// ---------------------------------------------------------------------------

// secHelperRef matches references to a chart's named templates, e.g.
// `define "app.fullname"` or `include "app.labels"`.
var secHelperRef = regexp.MustCompile(`(?:define|include|template) "([^"]+)\.(fullname|labels)"`)

// secChartHelpers knows how injected templates can reach the chart's own
// named templates (their prefix is the chart name in every output mode, but a
// library chart's wrappers use the library's prefix).
type secChartHelpers struct {
	fullnameTpl string
	labelsTpl   string
}

func newSecChartHelpers(chart *types.GeneratedChart) secChartHelpers {
	var h secChartHelpers
	sources := []string{chart.Helpers}
	for _, path := range secSortedTemplatePaths(chart) {
		sources = append(sources, chart.Templates[path])
	}
	for _, src := range sources {
		for _, m := range secHelperRef.FindAllStringSubmatch(src, -1) {
			name := m[1] + "." + m[2]
			if m[2] == "fullname" && h.fullnameTpl == "" {
				h.fullnameTpl = name
			}
			if m[2] == "labels" && h.labelsTpl == "" {
				h.labelsTpl = name
			}
		}
	}
	return h
}

// fullname returns a template expression (usable as a pipeline operand) for
// the release-scoped resource name prefix.
func (h secChartHelpers) fullname() string {
	if h.fullnameTpl == "" {
		return "$.Release.Name"
	}
	return fmt.Sprintf("(include %q $)", h.fullnameTpl)
}

// name returns a template action producing "<fullname>-<suffix>", truncated
// to a valid object name.
func (h secChartHelpers) name(suffixExpr string) string {
	return fmt.Sprintf(`{{ printf "%%s-%%s" %s %s | trunc 63 | trimSuffix "-" }}`, h.fullname(), suffixExpr)
}

// labels returns a metadata.labels block at the given indentation (the
// "labels:" key itself is indented by indent-2).
func (h secChartHelpers) labels(indent int) string {
	pad := strings.Repeat(" ", indent-2)
	if h.labelsTpl == "" {
		return pad + "labels:\n" + pad + "  app.kubernetes.io/managed-by: {{ $.Release.Service }}\n"
	}
	return fmt.Sprintf("%slabels:\n%s  {{- include %q $ | nindent %d }}\n", pad, pad, h.labelsTpl, indent)
}

func secSortedTemplatePaths(chart *types.GeneratedChart) []string {
	paths := make([]string, 0, len(chart.Templates))
	for p := range chart.Templates {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// secWorkloadKinds are the kinds whose objects carry a pod template.
var secWorkloadKinds = map[string]bool{
	"Deployment": true, "StatefulSet": true, "DaemonSet": true, "ReplicaSet": true,
	"Job": true, "CronJob": true, "Pod": true,
}

// secPodSpec returns the pod spec of a workload object, or nil.
func secPodSpec(obj map[string]interface{}, kind string) map[string]interface{} {
	var path []string
	switch kind {
	case "Pod":
		path = []string{"spec"}
	case "CronJob":
		path = []string{"spec", "jobTemplate", "spec", "template", "spec"}
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job":
		path = []string{"spec", "template", "spec"}
	default:
		return nil
	}
	cur := obj
	for _, p := range path {
		next, ok := cur[p].(map[string]interface{})
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

// secContainers returns all containers and init containers of a pod spec.
func secContainers(podSpec map[string]interface{}) []map[string]interface{} {
	var out []map[string]interface{}
	for _, field := range []string{"initContainers", "containers"} {
		list, _ := podSpec[field].([]interface{})
		for _, c := range list {
			if m, ok := c.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
	}
	return out
}

// secWorkload is an input workload that is rendered by a given chart.
type secWorkload struct {
	res     *types.ProcessedResource
	kind    string
	name    string
	podSpec map[string]interface{}
}

// secAllWorkloads returns every workload of the graph in a stable order.
func secAllWorkloads(graph *types.ResourceGraph) []secWorkload {
	if graph == nil {
		return nil
	}
	keys := make([]types.ResourceKey, 0, len(graph.Resources))
	for k := range graph.Resources {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })

	var out []secWorkload
	for _, k := range keys {
		r := graph.Resources[k]
		if r == nil || r.Original == nil || r.Original.Object == nil {
			continue
		}
		kind := r.Original.Object.GetKind()
		if !secWorkloadKinds[kind] {
			continue
		}
		spec := secPodSpec(r.Original.Object.Object, kind)
		if spec == nil {
			continue
		}
		out = append(out, secWorkload{res: r, kind: kind, name: r.Original.Object.GetName(), podSpec: spec})
	}
	return out
}

// inChart reports whether the workload is rendered by chart.
func (w secWorkload) inChart(chart *types.GeneratedChart) bool {
	if w.res.TemplatePath == "" {
		return false
	}
	_, ok := chart.Templates[w.res.TemplatePath]
	return ok
}

// secChartWorkloads returns the graph workloads rendered by chart.
func secChartWorkloads(chart *types.GeneratedChart, graph *types.ResourceGraph) []secWorkload {
	var out []secWorkload
	for _, w := range secAllWorkloads(graph) {
		if w.inChart(chart) {
			out = append(out, w)
		}
	}
	return out
}

// secHasWorkloadTemplate reports whether any template of chart renders a pod
// controller (used when the graph cannot be mapped to the chart).
var secWorkloadKindLine = regexp.MustCompile(`(?m)^kind: (Deployment|StatefulSet|DaemonSet|ReplicaSet|Job|CronJob|Pod)\s*$`)

func secHasWorkloadTemplate(chart *types.GeneratedChart) bool {
	for _, content := range chart.Templates {
		if secWorkloadKindLine.MatchString(content) {
			return true
		}
	}
	return false
}

// secSecretRef is one Secret used by a workload.
type secSecretRef struct {
	Name string
	// Keys are the individual keys referenced via secretKeyRef or volume items.
	Keys []string
	// Whole is true when the whole Secret is consumed (envFrom, volume
	// without items), so all its keys are needed.
	Whole bool
}

// secSecretRefs returns the Secrets a pod spec consumes, sorted by name.
// Image pull secrets are not included: they hold registry credentials that
// are provisioned differently from application secrets.
func secSecretRefs(podSpec map[string]interface{}) []secSecretRef {
	refs := map[string]*secSecretRef{}
	get := func(name string) *secSecretRef {
		if refs[name] == nil {
			refs[name] = &secSecretRef{Name: name}
		}
		return refs[name]
	}
	addKey := func(name, key string) {
		r := get(name)
		for _, k := range r.Keys {
			if k == key {
				return
			}
		}
		r.Keys = append(r.Keys, key)
	}

	for _, c := range secContainers(podSpec) {
		envList, _ := c["env"].([]interface{})
		for _, e := range envList {
			em, _ := e.(map[string]interface{})
			vf, _ := em["valueFrom"].(map[string]interface{})
			skr, _ := vf["secretKeyRef"].(map[string]interface{})
			name, _ := skr["name"].(string)
			key, _ := skr["key"].(string)
			if name != "" && key != "" {
				addKey(name, key)
			}
		}
		envFrom, _ := c["envFrom"].([]interface{})
		for _, e := range envFrom {
			em, _ := e.(map[string]interface{})
			sr, _ := em["secretRef"].(map[string]interface{})
			if name, _ := sr["name"].(string); name != "" {
				get(name).Whole = true
			}
		}
	}

	addVolumeSecret := func(name string, items []interface{}) {
		if name == "" {
			return
		}
		if len(items) == 0 {
			get(name).Whole = true
			return
		}
		for _, it := range items {
			im, _ := it.(map[string]interface{})
			if key, _ := im["key"].(string); key != "" {
				addKey(name, key)
			}
		}
	}
	volumes, _ := podSpec["volumes"].([]interface{})
	for _, v := range volumes {
		vm, _ := v.(map[string]interface{})
		if s, ok := vm["secret"].(map[string]interface{}); ok {
			name, _ := s["secretName"].(string)
			items, _ := s["items"].([]interface{})
			addVolumeSecret(name, items)
		}
		projected, _ := vm["projected"].(map[string]interface{})
		sources, _ := projected["sources"].([]interface{})
		for _, src := range sources {
			sm, _ := src.(map[string]interface{})
			if s, ok := sm["secret"].(map[string]interface{}); ok {
				name, _ := s["name"].(string)
				items, _ := s["items"].([]interface{})
				addVolumeSecret(name, items)
			}
		}
	}

	out := make([]secSecretRef, 0, len(refs))
	for _, r := range refs {
		sort.Strings(r.Keys)
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// secAddValues appends a top-level values block to the chart, failing with a
// clear message if the key is already taken.
func secAddValues(chart *types.GeneratedChart, key, comment string, value interface{}) error {
	values := chart.ValuesYAML
	if comment != "" {
		if values != "" && !strings.HasSuffix(values, "\n") {
			values += "\n"
		}
		values += "\n" + comment
	}
	out, err := appendTopLevelValues(values, key, value)
	if err != nil {
		return err
	}
	if comment != "" {
		// appendTopLevelValues separates the block with a blank line; keep the
		// comment directly above the key.
		out = strings.Replace(out, comment+"\n"+key+":", comment+key+":", 1)
	}
	chart.ValuesYAML = out
	return nil
}

// secAddTemplate adds a template under a path that must not exist yet.
func secAddTemplate(chart *types.GeneratedChart, path, content string) error {
	if _, exists := chart.Templates[path]; exists {
		return fmt.Errorf("template %s already exists", path)
	}
	chart.Templates[path] = content
	return nil
}
