package extractor

import (
	"context"
	"fmt"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ClusterExtractor extracts Kubernetes resources from a live cluster.
type ClusterExtractor struct {
	// TODO: Add client-go client
}

// NewClusterExtractor creates a new cluster extractor.
func NewClusterExtractor() *ClusterExtractor {
	return &ClusterExtractor{}
}

// Source returns the source type.
func (e *ClusterExtractor) Source() types.Source {
	return types.SourceCluster
}

// Validate checks if the cluster connection is valid.
func (e *ClusterExtractor) Validate(ctx context.Context, opts Options) error {
	// TODO: Implement cluster validation
	return fmt.Errorf("cluster extractor not yet implemented")
}

// Extract extracts resources from a Kubernetes cluster.
func (e *ClusterExtractor) Extract(ctx context.Context, opts Options) (<-chan *types.ExtractedResource, <-chan error) {
	resources := make(chan *types.ExtractedResource)
	errors := make(chan error, 1)

	go func() {
		defer close(resources)
		defer close(errors)

		errors <- fmt.Errorf("cluster extractor not yet implemented")
	}()

	return resources, errors
}
