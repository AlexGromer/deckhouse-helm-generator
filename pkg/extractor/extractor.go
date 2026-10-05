// Package extractor provides interfaces and implementations for extracting
// Kubernetes resources from various sources (files, clusters, git repos).
package extractor

import (
	"context"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// Options configures the extractor behavior.
type Options struct {
	// Paths contains file or directory paths for file extraction.
	Paths []string

	// Namespace filters resources by namespace (empty = all namespaces).
	Namespace string

	// Namespaces filters resources to specific namespaces.
	Namespaces []string

	// LabelSelector filters resources by labels.
	LabelSelector string

	// IncludeKinds limits extraction to specific resource kinds.
	IncludeKinds []string

	// ExcludeKinds excludes specific resource kinds from extraction.
	ExcludeKinds []string

	// Recursive enables recursive directory scanning for file extraction.
	Recursive bool

	// KubeConfig is the path to kubeconfig for cluster extraction.
	KubeConfig string

	// KubeContext is the kubeconfig context to use.
	KubeContext string

	// ClusterSecrets controls Secrets in cluster extraction: "skip" (default),
	// "mask" (values replaced with REDACTED) or "include".
	ClusterSecrets string

	// GitURL is the git repository URL for gitops extraction.
	GitURL string

	// GitBranch is the branch to checkout.
	GitBranch string

	// GitPath is the subdirectory within the git repo.
	GitPath string

	// GitAuth contains authentication credentials for private repos.
	GitAuth *GitAuthOptions
}

// GitAuthOptions contains git authentication options.
type GitAuthOptions struct {
	// SSHKeyPath is the SSH private key used for ssh:// and git@ URLs. HTTPS
	// credentials come from the user's git configuration (credential helper).
	SSHKeyPath string
}

// Extractor defines the interface for extracting Kubernetes resources.
type Extractor interface {
	// Extract returns channels for extracted resources and errors.
	// The resource channel is closed when extraction is complete.
	// The error channel receives any errors during extraction.
	Extract(ctx context.Context, opts Options) (<-chan *types.ExtractedResource, <-chan error)

	// Source returns the source type of this extractor.
	Source() types.Source

	// Validate checks if the extractor is properly configured.
	Validate(ctx context.Context, opts Options) error
}

// Registry holds registered extractors.
type Registry struct {
	extractors map[types.Source]Extractor
}

// NewRegistry creates a new extractor registry.
func NewRegistry() *Registry {
	return &Registry{
		extractors: make(map[types.Source]Extractor),
	}
}

// Register adds an extractor to the registry.
func (r *Registry) Register(e Extractor) {
	r.extractors[e.Source()] = e
}

// Get returns an extractor by source type.
func (r *Registry) Get(source types.Source) (Extractor, bool) {
	e, ok := r.extractors[source]
	return e, ok
}

// DefaultRegistry returns a registry with all default extractors.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(NewFileExtractor())
	r.Register(NewClusterExtractor())
	r.Register(NewGitOpsExtractor())
	return r
}
