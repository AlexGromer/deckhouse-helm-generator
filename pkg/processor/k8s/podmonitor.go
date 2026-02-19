package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// PodMonitorProcessor processes Prometheus Operator PodMonitor resources.
type PodMonitorProcessor struct {
	processor.BaseProcessor
}

// NewPodMonitorProcessor creates a new PodMonitor processor.
func NewPodMonitorProcessor() *PodMonitorProcessor {
	return &PodMonitorProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"podmonitor",
			70,
			schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "PodMonitor"},
		),
	}
}

// Process processes a PodMonitor resource.
func (p *PodMonitorProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("PodMonitor object is nil")
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
		TemplatePath:    fmt.Sprintf("templates/%s-podmonitor.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.podMonitor", serviceName),
		Values:          values,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *PodMonitorProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	// Preserve full spec for pipeline integration
	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	// Extract podMetricsEndpoints
	if endpoints, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "podMetricsEndpoints"); ok && len(endpoints) > 0 {
		values["podMetricsEndpoints"] = endpoints
	}

	// Extract jobLabel
	if jobLabel, ok, _ := unstructured.NestedString(obj.Object, "spec", "jobLabel"); ok {
		values["jobLabel"] = jobLabel
	}

	// Extract selector
	if selector, ok, _ := unstructured.NestedMap(obj.Object, "spec", "selector"); ok {
		values["selector"] = selector
	}

	return values
}

func (p *PodMonitorProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.podMonitor }}
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  {{- with .jobLabel }}
  jobLabel: {{ . }}
  {{- end }}
  {{- with .podMetricsEndpoints }}
  podMetricsEndpoints:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .selector }}
  selector:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end }}
`, sanitized, serviceName, ctx.ChartName)
}
