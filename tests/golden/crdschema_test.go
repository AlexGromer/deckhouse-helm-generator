package golden

import (
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"
)

//go:embed testdata/crds/*.json
var crdSchemasFS embed.FS

var crdSchemas map[string]map[string]interface{}

func init() {
	crdSchemas = make(map[string]map[string]interface{})
	entries, err := crdSchemasFS.ReadDir("testdata/crds")
	if err != nil {
		panic(err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := crdSchemasFS.ReadFile(path.Join("testdata/crds", entry.Name()))
		if err != nil {
			panic(err)
		}
		var schema map[string]interface{}
		if err := json.Unmarshal(data, &schema); err != nil {
			panic(err)
		}

		// The filename is <group>_<version>_<kind>.json
		parts := strings.Split(strings.TrimSuffix(entry.Name(), ".json"), "_")
		if len(parts) >= 3 {
			group := parts[0]
			version := parts[1]
			kind := parts[2]
			key := fmt.Sprintf("%s/%s/%s", group, version, kind)
			crdSchemas[key] = schema
		}
	}
}

func checkCRSchemas(objs []map[string]interface{}) []string {
	var problems []string
	for _, obj := range objs {
		apiVersion, ok1 := obj["apiVersion"].(string)
		kind, ok2 := obj["kind"].(string)
		if !ok1 || !ok2 {
			continue
		}
		parts := strings.Split(apiVersion, "/")
		if len(parts) != 2 {
			continue // core group
		}
		key := fmt.Sprintf("%s/%s/%s", parts[0], parts[1], kind)
		schema, ok := crdSchemas[key]
		if !ok {
			continue
		}

		// Only check spec if it exists, metadata is skipped as per specs
		spec, ok := obj["spec"]
		if !ok {
			continue
		}

		schemaSpec := schema
		if props, ok := schema["properties"].(map[string]interface{}); ok {
			if s, ok := props["spec"].(map[string]interface{}); ok {
				schemaSpec = s
			}
		}

		errs := validateAgainstSchema("spec", spec, schemaSpec)
		for _, err := range errs {
			source := getSource(obj)
			if source != "" {
				problems = append(problems, fmt.Sprintf("%s: %s", source, err))
			} else {
				problems = append(problems, fmt.Sprintf("%s/%s: %s", apiVersion, kind, err))
			}
		}
	}
	return problems
}

func getSource(obj map[string]interface{}) string {
	if meta, ok := obj["metadata"].(map[string]interface{}); ok {
		if ann, ok := meta["annotations"].(map[string]interface{}); ok {
			if src, ok := ann["dhg.deckhouse.io/source"].(string); ok {
				return src
			}
		}
	}
	return ""
}

func validateAgainstSchema(path string, val interface{}, schema map[string]interface{}) []string {
	if val == nil {
		return nil
	}
	if schema == nil {
		return nil
	}

	// Handle anyOf/oneOf
	if anyOf, ok := schema["anyOf"].([]interface{}); ok && len(anyOf) > 0 {
		var anyErrs []string
		validOne := false
		for _, s := range anyOf {
			if sMap, ok := s.(map[string]interface{}); ok {
				errs := validateAgainstSchema(path, val, sMap)
				if len(errs) == 0 {
					validOne = true
					break
				}
				anyErrs = append(anyErrs, errs...)
			}
		}
		if !validOne {
			// Simplified error for anyOf failure to avoid noise
			return []string{fmt.Sprintf("%s: does not match anyOf", path)}
		}
		return nil
	}
	if oneOf, ok := schema["oneOf"].([]interface{}); ok && len(oneOf) > 0 {
		var oneOfErrs []string
		validOne := false
		for _, s := range oneOf {
			if sMap, ok := s.(map[string]interface{}); ok {
				errs := validateAgainstSchema(path, val, sMap)
				if len(errs) == 0 {
					validOne = true
					break
				}
				oneOfErrs = append(oneOfErrs, errs...)
			}
		}
		if !validOne {
			return []string{fmt.Sprintf("%s: does not match oneOf", path)}
		}
		return nil
	}

	if xK8sIntOrString, _ := schema["x-kubernetes-int-or-string"].(bool); xK8sIntOrString {
		switch val.(type) {
		case string, int, int32, int64, float64:
			return nil
		default:
			return []string{fmt.Sprintf("%s: expected int or string, got %T", path, val)}
		}
	}

	if preserve, _ := schema["x-kubernetes-preserve-unknown-fields"].(bool); preserve {
		return nil
	}

	typ, _ := schema["type"].(string)

	var errs []string

	switch typ {
	case "object":
		vMap, ok := val.(map[string]interface{})
		if !ok {
			return []string{fmt.Sprintf("%s: expected object, got %T", path, val)}
		}

		props, hasProps := schema["properties"].(map[string]interface{})

		addProps, hasAddProps := schema["additionalProperties"]
		addPropsSchema, isAddPropsSchema := addProps.(map[string]interface{})
		addPropsBool, isAddPropsBool := addProps.(bool)

		if !hasAddProps {
			addPropsBool = true // Default is true in OpenAPI 3
			isAddPropsBool = true
		}

		for k, v := range vMap {
			var pSchema map[string]interface{}
			if hasProps {
				if p, ok := props[k].(map[string]interface{}); ok {
					pSchema = p
				}
			}

			if pSchema == nil {
				if isAddPropsSchema {
					pSchema = addPropsSchema
				} else if isAddPropsBool && !addPropsBool {
					errs = append(errs, fmt.Sprintf("%s.%s: field not in schema", path, k))
					continue
				} else {
					continue // Allowed additional properties without schema
				}
			}

			if pSchema != nil {
				errs = append(errs, validateAgainstSchema(fmt.Sprintf("%s.%s", path, k), v, pSchema)...)
			}
		}

		if req, ok := schema["required"].([]interface{}); ok {
			for _, r := range req {
				rStr, _ := r.(string)
				if _, ok := vMap[rStr]; !ok {
					errs = append(errs, fmt.Sprintf("%s: missing required field %s", path, rStr))
				}
			}
		}

	case "array":
		vList, ok := val.([]interface{})
		if !ok {
			return []string{fmt.Sprintf("%s: expected array, got %T", path, val)}
		}
		if maxItemsF, ok := schema["maxItems"].(float64); ok {
			if len(vList) > int(maxItemsF) {
				errs = append(errs, fmt.Sprintf("%s: array length %d exceeds maxItems %d", path, len(vList), int(maxItemsF)))
			}
		}
		if itemsSchema, ok := schema["items"].(map[string]interface{}); ok {
			for i, v := range vList {
				errs = append(errs, validateAgainstSchema(fmt.Sprintf("%s[%d]", path, i), v, itemsSchema)...)
			}
		}

	case "string":
		vStr, ok := val.(string)
		if !ok {
			return []string{fmt.Sprintf("%s: expected string, got %T", path, val)}
		}
		if enum, ok := schema["enum"].([]interface{}); ok {
			found := false
			for _, e := range enum {
				if eStr, ok := e.(string); ok && eStr == vStr {
					found = true
					break
				}
			}
			if !found {
				errs = append(errs, fmt.Sprintf("%s: string '%s' not in enum %v", path, vStr, enum))
			}
		}
		if maxLenF, ok := schema["maxLength"].(float64); ok {
			if len(vStr) > int(maxLenF) {
				errs = append(errs, fmt.Sprintf("%s: string length %d exceeds maxLength %d", path, len(vStr), int(maxLenF)))
			}
		}
		if pattern, ok := schema["pattern"].(string); ok {
			re, err := regexp.Compile(pattern)
			if err == nil {
				if !re.MatchString(vStr) {
					errs = append(errs, fmt.Sprintf("%s: string '%s' does not match pattern '%s'", path, vStr, pattern))
				}
			}
		}

	case "integer":
		vNum, ok := val.(float64) // JSON unmarshals numbers to float64
		if !ok {
			return []string{fmt.Sprintf("%s: expected integer, got %T", path, val)}
		}
		if enum, ok := schema["enum"].([]interface{}); ok {
			found := false
			for _, e := range enum {
				if eNum, ok := e.(float64); ok && eNum == vNum {
					found = true
					break
				}
			}
			if !found {
				errs = append(errs, fmt.Sprintf("%s: integer %v not in enum %v", path, vNum, enum))
			}
		}
		if minF, ok := schema["minimum"].(float64); ok {
			if vNum < minF {
				errs = append(errs, fmt.Sprintf("%s: integer %v is less than minimum %v", path, vNum, minF))
			}
		}
		if maxF, ok := schema["maximum"].(float64); ok {
			if vNum > maxF {
				errs = append(errs, fmt.Sprintf("%s: integer %v is greater than maximum %v", path, vNum, maxF))
			}
		}

	case "number":
		vNum, ok := val.(float64) // JSON unmarshals numbers to float64
		if !ok {
			return []string{fmt.Sprintf("%s: expected number, got %T", path, val)}
		}
		if enum, ok := schema["enum"].([]interface{}); ok {
			found := false
			for _, e := range enum {
				if eNum, ok := e.(float64); ok && eNum == vNum {
					found = true
					break
				}
			}
			if !found {
				errs = append(errs, fmt.Sprintf("%s: number %v not in enum %v", path, vNum, enum))
			}
		}
		if minF, ok := schema["minimum"].(float64); ok {
			if vNum < minF {
				errs = append(errs, fmt.Sprintf("%s: number %v is less than minimum %v", path, vNum, minF))
			}
		}
		if maxF, ok := schema["maximum"].(float64); ok {
			if vNum > maxF {
				errs = append(errs, fmt.Sprintf("%s: number %v is greater than maximum %v", path, vNum, maxF))
			}
		}

	case "boolean":
		_, ok := val.(bool)
		if !ok {
			return []string{fmt.Sprintf("%s: expected boolean, got %T", path, val)}
		}
	}

	return errs
}

// ValidateCRDsAgainstSchemas validates rendered YAML objects against loaded CRD schemas.
// Exported for use in tests.
func ValidateCRDsAgainstSchemas(t interface {
	Errorf(format string, args ...interface{})
}, renderedYAML string) {
	objs := parseStreamForCRD(renderedYAML)
	errs := checkCRSchemas(objs)
	for _, err := range errs {
		t.Errorf(err)
	}
}

func parseStreamForCRD(stream string) []map[string]interface{} {
	var objs []map[string]interface{}
	for _, doc := range strings.Split(stream, "\n---") {
		var obj map[string]interface{}
		if err := yaml.Unmarshal([]byte(doc), &obj); err == nil && obj != nil {
			objs = append(objs, obj)
		}
	}
	return objs
}
