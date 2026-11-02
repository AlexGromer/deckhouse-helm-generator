package k8s

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
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
		return nil, errors.New("nil Job object")
	}

	serviceName := processor.SanitizeServiceName(processor.ServiceNameFromResource(obj))
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	values, deps := p.extractValues(obj, finishedJobState(ctx, obj))
	setHookEvents(ctx, values, deps)

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

// Defaults of the Job hook switches (values <job>.hook.*).
const (
	jobHookEvents = "pre-install,pre-upgrade"
	// jobHookEventsAfter is the default for a Job that uses objects of the
	// chart: a pre-install hook runs before any of them exists.
	jobHookEventsAfter  = "post-install,post-upgrade"
	jobHookDeletePolicy = "before-hook-creation"
	jobHookWeight       = "0"
)

// setHookEvents makes a Job that reads an object of the chart (a Secret,
// ConfigMap, ServiceAccount or claim in the input) run after the install and
// the upgrade, not before: a pre-install hook runs before any object of the
// chart exists and would hang on the missing object until the timeout.
func setHookEvents(ctx processor.Context, values map[string]interface{}, deps []types.ResourceKey) {
	hook, ok := values["hook"].(map[string]interface{})
	if !ok {
		return
	}
	for _, dep := range deps {
		if _, inChart := ctx.AllResources[dep]; inChart {
			hook["events"] = jobHookEventsAfter
			return
		}
	}
}

// finishedJobState returns "succeeded" or "failed" when the cluster extractor
// found obj already run to its end, and "" for any other Job.
func finishedJobState(ctx processor.Context, obj *unstructured.Unstructured) string {
	r := ctx.AllResources[types.ResourceKey{
		GVK:       obj.GroupVersionKind(),
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}]
	if r == nil {
		return ""
	}
	return r.FinishedState
}

// hasHookAnnotation reports whether the input already carries a Helm hook
// annotation (helm.sh/hook, helm.sh/hook-weight, ...). Such a Job keeps its
// own hook annotations and gets no hook switch, so no key is rendered twice.
func hasHookAnnotation(annotations map[string]string) bool {
	for k := range annotations {
		if strings.HasPrefix(k, "helm.sh/hook") {
			return true
		}
	}
	return false
}

func (p *JobProcessor) extractValues(obj *unstructured.Unstructured, finished string) (map[string]interface{}, []types.ResourceKey) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey

	// Switches: the whole Job can be turned off, and a Job that finished
	// successfully in the cluster is rendered as a Helm hook by default, so it
	// does not rerun on every install. A failed Job is not: a hook that fails
	// aborts the install and every upgrade.
	values["enabled"] = true
	if !hasHookAnnotation(obj.GetAnnotations()) {
		values["hook"] = map[string]interface{}{
			"enabled":      finished == "succeeded",
			"events":       jobHookEvents,
			"deletePolicy": jobHookDeletePolicy,
			"weight":       jobHookWeight,
		}
	}

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
// Helm annotations of the input are embedded directly; regular annotations use
// toYaml from values. A Job whose input has no hook annotation also renders
// the helm.sh/hook* annotations from the hook switches of its values.
func buildAnnotationsBlock(annotations map[string]string) string {
	switchHook := !hasHookAnnotation(annotations)

	// Collect Helm annotations of the input
	var helmLines []string
	keys := make([]string, 0, len(annotations))
	for k := range annotations {
		if strings.HasPrefix(k, "helm.sh/") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	for _, k := range keys {
		helmLines = append(helmLines, fmt.Sprintf("    %s: %q", k, annotations[k]))
	}

	if len(helmLines) == 0 && !switchHook {
		// Only regular annotations — use toYaml from values
		return `  {{- with .annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}`
	}

	// Build combined annotations block
	var sb strings.Builder
	closeIf := false
	if len(helmLines) == 0 {
		// Nothing literal: emit the key only when something goes under it.
		sb.WriteString("  {{- if or $hook.enabled .annotations }}\n")
		closeIf = true
	}
	sb.WriteString("  annotations:\n")
	for _, line := range helmLines {
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	if switchHook {
		sb.WriteString("    {{- if $hook.enabled }}\n")
		sb.WriteString("    helm.sh/hook: {{ default \"" + jobHookEvents + "\" $hook.events | quote }}\n")
		sb.WriteString("    helm.sh/hook-weight: {{ default \"" + jobHookWeight + "\" $hook.weight | quote }}\n")
		sb.WriteString("    helm.sh/hook-delete-policy: {{ default \"" + jobHookDeletePolicy + "\" $hook.deletePolicy | quote }}\n")
		sb.WriteString("    {{- end }}\n")
	}
	sb.WriteString("    {{- with .annotations }}\n")
	sb.WriteString("    {{- toYaml . | nindent 4 }}\n")
	sb.WriteString("    {{- end }}")
	if closeIf {
		sb.WriteString("\n  {{- end }}")
	}

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
{{- if (hasKey . "enabled" | ternary .enabled true) }}
{{- $hook := default (dict) .hook }}
apiVersion: batch/v1
kind: Job
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s%s
spec:
  {{- if hasKey . "completions" }}
  completions: {{ .completions }}
  {{- end }}
  {{- if hasKey . "parallelism" }}
  parallelism: {{ .parallelism }}
  {{- end }}
  {{- if hasKey . "backoffLimit" }}
  backoffLimit: {{ .backoffLimit }}
  {{- end }}
  {{- if hasKey . "activeDeadlineSeconds" }}
  activeDeadlineSeconds: {{ .activeDeadlineSeconds }}
  {{- end }}
  {{- if hasKey . "ttl" }}
  ttlSecondsAfterFinished: {{ .ttl }}
  {{- end }}
  {{- with .completionMode }}
  completionMode: {{ . }}
  {{- end }}
  {{- if hasKey . "suspend" }}
  suspend: {{ .suspend }}
  {{- end }}
%s{{- end }}
{{- end }}
{{- end }}
`, serviceName, processor.ObjName(ctx.ChartName, name), ctx.ChartName, serviceName, annotationsPart,
		podTemplate(ctx.ChartName, 2, "Never", "Job/"+name))
}
