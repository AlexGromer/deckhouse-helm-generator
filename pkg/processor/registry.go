package processor

import (
	"regexp"
	"sort"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
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
	result, err := r.process(ctx, obj)
	if err != nil || result == nil {
		return result, err
	}
	normalizeServiceName(result)
	result.TemplateContent = NormalizeResourceNames(result.TemplateContent, result.ServiceName)
	return result, nil
}

// normalizeServiceName enforces the invariant every template relies on: the
// service name (and the services.<name> segment of ValuesPath) is the
// sanitized identifier used in `.Values.services.<name>`.
func normalizeServiceName(result *Result) {
	sanitized := SanitizeServiceName(result.ServiceName)
	if sanitized == result.ServiceName {
		return
	}
	prefix := "services." + result.ServiceName + "."
	if strings.HasPrefix(result.ValuesPath, prefix) {
		result.ValuesPath = "services." + sanitized + "." + strings.TrimPrefix(result.ValuesPath, prefix)
	}
	result.ServiceName = sanitized
}

func (r *Registry) process(ctx Context, obj *unstructured.Unstructured) (*Result, error) {
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

// genericSkippedFields are top-level fields the fallback never copies into a
// chart: the envelope it renders itself and server-populated status.
var genericSkippedFields = map[string]bool{
	"apiVersion": true,
	"kind":       true,
	"metadata":   true,
	"status":     true,
}

// fixedNameKinds are kinds whose metadata.name is dictated by their content
// (CRD: <plural>.<group>, APIService: <version>.<group>); the API server
// rejects them under any other name, so the release prefix must not be added.
var fixedNameKinds = map[string]bool{
	"CustomResourceDefinition": true,
	"APIService":               true,
}

// staticBodyKinds are kinds whose body is rendered verbatim instead of being
// moved to values.yaml. A CRD's OpenAPI schema is not chart configuration and
// would bloat values.yaml (and every schema derived from it).
var staticBodyKinds = map[string]bool{
	"CustomResourceDefinition": true,
}

// chartLabels are the keys emitted by the generated <chart>.labels helper.
var chartLabels = map[string]bool{
	"helm.sh/chart":                true,
	"app.kubernetes.io/name":       true,
	"app.kubernetes.io/instance":   true,
	"app.kubernetes.io/version":    true,
	"app.kubernetes.io/managed-by": true,
}

var goIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// generateGenericTemplate creates a template for a resource no processor
// claims, e.g. an instance of a custom resource. Every top-level field except
// apiVersion, kind, metadata and status is kept: most CRDs use spec, but many
// kinds carry their payload elsewhere (data, rules, webhooks, provisioner,
// parameters, ...). Each field becomes services.<svc>.<kind>.<field> in values.
// serviceName must be pre-sanitized for use in Go templates (no hyphens).
func generateGenericTemplate(ctx Context, obj *unstructured.Unstructured, serviceName string) (string, map[string]interface{}) {
	kind := obj.GetKind()
	name := obj.GetName()
	namespace := obj.GetNamespace()

	valuesRef := ".Values.services." + serviceName + "." + kindToValuesKey(kind)

	var b strings.Builder
	// `enabled | default true` would ignore enabled=false; compare as string
	// so both a boolean and a --set string "false" switch the resource off.
	b.WriteString("{{- if ne (toString " + valuesRef + ".enabled) \"false\" }}\n")
	b.WriteString("apiVersion: " + obj.GetAPIVersion() + "\n")
	b.WriteString("kind: " + kind + "\n")

	b.WriteString("metadata:\n")
	if fixedNameKinds[kind] {
		b.WriteString("  name: " + name + "\n")
	} else {
		b.WriteString("  name: {{ include \"" + ctx.ChartName + ".fullname\" . }}-" + name + "\n")
	}
	if namespace != "" {
		b.WriteString("  namespace: {{ .Release.Namespace }}\n")
	}
	b.WriteString("  labels:\n")
	b.WriteString("    {{- include \"" + ctx.ChartName + ".labels\" . | nindent 4 }}\n")
	labels := obj.GetLabels()
	for k := range labels {
		if chartLabels[k] {
			delete(labels, k) // already set by <chart>.labels; a duplicate key is invalid YAML
		}
	}
	writeStringMap(&b, labels)
	if annotations := obj.GetAnnotations(); len(annotations) > 0 {
		b.WriteString("  annotations:\n")
		writeStringMap(&b, annotations)
	}

	values := map[string]interface{}{
		"enabled": true,
	}

	fields := make([]string, 0, len(obj.Object))
	for field := range obj.Object {
		if !genericSkippedFields[field] {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)

	if staticBodyKinds[kind] {
		body := make(map[string]interface{}, len(fields))
		for _, field := range fields {
			body[field] = obj.Object[field]
		}
		if len(body) > 0 {
			if out, err := yaml.Marshal(body); err == nil {
				b.WriteString(escapeTemplateDelimiters(string(out)))
			}
		}
	} else {
		for _, field := range fields {
			key := field
			if key == "enabled" {
				key = "enabledField" // "enabled" is the on/off switch
			}
			ref := valuesRef + "." + key
			if !goIdentifier.MatchString(key) {
				ref = "(index " + valuesRef + " \"" + key + "\")"
			}
			values[key] = runtime.DeepCopyJSONValue(obj.Object[field])
			b.WriteString(field + ":\n")
			b.WriteString("  {{- toYaml " + ref + " | nindent 2 }}\n")
		}
	}

	b.WriteString("{{- end }}\n")
	return b.String(), values
}

// writeStringMap writes metadata labels or annotations as sorted, quoted
// entries indented under their parent key.
func writeStringMap(b *strings.Builder, m map[string]string) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString("    " + k + ": \"" + escapeTemplateString(m[k]) + "\"\n")
	}
}

// escapeTemplateDelimiters makes literal {{ and }} in static content survive
// Helm rendering.
func escapeTemplateDelimiters(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if i+1 < len(s) && s[i] == '{' && s[i+1] == '{' {
			b.WriteString(`{{"{{"}}`)
			i++
			continue
		}
		if i+1 < len(s) && s[i] == '}' && s[i+1] == '}' {
			b.WriteString(`{{"}}"}}`)
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// escapeTemplateString escapes special characters for use in Helm templates.
// Helm delimiters {{ and }} are wrapped so they are not interpreted as actions.
func escapeTemplateString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		// Escape Helm delimiters — wrap them so the template engine ignores them.
		if i+1 < len(s) && s[i] == '{' && s[i+1] == '{' {
			b.WriteString("{{\"{{\"}}") // outputs literal {{
			i += 2
			continue
		}
		if i+1 < len(s) && s[i] == '}' && s[i+1] == '}' {
			b.WriteString("{{\"}}\"}}") // outputs literal }}
			i += 2
			continue
		}
		switch s[i] {
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		case '\n':
			b.WriteString("\\n")
		case '\t':
			b.WriteString("\\t")
		default:
			b.WriteByte(s[i])
		}
		i++
	}
	return b.String()
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
