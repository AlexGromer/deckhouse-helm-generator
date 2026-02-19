package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// GatewayProcessor processes Gateway API Gateway resources.
type GatewayProcessor struct {
	processor.BaseProcessor
}

// NewGatewayProcessor creates a new Gateway processor.
func NewGatewayProcessor() *GatewayProcessor {
	return &GatewayProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"gateway",
			70,
			schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway"},
		),
	}
}

// Process processes a Gateway resource.
func (p *GatewayProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("Gateway object is nil")
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
		TemplatePath:    fmt.Sprintf("templates/%s-gateway.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.gateway", serviceName),
		Values:          values,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *GatewayProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	// Preserve full spec for pipeline integration
	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	// Extract gatewayClassName
	if className, ok, _ := unstructured.NestedString(obj.Object, "spec", "gatewayClassName"); ok {
		values["gatewayClassName"] = className
	}

	// Extract listeners
	if listeners, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "listeners"); ok && len(listeners) > 0 {
		values["listeners"] = listeners
	}

	return values
}

func (p *GatewayProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.gateway }}
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  gatewayClassName: {{ .gatewayClassName }}
  {{- with .listeners }}
  listeners:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end }}
`, sanitized, serviceName, ctx.ChartName)
}
