package generator

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestCIWorkflow_GitHub(t *testing.T) {
	chart := &types.GeneratedChart{
		ExternalFiles: []types.ExternalFileInfo{},
	}

	fc := FeatureContext{
		Params: map[string]string{
			"providers":      "github",
			"sarif":          "true",
			"benchmark-gate": "true",
		},
	}

	f, _ := LookupFeature("ci-workflow")
	out, err := f.Apply(chart, fc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, ext := range out.ExternalFiles {
		if ext.Path == ".github/workflows/helm-ci.yaml" {
			found = true
			if !strings.Contains(ext.Content, "upload-sarif") {
				t.Errorf("Expected GitHub actions to include upload-sarif")
			}
		}
	}
	if !found {
		t.Errorf("Expected .github/workflows/helm-ci.yaml to be created")
	}
}

func TestCIWorkflow_GitLab(t *testing.T) {
	chart := &types.GeneratedChart{
		ExternalFiles: []types.ExternalFileInfo{},
	}

	fc := FeatureContext{
		Params: map[string]string{
			"providers":      "gitlab",
			"sarif":          "true",
			"benchmark-gate": "true",
		},
	}

	f, _ := LookupFeature("ci-workflow")
	out, err := f.Apply(chart, fc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, ext := range out.ExternalFiles {
		if ext.Path == ".gitlab-ci.yml" {
			found = true
			if !strings.Contains(ext.Content, "security-sarif") {
				t.Errorf("Expected GitLab actions to include security-sarif stage")
			}
		}
	}
	if !found {
		t.Errorf("Expected .gitlab-ci.yml to be created")
	}
}

func TestCIWorkflow_Both(t *testing.T) {
	chart := &types.GeneratedChart{
		ExternalFiles: []types.ExternalFileInfo{},
	}

	fc := FeatureContext{
		Params: map[string]string{
			"providers":      "github,gitlab",
			"sarif":          "true",
			"benchmark-gate": "true",
		},
		Graph: &types.ResourceGraph{
			Resources: make(map[types.ResourceKey]*types.ProcessedResource),
		},
	}

	f, _ := LookupFeature("ci-workflow")
	out, err := f.Apply(chart, fc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hasGitHub := false
	hasGitLab := false
	hasSARIF := false
	hasHTML := false

	for _, ext := range out.ExternalFiles {
		if ext.Path == ".github/workflows/helm-ci.yaml" {
			hasGitHub = true
		}
		if ext.Path == ".gitlab-ci.yml" {
			hasGitLab = true
		}
		if ext.Path == "docs/audit.sarif.json" {
			hasSARIF = true
		}
		if ext.Path == "docs/audit-report.html" {
			hasHTML = true
		}
	}

	if !hasGitHub {
		t.Errorf("Missing GitHub workflow")
	}
	if !hasGitLab {
		t.Errorf("Missing GitLab CI")
	}
	if !hasSARIF {
		t.Errorf("Missing SARIF output")
	}
	if !hasHTML {
		t.Errorf("Missing HTML output")
	}
}
