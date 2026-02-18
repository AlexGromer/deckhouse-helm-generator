// Package types provides shared types for the Deckhouse Helm Generator.
package types

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Source represents the origin of extracted resources.
type Source string

const (
	SourceCluster Source = "cluster"
	SourceFile    Source = "file"
	SourceGitOps  Source = "gitops"
)

// ExtractedResource represents a Kubernetes resource extracted from any source.
type ExtractedResource struct {
	// Object is the unstructured Kubernetes object.
	Object *unstructured.Unstructured

	// Source indicates where this resource was extracted from.
	Source Source

	// SourcePath is the file path or URL for file/gitops sources.
	SourcePath string

	// GVK is the GroupVersionKind of the resource.
	GVK schema.GroupVersionKind
}

// ResourceKey creates a unique identifier for a resource.
func (r *ExtractedResource) ResourceKey() ResourceKey {
	return ResourceKey{
		GVK:       r.GVK,
		Namespace: r.Object.GetNamespace(),
		Name:      r.Object.GetName(),
	}
}

// ResourceKey uniquely identifies a Kubernetes resource.
type ResourceKey struct {
	GVK       schema.GroupVersionKind
	Namespace string
	Name      string
}

// String returns a human-readable representation of the resource key.
func (k ResourceKey) String() string {
	if k.Namespace == "" {
		return k.GVK.Kind + "/" + k.Name
	}
	return k.GVK.Kind + "/" + k.Namespace + "/" + k.Name
}

// ProcessedResource represents a resource after processing.
type ProcessedResource struct {
	// Original is the extracted resource.
	Original *ExtractedResource

	// ServiceName is the detected or assigned service name this resource belongs to.
	ServiceName string

	// TemplatePath is the path within the chart templates directory.
	TemplatePath string

	// TemplateContent is the generated Helm template content.
	TemplateContent string

	// ValuesPath is the path within values.yaml for this resource's values.
	ValuesPath string

	// Values contains the extracted values for this resource.
	Values map[string]interface{}

	// Dependencies lists resource keys this resource depends on.
	Dependencies []ResourceKey
}

// ResourceGroup represents a group of related resources (typically a service).
type ResourceGroup struct {
	// Name is the group/service name.
	Name string

	// Resources contains all resources in this group.
	Resources []*ProcessedResource

	// Namespace is the primary namespace for this group.
	Namespace string
}

// OutputMode represents the chart generation strategy.
type OutputMode string

const (
	// OutputModeUniversal generates one chart with all services in values.yaml.
	OutputModeUniversal OutputMode = "universal"

	// OutputModeSeparate generates separate charts per service.
	OutputModeSeparate OutputMode = "separate"

	// OutputModeLibrary generates a library chart with thin wrappers.
	OutputModeLibrary OutputMode = "library"
)

// GeneratedChart represents a generated Helm chart.
type GeneratedChart struct {
	// Name is the chart name.
	Name string

	// Path is the output directory path.
	Path string

	// ChartYAML is the Chart.yaml content.
	ChartYAML string

	// ValuesYAML is the values.yaml content.
	ValuesYAML string

	// Templates is a map of template path to content.
	Templates map[string]string

	// Helpers is the _helpers.tpl content.
	Helpers string

	// Notes is the NOTES.txt content.
	Notes string

	// ValuesSchema is the values.schema.json content (optional).
	ValuesSchema string

	// ExternalFiles is a list of external files (path, content).
	ExternalFiles []ExternalFileInfo
}

// ExternalFileInfo represents an external file in the chart.
type ExternalFileInfo struct {
	// Path is the relative path in the chart (e.g., "files/config.json").
	Path string

	// Content is the file content.
	Content string
}
