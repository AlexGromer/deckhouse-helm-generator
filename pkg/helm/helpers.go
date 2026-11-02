package helm

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// GenerateHelpers generates the _helpers.tpl content for a chart that has no
// objects of its own to prefix (see GenerateHelpersWithRefs).
func GenerateHelpers(chartName string) string {
	return GenerateHelpersWithRefs(chartName, nil)
}

// GenerateHelpersWithRefs generates the _helpers.tpl content. inChart lists
// the names of the objects the whole chart renders, by Kind: global.namePrefix
// is put in front of these names, wherever they are referenced.
func GenerateHelpersWithRefs(chartName string, inChart map[string][]string) string {
	return generateHelpers(chartName, inChart, false)
}

// GenerateLibraryHelpers generates the _helpers.tpl content of a library
// chart. The library is published once and shared by service charts, so
// nothing in it depends on the input: it has no list of object names. Its
// ref helper reads the list from the calling chart, from the template
// "<chart name>.inChart" that GenerateInChartTemplate writes into the chart.
func GenerateLibraryHelpers(chartName string) string {
	return generateHelpers(chartName, nil, true)
}

// GenerateInChartTemplate returns the content of a template file of a service
// chart that uses the helpers of a library chart: it defines
// "<chartName>.inChart", the names of the objects the chart renders, by Kind.
func GenerateInChartTemplate(chartName string, inChart map[string][]string) string {
	var sb strings.Builder
	sb.WriteString("{{/*\nObjects the chart renders, by Kind (generated). The library helpers read it\nto put global.namePrefix in front of references to these objects.\n*/}}\n")
	writeInChartDefine(&sb, chartName, inChart)
	return sb.String()
}

func generateHelpers(chartName string, inChart map[string][]string, library bool) string {
	var sb strings.Builder

	sb.WriteString("{{/*\n")
	sb.WriteString("Expand the name of the chart.\n")
	sb.WriteString("*/}}\n")
	fmt.Fprintf(&sb, "{{- define \"%s.name\" -}}\n", chartName)
	sb.WriteString("{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix \"-\" }}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\n")
	sb.WriteString("Create a default fully qualified app name.\n")
	sb.WriteString("We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).\n")
	sb.WriteString("If release name contains chart name it will be used as a full name.\n")
	sb.WriteString("*/}}\n")
	fmt.Fprintf(&sb, "{{- define \"%s.fullname\" -}}\n", chartName)
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
	fmt.Fprintf(&sb, "{{- define \"%s.chart\" -}}\n", chartName)
	sb.WriteString("{{- printf \"%s-%s\" .Chart.Name .Chart.Version | replace \"+\" \"_\" | trunc 63 | trimSuffix \"-\" }}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\n")
	sb.WriteString("Common labels\n")
	sb.WriteString("*/}}\n")
	fmt.Fprintf(&sb, "{{- define \"%s.labels\" -}}\n", chartName)
	fmt.Fprintf(&sb, "helm.sh/chart: {{ include \"%s.chart\" . }}\n", chartName)
	fmt.Fprintf(&sb, "{{ include \"%s.selectorLabels\" . }}\n", chartName)
	sb.WriteString("{{- if .Chart.AppVersion }}\n")
	sb.WriteString("app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}\n")
	sb.WriteString("{{- end }}\n")
	sb.WriteString("app.kubernetes.io/managed-by: {{ .Release.Service }}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\n")
	sb.WriteString("Selector labels\n")
	sb.WriteString("*/}}\n")
	fmt.Fprintf(&sb, "{{- define \"%s.selectorLabels\" -}}\n", chartName)
	fmt.Fprintf(&sb, "app.kubernetes.io/name: {{ include \"%s.name\" . }}\n", chartName)
	sb.WriteString("app.kubernetes.io/instance: {{ .Release.Name }}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\n")
	sb.WriteString("Create the name of the service account to use\n")
	sb.WriteString("*/}}\n")
	fmt.Fprintf(&sb, "{{- define \"%s.serviceAccountName\" -}}\n", chartName)
	sb.WriteString("{{- if .Values.serviceAccount.create }}\n")
	fmt.Fprintf(&sb, "{{- default (include \"%s.fullname\" .) .Values.serviceAccount.name }}\n", chartName)
	sb.WriteString("{{- else }}\n")
	sb.WriteString("{{- default \"default\" .Values.serviceAccount.name }}\n")
	sb.WriteString("{{- end }}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\n")
	sb.WriteString("Image pull secrets\n")
	sb.WriteString("*/}}\n")
	fmt.Fprintf(&sb, "{{- define \"%s.imagePullSecrets\" -}}\n", chartName)
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
	fmt.Fprintf(&sb, "{{- define \"%s.image\" -}}\n", chartName)
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
	fmt.Fprintf(&sb, "{{- define \"%s.isDeckhouseAvailable\" -}}\n", chartName)
	sb.WriteString("{{- if .Values.deckhouse }}\n")
	sb.WriteString("{{- if .Values.deckhouse.enabled }}\n")
	sb.WriteString("true\n")
	sb.WriteString("{{- end }}\n")
	sb.WriteString("{{- end }}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\n")
	sb.WriteString("Generate standard annotations\n")
	sb.WriteString("*/}}\n")
	fmt.Fprintf(&sb, "{{- define \"%s.annotations\" -}}\n", chartName)
	sb.WriteString("{{- if .Values.commonAnnotations }}\n")
	sb.WriteString("{{- toYaml .Values.commonAnnotations }}\n")
	sb.WriteString("{{- end }}\n")
	sb.WriteString("{{- end }}\n")

	writeNamePrefixHelpers(&sb, chartName, inChart, library)

	return sb.String()
}

