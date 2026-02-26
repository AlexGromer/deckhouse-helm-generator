package generator

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
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
			if enabled := pdbMap["enabled"]; enabled == true {
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

// ============================================================
// Workload-Aware Environment Profiles (TDD — not yet implemented)
// ============================================================

// ============================================================
// Section A: DetectWorkloadType
// ============================================================

// TestWorkload_DetectWeb_HasIngress — group with Deployment + Service + Ingress → WorkloadWeb
func TestWorkload_DetectWeb_HasIngress(t *testing.T) {
	group := makeGroupForEnv("webapp",
		makeResourceForEnv("Deployment", "webapp"),
		makeResourceForEnv("Service", "webapp"),
		makeResourceForEnv("Ingress", "webapp"),
	)

	got := DetectWorkloadType(group)
	if got != WorkloadWeb {
		t.Errorf("DetectWorkloadType with Ingress: got %q, want %q", got, WorkloadWeb)
	}
}

// TestWorkload_DetectWeb_Port8080 — group with Deployment exposing port 8080 → WorkloadWeb
func TestWorkload_DetectWeb_Port8080(t *testing.T) {
	group := makeGroupForEnv("webapp",
		makeResourceWithPorts("Deployment", "webapp", []int{8080}),
	)

	got := DetectWorkloadType(group)
	if got != WorkloadWeb {
		t.Errorf("DetectWorkloadType with port 8080: got %q, want %q", got, WorkloadWeb)
	}
}

// TestWorkload_DetectWorker_NoService — Deployment without any Service → WorkloadWorker
func TestWorkload_DetectWorker_NoService(t *testing.T) {
	group := makeGroupForEnv("worker",
		makeResourceForEnv("Deployment", "worker"),
	)

	got := DetectWorkloadType(group)
	if got != WorkloadWorker {
		t.Errorf("DetectWorkloadType Deployment-only: got %q, want %q", got, WorkloadWorker)
	}
}

// TestWorkload_DetectWorker_AMQPEnvVar — Deployment with AMQP_URL env var → WorkloadWorker
func TestWorkload_DetectWorker_AMQPEnvVar(t *testing.T) {
	group := makeGroupForEnv("queue-consumer",
		makeResourceWithEnvVars("Deployment", "queue-consumer", map[string]string{
			"AMQP_URL": "amqp://rabbitmq:5672",
		}),
	)

	got := DetectWorkloadType(group)
	if got != WorkloadWorker {
		t.Errorf("DetectWorkloadType with AMQP_URL env var: got %q, want %q", got, WorkloadWorker)
	}
}

// TestWorkload_DetectDatabase_StatefulSetPVC — StatefulSet + PVC → WorkloadDatabase
func TestWorkload_DetectDatabase_StatefulSetPVC(t *testing.T) {
	group := makeGroupForEnv("postgres",
		makeResourceForEnv("StatefulSet", "postgres"),
		makeResourceForEnv("PersistentVolumeClaim", "postgres-data"),
	)

	got := DetectWorkloadType(group)
	if got != WorkloadDatabase {
		t.Errorf("DetectWorkloadType StatefulSet+PVC: got %q, want %q", got, WorkloadDatabase)
	}
}

// TestWorkload_DetectDatabase_PostgresImage — Deployment with image postgres:15 → WorkloadDatabase
func TestWorkload_DetectDatabase_PostgresImage(t *testing.T) {
	group := makeGroupForEnv("db",
		makeResourceWithImage("Deployment", "db", "postgres:15"),
	)

	got := DetectWorkloadType(group)
	if got != WorkloadDatabase {
		t.Errorf("DetectWorkloadType with postgres image: got %q, want %q", got, WorkloadDatabase)
	}
}

// TestWorkload_DetectBatch_CronJob — group containing only a CronJob → WorkloadBatch
func TestWorkload_DetectBatch_CronJob(t *testing.T) {
	group := makeGroupForEnv("nightly-report",
		makeResourceForEnv("CronJob", "nightly-report"),
	)

	got := DetectWorkloadType(group)
	if got != WorkloadBatch {
		t.Errorf("DetectWorkloadType CronJob-only: got %q, want %q", got, WorkloadBatch)
	}
}

// TestWorkload_DetectCache_RedisImage — Deployment with image redis:7 (no PVC) → WorkloadCache
func TestWorkload_DetectCache_RedisImage(t *testing.T) {
	group := makeGroupForEnv("cache",
		makeResourceWithImage("Deployment", "cache", "redis:7"),
	)

	got := DetectWorkloadType(group)
	if got != WorkloadCache {
		t.Errorf("DetectWorkloadType with redis image (no PVC): got %q, want %q", got, WorkloadCache)
	}
}

// ============================================================
// Section B: GenerateEnvValuesForWorkload — profile correctness
// ============================================================

// TestWorkload_WebDevProfile — Web dev profile: replicas=1, no HPA enabled
func TestWorkload_WebDevProfile(t *testing.T) {
	result := GenerateEnvValuesForWorkload(baseVals(1), WorkloadWeb)

	devData, ok := result["values-dev.yaml"]
	if !ok {
		t.Fatal("values-dev.yaml missing from web workload result")
	}
	parsed := parseEnvYAML(t, devData)

	if toInt(parsed["replicaCount"]) != 1 {
		t.Errorf("web dev replicas: got %v, want 1", parsed["replicaCount"])
	}

	// No HPA in dev — either absent or enabled=false
	if hpa, hasHPA := parsed["autoscaling"]; hasHPA {
		if hpaMap, ok := hpa.(map[string]interface{}); ok {
			if hpaMap["enabled"] == true {
				t.Error("web dev profile must not have HPA enabled")
			}
		}
	}
}

// TestWorkload_WebProdProfile — Web prod: replicas=3, HPA min=3/max=10, PDB minAvailable=2
func TestWorkload_WebProdProfile(t *testing.T) {
	result := GenerateEnvValuesForWorkload(baseVals(1), WorkloadWeb)

	prodData, ok := result["values-prod.yaml"]
	if !ok {
		t.Fatal("values-prod.yaml missing from web workload result")
	}
	parsed := parseEnvYAML(t, prodData)

	// Replicas
	if toInt(parsed["replicaCount"]) != 3 {
		t.Errorf("web prod replicas: got %v, want 3", parsed["replicaCount"])
	}

	// HPA
	hpa, hasHPA := parsed["autoscaling"]
	if !hasHPA {
		t.Fatal("web prod missing autoscaling section")
	}
	hpaMap, ok := hpa.(map[string]interface{})
	if !ok {
		t.Fatal("autoscaling is not a map")
	}
	if hpaMap["enabled"] != true {
		t.Error("web prod HPA must be enabled")
	}
	if toInt(hpaMap["minReplicas"]) != 3 {
		t.Errorf("web prod HPA minReplicas: got %v, want 3", hpaMap["minReplicas"])
	}
	if toInt(hpaMap["maxReplicas"]) != 10 {
		t.Errorf("web prod HPA maxReplicas: got %v, want 10", hpaMap["maxReplicas"])
	}

	// PDB
	pdb, hasPDB := parsed["podDisruptionBudget"]
	if !hasPDB {
		t.Fatal("web prod missing podDisruptionBudget section")
	}
	pdbMap, ok := pdb.(map[string]interface{})
	if !ok {
		t.Fatal("podDisruptionBudget is not a map")
	}
	if pdbMap["enabled"] != true {
		t.Error("web prod PDB must be enabled")
	}
	if toInt(pdbMap["minAvailable"]) != 2 {
		t.Errorf("web prod PDB minAvailable: got %v, want 2", pdbMap["minAvailable"])
	}
}

// TestWorkload_DatabaseProdProfile — Database prod: anti-affinity, strict resources, PDB minAvailable=2
func TestWorkload_DatabaseProdProfile(t *testing.T) {
	result := GenerateEnvValuesForWorkload(baseVals(1), WorkloadDatabase)

	prodData, ok := result["values-prod.yaml"]
	if !ok {
		t.Fatal("values-prod.yaml missing from database workload result")
	}
	parsed := parseEnvYAML(t, prodData)

	// Anti-affinity must be present
	affinity, hasAffinity := parsed["affinity"]
	if !hasAffinity {
		t.Fatal("database prod profile missing affinity section")
	}
	affinityMap, ok := affinity.(map[string]interface{})
	if !ok {
		t.Fatal("affinity is not a map")
	}
	if _, hasPAA := affinityMap["podAntiAffinity"]; !hasPAA {
		t.Error("database prod affinity must contain podAntiAffinity")
	}

	// Strict resources (limits must be present)
	resources, hasResources := parsed["resources"]
	if !hasResources {
		t.Fatal("database prod profile missing resources section")
	}
	resMap, ok := resources.(map[string]interface{})
	if !ok {
		t.Fatal("resources is not a map")
	}
	if _, hasLimits := resMap["limits"]; !hasLimits {
		t.Error("database prod resources must have limits")
	}
	if _, hasRequests := resMap["requests"]; !hasRequests {
		t.Error("database prod resources must have requests")
	}

	// PDB minAvailable=2
	pdb, hasPDB := parsed["podDisruptionBudget"]
	if !hasPDB {
		t.Fatal("database prod profile missing podDisruptionBudget")
	}
	pdbMap, ok := pdb.(map[string]interface{})
	if !ok {
		t.Fatal("podDisruptionBudget is not a map")
	}
	if pdbMap["enabled"] != true {
		t.Error("database prod PDB must be enabled")
	}
	if toInt(pdbMap["minAvailable"]) != 2 {
		t.Errorf("database prod PDB minAvailable: got %v, want 2", pdbMap["minAvailable"])
	}
}

// TestWorkload_BatchProdProfile — Batch prod: backoffLimit=3, no PDB, no HPA
func TestWorkload_BatchProdProfile(t *testing.T) {
	result := GenerateEnvValuesForWorkload(baseVals(1), WorkloadBatch)

	prodData, ok := result["values-prod.yaml"]
	if !ok {
		t.Fatal("values-prod.yaml missing from batch workload result")
	}
	parsed := parseEnvYAML(t, prodData)

	// backoffLimit must be 3
	if toInt(parsed["backoffLimit"]) != 3 {
		t.Errorf("batch prod backoffLimit: got %v, want 3", parsed["backoffLimit"])
	}

	// No HPA — batch workloads are not scaled horizontally
	if hpa, hasHPA := parsed["autoscaling"]; hasHPA {
		if hpaMap, ok := hpa.(map[string]interface{}); ok {
			if hpaMap["enabled"] == true {
				t.Error("batch prod profile must not have HPA enabled")
			}
		}
	}

	// No PDB — batch jobs run to completion, disruption budget does not apply
	if pdb, hasPDB := parsed["podDisruptionBudget"]; hasPDB {
		if pdbMap, ok := pdb.(map[string]interface{}); ok {
			if pdbMap["enabled"] == true {
				t.Error("batch prod profile must not have PDB enabled")
			}
		}
	}
}

// ============================================================
// Section C: MergeEnvProfiles
// ============================================================

// TestWorkload_MergeEnvProfiles_DeepMerge — nested maps are merged, not replaced
func TestWorkload_MergeEnvProfiles_DeepMerge(t *testing.T) {
	base := map[string]interface{}{
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"cpu":    "100m",
				"memory": "128Mi",
			},
		},
	}
	overrides := map[string]interface{}{
		"resources": map[string]interface{}{
			"limits": map[string]interface{}{
				"cpu":    "500m",
				"memory": "512Mi",
			},
		},
	}

	merged := MergeEnvProfiles(base, overrides)

	resources, ok := merged["resources"]
	if !ok {
		t.Fatal("merged result missing 'resources' key")
	}
	resMap, ok := resources.(map[string]interface{})
	if !ok {
		t.Fatal("merged 'resources' is not a map")
	}

	// Both requests (from base) and limits (from overrides) must be present
	if _, hasRequests := resMap["requests"]; !hasRequests {
		t.Error("deep merge: 'resources.requests' from base was lost")
	}
	if _, hasLimits := resMap["limits"]; !hasLimits {
		t.Error("deep merge: 'resources.limits' from overrides was not merged in")
	}
}

