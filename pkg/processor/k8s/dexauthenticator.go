package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
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
			schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1alpha1", Kind: "DexAuthenticator"},
			schema.GroupVersionKind{Group: "deckhouse.io", Version: "v2alpha1", Kind: "DexAuthenticator"},
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
	template := p.generateTemplate(ctx, name, obj.GetAPIVersion())

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

	for _, key := range []string{"applicationDomain", "sendAuthorizationHeader", "applicationIngressClassName", "applicationIngressCertificateSecretName", "keepUsersLoggedInFor", "allowedGroups", "whitelistSourceRanges", "nodeSelector", "tolerations", "applications"} {
		if val, ok, _ := unstructured.NestedFieldNoCopy(obj.Object, "spec", key); ok {
			values[key] = val
		}
	}

	return values
}

func (p *DexAuthenticatorProcessor) generateTemplate(ctx processor.Context, name, apiVersion string) string {
	sanitized := processor.SanitizeServiceName(name)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- with $svc.dexAuthenticator }}
apiVersion: %s
kind: DexAuthenticator
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
%s{{- end }}
`, sanitized, apiVersion, processor.ObjectName(name), ctx.ChartName,
		processor.SpecOverlay(".", "applicationDomain", "sendAuthorizationHeader", "applicationIngressClassName", "applicationIngressCertificateSecretName", "keepUsersLoggedInFor", "allowedGroups", "whitelistSourceRanges", "nodeSelector", "tolerations", "applications"))
}
