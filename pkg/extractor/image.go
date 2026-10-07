package extractor

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth/registry"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// ImageExtractor builds the manifests of applications from their images'
// configuration, read from the registry (no layers are downloaded).
type ImageExtractor struct {
	synthesized
	// client is replaceable in tests.
	client *registry.Client
}

// NewImageExtractor creates an image extractor.
func NewImageExtractor() *ImageExtractor { return &ImageExtractor{} }

// Source implements Extractor.
func (e *ImageExtractor) Source() types.Source { return types.SourceImage }

// Validate implements Extractor.
func (e *ImageExtractor) Validate(_ context.Context, opts Options) error {
	if len(opts.Images) == 0 {
		return errors.New("at least one --image is required for the image source")
	}
	for _, img := range opts.Images {
		if _, err := registry.ParseReference(img); err != nil {
			return err
		}
	}
	return nil
}

// Extract implements Extractor.
func (e *ImageExtractor) Extract(ctx context.Context, opts Options) (<-chan *types.ExtractedResource, <-chan error) {
	client := e.client
	if client == nil {
		creds, helpers := registry.DockerConfigCredentials()
		if helpers {
			e.notes.Addf("docker config.json uses credential helpers, which dhg does not run; only its \"auths\" entries were used")
		}
		client = &registry.Client{Credentials: creds}
	}
	client.PlainHTTP = opts.InsecureRegistry
	client.Platform = opts.Platform

	var apps []synth.App
	for _, img := range opts.Images {
		ref, err := registry.ParseReference(img)
		if err != nil {
			return failed(err)
		}
		cfg, digest, err := client.Config(ctx, ref)
		if err != nil {
			return failed(err)
		}
		e.inputs = append(e.inputs, fmt.Sprintf("image `%s` (%s/%s, manifest %s)", ref.Name(), cfg.OS, cfg.Architecture, digest))
		apps = append(apps, synth.FromImage(ref, cfg, &e.notes))
	}
	return emitApps(ctx, types.SourceImage, strings.Join(opts.Images, ","), apps, opts)
}
