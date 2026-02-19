package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ScaledObjectProcessor processes KEDA ScaledObject resources.
type ScaledObjectProcessor struct {
	processor.BaseProcessor
}

// NewScaledObjectProcessor creates a new ScaledObject processor.
func NewScaledObjectProcessor() *ScaledObjectProcessor {
	return &ScaledObjectProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"scaledobject",
			70,
			schema.GroupVersionKind{Group: "keda.sh", Version: "v1alpha1", Kind: "ScaledObject"},
		),
	}
}

// Process processes a ScaledObject resource.
func (p *ScaledObjectProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("ScaledObject object is nil")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	values, deps := p.extractValues(obj)
	template := p.generateTemplate(ctx, serviceName)

	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}

	// Flag scale-to-zero
	if minReplica, ok := values["minReplicaCount"]; ok {
		if v, ok := minReplica.(int64); ok && v == 0 {
			metadata["scale_to_zero"] = true
		}
	}

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-scaledobject.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.scaledObject", serviceName),
		Values:          values,
		Dependencies:    deps,
		Metadata:        metadata,
	}, nil
}

func (p *ScaledObjectProcessor) extractValues(obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey

	// Preserve full spec for pipeline integration
	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	// Extract scaleTargetRef
	if targetRef, ok, _ := unstructured.NestedMap(obj.Object, "spec", "scaleTargetRef"); ok {
		values["scaleTargetRef"] = targetRef

		targetName, _ := targetRef["name"].(string)
		targetKind, _ := targetRef["kind"].(string)
		if targetKind == "" {
			targetKind = "Deployment"
		}
		if targetName != "" {
			group := ""
			if targetKind == "Deployment" || targetKind == "StatefulSet" || targetKind == "DaemonSet" {
				group = "apps"
			}
			deps = append(deps, types.ResourceKey{
				GVK:       schema.GroupVersionKind{Group: group, Kind: targetKind},
				Namespace: obj.GetNamespace(),
				Name:      targetName,
			})
		}
	}

	// Extract minReplicaCount
	if minReplicas, ok := nestedInt64(obj.Object, "spec", "minReplicaCount"); ok {
		values["minReplicaCount"] = minReplicas
	}

	// Extract maxReplicaCount
	if maxReplicas, ok := nestedInt64(obj.Object, "spec", "maxReplicaCount"); ok {
		values["maxReplicaCount"] = maxReplicas
	}

	// Extract triggers
	if triggers, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "triggers"); ok && len(triggers) > 0 {
		values["triggers"] = triggers
	}

	return values, deps
}

func (p *ScaledObjectProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.scaledObject }}
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  scaleTargetRef:
    {{- toYaml .scaleTargetRef | nindent 4 }}
  {{- with .minReplicaCount }}
  minReplicaCount: {{ . }}
  {{- end }}
  {{- with .maxReplicaCount }}
  maxReplicaCount: {{ . }}
  {{- end }}
  {{- with .triggers }}
  triggers:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end }}
`, sanitized, serviceName, ctx.ChartName)
}
