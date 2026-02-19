// Package processor provides interfaces and implementations for processing
// Kubernetes resources into Helm template components.
package processor

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor/value"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// Context provides context for processing a resource.
type Context struct {
	// Ctx is the Go context for cancellation.
	Ctx context.Context

	// ChartName is the name of the chart being generated.
	ChartName string

	// OutputMode is the generation mode (universal, separate, library).
	OutputMode types.OutputMode

	// ServiceName is the detected or assigned service name.
	ServiceName string

	// Namespace is the resource's namespace.
	Namespace string

	// AllResources provides access to all extracted resources (for reference resolution).
	AllResources map[types.ResourceKey]*types.ExtractedResource

	// ExternalFileManager manages external files for the chart.
	ExternalFileManager *value.ExternalFileManager

	// ValueProcessor processes complex data values (JSON, XML, base64, etc.).
	ValueProcessor *value.Processor

	// Options contains additional processor options.
	Options map[string]interface{}
}

// Result contains the processing result for a resource.
type Result struct {
	// Processed indicates if the processor handled this resource.
	Processed bool

	// ServiceName is the detected or assigned service name.
	ServiceName string

	// TemplatePath is the relative path for the template within the chart.
	TemplatePath string

	// TemplateContent is the generated Helm template content.
	TemplateContent string

	// ValuesPath is the path within values.yaml for this resource's values.
	ValuesPath string

	// Values contains the extracted values for this resource.
	Values map[string]interface{}

	// Dependencies lists related resource keys (for relationship detection).
	Dependencies []types.ResourceKey

	// ExternalFiles lists external files created by this processor.
	ExternalFiles []*value.ExternalFile

	// Metadata contains additional processor-specific metadata.
	Metadata map[string]interface{}
}

// Processor defines the interface for processing Kubernetes resources.
type Processor interface {
	// Process processes a Kubernetes resource and returns the result.
	// If the processor doesn't support this resource, it returns (false, nil, nil).
	Process(ctx Context, obj *unstructured.Unstructured) (*Result, error)

	// Supports returns the list of GVKs this processor supports.
	Supports() []schema.GroupVersionKind

	// Priority returns the processor priority (higher = processed first).
	// Used when multiple processors can handle the same GVK.
	Priority() int

	// Name returns the processor name for logging/debugging.
	Name() string
}

// BaseProcessor provides common functionality for processors.
type BaseProcessor struct {
	name     string
	priority int
	gvks     []schema.GroupVersionKind
}

// NewBaseProcessor creates a new base processor.
func NewBaseProcessor(name string, priority int, gvks ...schema.GroupVersionKind) BaseProcessor {
	return BaseProcessor{
		name:     name,
		priority: priority,
		gvks:     gvks,
	}
}

// Name returns the processor name.
func (p BaseProcessor) Name() string {
	return p.name
}

// Priority returns the processor priority.
func (p BaseProcessor) Priority() int {
	return p.priority
}

// Supports returns the supported GVKs.
func (p BaseProcessor) Supports() []schema.GroupVersionKind {
	return p.gvks
}

// Helper functions for common template generation patterns.

// ServiceNameFromLabels extracts a service name from common label patterns.
func ServiceNameFromLabels(obj *unstructured.Unstructured) string {
	labels := obj.GetLabels()
	if labels == nil {
		return ""
	}

	// Try common label patterns in order of preference
	patterns := []string{
		"app.kubernetes.io/name",
		"app.kubernetes.io/instance",
		"app",
		"name",
		"component",
	}

	for _, pattern := range patterns {
		if v, ok := labels[pattern]; ok && v != "" {
			return v
		}
	}

	return ""
}

// ServiceNameFromResource determines the service name for a resource.
func ServiceNameFromResource(obj *unstructured.Unstructured) string {
	// First try labels
	if name := ServiceNameFromLabels(obj); name != "" {
		return name
	}

	// Fall back to resource name (strip common suffixes)
	name := obj.GetName()
	return name
}

// SanitizeServiceName converts a service name to a valid Go template identifier.
// Hyphens and dots are converted to camelCase (e.g., "test-module" → "testModule").
func SanitizeServiceName(name string) string {
	if name == "" {
		return name
	}

	result := make([]byte, 0, len(name))
	capitalizeNext := false

	for i, c := range name {
		if c == '-' || c == '.' {
			capitalizeNext = true
			continue
		}
		if capitalizeNext && c >= 'a' && c <= 'z' {
			result = append(result, byte(c-32))
			capitalizeNext = false
		} else if i == 0 && c >= 'A' && c <= 'Z' {
			result = append(result, byte(c+32))
		} else {
			result = append(result, byte(c))
		}
	}

	if len(result) == 0 {
		return name
	}
	return string(result)
}

// ValuesPathForKind returns the standard values path for a resource kind.
func ValuesPathForKind(kind, serviceName string) string {
	kindPath := kindToValuesKey(kind)
	if serviceName == "" {
		return kindPath
	}
	return fmt.Sprintf("services.%s.%s", serviceName, kindPath)
}

// TemplatePathForResource returns the template path for a resource.
func TemplatePathForResource(kind, name, _ string) string {
	return fmt.Sprintf("templates/%s-%s.yaml", kindToFileName(kind), name)
}

// kindToValuesKey converts a Kind to a values.yaml key.
func kindToValuesKey(kind string) string {
	mapping := map[string]string{
		"Deployment":         "deployment",
		"StatefulSet":        "statefulSet",
		"DaemonSet":          "daemonSet",
		"Service":            "service",
		"Ingress":            "ingress",
		"ConfigMap":          "configMap",
		"Secret":             "secret",
		"PersistentVolumeClaim": "persistentVolumeClaim",
		"ServiceAccount":     "serviceAccount",
		"Role":               "role",
		"RoleBinding":        "roleBinding",
		"ClusterRole":        "clusterRole",
		"ClusterRoleBinding": "clusterRoleBinding",
		"HorizontalPodAutoscaler": "hpa",
		"PodDisruptionBudget": "pdb",
		"NetworkPolicy":      "networkPolicy",
	}

	if v, ok := mapping[kind]; ok {
		return v
	}

	// Default: lowercase first letter
	if len(kind) > 0 {
		return string(kind[0]|32) + kind[1:]
	}
	return kind
}

// kindToFileName converts a Kind to a file name component.
func kindToFileName(kind string) string {
	mapping := map[string]string{
		"Deployment":         "deployment",
		"StatefulSet":        "statefulset",
		"DaemonSet":          "daemonset",
		"Service":            "service",
		"Ingress":            "ingress",
		"ConfigMap":          "configmap",
		"Secret":             "secret",
		"PersistentVolumeClaim": "pvc",
		"ServiceAccount":     "serviceaccount",
		"Role":               "role",
		"RoleBinding":        "rolebinding",
		"ClusterRole":        "clusterrole",
		"ClusterRoleBinding": "clusterrolebinding",
		"HorizontalPodAutoscaler": "hpa",
		"PodDisruptionBudget": "pdb",
		"NetworkPolicy":      "networkpolicy",
	}

	if v, ok := mapping[kind]; ok {
		return v
	}

	// Default: lowercase
	return stringToLower(kind)
}

// stringToLower converts ASCII uppercase letters to lowercase without
// allocating a rune slice. This is intentionally used instead of strings.ToLower
// because Kubernetes resource names and kinds are always ASCII.
func stringToLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		b[i] = c
	}
	return string(b)
}
