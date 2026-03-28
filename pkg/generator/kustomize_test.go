package generator

import (
	"sort"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// makeChart is defined in airgap_test.go — do not redefine here.
// Signature: func makeChart(name string, templates map[string]string) *types.GeneratedChart

// ============================================================
// Subtask 1: Single Deployment — base has exactly one resource
// ============================================================

func TestKustomize_SingleDeployment_BaseHasOneResource(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	out, err := GenerateKustomizeLayout(chart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Base == nil {
		t.Fatal("expected non-nil Base")
	}

	// Base.Resources must have exactly one entry.
	if len(out.Base.Resources) != 1 {
		t.Errorf("expected 1 resource in base, got %d", len(out.Base.Resources))
	}
}

// ============================================================
// Subtask 2: Three templates — base lists all three resources
// ============================================================

func TestKustomize_ThreeResources_BaseListsAll(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
		"templates/service.yaml":    "apiVersion: v1\nkind: Service\nmetadata:\n  name: myapp\n",
		"templates/configmap.yaml":  "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: myapp-config\n",
	})

	out, err := GenerateKustomizeLayout(chart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.Base.Resources) != 3 {
		t.Errorf("expected 3 resources in base, got %d", len(out.Base.Resources))
	}
}

// ============================================================
// Subtask 3: Base kustomization.yaml contains correct apiVersion
// ============================================================

