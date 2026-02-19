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

// DeploymentProcessor processes Kubernetes Deployments.
type DeploymentProcessor struct {
	processor.BaseProcessor
}

// NewDeploymentProcessor creates a new Deployment processor.
func NewDeploymentProcessor() *DeploymentProcessor {
	return &DeploymentProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"deployment",
			100,
			schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		),
	}
}

// Process processes a Deployment resource.
func (p *DeploymentProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("deployment object is nil")
	}

	serviceName := processor.SanitizeServiceName(processor.ServiceNameFromResource(obj))
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	// Extract values from the deployment
	values, deps := p.extractValues(obj)

	// Generate template
	template := p.generateTemplate(ctx, obj, serviceName)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-deployment.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.deployment", serviceName),
		Values:          values,
		Dependencies:    deps,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *DeploymentProcessor) extractValues(obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey

	spec, _, _ := unstructured.NestedMap(obj.Object, "spec")
	if spec == nil {
		return values, deps
	}

	// Replicas (default to 1 when not specified)
	if replicas, found, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas"); found {
		values["replicas"] = replicas
	} else {
		values["replicas"] = int64(1)
	}

	// Pod template spec
	podSpec, _, _ := unstructured.NestedMap(obj.Object, "spec", "template", "spec")
	if podSpec == nil {
		return values, deps
	}

	// Pod-level securityContext
	if podSC, found, _ := unstructured.NestedMap(obj.Object, "spec", "template", "spec", "securityContext"); found {
		values["podSecurityContext"] = podSC
	}

	// Containers
	containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if len(containers) > 0 {
		containerValues := make([]map[string]interface{}, 0, len(containers))
		for _, c := range containers {
			container, ok := c.(map[string]interface{})
			if !ok {
				continue
			}

			cv := make(map[string]interface{})

			// Name
			if name, ok := container["name"].(string); ok {
				cv["name"] = name
			}

			// Image
			if image, ok := container["image"].(string); ok {
				repo, tag := parseImage(image)
				cv["image"] = map[string]interface{}{
					"repository": repo,
					"tag":        tag,
				}
			}

			// Resources
			if resources, ok := container["resources"].(map[string]interface{}); ok {
				cv["resources"] = resources
			}

			// Ports
			if ports, ok := container["ports"].([]interface{}); ok {
				cv["ports"] = ports
			}

			// Environment variables
			if env, ok := container["env"].([]interface{}); ok {
				cv["env"] = env
				// Detect ConfigMap/Secret references in env
				deps = append(deps, extractEnvDependencies(env, obj.GetNamespace())...)
			}

			// EnvFrom
			if envFrom, ok := container["envFrom"].([]interface{}); ok {
				cv["envFrom"] = envFrom
				deps = append(deps, extractEnvFromDependencies(envFrom, obj.GetNamespace())...)
			}

			// Volume mounts
			if volumeMounts, ok := container["volumeMounts"].([]interface{}); ok {
				cv["volumeMounts"] = volumeMounts
			}

			// Liveness probe
			if probe, ok := container["livenessProbe"].(map[string]interface{}); ok {
				cv["livenessProbe"] = probe
			}

			// Readiness probe
			if probe, ok := container["readinessProbe"].(map[string]interface{}); ok {
				cv["readinessProbe"] = probe
			}

			// Startup probe
			if probe, ok := container["startupProbe"].(map[string]interface{}); ok {
				cv["startupProbe"] = probe
			}

			// Container-level securityContext
			if sc, ok := container["securityContext"].(map[string]interface{}); ok {
				cv["securityContext"] = sc
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

	// ImagePullSecrets
	if secrets, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "imagePullSecrets"); len(secrets) > 0 {
		values["imagePullSecrets"] = secrets
		for _, s := range secrets {
			if secret, ok := s.(map[string]interface{}); ok {
				if name, ok := secret["name"].(string); ok {
					deps = append(deps, types.ResourceKey{
						GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Secret"},
						Namespace: obj.GetNamespace(),
						Name:      name,
					})
				}
			}
		}
	}

	// Node selector
	if nodeSelector, found, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "spec", "nodeSelector"); found {
		values["nodeSelector"] = nodeSelector
	}

	// Tolerations
	if tolerations, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "tolerations"); len(tolerations) > 0 {
		values["tolerations"] = tolerations
	}

	// Affinity
	if affinity, found, _ := unstructured.NestedMap(obj.Object, "spec", "template", "spec", "affinity"); found {
		values["affinity"] = affinity
	}

	// TopologySpreadConstraints
	if tsc, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "topologySpreadConstraints"); len(tsc) > 0 {
		values["topologySpreadConstraints"] = tsc
	}

	// Strategy
	if strategy, found, _ := unstructured.NestedMap(obj.Object, "spec", "strategy"); found {
		values["strategy"] = strategy
	}

	// Selector (for reference, usually shouldn't be templated)
	if selector, found, _ := unstructured.NestedMap(obj.Object, "spec", "selector"); found {
		values["selector"] = selector
	}

	// Pod labels
	if labels, found, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "labels"); found {
		values["podLabels"] = labels
	}

	// Pod annotations
	if annotations, found, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "annotations"); found {
		values["podAnnotations"] = annotations
	}

	return values, deps
}

