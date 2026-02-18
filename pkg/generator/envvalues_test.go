package generator

import (
	"testing"

	"sigs.k8s.io/yaml"
)

// baseVals returns a simple base values map for testing.
func baseVals(replicas int) map[string]interface{} {
	return map[string]interface{}{
		"replicaCount": replicas,
		"image": map[string]interface{}{
			"repository": "nginx",
			"tag":        "latest",
		},
		"logLevel": "info",
	}
}

func parseEnvYAML(t *testing.T, data []byte) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid YAML in env values: %v\ncontent:\n%s", err, string(data))
	}
	return out
}

// toInt converts an interface{} numeric value to int.
// sigs.k8s.io/yaml.Unmarshal returns float64 for integers (via JSON round-trip).
func toInt(v interface{}) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}

// ============================================================
// Subtask 1: Dev values profile
// ============================================================

func TestEnvValues_DevProfile_Replicas(t *testing.T) {
	envVals := GenerateEnvValues(baseVals(3))
	devData, ok := envVals["values-dev.yaml"]
	if !ok {
		t.Fatal("values-dev.yaml not in output")
	}
	parsed := parseEnvYAML(t, devData)

	if toInt(parsed["replicaCount"]) != 1 {
		t.Errorf("dev replicaCount: got %v, want 1", parsed["replicaCount"])
	}
}

func TestEnvValues_DevProfile_LogLevel(t *testing.T) {
	envVals := GenerateEnvValues(baseVals(3))
	parsed := parseEnvYAML(t, envVals["values-dev.yaml"])

	if parsed["logLevel"] != "debug" {
		t.Errorf("dev logLevel: got %v, want debug", parsed["logLevel"])
	}
}

func TestEnvValues_DevProfile_NoPDB(t *testing.T) {
	envVals := GenerateEnvValues(baseVals(3))
	parsed := parseEnvYAML(t, envVals["values-dev.yaml"])

	// Either no PDB section OR podDisruptionBudget.enabled: false
	if pdb, ok := parsed["podDisruptionBudget"]; ok {
		if pdbMap, ok := pdb.(map[string]interface{}); ok {
			if enabled, _ := pdbMap["enabled"]; enabled == true {
				t.Error("dev profile should have PDB disabled")
			}
		}
	}
}

func TestEnvValues_DevProfile_RelaxedResources(t *testing.T) {
	envVals := GenerateEnvValues(baseVals(3))
	parsed := parseEnvYAML(t, envVals["values-dev.yaml"])

	// Dev should have no resource limits (no resources section, or limits absent)
	if resources, ok := parsed["resources"]; ok {
		if resMap, ok := resources.(map[string]interface{}); ok {
			if _, hasLimits := resMap["limits"]; hasLimits {
				t.Error("dev profile should not have resource limits")
			}
		}
	}
}

// ============================================================
// Subtask 2: Staging values profile
// ============================================================

func TestEnvValues_StagingProfile_Replicas(t *testing.T) {
	envVals := GenerateEnvValues(baseVals(1))
	parsed := parseEnvYAML(t, envVals["values-staging.yaml"])

	if toInt(parsed["replicaCount"]) != 2 {
		t.Errorf("staging replicaCount: got %v, want 2", parsed["replicaCount"])
	}
}

func TestEnvValues_StagingProfile_LogLevel(t *testing.T) {
	envVals := GenerateEnvValues(baseVals(1))
	parsed := parseEnvYAML(t, envVals["values-staging.yaml"])

	if parsed["logLevel"] != "info" {
		t.Errorf("staging logLevel: got %v, want info", parsed["logLevel"])
	}
}

func TestEnvValues_StagingProfile_OptionalPDB(t *testing.T) {
	envVals := GenerateEnvValues(baseVals(3))
	parsed := parseEnvYAML(t, envVals["values-staging.yaml"])

	pdb, ok := parsed["podDisruptionBudget"]
	if !ok {
		t.Error("staging profile missing podDisruptionBudget section")
		return
	}
	pdbMap, ok := pdb.(map[string]interface{})
	if !ok {
		t.Error("podDisruptionBudget is not a map")
		return
	}
	if pdbMap["enabled"] != true {
		t.Error("staging PDB should be enabled")
	}
	if toInt(pdbMap["minAvailable"]) != 1 {
		t.Errorf("staging PDB minAvailable: got %v, want 1", pdbMap["minAvailable"])
	}
}

