package generator

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func init() {
	RegisterFeature(Feature{
		Name:        "deckhouse-logging",
		Description: "Generate Deckhouse PodLoggingConfig for workloads",
		Params: map[string]string{
			"destination": "",
			"multiline":   "",
		},
		Apply: applyDeckhouseLoggingFeature,
	})
}

func applyDeckhouseLoggingFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	destinations := fc.ListParam("destination")
	multiline := fc.Param("multiline")

	// Validate multiline parameter
	validMultiline := map[string]bool{
		"":              true,
		"None":          true,
		"General":       true,
		"Backslash":     true,
		"LogWithTime":   true,
		"MultilineJSON": true,
	}
	if !validMultiline[multiline] {
		return nil, fmt.Errorf("deckhouse-logging: unsupported multiline parser type %q, allowed: None, General, Backslash, LogWithTime, MultilineJSON", multiline)
	}

	// Validate destinations using DNS-1123 subdomain logic
	for _, dest := range destinations {
		if errs := validation.IsDNS1123Subdomain(dest); len(errs) > 0 {
			return nil, fmt.Errorf("deckhouse-logging: destination %q is invalid: %v", dest, strings.Join(errs, ", "))
		}
	}

	out := cloneChart(chart)
	workloads := make(map[string]map[string]interface{})

	for _, r := range opsChartResources(out, fc.Graph) {
		kind := r.Original.GVK.Kind

		validKind := false
		switch kind {
		case "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob":
			validKind = true
		default:
			// Check for argoproj.io Rollout
			if kind == "Rollout" && r.Original.GVK.Group == "argoproj.io" {
				validKind = true
			}
		}

		if !validKind {
			continue
		}

		// Check if already covered by an incoming PodLoggingConfig
		covered := false
		for _, rel := range fc.Graph.GetRelationshipsTo(r.Original.ResourceKey()) {
			if rel.Type == types.RelationLabelSelector && rel.From.GVK.Kind == "PodLoggingConfig" && rel.From.Namespace == r.Original.ResourceKey().Namespace {
				covered = true
				break
			}
		}

		if covered {
			out.Notes = strings.TrimSpace(out.Notes + fmt.Sprintf("\ndeckhouse-logging: %s/%s skipped: covered by existing PodLoggingConfig", kind, r.Original.ResourceKey().Name))
			continue
		}

		// Extract selector
		var matchLabels map[string]interface{}

		if kind == "CronJob" {
			// First try spec.jobTemplate.spec.selector.matchLabels
			if sel, ok := nestedStringMapValue(r.Original.Object.Object, "spec", "jobTemplate", "spec", "selector", "matchLabels"); ok {
				matchLabels = sel
			} else if sel, ok := nestedStringMapValue(r.Original.Object.Object, "spec", "jobTemplate", "spec", "template", "metadata", "labels"); ok {
				matchLabels = sel
			}
		} else {
			if sel, ok := nestedStringMapValue(r.Original.Object.Object, "spec", "selector", "matchLabels"); ok {
				matchLabels = sel
			} else if kind == "Job" {
				if sel, ok := nestedStringMapValue(r.Original.Object.Object, "spec", "template", "metadata", "labels"); ok {
					matchLabels = sel
				}
			}
		}

		if len(matchLabels) == 0 {
			out.Notes = strings.TrimSpace(out.Notes + fmt.Sprintf("\ndeckhouse-logging: %s/%s skipped: no matchLabels selector", kind, r.Original.ResourceKey().Name))
			continue
		}

		workloads[r.Original.ResourceKey().Name] = map[string]interface{}{
			"matchLabels": matchLabels,
		}
	}

	if len(workloads) == 0 {
		return out, nil
	}

	if len(destinations) == 0 {
		out.Notes = strings.TrimSpace(out.Notes + "\n\n" + "deckhouse-logging: set logging.destinations to the ClusterLogDestination names given by the platform team; no PodLoggingConfig is rendered until then.")
	}

	loggingValues := map[string]interface{}{
		"enabled":         true,
		"destinations":    destinations,
		"labelSelector":   map[string]interface{}{},
		"multilineParser": multiline,
		"workloads":       workloads,
	}

	newValues, err := appendTopLevelValues(out.ValuesYAML, "logging", loggingValues)
	if err != nil {
		return nil, err
	}
	out.ValuesYAML = newValues

	// Create template
	helperPrefix := chartHelperPrefix(out)
	labelsBlock := ""
	if chartHasHelper(out, helperPrefix+".labels") {
		labelsBlock = fmt.Sprintf(`
  labels:
    {{- include "%s.labels" $ | nindent 4 }}`, helperPrefix)
	}

	template := fmt.Sprintf(`{{- if and .Values.logging.enabled .Values.logging.destinations }}
{{- range $name, $w := .Values.logging.workloads }}
---
apiVersion: deckhouse.io/v1alpha1
kind: PodLoggingConfig
metadata:
  name: {{ $name }}
  namespace: {{ $.Release.Namespace }}%s
spec:
  clusterDestinationRefs:
    {{- toYaml $.Values.logging.destinations | nindent 4 }}
  labelSelector:
    matchLabels:
      {{- toYaml $w.matchLabels | nindent 6 }}
  {{- with $.Values.logging.multilineParser }}
  multilineParser:
    type: {{ . }}
  {{- end }}
{{- end }}
{{- end }}
`, labelsBlock)

	out.Templates["templates/pod-logging-config.yaml"] = template

	return out, nil
}

// nestedStringMapValue extracts a map[string]interface{} that represents string values
// and is safe for YAML rendering
func nestedStringMapValue(obj map[string]interface{}, fields ...string) (map[string]interface{}, bool) {
	val, found, _ := unstructured.NestedMap(obj, fields...)
	if !found {
		return nil, false
	}

	result := make(map[string]interface{})
	for k, v := range val {
		if s, ok := v.(string); ok {
			result[k] = s
		} else {
			result[k] = fmt.Sprintf("%v", v)
		}
	}

	return result, true
}
