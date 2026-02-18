package generator

import (
	"context"
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/helm"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// LibraryGenerator generates a library chart with reusable named templates
// plus thin wrapper charts for each service.
type LibraryGenerator struct {
	BaseGenerator
}

// NewLibraryGenerator creates a new LibraryGenerator.
func NewLibraryGenerator() *LibraryGenerator {
	return &LibraryGenerator{
		BaseGenerator: NewBaseGenerator(types.OutputModeLibrary),
	}
}

// Generate creates a library chart and wrapper charts from the resource graph.
func (g *LibraryGenerator) Generate(ctx context.Context, graph *types.ResourceGraph, opts Options) ([]*types.GeneratedChart, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	charts := make([]*types.GeneratedChart, 0)

	// Always generate the library chart with all named templates.
	libChart := g.generateLibraryChart(opts)
	charts = append(charts, libChart)

	// Group resources and generate wrapper charts.
	groupResult, err := GroupResources(graph)
	if err != nil {
		return nil, fmt.Errorf("failed to group resources: %w", err)
	}

	for _, group := range groupResult.Groups {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		wrapper := g.generateWrapperChart(group, libChart.Name, opts)
		charts = append(charts, wrapper)
	}

	return charts, nil
}

// generateLibraryChart creates the base library chart with named templates for all K8s types.
func (g *LibraryGenerator) generateLibraryChart(opts Options) *types.GeneratedChart {
	chartName := "library"

	chartMeta := helm.ChartMetadata{
		Name:        chartName,
		Version:     opts.ChartVersion,
		AppVersion:  opts.AppVersion,
		Description: "Reusable library chart with named templates for Kubernetes resources",
		APIVersion:  "v2",
		Type:        "library",
		Keywords:    []string{"kubernetes", "library", "deckhouse"},
	}

	templates := make(map[string]string)

	// Generate named templates for all 18 supported K8s resource types.
	templates["templates/_deployment.tpl"] = generateNamedTemplate("deployment", deploymentTemplate)
	templates["templates/_statefulset.tpl"] = generateNamedTemplate("statefulset", statefulsetTemplate)
	templates["templates/_daemonset.tpl"] = generateNamedTemplate("daemonset", daemonsetTemplate)
	templates["templates/_service.tpl"] = generateNamedTemplate("service", serviceTemplate)
	templates["templates/_ingress.tpl"] = generateNamedTemplate("ingress", ingressTemplate)
	templates["templates/_configmap.tpl"] = generateNamedTemplate("configmap", configmapTemplate)
	templates["templates/_secret.tpl"] = generateNamedTemplate("secret", secretTemplate)
	templates["templates/_pvc.tpl"] = generateNamedTemplate("pvc", pvcTemplate)
	templates["templates/_hpa.tpl"] = generateNamedTemplate("hpa", hpaTemplate)
	templates["templates/_pdb.tpl"] = generateNamedTemplate("pdb", pdbTemplate)
	templates["templates/_networkpolicy.tpl"] = generateNamedTemplate("networkpolicy", networkpolicyTemplate)
	templates["templates/_cronjob.tpl"] = generateNamedTemplate("cronjob", cronjobTemplate)
	templates["templates/_job.tpl"] = generateNamedTemplate("job", jobTemplate)
	templates["templates/_serviceaccount.tpl"] = generateNamedTemplate("serviceaccount", serviceaccountTemplate)
	templates["templates/_role.tpl"] = generateNamedTemplate("role", roleTemplate)
	templates["templates/_clusterrole.tpl"] = generateNamedTemplate("clusterrole", clusterroleTemplate)
	templates["templates/_rolebinding.tpl"] = generateNamedTemplate("rolebinding", rolebindingTemplate)
	templates["templates/_clusterrolebinding.tpl"] = generateNamedTemplate("clusterrolebinding", clusterrolebindingTemplate)

	// Add DRY shared sub-templates (resources, probes, securityContext, env, volumeMounts, volumes, annotations).
	addSharedSubTemplates(templates)

	return &types.GeneratedChart{
		Name:       chartName,
		Path:       opts.OutputDir,
		ChartYAML:  helm.GenerateChartYAML(chartMeta),
		ValuesYAML: "# Library charts do not have values.yaml\n# Values are provided by wrapper charts\n",
		Templates:  templates,
		Helpers:    helm.GenerateHelpers(chartName),
	}
}

// generateWrapperChart creates a thin wrapper chart for a service group.
func (g *LibraryGenerator) generateWrapperChart(group *ServiceGroup, libraryName string, opts Options) *types.GeneratedChart {
	chartName := group.Name

	chartMeta := helm.ChartMetadata{
		Name:        chartName,
		Version:     opts.ChartVersion,
		AppVersion:  opts.AppVersion,
		Description: fmt.Sprintf("Wrapper chart for %s (uses library templates)", chartName),
		APIVersion:  "v2",
		Type:        "application",
		Dependencies: []helm.Dependency{
			{
				Name:       libraryName,
				Version:    opts.ChartVersion,
				Repository: fmt.Sprintf("file://../%s", libraryName),
			},
		},
	}

	// Build wrapper templates that call library includes.
	templates := make(map[string]string)
	kindsUsed := make(map[string]bool)
	for _, resource := range group.Resources {
		kind := strings.ToLower(resource.Original.GVK.Kind)
		if !kindsUsed[kind] {
			kindsUsed[kind] = true
			tmplName := fmt.Sprintf("templates/%s.yaml", kind)
			templates[tmplName] = generateWrapperTemplate(libraryName, kind)
		}
	}

	// Build flat values for this service.
	sep := &SeparateGenerator{}
	values := sep.buildFlatValues(group)
	valuesYAML, err := marshalFlatValues(chartName, values)
	if err != nil {
		valuesYAML = fmt.Sprintf("# Default values for %s\n# Error marshalling values: %v\n", chartName, err)
	}

	return &types.GeneratedChart{
		Name:       chartName,
		Path:       opts.OutputDir,
		ChartYAML:  helm.GenerateChartYAML(chartMeta),
		ValuesYAML: valuesYAML,
		Templates:  templates,
		Helpers:    helm.GenerateHelpers(chartName),
	}
}

// generateNamedTemplate wraps template content in a named define block.
func generateNamedTemplate(kind, body string) string {
	return fmt.Sprintf(`{{- define "library.%s" -}}
%s
{{- end -}}
`, kind, body)
}

// generateWrapperTemplate creates a template that calls a library include.
func generateWrapperTemplate(libraryName, kind string) string {
	return fmt.Sprintf(`{{- include "library.%s" (dict "context" . "values" .Values) -}}
`, kind)
}

// ============================================================
// Named template bodies for all 18 K8s resource types
// ============================================================

const deploymentTemplate = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
spec:
  replicas: {{ .values.replicaCount | default 1 }}
  selector:
    matchLabels:
      {{- include "library.selectorLabels" .context | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "library.selectorLabels" .context | nindent 8 }}
    spec:
      containers:
        - name: {{ .values.name | default .context.Chart.Name }}
          {{- $dvals := .values | toJson | fromJson }}
          image: "{{ dig "image" "repository" "nginx" $dvals }}:{{ dig "image" "tag" "latest" $dvals }}"
          ports:
            {{- range .values.ports | default (list (dict "containerPort" 80)) }}
            - containerPort: {{ .containerPort }}
            {{- end }}
          {{- include "library.env" . | nindent 10 }}
          {{- include "library.resources" . | nindent 10 }}
          {{- include "library.probes" . | nindent 10 }}
          {{- include "library.volumeMounts" . | nindent 10 }}
      {{- include "library.volumes" . | nindent 6 }}`

const statefulsetTemplate = `apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
spec:
  replicas: {{ .values.replicaCount | default 1 }}
  serviceName: {{ .values.serviceName | default (include "library.fullname" .context) }}
  selector:
    matchLabels:
      {{- include "library.selectorLabels" .context | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "library.selectorLabels" .context | nindent 8 }}
    spec:
      containers:
        - name: {{ .values.name | default .context.Chart.Name }}
          {{- $dvals := .values | toJson | fromJson }}
          image: "{{ dig "image" "repository" "nginx" $dvals }}:{{ dig "image" "tag" "latest" $dvals }}"
          {{- include "library.env" . | nindent 10 }}
          {{- include "library.resources" . | nindent 10 }}
          {{- include "library.probes" . | nindent 10 }}
          {{- include "library.volumeMounts" . | nindent 10 }}
      {{- include "library.volumes" . | nindent 6 }}`

const daemonsetTemplate = `apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
spec:
  selector:
    matchLabels:
      {{- include "library.selectorLabels" .context | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "library.selectorLabels" .context | nindent 8 }}
    spec:
      containers:
        - name: {{ .values.name | default .context.Chart.Name }}
          {{- $dvals := .values | toJson | fromJson }}
          image: "{{ dig "image" "repository" "nginx" $dvals }}:{{ dig "image" "tag" "latest" $dvals }}"
          {{- include "library.env" . | nindent 10 }}
          {{- include "library.resources" . | nindent 10 }}
          {{- include "library.volumeMounts" . | nindent 10 }}
      {{- include "library.volumes" . | nindent 6 }}`

const serviceTemplate = `apiVersion: v1
kind: Service
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
spec:
  type: {{ .values.type | default "ClusterIP" }}
  selector:
    {{- include "library.selectorLabels" .context | nindent 4 }}
  ports:
    {{- range .values.ports | default (list (dict "port" 80 "targetPort" 80 "protocol" "TCP" "name" "http")) }}
    - port: {{ .port }}
      targetPort: {{ .targetPort }}
      protocol: {{ .protocol | default "TCP" }}
      name: {{ .name | default "http" }}
    {{- end }}`

const ingressTemplate = `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
  {{- if .values.annotations }}
  annotations:
    {{- toYaml .values.annotations | nindent 4 }}
  {{- end }}
spec:
  {{- if .values.ingressClassName }}
  ingressClassName: {{ .values.ingressClassName }}
  {{- end }}
  {{- if .values.tls }}
  tls:
    {{- toYaml .values.tls | nindent 4 }}
  {{- end }}
  rules:
    {{- range .values.rules | default (list) }}
    - host: {{ .host }}
      http:
        paths:
          {{- range .paths | default (list) }}
          - path: {{ .path | default "/" }}
            pathType: {{ .pathType | default "Prefix" }}
            backend:
              service:
                name: {{ .serviceName | default (include "library.fullname" $.context) }}
                port:
                  number: {{ .servicePort | default 80 }}
          {{- end }}
    {{- end }}`

const configmapTemplate = `apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
{{- if .values.data }}
data:
  {{- range $key, $value := .values.data }}
  {{ $key }}: {{ $value | quote }}
  {{- end }}
{{- end }}`

const secretTemplate = `apiVersion: v1
kind: Secret
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
type: {{ .values.type | default "Opaque" }}
{{- if .values.data }}
data:
  {{- range $key, $value := .values.data }}
  {{ $key }}: {{ $value | b64enc | quote }}
  {{- end }}
{{- end }}`

const pvcTemplate = `apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
spec:
  accessModes:
    {{- range .values.accessModes | default (list "ReadWriteOnce") }}
    - {{ . }}
    {{- end }}
  resources:
    requests:
      storage: {{ .values.size | default "1Gi" }}
  {{- if .values.storageClassName }}
  storageClassName: {{ .values.storageClassName }}
  {{- end }}`

const hpaTemplate = `apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: {{ .values.targetKind | default "Deployment" }}
    name: {{ .values.targetName | default (include "library.fullname" .context) }}
  minReplicas: {{ .values.minReplicas | default 1 }}
  maxReplicas: {{ .values.maxReplicas | default 10 }}
  metrics:
    {{- if .values.metrics }}
    {{- toYaml .values.metrics | nindent 4 }}
    {{- end }}`

const pdbTemplate = `apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
spec:
  {{- if .values.minAvailable }}
  minAvailable: {{ .values.minAvailable }}
  {{- end }}
  {{- if .values.maxUnavailable }}
  maxUnavailable: {{ .values.maxUnavailable }}
  {{- end }}
  selector:
    matchLabels:
      {{- include "library.selectorLabels" .context | nindent 6 }}`

const networkpolicyTemplate = `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
spec:
  podSelector:
    matchLabels:
      {{- include "library.selectorLabels" .context | nindent 6 }}
  policyTypes:
    {{- range .values.policyTypes | default (list "Ingress") }}
    - {{ . }}
    {{- end }}
  {{- if .values.ingress }}
  ingress:
    {{- toYaml .values.ingress | nindent 4 }}
  {{- end }}
  {{- if .values.egress }}
  egress:
    {{- toYaml .values.egress | nindent 4 }}
  {{- end }}`

const cronjobTemplate = `apiVersion: batch/v1
kind: CronJob
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
spec:
  schedule: {{ .values.schedule | default "0 * * * *" | quote }}
  {{- if .values.concurrencyPolicy }}
  concurrencyPolicy: {{ .values.concurrencyPolicy }}
  {{- end }}
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: {{ .values.name | default .context.Chart.Name }}
              {{- $dvals := .values | toJson | fromJson }}
              image: "{{ dig "image" "repository" "busybox" $dvals }}:{{ dig "image" "tag" "latest" $dvals }}"
              {{- if .values.command }}
              command:
                {{- toYaml .values.command | nindent 16 }}
              {{- end }}
              {{- include "library.env" . | nindent 14 }}
              {{- include "library.resources" . | nindent 14 }}
              {{- include "library.volumeMounts" . | nindent 14 }}
          restartPolicy: {{ .values.restartPolicy | default "OnFailure" }}
          {{- include "library.volumes" . | nindent 10 }}`

const jobTemplate = `apiVersion: batch/v1
kind: Job
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
spec:
  {{- if .values.backoffLimit }}
  backoffLimit: {{ .values.backoffLimit }}
  {{- end }}
  template:
    spec:
      containers:
        - name: {{ .values.name | default .context.Chart.Name }}
          {{- $dvals := .values | toJson | fromJson }}
          image: "{{ dig "image" "repository" "busybox" $dvals }}:{{ dig "image" "tag" "latest" $dvals }}"
          {{- if .values.command }}
          command:
            {{- toYaml .values.command | nindent 12 }}
          {{- end }}
          {{- include "library.env" . | nindent 10 }}
          {{- include "library.resources" . | nindent 10 }}
          {{- include "library.volumeMounts" . | nindent 10 }}
      restartPolicy: {{ .values.restartPolicy | default "Never" }}
      {{- include "library.volumes" . | nindent 6 }}`

const serviceaccountTemplate = `apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
  {{- if .values.annotations }}
  annotations:
    {{- toYaml .values.annotations | nindent 4 }}
  {{- end }}`

const roleTemplate = `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
{{- if .values.rules }}
rules:
  {{- toYaml .values.rules | nindent 2 }}
{{- end }}`

const clusterroleTemplate = `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
{{- if .values.rules }}
rules:
  {{- toYaml .values.rules | nindent 2 }}
{{- end }}`

const rolebindingTemplate = `apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: {{ .values.roleRef.kind | default "Role" }}
  name: {{ .values.roleRef.name | default (include "library.fullname" .context) }}
subjects:
  {{- if .values.subjects }}
  {{- toYaml .values.subjects | nindent 2 }}
  {{- end }}`

const clusterrolebindingTemplate = `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: {{ .values.name | default (include "library.fullname" .context) }}
  labels:
    {{- include "library.labels" .context | nindent 4 }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: {{ .values.roleRef.kind | default "ClusterRole" }}
  name: {{ .values.roleRef.name | default (include "library.fullname" .context) }}
subjects:
  {{- if .values.subjects }}
  {{- toYaml .values.subjects | nindent 2 }}
  {{- end }}`
