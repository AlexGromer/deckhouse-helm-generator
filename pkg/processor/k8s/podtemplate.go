package k8s

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// Pod templates of workloads (Deployment, StatefulSet, DaemonSet, Job,
// CronJob) are extracted into the same values shape and rendered by the same
// template text, so every workload kind keeps the same pod spec fields.

// extractPodTemplateValues stores the fields of the pod template at
// templatePath (e.g. spec.template) in values and returns the objects the pod
// refers to. Pod labels are stored as podLabels: they are the pod's identity,
// which selectors from the input (Services, PodDisruptionBudgets,
// NetworkPolicies, ...) rely on.
func extractPodTemplateValues(obj *unstructured.Unstructured, values map[string]interface{}, templatePath ...string) []types.ResourceKey {
	var deps []types.ResourceKey
	ns := obj.GetNamespace()
	at := func(fields ...string) []string {
		return append(append([]string(nil), templatePath...), fields...)
	}

	if labels, found, _ := unstructured.NestedStringMap(obj.Object, at("metadata", "labels")...); found && len(labels) > 0 {
		values["podLabels"] = labels
	}
	if annotations, found, _ := unstructured.NestedStringMap(obj.Object, at("metadata", "annotations")...); found && len(annotations) > 0 {
		values["podAnnotations"] = annotations
	}

	podSpec, _, _ := unstructured.NestedMap(obj.Object, at("spec")...)
	if podSpec == nil {
		return deps
	}

	if podSC, ok := podSpec["securityContext"].(map[string]interface{}); ok {
		values["podSecurityContext"] = podSC
	}
	for _, field := range []string{"initContainers", "containers"} {
		list, _ := podSpec[field].([]interface{})
		if len(list) == 0 {
			continue
		}
		containers := make([]map[string]interface{}, 0, len(list))
		for _, c := range list {
			container, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			cv, cdeps := extractContainerValues(container, ns)
			containers = append(containers, cv)
			deps = append(deps, cdeps...)
		}
		values[field] = containers
	}

	if volumes, ok := podSpec["volumes"].([]interface{}); ok && len(volumes) > 0 {
		values["volumes"] = volumes
		deps = append(deps, extractVolumeDependencies(volumes, ns)...)
	}

	if sa, ok := podSpec["serviceAccountName"].(string); ok && sa != "" {
		values["serviceAccountName"] = sa
		deps = append(deps, types.ResourceKey{
			GVK:       schema.GroupVersionKind{Version: "v1", Kind: "ServiceAccount"},
			Namespace: ns,
			Name:      sa,
		})
	}

	if secrets, ok := podSpec["imagePullSecrets"].([]interface{}); ok && len(secrets) > 0 {
		values["imagePullSecrets"] = secrets
		for _, s := range secrets {
			if secret, ok := s.(map[string]interface{}); ok {
				if name, ok := secret["name"].(string); ok && name != "" {
					deps = append(deps, types.ResourceKey{
						GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Secret"},
						Namespace: ns,
						Name:      name,
					})
				}
			}
		}
	}

	if nodeSelector, found, _ := unstructured.NestedStringMap(podSpec, "nodeSelector"); found {
		values["nodeSelector"] = nodeSelector
	}
	if tolerations, ok := podSpec["tolerations"].([]interface{}); ok && len(tolerations) > 0 {
		values["tolerations"] = tolerations
	}
	if affinity, ok := podSpec["affinity"].(map[string]interface{}); ok {
		values["affinity"] = affinity
	}
	if tsc, ok := podSpec["topologySpreadConstraints"].([]interface{}); ok && len(tsc) > 0 {
		values["topologySpreadConstraints"] = tsc
	}
	return deps
}

// extractContainerValues returns the values of one container.
func extractContainerValues(container map[string]interface{}, namespace string) (map[string]interface{}, []types.ResourceKey) {
	var deps []types.ResourceKey
	cv := make(map[string]interface{})

	if name, ok := container["name"].(string); ok {
		cv["name"] = name
	}
	if image, ok := container["image"].(string); ok {
		repo, tag, digest := parseImage(image)
		img := map[string]interface{}{"repository": repo}
		if tag != "" {
			img["tag"] = tag
		}
		if digest != "" {
			img["digest"] = digest
		}
		if policy, ok := container["imagePullPolicy"].(string); ok && policy != "" {
			img["pullPolicy"] = policy
		}
		cv["image"] = img
	}
	// Fields copied as they are.
	for _, field := range []string{
		"command", "args", "workingDir", "ports", "resources",
		"volumeMounts", "livenessProbe", "readinessProbe", "startupProbe",
		"lifecycle", "securityContext",
	} {
		if v, ok := container[field]; ok && v != nil {
			cv[field] = v
		}
	}
	if env, ok := container["env"].([]interface{}); ok {
		cv["env"] = env
		deps = append(deps, extractEnvDependencies(env, namespace)...)
	}
	if envFrom, ok := container["envFrom"].([]interface{}); ok {
		cv["envFrom"] = envFrom
		deps = append(deps, extractEnvFromDependencies(envFrom, namespace)...)
	}
	return cv, deps
}

