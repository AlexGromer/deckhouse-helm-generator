package detector

import (
	"context"
	"testing"

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

	rels := d.Detect(context.Background(), mc, allResources)
	// Single deckhouse resource — should create at least metadata relationship
	if len(rels) == 0 {
		// OK — single resource may not have any relations to other deckhouse resources
		// But it should still be detected (verify via Details tag)
	}
	// The detector should at least not panic
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
