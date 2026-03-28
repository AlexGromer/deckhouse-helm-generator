# Architecture — deckhouse-helm-generator (DHG)

Version: 0.7.2 | Updated: 2026-03-28

## 1. Context

### Purpose

CLI tool (dhg) for automatic generation of Helm charts from Kubernetes manifests. Analyzes resource types, detects relationships and patterns, and produces production-ready Helm chart structures with environment overlays, multi-cloud support, and Deckhouse module scaffolding.

### Actors

| Actor | Role | Interface |
|-------|------|-----------|
| Developer | Runs dhg to convert K8s YAML to Helm charts | CLI |
| CI/CD | Runs dhg in release pipeline | CLI / shell |
| GoReleaser | Builds multi-platform binaries, Docker images, Homebrew | .goreleaser.yml |

### Scope

**In scope:** Helm chart generation from K8s YAML, resource analysis, pattern detection, multi-cloud annotations, air-gapped support, Kustomize overlays, monorepo layout, feature flags, spot/preemptible instances, auto-dependency detection, Deckhouse module scaffolding, helm-unittest test generation.

**Out of scope:** Helm chart deployment, Kubernetes cluster management, runtime configuration, security scanning (use Trivy/Checkov), GitOps CD (use ArgoCD/Flux), policy enforcement (use Kyverno/OPA).

### Design Philosophy

DHG follows Unix philosophy: one tool does one thing well. It generates Helm charts and composes with other tools via stdin/stdout/files:
```
dhg generate -f manifests/ -o chart/    # Generate
checkov -d chart/                       # Scan
helm lint chart/myapp                   # Validate
helmfile apply                          # Deploy
```

---

## 2. Pipeline Architecture

### Data Flow

```
K8s YAML manifests (files/)
    |
    v
[1. EXTRACT] ── Extractor.Extract() ──> []ExtractedResource
    |
    v
[2. PROCESS] ── Processor.Process() ──> []ProcessedResource (templates, values, deps)
    |
    v
[3. ANALYZE] ── Analyzer.Analyze() ──> ResourceGraph (groups, relationships)
    |
    v
[4. GENERATE] ── Generator.Generate() ──> []GeneratedChart
    |                                         |
    |   [4b..4j Phase 2 enhancements]        |
    |   sequential, copy-on-write             |
    |                                         v
[5. WRITE] ── WriteChart() ──> filesystem (Chart.yaml, values.yaml, templates/)
```

### Phase 2 Enhancement Order (cmd/dhg/main.go)

Applied sequentially after base generation. Order matters — later stages operate on results of earlier ones.

| Step | Feature | Flag | Function | Mutates? |
|------|---------|------|----------|----------|
| 4b | Module scaffold | --deckhouse-module | GenerateModuleScaffold | Copy-on-write |
| 4c | Air-gap artifacts | --airgap-registry | ExtractImageReferences + GenerateMirrorScript | Partial CoW |
| 4d | Namespace governance | --namespace-resources | GenerateNamespaceResources + GenerateAutoNetworkPolicies | Copy-on-write |
| 4e | Multi-tenant overlay | --multi-tenant | GenerateMultiTenantOverlay | Copy-on-write |
| 4f | Feature flags | --feature-flags | InjectFeatureFlags | Copy-on-write |
| 4g | Cloud annotations | --cloud-provider | InjectCloudAnnotations | Copy-on-write |
| 4h | Ingress detection | --detect-ingress | DetectIngressController + InjectIngressAnnotations | Copy-on-write |
| 4i | Spot config | --spot | InjectSpotConfig (tolerations + PDB) | Copy-on-write |
| 4j | Auto dependencies | --auto-deps | DetectCommonDependencies + InjectDependencies | Copy-on-write |
| 5b | Env values | --env-values | GenerateEnvValuesForWorkload | New files |
| 5c | Monorepo layout | --monorepo | GenerateMonorepoLayout | New files |
| 5d | Kustomize overlays | --kustomize | GenerateKustomizeLayout | New files |

### Copy-on-Write Immutability Contract

Every Phase 2 generator that modifies a GeneratedChart MUST:
1. Create a new `map[string]string` for Templates
2. Copy all entries from the original chart
3. Add/modify entries in the new map
4. Return a new `*GeneratedChart` struct with the new map

