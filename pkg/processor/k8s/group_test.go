package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

func makeGroupObj(name string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "deckhouse.io/v1",
			"kind":       "Group",
			"metadata":   map[string]interface{}{"name": name},
			"spec":       spec,
		},
	}
}

func TestGroupProcessor_Name(t *testing.T) {
	proc := NewGroupProcessor()
	testutil.AssertEqual(t, "group", proc.Name(), "processor name")
}

func TestGroupProcessor_Supports(t *testing.T) {
	proc := NewGroupProcessor()
	gvks := proc.Supports()
	if len(gvks) != 1 {
		t.Fatalf("Expected 1 GVK, got %d", len(gvks))
	}
	expected := schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: "Group"}
	testutil.AssertEqual(t, expected, gvks[0], "supported GVK")
}

func TestGroupProcessor_Members(t *testing.T) {
	proc := NewGroupProcessor()
	ctx := newTestProcessorContext()

	obj := makeGroupObj("admins", map[string]interface{}{
		"members": []interface{}{
			map[string]interface{}{"kind": "User", "name": "admin@example.com"},
			map[string]interface{}{"kind": "User", "name": "operator@example.com"},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	members, ok := result.Values["members"].([]interface{})
	if !ok {
		t.Fatal("Expected members as slice")
	}
	if len(members) != 2 {
		t.Fatalf("Expected 2 members, got %d", len(members))
	}
}

func TestGroupProcessor_EmptyMembers(t *testing.T) {
	proc := NewGroupProcessor()
	ctx := newTestProcessorContext()

	obj := makeGroupObj("empty-group", map[string]interface{}{})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed, "should be processed")
}

func TestGroupProcessor_ServiceName(t *testing.T) {
	proc := NewGroupProcessor()
	ctx := newTestProcessorContext()

	obj := makeGroupObj("developers", map[string]interface{}{
		"members": []interface{}{
			map[string]interface{}{"kind": "User", "name": "dev@example.com"},
		},
	})

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "developers", result.ServiceName, "ServiceName")
}
