package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// ClusterAuthorizationRuleProcessor processes Deckhouse ClusterAuthorizationRule resources.
type ClusterAuthorizationRuleProcessor struct {
	processor.BaseProcessor
}

// NewClusterAuthorizationRuleProcessor creates a new ClusterAuthorizationRule processor.
func NewClusterAuthorizationRuleProcessor() *ClusterAuthorizationRuleProcessor {
	return &ClusterAuthorizationRuleProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"clusterauthorizationrule",
			50,
			schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: "ClusterAuthorizationRule"},
		),
	}
}

// Process processes a ClusterAuthorizationRule resource.
func (p *ClusterAuthorizationRuleProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("ClusterAuthorizationRule object is nil")
	}

	name := obj.GetName()
	values := p.extractValues(obj)
	template := p.generateTemplate(ctx, name)

	return &processor.Result{
		Processed:       true,
		ServiceName:     name,
		TemplatePath:    fmt.Sprintf("templates/clusterauthorizationrule-%s.yaml", name),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.clusterAuthorizationRule", name),
		Values:          values,
		Metadata:        map[string]interface{}{"name": name},
	}, nil
}

func (p *ClusterAuthorizationRuleProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	if subjects, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "subjects"); ok {
		values["subjects"] = subjects
	}

	if accessLevel, ok, _ := unstructured.NestedString(obj.Object, "spec", "accessLevel"); ok {
		values["accessLevel"] = accessLevel
	}

	if ns, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "limitNamespaces"); ok {
		values["limitNamespaces"] = ns
	}

	if allowScale, ok, _ := unstructured.NestedBool(obj.Object, "spec", "allowScale"); ok {
		values["allowScale"] = allowScale
	}

	return values
}

func (p *ClusterAuthorizationRuleProcessor) generateTemplate(ctx processor.Context, name string) string {
	sanitized := processor.SanitizeServiceName(name)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- with $svc.clusterAuthorizationRule }}
apiVersion: deckhouse.io/v1
kind: ClusterAuthorizationRule
metadata:
  name: %s
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  subjects:
    {{- toYaml .subjects | nindent 4 }}
  accessLevel: {{ .accessLevel }}
  {{- with .limitNamespaces }}
  limitNamespaces:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- if .allowScale }}
  allowScale: {{ .allowScale }}
  {{- end }}
{{- end }}
`, sanitized, name, ctx.ChartName)
}
