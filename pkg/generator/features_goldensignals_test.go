package generator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"sigs.k8s.io/yaml"
)

func TestGoldenSignals(t *testing.T) {
	chart := &types.GeneratedChart{
		ValuesYAML: "",
		Templates:  map[string]string{},
	}

	fc := FeatureContext{
		Params: map[string]string{
			"latency-p99":       "1.0",
			"error-rate":        "0.05",
			"cpu-saturation":    "0.85",
			"memory-saturation": "0.90",
			"dashboard":         "true",
			"otel-collector":    "true",
		},
	}

	out, err := applyGoldenSignalsFeature(chart, fc)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal([]byte(out.ValuesYAML), &root); err != nil {
		t.Fatalf("yaml: %v", err)
	}

	goldenSignalsIface, ok := root["goldenSignals"]
	if !ok {
		t.Fatalf("not found")
	}

	goldenSignals := goldenSignalsIface.(map[string]interface{})
	if goldenSignals["enabled"] != true {
		t.Fatalf("expected true==goldenSignals[\"enabled\"]")
	}
	// sigs.k8s.io/yaml parses numbers as float64
	if goldenSignals["latencyP99"] != 1.0 {
		t.Fatalf("expected 1.0==goldenSignals[\"latencyP99\"]")
	}
	if goldenSignals["errorRate"] != 0.05 {
		t.Fatalf("expected 0.05==goldenSignals[\"errorRate\"]")
	}
	if goldenSignals["cpuSaturation"] != 0.85 {
		t.Fatalf("expected 0.85==goldenSignals[\"cpuSaturation\"]")
	}
	if goldenSignals["memorySaturation"] != 0.90 {
		t.Fatalf("expected 0.90==goldenSignals[\"memorySaturation\"]")
	}

	dashboardVal := goldenSignals["dashboard"].(map[string]interface{})
	if dashboardVal["enabled"] != true {
		t.Fatalf("expected true==dashboardVal[\"enabled\"]")
	}

	otelVal := goldenSignals["otelCollector"].(map[string]interface{})
	if otelVal["enabled"] != true {
		t.Fatalf("expected true==otelVal[\"enabled\"]")
	}
	if otelVal["endpoint"] != "http://otel-collector:4317" {
		t.Fatalf("expected \"http://otel-collector:4317\"==otelVal[\"endpoint\"]")
	}

	if _, ok := out.Templates["templates/golden-signals-rules.yaml"]; !ok {
		t.Fatalf("missing templates/golden-signals-rules.yaml")
	}
	rules := out.Templates["templates/golden-signals-rules.yaml"]
	if !strings.Contains(rules, "apiVersion: monitoring.coreos.com/v1") {
		t.Fatalf("missing apiVersion: monitoring.coreos.com/v1")
	}
	if !strings.Contains(rules, "kind: PrometheusRule") {
		t.Fatalf("missing kind: PrometheusRule")
	}
	if !strings.Contains(rules, "GoldenSignalHighLatencyP99") {
		t.Fatalf("missing GoldenSignalHighLatencyP99")
	}
	if !strings.Contains(rules, "GoldenSignalHighErrorRate") {
		t.Fatalf("missing GoldenSignalHighErrorRate")
	}
	if !strings.Contains(rules, "GoldenSignalTrafficDrop") {
		t.Fatalf("missing GoldenSignalTrafficDrop")
	}
	if !strings.Contains(rules, "GoldenSignalHighSaturation") {
		t.Fatalf("missing GoldenSignalHighSaturation")
	}

	if _, ok := out.Templates["templates/golden-signals-dashboard.yaml"]; !ok {
		t.Fatalf("missing templates/golden-signals-dashboard.yaml")
	}
	dashboard := out.Templates["templates/golden-signals-dashboard.yaml"]
	if !strings.Contains(dashboard, "kind: ConfigMap") {
		t.Fatalf("missing kind: ConfigMap")
	}
	if !strings.Contains(dashboard, "grafana_dashboard: \"1\"") {
		t.Fatalf("missing grafana_dashboard: \"1\"")
	}

	// Ensure it has valid JSON inside
	lines := strings.Split(dashboard, "\n")
	jsonStr := ""
	inJSON := false
	for _, line := range lines {
		if strings.Contains(line, "golden-signals.json: |-") {
			inJSON = true
			continue
		}
		if inJSON && strings.HasPrefix(line, "{{- end }}") {
			inJSON = false
			continue
		}
		if inJSON {
			jsonStr += strings.TrimPrefix(line, "    ") + "\n"
		}
	}
	var dash map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &dash)
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	if dash["title"] != "Golden Signals" {
		t.Fatalf("expected \"Golden Signals\"==dash[\"title\"]")
	}

	if _, ok := out.Templates["templates/otel-collector-config.yaml"]; !ok {
		t.Fatalf("missing templates/otel-collector-config.yaml")
	}
	otel := out.Templates["templates/otel-collector-config.yaml"]
	if !strings.Contains(otel, "kind: ConfigMap") {
		t.Fatalf("missing kind: ConfigMap")
	}
	if !strings.Contains(otel, "endpoint: {{ .Values.goldenSignals.otelCollector.endpoint }}") {
		t.Fatalf("missing endpoint: {{ .Values.goldenSignals.otelCollector.endpoint }}")
	}
}
