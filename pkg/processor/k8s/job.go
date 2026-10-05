package k8s

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// JobProcessor processes Kubernetes Job resources.
type JobProcessor struct {
	processor.BaseProcessor
}

// NewJobProcessor creates a new Job processor.
func NewJobProcessor() *JobProcessor {
	return &JobProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"job",
			100,
			schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"},
		),
	}
}

// Process processes a Job resource.
func (p *JobProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("Job object is nil")
	}

	serviceName := processor.SanitizeServiceName(processor.ServiceNameFromResource(obj))
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	values, deps := p.extractValues(obj)

	// Get annotations for inline embedding in template
	annotations := obj.GetAnnotations()

	template := p.generateTemplate(ctx, serviceName, name, annotations)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-job.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.job", serviceName),
		Values:          values,
		Dependencies:    deps,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *JobProcessor) extractValues(obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey

	// Extract completions
	if completions, ok := nestedInt64(obj.Object, "spec", "completions"); ok {
		values["completions"] = completions
	}

	// Extract parallelism
	if parallelism, ok := nestedInt64(obj.Object, "spec", "parallelism"); ok {
		values["parallelism"] = parallelism
	}

	// Extract backoffLimit
	if backoffLimit, ok := nestedInt64(obj.Object, "spec", "backoffLimit"); ok {
		values["backoffLimit"] = backoffLimit
	}

	// Extract activeDeadlineSeconds
	if deadline, ok := nestedInt64(obj.Object, "spec", "activeDeadlineSeconds"); ok {
		values["activeDeadlineSeconds"] = deadline
	}

	// Extract ttlSecondsAfterFinished → mapped to "ttl"
	if ttl, ok := nestedInt64(obj.Object, "spec", "ttlSecondsAfterFinished"); ok {
		values["ttl"] = ttl
	}

	// Extract completionMode
	if mode, ok, _ := unstructured.NestedString(obj.Object, "spec", "completionMode"); ok {
		values["completionMode"] = mode
	}

	// Extract suspend
	if suspend, ok, _ := unstructured.NestedBool(obj.Object, "spec", "suspend"); ok {
		values["suspend"] = suspend
	}

	// Extract non-Helm-hook annotations to values
	if annotations := obj.GetAnnotations(); len(annotations) > 0 {
		regularAnnotations := make(map[string]string)
		for k, v := range annotations {
			if !strings.HasPrefix(k, "helm.sh/") {
				regularAnnotations[k] = v
			}
		}
		if len(regularAnnotations) > 0 {
			values["annotations"] = regularAnnotations
		}
	}

	// Pod template (labels, annotations, containers, volumes, scheduling, ...)
	deps = append(deps, extractPodTemplateValues(obj, values, "spec", "template")...)

	// Extract restartPolicy
	if policy, ok, _ := unstructured.NestedString(obj.Object, "spec", "template", "spec", "restartPolicy"); ok {
		values["restartPolicy"] = policy
	}

	return values, deps
}

// buildAnnotationsBlock generates inline annotation YAML for the template.
// Helm hook annotations are embedded directly; regular annotations use toYaml from values.
func buildAnnotationsBlock(annotations map[string]string) string {
	if len(annotations) == 0 {
		return ""
	}

	// Collect Helm hook annotations
	var helmHooks []string
	keys := make([]string, 0, len(annotations))
	for k := range annotations {
		if strings.HasPrefix(k, "helm.sh/") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	for _, k := range keys {
		helmHooks = append(helmHooks, fmt.Sprintf("    %s: %q", k, annotations[k]))
	}

	if len(helmHooks) == 0 {
		// Only regular annotations — use toYaml from values
		return `  {{- with .annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}`
	}

	// Build combined annotations block
	var sb strings.Builder
	sb.WriteString("  annotations:\n")
	for _, line := range helmHooks {
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString("    {{- with .annotations }}\n")
	sb.WriteString("    {{- toYaml . | nindent 4 }}\n")
	sb.WriteString("    {{- end }}")

	return sb.String()
}

func (p *JobProcessor) generateTemplate(ctx processor.Context, serviceName, name string, annotations map[string]string) string {
	annotationsBlock := buildAnnotationsBlock(annotations)

	// Add newline before annotations block if it exists
	annotationsPart := ""
	if annotationsBlock != "" {
		annotationsPart = "\n" + annotationsBlock
	}

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.job }}
apiVersion: batch/v1
kind: Job
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s%s
spec:
  {{- with .completions }}
  completions: {{ . }}
  {{- end }}
  {{- with .parallelism }}
  parallelism: {{ . }}
  {{- end }}
  {{- with .backoffLimit }}
  backoffLimit: {{ . }}
  {{- end }}
  {{- with .activeDeadlineSeconds }}
  activeDeadlineSeconds: {{ . }}
  {{- end }}
  {{- with .ttl }}
  ttlSecondsAfterFinished: {{ . }}
  {{- end }}
  {{- with .completionMode }}
  completionMode: {{ . }}
  {{- end }}
  {{- if .suspend }}
  suspend: {{ .suspend }}
  {{- end }}
%s{{- end }}
{{- end }}
`, serviceName, processor.ObjectName(name), ctx.ChartName, serviceName, annotationsPart,
		podTemplate(ctx.ChartName, 2, "Never"))
}
