package extractor

import (
	"context"
	"fmt"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// GitOpsExtractor extracts Kubernetes resources from a git repository.
type GitOpsExtractor struct {
	// TODO: Add go-git client
}

// NewGitOpsExtractor creates a new gitops extractor.
func NewGitOpsExtractor() *GitOpsExtractor {
	return &GitOpsExtractor{}
}

// Source returns the source type.
func (e *GitOpsExtractor) Source() types.Source {
	return types.SourceGitOps
}

// Validate checks if the git repository is accessible.
func (e *GitOpsExtractor) Validate(ctx context.Context, opts Options) error {
	// TODO: Implement git validation
	return fmt.Errorf("gitops extractor not yet implemented")
}

// Extract extracts resources from a git repository.
func (e *GitOpsExtractor) Extract(ctx context.Context, opts Options) (<-chan *types.ExtractedResource, <-chan error) {
	resources := make(chan *types.ExtractedResource)
	errors := make(chan error, 1)

	go func() {
		defer close(resources)
		defer close(errors)

		errors <- fmt.Errorf("gitops extractor not yet implemented")
	}()

	return resources, errors
}
