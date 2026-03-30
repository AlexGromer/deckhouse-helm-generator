package generator

import (
	"os"
	"path/filepath"
	"testing"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// populateOverrideDir writes a set of files (name → content) into dir and
// returns the dir path. The files simulate user-provided template overrides.
func populateOverrideDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("populateOverrideDir: write %s: %v", name, err)
		}
	}
	return dir
}

// ── Test 1: MergeTemplateOverrides — "override" replaces generated content ───

func TestMergeTemplateOverrides_OverrideReplacesGenerated(t *testing.T) {
	generated := map[string]string{
		"templates/deploy.yaml": "# generated deployment",
		"templates/service.yaml": "# generated service",
	}
	overrides := map[string]string{
		"templates/deploy.yaml": "# user override deployment",
	}

	result := MergeTemplateOverrides(generated, overrides, string(MergeStrategyOverride))

	if got := result["templates/deploy.yaml"]; got != "# user override deployment" {
		t.Errorf("deploy.yaml = %q, want %q", got, "# user override deployment")
	}
	// Non-overridden template must be preserved.
	if got := result["templates/service.yaml"]; got != "# generated service" {
		t.Errorf("service.yaml = %q, want %q (should be unchanged)", got, "# generated service")
	}
}

// ── Test 2: MergeTemplateOverrides — "append" adds override after generated ──

func TestMergeTemplateOverrides_AppendAddsToEnd(t *testing.T) {
	generated := map[string]string{
		"templates/deploy.yaml": "# generated\n",
	}
	overrides := map[string]string{
		"templates/deploy.yaml": "# appended\n",
	}

	result := MergeTemplateOverrides(generated, overrides, string(MergeStrategyAppend))

	got := result["templates/deploy.yaml"]
	// Generated content must appear before override content.
	const wantPrefix = "# generated\n"
	const wantSuffix = "# appended\n"
	if len(got) < len(wantPrefix)+len(wantSuffix) {
		t.Fatalf("append result too short: %q", got)
	}
	if got[:len(wantPrefix)] != wantPrefix {
		t.Errorf("append: content does not start with generated: %q", got)
	}
	if got[len(got)-len(wantSuffix):] != wantSuffix {
		t.Errorf("append: content does not end with override: %q", got)
	}
}

// ── Test 3: MergeTemplateOverrides — "prepend" adds override before generated ─

func TestMergeTemplateOverrides_PrependAddsToStart(t *testing.T) {
	generated := map[string]string{
		"templates/deploy.yaml": "# generated\n",
	}
	overrides := map[string]string{
		"templates/deploy.yaml": "# prepended\n",
	}

	result := MergeTemplateOverrides(generated, overrides, string(MergeStrategyPrepend))

	got := result["templates/deploy.yaml"]
	const wantPrefix = "# prepended\n"
	const wantSuffix = "# generated\n"
	if len(got) < len(wantPrefix)+len(wantSuffix) {
		t.Fatalf("prepend result too short: %q", got)
	}
	if got[:len(wantPrefix)] != wantPrefix {
		t.Errorf("prepend: content does not start with override: %q", got)
	}
	if got[len(got)-len(wantSuffix):] != wantSuffix {
		t.Errorf("prepend: content does not end with generated: %q", got)
	}
}

// ── Test 4: LoadTemplateOverrides — no override dir returns empty map ─────────

func TestLoadTemplateOverrides_NoDir(t *testing.T) {
	overrides, err := LoadTemplateOverrides("")
	if err != nil {
		t.Fatalf("unexpected error for empty path: %v", err)
	}
	if len(overrides) != 0 {
		t.Errorf("expected empty map, got %d entries", len(overrides))
	}
}

// ── Test 5: LoadTemplateOverrides — non-existent directory returns error ──────

func TestLoadTemplateOverrides_InvalidDir(t *testing.T) {
	_, err := LoadTemplateOverrides("/nonexistent/template/override/dir-404")
	if err == nil {
		t.Fatal("expected error for non-existent directory, got nil")
	}
}

// ── Test 6: MergeTemplateOverrides — override only applies to matching keys ───

func TestMergeTemplateOverrides_OnlyMatchingKeysOverridden(t *testing.T) {
	generated := map[string]string{
		"templates/a.yaml": "# generated-a",
		"templates/b.yaml": "# generated-b",
		"templates/c.yaml": "# generated-c",
	}
	overrides := map[string]string{
		"templates/b.yaml": "# override-b",
	}

	result := MergeTemplateOverrides(generated, overrides, string(MergeStrategyOverride))

	if result["templates/a.yaml"] != "# generated-a" {
		t.Errorf("a.yaml unexpectedly changed: %q", result["templates/a.yaml"])
	}
	if result["templates/b.yaml"] != "# override-b" {
		t.Errorf("b.yaml = %q, want %q", result["templates/b.yaml"], "# override-b")
	}
	if result["templates/c.yaml"] != "# generated-c" {
		t.Errorf("c.yaml unexpectedly changed: %q", result["templates/c.yaml"])
	}
}

