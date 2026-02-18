package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// NetworkPolicyProcessor processes Kubernetes NetworkPolicy resources.
type NetworkPolicyProcessor struct {
	processor.BaseProcessor
}

// NewNetworkPolicyProcessor creates a new NetworkPolicy processor.
func NewNetworkPolicyProcessor() *NetworkPolicyProcessor {
	return &NetworkPolicyProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"networkpolicy",
			90,
			schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"},
		),
	}
}

// Process processes a NetworkPolicy resource.
func (p *NetworkPolicyProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("NetworkPolicy object is nil")
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
		TemplatePath:    fmt.Sprintf("templates/%s-networkpolicy.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.networkPolicy", serviceName),
		Values:          values,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *NetworkPolicyProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	// podSelector
	if podSelector, ok, _ := unstructured.NestedMap(obj.Object, "spec", "podSelector"); ok {
		values["podSelector"] = podSelector
	}

	// policyTypes
	if policyTypes, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "policyTypes"); ok && len(policyTypes) > 0 {
		values["policyTypes"] = policyTypes
	}

	// ingress rules
	if ingress, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "ingress"); ok && len(ingress) > 0 {
		values["ingress"] = ingress
	}

	// egress rules
	if egress, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "egress"); ok && len(egress) > 0 {
		values["egress"] = egress
	}

	return values
}

func (p *NetworkPolicyProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	fullnameHelper := fmt.Sprintf(`{{ include "%s.fullname" $ }}`, ctx.ChartName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.networkPolicy }}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: %s-%s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s
spec:
  {{- with .podSelector }}
  podSelector:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .policyTypes }}
  policyTypes:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .ingress }}
  ingress:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .egress }}
  egress:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end }}
`, serviceName, fullnameHelper, serviceName, ctx.ChartName, serviceName)
}
