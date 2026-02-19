package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create Rollout unstructured object
// ============================================================

func makeRolloutObj(name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Rollout",
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

func TestRolloutProcessor_Name(t *testing.T) {
	proc := NewRolloutProcessor()
	testutil.AssertEqual(t, "rollout", proc.Name(), "processor name")
}

// ============================================================
// Test 2: Supports GVK
// ============================================================

func TestRolloutProcessor_Supports(t *testing.T) {
	proc := NewRolloutProcessor()
	gvks := proc.Supports()

	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}

	expected := schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "Rollout",
	}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// Test 3: Canary strategy with steps
// ============================================================

func TestRolloutProcessor_Strategy_Canary_Steps(t *testing.T) {
	proc := NewRolloutProcessor()
	ctx := newTestProcessorContext()

	obj := makeRolloutObj("myapp", "default", map[string]interface{}{
		"strategy": map[string]interface{}{
			"canary": map[string]interface{}{
				"steps": []interface{}{
					map[string]interface{}{"setWeight": int64(20)},
					map[string]interface{}{"pause": map[string]interface{}{"duration": "5m"}},
					map[string]interface{}{"setWeight": int64(50)},
					map[string]interface{}{"pause": map[string]interface{}{"duration": "10m"}},
				},
			},
		},
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "app", "image": "myapp:v1"},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")

	strategy, ok := result.Values["strategy"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected strategy map in values")
	}

	canary, ok := strategy["canary"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected canary map in strategy")
	}

	steps, ok := canary["steps"].([]interface{})
	if !ok {
		t.Fatal("Expected steps slice in canary")
	}
	if len(steps) != 4 {
		t.Fatalf("Expected 4 steps, got %d", len(steps))
	}
}

// ============================================================
// Test 4: Canary maxSurge/maxUnavailable
// ============================================================

func TestRolloutProcessor_Strategy_Canary_MaxSurge(t *testing.T) {
	proc := NewRolloutProcessor()
	ctx := newTestProcessorContext()

	obj := makeRolloutObj("myapp", "default", map[string]interface{}{
		"strategy": map[string]interface{}{
			"canary": map[string]interface{}{
				"maxSurge":       "25%",
				"maxUnavailable": int64(0),
			},
		},
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "app", "image": "myapp:v1"},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	strategy := result.Values["strategy"].(map[string]interface{})
	canary := strategy["canary"].(map[string]interface{})
	testutil.AssertEqual(t, "25%", canary["maxSurge"], "maxSurge")
}

// ============================================================
// Test 5: BlueGreen autoPromotionEnabled
// ============================================================

func TestRolloutProcessor_Strategy_BlueGreen_AutoPromotion(t *testing.T) {
	proc := NewRolloutProcessor()
	ctx := newTestProcessorContext()

	obj := makeRolloutObj("myapp", "default", map[string]interface{}{
		"strategy": map[string]interface{}{
			"blueGreen": map[string]interface{}{
				"autoPromotionEnabled": false,
				"activeService":        "myapp-active",
				"previewService":       "myapp-preview",
			},
		},
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "app", "image": "myapp:v1"},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	strategy := result.Values["strategy"].(map[string]interface{})
	blueGreen, ok := strategy["blueGreen"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected blueGreen in strategy")
	}
	testutil.AssertEqual(t, false, blueGreen["autoPromotionEnabled"], "autoPromotionEnabled")
	testutil.AssertEqual(t, "myapp-active", blueGreen["activeService"], "activeService")
}

// ============================================================
// Test 6: BlueGreen prePromotionAnalysis
// ============================================================

func TestRolloutProcessor_Strategy_BlueGreen_PreAnalysis(t *testing.T) {
	proc := NewRolloutProcessor()
	ctx := newTestProcessorContext()

	obj := makeRolloutObj("myapp", "default", map[string]interface{}{
		"strategy": map[string]interface{}{
			"blueGreen": map[string]interface{}{
				"autoPromotionEnabled": true,
				"prePromotionAnalysis": map[string]interface{}{
					"templates": []interface{}{
						map[string]interface{}{
							"templateName": "success-rate",
						},
					},
				},
			},
		},
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "app", "image": "myapp:v1"},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	strategy := result.Values["strategy"].(map[string]interface{})
	blueGreen := strategy["blueGreen"].(map[string]interface{})

	preAnalysis, ok := blueGreen["prePromotionAnalysis"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected prePromotionAnalysis in blueGreen")
	}
	templates, ok := preAnalysis["templates"].([]interface{})
	if !ok {
		t.Fatal("Expected templates in prePromotionAnalysis")
	}
	if len(templates) != 1 {
		t.Fatalf("Expected 1 template, got %d", len(templates))
	}
}

// ============================================================
// Test 7: Pod spec preserved from template
// ============================================================

func TestRolloutProcessor_PodSpec_Preserved(t *testing.T) {
	proc := NewRolloutProcessor()
	ctx := newTestProcessorContext()

	obj := makeRolloutObj("myapp", "default", map[string]interface{}{
		"strategy": map[string]interface{}{
			"canary": map[string]interface{}{},
		},
		"template": map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{"app": "myapp"},
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "app",
						"image": "myapp:v1",
						"ports": []interface{}{
							map[string]interface{}{
								"containerPort": int64(8080),
							},
						},
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	// Pod template should be preserved in values
	podTemplate, ok := result.Values["template"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected template (pod spec) in values")
	}
	if podTemplate["spec"] == nil {
		t.Error("Expected spec in pod template")
	}
}

// ============================================================
// Test 8: Template content
// ============================================================

func TestRolloutProcessor_Template(t *testing.T) {
	proc := NewRolloutProcessor()
	ctx := newTestProcessorContext()

	obj := makeRolloutObj("myapp", "default", map[string]interface{}{
		"strategy": map[string]interface{}{
			"canary": map[string]interface{}{},
		},
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "app", "image": "myapp:v1"},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	testutil.AssertContains(t, tpl, "apiVersion: argoproj.io/v1alpha1", "apiVersion")
	testutil.AssertContains(t, tpl, "kind: Rollout", "kind")
	testutil.AssertContains(t, tpl, "$svc.enabled", "enabled check")
	if !strings.Contains(tpl, "strategy") {
		t.Error("Template should reference strategy")
	}
}
