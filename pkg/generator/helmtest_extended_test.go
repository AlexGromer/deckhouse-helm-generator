package generator

// helmtest_extended_test.go — TDD tests for Task 6.0.3
// Tests for GenerateSnapshotTests and GenerateValuePermutationTests.
// These tests define the contract; the implementation must satisfy them.

import (
	"strings"
	"testing"
)

var _ = (*struct{ X interface{} })(nil) // suppress unused import lint

// ============================================================
// Test 1: GenerateSnapshotTests produces one file per renderable template
// ============================================================

func TestGenerateSnapshotTests_PerTemplate(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
		"templates/service.yaml":    "apiVersion: v1\nkind: Service\nmetadata:\n  name: myapp\n",
		"templates/_helpers.tpl":    "{{- define \"myapp.name\" -}}myapp{{- end -}}",
		"templates/NOTES.txt":       "Thank you for installing myapp.",
	})

	opts := SnapshotTestOptions{MatchSnapshot: true, UpdateSnapshot: false}
	result := GenerateSnapshotTests(chart, opts)

	// _helpers.tpl and NOTES.txt must be skipped; 2 renderable templates remain.
	if len(result) != 2 {
		t.Fatalf("expected 2 snapshot test files (one per renderable template), got %d", len(result))
	}

	if _, ok := result["tests/deployment_snapshot_test.yaml"]; !ok {
		t.Error("expected tests/deployment_snapshot_test.yaml in snapshot results")
	}
	if _, ok := result["tests/service_snapshot_test.yaml"]; !ok {
		t.Error("expected tests/service_snapshot_test.yaml in snapshot results")
	}
}

// ============================================================
// Test 2: matchSnapshot assertion is present when MatchSnapshot=true
// ============================================================

func TestGenerateSnapshotTests_MatchSnapshotAssertion(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	opts := SnapshotTestOptions{MatchSnapshot: true, UpdateSnapshot: false}
	result := GenerateSnapshotTests(chart, opts)

	content, ok := result["tests/deployment_snapshot_test.yaml"]
	if !ok {
		t.Fatal("expected tests/deployment_snapshot_test.yaml to exist")
	}

	if !strings.Contains(content, "matchSnapshot") {
		t.Error("snapshot test must contain 'matchSnapshot' assertion")
	}
}

// ============================================================
// Test 3: UpdateSnapshot flag propagates into generated content
// ============================================================

func TestGenerateSnapshotTests_UpdateSnapshotFlag(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	opts := SnapshotTestOptions{MatchSnapshot: true, UpdateSnapshot: true}
	result := GenerateSnapshotTests(chart, opts)

	content, ok := result["tests/deployment_snapshot_test.yaml"]
	if !ok {
		t.Fatal("expected tests/deployment_snapshot_test.yaml to exist")
	}

	// The update-snapshot flag must be reflected in the test file
	// (e.g., as a comment, annotation, or helm-unittest flag).
	if !strings.Contains(content, "update") && !strings.Contains(content, "Update") {
		t.Error("snapshot test content must reference update-snapshot when UpdateSnapshot=true")
	}
}

// ============================================================
// Test 4: GenerateValuePermutationTests "all" mode generates cartesian product
// ============================================================

func TestGenerateValuePermutationTests_AllMode(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	opts := ValuePermutationOptions{
		ValueOverrides: map[string][]interface{}{
			"replicaCount": {1, 3},
			"image.tag":    {"latest", "v1.0"},
		},
		CombinationMode: "all",
	}

	result := GenerateValuePermutationTests(chart, opts)
	if len(result) == 0 {
		t.Fatal("expected non-empty result from GenerateValuePermutationTests with all mode")
	}

	// Combine all generated content to count test cases.
	var combined strings.Builder
	for _, content := range result {
		combined.WriteString(content)
	}
	all := combined.String()

	// Cartesian product of 2 keys × 2 values = 4 combinations.
	// Each combination produces an "- it:" block.
	count := strings.Count(all, "- it:")
	if count < 4 {
		t.Errorf("all-mode cartesian product of 2×2 must yield at least 4 test cases, got %d", count)
	}
}

// ============================================================
// Test 5: GenerateValuePermutationTests "pairwise" mode generates fewer cases
// ============================================================

