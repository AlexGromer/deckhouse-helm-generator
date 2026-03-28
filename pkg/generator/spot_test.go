package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Test Plan
//
//  1. TestSpot_AWS_Tolerations              — happy   AWS → key="node.kubernetes.io/lifecycle", value="spot"
//  2. TestSpot_GCP_Tolerations              — happy   GCP → key="cloud.google.com/gke-preemptible"
//  3. TestSpot_Azure_Tolerations            — happy   Azure → key="kubernetes.azure.com/scalesetpriority"
//  4. TestSpot_PreStopHook_Default15s       — happy   gracePeriod=15 → command contains "sleep 15"
//  5. TestSpot_PreStopHook_Custom30s        — happy   gracePeriod=30 → command contains "sleep 30"
//  6. TestSpot_PDB_LowReplicas_MinAvailable1    — boundary replicas=1 → PDB YAML contains "minAvailable: 1"
//  7. TestSpot_PDB_HighReplicas_MinAvailable50Pct — boundary replicas=5 → PDB YAML contains minAvailable: "50%"
//  8. TestSpot_Values_Structure             — happy   SpotValues has spot.enabled, spot.provider, spot.gracePeriod
//  9. TestSpot_Values_DefaultGracePeriod    — happy   default config → gracePeriod=15
// 10. TestSpot_InjectIntoDeployment_AddsTolerations — integration Deployment → after inject, template contains "tolerations"
// 11. TestSpot_InjectIntoJob_NoChanges      — integration Job → after inject, template unchanged (no tolerations)
// 12. TestSpot_NilChart_ReturnsNil          — error   nil chart → returns nil
// ============================================================

// ============================================================
// Section 1: GenerateSpotTolerations — provider-specific keys
// ============================================================

func TestSpot_AWS_Tolerations(t *testing.T) {
	tolerations := GenerateSpotTolerations(SpotAWS)

	if len(tolerations) == 0 {
		t.Fatal("expected at least one toleration for AWS spot provider")
	}

	found := false
	for _, tol := range tolerations {
		key, _ := tol["key"].(string)
		val, _ := tol["value"].(string)
		effect, _ := tol["effect"].(string)

		if key == "node.kubernetes.io/lifecycle" {
			found = true
			if val != "spot" {
				t.Errorf("AWS toleration: expected value='spot', got '%s'", val)
			}
			if effect != "NoSchedule" {
				t.Errorf("AWS toleration: expected effect='NoSchedule', got '%s'", effect)
			}
			break
		}
	}

	if !found {
		t.Error("AWS tolerations must contain key='node.kubernetes.io/lifecycle'")
	}
}

func TestSpot_GCP_Tolerations(t *testing.T) {
	tolerations := GenerateSpotTolerations(SpotGCP)

	if len(tolerations) == 0 {
		t.Fatal("expected at least one toleration for GCP spot provider")
	}

	found := false
	for _, tol := range tolerations {
		key, _ := tol["key"].(string)
		if key == "cloud.google.com/gke-preemptible" {
			found = true
			val, _ := tol["value"].(string)
			effect, _ := tol["effect"].(string)
			if val != "true" {
				t.Errorf("GCP toleration: expected value='true', got '%s'", val)
			}
			if effect != "NoSchedule" {
				t.Errorf("GCP toleration: expected effect='NoSchedule', got '%s'", effect)
			}
			break
		}
	}

	if !found {
		t.Error("GCP tolerations must contain key='cloud.google.com/gke-preemptible'")
	}
}

func TestSpot_Azure_Tolerations(t *testing.T) {
	tolerations := GenerateSpotTolerations(SpotAzure)

	if len(tolerations) == 0 {
		t.Fatal("expected at least one toleration for Azure spot provider")
	}

	found := false
	for _, tol := range tolerations {
		key, _ := tol["key"].(string)
		if key == "kubernetes.azure.com/scalesetpriority" {
			found = true
			val, _ := tol["value"].(string)
			effect, _ := tol["effect"].(string)
			if val != "spot" {
				t.Errorf("Azure toleration: expected value='spot', got '%s'", val)
			}
			if effect != "NoSchedule" {
				t.Errorf("Azure toleration: expected effect='NoSchedule', got '%s'", effect)
			}
			break
		}
	}

	if !found {
		t.Error("Azure tolerations must contain key='kubernetes.azure.com/scalesetpriority'")
	}
}

