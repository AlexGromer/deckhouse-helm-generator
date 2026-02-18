package k8s

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ServiceAccountProcessor processes Kubernetes ServiceAccounts.
type ServiceAccountProcessor struct {
	processor.BaseProcessor
}

// NewServiceAccountProcessor creates a new ServiceAccount processor.
func NewServiceAccountProcessor() *ServiceAccountProcessor {
	return &ServiceAccountProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"serviceaccount",
			100,
			schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ServiceAccount"},
		),
	}
}

// Process processes a ServiceAccount resource.
func (p *ServiceAccountProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, fmt.Errorf("serviceaccount object is nil")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	values := map[string]interface{}{
		"enabled":     true,
		"name":        name,
		"annotations": obj.GetAnnotations(),
	}

	// Image pull secrets
	if secrets, found, _ := unstructured.NestedSlice(obj.Object, "imagePullSecrets"); found {
		values["imagePullSecrets"] = secrets
	}

	// AutomountServiceAccountToken
	if automount, found, _ := unstructured.NestedBool(obj.Object, "automountServiceAccountToken"); found {
		values["automountServiceAccountToken"] = automount
	}

	template := fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.serviceAccount }}
{{- if .enabled }}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ .name }}
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
  {{- with .annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- with .automountServiceAccountToken }}
automountServiceAccountToken: {{ . }}
{{- end }}
{{- with .imagePullSecrets }}
imagePullSecrets:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
`, serviceName, ctx.ChartName)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-serviceaccount.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.serviceAccount", serviceName),
		Values:          values,
	}, nil
}

// StatefulSetProcessor processes Kubernetes StatefulSets.
type StatefulSetProcessor struct {
	processor.BaseProcessor
}

// NewStatefulSetProcessor creates a new StatefulSet processor.
func NewStatefulSetProcessor() *StatefulSetProcessor {
	return &StatefulSetProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"statefulset",
			100,
			schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"},
		),
	}
}

// Process processes a StatefulSet resource.
func (p *StatefulSetProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, fmt.Errorf("statefulset object is nil")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	values, deps := extractWorkloadValues(obj)

	// StatefulSet-specific values
	if svcName, found, _ := unstructured.NestedString(obj.Object, "spec", "serviceName"); found {
		values["serviceName"] = svcName
		deps = append(deps, types.ResourceKey{
			GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Service"},
			Namespace: obj.GetNamespace(),
			Name:      svcName,
		})
	}

	// Volume claim templates
	if vcts, found, _ := unstructured.NestedSlice(obj.Object, "spec", "volumeClaimTemplates"); found {
		values["volumeClaimTemplates"] = vcts
	}

	// Pod management policy
	if policy, found, _ := unstructured.NestedString(obj.Object, "spec", "podManagementPolicy"); found {
		values["podManagementPolicy"] = policy
	}

	// Update strategy
	if strategy, found, _ := unstructured.NestedMap(obj.Object, "spec", "updateStrategy"); found {
		values["updateStrategy"] = strategy
	}

	template := p.generateTemplate(ctx, serviceName)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-statefulset.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.statefulSet", serviceName),
		Values:          values,
		Dependencies:    deps,
	}, nil
}

func (p *StatefulSetProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	fullnameHelper := fmt.Sprintf("{{ include \"%s.fullname\" $ }}", ctx.ChartName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.statefulSet }}
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: %s-%s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s
spec:
  serviceName: {{ .serviceName }}
  replicas: {{ .replicas | default 1 }}
  {{- with .podManagementPolicy }}
  podManagementPolicy: {{ . }}
  {{- end }}
  selector:
    matchLabels:
      {{- include "%s.selectorLabels" $ | nindent 6 }}
      app.kubernetes.io/component: %s
  {{- with .updateStrategy }}
  updateStrategy:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  template:
    metadata:
      {{- with .podAnnotations }}
      annotations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      labels:
        {{- include "%s.labels" $ | nindent 8 }}
        app.kubernetes.io/component: %s
    spec:
      {{- with $.Values.global.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .serviceAccountName }}
      serviceAccountName: {{ . }}
      {{- end }}
      containers:
        {{- range .containers }}
        - name: {{ .name }}
          image: "{{ .image.repository }}:{{ .image.tag }}"
          imagePullPolicy: {{ .image.pullPolicy | default "IfNotPresent" }}
          {{- with .ports }}
          ports:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .env }}
          env:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .volumeMounts }}
          volumeMounts:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .resources }}
          resources:
            {{- toYaml . | nindent 12 }}
          {{- end }}
        {{- end }}
      {{- with .volumes }}
      volumes:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
  {{- with .volumeClaimTemplates }}
  volumeClaimTemplates:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
{{- end }}
`, serviceName, fullnameHelper, serviceName,
		ctx.ChartName, serviceName,
		ctx.ChartName, serviceName,
		ctx.ChartName, serviceName)
}

