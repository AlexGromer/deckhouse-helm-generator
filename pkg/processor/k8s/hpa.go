package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// HPAProcessor processes Kubernetes HorizontalPodAutoscaler resources (autoscaling/v2).
type HPAProcessor struct {
	processor.BaseProcessor
}

// NewHPAProcessor creates a new HPA processor.
func NewHPAProcessor() *HPAProcessor {
	return &HPAProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"hpa",
			90,
			schema.GroupVersionKind{Group: "autoscaling", Version: "v2", Kind: "HorizontalPodAutoscaler"},
		),
	}
}

// Process processes an HPA resource.
func (p *HPAProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("HPA object is nil")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	values, deps := p.extractValues(obj)

	template := p.generateTemplate(ctx, obj, serviceName)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-hpa.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.hpa", serviceName),
		Values:          values,
		Dependencies:    deps,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *HPAProcessor) extractValues(obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey

	spec, _, _ := unstructured.NestedMap(obj.Object, "spec")
	if spec == nil {
		return values, deps
	}

	// Extract scaleTargetRef
	if targetRef, ok, _ := unstructured.NestedMap(obj.Object, "spec", "scaleTargetRef"); ok {
		values["scaleTargetRef"] = targetRef

		// Add dependency on the target resource
		targetKind, _ := targetRef["kind"].(string)
		targetName, _ := targetRef["name"].(string)
		if targetKind != "" && targetName != "" {
			apiVersion, _ := targetRef["apiVersion"].(string)
			group := ""
			if apiVersion != "" {
				parts := splitAPIVersion(apiVersion)
				group = parts[0]
			}
			deps = append(deps, types.ResourceKey{
				GVK: schema.GroupVersionKind{
					Group: group,
					Kind:  targetKind,
				},
				Namespace: obj.GetNamespace(),
				Name:      targetName,
			})
		}
	}

	// Extract minReplicas
	if minReplicas, ok := nestedInt64(obj.Object, "spec", "minReplicas"); ok {
		values["minReplicas"] = minReplicas
	}

	// Extract maxReplicas
	if maxReplicas, ok := nestedInt64(obj.Object, "spec", "maxReplicas"); ok {
		values["maxReplicas"] = maxReplicas
	}

	// Extract metrics
	if metrics, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "metrics"); ok && len(metrics) > 0 {
		values["metrics"] = metrics
	}

	// Extract behavior
	if behavior, ok, _ := unstructured.NestedMap(obj.Object, "spec", "behavior"); ok {
		values["behavior"] = behavior
	}

	return values, deps
}

// splitAPIVersion splits "apps/v1" into ["apps", "v1"] or ["", "v1"] for core.
func splitAPIVersion(apiVersion string) [2]string {
	for i := len(apiVersion) - 1; i >= 0; i-- {
		if apiVersion[i] == '/' {
			return [2]string{apiVersion[:i], apiVersion[i+1:]}
		}
	}
	return [2]string{"", apiVersion}
}

func (p *HPAProcessor) generateTemplate(ctx processor.Context, obj *unstructured.Unstructured, serviceName string) string {
	fullnameHelper := fmt.Sprintf(`{{ include "%s.fullname" $ }}`, ctx.ChartName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.hpa }}
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: %s-%s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s
spec:
  scaleTargetRef:
    apiVersion: {{ .scaleTargetRef.apiVersion | default "apps/v1" }}
    kind: {{ .scaleTargetRef.kind | default "Deployment" }}
    name: %s-{{ .scaleTargetRef.name }}
  minReplicas: {{ .minReplicas | default 1 }}
  maxReplicas: {{ .maxReplicas | default 10 }}
  {{- with .metrics }}
  metrics:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .behavior }}
  behavior:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end }}
`, serviceName, fullnameHelper, serviceName, ctx.ChartName, serviceName, fullnameHelper)
}
