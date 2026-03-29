package generator

// ============================================================
// Test Plan — autofix_pdb_test.go
//
// Subject: GenerateAutoPDB(chart *types.GeneratedChart) (*types.GeneratedChart, AutoPDBResult)
//
//  # | Test Name                                           | Category    | Input                                    | Expected Output
// ---+-----------------------------------------------------+-------------+------------------------------------------+--------------------------------------------------
//  1 | TestGenerateAutoPDB_Deployment_Replicas2_MinAvail   | happy       | Deployment replicas=2                    | PDB generated, minAvailable: 1
//  2 | TestGenerateAutoPDB_Deployment_Replicas3_MinAvail   | happy       | Deployment replicas=3 (at threshold)     | PDB generated, minAvailable: 1
//  3 | TestGenerateAutoPDB_Deployment_Replicas4_MaxUnavail | happy       | Deployment replicas=4                    | PDB generated, maxUnavailable: "25%"
//  4 | TestGenerateAutoPDB_Deployment_Replicas10_MaxUnavail| happy       | Deployment replicas=10                   | PDB generated, maxUnavailable: "25%"
//  5 | TestGenerateAutoPDB_StatefulSet_Replicas2           | happy       | StatefulSet replicas=2                   | PDB generated (1 count)
//  6 | TestGenerateAutoPDB_MultiDeployment_MixedReplicas   | happy       | 3 Deployments replicas=1,2,5             | 1 skipped, 2 generated
//  7 | TestGenerateAutoPDB_Replicas1_Skipped               | boundary    | replicas=1                               | Skipped=1, Generated=0
//  8 | TestGenerateAutoPDB_PDBKeyExists_Idempotent         | boundary    | PDB key already present in Templates     | Skipped (no duplicate)
//  9 | TestGenerateAutoPDB_HelmExpression_Skipped          | boundary    | replicas: {{ .Values.replicas }}         | Skipped conservatively
// 10 | TestGenerateAutoPDB_EmptyChart_NoOp                 | boundary    | chart with no templates                  | Generated=0, Skipped=0
// 11 | TestGenerateAutoPDB_AfterInjectPDB_NoDuplicate      | integration | InjectPDB then GenerateAutoPDB           | no duplicate PDB keys added
// 12 | TestGenerateAutoPDB_ResultCounts_Correct            | integration | 2 Deployments replicas=2 and 5           | Generated=2, Skipped=0, Details non-empty
// ============================================================

