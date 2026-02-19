package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// PrometheusRuleProcessor processes Prometheus Operator PrometheusRule resources.
type PrometheusRuleProcessor struct {
	processor.BaseProcessor
}

// NewPrometheusRuleProcessor creates a new PrometheusRule processor.
func NewPrometheusRuleProcessor() *PrometheusRuleProcessor {
	return &PrometheusRuleProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"prometheusrule",
			70,
			schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "PrometheusRule"},
		),
	}
}

// Process processes a PrometheusRule resource.
func (p *PrometheusRuleProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("PrometheusRule object is nil")
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
		TemplatePath:    fmt.Sprintf("templates/%s-prometheusrule.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.prometheusRule", serviceName),
		Values:          values,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *PrometheusRuleProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	// Preserve full spec for pipeline integration
	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	// Extract groups (with rules containing alert/record, expr, for, labels, annotations)
	if groups, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "groups"); ok && len(groups) > 0 {
		values["groups"] = groups
	}

	return values
}

func (p *PrometheusRuleProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.prometheusRule }}
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  {{- with .groups }}
  groups:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end }}
`, sanitized, serviceName, ctx.ChartName)
}