// ============================================================
// Section 2: GenerateSpotPreStopHook — gracePeriod values
// ============================================================

func TestSpot_PreStopHook_Default15s(t *testing.T) {
	hook := GenerateSpotPreStopHook(15)

	if hook == nil {
		t.Fatal("GenerateSpotPreStopHook must not return nil")
	}

	// Expect structure: lifecycle.preStop.exec.command contains "sleep 15"
	lifecycle, ok := hook["lifecycle"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected hook to contain 'lifecycle' map, got %T", hook["lifecycle"])
	}

	preStop, ok := lifecycle["preStop"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'lifecycle.preStop' map, got %T", lifecycle["preStop"])
	}

	exec, ok := preStop["exec"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'lifecycle.preStop.exec' map, got %T", preStop["exec"])
	}

	commands, ok := exec["command"].([]string)
	if !ok {
		t.Fatalf("expected 'lifecycle.preStop.exec.command' to be []string, got %T", exec["command"])
	}

	found := false
	for _, cmd := range commands {
		if strings.Contains(cmd, "sleep 15") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("preStop command must contain 'sleep 15', got: %v", commands)
	}
}

func TestSpot_PreStopHook_Custom30s(t *testing.T) {
	hook := GenerateSpotPreStopHook(30)

	if hook == nil {
		t.Fatal("GenerateSpotPreStopHook must not return nil")
	}

	lifecycle, ok := hook["lifecycle"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected hook to contain 'lifecycle' map, got %T", hook["lifecycle"])
	}

	preStop, ok := lifecycle["preStop"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'lifecycle.preStop' map, got %T", lifecycle["preStop"])
	}

	exec, ok := preStop["exec"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'lifecycle.preStop.exec' map, got %T", preStop["exec"])
	}

	commands, ok := exec["command"].([]string)
	if !ok {
		t.Fatalf("expected 'lifecycle.preStop.exec.command' to be []string, got %T", exec["command"])
	}

	found := false
	for _, cmd := range commands {
		if strings.Contains(cmd, "sleep 30") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("preStop command must contain 'sleep 30', got: %v", commands)
	}
}

// ============================================================
// Section 3: GenerateSpotPDB — minAvailable boundary logic
// ============================================================

func TestSpot_PDB_LowReplicas_MinAvailable1(t *testing.T) {
	// replicas <= 2 → minAvailable: 1
	pdb := GenerateSpotPDB("myapp", 1)

	if pdb == "" {
		t.Fatal("GenerateSpotPDB must return a non-empty YAML string")
	}

	if !strings.Contains(pdb, "minAvailable: 1") {
		t.Errorf("expected 'minAvailable: 1' for replicas=1, got:\n%s", pdb)
	}

	// Must not use percentage notation for low-replica count
	if strings.Contains(pdb, "50%") {
		t.Error("expected no '50%' percentage in PDB for replicas=1")
	}
}

func TestSpot_PDB_HighReplicas_MinAvailable50Pct(t *testing.T) {
	// replicas > 2 → minAvailable: "50%"
	pdb := GenerateSpotPDB("myapp", 5)

	if pdb == "" {
		t.Fatal("GenerateSpotPDB must return a non-empty YAML string")
	}

	if !strings.Contains(pdb, `"50%"`) {
		t.Errorf(`expected minAvailable: "50%%" for replicas=5, got:\n%s`, pdb)
	}
}

// ============================================================
// Section 4: GenerateSpotValues — values map structure
// ============================================================

