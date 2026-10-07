package generator

import (
	"fmt"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// HookType identifies a Helm hook lifecycle event.
type HookType string

const (
	HookPreUpgrade  HookType = "pre-upgrade"
	HookPostInstall HookType = "post-install"
	HookPreDelete   HookType = "pre-delete"
)

// hookDefinition describes a single Helm hook Job template.
type hookDefinition struct {
	HookType    HookType
	Suffix      string
	Weight      string
	Command     []string
	Description string
}

// defaultHooks returns the three standard hook definitions.
func defaultHooks() []hookDefinition {
	return []hookDefinition{
		{
			HookType:    HookPreUpgrade,
			Suffix:      "pre-upgrade",
			Weight:      "-5",
			Command:     []string{"/bin/sh", "-c", "echo 'Running database migration...'"},
			Description: "Database migration job executed before upgrade",
		},
		{
			HookType:    HookPostInstall,
			Suffix:      "post-install",
			Weight:      "0",
			Command:     []string{"/bin/sh", "-c", "echo 'Running smoke test...'"},
			Description: "Smoke test job executed after install",
		},
		{
			HookType:    HookPreDelete,
			Suffix:      "pre-delete",
			Weight:      "-5",
			Command:     []string{"/bin/sh", "-c", "echo 'Running cleanup...'"},
			Description: "Cleanup job executed before delete",
		},
	}
}

// GenerateHelmHooks produces Job templates for standard Helm lifecycle hooks.
// Returns an empty map if the chart is nil or has no templates.
// The hook image can be overridden with .Values.hooks.image.
func GenerateHelmHooks(chart *types.GeneratedChart) map[string]string {
	if chart == nil || len(chart.Templates) == 0 {
		return map[string]string{}
	}

	hooks := defaultHooks()
	result := make(map[string]string, len(hooks))

	for _, h := range hooks {
		path := fmt.Sprintf("templates/hooks/%s-job.yaml", h.Suffix)
		result[path] = renderHookJob(chart.Name, h)
	}

	return result
}

// renderHookJob builds the YAML for a single hook Job template.
func renderHookJob(chartName string, h hookDefinition) string {
	// Build command array as YAML list
	cmdYAML := ""
	for _, c := range h.Command {
		cmdYAML += fmt.Sprintf("            - %q\n", c)
	}

	// Hook pods deliberately do not carry the chart's selector labels, so
	// Services of the release never route traffic to them.
	return fmt.Sprintf(`# %[1]s
{{- $hooks := .Values.hooks | default dict }}
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ include "%[2]s.fullname" . }}-%[3]s
  labels:
    {{- include "%[2]s.labels" . | nindent 4 }}
  annotations:
    "helm.sh/hook": %[4]s
    "helm.sh/hook-weight": "%[5]s"
    "helm.sh/hook-delete-policy": before-hook-creation
spec:
  backoffLimit: 1
  template:
    metadata:
      labels:
        app.kubernetes.io/instance: {{ .Release.Name }}
        app.kubernetes.io/component: hook-%[3]s
    spec:
      restartPolicy: Never
      containers:
        - name: %[3]s
          image: {{ $hooks.image | default "busybox:1.36" | quote }}
          command:
%[6]s`, h.Description, chartName, h.Suffix, string(h.HookType), h.Weight, cmdYAML)
}
