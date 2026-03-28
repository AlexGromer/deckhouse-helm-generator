package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// FlaggerCanaryProcessor processes Flagger Canary resources.
type FlaggerCanaryProcessor struct {
	processor.BaseProcessor
}

// NewFlaggerCanaryProcessor creates a new Flagger Canary processor.
func NewFlaggerCanaryProcessor() *FlaggerCanaryProcessor {
	return &FlaggerCanaryProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"canary",
			70,
			schema.GroupVersionKind{Group: "flagger.app", Version: "v1beta1", Kind: "Canary"},
		),
	}
}

// Process processes a Flagger Canary resource.
func (p *FlaggerCanaryProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("Canary object is nil")
	}

	serviceName := processor.ServiceNameFromResource(obj)
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
		TemplatePath:    fmt.Sprintf("templates/%s-canary.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.canary", serviceName),
		Values:          values,
		Dependencies:    deps,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *FlaggerCanaryProcessor) extractValues(obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey

	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	// Extract targetRef
	if targetRef, ok, _ := unstructured.NestedMap(obj.Object, "spec", "targetRef"); ok {
		values["targetRef"] = targetRef

		// Create dependency to target workload
		targetKind, _ := targetRef["kind"].(string)
		targetName, _ := targetRef["name"].(string)
		if targetKind != "" && targetName != "" {
			apiVersion, _ := targetRef["apiVersion"].(string)
			group := ""
			version := "v1"
			if apiVersion != "" {
				parts := splitAPIVersion(apiVersion)
				group = parts[0]
				version = parts[1]
			} else if targetKind == "Deployment" || targetKind == "DaemonSet" {
				group = "apps"
			}
			deps = append(deps, types.ResourceKey{
				GVK:       schema.GroupVersionKind{Group: group, Version: version, Kind: targetKind},
				Namespace: obj.GetNamespace(),
				Name:      targetName,
			})
		}
	}

	// Extract analysis
	if analysis, ok, _ := unstructured.NestedMap(obj.Object, "spec", "analysis"); ok {
		values["analysis"] = analysis
	}

	// Extract progressDeadlineSeconds
	if deadline, ok, _ := unstructured.NestedInt64(obj.Object, "spec", "progressDeadlineSeconds"); ok {
		values["progressDeadlineSeconds"] = deadline
	}

	return values, deps
}

// splitAPIVersion is defined in hpa.go (shared utility for this package).

func (p *FlaggerCanaryProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.canary }}
apiVersion: flagger.app/v1beta1
kind: Canary
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  {{- with .targetRef }}
  targetRef:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .progressDeadlineSeconds }}
  progressDeadlineSeconds: {{ . }}
  {{- end }}
  {{- with .analysis }}
  analysis:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end }}
`, sanitized, serviceName, ctx.ChartName)
}