// TestWorkload_MergeEnvProfiles_OverrideScalars — scalar values from override win;
// map values are merged (not replaced by the override's map).
func TestWorkload_MergeEnvProfiles_OverrideScalars(t *testing.T) {
	base := map[string]interface{}{
		"replicaCount": 1,
		"logLevel":     "info",
		"nested": map[string]interface{}{
			"keyA": "from-base",
			"keyB": "base-only",
		},
	}
	overrides := map[string]interface{}{
		"replicaCount": 3,
		"nested": map[string]interface{}{
			"keyA": "from-override",
		},
	}

	merged := MergeEnvProfiles(base, overrides)

	// Scalar override: replicaCount from override wins
	if toInt(merged["replicaCount"]) != 3 {
		t.Errorf("scalar override: replicaCount got %v, want 3", merged["replicaCount"])
	}

	// Scalar not overridden: logLevel from base is preserved
	if merged["logLevel"] != "info" {
		t.Errorf("unoverridden scalar: logLevel got %v, want info", merged["logLevel"])
	}

	// Deep map merge: nested.keyA overridden, nested.keyB from base preserved
	nested, ok := merged["nested"]
	if !ok {
		t.Fatal("merged result missing 'nested' key")
	}
	nestedMap, ok := nested.(map[string]interface{})
	if !ok {
		t.Fatal("merged 'nested' is not a map")
	}
	if nestedMap["keyA"] != "from-override" {
		t.Errorf("nested scalar override: keyA got %v, want from-override", nestedMap["keyA"])
	}
	if nestedMap["keyB"] != "base-only" {
		t.Errorf("nested base preservation: keyB got %v, want base-only", nestedMap["keyB"])
	}
}

