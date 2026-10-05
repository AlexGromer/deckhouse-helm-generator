package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// aaDeploymentTemplate has the shape of the Deployment processor's template.
const aaDeploymentTemplate = `{{- $svc := .Values -}}
{{- with $svc.deployment }}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: {{ .replicas | default 1 }}
  selector:
    {{- toYaml .selector | nindent 4 }}
  template:
    metadata:
      labels:
        {{- toYaml (merge (dict) (.podLabels | default dict) (include "web.labels" $ | fromYaml)) | nindent 8 }}
    spec:
      containers:
        {{- range .containers }}
        - name: {{ .name }}
        {{- end }}
      {{- with .affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .topologySpreadConstraints }}
      topologySpreadConstraints:
        {{- toYaml . | nindent 8 }}
      {{- end }}
{{- end }}
`

// aaLiteralSelectorTemplate is a workload with a literal selector using
// matchLabels and matchExpressions.
const aaLiteralSelectorTemplate = `apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: db
spec:
  selector:
    matchLabels:
      app: db
    matchExpressions:
      - key: tier
        operator: In
        values: [data]
  template:
    metadata:
      labels:
        app: db
        tier: data
    spec:
      containers:
        - name: db
`

func aaChart(templates map[string]string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name:       "web",
		ChartYAML:  "apiVersion: v2\nname: web\nversion: 0.1.0\n",
		ValuesYAML: "deployment:\n  replicas: 2\n",
		Templates:  templates,
	}
}

func TestInjectAntiAffinity_Deployment(t *testing.T) {
	in := aaChart(map[string]string{"templates/web-deployment.yaml": aaDeploymentTemplate})
	out, res, err := InjectAntiAffinity(in, AntiAffinityOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Injected) != 1 || len(res.Skipped) != 0 {
		t.Fatalf("result = %+v", res)
	}
	tpl := out.Templates["templates/web-deployment.yaml"]
	for _, want := range []string{
		"      {{- if and $dhgAntiAffinity.enabled (not .affinity) }}\n      affinity:\n        podAntiAffinity:",
		"preferredDuringSchedulingIgnoredDuringExecution:",
		"requiredDuringSchedulingIgnoredDuringExecution:",
		// The pods are selected by the workload's own selector.
		"              labelSelector:\n                {{- toYaml .selector | nindent 16 }}\n",
		"                labelSelector:\n                  {{- toYaml .selector | nindent 18 }}\n",
		"    topologyKey: " + zoneTopologyKey,
		"          labelSelector:\n            {{- toYaml .selector | nindent 12 }}\n",
		// Zone spread gives way to topologySpreadConstraints from values.
		"{{- if and $dhgAntiAffinity.enabled $dhgZoneSpread.enabled (not .topologySpreadConstraints) }}",
		// The values-driven affinity and spread constraints of the template are kept.
		"      {{- with .affinity }}",
		"      {{- with .topologySpreadConstraints }}",
	} {
		if !strings.Contains(tpl, want) {
			t.Errorf("template misses %q:\n%s", want, tpl)
		}
	}
	if strings.Index(tpl, "podAntiAffinity") > strings.Index(tpl, "      containers:") {
		t.Error("anti-affinity must be inserted before the containers list")
	}
	if !strings.HasPrefix(out.ValuesYAML, in.ValuesYAML) || !strings.Contains(out.ValuesYAML, "antiAffinity:\n  enabled: true\n  mode: preferred\n") {
		t.Errorf("unexpected values:\n%s", out.ValuesYAML)
	}
	if in.Templates["templates/web-deployment.yaml"] != aaDeploymentTemplate {
		t.Error("input chart was modified")
	}

	// A second application finds the block and changes nothing.
	again, res, err := InjectAntiAffinity(out, AntiAffinityOptions{})
	if err != nil || again != out || len(res.Injected) != 0 {
		t.Errorf("second application changed the chart (err=%v, res=%+v)", err, res)
	}
}

func TestInjectAntiAffinity_SkipsAndIgnores(t *testing.T) {
	custom := strings.Replace(aaDeploymentTemplate, "      containers:", "      affinity:\n        nodeAffinity: {}\n      containers:", 1)
	job := "apiVersion: batch/v1\nkind: Job\nspec:\n  template:\n    spec:\n      containers: []\n"
	daemonSet := strings.Replace(aaDeploymentTemplate, "kind: Deployment", "kind: DaemonSet", 1)
	noSelector := strings.Replace(aaDeploymentTemplate, "  selector:\n", "  selectorX:\n", 1)

	in := aaChart(map[string]string{
		"templates/custom.yaml":      custom,
		"templates/job.yaml":         job,
		"templates/daemonset.yaml":   daemonSet,
		"templates/no-selector.yaml": noSelector,
	})
	out, res, err := InjectAntiAffinity(in, AntiAffinityOptions{Mode: AffinityModeRequired})
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Error("chart without injectable workloads must be returned unchanged")
	}
	if strings.Join(res.Skipped, ",") != "templates/custom.yaml,templates/no-selector.yaml" {
		t.Errorf("skipped = %v", res.Skipped)
	}

	if _, _, err := InjectAntiAffinity(in, AntiAffinityOptions{Mode: "sometimes"}); err == nil {
		t.Error("expected an error for an invalid mode")
	}
}

func TestReindentBlock(t *testing.T) {
	block := []string{
		`      {{- include "x.selectorLabels" $ | nindent 6 }}`,
		`      app: web`,
		`        nested: true`,
	}
	got := reindentBlock(block, 10)
	want := "          {{- include \"x.selectorLabels\" $ | nindent 10 }}\n          app: web\n            nested: true\n"
	if got != want {
		t.Errorf("reindentBlock =\n%q\nwant\n%q", got, want)
	}
}

func TestWorkloadSelectorLines(t *testing.T) {
	lines := strings.Split(aaDeploymentTemplate, "\n")
	got, ok := workloadSelectorLines(lines)
	if !ok || len(got) != 1 || strings.TrimSpace(got[0]) != "{{- toYaml .selector | nindent 4 }}" {
		t.Errorf("workloadSelectorLines = %q, %v", got, ok)
	}
	got, ok = workloadSelectorLines(strings.Split(aaLiteralSelectorTemplate, "\n"))
	if !ok || len(got) != 6 || strings.TrimSpace(got[0]) != "matchLabels:" || strings.TrimSpace(got[2]) != "matchExpressions:" {
		t.Errorf("workloadSelectorLines = %q, %v", got, ok)
	}
	if _, ok := workloadSelectorLines([]string{"spec:", "  replicas: 1"}); ok {
		t.Error("expected no selector")
	}
}

func TestInjectAntiAffinity_LiteralSelector(t *testing.T) {
	in := aaChart(map[string]string{"templates/db.yaml": aaLiteralSelectorTemplate})
	out, res, err := InjectAntiAffinity(in, AntiAffinityOptions{Mode: AffinityModeRequired})
	if err != nil || len(res.Injected) != 1 {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	want := "              labelSelector:\n                matchLabels:\n                  app: db\n                matchExpressions:\n                  - key: tier\n"
	if tpl := out.Templates["templates/db.yaml"]; !strings.Contains(tpl, want) {
		t.Errorf("template misses %q:\n%s", want, tpl)
	}
}
