package extractor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
	"sigs.k8s.io/yaml"
)

// GitAuthType represents the type of git authentication.
type GitAuthType string

const (
	GitAuthTypeToken      GitAuthType = "token"
	GitAuthTypeSSHKey     GitAuthType = "ssh-key"
	GitAuthTypeCredHelper GitAuthType = "credential-helper"
)

// TokenAuth holds token-based authentication details.
type TokenAuth struct {
	// Token is the personal access token or OAuth token.
	Token string
	// Username is optional; defaults to "git" for GitHub/GitLab.
	Username string
}

// SSHKeyAuth holds SSH key authentication details.
type SSHKeyAuth struct {
	// KeyPath is the path to the SSH private key.
	KeyPath string
	// Passphrase is the key passphrase (empty if none).
	Passphrase string
	// KnownHostsPath overrides the default known_hosts file.
	KnownHostsPath string
}

// CredentialHelper holds git credential helper configuration.
type CredentialHelper struct {
	// Helper is the credential helper command (e.g., "store", "cache", "osxkeychain").
	Helper string
}

// GitAuth holds authentication details for git operations.
type GitAuth struct {
	// Type indicates which auth method to use.
	Type GitAuthType

	// Token holds token-based auth (when Type == GitAuthTypeToken).
	Token *TokenAuth

	// SSHKey holds SSH key auth (when Type == GitAuthTypeSSHKey).
	SSHKey *SSHKeyAuth

	// CredHelper holds credential helper config (when Type == GitAuthTypeCredHelper).
	CredHelper *CredentialHelper
}

// Validate checks if the GitAuth is properly configured.
func (a *GitAuth) Validate() error {
	if a == nil {
		return nil
	}
	switch a.Type {
	case GitAuthTypeToken:
		if a.Token == nil || a.Token.Token == "" {
			return fmt.Errorf("token auth requires a non-empty token")
		}
	case GitAuthTypeSSHKey:
		if a.SSHKey == nil || a.SSHKey.KeyPath == "" {
			return fmt.Errorf("ssh-key auth requires a key path")
		}
		if _, err := os.Stat(a.SSHKey.KeyPath); err != nil {
			return fmt.Errorf("ssh key not found at %s: %w", a.SSHKey.KeyPath, err)
		}
	case GitAuthTypeCredHelper:
		if a.CredHelper == nil || a.CredHelper.Helper == "" {
			return fmt.Errorf("credential-helper auth requires a helper name")
		}
	case "":
		// No auth — fine for public repos
	default:
		return fmt.Errorf("unknown git auth type: %q", a.Type)
	}
	return nil
}

// GitOpsManifestType represents the type of a detected GitOps manifest.
type GitOpsManifestType string

const (
	GitOpsManifestArgoApplication   GitOpsManifestType = "argocd-application"
	GitOpsManifestFluxGitRepository GitOpsManifestType = "flux-gitrepository"
	GitOpsManifestFluxKustomization GitOpsManifestType = "flux-kustomization"
)

// GitOpsManifest represents a detected GitOps manifest in a directory.
type GitOpsManifest struct {
	// Type is the kind of GitOps manifest.
	Type GitOpsManifestType

	// Path is the file path where the manifest was found.
	Path string

	// Name is the metadata.name of the manifest.
	Name string

	// Namespace is the metadata.namespace of the manifest.
	Namespace string
}

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

	// Auth holds structured authentication config.
	Auth *GitAuth

	// ExcludeDirs lists directory names to exclude during YAML discovery.
	ExcludeDirs []string
}

// DefaultExcludeDirs returns the default directories to exclude from YAML discovery.
func DefaultExcludeDirs() []string {
	return []string{".git", "vendor", "node_modules", ".github", ".gitlab"}
}

// Validate checks if the GitOpsExtractorConfig is valid.
func (c *GitOpsExtractorConfig) Validate() error {
	if c.RepoURL == "" {
		return fmt.Errorf("repo URL is required")
	}

	if c.Depth < 0 {
		return fmt.Errorf("clone depth must be non-negative, got %d", c.Depth)
	}

	if c.Auth != nil {
		if err := c.Auth.Validate(); err != nil {
			return fmt.Errorf("auth config: %w", err)
		}
	}

	return nil
}

