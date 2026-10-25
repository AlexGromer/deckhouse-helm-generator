package detector

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"context"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// StrimziDetector detects relationships between Strimzi resources.
type StrimziDetector struct {
	priority int
}

// NewStrimziDetector creates a new Strimzi detector.
func NewStrimziDetector() *StrimziDetector {
	return &StrimziDetector{
		priority: 75,
	}
}

// Name returns the detector name.
func (d *StrimziDetector) Name() string {
	return "strimzi"
}

// Priority returns the detector priority.
func (d *StrimziDetector) Priority() int {
	return d.priority
}

// Detect detects relationships for a processed resource.
func (d *StrimziDetector) Detect(ctx context.Context, resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	gvk := resource.Original.GVK

	if gvk.Group == "kafka.strimzi.io" && gvk.Kind == "KafkaUser" {
		relationships = append(relationships, d.detectKafkaUserToTopic(resource, allResources)...)
		relationships = append(relationships, d.detectKafkaUserToSecret(resource, allResources)...)
	}

	if gvk.Group == "kafka.strimzi.io" && (gvk.Kind == "KafkaTopic" || gvk.Kind == "KafkaUser") {
		relationships = append(relationships, d.detectStrimziEntityToKafka(resource, allResources)...)
	}

	workloadKinds := map[string]bool{
		"Deployment":  true,
		"StatefulSet": true,
		"DaemonSet":   true,
		"Job":         true,
		"CronJob":     true,
	}

	if workloadKinds[gvk.Kind] {
		relationships = append(relationships, d.detectWorkloadToKafkaUser(resource, allResources)...)
	}

	return relationships
}

func (d *StrimziDetector) detectKafkaUserToTopic(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship
	namespace := resource.Original.Object.GetNamespace()

	acls, found, _ := unstructured.NestedSlice(resource.Original.Object.Object, "spec", "authorization", "acls")
	if !found {
		return relationships
	}

	for _, aclRaw := range acls {
		acl, ok := aclRaw.(map[string]interface{})
		if !ok {
			continue
		}

		resMap, ok := acl["resource"].(map[string]interface{})
		if !ok {
			continue
		}

		resType, _ := resMap["type"].(string)
		if resType != "topic" {
			continue
		}

		patternType, _ := resMap["patternType"].(string)
		if patternType != "literal" && patternType != "" {
			continue
		}

		topicName, _ := resMap["name"].(string)
		if topicName == "" {
			continue
		}

		// Look for KafkaTopic with matching topicName or metadata.name
		for key, targetRes := range allResources {
			if key.Namespace == namespace && key.GVK.Group == "kafka.strimzi.io" && key.GVK.Kind == "KafkaTopic" {
				targetTopicName, found, _ := unstructured.NestedString(targetRes.Original.Object.Object, "spec", "topicName")
				if !found || targetTopicName == "" {
					targetTopicName = targetRes.Original.Object.GetName()
				}
				if targetTopicName == topicName {
					relationships = append(relationships, types.Relationship{
						From:  resource.Original.ResourceKey(),
						To:    key,
						Type:  types.RelationNameReference,
						Field: "spec.authorization.acls",
						Details: map[string]string{
							"strimzi": "true",
						},
					})
				}
			}
		}
	}

	return relationships
}

func (d *StrimziDetector) detectKafkaUserToSecret(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship
	namespace := resource.Original.Object.GetNamespace()

	secretName, found, _ := unstructured.NestedString(resource.Original.Object.Object, "spec", "authentication", "password", "valueFrom", "secretKeyRef", "name")
	if !found || secretName == "" {
		return relationships
	}

	targetKey := types.ResourceKey{
		GVK: resource.Original.GVK,
	}
	targetKey.GVK.Group = ""
	targetKey.GVK.Version = "v1"
	targetKey.GVK.Kind = "Secret"
	targetKey.Namespace = namespace
	targetKey.Name = secretName

	if _, exists := allResources[targetKey]; exists {
		relationships = append(relationships, types.Relationship{
			From:  resource.Original.ResourceKey(),
			To:    targetKey,
			Type:  types.RelationNameReference,
			Field: "spec.authentication.password",
			Details: map[string]string{
				"strimzi": "true",
			},
		})
	}

	return relationships
}

