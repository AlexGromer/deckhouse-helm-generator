package golden

import (
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"
)

// object is a parsed manifest.
type object map[string]interface{}

func parseStream(stream string) []object {
	var out []object
	for _, doc := range strings.Split(stream, "\n---") {
		// Splitting consumed the line break that ends the document; a
		// trailing "|" block scalar needs it to keep its final newline.
		var obj map[string]interface{}
		if err := yaml.Unmarshal([]byte(doc+"\n"), &obj); err == nil && obj != nil && obj["kind"] != nil {
			out = append(out, obj)
		}
	}
	return out
}

func (o object) kind() string { s, _ := o["kind"].(string); return s }
func (o object) name() string {
	m, _ := o["metadata"].(map[string]interface{})
	s, _ := m["name"].(string)
	return s
}

func get(v interface{}, path ...string) interface{} {
	for _, p := range path {
		m, ok := v.(map[string]interface{})
		if !ok {
			return nil
		}
		v = m[p]
	}
	return v
}

func list(v interface{}) []interface{} { l, _ := v.([]interface{}); return l }

func str(v interface{}) string { s, _ := v.(string); return s }

// podSpecOf returns the pod spec of a workload, if any.
func podSpecOf(o object) interface{} {
	switch o.kind() {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job":
		return get(map[string]interface{}(o), "spec", "template", "spec")
	case "CronJob":
		return get(map[string]interface{}(o), "spec", "jobTemplate", "spec", "template", "spec")
	case "Pod":
		return get(map[string]interface{}(o), "spec")
	}
	return nil
}

func podLabelsOf(o object) map[string]interface{} {
	path := []string{"spec", "template", "metadata", "labels"}
	if o.kind() == "CronJob" {
		path = []string{"spec", "jobTemplate", "spec", "template", "metadata", "labels"}
	}
	m, _ := get(map[string]interface{}(o), path...).(map[string]interface{})
	return m
}

// references lists "Kind/name" references an object makes to other objects.
func references(o object) []string {
	var refs []string
	add := func(kind string, name interface{}) {
		if n := str(name); n != "" {
			refs = append(refs, kind+"/"+n)
		}
	}
	if spec := podSpecOf(o); spec != nil {
		add("ServiceAccount", get(spec, "serviceAccountName"))
		for _, v := range list(get(spec, "volumes")) {
			add("ConfigMap", get(v, "configMap", "name"))
			add("Secret", get(v, "secret", "secretName"))
			add("PersistentVolumeClaim", get(v, "persistentVolumeClaim", "claimName"))
			for _, src := range list(get(v, "projected", "sources")) {
				add("ConfigMap", get(src, "configMap", "name"))
				add("Secret", get(src, "secret", "name"))
			}
		}
		containers := append(list(get(spec, "initContainers")), list(get(spec, "containers"))...)
		for _, c := range containers {
			for _, e := range list(get(c, "env")) {
				add("ConfigMap", get(e, "valueFrom", "configMapKeyRef", "name"))
				add("Secret", get(e, "valueFrom", "secretKeyRef", "name"))
			}
			for _, e := range list(get(c, "envFrom")) {
				add("ConfigMap", get(e, "configMapRef", "name"))
				add("Secret", get(e, "secretRef", "name"))
			}
		}
		for _, s := range list(get(spec, "imagePullSecrets")) {
			add("Secret", get(s, "name"))
		}
	}
	m := map[string]interface{}(o)
	switch o.kind() {
	case "StatefulSet":
		add("Service", get(m, "spec", "serviceName"))
	case "Ingress":
		add("Service", get(m, "spec", "defaultBackend", "service", "name"))
		for _, rule := range list(get(m, "spec", "rules")) {
			for _, p := range list(get(rule, "http", "paths")) {
				add("Service", get(p, "backend", "service", "name"))
			}
		}
	case "HorizontalPodAutoscaler":
		add(str(get(m, "spec", "scaleTargetRef", "kind")), get(m, "spec", "scaleTargetRef", "name"))
	case "RoleBinding", "ClusterRoleBinding":
		add(str(get(m, "roleRef", "kind")), get(m, "roleRef", "name"))
		for _, s := range list(get(m, "subjects")) {
			if str(get(s, "kind")) == "ServiceAccount" {
				add("ServiceAccount", get(s, "name"))
			}
		}
	}
	return refs
}

// selectorOf returns the label selector a Service, PDB or NetworkPolicy uses
// to select pods.
func selectorOf(o object) map[string]interface{} {
	m := map[string]interface{}(o)
	var sel interface{}
	switch o.kind() {
	case "Service":
		sel = get(m, "spec", "selector")
	case "PodDisruptionBudget":
		sel = get(m, "spec", "selector", "matchLabels")
	case "NetworkPolicy":
		sel = get(m, "spec", "podSelector", "matchLabels")
	}
	s, _ := sel.(map[string]interface{})
	return s
}

func selects(selector, labels map[string]interface{}) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if fmt.Sprint(labels[k]) != fmt.Sprint(v) {
			return false
		}
	}
	return true
}

func countSelecting(objs []object) map[string]int {
	var pods []map[string]interface{}
	for _, o := range objs {
		if l := podLabelsOf(o); l != nil {
			pods = append(pods, l)
		}
	}
	counts := map[string]int{}
	for _, o := range objs {
		sel := selectorOf(o)
		for _, p := range pods {
			if selects(sel, p) {
				counts[o.kind()]++
				break
			}
		}
	}
	return counts
}

// checkIntegrity compares the rendered release with the input manifests:
// a reference the input could resolve must still resolve after rendering,
// and Services/PDBs/NetworkPolicies that selected pods must still do so.
func checkIntegrity(input, rendered []object) []string {
	defined := func(objs []object) map[string]bool {
		set := map[string]bool{}
		for _, o := range objs {
			set[o.kind()+"/"+o.name()] = true
		}
		return set
	}
	inDefined, outDefined := defined(input), defined(rendered)

	var problems []string
	for _, o := range rendered {
		for _, ref := range references(o) {
			if !outDefined[ref] && inDefined[ref] {
				problems = append(problems, fmt.Sprintf("%s/%s references %s, which the chart renders under another name", o.kind(), o.name(), ref))
			}
		}
	}
	inCounts, outCounts := countSelecting(input), countSelecting(rendered)
	for kind, n := range inCounts {
		if outCounts[kind] < n {
			problems = append(problems, fmt.Sprintf("%d %s selector(s) matched pods in the input, only %d after rendering", n, kind, outCounts[kind]))
		}
	}
	return problems
}
