package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
)

// PodLoggingConfigProcessor processes Deckhouse PodLoggingConfig resources.
type PodLoggingConfigProcessor struct {
	processor.BaseProcessor
}

// NewPodLoggingConfigProcessor creates a new PodLoggingConfig processor.
func NewPodLoggingConfigProcessor() *PodLoggingConfigProcessor {
	return &PodLoggingConfigProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"podloggingconfig",
			50,
			schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1alpha1", Kind: "PodLoggingConfig"},
		),
	}
}

// Process processes a PodLoggingConfig resource.
func (p *PodLoggingConfigProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("PodLoggingConfig object is nil")
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	serviceName := p.getServiceName(obj)
	sanitized := processor.SanitizeServiceName(serviceName)

	values := make(map[string]interface{})

	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	if dests, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "clusterDestinationRefs"); ok {
		values["clusterDestinationRefs"] = dests
	}

	if parser, ok, _ := unstructured.NestedMap(obj.Object, "spec", "multilineParser"); ok {
		values["multilineParser"] = parser
	}

	template := fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.podLoggingConfig }}
apiVersion: deckhouse.io/v1alpha1
kind: PodLoggingConfig
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
%s{{- end }}
{{- end }}
`, sanitized, processor.ObjectName(name), ctx.ChartName, processor.SpecOverlay(".", "clusterDestinationRefs"))

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/podloggingconfig-%s.yaml", name),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.podLoggingConfig", sanitized),
		Values:          values,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *PodLoggingConfigProcessor) getServiceName(obj *unstructured.Unstructured) string {
	if matchLabels, ok, _ := unstructured.NestedStringMap(obj.Object, "spec", "labelSelector", "matchLabels"); ok {
		mockObj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": func() map[string]interface{} {
						m := make(map[string]interface{})
						for k, v := range matchLabels {
							m[k] = v
						}
						return m
					}(),
				},
			},
		}
		if name := processor.ServiceNameFromLabels(mockObj); name != "" {
			return name
		}
	}
	name := processor.ServiceNameFromResource(obj)
	if name != "" {
		return name
	}
	return obj.GetName()
}

// ClusterLoggingConfigProcessor processes Deckhouse ClusterLoggingConfig resources.
type ClusterLoggingConfigProcessor struct {
	processor.BaseProcessor
}

// NewClusterLoggingConfigProcessor creates a new ClusterLoggingConfig processor.
func NewClusterLoggingConfigProcessor() *ClusterLoggingConfigProcessor {
	return &ClusterLoggingConfigProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"clusterloggingconfig",
			50,
			schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1alpha1", Kind: "ClusterLoggingConfig"},
		),
	}
}

// Process processes a ClusterLoggingConfig resource.
func (p *ClusterLoggingConfigProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("ClusterLoggingConfig object is nil")
	}

	name := obj.GetName()

	serviceName := p.getServiceName(obj)
	sanitized := processor.SanitizeServiceName(serviceName)

	values := make(map[string]interface{})

	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	if dests, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "destinationRefs"); ok {
		values["destinationRefs"] = dests
	}

	if parser, ok, _ := unstructured.NestedMap(obj.Object, "spec", "multilineParser"); ok {
		values["multilineParser"] = parser
	}

	template := fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.clusterLoggingConfig }}
apiVersion: deckhouse.io/v1alpha1
kind: ClusterLoggingConfig
metadata:
  name: %s
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
%s{{- end }}
{{- end }}
`, sanitized, processor.ObjectName(name), ctx.ChartName, processor.SpecOverlay(".", "destinationRefs"))

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/clusterloggingconfig-%s.yaml", name),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.clusterLoggingConfig", sanitized),
		Values:          values,
		Metadata: map[string]interface{}{
			"name": name,
		},
	}, nil
}

func (p *ClusterLoggingConfigProcessor) getServiceName(obj *unstructured.Unstructured) string {
	if matchLabels, ok, _ := unstructured.NestedStringMap(obj.Object, "spec", "labelSelector", "matchLabels"); ok {
		mockObj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": func() map[string]interface{} {
						m := make(map[string]interface{})
						for k, v := range matchLabels {
							m[k] = v
						}
						return m
					}(),
				},
			},
		}
		if name := processor.ServiceNameFromLabels(mockObj); name != "" {
			return name
		}
	}
	name := processor.ServiceNameFromResource(obj)
	if name != "" {
		return name
	}
	return obj.GetName()
}
