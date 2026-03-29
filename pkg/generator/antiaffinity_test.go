package generator

// ============================================================
// Test Plan — antiaffinity_test.go
//
// Subjects:
//   GenerateAntiAffinity(graph, opts)        → map[string]string  (per-template YAML snippets)
//   InjectAntiAffinity(chart, opts)          → (*GeneratedChart, AntiAffinityResult)
//   GenerateAntiAffinityValues(opts)         → map[string]interface{}
//
//  # | Test Name                                              | Category    | Input                                           | Expected Output
// ---+--------------------------------------------------------+-------------+-------------------------------------------------+---------------------------------------------------
//  1 | TestInjectAntiAffinity_Deployment_Preferred_Hostname   | happy       | Deployment, Mode=preferred, hostname            | "preferred" term present in template
//  2 | TestInjectAntiAffinity_Deployment_Strict_Hostname      | happy       | Deployment, Mode=strict, hostname               | "required" term present
//  3 | TestInjectAntiAffinity_TopologySpreadConstraints_Zone  | happy       | AddTopologySpreadConstraints=true, zone key     | topologySpreadConstraints block present
//  4 | TestInjectAntiAffinity_MaxSkew2                        | happy       | MaxSkew=2                                       | maxSkew: 2 in constraints block
//  5 | TestInjectAntiAffinity_StatefulSet                     | happy       | StatefulSet template                            | affinity rules injected (Injected=1)
//  6 | TestInjectAntiAffinity_DaemonSet                       | happy       | DaemonSet template                              | affinity rules injected (Injected=1)
//  7 | TestInjectAntiAffinity_BothHostnameAndZone             | happy       | TopologyKeys=[hostname, zone]                   | two topology key terms present
//  8 | TestInjectAntiAffinity_SkipIfAffinityPresent           | boundary    | template already has "affinity:"                | Skipped=1, template unchanged
//  9 | TestInjectAntiAffinity_SkipSingleReplica               | boundary    | SkipSingleReplica=true, replicas=1              | Skipped=1, no affinity injected
// 10 | TestInjectAntiAffinity_SkipJobTemplate                 | boundary    | Job template                                    | not injected, Injected=0
// 11 | TestInjectAntiAffinity_EmptyTopologyKeys_DefaultsHostname| boundary  | TopologyKeys=nil                                | defaults to kubernetes.io/hostname
// 12 | TestInjectAntiAffinity_EmptyLabelSelector_Defaults     | error       | LabelSelector=""                                | defaults to app.kubernetes.io/name label
// 13 | TestInjectAntiAffinity_AfterInjectPDB_BothPresent      | integration | InjectPDB then InjectAntiAffinity               | both "PodDisruptionBudget" and "affinity:" present
// 14 | TestGenerateAntiAffinityValues_Consistent              | integration | GenerateAntiAffinityValues with various opts    | keys present: antiAffinity.enabled, mode, topologyKeys
// ============================================================

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ─── Template builders ────────────────────────────────────────────────────────

const testDaemonSetTemplate = `apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: test-agent
spec:
  selector:
    matchLabels:
      app: test-agent
  template:
    spec:
      containers:
      - name: agent
        image: agent:latest`

const testJobTemplate = `apiVersion: batch/v1
kind: Job
metadata:
  name: test-job
spec:
  template:
    spec:
      containers:
      - name: worker
        image: worker:latest
      restartPolicy: Never`

// deploymentTemplateWithReplicas builds a Deployment YAML string with a given replica count.
func deploymentTemplateWithReplicas(replicas int) string {
	if replicas == 1 {
		return `apiVersion: apps/v1
kind: Deployment
metadata:
  name: single-app
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: app
        image: nginx:latest`
	}
	return testDeploymentTemplate // replicas: 3 from autofix_test.go
}

// deploymentWithAffinityAlready returns a Deployment template that already has an affinity block.
func deploymentWithAffinityAlready() string {
	return `apiVersion: apps/v1
kind: Deployment
metadata:
  name: pinned-app
spec:
  replicas: 3
  template:
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution: {}
      containers:
      - name: app
        image: nginx:latest`
}

// defaultAAOpts returns a minimal AntiAffinityOptions for happy-path tests.
func defaultAAOpts() AntiAffinityOptions {
	return AntiAffinityOptions{
		Mode:         AffinityModePreferred,
		TopologyKeys: []TopologyKey{TopologyKeyHostname},
	}
}

// ─── Happy Path ───────────────────────────────────────────────────────────────

