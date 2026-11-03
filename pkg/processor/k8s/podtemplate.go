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
		// Containers are a map by name, so `--set <path>.containers.<name>.image.tag=X`
		// reaches one container; the order list keeps the input order, which
		// the rendered pod spec must keep (init containers run in it).
		containers := make(map[string]interface{}, len(list))
		order := make([]interface{}, 0, len(list))
		for i, c := range list {
			container, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			cv, cdeps := extractContainerValues(container, ns)
			name, _ := container["name"].(string)
			if name == "" {
				name = fmt.Sprintf("container-%d", i)
			}
			if _, dup := containers[name]; !dup {
				order = append(order, name)
			}
			containers[name] = cv
			deps = append(deps, cdeps...)
		}
		values[field] = containers
		values[containerOrderKey(field)] = order
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
	extractPodSpecSettings(podSpec, values)
	return deps
}

// extractPodSpecSettings stores the pod-level settings the user set, under the
// pod spec field names. Fields at their API default are left out, so the chart
// stays as small as the input.
func extractPodSpecSettings(podSpec, values map[string]interface{}) {
	for _, field := range []string{"hostname", "subdomain", "priorityClassName", "runtimeClassName"} {
		if v, ok := podSpec[field].(string); ok && v != "" {
			values[field] = v
		}
	}
	// ClusterFirst is the default policy.
	if v, ok := podSpec["dnsPolicy"].(string); ok && v != "" && v != "ClusterFirst" {
		values["dnsPolicy"] = v
	}
	if v, ok := podSpec["dnsConfig"].(map[string]interface{}); ok && len(v) > 0 {
		values["dnsConfig"] = v
	}
	if v, ok := podSpec["hostAliases"].([]interface{}); ok && len(v) > 0 {
		values["hostAliases"] = v
	}
	// 0 is a real grace period (kill at once), so presence counts, not truth.
	// 30 is the server default on every pod template, so it is left out.
	if v, ok := podSpec["terminationGracePeriodSeconds"]; ok && v != nil {
		if n, isNum := toInt64(v); !isNum || n != 30 {
			values["terminationGracePeriodSeconds"] = v
		}
	}
	// All four are false by default. hostNetwork does not bring a dnsPolicy
	// with it: a pod that needs ClusterFirstWithHostNet has it in the input.
	for _, field := range []string{"shareProcessNamespace", "hostNetwork", "hostPID", "hostIPC"} {
		if v, ok := podSpec[field].(bool); ok && v {
			values[field] = v
		}
	}
	// true is the default.
	if v, ok := podSpec["enableServiceLinks"].(bool); ok && !v {
		values["enableServiceLinks"] = v
	}
}

// containerOrderKey returns the values key of the order list of the containers
// stored under field ("containers" or "initContainers").
func containerOrderKey(field string) string {
	if field == "initContainers" {
		return "initContainerOrder"
	}
	return "containerOrder"
}

