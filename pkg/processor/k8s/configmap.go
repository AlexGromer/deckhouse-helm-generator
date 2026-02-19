package k8s

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor/value"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ConfigMapProcessor processes Kubernetes ConfigMaps.
type ConfigMapProcessor struct {
	processor.BaseProcessor
}

// NewConfigMapProcessor creates a new ConfigMap processor.
func NewConfigMapProcessor() *ConfigMapProcessor {
	return &ConfigMapProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"configmap",
			100,
			schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"},
		),
	}
}

// Process processes a ConfigMap resource.
func (p *ConfigMapProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, fmt.Errorf("cannot process nil ConfigMap")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	// Extract values from the configmap with value processing
	values, externalFiles := p.extractValues(ctx, obj, serviceName, name)

	// Generate template
	template := p.generateTemplate(ctx, obj, serviceName, name)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-configmap-%s.yaml", serviceName, name),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.configMaps.%s", serviceName, sanitizeName(name)),
		Values:          values,
		Dependencies:    []types.ResourceKey{},
		ExternalFiles:   externalFiles,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *ConfigMapProcessor) extractValues(ctx processor.Context, obj *unstructured.Unstructured, serviceName, configMapName string) (map[string]interface{}, []*value.ExternalFile) {
	values := make(map[string]interface{})
	externalFiles := make([]*value.ExternalFile, 0)

	values["enabled"] = true

	// Process data with value processor if available
	if data, found, _ := unstructured.NestedStringMap(obj.Object, "data"); found && len(data) > 0 {
		if ctx.ValueProcessor != nil && ctx.ExternalFileManager != nil {
			processedData := make(map[string]interface{})

			for key, val := range data {
				pv := ctx.ValueProcessor.Process(key, val)

				if pv.ShouldExternalize {
					// Create external file
					sourceResource := fmt.Sprintf("ConfigMap/%s/%s", obj.GetNamespace(), configMapName)
					file, err := ctx.ExternalFileManager.AddFromProcessed(sourceResource, key, pv)
					if err == nil && file != nil {
						externalFiles = append(externalFiles, file)
						// Reference external file in values
						processedData[key] = map[string]interface{}{
							"_externalFile": file.Path,
							"_checksum":     pv.Checksum,
							"_type":         string(pv.DetectedType),
						}
					} else {
						// Fallback to inline if external file creation failed
						processedData[key] = pv.FormattedValue
					}
				} else {
					// Keep inline
					processedData[key] = pv.FormattedValue
				}
			}
			values["data"] = processedData
		} else {
			// No value processor, use raw data
			values["data"] = data
		}
	}

	// Binary data
	if binaryData, found, _ := unstructured.NestedMap(obj.Object, "binaryData"); found {
		values["binaryData"] = binaryData
	}

	// Immutable
	if immutable, found, _ := unstructured.NestedBool(obj.Object, "immutable"); found {
		values["immutable"] = immutable
	}

	// Annotations
	if annotations := obj.GetAnnotations(); len(annotations) > 0 {
		values["annotations"] = annotations
	}

	return values, externalFiles
}

func (p *ConfigMapProcessor) generateTemplate(ctx processor.Context, obj *unstructured.Unstructured, serviceName, configMapName string) string {
	sanitizedName := sanitizeName(configMapName)
	fullnameHelper := fmt.Sprintf("{{ include \"%s.fullname\" $ }}", ctx.ChartName)

	template := fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- $cm := $svc.configMaps.%s -}}
{{- if $cm.enabled }}
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s-%s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s
  {{- with $cm.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- with $cm.immutable }}
immutable: {{ . }}
{{- end }}
{{- with $cm.data }}
data:
  {{- range $key, $value := . }}
  {{- if kindIs "map" $value }}
  {{- if hasKey $value "_externalFile" }}
  {{ $key }}: |
    {{- $.Files.Get $value._externalFile | nindent 4 }}
  {{- else }}
  {{ $key }}: |
    {{- $value | nindent 4 }}
  {{- end }}
  {{- else }}
  {{ $key }}: |
    {{- $value | nindent 4 }}
  {{- end }}
  {{- end }}
{{- end }}
{{- with $cm.binaryData }}
binaryData:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}
{{- end }}
`, serviceName, sanitizedName,
		fullnameHelper, configMapName,
		ctx.ChartName, serviceName)

	return template
}

// sanitizeName converts a Kubernetes resource name to a valid Go/YAML key (camelCase).
// See also processor.SanitizeServiceName — same algorithm but returns "" for empty input.
func sanitizeName(name string) string {
	if name == "" {
		return "config"
	}

	final := make([]byte, 0, len(name))
	capitalizeNext := false

	for i, c := range name {
		if c == '-' || c == '_' || c == '.' {
			// Mark next alphanumeric character for capitalization
			capitalizeNext = true
			continue
		}

		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			if capitalizeNext && c >= 'a' && c <= 'z' {
				final = append(final, byte(c-32)) // Capitalize
				capitalizeNext = false
			} else if i == 0 && c >= 'A' && c <= 'Z' {
				// Lowercase first character
				final = append(final, byte(c+32))
			} else {
				final = append(final, byte(c))
			}
		}
	}

	if len(final) == 0 {
		return "config"
	}
	return string(final)
}
