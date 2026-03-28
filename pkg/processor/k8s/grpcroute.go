package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// GRPCRouteProcessor processes Gateway API GRPCRoute resources.
type GRPCRouteProcessor struct {
	processor.BaseProcessor
}

// NewGRPCRouteProcessor creates a new GRPCRoute processor.
func NewGRPCRouteProcessor() *GRPCRouteProcessor {
	return &GRPCRouteProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"grpcroute",
			70,
			schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "GRPCRoute"},
		),
	}
}

// Process processes a GRPCRoute resource.
func (p *GRPCRouteProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("GRPCRoute object is nil")
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
		TemplatePath:    fmt.Sprintf("templates/%s-grpcroute.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.grpcRoute", serviceName),
		Values:          values,
		Dependencies:    deps,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *GRPCRouteProcessor) extractValues(obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey

	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	// Extract parentRefs and create dependencies to Gateways
	if parentRefs, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "parentRefs"); ok && len(parentRefs) > 0 {
		values["parentRefs"] = parentRefs

		for _, ref := range parentRefs {
			refMap, ok := ref.(map[string]interface{})
			if !ok {
				continue
			}
			gwName, _ := refMap["name"].(string)
			gwNamespace, _ := refMap["namespace"].(string)
			if gwNamespace == "" {
				gwNamespace = obj.GetNamespace()
			}
			if gwName != "" {
				deps = append(deps, types.ResourceKey{
					GVK:       schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway"},
					Namespace: gwNamespace,
					Name:      gwName,
				})
			}
		}
	}

	// Extract rules (matches, backendRefs, filters)
	if rules, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "rules"); ok && len(rules) > 0 {
		values["rules"] = rules
	}

	return values, deps
}

func (p *GRPCRouteProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.grpcRoute }}
apiVersion: gateway.networking.k8s.io/v1
kind: GRPCRoute
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  {{- with .parentRefs }}
  parentRefs:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .rules }}
  rules:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end }}
`, sanitized, serviceName, ctx.ChartName)
}
