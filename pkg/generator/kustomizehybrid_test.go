package generator

// ============================================================
// Test Plan — kustomizehybrid_test.go
//
// Tests for GenerateFluxPostBuild and InjectFluxPostBuild.
// Flux CD integration: HelmRelease + Kustomization with postBuild.
// NOTE: kustomize.go and postrenderer.go already exist.
//       This file covers FLUX-SPECIFIC integration only.
//
//  1. TestFluxPostBuild_GeneratesHelmRelease                — happy   valid chart+opts → HelmRelease map non-empty
//  2. TestFluxPostBuild_GeneratesKustomizationWithPostBuild — happy   Kustomization YAML contains "postBuild" key
//  3. TestFluxPostBuild_Substitutions_InKustomization       — happy   Substitutions map → YAML contains key and value
//  4. TestFluxPostBuild_SubstituteFrom_ConfigMap            — happy   SubstitutionsFrom ConfigMap → YAML contains "ConfigMap"
//  5. TestFluxPostBuild_SubstituteFrom_Secret               — happy   SubstitutionsFrom Secret → YAML contains "Secret"
//  6. TestFluxPostBuild_SourceRef_HelmRepository            — happy   Kind="HelmRepository" → HelmRelease YAML contains "HelmRepository"
//  7. TestFluxPostBuild_SourceRef_GitRepository             — happy   Kind="GitRepository" → HelmRelease YAML contains "GitRepository"
//  8. TestFluxPostBuild_CustomInterval_InHelmRelease        — happy   Interval="10m" → HelmRelease YAML contains "10m"
//  9. TestFluxPostBuild_EmptyReleaseName_ReturnsError       — error   ReleaseName="" → GenerateFluxPostBuild returns nil/error
// 10. TestFluxPostBuild_NilChart_InjectReturnsZero          — error   nil chart → count==0, no panic
// 11. TestFluxPostBuild_CopyOnWrite_OriginalUnchanged       — happy   InjectFluxPostBuild returns new chart, original unchanged
// 12. TestFluxPostBuild_Idempotent_DoubleInject             — happy   second inject does not exceed first inject count
// 13. TestFluxPostBuild_NOTESTxt_MentionsFlux               — happy   NOTESTxt contains case-insensitive "flux"
// ============================================================

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

// makeFluxChart returns a minimal GeneratedChart for FluxPostBuild tests.
func makeFluxChart(name string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name:      name,
		ChartYAML: "apiVersion: v2\nname: " + name + "\nversion: 0.1.0\n",
		ValuesYAML: "replicaCount: 1\n",
		Templates: map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\n",
		},
		Notes: "",
	}
}

// defaultFluxOpts returns a complete FluxPostBuildOptions with all fields set.
func defaultFluxOpts(releaseName, namespace string) FluxPostBuildOptions {
	return FluxPostBuildOptions{
		ReleaseName: releaseName,
		Namespace:   namespace,
		Interval:    "5m",
		SourceRef: FluxSourceRef{
			Kind:      "HelmRepository",
			Name:      "my-repo",
			Namespace: namespace,
		},
		Substitutions:    map[string]string{},
		SubstitutionsFrom: nil,
	}
}

// ─── 1. Generates HelmRelease ────────────────────────────────────────────────

func TestFluxPostBuild_GeneratesHelmRelease(t *testing.T) {
	chart := makeFluxChart("myapp")
	opts := defaultFluxOpts("myapp", "default")

	result := GenerateFluxPostBuild(chart, opts)
	if result == nil {
		t.Fatal("GenerateFluxPostBuild returned nil result")
	}
	if len(result.HelmRelease) == 0 {
		t.Error("expected HelmRelease map to be non-empty, got empty map")
	}
}

// ─── 2. Kustomization contains postBuild ────────────────────────────────────

func TestFluxPostBuild_GeneratesKustomizationWithPostBuild(t *testing.T) {
	chart := makeFluxChart("myapp")
	opts := defaultFluxOpts("myapp", "default")

	result := GenerateFluxPostBuild(chart, opts)
	if result == nil {
		t.Fatal("GenerateFluxPostBuild returned nil result")
	}
	if len(result.Kustomization) == 0 {
		t.Fatal("expected Kustomization map to be non-empty")
	}

	found := false
	for _, yaml := range result.Kustomization {
		if strings.Contains(yaml, "postBuild") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'postBuild' key in at least one Kustomization YAML entry, got:\n%v",
			result.Kustomization)
	}
}

// ─── 3. Substitutions appear in Kustomization ───────────────────────────────