func TestKustomize_BaseKustomization_HasAPIVersion(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	out, err := GenerateKustomizeLayout(chart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	const want = "apiVersion: kustomize.config.k8s.io/v1beta1"
	if !strings.Contains(out.Base.Kustomization, want) {
		t.Errorf("base kustomization.yaml missing %q\ngot:\n%s", want, out.Base.Kustomization)
	}
}

// ============================================================
// Subtask 4: Base kustomization.yaml contains correct kind
// ============================================================

func TestKustomize_BaseKustomization_HasKind(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	out, err := GenerateKustomizeLayout(chart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	const want = "kind: Kustomization"
	if !strings.Contains(out.Base.Kustomization, want) {
		t.Errorf("base kustomization.yaml missing %q\ngot:\n%s", want, out.Base.Kustomization)
	}
}

// ============================================================
// Subtask 5: Dev overlay — replica-patch has replicas: 1
// ============================================================

func TestKustomize_DevOverlay_ReplicasPatch(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	out, err := GenerateKustomizeLayout(chart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dev, ok := out.Overlays["dev"]
	if !ok {
		t.Fatal("expected 'dev' overlay to exist")
	}

	// Find the replica patch among dev patches.
	found := false
	for _, p := range dev.Patches {
		if strings.Contains(p.Patch, "replicas: 1") {
			found = true
			if !strings.Contains(p.Patch, "name: myapp") {
				t.Error("dev overlay: replica patch must use chart name 'myapp', not hardcoded 'app'")
			}
			break
		}
	}
	if !found {
		t.Error("dev overlay: expected a patch with 'replicas: 1'")
	}
}

// ============================================================
// Subtask 6: Staging overlay — replica-patch has replicas: 2
// ============================================================

func TestKustomize_StagingOverlay_ReplicasPatch(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	out, err := GenerateKustomizeLayout(chart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	staging, ok := out.Overlays["staging"]
	if !ok {
		t.Fatal("expected 'staging' overlay to exist")
	}

	found := false
	for _, p := range staging.Patches {
		if strings.Contains(p.Patch, "replicas: 2") {
			found = true
			break
		}
	}
	if !found {
		t.Error("staging overlay: expected a patch with 'replicas: 2'")
	}
}

// ============================================================
// Subtask 7: Prod overlay — replica-patch has replicas: 3
// ============================================================

func TestKustomize_ProdOverlay_ReplicasPatch(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	out, err := GenerateKustomizeLayout(chart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prod, ok := out.Overlays["prod"]
	if !ok {
		t.Fatal("expected 'prod' overlay to exist")
	}

	found := false
	for _, p := range prod.Patches {
		if strings.Contains(p.Patch, "replicas: 3") {
			found = true
			break
		}
	}
	if !found {
		t.Error("prod overlay: expected a patch with 'replicas: 3'")
	}
}

// ============================================================
// Subtask 8: Prod overlay — resources patch contains limits
// ============================================================

func TestKustomize_ProdOverlay_HasResourcesPatch(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	out, err := GenerateKustomizeLayout(chart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prod, ok := out.Overlays["prod"]
	if !ok {
		t.Fatal("expected 'prod' overlay to exist")
	}

	// At least one patch must contain resource limits keywords.
	found := false
	for _, p := range prod.Patches {
		if strings.Contains(p.Patch, "limits") {
			found = true
			if !strings.Contains(p.Patch, "name: myapp") {
				t.Error("prod overlay: resource-limits patch must use chart name 'myapp', not hardcoded 'app'")
			}
			break
		}
	}
	if !found {
		t.Error("prod overlay: expected a patch containing 'limits' (resource limits patch)")
	}
}

// ============================================================
// Subtask 9: Overlay kustomization references "../../base"
// ============================================================

func TestKustomize_OverlayBases_CorrectRelativePath(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	out, err := GenerateKustomizeLayout(chart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for env, overlay := range out.Overlays {
		if !strings.Contains(overlay.Kustomization, "../../base") {
			t.Errorf("overlay %q kustomization.yaml must reference '../../base', got:\n%s",
				env, overlay.Kustomization)
		}
	}
}

// ============================================================
// Subtask 10: Output has exactly three overlays: dev, staging, prod
// ============================================================

func TestKustomize_ThreeOverlays_DevStagingProd(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	out, err := GenerateKustomizeLayout(chart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(out.Overlays) != 3 {
		t.Errorf("expected exactly 3 overlays, got %d: %v", len(out.Overlays), overlayKeys(out.Overlays))
	}

	for _, env := range []string{"dev", "staging", "prod"} {
		if _, ok := out.Overlays[env]; !ok {
			t.Errorf("expected overlay %q to exist", env)
		}
	}
}

// overlayKeys is a local helper to produce readable error messages.
func overlayKeys(m map[string]*KustomizeDir) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ============================================================
// Subtask 11: Base resources are listed in alphabetical order
// ============================================================

func TestKustomize_ResourcesSorted(t *testing.T) {
	// Supply templates intentionally out of alphabetical order.
	chart := makeChart("myapp", map[string]string{
		"templates/service.yaml":    "apiVersion: v1\nkind: Service\nmetadata:\n  name: myapp\n",
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
		"templates/configmap.yaml":  "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: myapp-config\n",
	})

	out, err := GenerateKustomizeLayout(chart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Build the expected sorted order from the resource map keys.
	expected := make([]string, 0, len(out.Base.Resources))
	for name := range out.Base.Resources {
		expected = append(expected, name)
	}
	sort.Strings(expected)

	// Parse the kustomization YAML and verify resource lines appear in
	// alphabetical order by comparing their positions in the text.
	kust := out.Base.Kustomization
	lastIdx := -1
	for i, name := range expected {
		idx := strings.Index(kust, name)
		if idx == -1 {
			t.Errorf("resource %q not found in base kustomization.yaml", name)
			continue
		}
		if idx <= lastIdx {
			t.Errorf("resource %q (index %d) appears at position %d, before previous resource %q at position %d — resources must be alphabetically sorted",
				name, i, idx, expected[i-1], lastIdx)
		}
		lastIdx = idx
	}
}

// ============================================================
// Subtask 12: Empty chart (no templates) — returns error
// ============================================================

func TestKustomize_NilChart_ReturnsError(t *testing.T) {
	out, err := GenerateKustomizeLayout(nil)
	if err == nil {
		t.Error("expected error for nil chart, got nil")
	}
	if out != nil {
		t.Errorf("expected nil output on error, got %+v", out)
	}
}

func TestKustomize_EmptyChart_ReturnsError(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:       "empty",
		ChartYAML:  "apiVersion: v2\nname: empty\nversion: 0.1.0\n",
		ValuesYAML: "replicaCount: 1\n",
		Templates:  map[string]string{},
	}

	out, err := GenerateKustomizeLayout(chart)
	if err == nil {
		t.Error("expected error for chart with no templates, got nil")
	}
	if out != nil {
		t.Errorf("expected nil output on error, got %+v", out)
	}
}

// ============================================================
// Subtask 13: YAML injection — newline in resource name rejected
// ============================================================

// ============================================================
// Subtask 13-14: YAML injection — unsafe chars rejected
// ============================================================

func TestKustomize_RejectsUnsafeResourceNames(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"newline in name", "templates/deploy\nment.yaml"},
		{"colon in name", "templates/deploy:ment.yaml"},
		{"hash in name", "templates/deploy#ment.yaml"},
		{"path traversal", "templates/../../etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chart := makeChart("myapp", map[string]string{
				tt.key: "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
			})

			out, err := GenerateKustomizeLayout(chart)
			if err == nil {
				t.Fatalf("expected error for template key %q, got nil", tt.key)
			}
			if out != nil {
				t.Errorf("expected nil output on error, got %+v", out)
			}
		})
	}
}

// ============================================================
// Subtask 15: Valid resource name with subdirectory accepted
// ============================================================

func TestKustomize_AcceptsValidResourceName(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/sub-dir/deploy_v2.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	out, err := GenerateKustomizeLayout(chart)
	if err != nil {
		t.Fatalf("unexpected error for valid resource name: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
	if _, ok := out.Base.Resources["sub-dir/deploy_v2.yaml"]; !ok {
		t.Error("expected resource 'sub-dir/deploy_v2.yaml' in base")
	}
}

// ============================================================
// Unit tests for unexported helpers
// ============================================================

func TestGenerateBaseKustomization_ContainsResources(t *testing.T) {
	resources := []string{"deployment.yaml", "service.yaml"}
	kust := generateBaseKustomization(resources)

	if !strings.Contains(kust, "apiVersion: kustomize.config.k8s.io/v1beta1") {
		t.Error("base kustomization missing apiVersion")
	}
	if !strings.Contains(kust, "kind: Kustomization") {
		t.Error("base kustomization missing kind")
	}
	for _, r := range resources {
		if !strings.Contains(kust, r) {
			t.Errorf("base kustomization missing resource %q", r)
		}
	}
}

func TestGenerateOverlayKustomization_ReferencesBase(t *testing.T) {
	patches := []string{"replica-patch.yaml"}
	kust := generateOverlayKustomization("dev", patches)

	if !strings.Contains(kust, "../../base") {
		t.Error("overlay kustomization must reference '../../base'")
	}
	if !strings.Contains(kust, "replica-patch.yaml") {
		t.Error("overlay kustomization must list the supplied patch")
	}
}

func TestGenerateReplicaPatch_DevReplicas(t *testing.T) {
	patch := generateReplicaPatch("myapp", 1)
	if !strings.Contains(patch, "replicas: 1") {
		t.Errorf("dev replica patch must contain 'replicas: 1', got:\n%s", patch)
	}
	if !strings.Contains(patch, "name: myapp") {
		t.Errorf("replica patch must use chart name in metadata.name, got:\n%s", patch)
	}
}

func TestGenerateReplicaPatch_ProdReplicas(t *testing.T) {
	patch := generateReplicaPatch("myapp", 3)
	if !strings.Contains(patch, "replicas: 3") {
		t.Errorf("prod replica patch must contain 'replicas: 3', got:\n%s", patch)
	}
	if !strings.Contains(patch, "name: myapp") {
		t.Errorf("replica patch must use chart name in metadata.name, got:\n%s", patch)
	}
}

// ============================================================
// Subtask: Overlay uses modern patches: syntax, not deprecated patchesStrategicMerge
// ============================================================

func TestKustomize_OverlayUsesPatchesNotStrategicMerge(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	out, err := GenerateKustomizeLayout(chart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for env, overlay := range out.Overlays {
		kust := overlay.Kustomization

		if strings.Contains(kust, "patchesStrategicMerge") {
			t.Errorf("overlay %q: kustomization.yaml must NOT contain deprecated 'patchesStrategicMerge', got:\n%s",
				env, kust)
		}

		if !strings.Contains(kust, "patches:") {
			t.Errorf("overlay %q: kustomization.yaml must contain modern 'patches:' key, got:\n%s",
				env, kust)
		}

		// Verify entries use "- path: <file>" format.
		for _, p := range overlay.Patches {
			expected := "- path: " + p.Target
			if !strings.Contains(kust, expected) {
				t.Errorf("overlay %q: expected %q in kustomization.yaml, got:\n%s",
					env, expected, kust)
			}
		}
	}
}
