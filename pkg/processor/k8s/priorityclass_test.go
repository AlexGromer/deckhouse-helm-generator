package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// PriorityClass Processor Tests (TDD — tests written first)
// ============================================================

// makePriorityClassObj creates an unstructured PriorityClass for testing.
func makePriorityClassObj(name string, labels map[string]interface{}, extra map[string]interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name": name,
	}
	if labels != nil {
		metadata["labels"] = labels
	}
	obj := map[string]interface{}{
		"apiVersion": "scheduling.k8s.io/v1",
		"kind":       "PriorityClass",
		"metadata":   metadata,
	}
	for k, v := range extra {
		obj[k] = v
	}
	return &unstructured.Unstructured{Object: obj}
}

// ============================================================
// Constructor tests
// ============================================================

func TestNewPriorityClassProcessor(t *testing.T) {
	p := NewPriorityClassProcessor()
	testutil.AssertEqual(t, "priorityclass", p.Name())
	testutil.AssertEqual(t, 80, p.Priority())

	gvks := p.Supports()
	testutil.AssertEqual(t, 1, len(gvks))
	testutil.AssertEqual(t, schema.GroupVersionKind{
		Group: "scheduling.k8s.io", Version: "v1", Kind: "PriorityClass",
	}, gvks[0])
}

// ============================================================
// Subtask 1: Extract value (int32)
// ============================================================

func TestProcessPriorityClass_ExtractsValue(t *testing.T) {
	p := NewPriorityClassProcessor()
	ctx := newTestProcessorContext()

	obj := makePriorityClassObj("high-priority", nil, map[string]interface{}{
		"value": int64(1000000),
	})

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)
	testutil.AssertEqual(t, int64(1000000), result.Values["value"])
}

// ============================================================
// Subtask 2: Extract globalDefault (bool)
// ============================================================

func TestProcessPriorityClass_ExtractsGlobalDefault(t *testing.T) {
	p := NewPriorityClassProcessor()
	ctx := newTestProcessorContext()

	obj := makePriorityClassObj("default-priority", nil, map[string]interface{}{
		"value":         int64(0),
		"globalDefault": true,
	})

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Values["globalDefault"])
}

// ============================================================
// Subtask 3: Extract preemptionPolicy
// ============================================================

func TestProcessPriorityClass_ExtractsPreemptionPolicy(t *testing.T) {
	p := NewPriorityClassProcessor()
	ctx := newTestProcessorContext()

	obj := makePriorityClassObj("non-preempting", nil, map[string]interface{}{
		"value":            int64(500000),
		"preemptionPolicy": "Never",
	})

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "Never", result.Values["preemptionPolicy"])
}

// ============================================================
// Subtask 4: Extract description
// ============================================================

func TestProcessPriorityClass_ExtractsDescription(t *testing.T) {
	p := NewPriorityClassProcessor()
	ctx := newTestProcessorContext()

	obj := makePriorityClassObj("described-priority", nil, map[string]interface{}{
		"value":       int64(100),
		"description": "Used for critical workloads",
	})

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "Used for critical workloads", result.Values["description"])
}

// ============================================================
// Subtask 5: Edge cases
// ============================================================

func TestProcessPriorityClass_EdgeCases(t *testing.T) {
	p := NewPriorityClassProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilPriorityClass", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil PriorityClass")
		}
	})

	t.Run("PriorityClassWithOnlyValue", func(t *testing.T) {
		obj := makePriorityClassObj("simple-priority", nil, map[string]interface{}{
			"value": int64(200),
		})

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)
		testutil.AssertEqual(t, int64(200), result.Values["value"])

		if _, exists := result.Values["description"]; exists {
			t.Error("description should not be set when not provided")
		}
		if _, exists := result.Values["preemptionPolicy"]; exists {
			t.Error("preemptionPolicy should not be set when not provided")
		}
	})
}

// ============================================================
// Result metadata tests (cluster-scoped)
// ============================================================

func TestProcessPriorityClass_ResultMetadata(t *testing.T) {
	p := NewPriorityClassProcessor()
	ctx := newTestProcessorContext()

	obj := makePriorityClassObj("high-priority", nil, map[string]interface{}{
		"value": int64(1000000),
	})

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	// Cluster-scoped: ServiceName is sanitized (hyphens → camelCase)
	testutil.AssertEqual(t, "highPriority", result.ServiceName)
	testutil.AssertEqual(t, "templates/priorityclass-highPriority.yaml", result.TemplatePath)
	testutil.AssertEqual(t, "priorityClasses.highPriority", result.ValuesPath)
	testutil.AssertContains(t, result.TemplateContent, "kind: PriorityClass")
	testutil.AssertContains(t, result.TemplateContent, "value:")
}

// ============================================================
// Template generation tests (cluster-scoped, no service wrapper)
// ============================================================

func TestProcessPriorityClass_GeneratesTemplate(t *testing.T) {
	p := NewPriorityClassProcessor()
	ctx := newTestProcessorContext()

	obj := makePriorityClassObj("my-priority", nil, map[string]interface{}{
		"value":            int64(999999),
		"globalDefault":    false,
		"preemptionPolicy": "PreemptLowerPriority",
		"description":      "Test priority class",
	})

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tmpl := result.TemplateContent
	testutil.AssertContains(t, tmpl, "scheduling.k8s.io/v1")
	testutil.AssertContains(t, tmpl, "PriorityClass")
	testutil.AssertContains(t, tmpl, "value:")
	testutil.AssertContains(t, tmpl, "globalDefault")
	testutil.AssertContains(t, tmpl, "preemptionPolicy")
	testutil.AssertContains(t, tmpl, "test-chart")
	// Cluster-scoped: should NOT have services wrapper
	if strings.Contains(tmpl, "$svc := .Values.services") {
		t.Error("PriorityClass template should NOT have service wrapper (cluster-scoped)")
	}
	testutil.AssertContains(t, tmpl, "priorityClasses")
}
