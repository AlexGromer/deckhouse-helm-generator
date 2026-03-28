package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// InstanceClassProcessor processes Deckhouse cloud InstanceClass resources
// (OpenstackInstanceClass, AWSInstanceClass, GCPInstanceClass, AzureInstanceClass, YandexInstanceClass).
type InstanceClassProcessor struct {
	processor.BaseProcessor
	kind string
}

// newInstanceClassProcessor creates a processor for the given cloud InstanceClass kind.
func newInstanceClassProcessor(kind string) *InstanceClassProcessor {
	return &InstanceClassProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"instanceclass-"+kind,
			50,
			schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1", Kind: kind},
		),
		kind: kind,
	}
}

// NewOpenstackInstanceClassProcessor creates a processor for OpenstackInstanceClass.
func NewOpenstackInstanceClassProcessor() *InstanceClassProcessor {
	return newInstanceClassProcessor("OpenStackInstanceClass")
}

// NewAWSInstanceClassProcessor creates a processor for AWSInstanceClass.
func NewAWSInstanceClassProcessor() *InstanceClassProcessor {
	return newInstanceClassProcessor("AWSInstanceClass")
}

// NewGCPInstanceClassProcessor creates a processor for GCPInstanceClass.
func NewGCPInstanceClassProcessor() *InstanceClassProcessor {
	return newInstanceClassProcessor("GCPInstanceClass")
}

// NewAzureInstanceClassProcessor creates a processor for AzureInstanceClass.
func NewAzureInstanceClassProcessor() *InstanceClassProcessor {
	return newInstanceClassProcessor("AzureInstanceClass")
}

// NewYandexInstanceClassProcessor creates a processor for YandexInstanceClass.
func NewYandexInstanceClassProcessor() *InstanceClassProcessor {
	return newInstanceClassProcessor("YandexInstanceClass")
}

// Process processes an InstanceClass resource.
func (p *InstanceClassProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("InstanceClass object is nil")
	}

	name := obj.GetName()
	values := p.extractValues(obj)
	template := p.generateTemplate(ctx, name)

	return &processor.Result{
		Processed:       true,
		ServiceName:     name,
		TemplatePath:    fmt.Sprintf("templates/instanceclass-%s.yaml", name),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("services.%s.instanceClass", name),
		Values:          values,
		Metadata:        map[string]interface{}{"name": name, "kind": p.kind},
	}, nil
}

func (p *InstanceClassProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	if spec, ok, _ := unstructured.NestedMap(obj.Object, "spec"); ok {
		values["spec"] = spec
	}

	// Extract flavor/machineType (provider-specific field names)
	if flavor, ok, _ := unstructured.NestedString(obj.Object, "spec", "flavorName"); ok {
		values["flavorName"] = flavor
	}
	if machineType, ok, _ := unstructured.NestedString(obj.Object, "spec", "machineType"); ok {
		values["machineType"] = machineType
	}
	if instanceType, ok, _ := unstructured.NestedString(obj.Object, "spec", "instanceType"); ok {
		values["instanceType"] = instanceType
	}

	// Extract image
	if image, ok, _ := unstructured.NestedString(obj.Object, "spec", "imageName"); ok {
		values["imageName"] = image
	}

	// Extract rootDiskSize
	if rootDiskSize, ok, _ := unstructured.NestedInt64(obj.Object, "spec", "rootDiskSize"); ok {
		values["rootDiskSize"] = rootDiskSize
	}

	// Extract mainNetwork
	if mainNetwork, ok, _ := unstructured.NestedString(obj.Object, "spec", "mainNetwork"); ok {
		values["mainNetwork"] = mainNetwork
	}

	return values
}

func (p *InstanceClassProcessor) generateTemplate(ctx processor.Context, name string) string {
	sanitized := processor.SanitizeServiceName(name)

	return fmt.Sprintf(`{{- $svc := .Values.services.%s -}}
{{- with $svc.instanceClass }}
apiVersion: deckhouse.io/v1
kind: %s
metadata:
  name: %s
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
spec:
  {{- toYaml .spec | nindent 2 }}
{{- end }}
`, sanitized, p.kind, name, ctx.ChartName)
}
