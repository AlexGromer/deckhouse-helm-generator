package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Task 4.1: HPA Processor Tests (TDD — tests written first)
// ============================================================

// makeHPAObj creates an unstructured HorizontalPodAutoscaler for testing.
func makeHPAObj(name, namespace string, labels map[string]interface{}, spec map[string]interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}
	if labels != nil {
		metadata["labels"] = labels
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "autoscaling/v2",
			"kind":       "HorizontalPodAutoscaler",
			"metadata":   metadata,
			"spec":       spec,
		},
	}
}

// ============================================================
// Subtask 1: Extract scaleTargetRef
// ============================================================

func TestProcessHPA_ExtractsScaleTargetRef(t *testing.T) {
	p := NewHPAProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "myapp",
		},
		"minReplicas": int64(1),
		"maxReplicas": int64(10),
	}
	obj := makeHPAObj("myapp-hpa", "default",
		map[string]interface{}{"app": "myapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)

	targetRef, ok := result.Values["scaleTargetRef"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected scaleTargetRef in values")
	}
	testutil.AssertEqual(t, "Deployment", targetRef["kind"])
	testutil.AssertEqual(t, "myapp", targetRef["name"])

	// Should add dependency on the target deployment
	found := false
	for _, dep := range result.Dependencies {
		if dep.GVK.Kind == "Deployment" && dep.Name == "myapp" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected Deployment dependency for scaleTargetRef")
	}
}

// ============================================================
// Subtask 2: Extract min/max replicas
// ============================================================

func TestProcessHPA_ExtractsMinMaxReplicas(t *testing.T) {
	p := NewHPAProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "webapp",
		},
		"minReplicas": int64(2),
		"maxReplicas": int64(10),
	}
	obj := makeHPAObj("webapp-hpa", "default",
		map[string]interface{}{"app": "webapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, int64(2), result.Values["minReplicas"])
	testutil.AssertEqual(t, int64(10), result.Values["maxReplicas"])
}

// ============================================================
// Subtask 3: Extract CPU metric (v2)
// ============================================================

func TestProcessHPA_ExtractsCPUMetric(t *testing.T) {
	p := NewHPAProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "cpuapp",
		},
		"minReplicas": int64(1),
		"maxReplicas": int64(10),
		"metrics": []interface{}{
			map[string]interface{}{
				"type": "Resource",
				"resource": map[string]interface{}{
					"name": "cpu",
					"target": map[string]interface{}{
						"type":               "Utilization",
						"averageUtilization": int64(80),
					},
				},
			},
		},
	}
	obj := makeHPAObj("cpuapp-hpa", "default",
		map[string]interface{}{"app": "cpuapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	metrics, ok := result.Values["metrics"].([]interface{})
	if !ok {
		t.Fatal("Expected metrics in values")
	}
	if len(metrics) == 0 {
		t.Fatal("Expected at least 1 metric")
	}
	metric, ok := metrics[0].(map[string]interface{})
	if !ok {
		t.Fatal("Expected metric to be a map")
	}
	testutil.AssertEqual(t, "Resource", metric["type"])

	resource, ok := metric["resource"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected resource in metric")
	}
	testutil.AssertEqual(t, "cpu", resource["name"])
}

// ============================================================
// Subtask 4: Extract memory metric (v2)
// ============================================================

func TestProcessHPA_ExtractsMemoryMetric(t *testing.T) {
	p := NewHPAProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "memapp",
		},
		"minReplicas": int64(1),
		"maxReplicas": int64(5),
		"metrics": []interface{}{
			map[string]interface{}{
				"type": "Resource",
				"resource": map[string]interface{}{
					"name": "memory",
					"target": map[string]interface{}{
						"type":               "Utilization",
						"averageUtilization": int64(70),
					},
				},
			},
		},
	}
	obj := makeHPAObj("memapp-hpa", "default",
		map[string]interface{}{"app": "memapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	metrics, ok := result.Values["metrics"].([]interface{})
	if !ok {
		t.Fatal("Expected metrics in values")
	}
	if len(metrics) == 0 {
		t.Fatal("Expected at least 1 metric")
	}
	metric := metrics[0].(map[string]interface{})
	resource := metric["resource"].(map[string]interface{})
	testutil.AssertEqual(t, "memory", resource["name"])
}

// ============================================================
// Subtask 5: Extract custom metric
// ============================================================

func TestProcessHPA_ExtractsCustomMetric(t *testing.T) {
	p := NewHPAProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "customapp",
		},
		"minReplicas": int64(2),
		"maxReplicas": int64(20),
		"metrics": []interface{}{
			map[string]interface{}{
				"type": "Pods",
				"pods": map[string]interface{}{
					"metric": map[string]interface{}{
						"name": "http_requests_per_second",
					},
					"target": map[string]interface{}{
						"type":         "AverageValue",
						"averageValue": "1000m",
					},
				},
			},
		},
	}
	obj := makeHPAObj("customapp-hpa", "default",
		map[string]interface{}{"app": "customapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	metrics := result.Values["metrics"].([]interface{})
	testutil.AssertEqual(t, 1, len(metrics))
	metric := metrics[0].(map[string]interface{})
	testutil.AssertEqual(t, "Pods", metric["type"])

	pods, ok := metric["pods"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected pods in metric")
	}
	metricInfo := pods["metric"].(map[string]interface{})
	testutil.AssertEqual(t, "http_requests_per_second", metricInfo["name"])
}

// ============================================================
// Subtask 6: Extract external metric
// ============================================================

func TestProcessHPA_ExtractsExternalMetric(t *testing.T) {
	p := NewHPAProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "externalapp",
		},
		"minReplicas": int64(1),
		"maxReplicas": int64(50),
		"metrics": []interface{}{
			map[string]interface{}{
				"type": "External",
				"external": map[string]interface{}{
					"metric": map[string]interface{}{
						"name": "queue_messages_ready",
						"selector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"queue": "worker_tasks",
							},
						},
					},
					"target": map[string]interface{}{
						"type":         "AverageValue",
						"averageValue": "30",
					},
				},
			},
		},
	}
	obj := makeHPAObj("externalapp-hpa", "default",
		map[string]interface{}{"app": "externalapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	metrics := result.Values["metrics"].([]interface{})
	testutil.AssertEqual(t, 1, len(metrics))
	metric := metrics[0].(map[string]interface{})
	testutil.AssertEqual(t, "External", metric["type"])

	external, ok := metric["external"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected external in metric")
	}
	metricInfo := external["metric"].(map[string]interface{})
	testutil.AssertEqual(t, "queue_messages_ready", metricInfo["name"])
}

// ============================================================
// Subtask 7: Extract behavior (v2) — scaleDown
// ============================================================

func TestProcessHPA_ExtractsScaleDownBehavior(t *testing.T) {
	p := NewHPAProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "behaviorapp",
		},
		"minReplicas": int64(1),
		"maxReplicas": int64(10),
		"behavior": map[string]interface{}{
			"scaleDown": map[string]interface{}{
				"stabilizationWindowSeconds": int64(300),
				"policies": []interface{}{
					map[string]interface{}{
						"type":          "Percent",
						"value":         int64(10),
						"periodSeconds": int64(60),
					},
				},
			},
		},
	}
	obj := makeHPAObj("behaviorapp-hpa", "default",
		map[string]interface{}{"app": "behaviorapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	behavior, ok := result.Values["behavior"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected behavior in values")
	}
	scaleDown, ok := behavior["scaleDown"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected scaleDown in behavior")
	}
	testutil.AssertEqual(t, int64(300), scaleDown["stabilizationWindowSeconds"])

	policies, ok := scaleDown["policies"].([]interface{})
	if !ok {
		t.Fatal("Expected policies in scaleDown")
	}
	testutil.AssertEqual(t, 1, len(policies))
}

