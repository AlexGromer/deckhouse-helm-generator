package detector

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// VolumeMountDetector detects relationships based on volume mounts.
// Example: Deployment mounting ConfigMap or Secret.
type VolumeMountDetector struct {
	priority int
}

// NewVolumeMountDetector creates a new volume mount detector.
func NewVolumeMountDetector() *VolumeMountDetector {
	return &VolumeMountDetector{
		priority: 80,
	}
}

// Name returns the detector name.
func (d *VolumeMountDetector) Name() string {
	return "volume_mount"
}

// Priority returns the detector priority.
func (d *VolumeMountDetector) Priority() int {
	return d.priority
}

// Detect detects volume mount relationships.
func (d *VolumeMountDetector) Detect(ctx context.Context, resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	kind := obj.GetKind()
	namespace := obj.GetNamespace()

	// Check if this is a workload resource
	workloadKinds := map[string]bool{
		"Deployment":  true,
		"StatefulSet": true,
		"DaemonSet":   true,
		"Job":         true,
		"CronJob":     true,
		"Pod":         true,
	}

	if !workloadKinds[kind] {
		return relationships
	}

	// Get volumes from the pod spec
	var volumes []interface{}
	var found bool

	if kind == "CronJob" {
		volumes, found, _ = unstructured.NestedSlice(obj.Object, "spec", "jobTemplate", "spec", "template", "spec", "volumes")
	} else if kind == "Pod" {
		volumes, found, _ = unstructured.NestedSlice(obj.Object, "spec", "volumes")
	} else {
		volumes, found, _ = unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "volumes")
	}

	if !found {
		return relationships
	}

	// Analyze each volume
	for _, vol := range volumes {
		volumeMap, ok := vol.(map[string]interface{})
		if !ok {
			continue
		}

		// ConfigMap volume
		if cmMap, ok := volumeMap["configMap"].(map[string]interface{}); ok {
			if cmName, ok := cmMap["name"].(string); ok && cmName != "" {
				targetKey := types.ResourceKey{
					GVK:       schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
					Namespace: namespace,
					Name:      cmName,
				}

				if _, exists := allResources[targetKey]; exists {
					relationships = append(relationships, types.Relationship{
						From: resource.Original.ResourceKey(),
						To:   targetKey,
						Type: types.RelationVolumeMount,
						Field: "spec.template.spec.volumes[].configMap",
						Details: map[string]string{
							"volumeName":    volumeMap["name"].(string),
							"configMapName": cmName,
						},
					})
				}
			}
		}

		// Secret volume
		if secretMap, ok := volumeMap["secret"].(map[string]interface{}); ok {
			if secretName, ok := secretMap["secretName"].(string); ok && secretName != "" {
				targetKey := types.ResourceKey{
					GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Secret"},
					Namespace: namespace,
					Name:      secretName,
				}

				if _, exists := allResources[targetKey]; exists {
					relationships = append(relationships, types.Relationship{
						From: resource.Original.ResourceKey(),
						To:   targetKey,
						Type: types.RelationVolumeMount,
						Field: "spec.template.spec.volumes[].secret",
						Details: map[string]string{
							"volumeName": volumeMap["name"].(string),
							"secretName": secretName,
						},
					})
				}
			}
		}

		// PVC volume
		if pvcMap, ok := volumeMap["persistentVolumeClaim"].(map[string]interface{}); ok {
			if pvcName, ok := pvcMap["claimName"].(string); ok && pvcName != "" {
				targetKey := types.ResourceKey{
					GVK:       schema.GroupVersionKind{Version: "v1", Kind: "PersistentVolumeClaim"},
					Namespace: namespace,
					Name:      pvcName,
				}

				if _, exists := allResources[targetKey]; exists {
					relationships = append(relationships, types.Relationship{
						From: resource.Original.ResourceKey(),
						To:   targetKey,
						Type: types.RelationPVC,
						Field: "spec.template.spec.volumes[].persistentVolumeClaim",
						Details: map[string]string{
							"volumeName": volumeMap["name"].(string),
							"pvcName":    pvcName,
						},
					})
				}
			}
		}

		// Projected volume (can contain configMap, secret, serviceAccountToken, downwardAPI)
		if projectedMap, ok := volumeMap["projected"].(map[string]interface{}); ok {
			if sources, ok := projectedMap["sources"].([]interface{}); ok {
				for _, source := range sources {
					sourceMap, ok := source.(map[string]interface{})
					if !ok {
						continue
					}

					// ConfigMap in projected volume
					if cmMap, ok := sourceMap["configMap"].(map[string]interface{}); ok {
						if cmName, ok := cmMap["name"].(string); ok && cmName != "" {
							targetKey := types.ResourceKey{
								GVK:       schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
								Namespace: namespace,
								Name:      cmName,
							}

							if _, exists := allResources[targetKey]; exists {
								relationships = append(relationships, types.Relationship{
									From: resource.Original.ResourceKey(),
									To:   targetKey,
									Type: types.RelationVolumeMount,
									Field: "spec.template.spec.volumes[].projected.sources[].configMap",
									Details: map[string]string{
										"volumeName":    volumeMap["name"].(string),
										"configMapName": cmName,
									},
								})
							}
						}
					}

					// Secret in projected volume
					if secretMap, ok := sourceMap["secret"].(map[string]interface{}); ok {
						if secretName, ok := secretMap["name"].(string); ok && secretName != "" {
							targetKey := types.ResourceKey{
								GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Secret"},
								Namespace: namespace,
								Name:      secretName,
							}

							if _, exists := allResources[targetKey]; exists {
								relationships = append(relationships, types.Relationship{
									From: resource.Original.ResourceKey(),
									To:   targetKey,
									Type: types.RelationVolumeMount,
									Field: "spec.template.spec.volumes[].projected.sources[].secret",
									Details: map[string]string{
										"volumeName": volumeMap["name"].(string),
										"secretName": secretName,
									},
								})
							}
						}
					}
				}
			}
		}
	}

	// Check environment variables (envFrom)
	relationships = append(relationships, d.detectEnvFromReferences(resource, allResources)...)

	// Check environment variables (env[].valueFrom)
	relationships = append(relationships, d.detectEnvValueFromReferences(resource, allResources)...)

	return relationships
}

