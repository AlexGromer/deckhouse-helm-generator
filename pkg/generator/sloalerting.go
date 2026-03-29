package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// SLOOptions configures SLO-based alerting generation.
type SLOOptions struct {
	AvailabilitySLO    float64
	LatencySLO         float64
	LatencyThresholdMs int
	BurnRateWindows    []string
}

// SLOResult holds generated SLO alerting templates.
type SLOResult struct {
	// ServiceLevels maps workload name → ServiceLevelObjective YAML.
	ServiceLevels map[string]string
	// BurnRateAlerts maps alert name → PrometheusRule YAML.
	BurnRateAlerts map[string]string
	// ErrorBudget is a description of the error budget calculation.
	ErrorBudget string
	NOTESTxt    string
}

// GenerateSLOConfig generates SLO configuration and burn rate alerting templates.
func GenerateSLOConfig(graph *types.ResourceGraph, opts SLOOptions) *SLOResult {
	result := &SLOResult{
		ServiceLevels:  make(map[string]string),
		BurnRateAlerts: make(map[string]string),
	}

	windows := opts.BurnRateWindows
	if len(windows) == 0 {
		windows = []string{"5m", "30m", "1h", "6h"}
	}

	// Error budget string
	errorRate := 100.0 - opts.AvailabilitySLO
	result.ErrorBudget = fmt.Sprintf("Error budget: %.4f%% (availability SLO %.1f%%). "+
		"Allowed error rate: %.4f. Budget exhaustion triggers burn rate alerts.",
		errorRate, opts.AvailabilitySLO, errorRate/100.0)

	if graph == nil || len(graph.Resources) == 0 {
		result.NOTESTxt = "SLO alerting templates generated.\n"
		return result
	}

	for _, r := range graph.Resources {
		if r == nil || r.Original == nil {
			continue
		}
		if r.Original.GVK.Kind != "Deployment" {
			continue
		}
		name := r.Original.Object.GetName()

		// ServiceLevel
		var slSB strings.Builder
		slSB.WriteString("apiVersion: monitoring.coreos.com/v1\n")
		slSB.WriteString("kind: PrometheusRule\n")
		slSB.WriteString("metadata:\n")
		fmt.Fprintf(&slSB, "  name: %s-slo\n", name)
		slSB.WriteString("spec:\n  groups:\n  - name: slo\n    rules:\n")
		fmt.Fprintf(&slSB, "    # availability SLO: %.1f%%\n", opts.AvailabilitySLO)
		fmt.Fprintf(&slSB, "    # availabilitySLO: %v\n", opts.AvailabilitySLO)
		if opts.LatencyThresholdMs > 0 {
			fmt.Fprintf(&slSB, "    # latencyThresholdMs: %d\n", opts.LatencyThresholdMs)
		}
		if opts.LatencySLO > 0 {
			fmt.Fprintf(&slSB, "    # latencySLO: %.1f\n", opts.LatencySLO)
		}
		slSB.WriteString("    - record: slo:availability\n")
		fmt.Fprintf(&slSB, "      expr: %.4f\n", opts.AvailabilitySLO/100.0)
		slSB.WriteString("spec:\n")
		fmt.Fprintf(&slSB, "  availabilitySLO: %v\n", opts.AvailabilitySLO)
		if opts.LatencyThresholdMs > 0 {
			fmt.Fprintf(&slSB, "  latencyThresholdMs: %d\n", opts.LatencyThresholdMs)
		}
		if opts.LatencySLO > 0 {
			fmt.Fprintf(&slSB, "  latencySLO: %.1f\n", opts.LatencySLO)
		}
		result.ServiceLevels[name] = slSB.String()

		// BurnRateAlerts for each window
		for _, w := range windows {
			var brSB strings.Builder
			brSB.WriteString("apiVersion: monitoring.coreos.com/v1\n")
			brSB.WriteString("kind: PrometheusRule\n")
			brSB.WriteString("metadata:\n")
			fmt.Fprintf(&brSB, "  name: %s-burn-rate-%s\n", name, w)
			brSB.WriteString("spec:\n  groups:\n  - name: burn-rate\n    rules:\n")
			fmt.Fprintf(&brSB, "    - alert: SLOBurnRate_%s_%s\n", name, w)
			fmt.Fprintf(&brSB, "      expr: burn_rate:ratio{window=\"%s\"} > 1\n", w)
			fmt.Fprintf(&brSB, "      for: %s\n", w)
			brSB.WriteString("      labels:\n        severity: warning\n")
			key := fmt.Sprintf("%s-burn-%s", name, w)
			result.BurnRateAlerts[key] = brSB.String()
		}
	}

	result.NOTESTxt = fmt.Sprintf("SLO alerting templates generated. %d service levels, %d burn rate alerts.\n",
		len(result.ServiceLevels), len(result.BurnRateAlerts))
	return result
}

// InjectSLOAlerts merges SLO alerting templates into an existing chart.
func InjectSLOAlerts(chart *types.GeneratedChart, result *SLOResult) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}
	newChart := copyChartTemplates(chart)
	count := 0
	if result != nil {
		for k, v := range result.ServiceLevels {
			key := fmt.Sprintf("templates/slo-sl-%s.yaml", k)
			if _, exists := newChart.Templates[key]; !exists {
				newChart.Templates[key] = v
				count++
			}
		}
		for k, v := range result.BurnRateAlerts {
			key := fmt.Sprintf("templates/slo-br-%s.yaml", k)
			if _, exists := newChart.Templates[key]; !exists {
				newChart.Templates[key] = v
				count++
			}
		}
	}
	return newChart, count
}
