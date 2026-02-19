package generator

import (
	"strings"
	"testing"
)

func TestOpenAPIFromValues_EmptyMap(t *testing.T) {
	result := GenerateOpenAPISchema(map[string]interface{}{})

	if !strings.Contains(result, "type: object") {
		t.Error("Expected 'type: object' in schema")
	}
	if !strings.Contains(result, "properties:") {
		t.Error("Expected 'properties:' in schema")
	}
}

func TestOpenAPIFromValues_StringField(t *testing.T) {
	result := GenerateOpenAPISchema(map[string]interface{}{
		"name": "test",
	})

	if !strings.Contains(result, "name:") {
		t.Error("Expected 'name:' property in schema")
	}
	if !strings.Contains(result, "type: string") {
		t.Error("Expected 'type: string' for string field")
	}
}

func TestOpenAPIFromValues_IntField(t *testing.T) {
	result := GenerateOpenAPISchema(map[string]interface{}{
		"replicas": int64(3),
	})

	if !strings.Contains(result, "replicas:") {
		t.Error("Expected 'replicas:' property")
	}
	if !strings.Contains(result, "type: integer") {
		t.Error("Expected 'type: integer' for int field")
	}
}

func TestOpenAPIFromValues_BoolField(t *testing.T) {
	result := GenerateOpenAPISchema(map[string]interface{}{
		"enabled": true,
	})

	if !strings.Contains(result, "enabled:") {
		t.Error("Expected 'enabled:' property")
	}
	if !strings.Contains(result, "type: boolean") {
		t.Error("Expected 'type: boolean' for bool field")
	}
}

func TestOpenAPIFromValues_NestedMap(t *testing.T) {
	result := GenerateOpenAPISchema(map[string]interface{}{
		"auth": map[string]interface{}{
			"type": "dex",
		},
	})

	if !strings.Contains(result, "auth:") {
		t.Error("Expected 'auth:' property")
	}
	// Nested object should have its own type: object
	// Count occurrences of "type: object"
	count := strings.Count(result, "type: object")
	if count < 2 {
		t.Errorf("Expected at least 2 'type: object' (root + nested), got %d", count)
	}
}

func TestOpenAPIFromValues_Array(t *testing.T) {
	result := GenerateOpenAPISchema(map[string]interface{}{
		"zones": []interface{}{"eu-west-1a", "eu-west-1b"},
	})

	if !strings.Contains(result, "zones:") {
		t.Error("Expected 'zones:' property")
	}
	if !strings.Contains(result, "type: array") {
		t.Error("Expected 'type: array' for array field")
	}
}
