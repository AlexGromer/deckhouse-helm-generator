package generator

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

const configChecksumsValuesKey = "configChecksums"

// ChecksumAnnotation is a pod annotation holding the hash of a rendered
// ConfigMap or Secret template, so that pods roll when its content changes.
type ChecksumAnnotation struct {
	// Key is the annotation key, e.g. "checksum/configmap-app-config".
	Key string
	// TemplatePath is the chart template rendering the referenced object
	// (e.g. "templates/web-configmap-app-config.yaml").
	TemplatePath string
	// Expression is the Helm expression computing the checksum.
	Expression string
}

// GenerateChecksumAnnotations returns checksum annotations for the ConfigMaps
// and Secrets that workload references (volumes, envFrom, env valueFrom) and
// that are rendered by the same chart, sorted by key. References to objects outside the
// chart (another subchart, or objects not part of the input) are ignored.
func GenerateChecksumAnnotations(workload *types.ProcessedResource, graph *types.ResourceGraph, chart *types.GeneratedChart) []ChecksumAnnotation {
	if workload == nil || workload.Original == nil || graph == nil || chart == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []ChecksumAnnotation
	for _, ref := range configReferences(workload) {
		target, ok := graph.GetResourceByKey(ref)
		if !ok || target.TemplatePath == "" || !strings.HasPrefix(target.TemplatePath, "templates/") {
			continue
		}
		content, ok := chart.Templates[target.TemplatePath]
		if !ok || opsTemplateKind(content) != ref.GVK.Kind {
			continue
		}
		key := checksumAnnotationKey(ref.GVK.Kind, ref.Name)
		if seen[key] {
			continue
		}
		seen[key] = true
		rel := strings.TrimPrefix(target.TemplatePath, "templates/")
		out = append(out, ChecksumAnnotation{
			Key:          key,
			TemplatePath: target.TemplatePath,
			Expression:   fmt.Sprintf(`{{ include (print $.Template.BasePath %q) $ | sha256sum | quote }}`, "/"+rel),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// configReferences returns the keys of the ConfigMaps and Secrets a
// workload's pod template references: volumes (including projected
// sources), envFrom and env valueFrom of all containers and init containers.
// The pod spec is read directly so the result does not depend on which
// relationships the analyzer detected.
func configReferences(workload *types.ProcessedResource) []types.ResourceKey {
	obj := workload.Original.Object
	podSpecPath := []string{"spec", "template", "spec"}
	if workload.Original.GVK.Kind == "CronJob" {
		podSpecPath = []string{"spec", "jobTemplate", "spec", "template", "spec"}
	}
	podSpec, found, _ := unstructured.NestedMap(obj.Object, podSpecPath...)
	if !found {
		return nil
	}
	ns := obj.GetNamespace()
	seen := map[types.ResourceKey]bool{}
	var refs []types.ResourceKey
	add := func(kind string, name interface{}) {
		n, ok := name.(string)
		if !ok || n == "" {
			return
		}
		key := types.ResourceKey{GVK: schema.GroupVersionKind{Version: "v1", Kind: kind}, Namespace: ns, Name: n}
		if !seen[key] {
			seen[key] = true
			refs = append(refs, key)
		}
	}
	asMap := func(v interface{}) map[string]interface{} {
		m, _ := v.(map[string]interface{})
		return m
	}
	asSlice := func(v interface{}) []interface{} {
		s, _ := v.([]interface{})
		return s
	}

	for _, v := range asSlice(podSpec["volumes"]) {
		vol := asMap(v)
		if cm := asMap(vol["configMap"]); cm != nil {
			add("ConfigMap", cm["name"])
		}
		if sec := asMap(vol["secret"]); sec != nil {
			add("Secret", sec["secretName"])
		}
		for _, src := range asSlice(asMap(vol["projected"])["sources"]) {
			if cm := asMap(asMap(src)["configMap"]); cm != nil {
				add("ConfigMap", cm["name"])
			}
			if sec := asMap(asMap(src)["secret"]); sec != nil {
				add("Secret", sec["name"])
			}
		}
	}
	containers := append(asSlice(podSpec["initContainers"]), asSlice(podSpec["containers"])...)
	for _, c := range containers {
		container := asMap(c)
		for _, e := range asSlice(container["envFrom"]) {
			if cm := asMap(asMap(e)["configMapRef"]); cm != nil {
				add("ConfigMap", cm["name"])
			}
			if sec := asMap(asMap(e)["secretRef"]); sec != nil {
				add("Secret", sec["name"])
			}
		}
		for _, e := range asSlice(container["env"]) {
			valueFrom := asMap(asMap(e)["valueFrom"])
			if cm := asMap(valueFrom["configMapKeyRef"]); cm != nil {
				add("ConfigMap", cm["name"])
			}
			if sec := asMap(valueFrom["secretKeyRef"]); sec != nil {
				add("Secret", sec["name"])
			}
		}
	}
	return refs
}

// checksumAnnotationKey builds "checksum/<kind>-<name>", keeping the name part
// within the 63-character limit of annotation names.
func checksumAnnotationKey(kind, name string) string {
	part := strings.ToLower(kind) + "-" + name
	if len(part) > 63 {
		part = strings.TrimRight(part[:63], "-.")
	}
	return "checksum/" + part
}

// podAnnotationsBlockRe matches the values-driven pod annotations block the
// workload processors render inside spec.template.metadata.
var podAnnotationsBlockRe = regexp.MustCompile(`(?m)^( *)\{\{- with \.podAnnotations \}\}\n *annotations:\n *\{\{- toYaml \. \| nindent \d+ \}\}\n *\{\{- end \}\}\n`)

// InjectConfigChecksums adds checksum/* pod annotations to every Deployment,
// StatefulSet and DaemonSet template whose workload references ConfigMaps or
// Secrets rendered by the same chart. The annotations render while
// configChecksums.enabled is true (appended to values.yaml) and are merged
// with the workload's own podAnnotations. Templates whose pod annotations
// block has been customised are left unchanged.
//
// It returns the new chart and the template paths that were changed.
func InjectConfigChecksums(chart *types.GeneratedChart, graph *types.ResourceGraph) (*types.GeneratedChart, []string, error) {
	if chart == nil {
		return nil, nil, nil
	}
	out := cloneChart(chart)
	var changed []string
	for _, r := range opsChartResources(chart, graph) {
		kind := r.Original.GVK.Kind
		if kind != "Deployment" && kind != "StatefulSet" && kind != "DaemonSet" {
			continue
		}
		content := out.Templates[r.TemplatePath]
		if opsTemplateKind(content) != kind || strings.Contains(content, "checksum/") {
			continue
		}
		annotations := GenerateChecksumAnnotations(r, graph, chart)
		if len(annotations) == 0 {
			continue
		}
		loc := podAnnotationsBlockRe.FindStringSubmatchIndex(content)
		if loc == nil {
			continue
		}
		indent := content[loc[2]:loc[3]]
		content = content[:loc[0]] + checksumAnnotationsBlock(indent, annotations) + content[loc[1]:]
		out.Templates[r.TemplatePath] = content
		changed = append(changed, r.TemplatePath)
	}
	if len(changed) == 0 {
		return chart, nil, nil
	}
	sort.Strings(changed)
	values, err := appendTopLevelValues(out.ValuesYAML, configChecksumsValuesKey, map[string]interface{}{"enabled": true})
	if err != nil {
		return nil, nil, err
	}
	out.ValuesYAML = values
	return out, changed, nil
}

// checksumAnnotationsBlock renders the pod annotations block: checksum
// annotations (when enabled) followed by the workload's podAnnotations.
func checksumAnnotationsBlock(indent string, annotations []ChecksumAnnotation) string {
	var b strings.Builder
	w := func(s string) { b.WriteString(indent + s + "\n") }
	w(`{{- $dhgConfigChecksums := $.Values.configChecksums | default dict }}`)
	w(`{{- if or .podAnnotations $dhgConfigChecksums.enabled }}`)
	w(`annotations:`)
	w(`  {{- if $dhgConfigChecksums.enabled }}`)
	for _, a := range annotations {
		w(fmt.Sprintf(`  %s: %s`, a.Key, a.Expression))
	}
	w(`  {{- end }}`)
	w(`  {{- with .podAnnotations }}`)
	w(fmt.Sprintf(`  {{- toYaml . | nindent %d }}`, len(indent)+2))
	w(`  {{- end }}`)
	w(`{{- end }}`)
	return b.String()
}