import (
	"fmt"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// makeAutoPDBTestChart builds a chart with a single Deployment template.
// templateName is used as the map key (e.g. "templates/deploy.yaml").
// replicas is inserted as a literal integer; pass -1 to inject a Helm expression instead.
func makeAutoPDBTestChart(templateName string, replicas int) *types.GeneratedChart {
	var replicasField string
	if replicas < 0 {
		replicasField = "replicas: {{ .Values.replicaCount }}"
	} else {
		replicasField = fmt.Sprintf("replicas: %d", replicas)
	}

	content := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
spec:
  %s
  selector:
    matchLabels:
      app: test-app
  template:
    spec:
      containers:
      - name: app
        image: nginx:latest`, replicasField)

	return &types.GeneratedChart{
		Name: "test-chart",
		Templates: map[string]string{
			templateName: content,
		},
	}
}

// makeAutoPDBStatefulSetChart builds a chart with a single StatefulSet template.
func makeAutoPDBStatefulSetChart(templateName string, replicas int) *types.GeneratedChart {
	content := fmt.Sprintf(`apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: test-db
spec:
  replicas: %d
  selector:
    matchLabels:
      app: test-db
  template:
    spec:
      containers:
      - name: db
        image: postgres:16`, replicas)

	return &types.GeneratedChart{
		Name: "test-chart",
		Templates: map[string]string{
			templateName: content,
		},
	}
}

// assertAutoPDBGenerated is a t.Helper that asserts a PDB was placed at pdbKey
// and that the PDB YAML contains the expected strategy substring.
func assertAutoPDBGenerated(t *testing.T, result *types.GeneratedChart, pdbKey, wantStrategy string) {
	t.Helper()
	pdb, ok := result.Templates[pdbKey]
	if !ok {
		t.Fatalf("expected PDB at key %q but not found; templates: %v", pdbKey, autoPDBTemplateKeys(result))
	}
	if !strings.Contains(pdb, "PodDisruptionBudget") {
		t.Errorf("PDB at %q does not contain 'PodDisruptionBudget':\n%s", pdbKey, pdb)
	}
	if wantStrategy != "" && !strings.Contains(pdb, wantStrategy) {
		t.Errorf("PDB at %q: expected strategy %q not found:\n%s", pdbKey, wantStrategy, pdb)
	}
}

// templateKeys returns the sorted list of template map keys for diagnostic output.
func autoPDBTemplateKeys(chart *types.GeneratedChart) []string {
	keys := make([]string, 0, len(chart.Templates))
	for k := range chart.Templates {
		keys = append(keys, k)
	}
	return keys
}

// ─── Happy Path ───────────────────────────────────────────────────────────────

// Test 1: Deployment replicas=2 → PDB with minAvailable: 1
// At the PDBSmallReplicaThreshold=3 and below (but >=PDBSkipReplicas+1=3) wait...
// PDBSkipReplicas=2 means replicas<=2 are skipped according to spec — but
// the spec says replicas=2 → PDB with minAvailable:1. Accepting spec as truth.
func TestGenerateAutoPDB_Deployment_Replicas2_MinAvail(t *testing.T) {
	chart := makeAutoPDBTestChart("templates/deployment.yaml", 2)

	result, res := GenerateAutoPDB(chart)

	if res.Generated != 1 {
		t.Fatalf("expected Generated=1, got %d (Skipped=%d)", res.Generated, res.Skipped)
	}
	assertAutoPDBGenerated(t, result, "templates/deployment-pdb.yaml", "minAvailable: 1")
}

// Test 2: Deployment replicas=3 → minAvailable:1 (at threshold PDBSmallReplicaThreshold=3)
func TestGenerateAutoPDB_Deployment_Replicas3_MinAvail(t *testing.T) {
	chart := makeAutoPDBTestChart("templates/deployment.yaml", 3)

	result, res := GenerateAutoPDB(chart)

	if res.Generated != 1 {
		t.Fatalf("expected Generated=1, got %d", res.Generated)
	}
	assertAutoPDBGenerated(t, result, "templates/deployment-pdb.yaml", "minAvailable: 1")
}

// Test 3: Deployment replicas=4 → maxUnavailable: "25%"
func TestGenerateAutoPDB_Deployment_Replicas4_MaxUnavail(t *testing.T) {
	chart := makeAutoPDBTestChart("templates/deployment.yaml", 4)

	result, res := GenerateAutoPDB(chart)

	if res.Generated != 1 {
		t.Fatalf("expected Generated=1, got %d", res.Generated)
	}
	assertAutoPDBGenerated(t, result, "templates/deployment-pdb.yaml", PDBMaxUnavailablePercent)
}

// Test 4: Deployment replicas=10 → maxUnavailable: "25%"
func TestGenerateAutoPDB_Deployment_Replicas10_MaxUnavail(t *testing.T) {
	chart := makeAutoPDBTestChart("templates/deployment.yaml", 10)

	result, res := GenerateAutoPDB(chart)

	if res.Generated != 1 {
		t.Fatalf("expected Generated=1, got %d", res.Generated)
	}
	pdbKey := "templates/deployment-pdb.yaml"
	assertAutoPDBGenerated(t, result, pdbKey, PDBMaxUnavailablePercent)

	// Confirm it does NOT use minAvailable for large replicas.
	pdb := result.Templates[pdbKey]
	if strings.Contains(pdb, "minAvailable:") {
		t.Errorf("replicas=10 should use maxUnavailable, not minAvailable; got:\n%s", pdb)
	}
}

// Test 5: StatefulSet replicas=2 → PDB generated
func TestGenerateAutoPDB_StatefulSet_Replicas2(t *testing.T) {
	chart := makeAutoPDBStatefulSetChart("templates/statefulset.yaml", 2)

	result, res := GenerateAutoPDB(chart)

	if res.Generated != 1 {
		t.Fatalf("expected Generated=1 for StatefulSet replicas=2, got %d", res.Generated)
	}
	assertAutoPDBGenerated(t, result, "templates/statefulset-pdb.yaml", "")
}

// Test 6: 3 Deployments with replicas=1, 2, 5 → 1 skipped, 2 generated
// replicas=1 is skipped (below skip threshold), replicas=2 and 5 generate PDBs.
func TestGenerateAutoPDB_MultiDeployment_MixedReplicas(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "test-chart",
		Templates: map[string]string{
			"templates/deploy-a.yaml": deploymentWithReplicas("app-a", 1),
			"templates/deploy-b.yaml": deploymentWithReplicas("app-b", 2),
			"templates/deploy-c.yaml": deploymentWithReplicas("app-c", 5),
		},
	}

	_, res := GenerateAutoPDB(chart)

	if res.Skipped != 1 {
		t.Errorf("expected Skipped=1, got %d", res.Skipped)
	}
	if res.Generated != 2 {
		t.Errorf("expected Generated=2, got %d", res.Generated)
	}
}

// deploymentWithReplicas builds a Deployment template string for use in multi-template tests.
func deploymentWithReplicas(name string, replicas int) string {
	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
spec:
  replicas: %d
  template:
    spec:
      containers:
      - name: app
        image: nginx:latest`, name, replicas)
}

// ─── Boundary Cases ───────────────────────────────────────────────────────────

// Test 7: replicas=1 → skipped (below PDBSkipReplicas+1 threshold)
func TestGenerateAutoPDB_Replicas1_Skipped(t *testing.T) {
	chart := makeAutoPDBTestChart("templates/deployment.yaml", 1)

	result, res := GenerateAutoPDB(chart)

	if res.Generated != 0 {
		t.Errorf("expected Generated=0 for replicas=1, got %d", res.Generated)
	}
	if res.Skipped != 1 {
		t.Errorf("expected Skipped=1 for replicas=1, got %d", res.Skipped)
	}
	// No PDB key should appear.
	if _, exists := result.Templates["templates/deployment-pdb.yaml"]; exists {
		t.Error("no PDB should be created for replicas=1")
	}
}

