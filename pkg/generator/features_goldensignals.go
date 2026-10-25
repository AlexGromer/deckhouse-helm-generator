package generator

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func init() {
	RegisterFeature(Feature{
		Name:        "golden-signals",
		Description: "Generate Golden Signals for observability (alerting rules, Grafana dashboard, OTel collector config)",
		Params: map[string]string{
			"latency-p99":       "0.5",
			"error-rate":        "0.05",
			"cpu-saturation":    "0.85",
			"memory-saturation": "0.90",
			"dashboard":         "true",
			"otel-collector":    "false",
		},
		Apply: applyGoldenSignalsFeature,
	})
}

func applyGoldenSignalsFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	latencyP99Str := fc.Param("latency-p99")
	errorRateStr := fc.Param("error-rate")
	cpuSaturationStr := fc.Param("cpu-saturation")
	memorySaturationStr := fc.Param("memory-saturation")

	latencyP99, err := strconv.ParseFloat(latencyP99Str, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid latency-p99: %w", err)
	}
	errorRate, err := strconv.ParseFloat(errorRateStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid error-rate: %w", err)
	}
	cpuSaturation, err := strconv.ParseFloat(cpuSaturationStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid cpu-saturation: %w", err)
	}
	memorySaturation, err := strconv.ParseFloat(memorySaturationStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid memory-saturation: %w", err)
	}

	dashboardEnabled := fc.Param("dashboard") == "true"
	otelCollectorEnabled := fc.Param("otel-collector") == "true"

	out := cloneChart(chart)

	values := map[string]interface{}{
		"enabled":          true,
		"latencyP99":       latencyP99,
		"errorRate":        errorRate,
		"cpuSaturation":    cpuSaturation,
		"memorySaturation": memorySaturation,
		"dashboard": map[string]interface{}{
			"enabled": dashboardEnabled,
		},
		"otelCollector": map[string]interface{}{
			"enabled":  otelCollectorEnabled,
			"endpoint": "http://otel-collector:4317",
		},
	}

	if err := addFeatureValues(out, "goldenSignals", values); err != nil {
		return nil, err
	}

	rulesTmpl := `{{- if (.Values.goldenSignals).enabled }}
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: golden-signals-rules
  namespace: {{ .Release.Namespace }}
spec:
  groups:
    - name: golden-signals
      rules:
        - alert: GoldenSignalHighLatencyP99
          expr: histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket[5m])) by (le)) > {{ .Values.goldenSignals.latencyP99 }}
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: "High Latency (p99) on {{ .Release.Namespace }}"
        - alert: GoldenSignalHighErrorRate
          expr: sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m])) > {{ .Values.goldenSignals.errorRate }}
          for: 5m
          labels:
            severity: critical
          annotations:
            summary: "High Error Rate on {{ .Release.Namespace }}"
        - alert: GoldenSignalTrafficDrop
          expr: sum(rate(http_requests_total[5m])) == 0
          for: 10m
          labels:
            severity: warning
          annotations:
            summary: "Traffic Drop on {{ .Release.Namespace }}"
        - alert: GoldenSignalHighSaturation
          expr: (sum(rate(container_cpu_usage_seconds_total[5m])) by (pod) > {{ .Values.goldenSignals.cpuSaturation }}) or (sum(container_memory_working_set_bytes) by (pod) / sum(kube_pod_container_resource_limits{resource="memory"}) by (pod) > {{ .Values.goldenSignals.memorySaturation }})
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: "High CPU/Memory Saturation on {{ .Release.Namespace }}"
{{- end }}
`
	if err := addTemplate(out, "templates/golden-signals-rules.yaml", strings.TrimPrefix(rulesTmpl, "\n")); err != nil {
		return nil, err
	}

	// avoid reindent logic messing up JSON string literals parsing in tests
	dashboardTmpl := `{{- if and (.Values.goldenSignals).enabled ((.Values.goldenSignals).dashboard).enabled }}
apiVersion: v1
kind: ConfigMap
metadata:
  name: golden-signals-dashboard
  namespace: {{ .Release.Namespace }}
  labels:
    grafana_dashboard: "1"
data:
  golden-signals.json: |-
    {
      "title": "Golden Signals",
      "panels": [
        {
          "title": "Latency (p99)",
          "type": "timeseries"
        },
        {
          "title": "Traffic (RPS)",
          "type": "timeseries"
        },
        {
          "title": "Errors (5xx rate)",
          "type": "timeseries"
        },
        {
          "title": "Saturation (CPU & Memory)",
          "type": "timeseries"
        }
      ]
    }
{{- end }}
`

	if err := addTemplate(out, "templates/golden-signals-dashboard.yaml", strings.TrimPrefix(dashboardTmpl, "\n")); err != nil {
		return nil, err
	}

	otelTmpl := `{{- if and (.Values.goldenSignals).enabled ((.Values.goldenSignals).otelCollector).enabled }}
apiVersion: v1
kind: ConfigMap
metadata:
  name: otel-collector-config
  namespace: {{ .Release.Namespace }}
data:
  otel-collector-config.yaml: |
    receivers:
      otlp:
        protocols:
          grpc:
          http:
    processors:
      batch:
    exporters:
      otlp:
        endpoint: {{ .Values.goldenSignals.otelCollector.endpoint }}
    service:
      pipelines:
        traces:
          receivers: [otlp]
          processors: [batch]
          exporters: [otlp]
        metrics:
          receivers: [otlp]
          processors: [batch]
          exporters: [otlp]
{{- end }}
`
	if err := addTemplate(out, "templates/otel-collector-config.yaml", strings.TrimPrefix(otelTmpl, "\n")); err != nil {
		return nil, err
	}

	return out, nil
}
