package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// UserProcessor processes Deckhouse User resources.
type UserProcessor struct {
	processor.BaseProcessor
}

// NewUserProcessor creates a new User processor.
func NewUserProcessor() *UserProcessor {
	return &UserProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"user",
			50,
			schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: "User"},
		),
	}
}

// Process processes a User resource.
func (p *UserProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("User object is nil")
	}

	name := obj.GetName()
	values, sensitiveFields := p.extractValues(obj)
	template := p.generateTemplate(ctx, name)

	metadata := map[string]interface{}{"name": name}
	if len(sensitiveFields) > 0 {
		metadata["sensitive_fields"] = sensitiveFields
	}

	return &processor.Result{
		Processed:       true,
		ServiceName:     name,
		TemplatePath:    fmt.Sprintf("templates/user-%s.yaml", name),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.user", name),
		Values:          values,
		Metadata:        metadata,
	}, nil
}

func (p *UserProcessor) extractValues(obj *unstructured.Unstructured) (map[string]interface{}, []string) {
	values := make(map[string]interface{})
	var sensitiveFields []string

	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	if email, ok, _ := unstructured.NestedString(obj.Object, "spec", "email"); ok {
		values["email"] = email
	}

	if groups, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "groups"); ok {
		values["groups"] = groups
	}

	if ttl, ok, _ := unstructured.NestedString(obj.Object, "spec", "ttl"); ok {
		values["ttl"] = ttl
	}

	// Password is sensitive — do NOT put in values, flag in metadata
	if _, ok, _ := unstructured.NestedString(obj.Object, "spec", "password"); ok {
		sensitiveFields = append(sensitiveFields, "password")
	}

	return values, sensitiveFields
}

func (p *UserProcessor) generateTemplate(ctx processor.Context, name string) string {
	sanitized := processor.SanitizeServiceName(name)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- with $svc.user }}
apiVersion: deckhouse.io/v1
kind: User
metadata:
  name: %s
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  email: {{ .email }}
  {{- with .groups }}
  groups:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .ttl }}
  ttl: {{ . }}
  {{- end }}
  password: {{ .passwordRef | default "CHANGE_ME" }}
{{- end }}
`, sanitized, name, ctx.ChartName)
}
