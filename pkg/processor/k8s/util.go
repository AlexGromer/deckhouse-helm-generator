package k8s

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// nestedInt64 extracts an int64 value from a nested field path in an unstructured object.
// It handles both int64 (from programmatic construction) and float64 (from YAML/JSON parsing)
// value types, since Go's encoding/json and sigs.k8s.io/yaml unmarshal numbers as float64.
func nestedInt64(obj map[string]interface{}, fields ...string) (int64, bool) {
	val, ok, _ := unstructured.NestedInt64(obj, fields...)
	if ok {
		return val, true
	}

	// Fallback: try float64 (YAML/JSON parsed numbers)
	raw, ok, _ := unstructured.NestedFieldNoCopy(obj, fields...)
	if !ok || raw == nil {
		return 0, false
	}

	switch v := raw.(type) {
	case float64:
		return int64(v), true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	default:
		return 0, false
	}
}
