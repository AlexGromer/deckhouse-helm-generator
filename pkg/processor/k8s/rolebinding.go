package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// RoleBindingProcessor processes Kubernetes RoleBinding resources.
type RoleBindingProcessor struct {
	processor.BaseProcessor
}

// NewRoleBindingProcessor creates a new RoleBinding processor.
func NewRoleBindingProcessor() *RoleBindingProcessor {
	return &RoleBindingProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"rolebinding",
			70,
			schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"},
		),
	}
}

// Process processes a RoleBinding resource.
func (p *RoleBindingProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("RoleBinding object is nil")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()
	namespace := obj.GetNamespace()

	values, deps := p.extractValues(obj)

	template := p.generateTemplate(ctx, serviceName)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-rolebinding.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.roleBinding", serviceName),
		Values:          values,
		Dependencies:    deps,
		Metadata: map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}, nil
}

func (p *RoleBindingProcessor) extractValues(obj *unstructured.Unstructured) (map[string]interface{}, []types.ResourceKey) {
	values := make(map[string]interface{})
	var deps []types.ResourceKey

	// Extract roleRef
	if roleRef, ok, _ := unstructured.NestedMap(obj.Object, "roleRef"); ok {
		values["roleRef"] = roleRef

		// Add dependency on the referenced Role/ClusterRole
		if kind, ok := roleRef["kind"].(string); ok {
			if name, ok := roleRef["name"].(string); ok {
				deps = append(deps, types.ResourceKey{
					GVK: schema.GroupVersionKind{
						Group: "rbac.authorization.k8s.io",
						Kind:  kind,
					},
					Namespace: obj.GetNamespace(),
					Name:      name,
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
				if ns == "" {
					ns = obj.GetNamespace()
				}
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

func (p *RoleBindingProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	fullnameHelper := fmt.Sprintf(`{{ include "%s.fullname" $ }}`, ctx.ChartName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.roleBinding }}
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: %s-%s
  namespace: {{ $.Release.Namespace }}
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
