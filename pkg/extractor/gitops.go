package extractor

import (
	"context"
	"fmt"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// GitOpsExtractorConfig holds configuration for extracting resources from a git repository.
type GitOpsExtractorConfig struct {
	// RepoURL is the URL of the git repository.
	RepoURL string

	// Branch is the branch to check out (default: main).
	Branch string

	// SSHKey is the path to an SSH private key for authentication.
	SSHKey string

	// Depth limits the clone depth (0 = full clone).
	Depth int
}

// GitOpsExtractor extracts Kubernetes resources from a git repository.
type GitOpsExtractor struct {
	config GitOpsExtractorConfig
}

// NewGitOpsExtractor creates a new gitops extractor with default config.
func NewGitOpsExtractor() *GitOpsExtractor {
	return &GitOpsExtractor{}
}

// NewGitOpsExtractorWithConfig creates a new gitops extractor with the given config.
func NewGitOpsExtractorWithConfig(cfg GitOpsExtractorConfig) *GitOpsExtractor {
	return &GitOpsExtractor{config: cfg}
}

// Source returns the source type.
func (e *GitOpsExtractor) Source() types.Source {
	return types.SourceGitOps
}

// Validate checks if the git repository is accessible.
func (e *GitOpsExtractor) Validate(ctx context.Context, opts Options) error {
	// TODO: Implement git validation
	return fmt.Errorf("gitops extraction not yet implemented (use --file instead)")
}

// Extract extracts resources from a git repository.
func (e *GitOpsExtractor) Extract(ctx context.Context, opts Options) (<-chan *types.ExtractedResource, <-chan error) {
	resources := make(chan *types.ExtractedResource)
	errors := make(chan error, 1)

	go func() {
		defer close(resources)
		defer close(errors)

		errors <- fmt.Errorf("gitops extraction not yet implemented (use --file instead)")
	}()

	return resources, errors
}
