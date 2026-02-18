// Package detector provides relationship detectors for Kubernetes resources.
package detector

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// LabelSelectorDetector detects relationships based on label selectors.
// Example: Service selecting Deployment pods.
type LabelSelectorDetector struct {
	priority int
}

// NewLabelSelectorDetector creates a new label selector detector.
func NewLabelSelectorDetector() *LabelSelectorDetector {
	return &LabelSelectorDetector{
		priority: 100,
	}
}

// Name returns the detector name.
func (d *LabelSelectorDetector) Name() string {
	return "label_selector"
}

// Priority returns the detector priority.
func (d *LabelSelectorDetector) Priority() int {
	return d.priority
}

// Detect detects label selector relationships.
func (d *LabelSelectorDetector) Detect(ctx context.Context, resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	kind := obj.GetKind()

	switch kind {
	case "Service":
		relationships = append(relationships, d.detectServiceToWorkload(resource, allResources)...)
	case "ServiceMonitor":
		relationships = append(relationships, d.detectServiceMonitorToService(resource, allResources)...)
	}

	return relationships
}

// detectServiceToWorkload detects Service -> Deployment/StatefulSet/DaemonSet relationships.
func (d *LabelSelectorDetector) detectServiceToWorkload(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	namespace := obj.GetNamespace()

	// Get service selector
	selector, found, err := unstructured.NestedStringMap(obj.Object, "spec", "selector")
	if !found || err != nil || len(selector) == 0 {
		return relationships
	}

	// Convert to label selector
	labelSelector := labels.Set(selector).AsSelector()

	// Check all workload resources
	workloadKinds := []string{"Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Pod"}

	for key, targetResource := range allResources {
		if key.Namespace != namespace {
			continue
		}

		// Check if target is a workload
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

		// Get pod labels from workload
		var podLabels map[string]string
		targetObj := targetResource.Original.Object

		if key.GVK.Kind == "Pod" {
			podLabels = targetObj.GetLabels()
		} else {
			// For Deployment, StatefulSet, DaemonSet - check template labels
			podLabels, _, _ = unstructured.NestedStringMap(targetObj.Object, "spec", "template", "metadata", "labels")
		}

		if len(podLabels) == 0 {
			continue
		}

		// Check if selector matches pod labels
		if labelSelector.Matches(labels.Set(podLabels)) {
			relationships = append(relationships, types.Relationship{
				From: resource.Original.ResourceKey(),
				To:   key,
				Type: types.RelationLabelSelector,
				Field: "spec.selector",
				Details: map[string]string{
					"selector": labelSelector.String(),
				},
			})
		}
	}

	return relationships
}

// detectServiceMonitorToService detects ServiceMonitor -> Service relationships.
func (d *LabelSelectorDetector) detectServiceMonitorToService(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	namespace := obj.GetNamespace()

	// Get ServiceMonitor selector
	selectorMap, found, err := unstructured.NestedMap(obj.Object, "spec", "selector")
	if !found || err != nil {
		return relationships
	}

	matchLabels, found, err := unstructured.NestedStringMap(selectorMap, "matchLabels")
	if !found || err != nil || len(matchLabels) == 0 {
		return relationships
	}

	labelSelector := labels.Set(matchLabels).AsSelector()

	// Check all Services
	for key, targetResource := range allResources {
		if key.GVK.Kind != "Service" {
			continue
		}
		if key.Namespace != namespace {
			continue
		}

		targetObj := targetResource.Original.Object
		serviceLabels := targetObj.GetLabels()

		if len(serviceLabels) == 0 {
			continue
		}

		if labelSelector.Matches(labels.Set(serviceLabels)) {
			relationships = append(relationships, types.Relationship{
				From: resource.Original.ResourceKey(),
				To:   key,
				Type: types.RelationServiceMonitor,
				Field: "spec.selector",
				Details: map[string]string{
					"selector": labelSelector.String(),
				},
			})
		}
	}

	return relationships
}
