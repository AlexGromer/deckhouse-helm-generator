package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// VPA Processor Tests (TDD — tests written first)
// ============================================================

// makeVPAObj creates an unstructured VerticalPodAutoscaler for testing.
func makeVPAObj(name, namespace string, labels map[string]interface{}, spec map[string]interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}
	if labels != nil {
		metadata["labels"] = labels
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "autoscaling.k8s.io/v1",
			"kind":       "VerticalPodAutoscaler",
			"metadata":   metadata,
			"spec":       spec,
		},
	}
}

// ============================================================
// Constructor tests
// ============================================================

func TestNewVPAProcessor(t *testing.T) {
	p := NewVPAProcessor()
	testutil.AssertEqual(t, "vpa", p.Name())
	testutil.AssertEqual(t, 90, p.Priority())

	gvks := p.Supports()
	testutil.AssertEqual(t, 1, len(gvks))
	testutil.AssertEqual(t, schema.GroupVersionKind{
		Group: "autoscaling.k8s.io", Version: "v1", Kind: "VerticalPodAutoscaler",
	}, gvks[0])
}

// ============================================================
// Subtask 1: Extract targetRef + dependency
// ============================================================

func TestProcessVPA_ExtractsTargetRef(t *testing.T) {
	p := NewVPAProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"targetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "myapp",
		},
	}
	obj := makeVPAObj("myapp-vpa", "default",
		map[string]interface{}{"app": "myapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)

	targetRef, ok := result.Values["targetRef"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected targetRef in values")
	}
	testutil.AssertEqual(t, "Deployment", targetRef["kind"])
	testutil.AssertEqual(t, "myapp", targetRef["name"])

	// Should add dependency on the target
	found := false
	for _, dep := range result.Dependencies {
		if dep.GVK.Kind == "Deployment" && dep.Name == "myapp" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected Deployment dependency for targetRef")
	}
}

// ============================================================
// Subtask 2: Extract updatePolicy
// ============================================================

func TestProcessVPA_ExtractsUpdatePolicy(t *testing.T) {
	p := NewVPAProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"targetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "webapp",
		},
		"updatePolicy": map[string]interface{}{
			"updateMode": "Auto",
		},
	}
	obj := makeVPAObj("webapp-vpa", "default",
		map[string]interface{}{"app": "webapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	updatePolicy, ok := result.Values["updatePolicy"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected updatePolicy in values")
	}
	testutil.AssertEqual(t, "Auto", updatePolicy["updateMode"])
}

// ============================================================
// Subtask 3: Extract resourcePolicy
// ============================================================

func TestProcessVPA_ExtractsResourcePolicy(t *testing.T) {
	p := NewVPAProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"targetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "resapp",
		},
		"resourcePolicy": map[string]interface{}{
			"containerPolicies": []interface{}{
				map[string]interface{}{
					"containerName": "main",
					"minAllowed": map[string]interface{}{
						"cpu":    "100m",
						"memory": "128Mi",
					},
				},
			},
		},
	}
	obj := makeVPAObj("resapp-vpa", "default",
		map[string]interface{}{"app": "resapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	resourcePolicy, ok := result.Values["resourcePolicy"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected resourcePolicy in values")
	}
	policies, ok := resourcePolicy["containerPolicies"].([]interface{})
	if !ok {
		t.Fatal("Expected containerPolicies in resourcePolicy")
	}
	testutil.AssertEqual(t, 1, len(policies))
}

// ============================================================
// Subtask 4: Extract recommenders
// ============================================================

func TestProcessVPA_ExtractsRecommenders(t *testing.T) {
	p := NewVPAProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"targetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "recapp",
		},
		"recommenders": []interface{}{
			map[string]interface{}{
				"name": "custom-recommender",
			},
		},
	}
	obj := makeVPAObj("recapp-vpa", "default",
		map[string]interface{}{"app": "recapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	recommenders, ok := result.Values["recommenders"].([]interface{})
	if !ok {
		t.Fatal("Expected recommenders in values")
	}
	testutil.AssertEqual(t, 1, len(recommenders))
	rec := recommenders[0].(map[string]interface{})
	testutil.AssertEqual(t, "custom-recommender", rec["name"])
}

// ============================================================
// Subtask 5: Edge cases
// ============================================================

func TestProcessVPA_EdgeCases(t *testing.T) {
	p := NewVPAProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilVPA", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil VPA")
		}
	})

	t.Run("VPAWithoutOptionalFields", func(t *testing.T) {
		spec := map[string]interface{}{
			"targetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "simple",
			},
		}
		obj := makeVPAObj("simple-vpa", "default",
			map[string]interface{}{"app": "simple"}, spec)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)

		if _, exists := result.Values["updatePolicy"]; exists {
			t.Error("updatePolicy should not be set when not provided")
		}
		if _, exists := result.Values["resourcePolicy"]; exists {
			t.Error("resourcePolicy should not be set when not provided")
		}
		if _, exists := result.Values["recommenders"]; exists {
			t.Error("recommenders should not be set when not provided")
		}
	})

	t.Run("VPATargetsStatefulSet", func(t *testing.T) {
		spec := map[string]interface{}{
			"targetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "StatefulSet",
				"name":       "db",
			},
		}
		obj := makeVPAObj("db-vpa", "default",
			map[string]interface{}{"app": "db"}, spec)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		targetRef := result.Values["targetRef"].(map[string]interface{})
		testutil.AssertEqual(t, "StatefulSet", targetRef["kind"])

		found := false
		for _, dep := range result.Dependencies {
			if dep.GVK.Kind == "StatefulSet" && dep.Name == "db" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected StatefulSet dependency")
		}
	})
}

// ============================================================
// Result metadata tests
// ============================================================

func TestProcessVPA_ResultMetadata(t *testing.T) {
	p := NewVPAProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"targetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "webapp",
		},
	}
	obj := makeVPAObj("webapp-vpa", "default",
		map[string]interface{}{"app": "webapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "webapp", result.ServiceName)
	testutil.AssertEqual(t, "templates/webapp-vpa.yaml", result.TemplatePath)
	testutil.AssertEqual(t, "services.webapp.vpa", result.ValuesPath)
	testutil.AssertContains(t, result.TemplateContent, "kind: VerticalPodAutoscaler")
	testutil.AssertContains(t, result.TemplateContent, "targetRef")
}

// ============================================================
// Template generation tests
// ============================================================

func TestProcessVPA_GeneratesTemplate(t *testing.T) {
	p := NewVPAProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"targetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "tmplapp",
		},
		"updatePolicy": map[string]interface{}{
			"updateMode": "Auto",
		},
		"resourcePolicy": map[string]interface{}{
			"containerPolicies": []interface{}{},
		},
	}
	obj := makeVPAObj("tmplapp-vpa", "default",
		map[string]interface{}{"app": "tmplapp"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tmpl := result.TemplateContent
	testutil.AssertContains(t, tmpl, "autoscaling.k8s.io/v1")
	testutil.AssertContains(t, tmpl, "VerticalPodAutoscaler")
	testutil.AssertContains(t, tmpl, "targetRef")
	testutil.AssertContains(t, tmpl, "updatePolicy")
	testutil.AssertContains(t, tmpl, "resourcePolicy")
	testutil.AssertContains(t, tmpl, "test-chart")
}
