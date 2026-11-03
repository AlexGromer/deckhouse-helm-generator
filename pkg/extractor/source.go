package extractor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/synth/registry"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// SourceExtractor builds the manifests of an application from its project
// directory: Dockerfile and, for Spring Boot, Quarkus and Micronaut, build
// file and configuration. With Options.ImageConfig it also reads the
// configuration of the built image from the registry.
type SourceExtractor struct {
	synthesized
	// client is replaceable in tests.
	client *registry.Client
	// kube is replaceable in tests.
	kube *clusterClient
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
	if opts.ImageConfig && len(opts.Images) == 0 {
		return errors.New("--image-config reads the image given with --image; give one --image per project directory")
	}
	if len(opts.Images) > 0 && len(opts.Images) != len(opts.Paths) {
		return fmt.Errorf("give one --image per project directory (%d directories, %d images)", len(opts.Paths), len(opts.Images))
	}
	if err := validateSynthProfile(opts); err != nil {
		return err
	}
	if opts.KnownKeysFromCluster && opts.SynthProfile == "" {
		return errors.New("--known-keys-from-cluster reads the ConfigMaps named in envFrom of the platform profile; give one with --synth-profile")
	}
	if opts.KnownKeysFile != "" {
		if _, err := synth.LoadKnownKeys(opts.KnownKeysFile); err != nil {
			return fmt.Errorf("--known-keys: %w", err)
		}
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
	opts, err := e.withSynthProfile(opts)
	if err != nil {
		return failed(err)
	}
	var client *registry.Client
	var creds *registry.DockerCredentials
	if opts.ImageConfig {
		client, creds = registryClient(e.client, opts)
	}
	var fileKeys map[string][]string
	if opts.KnownKeysFile != "" {
		fileKeys, err = synth.LoadKnownKeys(opts.KnownKeysFile)
		if err != nil {
			return failed(fmt.Errorf("--known-keys: %w", err))
		}
		e.inputs = append(e.inputs, "known keys `"+opts.KnownKeysFile+"` (names only)")
	}
	cluster := &knownKeysCluster{opts: opts, ext: e}
	var projects []*synth.SourceProject
	images := make([]string, len(opts.Paths))
	profiles := map[string]bool{}
	for i, dir := range opts.Paths {
		if i < len(opts.Images) {
			images[i] = opts.Images[i]
		}
		s, err := synth.ReadSourceWith(dir, &e.notes, opts.profile)
		if err != nil {
			return failed(err)
		}
		input := "project directory `" + dir + "`"
		if s.Project != nil {
			input += " (" + string(s.Project.Framework) + ")"
		}
		e.inputs = append(e.inputs, input)
		if client != nil {
			ref, err := registry.ParseReference(images[i])
			if err != nil {
				return failed(err)
			}
			cfg, digest, err := fetchConfig(ctx, client, creds, ref)
			if err != nil {
				return failed(err)
			}
			s.ImageConfig = cfg
			e.inputs = append(e.inputs, fmt.Sprintf("image `%s` (%s/%s, manifest %s)", ref.Name(), cfg.OS, cfg.Architecture, digest))
		}
		clusterKeys, err := cluster.keysFor(ctx, s.Name())
		if err != nil {
			return failed(err)
		}
		s.KnownKeys = synth.MergeKnownKeys(fileKeys, clusterKeys)
		for _, p := range s.Profiles() {
			profiles[p] = true
		}
		projects = append(projects, s)
	}
	for _, w := range creds.Warnings() {
		e.notes.Addf("%s", w)
	}

	apps := make([]synth.App, len(projects))
	for i, s := range projects {
		apps[i] = s.App(images[i], "", &e.notes)
	}
	if len(profiles) > 0 {
		names := make([]string, 0, len(profiles))
		for p := range profiles {
			names = append(names, p)
		}
		sort.Strings(names)
		e.variants = map[string][]*types.ExtractedResource{}
		for _, p := range names {
			// The notes of a profile repeat those of the default configuration.
			var quiet synth.Notes
			variant := make([]synth.App, len(projects))
			for i, s := range projects {
				variant[i] = s.App(images[i], p, &quiet)
			}
			resources, err := appResources(types.SourceCode, dir0(opts.Paths), variant, opts)
			if err != nil {
				return failed(err)
			}
			e.variants[p] = resources
		}
		e.notes.Addf("configuration profiles %s: values-profile-<profile>.yaml next to values.yaml holds what each profile changes in the chart (helm install -f values-profile-<profile>.yaml); a profile that changes nothing dhg maps gets no file", strings.Join(names, ", "))
	}
	return emitApps(ctx, types.SourceCode, dir0(opts.Paths), apps, opts)
}

func dir0(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}

// knownKeysCluster reads the key names of the ConfigMaps of the profile's
// envFrom once per ConfigMap, for --known-keys-from-cluster.
type knownKeysCluster struct {
	opts Options
	ext  *SourceExtractor
	read map[string][]string
	ns   string
}

// keysFor returns the key names of the ConfigMaps the profile's envFrom
// names for the app; nil when the flag is off.
func (k *knownKeysCluster) keysFor(ctx context.Context, app string) (map[string][]string, error) {
	if !k.opts.KnownKeysFromCluster {
		return nil, nil
	}
	names, err := k.opts.profile.ConfigMapNames(app)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", app, err)
	}
	if len(names) == 0 {
		k.ext.notes.Addf("%s: --known-keys-from-cluster: the platform profile has no envFrom configMapRef, so there is no ConfigMap to read", app)
		return nil, nil
	}
	if k.read == nil {
		k.read = map[string][]string{}
		k.ns = knownKeysNamespace(k.opts)
	}
	var missing []string
	for _, n := range names {
		if _, ok := k.read[n]; !ok {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		client, err := k.ext.kubeClient(k.opts)
		if err != nil {
			return nil, fmt.Errorf("--known-keys-from-cluster: %w", err)
		}
		keys, notes, err := configMapKeys(ctx, client, k.ns, missing)
		if err != nil {
			return nil, err
		}
		for _, n := range missing {
			k.read[n] = keys[n] // nil for a ConfigMap that is not there
		}
		for _, note := range notes {
			k.ext.notes.Addf("%s", note)
		}
		k.ext.inputs = append(k.ext.inputs, fmt.Sprintf("ConfigMaps %s of namespace `%s` (key names only)", "`"+strings.Join(missing, "`, `")+"`", k.ns))
	}
	out := map[string][]string{}
	for _, n := range names {
		if len(k.read[n]) > 0 {
			out[n] = k.read[n]
		}
	}
	return out, nil
}
