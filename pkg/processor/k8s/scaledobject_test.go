package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helper: create ScaledObject unstructured object
// ============================================================

func makeScaledObjectObj(name, namespace string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "keda.sh/v1alpha1",
			"kind":       "ScaledObject",
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

func TestScaledObjectProcessor_Name(t *testing.T) {
	proc := NewScaledObjectProcessor()
	testutil.AssertEqual(t, "scaledobject", proc.Name(), "processor name")
}

// ============================================================
// Test 2: Supports GVK
// ============================================================

func TestScaledObjectProcessor_Supports(t *testing.T) {
	proc := NewScaledObjectProcessor()
	gvks := proc.Supports()

	if len(gvks) != 1 {
		t.Fatalf("Expected 1 supported GVK, got %d", len(gvks))
	}

	expected := schema.GroupVersionKind{
		Group:   "keda.sh",
		Version: "v1alpha1",
		Kind:    "ScaledObject",
	}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

// ============================================================
// Test 3: ScaleTargetRef extraction
// ============================================================

func TestScaledObjectProcessor_ScaleTargetRef(t *testing.T) {
	proc := NewScaledObjectProcessor()
	ctx := newTestProcessorContext()

	obj := makeScaledObjectObj("myapp-scaler", "default", map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"name": "myapp",
			"kind": "Deployment",
		},
		"minReplicaCount": int64(2),
		"maxReplicaCount": int64(10),
		"triggers":        []interface{}{},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")

	targetRef, ok := result.Values["scaleTargetRef"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected scaleTargetRef map in values")
	}
	testutil.AssertEqual(t, "myapp", targetRef["name"], "target name")
	testutil.AssertEqual(t, "Deployment", targetRef["kind"], "target kind")
}

// ============================================================
// Test 4: MinReplicaCount normal
// ============================================================

func TestScaledObjectProcessor_MinReplicaCount_Normal(t *testing.T) {
	proc := NewScaledObjectProcessor()
	ctx := newTestProcessorContext()

	obj := makeScaledObjectObj("myapp-scaler", "default", map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"name": "myapp",
		},
		"minReplicaCount": int64(2),
		"maxReplicaCount": int64(10),
		"triggers":        []interface{}{},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, int64(2), result.Values["minReplicaCount"], "minReplicaCount")
}

// ============================================================
// Test 5: MinReplicaCount zero (scale-to-zero)
// ============================================================

func TestScaledObjectProcessor_MinReplicaCount_Zero(t *testing.T) {
	proc := NewScaledObjectProcessor()
	ctx := newTestProcessorContext()

	obj := makeScaledObjectObj("myapp-scaler", "default", map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"name": "myapp",
		},
		"minReplicaCount": int64(0),
		"maxReplicaCount": int64(5),
		"triggers":        []interface{}{},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, int64(0), result.Values["minReplicaCount"], "minReplicaCount should be 0")

	// Should flag scale-to-zero in metadata
	scaleToZero, _ := result.Metadata["scale_to_zero"].(bool)
	if !scaleToZero {
		t.Error("Expected scale_to_zero=true in metadata for minReplicaCount=0")
	}
}

// ============================================================
// Test 6: MaxReplicaCount
// ============================================================

func TestScaledObjectProcessor_MaxReplicaCount(t *testing.T) {
	proc := NewScaledObjectProcessor()
	ctx := newTestProcessorContext()

	obj := makeScaledObjectObj("myapp-scaler", "default", map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"name": "myapp",
		},
		"maxReplicaCount": int64(100),
		"triggers":        []interface{}{},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, int64(100), result.Values["maxReplicaCount"], "maxReplicaCount")
}

// ============================================================
// Test 7: Single trigger
// ============================================================

func TestScaledObjectProcessor_Triggers_Single(t *testing.T) {
	proc := NewScaledObjectProcessor()
	ctx := newTestProcessorContext()

	obj := makeScaledObjectObj("myapp-scaler", "default", map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"name": "myapp",
		},
		"triggers": []interface{}{
			map[string]interface{}{
				"type": "prometheus",
				"metadata": map[string]interface{}{
					"serverAddress": "http://prometheus:9090",
					"query":         "sum(rate(http_requests_total[2m]))",
					"threshold":     "100",
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	triggers, ok := result.Values["triggers"].([]interface{})
	if !ok {
		t.Fatal("Expected triggers slice in values")
	}
	if len(triggers) != 1 {
		t.Fatalf("Expected 1 trigger, got %d", len(triggers))
	}

	trigger := triggers[0].(map[string]interface{})
	testutil.AssertEqual(t, "prometheus", trigger["type"], "trigger type")
}

// ============================================================
// Test 8: Multiple triggers with auth
// ============================================================

func TestScaledObjectProcessor_Triggers_Multi_WithAuth(t *testing.T) {
	proc := NewScaledObjectProcessor()
	ctx := newTestProcessorContext()

	obj := makeScaledObjectObj("myapp-scaler", "default", map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"name": "myapp",
		},
		"triggers": []interface{}{
			map[string]interface{}{
				"type": "kafka",
				"metadata": map[string]interface{}{
					"bootstrapServers": "kafka:9092",
					"consumerGroup":    "myapp",
					"topic":            "events",
				},
				"authenticationRef": map[string]interface{}{
					"name": "kafka-auth",
				},
			},
			map[string]interface{}{
				"type": "cpu",
				"metadata": map[string]interface{}{
					"type":  "Utilization",
					"value": "80",
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	triggers := result.Values["triggers"].([]interface{})
	if len(triggers) != 2 {
		t.Fatalf("Expected 2 triggers, got %d", len(triggers))
	}

	// First trigger should have authenticationRef
	t1 := triggers[0].(map[string]interface{})
	authRef, ok := t1["authenticationRef"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected authenticationRef in first trigger")
	}
	testutil.AssertEqual(t, "kafka-auth", authRef["name"], "authRef name")
}

// ============================================================
// Test 9: Dependency to Deployment via scaleTargetRef
// ============================================================

func TestScaledObjectProcessor_Dependency_ToDeployment(t *testing.T) {
	proc := NewScaledObjectProcessor()
	ctx := newTestProcessorContext()

	obj := makeScaledObjectObj("myapp-scaler", "default", map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"name": "myapp",
			"kind": "Deployment",
		},
		"triggers": []interface{}{},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	if len(result.Dependencies) == 0 {
		t.Fatal("Expected at least 1 dependency (Deployment)")
	}

	found := hasDependency(result.Dependencies, "Deployment", "default", "myapp")
	if !found {
		t.Errorf("Expected dependency to Deployment 'myapp', got: %v", result.Dependencies)
	}
}

// ============================================================
// Test 10: Template content
// ============================================================

func TestScaledObjectProcessor_Template(t *testing.T) {
	proc := NewScaledObjectProcessor()
	ctx := newTestProcessorContext()

	obj := makeScaledObjectObj("myapp-scaler", "default", map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"name": "myapp",
		},
		"triggers": []interface{}{
			map[string]interface{}{
				"type":     "cpu",
				"metadata": map[string]interface{}{"value": "80"},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	testutil.AssertContains(t, tpl, "apiVersion: keda.sh/v1alpha1", "apiVersion")
	testutil.AssertContains(t, tpl, "kind: ScaledObject", "kind")
	testutil.AssertContains(t, tpl, "$svc.enabled", "enabled check")
	if !strings.Contains(tpl, "scaleTargetRef") {
		t.Error("Template should reference scaleTargetRef")
	}
}
