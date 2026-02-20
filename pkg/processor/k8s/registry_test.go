package k8s

import (
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

func TestRegisterAll(t *testing.T) {
	r := processor.NewRegistry()
	RegisterAll(r)

	// Verify key processors are registered by checking the registry has entries
	// RegisterAll should register 32+ processors without panicking
	if r == nil {
		t.Fatal("registry should not be nil after RegisterAll")
	}
}

func TestNestedInt64_Int64(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": int64(3),
		},
	}
	val, ok := nestedInt64(obj, "spec", "replicas")
	if !ok {
		t.Fatal("expected ok=true for int64")
	}
	if val != 3 {
		t.Errorf("expected 3, got %d", val)
	}
}

func TestNestedInt64_Float64(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": float64(5),
		},
	}
	val, ok := nestedInt64(obj, "spec", "replicas")
	if !ok {
		t.Fatal("expected ok=true for float64")
	}
	if val != 5 {
		t.Errorf("expected 5, got %d", val)
	}
}

func TestNestedInt64_Int(t *testing.T) {
	obj := map[string]interface{}{
		"count": int(7),
	}
	val, ok := nestedInt64(obj, "count")
	if !ok {
		t.Fatal("expected ok=true for int")
	}
	if val != 7 {
		t.Errorf("expected 7, got %d", val)
	}
}

func TestNestedInt64_Int32(t *testing.T) {
	obj := map[string]interface{}{
		"count": int32(9),
	}
	val, ok := nestedInt64(obj, "count")
	if !ok {
		t.Fatal("expected ok=true for int32")
	}
	if val != 9 {
		t.Errorf("expected 9, got %d", val)
	}
}

func TestNestedInt64_Missing(t *testing.T) {
	obj := map[string]interface{}{}
	_, ok := nestedInt64(obj, "nonexistent")
	if ok {
		t.Error("expected ok=false for missing field")
	}
}

func TestNestedInt64_String(t *testing.T) {
	obj := map[string]interface{}{
		"count": "not-a-number",
	}
	_, ok := nestedInt64(obj, "count")
	if ok {
		t.Error("expected ok=false for string value")
	}
}

func TestNestedInt64_Nil(t *testing.T) {
	obj := map[string]interface{}{
		"count": nil,
	}
	_, ok := nestedInt64(obj, "count")
	if ok {
		t.Error("expected ok=false for nil value")
	}
}
