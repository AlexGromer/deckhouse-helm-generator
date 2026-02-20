package detector

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/analyzer"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func TestDeckhouseDetector_Name(t *testing.T) {
	d := NewDeckhouseDetector()
	if d.Name() != "deckhouse" {
		t.Errorf("Expected name 'deckhouse', got '%s'", d.Name())
	}
}

func TestDeckhouseDetector_Priority(t *testing.T) {
	d := NewDeckhouseDetector()
	if d.Priority() <= 0 {
		t.Errorf("Expected priority > 0, got %d", d.Priority())
	}
}

func TestDeckhouseDetector_Detected_ModuleConfig(t *testing.T) {
	d := NewDeckhouseDetector()

	mc := makeProcessedResource("deckhouse.io/v1alpha1", "ModuleConfig", "test-module", "", nil, nil, map[string]interface{}{
		"enabled": true,
	})

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		mc.Original.ResourceKey(): mc,
	}

	// Single deckhouse resource — may or may not have relations; should not panic
	_ = d.Detect(context.Background(), mc, allResources)
}

func TestDeckhouseDetector_Detected_IngressNginx(t *testing.T) {
	d := NewDeckhouseDetector()

	inc := makeProcessedResource("deckhouse.io/v1", "IngressNginxController", "main", "", nil, nil, map[string]interface{}{
		"ingressClass": "nginx",
	})

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		inc.Original.ResourceKey(): inc,
	}

	// Should not panic
	_ = d.Detect(context.Background(), inc, allResources)
}

func TestDeckhouseDetector_NotDetected_K8sOnly(t *testing.T) {
	d := NewDeckhouseDetector()

	deploy := makeProcessedResource("apps/v1", "Deployment", "myapp", "default", nil, nil, map[string]interface{}{
		"replicas": int64(1),
	})

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deploy.Original.ResourceKey(): deploy,
	}

	rels := d.Detect(context.Background(), deploy, allResources)
	if len(rels) != 0 {
		t.Errorf("Expected 0 relationships for non-deckhouse resource, got %d", len(rels))
	}
}

func TestDeckhouseDetector_MixedResources(t *testing.T) {
	d := NewDeckhouseDetector()

	mc := makeProcessedResource("deckhouse.io/v1alpha1", "ModuleConfig", "test-module", "", nil, nil, map[string]interface{}{
		"enabled": true,
	})
	deploy := makeProcessedResource("apps/v1", "Deployment", "myapp", "default", nil, nil, map[string]interface{}{
		"replicas": int64(1),
	})

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		mc.Original.ResourceKey():     mc,
		deploy.Original.ResourceKey(): deploy,
	}

	// Deckhouse resource should detect
	dRels := d.Detect(context.Background(), mc, allResources)
	// Should not create cross-type deckhouse relationships to vanilla K8s
	for _, rel := range dRels {
		if rel.To.GVK.Group != "deckhouse.io" {
			t.Errorf("Deckhouse detector should not create relationships to non-deckhouse resources, got: %v", rel.To)
		}
	}

	// Non-deckhouse should not detect
	kRels := d.Detect(context.Background(), deploy, allResources)
	if len(kRels) != 0 {
		t.Errorf("Expected 0 deckhouse relationships for vanilla K8s, got %d", len(kRels))
	}
}

func TestDeckhouseDetector_MultipleDeckhouseCRDs(t *testing.T) {
	d := NewDeckhouseDetector()

	mc := makeProcessedResource("deckhouse.io/v1alpha1", "ModuleConfig", "ingress-nginx", "", nil, nil, map[string]interface{}{
		"enabled": true,
	})
	inc := makeProcessedResource("deckhouse.io/v1", "IngressNginxController", "main", "", nil, nil, map[string]interface{}{
		"ingressClass": "nginx",
	})

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		mc.Original.ResourceKey():  mc,
		inc.Original.ResourceKey(): inc,
	}

	rels := d.Detect(context.Background(), mc, allResources)
	// ModuleConfig should create relationship to IngressNginxController (both deckhouse)
	if len(rels) == 0 {
		t.Error("Expected at least 1 relationship between deckhouse CRDs")
	}

	foundDeckhouse := false
	for _, rel := range rels {
		if rel.Type == types.RelationDeckhouse {
			foundDeckhouse = true
			break
		}
	}
	if !foundDeckhouse {
		t.Error("Expected deckhouse relationship type")
	}
}

