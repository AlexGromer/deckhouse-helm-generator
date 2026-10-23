package detector

import (
	"context"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestDexAndSPEDetector_DexAuthenticatorFromAuthURL(t *testing.T) {
	d := NewDexAndSPEDetector()

	tests := []struct {
		url      string
		wantName string
		wantNs   string
		wantOk   bool
	}{
		{"https://orders-dex-authenticator.shop.svc.cluster.local/dex-authenticator/auth", "orders", "shop", true},
		{"https://dex.example.com/auth", "", "", false},
		{"https://orders.shop.svc/auth", "", "", false},
	}

	for _, tt := range tests {
		name, ns, ok := d.dexAuthenticatorFromAuthURL(tt.url)
		if ok != tt.wantOk || name != tt.wantName || ns != tt.wantNs {
			t.Errorf("dexAuthenticatorFromAuthURL(%q) = (%q, %q, %v), want (%q, %q, %v)", tt.url, name, ns, ok, tt.wantName, tt.wantNs, tt.wantOk)
		}
	}
}

func TestDexAndSPEDetector_DexAuthenticator(t *testing.T) {
	ingress := makeAnnotationResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", "shop",
		map[string]string{
			"nginx.ingress.kubernetes.io/auth-url": "https://orders-dex-authenticator.shop.svc.cluster.local/dex-authenticator/auth",
		},
		nil,
	)

	dexAuthV1 := makeAnnotationResource(
		"deckhouse.io/v1", "DexAuthenticator", "orders", "shop",
		nil, nil,
	)

	allResources := buildAnnotationAllResources(ingress, dexAuthV1)

	d := NewDexAndSPEDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(rels))
	}

	rel := rels[0]
	if rel.Type != types.RelationDeckhouse {
		t.Errorf("Type = %q, want %q", rel.Type, types.RelationDeckhouse)
	}
	if rel.To.Name != "orders" || rel.To.Namespace != "shop" {
		t.Errorf("To = %v", rel.To)
	}
}

func TestDexAndSPEDetector_SecurityPolicyException(t *testing.T) {
	deploy := makeProcessedResource(
		"apps/v1", "Deployment", "legacy", "shop",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"security.deckhouse.io/security-policy-exception": "legacy-hostpath",
					},
				},
			},
		},
	)

	spe := makeProcessedResource(
		"deckhouse.io/v1alpha1", "SecurityPolicyException", "legacy-hostpath", "shop",
		nil, nil, nil,
	)

	allResources := buildAllResources(deploy, spe)

	d := NewDexAndSPEDetector()
	rels := d.Detect(context.Background(), deploy, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(rels))
	}

	rel := rels[0]
	if rel.Type != types.RelationNameReference {
		t.Errorf("Type = %q, want %q", rel.Type, types.RelationNameReference)
	}
	if rel.To.Name != "legacy-hostpath" || rel.To.Namespace != "shop" {
		t.Errorf("To = %v", rel.To)
	}
	if rel.Field != "spec.template.metadata.labels[security.deckhouse.io/security-policy-exception]" {
		t.Errorf("Field = %q", rel.Field)
	}
}

func TestDexAndSPEDetector_SecurityPolicyException_Container(t *testing.T) {
	deploy := makeProcessedResource(
		"apps/v1", "Deployment", "legacy", "shop",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"security.deckhouse.io/security-policy-exception.container.legacy": "legacy-hostpath",
					},
				},
			},
		},
	)

	spe := makeProcessedResource(
		"deckhouse.io/v1alpha1", "SecurityPolicyException", "legacy-hostpath", "shop",
		nil, nil, nil,
	)

	allResources := buildAllResources(deploy, spe)

	d := NewDexAndSPEDetector()
	rels := d.Detect(context.Background(), deploy, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(rels))
	}

	rel := rels[0]
	if rel.Type != types.RelationNameReference {
		t.Errorf("Type = %q, want %q", rel.Type, types.RelationNameReference)
	}
	if rel.To.Name != "legacy-hostpath" || rel.To.Namespace != "shop" {
		t.Errorf("To = %v", rel.To)
	}
}
