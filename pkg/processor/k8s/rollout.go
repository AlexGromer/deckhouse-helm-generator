package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// RolloutProcessor processes Argo Rollouts resources.
type RolloutProcessor struct {
	processor.BaseProcessor
}

// NewRolloutProcessor creates a new Rollout processor.
func NewRolloutProcessor() *RolloutProcessor {
	return &RolloutProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"rollout",
			70,
			schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "Rollout"},
		),
	}
}

// Process processes a Rollout resource.
func (p *RolloutProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("Rollout object is nil")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	values := p.extractValues(obj)
	template := p.generateTemplate(ctx, serviceName)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-rollout.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.rollout", serviceName),
		Values:          values,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *RolloutProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	// Preserve full spec for pipeline integration
	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	// Extract strategy (canary or blueGreen)
	if strategy, ok, _ := unstructured.NestedMap(obj.Object, "spec", "strategy"); ok {
		values["strategy"] = strategy
	}

	// Preserve pod template
	if tmpl, ok, _ := unstructured.NestedMap(obj.Object, "spec", "template"); ok {
		values["template"] = tmpl
	}

	return values
}

func (p *RolloutProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.rollout }}
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  {{- with .strategy }}
  strategy:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .template }}
  template:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end }}
`, sanitized, serviceName, ctx.ChartName)
}
