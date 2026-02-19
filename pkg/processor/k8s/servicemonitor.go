package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ServiceMonitorProcessor processes Prometheus Operator ServiceMonitor resources.
type ServiceMonitorProcessor struct {
	processor.BaseProcessor
}

// NewServiceMonitorProcessor creates a new ServiceMonitor processor.
func NewServiceMonitorProcessor() *ServiceMonitorProcessor {
	return &ServiceMonitorProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"servicemonitor",
			70,
			schema.GroupVersionKind{Group: "monitoring.coreos.com", Version: "v1", Kind: "ServiceMonitor"},
		),
	}
}

// Process processes a ServiceMonitor resource.
func (p *ServiceMonitorProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("ServiceMonitor object is nil")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	values, deps := p.extractValues(obj)
	template := p.generateTemplate(ctx, serviceName)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-servicemonitor.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.serviceMonitor", serviceName),
		Values:          values,
		Dependencies:    deps,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *ServiceMonitorProcessor) extractValues(obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey

	// Preserve full spec for pipeline integration
	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	// Extract endpoints
	if endpoints, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "endpoints"); ok && len(endpoints) > 0 {
		values["endpoints"] = endpoints
	}

	// Extract namespaceSelector
	if nsSelector, ok, _ := unstructured.NestedMap(obj.Object, "spec", "namespaceSelector"); ok {
		values["namespaceSelector"] = nsSelector
	}

	// Extract selector
	if selector, ok, _ := unstructured.NestedMap(obj.Object, "spec", "selector"); ok {
		values["selector"] = selector
	}

	// ServiceMonitor depends on the Service it selects
	deps = append(deps, types.ResourceKey{
		GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Service"},
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	})

	return values, deps
}

func (p *ServiceMonitorProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.serviceMonitor }}
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  {{- with .endpoints }}
  endpoints:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .namespaceSelector }}
  namespaceSelector:
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
