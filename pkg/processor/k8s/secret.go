package k8s

import (
	"encoding/base64"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor/value"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// SecretProcessor processes Kubernetes Secrets.
type SecretProcessor struct {
	processor.BaseProcessor
}

// NewSecretProcessor creates a new Secret processor.
func NewSecretProcessor() *SecretProcessor {
	return &SecretProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"secret",
			100,
			schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"},
		),
	}
}

// Process processes a Secret resource.
func (p *SecretProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, fmt.Errorf("cannot process nil Secret")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	// Extract values from the secret with value processing
	values, externalFiles := p.extractValues(ctx, obj, serviceName, name)

	// Generate template
	template := p.generateTemplate(ctx, obj, serviceName, name)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-secret-%s.yaml", serviceName, name),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.secrets.%s", serviceName, sanitizeName(name)),
		Values:          values,
		Dependencies:    []types.ResourceKey{},
		ExternalFiles:   externalFiles,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *SecretProcessor) extractValues(ctx processor.Context, obj *unstructured.Unstructured, serviceName, secretName string) (map[string]interface{}, []*value.ExternalFile) {
	values := make(map[string]interface{})
	externalFiles := make([]*value.ExternalFile, 0)

	values["enabled"] = true

	// Type
	if secretType, found, _ := unstructured.NestedString(obj.Object, "type"); found {
		values["type"] = secretType
	} else {
		values["type"] = "Opaque"
	}

	// Process data (base64 encoded) with value processor if available
	if data, found, _ := unstructured.NestedStringMap(obj.Object, "data"); found && len(data) > 0 {
		if ctx.ValueProcessor != nil && ctx.ExternalFileManager != nil {
			processedData := make(map[string]interface{})

			for key, val := range data {
				// Decode base64 for processing
				decodedBytes, err := base64.StdEncoding.DecodeString(val)
				var decodedValue string
				if err != nil {
					// If not valid base64, use as-is
					decodedValue = val
				} else {
					decodedValue = string(decodedBytes)
				}

				pv := ctx.ValueProcessor.Process(key, decodedValue)

				if pv.ShouldExternalize {
					// Create external file
					sourceResource := fmt.Sprintf("Secret/%s/%s", obj.GetNamespace(), secretName)
					file, err := ctx.ExternalFileManager.AddFromProcessed(sourceResource, key, pv)
					if err == nil && file != nil {
						externalFiles = append(externalFiles, file)
						// Reference external file in values
						processedData[key] = map[string]interface{}{
							"_externalFile": file.Path,
							"_checksum":     pv.Checksum,
							"_type":         string(pv.DetectedType),
							"_base64":       true, // Indicate this should be base64 encoded
						}
					} else {
						// Fallback to inline if external file creation failed
						// Keep as base64 for secrets
						processedData[key] = val
					}
				} else {
					// Keep inline as base64
					processedData[key] = val
				}
			}
			values["data"] = processedData
		} else {
			// No value processor, use raw data
			values["data"] = data
		}
	}

	// StringData (plain text) - process similarly
	if stringData, found, _ := unstructured.NestedStringMap(obj.Object, "stringData"); found && len(stringData) > 0 {
		if ctx.ValueProcessor != nil && ctx.ExternalFileManager != nil {
			processedStringData := make(map[string]interface{})

			for key, val := range stringData {
				pv := ctx.ValueProcessor.Process(key, val)

				if pv.ShouldExternalize {
					// Create external file
					sourceResource := fmt.Sprintf("Secret/%s/%s", obj.GetNamespace(), secretName)
					file, err := ctx.ExternalFileManager.AddFromProcessed(sourceResource, key, pv)
					if err == nil && file != nil {
						externalFiles = append(externalFiles, file)
						// Reference external file in values
						processedStringData[key] = map[string]interface{}{
							"_externalFile": file.Path,
							"_checksum":     pv.Checksum,
							"_type":         string(pv.DetectedType),
						}
					} else {
						// Fallback to inline if external file creation failed
						processedStringData[key] = pv.FormattedValue
					}
				} else {
					// Keep inline
					processedStringData[key] = pv.FormattedValue
				}
			}
			values["stringData"] = processedStringData
		} else {
			// No value processor, use raw data
			values["stringData"] = stringData
		}
	}

	// Immutable
	if immutable, found, _ := unstructured.NestedBool(obj.Object, "immutable"); found {
		values["immutable"] = immutable
	}

	// Annotations
	if annotations := obj.GetAnnotations(); len(annotations) > 0 {
		values["annotations"] = annotations

		// ESO (External Secrets Operator) detection
		// Check for annotation external-secrets.io/managed-by
		if esoStrategy, ok := annotations["external-secrets.io/managed-by"]; ok && esoStrategy != "" {
			values["esoManaged"] = true
			values["esoStrategy"] = esoStrategy
		}
	}

	// ESO detection via labels (external-secrets.io/type)
	if labels := obj.GetLabels(); len(labels) > 0 {
		if _, alreadySet := values["esoManaged"]; !alreadySet {
			if esoType, ok := labels["external-secrets.io/type"]; ok && esoType != "" {
				values["esoManaged"] = true
				values["esoStrategy"] = esoType
			}
		}
	}

	return values, externalFiles
}

func (p *SecretProcessor) generateTemplate(ctx processor.Context, obj *unstructured.Unstructured, serviceName, secretName string) string {
	sanitizedName := sanitizeName(secretName)
	fullnameHelper := fmt.Sprintf("{{ include \"%s.fullname\" $ }}", ctx.ChartName)

	template := fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- $secret := $svc.secrets.%s -}}
{{- if $secret.enabled }}
apiVersion: v1
kind: Secret
metadata:
  name: %s-%s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s
  {{- with $secret.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
type: {{ $secret.type | default "Opaque" }}
{{- with $secret.immutable }}
immutable: {{ . }}
{{- end }}
{{- with $secret.data }}
data:
  {{- range $key, $value := . }}
  {{- if kindIs "map" $value }}
  {{- if hasKey $value "_externalFile" }}
  {{- if hasKey $value "_base64" }}
  {{ $key }}: {{ $.Files.Get $value._externalFile | b64enc | quote }}
  {{- else }}
  {{ $key }}: |
    {{- $.Files.Get $value._externalFile | nindent 4 }}
  {{- end }}
  {{- else }}
  {{ $key }}: {{ $value | quote }}
  {{- end }}
  {{- else }}
  {{ $key }}: {{ $value | quote }}
  {{- end }}
  {{- end }}
{{- end }}
{{- with $secret.stringData }}
stringData:
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
{{- end }}
{{- end }}
`, serviceName, sanitizedName,
		fullnameHelper, secretName,
		ctx.ChartName, serviceName)

	return template
}