// ============================================================
// Subtask 3: Prod values profile
// ============================================================

func TestEnvValues_ProdProfile_Replicas(t *testing.T) {
	envVals := GenerateEnvValues(baseVals(1))
	parsed := parseEnvYAML(t, envVals["values-prod.yaml"])

	rc := toInt(parsed["replicaCount"])
	if rc < 3 {
		t.Errorf("prod replicaCount: got %v, want >=3", parsed["replicaCount"])
	}
}

func TestEnvValues_ProdProfile_LogLevel(t *testing.T) {
	envVals := GenerateEnvValues(baseVals(1))
	parsed := parseEnvYAML(t, envVals["values-prod.yaml"])

	if parsed["logLevel"] != "warn" {
		t.Errorf("prod logLevel: got %v, want warn", parsed["logLevel"])
	}
}

func TestEnvValues_ProdProfile_PDB(t *testing.T) {
	envVals := GenerateEnvValues(baseVals(3))
	parsed := parseEnvYAML(t, envVals["values-prod.yaml"])

	pdb, ok := parsed["podDisruptionBudget"]
	if !ok {
		t.Error("prod profile missing podDisruptionBudget section")
		return
	}
	pdbMap, ok := pdb.(map[string]interface{})
	if !ok {
		t.Error("podDisruptionBudget is not a map")
		return
	}
	if pdbMap["enabled"] != true {
		t.Error("prod PDB should be enabled")
	}
	if toInt(pdbMap["minAvailable"]) != 2 {
		t.Errorf("prod PDB minAvailable: got %v, want 2", pdbMap["minAvailable"])
	}
}

func TestEnvValues_ProdProfile_ResourceLimits(t *testing.T) {
	envVals := GenerateEnvValues(baseVals(3))
	parsed := parseEnvYAML(t, envVals["values-prod.yaml"])

	resources, ok := parsed["resources"]
	if !ok {
		t.Error("prod profile missing resources section")
		return
	}
	resMap, ok := resources.(map[string]interface{})
	if !ok {
		t.Error("resources is not a map")
		return
	}
	if _, hasLimits := resMap["limits"]; !hasLimits {
		t.Error("prod resources missing limits")
	}
	if _, hasRequests := resMap["requests"]; !hasRequests {
		t.Error("prod resources missing requests")
	}
}

func TestEnvValues_ProdProfile_Affinity(t *testing.T) {
	envVals := GenerateEnvValues(baseVals(3))
	parsed := parseEnvYAML(t, envVals["values-prod.yaml"])

	// Prod should have affinity or topologySpreadConstraints
	_, hasAffinity := parsed["affinity"]
	_, hasTopology := parsed["topologySpreadConstraints"]
	if !hasAffinity && !hasTopology {
		t.Error("prod profile missing affinity or topologySpreadConstraints")
	}
}

// ============================================================
// Subtask 4: Environment detection from source resources
// ============================================================

func TestEnvValues_Detection_HighReplicasIsProdLike(t *testing.T) {
	// Input: 5 replicas (prod-like) → dev/staging scale DOWN
	envVals := GenerateEnvValues(baseVals(5))

	devParsed := parseEnvYAML(t, envVals["values-dev.yaml"])
	if toInt(devParsed["replicaCount"]) != 1 {
		t.Errorf("dev should scale down from 5 replicas to 1, got %v", devParsed["replicaCount"])
	}

	stagingParsed := parseEnvYAML(t, envVals["values-staging.yaml"])
	if toInt(stagingParsed["replicaCount"]) != 2 {
		t.Errorf("staging should use 2 replicas regardless of base 5, got %v", stagingParsed["replicaCount"])
	}
}

