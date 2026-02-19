package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create ModuleConfig unstructured object
// ============================================================

func makeModuleConfigObj(name string, spec map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "deckhouse.io/v1alpha1",
			"kind":       "ModuleConfig",
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

func TestModuleConfigProcessor_Name(t *testing.T) {
	proc := NewModuleConfigProcessor()
	testutil.AssertEqual(t, "moduleconfig", proc.Name(), "processor name")
}

// ============================================================
// Test 2: Supports GVK
// ============================================================

func TestModuleConfigProcessor_Supports(t *testing.T) {
	proc := NewModuleConfigProcessor()
	gvks := proc.Supports()

	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}

	expected := schema.GroupVersionKind{
		Group:   "deckhouse.io",
		Version: "v1alpha1",
		Kind:    "ModuleConfig",
	}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// Test 3: spec.enabled = true
// ============================================================

func TestModuleConfigProcessor_Enabled_True(t *testing.T) {
	proc := NewModuleConfigProcessor()
	ctx := newTestProcessorContext()

	obj := makeModuleConfigObj("test-module", map[string]interface{}{
		"enabled": true,
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")
	testutil.AssertEqual(t, true, result.Values["enabled"], "enabled should be true")
}

// ============================================================
// Test 4: spec.enabled = false
// ============================================================

func TestModuleConfigProcessor_Enabled_False(t *testing.T) {
	proc := NewModuleConfigProcessor()
	ctx := newTestProcessorContext()

	obj := makeModuleConfigObj("test-module", map[string]interface{}{
		"enabled": false,
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, false, result.Values["enabled"], "enabled should be false")
}

// ============================================================
// Test 5: spec.version
// ============================================================

func TestModuleConfigProcessor_Version(t *testing.T) {
	proc := NewModuleConfigProcessor()
	ctx := newTestProcessorContext()

	obj := makeModuleConfigObj("test-module", map[string]interface{}{
		"enabled": true,
		"version": int64(1),
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, int64(1), result.Values["version"], "version should be 1")
}

// ============================================================
// Test 6: spec.settings flat
// ============================================================

func TestModuleConfigProcessor_Settings_Flat(t *testing.T) {
	proc := NewModuleConfigProcessor()
	ctx := newTestProcessorContext()

	obj := makeModuleConfigObj("test-module", map[string]interface{}{
		"enabled": true,
		"settings": map[string]interface{}{
			"logLevel": "debug",
			"port":     int64(8080),
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	settings, ok := result.Values["settings"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected settings map in values")
	}
	testutil.AssertEqual(t, "debug", settings["logLevel"], "logLevel")
	testutil.AssertEqual(t, int64(8080), settings["port"], "port")
}

// ============================================================
// Test 7: spec.settings nested
// ============================================================

func TestModuleConfigProcessor_Settings_Nested(t *testing.T) {
	proc := NewModuleConfigProcessor()
	ctx := newTestProcessorContext()

	obj := makeModuleConfigObj("test-module", map[string]interface{}{
		"enabled": true,
		"settings": map[string]interface{}{
			"auth": map[string]interface{}{
				"type":     "dex",
				"provider": "github",
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	settings, ok := result.Values["settings"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected settings map in values")
	}

	auth, ok := settings["auth"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected auth map in settings")
	}
	testutil.AssertEqual(t, "dex", auth["type"], "auth type")
	testutil.AssertEqual(t, "github", auth["provider"], "auth provider")
}

// ============================================================
// Test 8: spec.settings nil/empty — no panic
// ============================================================

func TestModuleConfigProcessor_Settings_Empty(t *testing.T) {
	proc := NewModuleConfigProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilSettings", func(t *testing.T) {
		obj := makeModuleConfigObj("test-module", map[string]interface{}{
			"enabled": true,
		})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed, "should be processed")

		// No settings key or empty settings
		if settings, ok := result.Values["settings"]; ok {
			if settingsMap, ok := settings.(map[string]interface{}); ok && len(settingsMap) > 0 {
				t.Error("Expected empty or no settings for nil spec.settings")
			}
		}
	})

	t.Run("EmptySettings", func(t *testing.T) {
		obj := makeModuleConfigObj("test-module", map[string]interface{}{
			"enabled":  true,
			"settings": map[string]interface{}{},
		})

		result, err := proc.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed, "should be processed")
	})
}

// ============================================================
// Test 9: Template contains {{ if .Values.enabled }}
// ============================================================

func TestModuleConfigProcessor_Template(t *testing.T) {
	proc := NewModuleConfigProcessor()
	ctx := newTestProcessorContext()

	obj := makeModuleConfigObj("test-module", map[string]interface{}{
		"enabled": true,
		"settings": map[string]interface{}{
			"logLevel": "info",
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	if tpl == "" {
		t.Fatal("Expected non-empty template content")
	}

	testutil.AssertContains(t, tpl, "apiVersion: deckhouse.io/v1alpha1", "template should have apiVersion")
	testutil.AssertContains(t, tpl, "kind: ModuleConfig", "template should have kind")
	testutil.AssertContains(t, tpl, "$svc.enabled", "template should contain enabled condition")
}

// ============================================================
// Test 10: ServiceName = metadata.name
// ============================================================

func TestModuleConfigProcessor_ServiceName(t *testing.T) {
	proc := NewModuleConfigProcessor()
	ctx := newTestProcessorContext()

	obj := makeModuleConfigObj("ingress-nginx", map[string]interface{}{
		"enabled": true,
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "ingress-nginx", result.ServiceName, "ServiceName should be metadata.name")
}