// ============================================================
// Subtask 8: Extract scaleUp behavior
// ============================================================

func TestProcessHPA_ExtractsScaleUpBehavior(t *testing.T) {
	p := NewHPAProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "scaleupapp",
		},
		"minReplicas": int64(1),
		"maxReplicas": int64(100),
		"behavior": map[string]interface{}{
			"scaleUp": map[string]interface{}{
				"stabilizationWindowSeconds": int64(0),
				"policies": []interface{}{
					map[string]interface{}{
						"type":          "Pods",
						"value":         int64(4),
						"periodSeconds": int64(60),
					},
					map[string]interface{}{
						"type":          "Percent",
						"value":         int64(100),
						"periodSeconds": int64(60),
					},
				},
				"selectPolicy": "Max",
			},
		},
	}
	obj := makeHPAObj("scaleupapp-hpa", "default",
		map[string]interface{}{"app": "scaleupapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	behavior := result.Values["behavior"].(map[string]interface{})
	scaleUp, ok := behavior["scaleUp"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected scaleUp in behavior")
	}
	testutil.AssertEqual(t, int64(0), scaleUp["stabilizationWindowSeconds"])
	testutil.AssertEqual(t, "Max", scaleUp["selectPolicy"])

	policies := scaleUp["policies"].([]interface{})
	testutil.AssertEqual(t, 2, len(policies))
}

// ============================================================
// Subtask 9: Edge cases
// ============================================================

func TestProcessHPA_EdgeCases(t *testing.T) {
	p := NewHPAProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilHPA", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil HPA")
		}
	})

	t.Run("HPAWithoutBehavior", func(t *testing.T) {
		// v1-compatible: only CPU metric, no behavior
		spec := map[string]interface{}{
			"scaleTargetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "simple",
			},
			"minReplicas": int64(1),
			"maxReplicas": int64(5),
			"metrics": []interface{}{
				map[string]interface{}{
					"type": "Resource",
					"resource": map[string]interface{}{
						"name": "cpu",
						"target": map[string]interface{}{
							"type":               "Utilization",
							"averageUtilization": int64(50),
						},
					},
				},
			},
		}
		obj := makeHPAObj("simple-hpa", "default",
			map[string]interface{}{"app": "simple"}, spec)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)

		// behavior should NOT be set
		if _, exists := result.Values["behavior"]; exists {
			t.Error("behavior should not be set for HPA without behavior spec")
		}
	})

	t.Run("HPAOnlyCPU", func(t *testing.T) {
		spec := map[string]interface{}{
			"scaleTargetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "cpuonly",
			},
			"minReplicas": int64(2),
			"maxReplicas": int64(8),
		}
		obj := makeHPAObj("cpuonly-hpa", "default",
			map[string]interface{}{"app": "cpuonly"}, spec)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, int64(2), result.Values["minReplicas"])
		testutil.AssertEqual(t, int64(8), result.Values["maxReplicas"])
	})

	t.Run("HPATargetsStatefulSet", func(t *testing.T) {
		spec := map[string]interface{}{
			"scaleTargetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "StatefulSet",
				"name":       "db",
			},
			"minReplicas": int64(3),
			"maxReplicas": int64(6),
		}
		obj := makeHPAObj("db-hpa", "default",
			map[string]interface{}{"app": "db"}, spec)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)

		targetRef := result.Values["scaleTargetRef"].(map[string]interface{})
		testutil.AssertEqual(t, "StatefulSet", targetRef["kind"])

		// Should have StatefulSet dependency
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
// Constructor and metadata tests
// ============================================================

