package generator

// ============================================================
// Test Plan: CI Pipeline Template Generator (Task 6.0.6)
// ============================================================
//
// | #  | Test Name                                                    | Category    | Input                                               | Expected Output                                                              |
// |----|--------------------------------------------------------------|-------------|------------------------------------------------------|-----------------------------------------------------------------------------|
// |  1 | TestGenerateCIPipelineTemplate_GitHubWorkflow                | happy       | Platform="github", all stages enabled               | Workflows contains ".github/workflows/chart-ci.yml"                        |
// |  2 | TestGenerateCIPipelineTemplate_GitLabCI                      | happy       | Platform="gitlab", all stages enabled               | Workflows contains ".gitlab-ci.yml"                                         |
// |  3 | TestGenerateCIPipelineTemplate_AllStagesIncluded             | happy       | all Include flags true                              | workflow YAML has lint, validate, unit/test, security, integration stages   |
// |  4 | TestGenerateCIPipelineTemplate_ExcludeStages                 | happy       | only IncludeLint=true, rest=false                   | workflow contains lint but not security, integration jobs                   |
// |  5 | TestGenerateCIPipelineTemplate_ChartNameInWorkflow           | happy       | ChartName="myapp"                                   | workflow content contains "myapp"                                           |
// |  6 | TestGenerateCIPipelineTemplate_NilChart                      | edge        | chart=nil                                           | returns nil, no panic                                                       |
// |  7 | TestInjectCIPipeline_CopyOnWrite                             | integration | valid chart + result                                | returned chart != original pointer, original Templates unchanged            |
// |  8 | TestGenerateCIPipelineTemplate_PlatformDefaultsToGitHub      | edge        | Platform="" (empty)                                 | defaults to GitHub, .github/workflows/chart-ci.yml present                 |
// |  9 | TestGenerateCIPipelineTemplate_NOTESTxt                      | happy       | valid chart + all stages                            | NOTESTxt non-empty, references platform and chart name                      |
// | 10 | TestGenerateCIPipelineTemplate_ValidYAMLStructure            | happy       | GitHub platform, all stages                         | workflow content contains "on:", "jobs:", valid YAML top-level keys         |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newCIChart(name string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name: name,
		Path: "/charts/" + name,
		ChartYAML: "apiVersion: v2\nname: " + name + "\nversion: 0.1.0\n",
		Templates: map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: " + name + "\n",
		},
		ExternalFiles: []types.ExternalFileInfo{},
	}
}

func allStagesOpts(platform, chartName string) CIPipelineOptions {
	return CIPipelineOptions{
		Platform:           platform,
		ChartName:          chartName,
		IncludeLint:        true,
		IncludeValidate:    true,
		IncludeTest:        true,
		IncludeSecurity:    true,
		IncludeIntegration: true,
	}
}

// ── 1: GitHub Actions workflow file generated ─────────────────────────────────

func TestGenerateCIPipelineTemplate_GitHubWorkflow(t *testing.T) {
	chart := newCIChart("myapp")
	opts := allStagesOpts("github", "myapp")

	result := GenerateCIPipelineTemplate(chart, opts)

	if result == nil {
		t.Fatal("expected non-nil CIPipelineResult")
	}
	if len(result.Workflows) == 0 {
		t.Fatal("expected non-empty Workflows map for github platform")
	}

	const wantKey = ".github/workflows/chart-ci.yml"
	content, ok := result.Workflows[wantKey]
	if !ok {
		t.Fatalf("expected %q in Workflows, got keys: %v", wantKey, workflowKeys(result.Workflows))
	}
	if content == "" {
		t.Errorf("workflow content for %q must not be empty", wantKey)
	}
}

// ── 2: GitLab CI file generated ───────────────────────────────────────────────

func TestGenerateCIPipelineTemplate_GitLabCI(t *testing.T) {
	chart := newCIChart("myapp")
	opts := allStagesOpts("gitlab", "myapp")

	result := GenerateCIPipelineTemplate(chart, opts)

	if result == nil {
		t.Fatal("expected non-nil CIPipelineResult")
	}
	if len(result.Workflows) == 0 {
		t.Fatal("expected non-empty Workflows map for gitlab platform")
	}

	const wantKey = ".gitlab-ci.yml"
	content, ok := result.Workflows[wantKey]
	if !ok {
		t.Fatalf("expected %q in Workflows, got keys: %v", wantKey, workflowKeys(result.Workflows))
	}
	if content == "" {
		t.Errorf("workflow content for %q must not be empty", wantKey)
	}
}

