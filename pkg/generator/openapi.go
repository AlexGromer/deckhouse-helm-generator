package generator

import (
	"fmt"
	"sort"
	"strings"
)

// GenerateOpenAPISchema generates an OpenAPI v3 schema YAML from values map.
func GenerateOpenAPISchema(values map[string]interface{}) string {
	var sb strings.Builder
	sb.WriteString("type: object\n")
	if len(values) == 0 {
		sb.WriteString("properties: {}\n")
		return sb.String()
	}
	sb.WriteString("properties:\n")
	writeProperties(&sb, values, 2)
	return sb.String()
}

func writeProperties(sb *strings.Builder, values map[string]interface{}, indent int) {
	// Sort keys for deterministic output
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	prefix := strings.Repeat(" ", indent)

	for _, key := range keys {
		val := values[key]
		fmt.Fprintf(sb, "%s%s:\n", prefix, key)

		switch v := val.(type) {
		case map[string]interface{}:
			fmt.Fprintf(sb, "%s  type: object\n", prefix)
			if len(v) == 0 {
				fmt.Fprintf(sb, "%s  properties: {}\n", prefix)
				continue
			}
			fmt.Fprintf(sb, "%s  properties:\n", prefix)
			writeProperties(sb, v, indent+4)
		case []interface{}:
			fmt.Fprintf(sb, "%s  type: array\n", prefix)
			fmt.Fprintf(sb, "%s  items:\n", prefix)
			if len(v) > 0 {
				fmt.Fprintf(sb, "%s    type: %s\n", prefix, inferType(v[0]))
			} else {
				fmt.Fprintf(sb, "%s    type: string\n", prefix)
			}
		default:
			fmt.Fprintf(sb, "%s  type: %s\n", prefix, inferType(val))
		}
	}
}

func inferType(val interface{}) string {
	switch val.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case int, int32, int64:
		return "integer"
	case float32, float64:
		return "number"
	default:
		return "string"
	}
}
