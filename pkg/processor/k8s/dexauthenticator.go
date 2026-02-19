package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// DexAuthenticatorProcessor processes Deckhouse DexAuthenticator resources.
type DexAuthenticatorProcessor struct {
	processor.BaseProcessor
}

// NewDexAuthenticatorProcessor creates a new DexAuthenticator processor.
func NewDexAuthenticatorProcessor() *DexAuthenticatorProcessor {
	return &DexAuthenticatorProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"dexauthenticator",
			50,
			schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: "DexAuthenticator"},
		),
	}
}

// Process processes a DexAuthenticator resource.
func (p *DexAuthenticatorProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("DexAuthenticator object is nil")
	}

	name := obj.GetName()
	values := p.extractValues(obj)
	template := p.generateTemplate(ctx, name)

	return &processor.Result{
		Processed:       true,
		ServiceName:     name,
		TemplatePath:    fmt.Sprintf("templates/dexauthenticator-%s.yaml", name),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.dexAuthenticator", name),
		Values:          values,
		Metadata:        map[string]interface{}{"name": name},
	}, nil
}

func (p *DexAuthenticatorProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	if domain, ok, _ := unstructured.NestedString(obj.Object, "spec", "applicationDomain"); ok {
		values["applicationDomain"] = domain
	}

	if sendAuth, ok, _ := unstructured.NestedBool(obj.Object, "spec", "sendAuthorizationHeader"); ok {
		values["sendAuthorizationHeader"] = sendAuth
	}

	if className, ok, _ := unstructured.NestedString(obj.Object, "spec", "applicationIngressClassName"); ok {
		values["applicationIngressClassName"] = className
	}

	if groups, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "allowedGroups"); ok {
		values["allowedGroups"] = groups
	}

	return values
}

func (p *DexAuthenticatorProcessor) generateTemplate(ctx processor.Context, name string) string {
	sanitized := processor.SanitizeServiceName(name)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- with $svc.dexAuthenticator }}
apiVersion: deckhouse.io/v1
kind: DexAuthenticator
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  applicationDomain: {{ .applicationDomain }}
  {{- if .sendAuthorizationHeader }}
  sendAuthorizationHeader: {{ .sendAuthorizationHeader }}
  {{- end }}
  {{- with .applicationIngressClassName }}
  applicationIngressClassName: {{ . }}
  {{- end }}
  {{- with .allowedGroups }}
  allowedGroups:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
`, sanitized, name, ctx.ChartName)
}