// DiscoverYAMLFiles recursively discovers YAML files under rootDir,
// excluding directories whose names appear in the excludes list.
// If excludes is nil, DefaultExcludeDirs() is used.
func DiscoverYAMLFiles(rootDir string, excludes []string) ([]string, error) {
	if excludes == nil {
		excludes = DefaultExcludeDirs()
	}

	// Build a set for O(1) lookup
	excludeSet := make(map[string]bool, len(excludes))
	for _, e := range excludes {
		excludeSet[e] = true
	}

	info, err := os.Stat(rootDir)
	if err != nil {
		return nil, fmt.Errorf("cannot access root dir %s: %w", rootDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", rootDir)
	}

	var files []string
	err = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if excludeSet[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if isYAMLFile(path) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", rootDir, err)
	}

	return files, nil
}

// DetectKustomization checks if a kustomization.yaml (or kustomization.yml, Kustomization)
// exists in the given directory.
func DetectKustomization(dir string) bool {
	kustomizationFiles := []string{
		"kustomization.yaml",
		"kustomization.yml",
		"Kustomization",
	}
	for _, name := range kustomizationFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// DetectGitOpsManifests scans a directory (non-recursively) for GitOps manifests:
// ArgoCD Application, Flux GitRepository, Flux Kustomization.
func DetectGitOpsManifests(dir string) ([]GitOpsManifest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var manifests []GitOpsManifest
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !isYAMLFile(entry.Name()) {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue // skip unreadable files
		}

		// Split multi-doc YAML
		docs := splitYAMLDocuments(data)
		for _, doc := range docs {
			manifest, ok := parseGitOpsManifest(doc, path)
			if ok {
				manifests = append(manifests, manifest)
			}
		}
	}

	return manifests, nil
}

// parseGitOpsManifest tries to parse a YAML document as a GitOps manifest.
func parseGitOpsManifest(data []byte, path string) (GitOpsManifest, bool) {
	var meta struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Metadata   struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
	}

	if err := yaml.Unmarshal(data, &meta); err != nil {
		return GitOpsManifest{}, false
	}

	var manifestType GitOpsManifestType

	switch {
	case meta.Kind == "Application" && strings.HasPrefix(meta.APIVersion, "argoproj.io/"):
		manifestType = GitOpsManifestArgoApplication
	case meta.Kind == "GitRepository" && strings.HasPrefix(meta.APIVersion, "source.toolkit.fluxcd.io/"):
		manifestType = GitOpsManifestFluxGitRepository
	case meta.Kind == "Kustomization" && strings.HasPrefix(meta.APIVersion, "kustomize.toolkit.fluxcd.io/"):
		manifestType = GitOpsManifestFluxKustomization
	default:
		return GitOpsManifest{}, false
	}

	return GitOpsManifest{
		Type:      manifestType,
		Path:      path,
		Name:      meta.Metadata.Name,
		Namespace: meta.Metadata.Namespace,
	}, true
}

// GitOpsExtractor extracts Kubernetes resources from a git repository.
type GitOpsExtractor struct {
	config GitOpsExtractorConfig
}

// NewGitOpsExtractor creates a new gitops extractor with default config.
func NewGitOpsExtractor() *GitOpsExtractor {
	return &GitOpsExtractor{
		config: GitOpsExtractorConfig{
			Branch: "main",
		},
	}
}

// NewGitOpsExtractorWithConfig creates a new gitops extractor with the given config.
func NewGitOpsExtractorWithConfig(cfg GitOpsExtractorConfig) *GitOpsExtractor {
	if cfg.Branch == "" {
		cfg.Branch = "main"
	}
	return &GitOpsExtractor{config: cfg}
}

// Config returns a copy of the extractor's configuration.
func (e *GitOpsExtractor) Config() GitOpsExtractorConfig {
	return e.config
}

// Source returns the source type.
func (e *GitOpsExtractor) Source() types.Source {
	return types.SourceGitOps
}

// Validate checks if the git repository configuration is valid.
func (e *GitOpsExtractor) Validate(ctx context.Context, opts Options) error {
	if err := e.config.Validate(); err != nil {
		return fmt.Errorf("invalid gitops config: %w", err)
	}
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
