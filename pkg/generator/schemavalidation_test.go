package generator

// Test Plan: schemavalidation_test.go
//
// | #  | Test Name                                      | Category    | Input                                         | Expected Output                                 | Notes                               |
// |----|------------------------------------------------|-------------|-----------------------------------------------|-------------------------------------------------|-------------------------------------|
// | 1  | SimpleStringWithPattern                        | happy       | string property, pattern "^[a-z]+:[0-9]+$"   | JSON contains "pattern" key                     | baseline pattern rendering          |
// | 2  | EnumWithDescriptions                           | happy       | 3 enum values with descriptions               | JSON enum array has 3 elements                  | enum contract                       |
// | 3  | OneOfPolymorphicIngress                        | happy       | oneOf with nginx/istio branches               | JSON contains "oneOf"                           | polymorphic discriminator           |
// | 4  | ConditionalTLSRequired                         | happy       | if tls.enabled then secretName required       | JSON contains "if", "then", "required"          | conditional rules                   |
// | 5  | NestedObjectRequired                           | happy       | object with required fields list              | JSON contains "required"                        | required field enforcement          |
// | 6  | BuildSchemaFromValues_Roundtrip                | happy       | map values with multiple types                | GenerateAdvancedValuesSchema returns valid JSON  | roundtrip consistency               |
// | 7  | InjectReplacesExisting                         | happy       | chart with existing ValuesSchema              | changed=true, new schema stored                 | inject contract                     |
// | 8  | InjectNoChangeIdempotent                       | happy       | same opts injected twice                      | second inject changed=false                     | idempotent inject                   |
// | 9  | EmptyPropertiesMap                             | edge        | AdvancedSchemaOptions with no properties      | JSON contains minimal {"type":"object"}         | degenerate schema                   |
// | 10 | InvalidRegexPattern                            | error       | property with invalid regex in Pattern        | ValidateSchemaOptions returns at least 1 error  | validation catches bad regex        |
// | 11 | NilChartInject                                 | error       | nil chart pointer                             | error returned, no panic                        | nil safety                          |
// | 12 | MergePropertiesOverwrite                       | happy       | dst and src share a key                       | merged map uses src value for shared key        | merge semantics                     |
// | 13 | AdditionalPropertiesFalse                      | happy       | AdditionalProperties: pointer-to-false        | JSON contains "additionalProperties":false      | strict object schemas               |
// | 14 | NewStringProperty_ConvenienceConstructor        | happy       | description, pattern, minLen, maxLen          | returned *SchemaProperty fields match inputs    | constructor contract                |
// | 15 | NewEnumProperty_ConvenienceConstructor          | happy       | enumType, description, values, descriptions   | returned *SchemaProperty Enum and Type correct  | constructor contract                |

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func minInt(v int) *int { return &v }

func makeMinimalChart(name string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name:       name,
		ChartYAML:  "apiVersion: v2\nname: " + name + "\nversion: 0.1.0\n",
		ValuesYAML: "replicaCount: 1\n",
		Templates: map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\n",
		},
	}
}

func mustUnmarshalSchema(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("schema is not valid JSON: %v\ngot:\n%s", err, s)
	}
	return m
}

// ── 1. SimpleStringWithPattern ─────────────────────────────────────────────────

func TestSchemaValidation_SimpleStringWithPattern_RendersInJSON(t *testing.T) {
	pattern := `^[a-z]+:[0-9]+$`
	prop := NewStringProperty("image tag", pattern, nil, nil)

	opts := AdvancedSchemaOptions{
		Title: "test",
		Properties: map[string]*SchemaProperty{
			"imageTag": prop,
		},
	}

	out, err := GenerateAdvancedValuesSchema(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "pattern") {
		t.Errorf("expected 'pattern' key in generated schema, got:\n%s", out)
	}
	if !strings.Contains(out, pattern) {
		t.Errorf("expected pattern %q in generated schema, got:\n%s", pattern, out)
	}
}

// ── 2. EnumWithDescriptions ────────────────────────────────────────────────────

