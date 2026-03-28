package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// TLSRouteProcessor processes Gateway API TLSRoute resources.
type TLSRouteProcessor struct {
	processor.BaseProcessor
}

// NewTLSRouteProcessor creates a new TLSRoute processor.
func NewTLSRouteProcessor() *TLSRouteProcessor {
	return &TLSRouteProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"tlsroute",
			70,
			schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1alpha2", Kind: "TLSRoute"},
		),
	}
}

// Process processes a TLSRoute resource.
func (p *TLSRouteProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("TLSRoute object is nil")
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
		TemplatePath:    fmt.Sprintf("templates/%s-tlsroute.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.tlsRoute", serviceName),
		Values:          values,
		Dependencies:    deps,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *TLSRouteProcessor) extractValues(obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey) {
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

	// Extract rules (backendRefs)
	if rules, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "rules"); ok && len(rules) > 0 {
		values["rules"] = rules
	}

	return values, deps
}

func (p *TLSRouteProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.tlsRoute }}
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: TLSRoute
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
