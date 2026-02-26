package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// LimitRangeProcessor processes Kubernetes LimitRange resources (v1).
type LimitRangeProcessor struct {
	processor.BaseProcessor
}

// NewLimitRangeProcessor creates a new LimitRange processor.
func NewLimitRangeProcessor() *LimitRangeProcessor {
	return &LimitRangeProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"limitrange",
			80,
			schema.GroupVersionKind{Group: "", Version: "v1", Kind: "LimitRange"},
		),
	}
}

// Process processes a LimitRange resource.
func (p *LimitRangeProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("LimitRange object is nil")
	}

	serviceName := processor.SanitizeServiceName(processor.ServiceNameFromResource(obj))
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
		TemplatePath:    fmt.Sprintf("templates/%s-limitrange.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.limitRange", serviceName),
		Values:          values,
		Dependencies:    []types.ResourceKey{},
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *LimitRangeProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	// Extract limits array from spec
	if limits, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "limits"); ok && len(limits) > 0 {
		values["limits"] = limits
	}

	return values
}

func (p *LimitRangeProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	fullnameHelper := fmt.Sprintf(`{{ include "%s.fullname" $ }}`, ctx.ChartName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.limitRange }}
apiVersion: v1
kind: LimitRange
metadata:
  name: %s-%s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s
spec:
  {{- with .limits }}
  limits:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end }}
`, serviceName, fullnameHelper, serviceName, ctx.ChartName, serviceName)
}
