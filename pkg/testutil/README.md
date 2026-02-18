# Test Utilities (`pkg/testutil`)

Reusable test infrastructure for consistent testing across all Deckhouse Helm Generator processors and components.

## Overview

This package provides:
- **Test helpers**: YAML fixture loading, assertions, comparisons
- **Resource factories**: Fluent builders for K8s resources (Deployment, Service, etc.)
- **Mock processors**: Configurable mocks for testing processor chains
- **Coverage helpers**: Coverage assertions and reporting

## Usage

### 1. Loading YAML Fixtures

```go
func TestMyProcessor(t *testing.T) {
    // Load fixture from pkg/testutil/fixtures/
    deployment := testutil.LoadYAMLFixture(t, "deployment.yaml")

    // Or load raw bytes
    yamlBytes := testutil.LoadYAMLFixtureBytes(t, "service.yaml")
}
```

Available fixtures:
- `deployment.yaml` — Standard Deployment with nginx
- `statefulset.yaml` — PostgreSQL StatefulSet with PVC
- `service.yaml` — ClusterIP Service
- `configmap.yaml` — ConfigMap with multiple data keys
- `secret.yaml` — Opaque Secret with credentials

### 2. Resource Factories

Create test resources using fluent builder pattern:

```go
// Simple Deployment
deployment := testutil.NewDeployment("myapp", "default")

// Deployment with options
deployment := testutil.NewDeployment("myapp", "default",
    testutil.WithReplicas(3),
    testutil.WithImage("nginx:1.21"),
    testutil.WithLabels(map[string]string{
        "tier": "frontend",
    }),
)

// Service
service := testutil.NewService("myapp", "default",
    testutil.WithServiceType(corev1.ServiceTypeLoadBalancer),
    testutil.WithServicePort("https", 443, 8443),
)

// ConfigMap
cm := testutil.NewConfigMap("myapp-config", "default", map[string]string{
    "key1": "value1",
    "key2": "value2",
})

// Secret
secret := testutil.NewSecret("myapp-secret", "default", map[string][]byte{
    "password": []byte("secret123"),
})
```

### 3. Assertions

```go
func TestExtraction(t *testing.T) {
    // Assert equality
    testutil.AssertEqual(t, expected, actual, "Should match")

    // Assert string contains
    testutil.AssertContains(t, haystack, needle, "Should contain substring")

    // Assert no error
    testutil.AssertNoError(t, err)

    // Assert error contains message
    testutil.AssertErrorContains(t, err, "expected message")

    // Compare Helm values (ignores comments, whitespace)
    testutil.CompareHelmValues(t, expectedYAML, actualYAML)
}
```

### 4. Field Extraction

```go
obj := testutil.LoadYAMLFixture(t, "deployment.yaml")

// Extract nested fields
replicas := testutil.ExtractField(t, obj, "spec", "replicas")
image := testutil.ExtractField(t, obj, "spec", "template", "spec", "containers", 0, "image")

// Create unstructured objects
gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
obj := testutil.CreateUnstructured(gvk, "myapp", "default", spec)
```

### 5. Mock Processors

Test processor chains and registries:

```go
// Create mock with specific behavior
mock := testutil.NewMockProcessor("test-processor",
    []schema.GroupVersionKind{
        {Group: "apps", Version: "v1", Kind: "Deployment"},
    },
    100, // priority
)

// Configure mock behavior
mock.ProcessFunc = func(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
    return &processor.Result{
        Processed: true,
        ServiceName: "test-service",
    }, nil
}

// Use in tests
result, err := mock.Process(ctx, deployment)
testutil.AssertEqual(t, 1, mock.CallCount, "Should be called once")
```

### 6. Coverage Assertions

