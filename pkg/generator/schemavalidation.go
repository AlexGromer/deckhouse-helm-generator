package generator

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// SchemaType represents a JSON Schema type string.
type SchemaType string

const (
	SchemaTypeString  SchemaType = "string"
	SchemaTypeInteger SchemaType = "integer"
	SchemaTypeNumber  SchemaType = "number"
	SchemaTypeBoolean SchemaType = "boolean"
	SchemaTypeObject  SchemaType = "object"
	SchemaTypeArray   SchemaType = "array"
)

// SubSchema represents a sub-schema (e.g. for oneOf/anyOf/allOf entries).
type SubSchema struct {
	Type       SchemaType
	Properties map[string]*SchemaProperty
	Required   []string
	Const      interface{}
}

// ConditionalRule represents a JSON Schema if/then/else conditional.
type ConditionalRule struct {
	If   *SubSchema
	Then *SubSchema
	Else *SubSchema
}

// SchemaProperty represents a JSON Schema property definition.
type SchemaProperty struct {
	Name         string
	Type         SchemaType
	Description  string
	Required     bool
	Enum         []interface{}
	Default      interface{}
	Properties   map[string]*SchemaProperty
	Items        *SchemaProperty
	Metadata     map[string]string
	OneOf        []*SchemaProperty
	Conditionals []ConditionalRule
	Pattern      string
	MinLength    *int
	MaxLength    *int
	Minimum      *float64
	Maximum      *float64
}

// NewStringProperty creates a simple string *SchemaProperty.
// Signature: (description, pattern string, minLength, maxLength *int)
func NewStringProperty(description, pattern string, minLength, maxLength *int) *SchemaProperty {
	p := &SchemaProperty{
		Type:        SchemaTypeString,
		Description: description,
		Pattern:     pattern,
		MinLength:   minLength,
		MaxLength:   maxLength,
	}
	return p
}

// NewEnumProperty creates a property with allowed enum values.
// descriptions maps each enum value to a human-readable description (optional, may be nil).
func NewEnumProperty(schemaType SchemaType, description string, values []interface{}, descriptions map[string]string) *SchemaProperty {
	return &SchemaProperty{
		Type:        schemaType,
		Description: description,
		Enum:        values,
		Metadata:    descriptions,
	}
}

// AdvancedSchemaOptions configures advanced values schema generation.
type AdvancedSchemaOptions struct {
	Title                string
	Description          string
	Properties           map[string]*SchemaProperty
	Required             []string
	AdditionalProperties *bool
}

// ValidateSchemaOptions validates AdvancedSchemaOptions and returns a list of errors.
func ValidateSchemaOptions(opts AdvancedSchemaOptions) []error {
	var errs []error
	for name, prop := range opts.Properties {
		if prop == nil {
			continue
		}
		if prop.Pattern != "" {
			if _, err := regexp.Compile(prop.Pattern); err != nil {
				errs = append(errs, fmt.Errorf("property %q has invalid regex pattern %q: %w", name, prop.Pattern, err))
			}
		}
	}
	return errs
}

// MergeSchemaProperties merges two property maps (b into a), returning the merged map.
func MergeSchemaProperties(a, b map[string]*SchemaProperty) map[string]*SchemaProperty {
	result := make(map[string]*SchemaProperty, len(a)+len(b))
	for k, v := range a {
		result[k] = v
	}
	for k, v := range b {
		result[k] = v
	}
	return result
}

// subSchemaToMap converts a *SubSchema to a JSON-compatible map.
func subSchemaToMap(s *SubSchema) map[string]interface{} {
	if s == nil {
		return nil
	}
	m := map[string]interface{}{}
	if s.Type != "" {
		m["type"] = string(s.Type)
	}
	if len(s.Required) > 0 {
		m["required"] = s.Required
	}
	if s.Const != nil {
		m["const"] = s.Const
	}
	if len(s.Properties) > 0 {
		props := make(map[string]interface{}, len(s.Properties))
		for k, child := range s.Properties {
			props[k] = schemaPropertyToMap(child)
		}
		m["properties"] = props
	}
	return m
}

