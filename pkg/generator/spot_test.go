package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Section 1: GenerateSpotTolerations — provider-specific keys
// ============================================================

func TestSpot_AWS_Tolerations(t *testing.T) {
	tolerations := GenerateSpotTolerations(SpotAWS)

	if len(tolerations) == 0 {
		t.Fatal("expected at least one toleration for AWS spot provider")
	}

	found := false
	for _, tol := range tolerations {
		key, _ := tol["key"].(string)
		val, _ := tol["value"].(string)
		effect, _ := tol["effect"].(string)

		if key == "node.kubernetes.io/lifecycle" {
			found = true
			if val != "spot" {
				t.Errorf("AWS toleration: expected value='spot', got '%s'", val)
			}
			if effect != "NoSchedule" {
				t.Errorf("AWS toleration: expected effect='NoSchedule', got '%s'", effect)
			}
			break
		}
	}

	if !found {
		t.Error("AWS tolerations must contain key='node.kubernetes.io/lifecycle'")
	}
}

func TestSpot_GCP_Tolerations(t *testing.T) {
	tolerations := GenerateSpotTolerations(SpotGCP)

	if len(tolerations) == 0 {
		t.Fatal("expected at least one toleration for GCP spot provider")
	}

	found := false
	for _, tol := range tolerations {
		key, _ := tol["key"].(string)
		if key == "cloud.google.com/gke-preemptible" {
			found = true
			val, _ := tol["value"].(string)
			effect, _ := tol["effect"].(string)
			if val != "true" {
				t.Errorf("GCP toleration: expected value='true', got '%s'", val)
			}
			if effect != "NoSchedule" {
				t.Errorf("GCP toleration: expected effect='NoSchedule', got '%s'", effect)
			}
			break
		}
	}

	if !found {
		t.Error("GCP tolerations must contain key='cloud.google.com/gke-preemptible'")
	}
}

func TestSpot_Azure_Tolerations(t *testing.T) {
	tolerations := GenerateSpotTolerations(SpotAzure)

	if len(tolerations) == 0 {
		t.Fatal("expected at least one toleration for Azure spot provider")
	}

	found := false
	for _, tol := range tolerations {
		key, _ := tol["key"].(string)
		if key == "kubernetes.azure.com/scalesetpriority" {
			found = true
			val, _ := tol["value"].(string)
			effect, _ := tol["effect"].(string)
			if val != "spot" {
				t.Errorf("Azure toleration: expected value='spot', got '%s'", val)
			}
			if effect != "NoSchedule" {
				t.Errorf("Azure toleration: expected effect='NoSchedule', got '%s'", effect)
			}
			break
		}
	}

	if !found {
		t.Error("Azure tolerations must contain key='kubernetes.azure.com/scalesetpriority'")
	}
}

// ============================================================
// InjectSpotConfig on dhg workload templates
// ============================================================

const spotDeploymentTemplate = `{{- $svc := .Values.services.web -}}
{{- with $svc.deployment }}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "app.fullname" $ }}-web
spec:
  template:
    metadata:
      labels:
        app.kubernetes.io/component: web
    spec:
      containers:
        - name: web
      {{- with .tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
{{- end }}
`

func spotTestChart(templates map[string]string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name:       "app",
		ChartYAML:  "apiVersion: v2\nname: app\nversion: 0.1.0\n",
		ValuesYAML: "services:\n  web:\n    enabled: true\n",
		Templates:  templates,
	}
}

func TestInjectSpotConfig_MergesTolerationsFromValues(t *testing.T) {
	chart := spotTestChart(map[string]string{"templates/web-deployment.yaml": spotDeploymentTemplate})
	out, err := InjectSpotConfig(chart, SpotConfig{Provider: SpotGCP, GracePeriod: 45, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	tmpl := out.Templates["templates/web-deployment.yaml"]
	for _, want := range []string{
		"{{- $dhgTolerations := .tolerations | default list }}",
		"concat $dhgTolerations $.Values.spot.tolerations",
		"terminationGracePeriodSeconds: {{ $.Values.spot.terminationGracePeriodSeconds }}",
		"{{- toYaml . | nindent 8 }}",
	} {
		if !strings.Contains(tmpl, want) {
			t.Errorf("missing %q in:\n%s", want, tmpl)
		}
	}
	if n := strings.Count(tmpl, "tolerations:"); n != 1 {
		t.Errorf("expected exactly one tolerations key, got %d", n)
	}
	for _, want := range []string{"spot:", "enabled: true", "provider: gcp", "terminationGracePeriodSeconds: 45", "cloud.google.com/gke-preemptible"} {
		if !strings.Contains(out.ValuesYAML, want) {
			t.Errorf("values missing %q:\n%s", want, out.ValuesYAML)
		}
	}
	if chart.Templates["templates/web-deployment.yaml"] != spotDeploymentTemplate {
		t.Error("input chart was mutated")
	}
}

func TestInjectSpotConfig_PDBPerComponent(t *testing.T) {
	chart := spotTestChart(map[string]string{"templates/web-deployment.yaml": spotDeploymentTemplate})
	out, err := InjectSpotConfig(chart, SpotConfig{Provider: SpotAWS, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	pdb, ok := out.Templates["templates/web-deployment-spot-pdb.yaml"]
	if !ok {
		t.Fatal("expected a spot PDB for the Deployment")
	}
	for _, want := range []string{"kind: PodDisruptionBudget", "maxUnavailable: 1", "app.kubernetes.io/component: web", `include "app.selectorLabels"`, "if and .Values.spot .Values.spot.enabled"} {
		if !strings.Contains(pdb, want) {
			t.Errorf("PDB missing %q:\n%s", want, pdb)
		}
	}

	// A workload that already has a PDB does not get a second one.
	chart.Templates["templates/web-pdb.yaml"] = "kind: PodDisruptionBudget\n    app.kubernetes.io/component: web\n"
	out, err = InjectSpotConfig(chart, SpotConfig{Provider: SpotAWS, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out.Templates["templates/web-deployment-spot-pdb.yaml"]; ok {
		t.Error("spot PDB must not duplicate an existing PDB")
	}
}

func TestInjectSpotConfig_SkipsJobsAndForeignTemplates(t *testing.T) {
	job := strings.Replace(spotDeploymentTemplate, "kind: Deployment", "kind: Job", 1)
	foreign := "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: x\nspec: {}\n"
	chart := spotTestChart(map[string]string{"templates/job.yaml": job, "templates/foreign.yaml": foreign})
	out, err := InjectSpotConfig(chart, SpotConfig{Provider: SpotAWS, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Templates["templates/job.yaml"] != job || out.Templates["templates/foreign.yaml"] != foreign {
		t.Error("jobs and templates without a values tolerations block must be left unchanged")
	}
	if strings.Contains(out.ValuesYAML, "spot:") {
		t.Error("spot values must not be added when nothing was injected")
	}
}

func TestInjectSpotConfig_NilChart(t *testing.T) {
	out, err := InjectSpotConfig(nil, SpotConfig{Provider: SpotAWS})
	if out != nil || err != nil {
		t.Errorf("expected nil, nil; got %v, %v", out, err)
	}
}
