package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// ClusterRoleProcessor processes Kubernetes ClusterRole resources.
type ClusterRoleProcessor struct {
	processor.BaseProcessor
}

// NewClusterRoleProcessor creates a new ClusterRole processor.
func NewClusterRoleProcessor() *ClusterRoleProcessor {
	return &ClusterRoleProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"clusterrole",
			80,
			schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"},
		),
	}
}

// Process processes a ClusterRole resource.
func (p *ClusterRoleProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("ClusterRole object is nil")
	}

	serviceName := processor.ServiceNameFromResource(obj)
	if serviceName == "" {
		serviceName = obj.GetName()
	}

	name := obj.GetName()

	values := p.extractValues(obj)

	template := p.generateTemplate(ctx, serviceName)

	return &processor.Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    fmt.Sprintf("templates/%s-clusterrole.yaml", serviceName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.clusterRole", serviceName),
		Values:          values,
		Metadata: map[string]interface{}{
			"name": name,
		},
	}, nil
}

func (p *ClusterRoleProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	if rules, ok, _ := unstructured.NestedSlice(obj.Object, "rules"); ok && len(rules) > 0 {
		values["rules"] = rules
	}

	if aggRule, ok, _ := unstructured.NestedMap(obj.Object, "aggregationRule"); ok {
		values["aggregationRule"] = aggRule
	}

	return values
}

func (p *ClusterRoleProcessor) generateTemplate(ctx processor.Context, serviceName string) string {
	fullnameHelper := fmt.Sprintf(`{{ include "%s.fullname" $ }}`, ctx.ChartName)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- if $svc.enabled }}
{{- with $svc.clusterRole }}
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: %s-%s
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
    app.kubernetes.io/component: %s
{{- with .aggregationRule }}
aggregationRule:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .rules }}
rules:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}
{{- end }}
`, serviceName, fullnameHelper, serviceName, ctx.ChartName, serviceName)
}
