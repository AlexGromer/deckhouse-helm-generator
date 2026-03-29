package generator

// Test Plan: postrenderer_test.go
//
// | #  | Test Name                                         | Category    | Input                                           | Expected Output                                      | Notes                                   |
// |----|---------------------------------------------------|-------------|-------------------------------------------------|------------------------------------------------------|-----------------------------------------|
// | 1  | GenerateLayout_NilChart_ReturnsError              | error       | nil chart                                       | error returned, no panic                             | nil safety                              |
// | 2  | GenerateLayout_EmptyNamespace_Defaults            | edge        | opts with empty Namespace                       | output non-nil, no error                             | optional namespace                      |
// | 3  | GenerateLayout_SingleEnv_SingleOverlay            | happy       | opts with Envs=["dev"]                          | Output.Overlays has 1 entry                          | baseline overlay count                  |
// | 4  | GenerateLayout_ThreeEnvs_ThreeOverlays            | happy       | opts with Envs=["dev","staging","prod"]         | Output.Overlays has 3 entries                        | standard 3-env layout                   |
// | 5  | GenerateLayout_OutputContainsBaseDir              | happy       | valid chart + opts                              | Output.BaseDir is non-empty                          | directory structure                     |
// | 6  | RenderOverlayKustomization_NilOverlay_ReturnsError| error       | nil overlay                                     | error returned, no panic                             | nil safety                              |
// | 7  | RenderOverlayKustomization_StrategicMergePatch    | happy       | overlay with StrategicMergePatches              | rendered YAML contains patch filename                | strategic merge rendering               |
// | 8  | RenderOverlayKustomization_JSON6902Patch          | happy       | overlay with JSON6902 patches                   | rendered YAML contains json6902 reference            | JSON patch rendering                    |
// | 9  | RenderOverlayKustomization_BothPatchTypes         | happy       | overlay with both patch types                   | rendered YAML contains both filenames                | mixed patch rendering                   |
// | 10 | InjectPostRenderer_NilChart_ReturnsError          | error       | nil chart pointer                               | error, count=0                                       | nil safety                              |
// | 11 | InjectPostRenderer_ReturnsCount                   | happy       | chart + valid opts                              | count > 0, no error                                  | inject file count                       |
// | 12 | InjectPostRenderer_FilesAppendedToExternalFiles   | happy       | chart with empty ExternalFiles                  | updated chart.ExternalFiles len > 0                  | files injected                          |
// | 13 | BuildDefaultOptions_NamespaceSet                  | happy       | chart name "myapp", namespace "prod"            | opts.Namespace = "prod"                              | default option construction             |
// | 14 | BuildDefaultOptions_EnvsPresent                   | happy       | chart + namespace                               | opts.Envs contains at least one env                  | env defaults                            |
// | 15 | ValidateOptions_MissingChartName_ReturnsError     | error       | opts with empty ChartName                       | ValidatePostRendererOptions returns error             | required field validation               |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func makePostRendererChart(name string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name:       name,
		ChartYAML:  "apiVersion: v2\nname: " + name + "\nversion: 0.1.0\n",
		ValuesYAML: "replicaCount: 1\n",
		Templates: map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\n",
		},
	}
}

func makeDefaultPostRendererOpts(chartName, namespace string) PostRendererOptions {
	return PostRendererOptions{
		ChartName: chartName,
		Namespace: namespace,
		Envs:      []PostRendererEnv{PostRendererEnvDev, PostRendererEnvStaging, PostRendererEnvProd},
	}
}

// ── 1. GenerateLayout_NilChart_ReturnsError ───────────────────────────────────

func TestPostRenderer_GenerateLayout_NilChart_ReturnsError(t *testing.T) {
	opts := makeDefaultPostRendererOpts("myapp", "default")

	out, err := GeneratePostRendererLayout(nil, opts)
	if err == nil {
		t.Error("expected error for nil chart, got nil")
	}
	if out != nil {
		t.Errorf("expected nil output on error, got %+v", out)
	}
}

// ── 2. GenerateLayout_EmptyNamespace_Defaults ─────────────────────────────────

