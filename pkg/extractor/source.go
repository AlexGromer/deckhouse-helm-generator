package extractor

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// SourceExtractor builds the manifests of an application from its project
// directory: Dockerfile and, for Spring Boot, build file and configuration.
type SourceExtractor struct {
	synthesized
}

// NewSourceExtractor creates a source extractor.
func NewSourceExtractor() *SourceExtractor { return &SourceExtractor{} }

// Source implements Extractor.
func (e *SourceExtractor) Source() types.Source { return types.SourceCode }

// Validate implements Extractor.
func (e *SourceExtractor) Validate(_ context.Context, opts Options) error {
	if len(opts.Paths) == 0 {
		return errors.New("at least one project directory is required (-f ./service)")
	}
	if len(opts.Images) > 0 && len(opts.Images) != len(opts.Paths) {
		return fmt.Errorf("give one --image per project directory (%d directories, %d images)", len(opts.Paths), len(opts.Images))
	}
	for _, p := range opts.Paths {
		info, err := os.Stat(p)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory; the source source reads a project directory", p)
		}
	}
	return nil
}

// Extract implements Extractor.
func (e *SourceExtractor) Extract(ctx context.Context, opts Options) (<-chan *types.ExtractedResource, <-chan error) {
	var apps []synth.App
	for i, dir := range opts.Paths {
		image := ""
		if i < len(opts.Images) {
			image = opts.Images[i]
		}
		app, err := synth.FromSource(dir, image, &e.notes)
		if err != nil {
			return failed(err)
		}
		e.inputs = append(e.inputs, "project directory `"+dir+"`")
		apps = append(apps, app)
	}
	return emitApps(ctx, types.SourceCode, dir0(opts.Paths), apps, opts)
}

func dir0(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}
