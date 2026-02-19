package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create HTTPRoute unstructured object
// ============================================================

func makeHTTPRouteObj(name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
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

func TestHTTPRouteProcessor_Name(t *testing.T) {
	proc := NewHTTPRouteProcessor()
	testutil.AssertEqual(t, "httproute", proc.Name(), "processor name")
}

// ============================================================
// Test 2: Supports GVK
// ============================================================

func TestHTTPRouteProcessor_Supports(t *testing.T) {
	proc := NewHTTPRouteProcessor()
	gvks := proc.Supports()

	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}

	expected := schema.GroupVersionKind{
		Group:   "gateway.networking.k8s.io",
		Version: "v1",
		Kind:    "HTTPRoute",
	}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// Test 3: ParentRefs extraction
// ============================================================

func TestHTTPRouteProcessor_ParentRefs(t *testing.T) {
	proc := NewHTTPRouteProcessor()
	ctx := newTestProcessorContext()

	obj := makeHTTPRouteObj("myapp-route", "default", map[string]interface{}{
		"parentRefs": []interface{}{
			map[string]interface{}{
				"name":        "my-gateway",
				"namespace":   "gateway-ns",
				"sectionName": "https",
			},
		},
		"rules": []interface{}{},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")

	parentRefs, ok := result.Values["parentRefs"].([]interface{})
	if !ok {
		t.Fatal("Expected parentRefs slice in values")
	}
	if len(parentRefs) != 1 {
		t.Fatalf("Expected 1 parentRef, got %d", len(parentRefs))
	}

	ref := parentRefs[0].(map[string]interface{})
	testutil.AssertEqual(t, "my-gateway", ref["name"], "parentRef name")
}

// ============================================================
// Test 4: Hostnames extraction
// ============================================================

func TestHTTPRouteProcessor_Hostnames(t *testing.T) {
	proc := NewHTTPRouteProcessor()
	ctx := newTestProcessorContext()

	obj := makeHTTPRouteObj("myapp-route", "default", map[string]interface{}{
		"parentRefs": []interface{}{
			map[string]interface{}{"name": "gw"},
		},
		"hostnames": []interface{}{"app.example.com", "api.example.com"},
		"rules":     []interface{}{},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	hostnames, ok := result.Values["hostnames"].([]interface{})
	if !ok {
		t.Fatal("Expected hostnames slice in values")
	}
	if len(hostnames) != 2 {
		t.Fatalf("Expected 2 hostnames, got %d", len(hostnames))
	}
	testutil.AssertEqual(t, "app.example.com", hostnames[0], "first hostname")
}

// ============================================================
// Test 5: Rules with path match
// ============================================================

func TestHTTPRouteProcessor_Rules_PathMatch(t *testing.T) {
	proc := NewHTTPRouteProcessor()
	ctx := newTestProcessorContext()

	obj := makeHTTPRouteObj("myapp-route", "default", map[string]interface{}{
		"parentRefs": []interface{}{
			map[string]interface{}{"name": "gw"},
		},
		"rules": []interface{}{
			map[string]interface{}{
				"matches": []interface{}{
					map[string]interface{}{
						"path": map[string]interface{}{
							"type":  "PathPrefix",
							"value": "/api",
						},
					},
				},
				"backendRefs": []interface{}{
					map[string]interface{}{
						"name": "myapp-svc",
						"port": int64(8080),
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	rules, ok := result.Values["rules"].([]interface{})
	if !ok {
		t.Fatal("Expected rules slice in values")
	}
	if len(rules) != 1 {
		t.Fatalf("Expected 1 rule, got %d", len(rules))
	}
}

// ============================================================
// Test 6: Rules with backendRefs
// ============================================================

func TestHTTPRouteProcessor_Rules_BackendRefs(t *testing.T) {
	proc := NewHTTPRouteProcessor()
	ctx := newTestProcessorContext()

	obj := makeHTTPRouteObj("myapp-route", "default", map[string]interface{}{
		"parentRefs": []interface{}{
			map[string]interface{}{"name": "gw"},
		},
		"rules": []interface{}{
			map[string]interface{}{
				"backendRefs": []interface{}{
					map[string]interface{}{
						"name":   "svc-a",
						"port":   int64(80),
						"weight": int64(80),
					},
					map[string]interface{}{
						"name":   "svc-b",
						"port":   int64(80),
						"weight": int64(20),
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	rules := result.Values["rules"].([]interface{})
	rule := rules[0].(map[string]interface{})
	backends := rule["backendRefs"].([]interface{})
	if len(backends) != 2 {
		t.Fatalf("Expected 2 backendRefs, got %d", len(backends))
	}
}

// ============================================================
// Test 7: Rules with filters
// ============================================================

func TestHTTPRouteProcessor_Rules_Filters(t *testing.T) {
	proc := NewHTTPRouteProcessor()
	ctx := newTestProcessorContext()

	obj := makeHTTPRouteObj("redirect-route", "default", map[string]interface{}{
		"parentRefs": []interface{}{
			map[string]interface{}{"name": "gw"},
		},
		"rules": []interface{}{
			map[string]interface{}{
				"filters": []interface{}{
					map[string]interface{}{
						"type": "RequestRedirect",
						"requestRedirect": map[string]interface{}{
							"scheme":     "https",
							"statusCode": int64(301),
						},
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	rules := result.Values["rules"].([]interface{})
	rule := rules[0].(map[string]interface{})
	filters := rule["filters"].([]interface{})
	if len(filters) != 1 {
		t.Fatalf("Expected 1 filter, got %d", len(filters))
	}
}

// ============================================================
// Test 8: Dependency to Gateway via parentRefs
// ============================================================

func TestHTTPRouteProcessor_Dependency_ToGateway(t *testing.T) {
	proc := NewHTTPRouteProcessor()
	ctx := newTestProcessorContext()

	obj := makeHTTPRouteObj("myapp-route", "default", map[string]interface{}{
		"parentRefs": []interface{}{
			map[string]interface{}{
				"name":      "my-gateway",
				"namespace": "gateway-ns",
			},
		},
		"rules": []interface{}{},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	if len(result.Dependencies) == 0 {
		t.Fatal("Expected at least 1 dependency (Gateway)")
	}

	found := hasDependency(result.Dependencies, "Gateway", "gateway-ns", "my-gateway")
	if !found {
		t.Errorf("Expected dependency to Gateway 'my-gateway' in 'gateway-ns', got: %v", result.Dependencies)
	}
}

// ============================================================
// Test 9: Template content
// ============================================================

func TestHTTPRouteProcessor_Template(t *testing.T) {
	proc := NewHTTPRouteProcessor()
	ctx := newTestProcessorContext()

	obj := makeHTTPRouteObj("myapp-route", "default", map[string]interface{}{
		"parentRefs": []interface{}{
			map[string]interface{}{"name": "gw"},
		},
		"rules": []interface{}{},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	testutil.AssertContains(t, tpl, "apiVersion: gateway.networking.k8s.io/v1", "apiVersion")
	testutil.AssertContains(t, tpl, "kind: HTTPRoute", "kind")
	testutil.AssertContains(t, tpl, "$svc.enabled", "enabled check")
	if !strings.Contains(tpl, "parentRefs") {
		t.Error("Template should reference parentRefs")
	}
}

// ============================================================
// Test 10: ServiceName
// ============================================================

func TestHTTPRouteProcessor_ServiceName(t *testing.T) {
	proc := NewHTTPRouteProcessor()
	ctx := newTestProcessorContext()

	obj := makeHTTPRouteObj("myapp-route", "default", map[string]interface{}{
		"parentRefs": []interface{}{
			map[string]interface{}{"name": "gw"},
		},
		"rules": []interface{}{},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "myapp-route", result.ServiceName, "ServiceName")
}
