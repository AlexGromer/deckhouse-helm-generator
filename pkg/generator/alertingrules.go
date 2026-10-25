package generator

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// Prometheus alerting and SLO rules (`dhg generate --with prometheus-rules`).
// ...

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

func promLabelRef(label string) string {
	return `{{ "{{" }} $labels.` + label + ` {{ "}}" }}`
}

func buildPrometheusRulesTemplate(prefix string, withLabelsHelper bool, names []string, isObservabilityKind bool) string {
	fullname := fmt.Sprintf(`include "%s.fullname" $`, prefix)
	var b strings.Builder
	w := func(format string, args ...interface{}) { fmt.Fprintf(&b, format+"\n", args...) }

	w("{{- if .Values.prometheusRules.enabled }}")
	w("{{- $cfg := .Values.prometheusRules }}")
	w("{{- $pods := list %s }}", strings.Join(names, " "))
	w(`{{- $sel := printf "namespace=%%q, pod=~%%q" .Release.Namespace (printf "(%%s)-.+" (join "|" $pods)) }}`)
	w(`{{- $matcher := printf "{%%s}" $sel }}`)

	writeMetadata := func(nameSuffix string) {
		if isObservabilityKind {
			w("apiVersion: observability.deckhouse.io/v1alpha1")
			w("kind: ObservabilityMetricsRulesGroup")
		} else {
			w("apiVersion: monitoring.coreos.com/v1")
			w("kind: PrometheusRule")
		}
		w("metadata:")
		if isObservabilityKind {
			w("  name: {{ %s }}-%s", fullname, nameSuffix)
		} else {
			w("  name: {{ %s }}-rules", fullname)
		}
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
	}

	writeMetadata("workloads")

	if !isObservabilityKind {
		w("  groups:")
		w("    - name: {{ %s }}.workloads", fullname)
		w("      rules:")
	} else {
		w("  rules:")
	}

	ident := "      "
	if isObservabilityKind {
		ident = "  "
	}

	for _, a := range workloadAlerts {
		desc := a.description
		for _, l := range []string{"container", "namespace", "pod"} {
			desc = strings.ReplaceAll(desc, "{{"+l+"}}", promLabelRef(l))
		}
		w("%s  - alert: %s", ident, a.name)
		w("%s    expr: |-", ident)
		expr := strings.ReplaceAll(a.expr, "{SEL}", "{{ $matcher }}")
		w("%s      %s", ident, strings.ReplaceAll(expr, "SEL", "{{ $sel }}"))
		w("%s    for: %s", ident, a.forDuration)
		w("%s    labels:", ident)
		w("%s      severity: {{ $cfg.severity | default \"warning\" }}", ident)
		w("%s    annotations:", ident)
		w("%s      summary: %s", ident, a.summary)
		w("%s      description: '%s'", ident, desc)
		w("%s      {{- with $cfg.runbookUrl }}", ident)
		w("%s      runbook_url: {{ printf \"%%s/%s\" (trimSuffix \"/\" .) }}", ident, a.name)
		w("%s      {{- end }}", ident)
	}

	w("    {{- with $cfg.slo }}")
	w("    {{- if .enabled }}")
	w("    {{- $slo := %s }}", fullname)
	w(`    {{- $errors := printf "%%s{%%s, %%s}" .requestsMetric $sel .errorsSelector }}`)
	w(`    {{- $total := printf "%%s{%%s}" .requestsMetric $sel }}`)
	w(`    {{- $budget := printf "(1 - %%v / 100)" .availability }}`)

	if isObservabilityKind {
		w("---")
		writeMetadata("slo")
		w("  rules:")
	} else {
		w("    - name: {{ $slo }}.slo")
		w("      rules:")
	}

	w(`        {{- range $window := list "5m" "30m" "1h" "2h" "6h" "1d" "3d" }}`)
	w("%s  - record: slo:sli_error:ratio_rate{{ $window }}", ident)
	w("%s    expr: |-", ident)
	w("%s      sum(rate({{ $errors }}[{{ $window }}])) / sum(rate({{ $total }}[{{ $window }}]))", ident)
	w("%s    labels:", ident)
	w("%s      slo: {{ $slo }}", ident)
	w("        {{- end }}")

	burn := func(long, short, factor string) string {
		return fmt.Sprintf(`(slo:sli_error:ratio_rate%[1]s{slo="{{ $slo }}"} > (%[3]s * {{ $budget }}) and slo:sli_error:ratio_rate%[2]s{slo="{{ $slo }}"} > (%[3]s * {{ $budget }}))`, long, short, factor)
	}
	for _, alert := range []struct{ name, severity, expr, forDuration, summary string }{
		{"SLOErrorBudgetBurnFast", "critical", burn("1h", "5m", "14.4") + " or " + burn("6h", "30m", "6"), "2m", "is burning fast"},
		{"SLOErrorBudgetBurnSlow", "warning", burn("1d", "2h", "3") + " or " + burn("3d", "6h", "1"), "1h", "is burning steadily"},
	} {
		w("%s  - alert: %s", ident, alert.name)
		w("%s    expr: |-", ident)
		w("%s      %s", ident, alert.expr)
		w("%s    for: %s", ident, alert.forDuration)
		w("%s    labels:", ident)
		w("%s      severity: %s", ident, alert.severity)
		w("%s      slo: {{ $slo }}", ident)
		w("%s    annotations:", ident)
		w("%s      summary: Error budget of {{ $slo }} %s.", ident, alert.summary)
		w("%s      description: 'The error ratio of {{ $slo }} threatens its {{ .availability }}%% availability objective.'", ident)
	}

	w("    {{- end }}")
	w("    {{- end }}")
	w("{{- end }}")
	return b.String()
}

func applyPrometheusRulesFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	kind := fc.Param("kind")
	if kind != "PrometheusRule" && kind != "ObservabilityMetricsRulesGroup" {
		return nil, fmt.Errorf("prometheus-rules: kind must be PrometheusRule or ObservabilityMetricsRulesGroup")
	}

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

	labelsMap, err := fc.MapParam("labels")
	if err != nil {
		return nil, fmt.Errorf("prometheus-rules: %w", err)
	}

	prom := fc.Param("prometheus")
	if prom != "main" || labelsMap["prometheus"] == "main" {
		labelsMap["prometheus"] = prom
	}

	labelsIface := map[string]interface{}{}
	for k, v := range labelsMap {
		labelsIface[k] = v
	}

	workloads := resourceTemplates(chart, isKind("Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob"))
	names := nameExpressions(workloads)
	prefix := chartHelperPrefix(chart)
	if len(names) == 0 || prefix == "" {
		return chart, nil
	}

	out := cloneChart(chart)
	content := buildPrometheusRulesTemplate(prefix, chartHasHelper(chart, prefix+".labels"), names, kind == "ObservabilityMetricsRulesGroup")
	if err := addTemplate(out, "templates/prometheus-rules.yaml", content); err != nil {
		return nil, err
	}
	err = addFeatureValues(out, "prometheusRules", map[string]interface{}{
		"enabled":    true,
		"labels":     labelsIface,
		"severity":   fc.Param("severity"),
		"runbookUrl": fc.Param("runbook-url"),
		"slo":        slo,
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
