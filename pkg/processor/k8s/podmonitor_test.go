package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create PodMonitor unstructured object
// ============================================================

func makePodMonitorObj(name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "monitoring.coreos.com/v1",
			"kind":       "PodMonitor",
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

func TestPodMonitorProcessor_Name(t *testing.T) {
	proc := NewPodMonitorProcessor()
	testutil.AssertEqual(t, "podmonitor", proc.Name(), "processor name")
}

// ============================================================
// Test 2: Supports GVK
// ============================================================

func TestPodMonitorProcessor_Supports(t *testing.T) {
	proc := NewPodMonitorProcessor()
	gvks := proc.Supports()

	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}

	expected := schema.GroupVersionKind{
		Group:   "monitoring.coreos.com",
		Version: "v1",
		Kind:    "PodMonitor",
	}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// Test 3: PodMetricsEndpoints
// ============================================================

func TestPodMonitorProcessor_PodMetricsEndpoints(t *testing.T) {
	proc := NewPodMonitorProcessor()
	ctx := newTestProcessorContext()

	obj := makePodMonitorObj("myapp-pods", "monitoring", map[string]interface{}{
		"podMetricsEndpoints": []interface{}{
			map[string]interface{}{
				"port":     "metrics",
				"path":     "/metrics",
				"interval": "30s",
			},
		},
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"app": "myapp"},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")

	endpoints, ok := result.Values["podMetricsEndpoints"].([]interface{})
	if !ok {
		t.Fatal("Expected podMetricsEndpoints slice in values")
	}
	if len(endpoints) != 1 {
		t.Fatalf("Expected 1 endpoint, got %d", len(endpoints))
	}

	ep := endpoints[0].(map[string]interface{})
	testutil.AssertEqual(t, "metrics", ep["port"], "port")
	testutil.AssertEqual(t, "/metrics", ep["path"], "path")
	testutil.AssertEqual(t, "30s", ep["interval"], "interval")
}

// ============================================================
// Test 4: JobLabel
// ============================================================

func TestPodMonitorProcessor_JobLabel(t *testing.T) {
	proc := NewPodMonitorProcessor()
	ctx := newTestProcessorContext()

	obj := makePodMonitorObj("myapp-pods", "monitoring", map[string]interface{}{
		"jobLabel": "app.kubernetes.io/name",
		"podMetricsEndpoints": []interface{}{
			map[string]interface{}{"port": "metrics"},
		},
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"app": "myapp"},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "app.kubernetes.io/name", result.Values["jobLabel"], "jobLabel")
}

// ============================================================
// Test 5: Selector
// ============================================================

func TestPodMonitorProcessor_Selector(t *testing.T) {
	proc := NewPodMonitorProcessor()
	ctx := newTestProcessorContext()

	obj := makePodMonitorObj("myapp-pods", "monitoring", map[string]interface{}{
		"podMetricsEndpoints": []interface{}{
			map[string]interface{}{"port": "metrics"},
		},
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"app":       "myapp",
				"component": "worker",
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	selector, ok := result.Values["selector"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected selector map in values")
	}
	matchLabels, ok := selector["matchLabels"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected matchLabels in selector")
	}
	testutil.AssertEqual(t, "myapp", matchLabels["app"], "app label")
}

// ============================================================
// Test 6: Template content
// ============================================================

func TestPodMonitorProcessor_Template(t *testing.T) {
	proc := NewPodMonitorProcessor()
	ctx := newTestProcessorContext()

	obj := makePodMonitorObj("myapp-pods", "monitoring", map[string]interface{}{
		"podMetricsEndpoints": []interface{}{
			map[string]interface{}{"port": "metrics"},
		},
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"app": "myapp"},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	testutil.AssertContains(t, tpl, "apiVersion: monitoring.coreos.com/v1", "apiVersion")
	testutil.AssertContains(t, tpl, "kind: PodMonitor", "kind")
	testutil.AssertContains(t, tpl, "$svc.enabled", "enabled check")
	if !strings.Contains(tpl, "podMetricsEndpoints") {
		t.Error("Template should reference podMetricsEndpoints")
	}
}

// ============================================================
// Test 7: ServiceName
// ============================================================

func TestPodMonitorProcessor_ServiceName(t *testing.T) {
	proc := NewPodMonitorProcessor()
	ctx := newTestProcessorContext()

	obj := makePodMonitorObj("myapp-pods", "monitoring", map[string]interface{}{
		"podMetricsEndpoints": []interface{}{
			map[string]interface{}{"port": "metrics"},
		},
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"app": "myapp"},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "myapp-pods", result.ServiceName, "ServiceName should be metadata.name")
}