// ── Test 7: MergeTemplateOverrides — unmatched overrides are added to result ──

func TestMergeTemplateOverrides_UnmatchedOverridesAdded(t *testing.T) {
	generated := map[string]string{
		"templates/existing.yaml": "# existing",
	}
	overrides := map[string]string{
		"templates/brand-new.yaml": "# brand new",
	}

	result := MergeTemplateOverrides(generated, overrides, string(MergeStrategyOverride))

	if result["templates/existing.yaml"] != "# existing" {
		t.Errorf("existing.yaml unexpectedly changed: %q", result["templates/existing.yaml"])
	}
	if got, ok := result["templates/brand-new.yaml"]; !ok {
		t.Error("brand-new.yaml should be added to result")
	} else if got != "# brand new" {
		t.Errorf("brand-new.yaml = %q, want %q", got, "# brand new")
	}
}

// ── Test 8: MergeTemplateOverrides — nil generated map is treated as empty ───

func TestMergeTemplateOverrides_NilGenerated(t *testing.T) {
	overrides := map[string]string{
		"templates/new.yaml": "# override only",
	}

	result := MergeTemplateOverrides(nil, overrides, string(MergeStrategyOverride))

	if result == nil {
		t.Fatal("expected non-nil result map")
	}
	if result["templates/new.yaml"] != "# override only" {
		t.Errorf("new.yaml = %q, want %q", result["templates/new.yaml"], "# override only")
	}
}

// ── Test 9: MergeTemplateOverrides — empty strategy defaults to override ──────

func TestMergeTemplateOverrides_EmptyStrategyDefaultsToOverride(t *testing.T) {
	generated := map[string]string{
		"templates/deploy.yaml": "# generated",
	}
	overrides := map[string]string{
		"templates/deploy.yaml": "# overridden",
	}

	// Empty string strategy must behave like "override".
	result := MergeTemplateOverrides(generated, overrides, "")

	if result["templates/deploy.yaml"] != "# overridden" {
		t.Errorf("empty strategy: deploy.yaml = %q, want %q", result["templates/deploy.yaml"], "# overridden")
	}
}

// ── Test 10: MergeTemplateOverrides — non-overridden templates preserved ──────

func TestMergeTemplateOverrides_PreservesNonOverriddenTemplates(t *testing.T) {
	generated := map[string]string{
		"templates/alpha.yaml":   "# alpha",
		"templates/beta.yaml":    "# beta",
		"templates/gamma.yaml":   "# gamma",
		"templates/delta.yaml":   "# delta",
	}
	overrides := map[string]string{
		"templates/beta.yaml": "# beta-override",
	}

	for _, strategy := range []string{
		string(MergeStrategyOverride),
		string(MergeStrategyAppend),
		string(MergeStrategyPrepend),
	} {
		t.Run("strategy="+strategy, func(t *testing.T) {
			result := MergeTemplateOverrides(generated, overrides, strategy)

			// alpha, gamma, delta must be untouched regardless of strategy.
			for _, key := range []string{"templates/alpha.yaml", "templates/gamma.yaml", "templates/delta.yaml"} {
				if result[key] != generated[key] {
					t.Errorf("[%s] %s = %q, want %q", strategy, key, result[key], generated[key])
				}
			}
			// beta must be modified.
			if result["templates/beta.yaml"] == "# beta" {
				t.Errorf("[%s] beta.yaml was not modified by override", strategy)
			}
		})
	}
}

// ── Test 11 (bonus): LoadTemplateOverrides — loads files from directory ───────

func TestLoadTemplateOverrides_LoadsFilesFromDir(t *testing.T) {
	files := map[string]string{
		"deploy.yaml":  "# deploy override",
		"service.yaml": "# service override",
	}
	dir := populateOverrideDir(t, files)

	overrides, err := LoadTemplateOverrides(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(overrides) != 2 {
		t.Errorf("expected 2 overrides, got %d", len(overrides))
	}

	for name, want := range files {
		// Keys may be bare names or full paths — check both.
		found := false
		for k, v := range overrides {
			base := filepath.Base(k)
			if base == name || k == name {
				found = true
				if v != want {
					t.Errorf("override[%s] = %q, want %q", name, v, want)
				}
				break
			}
		}
		if !found {
			t.Errorf("override for %q not found in result: %v", name, overrides)
		}
	}
}
