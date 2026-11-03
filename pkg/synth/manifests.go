package synth

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const nameLabel = "app.kubernetes.io/name"

var configMapKey = regexp.MustCompile(`[^-._a-zA-Z0-9]+`)

// Manifests builds the objects that run app: a Deployment, a Service when it
// has ports, ConfigMap/Secret <name>-env for its variables, ConfigMap
// <name>-files for mounted files and a PersistentVolumeClaim per persistent
// volume. Numbers are int64 so the objects can be deep-copied.
func Manifests(app App) []*unstructured.Unstructured {
	name := DNSName(app.Name)
	labels := map[string]interface{}{nameLabel: name}
	deploymentName := firstNonEmpty(app.DeploymentName, name)
	containerName := firstNonEmpty(app.ContainerName, name)
	serviceName := firstNonEmpty(app.ServiceName, name)
	selector := copyMap(labels)
	if len(app.Selector) > 0 {
		selector = stringMap(app.Selector)
	}

	var objects []*unstructured.Unstructured
	container := map[string]interface{}{
		"name":  containerName,
		"image": app.Image,
	}
	if len(app.Command) > 0 {
		container["command"] = stringSlice(app.Command)
	}
	if len(app.Args) > 0 {
		container["args"] = stringSlice(app.Args)
	}
	if app.WorkingDir != "" {
		container["workingDir"] = app.WorkingDir
	}

	if len(app.Ports) > 0 {
		var containerPorts []interface{}
		for _, p := range app.Ports {
			containerPorts = append(containerPorts, map[string]interface{}{
				"name": p.Name, "containerPort": p.Port, "protocol": p.Protocol,
			})
		}
		container["ports"] = containerPorts
	}
	if len(app.Ports) > 0 || len(app.ExtraServicePorts) > 0 {
		var servicePorts []interface{}
		for _, p := range app.Ports {
			servicePorts = append(servicePorts, map[string]interface{}{
				"name": p.Name, "port": p.Port, "targetPort": p.Port, "protocol": p.Protocol,
			})
		}
		for _, p := range app.ExtraServicePorts {
			servicePorts = append(servicePorts, extraServicePort(p))
		}
		spec := map[string]interface{}{
			"selector": copyMap(selector),
			"ports":    servicePorts,
		}
		if app.ServiceType != "" {
			spec["type"] = app.ServiceType
		}
		objects = append(objects, object("v1", "Service", serviceName, labels, map[string]interface{}{"spec": spec}))
	}

	var envFrom []interface{}
	for _, ref := range app.EnvFromExtra {
		key := "configMapRef"
		if ref.Secret {
			key = "secretRef"
		}
		envFrom = append(envFrom, map[string]interface{}{key: map[string]interface{}{"name": ref.Name}})
	}
	if len(app.Env) > 0 {
		envFrom = append(envFrom, map[string]interface{}{"configMapRef": map[string]interface{}{"name": name + "-env"}})
		objects = append(objects, object("v1", "ConfigMap", name+"-env", labels, map[string]interface{}{
			"data": envData(app.Env),
		}))
	}
	if len(app.SecretEnv) > 0 {
		envFrom = append(envFrom, map[string]interface{}{"secretRef": map[string]interface{}{"name": name + "-env"}})
		objects = append(objects, object("v1", "Secret", name+"-env", labels, map[string]interface{}{
			"type":       "Opaque",
			"stringData": envData(app.SecretEnv),
		}))
	}
	if len(envFrom) > 0 {
		container["envFrom"] = envFrom
	}

	var volumes, mounts []interface{}
	files := map[string]interface{}{}
	for _, v := range app.Volumes {
		volName := DNSName(v.Name)
		mount := map[string]interface{}{"name": volName, "mountPath": v.MountPath}
		if v.ReadOnly {
			mount["readOnly"] = true
		}
		switch v.Kind {
		case VolumePVC:
			claim := name + "-" + volName
			size := v.Size
			if size == "" {
				size = "1Gi"
			}
			objects = append(objects, object("v1", "PersistentVolumeClaim", claim, labels, map[string]interface{}{
				"spec": map[string]interface{}{
					"accessModes": []interface{}{"ReadWriteOnce"},
					"resources":   map[string]interface{}{"requests": map[string]interface{}{"storage": size}},
				},
			}))
			volumes = append(volumes, map[string]interface{}{
				"name": volName, "persistentVolumeClaim": map[string]interface{}{"claimName": claim},
			})
		case VolumeConfigFile:
			key := uniqueKey(files, configMapKey.ReplaceAllString(v.FileName, "-"))
			files[key] = v.Content
			mount["name"] = "files"
			mount["subPath"] = key
			mount["readOnly"] = true
			mounts = append(mounts, mount)
			continue
		default:
			volumes = append(volumes, map[string]interface{}{"name": volName, "emptyDir": map[string]interface{}{}})
		}
		mounts = append(mounts, mount)
	}
	if len(files) > 0 {
		objects = append(objects, object("v1", "ConfigMap", name+"-files", labels, map[string]interface{}{"data": files}))
		volumes = append(volumes, map[string]interface{}{
			"name": "files", "configMap": map[string]interface{}{"name": name + "-files"},
		})
	}
	for _, v := range app.ExtraVolumes {
		volumes = append(volumes, deepCopyMap(v))
	}
	for _, m := range app.ExtraMounts {
		mounts = append(mounts, deepCopyMap(m))
	}
	if len(mounts) > 0 {
		container["volumeMounts"] = mounts
	}

	if app.Liveness != nil {
		container["livenessProbe"] = probe(*app.Liveness)
	}
	if app.Readiness != nil {
		container["readinessProbe"] = probe(*app.Readiness)
	}
	if app.Startup != nil {
		container["startupProbe"] = probe(*app.Startup)
	}
	if app.Resources != nil {
		res := map[string]interface{}{}
		if len(app.Resources.Requests) > 0 {
			res["requests"] = stringMap(app.Resources.Requests)
		}
		if len(app.Resources.Limits) > 0 {
			res["limits"] = stringMap(app.Resources.Limits)
		}
		if len(res) > 0 {
			container["resources"] = res
		}
	}

	podLabels := copyMap(labels)
	for _, extra := range []map[string]string{app.Labels, app.PodLabels, app.Selector} {
		for k, v := range extra {
			podLabels[k] = v
		}
	}
	podMeta := map[string]interface{}{"labels": podLabels}
	if len(app.PodAnnotations) > 0 {
		podMeta["annotations"] = stringMap(app.PodAnnotations)
	}
	podSpec := map[string]interface{}{"containers": []interface{}{container}}
	if len(app.InitContainers) > 0 {
		inits := make([]interface{}, len(app.InitContainers))
		for i, c := range app.InitContainers {
			inits[i] = deepCopyMap(c)
		}
		podSpec["initContainers"] = inits
	}
	if len(volumes) > 0 {
		podSpec["volumes"] = volumes
	}
	if sc := securityContext(app); len(sc) > 0 {
		podSpec["securityContext"] = sc
	}
	if len(app.ContainerPatch) > 0 {
		mergePatch(container, deepCopyMap(app.ContainerPatch))
	}
	if len(app.PodPatch) > 0 {
		mergePatch(podSpec, deepCopyMap(app.PodPatch))
	}
	replicas := int64(1)
	if app.Replicas != nil {
		replicas = *app.Replicas
	}
	deploymentLabels := copyMap(labels)
	for k, v := range app.DeploymentLabels {
		deploymentLabels[k] = v
	}
	deploymentSpec := map[string]interface{}{
		"replicas": replicas,
		"selector": map[string]interface{}{"matchLabels": copyMap(selector)},
		"template": map[string]interface{}{
			"metadata": podMeta,
			"spec":     podSpec,
		},
	}
	if len(app.Strategy) > 0 {
		deploymentSpec["strategy"] = deepCopyMap(app.Strategy)
	}
	deployment := object("apps/v1", "Deployment", deploymentName, deploymentLabels, map[string]interface{}{
		"spec": deploymentSpec,
	})
	for _, j := range app.Jobs {
		objects = append(objects, &unstructured.Unstructured{Object: deepCopyMap(j.Object)})
	}
	return append([]*unstructured.Unstructured{deployment}, objects...)
}

