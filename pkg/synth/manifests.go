package synth

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

	var objects []*unstructured.Unstructured
	container := map[string]interface{}{
		"name":  name,
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
		var containerPorts, servicePorts []interface{}
		for _, p := range app.Ports {
			containerPorts = append(containerPorts, map[string]interface{}{
				"name": p.Name, "containerPort": p.Port, "protocol": p.Protocol,
			})
			servicePorts = append(servicePorts, map[string]interface{}{
				"name": p.Name, "port": p.Port, "targetPort": p.Port, "protocol": p.Protocol,
			})
		}
		container["ports"] = containerPorts
		objects = append(objects, object("v1", "Service", name, labels, map[string]interface{}{
			"spec": map[string]interface{}{
				"selector": copyMap(labels),
				"ports":    servicePorts,
			},
		}))
	}

	var envFrom []interface{}
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
	if len(mounts) > 0 {
		container["volumeMounts"] = mounts
	}

	if app.Liveness != nil {
		container["livenessProbe"] = probe(*app.Liveness)
	}
	if app.Readiness != nil {
		container["readinessProbe"] = probe(*app.Readiness)
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
	for k, v := range app.Labels {
		podLabels[k] = v
	}
	podSpec := map[string]interface{}{"containers": []interface{}{container}}
	if len(volumes) > 0 {
		podSpec["volumes"] = volumes
	}
	if sc := securityContext(app); len(sc) > 0 {
		podSpec["securityContext"] = sc
	}
	replicas := int64(1)
	if app.Replicas != nil {
		replicas = *app.Replicas
	}
	deployment := object("apps/v1", "Deployment", name, labels, map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": replicas,
			"selector": map[string]interface{}{"matchLabels": copyMap(labels)},
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{"labels": podLabels},
				"spec":     podSpec,
			},
		},
	})
	return append([]*unstructured.Unstructured{deployment}, objects...)
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
	if len(p.Exec) > 0 {
		out["exec"] = map[string]interface{}{"command": stringSlice(p.Exec)}
	} else {
		out["httpGet"] = map[string]interface{}{"path": p.HTTPPath, "port": p.Port}
	}
	for key, v := range map[string]int64{
		"periodSeconds": p.PeriodSeconds, "timeoutSeconds": p.TimeoutSeconds,
		"failureThreshold": p.FailureThreshold, "initialDelaySeconds": p.InitialDelaySeconds,
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

func copyMap(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
