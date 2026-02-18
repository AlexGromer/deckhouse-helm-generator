package detector

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// makeProcessedResource creates a ProcessedResource for use in tests.
// labels and annotations are set on the object metadata.
// spec is set as the object's "spec" field.
func makeProcessedResource(apiVersion, kind, name, namespace string, labels, annotations map[string]interface{}, spec map[string]interface{}) *types.ProcessedResource {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
		},
	}

	if len(labels) > 0 {
		metadata := obj.Object["metadata"].(map[string]interface{})
		metadata["labels"] = labels
	}

	if len(annotations) > 0 {
		metadata := obj.Object["metadata"].(map[string]interface{})
		metadata["annotations"] = annotations
	}

	if spec != nil {
		obj.Object["spec"] = spec
	}

	gvk := obj.GroupVersionKind()

	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    gvk,
		},
	}
}

// resourceKey builds a ResourceKey matching what ExtractedResource.ResourceKey() returns.
func resourceKey(apiVersion, kind, namespace, name string) types.ResourceKey {
	gv, _ := schema.ParseGroupVersion(apiVersion)
	gvk := gv.WithKind(kind)
	return types.ResourceKey{
		GVK:       gvk,
		Namespace: namespace,
		Name:      name,
	}
}

// buildAllResources converts a slice of ProcessedResources into the allResources map
// keyed by their ResourceKey.
func buildAllResources(resources ...*types.ProcessedResource) map[types.ResourceKey]*types.ProcessedResource {
	m := make(map[types.ResourceKey]*types.ProcessedResource, len(resources))
	for _, r := range resources {
		m[r.Original.ResourceKey()] = r
	}
	return m
}

// TestNewLabelSelectorDetector verifies the constructor, Name(), and Priority().
func TestNewLabelSelectorDetector(t *testing.T) {
	d := NewLabelSelectorDetector()

	if d == nil {
		t.Fatal("NewLabelSelectorDetector() returned nil")
	}

	if got := d.Name(); got != "label_selector" {
		t.Errorf("Name() = %q; want %q", got, "label_selector")
	}

	if got := d.Priority(); got != 100 {
		t.Errorf("Priority() = %d; want 100", got)
	}
}

// TestLabelDetector_ServiceToDeployment verifies that a Service whose spec.selector
// matches a Deployment's pod template labels produces exactly one relationship.
func TestLabelDetector_ServiceToDeployment(t *testing.T) {
	svc := makeProcessedResource(
		"v1", "Service", "my-svc", "default",
		nil, nil,
		map[string]interface{}{
			"selector": map[string]interface{}{
				"app": "my-app",
			},
		},
	)

	deploy := makeProcessedResource(
		"apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app": "my-app",
					},
				},
			},
		},
	)

	allResources := buildAllResources(svc, deploy)

	d := NewLabelSelectorDetector()
	rels := d.Detect(context.Background(), svc, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(rels))
	}

	rel := rels[0]
	if rel.Type != types.RelationLabelSelector {
		t.Errorf("Type = %q; want %q", rel.Type, types.RelationLabelSelector)
	}
	if rel.Field != "spec.selector" {
		t.Errorf("Field = %q; want %q", rel.Field, "spec.selector")
	}
	if rel.From != svc.Original.ResourceKey() {
		t.Errorf("From = %v; want %v", rel.From, svc.Original.ResourceKey())
	}
	if rel.To != deploy.Original.ResourceKey() {
		t.Errorf("To = %v; want %v", rel.To, deploy.Original.ResourceKey())
	}
}

// TestLabelDetector_MultipleMatches verifies that a single Service selector that matches
// two different Deployments produces two relationships.
func TestLabelDetector_MultipleMatches(t *testing.T) {
	svc := makeProcessedResource(
		"v1", "Service", "my-svc", "default",
		nil, nil,
		map[string]interface{}{
			"selector": map[string]interface{}{
				"tier": "frontend",
			},
		},
	)

	deploy1 := makeProcessedResource(
		"apps/v1", "Deployment", "deploy-1", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"tier": "frontend",
						"env":  "prod",
					},
				},
			},
		},
	)

	deploy2 := makeProcessedResource(
		"apps/v1", "Deployment", "deploy-2", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"tier": "frontend",
						"env":  "staging",
					},
				},
			},
		},
	)

	allResources := buildAllResources(svc, deploy1, deploy2)

	d := NewLabelSelectorDetector()
	rels := d.Detect(context.Background(), svc, allResources)

	if len(rels) != 2 {
		t.Fatalf("expected 2 relationships, got %d", len(rels))
	}

	for _, rel := range rels {
		if rel.Type != types.RelationLabelSelector {
			t.Errorf("Type = %q; want %q", rel.Type, types.RelationLabelSelector)
		}
		if rel.From != svc.Original.ResourceKey() {
			t.Errorf("From = %v; want %v", rel.From, svc.Original.ResourceKey())
		}
	}
}

