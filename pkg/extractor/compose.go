package extractor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// ComposeExtractor builds the manifests of the services of docker-compose
// files.
type ComposeExtractor struct {
	synthesized
	// env looks up interpolation variables; os.LookupEnv by default.
	env func(string) (string, bool)
}

// NewComposeExtractor creates a compose extractor.
func NewComposeExtractor() *ComposeExtractor { return &ComposeExtractor{env: os.LookupEnv} }

// Source implements Extractor.
func (e *ComposeExtractor) Source() types.Source { return types.SourceCompose }

// Validate implements Extractor.
func (e *ComposeExtractor) Validate(_ context.Context, opts Options) error {
	if len(opts.Paths) == 0 {
		return errors.New("at least one compose file is required (-f docker-compose.yml)")
	}
	for _, p := range opts.Paths {
		info, err := os.Stat(p)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("%s is a directory; pass the compose file itself", p)
		}
	}
	return nil
}

// Extract implements Extractor.
func (e *ComposeExtractor) Extract(ctx context.Context, opts Options) (<-chan *types.ExtractedResource, <-chan error) {
	apps, err := synth.FromCompose(opts.Paths, e.env, &e.notes)
	if err != nil {
		return failed(err)
	}
	for _, p := range opts.Paths {
		e.inputs = append(e.inputs, "compose file `"+p+"`")
	}
	return emitApps(ctx, types.SourceCompose, strings.Join(opts.Paths, ","), apps, opts)
}