// Test 8: PDB key already exists → skipped (idempotent)
func TestGenerateAutoPDB_PDBKeyExists_Idempotent(t *testing.T) {
	const existingPDB = `apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: existing-pdb`

	chart := &types.GeneratedChart{
		Name: "test-chart",
		Templates: map[string]string{
			"templates/deployment.yaml":     makeAutoPDBTestChart("templates/deployment.yaml", 4).Templates["templates/deployment.yaml"],
			"templates/deployment-pdb.yaml": existingPDB,
		},
	}

	result, res := GenerateAutoPDB(chart)

	// Should not overwrite existing PDB.
	if res.Generated != 0 {
		t.Errorf("expected Generated=0 when PDB already exists, got %d", res.Generated)
	}
	if res.Skipped != 1 {
		t.Errorf("expected Skipped=1 when PDB already exists, got %d", res.Skipped)
	}
	// The existing PDB must be preserved unchanged.
	if got := result.Templates["templates/deployment-pdb.yaml"]; got != existingPDB {
		t.Errorf("existing PDB was modified; want:\n%s\ngot:\n%s", existingPDB, got)
	}
}

// Test 9: Helm expression for replicas → skipped conservatively
func TestGenerateAutoPDB_HelmExpression_Skipped(t *testing.T) {
	// replicas < 0 triggers Helm-expression path in makeAutoPDBTestChart.
	chart := makeAutoPDBTestChart("templates/deployment.yaml", -1)

	_, res := GenerateAutoPDB(chart)

	// When replicas cannot be parsed as an integer, the function must skip safely.
	if res.Generated != 0 {
		t.Errorf("expected Generated=0 for Helm expression replicas, got %d", res.Generated)
	}
}

// Test 10: Empty chart → no-op
func TestGenerateAutoPDB_EmptyChart_NoOp(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:      "empty-chart",
		Templates: map[string]string{},
	}

	result, res := GenerateAutoPDB(chart)

	if res.Generated != 0 || res.Skipped != 0 {
		t.Errorf("empty chart must produce Generated=0, Skipped=0; got Generated=%d Skipped=%d",
			res.Generated, res.Skipped)
	}
	if len(result.Templates) != 0 {
		t.Errorf("result should have no templates, got %d", len(result.Templates))
	}
}

// ─── Integration Tests ────────────────────────────────────────────────────────

// Test 11: InjectPDB then GenerateAutoPDB → no duplicate PDB keys
func TestGenerateAutoPDB_AfterInjectPDB_NoDuplicate(t *testing.T) {
	// testDeploymentTemplate has replicas=3, so InjectPDB will create a PDB.
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})

	afterInject, injectCount := InjectPDB(chart)
	if injectCount == 0 {
		t.Fatal("InjectPDB should have generated at least 1 PDB for setup")
	}

	beforeCount := len(afterInject.Templates)
	afterAuto, res := GenerateAutoPDB(afterInject)

	// GenerateAutoPDB must not create a duplicate where InjectPDB already created one.
	if len(afterAuto.Templates) > beforeCount {
		t.Errorf("GenerateAutoPDB added templates after InjectPDB; beforeCount=%d afterCount=%d",
			beforeCount, len(afterAuto.Templates))
	}
	if res.Generated != 0 {
		t.Errorf("expected Generated=0 after InjectPDB already ran, got %d (Skipped=%d)",
			res.Generated, res.Skipped)
	}
}

// Test 12: GenerateAutoPDB result counts correct for 2 qualifying Deployments
func TestGenerateAutoPDB_ResultCounts_Correct(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "test-chart",
		Templates: map[string]string{
			"templates/deploy-x.yaml": deploymentWithReplicas("deploy-x", 2),
			"templates/deploy-y.yaml": deploymentWithReplicas("deploy-y", 5),
		},
	}

	result, res := GenerateAutoPDB(chart)

	if res.Generated != 2 {
		t.Errorf("expected Generated=2, got %d", res.Generated)
	}
	if res.Skipped != 0 {
		t.Errorf("expected Skipped=0, got %d", res.Skipped)
	}
	if res.Details == nil {
		t.Error("Details map must not be nil")
	}
	if len(res.Details) != 2 {
		t.Errorf("expected 2 Detail entries, got %d: %v", len(res.Details), res.Details)
	}
	// Both PDB keys must exist in the result chart.
	for _, key := range []string{"templates/deploy-x-pdb.yaml", "templates/deploy-y-pdb.yaml"} {
		if _, ok := result.Templates[key]; !ok {
			t.Errorf("expected PDB key %q in result templates", key)
		}
	}
	// copy-on-write: original chart must be unchanged.
	if _, exists := chart.Templates["templates/deploy-x-pdb.yaml"]; exists {
		t.Error("original chart must not be mutated by GenerateAutoPDB")
	}
}
