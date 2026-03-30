# Developer Guide — DHG

> **Type:** How-To
> **Audience:** intermediate Go developers contributing to DHG
> **Last updated:** 2026-03-30
> **Related:** [ARCHITECTURE.md](../ARCHITECTURE.md), [ADR.md](ADR.md)

## Overview

This guide explains how to set up a development environment and how to extend DHG by adding new resource processors, relationship detectors, and chart generators. All three extension points follow a plugin-registry pattern (ADR-003) — you create a struct that implements an interface and register it; the pipeline picks it up automatically.

---

## 1. Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.26+ | Build and test |
| Make | any | Build targets (`make build`, `make test`) |
| golangci-lint | v1.62+ | Lint (`make lint`) |
| Helm | 3.x | Integration and e2e tests |
| git | any | Source control |

Install Go 1.26:

```bash
# Linux AMD64
curl -LO https://go.dev/dl/go1.26.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.26.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

Install golangci-lint:

```bash
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
  | sh -s -- -b $(go env GOPATH)/bin
```

Clone and build:

```bash
git clone https://github.com/AlexGromer/deckhouse-helm-generator.git
cd deckhouse-helm-generator
make build
# Binary: ./bin/dhg
```

---

## 2. Project Structure

```
deckhouse-helm-generator/
├── cmd/dhg/
│   ├── main.go              # CLI (Cobra root + all subcommands), pipeline orchestration
│   └── main_test.go         # CLI integration tests
├── pkg/
│   ├── analyzer/            # Pattern detection, relationship graph, DOT generation
│   │   ├── analyzer.go      # Core Analyzer, Analyze() entry point
│   │   ├── graph.go         # GenerateDOTGraph(), circular dependency detection
│   │   ├── detector/        # Relationship detectors (label, reference, annotation, volume, deckhouse)
│   │   └── pattern/         # Pattern checkers (11), pattern detectors (6), Recommender, Formatter
│   ├── extractor/           # Resource extraction from YAML files, cluster, gitops
│   │   ├── extractor.go     # Extractor interface + file implementation
│   │   ├── cluster.go       # Cluster extractor (client-go dynamic client)
│   │   ├── gitops.go        # GitOps extractor (go-git)
│   │   └── merger.go        # Multi-source deduplication and conflict resolution
│   ├── generator/           # Helm chart generation (70+ generator files)
│   │   ├── generator.go     # Generator interface, DefaultRegistry(), Options
│   │   ├── universal.go     # Universal (single chart) mode
│   │   ├── separate.go      # Separate (chart per service) mode
│   │   ├── library.go       # Library chart mode
│   │   ├── umbrella.go      # Umbrella chart mode
│   │   └── ...              # 60+ phase-specific generators
│   ├── processor/           # Processor interface and registry
│   │   ├── processor.go     # Processor interface, Context, Result, BaseProcessor
│   │   ├── registry.go      # Registry — GVK-based routing
│   │   └── k8s/             # 50 per-resource-type processors
│   ├── helm/                # Chart.yaml and values.yaml data models
│   └── types/               # Shared types (ExtractedResource, ProcessedResource, GeneratedChart, etc.)
├── tests/
│   ├── integration/         # Full pipeline tests with real YAML fixtures
│   └── e2e/                 # End-to-end tests (generate + helm lint)
├── Makefile
├── .goreleaser.yml
└── .golangci.yml
```

### Pipeline stages (from `cmd/dhg/main.go`)

```
[1] Extract   → extractor.Extract()     → []ExtractedResource
[2] Process   → processor.Process()     → []ProcessedResource
[3] Analyze   → analyzer.Analyze()      → ResourceGraph
[4] Generate  → generator.Generate()    → []GeneratedChart
[4b..4j]      → Phase 2 post-processors (copy-on-write)
[4k..4t]      → Phase 2.5 security post-processors
[5] Write     → generator.WriteChart()  → filesystem
```

---

## 3. Adding a New K8s Resource Processor

A processor converts one `*unstructured.Unstructured` object into a Helm template and a `values.yaml` fragment.

### 3.1 Create the processor file

Create `pkg/processor/k8s/mykind.go`:

```go
package k8s