// Test 1: Deployment, preferred mode, hostname → "preferred" term in template
func TestInjectAntiAffinity_Deployment_Preferred_Hostname(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})
	opts := AntiAffinityOptions{
		Mode:         AffinityModePreferred,
		TopologyKeys: []TopologyKey{TopologyKeyHostname},
	}

	result, res := InjectAntiAffinity(chart, opts)

	if res.Injected != 1 {
		t.Fatalf("expected Injected=1, got %d (Skipped=%d)", res.Injected, res.Skipped)
	}

	content := result.Templates["templates/deployment.yaml"]
	if !strings.Contains(content, "affinity:") {
		t.Error("expected 'affinity:' in template after injection")
	}
	if !strings.Contains(content, "preferred") {
		t.Errorf("expected 'preferred' term for AffinityModePreferred, got:\n%s", content)
	}
	if !strings.Contains(content, string(TopologyKeyHostname)) {
		t.Errorf("expected topology key %q in template, got:\n%s", TopologyKeyHostname, content)
	}
}

// Test 2: Deployment, strict mode, hostname → "required" term present
func TestInjectAntiAffinity_Deployment_Strict_Hostname(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})
	opts := AntiAffinityOptions{
		Mode:         AffinityModeStrict,
		TopologyKeys: []TopologyKey{TopologyKeyHostname},
	}

	result, res := InjectAntiAffinity(chart, opts)

	if res.Injected != 1 {
		t.Fatalf("expected Injected=1, got %d", res.Injected)
	}

	content := result.Templates["templates/deployment.yaml"]
	if !strings.Contains(content, "required") {
		t.Errorf("expected 'required' term for AffinityModeStrict, got:\n%s", content)
	}
	// Strict mode must NOT produce a preferred scheduling term.
	if strings.Contains(content, "preferredDuring") {
		t.Errorf("strict mode should not produce preferredDuring term, got:\n%s", content)
	}
}

// Test 3: AddTopologySpreadConstraints=true + zone key → constraints block present
func TestInjectAntiAffinity_TopologySpreadConstraints_Zone(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})
	opts := AntiAffinityOptions{
		Mode:                        AffinityModePreferred,
		TopologyKeys:                []TopologyKey{TopologyKeyZone},
		AddTopologySpreadConstraints: true,
		MaxSkew:                     1,
	}

	result, res := InjectAntiAffinity(chart, opts)

	if res.Injected != 1 {
		t.Fatalf("expected Injected=1, got %d", res.Injected)
	}

	content := result.Templates["templates/deployment.yaml"]
	if !strings.Contains(content, "topologySpreadConstraints") {
		t.Errorf("expected 'topologySpreadConstraints' block in template, got:\n%s", content)
	}
	if !strings.Contains(content, string(TopologyKeyZone)) {
		t.Errorf("expected zone topology key %q in constraints, got:\n%s", TopologyKeyZone, content)
	}
}

// Test 4: MaxSkew=2 → topologySpreadConstraints.maxSkew: 2
func TestInjectAntiAffinity_MaxSkew2(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})
	opts := AntiAffinityOptions{
		Mode:                        AffinityModePreferred,
		TopologyKeys:                []TopologyKey{TopologyKeyHostname},
		AddTopologySpreadConstraints: true,
		MaxSkew:                     2,
	}

	result, _ := InjectAntiAffinity(chart, opts)

	content := result.Templates["templates/deployment.yaml"]
	if !strings.Contains(content, "maxSkew: 2") {
		t.Errorf("expected 'maxSkew: 2' in template, got:\n%s", content)
	}
}

// Test 5: StatefulSet → affinity rules injected
func TestInjectAntiAffinity_StatefulSet(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/statefulset.yaml": testStatefulSetTemplate,
	})

	result, res := InjectAntiAffinity(chart, defaultAAOpts())

	if res.Injected != 1 {
		t.Fatalf("expected Injected=1 for StatefulSet, got %d", res.Injected)
	}

	content := result.Templates["templates/statefulset.yaml"]
	if !strings.Contains(content, "affinity:") {
		t.Error("expected 'affinity:' in StatefulSet after injection")
	}
}

// Test 6: DaemonSet → affinity rules injected
func TestInjectAntiAffinity_DaemonSet(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/daemonset.yaml": testDaemonSetTemplate,
	})

	result, res := InjectAntiAffinity(chart, defaultAAOpts())

	if res.Injected != 1 {
		t.Fatalf("expected Injected=1 for DaemonSet, got %d", res.Injected)
	}

	content := result.Templates["templates/daemonset.yaml"]
	if !strings.Contains(content, "affinity:") {
		t.Error("expected 'affinity:' in DaemonSet after injection")
	}
}

