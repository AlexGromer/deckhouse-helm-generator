package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// ReferenceGrantProcessor processes Gateway API ReferenceGrant resources.
type ReferenceGrantProcessor struct {
	processor.BaseProcessor
}

// NewReferenceGrantProcessor creates a new ReferenceGrant processor.
func NewReferenceGrantProcessor() *ReferenceGrantProcessor {
	return &ReferenceGrantProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"referencegrant",
			72,
			schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1beta1", Kind: "ReferenceGrant"},
			schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1alpha2", Kind: "ReferenceGrant"},
		),
	}
}

// Process processes a ReferenceGrant resource.
func (p *ReferenceGrantProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("ReferenceGrant object is nil")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()


	values, deps := p.extractValues(serviceName, obj)
	template := p.generateTemplate(ctx, serviceName, name)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-referencegrant.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s", serviceName),
		Values:          values,
		Dependencies:    deps,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *ReferenceGrantProcessor) extractValues(serviceName string, obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey

	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	// Extract from and to
	if from, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "from"); ok {
		values["from"] = from
	}

	if to, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "to"); ok {
		values["to"] = to
	}

	return values, deps
}

func (p *ReferenceGrantProcessor) generateTemplate(ctx processor.Context, serviceName, name string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- if or (hasKey $svc "from") (hasKey $svc "to") }}
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
%s{{- end }}
{{- end }}
`, sanitized, processor.ObjectName(name), ctx.ChartName,
		processor.SpecOverlay(".", "from", "to"))
}
