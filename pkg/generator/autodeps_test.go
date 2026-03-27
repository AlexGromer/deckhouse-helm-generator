package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/helm"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// ============================================================
// Test Helper (unique to autodeps tests)
// ============================================================

// makeDeploymentWithPort creates a ProcessedResource with a container exposing the given port.
func makeDeploymentWithPort(name string, port int) *types.ProcessedResource {
	obj := &unstructured.Unstructured{}
	obj.SetKind("Deployment")
	obj.SetName(name)
	obj.SetAPIVersion("apps/v1")
	obj.Object["spec"] = map[string]interface{}{
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name": "main",
						"ports": []interface{}{
							map[string]interface{}{"containerPort": int64(port)},
						},
					},
				},
			},
		},
	}
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		},
	}
}

// ============================================================
// Section 1: DetectCommonDependencies — env-based detection
// ============================================================

// TestAutoDeps_PostgresFromEnv verifies that a Deployment with POSTGRES_HOST
// env var is detected as requiring the postgresql dependency.
func TestAutoDeps_PostgresFromEnv(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeDeploymentWithEnv("myapp", "default", map[string]string{
			"POSTGRES_HOST": "pg.default.svc",
		}),
	}

	deps := DetectCommonDependencies(resources)

	if len(deps) == 0 {
		t.Fatal("expected at least 1 dependency detected from POSTGRES_HOST env var")
	}

	found := false
	for _, d := range deps {
		if d.Name == "postgresql" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'postgresql' dependency from POSTGRES_HOST env var, got: %+v", deps)
	}
}

// TestAutoDeps_PostgresFromImage verifies that a Deployment running the
// "postgres" image is detected as requiring the postgresql dependency.
func TestAutoDeps_PostgresFromImage(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeDeploymentWithImage("db", "postgres:15"),
	}

	deps := DetectCommonDependencies(resources)

	if len(deps) == 0 {
		t.Fatal("expected at least 1 dependency detected from postgres:15 image")
	}

	found := false
	for _, d := range deps {
		if d.Name == "postgresql" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'postgresql' dependency from postgres:15 image, got: %+v", deps)
	}
}

// TestAutoDeps_PostgresFromPort verifies that a Deployment exposing port 5432
// is detected as requiring the postgresql dependency.
func TestAutoDeps_PostgresFromPort(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeDeploymentWithPort("db", 5432),
	}

	deps := DetectCommonDependencies(resources)

	if len(deps) == 0 {
		t.Fatal("expected at least 1 dependency detected from port 5432")
	}

	found := false
	for _, d := range deps {
		if d.Name == "postgresql" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'postgresql' dependency from port 5432, got: %+v", deps)
	}
}

// TestAutoDeps_RedisFromEnv verifies that a Deployment with REDIS_URL env var
// is detected as requiring the redis dependency.
func TestAutoDeps_RedisFromEnv(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeDeploymentWithEnv("myapp", "default", map[string]string{
			"REDIS_URL": "redis://cache.default.svc:6379",
		}),
	}

	deps := DetectCommonDependencies(resources)

	if len(deps) == 0 {
		t.Fatal("expected at least 1 dependency detected from REDIS_URL env var")
	}

	found := false
	for _, d := range deps {
		if d.Name == "redis" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'redis' dependency from REDIS_URL env var, got: %+v", deps)
	}
}

// ============================================================
// Section 2: DetectCommonDependencies — multi-dependency detection
// ============================================================

// TestAutoDeps_RabbitMQAndPostgres verifies that a single Deployment carrying
// both AMQP_URL and POSTGRES_HOST signals yields exactly 2 dependencies.
func TestAutoDeps_RabbitMQAndPostgres(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeDeploymentWithEnv("worker", "default", map[string]string{
			"AMQP_URL":      "amqp://rabbitmq.default.svc:5672",
			"POSTGRES_HOST": "pg.default.svc",
		}),
	}

	deps := DetectCommonDependencies(resources)

	hasRabbit := false
	hasPostgres := false
	for _, d := range deps {
		switch d.Name {
		case "rabbitmq":
			hasRabbit = true
		case "postgresql":
			hasPostgres = true
		}
	}

	if !hasRabbit {
		t.Error("expected 'rabbitmq' dependency from AMQP_URL env var")
	}
	if !hasPostgres {
		t.Error("expected 'postgresql' dependency from POSTGRES_HOST env var")
	}
	if len(deps) < 2 {
		t.Errorf("expected at least 2 dependencies, got %d: %+v", len(deps), deps)
	}
}

