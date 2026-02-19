package k8s

import (
	"errors"
	"fmt"
	"strings"

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

	template := p.generateTemplate(ctx, serviceName)

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

	// Extract containers from pod template
	if containers, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "jobTemplate", "spec", "template", "spec", "containers"); ok {
		extractedContainers := make([]map[string]interface{}, 0, len(containers))
		for _, c := range containers {
			cm, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			container := make(map[string]interface{})
			if name, ok := cm["name"].(string); ok {
				container["name"] = name
			}
			if image, ok := cm["image"].(string); ok {
				parts := strings.SplitN(image, ":", 2)
				imageMap := map[string]interface{}{"repository": parts[0]}
				if len(parts) > 1 {
					imageMap["tag"] = parts[1]
				}
				container["image"] = imageMap
			}
			if resources, ok := cm["resources"].(map[string]interface{}); ok {
				container["resources"] = resources
			}
			if env, ok := cm["env"].([]interface{}); ok {
				container["env"] = env
			}
			if cmd, ok := cm["command"].([]interface{}); ok {
				container["command"] = cmd
			}
			if args, ok := cm["args"].([]interface{}); ok {
				container["args"] = args
			}
			extractedContainers = append(extractedContainers, container)
		}
		if len(extractedContainers) > 0 {
			values["containers"] = extractedContainers
		}
	}

	// Extract restartPolicy
	if policy, ok, _ := unstructured.NestedString(obj.Object, "spec", "jobTemplate", "spec", "template", "spec", "restartPolicy"); ok {
		values["restartPolicy"] = policy
	}

	return values, deps
}

func (p *CronJobProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	fullnameHelper := fmt.Sprintf(`{{ include "%s.fullname" $ }}`, ctx.ChartName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.cronJob }}
apiVersion: batch/v1
kind: CronJob
metadata:
  name: %s-%s
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
  {{- if .suspend }}
  suspend: {{ .suspend }}
  {{- end }}
  {{- with .successfulJobsHistoryLimit }}
  successfulJobsHistoryLimit: {{ . }}
  {{- end }}
  {{- with .failedJobsHistoryLimit }}
  failedJobsHistoryLimit: {{ . }}
  {{- end }}
  {{- with .startingDeadlineSeconds }}
  startingDeadlineSeconds: {{ . }}
  {{- end }}
  jobTemplate:
    spec:
      {{- with .jobTemplate }}
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
      {{- end }}
      template:
        spec:
          restartPolicy: {{ .restartPolicy | default "OnFailure" }}
          {{- with .containers }}
          containers:
            {{- range . }}
            - name: {{ .name }}
              image: "{{ .image.repository }}:{{ .image.tag | default "latest" }}"
              {{- with .command }}
              command:
                {{- toYaml . | nindent 16 }}
              {{- end }}
              {{- with .args }}
              args:
                {{- toYaml . | nindent 16 }}
              {{- end }}
              {{- with .resources }}
              resources:
                {{- toYaml . | nindent 16 }}
              {{- end }}
            {{- end }}
          {{- end }}
{{- end }}
{{- end }}
`, serviceName, fullnameHelper, serviceName, ctx.ChartName, serviceName)
}
