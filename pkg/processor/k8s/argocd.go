package k8s

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
)

func extractArgoCDAnnotations(obj *unstructured.Unstructured) map[string]string {
	annotations := obj.GetAnnotations()
	if len(annotations) == 0 {
		return nil
	}

	result := make(map[string]string)
	for k, v := range annotations {
		if k == "argocd.argoproj.io/sync-wave" || k == "argocd.argoproj.io/hook" || k == "argocd.argoproj.io/hook-delete-policy" {
			result[k] = v
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// buildArgoCDAnnotationsBlock generates inline annotation YAML for the template.
func buildArgoCDAnnotationsBlock(annotations map[string]string) string {
	if len(annotations) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("  annotations:\n")

	// Add static extracted annotations (Helm format to pick up from values)
	sb.WriteString("    {{- with .annotations }}\n")
	sb.WriteString("    {{- toYaml . | nindent 4 }}\n")
	sb.WriteString("    {{- end }}")

	return sb.String()
}

func buildArgoCDValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	if project, ok, _ := unstructured.NestedString(obj.Object, "spec", "project"); ok {
		values["project"] = project
	}

	if source, ok, _ := unstructured.NestedMap(obj.Object, "spec", "source"); ok {
		extractedSource := make(map[string]interface{})
		for _, key := range []string{"repoURL", "path", "targetRevision", "chart", "helm"} {
			if val, ok := source[key]; ok {
				extractedSource[key] = val
			}
		}
		if len(extractedSource) > 0 {
			values["source"] = extractedSource
		}
	}

	if destination, ok, _ := unstructured.NestedMap(obj.Object, "spec", "destination"); ok {
		extractedDest := make(map[string]interface{})
		for _, key := range []string{"server", "namespace"} {
			if val, ok := destination[key]; ok {
				extractedDest[key] = val
			}
		}
		if len(extractedDest) > 0 {
			values["destination"] = extractedDest
		}
	}

	if syncPolicy, ok, _ := unstructured.NestedMap(obj.Object, "spec", "syncPolicy"); ok {
		values["syncPolicy"] = syncPolicy
	}

	annotations := extractArgoCDAnnotations(obj)
	if annotations != nil {
		values["annotations"] = annotations
	}

	return values
}

// ArgoCDApplicationProcessor processes argoproj.io/v1alpha1 Application resources.
type ArgoCDApplicationProcessor struct {
	processor.BaseProcessor
}

// NewArgoCDApplicationProcessor creates a new ArgoCD Application processor.
func NewArgoCDApplicationProcessor() *ArgoCDApplicationProcessor {
	return &ArgoCDApplicationProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"argoApplication",
			75,
			schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "Application"},
		),
	}
}

// Process processes an Application resource.
func (p *ArgoCDApplicationProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, fmt.Errorf("nil Application object")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	values := buildArgoCDValues(obj)
	annotations := extractArgoCDAnnotations(obj)

	template := p.generateTemplate(ctx, serviceName, name, annotations)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-application.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.argoApplication", serviceName),
		Values:          values,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *ArgoCDApplicationProcessor) generateTemplate(ctx processor.Context, serviceName, name string, annotations map[string]string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	annotationsPart := ""
	if len(annotations) > 0 {
		annotationsPart = "\n" + buildArgoCDAnnotationsBlock(annotations)
	}

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.argoApplication }}
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
%s
%s{{- end }}
{{- end }}
`, sanitized, processor.ObjectName(name), ctx.ChartName, annotationsPart,
		processor.SpecOverlay(".", "project", "source", "destination", "syncPolicy"))
}

// ArgoCDApplicationSetProcessor processes argoproj.io/v1alpha1 ApplicationSet resources.
type ArgoCDApplicationSetProcessor struct {
	processor.BaseProcessor
}

// NewArgoCDApplicationSetProcessor creates a new ArgoCD ApplicationSet processor.
func NewArgoCDApplicationSetProcessor() *ArgoCDApplicationSetProcessor {
	return &ArgoCDApplicationSetProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"argoApplicationSet",
			75,
			schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: "ApplicationSet"},
		),
	}
}

// Process processes an ApplicationSet resource.
func (p *ArgoCDApplicationSetProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, fmt.Errorf("nil ApplicationSet object")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	values := buildArgoCDValues(obj)

	// ApplicationSet also has generators and template.
	// We extract these via values so they are parameterized.
	if generators, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "generators"); ok {
		values["generators"] = generators
	}
	if template, ok, _ := unstructured.NestedMap(obj.Object, "spec", "template"); ok {
		values["template"] = template
	}

	annotations := extractArgoCDAnnotations(obj)

	template := p.generateTemplate(ctx, serviceName, name, annotations)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-applicationset.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.argoApplicationSet", serviceName),
		Values:          values,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *ArgoCDApplicationSetProcessor) generateTemplate(ctx processor.Context, serviceName, name string, annotations map[string]string) string {
	sanitized := processor.SanitizeServiceName(serviceName)

	annotationsPart := ""
	if len(annotations) > 0 {
		annotationsPart = "\n" + buildArgoCDAnnotationsBlock(annotations)
	}

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.argoApplicationSet }}
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
%s
%s{{- end }}
{{- end }}
`, sanitized, processor.ObjectName(name), ctx.ChartName, annotationsPart,
		processor.SpecOverlay(".", "project", "source", "destination", "syncPolicy", "generators", "template"))
}