```go
func TestCoverage(t *testing.T) {
    // Assert minimum coverage
    testutil.AssertCoverage(t, "./pkg/processor/deployment", 80.0)

    // Require coverage (fails immediately if not met)
    testutil.RequireMinCoverage(t, "./pkg/processor/deployment", 80.0)

    // Coverage range (catch over-mocked tests)
    testutil.AssertCoverageRange(t, "./pkg/processor/deployment", 80.0, 95.0)

    // Get coverage programmatically
    coverage := testutil.GetPackageCoverage("./pkg/processor/deployment")
    if coverage < 80.0 {
        t.Errorf("Coverage %.1f%% below target", coverage)
    }
}
```

### 7. Coverage Reports

```go
func TestProjectCoverage(t *testing.T) {
    packages := map[string]float64{
        "./pkg/processor/deployment": 80.0,
        "./pkg/processor/service":    80.0,
        "./pkg/generator":            75.0,
    }

    report := testutil.GenerateCoverageReport(packages)
    t.Log(report.String())

    if !report.AllPass() {
        t.Error("Some packages below coverage threshold")
    }
}
```

### 8. Temporary Files

```go
func TestWithTempFiles(t *testing.T) {
    // Create temp directory (auto-cleanup via t.Cleanup)
    tmpDir := testutil.CreateTempDir(t, "dhg-test-*")

    // Write files
    configPath := testutil.WriteFile(t, tmpDir, "config.yaml", "key: value")

    // Use files...
    // (cleaned up automatically after test)
}
```

## Best Practices

### TDD Workflow

1. **Write tests FIRST** using expected behavior
2. Run tests → expect failures
3. **Fix code** (not tests!) → tests pass
4. Verify coverage ≥ 80%

### Fixture Usage

- Use fixtures for **complex resources** (multi-container, affinity, tolerations)
- Use factories for **simple resources** (single container, basic config)
- Keep fixtures **realistic** (avoid artificial test-only structures)

### Mock Processors

- Mock **external dependencies** (processors you don't own)
- Test **real implementations** for processors you're testing
- Track **call counts** to verify processor chain behavior

### Coverage

- Target: **≥80% statement coverage**
- Focus on **edge cases** (nil values, empty arrays, missing fields)
- Don't chase **100% coverage** by testing trivial code

## Anti-Patterns

❌ **DON'T change tests to match broken code** (violates TDD)
```go
// WRONG: Test expected int64, code returns float64 → changed test
AssertEqual(t, float64(3), replicas) // ❌

// RIGHT: Fix the code to return int64
AssertEqual(t, int64(3), replicas)   // ✅
```

❌ **DON'T use fixtures for everything** (factories are more flexible)
```go
// WRONG: Create fixture for every slight variation
LoadYAMLFixture(t, "deployment-3-replicas.yaml")
LoadYAMLFixture(t, "deployment-5-replicas.yaml")

// RIGHT: Use factory with options
NewDeployment("app", "default", WithReplicas(3))
NewDeployment("app", "default", WithReplicas(5))
```

❌ **DON'T skip coverage checks** ("tests pass = good enough")
```go
// WRONG: No coverage verification
func TestProcessor(t *testing.T) {
    // tests here, but only 40% coverage
}

// RIGHT: Assert coverage requirement
func TestProcessorCoverage(t *testing.T) {
    AssertCoverage(t, "./pkg/processor/myprocessor", 80.0)
}
```

## Examples

See `test_utils_test.go` for complete examples of all helper functions.

## Architecture

```
pkg/testutil/
├── test_utils.go       # Core helpers (LoadFixture, Assert*, Extract*)
├── factory.go          # Resource factories (NewDeployment, NewService, etc.)
├── mock_processor.go   # Mock processor implementation
├── coverage.go         # Coverage assertions and reporting
├── fixtures/           # YAML test fixtures
│   ├── deployment.yaml
│   ├── statefulset.yaml
│   ├── service.yaml
│   ├── configmap.yaml
│   └── secret.yaml
└── README.md          # This file
```

## Testing the Test Utilities

The test utilities are tested in `test_utils_test.go` (meta-testing):
- 13 test cases covering all helper functions
- 71.6% coverage (remaining uncovered: error paths, edge cases)
- All fixtures loadable and parseable

Run tests:
```bash
go test -v -race -cover ./pkg/testutil/...
```