// TestLabelDetector_NoMatch verifies that when a Service selector does not match
// any workload pod template labels, no relationships are returned.
func TestLabelDetector_NoMatch(t *testing.T) {
	svc := makeProcessedResource(
		"v1", "Service", "my-svc", "default",
		nil, nil,
		map[string]interface{}{
			"selector": map[string]interface{}{
				"app": "nonexistent",
			},
		},
	)

	deploy := makeProcessedResource(
		"apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app": "other-app",
					},
				},
			},
		},
	)

	allResources := buildAllResources(svc, deploy)

	d := NewLabelSelectorDetector()
	rels := d.Detect(context.Background(), svc, allResources)

	if len(rels) != 0 {
		t.Errorf("expected 0 relationships, got %d", len(rels))
	}
}

// TestLabelDetector_EmptySelector verifies that a Service with no spec.selector
// produces no relationships.
func TestLabelDetector_EmptySelector(t *testing.T) {
	// Service with an empty selector map (present but empty)
	svcEmptyMap := makeProcessedResource(
		"v1", "Service", "svc-empty-map", "default",
		nil, nil,
		map[string]interface{}{
			"selector": map[string]interface{}{},
		},
	)

	// Service with no selector field at all
	svcNoSelector := makeProcessedResource(
		"v1", "Service", "svc-no-selector", "default",
		nil, nil,
		map[string]interface{}{
			"type": "ClusterIP",
		},
	)

	deploy := makeProcessedResource(
		"apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app": "my-app",
					},
				},
			},
		},
	)

	allResources := buildAllResources(svcEmptyMap, svcNoSelector, deploy)

	d := NewLabelSelectorDetector()

	relsEmptyMap := d.Detect(context.Background(), svcEmptyMap, allResources)
	if len(relsEmptyMap) != 0 {
		t.Errorf("empty selector map: expected 0 relationships, got %d", len(relsEmptyMap))
	}

	relsNoSelector := d.Detect(context.Background(), svcNoSelector, allResources)
	if len(relsNoSelector) != 0 {
		t.Errorf("no selector field: expected 0 relationships, got %d", len(relsNoSelector))
	}
}

// TestLabelDetector_CrossNamespace verifies that a Service in namespace "ns-a" does not
// match a Deployment in namespace "ns-b", even if labels match.
func TestLabelDetector_CrossNamespace(t *testing.T) {
	svc := makeProcessedResource(
		"v1", "Service", "my-svc", "ns-a",
		nil, nil,
		map[string]interface{}{
			"selector": map[string]interface{}{
				"app": "my-app",
			},
		},
	)

	deploy := makeProcessedResource(
		"apps/v1", "Deployment", "my-deploy", "ns-b",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app": "my-app",
					},
				},
			},
		},
	)

	allResources := buildAllResources(svc, deploy)

	d := NewLabelSelectorDetector()
	rels := d.Detect(context.Background(), svc, allResources)

	if len(rels) != 0 {
		t.Errorf("cross-namespace: expected 0 relationships, got %d", len(rels))
	}
}

// TestLabelDetector_PodKind verifies that a Service selector can match a Pod directly
// (using the Pod's metadata labels rather than spec.template.metadata.labels).
func TestLabelDetector_PodKind(t *testing.T) {
	svc := makeProcessedResource(
		"v1", "Service", "my-svc", "default",
		nil, nil,
		map[string]interface{}{
			"selector": map[string]interface{}{
				"app": "standalone-pod",
			},
		},
	)

	pod := makeProcessedResource(
		"v1", "Pod", "my-pod", "default",
		map[string]interface{}{
			"app": "standalone-pod",
		},
		nil,
		nil,
	)

	allResources := buildAllResources(svc, pod)

	d := NewLabelSelectorDetector()
	rels := d.Detect(context.Background(), svc, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship for Pod kind, got %d", len(rels))
	}

	rel := rels[0]
	if rel.Type != types.RelationLabelSelector {
		t.Errorf("Type = %q; want %q", rel.Type, types.RelationLabelSelector)
	}
	if rel.To != pod.Original.ResourceKey() {
		t.Errorf("To = %v; want %v", rel.To, pod.Original.ResourceKey())
	}
}

// TestLabelDetector_ServiceMonitorToService verifies that a ServiceMonitor whose
// spec.selector.matchLabels matches a Service's metadata labels produces one relationship
// of type RelationServiceMonitor.
func TestLabelDetector_ServiceMonitorToService(t *testing.T) {
	sm := makeProcessedResource(
		"monitoring.coreos.com/v1", "ServiceMonitor", "my-sm", "default",
		nil, nil,
		map[string]interface{}{
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": "my-app",
				},
			},
		},
	)

	svc := makeProcessedResource(
		"v1", "Service", "my-svc", "default",
		map[string]interface{}{
			"app": "my-app",
		},
		nil,
		nil,
	)

	allResources := buildAllResources(sm, svc)

	d := NewLabelSelectorDetector()
	rels := d.Detect(context.Background(), sm, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship for ServiceMonitor, got %d", len(rels))
	}

	rel := rels[0]
	if rel.Type != types.RelationServiceMonitor {
		t.Errorf("Type = %q; want %q", rel.Type, types.RelationServiceMonitor)
	}
	if rel.Field != "spec.selector" {
		t.Errorf("Field = %q; want %q", rel.Field, "spec.selector")
	}
	if rel.From != sm.Original.ResourceKey() {
		t.Errorf("From = %v; want %v", rel.From, sm.Original.ResourceKey())
	}
	if rel.To != svc.Original.ResourceKey() {
		t.Errorf("To = %v; want %v", rel.To, svc.Original.ResourceKey())
	}
}