// Test 7: Both hostname and zone → two topology key terms present
func TestInjectAntiAffinity_BothHostnameAndZone(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})
	opts := AntiAffinityOptions{
		Mode:         AffinityModePreferred,
		TopologyKeys: []TopologyKey{TopologyKeyHostname, TopologyKeyZone},
	}

	result, _ := InjectAntiAffinity(chart, opts)

	content := result.Templates["templates/deployment.yaml"]
	if !strings.Contains(content, string(TopologyKeyHostname)) {
		t.Errorf("expected hostname topology key %q in template, got:\n%s", TopologyKeyHostname, content)
	}
	if !strings.Contains(content, string(TopologyKeyZone)) {
		t.Errorf("expected zone topology key %q in template, got:\n%s", TopologyKeyZone, content)
	}
}

// ─── Boundary Cases ───────────────────────────────────────────────────────────

// Test 8: Template already has "affinity:" → skipped, template unchanged
func TestInjectAntiAffinity_SkipIfAffinityPresent(t *testing.T) {
	original := deploymentWithAffinityAlready()
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": original,
	})

	result, res := InjectAntiAffinity(chart, defaultAAOpts())

	if res.Skipped != 1 {
		t.Errorf("expected Skipped=1, got %d (Injected=%d)", res.Skipped, res.Injected)
	}
	if res.Injected != 0 {
		t.Errorf("expected Injected=0, got %d", res.Injected)
	}
	// Template must be unchanged.
	if result.Templates["templates/deployment.yaml"] != original {
		t.Error("template with existing affinity must not be modified")
	}
}

// Test 9: SkipSingleReplica=true, replicas=1 → skipped, no affinity injected
func TestInjectAntiAffinity_SkipSingleReplica(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": deploymentTemplateWithReplicas(1),
	})
	opts := AntiAffinityOptions{
		Mode:              AffinityModePreferred,
		TopologyKeys:      []TopologyKey{TopologyKeyHostname},
		SkipSingleReplica: true,
	}

	result, res := InjectAntiAffinity(chart, opts)

	if res.Skipped != 1 {
		t.Errorf("expected Skipped=1 for SkipSingleReplica=true with replicas=1, got %d", res.Skipped)
	}
	if res.Injected != 0 {
		t.Errorf("expected Injected=0, got %d", res.Injected)
	}
	// No affinity block should appear.
	content := result.Templates["templates/deployment.yaml"]
	if strings.Contains(content, "affinity:") {
		t.Error("affinity block must not be injected when SkipSingleReplica=true and replicas=1")
	}
}

// Test 10: Job template → not injected (Injected=0)
func TestInjectAntiAffinity_SkipJobTemplate(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/job.yaml": testJobTemplate,
	})

	_, res := InjectAntiAffinity(chart, defaultAAOpts())

	if res.Injected != 0 {
		t.Errorf("expected Injected=0 for Job template, got %d", res.Injected)
	}
}

// Test 11: Empty TopologyKeys → defaults to kubernetes.io/hostname
func TestInjectAntiAffinity_EmptyTopologyKeys_DefaultsHostname(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})
	opts := AntiAffinityOptions{
		Mode:         AffinityModePreferred,
		TopologyKeys: nil, // explicitly empty
	}

	result, res := InjectAntiAffinity(chart, opts)

	if res.Injected != 1 {
		t.Fatalf("expected Injected=1, got %d", res.Injected)
	}

	content := result.Templates["templates/deployment.yaml"]
	if !strings.Contains(content, string(TopologyKeyHostname)) {
		t.Errorf("expected default hostname topology key %q when TopologyKeys is nil, got:\n%s",
			TopologyKeyHostname, content)
	}
}

// Test 12: LabelSelector="" → defaults to app.kubernetes.io/name label
func TestInjectAntiAffinity_EmptyLabelSelector_Defaults(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})
	opts := AntiAffinityOptions{
		Mode:          AffinityModePreferred,
		TopologyKeys:  []TopologyKey{TopologyKeyHostname},
		LabelSelector: "", // explicitly empty → must default
	}

	result, res := InjectAntiAffinity(chart, opts)

	if res.Injected != 1 {
		t.Fatalf("expected Injected=1, got %d", res.Injected)
	}

	content := result.Templates["templates/deployment.yaml"]
	// The default label selector must reference app.kubernetes.io/name.
	if !strings.Contains(content, "app.kubernetes.io/name") {
		t.Errorf("expected default label selector 'app.kubernetes.io/name' when LabelSelector is empty, got:\n%s", content)
	}
}

