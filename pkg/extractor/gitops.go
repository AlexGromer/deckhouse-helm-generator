package extractor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// GitOpsExtractor extracts manifests from a Git repository: it makes a
// shallow clone with the git CLI and reads the YAML files below GitPath like
// the file extractor, with the same filters.
type GitOpsExtractor struct {
	// git is the git executable; tests may override it.
	git string
}

// NewGitOpsExtractor creates a GitOps extractor using git from PATH.
func NewGitOpsExtractor() *GitOpsExtractor {
	return &GitOpsExtractor{git: "git"}
}

// Source returns the source type.
func (e *GitOpsExtractor) Source() types.Source {
	return types.SourceGitOps
}

// Validate checks the options and that git is available.
func (e *GitOpsExtractor) Validate(_ context.Context, opts Options) error {
	if opts.GitURL == "" {
		return fmt.Errorf("--git-repo is required for gitops extraction")
	}
	if filepath.IsAbs(opts.GitPath) || strings.Contains(filepath.ToSlash(opts.GitPath), "..") {
		return fmt.Errorf("--git-path must be a relative path inside the repository, got %q", opts.GitPath)
	}
	if opts.GitAuth != nil && opts.GitAuth.SSHKeyPath != "" {
		if _, err := os.Stat(opts.GitAuth.SSHKeyPath); err != nil {
			return fmt.Errorf("ssh key: %w", err)
		}
	}
	if _, err := exec.LookPath(e.git); err != nil {
		return fmt.Errorf("gitops extraction needs the git CLI: %w", err)
	}
	return nil
}

// Extract clones the repository and streams the manifests found in it.
func (e *GitOpsExtractor) Extract(ctx context.Context, opts Options) (<-chan *types.ExtractedResource, <-chan error) {
	resources := make(chan *types.ExtractedResource, 100)
	errs := make(chan error, 10)

	go func() {
		defer close(resources)
		defer close(errs)

		dir, err := os.MkdirTemp("", "dhg-gitops-")
		if err != nil {
			errs <- err
			return
		}
		defer os.RemoveAll(dir)

		if err := e.clone(ctx, opts, dir); err != nil {
			errs <- err
			return
		}

		root := filepath.Join(dir, filepath.FromSlash(opts.GitPath))
		if _, err := os.Stat(root); err != nil {
			errs <- fmt.Errorf("path %q not found in %s: %w", opts.GitPath, opts.GitURL, err)
			return
		}

		fileOpts := opts
		fileOpts.Paths = []string{root}
		fileOpts.Recursive = true
		fileRes, fileErrs := NewFileExtractor().Extract(ctx, fileOpts)
		ref := opts.GitURL
		if opts.GitBranch != "" {
			ref += "@" + opts.GitBranch
		}
		for fileRes != nil || fileErrs != nil {
			select {
			case r, ok := <-fileRes:
				if !ok {
					fileRes = nil
					continue
				}
				rel, _ := filepath.Rel(dir, r.SourcePath)
				r.Source = types.SourceGitOps
				r.SourcePath = ref + ":" + filepath.ToSlash(rel)
				resources <- r
			case err, ok := <-fileErrs:
				if !ok {
					fileErrs = nil
					continue
				}
				errs <- err
			}
		}
	}()

	return resources, errs
}

// clone makes a shallow, single-branch clone of opts.GitURL into dir.
func (e *GitOpsExtractor) clone(ctx context.Context, opts Options, dir string) error {
	args := []string{"clone", "--depth", "1", "--single-branch"}
	if opts.GitBranch != "" {
		args = append(args, "--branch", opts.GitBranch)
	}
	args = append(args, "--", opts.GitURL, dir)

	cmd := exec.CommandContext(ctx, e.git, args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if opts.GitAuth != nil && opts.GitAuth.SSHKeyPath != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %q -o IdentitiesOnly=yes", opts.GitAuth.SSHKeyPath))
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone %s failed: %w: %s", opts.GitURL, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