// schemaPropertyToMap converts a *SchemaProperty to a JSON-compatible map.
func schemaPropertyToMap(p *SchemaProperty) map[string]interface{} {
	if p == nil {
		return map[string]interface{}{}
	}
	m := map[string]interface{}{
		"type": string(p.Type),
	}
	if p.Description != "" {
		m["description"] = p.Description
	}
	if len(p.Enum) > 0 {
		m["enum"] = p.Enum
	}
	if p.Default != nil {
		m["default"] = p.Default
	}
	if p.Pattern != "" {
		m["pattern"] = p.Pattern
	}
	if p.MinLength != nil {
		m["minLength"] = *p.MinLength
	}
	if p.MaxLength != nil {
		m["maxLength"] = *p.MaxLength
	}
	if p.Minimum != nil {
		m["minimum"] = *p.Minimum
	}
	if p.Maximum != nil {
		m["maximum"] = *p.Maximum
	}
	if p.Items != nil {
		m["items"] = schemaPropertyToMap(p.Items)
	}
	if len(p.Properties) > 0 {
		props := make(map[string]interface{}, len(p.Properties))
		for k, child := range p.Properties {
			props[k] = schemaPropertyToMap(child)
		}
		m["properties"] = props
	}
	if len(p.OneOf) > 0 {
		oneOf := make([]interface{}, 0, len(p.OneOf))
		for _, sub := range p.OneOf {
			oneOf = append(oneOf, schemaPropertyToMap(sub))
		}
		m["oneOf"] = oneOf
	}
	// Render if/then/else conditionals (only first rule for now; JSON Schema merges via allOf if multiple)
	if len(p.Conditionals) > 0 {
		if len(p.Conditionals) == 1 {
			rule := p.Conditionals[0]
			if rule.If != nil {
				m["if"] = subSchemaToMap(rule.If)
			}
			if rule.Then != nil {
				m["then"] = subSchemaToMap(rule.Then)
			}
			if rule.Else != nil {
				m["else"] = subSchemaToMap(rule.Else)
			}
		} else {
			// Multiple conditionals → wrap each in allOf
			allOf := make([]interface{}, 0, len(p.Conditionals))
			for _, rule := range p.Conditionals {
				cond := map[string]interface{}{}
				if rule.If != nil {
					cond["if"] = subSchemaToMap(rule.If)
				}
				if rule.Then != nil {
					cond["then"] = subSchemaToMap(rule.Then)
				}
				if rule.Else != nil {
					cond["else"] = subSchemaToMap(rule.Else)
				}
				allOf = append(allOf, cond)
			}
			m["allOf"] = allOf
		}
	}
	return m
}

// GenerateAdvancedValuesSchema generates a JSON Schema string for Helm values validation.
func GenerateAdvancedValuesSchema(opts AdvancedSchemaOptions) (string, error) {
	schema := map[string]interface{}{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type":    "object",
	}
	if opts.Title != "" {
		schema["title"] = opts.Title
	}
	if opts.Description != "" {
		schema["description"] = opts.Description
	}
	if opts.AdditionalProperties != nil {
		schema["additionalProperties"] = *opts.AdditionalProperties
	}
	if len(opts.Properties) > 0 {
		props := make(map[string]interface{}, len(opts.Properties))
		for k, p := range opts.Properties {
			props[k] = schemaPropertyToMap(p)
		}
		schema["properties"] = props
	}
	if len(opts.Required) > 0 {
		schema["required"] = opts.Required
	}

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal schema: %w", err)
	}
	return strings.TrimSpace(string(data)) + "\n", nil
}

// BuildSchemaFromValues infers a JSON Schema string from an existing values map.
func BuildSchemaFromValues(values map[string]interface{}, title string) AdvancedSchemaOptions {
	props := make(map[string]*SchemaProperty, len(values))
	for k, v := range values {
		var schemaType SchemaType
		switch v.(type) {
		case int, int64, float64:
			schemaType = SchemaTypeNumber
		case bool:
			schemaType = SchemaTypeBoolean
		case map[string]interface{}:
			schemaType = SchemaTypeObject
		case []interface{}:
			schemaType = SchemaTypeArray
		default:
			schemaType = SchemaTypeString
		}
		props[k] = &SchemaProperty{Name: k, Type: schemaType}
	}
	return AdvancedSchemaOptions{
		Title:      title,
		Properties: props,
	}
}

// InjectAdvancedValuesSchema injects a generated JSON Schema into the chart's ValuesSchema field.
// Returns the updated chart, a boolean indicating whether the schema changed, and any error.
func InjectAdvancedValuesSchema(chart *types.GeneratedChart, opts AdvancedSchemaOptions) (*types.GeneratedChart, bool, error) {
	if chart == nil {
		return nil, false, fmt.Errorf("chart is nil")
	}
	schema, err := GenerateAdvancedValuesSchema(opts)
	if err != nil {
		return nil, false, err
	}
	// Idempotency: compare normalized JSON to avoid spurious changes.
	if strings.TrimSpace(chart.ValuesSchema) == strings.TrimSpace(schema) {
		result := copyChartTemplates(chart)
		return result, false, nil
	}
	result := copyChartTemplates(chart)
	result.ValuesSchema = schema
	return result, true, nil
}
