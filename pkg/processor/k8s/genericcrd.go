package k8s

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// GenericCRDProcessor processes any unknown CRD resource as a fallback.
// It extracts GVK, spec fields, and status fields using generic map access.
type GenericCRDProcessor struct {
	processor.BaseProcessor
}

// NewGenericCRDProcessor creates a new generic CRD processor.
// It is registered with the lowest priority (1) so it only handles
// resources that no other processor claims.
func NewGenericCRDProcessor() *GenericCRDProcessor {
	return &GenericCRDProcessor{
		BaseProcessor: processor.NewBaseProcessor("genericcrd", 1),
	}
}

// Supports returns an empty list — this processor is a fallback and does not
// declare specific GVKs. It is invoked by the registry's processGeneric path
// or can be called explicitly after all other processors decline.
func (p *GenericCRDProcessor) Supports() []schema.GroupVersionKind {
	return nil
}

// Process processes any CRD resource generically.
func (p *GenericCRDProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, fmt.Errorf("generic CRD object is nil")
	}

	gvk := obj.GroupVersionKind()
	serviceName := processor.SanitizeServiceName(processor.ServiceNameFromResource(obj))
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	values := map[string]interface{}{
		"enabled": true,
		"gvk": map[string]interface{}{
			"group":   gvk.Group,
			"version": gvk.Version,
			"kind":    gvk.Kind,
		},
	}

	// Extract spec fields
	if spec, found, _ := unstructured.NestedMap(obj.Object, "spec"); found {
		values["spec"] = spec
	}

	// Extract status fields
	if status, found, _ := unstructured.NestedMap(obj.Object, "status"); found {
		values["status"] = status
	}

	// Extract any spec.versions[].schema.openAPIV3Schema for CRD definitions
	valuesSchema := extractCRDValuesSchema(obj)
	if valuesSchema != nil {
		values["valuesSchema"] = valuesSchema
	}

	template := p.generateTemplate(ctx, obj, serviceName)
	kindLower := strings.ToLower(gvk.Kind)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-%s.yaml", serviceName, kindLower),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.%s", serviceName, kindLower),
		Values:          values,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"gvk":       gvk.String(),
			"isCRD":     true,
		},
	}, nil
}

// generateTemplate creates a Helm template for the generic CRD.
func (p *GenericCRDProcessor) generateTemplate(ctx processor.Context, obj *unstructured.Unstructured, serviceName string) string {
	gvk := obj.GroupVersionKind()
	kindLower := strings.ToLower(gvk.Kind)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("{{- $svc := .Values.services.%s -}}\n", serviceName))
	b.WriteString("{{- if $svc.enabled }}\n")
	b.WriteString(fmt.Sprintf("{{- with $svc.%s }}\n", kindLower))
	b.WriteString(fmt.Sprintf("{{- if .enabled }}\n"))
	b.WriteString(fmt.Sprintf("apiVersion: %s\n", obj.GetAPIVersion()))
	b.WriteString(fmt.Sprintf("kind: %s\n", gvk.Kind))
	b.WriteString("metadata:\n")
	b.WriteString(fmt.Sprintf("  name: {{ include \"%s.fullname\" $ }}-%s\n", ctx.ChartName, obj.GetName()))

	if obj.GetNamespace() != "" {
		b.WriteString("  namespace: {{ $.Release.Namespace }}\n")
	}

	b.WriteString("  labels:\n")
	b.WriteString(fmt.Sprintf("    {{- include \"%s.labels\" $ | nindent 4 }}\n", ctx.ChartName))

	// Annotations
	b.WriteString("  {{- with .annotations }}\n")
	b.WriteString("  annotations:\n")
	b.WriteString("    {{- toYaml . | nindent 4 }}\n")
	b.WriteString("  {{- end }}\n")

	// Spec
	b.WriteString("{{- with .spec }}\n")
	b.WriteString("spec:\n")
	b.WriteString("  {{- toYaml . | nindent 2 }}\n")
	b.WriteString("{{- end }}\n")

	b.WriteString("{{- end }}\n")
	b.WriteString("{{- end }}\n")
	b.WriteString("{{- end }}\n")

	return b.String()
}

