package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create Certificate unstructured object
// ============================================================

func makeCertificateObj(name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cert-manager.io/v1",
			"kind":       "Certificate",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
}

// ============================================================
// Test 1: Processor name
// ============================================================

func TestCertificateProcessor_Name(t *testing.T) {
	proc := NewCertificateProcessor()
	testutil.AssertEqual(t, "certificate", proc.Name(), "processor name")
}

// ============================================================
// Test 2: Supports GVK
// ============================================================

func TestCertificateProcessor_Supports(t *testing.T) {
	proc := NewCertificateProcessor()
	gvks := proc.Supports()

	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}

	expected := schema.GroupVersionKind{
		Group:   "cert-manager.io",
		Version: "v1",
		Kind:    "Certificate",
	}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// Test 3: DNSNames
// ============================================================

func TestCertificateProcessor_DNSNames(t *testing.T) {
	proc := NewCertificateProcessor()
	ctx := newTestProcessorContext()

	obj := makeCertificateObj("myapp-cert", "default", map[string]interface{}{
		"dnsNames":   []interface{}{"app.example.com", "www.example.com"},
		"secretName": "myapp-tls",
		"issuerRef": map[string]interface{}{
			"name": "letsencrypt-prod",
			"kind": "ClusterIssuer",
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")

	dnsNames, ok := result.Values["dnsNames"].([]interface{})
	if !ok {
		t.Fatal("Expected dnsNames slice in values")
	}
	if len(dnsNames) != 2 {
		t.Fatalf("Expected 2 dnsNames, got %d", len(dnsNames))
	}
	testutil.AssertEqual(t, "app.example.com", dnsNames[0], "first dnsName")
}

// ============================================================
// Test 4: IssuerRef
// ============================================================

func TestCertificateProcessor_IssuerRef(t *testing.T) {
	proc := NewCertificateProcessor()
	ctx := newTestProcessorContext()

	obj := makeCertificateObj("myapp-cert", "default", map[string]interface{}{
		"dnsNames":   []interface{}{"app.example.com"},
		"secretName": "myapp-tls",
		"issuerRef": map[string]interface{}{
			"name":  "letsencrypt-prod",
			"kind":  "ClusterIssuer",
			"group": "cert-manager.io",
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	issuerRef, ok := result.Values["issuerRef"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected issuerRef map in values")
	}
	testutil.AssertEqual(t, "letsencrypt-prod", issuerRef["name"], "issuer name")
	testutil.AssertEqual(t, "ClusterIssuer", issuerRef["kind"], "issuer kind")
}

// ============================================================
// Test 5: SecretName
// ============================================================

func TestCertificateProcessor_SecretName(t *testing.T) {
	proc := NewCertificateProcessor()
	ctx := newTestProcessorContext()

	obj := makeCertificateObj("myapp-cert", "default", map[string]interface{}{
		"dnsNames":   []interface{}{"app.example.com"},
		"secretName": "myapp-tls",
		"issuerRef": map[string]interface{}{
			"name": "letsencrypt-prod",
			"kind": "ClusterIssuer",
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "myapp-tls", result.Values["secretName"], "secretName")
}

// ============================================================
// Test 6: Duration and RenewBefore
// ============================================================

func TestCertificateProcessor_Duration(t *testing.T) {
	proc := NewCertificateProcessor()
	ctx := newTestProcessorContext()

	obj := makeCertificateObj("myapp-cert", "default", map[string]interface{}{
		"dnsNames":   []interface{}{"app.example.com"},
		"secretName": "myapp-tls",
		"issuerRef": map[string]interface{}{
			"name": "letsencrypt-prod",
			"kind": "ClusterIssuer",
		},
		"duration":    "2160h",
		"renewBefore": "360h",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "2160h", result.Values["duration"], "duration")
	testutil.AssertEqual(t, "360h", result.Values["renewBefore"], "renewBefore")
}

// ============================================================
// Test 7: Template content
// ============================================================

func TestCertificateProcessor_Template(t *testing.T) {
	proc := NewCertificateProcessor()
	ctx := newTestProcessorContext()

	obj := makeCertificateObj("myapp-cert", "default", map[string]interface{}{
		"dnsNames":   []interface{}{"app.example.com"},
		"secretName": "myapp-tls",
		"issuerRef": map[string]interface{}{
			"name": "letsencrypt-prod",
			"kind": "ClusterIssuer",
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	testutil.AssertContains(t, tpl, "apiVersion: cert-manager.io/v1", "apiVersion")
	testutil.AssertContains(t, tpl, "kind: Certificate", "kind")
	testutil.AssertContains(t, tpl, "$svc.enabled", "enabled check")
	if !strings.Contains(tpl, "issuerRef") {
		t.Error("Template should reference issuerRef")
	}
}

// ============================================================
// Test 8: ServiceName
// ============================================================

func TestCertificateProcessor_ServiceName(t *testing.T) {
	proc := NewCertificateProcessor()
	ctx := newTestProcessorContext()

	obj := makeCertificateObj("myapp-cert", "default", map[string]interface{}{
		"dnsNames":   []interface{}{"app.example.com"},
		"secretName": "myapp-tls",
		"issuerRef": map[string]interface{}{
			"name": "letsencrypt-prod",
			"kind": "ClusterIssuer",
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "myapp-cert", result.ServiceName, "ServiceName")
}