// ============================================================
// Section 3: DetectCommonDependencies — negative / edge cases
// ============================================================

// TestAutoDeps_NoSignals_EmptyList verifies that a Deployment with no matching
// env vars, images, or ports yields an empty dependency list.
func TestAutoDeps_NoSignals_EmptyList(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeDeploymentWithEnv("myapp", "default", map[string]string{
			"APP_ENV":  "production",
			"LOG_LEVEL": "info",
		}),
	}

	deps := DetectCommonDependencies(resources)

	if len(deps) != 0 {
		t.Errorf("expected 0 dependencies for a Deployment with no known signals, got %d: %+v", len(deps), deps)
	}
}

// TestAutoDeps_DuplicateSignals_SingleDep verifies that when both an env var
// (POSTGRES_HOST) and an image (postgres) signal the same dependency, only one
// postgresql entry is emitted — no duplicates.
func TestAutoDeps_DuplicateSignals_SingleDep(t *testing.T) {
	// Build a resource that carries both an image "postgres:15" and the env var
	// POSTGRES_HOST so that two independent signals point to the same dep.
	obj := &unstructured.Unstructured{}
	obj.SetKind("Deployment")
	obj.SetName("dual-signal-app")
	obj.SetAPIVersion("apps/v1")
	obj.Object["spec"] = map[string]interface{}{
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"image": "postgres:15",
						"env": []interface{}{
							map[string]interface{}{
								"name":  "POSTGRES_HOST",
								"value": "pg.default.svc",
							},
						},
					},
				},
			},
		},
	}
	resource := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		},
	}

	deps := DetectCommonDependencies([]*types.ProcessedResource{resource})

	count := 0
	for _, d := range deps {
		if d.Name == "postgresql" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 'postgresql' dependency (deduplication), got %d", count)
	}
}

// ============================================================
// Section 4: FilterExistingDependencies
// ============================================================

// TestAutoDeps_FilterExisting_RemovesDuplicates verifies that
// FilterExistingDependencies removes any dependency already present in the
// existing list and keeps the rest.
func TestAutoDeps_FilterExisting_RemovesDuplicates(t *testing.T) {
	detected := []helm.Dependency{
		{Name: "postgresql", Version: "12.x.x", Repository: "https://charts.bitnami.com/bitnami"},
		{Name: "redis", Version: "18.x.x", Repository: "https://charts.bitnami.com/bitnami"},
	}
	existing := []helm.Dependency{
		{Name: "postgresql", Version: "12.x.x", Repository: "https://charts.bitnami.com/bitnami"},
	}

	filtered := FilterExistingDependencies(detected, existing)

	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered dependency (redis only), got %d: %+v", len(filtered), filtered)
	}
	if filtered[0].Name != "redis" {
		t.Errorf("expected filtered[0].Name == 'redis', got %q", filtered[0].Name)
	}
}

// ============================================================
// Section 5: DetectCommonDependencies — all seven known deps
// ============================================================

