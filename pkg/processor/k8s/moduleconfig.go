package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// ModuleConfigProcessor processes Deckhouse ModuleConfig resources (deckhouse.io/v1alpha1).
type ModuleConfigProcessor struct {
	processor.BaseProcessor
}

// NewModuleConfigProcessor creates a new ModuleConfig processor.
func NewModuleConfigProcessor() *ModuleConfigProcessor {
	return &ModuleConfigProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"moduleconfig",
			50,
			schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1alpha1", Kind: "ModuleConfig"},
		),
	}
}

// Process processes a ModuleConfig resource.
func (p *ModuleConfigProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("ModuleConfig object is nil")
	}

	name := obj.GetName()
	serviceName := name

	values := p.extractValues(obj)
	template := p.generateTemplate(ctx, obj, serviceName)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/moduleconfig-%s.yaml", name),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.moduleConfig", serviceName),
		Values:          values,
		Metadata: map[string]interface{}{
			"name": name,
		},
	}, nil
}

func (p *ModuleConfigProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	// Preserve full spec for pipeline integration.
	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	// Extract spec.enabled
	if enabled, ok, _ := unstructured.NestedBool(obj.Object, "spec", "enabled"); ok {
		values["enabled"] = enabled
	}

	// Extract spec.version (integer)
	if version, ok := nestedInt64(obj.Object, "spec", "version"); ok {
		values["version"] = version
	}

	// Extract spec.settings as a whole map
	if settings, ok, _ := unstructured.NestedMap(obj.Object, "spec", "settings"); ok && len(settings) > 0 {
		values["settings"] = settings
	}

	return values
}

func (p *ModuleConfigProcessor) generateTemplate(ctx processor.Context, obj *unstructured.Unstructured, serviceName string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
apiVersion: deckhouse.io/v1alpha1
kind: ModuleConfig
metadata:
  name: %s
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  enabled: {{ $svc.moduleConfig.enabled }}
  {{- with $svc.moduleConfig.version }}
  version: {{ . }}
  {{- end }}
  {{- with $svc.moduleConfig.settings }}
  settings:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
`, sanitized, serviceName, ctx.ChartName)
}