func TestFluxPostBuild_Substitutions_InKustomization(t *testing.T) {
	chart := makeFluxChart("myapp")
	opts := defaultFluxOpts("myapp", "default")
	opts.Substitutions = map[string]string{
		"ENV":     "prod",
		"CLUSTER": "eu-west-1",
	}

	result := GenerateFluxPostBuild(chart, opts)
	if result == nil {
		t.Fatal("GenerateFluxPostBuild returned nil result")
	}

	combined := joinYAMLValues(result.Kustomization)

	for k, v := range opts.Substitutions {
		if !strings.Contains(combined, k) {
			t.Errorf("expected substitution key %q to appear in Kustomization YAML", k)
		}
		if !strings.Contains(combined, v) {
			t.Errorf("expected substitution value %q to appear in Kustomization YAML", v)
		}
	}
}

// ─── 4. SubstituteFrom ConfigMap in Kustomization ───────────────────────────

func TestFluxPostBuild_SubstituteFrom_ConfigMap(t *testing.T) {
	chart := makeFluxChart("myapp")
	opts := defaultFluxOpts("myapp", "default")
	opts.SubstitutionsFrom = []FluxSubstitutionSource{
		{Kind: "ConfigMap", Name: "env-config"},
	}

	result := GenerateFluxPostBuild(chart, opts)
	if result == nil {
		t.Fatal("GenerateFluxPostBuild returned nil result")
	}

	combined := joinYAMLValues(result.Kustomization)
	if !strings.Contains(combined, "ConfigMap") {
		t.Errorf("expected 'ConfigMap' in Kustomization YAML when SubstitutionsFrom includes a ConfigMap, got:\n%s",
			combined)
	}
	if !strings.Contains(combined, "env-config") {
		t.Errorf("expected ConfigMap name 'env-config' in Kustomization YAML, got:\n%s", combined)
	}
}

// ─── 5. SubstituteFrom Secret in Kustomization ──────────────────────────────

func TestFluxPostBuild_SubstituteFrom_Secret(t *testing.T) {
	chart := makeFluxChart("myapp")
	opts := defaultFluxOpts("myapp", "default")
	opts.SubstitutionsFrom = []FluxSubstitutionSource{
		{Kind: "Secret", Name: "cluster-secrets"},
	}

	result := GenerateFluxPostBuild(chart, opts)
	if result == nil {
		t.Fatal("GenerateFluxPostBuild returned nil result")
	}

	combined := joinYAMLValues(result.Kustomization)
	if !strings.Contains(combined, "Secret") {
		t.Errorf("expected 'Secret' in Kustomization YAML when SubstitutionsFrom includes a Secret, got:\n%s",
			combined)
	}
	if !strings.Contains(combined, "cluster-secrets") {
		t.Errorf("expected Secret name 'cluster-secrets' in Kustomization YAML, got:\n%s", combined)
	}
}

// ─── 6. SourceRef HelmRepository ────────────────────────────────────────────

func TestFluxPostBuild_SourceRef_HelmRepository(t *testing.T) {
	chart := makeFluxChart("myapp")
	opts := defaultFluxOpts("myapp", "default")
	opts.SourceRef = FluxSourceRef{
		Kind:      "HelmRepository",
		Name:      "my-helm-repo",
		Namespace: "flux-system",
	}

	result := GenerateFluxPostBuild(chart, opts)
	if result == nil {
		t.Fatal("GenerateFluxPostBuild returned nil result")
	}

	combined := joinYAMLValues(result.HelmRelease)
	if !strings.Contains(combined, "HelmRepository") {
		t.Errorf("expected 'HelmRepository' in HelmRelease YAML for SourceRef.Kind=HelmRepository, got:\n%s",
			combined)
	}
}

// ─── 7. SourceRef GitRepository ─────────────────────────────────────────────

func TestFluxPostBuild_SourceRef_GitRepository(t *testing.T) {
	chart := makeFluxChart("myapp")
	opts := defaultFluxOpts("myapp", "default")
	opts.SourceRef = FluxSourceRef{
		Kind:      "GitRepository",
		Name:      "my-git-repo",
		Namespace: "flux-system",
	}

	result := GenerateFluxPostBuild(chart, opts)
	if result == nil {
		t.Fatal("GenerateFluxPostBuild returned nil result")
	}

	combined := joinYAMLValues(result.HelmRelease)
	if !strings.Contains(combined, "GitRepository") {
		t.Errorf("expected 'GitRepository' in HelmRelease YAML for SourceRef.Kind=GitRepository, got:\n%s",
			combined)
	}
}

// ─── 8. Custom interval in HelmRelease ──────────────────────────────────────

func TestFluxPostBuild_CustomInterval_InHelmRelease(t *testing.T) {
	chart := makeFluxChart("myapp")
	opts := defaultFluxOpts("myapp", "default")
	opts.Interval = "10m"

	result := GenerateFluxPostBuild(chart, opts)
	if result == nil {
		t.Fatal("GenerateFluxPostBuild returned nil result")
	}

	combined := joinYAMLValues(result.HelmRelease)
	if !strings.Contains(combined, "10m") {
		t.Errorf("expected custom interval '10m' in HelmRelease YAML, got:\n%s", combined)
	}
}

