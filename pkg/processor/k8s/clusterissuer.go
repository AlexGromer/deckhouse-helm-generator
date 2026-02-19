package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// ClusterIssuerProcessor processes cert-manager ClusterIssuer resources.
type ClusterIssuerProcessor struct {
	processor.BaseProcessor
}

// NewClusterIssuerProcessor creates a new ClusterIssuer processor.
func NewClusterIssuerProcessor() *ClusterIssuerProcessor {
	return &ClusterIssuerProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"clusterissuer",
			70,
			schema.GroupVersionKind{Group: "cert-manager.io", Version: "v1", Kind: "ClusterIssuer"},
		),
	}
}

// Process processes a ClusterIssuer resource.
func (p *ClusterIssuerProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("ClusterIssuer object is nil")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()

	values := p.extractValues(obj)
	template := p.generateTemplate(ctx, serviceName)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-clusterissuer.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.clusterIssuer", serviceName),
		Values:          values,
		Metadata: map[string]interface{}{
			"name": name,
		},
	}, nil
}

func (p *ClusterIssuerProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	// Preserve full spec for pipeline integration
	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	// Extract acme config
	if acme, ok, _ := unstructured.NestedMap(obj.Object, "spec", "acme"); ok {
		values["acme"] = acme
	}

	// Extract selfSigned
	if selfSigned, ok, _ := unstructured.NestedMap(obj.Object, "spec", "selfSigned"); ok {
		values["selfSigned"] = selfSigned
	}

	// Extract ca
	if ca, ok, _ := unstructured.NestedMap(obj.Object, "spec", "ca"); ok {
		values["ca"] = ca
	}

	return values
}

func (p *ClusterIssuerProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.clusterIssuer }}
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: %s
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  {{- with .acme }}
  acme:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .selfSigned }}
  selfSigned:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .ca }}
  ca:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end }}
`, sanitized, serviceName, ctx.ChartName)
}
