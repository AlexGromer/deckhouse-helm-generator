package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

func makeNodeGroupObj(name string, spec map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "deckhouse.io/v1",
			"kind":       "NodeGroup",
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}
	if spec != nil {
		obj.Object["spec"] = spec
	}
	return obj
}

func TestNodeGroupProcessor_Name(t *testing.T) {
	proc := NewNodeGroupProcessor()
	testutil.AssertEqual(t, "nodegroup", proc.Name(), "processor name")
}

func TestNodeGroupProcessor_Supports(t *testing.T) {
	proc := NewNodeGroupProcessor()
	gvks := proc.Supports()
	if len(gvks) != 1 {
		t.Fatalf("Expected 1 GVK, got %d", len(gvks))
	}
	expected := schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: "NodeGroup"}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

func TestNodeGroupProcessor_NodeType_CloudEphemeral(t *testing.T) {
	proc := NewNodeGroupProcessor()
	ctx := newTestProcessorContext()

	obj := makeNodeGroupObj("worker", map[string]interface{}{
		"nodeType": "CloudEphemeral",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "CloudEphemeral", result.Values["nodeType"], "nodeType")
}

func TestNodeGroupProcessor_NodeType_CloudPermanent(t *testing.T) {
	proc := NewNodeGroupProcessor()
	ctx := newTestProcessorContext()

	obj := makeNodeGroupObj("master", map[string]interface{}{
		"nodeType": "CloudPermanent",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "CloudPermanent", result.Values["nodeType"], "nodeType")
}

func TestNodeGroupProcessor_NodeType_Static(t *testing.T) {
	proc := NewNodeGroupProcessor()
	ctx := newTestProcessorContext()

	obj := makeNodeGroupObj("static-nodes", map[string]interface{}{
		"nodeType": "Static",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "Static", result.Values["nodeType"], "nodeType")
}

func TestNodeGroupProcessor_Disruptions_ApprovalMode(t *testing.T) {
	proc := NewNodeGroupProcessor()
	ctx := newTestProcessorContext()

	obj := makeNodeGroupObj("worker", map[string]interface{}{
		"nodeType": "CloudEphemeral",
		"disruptions": map[string]interface{}{
			"approvalMode": "Automatic",
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	disruptions, ok := result.Values["disruptions"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected disruptions map in values")
	}
	testutil.AssertEqual(t, "Automatic", disruptions["approvalMode"], "approvalMode")
}

func TestNodeGroupProcessor_Kubelet_MaxPods(t *testing.T) {
	proc := NewNodeGroupProcessor()
	ctx := newTestProcessorContext()

	obj := makeNodeGroupObj("worker", map[string]interface{}{
		"nodeType": "CloudEphemeral",
		"kubelet": map[string]interface{}{
			"maxPods":             int64(150),
			"containerLogMaxSize": "20Mi",
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	kubelet, ok := result.Values["kubelet"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected kubelet map in values")
	}
	testutil.AssertEqual(t, int64(150), kubelet["maxPods"], "maxPods")
	testutil.AssertEqual(t, "20Mi", kubelet["containerLogMaxSize"], "containerLogMaxSize")
}

func TestNodeGroupProcessor_CloudInstances_MinMax(t *testing.T) {
	proc := NewNodeGroupProcessor()
	ctx := newTestProcessorContext()

	obj := makeNodeGroupObj("worker", map[string]interface{}{
		"nodeType": "CloudEphemeral",
		"cloudInstances": map[string]interface{}{
			"minPerZone": int64(2),
			"maxPerZone": int64(5),
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	ci, ok := result.Values["cloudInstances"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected cloudInstances map in values")
	}
	testutil.AssertEqual(t, int64(2), ci["minPerZone"], "minPerZone")
	testutil.AssertEqual(t, int64(5), ci["maxPerZone"], "maxPerZone")
}

func TestNodeGroupProcessor_CloudInstances_Zones(t *testing.T) {
	proc := NewNodeGroupProcessor()
	ctx := newTestProcessorContext()

	obj := makeNodeGroupObj("worker", map[string]interface{}{
		"nodeType": "CloudEphemeral",
		"cloudInstances": map[string]interface{}{
			"minPerZone": int64(1),
			"maxPerZone": int64(3),
			"zones":      []interface{}{"eu-west-1a", "eu-west-1b"},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	ci := result.Values["cloudInstances"].(map[string]interface{})
	zones, ok := ci["zones"].([]interface{})
	if !ok {
		t.Fatal("Expected zones slice in cloudInstances")
	}
	if len(zones) != 2 {
		t.Fatalf("Expected 2 zones, got %d", len(zones))
	}
	testutil.AssertEqual(t, "eu-west-1a", zones[0], "first zone")
}

func TestNodeGroupProcessor_Template(t *testing.T) {
	proc := NewNodeGroupProcessor()
	ctx := newTestProcessorContext()

	obj := makeNodeGroupObj("worker", map[string]interface{}{
		"nodeType": "CloudEphemeral",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tpl := result.TemplateContent
	if tpl == "" {
		t.Fatal("Expected non-empty template")
	}
	testutil.AssertContains(t, tpl, "apiVersion: deckhouse.io/v1", "template apiVersion")
	testutil.AssertContains(t, tpl, "kind: NodeGroup", "template kind")
	testutil.AssertContains(t, tpl, ".nodeType", "template should reference nodeType")
}

func TestNodeGroupProcessor_ServiceName(t *testing.T) {
	proc := NewNodeGroupProcessor()
	ctx := newTestProcessorContext()

	obj := makeNodeGroupObj("worker-pool", map[string]interface{}{
		"nodeType": "CloudEphemeral",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "worker-pool", result.ServiceName, "ServiceName")
}

func TestNodeGroupProcessor_EmptyCloudInstances(t *testing.T) {
	proc := NewNodeGroupProcessor()
	ctx := newTestProcessorContext()

	obj := makeNodeGroupObj("static-nodes", map[string]interface{}{
		"nodeType": "Static",
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")

	// Static nodes should not have cloudInstances
	if _, ok := result.Values["cloudInstances"]; ok {
		t.Error("Static NodeGroup should not have cloudInstances in values")
	}
}
