package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// ResourceQuota Processor Tests (TDD — tests written first)
// ============================================================

// makeResourceQuotaObj creates an unstructured ResourceQuota for testing.
func makeResourceQuotaObj(name, namespace string, labels map[string]interface{}, spec map[string]interface{}) *unstructured.Unstructured {
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
			"kind":       "ResourceQuota",
			"metadata":   metadata,
			"spec":       spec,
		},
	}
}

// ============================================================
// Constructor tests
// ============================================================

func TestNewResourceQuotaProcessor(t *testing.T) {
	p := NewResourceQuotaProcessor()
	testutil.AssertEqual(t, "resourcequota", p.Name())
	testutil.AssertEqual(t, 80, p.Priority())

	gvks := p.Supports()
	testutil.AssertEqual(t, 1, len(gvks))
	testutil.AssertEqual(t, schema.GroupVersionKind{
		Group: "", Version: "v1", Kind: "ResourceQuota",
	}, gvks[0])
}

// ============================================================
// Subtask 1: Extract hard limits map
// ============================================================

func TestProcessResourceQuota_ExtractsHard(t *testing.T) {
	p := NewResourceQuotaProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"hard": map[string]interface{}{
			"cpu":                    "10",
			"memory":                 "20Gi",
			"pods":                   "50",
			"requests.storage":       "100Gi",
			"persistentvolumeclaims": "20",
		},
	}
	obj := makeResourceQuotaObj("myapp-rq", "default",
		map[string]interface{}{"app": "myapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)

	hard, ok := result.Values["hard"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected hard in values")
	}
	testutil.AssertEqual(t, "10", hard["cpu"])
	testutil.AssertEqual(t, "20Gi", hard["memory"])
	testutil.AssertEqual(t, "50", hard["pods"])
}

// ============================================================
// Subtask 2: Extract scopeSelector
// ============================================================

func TestProcessResourceQuota_ExtractsScopeSelector(t *testing.T) {
	p := NewResourceQuotaProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"hard": map[string]interface{}{
			"pods": "10",
		},
		"scopeSelector": map[string]interface{}{
			"matchExpressions": []interface{}{
				map[string]interface{}{
					"operator":  "In",
					"scopeName": "PriorityClass",
					"values":    []interface{}{"high"},
				},
			},
		},
	}
	obj := makeResourceQuotaObj("scoped-rq", "default",
		map[string]interface{}{"app": "scoped"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	scopeSelector, ok := result.Values["scopeSelector"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected scopeSelector in values")
	}
	matchExprs, ok := scopeSelector["matchExpressions"].([]interface{})
	if !ok {
		t.Fatal("Expected matchExpressions in scopeSelector")
	}
	testutil.AssertEqual(t, 1, len(matchExprs))
}

// ============================================================
// Subtask 3: Extract scopes
// ============================================================

func TestProcessResourceQuota_ExtractsScopes(t *testing.T) {
	p := NewResourceQuotaProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"hard": map[string]interface{}{
			"pods": "5",
		},
		"scopes": []interface{}{
			"BestEffort",
		},
	}
	obj := makeResourceQuotaObj("scopes-rq", "default",
		map[string]interface{}{"app": "scopes"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	scopes, ok := result.Values["scopes"].([]interface{})
	if !ok {
		t.Fatal("Expected scopes in values")
	}
	testutil.AssertEqual(t, 1, len(scopes))
	testutil.AssertEqual(t, "BestEffort", scopes[0])
}

// ============================================================
// Subtask 4: Edge cases
// ============================================================

func TestProcessResourceQuota_EdgeCases(t *testing.T) {
	p := NewResourceQuotaProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilResourceQuota", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil ResourceQuota")
		}
	})

	t.Run("ResourceQuotaWithoutOptionalFields", func(t *testing.T) {
		spec := map[string]interface{}{
			"hard": map[string]interface{}{
				"pods": "10",
			},
		}
		obj := makeResourceQuotaObj("simple-rq", "default",
			map[string]interface{}{"app": "simple"}, spec)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)

		if _, exists := result.Values["scopeSelector"]; exists {
			t.Error("scopeSelector should not be set when not provided")
		}
		if _, exists := result.Values["scopes"]; exists {
			t.Error("scopes should not be set when not provided")
		}
	})

	t.Run("ResourceQuotaNoSpec", func(t *testing.T) {
		obj := makeResourceQuotaObj("nospec-rq", "default", nil, nil)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)
	})
}

// ============================================================
// Result metadata tests
// ============================================================

func TestProcessResourceQuota_ResultMetadata(t *testing.T) {
	p := NewResourceQuotaProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"hard": map[string]interface{}{
			"pods": "20",
		},
	}
	obj := makeResourceQuotaObj("webapp-rq", "default",
		map[string]interface{}{"app": "webapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "webapp", result.ServiceName)
	testutil.AssertEqual(t, "templates/webapp-resourcequota.yaml", result.TemplatePath)
	testutil.AssertEqual(t, "services.webapp.resourceQuota", result.ValuesPath)
	testutil.AssertContains(t, result.TemplateContent, "kind: ResourceQuota")
	testutil.AssertContains(t, result.TemplateContent, "hard")
}

// ============================================================
// Template generation tests
// ============================================================

func TestProcessResourceQuota_GeneratesTemplate(t *testing.T) {
	p := NewResourceQuotaProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"hard": map[string]interface{}{
			"cpu":    "4",
			"memory": "8Gi",
			"pods":   "20",
		},
		"scopes": []interface{}{"NotTerminating"},
	}
	obj := makeResourceQuotaObj("tmplapp-rq", "default",
		map[string]interface{}{"app": "tmplapp"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tmpl := result.TemplateContent
	testutil.AssertContains(t, tmpl, "v1")
	testutil.AssertContains(t, tmpl, "ResourceQuota")
	testutil.AssertContains(t, tmpl, "hard")
	testutil.AssertContains(t, tmpl, "scopes")
	testutil.AssertContains(t, tmpl, "test-chart")
}