// extractWorkloadSelector stores the workload's spec.selector from the input
// in values. The pod labels must carry every matchLabels entry (the API
// server rejects the workload otherwise); a missing one is added to
// podLabels. An input without a selector (not valid for apps/v1, but
// accepted here) is given one made of its pod labels, or app=<name>.
func extractWorkloadSelector(obj *unstructured.Unstructured, values map[string]interface{}) {
	podLabels, _ := values["podLabels"].(map[string]string)
	if podLabels == nil {
		podLabels = map[string]string{}
	}

	selector, found, _ := unstructured.NestedMap(obj.Object, "spec", "selector")
	if !found || len(selector) == 0 {
		if len(podLabels) == 0 {
			podLabels["app"] = obj.GetName()
		}
		matchLabels := make(map[string]interface{}, len(podLabels))
		for k, v := range podLabels {
			matchLabels[k] = v
		}
		selector = map[string]interface{}{"matchLabels": matchLabels}
	} else if matchLabels, ok := selector["matchLabels"].(map[string]interface{}); ok {
		for k, v := range matchLabels {
			if _, set := podLabels[k]; !set {
				podLabels[k] = fmt.Sprint(v)
			}
		}
	}

	values["selector"] = selector
	if len(podLabels) > 0 {
		values["podLabels"] = podLabels
	}
}

// workloadSelectorTemplate renders spec.selector of a Deployment, StatefulSet
// or DaemonSet from values (the selector of the input workload).
const workloadSelectorTemplate = `  selector:
    {{- toYaml .selector | nindent 4 }}
`

// podTemplate renders a pod template (the `template:` key and everything
// below it) from the values written by extractPodTemplateValues. indent is
// the indentation of the `template:` key. restartPolicy, when not empty, is
// the default spec.restartPolicy (Jobs need one).
//
// Pod labels are the input's pod labels plus the chart labels the input does
// not set: the input's labels win, so every selector of the input (the
// workload's own, Services, PodDisruptionBudgets, NetworkPolicies) keeps
// selecting these pods.
func podTemplate(chartName string, indent int, restartPolicy string) string {
	pad := strings.Repeat(" ", indent)
	n := func(extra int) int { return indent + extra }

	var b strings.Builder
	w := func(format string, args ...interface{}) {
		b.WriteString(pad + fmt.Sprintf(format, args...) + "\n")
	}
	w(`template:`)
	w(`  metadata:`)
	w(`    {{- with .podAnnotations }}`)
	w(`    annotations:`)
	w(`      {{- toYaml . | nindent %d }}`, n(6))
	w(`    {{- end }}`)
	w(`    labels:`)
	w(`      {{- /* Pod labels from the input win over the chart labels: the input's selectors must keep matching. */}}`)
	w(`      {{- toYaml (merge (dict) (.podLabels | default dict) (include "%s.labels" $ | fromYaml)) | nindent %d }}`, chartName, n(6))
	w(`  spec:`)
	if restartPolicy != "" {
		w(`    restartPolicy: {{ .restartPolicy | default %q }}`, restartPolicy)
	}
	w(`    {{- with concat ($.Values.global.imagePullSecrets | default list) (.imagePullSecrets | default list) }}`)
	w(`    imagePullSecrets:`)
	w(`      {{- toYaml . | nindent %d }}`, n(6))
	w(`    {{- end }}`)
	w(`    {{- with .serviceAccountName }}`)
	w(`    serviceAccountName: {{ . }}`)
	w(`    {{- end }}`)
	w(`    {{- with .podSecurityContext }}`)
	w(`    securityContext:`)
	w(`      {{- toYaml . | nindent %d }}`, n(6))
	w(`    {{- end }}`)
	w(`    {{- with .initContainers }}`)
	w(`    initContainers:`)
	w(`      {{- range . }}`)
	b.WriteString(containerTemplate(indent + 6))
	w(`      {{- end }}`)
	w(`    {{- end }}`)
	w(`    containers:`)
	w(`      {{- range .containers }}`)
	b.WriteString(containerTemplate(indent + 6))
	w(`      {{- end }}`)
	for _, field := range []string{"volumes", "nodeSelector", "affinity", "tolerations", "topologySpreadConstraints"} {
		w(`    {{- with .%s }}`, field)
		w(`    %s:`, field)
		w(`      {{- toYaml . | nindent %d }}`, n(6))
		w(`    {{- end }}`)
	}
	return b.String()
}

// containerTemplate renders one entry of a containers list from the values
// written by extractContainerValues; indent is the indentation of the "- ".
func containerTemplate(indent int) string {
	pad := strings.Repeat(" ", indent)
	var b strings.Builder
	w := func(s string) { b.WriteString(pad + s + "\n") }
	w(`- name: {{ .name }}`)
	w(`  image: "{{ .image.repository }}{{ with .image.tag }}:{{ . }}{{ end }}{{ with .image.digest }}@{{ . }}{{ end }}"`)
	w(`  {{- with .image.pullPolicy }}`)
	w(`  imagePullPolicy: {{ . }}`)
	w(`  {{- end }}`)
	for _, field := range []string{
		"command", "args", "workingDir", "ports", "env", "envFrom", "volumeMounts",
		"resources", "livenessProbe", "readinessProbe", "startupProbe", "lifecycle",
		"securityContext",
	} {
		if field == "workingDir" {
			w(`  {{- with .workingDir }}`)
			w(`  workingDir: {{ . | quote }}`)
			w(`  {{- end }}`)
			continue
		}
		w(`  {{- with .` + field + ` }}`)
		w(`  ` + field + `:`)
		w(fmt.Sprintf(`    {{- toYaml . | nindent %d }}`, indent+4))
		w(`  {{- end }}`)
	}
	return b.String()
}
