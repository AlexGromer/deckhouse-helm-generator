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

	spec, _, _ := unstructured.NestedMap(obj.Object, "spec")
	if spec == nil {
		return values, nil
	}

	// Replicas (default to 1 when not specified)
	if replicas, ok := nestedInt64(obj.Object, "spec", "replicas"); ok {
		values["replicas"] = replicas
	} else {
		values["replicas"] = int64(1)
	}

	deps := extractPodTemplateValues(obj, values, "spec", "template")
	extractWorkloadSelector(obj, values)

	// Strategy
	if strategy, found, _ := unstructured.NestedMap(obj.Object, "spec", "strategy"); found {
		values["strategy"] = strategy
	}

	return values, deps
}

func (p *DeploymentProcessor) generateTemplate(ctx processor.Context, obj *unstructured.Unstructured, serviceName string) string {
	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.deployment }}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s
spec:
  {{- if not .autoscaling }}
  replicas: {{ .replicas | default 1 }}
  {{- end }}
%s  {{- with .strategy }}
  strategy:
    {{- toYaml . | nindent 4 }}
  {{- end }}
%s{{- end }}
{{- end }}
`, serviceName, processor.ObjectName(obj.GetName()), ctx.ChartName, serviceName,
		workloadSelectorTemplate, podTemplate(ctx.ChartName, 2, ""))
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
