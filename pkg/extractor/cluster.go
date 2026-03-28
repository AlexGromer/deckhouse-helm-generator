package extractor

import (
	"context"
	"fmt"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ClusterExtractorConfig holds configuration for extracting resources from a live cluster.
type ClusterExtractorConfig struct {
	// Kubeconfig is the path to the kubeconfig file.
	Kubeconfig string

	// Context is the kubeconfig context to use.
	Context string

	// Namespace limits extraction to a specific namespace.
	Namespace string

	// Selector is a label selector to filter resources.
	Selector string

	// ExcludeNamespaces lists namespaces to skip during extraction.
	ExcludeNamespaces []string

	// IncludeSecrets controls whether Secret resources are extracted.
	IncludeSecrets bool

	// SecretStrategy defines how secrets are handled: "mask", "ref", or "include".
	SecretStrategy string
}

// ClusterExtractor extracts Kubernetes resources from a live cluster.
type ClusterExtractor struct {
	config ClusterExtractorConfig
}

// NewClusterExtractor creates a new cluster extractor with default config.
func NewClusterExtractor() *ClusterExtractor {
	return &ClusterExtractor{}
}

// NewClusterExtractorWithConfig creates a new cluster extractor with the given config.
func NewClusterExtractorWithConfig(cfg ClusterExtractorConfig) *ClusterExtractor {
	return &ClusterExtractor{config: cfg}
}

// Source returns the source type.
func (e *ClusterExtractor) Source() types.Source {
	return types.SourceCluster
}

// Validate checks if the cluster connection is valid.
func (e *ClusterExtractor) Validate(ctx context.Context, opts Options) error {
	// TODO: Implement cluster validation
	return fmt.Errorf("cluster extraction not yet implemented (use --file instead)")
}

// Extract extracts resources from a Kubernetes cluster.
func (e *ClusterExtractor) Extract(ctx context.Context, opts Options) (<-chan *types.ExtractedResource, <-chan error) {
	resources := make(chan *types.ExtractedResource)
	errors := make(chan error, 1)

	go func() {
		defer close(resources)
		defer close(errors)

		errors <- fmt.Errorf("cluster extraction not yet implemented (use --file instead)")
	}()

	return resources, errors
}