// TestAutoDeps_AllSevenDetectable verifies that all seven known dependency
// signals are individually detectable: postgresql, mysql, redis, mongodb,
// rabbitmq, elasticsearch, kafka.
func TestAutoDeps_AllSevenDetectable(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeDeploymentWithEnv("pg-app", "default", map[string]string{"POSTGRES_HOST": "pg"}),
		makeDeploymentWithEnv("mysql-app", "default", map[string]string{"MYSQL_HOST": "mysql"}),
		makeDeploymentWithEnv("redis-app", "default", map[string]string{"REDIS_HOST": "redis"}),
		makeDeploymentWithEnv("mongo-app", "default", map[string]string{"MONGO_URI": "mongodb://"}),
		makeDeploymentWithEnv("rabbit-app", "default", map[string]string{"RABBITMQ_HOST": "rabbit"}),
		makeDeploymentWithEnv("es-app", "default", map[string]string{"ELASTIC_HOST": "elastic"}),
		makeDeploymentWithEnv("kafka-app", "default", map[string]string{"KAFKA_BROKER": "kafka"}),
	}

	deps := DetectCommonDependencies(resources)

	expected := []string{"postgresql", "mysql", "redis", "mongodb", "rabbitmq", "elasticsearch", "kafka"}
	found := make(map[string]bool, len(expected))
	for _, d := range deps {
		found[d.Name] = true
	}

	for _, name := range expected {
		if !found[name] {
			t.Errorf("expected dependency %q to be detected but it was not; all detected: %+v", name, deps)
		}
	}

	if len(deps) < len(expected) {
		t.Errorf("expected at least %d dependencies, got %d: %+v", len(expected), len(deps), deps)
	}
}

// ============================================================
// Section 6: Dependency metadata — repository and condition
// ============================================================

// TestAutoDeps_BitnamiRepo verifies that all detected dependencies reference
// the official Bitnami chart repository.
func TestAutoDeps_BitnamiRepo(t *testing.T) {
	const bitnamiRepo = "https://charts.bitnami.com/bitnami"

	resources := []*types.ProcessedResource{
		makeDeploymentWithEnv("app", "default", map[string]string{
			"REDIS_HOST":    "redis",
			"POSTGRES_HOST": "pg",
		}),
	}

	deps := DetectCommonDependencies(resources)

	if len(deps) == 0 {
		t.Fatal("expected dependencies to be detected")
	}

	for _, d := range deps {
		if d.Repository != bitnamiRepo {
			t.Errorf("dependency %q: expected repository %q, got %q", d.Name, bitnamiRepo, d.Repository)
		}
	}
}

// TestAutoDeps_ConditionFormat verifies that each detected dependency carries a
// condition field in the form "<name>.enabled" (e.g. "postgresql.enabled").
func TestAutoDeps_ConditionFormat(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeDeploymentWithEnv("app", "default", map[string]string{
			"REDIS_HOST":    "redis",
			"POSTGRES_HOST": "pg",
		}),
	}

	deps := DetectCommonDependencies(resources)

	if len(deps) == 0 {
		t.Fatal("expected dependencies to be detected")
	}

	for _, d := range deps {
		expectedCondition := d.Name + ".enabled"
		if d.Condition != expectedCondition {
			t.Errorf("dependency %q: expected condition %q, got %q", d.Name, expectedCondition, d.Condition)
		}
	}
}

// ============================================================
// Section 7: InjectDependencies — chart mutation
// ============================================================

// TestAutoDeps_InjectDeps_UpdatesChartYAML verifies that InjectDependencies
// appends dependency entries into the chart's ChartYAML field.
func TestAutoDeps_InjectDeps_UpdatesChartYAML(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	deps := []helm.Dependency{
		{
			Name:       "postgresql",
			Version:    "12.x.x",
			Repository: "https://charts.bitnami.com/bitnami",
			Condition:  "postgresql.enabled",
		},
	}

	result := InjectDependencies(chart, deps)

	if result == nil {
		t.Fatal("InjectDependencies returned nil for valid chart")
	}

	if !strings.Contains(result.ChartYAML, "dependencies") {
		t.Error("expected 'dependencies' section in ChartYAML after injection")
	}
	if !strings.Contains(result.ChartYAML, "postgresql") {
		t.Error("expected 'postgresql' entry in ChartYAML after injection")
	}
	if !strings.Contains(result.ChartYAML, "https://charts.bitnami.com/bitnami") {
		t.Error("expected Bitnami repository URL in ChartYAML after injection")
	}
}