// writeNamePrefixHelpers writes the helpers behind global.namePrefix:
//
//	<chart>.objname   (list $ "name")        the name of an object of the chart
//	<chart>.ref       (list $ "Kind" "name") a reference to a name, prefixed when the chart renders it
//	<chart>.refs      (list $ node)          node as YAML, the references in it passed through <chart>.ref
//
// With an empty prefix every helper returns its input unchanged.
//
// library is true for the helpers of a library chart: they carry no list of
// object names (see GenerateLibraryHelpers).
func writeNamePrefixHelpers(sb *strings.Builder, c string, inChart map[string][]string, library bool) {
	sb.WriteString("\n{{/*\n")
	sb.WriteString("Name prefix: global.namePrefix is put in front of the name of every object\n")
	sb.WriteString("of the chart, so that several releases fit into one namespace. References\n")
	sb.WriteString("to objects of the chart get the same prefix; references to other objects\n")
	sb.WriteString("stay as they are.\n")
	sb.WriteString("*/}}\n")

	if !library {
		sb.WriteString("{{/*\nObjects the chart renders, by Kind (generated).\n*/}}\n")
		writeInChartDefine(sb, c, inChart)
		sb.WriteString("\n")
	}

	sb.WriteString("{{/*\nThe prefix, \"\" when unset. Argument: the root context.\n*/}}\n")
	fmt.Fprintf(sb, "{{- define \"%s.namePrefix\" -}}\n", c)
	sb.WriteString("{{- (default (dict) .Values.global).namePrefix | default \"\" -}}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\nName of an object of the chart. Argument: (list $ \"name\").\n*/}}\n")
	fmt.Fprintf(sb, "{{- define \"%s.objname\" -}}\n", c)
	fmt.Fprintf(sb, "{{- printf \"%%s%%s\" (include \"%s.namePrefix\" (index . 0)) (index . 1) -}}\n", c)
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\nReference to an object by Kind and name. Argument: (list $ \"Kind\" \"name\").\n*/}}\n")
	fmt.Fprintf(sb, "{{- define \"%s.ref\" -}}\n", c)
	sb.WriteString("{{- $root := index . 0 -}}\n")
	sb.WriteString("{{- $kind := index . 1 -}}\n")
	sb.WriteString("{{- $name := index . 2 -}}\n")
	fmt.Fprintf(sb, "{{- $prefix := include \"%s.namePrefix\" $root -}}\n", c)
	if library {
		// The chart that includes the library defines the list.
		sb.WriteString("{{- if and $prefix (has $name (get (fromYaml (include (printf \"%s.inChart\" $root.Chart.Name) $root)) $kind | default list)) -}}\n")
	} else {
		fmt.Fprintf(sb, "{{- if and $prefix (has $name (get (fromYaml (include \"%s.inChart\" $root)) $kind | default list)) -}}\n", c)
	}
	sb.WriteString("{{- printf \"%s%s\" $prefix $name -}}\n")
	sb.WriteString("{{- else -}}\n")
	sb.WriteString("{{- $name -}}\n")
	sb.WriteString("{{- end -}}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\nSets field of the map m to the reference it holds. Argument: (list $ m field defaultKind).\n*/}}\n")
	fmt.Fprintf(sb, "{{- define \"%s.refone\" -}}\n", c)
	sb.WriteString("{{- $m := index . 1 -}}\n")
	sb.WriteString("{{- $field := index . 2 -}}\n")
	sb.WriteString("{{- if kindIs \"string\" (get $m $field) -}}\n")
	fmt.Fprintf(sb, "{{- $_ := set $m $field (include \"%s.ref\" (list (index . 0) ($m.kind | default (index . 3)) (get $m $field))) -}}\n", c)
	sb.WriteString("{{- end -}}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\nWalks a copy of pod or spec values and prefixes the references to objects of the\nchart. Argument: (list $ node).\n*/}}\n")
	fmt.Fprintf(sb, "{{- define \"%s.refwalk\" -}}\n", c)
	sb.WriteString("{{- $root := index . 0 -}}\n")
	sb.WriteString("{{- $n := index . 1 -}}\n")
	sb.WriteString("{{- if kindIs \"map\" $n -}}\n")
	sb.WriteString("{{- range $k, $v := $n -}}\n")
	sb.WriteString("{{- if kindIs \"map\" $v -}}\n")
	one := fmt.Sprintf("include \"%s.refone\"", c)
	sb.WriteString("{{- if has $k (list \"configMapRef\" \"configMapKeyRef\" \"configMap\") -}}{{- $_ := " + one + " (list $root $v \"name\" \"ConfigMap\") -}}\n")
	sb.WriteString("{{- else if has $k (list \"secretRef\" \"secretKeyRef\") -}}{{- $_ := " + one + " (list $root $v \"name\" \"Secret\") -}}\n")
	sb.WriteString("{{- else if eq $k \"secret\" -}}{{- $_ := " + one + " (list $root $v (ternary \"secretName\" \"name\" (hasKey $v \"secretName\")) \"Secret\") -}}\n")
	sb.WriteString("{{- else if eq $k \"persistentVolumeClaim\" -}}{{- $_ := " + one + " (list $root $v \"claimName\" \"PersistentVolumeClaim\") -}}\n")
	sb.WriteString("{{- else if has $k (list \"scaleTargetRef\" \"targetRef\") -}}{{- $_ := " + one + " (list $root $v \"name\" \"Deployment\") -}}\n")
	sb.WriteString("{{- else if eq $k \"roleRef\" -}}{{- $_ := " + one + " (list $root $v \"name\" \"Role\") -}}\n")
	sb.WriteString("{{- else if eq $k \"issuerRef\" -}}{{- $_ := " + one + " (list $root $v \"name\" \"Issuer\") -}}\n")
	sb.WriteString("{{- else if and (eq $k \"clientConfig\") (kindIs \"map\" (get $v \"service\")) -}}{{- $_ := " + one + " (list $root (get $v \"service\") \"name\" \"Service\") -}}\n")
	sb.WriteString("{{- end -}}\n")
	fmt.Fprintf(sb, "{{- $_ := include \"%s.refwalk\" (list $root $v) -}}\n", c)
	sb.WriteString("{{- else if kindIs \"slice\" $v -}}\n")
	sb.WriteString("{{- range $v -}}\n")
	sb.WriteString("{{- if kindIs \"map\" . -}}\n")
	sb.WriteString("{{- if eq $k \"parentRefs\" -}}{{- $_ := " + one + " (list $root . \"name\" \"Gateway\") -}}\n")
	sb.WriteString("{{- else if eq $k \"backendRefs\" -}}{{- $_ := " + one + " (list $root . \"name\" \"Service\") -}}\n")
	sb.WriteString("{{- else if eq $k \"certificateRefs\" -}}{{- $_ := " + one + " (list $root . \"name\" \"Secret\") -}}\n")
	sb.WriteString("{{- else if and (eq $k \"subjects\") (eq (.kind | default \"\") \"ServiceAccount\") -}}{{- $_ := " + one + " (list $root . \"name\" \"ServiceAccount\") -}}\n")
	sb.WriteString("{{- end -}}\n")
	sb.WriteString("{{- end -}}\n")
	fmt.Fprintf(sb, "{{- $_ := include \"%s.refwalk\" (list $root .) -}}\n", c)
	sb.WriteString("{{- end -}}\n")
	sb.WriteString("{{- else if and (kindIs \"string\" $v) (has $k (list \"storageClassName\" \"ingressClassName\")) -}}\n")
	sb.WriteString("{{- $_ := set $n $k (include \"" + c + ".ref\" (list $root (ternary \"StorageClass\" \"IngressClass\" (eq $k \"storageClassName\")) $v)) -}}\n")
	sb.WriteString("{{- end -}}\n")
	sb.WriteString("{{- end -}}\n")
	sb.WriteString("{{- else if kindIs \"slice\" $n -}}\n")
	sb.WriteString("{{- range $n -}}\n")
	fmt.Fprintf(sb, "{{- $_ := include \"%s.refwalk\" (list $root .) -}}\n", c)
	sb.WriteString("{{- end -}}\n")
	sb.WriteString("{{- end -}}\n")
	sb.WriteString("{{- end }}\n\n")

	sb.WriteString("{{/*\nYAML of a node (values of a pod, a spec), the references to objects of the chart\nprefixed. Argument: (list $ node) or (list $ node \"key\") when node is the value of\nthe field key (roleRef, subjects, ...) of the object.\n*/}}\n")
	fmt.Fprintf(sb, "{{- define \"%s.refs\" -}}\n", c)
	sb.WriteString("{{- $root := index . 0 -}}\n")
	sb.WriteString("{{- $node := index . 1 -}}\n")
	fmt.Fprintf(sb, "{{- if include \"%s.namePrefix\" $root -}}\n", c)
	sb.WriteString("{{- if gt (len .) 2 -}}\n")
	sb.WriteString("{{- $key := index . 2 -}}\n")
	sb.WriteString("{{- $holder := dict $key (deepCopy $node) -}}\n")
	fmt.Fprintf(sb, "{{- $_ := include \"%s.refwalk\" (list $root $holder) -}}\n", c)
	sb.WriteString("{{- $node = get $holder $key -}}\n")
	sb.WriteString("{{- else -}}\n")
	sb.WriteString("{{- $node = deepCopy $node -}}\n")
	fmt.Fprintf(sb, "{{- $_ := include \"%s.refwalk\" (list $root $node) -}}\n", c)
	sb.WriteString("{{- end -}}\n")
	sb.WriteString("{{- end -}}\n")
	sb.WriteString("{{- toYaml $node -}}\n")
	sb.WriteString("{{- end }}\n")
}

// writeInChartDefine writes the define "<c>.inChart": the names of the objects
// of the chart as a YAML map, Kind to a list of names.
func writeInChartDefine(sb *strings.Builder, c string, inChart map[string][]string) {
	kinds := make([]string, 0, len(inChart))
	for k := range inChart {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)

	fmt.Fprintf(sb, "{{- define \"%s.inChart\" -}}\n", c)
	if len(kinds) == 0 {
		sb.WriteString("{}\n")
	}
	for _, k := range kinds {
		names := append([]string(nil), inChart[k]...)
		sort.Strings(names)
		b, _ := json.Marshal(names)
		fmt.Fprintf(sb, "%s: %s\n", k, b)
	}
	sb.WriteString("{{- end }}\n")
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