// DaemonSetProcessor processes Kubernetes DaemonSets.
type DaemonSetProcessor struct {
	processor.BaseProcessor
}

// NewDaemonSetProcessor creates a new DaemonSet processor.
func NewDaemonSetProcessor() *DaemonSetProcessor {
	return &DaemonSetProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"daemonset",
			100,
			schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"},
		),
	}
}

// Process processes a DaemonSet resource.
func (p *DaemonSetProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, fmt.Errorf("daemonset object is nil")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	values, deps := extractWorkloadValues(obj)

	// Update strategy
	if strategy, found, _ := unstructured.NestedMap(obj.Object, "spec", "updateStrategy"); found {
		values["updateStrategy"] = strategy
	}

	template := p.generateTemplate(ctx, serviceName)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-daemonset.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.daemonSet", serviceName),
		Values:          values,
		Dependencies:    deps,
	}, nil
}

func (p *DaemonSetProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	fullnameHelper := fmt.Sprintf("{{ include \"%s.fullname\" $ }}", ctx.ChartName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.daemonSet }}
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: %s-%s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s
spec:
  selector:
    matchLabels:
      {{- include "%s.selectorLabels" $ | nindent 6 }}
      app.kubernetes.io/component: %s
  {{- with .updateStrategy }}
  updateStrategy:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  template:
    metadata:
      {{- with .podAnnotations }}
      annotations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      labels:
        {{- include "%s.labels" $ | nindent 8 }}
        app.kubernetes.io/component: %s
    spec:
      {{- with $.Values.global.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .serviceAccountName }}
      serviceAccountName: {{ . }}
      {{- end }}
      containers:
        {{- range .containers }}
        - name: {{ .name }}
          image: "{{ .image.repository }}:{{ .image.tag }}"
          imagePullPolicy: {{ .image.pullPolicy | default "IfNotPresent" }}
          {{- with .ports }}
          ports:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .env }}
          env:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .volumeMounts }}
          volumeMounts:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .resources }}
          resources:
            {{- toYaml . | nindent 12 }}
          {{- end }}
        {{- end }}
      {{- with .volumes }}
      volumes:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
{{- end }}
{{- end }}
`, serviceName, fullnameHelper, serviceName,
		ctx.ChartName, serviceName,
		ctx.ChartName, serviceName,
		ctx.ChartName, serviceName)
}

// PVCProcessor processes Kubernetes PersistentVolumeClaims.
type PVCProcessor struct {
	processor.BaseProcessor
}

// NewPVCProcessor creates a new PVC processor.
func NewPVCProcessor() *PVCProcessor {
	return &PVCProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"pvc",
			100,
			schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"},
		),
	}
}

// Process processes a PVC resource.
func (p *PVCProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, fmt.Errorf("pvc object is nil")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	values := map[string]interface{}{
		"enabled": true,
		"name":    name,
	}

	// Access modes
	if modes, found, _ := unstructured.NestedStringSlice(obj.Object, "spec", "accessModes"); found {
		values["accessModes"] = modes
	}

	// Storage class
	if sc, found, _ := unstructured.NestedString(obj.Object, "spec", "storageClassName"); found {
		values["storageClassName"] = sc
	}

	// Resources
	if resources, found, _ := unstructured.NestedMap(obj.Object, "spec", "resources"); found {
		values["resources"] = resources
	}

	// Volume mode
	if mode, found, _ := unstructured.NestedString(obj.Object, "spec", "volumeMode"); found {
		values["volumeMode"] = mode
	}

	// Data source (e.g. VolumeSnapshot clone)
	if ds, found, _ := unstructured.NestedMap(obj.Object, "spec", "dataSource"); found {
		values["dataSource"] = ds
	}

	// Selector (matchLabels / matchExpressions)
	if selector, found, _ := unstructured.NestedMap(obj.Object, "spec", "selector"); found {
		values["selector"] = selector
	}

	template := fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.pvc }}
{{- if .enabled }}
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{ include "%s.fullname" $ }}-%s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  accessModes:
    {{- toYaml .accessModes | nindent 4 }}
  {{- with .storageClassName }}
  storageClassName: {{ . }}
  {{- end }}
  {{- with .volumeMode }}
  volumeMode: {{ . }}
  {{- end }}
  resources:
    {{- toYaml .resources | nindent 4 }}
{{- end }}
{{- end }}
{{- end }}
`, serviceName, ctx.ChartName, name, ctx.ChartName)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-pvc-%s.yaml", serviceName, name),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.pvc", serviceName),
		Values:          values,
	}, nil
}

