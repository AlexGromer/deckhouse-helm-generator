package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// PDBProcessor processes Kubernetes PodDisruptionBudget resources (policy/v1).
type PDBProcessor struct {
	processor.BaseProcessor
}

// NewPDBProcessor creates a new PDB processor.
func NewPDBProcessor() *PDBProcessor {
	return &PDBProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"pdb",
			90,
			schema.GroupVersionKind{Group: "policy", Version: "v1", Kind: "PodDisruptionBudget"},
		),
	}
}

// Process processes a PodDisruptionBudget resource.
func (p *PDBProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("PDB object is nil")
	}

	serviceName := processor.SanitizeServiceName(processor.ServiceNameFromResource(obj))
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
		TemplatePath:    fmt.Sprintf("templates/%s-pdb.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.pdb", serviceName),
		Values:          values,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *PDBProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	// minAvailable can be int or string (percentage)
	if val, exists, _ := unstructured.NestedFieldNoCopy(obj.Object, "spec", "minAvailable"); exists {
		values["minAvailable"] = val
	}

	// maxUnavailable can be int or string (percentage)
	if val, exists, _ := unstructured.NestedFieldNoCopy(obj.Object, "spec", "maxUnavailable"); exists {
		values["maxUnavailable"] = val
	}

	// selector
	if selector, ok, _ := unstructured.NestedMap(obj.Object, "spec", "selector"); ok {
		values["selector"] = selector
	}

	// unhealthyPodEvictionPolicy (v1 feature gate)
	if policy, ok, _ := unstructured.NestedString(obj.Object, "spec", "unhealthyPodEvictionPolicy"); ok {
		values["unhealthyPodEvictionPolicy"] = policy
	}

	return values
}

func (p *PDBProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	fullnameHelper := fmt.Sprintf(`{{ include "%s.fullname" $ }}`, ctx.ChartName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.pdb }}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: %s-%s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s
spec:
  {{- with .minAvailable }}
  minAvailable: {{ . }}
  {{- end }}
  {{- with .maxUnavailable }}
  maxUnavailable: {{ . }}
  {{- end }}
  {{- with .unhealthyPodEvictionPolicy }}
  unhealthyPodEvictionPolicy: {{ . }}
  {{- end }}
  {{- with .selector }}
  selector:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end }}
`, serviceName, fullnameHelper, serviceName, ctx.ChartName, serviceName)
}
