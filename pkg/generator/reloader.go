package generator

import (
	"sort"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// Stakater Reloader integration (`dhg generate --with reloader`).
//
// Reloader watches ConfigMaps and Secrets and performs a rolling restart of
// the workloads that use them. The feature annotates the workload (not its
// pod template, which is where Reloader reads its annotations) with
// `reloader.stakater.com/auto: "true"`, guarded by `.Values.reloader.enabled`.

// ReloaderCandidate is a workload that references ConfigMaps or Secrets.
type ReloaderCandidate struct {
	Key        types.ResourceKey
	ConfigMaps []string
	Secrets    []string
}

// DetectReloaderCandidates returns the Deployments, StatefulSets and
// DaemonSets of the graph that reference a ConfigMap or Secret (volumes,
// projected volumes, env valueFrom, envFrom), sorted by resource key.
func DetectReloaderCandidates(graph *types.ResourceGraph) []ReloaderCandidate {
	if graph == nil {
		return nil
	}
	var out []ReloaderCandidate
	for key, r := range graph.Resources {
		if r == nil || r.Original == nil || r.Original.Object == nil || !podWorkloadKinds[r.Original.GVK.Kind] {
			continue
		}
		var cms, secrets []string
		for _, ref := range configReferences(r) {
			if ref.GVK.Kind == "ConfigMap" {
				cms = append(cms, ref.Name)
			} else {
				secrets = append(secrets, ref.Name)
			}
		}
		sort.Strings(cms)
		sort.Strings(secrets)
		if len(cms) > 0 || len(secrets) > 0 {
			out = append(out, ReloaderCandidate{Key: key, ConfigMaps: cms, Secrets: secrets})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key.String() < out[j].Key.String() })
	return out
}

func nestedMap(obj map[string]interface{}, path ...string) map[string]interface{} {
	cur := obj
	for _, p := range path {
		if cur == nil {
			return nil
		}
		cur, _ = cur[p].(map[string]interface{})
	}
	return cur
}

func asList(v interface{}) []interface{} {
	l, _ := v.([]interface{})
	return l
}


// applyReloaderFeature implements the `reloader` feature. By default only
// workloads that reference a ConfigMap or Secret are annotated; the
// all-workloads parameter annotates every Deployment/StatefulSet/DaemonSet.
func applyReloaderFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	all := fc.BoolParam("all-workloads")
	candidates := map[types.ResourceKey]bool{}
	for _, c := range DetectReloaderCandidates(fc.Graph) {
		candidates[c.Key] = true
	}

	out := cloneChart(chart)
	changed := false
	for _, rt := range resourceTemplates(chart, func(k string) bool { return podWorkloadKinds[k] }) {
		if !all {
			r := graphResourceFor(fc.Graph, rt)
			if r == nil || !candidates[r.Original.ResourceKey()] {
				continue
			}
		}
		ok := rt.injectAnnotations(rt.topLevel("metadata:"), "$.Values.reloader.enabled", func(indent int) []string {
			return annotationLines("$.Values.reloader.enabled", indent,
				[][2]string{{"reloader.stakater.com/auto", `"true"`}})
		})
		if !ok {
			continue
		}
		out.Templates[rt.path] = rt.render()
		changed = true
	}
	if !changed {
		return chart, nil
	}
	if err := addFeatureValues(out, "reloader", map[string]interface{}{"enabled": true}); err != nil {
		return nil, err
	}
	return out, nil
}