// Helper function to extract common workload values (used by Deployment, StatefulSet, DaemonSet).
func extractWorkloadValues(obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey

	// Replicas (not for DaemonSet)
	if replicas, found, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas"); found {
		values["replicas"] = replicas
	}

	// Containers
	if containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers"); len(containers) > 0 {
		containerValues := make([]map[string]interface{}, 0, len(containers))
		for _, c := range containers {
			container, ok := c.(map[string]interface{})
			if !ok {
				continue
			}

			cv := make(map[string]interface{})
			if name, ok := container["name"].(string); ok {
				cv["name"] = name
			}
			if image, ok := container["image"].(string); ok {
				repo, tag := parseImage(image)
				cv["image"] = map[string]interface{}{
					"repository": repo,
					"tag":        tag,
				}
			}
			if resources, ok := container["resources"].(map[string]interface{}); ok {
				cv["resources"] = resources
			}
			if ports, ok := container["ports"].([]interface{}); ok {
				cv["ports"] = ports
			}
			if env, ok := container["env"].([]interface{}); ok {
				cv["env"] = env
				deps = append(deps, extractEnvDependencies(env, obj.GetNamespace())...)
			}
			if volumeMounts, ok := container["volumeMounts"].([]interface{}); ok {
				cv["volumeMounts"] = volumeMounts
			}

			containerValues = append(containerValues, cv)
		}
		values["containers"] = containerValues
	}

	// Volumes
	if volumes, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "volumes"); len(volumes) > 0 {
		values["volumes"] = volumes
		deps = append(deps, extractVolumeDependencies(volumes, obj.GetNamespace())...)
	}

	// ServiceAccount
	if sa, found, _ := unstructured.NestedString(obj.Object, "spec", "template", "spec", "serviceAccountName"); found && sa != "" {
		values["serviceAccountName"] = sa
		deps = append(deps, types.ResourceKey{
			GVK:       schema.GroupVersionKind{Version: "v1", Kind: "ServiceAccount"},
			Namespace: obj.GetNamespace(),
			Name:      sa,
		})
	}

	// Node selector
	if nodeSelector, found, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "spec", "nodeSelector"); found {
		values["nodeSelector"] = nodeSelector
	}

	// Tolerations
	if tolerations, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "tolerations"); len(tolerations) > 0 {
		values["tolerations"] = tolerations
	}

	// Pod annotations
	if annotations, found, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "annotations"); found {
		values["podAnnotations"] = annotations
	}

	return values, deps
}
