package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create ConfigMap with/without grafana_dashboard label
// ============================================================

func makeGrafanaDashboardObj(name, namespace string, labels map[string]interface{}, data map[string]interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}
	if labels != nil {
		metadata["labels"] = labels
	}
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   metadata,
		},
	}
	if data != nil {
		obj.Object["data"] = data
	}
	return obj
}

// ============================================================
// Test 1: Processor name
// ============================================================

func TestGrafanaDashboardProcessor_Name(t *testing.T) {
	proc := NewGrafanaDashboardProcessor()
	testutil.AssertEqual(t, "grafanadashboard", proc.Name(), "processor name")
}

// ============================================================
// Test 2: Supports ConfigMap GVK (with label filtering in Process)
// ============================================================

func TestGrafanaDashboardProcessor_Supports_ConfigMapWithLabel(t *testing.T) {
	proc := NewGrafanaDashboardProcessor()
	gvks := proc.Supports()

	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}

	expected := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	}
	testutil.AssertEqual(t, expected, gvks[0], "should support ConfigMap GVK")

	// Verify it actually processes a ConfigMap with the label
	ctx := newTestProcessorContext()
	obj := makeGrafanaDashboardObj("my-dashboard", "monitoring",
		map[string]interface{}{"grafana_dashboard": "1"},
		map[string]interface{}{"dashboard.json": `{"title": "Test"}`},
	)

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should process ConfigMap with grafana_dashboard label")
}

// ============================================================
// Test 3: Does NOT process ConfigMap without grafana_dashboard label
// ============================================================

func TestGrafanaDashboardProcessor_NotSupports_ConfigMapWithoutLabel(t *testing.T) {
	proc := NewGrafanaDashboardProcessor()
	ctx := newTestProcessorContext()

	obj := makeGrafanaDashboardObj("regular-config", "default",
		map[string]interface{}{"app": "myapp"},
		map[string]interface{}{"config.yaml": "key: value"},
	)

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	// Should return non-processed result to let ConfigMapProcessor handle it
	if result != nil && result.Processed {
		t.Error("Should NOT process ConfigMap without grafana_dashboard label")
	}
}

// ============================================================
// Test 4: Dashboard JSON in values
// ============================================================

func TestGrafanaDashboardProcessor_Dashboard_Values(t *testing.T) {
	proc := NewGrafanaDashboardProcessor()
	ctx := newTestProcessorContext()

	dashboardJSON := `{"title":"My Dashboard","panels":[{"type":"graph"}]}`
	obj := makeGrafanaDashboardObj("my-dashboard", "monitoring",
		map[string]interface{}{"grafana_dashboard": "1"},
		map[string]interface{}{"dashboard.json": dashboardJSON},
	)

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")

	// Dashboard data should be in values
	dashboards, ok := result.Values["dashboards"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected dashboards map in values")
	}
	if _, ok := dashboards["dashboard.json"]; !ok {
		t.Error("Expected dashboard.json key in dashboards map")
	}
}

// ============================================================
// Test 5: Dashboard file path
// ============================================================

func TestGrafanaDashboardProcessor_Dashboard_FilePath(t *testing.T) {
	proc := NewGrafanaDashboardProcessor()
	ctx := newTestProcessorContext()

	obj := makeGrafanaDashboardObj("my-dashboard", "monitoring",
		map[string]interface{}{"grafana_dashboard": "1"},
		map[string]interface{}{"dashboard.json": `{"title": "Test"}`},
	)

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	// Template path should include grafana-dashboard prefix
	if !strings.Contains(result.TemplatePath, "grafana-dashboard") {
		t.Errorf("Expected template path to contain 'grafana-dashboard', got: %s", result.TemplatePath)
	}
}

// ============================================================
// Test 6: Template content
// ============================================================

func TestGrafanaDashboardProcessor_Template(t *testing.T) {
	proc := NewGrafanaDashboardProcessor()
	ctx := newTestProcessorContext()

	obj := makeGrafanaDashboardObj("my-dashboard", "monitoring",
		map[string]interface{}{"grafana_dashboard": "1"},
		map[string]interface{}{"dashboard.json": `{"title": "Test"}`},
	)

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	testutil.AssertContains(t, tpl, "kind: ConfigMap", "kind")
	testutil.AssertContains(t, tpl, "grafana_dashboard", "grafana label")
	testutil.AssertContains(t, tpl, "$svc.enabled", "enabled check")
}
