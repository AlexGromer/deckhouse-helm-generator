package golden

import (
	"testing"
)

func TestCRDSchemaSelf(t *testing.T) {
	// A simple test just to make sure logic is sound
	if len(crdSchemas) == 0 {
		t.Fatalf("Expected some schemas to be loaded")
	}
}

func TestValidateAgainstSchema_UnknownField(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"spec": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"rules": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"backendRefs": map[string]interface{}{
									"type": "array",
								},
							},
							"additionalProperties": false,
						},
					},
				},
			},
		},
	}

	val := map[string]interface{}{
		"spec": map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"backendRef": []interface{}{}, // Typo, should be backendRefs
				},
			},
		},
	}

	errs := validateAgainstSchema("root", val, schema)
	if len(errs) != 1 {
		t.Fatalf("Expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0] != "root.spec.rules[0].backendRef: field not in schema" {
		t.Errorf("Unexpected error message: %s", errs[0])
	}
}

func TestValidateAgainstSchema_Enum(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"spec": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"protocol": map[string]interface{}{
						"type": "string",
						"enum": []interface{}{"TCP", "UDP"},
					},
				},
			},
		},
	}

	val1 := map[string]interface{}{
		"spec": map[string]interface{}{
			"protocol": "ICMP",
		},
	}

	errs1 := validateAgainstSchema("root", val1, schema)
	if len(errs1) != 1 {
		t.Fatalf("Expected 1 error, got %v", errs1)
	}

	val2 := map[string]interface{}{
		"spec": map[string]interface{}{
			"protocol": "TCP",
		},
	}

	errs2 := validateAgainstSchema("root", val2, schema)
	if len(errs2) != 0 {
		t.Fatalf("Expected 0 errors, got %v", errs2)
	}
}

func TestValidateAgainstSchema_MaxItems(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"spec": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"rules": map[string]interface{}{
						"type":     "array",
						"maxItems": float64(16),
					},
				},
			},
		},
	}

	rules := make([]interface{}, 17)
	val := map[string]interface{}{
		"spec": map[string]interface{}{
			"rules": rules,
		},
	}

	errs := validateAgainstSchema("root", val, schema)
	if len(errs) != 1 {
		t.Fatalf("Expected 1 error, got %v", errs)
	}
}

func TestValidateAgainstSchema_Required(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"spec": map[string]interface{}{
				"type":     "object",
				"required": []interface{}{"rules"},
				"properties": map[string]interface{}{
					"rules": map[string]interface{}{
						"type": "array",
					},
				},
			},
		},
	}

	val := map[string]interface{}{
		"spec": map[string]interface{}{
			// missing rules
		},
	}

	errs := validateAgainstSchema("root", val, schema)
	if len(errs) != 1 {
		t.Fatalf("Expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0] != "root.spec: missing required field rules" {
		t.Errorf("Unexpected error message: %s", errs[0])
	}
}

func TestValidateAgainstSchema_HostnamesTypo(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"spec": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"hostnames": map[string]interface{}{
						"type": "array",
					},
				},
			},
		},
	}

	val := map[string]interface{}{
		"spec": map[string]interface{}{
			"hostNames": []interface{}{"example.com"}, // Typo here (hostNames instead of hostnames)
		},
	}

	errs := validateAgainstSchema("root", val, schema)
	if len(errs) != 1 {
		t.Fatalf("Expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0] != "root.spec.hostNames: field not in schema" {
		t.Errorf("Unexpected error message: %s", errs[0])
	}
}