func TestSchemaValidation_EnumWithDescriptions_ProducesEnumArray(t *testing.T) {
	values := []interface{}{"debug", "info", "error"}
	descs := map[string]string{
		"debug": "verbose output",
		"info":  "normal output",
		"error": "errors only",
	}
	prop := NewEnumProperty(SchemaTypeString, "log level", values, descs)

	opts := AdvancedSchemaOptions{
		Title: "test",
		Properties: map[string]*SchemaProperty{
			"logLevel": prop,
		},
	}

	out, err := GenerateAdvancedValuesSchema(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := mustUnmarshalSchema(t, out)
	props, ok := m["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'properties' object in schema")
	}
	logLevelProp, ok := props["logLevel"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'logLevel' in properties")
	}
	enumArr, ok := logLevelProp["enum"].([]interface{})
	if !ok {
		t.Fatal("expected 'enum' array in logLevel property")
	}
	if len(enumArr) != 3 {
		t.Errorf("expected 3 enum values, got %d", len(enumArr))
	}
}

// ── 3. OneOfPolymorphicIngress ─────────────────────────────────────────────────

func TestSchemaValidation_OneOfPolymorphicIngress_ContainsOneOf(t *testing.T) {
	nginxBranch := &SchemaProperty{
		Type:        SchemaTypeObject,
		Description: "nginx ingress config",
		Properties: map[string]*SchemaProperty{
			"class": {Type: SchemaTypeString, Description: "nginx"},
		},
	}
	istioBranch := &SchemaProperty{
		Type:        SchemaTypeObject,
		Description: "istio ingress config",
		Properties: map[string]*SchemaProperty{
			"gateway": {Type: SchemaTypeString, Description: "istio-gateway"},
		},
	}
	ingressProp := &SchemaProperty{
		Type:        SchemaTypeObject,
		Description: "ingress configuration",
		OneOf:       []*SchemaProperty{nginxBranch, istioBranch},
	}

	opts := AdvancedSchemaOptions{
		Title: "ingress-test",
		Properties: map[string]*SchemaProperty{
			"ingress": ingressProp,
		},
	}

	out, err := GenerateAdvancedValuesSchema(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "oneOf") {
		t.Errorf("expected 'oneOf' in generated schema, got:\n%s", out)
	}
}

// ── 4. ConditionalTLSRequired ──────────────────────────────────────────────────