func TestPostRenderer_GenerateLayout_EmptyNamespace_Succeeds(t *testing.T) {
	chart := makePostRendererChart("myapp")
	opts := PostRendererOptions{
		ChartName: "myapp",
		Namespace: "",
		Envs:      []PostRendererEnv{PostRendererEnvDev},
	}

	out, err := GeneratePostRendererLayout(chart, opts)
	if err != nil {
		t.Fatalf("unexpected error with empty namespace: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
}

// ── 3. GenerateLayout_SingleEnv_SingleOverlay ─────────────────────────────────

func TestPostRenderer_GenerateLayout_SingleEnv_SingleOverlay(t *testing.T) {
	chart := makePostRendererChart("myapp")
	opts := PostRendererOptions{
		ChartName: "myapp",
		Namespace: "default",
		Envs:      []PostRendererEnv{PostRendererEnvDev},
	}

	out, err := GeneratePostRendererLayout(chart, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Overlays) != 1 {
		t.Errorf("expected 1 overlay, got %d", len(out.Overlays))
	}
}

// ── 4. GenerateLayout_ThreeEnvs_ThreeOverlays ────────────────────────────────

func TestPostRenderer_GenerateLayout_ThreeEnvs_ThreeOverlays(t *testing.T) {
	chart := makePostRendererChart("myapp")
	opts := makeDefaultPostRendererOpts("myapp", "default")

	out, err := GeneratePostRendererLayout(chart, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Overlays) != 3 {
		t.Errorf("expected 3 overlays, got %d", len(out.Overlays))
	}
	for _, env := range []PostRendererEnv{PostRendererEnvDev, PostRendererEnvStaging, PostRendererEnvProd} {
		found := false
		for _, o := range out.Overlays {
			if o.Env == env {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected overlay for env %q not found", env)
		}
	}
}

// ── 5. GenerateLayout_OutputContainsBaseDir ───────────────────────────────────

func TestPostRenderer_GenerateLayout_OutputContainsBaseDir(t *testing.T) {
	chart := makePostRendererChart("myapp")
	opts := makeDefaultPostRendererOpts("myapp", "default")

	out, err := GeneratePostRendererLayout(chart, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.BaseDir == "" {
		t.Error("expected non-empty BaseDir in PostRendererOutput")
	}
}

// ── 6. RenderOverlayKustomization_NilOverlay_ReturnsError ────────────────────

func TestPostRenderer_RenderOverlayKustomization_NilOverlay_ReturnsError(t *testing.T) {
	out, err := RenderOverlayKustomization(nil)
	if err == nil {
		t.Error("expected error for nil overlay, got nil")
	}
	if out != "" {
		t.Errorf("expected empty string on error, got %q", out)
	}
}

// ── 7. RenderOverlayKustomization_StrategicMergePatch ────────────────────────

func TestPostRenderer_RenderOverlayKustomization_StrategicMergePatch(t *testing.T) {
	patchFile := "replica-patch.yaml"
	overlay := &PostRendererOverlay{
		Env:       PostRendererEnvDev,
		ChartName: "myapp",
		Namespace: "default",
		StrategicMergePatches: []StrategicMergePatch{
			{FileName: patchFile, Content: "apiVersion: apps/v1\nkind: Deployment\n"},
		},
	}

	out, err := RenderOverlayKustomization(overlay)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, patchFile) {
		t.Errorf("expected patch filename %q in kustomization output, got:\n%s", patchFile, out)
	}
}

// ── 8. RenderOverlayKustomization_JSON6902Patch ───────────────────────────────

func TestPostRenderer_RenderOverlayKustomization_JSON6902Patch(t *testing.T) {
	patchFile := "add-label-patch.yaml"
	overlay := &PostRendererOverlay{
		Env:       PostRendererEnvProd,
		ChartName: "myapp",
		Namespace: "default",
		JSON6902Patches: []JSON6902Patch{
			{
				FileName: patchFile,
				Target: JSON6902Target{
					Group:     "apps",
					Version:   "v1",
					Kind:      "Deployment",
					Name:      "myapp",
					Namespace: "default",
				},
				Ops: []JSON6902Op{
					{Op: "add", Path: "/metadata/labels/env", Value: "prod"},
				},
			},
		},
	}

	out, err := RenderOverlayKustomization(overlay)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, patchFile) {
		t.Errorf("expected patch filename %q in kustomization output, got:\n%s", patchFile, out)
	}
}

// ── 9. RenderOverlayKustomization_BothPatchTypes ─────────────────────────────

func TestPostRenderer_RenderOverlayKustomization_BothPatchTypes_BothPresent(t *testing.T) {
	smFile := "strategic-patch.yaml"
	jsonFile := "json6902-patch.yaml"
	overlay := &PostRendererOverlay{
		Env:       PostRendererEnvStaging,
		ChartName: "myapp",
		Namespace: "staging",
		StrategicMergePatches: []StrategicMergePatch{
			{FileName: smFile, Content: "apiVersion: apps/v1\nkind: Deployment\n"},
		},
		JSON6902Patches: []JSON6902Patch{
			{
				FileName: jsonFile,
				Target:   JSON6902Target{Kind: "Deployment", Name: "myapp"},
				Ops:      []JSON6902Op{{Op: "replace", Path: "/spec/replicas", Value: 2}},
			},
		},
	}

	out, err := RenderOverlayKustomization(overlay)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, smFile) {
		t.Errorf("expected strategic merge patch file %q in output, got:\n%s", smFile, out)
	}
	if !strings.Contains(out, jsonFile) {
		t.Errorf("expected JSON6902 patch file %q in output, got:\n%s", jsonFile, out)
	}
}

// ── 10. InjectPostRenderer_NilChart_ReturnsError ─────────────────────────────

func TestPostRenderer_InjectPostRenderer_NilChart_ReturnsError(t *testing.T) {
	opts := makeDefaultPostRendererOpts("myapp", "default")

	result, count, err := InjectPostRenderer(nil, opts)
	if err == nil {
		t.Error("expected error for nil chart, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on error, got %+v", result)
	}
	if count != 0 {
		t.Errorf("expected count=0 on error, got %d", count)
	}
}

// ── 11. InjectPostRenderer_ReturnsPositiveCount ───────────────────────────────

func TestPostRenderer_InjectPostRenderer_ReturnsPositiveCount(t *testing.T) {
	chart := makePostRendererChart("myapp")
	opts := makeDefaultPostRendererOpts("myapp", "default")

	_, count, err := InjectPostRenderer(chart, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count <= 0 {
		t.Errorf("expected count > 0, got %d", count)
	}
}

// ── 12. InjectPostRenderer_FilesAppendedToExternalFiles ───────────────────────

func TestPostRenderer_InjectPostRenderer_FilesAppendedToExternalFiles(t *testing.T) {
	chart := makePostRendererChart("myapp")
	if len(chart.ExternalFiles) != 0 {
		t.Fatalf("pre-condition: expected empty ExternalFiles, got %d", len(chart.ExternalFiles))
	}
	opts := makeDefaultPostRendererOpts("myapp", "default")

	updated, _, err := InjectPostRenderer(chart, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updated.ExternalFiles) == 0 {
		t.Error("expected ExternalFiles to be populated after inject, got empty slice")
	}
}

// ── 13. BuildDefaultOptions_NamespaceSet ─────────────────────────────────────

func TestPostRenderer_BuildDefaultOptions_NamespaceSet(t *testing.T) {
	chart := makePostRendererChart("myapp")
	opts := BuildDefaultPostRendererOptions(chart, "production")

	if opts.Namespace != "production" {
		t.Errorf("expected Namespace=%q, got %q", "production", opts.Namespace)
	}
}

// ── 14. BuildDefaultOptions_EnvsPresent ──────────────────────────────────────

func TestPostRenderer_BuildDefaultOptions_EnvsPresent(t *testing.T) {
	chart := makePostRendererChart("myapp")
	opts := BuildDefaultPostRendererOptions(chart, "default")

	if len(opts.Envs) == 0 {
		t.Error("expected BuildDefaultPostRendererOptions to populate Envs with at least one environment")
	}
}

// ── 15. ValidateOptions_MissingChartName_ReturnsError ────────────────────────

func TestPostRenderer_ValidateOptions_MissingChartName_ReturnsError(t *testing.T) {
	opts := PostRendererOptions{
		ChartName: "",
		Namespace: "default",
		Envs:      []PostRendererEnv{PostRendererEnvDev},
	}

	errs := ValidatePostRendererOptions(opts)
	if len(errs) == 0 {
		t.Error("expected ValidatePostRendererOptions to return at least 1 error for empty ChartName, got none")
	}

	// At least one error should mention the chart name field.
	mentionsChart := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "ChartName") || strings.Contains(e.Error(), "chart") || strings.Contains(e.Error(), "name") {
			mentionsChart = true
			break
		}
	}
	if !mentionsChart {
		t.Errorf("expected an error message referencing the missing chart name, got: %v", errs)
	}
}

// ── 16. RenderOverlayKustomization_BothPatchTypes_SinglePatchesKey (regression CR-2) ─
// Ensures the rendered YAML contains exactly one "patches:" key when both
// StrategicMergePatches and JSON6902Patches are present. Two "patches:" keys
// would produce invalid YAML.

func TestPostRenderer_RenderOverlayKustomization_BothPatchTypes_SinglePatchesKey(t *testing.T) {
	overlay := &PostRendererOverlay{
		Env:       PostRendererEnvStaging,
		ChartName: "myapp",
		Namespace: "staging",
		StrategicMergePatches: []StrategicMergePatch{
			{FileName: "strategic.yaml", Content: ""},
		},
		JSON6902Patches: []JSON6902Patch{
			{FileName: "json6902.yaml", Target: JSON6902Target{Kind: "Deployment", Name: "myapp"}},
		},
	}

	out, err := RenderOverlayKustomization(overlay)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	count := strings.Count(out, "patches:")
	if count != 1 {
		t.Errorf("expected exactly 1 'patches:' key in kustomization YAML, got %d\noutput:\n%s", count, out)
	}
}
