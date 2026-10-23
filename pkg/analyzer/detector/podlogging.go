package detector

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// PodLoggingDetector detects relationships between PodLoggingConfig and workloads based on label selector.
type PodLoggingDetector struct {
	priority int
}

// NewPodLoggingDetector creates a new pod logging detector.
func NewPodLoggingDetector() *PodLoggingDetector {
	return &PodLoggingDetector{
		priority: 100, // Same priority as LabelSelectorDetector
	}
}

// Name returns the detector name.
func (d *PodLoggingDetector) Name() string {
	return "pod_logging_selector"
}

// Priority returns the detector priority.
func (d *PodLoggingDetector) Priority() int {
	return d.priority
}

// Detect detects label selector relationships for PodLoggingConfig.
func (d *PodLoggingDetector) Detect(ctx context.Context, resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	kind := obj.GetKind()

	if kind == "PodLoggingConfig" {
		relationships = append(relationships, d.detectPodLoggingConfigToWorkload(resource, allResources)...)
	}

	return relationships
}

func (d *PodLoggingDetector) detectPodLoggingConfigToWorkload(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	namespace := obj.GetNamespace()

	// Get podloggingconfig selector
	selectorMap, found, err := unstructured.NestedMap(obj.Object, "spec", "labelSelector")
	if !found || err != nil || len(selectorMap) == 0 {
		return relationships
	}

	var labelSelector metav1.LabelSelector
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(selectorMap, &labelSelector); err != nil {
		return relationships
	}

	sel, err := metav1.LabelSelectorAsSelector(&labelSelector)
	if err != nil {
		return relationships
	}

	// Check all workload resources
	workloadKinds := []string{"Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job", "CronJob", "Pod", "Rollout"}

	for key, targetResource := range allResources {
		if key.Namespace != namespace {
			continue
		}

		isWorkload := false
		for _, wk := range workloadKinds {
			if key.GVK.Kind == wk {
				isWorkload = true
				break
			}
		}
		if !isWorkload {
			continue
		}

		var podLabels map[string]string
		targetObj := targetResource.Original.Object

		switch key.GVK.Kind {
		case "Pod":
			podLabels = targetObj.GetLabels()
		case "CronJob":
			podLabels, _, _ = unstructured.NestedStringMap(targetObj.Object, "spec", "jobTemplate", "spec", "template", "metadata", "labels")
		default:
			podLabels, _, _ = unstructured.NestedStringMap(targetObj.Object, "spec", "template", "metadata", "labels")
		}

		if len(podLabels) == 0 {
			continue
		}

		if sel.Matches(labels.Set(podLabels)) {
			relationships = append(relationships, types.Relationship{
				From:  resource.Original.ResourceKey(),
				To:    key,
				Type:  types.RelationLabelSelector,
				Field: "spec.labelSelector",
				Details: map[string]string{
					"selector": sel.String(),
				},
			})
		}
	}

	return relationships
}
