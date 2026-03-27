# FILEMAP — deckhouse-helm-generator

## Root

| Path | Purpose | Key exports |
|------|---------|-------------|
| `cmd/dhg/main.go` | CLI entry point | `main()`, Cobra root command |
| `cmd/dhg/main_test.go` | CLI tests | — |
| `go.mod` | Go 1.24 module definition | `github.com/AlexGromer/deckhouse-helm-generator` |
| `Makefile` | Build, test, lint targets | `build`, `test`, `lint`, `coverage` |
| `.goreleaser.yml` | GoReleaser config | GitHub Releases + GHCR + Homebrew |
| `Dockerfile` | Container build | — |
| `Dockerfile.goreleaser` | GoReleaser container build | — |
| `.golangci.yml` | Linter configuration | — |

## pkg/analyzer — Resource analysis engine

| Path | Purpose | Key exports |
|------|---------|-------------|
| `pkg/analyzer/analyzer.go` | Core resource analyzer | `Analyzer`, `Analyze()` |
| `pkg/analyzer/analyzer_test.go` | Analyzer tests | — |
| `pkg/analyzer/detector/` | Resource type detectors | Annotation, Deckhouse, Label, Reference, Volume detectors |
| `pkg/analyzer/pattern/` | Pattern analysis | Checkers, Detectors, Formatter, Recommender, Types |

## pkg/extractor — Resource extraction from files

| Path | Purpose | Key exports |
|------|---------|-------------|
| `pkg/extractor/extractor.go` | Core extractor | `Extractor`, `Extract()` |
| `pkg/extractor/cluster.go` | Cluster-scoped resources | Cluster resource extraction |
| `pkg/extractor/file.go` | File parsing | YAML/JSON file reading |
| `pkg/extractor/gitops.go` | GitOps detection | GitOps pattern detection |
| `pkg/extractor/extractor_test.go` | Extractor tests | — |

## pkg/generator — Helm chart generation (core)

| Path | Purpose | Key exports |
|------|---------|-------------|
| `pkg/generator/generator.go` | Core generator orchestrator | `Generator`, `Generate()`, `Options` |
| `pkg/generator/universal.go` | Universal chart generation | `GenerateUniversalChart()` |
| `pkg/generator/separate.go` | Separate chart per resource | `GenerateSeparateCharts()` |
| `pkg/generator/library.go` | Library chart generation | `GenerateLibraryChart()` |
| `pkg/generator/library_helpers.go` | Library chart helpers | Helper templates |
| `pkg/generator/umbrella.go` | Umbrella chart generation | `GenerateUmbrellaChart()` |
| `pkg/generator/grouping.go` | Resource grouping logic | `GroupResources()` |
| `pkg/generator/dependencies.go` | Chart dependency detection | `DetectDependencies()` |
| `pkg/generator/envvalues.go` | Environment-specific values | `GenerateEnvValues()`, workload-aware profiles |
| `pkg/generator/globalvalues.go` | Global values generation | `GenerateGlobalValues()` |
| `pkg/generator/openapi.go` | OpenAPI schema generation | `GenerateOpenAPISchema()` |
| `pkg/generator/modulescaffold.go` | Deckhouse module scaffold | `GenerateModuleScaffold()` |
| `pkg/generator/checksum.go` | Config checksum annotations | `AddChecksumAnnotations()` |
| `pkg/generator/apimigration.go` | API version migration | `MigrateAPIVersions()` |

### Phase 2 Tier 1 — Infrastructure generators (v0.7.0)

| Path | Purpose | Key exports |
|------|---------|-------------|
| `pkg/generator/airgap.go` | Air-gapped environment support | `ExtractImageReferences()`, `GenerateImageList()`, `GenerateAirgapValues()`, `GenerateMirrorScript()` |
| `pkg/generator/airgap_test.go` | Airgap tests (316 LOC, 14 tests) | — |
| `pkg/generator/namespace.go` | ResourceQuota/LimitRange/NetworkPolicy | `GenerateNamespaceResources()`, `GenerateResourceQuotaTemplate()`, `GenerateLimitRangeTemplate()` |
| `pkg/generator/namespace_test.go` | Namespace tests (334 LOC, 15 tests) | — |
| `pkg/generator/networkpolicy.go` | Auto-NetworkPolicy from service analysis | `GenerateAutoNetworkPolicies()` |
| `pkg/generator/networkpolicy_test.go` | NetworkPolicy tests (296 LOC, 11 tests) | — |
| `pkg/generator/multitenant.go` | Multi-tenant overlay | `GenerateMultiTenantOverlay()` |
| `pkg/generator/multitenant_test.go` | Multi-tenant tests (279 LOC, 10 tests) | — |

