package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create ServiceMonitor unstructured object
// ============================================================

func makeServiceMonitorObj(name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "monitoring.coreos.com/v1",
			"kind":       "ServiceMonitor",
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

func TestServiceMonitorProcessor_Name(t *testing.T) {
	proc := NewServiceMonitorProcessor()
	testutil.AssertEqual(t, "servicemonitor", proc.Name(), "processor name")
}

// ============================================================
// Test 2: Supports GVK
// ============================================================

func TestServiceMonitorProcessor_Supports(t *testing.T) {
	proc := NewServiceMonitorProcessor()
	gvks := proc.Supports()

	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}

	expected := schema.GroupVersionKind{
		Group:   "monitoring.coreos.com",
		Version: "v1",
		Kind:    "ServiceMonitor",
	}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// Test 3: Single endpoint extraction
// ============================================================

func TestServiceMonitorProcessor_Endpoints_Single(t *testing.T) {
	proc := NewServiceMonitorProcessor()
	ctx := newTestProcessorContext()

	obj := makeServiceMonitorObj("myapp-metrics", "monitoring", map[string]interface{}{
		"endpoints": []interface{}{
			map[string]interface{}{
				"port":     "metrics",
				"path":     "/metrics",
				"interval": "30s",
			},
		},
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"app": "myapp",
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")

	endpoints, ok := result.Values["endpoints"].([]interface{})
	if !ok {
		t.Fatal("Expected endpoints slice in values")
	}
	if len(endpoints) != 1 {
		t.Fatalf("Expected 1 endpoint, got %d", len(endpoints))
	}
}

// ============================================================
// Test 4: Multiple endpoints
// ============================================================

func TestServiceMonitorProcessor_Endpoints_Multi(t *testing.T) {
	proc := NewServiceMonitorProcessor()
	ctx := newTestProcessorContext()

	obj := makeServiceMonitorObj("myapp-metrics", "monitoring", map[string]interface{}{
		"endpoints": []interface{}{
			map[string]interface{}{
				"port":     "metrics",
				"path":     "/metrics",
				"interval": "15s",
			},
			map[string]interface{}{
				"port":     "admin-metrics",
				"path":     "/admin/metrics",
				"interval": "60s",
			},
		},
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"app": "myapp",
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	endpoints, ok := result.Values["endpoints"].([]interface{})
	if !ok {
		t.Fatal("Expected endpoints slice in values")
	}
	if len(endpoints) != 2 {
		t.Fatalf("Expected 2 endpoints, got %d", len(endpoints))
	}
}

// ============================================================
// Test 5: NamespaceSelector
// ============================================================

func TestServiceMonitorProcessor_NamespaceSelector(t *testing.T) {
	proc := NewServiceMonitorProcessor()
	ctx := newTestProcessorContext()

	obj := makeServiceMonitorObj("cross-ns-monitor", "monitoring", map[string]interface{}{
		"endpoints": []interface{}{
			map[string]interface{}{"port": "metrics"},
		},
		"namespaceSelector": map[string]interface{}{
			"matchNames": []interface{}{"app-ns", "backend-ns"},
		},
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"app": "myapp"},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	nsSelector, ok := result.Values["namespaceSelector"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected namespaceSelector map in values")
	}
	matchNames, ok := nsSelector["matchNames"].([]interface{})
	if !ok {
		t.Fatal("Expected matchNames in namespaceSelector")
	}
	if len(matchNames) != 2 {
		t.Fatalf("Expected 2 matchNames, got %d", len(matchNames))
	}
}

// ============================================================
// Test 6: Selector matchLabels
// ============================================================

func TestServiceMonitorProcessor_Selector(t *testing.T) {
	proc := NewServiceMonitorProcessor()
	ctx := newTestProcessorContext()

	obj := makeServiceMonitorObj("myapp-metrics", "monitoring", map[string]interface{}{
		"endpoints": []interface{}{
			map[string]interface{}{"port": "metrics"},
		},
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"app":       "myapp",
				"component": "backend",
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
	testutil.AssertEqual(t, "backend", matchLabels["component"], "component label")
}

// ============================================================
// Test 7: Interval extraction
// ============================================================

func TestServiceMonitorProcessor_Interval(t *testing.T) {
	proc := NewServiceMonitorProcessor()
	ctx := newTestProcessorContext()

	obj := makeServiceMonitorObj("myapp-metrics", "monitoring", map[string]interface{}{
		"endpoints": []interface{}{
			map[string]interface{}{
				"port":     "metrics",
				"interval": "15s",
			},
		},
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"app": "myapp"},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	endpoints := result.Values["endpoints"].([]interface{})
	ep := endpoints[0].(map[string]interface{})
	testutil.AssertEqual(t, "15s", ep["interval"], "endpoint interval")
}

// ============================================================
// Test 8: ScrapeTimeout
// ============================================================

func TestServiceMonitorProcessor_ScrapeTimeout(t *testing.T) {
	proc := NewServiceMonitorProcessor()
	ctx := newTestProcessorContext()

	obj := makeServiceMonitorObj("myapp-metrics", "monitoring", map[string]interface{}{
		"endpoints": []interface{}{
			map[string]interface{}{
				"port":          "metrics",
				"interval":      "30s",
				"scrapeTimeout": "10s",
			},
		},
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"app": "myapp"},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	endpoints := result.Values["endpoints"].([]interface{})
	ep := endpoints[0].(map[string]interface{})
	testutil.AssertEqual(t, "10s", ep["scrapeTimeout"], "scrapeTimeout")
}

// ============================================================
// Test 9: Dependency to Service via selector
// ============================================================

func TestServiceMonitorProcessor_Dependency_ToService(t *testing.T) {
	proc := NewServiceMonitorProcessor()
	ctx := newTestProcessorContext()

	obj := makeServiceMonitorObj("myapp-metrics", "monitoring", map[string]interface{}{
		"endpoints": []interface{}{
			map[string]interface{}{"port": "metrics"},
		},
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"app": "myapp",
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	// ServiceMonitor depends on matching Service
	if len(result.Dependencies) == 0 {
		t.Fatal("Expected at least 1 dependency (Service)")
	}

	found := hasDependency(result.Dependencies, "Service", "monitoring", "myapp-metrics")
	if !found {
		t.Errorf("Expected dependency to Service 'myapp-metrics', got: %v", result.Dependencies)
	}
}

// ============================================================
// Test 10: Template content
// ============================================================

func TestServiceMonitorProcessor_Template(t *testing.T) {
	proc := NewServiceMonitorProcessor()
	ctx := newTestProcessorContext()

	obj := makeServiceMonitorObj("myapp-metrics", "monitoring", map[string]interface{}{
		"endpoints": []interface{}{
			map[string]interface{}{
				"port": "metrics",
				"path": "/metrics",
			},
		},
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{"app": "myapp"},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	if tpl == "" {
		t.Fatal("Expected non-empty template")
	}

	testutil.AssertContains(t, tpl, "apiVersion: monitoring.coreos.com/v1", "apiVersion")
	testutil.AssertContains(t, tpl, "kind: ServiceMonitor", "kind")
	testutil.AssertContains(t, tpl, "$svc.enabled", "enabled check")
	if !strings.Contains(tpl, "endpoints") {
		t.Error("Template should reference endpoints")
	}
}
