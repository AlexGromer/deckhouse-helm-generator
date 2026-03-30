package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

const hpaV2Template = `apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: my-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: my-app
  minReplicas: 1
  maxReplicas: 10`

// autoscaling/v2beta1 requires <1.26 and is removed in 1.26.
const hpaV2beta1Template = `apiVersion: autoscaling/v2beta1
kind: HorizontalPodAutoscaler
metadata:
  name: old-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: my-app
  minReplicas: 1
  maxReplicas: 5`

const simpleDeploymentForMatrix = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: simple-app
spec:
  replicas: 1`

func newMatrixChart(templates map[string]string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name:      "matrix-test",
		Templates: templates,
	}
}

// ── Test 1: compatible chart (modern APIs) ────────────────────────────────────

func TestValidateK8sVersionMatrix_CompatibleChart(t *testing.T) {
	chart := newMatrixChart(map[string]string{
		"templates/deployment.yaml": simpleDeploymentForMatrix,
	})
	opts := K8sVersionOptions{
		MinVersion:     "1.27",
		MaxVersion:     "1.32",
		TargetVersions: []string{"1.29"},
	}

	result := ValidateK8sVersionMatrix(chart, opts)

	if result == nil {
		t.Fatal("expected non-nil K8sVersionResult")
	}
	compat, ok := result.Compatibility["1.29"]
	if !ok {
		t.Fatal("expected compatibility entry for version 1.29")
	}
	if !compat.Compatible {
		t.Errorf("expected apps/v1 Deployment to be compatible with 1.29, issues: %v", compat.Issues)
	}
}

// ── Test 2: incompatible API for old K8s version ──────────────────────────────

func TestValidateK8sVersionMatrix_IncompatibleAPIForOldVersion(t *testing.T) {
	// autoscaling/v2 is GA from 1.23; on 1.21 it does not exist.
	chart := newMatrixChart(map[string]string{
		"templates/hpa.yaml": hpaV2Template,
	})
	opts := K8sVersionOptions{
		TargetVersions: []string{"1.21"},
	}

	result := ValidateK8sVersionMatrix(chart, opts)

	compat, ok := result.Compatibility["1.21"]
	if !ok {
		t.Fatal("expected compatibility entry for version 1.21")
	}
	if compat.Compatible {
		t.Error("expected autoscaling/v2 to be incompatible with K8s 1.21")
	}
	if len(compat.Issues) == 0 {
		t.Error("expected at least one issue describing the incompatibility")
	}
}

// ── Test 3: range check — min/max version bounds ──────────────────────────────

func TestValidateK8sVersionMatrix_RangeCheck(t *testing.T) {
	chart := newMatrixChart(map[string]string{
		"templates/deployment.yaml": simpleDeploymentForMatrix,
	})
	opts := K8sVersionOptions{
		MinVersion: "1.27",
		MaxVersion: "1.32",
	}

	result := ValidateK8sVersionMatrix(chart, opts)

	// Compatibility map must contain entries for versions in [1.27, 1.32]
	for _, v := range []string{"1.27", "1.28", "1.29", "1.30", "1.31", "1.32"} {
		if _, ok := result.Compatibility[v]; !ok {
			t.Errorf("expected compatibility entry for version %s within MinVersion/MaxVersion range", v)
		}
	}
}

// ── Test 4: multiple target versions all checked ──────────────────────────────

func TestValidateK8sVersionMatrix_MultipleVersions(t *testing.T) {
	chart := newMatrixChart(map[string]string{
		"templates/deployment.yaml": simpleDeploymentForMatrix,
	})
	targets := []string{"1.27", "1.29", "1.31"}
	opts := K8sVersionOptions{TargetVersions: targets}

	result := ValidateK8sVersionMatrix(chart, opts)

	for _, v := range targets {
		if _, ok := result.Compatibility[v]; !ok {
			t.Errorf("expected compatibility entry for target version %s", v)
		}
	}
	if len(result.Compatibility) < len(targets) {
		t.Errorf("expected %d entries in Compatibility, got %d", len(targets), len(result.Compatibility))
	}
}

// ── Test 5: feature gate warning when using removed API ───────────────────────

func TestValidateK8sVersionMatrix_FeatureGateWarning(t *testing.T) {
	// autoscaling/v2beta1 removed in 1.26 — should trigger a warning/issue.
	chart := newMatrixChart(map[string]string{
		"templates/hpa.yaml": hpaV2beta1Template,
	})
	opts := K8sVersionOptions{
		TargetVersions: []string{"1.26"},
	}

	result := ValidateK8sVersionMatrix(chart, opts)

	// Either Warnings or Issues on the version should be non-empty
	hasWarning := len(result.Warnings) > 0
	if compat, ok := result.Compatibility["1.26"]; ok {
		hasWarning = hasWarning || len(compat.Issues) > 0
	}
	if !hasWarning {
		t.Error("expected a warning or issue for autoscaling/v2beta1 removed in 1.26")
	}
}

// ── Test 6: nil chart returns nil without panic ───────────────────────────────

func TestValidateK8sVersionMatrix_NilChart(t *testing.T) {
	opts := K8sVersionOptions{TargetVersions: []string{"1.29"}}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ValidateK8sVersionMatrix panicked on nil chart: %v", r)
		}
	}()

	result := ValidateK8sVersionMatrix(nil, opts)
	if result != nil {
		t.Error("expected nil result for nil chart")
	}
}

// ── Test 7: empty templates — no issues ───────────────────────────────────────

func TestValidateK8sVersionMatrix_EmptyTemplates(t *testing.T) {
	chart := newMatrixChart(map[string]string{})
	opts := K8sVersionOptions{TargetVersions: []string{"1.29"}}

	result := ValidateK8sVersionMatrix(chart, opts)

	if result == nil {
		t.Fatal("expected non-nil result even for empty chart")
	}
	if compat, ok := result.Compatibility["1.29"]; ok {
		if !compat.Compatible {
			t.Errorf("expected empty chart to be compatible with 1.29, issues: %v", compat.Issues)
		}
	}
}

// ── Test 8: NOTESTxt summary is present ───────────────────────────────────────

func TestValidateK8sVersionMatrix_NOTESTxtSummary(t *testing.T) {
	chart := newMatrixChart(map[string]string{
		"templates/deployment.yaml": simpleDeploymentForMatrix,
	})
	opts := K8sVersionOptions{TargetVersions: []string{"1.29"}}

	result := ValidateK8sVersionMatrix(chart, opts)

	if result.NOTESTxt == "" {
		t.Error("expected non-empty NOTESTxt summary")
	}
	if !strings.Contains(result.NOTESTxt, "1.29") {
		t.Errorf("expected NOTESTxt to mention target version 1.29, got:\n%s", result.NOTESTxt)
	}
}

// ── Test 9: all versions compatible when chart uses only stable APIs ───────────

func TestValidateK8sVersionMatrix_AllVersionsCompatible(t *testing.T) {
	chart := newMatrixChart(map[string]string{
		"templates/deployment.yaml": simpleDeploymentForMatrix,
	})
	// apps/v1 Deployment is available since 1.9 — all modern versions are fine.
	opts := K8sVersionOptions{
		TargetVersions: []string{"1.27", "1.28", "1.29", "1.30", "1.31", "1.32"},
	}

	result := ValidateK8sVersionMatrix(chart, opts)

	for v, compat := range result.Compatibility {
		if !compat.Compatible {
			t.Errorf("expected version %s to be compatible for apps/v1 Deployment, issues: %v", v, compat.Issues)
		}
	}
}

// ── Test 10: single version target — Compatibility has exactly one entry ──────

func TestValidateK8sVersionMatrix_SingleVersionTarget(t *testing.T) {
	chart := newMatrixChart(map[string]string{
		"templates/deployment.yaml": simpleDeploymentForMatrix,
	})
	opts := K8sVersionOptions{TargetVersions: []string{"1.30"}}

	result := ValidateK8sVersionMatrix(chart, opts)

	if len(result.Compatibility) != 1 {
		t.Errorf("expected exactly 1 entry in Compatibility for single target, got %d", len(result.Compatibility))
	}
	compat, ok := result.Compatibility["1.30"]
	if !ok {
		t.Error("expected entry for 1.30 in Compatibility")
	}
	if compat.Version != "1.30" {
		t.Errorf("expected VersionCompatibility.Version=1.30, got %q", compat.Version)
	}
}
