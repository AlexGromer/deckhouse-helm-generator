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
		sb.WriteString(fmt.Sprintf("%s%s:\n", prefix, key))

		switch v := val.(type) {
		case map[string]interface{}:
			sb.WriteString(fmt.Sprintf("%s  type: object\n", prefix))
			sb.WriteString(fmt.Sprintf("%s  properties:\n", prefix))
			writeProperties(sb, v, indent+4)
		case []interface{}:
			sb.WriteString(fmt.Sprintf("%s  type: array\n", prefix))
			sb.WriteString(fmt.Sprintf("%s  items:\n", prefix))
			if len(v) > 0 {
				sb.WriteString(fmt.Sprintf("%s    type: %s\n", prefix, inferType(v[0])))
			} else {
				sb.WriteString(fmt.Sprintf("%s    type: string\n", prefix))
			}
		default:
			sb.WriteString(fmt.Sprintf("%s  type: %s\n", prefix, inferType(val)))
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
