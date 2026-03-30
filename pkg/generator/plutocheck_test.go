package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

const legacyIngressTemplate = `apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: my-ingress
spec:
  rules:
  - host: example.com`

const legacyPDBTemplate = `apiVersion: policy/v1beta1
kind: PodDisruptionBudget
metadata:
  name: my-pdb
spec:
  minAvailable: 1`

const modernDeploymentTemplate = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: modern-app
spec:
  replicas: 1`

func newPlutoChart(templates map[string]string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name:      "pluto-test",
		Templates: templates,
	}
}

// ── Test 1: detect extensions/v1beta1 Ingress ────────────────────────────────

func TestCheckDeprecatedAPIs_DetectsExtensionsV1Beta1Ingress(t *testing.T) {
	chart := newPlutoChart(map[string]string{
		"templates/ingress.yaml": legacyIngressTemplate,
	})
	opts := PlutoCheckOptions{TargetVersion: "1.22"}

	result := CheckDeprecatedAPIs(chart, opts)

	if result == nil {
		t.Fatal("expected non-nil PlutoCheckResult")
	}
	if !result.HasDeprecations {
		t.Error("expected HasDeprecations=true for extensions/v1beta1 Ingress")
	}
	if len(result.Deprecations) == 0 {
		t.Fatal("expected at least one deprecation entry")
	}
	dep := result.Deprecations[0]
	if dep.APIVersion != "extensions/v1beta1" {
		t.Errorf("expected APIVersion=extensions/v1beta1, got %q", dep.APIVersion)
	}
	if dep.Kind != "Ingress" {
		t.Errorf("expected Kind=Ingress, got %q", dep.Kind)
	}
}

// ── Test 2: detect policy/v1beta1 PodDisruptionBudget ────────────────────────

func TestCheckDeprecatedAPIs_DetectsPolicyV1Beta1PDB(t *testing.T) {
	chart := newPlutoChart(map[string]string{
		"templates/pdb.yaml": legacyPDBTemplate,
	})
	opts := PlutoCheckOptions{TargetVersion: "1.25"}

	result := CheckDeprecatedAPIs(chart, opts)

	if !result.HasDeprecations {
		t.Error("expected HasDeprecations=true for policy/v1beta1 PodDisruptionBudget")
	}
	found := false
	for _, d := range result.Deprecations {
		if d.Kind == "PodDisruptionBudget" && d.APIVersion == "policy/v1beta1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected PodDisruptionBudget/policy/v1beta1 in Deprecations")
	}
}

// ── Test 3: no deprecations for modern APIs ───────────────────────────────────

func TestCheckDeprecatedAPIs_NoDeprecationsForModernAPIs(t *testing.T) {
	chart := newPlutoChart(map[string]string{
		"templates/deployment.yaml": modernDeploymentTemplate,
	})
	opts := PlutoCheckOptions{TargetVersion: "1.29"}

	result := CheckDeprecatedAPIs(chart, opts)

	if result.HasDeprecations {
		t.Errorf("expected no deprecations for modern chart, got %d", len(result.Deprecations))
	}
	if len(result.Deprecations) != 0 {
		t.Errorf("expected empty Deprecations slice, got %v", result.Deprecations)
	}
}

// ── Test 4: target version filtering — API deprecated but not removed yet ─────

func TestCheckDeprecatedAPIs_TargetVersionFiltering(t *testing.T) {
	// extensions/v1beta1 Ingress was removed in 1.22.
	// With target 1.20 the API exists but is deprecated.
	chart := newPlutoChart(map[string]string{
		"templates/ingress.yaml": legacyIngressTemplate,
	})
	opts := PlutoCheckOptions{TargetVersion: "1.20"}

	result := CheckDeprecatedAPIs(chart, opts)

	// Must still report as deprecated (deprecated in 1.14, removed in 1.22)
	if !result.HasDeprecations {
		t.Error("expected deprecation reported even when API is not yet removed at target version")
	}
}

// ── Test 5: multiple deprecations across templates ────────────────────────────

func TestCheckDeprecatedAPIs_MultipleDeprecations(t *testing.T) {
	chart := newPlutoChart(map[string]string{
		"templates/ingress.yaml": legacyIngressTemplate,
		"templates/pdb.yaml":     legacyPDBTemplate,
	})
	opts := PlutoCheckOptions{TargetVersion: "1.26"}

	result := CheckDeprecatedAPIs(chart, opts)

	if len(result.Deprecations) < 2 {
		t.Errorf("expected at least 2 deprecations, got %d", len(result.Deprecations))
	}
}

// ── Test 6: report format is non-empty and human-readable ─────────────────────

func TestCheckDeprecatedAPIs_ReportFormat(t *testing.T) {
	chart := newPlutoChart(map[string]string{
		"templates/ingress.yaml": legacyIngressTemplate,
	})
	opts := PlutoCheckOptions{TargetVersion: "1.22"}

	result := CheckDeprecatedAPIs(chart, opts)

	if result.Report == "" {
		t.Error("expected non-empty Report string")
	}
	// Report should reference the deprecated kind or API
	if !strings.Contains(result.Report, "Ingress") && !strings.Contains(result.Report, "extensions/v1beta1") {
		t.Errorf("expected Report to mention deprecated resource, got:\n%s", result.Report)
	}
}

// ── Test 7: nil chart returns nil without panic ───────────────────────────────

func TestCheckDeprecatedAPIs_NilChart(t *testing.T) {
	opts := PlutoCheckOptions{TargetVersion: "1.29"}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("CheckDeprecatedAPIs panicked on nil chart: %v", r)
		}
	}()

	result := CheckDeprecatedAPIs(nil, opts)
	if result != nil {
		t.Error("expected nil result for nil chart")
	}
}

// ── Test 8: HasDeprecations flag set correctly ────────────────────────────────

func TestCheckDeprecatedAPIs_HasDeprecationsFlagSetCorrectly(t *testing.T) {
	cleanChart := newPlutoChart(map[string]string{
		"templates/deployment.yaml": modernDeploymentTemplate,
	})
	dirtyChart := newPlutoChart(map[string]string{
		"templates/ingress.yaml": legacyIngressTemplate,
	})
	opts := PlutoCheckOptions{TargetVersion: "1.29"}

	cleanResult := CheckDeprecatedAPIs(cleanChart, opts)
	dirtyResult := CheckDeprecatedAPIs(dirtyChart, opts)

	if cleanResult.HasDeprecations {
		t.Error("expected HasDeprecations=false for clean chart")
	}
	if !dirtyResult.HasDeprecations {
		t.Error("expected HasDeprecations=true for chart with deprecated APIs")
	}
}

// ── Test 9: Message contains replacement API ──────────────────────────────────

func TestCheckDeprecatedAPIs_MessageContainsReplacement(t *testing.T) {
	chart := newPlutoChart(map[string]string{
		"templates/ingress.yaml": legacyIngressTemplate,
	})
	opts := PlutoCheckOptions{TargetVersion: "1.22"}

	result := CheckDeprecatedAPIs(chart, opts)

	if len(result.Deprecations) == 0 {
		t.Fatal("expected at least one deprecation")
	}
	dep := result.Deprecations[0]
	// ReplacementAPI or Message should reference networking.k8s.io/v1
	hasReplacement := dep.ReplacementAPI != "" || strings.Contains(dep.Message, "networking.k8s.io/v1")
	if !hasReplacement {
		t.Errorf("expected Message or ReplacementAPI to reference networking.k8s.io/v1, got Message=%q ReplacementAPI=%q",
			dep.Message, dep.ReplacementAPI)
	}
}

// ── Test 10: TemplatePath set on deprecation entry ───────────────────────────

func TestCheckDeprecatedAPIs_TemplatePathPresent(t *testing.T) {
	chart := newPlutoChart(map[string]string{
		"templates/ingress.yaml": legacyIngressTemplate,
	})
	opts := PlutoCheckOptions{TargetVersion: "1.22"}

	result := CheckDeprecatedAPIs(chart, opts)

	if len(result.Deprecations) == 0 {
		t.Fatal("expected at least one deprecation")
	}
	dep := result.Deprecations[0]
	if dep.TemplatePath == "" {
		t.Error("expected TemplatePath to be set on APIDeprecation entry")
	}
	if !strings.Contains(dep.TemplatePath, "ingress.yaml") {
		t.Errorf("expected TemplatePath to reference ingress.yaml, got %q", dep.TemplatePath)
	}
}

// ── Test 11: ShowAll includes non-deprecated resources in report ──────────────

func TestCheckDeprecatedAPIs_ShowAllIncludesNonDeprecated(t *testing.T) {
	chart := newPlutoChart(map[string]string{
		"templates/deployment.yaml": modernDeploymentTemplate,
		"templates/ingress.yaml":    legacyIngressTemplate,
	})
	optsAll := PlutoCheckOptions{TargetVersion: "1.29", ShowAll: true}
	optsDefault := PlutoCheckOptions{TargetVersion: "1.29", ShowAll: false}

	resultAll := CheckDeprecatedAPIs(chart, optsAll)
	resultDefault := CheckDeprecatedAPIs(chart, optsDefault)

	// ShowAll report should be longer (includes the clean Deployment)
	if len(resultAll.Report) <= len(resultDefault.Report) {
		t.Error("expected ShowAll=true to produce a longer report than ShowAll=false")
	}
}

// ── Test 12: DeprecatedIn and RemovedIn populated from migration table ─────────

func TestCheckDeprecatedAPIs_DeprecatedInAndRemovedInFromTable(t *testing.T) {
	chart := newPlutoChart(map[string]string{
		"templates/pdb.yaml": legacyPDBTemplate,
	})
	opts := PlutoCheckOptions{TargetVersion: "1.25"}

	result := CheckDeprecatedAPIs(chart, opts)

	if len(result.Deprecations) == 0 {
		t.Fatal("expected at least one deprecation")
	}
	dep := result.Deprecations[0]
	// policy/v1beta1 PDB: DeprecatedIn=1.21, RemovedIn=1.25 per apiMigrations table
	if dep.DeprecatedIn == "" {
		t.Error("expected DeprecatedIn to be populated")
	}
	if dep.RemovedIn == "" {
		t.Error("expected RemovedIn to be populated")
	}
}
