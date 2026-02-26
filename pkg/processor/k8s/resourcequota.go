package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ResourceQuotaProcessor processes Kubernetes ResourceQuota resources (v1).
type ResourceQuotaProcessor struct {
	processor.BaseProcessor
}

// NewResourceQuotaProcessor creates a new ResourceQuota processor.
func NewResourceQuotaProcessor() *ResourceQuotaProcessor {
	return &ResourceQuotaProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"resourcequota",
			80,
			schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ResourceQuota"},
		),
	}
}

// Process processes a ResourceQuota resource.
func (p *ResourceQuotaProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("ResourceQuota object is nil")
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
		TemplatePath:    fmt.Sprintf("templates/%s-resourcequota.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.resourceQuota", serviceName),
		Values:          values,
		Dependencies:    []types.ResourceKey{},
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *ResourceQuotaProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	// Extract hard (resource limits map)
	if hard, ok, _ := unstructured.NestedMap(obj.Object, "spec", "hard"); ok {
		values["hard"] = hard
	}

	// Extract scopeSelector
	if scopeSelector, ok, _ := unstructured.NestedMap(obj.Object, "spec", "scopeSelector"); ok {
		values["scopeSelector"] = scopeSelector
	}

	// Extract scopes
	if scopes, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "scopes"); ok && len(scopes) > 0 {
		values["scopes"] = scopes
	}

	return values
}

func (p *ResourceQuotaProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	fullnameHelper := fmt.Sprintf(`{{ include "%s.fullname" $ }}`, ctx.ChartName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.resourceQuota }}
apiVersion: v1
kind: ResourceQuota
metadata:
  name: %s-%s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s
spec:
  {{- with .hard }}
  hard:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .scopes }}
  scopes:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .scopeSelector }}
  scopeSelector:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end }}
`, serviceName, fullnameHelper, serviceName, ctx.ChartName, serviceName)
}