This prevents mutation of charts passed to earlier pipeline stages. Verified in code review (#29, #31, #32).

---

## 3. Components

### Overview

| Component | Package | Purpose | Files |
|-----------|---------|---------|-------|
| CLI | cmd/dhg | Cobra commands, pipeline orchestration | main.go |
| Extractor | pkg/extractor | K8s manifest parsing | 5 files |
| Analyzer | pkg/analyzer | Pattern detection, relationship building | 12 files |
| Generator | pkg/generator | Helm chart generation (28 files) | 28 .go + 28 _test.go |
| Processor | pkg/processor | Per-resource-type processors | 38+ types |
| Helm | pkg/helm | Chart.yaml, values.yaml models | 4 files |
| Types | pkg/types | Shared domain types | 3 files |

### Generator Catalog

#### Core Generators

| File | Purpose | Key Functions | Error Handling |
|------|---------|---------------|----------------|
| generator.go | Interface, registry, chart writer | Generate(), WriteChart(), ValidateChart() | Returns error |
| universal.go | Single chart with all services | (internal to Generator) | Returns error |
| separate.go | One chart per service group | (internal to Generator) | Returns error |
| library.go | Shared library + thin wrappers | (internal to Generator) | Returns error |
| umbrella.go | Parent chart with subcharts | (internal to Generator) | Returns error |

#### Support Generators

| File | Purpose | Key Functions |
|------|---------|---------------|
| grouping.go | Resource grouping by label/relationship/namespace | GroupResources() |
| dependencies.go | Cross-chart dependency detection + circular check | DetectCrossChartDeps() |
| envvalues.go | Environment-specific values (dev/staging/prod) | GenerateEnvValues(), GenerateEnvValuesForWorkload(), DetectWorkloadType() |
| globalvalues.go | Extract common values across groups | ExtractGlobalValues() |
| openapi.go | values.yaml to OpenAPI v3 schema | GenerateOpenAPISchema() |
| modulescaffold.go | Deckhouse module structure | GenerateModuleScaffold() |
| checksum.go | ConfigMap/Secret change detection annotations | AddChecksumAnnotations() |
| apimigration.go | Deprecated API auto-migration (12 entries) | MigrateAPIVersions() |

#### Phase 2 Tier 1 — Infrastructure

| File | Purpose | Input | Output |
|------|---------|-------|--------|
| airgap.go | Air-gapped deployment artifacts | GeneratedChart | images.txt, mirror-images.sh, values-airgap.yaml |
| namespace.go | Namespace governance | ServiceGroup[], NamespaceOpts | ResourceQuota, LimitRange, NetworkPolicy templates |
| networkpolicy.go | Auto-NetworkPolicy from service analysis | ResourceGraph, ServiceGroup[] | Fine-grained NetworkPolicy per group |
| multitenant.go | Multi-tenant overlay | GeneratedChart, tenantCount | Per-tenant namespace/quota/limitrange/NP |

#### Phase 2 Tier 2 — Detection & Annotation

| File | Purpose | Input | Output |
|------|---------|-------|--------|
| featureflags.go | Conditional Helm guards (6 categories) | GeneratedChart, FeatureFlagConfig | Wrapped templates + features values |
| cloudannotations.go | AWS/GCP/Azure LB annotations | GeneratedChart, CloudAnnotationConfig | Annotated Service/Ingress templates |
| ingressdetect.go | Ingress controller detection + annotations | ProcessedResource[], IngressController | Controller-specific annotations |

#### Phase 2 Tier 3 — Advanced Orchestration

| File | Purpose | Input | Output |
|------|---------|-------|--------|
| monorepo.go | Multi-chart monorepo structure | GeneratedChart[], projectName | Makefile, .helmignore, ct.yaml |
| spot.go | Spot/preemptible instance support | GeneratedChart, SpotConfig | Tolerations in pod spec + PDB templates |
| kustomize.go | Helm-to-Kustomize conversion | GeneratedChart | base/ + overlays/{dev,staging,prod}/ |
| autodeps.go | Auto dependency detection (7 infra services) | ProcessedResource[] | Bitnami chart dependencies |

#### Utilities

| File | Purpose | Key Functions |
|------|---------|---------------|
| sanitize.go | Security-sensitive input validation | validateShellSafe(), validateResourceName() |
| helmtest.go | Helm-unittest test scaffold generation | GenerateHelmTests() |

### Shared Helpers (Cross-Generator)

| Function | Defined In | Used By | Purpose |
|----------|-----------|---------|---------|
| extractKind() | featureflags.go | cloudannotations, ingressdetect, helmtest | Parse top-level `kind:` from YAML |
| injectAnnotationsIntoTemplate() | cloudannotations.go | ingressdetect | Idempotent annotation block injection |
| injectTolerationsIntoTemplate() | spot.go | (spot only) | Insert tolerations before `containers:` |
| extractContainers() | envvalues.go | autodeps | Extract container maps from pod spec |
| validateShellSafe() | sanitize.go | airgap | Validate chars for shell interpolation |
| validateResourceName() | sanitize.go | kustomize | Validate resource name + path traversal guard |

### Processor Registry (38+ types)

K8s resource types with dedicated processors:
Certificate, ClusterAuthorizationRule, ClusterIssuer, ClusterRole, ClusterRoleBinding, ConfigMap, CronJob, Deployment, DexAuthenticator, ExternalDNS, Gateway, GrafanaDashboard, Group, HPA, HTTPRoute, Ingress, IngressNginxController, Job, LimitRange, ModuleConfig, NetworkPolicy, NodeGroup, PDB, PodMonitor, PriorityClass, PrometheusRule, ResourceQuota, Role, RoleBinding, Rollout, ScaledObject, Secret, Service, ServiceMonitor, StatefulSet, DaemonSet, PVC, ServiceAccount, TriggerAuthentication, User, VPA.

---

## 4. Decisions (ADR Log)

| ID | Date | Decision | Status | Context |
|----|------|----------|--------|---------|
| ADR-001 | 2026-02-26 | Go 1.24 + Cobra CLI framework | Accepted | Standard for K8s ecosystem tooling |
| ADR-002 | 2026-02-26 | GoReleaser for distribution (GitHub Releases + GHCR + Homebrew) | Accepted | Multi-platform binary distribution |
| ADR-003 | 2026-02-26 | Plugin-style processor registry per resource type | Accepted | 35+ resource types need isolated processing |
| ADR-004 | 2026-02-26 | TDD approach for Phase 2 generators | Accepted | Tests written first, then implementation |
| ADR-005 | 2026-02-27 | Phase 2 split into 3 tiers (Infra / Detection / Advanced) | Accepted | Manageable incremental delivery |
| ADR-006 | 2026-03-27 | Release v0.7.0 with known findings tracked in #29 | Accepted | 0 critical-security, 71 findings |
| ADR-007 | 2026-03-27 | Shared sanitize.go for all input validation | Accepted | Single audit point for security-relevant validation |
| ADR-008 | 2026-03-27 | Copy-on-write as mandatory contract for chart mutations | Accepted | Prevent hidden mutation bugs across pipeline stages |
| ADR-009 | 2026-03-28 | Helm-unittest test scaffold as generator output | Accepted | --include-tests flag, helmtest.go |

### ADR Details

**ADR-007: Shared sanitize.go**
Problem: Shell injection in mirror scripts, Makefile injection via chart names, YAML injection in kustomize.
Decision: Centralize all input validation in `pkg/generator/sanitize.go` with compiled regexes.
Functions: `validateShellSafe()` for shell strings, `validateResourceName()` for YAML entries with path traversal guard, `safeChartName` regex for Makefile/CI targets.
Consequence: All generators import sanitize.go for security checks. Single file to audit.

**ADR-008: Copy-on-write contract**
Problem: Phase 2 generators modify GeneratedChart.Templates map. Shared references cause hidden mutation.
Decision: Every generator that returns a modified chart MUST create a new Templates map via `make()` + copy loop. Original chart must never be mutated.
Consequence: InjectSpotConfig, InjectFeatureFlags, InjectCloudAnnotations, InjectIngressAnnotations, InjectDependencies all follow this pattern. Violation detected and fixed in autodeps.go (CR-2) and namespace-resources block (CR-1).

---

## 5. Constraints

| Constraint | Type | Impact | Mitigation |
|------------|------|--------|------------|
| Go 1.24 minimum | Technical | Library compatibility | Pin in go.mod |
| Coverage gate 70% | Quality | Blocks CI on regressions | Current: 86%+ |
| No K8s cluster required | Design | Air-gapped support | File extraction only |
| Deckhouse module compatibility | Domain | Scaffold must match conventions | Processor/k8s covers 38+ types |
| Copy-on-write for GeneratedChart | Architecture | All Phase 2 generators | ADR-008, verified in code review |

---

## 6. Principles

1. **Resource-type isolation**: each K8s resource type has its own processor — no shared mutable state
2. **TDD for new generators**: write tests first, then implement (ADR-004)
3. **Backward compatibility**: new CLI flags are additive, never breaking
4. **Coverage gate enforced**: 70% minimum, target 86%+
5. **Copy-on-write immutability**: generators must not mutate input charts (ADR-008)
6. **Centralized input validation**: all security-sensitive checks in sanitize.go (ADR-007)
7. **Deterministic output**: sorted keys, sorted annotations, sorted namespace lists for GitOps compatibility

---

## 7. Module Dependencies

| Module | Dependencies | Dependents | Coupling |
|--------|-------------|-----------|----------|
| pkg/types | stdlib | all packages | Low |
| pkg/helm | pkg/types | pkg/generator | Low |
| pkg/extractor | pkg/types | pkg/generator, cmd/dhg | Medium |
| pkg/analyzer | pkg/types, pkg/processor | pkg/generator, cmd/dhg | Medium |
| pkg/processor | pkg/types, pkg/helm | pkg/analyzer, pkg/generator | High |
| pkg/generator | all pkg/* | cmd/dhg | High (28 files) |
| cmd/dhg | pkg/generator, pkg/extractor, pkg/analyzer | — | Entry point |

---

## 8. API Contracts

### Internal APIs

| Interface | Package | Key Methods |
|-----------|---------|-------------|
| Generator | pkg/generator | Generate(ctx, graph, opts) ([]*GeneratedChart, error) |
| Extractor | pkg/extractor | Extract(path) ([]Resource, error) |
| Analyzer | pkg/analyzer | Analyze(resources) AnalysisResult |
| Processor | pkg/processor | Process(resource) ProcessedResource |

### Key Types

```
GeneratedChart {
  Name, Path, ChartYAML, ValuesYAML string
  Templates     map[string]string      // path -> content
  Helpers       string                 // _helpers.tpl
  Notes         string                 // NOTES.txt
  ValuesSchema  string                 // values.schema.json
  ExternalFiles []ExternalFileInfo     // large files in files/
}

Options {
  OutputDir, ChartName, ChartVersion, AppVersion string
  Mode          OutputMode
  Namespace     string
  IncludeTests  bool     // --include-tests → helmtest.go
  IncludeREADME bool
  IncludeSchema bool
  EnvValues     bool     // --env-values
  DeckhouseModule bool   // --deckhouse-module
}
```

### External Integrations

| Service | Protocol | Auth | Purpose |
|---------|----------|------|---------|
| GitHub Releases | REST | GITHUB_TOKEN | Binary distribution via GoReleaser |
| GHCR (ghcr.io) | OCI | GITHUB_TOKEN | Docker image distribution |
| Homebrew tap | git | GITHUB_TOKEN | Homebrew formula update |

---

## 9. CI/CD Pipeline

### Pipeline Stages

| Stage | Tool | Trigger | Gate |
|-------|------|---------|------|
| Lint | golangci-lint v9 | PR | Informational (continue-on-error) |
| Unit tests | go test (Go 1.23 + 1.24 matrix) | PR | Block on fail |
| Integration tests | go test ./tests/integration/... | PR | Block on fail |
| E2E tests | go test ./tests/e2e/... + helm | PR | Block on fail |
| Security scan | gitleaks + Trivy + CodeQL | PR + push | Block on critical |
| Coverage gate | go test -coverprofile | PR | Block if < 70% |
| Build | make build | PR | Block on fail |
| Release | GoReleaser | tag push (v*) | Publish binaries + Docker + Homebrew |
| Auto-approve | GitHub App bot | PR (owner only) | Review approval |

### Branch Strategy

GitHub flow: feature branches -> PR -> squash merge to main. Release via `git tag v*` -> GoReleaser.

Branch protection: 4 required status checks + 1 review (auto-approve bot for owner PRs).

---

## 10. Environments

| Environment | Purpose | Infra | Access |
|-------------|---------|-------|--------|
| local | Development | Go toolchain + Helm CLI | Developer machine |
| CI | Testing | GitHub Actions (ubuntu-latest) | PR / push events |
| release | Distribution | GoReleaser + GHCR | On v* tag push |

### Local Development

```bash
make build          # Build binary to ./bin/dhg
make test           # Run all tests
make lint           # Run golangci-lint
make coverage       # Generate coverage report
./bin/dhg generate --help
```

### Release Process

1. Update CHANGELOG.md with new version
2. Create PR, merge to main
3. `git tag v0.X.Y && git push origin v0.X.Y`
4. GoReleaser automatically: build binaries (6 platforms), push Docker image, update Homebrew formula
5. Verify: `gh release view v0.X.Y`, `docker pull ghcr.io/alexgromer/dhg:0.X.Y`

---

## 11. Code Review Status (Issue #29)

71 findings from Phase 2 code review (2026-03-27).

| Batch | Version | Findings | Status |
|-------|---------|----------|--------|
| P1 (security + critical correctness) | v0.7.1 | 10 | Done |
| P2 (high correctness + quality) | v0.7.2 | 10 | Done |
| P3 (medium findings) | v0.7.3-dev | 8 of 10 | Done (pending PR) |
| Remaining (LOW + INFO) | backlog | 43 | Queue |

Key fixes applied:
- Input sanitization (shell, Makefile, YAML, path traversal) via sanitize.go
- Copy-on-write enforcement across all generators
- Cloud provider validation, port range validation
- Annotation injection idempotency
- Tolerations placement fix (inside pod spec, not document root)
- Deterministic output (sorted keys, sorted namespaces)

---

## 12. Roadmap Phases

| Phase | Status | Description |
|-------|--------|-------------|
| Phase 1 | Done (v0.6.0) | Core pipeline, 38+ processors, pattern detectors, CLI |
| Phase 2 | Done (v0.7.0) | 12 architecture generators (3 tiers), 161 tests |
| Phase 2.5 | Planned | Security & Compliance (PSS, RBAC, External Secrets) — 10 tasks |
| Phase 3 | Partial (v0.4.0) | Deckhouse CRDs, monitoring, Gateway API, KEDA, cert-manager |
| Phase 4 | Planned | Cluster Extractor + GitOps Extractor — 14 tasks |
| Phase 5 | Planned | Auto-Fix Engine, CRD Support, Migration, Smart Analysis — 38 tasks |
| Pass 7-15 | Research done | Service Mesh, AI/ML, Database Operators, Edge Computing |

Full roadmap: docs/ROADMAP.md (178K, 15 research passes).

---

## 13. Change Log

| Date | Change | ADR | Author |
|------|--------|-----|--------|
| 2026-02-26 | Phase 1: core pipeline | ADR-001 | @AlexGromer |
| 2026-02-26 | Phase 2 Tier 1: airgap, namespace, networkpolicy, multitenant | ADR-004 | @AlexGromer |
| 2026-02-26 | Phase 2 Tier 2: featureflags, cloudannotations, ingressdetect | ADR-004 | @AlexGromer |
| 2026-02-27 | Phase 2 Tier 3: monorepo, spot, kustomize, autodeps | ADR-005 | @AlexGromer |
| 2026-03-27 | Code review (71 findings) + Release v0.7.0 | ADR-006 | @AlexGromer |
| 2026-03-27 | v0.7.1: 10 P1 fixes (security + correctness) | ADR-007, ADR-008 | @AlexGromer |
| 2026-03-27 | v0.7.2: 10 P2 fixes + deps bump | — | @AlexGromer |
| 2026-03-28 | v0.7.3-dev: 8 P2/P3 fixes + helmtest.go (1.3.2) | ADR-009 | @AlexGromer |
| 2026-03-28 | ARCHITECTURE.md full rewrite | — | @AlexGromer |
