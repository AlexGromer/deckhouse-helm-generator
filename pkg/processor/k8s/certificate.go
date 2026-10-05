package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// CertificateProcessor processes cert-manager Certificate resources.
type CertificateProcessor struct {
	processor.BaseProcessor
}

// NewCertificateProcessor creates a new Certificate processor.
func NewCertificateProcessor() *CertificateProcessor {
	return &CertificateProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"certificate",
			70,
			schema.GroupVersionKind{Group: "cert-manager.io", Version: "v1", Kind: "Certificate"},
		),
	}
}

// Process processes a Certificate resource.
func (p *CertificateProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("Certificate object is nil")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	values := p.extractValues(obj)
	template := p.generateTemplate(ctx, serviceName, obj.GetName())

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-certificate.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.certificate", serviceName),
		Values:          values,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *CertificateProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	// Preserve full spec for pipeline integration
	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	// Extract dnsNames
	if dnsNames, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "dnsNames"); ok && len(dnsNames) > 0 {
		values["dnsNames"] = dnsNames
	}

	// Extract issuerRef
	if issuerRef, ok, _ := unstructured.NestedMap(obj.Object, "spec", "issuerRef"); ok {
		values["issuerRef"] = issuerRef
	}

	// Extract secretName
	if secretName, ok, _ := unstructured.NestedString(obj.Object, "spec", "secretName"); ok {
		values["secretName"] = secretName
	}

	// Extract duration
	if duration, ok, _ := unstructured.NestedString(obj.Object, "spec", "duration"); ok {
		values["duration"] = duration
	}

	// Extract renewBefore
	if renewBefore, ok, _ := unstructured.NestedString(obj.Object, "spec", "renewBefore"); ok {
		values["renewBefore"] = renewBefore
	}

	return values
}

func (p *CertificateProcessor) generateTemplate(ctx processor.Context, serviceName, name string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.certificate }}
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
%s{{- end }}
{{- end }}
`, sanitized, processor.ObjectName(name), ctx.ChartName,
		processor.SpecOverlay(".", "dnsNames", "issuerRef", "secretName", "duration", "renewBefore"))
}