func TestGenerateValuePermutationTests_PairwiseMode(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	opts := ValuePermutationOptions{
		ValueOverrides: map[string][]interface{}{
			"replicaCount": {1, 3, 5},
			"image.tag":    {"latest", "v1.0", "v2.0"},
		},
		CombinationMode: "pairwise",
	}

	result := GenerateValuePermutationTests(chart, opts)
	if len(result) == 0 {
		t.Fatal("expected non-empty result from GenerateValuePermutationTests with pairwise mode")
	}

	var combined strings.Builder
	for _, content := range result {
		combined.WriteString(content)
	}
	all := combined.String()

	// Pairwise on 2 factors of 3 levels = at most 9 (all-mode full product).
	// Pairwise must be strictly fewer than the full 9-case cartesian product.
	count := strings.Count(all, "- it:")
	if count >= 9 {
		t.Errorf("pairwise mode must produce fewer than full cartesian product (9), got %d", count)
	}
	if count < 1 {
		t.Errorf("pairwise mode must produce at least 1 test case, got %d", count)
	}
}

// ============================================================
// Test 6: GenerateSnapshotTests with nil chart returns nil
// ============================================================

func TestGenerateSnapshotTests_NilChart(t *testing.T) {
	opts := SnapshotTestOptions{MatchSnapshot: true, UpdateSnapshot: false}
	result := GenerateSnapshotTests(nil, opts)

	if result != nil {
		t.Errorf("expected nil for nil chart, got %v", result)
	}
}

// ============================================================
// Test 7: GenerateValuePermutationTests with empty overrides returns nil/empty
// ============================================================

func TestGenerateValuePermutationTests_EmptyValues(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	opts := ValuePermutationOptions{
		ValueOverrides:  map[string][]interface{}{},
		CombinationMode: "all",
	}

	result := GenerateValuePermutationTests(chart, opts)

	// Empty overrides means no permutation tests to generate.
	if len(result) != 0 {
		t.Errorf("expected empty result for empty ValueOverrides, got %d files", len(result))
	}
}

// ============================================================
// Test 8: Snapshot tests and GenerateHelmTests both target the same template
// ============================================================

func TestGenerateSnapshotTests_CombinedWithExistingHelmTests(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	opts := SnapshotTestOptions{MatchSnapshot: true, UpdateSnapshot: false}
	snapshotTests := GenerateSnapshotTests(chart, opts)
	helmTests := GenerateHelmTests(chart)

	// Both must produce results for the deployment template.
	if _, ok := snapshotTests["tests/deployment_snapshot_test.yaml"]; !ok {
		t.Error("snapshot tests must contain deployment snapshot file")
	}
	if _, ok := helmTests["tests/deployment_test.yaml"]; !ok {
		t.Error("helm tests must contain deployment test file")
	}

	// The two outputs must reference the same template path.
	snapshotContent := snapshotTests["tests/deployment_snapshot_test.yaml"]
	helmContent := helmTests["tests/deployment_test.yaml"]

	if !strings.Contains(snapshotContent, "templates/deployment.yaml") {
		t.Error("snapshot test must reference templates/deployment.yaml")
	}
	if !strings.Contains(helmContent, "templates/deployment.yaml") {
		t.Error("helm test must reference templates/deployment.yaml")
	}
}

// ============================================================
// Test 9: "all" mode with 3 keys × 2 values each yields 2³ = 8 test cases
// ============================================================

func TestGenerateValuePermutationTests_OverrideCountMatches(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	opts := ValuePermutationOptions{
		ValueOverrides: map[string][]interface{}{
			"replicaCount":     {1, 2},
			"image.tag":        {"latest", "v1.0"},
			"resources.limits": {"small", "large"},
		},
		CombinationMode: "all",
	}

	result := GenerateValuePermutationTests(chart, opts)
	if len(result) == 0 {
		t.Fatal("expected non-empty result")
	}

	var combined strings.Builder
	for _, content := range result {
		combined.WriteString(content)
	}

	count := strings.Count(combined.String(), "- it:")
	if count != 8 {
		t.Errorf("cartesian product of 3 keys × 2 values = 8 test cases, got %d", count)
	}
}

// ============================================================
// Test 10: Snapshot test files use the _snapshot_test.yaml naming convention
// ============================================================

func TestGenerateSnapshotTests_TestFileNaming(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/configmap.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: myapp\n",
	})

	opts := SnapshotTestOptions{MatchSnapshot: true, UpdateSnapshot: false}
	result := GenerateSnapshotTests(chart, opts)

	if len(result) == 0 {
		t.Fatal("expected snapshot test files to be generated")
	}

	for path := range result {
		if !strings.HasSuffix(path, "_snapshot_test.yaml") {
			t.Errorf("snapshot test file %q must use _snapshot_test.yaml suffix", path)
		}
		if !strings.HasPrefix(path, "tests/") {
			t.Errorf("snapshot test file %q must be placed under tests/", path)
		}
	}
}
