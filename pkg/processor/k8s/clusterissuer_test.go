package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create ClusterIssuer unstructured object
// ============================================================

func makeClusterIssuerObj(name string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "ClusterIssuer",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": spec,
		},
	}
}

// ============================================================
// Test 1: Processor name
// ============================================================

func TestClusterIssuerProcessor_Name(t *testing.T) {
	proc := NewClusterIssuerProcessor()
	testutil.AssertEqual(t, "clusterissuer", proc.Name(), "processor name")
}

// ============================================================
// Test 2: Supports GVK
// ============================================================

func TestClusterIssuerProcessor_Supports(t *testing.T) {
	proc := NewClusterIssuerProcessor()
	gvks := proc.Supports()

	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}

	expected := schema.GroupVersionKind{
		Group:   "cert-manager.io",
		Version: "v1",
		Kind:    "ClusterIssuer",
	}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// Test 3: ACME Server
// ============================================================

func TestClusterIssuerProcessor_ACME_Server(t *testing.T) {
	proc := NewClusterIssuerProcessor()
	ctx := newTestProcessorContext()

	obj := makeClusterIssuerObj("letsencrypt-prod", map[string]interface{}{
		"acme": map[string]interface{}{
			"server": "https://acme-v02.api.letsencrypt.org/directory",
			"email":  "admin@example.com",
			"privateKeySecretRef": map[string]interface{}{
				"name": "letsencrypt-prod-key",
			},
			"solvers": []interface{}{
				map[string]interface{}{
					"http01": map[string]interface{}{
						"ingress": map[string]interface{}{
							"class": "nginx",
						},
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")

	acme, ok := result.Values["acme"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected acme map in values")
	}
	testutil.AssertEqual(t, "https://acme-v02.api.letsencrypt.org/directory", acme["server"], "ACME server")
}

// ============================================================
// Test 4: ACME Email
// ============================================================

func TestClusterIssuerProcessor_ACME_Email(t *testing.T) {
	proc := NewClusterIssuerProcessor()
	ctx := newTestProcessorContext()

	obj := makeClusterIssuerObj("letsencrypt-prod", map[string]interface{}{
		"acme": map[string]interface{}{
			"server": "https://acme-v02.api.letsencrypt.org/directory",
			"email":  "admin@example.com",
			"privateKeySecretRef": map[string]interface{}{
				"name": "letsencrypt-prod-key",
			},
			"solvers": []interface{}{},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	acme := result.Values["acme"].(map[string]interface{})
	testutil.AssertEqual(t, "admin@example.com", acme["email"], "ACME email")
}

// ============================================================
// Test 5: ACME Solvers
// ============================================================

func TestClusterIssuerProcessor_ACME_Solvers(t *testing.T) {
	proc := NewClusterIssuerProcessor()
	ctx := newTestProcessorContext()

	obj := makeClusterIssuerObj("letsencrypt-prod", map[string]interface{}{
		"acme": map[string]interface{}{
			"server": "https://acme-v02.api.letsencrypt.org/directory",
			"email":  "admin@example.com",
			"privateKeySecretRef": map[string]interface{}{
				"name": "letsencrypt-prod-key",
			},
			"solvers": []interface{}{
				map[string]interface{}{
					"http01": map[string]interface{}{
						"ingress": map[string]interface{}{"class": "nginx"},
					},
				},
				map[string]interface{}{
					"dns01": map[string]interface{}{
						"cloudflare": map[string]interface{}{
							"apiTokenSecretRef": map[string]interface{}{
								"name": "cloudflare-api",
								"key":  "token",
							},
						},
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	acme := result.Values["acme"].(map[string]interface{})
	solvers, ok := acme["solvers"].([]interface{})
	if !ok {
		t.Fatal("Expected solvers slice in acme")
	}
	if len(solvers) != 2 {
		t.Fatalf("Expected 2 solvers, got %d", len(solvers))
	}
}

// ============================================================
// Test 6: SelfSigned issuer
// ============================================================

func TestClusterIssuerProcessor_SelfSigned(t *testing.T) {
	proc := NewClusterIssuerProcessor()
	ctx := newTestProcessorContext()

	obj := makeClusterIssuerObj("selfsigned", map[string]interface{}{
		"selfSigned": map[string]interface{}{},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	selfSigned, ok := result.Values["selfSigned"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected selfSigned map in values")
	}
	_ = selfSigned // exists is enough
}

// ============================================================
// Test 7: Template content
// ============================================================

func TestClusterIssuerProcessor_Template(t *testing.T) {
	proc := NewClusterIssuerProcessor()
	ctx := newTestProcessorContext()

	obj := makeClusterIssuerObj("letsencrypt-prod", map[string]interface{}{
		"acme": map[string]interface{}{
			"server": "https://acme-v02.api.letsencrypt.org/directory",
			"email":  "admin@example.com",
			"privateKeySecretRef": map[string]interface{}{
				"name": "key",
			},
			"solvers": []interface{}{},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	testutil.AssertContains(t, tpl, "apiVersion: cert-manager.io/v1", "apiVersion")
	testutil.AssertContains(t, tpl, "kind: ClusterIssuer", "kind")
	testutil.AssertContains(t, tpl, "$svc.enabled", "enabled check")
}