func TestSchemaValidation_ConditionalTLSRequired_ContainsIfThenRequired(t *testing.T) {
	rule := ConditionalRule{
		If: &SubSchema{
			Properties: map[string]*SchemaProperty{
				"enabled": {Type: SchemaTypeBoolean, Default: true},
			},
		},
		Then: &SubSchema{
			Required: []string{"secretName"},
		},
	}

	tlsProp := &SchemaProperty{
		Type:        SchemaTypeObject,
		Description: "TLS configuration",
		Properties: map[string]*SchemaProperty{
			"enabled":    {Type: SchemaTypeBoolean, Description: "enable TLS"},
			"secretName": {Type: SchemaTypeString, Description: "TLS secret name"},
		},
		Conditionals: []ConditionalRule{rule},
	}

	opts := AdvancedSchemaOptions{
		Title: "tls-test",
		Properties: map[string]*SchemaProperty{
			"tls": tlsProp,
		},
	}

	out, err := GenerateAdvancedValuesSchema(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, keyword := range []string{"if", "then", "required"} {
		if !strings.Contains(out, keyword) {
			t.Errorf("expected keyword %q in generated schema, got:\n%s", keyword, out)
		}
	}
}

// ── 5. NestedObjectRequired ────────────────────────────────────────────────────

func TestSchemaValidation_NestedObjectRequired_EnforcesRequiredFields(t *testing.T) {
	opts := AdvancedSchemaOptions{
		Title:    "nested-test",
		Required: []string{"name", "namespace"},
		Properties: map[string]*SchemaProperty{
			"name":      {Type: SchemaTypeString, Description: "resource name"},
			"namespace": {Type: SchemaTypeString, Description: "resource namespace"},
		},
	}

	out, err := GenerateAdvancedValuesSchema(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "required") {
		t.Errorf("expected 'required' keyword in schema, got:\n%s", out)
	}

	m := mustUnmarshalSchema(t, out)
	reqRaw, ok := m["required"]
	if !ok {
		t.Fatal("expected 'required' field in JSON schema root")
	}
	reqArr, ok := reqRaw.([]interface{})
	if !ok {
		t.Fatal("expected 'required' to be an array")
	}
	if len(reqArr) != 2 {
		t.Errorf("expected 2 required fields, got %d", len(reqArr))
	}
}

// ── 6. BuildSchemaFromValues_Roundtrip ────────────────────────────────────────

func TestSchemaValidation_BuildSchemaFromValues_Roundtrip(t *testing.T) {
	values := map[string]interface{}{
		"replicaCount": 3,
		"image":        "nginx:latest",
		"enabled":      true,
	}

	opts := BuildSchemaFromValues(values, "roundtrip-test")

	out, err := GenerateAdvancedValuesSchema(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected non-empty schema output")
	}

	// Must produce valid JSON.
	mustUnmarshalSchema(t, out)

	// Must contain entries for the provided values.
	for _, key := range []string{"replicaCount", "image", "enabled"} {
		if !strings.Contains(out, key) {
			t.Errorf("expected key %q in roundtrip schema, got:\n%s", key, out)
		}
	}
}

// ── 7. InjectReplacesExisting ─────────────────────────────────────────────────

func TestSchemaValidation_InjectReplacesExisting_ChangedTrue(t *testing.T) {
	chart := makeMinimalChart("inject-test")
	chart.ValuesSchema = `{"type":"object","properties":{}}`

	opts := AdvancedSchemaOptions{
		Title: "new-schema",
		Properties: map[string]*SchemaProperty{
			"timeout": {Type: SchemaTypeInteger, Description: "request timeout"},
		},
	}

	updated, changed, err := InjectAdvancedValuesSchema(chart, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Error("expected changed=true when schema content differs")
	}
	if updated == nil {
		t.Fatal("expected non-nil updated chart")
	}
	if !strings.Contains(updated.ValuesSchema, "timeout") {
		t.Errorf("expected 'timeout' in updated ValuesSchema, got:\n%s", updated.ValuesSchema)
	}
}

// ── 8. InjectNoChangeIdempotent ───────────────────────────────────────────────

func TestSchemaValidation_InjectNoChangeIdempotent_SecondInjectFalse(t *testing.T) {
	chart := makeMinimalChart("idempotent-test")
	opts := AdvancedSchemaOptions{
		Title: "stable-schema",
		Properties: map[string]*SchemaProperty{
			"port": {Type: SchemaTypeInteger, Description: "service port"},
		},
	}

	first, changed1, err := InjectAdvancedValuesSchema(chart, opts)
	if err != nil {
		t.Fatalf("first inject unexpected error: %v", err)
	}
	if !changed1 {
		t.Error("expected changed=true on first inject")
	}

	_, changed2, err := InjectAdvancedValuesSchema(first, opts)
	if err != nil {
		t.Fatalf("second inject unexpected error: %v", err)
	}
	if changed2 {
		t.Error("expected changed=false on idempotent second inject")
	}
}

// ── 9. EmptyPropertiesMap ─────────────────────────────────────────────────────

func TestSchemaValidation_EmptyPropertiesMap_MinimalObjectSchema(t *testing.T) {
	opts := AdvancedSchemaOptions{
		Title:      "empty",
		Properties: map[string]*SchemaProperty{},
	}

	out, err := GenerateAdvancedValuesSchema(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := mustUnmarshalSchema(t, out)
	if m["type"] != "object" {
		t.Errorf(`expected "type":"object" in minimal schema, got type=%v`, m["type"])
	}
}

// ── 10. InvalidRegexPattern ───────────────────────────────────────────────────

func TestSchemaValidation_InvalidRegexPattern_ValidateReturnsError(t *testing.T) {
	opts := AdvancedSchemaOptions{
		Title: "bad-pattern",
		Properties: map[string]*SchemaProperty{
			"field": {
				Type:    SchemaTypeString,
				Pattern: "[invalid(regex",
			},
		},
	}

	errs := ValidateSchemaOptions(opts)
	if len(errs) == 0 {
		t.Error("expected ValidateSchemaOptions to return at least 1 error for invalid regex pattern, got none")
	}
}

// ── 11. NilChartInject ────────────────────────────────────────────────────────

func TestSchemaValidation_NilChartInject_ReturnsErrorNoPanic(t *testing.T) {
	opts := AdvancedSchemaOptions{
		Title:      "nil-test",
		Properties: map[string]*SchemaProperty{},
	}

	result, changed, err := InjectAdvancedValuesSchema(nil, opts)
	if err == nil {
		t.Error("expected error for nil chart, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on error, got %+v", result)
	}
	if changed {
		t.Error("expected changed=false when chart is nil")
	}
}

// ── 12. MergePropertiesOverwrite ──────────────────────────────────────────────

func TestSchemaValidation_MergePropertiesOverwrite_SharedKeyReplaced(t *testing.T) {
	dst := map[string]*SchemaProperty{
		"shared": {Type: SchemaTypeString, Description: "original"},
		"only-dst": {Type: SchemaTypeInteger, Description: "only in dst"},
	}
	src := map[string]*SchemaProperty{
		"shared": {Type: SchemaTypeBoolean, Description: "overwritten"},
		"only-src": {Type: SchemaTypeNumber, Description: "only in src"},
	}

	merged := MergeSchemaProperties(dst, src)

	if len(merged) != 3 {
		t.Errorf("expected 3 keys after merge, got %d", len(merged))
	}
	if merged["shared"].Type != SchemaTypeBoolean {
		t.Errorf("expected shared key to be overwritten with SchemaTypeBoolean, got %v", merged["shared"].Type)
	}
	if _, ok := merged["only-dst"]; !ok {
		t.Error("expected 'only-dst' key to be preserved in merged map")
	}
	if _, ok := merged["only-src"]; !ok {
		t.Error("expected 'only-src' key to be present in merged map")
	}
}

// ── 13. AdditionalPropertiesFalse ─────────────────────────────────────────────

func TestSchemaValidation_AdditionalPropertiesFalse_RendersCorrectly(t *testing.T) {
	additionalFalse := false
	opts := AdvancedSchemaOptions{
		Title:                "strict-object",
		AdditionalProperties: &additionalFalse,
		Properties: map[string]*SchemaProperty{
			"name": {Type: SchemaTypeString, Description: "strict name"},
		},
	}

	out, err := GenerateAdvancedValuesSchema(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := mustUnmarshalSchema(t, out)
	ap, ok := m["additionalProperties"]
	if !ok {
		t.Fatal("expected 'additionalProperties' in generated schema")
	}
	apBool, ok := ap.(bool)
	if !ok {
		t.Fatalf("expected additionalProperties to be bool, got %T", ap)
	}
	if apBool {
		t.Error("expected additionalProperties=false, got true")
	}
}

// ── 14. NewStringProperty_ConvenienceConstructor ───────────────────────────────

func TestSchemaValidation_NewStringProperty_ConvenienceConstructor(t *testing.T) {
	desc := "an image tag"
	pattern := `^[a-z0-9]+$`
	minLen := minInt(1)
	maxLen := minInt(128)

	prop := NewStringProperty(desc, pattern, minLen, maxLen)

	if prop == nil {
		t.Fatal("NewStringProperty returned nil")
	}
	if prop.Type != SchemaTypeString {
		t.Errorf("expected Type=SchemaTypeString, got %v", prop.Type)
	}
	if prop.Description != desc {
		t.Errorf("expected Description=%q, got %q", desc, prop.Description)
	}
	if prop.Pattern != pattern {
		t.Errorf("expected Pattern=%q, got %q", pattern, prop.Pattern)
	}
	if prop.MinLength == nil || *prop.MinLength != 1 {
		t.Errorf("expected MinLength=1, got %v", prop.MinLength)
	}
	if prop.MaxLength == nil || *prop.MaxLength != 128 {
		t.Errorf("expected MaxLength=128, got %v", prop.MaxLength)
	}
}

// ── 15. NewEnumProperty_ConvenienceConstructor ────────────────────────────────

func TestSchemaValidation_NewEnumProperty_ConvenienceConstructor(t *testing.T) {
	values := []interface{}{"ClusterIP", "NodePort", "LoadBalancer"}
	descs := map[string]string{
		"ClusterIP":    "internal only",
		"NodePort":     "node-level exposure",
		"LoadBalancer": "external load balancer",
	}

	prop := NewEnumProperty(SchemaTypeString, "service type", values, descs)

	if prop == nil {
		t.Fatal("NewEnumProperty returned nil")
	}
	if prop.Type != SchemaTypeString {
		t.Errorf("expected Type=SchemaTypeString, got %v", prop.Type)
	}
	if prop.Description != "service type" {
		t.Errorf("expected Description=%q, got %q", "service type", prop.Description)
	}
	if len(prop.Enum) != 3 {
		t.Errorf("expected 3 enum values, got %d", len(prop.Enum))
	}
	// All provided values must appear in Enum.
	enumSet := make(map[interface{}]bool, len(prop.Enum))
	for _, v := range prop.Enum {
		enumSet[v] = true
	}
	for _, v := range values {
		if !enumSet[v] {
			t.Errorf("expected enum value %v in property, not found", v)
		}
	}
}