// ─── 9. Empty ReleaseName → error ───────────────────────────────────────────

func TestFluxPostBuild_EmptyReleaseName_ReturnsError(t *testing.T) {
	chart := makeFluxChart("myapp")
	opts := defaultFluxOpts("", "default") // empty ReleaseName

	result := GenerateFluxPostBuild(chart, opts)
	if result != nil {
		// If the implementation returns a non-nil result, the HelmRelease and
		// Kustomization maps must both be empty — an empty release name is not
		// a valid Flux resource.
		if len(result.HelmRelease) > 0 || len(result.Kustomization) > 0 {
			t.Errorf("expected empty HelmRelease and Kustomization for empty ReleaseName, "+
				"got HelmRelease=%d entries, Kustomization=%d entries",
				len(result.HelmRelease), len(result.Kustomization))
		}
	}
	// If result is nil, the implementation signalled an error by returning nil — that is acceptable.
	// The contract requires: no valid CRDs generated when ReleaseName is empty.
}

// ─── 10. Nil chart → InjectFluxPostBuild returns zero ───────────────────────

func TestFluxPostBuild_NilChart_InjectReturnsZero(t *testing.T) {
	opts := defaultFluxOpts("myapp", "default")
	result := GenerateFluxPostBuild(makeFluxChart("myapp"), opts)

	// Must not panic; must return count 0 and nil chart.
	updated, count := InjectFluxPostBuild(nil, result)
	if count != 0 {
		t.Errorf("expected count==0 for nil chart, got %d", count)
	}
	if updated != nil {
		t.Errorf("expected nil chart returned for nil input, got non-nil")
	}
}

// ─── 11. Copy-on-write: original chart unchanged ─────────────────────────────

func TestFluxPostBuild_CopyOnWrite_OriginalUnchanged(t *testing.T) {
	chart := makeFluxChart("myapp")
	originalNotes := chart.Notes
	originalTemplateCount := len(chart.Templates)

	opts := defaultFluxOpts("myapp", "default")
	result := GenerateFluxPostBuild(chart, opts)
	if result == nil {
		t.Fatal("GenerateFluxPostBuild returned nil result")
	}

	updated, count := InjectFluxPostBuild(chart, result)
	if count == 0 {
		t.Skip("no injection performed — skipping copy-on-write check")
	}

	// Original chart must be unchanged.
	if chart.Notes != originalNotes {
		t.Errorf("original chart.Notes was modified: got %q, want %q", chart.Notes, originalNotes)
	}
	if len(chart.Templates) != originalTemplateCount {
		t.Errorf("original chart.Templates was modified: len=%d, want %d",
			len(chart.Templates), originalTemplateCount)
	}

	// Updated chart must be a different pointer.
	if updated == chart {
		t.Error("InjectFluxPostBuild returned the same chart pointer — expected a copy")
	}
}

// ─── 12. Idempotent: second inject does not exceed first inject count ────────

func TestFluxPostBuild_Idempotent_DoubleInject(t *testing.T) {
	chart := makeFluxChart("myapp")
	opts := defaultFluxOpts("myapp", "default")
	result := GenerateFluxPostBuild(chart, opts)
	if result == nil {
		t.Fatal("GenerateFluxPostBuild returned nil result")
	}

	first, count1 := InjectFluxPostBuild(chart, result)
	if first == nil || count1 == 0 {
		t.Skip("first inject produced no changes — skipping idempotency check")
	}

	second, count2 := InjectFluxPostBuild(first, result)
	if second == nil {
		t.Fatal("second inject returned nil chart")
	}

	// Idempotent: second inject must not add more items than the first.
	if count2 > count1 {
		t.Errorf("second inject produced more items (%d) than first (%d) — not idempotent",
			count2, count1)
	}
}

// ─── 13. NOTESTxt mentions flux ─────────────────────────────────────────────

func TestFluxPostBuild_NOTESTxt_MentionsFlux(t *testing.T) {
	chart := makeFluxChart("myapp")
	opts := defaultFluxOpts("myapp", "default")

	result := GenerateFluxPostBuild(chart, opts)
	if result == nil {
		t.Fatal("GenerateFluxPostBuild returned nil result")
	}
	if !strings.Contains(strings.ToLower(result.NOTESTxt), "flux") {
		t.Errorf("expected NOTESTxt to mention 'flux' (case-insensitive), got: %q", result.NOTESTxt)
	}
}

// ─── utility ─────────────────────────────────────────────────────────────────

// joinYAMLValues concatenates all values in a map[string]string for substring searches.
func joinYAMLValues(m map[string]string) string {
	var b strings.Builder
	for _, v := range m {
		b.WriteString(v)
		b.WriteByte('\n')
	}
	return b.String()
}
