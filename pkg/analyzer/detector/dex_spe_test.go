package detector

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestDexAndSPEDetector(t *testing.T) {
	ingress := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			GVK: schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"},
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "my-ingress",
						"namespace": "test-ns",
						"annotations": map[string]interface{}{
							"nginx.ingress.kubernetes.io/auth-url": "https://orders-dex-authenticator.test-ns.svc.cluster.local/dex-authenticator/auth",
						},
					},
				},
			},
		},
	}

	dex := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			GVK: schema.GroupVersionKind{Group: "deckhouse.io", Version: "v2alpha1", Kind: "DexAuthenticator"},
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "orders",
						"namespace": "test-ns",
					},
				},
			},
		},
	}

	workload := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			GVK: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "my-deploy",
						"namespace": "test-ns",
					},
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"security.deckhouse.io/security-policy-exception": "legacy-hostpath",
								},
							},
						},
					},
				},
			},
		},
	}

	spe := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			GVK: schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1alpha1", Kind: "SecurityPolicyException"},
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "legacy-hostpath",
						"namespace": "test-ns",
					},
				},
			},
		},
	}

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		dex.Original.ResourceKey(): dex,
		spe.Original.ResourceKey(): spe,
	}

	detector := NewDexAndSPEDetector()

	// Test Ingress -> Dex
	rels := detector.Detect(context.Background(), ingress, allResources)
	if len(rels) != 1 {
		t.Fatalf("expected 1 Dex relation, got %d", len(rels))
	}
	if rels[0].To.GVK.Kind != "DexAuthenticator" {
		t.Errorf("expected to DexAuthenticator, got %s", rels[0].To.GVK.Kind)
	}

	// Test Workload -> SPE
	rels = detector.Detect(context.Background(), workload, allResources)
	if len(rels) != 1 {
		t.Fatalf("expected 1 SPE relation, got %d", len(rels))
	}
	if rels[0].To.GVK.Kind != "SecurityPolicyException" {
		t.Errorf("expected to SecurityPolicyException, got %s", rels[0].To.GVK.Kind)
	}
}
