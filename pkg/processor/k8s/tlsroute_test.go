package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create TLSRoute unstructured object
// ============================================================

func makeTLSRouteObj(name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1alpha2",
			"kind":       "TLSRoute",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
}

// ============================================================
// Test: TLSRoute extraction
// ============================================================

func TestTLSRoute_Extraction(t *testing.T) {
	proc := NewTLSRouteProcessor()
	ctx := newTestProcessorContext()

	obj := makeTLSRouteObj("tls-route", "default", map[string]interface{}{
		"parentRefs": []interface{}{
			map[string]interface{}{
				"name":      "my-gateway",
				"namespace": "gateway-ns",
			},
		},
		"rules": []interface{}{
			map[string]interface{}{
				"backendRefs": []interface{}{
					map[string]interface{}{
						"name": "tls-backend",
						"port": int64(8443),
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")
	testutil.AssertEqual(t, "tls-route", result.ServiceName, "serviceName")

	// Check parentRefs extracted
	parentRefs, ok := result.Values["parentRefs"].([]interface{})
	if !ok || len(parentRefs) != 1 {
		t.Fatal("Expected 1 parentRef")
	}

	// Check rules extracted
	rules, ok := result.Values["rules"].([]interface{})
	if !ok || len(rules) != 1 {
		t.Fatal("Expected 1 rule")
	}

	// Check Gateway dependency
	if len(result.Dependencies) == 0 {
		t.Fatal("Expected dependency to Gateway")
	}
	found := hasDependency(result.Dependencies, "Gateway", "gateway-ns", "my-gateway")
	if !found {
		t.Errorf("Expected dependency to Gateway 'my-gateway', got: %v", result.Dependencies)
	}

	// Check GVK
	gvks := proc.Supports()
	expected := schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1alpha2", Kind: "TLSRoute"}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")

	// Check template
	tpl := result.TemplateContent
	testutil.AssertContains(t, tpl, "kind: TLSRoute", "kind")
	if !strings.Contains(tpl, "parentRefs") {
		t.Error("Template should reference parentRefs")
	}
}

// ============================================================
// Test: TLSRoute processor name
// ============================================================

func TestTLSRoute_Name(t *testing.T) {
	proc := NewTLSRouteProcessor()
	testutil.AssertEqual(t, "tlsroute", proc.Name(), "processor name")
}