### Phase 2 Tier 2 — Detection & annotation generators (v0.7.0)

| Path | Purpose | Key exports |
|------|---------|-------------|
| `pkg/generator/featureflags.go` | Feature-flag guards (6 categories) | `DefaultFeatureFlagConfig()`, `InjectFeatureFlags()` |
| `pkg/generator/featureflags_test.go` | Feature flags tests (447 LOC, 12 tests) | — |
| `pkg/generator/cloudannotations.go` | AWS/GCP/Azure annotations | `GenerateCloudAnnotations()`, `InjectCloudAnnotations()`, `injectAnnotationsIntoTemplate()` |
| `pkg/generator/cloudannotations_test.go` | Cloud annotations tests (413 LOC, 13 tests) | — |
| `pkg/generator/ingressdetect.go` | Ingress controller detection (nginx/traefik/haproxy/istio) | `DetectIngressController()`, `GenerateIngressAnnotations()`, `InjectIngressAnnotations()` |
| `pkg/generator/ingressdetect_test.go` | Ingress detection tests (445 LOC, 20 tests) | — |

### Phase 2 Tier 3 — Advanced generators (v0.7.0)

| Path | Purpose | Key exports |
|------|---------|-------------|
| `pkg/generator/monorepo.go` | Monorepo layout (Makefile, ct.yaml) | `GenerateMonorepoLayout()` |
| `pkg/generator/monorepo_test.go` | Monorepo tests (275 LOC, 10 tests) | — |
| `pkg/generator/spot.go` | Spot/preemptible instance support | `GenerateSpotTolerations()`, `GenerateSpotPDB()`, `InjectSpotConfig()` |
| `pkg/generator/spot_test.go` | Spot tests (388 LOC, 12 tests) | — |
| `pkg/generator/kustomize.go` | Kustomize overlay generation (base + 3 envs) | `GenerateKustomizeLayout()` |
| `pkg/generator/kustomize_test.go` | Kustomize tests (395 LOC, 16 tests) | — |
| `pkg/generator/autodeps.go` | Auto dependency detection (7 infra services) | `DetectCommonDependencies()`, `FilterExistingDependencies()`, `InjectDependencies()` |
| `pkg/generator/autodeps_test.go` | Autodeps tests (463 LOC, 14 tests) | — |

## pkg/helm — Helm chart structure

| Path | Purpose | Key exports |
|------|---------|-------------|
| `pkg/helm/chart.go` | Chart.yaml model | `Chart`, `NewChart()` |
| `pkg/helm/values.go` | values.yaml model | `Values` |
| `pkg/helm/helpers.go` | Template helpers | `_helpers.tpl` generation |
| `pkg/helm/helm_test.go` | Helm tests | — |

## pkg/processor — Resource processors

| Path | Purpose | Key exports |
|------|---------|-------------|
| `pkg/processor/processor.go` | Processor interface & orchestration | `Processor`, `Process()` |
| `pkg/processor/registry.go` | Processor registry | `Registry`, `Register()` |
| `pkg/processor/deckhouse/` | Deckhouse-specific processors | (empty — reserved) |
| `pkg/processor/k8s/` | Kubernetes resource processors | 35+ resource type processors |
| `pkg/processor/monitoring/` | Monitoring resource processors | `external.go`, `processor.go` |
| `pkg/processor/value/` | (reserved) | — |

### pkg/processor/k8s — Individual resource processors

