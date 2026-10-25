package generator

import (
	"reflect"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestPrometheusRulesFeature(t *testing.T) {
	out := applyObsFeatures(t, []string{"prometheus-rules"}, nil)

	content, ok := out.Templates["templates/prometheus-rules.yaml"]
	if !ok {
		t.Fatalf("prometheus-rules template missing; templates: %v", keysOfMap(out.Templates))
	}
	for _, want := range []string{
		"{{- if .Values.prometheusRules.enabled }}",
		"apiVersion: monitoring.coreos.com/v1",
		"kind: PrometheusRule",
		// Every workload of the chart (sorted by template path) is selected by pod name.
		`{{- $pods := list "backup" "db" "web" }}`,
		`max_over_time(kube_pod_container_status_waiting_reason{reason="CrashLoopBackOff", {{ $sel }}}[5m]) >= 1`,
		`increase(kube_pod_container_status_restarts_total{{ $matcher }}[1h]) > 5`,
		// Prometheus label references are escaped from Helm.
		`{{ "{{" }} $labels.pod {{ "}}" }}`,
		"            severity: {{ $cfg.severity | default \"warning\" }}",
		"    {{- with $cfg.slo }}\n    {{- if .enabled }}",
		"        - record: slo:sli_error:ratio_rate{{ $window }}",
		"        - alert: SLOErrorBudgetBurnFast",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("template misses %q:\n%s", want, content)
		}
	}
	for _, alert := range workloadAlerts {
		if !strings.Contains(content, "- alert: "+alert.name+"\n") {
			t.Errorf("alert %s missing", alert.name)
		}
	}
	if strings.Contains(content, "{{{") {
		t.Error("template contains '{{{', which Helm cannot parse")
	}

	want := map[string]interface{}{
		"enabled":    true,
		"labels":     map[string]interface{}{"prometheus": "main", "component": "rules"},
		"severity":   "warning",
		"runbookUrl": "",
		"slo": map[string]interface{}{
			"enabled":        false,
			"availability":   99.9,
			"requestsMetric": "http_requests_total",
			"errorsSelector": `code=~"5.."`,
		},
	}
	if got := valuesOf(t, out)["prometheusRules"]; !reflect.DeepEqual(got, want) {
		t.Errorf("values.prometheusRules =\n%v\nwant\n%v", got, want)
	}
}

func TestPrometheusRulesFeature_ObservabilityMetricsRulesGroup(t *testing.T) {
	out := applyObsFeatures(t, []string{"prometheus-rules"}, map[string]map[string]string{"prometheus-rules": {
		"kind":             "ObservabilityMetricsRulesGroup",
		"slo-availability": "99.5",
	}})

	content, ok := out.Templates["templates/prometheus-rules.yaml"]
	if !ok {
		t.Fatalf("prometheus-rules template missing; templates: %v", keysOfMap(out.Templates))
	}

	// Should not have "groups" field
	if strings.Contains(content, "groups:") {
		t.Errorf("template contains 'groups:', but ObservabilityMetricsRulesGroup should use 'rules:' directly")
	}

	for _, want := range []string{
		"apiVersion: observability.deckhouse.io/v1alpha1",
		"kind: ObservabilityMetricsRulesGroup",
		"name: {{ include \"app.fullname\" $ }}-workloads",
		"name: {{ include \"app.fullname\" $ }}-slo",
		"  rules:\n",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("template misses %q:\n%s", want, content)
		}
	}
}

func TestPrometheusRulesFeature_InvalidKind(t *testing.T) {
	_, err := ApplyFeatures([]*types.GeneratedChart{obsTestChart()}, []string{"prometheus-rules"},
		map[string]map[string]string{"prometheus-rules": {"kind": "InvalidKind"}}, obsTestGraph())
	if err == nil || !strings.Contains(err.Error(), "prometheus-rules: kind must be PrometheusRule or ObservabilityMetricsRulesGroup") {
		t.Errorf("expected error about invalid kind, got %v", err)
	}
}

func TestPrometheusRulesFeature_SLO(t *testing.T) {
	out := applyObsFeatures(t, []string{"prometheus-rules"}, map[string]map[string]string{"prometheus-rules": {
		"slo-availability": "99.5", "severity": "critical", "runbook-url": "https://runbooks.example.com",
	}})
	cfg := valuesOf(t, out)["prometheusRules"].(map[string]interface{})
	slo := cfg["slo"].(map[string]interface{})
	if slo["enabled"] != true || slo["availability"] != 99.5 || cfg["severity"] != "critical" || cfg["runbookUrl"] != "https://runbooks.example.com" {
		t.Errorf("values.prometheusRules = %v", cfg)
	}

	for _, bad := range []map[string]string{{"slo-availability": "100"}, {"slo-availability": "high"}, {"slo-requests-metric": ""}} {
		_, err := ApplyFeatures([]*types.GeneratedChart{obsTestChart()}, []string{"prometheus-rules"},
			map[string]map[string]string{"prometheus-rules": bad}, obsTestGraph())
		if err == nil {
			t.Errorf("options %v must be rejected", bad)
		}
	}
}

func TestPrometheusRulesFeature_NoWorkloads(t *testing.T) {
	chart := obsTestChart()
	chart.Templates = map[string]string{obsServicePath: obsServiceTemplate}
	charts, err := ApplyFeatures([]*types.GeneratedChart{chart}, []string{"prometheus-rules"}, nil, obsTestGraph())
	if err != nil {
		t.Fatal(err)
	}
	if charts[0] != chart {
		t.Error("chart without workloads must be returned unchanged")
	}
}