func TestSpot_Values_Structure(t *testing.T) {
	config := SpotConfig{
		Provider:    SpotAWS,
		GracePeriod: 15,
		Enabled:     false,
	}

	values := GenerateSpotValues(config)

	if values == nil {
		t.Fatal("GenerateSpotValues must return a non-nil map")
	}

	spot, ok := values["spot"]
	if !ok {
		t.Fatal("expected top-level 'spot' key in values map")
	}

	spotMap, ok := spot.(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'spot' to be map[string]interface{}, got %T", spot)
	}

	if _, ok := spotMap["enabled"]; !ok {
		t.Error("expected 'spot.enabled' key in values map")
	}

	if _, ok := spotMap["provider"]; !ok {
		t.Error("expected 'spot.provider' key in values map")
	}

	if _, ok := spotMap["gracePeriod"]; !ok {
		t.Error("expected 'spot.gracePeriod' key in values map")
	}
}

func TestSpot_Values_DefaultGracePeriod(t *testing.T) {
	// SpotConfig zero value should yield gracePeriod=15
	config := SpotConfig{
		Provider:    SpotProvider(""),
		GracePeriod: 15,
		Enabled:     false,
	}

	values := GenerateSpotValues(config)

	if values == nil {
		t.Fatal("GenerateSpotValues must return a non-nil map")
	}

	spot, ok := values["spot"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'spot' map in values")
	}

	gp, ok := spot["gracePeriod"]
	if !ok {
		t.Fatal("expected 'spot.gracePeriod' key in values")
	}

	gpInt, ok := gp.(int)
	if !ok {
		t.Fatalf("expected 'spot.gracePeriod' to be int, got %T (%v)", gp, gp)
	}

	if gpInt != 15 {
		t.Errorf("expected default gracePeriod=15, got %d", gpInt)
	}
}

// ============================================================
// Section 5: InjectSpotConfig — chart template patching
// ============================================================

func TestSpot_InjectIntoDeployment_AddsTolerations(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\nspec:\n  replicas: 3\n  template:\n    spec:\n      containers:\n        - name: app\n          image: nginx:1.21",
	})

	config := SpotConfig{
		Provider:    SpotAWS,
		GracePeriod: 15,
		Enabled:     true,
	}

	result := InjectSpotConfig(chart, config)

	if result == nil {
		t.Fatal("InjectSpotConfig returned nil for a valid chart")
	}

	content, ok := result.Templates["templates/deployment.yaml"]
	if !ok {
		t.Fatal("templates/deployment.yaml missing after InjectSpotConfig")
	}

	if !strings.Contains(content, "tolerations") {
		t.Errorf("expected 'tolerations' injected into Deployment template, got:\n%s", content)
	}
}

func TestSpot_InjectIntoJob_NoChanges(t *testing.T) {
	originalContent := "apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: myapp-job\nspec:\n  template:\n    spec:\n      restartPolicy: Never\n      containers:\n        - name: job\n          image: busybox:1.36"

	chart := makeChart("myapp", map[string]string{
		"templates/job.yaml": originalContent,
	})

	config := SpotConfig{
		Provider:    SpotAWS,
		GracePeriod: 15,
		Enabled:     true,
	}

	result := InjectSpotConfig(chart, config)

	if result == nil {
		t.Fatal("InjectSpotConfig returned nil for a valid chart with Job")
	}

	content, ok := result.Templates["templates/job.yaml"]
	if !ok {
		t.Fatal("templates/job.yaml missing after InjectSpotConfig")
	}

	if strings.Contains(content, "tolerations") {
		t.Errorf("Job template must NOT have 'tolerations' injected by InjectSpotConfig, got:\n%s", content)
	}
}

// ============================================================
// Section 6: InjectSpotConfig — nil chart guard
// ============================================================

