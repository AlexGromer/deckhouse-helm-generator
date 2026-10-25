package generator

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestMultiEnv(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:         "test-chart",
		ValuesYAML:   "replicaCount: 1\n",
		ValuesSchema: `{"type":"object","properties":{"replicaCount":{"type":"integer"}}}`,
	}
	var notes []string

	fc := FeatureContext{
		Params: map[string]string{
			"environments":  "dev,staging,prod,qa",
			"delivery":      "helmfile,kustomize",
			"strict-schema": "true",
		},
		Notes: &notes,
	}

	out, err := applyMultiEnvFeature(chart, fc)
	if err != nil {
		t.Fatalf("applyMultiEnvFeature failed: %v", err)
	}

	// 1. Verify standard values files are created
	expectedFiles := []string{
		"values-dev.yaml",
		"values-staging.yaml",
		"values-prod.yaml",
		"values-qa.yaml",
		"helmfile.yaml",
		"overlays/dev/kustomization.yaml",
		"overlays/staging/kustomization.yaml",
		"overlays/prod/kustomization.yaml",
		"overlays/qa/kustomization.yaml",
	}

	actualFiles := make(map[string]string)
	for _, f := range out.ExternalFiles {
		actualFiles[f.Path] = f.Content
	}

	for _, expected := range expectedFiles {
		if _, ok := actualFiles[expected]; !ok {
			t.Errorf("Missing expected file: %s", expected)
		}
	}

	// 2. Verify QA profile (non-standard env)
	if qaContent, ok := actualFiles["values-qa.yaml"]; ok {
		if !strings.HasPrefix(qaContent, "# qa environment overrides") {
			t.Errorf("values-qa.yaml doesn't have the correct override comment: %s", qaContent)
		}
		if !strings.Contains(qaContent, "replicaCount: 2") { // staging default
			t.Errorf("values-qa.yaml does not contain staging defaults: %s", qaContent)
		}
	}

	// 3. Verify helmfile
	if hfContent, ok := actualFiles["helmfile.yaml"]; ok {
		if !strings.Contains(hfContent, "dev:") || !strings.Contains(hfContent, "qa:") {
			t.Errorf("helmfile.yaml missing environments block: %s", hfContent)
		}
		if !strings.Contains(hfContent, "- name: test-chart") {
			t.Errorf("helmfile.yaml missing release name block: %s", hfContent)
		}
	}

	// 4. Verify kustomize
	if ksContent, ok := actualFiles["overlays/qa/kustomization.yaml"]; ok {
		if !strings.Contains(ksContent, "releaseName: test-chart") {
			t.Errorf("overlays/qa/kustomization.yaml missing correct release name: %s", ksContent)
		}
		if !strings.Contains(ksContent, "../../values-qa.yaml") {
			t.Errorf("overlays/qa/kustomization.yaml missing correct valuesFile: %s", ksContent)
		}
	}

	// 5. Verify strict schema
	if !strings.Contains(out.ValuesSchema, `"additionalProperties": false`) {
		t.Errorf("ValuesSchema does not contain additionalProperties=false: %s", out.ValuesSchema)
	}

	// 6. Verify notes
	if len(notes) == 0 {
		t.Errorf("Expected a note to be added, got 0 notes")
	} else if !strings.Contains(notes[0], "dev, staging, prod, qa") {
		t.Errorf("Note did not mention the generated envs: %s", notes[0])
	}
}
