package generator

import (
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func TestFluxFeature_HelmRelease(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:      "app",
		ChartYAML: "apiVersion: v2\nname: app\nversion: 1.2.3\n",
		Templates: map[string]string{"templates/x.yaml": "x"},
	}
	charts, err := ApplyFeatures([]*types.GeneratedChart{chart}, []string{"flux"},
		map[string]map[string]string{"flux": {"namespace": "prod"}}, types.NewResourceGraph())
	if err != nil {
		t.Fatal(err)
	}
	files := charts[0].ExternalFiles
	if len(files) != 1 || files[0].Path != "flux/helmrelease.yaml" {
		t.Fatalf("expected flux/helmrelease.yaml, got %+v", files)
	}
	var hr map[string]interface{}
	if err := yaml.Unmarshal([]byte(files[0].Content), &hr); err != nil {
		t.Fatalf("invalid YAML: %v\n%s", err, files[0].Content)
	}
	for _, want := range []string{"apiVersion: helm.toolkit.fluxcd.io/v2", "kind: HelmRelease", `namespace: "prod"`, `chart: "app"`, `version: "1.2.3"`, "kind: HelmRepository"} {
		if !strings.Contains(files[0].Content, want) {
			t.Errorf("missing %q in:\n%s", want, files[0].Content)
		}
	}
	if len(chart.ExternalFiles) != 0 {
		t.Error("input chart mutated")
	}
}

func TestFluxFeature_GitRepositoryUsesPath(t *testing.T) {
	chart := &types.GeneratedChart{Name: "app", ChartYAML: "name: app\nversion: 1.0.0\n", Templates: map[string]string{}}
	charts, err := ApplyFeatures([]*types.GeneratedChart{chart}, []string{"flux"},
		map[string]map[string]string{"flux": {"source-kind": "GitRepository"}}, types.NewResourceGraph())
	if err != nil {
		t.Fatal(err)
	}
	if c := charts[0].ExternalFiles[0].Content; !strings.Contains(c, `chart: "./app"`) || strings.Contains(c, "version:") {
		t.Errorf("GitRepository source must reference the chart path without version:\n%s", c)
	}

	_, err = ApplyFeatures([]*types.GeneratedChart{chart}, []string{"flux"},
		map[string]map[string]string{"flux": {"source-kind": "Nope"}}, types.NewResourceGraph())
	if err == nil {
		t.Error("expected error for unsupported source kind")
	}
}
