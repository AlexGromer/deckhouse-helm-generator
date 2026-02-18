package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Task 4.2: PDB Processor Tests (TDD)
// ============================================================

func makePDBObj(name, namespace string, labels map[string]interface{}, spec map[string]interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}
	if labels != nil {
		metadata["labels"] = labels
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy/v1",
			"kind":       "PodDisruptionBudget",
			"metadata":   metadata,
			"spec":       spec,
		},
	}
}

// ============================================================
// Subtask 1: Extract minAvailable (integer)
// ============================================================

func TestProcessPDB_ExtractsMinAvailableInt(t *testing.T) {
	p := NewPDBProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"minAvailable": int64(2),
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"app": "myapp",
			},
		},
	}
	obj := makePDBObj("myapp-pdb", "default",
		map[string]interface{}{"app": "myapp"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)
	testutil.AssertEqual(t, int64(2), result.Values["minAvailable"])
}

// ============================================================
// Subtask 2: Extract minAvailable (percentage)
// ============================================================

func TestProcessPDB_ExtractsMinAvailablePercent(t *testing.T) {
	p := NewPDBProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"minAvailable": "50%",
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"app": "percentapp",
			},
		},
	}
	obj := makePDBObj("percentapp-pdb", "default",
		map[string]interface{}{"app": "percentapp"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "50%", result.Values["minAvailable"])
}

// ============================================================
// Subtask 3: Extract maxUnavailable (integer)
// ============================================================

func TestProcessPDB_ExtractsMaxUnavailableInt(t *testing.T) {
	p := NewPDBProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"maxUnavailable": int64(1),
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"app": "maxapp",
			},
		},
	}
	obj := makePDBObj("maxapp-pdb", "default",
		map[string]interface{}{"app": "maxapp"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, int64(1), result.Values["maxUnavailable"])
}

// ============================================================
// Subtask 4: Extract maxUnavailable (percentage)
// ============================================================

func TestProcessPDB_ExtractsMaxUnavailablePercent(t *testing.T) {
	p := NewPDBProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"maxUnavailable": "25%",
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"app": "pctapp",
			},
		},
	}
	obj := makePDBObj("pctapp-pdb", "default",
		map[string]interface{}{"app": "pctapp"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "25%", result.Values["maxUnavailable"])
}

// ============================================================
// Subtask 5: Extract selector
// ============================================================

func TestProcessPDB_ExtractsSelector(t *testing.T) {
	p := NewPDBProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"minAvailable": int64(1),
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"app":     "myapp",
				"version": "v2",
			},
		},
	}
	obj := makePDBObj("myapp-pdb", "default",
		map[string]interface{}{"app": "myapp"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	selector, ok := result.Values["selector"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected selector in values")
	}
	matchLabels, ok := selector["matchLabels"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected matchLabels in selector")
	}
	testutil.AssertEqual(t, "myapp", matchLabels["app"])
	testutil.AssertEqual(t, "v2", matchLabels["version"])
}

// ============================================================
// Subtask 6: Extract unhealthyPodEvictionPolicy
// ============================================================

func TestProcessPDB_ExtractsUnhealthyPodEvictionPolicy(t *testing.T) {
	p := NewPDBProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"minAvailable": int64(1),
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"app": "evictapp",
			},
		},
		"unhealthyPodEvictionPolicy": "AlwaysAllow",
	}
	obj := makePDBObj("evictapp-pdb", "default",
		map[string]interface{}{"app": "evictapp"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "AlwaysAllow", result.Values["unhealthyPodEvictionPolicy"])
}

// ============================================================
// Subtask 7: Edge cases
// ============================================================

func TestProcessPDB_EdgeCases(t *testing.T) {
	p := NewPDBProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilPDB", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil PDB")
		}
	})

	t.Run("BothMinAndMax", func(t *testing.T) {
		// PDB can have both minAvailable and maxUnavailable
		// (though K8s rejects this, we should extract what's there)
		spec := map[string]interface{}{
			"minAvailable":   int64(2),
			"maxUnavailable": int64(1),
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": "both",
				},
			},
		}
		obj := makePDBObj("both-pdb", "default",
			map[string]interface{}{"app": "both"}, spec)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, int64(2), result.Values["minAvailable"])
		testutil.AssertEqual(t, int64(1), result.Values["maxUnavailable"])
	})

	t.Run("MinimalPDB", func(t *testing.T) {
		spec := map[string]interface{}{
			"maxUnavailable": int64(0),
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": "minimal",
				},
			},
		}
		obj := makePDBObj("minimal-pdb", "default",
			map[string]interface{}{"app": "minimal"}, spec)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)
	})
}

// ============================================================
// Constructor and metadata tests
// ============================================================

func TestNewPDBProcessor(t *testing.T) {
	p := NewPDBProcessor()
	testutil.AssertEqual(t, "pdb", p.Name())
	testutil.AssertEqual(t, 90, p.Priority())

	gvks := p.Supports()
	testutil.AssertEqual(t, 1, len(gvks))
	testutil.AssertEqual(t, schema.GroupVersionKind{
		Group: "policy", Version: "v1", Kind: "PodDisruptionBudget",
	}, gvks[0])
}

func TestProcessPDB_ResultMetadata(t *testing.T) {
	p := NewPDBProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"minAvailable": int64(1),
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"app": "webapp",
			},
		},
	}
	obj := makePDBObj("webapp-pdb", "default",
		map[string]interface{}{"app": "webapp"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "webapp", result.ServiceName)
	testutil.AssertEqual(t, "templates/webapp-pdb.yaml", result.TemplatePath)
	testutil.AssertEqual(t, "services.webapp.pdb", result.ValuesPath)
	testutil.AssertContains(t, result.TemplateContent, "kind: PodDisruptionBudget")
	testutil.AssertContains(t, result.TemplateContent, "minAvailable")
}

func TestProcessPDB_GeneratesTemplate(t *testing.T) {
	p := NewPDBProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"maxUnavailable": int64(1),
		"selector": map[string]interface{}{
			"matchLabels": map[string]interface{}{
				"app": "tmplapp",
			},
		},
	}
	obj := makePDBObj("tmplapp-pdb", "default",
		map[string]interface{}{"app": "tmplapp"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tmpl := result.TemplateContent
	testutil.AssertContains(t, tmpl, "policy/v1")
	testutil.AssertContains(t, tmpl, "PodDisruptionBudget")
	testutil.AssertContains(t, tmpl, "maxUnavailable")
	testutil.AssertContains(t, tmpl, "selector")
	if !strings.Contains(tmpl, "test-chart") {
		t.Error("Expected template to reference chart name")
	}
}
