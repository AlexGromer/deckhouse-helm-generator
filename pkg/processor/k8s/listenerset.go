package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// ListenerSetProcessor processes Gateway API ListenerSet resources.
type ListenerSetProcessor struct {
	processor.BaseProcessor
}

// NewListenerSetProcessor creates a new ListenerSet processor.
func NewListenerSetProcessor() *ListenerSetProcessor {
	return &ListenerSetProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"listenerset",
			71,
			schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "ListenerSet"},
			schema.GroupVersionKind{Group: "gateway.networking.x-k8s.io", Version: "v1alpha1", Kind: "ListenerSet"},
		),
	}
}

// Process processes a ListenerSet resource.
func (p *ListenerSetProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("ListenerSet object is nil")
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
		TemplatePath:    fmt.Sprintf("templates/%s-listenerset.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.listenerSet", serviceName),
		Values:          values,
		Dependencies:    deps,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *ListenerSetProcessor) extractValues(serviceName string, obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey

	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	// Extract parentRef and listeners
	if parentRef, ok, _ := unstructured.NestedMap(obj.Object, "spec", "parentRef"); ok {
		values["parentRef"] = parentRef
	}

	if listeners, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "listeners"); ok {
		values["listeners"] = listeners
	}

	return values, deps
}

func (p *ListenerSetProcessor) generateTemplate(ctx processor.Context, serviceName, name string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.listenerSet }}
apiVersion: gateway.networking.k8s.io/v1
kind: ListenerSet
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
%s{{- end }}
{{- end }}
`, sanitized, processor.ObjectName(name), ctx.ChartName,
		processor.SpecOverlay(".", "parentRef", "listeners"))
}