Covers: Certificate, ClusterAuthorizationRule, ClusterIssuer, ClusterRole, ClusterRoleBinding, Common, ConfigMap, CronJob, Deployment, DexAuthenticator, ExternalDNS, Gateway, GrafanaDashboard, Group, HPA, HTTPRoute, Ingress, IngressNginxController, Job, LimitRange, ModuleConfig, NetworkPolicy, NodeGroup, PDB, PodMonitor, PriorityClass, PrometheusRule, ResourceQuota, Role, RoleBinding, Rollout, ScaledObject, Secret, Service, ServiceMonitor, TriggerAuthentication, User, VPA.

## pkg/types — Shared types

| Path | Purpose | Key exports |
|------|---------|-------------|
| `pkg/types/resource.go` | Resource model | `Resource` struct |
| `pkg/types/relationship.go` | Relationship model | `Relationship` struct |
| `pkg/types/types_test.go` | Types tests | — |

## pkg/testutil — Test utilities

| Path | Purpose | Key exports |
|------|---------|-------------|
| `pkg/testutil/factory.go` | Test resource factory | `NewResource()`, `NewDeployment()` |
| `pkg/testutil/test_utils.go` | Shared test helpers | `AssertContains()`, `TempDir()` |
| `pkg/testutil/coverage.go` | Coverage tracking | `CoverageReport()` |
| `pkg/testutil/mock_processor.go` | Mock processor for tests | `MockProcessor` |
| `pkg/testutil/fixtures/` | Test fixture YAML files | — |

## tests/ — Integration & E2E tests

| Path | Purpose | Key exports |
|------|---------|-------------|
| `tests/integration/` | Integration test suite | Pipeline tests (autoscaling, batch, deckhouse, fullstack, library, rbac, security, separate, simple, umbrella) |
| `tests/integration/framework.go` | Integration test framework | `SetupIntegration()` |
| `tests/integration/benchmark_test.go` | Performance benchmarks | — |
| `tests/integration/validate_test.go` | Validation tests | — |
| `tests/e2e/` | End-to-end test suite | Helm lint, template, install tests |
| `tests/e2e/framework.go` | E2E test framework | `SetupE2E()` |
| `tests/e2e/helm.go` | Helm operations for E2E | — |
| `tests/e2e/kubernetes.go` | K8s operations for E2E | — |

## data/ — Static data and samples

| Path | Purpose | Key exports |
|------|---------|-------------|
| `data/rules/` | Deckhouse analysis rules | — |
| `data/samples/` | Sample Kubernetes manifests | — |
| `data/results/` | Analysis result examples | — |

## .github/workflows/ — CI/CD

| Path | Purpose | Key exports |
|------|---------|-------------|
| `.github/workflows/test.yml` | Unit + integration + e2e + lint + security + coverage gate (70%) | — |
| `.github/workflows/release.yml` | GoReleaser release pipeline | — |
| `.github/workflows/codeql.yml` | CodeQL security analysis | — |
| `.github/workflows/auto-approve.yml` | Auto-approve dependabot PRs | — |

## docs/ — Documentation

| Path | Purpose | Key exports |
|------|---------|-------------|
| `docs/ROADMAP.md` | Project roadmap (15 research passes) | — |
| `docs/velocity.md` | Development velocity tracking (v0.2.0 baseline + v0.7.0) | — |
| `docs/velocity_v0.3.0.md` | Velocity tracking v0.3.0 (~28.5x avg) | — |
| `docs/velocity_v0.4.0.md` | Velocity tracking v0.4.0 (timestamp protocol) | — |
| `docs/benchmark_baseline.md` | Performance baseline (v0.2.0, valid through v0.7.0) | — |
| `docs/RELEASE_v0.2.0.md` | Release notes v0.2.0 | — |
| `docs/RELEASE_v0.3.0.md` | Release notes v0.3.0 | — |
| `docs/RELEASE_v0.4.0.md` | Release notes v0.4.0 | — |
| `docs/RELEASE_v0.7.0.md` | Release notes v0.7.0 (Phase 1 + Phase 2) | — |

## Root project files

| Path | Purpose | Key exports |
|------|---------|-------------|
| `CHANGELOG.md` | Keep a Changelog format, all versions | — |
| `ARCHITECTURE.md` | C4 model, ADR log, module deps, CI/CD | — |
| `BACKLOG.md` | Task tracking (Claude Code integration) | — |
| `FILEMAP.md` | This file — project file map | — |