func TestNewHPAProcessor(t *testing.T) {
	p := NewHPAProcessor()
	testutil.AssertEqual(t, "hpa", p.Name())
	testutil.AssertEqual(t, 90, p.Priority())

	gvks := p.Supports()
	testutil.AssertEqual(t, 1, len(gvks))
	testutil.AssertEqual(t, schema.GroupVersionKind{
		Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler",
	}, gvks[0])
}

func TestProcessHPA_ResultMetadata(t *testing.T) {
	p := NewHPAProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "webapp",
		},
		"minReplicas": int64(1),
		"maxReplicas": int64(10),
	}
	obj := makeHPAObj("webapp-hpa", "default",
		map[string]interface{}{"app": "webapp"}, spec)

	result, err := p.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "webapp", result.ServiceName)
	testutil.AssertEqual(t, "templates/webapp-hpa.yaml", result.TemplatePath)
	testutil.AssertEqual(t, "services.webapp.hpa", result.ValuesPath)
	testutil.AssertContains(t, result.TemplateContent, "kind: HorizontalPodAutoscaler")
	testutil.AssertContains(t, result.TemplateContent, "scaleTargetRef")
}

func TestProcessHPA_GeneratesTemplate(t *testing.T) {
	p := NewHPAProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       "tmplapp",
		},
		"minReplicas": int64(1),
		"maxReplicas": int64(10),
		"metrics": []interface{}{
			map[string]interface{}{
				"type": "Resource",
				"resource": map[string]interface{}{
					"name": "cpu",
					"target": map[string]interface{}{
						"type":               "Utilization",
						"averageUtilization": int64(80),
					},
				},
			},
		},
		"behavior": map[string]interface{}{
			"scaleDown": map[string]interface{}{
				"stabilizationWindowSeconds": int64(300),
			},
		},
	}
	obj := makeHPAObj("tmplapp-hpa", "default",
		map[string]interface{}{"app": "tmplapp"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tmpl := result.TemplateContent
	testutil.AssertContains(t, tmpl, "autoscaling/v2")
	testutil.AssertContains(t, tmpl, "HorizontalPodAutoscaler")
	testutil.AssertContains(t, tmpl, "minReplicas")
	testutil.AssertContains(t, tmpl, "maxReplicas")
	testutil.AssertContains(t, tmpl, "metrics")
	testutil.AssertContains(t, tmpl, "behavior")
	if !strings.Contains(tmpl, "test-chart") {
		t.Error("Expected template to reference chart name")
	}
}