// extraServicePort is a Service port the profile adds.
func extraServicePort(p ServicePort) map[string]interface{} {
	protocol := p.Protocol
	if protocol == "" {
		protocol = "TCP"
	}
	var target interface{} = p.Port
	switch {
	case p.TargetPortName != "":
		target = p.TargetPortName
	case p.TargetPort > 0:
		target = p.TargetPort
	}
	return map[string]interface{}{"name": p.Name, "port": p.Port, "targetPort": target, "protocol": protocol}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func object(apiVersion, kind, name string, labels map[string]interface{}, fields map[string]interface{}) *unstructured.Unstructured {
	obj := map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": name, "labels": copyMap(labels)},
	}
	for k, v := range fields {
		obj[k] = v
	}
	return &unstructured.Unstructured{Object: obj}
}

func probe(p Probe) map[string]interface{} {
	out := map[string]interface{}{}
	switch {
	case len(p.Exec) > 0:
		out["exec"] = map[string]interface{}{"command": stringSlice(p.Exec)}
	case p.TCP:
		out["tcpSocket"] = map[string]interface{}{"port": p.Port}
	default:
		out["httpGet"] = map[string]interface{}{"path": p.HTTPPath, "port": p.Port}
	}
	for key, v := range map[string]int64{
		"periodSeconds": p.PeriodSeconds, "timeoutSeconds": p.TimeoutSeconds,
		"failureThreshold": p.FailureThreshold, "initialDelaySeconds": p.InitialDelaySeconds,
		"successThreshold": p.SuccessThreshold,
	} {
		if v > 0 {
			out[key] = v
		}
	}
	return out
}

func securityContext(app App) map[string]interface{} {
	sc := map[string]interface{}{}
	if app.RunAsUser != nil {
		sc["runAsUser"] = *app.RunAsUser
		if *app.RunAsUser != 0 {
			sc["runAsNonRoot"] = true
		}
	}
	if app.RunAsGroup != nil {
		sc["runAsGroup"] = *app.RunAsGroup
	}
	return sc
}

func envData(env []EnvVar) map[string]interface{} {
	out := make(map[string]interface{}, len(env))
	for _, e := range env {
		out[e.Name] = e.Value
	}
	return out
}

func uniqueKey(taken map[string]interface{}, key string) string {
	key = strings.Trim(key, "-.")
	if key == "" {
		key = "file"
	}
	candidate := key
	for i := 2; ; i++ {
		if _, exists := taken[candidate]; !exists {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", key, i)
	}
}

func stringSlice(s []string) []interface{} {
	out := make([]interface{}, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

func stringMap(m map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for _, k := range sortedKeys(m) {
		out[k] = m[k]
	}
	return out
}

// deepCopyMap copies a JSON-typed map: the objects are deep-copied later and
// must not share maps with the app.
func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	return runtime.DeepCopyJSON(m)
}

func copyMap(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