// detectEnvFromReferences detects envFrom references to ConfigMaps and Secrets.
func (d *VolumeMountDetector) detectEnvFromReferences(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	kind := obj.GetKind()
	namespace := obj.GetNamespace()

	// Get containers
	var containers []interface{}
	var found bool

	if kind == "CronJob" {
		containers, found, _ = unstructured.NestedSlice(obj.Object, "spec", "jobTemplate", "spec", "template", "spec", "containers")
	} else if kind == "Pod" {
		containers, found, _ = unstructured.NestedSlice(obj.Object, "spec", "containers")
	} else {
		containers, found, _ = unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	}

	if !found {
		return relationships
	}

	for _, cont := range containers {
		container, ok := cont.(map[string]interface{})
		if !ok {
			continue
		}

		envFrom, ok := container["envFrom"].([]interface{})
		if !ok {
			continue
		}

		for _, ef := range envFrom {
			envFromSource, ok := ef.(map[string]interface{})
			if !ok {
				continue
			}

			// ConfigMapRef
			if cmRef, ok := envFromSource["configMapRef"].(map[string]interface{}); ok {
				if cmName, ok := cmRef["name"].(string); ok && cmName != "" {
					targetKey := types.ResourceKey{
						GVK:       schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
						Namespace: namespace,
						Name:      cmName,
					}

					if _, exists := allResources[targetKey]; exists {
						relationships = append(relationships, types.Relationship{
							From: resource.Original.ResourceKey(),
							To:   targetKey,
							Type: types.RelationEnvFrom,
							Field: "spec.template.spec.containers[].envFrom[].configMapRef",
							Details: map[string]string{
								"configMapName": cmName,
							},
						})
					}
				}
			}

			// SecretRef
			if secretRef, ok := envFromSource["secretRef"].(map[string]interface{}); ok {
				if secretName, ok := secretRef["name"].(string); ok && secretName != "" {
					targetKey := types.ResourceKey{
						GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Secret"},
						Namespace: namespace,
						Name:      secretName,
					}

					if _, exists := allResources[targetKey]; exists {
						relationships = append(relationships, types.Relationship{
							From: resource.Original.ResourceKey(),
							To:   targetKey,
							Type: types.RelationEnvFrom,
							Field: "spec.template.spec.containers[].envFrom[].secretRef",
							Details: map[string]string{
								"secretName": secretName,
							},
						})
					}
				}
			}
		}
	}

	return relationships
}

// detectEnvValueFromReferences detects env[].valueFrom references to ConfigMaps and Secrets.
func (d *VolumeMountDetector) detectEnvValueFromReferences(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	kind := obj.GetKind()
	namespace := obj.GetNamespace()

	// Get containers
	var containers []interface{}
	var found bool

	if kind == "CronJob" {
		containers, found, _ = unstructured.NestedSlice(obj.Object, "spec", "jobTemplate", "spec", "template", "spec", "containers")
	} else if kind == "Pod" {
		containers, found, _ = unstructured.NestedSlice(obj.Object, "spec", "containers")
	} else {
		containers, found, _ = unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	}

	if !found {
		return relationships
	}

	for _, cont := range containers {
		container, ok := cont.(map[string]interface{})
		if !ok {
			continue
		}

		env, ok := container["env"].([]interface{})
		if !ok {
			continue
		}

		for _, e := range env {
			envVar, ok := e.(map[string]interface{})
			if !ok {
				continue
			}

			valueFrom, ok := envVar["valueFrom"].(map[string]interface{})
			if !ok {
				continue
			}

			// ConfigMapKeyRef
			if cmKeyRef, ok := valueFrom["configMapKeyRef"].(map[string]interface{}); ok {
				if cmName, ok := cmKeyRef["name"].(string); ok && cmName != "" {
					targetKey := types.ResourceKey{
						GVK:       schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
						Namespace: namespace,
						Name:      cmName,
					}

					if _, exists := allResources[targetKey]; exists {
						relationships = append(relationships, types.Relationship{
							From: resource.Original.ResourceKey(),
							To:   targetKey,
							Type: types.RelationEnvValueFrom,
							Field: "spec.template.spec.containers[].env[].valueFrom.configMapKeyRef",
							Details: map[string]string{
								"configMapName": cmName,
								"envVarName":    envVar["name"].(string),
							},
						})
					}
				}
			}

			// SecretKeyRef
			if secretKeyRef, ok := valueFrom["secretKeyRef"].(map[string]interface{}); ok {
				if secretName, ok := secretKeyRef["name"].(string); ok && secretName != "" {
					targetKey := types.ResourceKey{
						GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Secret"},
						Namespace: namespace,
						Name:      secretName,
					}

					if _, exists := allResources[targetKey]; exists {
						relationships = append(relationships, types.Relationship{
							From: resource.Original.ResourceKey(),
							To:   targetKey,
							Type: types.RelationEnvValueFrom,
							Field: "spec.template.spec.containers[].env[].valueFrom.secretKeyRef",
							Details: map[string]string{
								"secretName": secretName,
								"envVarName": envVar["name"].(string),
							},
						})
					}
				}
			}
		}
	}

	return relationships
}