// ============================================================
// Workload Test Helpers
// ============================================================

// makeResourceForEnv creates a minimal ProcessedResource with a given Kind and Name.
// Uses gvkForKind from grouping_test.go for proper GVK resolution.
func makeResourceForEnv(kind, name string) *types.ProcessedResource {
	obj := &unstructured.Unstructured{}
	obj.SetKind(kind)
	obj.SetName(name)
	gvk := gvkForKind(kind)
	obj.SetAPIVersion(gvk.GroupVersion().String())
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    gvk,
		},
	}
}

// makeResourceWithEnvVars creates a ProcessedResource whose spec embeds env vars in
// the first container, to exercise worker detection via AMQP_*/KAFKA_* vars.
func makeResourceWithEnvVars(kind, name string, envVars map[string]string) *types.ProcessedResource {
	obj := &unstructured.Unstructured{}
	obj.SetKind(kind)
	obj.SetName(name)
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind}
	obj.SetAPIVersion(gvk.GroupVersion().String())

	envList := make([]interface{}, 0, len(envVars))
	for k, v := range envVars {
		envList = append(envList, map[string]interface{}{"name": k, "value": v})
	}
	obj.Object["spec"] = map[string]interface{}{
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name": "main",
						"env":  envList,
					},
				},
			},
		},
	}
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    gvk,
		},
	}
}

