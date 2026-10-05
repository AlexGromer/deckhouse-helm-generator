package generator

import (
	"reflect"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestParseCanarySteps(t *testing.T) {
	got, err := ParseCanarySteps(" 10:30s, 50:manual ,100")
	if err != nil {
		t.Fatal(err)
	}
	want := []CanaryStep{{10, "30s"}, {50, "manual"}, {100, ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseCanarySteps = %v, want %v", got, want)
	}
	wantValues := []interface{}{
		map[string]interface{}{"setWeight": 10},
		map[string]interface{}{"pause": map[string]interface{}{"duration": "30s"}},
		map[string]interface{}{"setWeight": 50},
		map[string]interface{}{"pause": map[string]interface{}{}},
		map[string]interface{}{"setWeight": 100},
	}
	if v := canaryStepsValues(got); !reflect.DeepEqual(v, wantValues) {
		t.Errorf("canaryStepsValues = %v, want %v", v, wantValues)
	}

	for _, bad := range []string{"", "0:1m", "101", "abc", "20:soon"} {
		if _, err := ParseCanarySteps(bad); err == nil {
			t.Errorf("ParseCanarySteps(%q) must fail", bad)
		}
	}
}

func TestArgoRolloutsFeature(t *testing.T) {
	out := applyObsFeatures(t, []string{"argo-rollouts"}, nil)

	content, ok := out.Templates["templates/argo-rollout-web-deployment.yaml"]
	if !ok {
		t.Fatalf("Rollout template missing; templates: %v", keysOfMap(out.Templates))
	}
	want := `{{- if $.Values.argoRollouts.enabled }}
{{- $svc := .Values.services.web -}}
{{- if $svc.enabled }}
{{- with $svc.deployment }}
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: web
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "app.labels" $ | nindent 4 }}
    app.kubernetes.io/component: web
spec:
  {{- if not .autoscaling }}
  replicas: {{ .replicas | default 1 }}
  {{- end }}
  selector:
    {{- toYaml .selector | nindent 4 }}
  workloadRef:
    apiVersion: apps/v1
    kind: "Deployment"
    name: web
    scaleDown: {{ $.Values.argoRollouts.scaleDown | default "progressively" }}
  strategy:
    canary:
      {{- with $.Values.argoRollouts.steps }}
      steps:
        {{- toYaml . | nindent 8 }}
      {{- end }}
{{- end }}
{{- end }}
{{- end }}
`
	if content != want {
		t.Errorf("Rollout template:\n%s\nwant:\n%s", content, want)
	}
	for path := range out.Templates {
		if strings.HasPrefix(path, "templates/argo-rollout-") && path != "templates/argo-rollout-web-deployment.yaml" {
			t.Errorf("only Deployments get a Rollout, got %s", path)
		}
	}
	if out.Templates[obsDeploymentPath] != obsDeploymentTemplate {
		t.Error("the referenced Deployment must stay unchanged")
	}

	values := valuesOf(t, out)["argoRollouts"].(map[string]interface{})
	if values["enabled"] != true || values["scaleDown"] != "progressively" || len(values["steps"].([]interface{})) != 4 {
		t.Errorf("values.argoRollouts = %v", values)
	}
}

func TestArgoRolloutsFeature_InvalidParams(t *testing.T) {
	for _, opts := range []map[string]string{{"steps": "x"}, {"scale-down": "always"}} {
		_, err := ApplyFeatures([]*types.GeneratedChart{obsTestChart()}, []string{"argo-rollouts"},
			map[string]map[string]string{"argo-rollouts": opts}, obsTestGraph())
		if err == nil {
			t.Errorf("options %v must be rejected", opts)
		}
	}
}
