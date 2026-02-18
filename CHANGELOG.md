# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.0] - 2026-02-18

> Released: 4 new output modes (Separate, Library, Umbrella) + Environment-Specific Values

### Added

#### New Output Modes
- **Separate mode** (`--mode separate`): generates an independent Helm chart per service group with inter-chart dependency declarations
- **Library mode** (`--mode library`): generates a shared library chart with DRY named templates (`library.resources`, `library.probes`, `library.env`, `library.volumeMounts`, etc.) plus thin wrapper charts per service
- **Umbrella mode** (`--mode umbrella`): generates a parent umbrella chart with all services as subcharts in `charts/` directory; each subchart has `condition: <name>.enabled` for runtime toggling

#### Environment-Specific Values
- **`--env-values` flag**: generates `values-dev.yaml`, `values-staging.yaml`, `values-prod.yaml` with environment-specific overrides (override-only, no full base copy)
- Dev profile: `replicaCount: 1`, `logLevel: debug`, PDB disabled, no resource limits
- Staging profile: `replicaCount: 2`, `logLevel: info`, PDB `minAvailable: 1`
- Prod profile: `replicaCount: 3`, `logLevel: warn`, PDB `minAvailable: 2`, resource limits, pod anti-affinity

#### Generators
- `SeparateGenerator`: implements `Generator` interface, groups resources by service, generates inter-chart dependency declarations
- `LibraryGenerator`: implements `Generator` interface, generates library chart + wrapper charts with DRY shared templates
- `UmbrellaGenerator`: implements `Generator` interface, generates umbrella parent chart with conditional subchart dependencies

#### DRY Shared Templates (Library mode)
- Named templates: `library.resources`, `library.probes`, `library.securityContext`, `library.containerSecurityContext`, `library.env`, `library.volumeMounts`, `library.volumes`, `library.labels`, `library.annotations`
- Workload templates (Deployment, StatefulSet, DaemonSet, Job, CronJob) use shared sub-templates via `include`

#### Testing
- 22 unit tests for `GenerateEnvValues` covering all 8 subtasks (dev/staging/prod profiles, detection, naming, override-only, CLI flags, multi-mode)
- 13 unit tests for `UmbrellaGenerator` (structure, cascading values, global values, conditional subcharts)
- 5 unit tests for conditional subchart toggling (condition field, default-enabled, helm template integration)
- 6 unit tests for DRY template structure
- 7 integration tests for Library pipeline
- 9 integration tests for Umbrella pipeline
- Regression tests: all 3 existing modes verified in new test suites

### Changed
- `Options` struct: added `EnvValues bool` field (defaults to `false`)
- README.md: documented all 4 output modes, `--env-values` flag, comparison table, usage examples
- `DefaultRegistry()`: now registers all 4 generators (Universal, Separate, Library, Umbrella)

### Fixed
- Library chart templates: `{{ .values | toJson | fromJson }}` conversion for `dig` compatibility with `chartutil.Values` named type

---

## [0.2.0] - 2026-02-18

### Added

#### New Processors (Phase 2)
- **HorizontalPodAutoscaler (HPA)** processor: extracts scaleTargetRef, min/maxReplicas, CPU/memory/custom/external metrics, scale-up/scale-down behavior policies
- **PodDisruptionBudget (PDB)** processor: extracts minAvailable (int/percent), maxUnavailable (int/percent), selector, unhealthyPodEvictionPolicy
- **NetworkPolicy** processor: extracts podSelector, policyTypes, ingress/egress rules with pod/namespace/IP block selectors and port specifications
- **CronJob** processor: extracts schedule, timeZone, concurrencyPolicy, suspend, history limits, startingDeadlineSeconds, jobTemplate spec, containers with image split
- **Job** processor: extracts completions, parallelism, backoffLimit, activeDeadlineSeconds, ttlSecondsAfterFinished, completionMode, suspend; embeds Helm hook annotations inline in templates
- **Role** processor: extracts RBAC rules (apiGroups, resources, verbs, resourceNames)
- **ClusterRole** processor: extracts rules and aggregationRule with clusterRoleSelectors
- **RoleBinding** processor: extracts roleRef, subjects; creates dependencies on referenced Role and ServiceAccount
- **ClusterRoleBinding** processor: extracts roleRef, subjects; creates dependencies on referenced ClusterRole and ServiceAccount

#### Testing
- E2E test framework with mock Kubernetes API server for Helm `--dry-run=client` validation
- Helm lint validation tests (5 test functions)
- Helm template rendering tests (8 test functions)
- Helm install dry-run tests (6 test functions)
- Integration tests for HPA, PDB, NetworkPolicy, CronJob, Job processors in full pipeline
- Integration test for RBAC chain (ServiceAccount + Role + RoleBinding + ClusterRole + ClusterRoleBinding)

#### Infrastructure
- CI/CD pipeline with separate jobs: unit-tests, integration-tests, e2e-tests, lint, coverage, build
- Coverage merging across test types with 80% threshold gate
- Helm CLI installation in CI for E2E tests

### Fixed
- `nestedInt64` helper to handle both `int64` (programmatic) and `float64` (YAML-parsed) numeric values in unstructured objects
- Mock API server returns 404 for resource lookups to prevent "exists and cannot be imported" errors in Helm dry-run
- Added `--disable-openapi-validation` flag for client-side dry-run to work with Helm v3.20+

## [0.1.0] - 2026-01-15

### Added
- Initial release with core processors: Deployment, StatefulSet, DaemonSet, Service, Ingress, ConfigMap, Secret, PVC, ServiceAccount
- 4-stage pipeline: Extract, Process, Analyze, Generate
- File extractor for YAML/JSON manifests
- Relationship detection: LabelSelector, NameReference, VolumeMount, EnvFrom, EnvValueFrom, Annotation, ServiceAccount, ImagePullSecret
- Universal chart generator with values.yaml, templates, _helpers.tpl
- CLI with `generate` command
