package extractor

import (
	"context"
	"fmt"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// synthesized holds what a synthesizing extractor produced, for Reporter.
type synthesized struct {
	notes    synth.Notes
	inputs   []string
	variants map[string][]*types.ExtractedResource
}

// Notes implements Reporter.
func (s *synthesized) Notes() []string { return s.notes.Items() }

// Inputs implements Reporter.
func (s *synthesized) Inputs() []string { return append([]string(nil), s.inputs...) }

// Variants implements Varianter.
func (s *synthesized) Variants() map[string][]*types.ExtractedResource { return s.variants }

// appResources builds the manifests of apps, honouring the kind filters.
func appResources(source types.Source, sourcePath string, apps []synth.App, opts Options) ([]*types.ExtractedResource, error) {
	var out []*types.ExtractedResource
	seen := map[string]string{}
	for _, app := range apps {
		for _, obj := range synth.Manifests(app) {
			key := obj.GetKind() + "/" + obj.GetName()
			if other, dup := seen[key]; dup {
				return nil, fmt.Errorf("%s: %s is built for both %s and %s; rename one of them", sourcePath, key, other, app.Name)
			}
			seen[key] = app.Name
			if matchesKinds(obj.GetKind(), opts) {
				out = append(out, &types.ExtractedResource{Object: obj, Source: source, SourcePath: sourcePath, GVK: obj.GroupVersionKind()})
			}
		}
	}
	return out, nil
}

// emitApps sends the manifests of apps, honouring the kind filters.
func emitApps(ctx context.Context, source types.Source, sourcePath string, apps []synth.App, opts Options) (<-chan *types.ExtractedResource, <-chan error) {
	list, err := appResources(source, sourcePath, apps, opts)
	if err != nil {
		return failed(err)
	}
	resources := make(chan *types.ExtractedResource)
	errs := make(chan error, 1)
	go func() {
		defer close(resources)
		defer close(errs)
		for _, r := range list {
			select {
			case resources <- r:
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			}
		}
	}()
	return resources, errs
}

// failed returns channels carrying only err.
func failed(err error) (<-chan *types.ExtractedResource, <-chan error) {
	resources := make(chan *types.ExtractedResource)
	errs := make(chan error, 1)
	close(resources)
	errs <- err
	close(errs)
	return resources, errs
}
