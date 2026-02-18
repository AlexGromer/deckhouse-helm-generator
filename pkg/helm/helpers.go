package helm

import (
	"fmt"
	"strings"
)

// GenerateHelpers generates the _helpers.tpl content.
func GenerateHelpers(chartName string) string {
	var sb strings.Builder

	sb.WriteString("{{/*\n")
	sb.WriteString("Expand the name of the chart.\n")
	sb.WriteString("*/}}\n")
	sb.WriteString(fmt.Sprintf("{{- define \"%s.name\" -}}\n", chartName))
	sb.WriteString("{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix \"-\" }}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\n")
	sb.WriteString("Create a default fully qualified app name.\n")
	sb.WriteString("We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).\n")
	sb.WriteString("If release name contains chart name it will be used as a full name.\n")
	sb.WriteString("*/}}\n")
	sb.WriteString(fmt.Sprintf("{{- define \"%s.fullname\" -}}\n", chartName))
	sb.WriteString("{{- if .Values.fullnameOverride }}\n")
	sb.WriteString("{{- .Values.fullnameOverride | trunc 63 | trimSuffix \"-\" }}\n")
	sb.WriteString("{{- else }}\n")
	sb.WriteString("{{- $name := default .Chart.Name .Values.nameOverride }}\n")
	sb.WriteString("{{- if contains $name .Release.Name }}\n")
	sb.WriteString("{{- .Release.Name | trunc 63 | trimSuffix \"-\" }}\n")
	sb.WriteString("{{- else }}\n")
	sb.WriteString("{{- printf \"%s-%s\" .Release.Name $name | trunc 63 | trimSuffix \"-\" }}\n")
	sb.WriteString("{{- end }}\n")
	sb.WriteString("{{- end }}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\n")
	sb.WriteString("Create chart name and version as used by the chart label.\n")
	sb.WriteString("*/}}\n")
	sb.WriteString(fmt.Sprintf("{{- define \"%s.chart\" -}}\n", chartName))
	sb.WriteString("{{- printf \"%s-%s\" .Chart.Name .Chart.Version | replace \"+\" \"_\" | trunc 63 | trimSuffix \"-\" }}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\n")
	sb.WriteString("Common labels\n")
	sb.WriteString("*/}}\n")
	sb.WriteString(fmt.Sprintf("{{- define \"%s.labels\" -}}\n", chartName))
	sb.WriteString(fmt.Sprintf("helm.sh/chart: {{ include \"%s.chart\" . }}\n", chartName))
	sb.WriteString(fmt.Sprintf("{{ include \"%s.selectorLabels\" . }}\n", chartName))
	sb.WriteString("{{- if .Chart.AppVersion }}\n")
	sb.WriteString("app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}\n")
	sb.WriteString("{{- end }}\n")
	sb.WriteString("app.kubernetes.io/managed-by: {{ .Release.Service }}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\n")
	sb.WriteString("Selector labels\n")
	sb.WriteString("*/}}\n")
	sb.WriteString(fmt.Sprintf("{{- define \"%s.selectorLabels\" -}}\n", chartName))
	sb.WriteString(fmt.Sprintf("app.kubernetes.io/name: {{ include \"%s.name\" . }}\n", chartName))
	sb.WriteString("app.kubernetes.io/instance: {{ .Release.Name }}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\n")
	sb.WriteString("Create the name of the service account to use\n")
	sb.WriteString("*/}}\n")
	sb.WriteString(fmt.Sprintf("{{- define \"%s.serviceAccountName\" -}}\n", chartName))
	sb.WriteString("{{- if .Values.serviceAccount.create }}\n")
	sb.WriteString(fmt.Sprintf("{{- default (include \"%s.fullname\" .) .Values.serviceAccount.name }}\n", chartName))
	sb.WriteString("{{- else }}\n")
	sb.WriteString("{{- default \"default\" .Values.serviceAccount.name }}\n")
	sb.WriteString("{{- end }}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\n")
	sb.WriteString("Image pull secrets\n")
	sb.WriteString("*/}}\n")
	sb.WriteString(fmt.Sprintf("{{- define \"%s.imagePullSecrets\" -}}\n", chartName))
	sb.WriteString("{{- if .Values.global.imagePullSecrets }}\n")
	sb.WriteString("imagePullSecrets:\n")
	sb.WriteString("{{- range .Values.global.imagePullSecrets }}\n")
	sb.WriteString("  - name: {{ . }}\n")
	sb.WriteString("{{- end }}\n")
	sb.WriteString("{{- end }}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\n")
	sb.WriteString("Image name helper\n")
	sb.WriteString("Combines repository, registry, and tag\n")
	sb.WriteString("*/}}\n")
	sb.WriteString(fmt.Sprintf("{{- define \"%s.image\" -}}\n", chartName))
	sb.WriteString("{{- $registry := .registry | default .global.imageRegistry -}}\n")
	sb.WriteString("{{- $repository := .repository | required \"image repository is required\" -}}\n")
	sb.WriteString("{{- $tag := .tag | default .global.imageTag | default \"latest\" -}}\n")
	sb.WriteString("{{- if $registry }}\n")
	sb.WriteString("{{- printf \"%s/%s:%s\" $registry $repository $tag -}}\n")
	sb.WriteString("{{- else }}\n")
	sb.WriteString("{{- printf \"%s:%s\" $repository $tag -}}\n")
	sb.WriteString("{{- end }}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\n")
	sb.WriteString("Deckhouse-specific helpers\n")
	sb.WriteString("*/}}\n\n")

	sb.WriteString("{{/*\n")
	sb.WriteString("Check if Deckhouse is available\n")
	sb.WriteString("*/}}\n")
	sb.WriteString(fmt.Sprintf("{{- define \"%s.isDeckhouseAvailable\" -}}\n", chartName))
	sb.WriteString("{{- if .Values.deckhouse }}\n")
	sb.WriteString("{{- if .Values.deckhouse.enabled }}\n")
	sb.WriteString("true\n")
	sb.WriteString("{{- end }}\n")
	sb.WriteString("{{- end }}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\n")
	sb.WriteString("Generate standard annotations\n")
	sb.WriteString("*/}}\n")
	sb.WriteString(fmt.Sprintf("{{- define \"%s.annotations\" -}}\n", chartName))
	sb.WriteString("{{- if .Values.commonAnnotations }}\n")
	sb.WriteString("{{- toYaml .Values.commonAnnotations }}\n")
	sb.WriteString("{{- end }}\n")
	sb.WriteString("{{- end }}\n")

	return sb.String()
}

// GenerateGitignore generates a .helmignore file.
func GenerateHelmIgnore() string {
	return `# Patterns to ignore when building packages.
# This supports shell glob matching, relative path matching, and
# negation (prefixed with !). Only one pattern per line.
.DS_Store
# Common VCS dirs
.git/
.gitignore
.bzr/
.bzrignore
.hg/
.hgignore
.svn/
# Common backup files
*.swp
*.bak
*.tmp
*.orig
*~
# Various IDEs
.project
.idea/
*.tmproj
.vscode/
# Generated files
*.test
*.out
# CI/CD
.github/
.gitlab-ci.yml
.travis.yml
# Documentation
README.md.gotmpl
docs/
examples/
`
}

// GenerateValuesYAMLComment generates a comment header for values.yaml.
func GenerateValuesYAMLComment(chartName string) string {
	return fmt.Sprintf(`# Default values for %s
# This is a YAML-formatted file.
# Declare variables to be passed into your templates.

`, chartName)
}
