package generator

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// Prometheus alerting and SLO rules (`dhg generate --with prometheus-rules`).
//
// The feature adds one prometheus-operator PrometheusRule per chart with:
//   - workload alerts based on kube-state-metrics (crash loops, OOM kills,
//     frequent restarts, pods not running, containers stuck waiting), scoped
//     to the chart's pods by namespace and pod-name prefix;
//   - optionally (`.Values.prometheusRules.slo.enabled`) multi-window,
//     multi-burn-rate SLO alerts, with the error-ratio recording rules they
//     are computed from, for an availability objective over a request
//     counter exposed by the application (e.g. http_requests_total).

// workloadAlert is one kube-state-metrics based alert. SEL in expr is
// replaced by the chart's pod selector; {{label}} in description by a
// Prometheus label reference.
type workloadAlert struct {
	name, expr, forDuration, summary, description string
}

var workloadAlerts = []workloadAlert{
	{
		name:        "PodCrashLooping",
		expr:        `max_over_time(kube_pod_container_status_waiting_reason{reason="CrashLoopBackOff", SEL}[5m]) >= 1`,
		forDuration: "15m",
		summary:     "Container is crash looping.",
		description: "Container {{container}} of pod {{namespace}}/{{pod}} is in CrashLoopBackOff.",
	},
	{
		name: "ContainerOOMKilled",
		expr: `(kube_pod_container_status_restarts_total{SEL} - kube_pod_container_status_restarts_total{SEL} offset 10m >= 1)` +
			` and ignoring (reason) min_over_time(kube_pod_container_status_last_terminated_reason{reason="OOMKilled", SEL}[10m]) == 1`,
		forDuration: "0m",
		summary:     "Container was OOM killed.",
		description: "Container {{container}} of pod {{namespace}}/{{pod}} was killed for exceeding its memory limit.",
	},
	{
		name:        "PodFrequentlyRestarting",
		expr:        `increase(kube_pod_container_status_restarts_total{SEL}[1h]) > 5`,
		forDuration: "0m",
		summary:     "Container restarts frequently.",
		description: "Container {{container}} of pod {{namespace}}/{{pod}} restarted more than 5 times in the last hour.",
	},
	{
		name:        "PodNotReady",
		expr:        `sum by (namespace, pod) (max by (namespace, pod) (kube_pod_status_phase{phase=~"Pending|Unknown|Failed", SEL})) > 0`,
		forDuration: "15m",
		summary:     "Pod has not been running for 15 minutes.",
		description: "Pod {{namespace}}/{{pod}} has been in a non-running state for longer than 15 minutes.",
	},
	{
		name:        "ContainerWaiting",
		expr:        `sum by (namespace, pod, container) (kube_pod_container_status_waiting_reason{reason!="CrashLoopBackOff", SEL}) > 0`,
		forDuration: "1h",
		summary:     "Container has been waiting for an hour.",
		description: "Container {{container}} of pod {{namespace}}/{{pod}} has been waiting for longer than 1 hour.",
	},
}

// promLabelRef renders a Prometheus template reference ({{ $labels.x }})
// escaped so that Helm leaves it alone.
func promLabelRef(label string) string {
	return `{{ "{{" }} $labels.` + label + ` {{ "}}" }}`
}

