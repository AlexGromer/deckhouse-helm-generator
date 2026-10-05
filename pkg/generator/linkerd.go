package generator

import (
	"fmt"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// Linkerd integration (`dhg generate --with linkerd`).
//
// The feature adds the pod annotations from `.Values.linkerd.podAnnotations`
// (by default `linkerd.io/inject: enabled`) to every Deployment, StatefulSet
// and DaemonSet pod template, guarded by `.Values.linkerd.enabled`. Users can
// add further `config.linkerd.io/*` annotations in values. Jobs and CronJobs
// are left alone: a meshed proxy keeps their pods from completing.
//
// ServiceProfiles and SMI TrafficSplits are deliberately not generated:
// ServiceProfiles need per-route definitions that cannot be derived from
// manifests, and SMI TrafficSplit is deprecated in Linkerd.

// linkerdInjectModes are the accepted values of the linkerd.io/inject annotation.
var linkerdInjectModes = map[string]bool{"enabled": true, "ingress": true, "disabled": true}

func applyLinkerdFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	mode := fc.Param("inject")
	if !linkerdInjectModes[mode] {
		return nil, fmt.Errorf("inject must be one of enabled, ingress, disabled; got %q", mode)
	}

	const cond = "and $.Values.linkerd.enabled $.Values.linkerd.podAnnotations"
	out := cloneChart(chart)
	changed := false
	for _, rt := range resourceTemplates(chart, func(k string) bool { return podWorkloadKinds[k] }) {
		if strings.Contains(chart.Templates[rt.path], "$.Values.linkerd.") {
			continue
		}
		ok := rt.injectAnnotations(rt.podTemplateMetadata(), cond, func(indent int) []string {
			pad := strings.Repeat(" ", indent)
			return []string{
				pad + "{{- if " + cond + " }}",
				fmt.Sprintf("%s{{- toYaml $.Values.linkerd.podAnnotations | nindent %d }}", pad, indent),
				pad + "{{- end }}",
			}
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
	err := addFeatureValues(out, "linkerd", map[string]interface{}{
		"enabled":        true,
		"podAnnotations": map[string]interface{}{"linkerd.io/inject": mode},
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