// extractContainerValues returns the values of one container. The name is not
// part of them: the container is stored under it.
func extractContainerValues(container map[string]interface{}, namespace string) (map[string]interface{}, []types.ResourceKey) {
	var deps []types.ResourceKey
	cv := make(map[string]interface{})

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
		// restartPolicy: Always makes an init container a native sidecar
		// (Kubernetes 1.29+); without it the pod waits for it forever.
		"restartPolicy", "resizePolicy", "volumeDevices",
		"terminationMessagePath", "terminationMessagePolicy", "stdin", "stdinOnce", "tty",
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
// Pod labels are the input's pod labels plus the stable chart labels
// (app.kubernetes.io/name and instance) the input does not set: the input's
// labels win, so every selector of the input (the workload's own, Services,
// PodDisruptionBudgets, NetworkPolicies) keeps selecting these pods. The
// chart, version and managed-by labels are left out: they change with every
// chart release and would restart the pods.
func podTemplate(chartName string, indent int, restartPolicy string, workloadKey string) string {
	pad := strings.Repeat(" ", indent)
	n := func(extra int) int { return indent + extra }

	var b strings.Builder
	w := func(format string, args ...interface{}) {
		b.WriteString(pad + fmt.Sprintf(format, args...) + "\n")
	}
	w(`{{- $dhgInjVolumes := list }}`)
	w(`{{- $dhgInjContainers := dict }}`)
	w(`{{- range $dhgKey, $dhgBlock := $.Values }}`)
	w(`{{- if and (kindIs "map" $dhgBlock) $dhgBlock.enabled (kindIs "map" $dhgBlock.inject) }}`)
	w(`{{- with index $dhgBlock.inject %q }}`, workloadKey)
	w(`{{- $dhgW := tpl (toYaml .) $ | fromYaml }}`)
	w(`{{- $dhgInjVolumes = concat $dhgInjVolumes ($dhgW.volumes | default list) }}`)
	w(`{{- range $dhgC, $dhgS := ($dhgW.containers | default dict) }}`)
	w(`{{- $dhgP := get $dhgInjContainers $dhgC | default dict }}`)
	w(`{{- $_ := set $dhgInjContainers $dhgC (dict`)
	w(`      "env" (concat ($dhgP.env | default list) ($dhgS.env | default list))`)
	w(`      "envFrom" (concat ($dhgP.envFrom | default list) ($dhgS.envFrom | default list))`)
	w(`      "volumeMounts" (concat ($dhgP.volumeMounts | default list) ($dhgS.volumeMounts | default list))) }}`)
	w(`{{- end }}`)
	w(`{{- end }}`)
	w(`{{- end }}`)
	w(`{{- end }}`)
	w(`template:`)
	w(`  metadata:`)
	w(`    {{- with .podAnnotations }}`)
	w(`    annotations:`)
	w(`      {{- toYaml . | nindent %d }}`, n(6))
	w(`    {{- end }}`)
	w(`    labels:`)
	w(`      {{- /* Pod labels from the input win over the chart labels: the input's selectors must keep matching. */}}`)
	w(`      {{- /* The chart, version and managed-by labels stay on the object only: in a pod template they restart the pods on every version bump. */}}`)
	w(`      {{- toYaml (merge (dict) (.podLabels | default dict) (omit (include "%s.labels" $ | fromYaml) "helm.sh/chart" "app.kubernetes.io/version" "app.kubernetes.io/managed-by")) | nindent %d }}`, chartName, n(6))
	w(`  spec:`)
	if restartPolicy != "" {
		w(`    restartPolicy: {{ .restartPolicy | default %q }}`, restartPolicy)
	}
	w(`    {{- with concat ($.Values.global.imagePullSecrets | default list) (.imagePullSecrets | default list) }}`)
	w(`    imagePullSecrets:`)
	w(`      {{- toYaml . | nindent %d }}`, n(6))
	w(`    {{- end }}`)
	w(`    {{- with .serviceAccountName }}`)
	w(`    serviceAccountName: {{ include "%s.ref" (list $ "ServiceAccount" .) }}`, chartName)
	w(`    {{- end }}`)
	w(`    {{- with merge (deepCopy (.podSecurityContext | default dict)) (deepCopy ((($.Values.global).securityContextDefaults).pod | default dict)) }}`)
	w(`    securityContext:`)
	w(`      {{- toYaml . | nindent %d }}`, n(6))
	w(`    {{- end }}`)
	// Containers are a map by name; the order list gives the rendered order
	// (without it, the names sorted). A name left out of the map renders
	// nothing, so an overlay can drop a container with `null`.
	w(`    {{- $dhgInitCs := .initContainers | default dict }}`)
	w(`    {{- $dhgCs := .containers | default dict }}`)
	w(`    {{- with .initContainerOrder | default (keys $dhgInitCs | sortAlpha) }}`)
	w(`    initContainers:`)
	w(`      {{- range . }}`)
	w(`      {{- $dhgName := . }}`)
	w(`      {{- with index $dhgInitCs $dhgName }}`)
	b.WriteString(containerTemplate(chartName, indent+6))
	w(`      {{- end }}`)
	w(`      {{- end }}`)
	w(`    {{- end }}`)
	w(`    containers:`)
	w(`      {{- range .containerOrder | default (keys $dhgCs | sortAlpha) }}`)
	w(`      {{- $dhgName := . }}`)
	w(`      {{- with index $dhgCs $dhgName }}`)
	b.WriteString(containerTemplate(chartName, indent+6))
	w(`      {{- end }}`)
	w(`      {{- end }}`)
	w(`    {{- with concat (.volumes | default list) $dhgInjVolumes }}`)
	w(`    volumes:`)
	w(`      {{- include "%s.refs" (list $ .) | nindent %d }}`, chartName, n(6))
	w(`    {{- end }}`)
	for _, field := range []string{"nodeSelector", "affinity", "tolerations", "topologySpreadConstraints", "hostAliases", "dnsConfig"} {
		w(`    {{- with .%s }}`, field)
		w(`    %s:`, field)
		w(`      {{- toYaml . | nindent %d }}`, n(6))
		w(`    {{- end }}`)
	}
	for _, field := range []string{"hostname", "subdomain", "priorityClassName", "runtimeClassName", "dnsPolicy"} {
		w(`    {{- with .%s }}`, field)
		if field == "priorityClassName" {
			w(`    %s: {{ include "%s.ref" (list $ "PriorityClass" .) | quote }}`, field, chartName)
		} else {
			w(`    %s: {{ . | quote }}`, field)
		}
		w(`    {{- end }}`)
	}
	for _, field := range []string{"shareProcessNamespace", "hostNetwork", "hostPID", "hostIPC"} {
		w(`    {{- with .%s }}`, field)
		w(`    %s: {{ . }}`, field)
		w(`    {{- end }}`)
	}
	// false is stored, and with would skip it.
	w(`    {{- if hasKey . "enableServiceLinks" }}`)
	w(`    enableServiceLinks: {{ .enableServiceLinks }}`)
	w(`    {{- end }}`)
	// Spot injection writes terminationGracePeriodSeconds into Deployments
	// and StatefulSets when spot is enabled; a second key would be invalid.
	// 0 is stored, so hasKey, not with. int64: a float from values.yaml
	// would print as 1e+06 for large numbers.
	spot := strings.HasPrefix(workloadKey, "Deployment/") || strings.HasPrefix(workloadKey, "StatefulSet/")
	cond := `hasKey . "terminationGracePeriodSeconds"`
	if spot {
		cond = `and (hasKey . "terminationGracePeriodSeconds") (not (and $.Values.spot $.Values.spot.enabled))`
	}
	w(`    {{- if %s }}`, cond)
	w(`    terminationGracePeriodSeconds: {{ .terminationGracePeriodSeconds | int64 }}`)
	w(`    {{- end }}`)
	return b.String()
}

// containerTemplate renders one entry of a containers list from the values
// written by extractContainerValues; indent is the indentation of the "- ".
// The dot is the container's values, $dhgName its name (the map key).
func containerTemplate(chartName string, indent int) string {
	pad := strings.Repeat(" ", indent)
	var b strings.Builder
	w := func(s string) { b.WriteString(pad + s + "\n") }
	w(`{{- $dhgInj := get $dhgInjContainers $dhgName | default dict }}`)
	w(`- name: {{ $dhgName }}`)
	w(`  image: "{{ .image.repository }}{{ with .image.tag }}:{{ . }}{{ end }}{{ with .image.digest }}@{{ . }}{{ end }}"`)
	w(`  {{- with .image.pullPolicy }}`)
	w(`  imagePullPolicy: {{ . }}`)
	w(`  {{- end }}`)
	for _, field := range []string{
		"command", "args", "workingDir", "ports", "env", "envFrom", "volumeMounts",
		"resources", "livenessProbe", "readinessProbe", "startupProbe", "lifecycle",
		"securityContext", "volumeDevices", "resizePolicy",
	} {
		if field == "env" || field == "envFrom" || field == "volumeMounts" {
			w(`  {{- with concat (.` + field + ` | default list) ($dhgInj.` + field + ` | default list) }}`)
			w(`  ` + field + `:`)
			if field == "volumeMounts" {
				w(fmt.Sprintf(`    {{- toYaml . | nindent %d }}`, indent+4))
			} else {
				w(fmt.Sprintf(`    {{- include "%s.refs" (list $ .) | nindent %d }}`, chartName, indent+4))
			}
			w(`  {{- end }}`)
			continue
		}
		if field == "workingDir" {
			w(`  {{- with .workingDir }}`)
			w(`  workingDir: {{ . | quote }}`)
			w(`  {{- end }}`)
			continue
		}
		if field == "securityContext" {
			w(`  {{- with merge (deepCopy (.securityContext | default dict)) (deepCopy ((($.Values.global).securityContextDefaults).container | default dict)) }}`)
			w(`  securityContext:`)
			w(fmt.Sprintf(`    {{- toYaml . | nindent %d }}`, indent+4))
			w(`  {{- end }}`)
			continue
		}
		w(`  {{- with .` + field + ` }}`)
		w(`  ` + field + `:`)
		w(fmt.Sprintf(`    {{- toYaml . | nindent %d }}`, indent+4))
		w(`  {{- end }}`)
	}
	// Scalars: false and "" are the API defaults, so with skips nothing.
	for _, field := range []string{"restartPolicy", "terminationMessagePath", "terminationMessagePolicy"} {
		w(`  {{- with .` + field + ` }}`)
		w(`  ` + field + `: {{ . | quote }}`)
		w(`  {{- end }}`)
	}
	for _, field := range []string{"stdin", "stdinOnce", "tty"} {
		w(`  {{- with .` + field + ` }}`)
		w(`  ` + field + `: {{ . }}`)
		w(`  {{- end }}`)
	}
	return b.String()
}
