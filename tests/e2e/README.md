# E2E Test Framework

End-to-end testing framework for Deckhouse Helm Generator using real Helm CLI.

## Overview

The E2E framework validates generated Helm charts by running them through the actual Helm toolchain:

- `helm lint` — validates chart structure and syntax
- `helm template` — renders templates with values
- `helm install --dry-run` — simulates installation (client-side or server-side)

## Architecture

### Components

| File | Purpose |
|------|---------|
| `framework.go` | `E2ETestHarness` — manages test lifecycle, chart generation, cleanup |
| `helm.go` | `HelmClient` — wraps Helm CLI commands with typed options |
| `kubernetes.go` | `KubernetesClient` — K8s dry-run validation (optional cluster) |
| `framework_test.go` | Meta-tests validating the framework itself |

### Directory Structure

```
tests/e2e/
├── framework.go          # Test harness
├── helm.go               # Helm CLI wrapper
├── kubernetes.go          # K8s client
├── framework_test.go      # Framework self-tests
├── fixtures/
│   └── charts/
│       ├── simple-app/    # Deployment + Service
│       ├── fullstack/     # Frontend + Backend
│       └── deckhouse/     # Deckhouse CRDs
└── README.md
```

## Usage

### Basic Test

```go
func TestMyChart(t *testing.T) {
    h := NewE2ETestHarness(t)

    h.WriteInput("deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
...
`)

    h.GenerateChart("my-chart")
    h.AssertLintSuccess()

    rendered := h.AssertTemplateSuccess("my-release")
    // ... assert on rendered output
}
```

### With Custom Values

```go
func TestCustomValues(t *testing.T) {
    h := NewE2ETestHarness(t)
    h.WriteInput("deployment.yaml", deploymentYAML)
    h.GenerateChart("my-chart")

    valuesFile := h.WriteCustomValues(`
services:
  myapp:
    deployment:
      replicas: 5
`)

    h.AssertLintSuccess(WithLintValues(valuesFile))
    h.AssertTemplateSuccess("release", WithTemplateValues(valuesFile))
}
```

### Helm Install Dry-Run

```go
func TestInstallDryRun(t *testing.T) {
    h := NewE2ETestHarness(t)
    h.WriteInput("deployment.yaml", deploymentYAML)
    h.GenerateChart("my-chart")

    result := h.Install("my-release")
    if !result.Success() {
        t.Fatalf("Install dry-run failed: %s", result.Stderr)
    }
}
```

## Requirements

- **Helm v3**: Must be in PATH or `~/.local/bin/`
- **Go 1.21+**: For running tests
- **kubectl** (optional): For server-side dry-run validation

## Running Tests

```bash
# Run E2E framework tests
go test ./tests/e2e/ -v

# Run with verbose helm output
go test ./tests/e2e/ -v -count=1
```

## Test Isolation

Each `E2ETestHarness` creates a unique temporary directory that is automatically cleaned up via `t.Cleanup`. Tests do not share filesystem state.
