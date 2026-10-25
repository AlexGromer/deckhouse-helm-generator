package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"

	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/extractor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/generator"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// writeProfileValues writes, for each configuration profile of a
// synthesizing source, values-profile-<profile>.yaml next to the values.yaml
// of every chart the profile changes. The default configuration and each
// profile are generated the same way, without --with features, so the
// difference holds only what the profile changes.
func writeProfileValues(ctx context.Context, p *pipelineResult, popts pipelineOptions, gen generator.Generator, genOpts generator.Options, outputDir string) ([]string, error) {
	v, ok := p.synthesis.(extractor.Varianter)
	if !ok || len(v.Variants()) == 0 {
		return nil, nil
	}
	variants := v.Variants()
	popts.quiet, popts.verbose = true, false

	base, err := chartValues(ctx, popts, gen, genOpts, p.extracted)
	if err != nil {
		return nil, err
	}
	profiles := make([]string, 0, len(variants))
	for name := range variants {
		profiles = append(profiles, name)
	}
	sort.Strings(profiles)

	var written []string
	for _, profile := range profiles {
		values, err := chartValues(ctx, popts, gen, genOpts, variants[profile])
		if err != nil {
			return nil, fmt.Errorf("profile %s: %w", profile, err)
		}
		charts := make([]string, 0, len(values))
		for name := range values {
			charts = append(charts, name)
		}
		sort.Strings(charts)
		for _, chart := range charts {
			overlay := valuesOverlay(base[chart], values[chart])
			if len(overlay) == 0 {
				continue
			}
			data, err := yaml.Marshal(overlay)
			if err != nil {
				return nil, err
			}
			file := "values-profile-" + profileFileName.ReplaceAllString(profile, "-") + ".yaml"
			header := fmt.Sprintf("# What configuration profile %q changes in this chart.\n# helm install <release> %s -f %s/%s\n", profile, chart, chart, file)
			path := filepath.Join(outputDir, chart, file)
			if err := os.WriteFile(path, append([]byte(header), data...), 0o644); err != nil {
				return nil, err
			}
			written = append(written, path)
		}
	}
	return written, nil
}

var profileFileName = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

// chartValues generates the charts of resources and returns their parsed
// values by chart name.
func chartValues(ctx context.Context, popts pipelineOptions, gen generator.Generator, genOpts generator.Options, resources []*types.ExtractedResource) (map[string]map[string]interface{}, error) {
	result, err := processResources(ctx, popts, resources)
	if err != nil {
		return nil, err
	}
	genOpts.ExternalFileManager = result.externalFiles
	charts, err := gen.Generate(ctx, result.graph, genOpts)
	if err != nil {
		return nil, err
	}
	out := make(map[string]map[string]interface{}, len(charts))
	for _, c := range charts {
		values := map[string]interface{}{}
		if err := yaml.Unmarshal([]byte(c.ValuesYAML), &values); err != nil {
			return nil, fmt.Errorf("chart %s: values.yaml: %w", c.Name, err)
		}
		out[c.Name] = values
	}
	return out, nil
}

// valuesOverlay returns the values that turn base into variant when Helm
// merges them over base: changed and new keys, and null for removed keys
// (Helm deletes a key whose override is null). Lists are replaced whole,
// as Helm does.
func valuesOverlay(base, variant map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, vv := range variant {
		bv, ok := base[k]
		if !ok {
			out[k] = vv
			continue
		}
		bm, bIsMap := bv.(map[string]interface{})
		vm, vIsMap := vv.(map[string]interface{})
		if bIsMap && vIsMap {
			if sub := valuesOverlay(bm, vm); len(sub) > 0 {
				out[k] = sub
			}
			continue
		}
		if !reflect.DeepEqual(bv, vv) {
			out[k] = vv
		}
	}
	for k := range base {
		if _, ok := variant[k]; !ok {
			out[k] = nil
		}
	}
	return out
}
