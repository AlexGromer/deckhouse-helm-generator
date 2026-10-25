package generator

import (
	"sort"
)

// InjectedEnvVar represents an injected environment variable.
type InjectedEnvVar struct {
	Name          string
	Value         string
	SecretName    string
	SecretKey     string
	ConfigMapName string
	ConfigMapKey  string
}

// InjectedVolumeMount represents an injected volume mount.
type InjectedVolumeMount struct {
	Name      string
	MountPath string
	SubPath   string
	ReadOnly  bool
}

// InjectedVolume represents an injected volume.
type InjectedVolume struct {
	Name          string
	SecretName    string
	ConfigMapName string
	Optional      *bool
}

// WorkloadInjectSpec contains the specification for injection into a workload.
type WorkloadInjectSpec struct {
	ContainerName    string
	Env              []InjectedEnvVar
	EnvFromSecret    []string
	EnvFromConfigMap []string
	VolumeMounts     []InjectedVolumeMount
	Volumes          []InjectedVolume
}

// AddWorkloadInject adds injected configuration to values[featureKey]["inject"][workloadKey].
// It merges without duplicates by Name, preserving stable order.
func AddWorkloadInject(values map[string]interface{}, featureKey, workloadKey string, spec WorkloadInjectSpec) {
	if values == nil {
		return
	}

	featVal, ok := values[featureKey]
	if !ok {
		featVal = make(map[string]interface{})
		values[featureKey] = featVal
	}
	featMap, ok := featVal.(map[string]interface{})
	if !ok {
		return
	}

	injVal, ok := featMap["inject"]
	if !ok {
		injVal = make(map[string]interface{})
		featMap["inject"] = injVal
	}
	injMap, ok := injVal.(map[string]interface{})
	if !ok {
		return
	}

	wlVal, ok := injMap[workloadKey]
	if !ok {
		wlVal = make(map[string]interface{})
		injMap[workloadKey] = wlVal
	}
	wlMap, ok := wlVal.(map[string]interface{})
	if !ok {
		return
	}

	if spec.ContainerName != "" {
		containersVal, ok := wlMap["containers"]
		if !ok {
			containersVal = make(map[string]interface{})
			wlMap["containers"] = containersVal
		}
		containersMap, ok := containersVal.(map[string]interface{})
		if ok {
			cVal, ok := containersMap[spec.ContainerName]
			if !ok {
				cVal = make(map[string]interface{})
				containersMap[spec.ContainerName] = cVal
			}
			cMap, ok := cVal.(map[string]interface{})
			if ok {
				// Merge Env
				if len(spec.Env) > 0 {
					var envList []interface{}
					if existing, ok := cMap["env"].([]interface{}); ok {
						envList = existing
					}
					for _, e := range spec.Env {
						if !containsName(envList, e.Name) {
							m := map[string]interface{}{"name": e.Name}
							if e.Value != "" {
								m["value"] = e.Value
							} else if e.SecretName != "" && e.SecretKey != "" {
								m["valueFrom"] = map[string]interface{}{
									"secretKeyRef": map[string]interface{}{
										"name": e.SecretName,
										"key":  e.SecretKey,
									},
								}
							} else if e.ConfigMapName != "" && e.ConfigMapKey != "" {
								m["valueFrom"] = map[string]interface{}{
									"configMapKeyRef": map[string]interface{}{
										"name": e.ConfigMapName,
										"key":  e.ConfigMapKey,
									},
								}
							}
							envList = append(envList, m)
						}
					}
					if len(envList) > 0 {
						cMap["env"] = envList
					}
				}

				// Merge EnvFromSecret and EnvFromConfigMap
				var envFromList []interface{}
				if existing, ok := cMap["envFrom"].([]interface{}); ok {
					envFromList = existing
				}
				for _, s := range spec.EnvFromSecret {
					if !containsRef(envFromList, "secretRef", s) {
						envFromList = append(envFromList, map[string]interface{}{
							"secretRef": map[string]interface{}{"name": s},
						})
					}
				}
				for _, c := range spec.EnvFromConfigMap {
					if !containsRef(envFromList, "configMapRef", c) {
						envFromList = append(envFromList, map[string]interface{}{
							"configMapRef": map[string]interface{}{"name": c},
						})
					}
				}
				if len(envFromList) > 0 {
					cMap["envFrom"] = envFromList
				}

				// Merge VolumeMounts
				if len(spec.VolumeMounts) > 0 {
					var vmList []interface{}
					if existing, ok := cMap["volumeMounts"].([]interface{}); ok {
						vmList = existing
					}
					for _, vm := range spec.VolumeMounts {
						if !containsName(vmList, vm.Name) {
							m := map[string]interface{}{
								"name":      vm.Name,
								"mountPath": vm.MountPath,
							}
							if vm.SubPath != "" {
								m["subPath"] = vm.SubPath
							}
							if vm.ReadOnly {
								m["readOnly"] = true
							}
							vmList = append(vmList, m)
						}
					}
					if len(vmList) > 0 {
						cMap["volumeMounts"] = vmList
					}
				}
			}
		}
	}

	// Merge Volumes
	if len(spec.Volumes) > 0 {
		var volList []interface{}
		if existing, ok := wlMap["volumes"].([]interface{}); ok {
			volList = existing
		}
		for _, v := range spec.Volumes {
			if !containsName(volList, v.Name) {
				m := map[string]interface{}{"name": v.Name}
				if v.SecretName != "" {
					sec := map[string]interface{}{"secretName": v.SecretName}
					if v.Optional != nil {
						sec["optional"] = *v.Optional
					}
					m["secret"] = sec
				} else if v.ConfigMapName != "" {
					cm := map[string]interface{}{"name": v.ConfigMapName}
					if v.Optional != nil {
						cm["optional"] = *v.Optional
					}
					m["configMap"] = cm
				} else {
					m["emptyDir"] = map[string]interface{}{}
				}
				volList = append(volList, m)
			}
		}
		if len(volList) > 0 {
			wlMap["volumes"] = volList
		}
	}
}

// InjectedFeatures returns a sorted list of top-level keys in values that contain a non-empty "inject" sub-map.
func InjectedFeatures(values map[string]interface{}) []string {
	var features []string
	for k, v := range values {
		m, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		injVal, ok := m["inject"]
		if !ok {
			continue
		}
		injMap, ok := injVal.(map[string]interface{})
		if !ok || len(injMap) == 0 {
			continue
		}
		features = append(features, k)
	}
	sort.Strings(features)
	return features
}

func containsName(list []interface{}, name string) bool {
	for _, item := range list {
		if m, ok := item.(map[string]interface{}); ok {
			if n, ok := m["name"].(string); ok && n == name {
				return true
			}
		}
	}
	return false
}

func containsRef(list []interface{}, refType, name string) bool {
	for _, item := range list {
		if m, ok := item.(map[string]interface{}); ok {
			if ref, ok := m[refType].(map[string]interface{}); ok {
				if n, ok := ref["name"].(string); ok && n == name {
					return true
				}
			}
		}
	}
	return false
}