// buildPrometheusRulesTemplate renders the PrometheusRule template for a
// chart whose helpers are named prefix and whose workloads have the given
// name expressions.
func buildPrometheusRulesTemplate(prefix string, withLabelsHelper bool, names []string) string {
	fullname := fmt.Sprintf(`include "%s.fullname" $`, prefix)
	var b strings.Builder
	w := func(format string, args ...interface{}) { fmt.Fprintf(&b, format+"\n", args...) }

	w("{{- if .Values.prometheusRules.enabled }}")
	w("{{- $cfg := .Values.prometheusRules }}")
	w("{{- $pods := list %s }}", strings.Join(names, " "))
	w(`{{- $sel := printf "namespace=%%q, pod=~%%q" .Release.Namespace (printf "(%%s)-.+" (join "|" $pods)) }}`)
	w(`{{- $matcher := printf "{%%s}" $sel }}`)
	w("apiVersion: monitoring.coreos.com/v1")
	w("kind: PrometheusRule")
	w("metadata:")
	w("  name: {{ %s }}-rules", fullname)
	w("  namespace: {{ .Release.Namespace }}")
	if withLabelsHelper {
		w("  labels:")
		w(`    {{- include "%s.labels" $ | nindent 4 }}`, prefix)
		w("    {{- with $cfg.labels }}")
		w("    {{- toYaml . | nindent 4 }}")
		w("    {{- end }}")
	} else {
		w("  {{- with $cfg.labels }}")
		w("  labels:")
		w("    {{- toYaml . | nindent 4 }}")
		w("  {{- end }}")
	}
	w("spec:")
	w("  groups:")
	w("    - name: {{ %s }}.workloads", fullname)
	w("      rules:")
	for _, a := range workloadAlerts {
		desc := a.description
		for _, l := range []string{"container", "namespace", "pod"} {
			desc = strings.ReplaceAll(desc, "{{"+l+"}}", promLabelRef(l))
		}
		w("        - alert: %s", a.name)
		w("          expr: |-")
		expr := strings.ReplaceAll(a.expr, "{SEL}", "{{ $matcher }}")
		w("            %s", strings.ReplaceAll(expr, "SEL", "{{ $sel }}"))
		w("          for: %s", a.forDuration)
		w("          labels:")
		w("            severity: {{ $cfg.severity | default \"warning\" }}")
		w("          annotations:")
		w("            summary: %s", a.summary)
		w("            description: '%s'", desc)
		w("            {{- with $cfg.runbookUrl }}")
		w("            runbook_url: {{ printf \"%%s/%s\" (trimSuffix \"/\" .) }}", a.name)
		w("            {{- end }}")
	}

	// SLO: error-ratio recording rules and the multi-window, multi-burn-rate
	// alerts of the Google SRE workbook. Fast burn (page): 14.4x over 1h or 6x
	// over 6h, i.e. 2% or 5% of a 30-day budget; slow burn (ticket): 3x over 1d
	// or 1x over 3d, i.e. 10% of the budget. Each long window is confirmed by a
	// short one so alerts stop soon after the burn does.
	w("    {{- with $cfg.slo }}")
	w("    {{- if .enabled }}")
	w("    {{- $slo := %s }}", fullname)
	w(`    {{- $errors := printf "%%s{%%s, %%s}" .requestsMetric $sel .errorsSelector }}`)
	w(`    {{- $total := printf "%%s{%%s}" .requestsMetric $sel }}`)
	w(`    {{- $budget := printf "(1 - %%v / 100)" .availability }}`)
	w("    - name: {{ $slo }}.slo")
	w("      rules:")
	w(`        {{- range $window := list "5m" "30m" "1h" "2h" "6h" "1d" "3d" }}`)
	w("        - record: slo:sli_error:ratio_rate{{ $window }}")
	w("          expr: |-")
	w("            sum(rate({{ $errors }}[{{ $window }}])) / sum(rate({{ $total }}[{{ $window }}]))")
	w("          labels:")
	w("            slo: {{ $slo }}")
	w("        {{- end }}")
	burn := func(long, short, factor string) string {
		return fmt.Sprintf(`(slo:sli_error:ratio_rate%[1]s{slo="{{ $slo }}"} > (%[3]s * {{ $budget }}) and slo:sli_error:ratio_rate%[2]s{slo="{{ $slo }}"} > (%[3]s * {{ $budget }}))`, long, short, factor)
	}
	for _, alert := range []struct{ name, severity, expr, forDuration, summary string }{
		{"SLOErrorBudgetBurnFast", "critical", burn("1h", "5m", "14.4") + " or " + burn("6h", "30m", "6"), "2m", "is burning fast"},
		{"SLOErrorBudgetBurnSlow", "warning", burn("1d", "2h", "3") + " or " + burn("3d", "6h", "1"), "1h", "is burning steadily"},
	} {
		w("        - alert: %s", alert.name)
		w("          expr: |-")
		w("            %s", alert.expr)
		w("          for: %s", alert.forDuration)
		w("          labels:")
		w("            severity: %s", alert.severity)
		w("            slo: {{ $slo }}")
		w("          annotations:")
		w("            summary: Error budget of {{ $slo }} %s.", alert.summary)
		w("            description: 'The error ratio of {{ $slo }} threatens its {{ .availability }}%% availability objective.'")
	}
	w("    {{- end }}")
	w("    {{- end }}")
	w("{{- end }}")
	return b.String()
}

func applyPrometheusRulesFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	slo := map[string]interface{}{
		"enabled":        false,
		"availability":   99.9,
		"requestsMetric": fc.Param("slo-requests-metric"),
		"errorsSelector": fc.Param("slo-errors-selector"),
	}
	if v := fc.Param("slo-availability"); v != "" {
		availability, err := strconv.ParseFloat(v, 64)
		if err != nil || availability <= 0 || availability >= 100 {
			return nil, fmt.Errorf("slo-availability must be a percentage between 0 and 100 (exclusive); got %q", v)
		}
		slo["enabled"] = true
		slo["availability"] = availability
	}
	if slo["requestsMetric"] == "" {
		return nil, fmt.Errorf("slo-requests-metric must not be empty")
	}

	workloads := resourceTemplates(chart, isKind("Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob"))
	names := nameExpressions(workloads)
	prefix := chartHelperPrefix(chart)
	if len(names) == 0 || prefix == "" {
		return chart, nil
	}

	out := cloneChart(chart)
	content := buildPrometheusRulesTemplate(prefix, chartHasHelper(chart, prefix+".labels"), names)
	if err := addTemplate(out, "templates/prometheus-rules.yaml", content); err != nil {
		return nil, err
	}
	err := addFeatureValues(out, "prometheusRules", map[string]interface{}{
		"enabled":    true,
		"labels":     map[string]interface{}{},
		"severity":   fc.Param("severity"),
		"runbookUrl": fc.Param("runbook-url"),
		"slo":        slo,
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
