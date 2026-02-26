package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// VPAProcessor processes Kubernetes VerticalPodAutoscaler resources (autoscaling.k8s.io/v1).
type VPAProcessor struct {
	processor.BaseProcessor
}

// NewVPAProcessor creates a new VPA processor.
func NewVPAProcessor() *VPAProcessor {
	return &VPAProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"vpa",
			90,
			schema.GroupVersionKind{Group: "autoscaling.k8s.io", Version: "v1", Kind: "VerticalPodAutoscaler"},
		),
	}
}

// Process processes a VPA resource.
func (p *VPAProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("VPA object is nil")
	}

	serviceName := processor.SanitizeServiceName(processor.ServiceNameFromResource(obj))
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	values, deps := p.extractValues(obj)

	template := p.generateTemplate(ctx, serviceName)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-vpa.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.vpa", serviceName),
		Values:          values,
		Dependencies:    deps,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *VPAProcessor) extractValues(obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey

	// Extract targetRef
	if targetRef, ok, _ := unstructured.NestedMap(obj.Object, "spec", "targetRef"); ok {
		values["targetRef"] = targetRef

		// Add dependency on the target resource
		targetKind, _ := targetRef["kind"].(string)
		targetName, _ := targetRef["name"].(string)
		if targetKind != "" && targetName != "" {
			apiVersion, _ := targetRef["apiVersion"].(string)
			group := ""
			if apiVersion != "" {
				parts := splitAPIVersion(apiVersion)
				group = parts[0]
			}
			deps = append(deps, types.ResourceKey{
				GVK: schema.GroupVersionKind{
					Group: group,
					Kind:  targetKind,
				},
				Namespace: obj.GetNamespace(),
				Name:      targetName,
			})
		}
	}

	// Extract updatePolicy
	if updatePolicy, ok, _ := unstructured.NestedMap(obj.Object, "spec", "updatePolicy"); ok {
		values["updatePolicy"] = updatePolicy
	}

	// Extract resourcePolicy
	if resourcePolicy, ok, _ := unstructured.NestedMap(obj.Object, "spec", "resourcePolicy"); ok {
		values["resourcePolicy"] = resourcePolicy
	}

	// Extract recommenders
	if recommenders, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "recommenders"); ok && len(recommenders) > 0 {
		values["recommenders"] = recommenders
	}

	return values, deps
}

func (p *VPAProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	fullnameHelper := fmt.Sprintf(`{{ include "%s.fullname" $ }}`, ctx.ChartName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.vpa }}
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: %s-%s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s
spec:
  targetRef:
    apiVersion: {{ .targetRef.apiVersion | default "apps/v1" }}
    kind: {{ .targetRef.kind | default "Deployment" }}
    name: %s-{{ .targetRef.name }}
  {{- with .updatePolicy }}
  updatePolicy:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .resourcePolicy }}
  resourcePolicy:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end }}
`, serviceName, fullnameHelper, serviceName, ctx.ChartName, serviceName, fullnameHelper)
}
