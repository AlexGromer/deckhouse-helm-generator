package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// CronJobProcessor processes Kubernetes CronJob resources.
type CronJobProcessor struct {
	processor.BaseProcessor
}

// NewCronJobProcessor creates a new CronJob processor.
func NewCronJobProcessor() *CronJobProcessor {
	return &CronJobProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"cronjob",
			100,
			schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"},
		),
	}
}

// Process processes a CronJob resource.
func (p *CronJobProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("CronJob object is nil")
	}

	serviceName := processor.SanitizeServiceName(processor.ServiceNameFromResource(obj))
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	values, deps := p.extractValues(obj)

	template := p.generateTemplate(ctx, serviceName, name)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-cronjob.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.cronJob", serviceName),
		Values:          values,
		Dependencies:    deps,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *CronJobProcessor) extractValues(obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey

	// Extract schedule
	if schedule, ok, _ := unstructured.NestedString(obj.Object, "spec", "schedule"); ok {
		values["schedule"] = schedule
	}

	// Extract timeZone (K8s 1.25+)
	if tz, ok, _ := unstructured.NestedString(obj.Object, "spec", "timeZone"); ok {
		values["timeZone"] = tz
	}

	// Extract concurrencyPolicy
	if policy, ok, _ := unstructured.NestedString(obj.Object, "spec", "concurrencyPolicy"); ok {
		values["concurrencyPolicy"] = policy
	}

	// Extract suspend
	if suspend, ok, _ := unstructured.NestedBool(obj.Object, "spec", "suspend"); ok {
		values["suspend"] = suspend
	}

	// Extract successfulJobsHistoryLimit
	if limit, ok := nestedInt64(obj.Object, "spec", "successfulJobsHistoryLimit"); ok {
		values["successfulJobsHistoryLimit"] = limit
	}

	// Extract failedJobsHistoryLimit
	if limit, ok := nestedInt64(obj.Object, "spec", "failedJobsHistoryLimit"); ok {
		values["failedJobsHistoryLimit"] = limit
	}

	// Extract startingDeadlineSeconds
	if deadline, ok := nestedInt64(obj.Object, "spec", "startingDeadlineSeconds"); ok {
		values["startingDeadlineSeconds"] = deadline
	}

	// Extract jobTemplate spec fields
	jobTemplate := make(map[string]interface{})
	if completions, ok := nestedInt64(obj.Object, "spec", "jobTemplate", "spec", "completions"); ok {
		jobTemplate["completions"] = completions
	}
	if parallelism, ok := nestedInt64(obj.Object, "spec", "jobTemplate", "spec", "parallelism"); ok {
		jobTemplate["parallelism"] = parallelism
	}
	if backoffLimit, ok := nestedInt64(obj.Object, "spec", "jobTemplate", "spec", "backoffLimit"); ok {
		jobTemplate["backoffLimit"] = backoffLimit
	}
	if deadline, ok := nestedInt64(obj.Object, "spec", "jobTemplate", "spec", "activeDeadlineSeconds"); ok {
		jobTemplate["activeDeadlineSeconds"] = deadline
	}
	if len(jobTemplate) > 0 {
		values["jobTemplate"] = jobTemplate
	}

	// Pod template (labels, annotations, containers, volumes, scheduling, ...)
	deps = append(deps, extractPodTemplateValues(obj, values, "spec", "jobTemplate", "spec", "template")...)

	// Extract restartPolicy
	if policy, ok, _ := unstructured.NestedString(obj.Object, "spec", "jobTemplate", "spec", "template", "spec", "restartPolicy"); ok {
		values["restartPolicy"] = policy
	}

	return values, deps
}

func (p *CronJobProcessor) generateTemplate(ctx processor.Context, serviceName, name string) string {

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.cronJob }}
apiVersion: batch/v1
kind: CronJob
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s
spec:
  schedule: {{ .schedule | quote }}
  {{- with .timeZone }}
  timeZone: {{ . | quote }}
  {{- end }}
  {{- with .concurrencyPolicy }}
  concurrencyPolicy: {{ . }}
  {{- end }}
  {{- if hasKey . "suspend" }}
  suspend: {{ .suspend }}
  {{- end }}
  {{- if hasKey . "successfulJobsHistoryLimit" }}
  successfulJobsHistoryLimit: {{ .successfulJobsHistoryLimit }}
  {{- end }}
  {{- if hasKey . "failedJobsHistoryLimit" }}
  failedJobsHistoryLimit: {{ .failedJobsHistoryLimit }}
  {{- end }}
  {{- if hasKey . "startingDeadlineSeconds" }}
  startingDeadlineSeconds: {{ .startingDeadlineSeconds }}
  {{- end }}
  jobTemplate:
    spec:
      {{- with .jobTemplate }}
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
      {{- end }}
%s{{- end }}
{{- end }}
`, serviceName, processor.ObjectName(name), ctx.ChartName, serviceName,
		podTemplate(ctx.ChartName, 6, "OnFailure"))
}
