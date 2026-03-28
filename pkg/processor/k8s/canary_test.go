package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create Canary unstructured object
// ============================================================

func makeCanaryObj(name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "flagger.app/v1beta1",
			"kind":       "Canary",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
}

// ============================================================
// Test: Flagger Canary extraction
// ============================================================

func TestFlaggerCanary_Extraction(t *testing.T) {
	proc := NewFlaggerCanaryProcessor()
	ctx := newTestProcessorContext()

	obj := makeCanaryObj("myapp", "default", map[string]interface{}{
		"targetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "myapp",
		},
		"progressDeadlineSeconds": int64(60),
		"analysis": map[string]interface{}{
			"interval":  "30s",
			"threshold": int64(5),
			"maxWeight": int64(50),
			"stepWeight": int64(10),
			"metrics": []interface{}{
				map[string]interface{}{
					"name":      "request-success-rate",
					"threshold": int64(99),
					"interval":  "1m",
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")
	testutil.AssertEqual(t, "myapp", result.ServiceName, "serviceName")

	// Check targetRef extracted
	targetRef, ok := result.Values["targetRef"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected targetRef map in values")
	}
	testutil.AssertEqual(t, "Deployment", targetRef["kind"], "targetRef kind")
	testutil.AssertEqual(t, "myapp", targetRef["name"], "targetRef name")

	// Check analysis extracted
	analysis, ok := result.Values["analysis"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected analysis map in values")
	}
	if analysis["interval"] != "30s" {
		t.Errorf("Expected analysis interval '30s', got '%v'", analysis["interval"])
	}

	// Check progressDeadlineSeconds
	testutil.AssertEqual(t, int64(60), result.Values["progressDeadlineSeconds"], "progressDeadlineSeconds")

	// Check dependency to target Deployment
	if len(result.Dependencies) == 0 {
		t.Fatal("Expected dependency to target Deployment")
	}
	found := hasDependency(result.Dependencies, "Deployment", "default", "myapp")
	if !found {
		t.Errorf("Expected dependency to Deployment 'myapp', got: %v", result.Dependencies)
	}

	// Check GVK
	gvks := proc.Supports()
	expected := schema.GroupVersionKind{Group: "flagger.app", Version: "v1beta1", Kind: "Canary"}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")

	// Check template
	tpl := result.TemplateContent
	testutil.AssertContains(t, tpl, "apiVersion: flagger.app/v1beta1", "apiVersion")
	testutil.AssertContains(t, tpl, "kind: Canary", "kind")
	if !strings.Contains(tpl, "targetRef") {
		t.Error("Template should reference targetRef")
	}
	if !strings.Contains(tpl, "analysis") {
		t.Error("Template should reference analysis")
	}
}

// ============================================================
// Test: Canary processor name
// ============================================================

func TestFlaggerCanary_Name(t *testing.T) {
	proc := NewFlaggerCanaryProcessor()
	testutil.AssertEqual(t, "canary", proc.Name(), "processor name")
}
