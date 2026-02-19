package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// GroupProcessor processes Deckhouse Group resources.
type GroupProcessor struct {
	processor.BaseProcessor
}

// NewGroupProcessor creates a new Group processor.
func NewGroupProcessor() *GroupProcessor {
	return &GroupProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"group",
			50,
			schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: "Group"},
		),
	}
}

// Process processes a Group resource.
func (p *GroupProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("Group object is nil")
	}

	name := obj.GetName()
	values := p.extractValues(obj)
	template := p.generateTemplate(ctx, name)

	return &processor.Result{
		Processed:       true,
		ServiceName:     name,
		TemplatePath:    fmt.Sprintf("templates/group-%s.yaml", name),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.group", name),
		Values:          values,
		Metadata:        map[string]interface{}{"name": name},
	}, nil
}

func (p *GroupProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	if members, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "members"); ok {
		values["members"] = members
	}

	return values
}

func (p *GroupProcessor) generateTemplate(ctx processor.Context, name string) string {
	sanitized := processor.SanitizeServiceName(name)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- with $svc.group }}
apiVersion: deckhouse.io/v1
kind: Group
metadata:
  name: %s
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  {{- with .members }}
  members:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
`, sanitized, name, ctx.ChartName)
}