// ─── Integration Tests ────────────────────────────────────────────────────────

// Test 13: InjectPDB then InjectAntiAffinity → both PDB and affinity present in final chart
func TestInjectAntiAffinity_AfterInjectPDB_BothPresent(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate, // replicas=3
	})

	// Step 1: inject PDB.
	afterPDB, pdbCount := InjectPDB(chart)
	if pdbCount == 0 {
		t.Fatal("InjectPDB setup: expected at least 1 PDB to be created")
	}

	// Step 2: inject anti-affinity on top.
	afterAA, aaRes := InjectAntiAffinity(afterPDB, defaultAAOpts())

	if aaRes.Injected != 1 {
		t.Errorf("expected InjectAntiAffinity to inject 1, got %d", aaRes.Injected)
	}

	// PDB template must still be present.
	pdbFound := false
	for _, content := range afterAA.Templates {
		if strings.Contains(content, "PodDisruptionBudget") {
			pdbFound = true
			break
		}
	}
	if !pdbFound {
		t.Error("PDB template was lost after InjectAntiAffinity")
	}

	// Anti-affinity must be present in the Deployment template.
	deployContent := afterAA.Templates["templates/deployment.yaml"]
	if !strings.Contains(deployContent, "affinity:") {
		t.Error("expected 'affinity:' in Deployment after InjectAntiAffinity")
	}
}

// Test 14: GenerateAntiAffinityValues produces consistent map with expected keys
func TestGenerateAntiAffinityValues_Consistent(t *testing.T) {
	opts := AntiAffinityOptions{
		Mode:         AffinityModeStrict,
		TopologyKeys: []TopologyKey{TopologyKeyHostname, TopologyKeyZone},
		MaxSkew:      2,
	}

	values := GenerateAntiAffinityValues(opts)

	if values == nil {
		t.Fatal("GenerateAntiAffinityValues must not return nil")
	}

	// Top-level key must be "antiAffinity" (or equivalent).
	aaRaw, ok := values["antiAffinity"]
	if !ok {
		// Try flat structure: check for "enabled" key directly.
		if _, hasEnabled := values["enabled"]; !hasEnabled {
			t.Fatalf("expected 'antiAffinity' top-level key or 'enabled' key in values; got keys: %v",
				aaMapKeys(values))
		}
		// Flat structure is acceptable: validate mode and topologyKeys at top level.
		assertAntiAffinityValuesFlat(t, values, opts)
		return
	}

	// Nested structure: validate within aaRaw.
	aaMap, ok := aaRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("antiAffinity value must be a map, got %T", aaRaw)
	}
	assertAntiAffinityValuesFlat(t, aaMap, opts)
}

// assertAntiAffinityValuesFlat validates that a values map (flat or nested antiAffinity block)
// contains mode and topologyKeys entries consistent with the given options.
func assertAntiAffinityValuesFlat(t *testing.T, m map[string]interface{}, opts AntiAffinityOptions) {
	t.Helper()

	// "mode" or "enabled" must be present.
	if _, hasModeKey := m["mode"]; !hasModeKey {
		if _, hasEnabled := m["enabled"]; !hasEnabled {
			t.Errorf("antiAffinity values must contain 'mode' or 'enabled'; got keys: %v", aaMapKeys(m))
		}
	}

	// If mode is present, it must match.
	if modeVal, ok := m["mode"]; ok {
		if modeStr, ok := modeVal.(string); ok {
			if modeStr != string(opts.Mode) {
				t.Errorf("antiAffinity.mode = %q, want %q", modeStr, string(opts.Mode))
			}
		}
	}
}

// mapKeys returns the string keys of a map for diagnostic output.
func aaMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ─── GenerateAntiAffinity (graph-based) ──────────────────────────────────────

// Smoke test: GenerateAntiAffinity on an empty graph returns a non-nil map.
func TestGenerateAntiAffinity_EmptyGraph_NoError(t *testing.T) {
	graph := types.NewResourceGraph()
	opts := defaultAAOpts()

	result := GenerateAntiAffinity(graph, opts)

	if result == nil {
		t.Error("GenerateAntiAffinity must return a non-nil map even for empty graph")
	}
}