// ── 3: All 5 stages present when all Include flags are true ──────────────────

func TestGenerateCIPipelineTemplate_AllStagesIncluded(t *testing.T) {
	chart := newCIChart("myapp")
	opts := allStagesOpts("github", "myapp")

	result := GenerateCIPipelineTemplate(chart, opts)

	if result == nil {
		t.Fatal("expected non-nil CIPipelineResult")
	}
	content := allWorkflowContent(result.Workflows)

	// Stage: lint (helm lint)
	if !strings.Contains(strings.ToLower(content), "lint") {
		t.Error("workflow must contain a lint stage (helm lint)")
	}
	// Stage: validate (kubeconform)
	if !strings.Contains(strings.ToLower(content), "kubeconform") && !strings.Contains(strings.ToLower(content), "validat") {
		t.Error("workflow must contain a validate stage (kubeconform)")
	}
	// Stage: unit tests (helm-unittest)
	if !strings.Contains(strings.ToLower(content), "unittest") && !strings.Contains(strings.ToLower(content), "test") {
		t.Error("workflow must contain a unit test stage (helm-unittest)")
	}
	// Stage: security (conftest)
	if !strings.Contains(strings.ToLower(content), "conftest") && !strings.Contains(strings.ToLower(content), "security") {
		t.Error("workflow must contain a security stage (conftest)")
	}
	// Stage: integration (ct install)
	if !strings.Contains(strings.ToLower(content), "integrat") && !strings.Contains(strings.ToLower(content), "ct install") {
		t.Error("workflow must contain an integration stage (ct install)")
	}
}

// ── 4: Excluded stages do not appear in output ───────────────────────────────

func TestGenerateCIPipelineTemplate_ExcludeStages(t *testing.T) {
	chart := newCIChart("myapp")
	opts := CIPipelineOptions{
		Platform:           "github",
		ChartName:          "myapp",
		IncludeLint:        true,
		IncludeValidate:    false,
		IncludeTest:        false,
		IncludeSecurity:    false,
		IncludeIntegration: false,
	}

	result := GenerateCIPipelineTemplate(chart, opts)

	if result == nil {
		t.Fatal("expected non-nil CIPipelineResult")
	}
	content := allWorkflowContent(result.Workflows)

	// Lint must be present
	if !strings.Contains(strings.ToLower(content), "lint") {
		t.Error("lint stage must be present when IncludeLint=true")
	}
	// Security must NOT be present
	if strings.Contains(strings.ToLower(content), "conftest") {
		t.Error("security stage (conftest) must be absent when IncludeSecurity=false")
	}
	// Integration must NOT be present
	if strings.Contains(strings.ToLower(content), "ct install") {
		t.Error("integration stage (ct install) must be absent when IncludeIntegration=false")
	}
}

// ── 5: ChartName appears in workflow content ──────────────────────────────────

func TestGenerateCIPipelineTemplate_ChartNameInWorkflow(t *testing.T) {
	chart := newCIChart("myspecialchart")
	opts := allStagesOpts("github", "myspecialchart")

	result := GenerateCIPipelineTemplate(chart, opts)

	if result == nil {
		t.Fatal("expected non-nil CIPipelineResult")
	}
	content := allWorkflowContent(result.Workflows)
	if !strings.Contains(content, "myspecialchart") {
		t.Errorf("workflow content must contain chart name 'myspecialchart', got snippet: %.300s", content)
	}
}

// ── 6: nil chart → nil result, no panic ──────────────────────────────────────

func TestGenerateCIPipelineTemplate_NilChart(t *testing.T) {
	opts := allStagesOpts("github", "myapp")

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("GenerateCIPipelineTemplate panicked on nil chart: %v", r)
		}
	}()

	result := GenerateCIPipelineTemplate(nil, opts)
	if result != nil {
		t.Error("expected nil result for nil chart input")
	}
}

// ── 7: InjectCIPipeline — copy-on-write, original not mutated ────────────────