// extractCRDValuesSchema extracts OpenAPI v3 schema from a CRD's spec.versions[].schema.
// This applies when the resource itself IS a CustomResourceDefinition.
func extractCRDValuesSchema(obj *unstructured.Unstructured) map[string]interface{} {
	if obj.GetKind() != "CustomResourceDefinition" {
		return nil
	}

	versions, found, _ := unstructured.NestedSlice(obj.Object, "spec", "versions")
	if !found || len(versions) == 0 {
		return nil
	}

	result := make(map[string]interface{})

	for _, v := range versions {
		version, ok := v.(map[string]interface{})
		if !ok {
			continue
		}

		versionName, _ := version["name"].(string)
		if versionName == "" {
			continue
		}

		schemaObj, found, _ := unstructured.NestedMap(version, "schema", "openAPIV3Schema")
		if !found {
			continue
		}

		// Parse nested properties into a flat values structure
		parsed := parseOpenAPISchemaToValues(schemaObj, "")
		if len(parsed) > 0 {
			result[versionName] = parsed
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// parseOpenAPISchemaToValues converts an OpenAPI v3 schema into a nested values map.
// It recursively walks object properties, extracting defaults and structure.
func parseOpenAPISchemaToValues(schema map[string]interface{}, prefix string) map[string]interface{} {
	result := make(map[string]interface{})

	schemaType, _ := schema["type"].(string)

	properties, found, _ := unstructured.NestedMap(schema, "properties")
	if found && schemaType == "object" {
		for propName, propVal := range properties {
			propSchema, ok := propVal.(map[string]interface{})
			if !ok {
				continue
			}

			propType, _ := propSchema["type"].(string)

			// Use default if available
			if defaultVal, hasDefault := propSchema["default"]; hasDefault {
				result[propName] = defaultVal
				continue
			}

			switch propType {
			case "object":
				nested := parseOpenAPISchemaToValues(propSchema, prefix+propName+".")
				if len(nested) > 0 {
					result[propName] = nested
				} else {
					result[propName] = map[string]interface{}{}
				}
			case "array":
				result[propName] = []interface{}{}
			case "string":
				result[propName] = ""
			case "integer":
				result[propName] = int64(0)
			case "boolean":
				result[propName] = false
			case "number":
				result[propName] = float64(0)
			default:
				result[propName] = nil
			}
		}
	}

	return result
}

// GenerateCRDInstallFiles generates files for a crds/ directory in the chart.
// CRD resources are installed before templates by Helm.
func GenerateCRDInstallFiles(resources []*unstructured.Unstructured) map[string]string {
	files := make(map[string]string)

	for _, obj := range resources {
		if obj.GetKind() != "CustomResourceDefinition" {
			continue
		}

		name := obj.GetName()
		if name == "" {
			continue
		}

		// Build the CRD YAML content
		var b strings.Builder
		b.WriteString("# WARNING: CRD resources in crds/ are installed by Helm before templates,\n")
		b.WriteString("# but Helm does NOT manage CRD updates or deletions.\n")
		b.WriteString("# To update a CRD, apply the new version manually with kubectl.\n")
		b.WriteString(fmt.Sprintf("apiVersion: %s\n", obj.GetAPIVersion()))
		b.WriteString(fmt.Sprintf("kind: %s\n", obj.GetKind()))
		b.WriteString("metadata:\n")
		b.WriteString(fmt.Sprintf("  name: %s\n", name))

		if labels := obj.GetLabels(); len(labels) > 0 {
			b.WriteString("  labels:\n")
			for k, v := range labels {
				b.WriteString(fmt.Sprintf("    %s: %s\n", k, v))
			}
		}

		if annotations := obj.GetAnnotations(); len(annotations) > 0 {
			b.WriteString("  annotations:\n")
			for k, v := range annotations {
				b.WriteString(fmt.Sprintf("    %s: \"%s\"\n", k, v))
			}
		}

		// Include spec as-is
		if spec, found, _ := unstructured.NestedMap(obj.Object, "spec"); found {
			b.WriteString("spec:\n")
			// We output spec via a simple recursive serializer
			writeNestedYAML(&b, spec, 2)
		}

		files[fmt.Sprintf("crds/%s.yaml", name)] = b.String()
	}

	return files
}

// writeNestedYAML writes a nested map as YAML with indentation.
func writeNestedYAML(b *strings.Builder, m map[string]interface{}, indent int) {
	prefix := strings.Repeat(" ", indent)
	for k, v := range m {
		switch val := v.(type) {
		case map[string]interface{}:
			b.WriteString(fmt.Sprintf("%s%s:\n", prefix, k))
			writeNestedYAML(b, val, indent+2)
		case []interface{}:
			b.WriteString(fmt.Sprintf("%s%s:\n", prefix, k))
			for _, item := range val {
				if itemMap, ok := item.(map[string]interface{}); ok {
					b.WriteString(fmt.Sprintf("%s- \n", prefix))
					writeNestedYAML(b, itemMap, indent+4)
				} else {
					b.WriteString(fmt.Sprintf("%s- %v\n", prefix, item))
				}
			}
		default:
			b.WriteString(fmt.Sprintf("%s%s: %v\n", prefix, k, v))
		}
	}
}
