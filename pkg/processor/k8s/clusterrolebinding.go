package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ClusterRoleBindingProcessor processes Kubernetes ClusterRoleBinding resources.
type ClusterRoleBindingProcessor struct {
	processor.BaseProcessor
}

// NewClusterRoleBindingProcessor creates a new ClusterRoleBinding processor.
func NewClusterRoleBindingProcessor() *ClusterRoleBindingProcessor {
	return &ClusterRoleBindingProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"clusterrolebinding",
			70,
			schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRoleBinding"},
		),
	}
}

// Process processes a ClusterRoleBinding resource.
func (p *ClusterRoleBindingProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("ClusterRoleBinding object is nil")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()

	values, deps := p.extractValues(obj)

	template := p.generateTemplate(ctx, serviceName)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-clusterrolebinding.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.clusterRoleBinding", serviceName),
		Values:          values,
		Dependencies:    deps,
		Metadata: map[string]interface{}{
			"name": name,
		},
	}, nil
}

func (p *ClusterRoleBindingProcessor) extractValues(obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey

	// Extract roleRef
	if roleRef, ok, _ := unstructured.NestedMap(obj.Object, "roleRef"); ok {
		values["roleRef"] = roleRef

		// Add dependency on the referenced ClusterRole
		if kind, ok := roleRef["kind"].(string); ok {
			if name, ok := roleRef["name"].(string); ok {
				deps = append(deps, types.ResourceKey{
					GVK: schema.GroupVersionKind{
						Group: "rbac.authorization.k8s.io",
						Kind:  kind,
					},
					Name: name,
				})
			}
		}
	}

	// Extract subjects
	if subjects, ok, _ := unstructured.NestedSlice(obj.Object, "subjects"); ok && len(subjects) > 0 {
		values["subjects"] = subjects

		// Add dependencies for ServiceAccount subjects
		for _, s := range subjects {
			subj, ok := s.(map[string]interface{})
			if !ok {
				continue
			}
			kind, _ := subj["kind"].(string)
			name, _ := subj["name"].(string)
			if kind == "ServiceAccount" && name != "" {
				ns, _ := subj["namespace"].(string)
				deps = append(deps, types.ResourceKey{
					GVK: schema.GroupVersionKind{
						Kind: "ServiceAccount",
					},
					Namespace: ns,
					Name:      name,
				})
			}
		}
	}

	return values, deps
}

func (p *ClusterRoleBindingProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	fullnameHelper := fmt.Sprintf(`{{ include "%s.fullname" $ }}`, ctx.ChartName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.clusterRoleBinding }}
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: %s-%s
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s
{{- with .roleRef }}
roleRef:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .subjects }}
subjects:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}
{{- end }}
`, serviceName, fullnameHelper, serviceName, ctx.ChartName, serviceName)
}
