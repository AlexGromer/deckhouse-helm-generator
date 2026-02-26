package k8s

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// PriorityClassProcessor processes Kubernetes PriorityClass resources (scheduling.k8s.io/v1).
// PriorityClass is cluster-scoped — no namespace, no service wrapper.
type PriorityClassProcessor struct {
	processor.BaseProcessor
}

// NewPriorityClassProcessor creates a new PriorityClass processor.
func NewPriorityClassProcessor() *PriorityClassProcessor {
	return &PriorityClassProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"priorityclass",
			80,
			schema.GroupVersionKind{Group: "scheduling.k8s.io", Version: "v1", Kind: "PriorityClass"},
		),
	}
}

// Process processes a PriorityClass resource.
func (p *PriorityClassProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, errors.New("PriorityClass object is nil")
	}

	// PriorityClass is cluster-scoped: use obj.GetName() directly as ServiceName.
	name := obj.GetName()
	safeName := processor.SanitizeServiceName(name)

	values := p.extractValues(obj)

	template := p.generateTemplate(ctx, name, safeName)

	return &processor.Result{
		Processed:       true,
		ServiceName:     safeName,
		TemplatePath:    fmt.Sprintf("templates/priorityclass-%s.yaml", safeName),
		TemplateContent: template,
		ValuesPath:      fmt.Sprintf("priorityClasses.%s", safeName),
		Values:          values,
		Dependencies:    []types.ResourceKey{},
		Metadata: map[string]interface{}{
			"name": name,
		},
	}, nil
}

func (p *PriorityClassProcessor) extractValues(obj *unstructured.Unstructured) map[string]interface{} {
	values := make(map[string]interface{})

	// Extract value (int32 stored as int64 in unstructured)
	if val, ok := nestedInt64(obj.Object, "value"); ok {
		values["value"] = val
	}

	// Extract globalDefault (bool)
	if globalDefault, ok, _ := unstructured.NestedBool(obj.Object, "globalDefault"); ok {
		values["globalDefault"] = globalDefault
	}

	// Extract preemptionPolicy (string)
	if preemptionPolicy, ok, _ := unstructured.NestedString(obj.Object, "preemptionPolicy"); ok && preemptionPolicy != "" {
		values["preemptionPolicy"] = preemptionPolicy
	}

	// Extract description (string)
	if description, ok, _ := unstructured.NestedString(obj.Object, "description"); ok && description != "" {
		values["description"] = description
	}

	return values
}

func (p *PriorityClassProcessor) generateTemplate(ctx processor.Context, name, safeName string) string {
	return fmt.Sprintf(`{{- with (index .Values.priorityClasses "%s") }}
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: %s
  labels:
    {{- include "%s.labels" $ | nindent 4 }}
value: {{ .value }}
{{- with .description }}
description: {{ . | quote }}
{{- end }}
globalDefault: {{ .globalDefault | default false }}
preemptionPolicy: {{ .preemptionPolicy | default "PreemptLowerPriority" }}
{{- end }}
`, safeName, name, ctx.ChartName)
}
