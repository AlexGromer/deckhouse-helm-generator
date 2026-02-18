package generator

// Shared sub-template constants for DRY library chart.
// These are named templates included by workload resource templates.

// resourcesTemplate renders a container's resources block when .values.resources is set.
const resourcesTemplate = `{{- if .values.resources }}
resources:
  {{- toYaml .values.resources | nindent 2 }}
{{- end }}`

// probesTemplate renders liveness, readiness and startup probes.
const probesTemplate = `{{- if .values.livenessProbe }}
livenessProbe:
  {{- toYaml .values.livenessProbe | nindent 2 }}
{{- end }}
{{- if .values.readinessProbe }}
readinessProbe:
  {{- toYaml .values.readinessProbe | nindent 2 }}
{{- end }}
{{- if .values.startupProbe }}
startupProbe:
  {{- toYaml .values.startupProbe | nindent 2 }}
{{- end }}`

// securityContextTemplate renders pod-level securityContext.
const securityContextTemplate = `{{- if .values.podSecurityContext }}
securityContext:
  {{- toYaml .values.podSecurityContext | nindent 2 }}
{{- end }}`

// containerSecurityContextTemplate renders container-level securityContext.
const containerSecurityContextTemplate = `{{- if .values.securityContext }}
securityContext:
  {{- toYaml .values.securityContext | nindent 2 }}
{{- end }}`

// envTemplate renders environment variables for a container.
const envTemplate = `{{- if .values.env }}
env:
  {{- toYaml .values.env | nindent 2 }}
{{- end }}`

// volumeMountsTemplate renders volume mounts for a container.
const volumeMountsTemplate = `{{- if .values.volumeMounts }}
volumeMounts:
  {{- toYaml .values.volumeMounts | nindent 2 }}
{{- end }}`

// volumesTemplate renders pod-level volumes.
const volumesTemplate = `{{- if .values.volumes }}
volumes:
  {{- toYaml .values.volumes | nindent 2 }}
{{- end }}`

// annotationsTemplate renders metadata annotations.
const annotationsTemplate = `{{- with .values.annotations }}
annotations:
  {{- toYaml . | nindent 2 }}
{{- end }}`

// addSharedSubTemplates registers all DRY sub-templates into the library chart's template map.
func addSharedSubTemplates(templates map[string]string) {
	templates["templates/_resources.tpl"] = generateNamedTemplate("resources", resourcesTemplate)
	templates["templates/_probes.tpl"] = generateNamedTemplate("probes", probesTemplate)
	templates["templates/_securitycontext.tpl"] = generateNamedTemplate("securityContext", securityContextTemplate) +
		generateNamedTemplate("containerSecurityContext", containerSecurityContextTemplate)
	templates["templates/_env.tpl"] = generateNamedTemplate("env", envTemplate)
	templates["templates/_volumemounts.tpl"] = generateNamedTemplate("volumeMounts", volumeMountsTemplate) +
		generateNamedTemplate("volumes", volumesTemplate)
	templates["templates/_annotations.tpl"] = generateNamedTemplate("annotations", annotationsTemplate)
}
