package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// NodeGroupProcessor processes Deckhouse NodeGroup resources.
type NodeGroupProcessor struct {
	processor.BaseProcessor
}

// NewNodeGroupProcessor creates a new NodeGroup processor.
func NewNodeGroupProcessor() *NodeGroupProcessor {
	return &NodeGroupProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"nodegroup",
			50,
			schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: "NodeGroup"},
		),
	}
}

// Process processes a NodeGroup resource.
func (p *NodeGroupProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("NodeGroup object is nil")
	}

	name := obj.GetName()
	values := p.extractValues(obj)
	template := p.generateTemplate(ctx, name)

	return &processor.Result{
		Processed:       true,
		ServiceName:     name,
		TemplatePath:    fmt.Sprintf("templates/nodegroup-%s.yaml", name),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.nodeGroup", name),
		Values:          values,
		Metadata:        map[string]interface{}{"name": name},
	}, nil
}

func (p *NodeGroupProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	if nodeType, ok, _ := unstructured.NestedString(obj.Object, "spec", "nodeType"); ok {
		values["nodeType"] = nodeType
	}

	if disruptions, ok, _ := unstructured.NestedMap(obj.Object, "spec", "disruptions"); ok {
		values["disruptions"] = disruptions
	}

	if kubelet, ok, _ := unstructured.NestedMap(obj.Object, "spec", "kubelet"); ok {
		values["kubelet"] = kubelet
	}

	if cloudInstances, ok, _ := unstructured.NestedMap(obj.Object, "spec", "cloudInstances"); ok {
		values["cloudInstances"] = cloudInstances
	}

	return values
}

func (p *NodeGroupProcessor) generateTemplate(ctx processor.Context, name string) string {
	sanitized := processor.SanitizeServiceName(name)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- with $svc.nodeGroup }}
apiVersion: deckhouse.io/v1
kind: NodeGroup
metadata:
  name: %s
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  nodeType: {{ .nodeType }}
  {{- with .disruptions }}
  disruptions:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .kubelet }}
  kubelet:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .cloudInstances }}
  cloudInstances:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
`, sanitized, name, ctx.ChartName)
}