// TestAutoDeps_InjectDeps_AddsConditionValues verifies that InjectDependencies
// adds a "<dep>.enabled: false" entry to the chart's ValuesYAML so the
// dependency is opt-in by default.
func TestAutoDeps_InjectDeps_AddsConditionValues(t *testing.T) {
	chart := makeChart("myapp", map[string]string{
		"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: myapp\n",
	})

	deps := []helm.Dependency{
		{
			Name:      "redis",
			Version:   "18.x.x",
			Condition: "redis.enabled",
		},
	}

	result := InjectDependencies(chart, deps)

	if result == nil {
		t.Fatal("InjectDependencies returned nil for valid chart")
	}

	if !strings.Contains(result.ValuesYAML, "redis") {
		t.Error("expected 'redis' key in ValuesYAML after injection")
	}
	if !strings.Contains(result.ValuesYAML, "enabled") {
		t.Error("expected 'enabled' key under the dependency entry in ValuesYAML after injection")
	}
	if !strings.Contains(result.ValuesYAML, "false") {
		t.Error("expected 'enabled: false' (opt-in default) in ValuesYAML after injection")
	}
}

// ============================================================
// Section 8: Nil / empty input safety
// ============================================================

// TestAutoDeps_NilResources_EmptyList verifies that DetectCommonDependencies
// handles a nil resource slice gracefully and returns an empty (non-nil) list
// without panicking.
func TestAutoDeps_NilResources_EmptyList(t *testing.T) {
	// Must not panic.
	deps := DetectCommonDependencies(nil)

	if deps == nil {
		t.Error("expected non-nil (empty) slice for nil input, got nil")
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 dependencies for nil input, got %d: %+v", len(deps), deps)
	}
}

// ============================================================
// Section 8b: DetectCommonDependencies — postgresql image variants
// ============================================================

// TestAutoDeps_PostgresFromBitnamiPostgresqlImage verifies that a Deployment
// running the "bitnami/postgresql:15" image is detected as requiring the
// postgresql dependency (the image name "postgresql" must match alongside "postgres").
func TestAutoDeps_PostgresFromBitnamiPostgresqlImage(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeDeploymentWithImage("db", "bitnami/postgresql:15"),
	}

	deps := DetectCommonDependencies(resources)

	if len(deps) == 0 {
		t.Fatal("expected at least 1 dependency detected from bitnami/postgresql:15 image")
	}

	found := false
	for _, d := range deps {
		if d.Name == "postgresql" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'postgresql' dependency from bitnami/postgresql:15 image, got: %+v", deps)
	}
}

// TestAutoDeps_PostgresFromPlainPostgresqlImage verifies that a Deployment
// running the plain "postgresql:14" image is detected as requiring the
// postgresql dependency.
func TestAutoDeps_PostgresFromPlainPostgresqlImage(t *testing.T) {
	resources := []*types.ProcessedResource{
		makeDeploymentWithImage("db", "postgresql:14"),
	}

	deps := DetectCommonDependencies(resources)

	if len(deps) == 0 {
		t.Fatal("expected at least 1 dependency detected from postgresql:14 image")
	}

	found := false
	for _, d := range deps {
		if d.Name == "postgresql" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'postgresql' dependency from postgresql:14 image, got: %+v", deps)
	}
}

// CR-2: InjectDependencies must deep-copy Templates map

func TestInjectDeps_TemplatesMapIsDeepCopy(t *testing.T) {
	original := &types.GeneratedChart{
		Name:      "test",
		ChartYAML: "apiVersion: v2\nname: test\nversion: 0.1.0\n",
		Templates: map[string]string{
			"templates/deployment.yaml": "kind: Deployment",
		},
		ExternalFiles: []types.ExternalFileInfo{
			{Path: "files/config.json", Content: `{"key":"value"}`},
		},
	}

	deps := []helm.Dependency{
		{Name: "redis", Version: "18.x.x", Repository: "https://charts.bitnami.com/bitnami"},
	}

	result := InjectDependencies(original, deps)

	// Mutate result's Templates — original must be unaffected
	result.Templates["templates/new.yaml"] = "new content"
	if _, exists := original.Templates["templates/new.yaml"]; exists {
		t.Error("InjectDependencies shares Templates map reference — mutation of result affected original")
	}

	// Mutate result's ExternalFiles — original must be unaffected
	result.ExternalFiles = append(result.ExternalFiles, types.ExternalFileInfo{Path: "files/new.json", Content: "new"})
	if len(original.ExternalFiles) != 1 {
		t.Errorf("InjectDependencies shares ExternalFiles slice — append to result affected original: len=%d", len(original.ExternalFiles))
	}
}