func (d *StrimziDetector) detectStrimziEntityToKafka(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship
	namespace := resource.Original.Object.GetNamespace()

	labels := resource.Original.Object.GetLabels()
	if labels == nil {
		return relationships
	}

	clusterName, found := labels["strimzi.io/cluster"]
	if !found || clusterName == "" {
		return relationships
	}

	for key := range allResources {
		if key.Namespace == namespace && key.GVK.Group == "kafka.strimzi.io" && key.GVK.Kind == "Kafka" && key.Name == clusterName {
			relationships = append(relationships, types.Relationship{
				From:  resource.Original.ResourceKey(),
				To:    key,
				Type:  types.RelationNameReference,
				Field: "metadata.labels[strimzi.io/cluster]",
				Details: map[string]string{
					"strimzi": "true",
				},
			})
			break
		}
	}

	return relationships
}

func podSpecOf(kind string, obj map[string]interface{}) map[string]interface{} {
	var podSpec map[string]interface{}
	var ok bool
	if kind == "CronJob" {
		podSpec, ok, _ = unstructured.NestedMap(obj, "spec", "jobTemplate", "spec", "template", "spec")
	} else {
		podSpec, ok, _ = unstructured.NestedMap(obj, "spec", "template", "spec")
	}
	if !ok {
		return nil
	}
	return podSpec
}

func (d *StrimziDetector) detectWorkloadToKafkaUser(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship
	namespace := resource.Original.Object.GetNamespace()

	podSpec := podSpecOf(resource.Original.Object.GetKind(), resource.Original.Object.Object)
	if podSpec == nil {
		return relationships
	}

	// Collect referenced secrets in the pod spec
	secretRefs := make(map[string]string) // name -> field path

	containers := []interface{}{}
	if c, ok, _ := unstructured.NestedSlice(podSpec, "containers"); ok {
		containers = append(containers, c...)
	}
	if c, ok, _ := unstructured.NestedSlice(podSpec, "initContainers"); ok {
		containers = append(containers, c...)
	}

	for _, containerRaw := range containers {
		container, ok := containerRaw.(map[string]interface{})
		if !ok {
			continue
		}

		if envs, ok, _ := unstructured.NestedSlice(container, "env"); ok {
			for _, envRaw := range envs {
				env, ok := envRaw.(map[string]interface{})
				if !ok {
					continue
				}
				if secretName, ok, _ := unstructured.NestedString(env, "valueFrom", "secretKeyRef", "name"); ok && secretName != "" {
					secretRefs[secretName] = "env[].valueFrom.secretKeyRef.name"
				}
			}
		}

		if envFroms, ok, _ := unstructured.NestedSlice(container, "envFrom"); ok {
			for _, envFromRaw := range envFroms {
				envFrom, ok := envFromRaw.(map[string]interface{})
				if !ok {
					continue
				}
				if secretName, ok, _ := unstructured.NestedString(envFrom, "secretRef", "name"); ok && secretName != "" {
					secretRefs[secretName] = "envFrom[].secretRef.name"
				}
			}
		}
	}

	if volumes, ok, _ := unstructured.NestedSlice(podSpec, "volumes"); ok {
		for _, volumeRaw := range volumes {
			volume, ok := volumeRaw.(map[string]interface{})
			if !ok {
				continue
			}

			if secretName, ok, _ := unstructured.NestedString(volume, "secret", "secretName"); ok && secretName != "" {
				secretRefs[secretName] = "volumes[].secret.secretName"
			}

			if sources, ok, _ := unstructured.NestedSlice(volume, "projected", "sources"); ok {
				for _, sourceRaw := range sources {
					source, ok := sourceRaw.(map[string]interface{})
					if !ok {
						continue
					}
					if secretName, ok, _ := unstructured.NestedString(source, "secret", "name"); ok && secretName != "" {
						secretRefs[secretName] = "volumes[].projected.sources[].secret.name"
					}
				}
			}
		}
	}

	for secretName, fieldPath := range secretRefs {
		for key := range allResources {
			if key.Namespace == namespace && key.GVK.Group == "kafka.strimzi.io" && key.GVK.Kind == "KafkaUser" && key.Name == secretName {
				relationships = append(relationships, types.Relationship{
					From:  resource.Original.ResourceKey(),
					To:    key,
					Type:  types.RelationNameReference,
					Field: fieldPath,
					Details: map[string]string{
						"strimzi": "true",
					},
				})
			}
		}
	}

	return relationships
}
