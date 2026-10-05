package generator

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// Argo Rollouts progressive delivery (`dhg generate --with argo-rollouts`).
//
// For every generated Deployment the feature adds an Argo Rollout that
// references it through `spec.workloadRef` (the Rollout reuses the
// Deployment's pod template, so the chart keeps a single source of truth)
// with a canary strategy whose steps come from `.Values.argoRollouts.steps`.
// `.Values.argoRollouts.scaleDown` controls how Argo scales the referenced
// Deployment down (never, onsuccess, progressively).
//
// If an HPA targets the Deployment, point it at the Rollout instead
// (scaleTargetRef apiVersion argoproj.io/v1alpha1, kind Rollout).

// CanaryStep is one canary step: shift Weight percent of the pods to the new
// version, then pause. Pause is a duration ("1m"), "manual" for an indefinite
// pause awaiting promotion, or "" for no pause.
type CanaryStep struct {
	Weight int
	Pause  string
}

var rolloutScaleDownModes = map[string]bool{"never": true, "onsuccess": true, "progressively": true}

// ParseCanarySteps parses steps written as "weight[:pause],..." such as
// "20:1m,50:manual,80".
func ParseCanarySteps(spec string) ([]CanaryStep, error) {
	var steps []CanaryStep
	for _, item := range strings.Split(spec, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		weightStr, pause, _ := strings.Cut(item, ":")
		weight, err := strconv.Atoi(weightStr)
		if err != nil || weight < 1 || weight > 100 {
			return nil, fmt.Errorf("invalid canary step %q: weight must be an integer between 1 and 100", item)
		}
		if pause != "" && pause != "manual" && !reDuration.MatchString(pause) {
			return nil, fmt.Errorf("invalid canary step %q: pause must be a duration such as 1m, or manual", item)
		}
		steps = append(steps, CanaryStep{Weight: weight, Pause: pause})
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("at least one canary step is required")
	}
	return steps, nil
}

// canaryStepsValues converts steps into Rollout `steps` entries.
func canaryStepsValues(steps []CanaryStep) []interface{} {
	var out []interface{}
	for _, s := range steps {
		out = append(out, map[string]interface{}{"setWeight": s.Weight})
		switch s.Pause {
		case "":
		case "manual":
			out = append(out, map[string]interface{}{"pause": map[string]interface{}{}})
		default:
			out = append(out, map[string]interface{}{"pause": map[string]interface{}{"duration": s.Pause}})
		}
	}
	return out
}

// rolloutReplicas returns the replicas lines of a Deployment body (with their
// guard, e.g. `if not .autoscaling`), or nil when they are not recognised.
func rolloutReplicas(dep *resourceTemplate) []string {
	spec := dep.topLevel("spec:")
	sel := dep.child(spec, "  selector:")
	if spec < 0 || sel < 0 {
		return nil
	}
	region := dep.body[spec+1 : sel]
	found := false
	for _, l := range region {
		switch {
		case strings.HasPrefix(l, "  replicas: "):
			found = true
		case reOpener.MatchString(l) || reEnd.MatchString(l):
		default:
			return nil
		}
	}
	if !found || !balancedControl(region) {
		return nil
	}
	return append([]string(nil), region...)
}

// rolloutBody builds the Rollout document for one Deployment template.
func rolloutBody(dep *resourceTemplate) []string {
	b := []string{
		"apiVersion: argoproj.io/v1alpha1",
		"kind: Rollout",
		"metadata:",
		"  name: " + dep.name,
		"  namespace: {{ $.Release.Namespace }}",
	}
	b = append(b, dep.labels()...)
	b = append(b, "spec:")
	b = append(b, rolloutReplicas(dep)...)
	b = append(b, dep.selector()...)
	b = append(b,
		"  workloadRef:",
		"    apiVersion: apps/v1",
		// Quoted so that tools grepping for "kind: Deployment" do not take
		// this Rollout for a Deployment template.
		`    kind: "Deployment"`,
		"    name: "+dep.name,
		`    scaleDown: {{ $.Values.argoRollouts.scaleDown | default "progressively" }}`,
		"  strategy:",
		"    canary:",
		"      {{- with $.Values.argoRollouts.steps }}",
		"      steps:",
		"        {{- toYaml . | nindent 8 }}",
		"      {{- end }}",
	)
	return b
}

func applyArgoRolloutsFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	steps, err := ParseCanarySteps(fc.Param("steps"))
	if err != nil {
		return nil, err
	}
	scaleDown := fc.Param("scale-down")
	if !rolloutScaleDownModes[scaleDown] {
		return nil, fmt.Errorf("scale-down must be one of never, onsuccess, progressively; got %q", scaleDown)
	}

	out := cloneChart(chart)
	changed := false
	for _, dep := range resourceTemplates(chart, isKind("Deployment")) {
		selector, labels := dep.selector(), dep.labels()
		if selector == nil || !balancedControl(selector[1:]) || (labels != nil && !balancedControl(labels[1:])) {
			continue
		}
		path := featureTemplatePath("argo-rollout", dep.path)
		if err := addTemplate(out, path, dep.wrap("$.Values.argoRollouts.enabled", rolloutBody(dep))); err != nil {
			return nil, err
		}
		changed = true
	}
	if !changed {
		return chart, nil
	}
	err = addFeatureValues(out, "argoRollouts", map[string]interface{}{
		"enabled":   true,
		"scaleDown": scaleDown,
		"steps":     canaryStepsValues(steps),
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
