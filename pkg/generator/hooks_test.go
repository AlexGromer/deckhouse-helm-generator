package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Test 1: GenerateHelmHooks produces three hooks for a non-empty chart
// ============================================================

func TestGenerateHelmHooks_ProducesThreeHooks(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp",
	})

	hooks := GenerateHelmHooks(chart)

	if len(hooks) != 3 {
		t.Fatalf("expected 3 hook templates, got %d", len(hooks))
	}

	expectedPaths := []string{
		"templates/hooks/pre-upgrade-job.yaml",
		"templates/hooks/post-install-job.yaml",
		"templates/hooks/pre-delete-job.yaml",
	}

	for _, path := range expectedPaths {
		if _, ok := hooks[path]; !ok {
			t.Errorf("missing expected hook template %q", path)
		}
	}
}

// ============================================================
// Test 2: GenerateHelmHooks returns empty map for empty chart
// ============================================================

func TestGenerateHelmHooks_EmptyChart_NoHooks(t *testing.T) {
	// Nil chart
	hooks := GenerateHelmHooks(nil)
	if len(hooks) != 0 {
		t.Errorf("expected 0 hooks for nil chart, got %d", len(hooks))
	}

	// Chart with no templates
	emptyChart := &types.GeneratedChart{
		Name:       "empty",
		ChartYAML:  "apiVersion: v2\nname: empty\nversion: 0.1.0\n",
		ValuesYAML: "replicaCount: 1\n",
		Templates:  map[string]string{},
	}

	hooks = GenerateHelmHooks(emptyChart)
	if len(hooks) != 0 {
		t.Errorf("expected 0 hooks for chart with no templates, got %d", len(hooks))
	}
}

// ============================================================
// Test 3: Hook templates contain required annotations
// ============================================================

func TestGenerateHelmHooks_HookAnnotations(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp",
	})

	hooks := GenerateHelmHooks(chart)

	testCases := []struct {
		path     string
		hookType string
	}{
		{"templates/hooks/pre-upgrade-job.yaml", "pre-upgrade"},
		{"templates/hooks/post-install-job.yaml", "post-install"},
		{"templates/hooks/pre-delete-job.yaml", "pre-delete"},
	}

	for _, tc := range testCases {
		content, ok := hooks[tc.path]
		if !ok {
			t.Fatalf("missing hook template %q", tc.path)
		}

		// Verify helm.sh/hook annotation
		expectedHook := `"helm.sh/hook": ` + tc.hookType
		if !strings.Contains(content, expectedHook) {
			t.Errorf("%s: missing annotation %q", tc.path, expectedHook)
		}

		// Verify hook-weight annotation
		if !strings.Contains(content, `"helm.sh/hook-weight":`) {
			t.Errorf("%s: missing helm.sh/hook-weight annotation", tc.path)
		}

		// Verify hook-delete-policy annotation
		if !strings.Contains(content, `"helm.sh/hook-delete-policy": before-hook-creation`) {
			t.Errorf("%s: missing helm.sh/hook-delete-policy annotation", tc.path)
		}

		// Verify Helm template syntax for name
		if !strings.Contains(content, `{{ include "chartname.fullname" . }}`) {
			t.Errorf("%s: missing Helm template fullname include", tc.path)
		}

		// Verify it's a Job
		if !strings.Contains(content, "kind: Job") {
			t.Errorf("%s: expected kind: Job", tc.path)
		}
	}
}
