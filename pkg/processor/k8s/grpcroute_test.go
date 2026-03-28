package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create GRPCRoute unstructured object
// ============================================================

func makeGRPCRouteObj(name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "GRPCRoute",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
}

// ============================================================
// Test: GRPCRoute extraction
// ============================================================

func TestGRPCRoute_Extraction(t *testing.T) {
	proc := NewGRPCRouteProcessor()
	ctx := newTestProcessorContext()

	obj := makeGRPCRouteObj("grpc-route", "default", map[string]interface{}{
		"parentRefs": []interface{}{
			map[string]interface{}{
				"name":      "my-gateway",
				"namespace": "gateway-ns",
			},
		},
		"rules": []interface{}{
			map[string]interface{}{
				"matches": []interface{}{
					map[string]interface{}{
						"method": map[string]interface{}{
							"service": "myapp.v1.MyService",
							"method":  "GetItem",
						},
					},
				},
				"backendRefs": []interface{}{
					map[string]interface{}{
						"name": "grpc-svc",
						"port": int64(50051),
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")
	testutil.AssertEqual(t, "grpc-route", result.ServiceName, "serviceName")

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
	expected := schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "GRPCRoute"}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")

	// Check template
	tpl := result.TemplateContent
	testutil.AssertContains(t, tpl, "kind: GRPCRoute", "kind")
	if !strings.Contains(tpl, "parentRefs") {
		t.Error("Template should reference parentRefs")
	}
}

// ============================================================
// Test: GRPCRoute processor name
// ============================================================

func TestGRPCRoute_Name(t *testing.T) {
	proc := NewGRPCRouteProcessor()
	testutil.AssertEqual(t, "grpcroute", proc.Name(), "processor name")
}