import (
    "errors"
    "fmt"

    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime/schema"

    "github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// MyKindProcessor processes MyKind resources.
type MyKindProcessor struct {
    processor.BaseProcessor
}

// NewMyKindProcessor creates a new MyKind processor.
func NewMyKindProcessor() *MyKindProcessor {
    return &MyKindProcessor{
        BaseProcessor: processor.NewBaseProcessor(
            "mykind",
            100, // priority — lower number = higher priority
            schema.GroupVersionKind{
                Group:   "example.io",
                Version: "v1",
                Kind:    "MyKind",
            },
        ),
    }
}

// Process converts a MyKind resource into a Helm template and values.
func (p *MyKindProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
    if obj == nil {
        return nil, errors.New("mykind object is nil")
    }

    serviceName := processor.SanitizeServiceName(processor.ServiceNameFromResource(obj))
    if serviceName == "" {
        serviceName = obj.GetName()
    }

    // Extract values you want to expose in values.yaml
    values := map[string]interface{}{
        "enabled": true,
    }

    // Example: read a spec field
    if field, found, _ := unstructured.NestedString(obj.Object, "spec", "myField"); found {
        values["myField"] = field
    }

    // Build the Helm template
    template := fmt.Sprintf(`apiVersion: example.io/v1
kind: MyKind
metadata:
  name: {{ include "%s.fullname" . }}-mykind
  labels:
    {{- include "%s.labels" . | nindent 4 }}
spec:
  myField: {{ .Values.%s.mykind.myField | quote }}
`, ctx.ChartName, ctx.ChartName, serviceName)

    return &processor.Result{
        Processed:       true,
        ServiceName:     serviceName,
        TemplatePath:    fmt.Sprintf("templates/%s-mykind.yaml", serviceName),
        TemplateContent: template,
        ValuesPath:      fmt.Sprintf("services.%s.mykind", serviceName),
        Values:          values,
    }, nil
}
```

### 3.2 Register the processor

Open `pkg/processor/k8s/registry.go` and add one line to `RegisterAll()`:

```go
func RegisterAll(r *processor.Registry) {
    // ... existing registrations ...
    r.Register(NewMyKindProcessor())   // add this line
}
```

### 3.3 Write tests

Create `pkg/processor/k8s/mykind_test.go`. Use the same pattern as other `_test.go` files in the package — construct an `*unstructured.Unstructured`, call `Process()`, and assert on `TemplatePath`, `Values`, and `TemplateContent`:

```go
package k8s

import (
    "testing"

    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

    "github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
    "github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func TestMyKindProcessor_Process(t *testing.T) {
    p := NewMyKindProcessor()

    obj := &unstructured.Unstructured{
        Object: map[string]interface{}{
            "apiVersion": "example.io/v1",
            "kind":       "MyKind",
            "metadata": map[string]interface{}{
                "name":      "my-resource",
                "namespace": "default",
            },
            "spec": map[string]interface{}{
                "myField": "hello",
            },
        },
    }

    ctx := processor.Context{
        ChartName:  "testchart",
        OutputMode: types.OutputModeUniversal,
    }

    result, err := p.Process(ctx, obj)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if result.TemplatePath != "templates/my-resource-mykind.yaml" {
        t.Errorf("unexpected template path: %s", result.TemplatePath)
    }

    if result.Values["myField"] != "hello" {
        t.Errorf("unexpected values: %v", result.Values)
    }
}
```

### 3.4 Verify

```bash
go test ./pkg/processor/k8s/... -run TestMyKind -v
```

Expected output:

```
--- PASS: TestMyKindProcessor_Process (0.00s)
PASS
```

---

## 4. Adding a New Relationship Detector

Detectors run during stage 3 (Analyze). They scan `[]ProcessedResource` and emit `Relationship` structs into the `ResourceGraph`. Use them to model connections like Service → Deployment or Certificate → Ingress.

### 4.1 Create the detector file

Create `pkg/analyzer/detector/mydetector.go`:

```go
package detector

import (
    "github.com/deckhouse/deckhouse-helm-generator/pkg/analyzer"
    "github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// MyDetector detects relationships between MyKind and Deployment resources.
type MyDetector struct{}

// Name returns the detector's identifier.
func (d *MyDetector) Name() string { return "my-detector" }

// Detect scans all processed resources and records relationships.
func (d *MyDetector) Detect(resources []*types.ProcessedResource, graph *analyzer.ResourceGraph) {
    for _, res := range resources {
        if res.Original.Object.GetKind() != "MyKind" {
            continue
        }

        // Example: find a Deployment with the same name prefix
        for _, other := range resources {
            if other.Original.Object.GetKind() != "Deployment" {
                continue
            }

            if res.ServiceName == other.ServiceName {
                graph.AddRelationship(analyzer.Relationship{
                    From:     res.Original.ResourceKey(),
                    To:       other.Original.ResourceKey(),
                    Type:     "MyKind->Deployment",
                    Strength: 1.0,
                })
            }
        }
    }
}
```

### 4.2 Register the detector

Open `pkg/analyzer/detector/registry.go` and add your detector to `RegisterAll()`:

```go
func RegisterAll(a *analyzer.Analyzer) {
    // ... existing detectors ...
    a.RegisterDetector(&MyDetector{})
}
```

### 4.3 Write tests

```go
package detector

import (
    "testing"
    // ... imports
)

func TestMyDetector_Detect(t *testing.T) {
    // build two ProcessedResources (MyKind + Deployment, same service name)
    // call Detect()
    // assert graph.Relationships has one entry with the correct Type
}
```

---

## 5. Adding a New Generator

Generators run after the Analyze stage as post-processors. They receive a `*GeneratedChart` and return a new one (copy-on-write — ADR-008). Use them to add new template files or values entries.

### 5.1 Understand the copy-on-write contract

Every generator that modifies a `GeneratedChart` **must**:

1. Create a new `map[string]string` for `Templates`.
2. Copy all entries from `chart.Templates` into the new map.
3. Add or modify entries in the new map.
4. Return a new `*types.GeneratedChart` with the new map.

Do not modify `chart.Templates` in place.

### 5.2 Create the generator file

Create `pkg/generator/myfeature.go`:

```go
package generator

import (
    "fmt"

    "github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// MyFeatureConfig holds configuration for MyFeature generation.
type MyFeatureConfig struct {
    Enabled bool
    Label   string
}

// InjectMyFeature adds a ConfigMap template that exposes MyFeature settings.
// It follows the copy-on-write contract (ADR-008).
func InjectMyFeature(chart *types.GeneratedChart, cfg MyFeatureConfig) *types.GeneratedChart {
    if !cfg.Enabled {
        return chart
    }

    // Step 1: new templates map
    templates := make(map[string]string, len(chart.Templates)+1)

    // Step 2: copy existing templates
    for k, v := range chart.Templates {
        templates[k] = v
    }

    // Step 3: add new template
    templates["templates/myfeature-configmap.yaml"] = fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "%s.fullname" . }}-myfeature
  labels:
    {{- include "%s.labels" . | nindent 4 }}
data:
  label: {{ .Values.myfeature.label | quote }}
`, chart.Name, chart.Name)

    // Step 4: return new chart struct
    return &types.GeneratedChart{
        Name:          chart.Name,
        Path:          chart.Path,
        ChartYAML:     chart.ChartYAML,
        ValuesYAML:    chart.ValuesYAML,
        Templates:     templates,
        Helpers:       chart.Helpers,
        Notes:         chart.Notes,
        ValuesSchema:  chart.ValuesSchema,
        ExternalFiles: chart.ExternalFiles,
    }
}
```

### 5.3 Wire the generator into the pipeline

Open `cmd/dhg/main.go`. In `runGenerate()`, add a flag variable at the top of `newGenerateCmd()`:

```go
var myFeature bool
// ...
cmd.Flags().BoolVar(&myFeature, "my-feature", false, "Generate MyFeature ConfigMap")
```

Then apply the generator after the base chart is generated (following the existing Phase 2 post-processor pattern):

```go
if opts.myFeature {
    cfg := generator.MyFeatureConfig{Enabled: true, Label: opts.chartName}
    for i, chart := range charts {
        charts[i] = generator.InjectMyFeature(chart, cfg)
    }
}
```

### 5.4 Write tests

Create `pkg/generator/myfeature_test.go`:

```go
package generator

import (
    "strings"
    "testing"

    "github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func TestInjectMyFeature_AddsConfigMap(t *testing.T) {
    chart := &types.GeneratedChart{
        Name:      "testchart",
        Templates: map[string]string{},
    }

    cfg := MyFeatureConfig{Enabled: true, Label: "test-label"}
    result := InjectMyFeature(chart, cfg)

    const key = "templates/myfeature-configmap.yaml"
    content, ok := result.Templates[key]
    if !ok {
        t.Fatalf("expected template %s not found", key)
    }

    if !strings.Contains(content, "kind: ConfigMap") {
        t.Errorf("template does not contain ConfigMap kind")
    }
}

func TestInjectMyFeature_Disabled_ReturnsOriginal(t *testing.T) {
    chart := &types.GeneratedChart{
        Name:      "testchart",
        Templates: map[string]string{},
    }

    result := InjectMyFeature(chart, MyFeatureConfig{Enabled: false})
    if result != chart {
        t.Error("expected original chart to be returned when disabled")
    }
}
```

---

## 6. Running Tests

### Unit tests

```bash
# All packages
make test

# Single package
go test ./pkg/processor/k8s/... -v

# Single test
go test ./pkg/generator/... -run TestInjectMyFeature -v
```

### With coverage

```bash
make coverage
# Opens coverage report; project gate is 70%, current baseline is 86%+
```

### Integration tests

```bash
go test ./tests/integration/... -v -timeout 60s
```

Integration tests use real YAML fixtures in `tests/integration/testdata/`. They run the full pipeline and assert on generated chart structure. Helm must be installed.

### End-to-end tests

```bash
go test ./tests/e2e/... -v -timeout 120s
```

E2E tests generate a chart and then run `helm lint` and `helm template` against it. Helm must be on `$PATH`.

### Benchmarks

```bash
go test ./pkg/... -bench=. -benchtime=5s
```

### Lint

```bash
make lint
# Equivalent to: golangci-lint run ./...
```

The linter config is in `.golangci.yml`. The CI pipeline runs lint with `continue-on-error: true` — lint failures do not block merges, but should be addressed.

---

## 7. CI/CD Pipeline Overview

All CI runs on GitHub Actions. The workflow files are in `.github/workflows/`.

| Workflow | File | Trigger | Stages |
|----------|------|---------|--------|
| Test | `test.yml` | push, PR | Unit (Go 1.25+1.26 matrix), Integration, E2E, Lint, Security, Coverage |
| Release | `release.yml` | tag push (`v*`) | GoReleaser: build, Docker, Homebrew |
| CodeQL | `codeql.yml` | push, schedule | Static analysis |
| Auto-approve | `auto-approve.yml` | PR | Auto-approve owner PRs via GitHub App bot |

### Branch protection rules

PRs to `main` must pass:

- `Unit Tests (Go 1.26)`
- `Lint Code`
- `Security Scan`
- `Build Binary`

### Coverage gate

The coverage merge step in `test.yml` combines all coverage profiles and enforces a **70% minimum**. The current project-wide coverage is 86%+. New code should maintain this baseline.

---

## 8. Release Process

Releases are fully automated via GoReleaser once a version tag is pushed.

### Create a release

```bash
# Ensure main is clean and tests pass
git checkout main
git pull origin main
make test

# Tag the release
git tag -a v0.8.0 -m "Release v0.8.0"
git push origin v0.8.0
```

GoReleaser then:
1. Builds binaries for Linux (amd64, arm64), macOS (amd64, arm64), Windows (amd64).
2. Creates a GitHub Release with checksums and a changelog.
3. Pushes a multi-arch Docker image to `ghcr.io/alexgromer/dhg:v0.8.0` and `:latest`.
4. Updates the Homebrew tap formula at `AlexGromer/homebrew-tap`.

### Verify a release

```bash
# Check the GitHub Release page
gh release view v0.8.0

# Pull and test the Docker image
docker pull ghcr.io/alexgromer/dhg:v0.8.0
docker run --rm ghcr.io/alexgromer/dhg:v0.8.0 version
```

### Goreleaser config

The full build config is in `.goreleaser.yml`. Key sections: `builds` (Go build flags, CGO disabled), `archives` (tar.gz + zip), `dockers` (multi-arch manifest), `brews` (Homebrew formula).

---

## Reference: Processor Interface

```go
// pkg/processor/processor.go

type Processor interface {
    // Name returns a unique identifier for this processor.
    Name() string

    // Priority controls ordering when multiple processors match the same GVK.
    // Lower number = higher priority.
    Priority() int

    // Supports returns true if this processor can handle the given GVK.
    Supports(gvk schema.GroupVersionKind) bool

    // Process transforms an unstructured resource into a Result.
    Process(ctx Context, obj *unstructured.Unstructured) (*Result, error)
}

type Context struct {
    Ctx                 context.Context
    ChartName           string
    OutputMode          types.OutputMode
    Namespace           string
    AllResources        map[types.ResourceKey]*types.ExtractedResource
    ExternalFileManager *value.ExternalFileManager
    ValueProcessor      *value.Processor
}

type Result struct {
    Processed       bool
    ServiceName     string
    TemplatePath    string
    TemplateContent string
    ValuesPath      string
    Values          map[string]interface{}
    Dependencies    []types.ResourceKey
    Metadata        map[string]interface{}
}
```
