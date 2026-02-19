package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create Gateway unstructured object
// ============================================================

func makeGatewayObj(name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "Gateway",
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

func TestGatewayProcessor_Name(t *testing.T) {
	proc := NewGatewayProcessor()
	testutil.AssertEqual(t, "gateway", proc.Name(), "processor name")
}

// ============================================================
// Test 2: Supports GVK
// ============================================================

func TestGatewayProcessor_Supports(t *testing.T) {
	proc := NewGatewayProcessor()
	gvks := proc.Supports()

	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}

	expected := schema.GroupVersionKind{
		Group:   "gateway.networking.k8s.io",
		Version: "v1",
		Kind:    "Gateway",
	}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// Test 3: GatewayClassName
// ============================================================

func TestGatewayProcessor_GatewayClassName(t *testing.T) {
	proc := NewGatewayProcessor()
	ctx := newTestProcessorContext()

	obj := makeGatewayObj("my-gateway", "default", map[string]interface{}{
		"gatewayClassName": "istio",
		"listeners": []interface{}{
			map[string]interface{}{
				"name":     "http",
				"port":     int64(80),
				"protocol": "HTTP",
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")
	testutil.AssertEqual(t, "istio", result.Values["gatewayClassName"], "gatewayClassName")
}

// ============================================================
// Test 4: HTTP Listener
// ============================================================

func TestGatewayProcessor_Listeners_HTTP(t *testing.T) {
	proc := NewGatewayProcessor()
	ctx := newTestProcessorContext()

	obj := makeGatewayObj("my-gateway", "default", map[string]interface{}{
		"gatewayClassName": "nginx",
		"listeners": []interface{}{
			map[string]interface{}{
				"name":     "http",
				"port":     int64(80),
				"protocol": "HTTP",
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	listeners, ok := result.Values["listeners"].([]interface{})
	if !ok {
		t.Fatal("Expected listeners slice in values")
	}
	if len(listeners) != 1 {
		t.Fatalf("Expected 1 listener, got %d", len(listeners))
	}

	l := listeners[0].(map[string]interface{})
	testutil.AssertEqual(t, "http", l["name"], "listener name")
	testutil.AssertEqual(t, "HTTP", l["protocol"], "listener protocol")
}

// ============================================================
// Test 5: HTTPS Listener with TLS
// ============================================================

func TestGatewayProcessor_Listeners_HTTPS_TLS(t *testing.T) {
	proc := NewGatewayProcessor()
	ctx := newTestProcessorContext()

	obj := makeGatewayObj("my-gateway", "default", map[string]interface{}{
		"gatewayClassName": "nginx",
		"listeners": []interface{}{
			map[string]interface{}{
				"name":     "https",
				"port":     int64(443),
				"protocol": "HTTPS",
				"hostname": "*.example.com",
				"tls": map[string]interface{}{
					"mode": "Terminate",
					"certificateRefs": []interface{}{
						map[string]interface{}{
							"name": "example-cert",
						},
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	listeners := result.Values["listeners"].([]interface{})
	l := listeners[0].(map[string]interface{})
	testutil.AssertEqual(t, "HTTPS", l["protocol"], "protocol")
	testutil.AssertEqual(t, "*.example.com", l["hostname"], "hostname")

	tls, ok := l["tls"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected tls map in listener")
	}
	testutil.AssertEqual(t, "Terminate", tls["mode"], "tls mode")
}

// ============================================================
// Test 6: Multiple listeners
// ============================================================

func TestGatewayProcessor_Listeners_Multi(t *testing.T) {
	proc := NewGatewayProcessor()
	ctx := newTestProcessorContext()

	obj := makeGatewayObj("multi-gw", "default", map[string]interface{}{
		"gatewayClassName": "nginx",
		"listeners": []interface{}{
			map[string]interface{}{
				"name":     "http",
				"port":     int64(80),
				"protocol": "HTTP",
			},
			map[string]interface{}{
				"name":     "https",
				"port":     int64(443),
				"protocol": "HTTPS",
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	listeners := result.Values["listeners"].([]interface{})
	if len(listeners) != 2 {
		t.Fatalf("Expected 2 listeners, got %d", len(listeners))
	}
}

// ============================================================
// Test 7: Template content
// ============================================================

func TestGatewayProcessor_Template(t *testing.T) {
	proc := NewGatewayProcessor()
	ctx := newTestProcessorContext()

	obj := makeGatewayObj("my-gateway", "default", map[string]interface{}{
		"gatewayClassName": "nginx",
		"listeners": []interface{}{
			map[string]interface{}{
				"name":     "http",
				"port":     int64(80),
				"protocol": "HTTP",
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	testutil.AssertContains(t, tpl, "apiVersion: gateway.networking.k8s.io/v1", "apiVersion")
	testutil.AssertContains(t, tpl, "kind: Gateway", "kind")
	testutil.AssertContains(t, tpl, "$svc.enabled", "enabled check")
	if !strings.Contains(tpl, "listeners") {
		t.Error("Template should reference listeners")
	}
}

// ============================================================
// Test 8: ServiceName
// ============================================================

func TestGatewayProcessor_ServiceName(t *testing.T) {
	proc := NewGatewayProcessor()
	ctx := newTestProcessorContext()

	obj := makeGatewayObj("my-gateway", "default", map[string]interface{}{
		"gatewayClassName": "nginx",
		"listeners":        []interface{}{},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "my-gateway", result.ServiceName, "ServiceName")
}
