package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create IngressNginxController unstructured object
// ============================================================

func makeIngressNginxControllerObj(name string, spec map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "deckhouse.io/v1",
			"kind":       "IngressNginxController",
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}
	if spec != nil {
		obj.Object["spec"] = spec
	}
	return obj
}

// ============================================================
// Test 1: Processor name
// ============================================================

func TestIngressNginxControllerProcessor_Name(t *testing.T) {
	proc := NewIngressNginxControllerProcessor()
	testutil.AssertEqual(t, "ingressnginxcontroller", proc.Name(), "processor name")
}

// ============================================================
// Test 2: Supports GVK
// ============================================================

func TestIngressNginxControllerProcessor_Supports(t *testing.T) {
	proc := NewIngressNginxControllerProcessor()
	gvks := proc.Supports()

	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}

	expected := schema.GroupVersionKind{
		Group:   "deckhouse.io",
		Version: "v1",
		Kind:    "IngressNginxController",
	}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// Test 3: spec.ingressClass
// ============================================================

func TestIngressNginxControllerProcessor_IngressClass(t *testing.T) {
	proc := NewIngressNginxControllerProcessor()
	ctx := newTestProcessorContext()

	obj := makeIngressNginxControllerObj("main", map[string]interface{}{
		"ingressClass": "nginx",
		"inlet":        "LoadBalancer",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")
	testutil.AssertEqual(t, "nginx", result.Values["ingressClass"], "ingressClass")
}

// ============================================================
// Test 4: spec.inlet = LoadBalancer
// ============================================================

func TestIngressNginxControllerProcessor_Inlet_LoadBalancer(t *testing.T) {
	proc := NewIngressNginxControllerProcessor()
	ctx := newTestProcessorContext()

	obj := makeIngressNginxControllerObj("main", map[string]interface{}{
		"ingressClass": "nginx",
		"inlet":        "LoadBalancer",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "LoadBalancer", result.Values["inlet"], "inlet should be LoadBalancer")
}

// ============================================================
// Test 5: spec.inlet = HostPort
// ============================================================

func TestIngressNginxControllerProcessor_Inlet_HostPort(t *testing.T) {
	proc := NewIngressNginxControllerProcessor()
	ctx := newTestProcessorContext()

	obj := makeIngressNginxControllerObj("main", map[string]interface{}{
		"ingressClass": "nginx",
		"inlet":        "HostPort",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "HostPort", result.Values["inlet"], "inlet should be HostPort")
}

// ============================================================
// Test 6: spec.inlet = HostWithFailover
// ============================================================

func TestIngressNginxControllerProcessor_Inlet_HostWithFailover(t *testing.T) {
	proc := NewIngressNginxControllerProcessor()
	ctx := newTestProcessorContext()

	obj := makeIngressNginxControllerObj("main", map[string]interface{}{
		"ingressClass": "nginx",
		"inlet":        "HostWithFailover",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "HostWithFailover", result.Values["inlet"], "inlet should be HostWithFailover")
}

// ============================================================
// Test 7: spec.controllerVersion
// ============================================================

func TestIngressNginxControllerProcessor_ControllerVersion(t *testing.T) {
	proc := NewIngressNginxControllerProcessor()
	ctx := newTestProcessorContext()

	obj := makeIngressNginxControllerObj("main", map[string]interface{}{
		"ingressClass":      "nginx",
		"inlet":             "LoadBalancer",
		"controllerVersion": "1.6",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "1.6", result.Values["controllerVersion"], "controllerVersion")
}

// ============================================================
// Test 8: spec.resourcesRequests
// ============================================================

func TestIngressNginxControllerProcessor_ResourcesRequests(t *testing.T) {
	proc := NewIngressNginxControllerProcessor()
	ctx := newTestProcessorContext()

	obj := makeIngressNginxControllerObj("main", map[string]interface{}{
		"ingressClass": "nginx",
		"inlet":        "LoadBalancer",
		"resourcesRequests": map[string]interface{}{
			"mode": "VPA",
			"vpa": map[string]interface{}{
				"mode": "Auto",
				"cpu": map[string]interface{}{
					"max": "100m",
				},
				"memory": map[string]interface{}{
					"max": "200Mi",
				},
			},
			"static": map[string]interface{}{
				"cpu":    "350m",
				"memory": "500Mi",
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	rr, ok := result.Values["resourcesRequests"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected resourcesRequests map in values")
	}
	testutil.AssertEqual(t, "VPA", rr["mode"], "resourcesRequests mode")

	vpa, ok := rr["vpa"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected vpa map in resourcesRequests")
	}
	testutil.AssertEqual(t, "Auto", vpa["mode"], "vpa mode")
}

// ============================================================
// Test 9: Template content
// ============================================================

func TestIngressNginxControllerProcessor_Template(t *testing.T) {
	proc := NewIngressNginxControllerProcessor()
	ctx := newTestProcessorContext()

	obj := makeIngressNginxControllerObj("main", map[string]interface{}{
		"ingressClass": "nginx",
		"inlet":        "LoadBalancer",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	if tpl == "" {
		t.Fatal("Expected non-empty template content")
	}

	testutil.AssertContains(t, tpl, "apiVersion: deckhouse.io/v1", "template should have apiVersion")
	testutil.AssertContains(t, tpl, "kind: IngressNginxController", "template should have kind")
	testutil.AssertContains(t, tpl, ".inlet", "template should reference inlet")
	testutil.AssertContains(t, tpl, ".ingressClass", "template should reference ingressClass")
}

// ============================================================
// Test 10: ServiceName = metadata.name
// ============================================================

func TestIngressNginxControllerProcessor_ServiceName(t *testing.T) {
	proc := NewIngressNginxControllerProcessor()
	ctx := newTestProcessorContext()

	obj := makeIngressNginxControllerObj("main-ingress", map[string]interface{}{
		"ingressClass": "nginx",
		"inlet":        "LoadBalancer",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "main-ingress", result.ServiceName, "ServiceName should be metadata.name")
}