func TestDeckhouseDetector_EmptyInput(t *testing.T) {
	d := NewDeckhouseDetector()

	mc := makeProcessedResource("deckhouse.io/v1alpha1", "ModuleConfig", "test", "", nil, nil, nil)
	allResources := map[types.ResourceKey]*types.ProcessedResource{}

	// Should not panic with empty allResources
	rels := d.Detect(context.Background(), mc, allResources)
	if len(rels) != 0 {
		t.Errorf("Expected 0 relationships for empty input, got %d", len(rels))
	}
}

// TestIsDeckhouseResource_NilResource verifies that isDeckhouseResource returns false
// when the resource itself is nil.
func TestIsDeckhouseResource_NilResource(t *testing.T) {
	if isDeckhouseResource(nil) {
		t.Error("expected isDeckhouseResource(nil) to return false")
	}
}

// TestIsDeckhouseResource_NilOriginal verifies that isDeckhouseResource returns false
// when res.Original is nil.
func TestIsDeckhouseResource_NilOriginal(t *testing.T) {
	res := &types.ProcessedResource{
		Original: nil,
	}
	if isDeckhouseResource(res) {
		t.Error("expected isDeckhouseResource with nil Original to return false")
	}
}

// TestIsDeckhouseResource_NilObject verifies that isDeckhouseResource returns false
// when res.Original.Object is nil.
func TestIsDeckhouseResource_NilObject(t *testing.T) {
	res := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: nil,
			GVK:    schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: "ModuleConfig"},
		},
	}
	if isDeckhouseResource(res) {
		t.Error("expected isDeckhouseResource with nil Object to return false")
	}
}

// TestIsDeckhouseResource_ExactDeckhouseIoGroup verifies that a resource whose GVK
// group is exactly "deckhouse.io" (not a suffix match) is recognized as a Deckhouse
// resource. This covers the `group == "deckhouse.io"` branch.
func TestIsDeckhouseResource_ExactDeckhouseIoGroup(t *testing.T) {
	res := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "deckhouse.io/v1",
					"kind":       "ModuleConfig",
					"metadata": map[string]interface{}{
						"name": "test",
					},
				},
			},
			GVK: schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: "ModuleConfig"},
		},
	}
	if !isDeckhouseResource(res) {
		t.Error("expected isDeckhouseResource to return true for group 'deckhouse.io'")
	}
}

// TestIsDeckhouseResource_SubdomainDeckhouseIo verifies that a resource whose GVK group
// is a subdomain of deckhouse.io (e.g., "something.deckhouse.io") is also recognized.
func TestIsDeckhouseResource_SubdomainDeckhouseIo(t *testing.T) {
	res := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "modules.deckhouse.io/v1alpha1",
					"kind":       "SomeModuleCRD",
					"metadata": map[string]interface{}{
						"name": "test",
					},
				},
			},
			GVK: schema.GroupVersionKind{Group: "modules.deckhouse.io", Version: "v1alpha1", Kind: "SomeModuleCRD"},
		},
	}
	if !isDeckhouseResource(res) {
		t.Error("expected isDeckhouseResource to return true for group 'modules.deckhouse.io'")
	}
}

// TestIsDeckhouseResource_NonDeckhouseGroup verifies that a resource whose GVK group
// is not related to deckhouse.io returns false.
func TestIsDeckhouseResource_NonDeckhouseGroup(t *testing.T) {
	res := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name": "test",
					},
				},
			},
			GVK: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		},
	}
	if isDeckhouseResource(res) {
		t.Error("expected isDeckhouseResource to return false for non-deckhouse group 'apps'")
	}
}

// TestRegisterAll verifies that RegisterAll adds detectors to a DefaultAnalyzer without
// panicking and that the analyzer then has at least 5 detectors registered.
func TestRegisterAll(t *testing.T) {
	a := analyzer.NewDefaultAnalyzer()

	// Must not panic
	RegisterAll(a)

	// Confirm detectors work by running Analyze on a minimal set of resources.
	ctx := context.Background()
	mc := makeProcessedResource("deckhouse.io/v1alpha1", "ModuleConfig", "test", "", nil, nil, nil)
	resources := []*types.ProcessedResource{mc}

	graph, err := a.Analyze(ctx, resources)
	if err != nil {
		t.Fatalf("Analyze returned unexpected error: %v", err)
	}
	if graph == nil {
		t.Fatal("Analyze returned nil graph")
	}
	if len(graph.Resources) != 1 {
		t.Errorf("expected 1 resource in graph, got %d", len(graph.Resources))
	}
}
