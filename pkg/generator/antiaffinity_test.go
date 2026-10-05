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
  name: {{ include "web.fullname" $ }}-web
spec:
  replicas: {{ .replicas | default 1 }}
  selector:
    matchLabels:
      {{- include "web.selectorLabels" $ | nindent 6 }}
      app.kubernetes.io/component: web
  template:
    metadata:
      labels:
        {{- include "web.labels" $ | nindent 8 }}
        app.kubernetes.io/component: web
    spec:
      containers:
        {{- range .containers }}
        - name: {{ .name }}
        {{- end }}
      {{- with .affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
{{- end }}
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
		"                    {{- include \"web.selectorLabels\" $ | nindent 20 }}\n                    app.kubernetes.io/component: web\n",
		"                  {{- include \"web.selectorLabels\" $ | nindent 18 }}\n                  app.kubernetes.io/component: web\n",
		"    topologyKey: " + zoneTopologyKey,
		// The values-driven affinity of the template is kept.
		"      {{- with .affinity }}",
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

func TestExtractSelectorMatchLabels(t *testing.T) {
	lines := strings.Split(aaDeploymentTemplate, "\n")
	got, ok := extractSelectorMatchLabels(lines)
	if !ok || len(got) != 2 || !strings.Contains(got[0], "selectorLabels") || strings.TrimSpace(got[1]) != "app.kubernetes.io/component: web" {
		t.Errorf("extractSelectorMatchLabels = %q, %v", got, ok)
	}
	if _, ok := extractSelectorMatchLabels([]string{"spec:", "  replicas: 1"}); ok {
		t.Error("expected no selector")
	}
}
