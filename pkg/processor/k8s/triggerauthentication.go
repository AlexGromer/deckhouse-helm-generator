package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// TriggerAuthenticationProcessor processes KEDA TriggerAuthentication resources.
type TriggerAuthenticationProcessor struct {
	processor.BaseProcessor
}

// NewTriggerAuthenticationProcessor creates a new TriggerAuthentication processor.
func NewTriggerAuthenticationProcessor() *TriggerAuthenticationProcessor {
	return &TriggerAuthenticationProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"triggerauthentication",
			70,
			schema.GroupVersionKind{Group: "keda.sh", Version: "v1alpha1", Kind: "TriggerAuthentication"},
		),
	}
}

// Process processes a TriggerAuthentication resource.
func (p *TriggerAuthenticationProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("TriggerAuthentication object is nil")
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
		TemplatePath:    fmt.Sprintf("templates/%s-triggerauthentication.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.triggerAuthentication", serviceName),
		Values:          values,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *TriggerAuthenticationProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	// Preserve full spec for pipeline integration
	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	// Extract secretTargetRef
	if secretRefs, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "secretTargetRef"); ok && len(secretRefs) > 0 {
		values["secretTargetRef"] = secretRefs
	}

	// Extract env
	if envRefs, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "env"); ok && len(envRefs) > 0 {
		values["env"] = envRefs
	}

	// Extract podIdentity
	if podIdentity, ok, _ := unstructured.NestedMap(obj.Object, "spec", "podIdentity"); ok {
		values["podIdentity"] = podIdentity
	}

	return values
}

func (p *TriggerAuthenticationProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.triggerAuthentication }}
apiVersion: keda.sh/v1alpha1
kind: TriggerAuthentication
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  {{- with .secretTargetRef }}
  secretTargetRef:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .env }}
  env:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .podIdentity }}
  podIdentity:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end }}
`, sanitized, serviceName, ctx.ChartName)
}