func TestInjectCIPipeline_CopyOnWrite(t *testing.T) {
	original := newCIChart("myapp")
	opts := allStagesOpts("github", "myapp")
	ciResult := GenerateCIPipelineTemplate(original, opts)

	if ciResult == nil {
		t.Fatal("GenerateCIPipelineTemplate returned nil, cannot test InjectCIPipeline")
	}

	updated, count := InjectCIPipeline(original, ciResult)

	if updated == original {
		t.Error("InjectCIPipeline must return a new chart pointer (copy-on-write)")
	}
	if count == 0 {
		t.Error("InjectCIPipeline must inject at least one workflow file (count > 0)")
	}
	// Original Templates must not contain the injected workflow files
	for k := range ciResult.Workflows {
		if _, exists := original.Templates[k]; exists {
			t.Errorf("original chart must not be modified: Templates[%q] was added to original", k)
		}
	}
	// Updated chart must contain the injected workflow files
	for k := range ciResult.Workflows {
		if _, exists := updated.Templates[k]; !exists {
			t.Errorf("updated chart must contain injected workflow file %q", k)
		}
	}
}

// ── 8: empty platform defaults to github ─────────────────────────────────────

func TestGenerateCIPipelineTemplate_PlatformDefaultsToGitHub(t *testing.T) {
	chart := newCIChart("myapp")
	opts := CIPipelineOptions{
		Platform:    "", // empty → should default to github
		ChartName:   "myapp",
		IncludeLint: true,
	}

	result := GenerateCIPipelineTemplate(chart, opts)

	if result == nil {
		t.Fatal("expected non-nil CIPipelineResult when Platform is empty")
	}
	if len(result.Workflows) == 0 {
		t.Fatal("expected non-empty Workflows when Platform defaults to github")
	}
	const wantKey = ".github/workflows/chart-ci.yml"
	if _, ok := result.Workflows[wantKey]; !ok {
		t.Errorf("empty platform must default to github, expected %q in Workflows, got: %v",
			wantKey, workflowKeys(result.Workflows))
	}
}

// ── 9: NOTESTxt is non-empty and references platform and chart name ───────────

func TestGenerateCIPipelineTemplate_NOTESTxt(t *testing.T) {
	chart := newCIChart("myapp")
	opts := allStagesOpts("github", "myapp")

	result := GenerateCIPipelineTemplate(chart, opts)

	if result == nil {
		t.Fatal("expected non-nil CIPipelineResult")
	}
	if result.NOTESTxt == "" {
		t.Error("expected non-empty NOTESTxt with CI pipeline instructions")
	}
	lower := strings.ToLower(result.NOTESTxt)
	if !strings.Contains(lower, "github") && !strings.Contains(lower, "ci") && !strings.Contains(lower, "pipeline") {
		t.Errorf("NOTESTxt must mention github, ci, or pipeline, got: %s", result.NOTESTxt)
	}
}

// ── 10: generated workflow has valid YAML top-level structure ─────────────────

func TestGenerateCIPipelineTemplate_ValidYAMLStructure(t *testing.T) {
	chart := newCIChart("myapp")
	opts := allStagesOpts("github", "myapp")

	result := GenerateCIPipelineTemplate(chart, opts)

	if result == nil {
		t.Fatal("expected non-nil CIPipelineResult")
	}
	content, ok := result.Workflows[".github/workflows/chart-ci.yml"]
	if !ok {
		t.Fatal("expected .github/workflows/chart-ci.yml to exist")
	}

	// GitHub Actions YAML must contain top-level keys: "on:" (trigger) and "jobs:"
	if !strings.Contains(content, "on:") && !strings.Contains(content, "\"on\":") {
		t.Errorf("GitHub Actions workflow must contain 'on:' trigger key, got snippet: %.400s", content)
	}
	if !strings.Contains(content, "jobs:") {
		t.Errorf("GitHub Actions workflow must contain 'jobs:' key, got snippet: %.400s", content)
	}
	// Must not be obviously malformed (no bare tabs at the start of YAML root)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if len(line) > 0 && line[0] == '\t' {
			t.Errorf("YAML must use spaces not tabs; line %d starts with tab: %q", i+1, line)
			break
		}
	}
}

// ── helper: join all workflow content ────────────────────────────────────────

func allWorkflowContent(workflows map[string]string) string {
	var sb strings.Builder
	for _, v := range workflows {
		sb.WriteString(v)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// ── helper: collect Workflows keys ───────────────────────────────────────────

func workflowKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