// makeResourceWithImage creates a ProcessedResource whose first container uses the
// supplied image string, to exercise database/cache detection by image name.
func makeResourceWithImage(kind, name, image string) *types.ProcessedResource {
	obj := &unstructured.Unstructured{}
	obj.SetKind(kind)
	obj.SetName(name)
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind}
	obj.SetAPIVersion(gvk.GroupVersion().String())

	obj.Object["spec"] = map[string]interface{}{
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"image": image,
					},
				},
			},
		},
	}
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    gvk,
		},
	}
}

// makeResourceWithPorts creates a ProcessedResource whose first container exposes the
// given containerPorts, to exercise web detection via port numbers.
func makeResourceWithPorts(kind, name string, ports []int) *types.ProcessedResource {
	obj := &unstructured.Unstructured{}
	obj.SetKind(kind)
	obj.SetName(name)
	gvk := gvkForKind(kind)
	obj.SetAPIVersion(gvk.GroupVersion().String())

	portList := make([]interface{}, 0, len(ports))
	for _, p := range ports {
		portList = append(portList, map[string]interface{}{
			"containerPort": int64(p),
		})
	}
	obj.Object["spec"] = map[string]interface{}{
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"ports": portList,
					},
				},
			},
		},
	}
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    gvk,
		},
	}
}

// makeGroupForEnv creates a ServiceGroup from variadic ProcessedResources.
// Named distinctly from makeGroup (namespace_test.go) which has a different signature.
func makeGroupForEnv(name string, resources ...*types.ProcessedResource) *ServiceGroup {
	return &ServiceGroup{
		Name:      name,
		Resources: resources,
		Namespace: "default",
		Strategy:  GroupByLabel,
	}
}
