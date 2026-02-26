package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// LimitRange Processor Tests (TDD — tests written first)
// ============================================================

// makeLimitRangeObj creates an unstructured LimitRange for testing.
func makeLimitRangeObj(name, namespace string, labels map[string]interface{}, spec map[string]interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}
	if labels != nil {
		metadata["labels"] = labels
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "LimitRange",
			"metadata":   metadata,
			"spec":       spec,
		},
	}
}

// ============================================================
// Constructor tests
// ============================================================

func TestNewLimitRangeProcessor(t *testing.T) {
	p := NewLimitRangeProcessor()
	testutil.AssertEqual(t, "limitrange", p.Name())
	testutil.AssertEqual(t, 80, p.Priority())

	gvks := p.Supports()
	testutil.AssertEqual(t, 1, len(gvks))
	testutil.AssertEqual(t, schema.GroupVersionKind{
		Group: "", Version: "v1", Kind: "LimitRange",
	}, gvks[0])
}

// ============================================================
// Subtask 1: Extract limits array
// ============================================================

func TestProcessLimitRange_ExtractsLimits(t *testing.T) {
	p := NewLimitRangeProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"limits": []interface{}{
			map[string]interface{}{
				"type": "Container",
				"default": map[string]interface{}{
					"cpu":    "500m",
					"memory": "128Mi",
				},
				"defaultRequest": map[string]interface{}{
					"cpu":    "100m",
					"memory": "64Mi",
				},
			},
		},
	}
	obj := makeLimitRangeObj("myapp-lr", "default",
		map[string]interface{}{"app": "myapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)

	limits, ok := result.Values["limits"].([]interface{})
	if !ok {
		t.Fatal("Expected limits in values")
	}
	testutil.AssertEqual(t, 1, len(limits))

	limit := limits[0].(map[string]interface{})
	testutil.AssertEqual(t, "Container", limit["type"])
}

// ============================================================
// Subtask 2: Multiple limit types
// ============================================================

func TestProcessLimitRange_ExtractsMultipleLimitTypes(t *testing.T) {
	p := NewLimitRangeProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"limits": []interface{}{
			map[string]interface{}{
				"type": "Container",
				"default": map[string]interface{}{
					"cpu": "200m",
				},
			},
			map[string]interface{}{
				"type": "Pod",
				"max": map[string]interface{}{
					"cpu":    "2",
					"memory": "2Gi",
				},
			},
			map[string]interface{}{
				"type": "PersistentVolumeClaim",
				"max": map[string]interface{}{
					"storage": "10Gi",
				},
				"min": map[string]interface{}{
					"storage": "1Gi",
				},
			},
		},
	}
	obj := makeLimitRangeObj("multi-lr", "default",
		map[string]interface{}{"app": "multi"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	limits, ok := result.Values["limits"].([]interface{})
	if !ok {
		t.Fatal("Expected limits in values")
	}
	testutil.AssertEqual(t, 3, len(limits))
}

// ============================================================
// Subtask 3: Edge cases
// ============================================================

func TestProcessLimitRange_EdgeCases(t *testing.T) {
	p := NewLimitRangeProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilLimitRange", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil LimitRange")
		}
	})

	t.Run("LimitRangeEmptyLimits", func(t *testing.T) {
		spec := map[string]interface{}{
			"limits": []interface{}{},
		}
		obj := makeLimitRangeObj("empty-lr", "default", nil, spec)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)
		// Empty limits — values may be empty or have empty slice
	})

	t.Run("LimitRangeNoSpec", func(t *testing.T) {
		obj := makeLimitRangeObj("nospec-lr", "default", nil, nil)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)
	})
}

// ============================================================
// Result metadata tests
// ============================================================

func TestProcessLimitRange_ResultMetadata(t *testing.T) {
	p := NewLimitRangeProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"limits": []interface{}{
			map[string]interface{}{
				"type": "Container",
			},
		},
	}
	obj := makeLimitRangeObj("webapp-lr", "default",
		map[string]interface{}{"app": "webapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "webapp", result.ServiceName)
	testutil.AssertEqual(t, "templates/webapp-limitrange.yaml", result.TemplatePath)
	testutil.AssertEqual(t, "services.webapp.limitRange", result.ValuesPath)
	testutil.AssertContains(t, result.TemplateContent, "kind: LimitRange")
	testutil.AssertContains(t, result.TemplateContent, "limits")
}

// ============================================================
// Template generation tests
// ============================================================

func TestProcessLimitRange_GeneratesTemplate(t *testing.T) {
	p := NewLimitRangeProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"limits": []interface{}{
			map[string]interface{}{
				"type": "Container",
				"default": map[string]interface{}{
					"cpu":    "500m",
					"memory": "128Mi",
				},
			},
		},
	}
	obj := makeLimitRangeObj("tmplapp-lr", "default",
		map[string]interface{}{"app": "tmplapp"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tmpl := result.TemplateContent
	testutil.AssertContains(t, tmpl, "v1")
	testutil.AssertContains(t, tmpl, "LimitRange")
	testutil.AssertContains(t, tmpl, "limits")
	testutil.AssertContains(t, tmpl, "test-chart")
}