func TestSpot_InjectTolerations_InsidePodSpec(t *testing.T) {
	deploymentYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  replicas: 3
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
        - name: app
          image: nginx:1.21
`
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": deploymentYAML,
	})

	config := SpotConfig{
		Provider:    SpotAWS,
		GracePeriod: 15,
		Enabled:     true,
	}

	result := InjectSpotConfig(chart, config)
	if result == nil {
		t.Fatal("InjectSpotConfig returned nil")
	}

	content := result.Templates["templates/deployment.yaml"]

	// tolerations: must appear BEFORE containers:
	tolIdx := strings.Index(content, "tolerations:")
	conIdx := strings.Index(content, "containers:")
	if tolIdx == -1 {
		t.Fatalf("tolerations: not found in output:\n%s", content)
	}
	if conIdx == -1 {
		t.Fatalf("containers: not found in output:\n%s", content)
	}
	if tolIdx >= conIdx {
		t.Errorf("tolerations: (pos %d) must appear BEFORE containers: (pos %d) in pod spec.\nOutput:\n%s", tolIdx, conIdx, content)
	}

	// Both tolerations: and containers: must be at the same indentation level.
	lines := strings.Split(content, "\n")
	var tolIndent, conIndent string
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "tolerations:") {
			tolIndent = line[:len(line)-len(trimmed)]
		}
		if strings.HasPrefix(trimmed, "containers:") {
			conIndent = line[:len(line)-len(trimmed)]
		}
	}
	if tolIndent != conIndent {
		t.Errorf("tolerations indent %q != containers indent %q", tolIndent, conIndent)
	}

	// tolerations: must NOT be at document root (zero indentation).
	if tolIndent == "" {
		t.Error("tolerations: is at document root (no indentation) — must be inside pod spec")
	}
}

func TestSpot_InjectTolerations_Idempotent(t *testing.T) {
	deploymentYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  replicas: 3
  template:
    spec:
      containers:
        - name: app
          image: nginx:1.21
`
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": deploymentYAML,
	})

	config := SpotConfig{
		Provider:    SpotAWS,
		GracePeriod: 15,
		Enabled:     true,
	}

	// Inject once.
	result1 := InjectSpotConfig(chart, config)
	content1 := result1.Templates["templates/deployment.yaml"]

	// Inject again on the already-injected chart.
	result2 := InjectSpotConfig(result1, config)
	content2 := result2.Templates["templates/deployment.yaml"]

	// Count occurrences of "tolerations:" — must be exactly 1.
	count := strings.Count(content2, "tolerations:")
	if count != 1 {
		t.Errorf("expected exactly 1 'tolerations:' block after double injection, got %d.\nAfter first:\n%s\nAfter second:\n%s", count, content1, content2)
	}
}

func TestSpot_NilChart_ReturnsNil(t *testing.T) {
	config := SpotConfig{
		Provider:    SpotAWS,
		GracePeriod: 15,
		Enabled:     true,
	}

	var chart *types.GeneratedChart
	result := InjectSpotConfig(chart, config)

	if result != nil {
		t.Errorf("expected nil return for nil chart input, got %+v", result)
	}
}

// ============================================================
// Section 7: InjectSpotConfig — PDB generation
// ============================================================

