# Integration Test Framework

End-to-end integration tests for the Deckhouse Helm Generator pipeline.

## Architecture

The framework validates the full pipeline: **Extract → Process → Analyze → Generate**.

### Key Components

- `framework.go` — Test harness, pipeline executor, validation helpers
- `framework_test.go` — Meta-tests validating the framework itself
- `fixtures/` — Pre-built YAML input sets for pipeline testing

### Types

| Type | Purpose |
|------|---------|
| `TestHarness` | Manages temp directories, input files, cleanup |
| `PipelineOptions` | Configures chart name, version, namespace |
| `ChartOutput` | Holds generated charts, graph, resources, output dir |

### Functions

| Function | Purpose |
|----------|---------|
| `ExecutePipeline(inputDir, opts)` | Runs full 4-stage pipeline |
| `ValidateChartStructure(t, chartDir)` | Checks Chart.yaml, values.yaml, templates/ |
| `ValidateValues(t, content)` | Validates YAML syntax of values |
| `ValidateTemplates(t, chartDir)` | Checks all template files are non-empty |
| `CompareGeneratedChart(actual, expected)` | Semantic diff between two charts |

## Usage

### Using TestHarness (dynamic input)

```go
func TestMyFeature(t *testing.T) {
    h := NewTestHarness(t)
    h.Setup()
    defer h.Cleanup()

    h.WriteInputFile("deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 1
  template:
    spec:
      containers:
        - name: app
          image: nginx:latest
`)

    output, err := ExecutePipeline(h.InputDir, PipelineOptions{
        ChartName: "my-chart",
    })
    if err != nil {
        t.Fatalf("pipeline failed: %v", err)
    }
    defer os.RemoveAll(output.OutputDir)

    // Assertions
    if len(output.Charts) == 0 {
        t.Fatal("no charts generated")
    }
    ValidateValues(t, output.Charts[0].ValuesYAML)
}
```

### Using fixtures (pre-built input)

```go
func TestSimpleApp(t *testing.T) {
    output, err := ExecutePipeline("fixtures/simple-app", PipelineOptions{
        ChartName:    "simple-app",
        ChartVersion: "1.0.0",
    })
    if err != nil {
        t.Fatalf("pipeline failed: %v", err)
    }
    defer os.RemoveAll(output.OutputDir)

    chartDir := filepath.Join(output.OutputDir, "simple-app")
    ValidateChartStructure(t, chartDir)
    ValidateTemplates(t, chartDir)
}
```

### Comparing charts

```go
diffs := CompareGeneratedChart(actualChart, expectedChart)
if len(diffs) > 0 {
    for _, d := range diffs {
        t.Error(d)
    }
}
```

## Fixtures

| Fixture | Resources | Purpose |
|---------|-----------|---------|
| `simple-app/` | Deployment, Service, ConfigMap | Basic single-service app |
| `full-stack/` | Frontend, Backend, Database (StatefulSet), Ingress, RBAC | Multi-service with relationships |

## Running Tests

```bash
# All integration tests
go test ./tests/integration/ -v

# Specific test
go test ./tests/integration/ -run TestExecutePipeline_SimpleApp -v
```
