package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// RecordingRulesOptions configures Prometheus recording rules generation.
type RecordingRulesOptions struct {
	Namespace           string
	Quantiles           []float64
	AggregationInterval string
}

// RecordingRulesResult holds generated Prometheus recording rules.
type RecordingRulesResult struct {
	// Rules maps rule name → PrometheusRule YAML.
	Rules     map[string]string
	RuleCount int
	NOTESTxt  string
}

// GenerateRecordingRules generates Prometheus recording rules templates.
func GenerateRecordingRules(graph *types.ResourceGraph, opts RecordingRulesOptions) *RecordingRulesResult {
	result := &RecordingRulesResult{
		Rules: make(map[string]string),
	}

	if graph == nil || len(graph.Resources) == 0 {
		return result
	}

	quantiles := opts.Quantiles
	if len(quantiles) == 0 {
		quantiles = []float64{0.5, 0.95, 0.99}
	}
	interval := opts.AggregationInterval
	if interval == "" {
		interval = "5m"
	}
	ns := opts.Namespace
	if ns == "" {
		ns = "default"
	}

	for _, r := range graph.Resources {
		if r == nil || r.Original == nil {
			continue
		}
		if r.Original.GVK.Kind != "Deployment" {
			continue
		}
		name := r.Original.Object.GetName()

		var sb strings.Builder
		sb.WriteString("apiVersion: monitoring.coreos.com/v1\n")
		sb.WriteString("kind: PrometheusRule\n")
		sb.WriteString("metadata:\n")
		fmt.Fprintf(&sb, "  name: %s-recording-rules\n", name)
		fmt.Fprintf(&sb, "  namespace: %s\n", ns)
		sb.WriteString("spec:\n  groups:\n  - name: recording-rules\n")
		fmt.Fprintf(&sb, "    interval: %s\n", interval)
		sb.WriteString("    rules:\n")
		for _, q := range quantiles {
			labelName := quantileLabel(q)
			sb.WriteString("    - record: ")
			fmt.Fprintf(&sb, "job:http_request_duration_seconds:%s\n", labelName)
			sb.WriteString("      expr: ")
			fmt.Fprintf(&sb, "histogram_quantile(%v, sum(rate(http_request_duration_seconds_bucket{namespace=\"%s\"}[%s])) by (le))\n",
				q, ns, interval)
		}

		key := fmt.Sprintf("templates/recordingrules-%s.yaml", name)
		result.Rules[key] = sb.String()
	}

	result.RuleCount = len(result.Rules)
	result.NOTESTxt = fmt.Sprintf("Prometheus recording rules generated. %d rule files created.\n", result.RuleCount)
	return result
}

// InjectRecordingRules merges recording rules templates into an existing chart.
func InjectRecordingRules(chart *types.GeneratedChart, result *RecordingRulesResult) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}
	newChart := copyChartTemplates(chart)
	count := 0
	if result != nil {
		for k, v := range result.Rules {
			if _, exists := newChart.Templates[k]; !exists {
				newChart.Templates[k] = v
				count++
			}
		}
	}
	return newChart, count
}

// quantileLabel converts a quantile float to a label string (e.g. 0.5 → "p50").
func quantileLabel(q float64) string {
	switch q {
	case 0.5:
		return "p50"
	case 0.75:
		return "p75"
	case 0.9:
		return "p90"
	case 0.95:
		return "p95"
	case 0.99:
		return "p99"
	default:
		return strings.ReplaceAll(fmt.Sprintf("p%v", q*100), ".", "_")
	}
}
