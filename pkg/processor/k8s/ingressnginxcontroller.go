package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// IngressNginxControllerProcessor processes Deckhouse IngressNginxController resources (deckhouse.io/v1).
type IngressNginxControllerProcessor struct {
	processor.BaseProcessor
}

// NewIngressNginxControllerProcessor creates a new IngressNginxController processor.
func NewIngressNginxControllerProcessor() *IngressNginxControllerProcessor {
	return &IngressNginxControllerProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"ingressnginxcontroller",
			50,
			schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: "IngressNginxController"},
		),
	}
}

// Process processes an IngressNginxController resource.
func (p *IngressNginxControllerProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("IngressNginxController object is nil")
	}

	name := obj.GetName()
	serviceName := name

	values := p.extractValues(obj)
	template := p.generateTemplate(ctx, obj, serviceName)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/ingressnginxcontroller-%s.yaml", name),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.ingressNginxController", serviceName),
		Values:          values,
		Metadata: map[string]interface{}{
			"name": name,
		},
	}, nil
}

func (p *IngressNginxControllerProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	// Preserve full spec for pipeline integration.
	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	// Extract spec.ingressClass
	if ingressClass, ok, _ := unstructured.NestedString(obj.Object, "spec", "ingressClass"); ok {
		values["ingressClass"] = ingressClass
	}

	// Extract spec.inlet (enum: LoadBalancer, HostPort, HostWithFailover)
	if inlet, ok, _ := unstructured.NestedString(obj.Object, "spec", "inlet"); ok {
		values["inlet"] = inlet
	}

	// Extract spec.controllerVersion
	if version, ok, _ := unstructured.NestedString(obj.Object, "spec", "controllerVersion"); ok {
		values["controllerVersion"] = version
	}

	// Extract spec.resourcesRequests as a whole map
	if rr, ok, _ := unstructured.NestedMap(obj.Object, "spec", "resourcesRequests"); ok {
		values["resourcesRequests"] = rr
	}

	return values
}

func (p *IngressNginxControllerProcessor) generateTemplate(ctx processor.Context, obj *unstructured.Unstructured, serviceName string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- with $svc.ingressNginxController }}
apiVersion: deckhouse.io/v1
kind: IngressNginxController
metadata:
  name: %s
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  ingressClass: {{ .ingressClass | quote }}
  inlet: {{ .inlet | quote }}
  {{- with .controllerVersion }}
  controllerVersion: {{ . | quote }}
  {{- end }}
  {{- with .resourcesRequests }}
  resourcesRequests:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
`, sanitized, serviceName, ctx.ChartName)
}
