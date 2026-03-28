package generator

import (
	"fmt"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
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
	HookType     HookType
	Suffix       string
	Weight       string
	Command      []string
	Description  string
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
func GenerateHelmHooks(chart *types.GeneratedChart) map[string]string {
	if chart == nil || len(chart.Templates) == 0 {
		return map[string]string{}
	}

	hooks := defaultHooks()
	result := make(map[string]string, len(hooks))

	for _, h := range hooks {
		path := fmt.Sprintf("templates/hooks/%s-job.yaml", h.Suffix)
		result[path] = renderHookJob(h)
	}

	return result
}

// renderHookJob builds the YAML for a single hook Job template.
func renderHookJob(h hookDefinition) string {
	// Build command array as YAML list
	cmdYAML := ""
	for _, c := range h.Command {
		cmdYAML += fmt.Sprintf("            - %q\n", c)
	}

	return fmt.Sprintf(`# %s
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ include "chartname.fullname" . }}-%s
  labels:
    {{- include "chartname.labels" . | nindent 4 }}
  annotations:
    "helm.sh/hook": %s
    "helm.sh/hook-weight": "%s"
    "helm.sh/hook-delete-policy": before-hook-creation
spec:
  template:
    metadata:
      labels:
        {{- include "chartname.selectorLabels" . | nindent 8 }}
    spec:
      restartPolicy: Never
      containers:
        - name: %s
          image: "{{ .Values.image.repository | default \"busybox\" }}:{{ .Values.image.tag | default \"latest\" }}"
          command:
%s  backoffLimit: 1
`, h.Description, h.Suffix, string(h.HookType), h.Weight, h.Suffix, cmdYAML)
}
