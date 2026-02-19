package k8s

import (
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// TopologySpreadConstraints tests verify extraction from Deployment pod spec.
// This uses the existing DeploymentProcessor since topology is part of pod spec.

// ============================================================
// Test 1: TopologySpread present — extracted in values
// ============================================================

func TestTopologySpread_Detect_Present(t *testing.T) {
	proc := NewDeploymentProcessor()
	ctx := newTestProcessorContext()

	obj := makeDeploymentObj("myapp", "default", nil, map[string]interface{}{
		"replicas": int64(3),
		"template": map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{"app": "myapp"},
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "app",
						"image": "myapp:v1",
					},
				},
				"topologySpreadConstraints": []interface{}{
					map[string]interface{}{
						"maxSkew":           int64(1),
						"topologyKey":       "topology.kubernetes.io/zone",
						"whenUnsatisfiable": "DoNotSchedule",
						"labelSelector": map[string]interface{}{
							"matchLabels": map[string]interface{}{
								"app": "myapp",
							},
						},
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tsc, ok := result.Values["topologySpreadConstraints"].([]interface{})
	if !ok {
		t.Fatal("Expected topologySpreadConstraints in values")
	}
	if len(tsc) != 1 {
		t.Fatalf("Expected 1 topologySpreadConstraint, got %d", len(tsc))
	}
}

// ============================================================
// Test 2: TopologySpread absent — not in values
// ============================================================

func TestTopologySpread_Detect_Absent(t *testing.T) {
	proc := NewDeploymentProcessor()
	ctx := newTestProcessorContext()

	obj := makeDeploymentObj("myapp", "default", nil, makeBasicSpec(1, "app", "myapp:v1"))

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	if _, ok := result.Values["topologySpreadConstraints"]; ok {
		t.Error("Expected no topologySpreadConstraints for deployment without them")
	}
}

// ============================================================
// Test 3: MaxSkew value extraction
// ============================================================

func TestTopologySpread_MaxSkew_Value(t *testing.T) {
	proc := NewDeploymentProcessor()
	ctx := newTestProcessorContext()

	obj := makeDeploymentObj("myapp", "default", nil, map[string]interface{}{
		"replicas": int64(3),
		"template": map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{"app": "myapp"},
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "app", "image": "myapp:v1"},
				},
				"topologySpreadConstraints": []interface{}{
					map[string]interface{}{
						"maxSkew":     int64(2),
						"topologyKey": "topology.kubernetes.io/zone",
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tsc := result.Values["topologySpreadConstraints"].([]interface{})
	constraint := tsc[0].(map[string]interface{})

	// After JSON round-trip, int64 values become float64
	maxSkew, ok := constraint["maxSkew"]
	if !ok {
		t.Fatal("Expected maxSkew in constraint")
	}
	// Check both int64 and float64 for flexibility
	switch v := maxSkew.(type) {
	case int64:
		testutil.AssertEqual(t, int64(2), v, "maxSkew")
	case float64:
		testutil.AssertEqual(t, float64(2), v, "maxSkew")
	default:
		t.Fatalf("Unexpected maxSkew type: %T", maxSkew)
	}
}

// ============================================================
// Test 4: TopologyKey zone
// ============================================================

func TestTopologySpread_TopologyKey_Zone(t *testing.T) {
	proc := NewDeploymentProcessor()
	ctx := newTestProcessorContext()

	obj := makeDeploymentObj("myapp", "default", nil, map[string]interface{}{
		"replicas": int64(3),
		"template": map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{"app": "myapp"},
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "app", "image": "myapp:v1"},
				},
				"topologySpreadConstraints": []interface{}{
					map[string]interface{}{
						"maxSkew":     int64(1),
						"topologyKey": "topology.kubernetes.io/zone",
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tsc := result.Values["topologySpreadConstraints"].([]interface{})
	constraint := tsc[0].(map[string]interface{})
	testutil.AssertEqual(t, "topology.kubernetes.io/zone", constraint["topologyKey"], "topologyKey")
}

// ============================================================
// Test 5: TopologyKey node
// ============================================================

func TestTopologySpread_TopologyKey_Node(t *testing.T) {
	proc := NewDeploymentProcessor()
	ctx := newTestProcessorContext()

	obj := makeDeploymentObj("myapp", "default", nil, map[string]interface{}{
		"replicas": int64(3),
		"template": map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{"app": "myapp"},
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{"name": "app", "image": "myapp:v1"},
				},
				"topologySpreadConstraints": []interface{}{
					map[string]interface{}{
						"maxSkew":     int64(1),
						"topologyKey": "kubernetes.io/hostname",
					},
				},
			},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tsc := result.Values["topologySpreadConstraints"].([]interface{})
	constraint := tsc[0].(map[string]interface{})
	testutil.AssertEqual(t, "kubernetes.io/hostname", constraint["topologyKey"], "topologyKey")
}