// ============================================================
// Section 9: HC-6 — InjectDependencies duplicate/broken YAML
// ============================================================

// TestInjectDeps_PreExistingDependencies_NoDuplicate verifies that when
// ChartYAML already contains a "dependencies:" key, InjectDependencies does
// NOT append a second dependencies block (no duplication).
func TestInjectDeps_PreExistingDependencies_NoDuplicate(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "existing-deps",
		ChartYAML: "apiVersion: v2\nname: existing-deps\nversion: 0.1.0\n" +
			"dependencies:\n" +
			"  - name: redis\n" +
			"    version: 18.x.x\n" +
			"    repository: https://charts.bitnami.com/bitnami\n",
		ValuesYAML: "replicaCount: 1\n",
		Templates:  map[string]string{},
	}

	deps := []helm.Dependency{
		{Name: "postgresql", Version: "12.x.x", Repository: "https://charts.bitnami.com/bitnami", Condition: "postgresql.enabled"},
	}

	result := InjectDependencies(chart, deps)

	count := strings.Count(result.ChartYAML, "dependencies:")
	if count != 1 {
		t.Errorf("expected exactly 1 'dependencies:' key in ChartYAML, got %d.\nChartYAML:\n%s", count, result.ChartYAML)
	}
}

// TestInjectDeps_DoubleCall_NoDuplicate verifies that calling
// InjectDependencies twice with the same deps does not produce duplicate
// dependencies blocks.
func TestInjectDeps_DoubleCall_NoDuplicate(t *testing.T) {
	chart := makeChart("doubletest", map[string]string{
		"templates/deployment.yaml": "kind: Deployment",
	})

	deps := []helm.Dependency{
		{Name: "redis", Version: "18.x.x", Repository: "https://charts.bitnami.com/bitnami", Condition: "redis.enabled"},
	}

	first := InjectDependencies(chart, deps)
	second := InjectDependencies(first, deps)

	count := strings.Count(second.ChartYAML, "dependencies:")
	if count != 1 {
		t.Errorf("expected exactly 1 'dependencies:' key after double injection, got %d.\nChartYAML:\n%s", count, second.ChartYAML)
	}
}

// TestInjectDeps_ValidYAMLOutput verifies that the ChartYAML produced by
// InjectDependencies is valid YAML that can be unmarshalled without error.
func TestInjectDeps_ValidYAMLOutput(t *testing.T) {
	chart := makeChart("yamltest", map[string]string{
		"templates/deployment.yaml": "kind: Deployment",
	})

	deps := []helm.Dependency{
		{Name: "postgresql", Version: "12.x.x", Repository: "https://charts.bitnami.com/bitnami", Condition: "postgresql.enabled"},
		{Name: "redis", Version: "18.x.x", Repository: "https://charts.bitnami.com/bitnami", Condition: "redis.enabled"},
	}

	result := InjectDependencies(chart, deps)

	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(result.ChartYAML), &parsed); err != nil {
		t.Errorf("InjectDependencies produced invalid YAML: %v\nChartYAML:\n%s", err, result.ChartYAML)
	}

	// Verify the dependencies key exists and has the right count.
	rawDeps, ok := parsed["dependencies"]
	if !ok {
		t.Fatal("parsed YAML is missing 'dependencies' key")
	}
	depsList, ok := rawDeps.([]interface{})
	if !ok {
		t.Fatalf("expected dependencies to be a list, got %T", rawDeps)
	}
	if len(depsList) != 2 {
		t.Errorf("expected 2 dependencies in parsed YAML, got %d", len(depsList))
	}
}
