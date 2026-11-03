package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	genOpts.OnShared = nil // reported once, with the base chart
	genOpts.OnSharedNote = nil

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
		missing := missingSecrets(p.extracted, variants[profile])
		for _, chart := range charts {
			overlay := valuesOverlay(base[chart], values[chart])
			if dropped := dropMissingSecrets(overlay, base[chart], missing); len(dropped) > 0 {
				note := fmt.Sprintf("profile %s: Secret %s is built only for this profile and is not in the base chart, so %s/values-profile-%s.yaml does not reference it; create the Secret in the cluster (keys: %s) and add the reference back (envFrom, env, volume or imagePullSecrets), or move the credential into the default configuration",
					profile, strings.Join(dropped, ", "), chart, profileFileName.ReplaceAllString(profile, "-"), missingKeys(variants[profile], dropped))
				p.notes = append(p.notes, note)
				fmt.Fprintf(os.Stderr, "Note: %s\n", note)
			}
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

// missingSecrets returns, by name, the Secrets the profile's manifests have
// and the base manifests lack: the base chart has no template for them.
func missingSecrets(base, variant []*types.ExtractedResource) map[string]bool {
	have := map[string]bool{}
	for _, r := range base {
		if r.Object.GetKind() == "Secret" {
			have[r.Object.GetName()] = true
		}
	}
	out := map[string]bool{}
	for _, r := range variant {
		if r.Object.GetKind() == "Secret" && !have[r.Object.GetName()] {
			out[r.Object.GetName()] = true
		}
	}
	return out
}

// missingKeys lists the data keys of the named Secrets of resources.
func missingKeys(resources []*types.ExtractedResource, names []string) string {
	var keys []string
	for _, r := range resources {
		if r.Object.GetKind() != "Secret" {
			continue
		}
		for _, n := range names {
			if r.Object.GetName() != n {
				continue
			}
			for _, field := range []string{"stringData", "data"} {
				m, _, _ := unstructured.NestedMap(r.Object.Object, field)
				for k := range m {
					keys = append(keys, k)
				}
			}
		}
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

// dropMissingSecrets removes from overlay the references (envFrom, env,
// volumes, imagePullSecrets) to the missing Secrets and the values of Secrets the base chart does not have
// (no template reads them). It returns the names of the Secrets it found
// referenced, sorted.
func dropMissingSecrets(overlay, base map[string]interface{}, missing map[string]bool) []string {
	if len(missing) == 0 {
		return nil
	}
	found := map[string]bool{}
	volumes := map[string]bool{}
	dropSecretRefs(overlay, missing, found, volumes)
	if len(found) == 0 {
		return nil
	}
	if len(volumes) > 0 {
		dropVolumeMounts(overlay, volumes)
	}
	services, _ := overlay["services"].(map[string]interface{})
	baseServices, _ := base["services"].(map[string]interface{})
	for svc, v := range services {
		sm, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		secrets, ok := sm["secrets"].(map[string]interface{})
		if !ok {
			continue
		}
		baseSvc, _ := baseServices[svc].(map[string]interface{})
		baseSecrets, _ := baseSvc["secrets"].(map[string]interface{})
		for key := range secrets {
			if _, inBase := baseSecrets[key]; !inBase {
				delete(secrets, key)
			}
		}
		if len(secrets) == 0 {
			delete(sm, "secrets")
		}
	}
	names := make([]string, 0, len(found))
	for n := range found {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// secretRefName returns the name of the Secret a list entry of the field
// references, or "". The shapes are those of the pod spec: envFrom[].secretRef,
// env[].valueFrom.secretKeyRef, volumes[].secret.secretName and
// imagePullSecrets[].
func secretRefName(field string, entry map[string]interface{}) string {
	var name string
	switch field {
	case "envFrom":
		ref, _ := entry["secretRef"].(map[string]interface{})
		name, _ = ref["name"].(string)
	case "env":
		from, _ := entry["valueFrom"].(map[string]interface{})
		ref, _ := from["secretKeyRef"].(map[string]interface{})
		name, _ = ref["name"].(string)
	case "volumes":
		sec, _ := entry["secret"].(map[string]interface{})
		name, _ = sec["secretName"].(string)
	case "imagePullSecrets":
		name, _ = entry["name"].(string)
	}
	return name
}

// dropSecretRefs removes the entries that reference a missing Secret from
// every envFrom, env, volumes and imagePullSecrets list under v. The names
// of removed volumes are added to volumes, so that their mounts can follow
// (dropVolumeMounts).
func dropSecretRefs(v interface{}, missing, found, volumes map[string]bool) {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, child := range t {
			list, isList := child.([]interface{})
			switch k {
			case "envFrom", "env", "volumes", "imagePullSecrets":
			default:
				isList = false
			}
			if !isList {
				dropSecretRefs(child, missing, found, volumes)
				continue
			}
			kept := make([]interface{}, 0, len(list))
			for _, e := range list {
				entry, _ := e.(map[string]interface{})
				if name := secretRefName(k, entry); name != "" && missing[name] {
					found[name] = true
					if k == "volumes" {
						if vol, _ := entry["name"].(string); vol != "" {
							volumes[vol] = true
						}
					}
					continue
				}
				dropSecretRefs(e, missing, found, volumes)
				kept = append(kept, e)
			}
			if len(kept) == 0 {
				delete(t, k)
			} else {
				t[k] = kept
			}
		}
	case []interface{}:
		for _, child := range t {
			dropSecretRefs(child, missing, found, volumes)
		}
	}
}

// dropVolumeMounts removes the volumeMounts of the named volumes from every
// volumeMounts list under v.
func dropVolumeMounts(v interface{}, volumes map[string]bool) {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, child := range t {
			list, isList := child.([]interface{})
			if k != "volumeMounts" || !isList {
				dropVolumeMounts(child, volumes)
				continue
			}
			kept := make([]interface{}, 0, len(list))
			for _, e := range list {
				entry, _ := e.(map[string]interface{})
				if name, _ := entry["name"].(string); volumes[name] {
					continue
				}
				kept = append(kept, e)
			}
			if len(kept) == 0 {
				delete(t, k)
			} else {
				t[k] = kept
			}
		}
	case []interface{}:
		for _, child := range t {
			dropVolumeMounts(child, volumes)
		}
	}
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
