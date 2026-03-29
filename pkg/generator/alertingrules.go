package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// AlertingOptions configures basic PrometheusRule alerting generation.
type AlertingOptions struct {
	Namespace      string
	Severity       string
	RunbookURLBase string
}

// AlertingResult holds generated alerting rules.
type AlertingResult struct {
	// Rules maps rule name → YAML content.
	Rules     map[string]string
	RuleCount int
	NOTESTxt  string
}

var defaultAlertRules = []struct {
	name  string
	expr  string
	forDuration string
}{
	{"CrashLooping", `rate(kube_pod_container_status_restarts_total[5m]) > 0`, "2m"},
	{"OOMKilled", `kube_pod_container_status_last_terminated_reason{reason="OOMKilled"} == 1`, "0m"},
	{"PodNotReady", `kube_pod_status_ready{condition="false"} == 1`, "5m"},
	{"HighRestartCount", `kube_pod_container_status_restarts_total > 5`, "0m"},
	{"ContainerWaiting", `kube_pod_container_status_waiting == 1`, "10m"},
}

// GenerateBasicAlerts generates basic PrometheusRule alerting rules for workloads in the graph.
func GenerateBasicAlerts(graph *types.ResourceGraph, opts AlertingOptions) *AlertingResult {
	result := &AlertingResult{
		Rules: make(map[string]string),
	}

	severity := opts.Severity
	if severity == "" {
		severity = "warning"
	}
	namespace := opts.Namespace
	if namespace == "" {
		namespace = "default"
	}
	runbookBase := opts.RunbookURLBase

	for _, rule := range defaultAlertRules {
		content := generatePrometheusRule(rule.name, rule.expr, rule.forDuration, severity, namespace, runbookBase)
		result.Rules[rule.name] = content
	}

	result.RuleCount = len(result.Rules)
	result.NOTESTxt = fmt.Sprintf("Generated %d PrometheusRule alerting rules for namespace %s. "+
		"Rules cover: CrashLooping, OOMKilled, PodNotReady, HighRestartCount, ContainerWaiting. "+
		"Prometheus/alerting integration required.", result.RuleCount, namespace)

	return result
}

func generatePrometheusRule(name, expr, forDuration, severity, namespace, runbookBase string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# PrometheusRule alert: %s\n", name))
	sb.WriteString(fmt.Sprintf("# namespace: %s\n", namespace))
	sb.WriteString(fmt.Sprintf("alert: %s\n", name))
	sb.WriteString(fmt.Sprintf("expr: %s\n", expr))
	sb.WriteString(fmt.Sprintf("for: %s\n", forDuration))
	sb.WriteString("labels:\n")
	sb.WriteString(fmt.Sprintf("  severity: %s\n", severity))
	sb.WriteString("annotations:\n")
	if runbookBase != "" {
		sb.WriteString(fmt.Sprintf("  runbook_url: %s/%s\n", runbookBase, name))
	}
	sb.WriteString(fmt.Sprintf("  summary: Alert %s fired in namespace %s\n", name, namespace))
	return sb.String()
}

// InjectAlertingRules injects PrometheusRule templates into a chart.
// Returns the updated chart (copy-on-write) and the count of new templates added.
// Idempotent: already-present alert templates are not re-added.
func InjectAlertingRules(chart *types.GeneratedChart, result *AlertingResult) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}

	newChart := copyChartTemplates(chart)
	count := 0

	if result == nil {
		return newChart, 0
	}

	for ruleName, content := range result.Rules {
		path := fmt.Sprintf("templates/alert-%s.yaml", strings.ToLower(ruleName))
		if _, exists := newChart.Templates[path]; exists {
			continue
		}
		newChart.Templates[path] = content
		count++
	}

	return newChart, count
}