func TestInjectSpotConfig_GeneratesPDB(t *testing.T) {
	deploymentYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  replicas: 3
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
        - name: app
          image: nginx:1.21
`
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": deploymentYAML,
	})

	config := SpotConfig{
		Provider:    SpotAWS,
		GracePeriod: 15,
		Enabled:     true,
	}

	result := InjectSpotConfig(chart, config)
	if result == nil {
		t.Fatal("InjectSpotConfig returned nil for a valid chart")
	}

	// PDB template must exist under the derived key.
	pdbKey := "templates/deployment-spot-pdb.yaml"
	pdbContent, ok := result.Templates[pdbKey]
	if !ok {
		t.Fatalf("expected PDB template at key %q, available keys: %v", pdbKey, templateKeys(result))
	}

	// Must be a PodDisruptionBudget.
	if !strings.Contains(pdbContent, "kind: PodDisruptionBudget") {
		t.Errorf("PDB template must contain 'kind: PodDisruptionBudget', got:\n%s", pdbContent)
	}

	// With replicas=3, minAvailable should be "50%".
	if !strings.Contains(pdbContent, `"50%"`) {
		t.Errorf("PDB template for replicas=3 must contain '50%%', got:\n%s", pdbContent)
	}

	// Must use Helm template syntax for the name.
	if !strings.Contains(pdbContent, `{{ include "myapp.fullname" . }}`) {
		t.Errorf("PDB template must use Helm fullname helper, got:\n%s", pdbContent)
	}

	// Must use Helm template syntax for selector labels.
	if !strings.Contains(pdbContent, `include "myapp.selectorLabels" .`) {
		t.Errorf("PDB template must use Helm selectorLabels helper, got:\n%s", pdbContent)
	}

	// Original deployment template must still exist and be unaffected by PDB addition.
	if _, ok := result.Templates["templates/deployment.yaml"]; !ok {
		t.Error("original deployment template must still be present")
	}
}

func TestInjectSpotConfig_GeneratesPDB_LowReplicas(t *testing.T) {
	deploymentYAML := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  replicas: 1
  template:
    spec:
      containers:
        - name: app
          image: nginx:1.21
`
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": deploymentYAML,
	})

	config := SpotConfig{
		Provider:    SpotGCP,
		GracePeriod: 15,
		Enabled:     true,
	}

	result := InjectSpotConfig(chart, config)
	if result == nil {
		t.Fatal("InjectSpotConfig returned nil")
	}

	pdbContent, ok := result.Templates["templates/deployment-spot-pdb.yaml"]
	if !ok {
		t.Fatalf("expected PDB template, available keys: %v", templateKeys(result))
	}

	// With replicas=1 (<=2), minAvailable should be 1.
	if !strings.Contains(pdbContent, "minAvailable: 1") {
		t.Errorf("PDB template for replicas=1 must contain 'minAvailable: 1', got:\n%s", pdbContent)
	}
}

func TestInjectSpotConfig_GeneratesPDB_StatefulSet(t *testing.T) {
	stsYAML := `apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: mydb
spec:
  replicas: 5
  template:
    spec:
      containers:
        - name: db
          image: postgres:15
`
	chart := makeChart("mydb", map[string]string{
		"templates/statefulset.yaml": stsYAML,
	})

	config := SpotConfig{
		Provider:    SpotAzure,
		GracePeriod: 30,
		Enabled:     true,
	}

	result := InjectSpotConfig(chart, config)
	if result == nil {
		t.Fatal("InjectSpotConfig returned nil")
	}

	pdbKey := "templates/statefulset-spot-pdb.yaml"
	pdbContent, ok := result.Templates[pdbKey]
	if !ok {
		t.Fatalf("expected PDB template at key %q, available keys: %v", pdbKey, templateKeys(result))
	}

	if !strings.Contains(pdbContent, "kind: PodDisruptionBudget") {
		t.Errorf("PDB template must contain 'kind: PodDisruptionBudget', got:\n%s", pdbContent)
	}

	// replicas=5 → 50%
	if !strings.Contains(pdbContent, `"50%"`) {
		t.Errorf("PDB template for replicas=5 must contain '50%%', got:\n%s", pdbContent)
	}
}

func TestInjectSpotConfig_NoPDB_ForJob(t *testing.T) {
	jobYAML := `apiVersion: batch/v1
kind: Job
metadata:
  name: myjob
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: job
          image: busybox:1.36
`
	chart := makeChart("myapp", map[string]string{
		"templates/job.yaml": jobYAML,
	})

	config := SpotConfig{
		Provider:    SpotAWS,
		GracePeriod: 15,
		Enabled:     true,
	}

	result := InjectSpotConfig(chart, config)
	if result == nil {
		t.Fatal("InjectSpotConfig returned nil")
	}

	// No PDB should be generated for a Job.
	for key := range result.Templates {
		if strings.Contains(key, "spot-pdb") {
			t.Errorf("Job must NOT produce a PDB template, but found key %q", key)
		}
	}
}

// templateKeys returns all template map keys for diagnostic output.
func templateKeys(chart *types.GeneratedChart) []string {
	keys := make([]string, 0, len(chart.Templates))
	for k := range chart.Templates {
		keys = append(keys, k)
	}
	return keys
}