func (p *DeploymentProcessor) generateTemplate(ctx processor.Context, obj *unstructured.Unstructured, serviceName string) string {
	valuesPath := fmt.Sprintf(".Values.services.%s.deployment", serviceName)
	fullnameHelper := fmt.Sprintf("{{ include \"%s.fullname\" $ }}", ctx.ChartName)

	template := fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.deployment }}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s-%s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s
spec:
  {{- if not .autoscaling }}
  replicas: {{ .replicas | default 1 }}
  {{- end }}
  selector:
    matchLabels:
      {{- include "%s.selectorLabels" $ | nindent 6 }}
      app.kubernetes.io/component: %s
  {{- with .strategy }}
  strategy:
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
        {{- with .podLabels }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
    spec:
      {{- with $.Values.global.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .serviceAccountName }}
      serviceAccountName: {{ . }}
      {{- end }}
      {{- with .podSecurityContext }}
      securityContext:
        {{- toYaml . | nindent 8 }}
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
          {{- with .envFrom }}
          envFrom:
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
          {{- with .livenessProbe }}
          livenessProbe:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .readinessProbe }}
          readinessProbe:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .startupProbe }}
          startupProbe:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .securityContext }}
          securityContext:
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
      {{- with .affinity }}
      affinity:
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

	// Remove unused variable warning
	_ = valuesPath

	return template
}

// Helper functions

func parseImage(image string) (repository, tag string) {
	// Handle digest format
	if strings.Contains(image, "@") {
		parts := strings.SplitN(image, "@", 2)
		return parts[0], parts[1]
	}

	// Handle tag format
	lastColon := strings.LastIndex(image, ":")
	if lastColon == -1 {
		return image, "latest"
	}

	// Check if colon is part of port (e.g., registry:5000/image)
	afterColon := image[lastColon+1:]
	if strings.Contains(afterColon, "/") {
		return image, "latest"
	}

	return image[:lastColon], afterColon
}

func extractEnvDependencies(env []interface{}, namespace string) []types.ResourceKey {
	var deps []types.ResourceKey
	for _, e := range env {
		envVar, ok := e.(map[string]interface{})
		if !ok {
			continue
		}

		valueFrom, ok := envVar["valueFrom"].(map[string]interface{})
		if !ok {
			continue
		}

		// ConfigMap reference
		if cmRef, ok := valueFrom["configMapKeyRef"].(map[string]interface{}); ok {
			if name, ok := cmRef["name"].(string); ok {
				deps = append(deps, types.ResourceKey{
					GVK:       schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
					Namespace: namespace,
					Name:      name,
				})
			}
		}

		// Secret reference
		if secretRef, ok := valueFrom["secretKeyRef"].(map[string]interface{}); ok {
			if name, ok := secretRef["name"].(string); ok {
				deps = append(deps, types.ResourceKey{
					GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Secret"},
					Namespace: namespace,
					Name:      name,
				})
			}
		}
	}
	return deps
}

func extractEnvFromDependencies(envFrom []interface{}, namespace string) []types.ResourceKey {
	var deps []types.ResourceKey
	for _, e := range envFrom {
		envFromSource, ok := e.(map[string]interface{})
		if !ok {
			continue
		}

		// ConfigMap reference
		if cmRef, ok := envFromSource["configMapRef"].(map[string]interface{}); ok {
			if name, ok := cmRef["name"].(string); ok {
				deps = append(deps, types.ResourceKey{
					GVK:       schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
					Namespace: namespace,
					Name:      name,
				})
			}
		}

		// Secret reference
		if secretRef, ok := envFromSource["secretRef"].(map[string]interface{}); ok {
			if name, ok := secretRef["name"].(string); ok {
				deps = append(deps, types.ResourceKey{
					GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Secret"},
					Namespace: namespace,
					Name:      name,
				})
			}
		}
	}
	return deps
}

func extractVolumeDependencies(volumes []interface{}, namespace string) []types.ResourceKey {
	var deps []types.ResourceKey
	for _, v := range volumes {
		volume, ok := v.(map[string]interface{})
		if !ok {
			continue
		}

		// ConfigMap volume
		if cm, ok := volume["configMap"].(map[string]interface{}); ok {
			if name, ok := cm["name"].(string); ok {
				deps = append(deps, types.ResourceKey{
					GVK:       schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
					Namespace: namespace,
					Name:      name,
				})
			}
		}

		// Secret volume
		if secret, ok := volume["secret"].(map[string]interface{}); ok {
			if name, ok := secret["secretName"].(string); ok {
				deps = append(deps, types.ResourceKey{
					GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Secret"},
					Namespace: namespace,
					Name:      name,
				})
			}
		}

		// PVC volume
		if pvc, ok := volume["persistentVolumeClaim"].(map[string]interface{}); ok {
			if name, ok := pvc["claimName"].(string); ok {
				deps = append(deps, types.ResourceKey{
					GVK:       schema.GroupVersionKind{Version: "v1", Kind: "PersistentVolumeClaim"},
					Namespace: namespace,
					Name:      name,
				})
			}
		}
	}
	return deps
}