func TestEnvValues_Detection_LowReplicasIsDevLike(t *testing.T) {
	// Input: 1 replica (dev-like) → staging/prod scale UP
	envVals := GenerateEnvValues(baseVals(1))

	stagingParsed := parseEnvYAML(t, envVals["values-staging.yaml"])
	if toInt(stagingParsed["replicaCount"]) != 2 {
		t.Errorf("staging should scale up to 2, got %v", stagingParsed["replicaCount"])
	}

	prodParsed := parseEnvYAML(t, envVals["values-prod.yaml"])
	rc := toInt(prodParsed["replicaCount"])
	if rc < 3 {
		t.Errorf("prod should scale up to >=3, got %v", prodParsed["replicaCount"])
	}
}

// ============================================================
// Subtask 5: File naming convention
// ============================================================

func TestEnvValues_FileNaming(t *testing.T) {
	envVals := GenerateEnvValues(baseVals(2))

	expected := []string{"values-dev.yaml", "values-staging.yaml", "values-prod.yaml"}
	for _, name := range expected {
		if _, ok := envVals[name]; !ok {
			t.Errorf("expected file %q in output, not found. Keys: %v", name, mapKeys(envVals))
		}
	}
}

func TestEnvValues_FileNaming_WithOutputDir(t *testing.T) {
	envVals := GenerateEnvValues(baseVals(2))

	// All files must use the standard naming convention
	if len(envVals) != 3 {
		t.Errorf("expected exactly 3 env value files, got %d: %v", len(envVals), mapKeys(envVals))
	}
}

// ============================================================
// Subtask 6: Values override structure (only differences from base)
// ============================================================

func TestEnvValues_OverrideOnly(t *testing.T) {
	base := baseVals(3)
	envVals := GenerateEnvValues(base)

	devParsed := parseEnvYAML(t, envVals["values-dev.yaml"])

	// Dev values MUST NOT include the full image section from base (override only)
	// Dev changes: replicaCount, logLevel, podDisruptionBudget
	// Dev should NOT change: image (not environment specific)
	if _, hasImage := devParsed["image"]; hasImage {
		t.Error("dev values should not include 'image' section (override-only principle)")
	}
}

// ============================================================
// Subtask 7: CLI flag integration (unit-level)
// ============================================================

func TestEnvValues_CLIFlag_Enabled(t *testing.T) {
	// When EnvValues flag is enabled, GenerateEnvValues returns 3 files
	result := GenerateEnvValues(baseVals(2))
	if len(result) != 3 {
		t.Errorf("expected 3 env value files when enabled, got %d", len(result))
	}
}

func TestEnvValues_CLIFlag_Disabled(t *testing.T) {
	// When EnvValues flag is disabled, no env files generated
	// This is tested via Options.EnvValues field
	opts := Options{}
	if opts.EnvValues {
		t.Error("EnvValues option should default to false (disabled)")
	}
}

// ============================================================
// Subtask 8: Works with all output modes
// ============================================================

func TestEnvValues_WithUniversalMode(t *testing.T) {
	// GenerateEnvValues works independently of output mode
	envVals := GenerateEnvValues(baseVals(2))
	// Universal mode: single values.yaml → env files are supplements
	if _, ok := envVals["values-dev.yaml"]; !ok {
		t.Error("values-dev.yaml missing for universal mode context")
	}
	if _, ok := envVals["values-prod.yaml"]; !ok {
		t.Error("values-prod.yaml missing for universal mode context")
	}
}

func TestEnvValues_WithSeparateMode(t *testing.T) {
	// GenerateEnvValues works for per-service values too
	envVals := GenerateEnvValues(baseVals(1))
	if len(envVals) != 3 {
		t.Errorf("expected 3 env files for separate mode context, got %d", len(envVals))
	}
}

func TestEnvValues_WithUmbrellaMode(t *testing.T) {
	// GenerateEnvValues can be applied to umbrella parent values
	envVals := GenerateEnvValues(map[string]interface{}{
		"global":   map[string]interface{}{"imageRegistry": ""},
		"frontend": map[string]interface{}{"replicaCount": 2, "enabled": true},
		"backend":  map[string]interface{}{"replicaCount": 3, "enabled": true},
	})
	if len(envVals) != 3 {
		t.Errorf("expected 3 env files for umbrella context, got %d", len(envVals))
	}
}

// ============================================================
// Helpers
// ============================================================

func mapKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
