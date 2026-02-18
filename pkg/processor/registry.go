package processor

import (
	"sort"
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Registry manages processor registration and lookup.
type Registry struct {
	mu         sync.RWMutex
	processors []Processor
	byGVK      map[schema.GroupVersionKind][]Processor
}

// NewRegistry creates a new processor registry.
func NewRegistry() *Registry {
	return &Registry{
		processors: make([]Processor, 0),
		byGVK:      make(map[schema.GroupVersionKind][]Processor),
	}
}

// Register adds a processor to the registry.
func (r *Registry) Register(p Processor) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.processors = append(r.processors, p)

	// Index by GVK
	for _, gvk := range p.Supports() {
		r.byGVK[gvk] = append(r.byGVK[gvk], p)
		// Keep sorted by priority (highest first)
		sort.Slice(r.byGVK[gvk], func(i, j int) bool {
			return r.byGVK[gvk][i].Priority() > r.byGVK[gvk][j].Priority()
		})
	}
}

// GetProcessor returns the highest-priority processor for a GVK.
func (r *Registry) GetProcessor(gvk schema.GroupVersionKind) (Processor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	processors, ok := r.byGVK[gvk]
	if !ok || len(processors) == 0 {
		return nil, false
	}

	return processors[0], true
}

// GetProcessors returns all processors for a GVK, sorted by priority.
func (r *Registry) GetProcessors(gvk schema.GroupVersionKind) []Processor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	processors, ok := r.byGVK[gvk]
	if !ok {
		return nil
	}

	// Return a copy
	result := make([]Processor, len(processors))
	copy(result, processors)
	return result
}

// Process processes a resource using the first matching processor.
func (r *Registry) Process(ctx Context, obj *unstructured.Unstructured) (*Result, error) {
	gvk := obj.GroupVersionKind()

	processors := r.GetProcessors(gvk)
	if len(processors) == 0 {
		// No processor found, use generic processor
		return r.processGeneric(ctx, obj)
	}

	// Try processors in priority order
	for _, p := range processors {
		result, err := p.Process(ctx, obj)
		if err != nil {
			return nil, err
		}
		if result != nil && result.Processed {
			return result, nil
		}
	}

	// No processor handled it, use generic
	return r.processGeneric(ctx, obj)
}

// processGeneric provides a fallback for unhandled resources.
func (r *Registry) processGeneric(ctx Context, obj *unstructured.Unstructured) (*Result, error) {
	serviceName := SanitizeServiceName(ServiceNameFromResource(obj))
	kind := obj.GetKind()
	name := obj.GetName()

	// Generate a basic template that just includes the resource as-is
	// but with some values templated
	template, values := generateGenericTemplate(ctx, obj, serviceName)

	return &Result{
		Processed:       true,
		ServiceName:     serviceName,
		TemplatePath:    TemplatePathForResource(kind, name, obj.GetNamespace()),
		TemplateContent: template,
		ValuesPath:      ValuesPathForKind(kind, serviceName),
		Values:          values,
	}, nil
}

// generateGenericTemplate creates a basic template for any resource.
// serviceName must be pre-sanitized for use in Go templates (no hyphens).
func generateGenericTemplate(ctx Context, obj *unstructured.Unstructured, serviceName string) (string, map[string]interface{}) {
	kind := obj.GetKind()
	name := obj.GetName()
	namespace := obj.GetNamespace()

	valuesKey := kindToValuesKey(kind)
	valuesPath := "services." + serviceName + "." + valuesKey

	// Start building the template
	var template string

	// Add enabled check
	template = "{{- if " + toTemplateRef(valuesPath+".enabled", true) + " }}\n"

	// API version and kind
	template += "apiVersion: " + obj.GetAPIVersion() + "\n"
	template += "kind: " + kind + "\n"

	// Metadata
	template += "metadata:\n"
	template += "  name: {{ include \"" + ctx.ChartName + ".fullname\" . }}-" + name + "\n"
	if namespace != "" {
		template += "  namespace: {{ .Release.Namespace }}\n"
	}

	// Labels
	template += "  labels:\n"
	template += "    {{- include \"" + ctx.ChartName + ".labels\" . | nindent 4 }}\n"

	// Add original labels if present
	if labels := obj.GetLabels(); len(labels) > 0 {
		for k, v := range labels {
			template += "    " + k + ": " + v + "\n"
		}
	}

	// Annotations if present
	if annotations := obj.GetAnnotations(); len(annotations) > 0 {
		template += "  annotations:\n"
		for k, v := range annotations {
			template += "    " + k + ": \"" + escapeTemplateString(v) + "\"\n"
		}
	}

	// Spec (as-is for generic resources)
	spec, found, _ := unstructured.NestedFieldCopy(obj.Object, "spec")
	if found && spec != nil {
		template += "spec:\n"
		template += "  {{- toYaml " + toTemplateRef(valuesPath+".spec", nil) + " | nindent 2 }}\n"
	}

	template += "{{- end }}\n"

	// Build values
	values := map[string]interface{}{
		"enabled": true,
	}
	if spec != nil {
		values["spec"] = spec
	}

	return template, values
}

// toTemplateRef converts a values path to a Helm template reference.
func toTemplateRef(path string, defaultValue interface{}) string {
	if defaultValue == nil {
		return ".Values." + path
	}
	switch v := defaultValue.(type) {
	case bool:
		if v {
			return "(.Values." + path + " | default true)"
		}
		return ".Values." + path
	default:
		return ".Values." + path
	}
}

// escapeTemplateString escapes special characters for use in templates.
func escapeTemplateString(s string) string {
	// Escape backslashes and quotes
	result := ""
	for _, c := range s {
		switch c {
		case '\\':
			result += "\\\\"
		case '"':
			result += "\\\""
		case '\n':
			result += "\\n"
		case '\t':
			result += "\\t"
		default:
			result += string(c)
		}
	}
	return result
}

// All returns all registered processors.
func (r *Registry) All() []Processor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Processor, len(r.processors))
	copy(result, r.processors)
	return result
}

// SupportedGVKs returns all GVKs that have registered processors.
func (r *Registry) SupportedGVKs() []schema.GroupVersionKind {
	r.mu.RLock()
	defer r.mu.RUnlock()

	gvks := make([]schema.GroupVersionKind, 0, len(r.byGVK))
	for gvk := range r.byGVK {
		gvks = append(gvks, gvk)
	}
	return gvks
}
