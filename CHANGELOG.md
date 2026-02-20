# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.6.0] - 2026-02-20

> Released: Repository infrastructure — GoReleaser, Docker, Homebrew, test coverage boost

### Added

#### Distribution
- **GoReleaser** release pipeline: multi-platform binaries (linux/darwin/windows, amd64/arm64), checksums, grouped changelog
- **Docker image** via GHCR (`ghcr.io/alexgromer/dhg`) with OCI labels, built by GoReleaser
- **Homebrew tap** formula (`brew install AlexGromer/tap/dhg`) via GoReleaser
- Docker and Homebrew installation sections in README

#### Testing
- Test coverage boost: 53.1% → 78.3% across all packages
- New test suites: types (100%), helm (98.7%), processor (98.1%), analyzer (98.3%), extractor (88.9%), value processor (93.0%)
- CLI smoke tests: 13 tests for command constructors, flags, required flags, help output
- CI now includes `./cmd/...` in unit test coverage

#### Infrastructure
- Apache-2.0 LICENSE, SECURITY.md, CONTRIBUTING.md, CODE_OF_CONDUCT.md
- Issue templates (bug report, feature request), PR template
- Dependabot for Go modules, GitHub Actions, Docker
- CodeQL security analysis workflow
- CODEOWNERS for branch protection
- Codecov integration with dynamic coverage badge

### Changed
- Coverage gate raised from 55% to 70%
- Go CI matrix: removed 1.22 (incompatible with go.mod 1.24), kept 1.23 + 1.24
- GoReleaser ldflags aligned with actual `main.go` vars

### Fixed
- `newVersionCmd` bug: `fmt.Printf` → `fmt.Fprintf(cmd.OutOrStdout())` — output now capturable by cobra
- README download links fixed to match GoReleaser archive naming
- Git clone URL corrected from `deckhouse/` to `AlexGromer/`

---

## [0.5.0] - 2026-02-19

> Released: Breaking change — SanitizeServiceName for all K8s processors

### Changed

- **BREAKING**: Service names derived from resource labels/names are now converted to camelCase (e.g. `my-app` → `myApp`) for consistency with Deckhouse CRD processors and correct Go template access. Users upgrading should rename hyphenated values keys to camelCase equivalents.

### Fixed

#### Code Review Findings (2 CRITICAL + 3 HIGH + 9 MEDIUM + 6 LOW)
- **CRITICAL**: Fix path traversal in volume detector — validate mount paths
- **CRITICAL**: Add Helm template injection escaping — `{{` / `}}` in resource values no longer break templates
- Fix `sanitizeName` bug — `continue` skipped camelCase marker
- Add `relFrom`/`relTo` adjacency indexes to `ResourceGraph` — O(1) lookups replace O(N) scans
- Remove dead `WithDefaultDetectors()` function
- Apply consistent 1MB scanner buffer to `isCommentOnly()`
- Upgrade `golang.org/x/net` v0.26.0 → v0.50.0
- Sort template keys before writing for deterministic output
- Remove unused `currentService` variable in values annotator
- Use `strings.Builder` for O(1) amortized string concatenation
- Use full SHA-256 (32 bytes) for ExternalFileManager checksum
- Clarify `--source` flag description — cluster/gitops sources not yet implemented
- Replace `goto` with labeled `break` in channel-draining loops

---

## [0.4.0] - 2026-02-19

> Released: Deckhouse CRDs + Monitoring Stack + Modern K8s Patterns (18 new processors, 36 total)

### Added

#### Deckhouse CRD Processors (Phase 1)
- **ModuleConfig** processor (`deckhouse.io/v1alpha1`): extracts module settings with version tracking
- **IngressNginxController** processor (`deckhouse.io/v1`): extracts inlet, hostPort/hostWithFailover config, resource requirements
- **ClusterAuthorizationRule** processor (`deckhouse.io/v1`): extracts subjects, accessLevel, namespace restrictions
- **NodeGroup** processor (`deckhouse.io/v1`): extracts nodeType, disruption settings, kubelet configuration, cloudInstances
- **DexAuthenticator** processor (`deckhouse.io/v1`): extracts applicationDomain, sendAuthorizationHeader, allowed groups
- **User** processor (`deckhouse.io/v1`): extracts email, groups, ttl
- **Group** processor (`deckhouse.io/v1`): extracts members list
- **Deckhouse Module Scaffold** (`--deckhouse-module`): generates helm_lib dependency, OpenAPI schemas, `images/` and `hooks/` directories
- **Deckhouse Pattern Detection**: auto-detects `global.enabledModules`, registry configuration, `global.modules.https`

#### Monitoring Stack (Prometheus Operator + Grafana) (Phase 2.1)
- **ServiceMonitor** processor (`monitoring.coreos.com/v1`): extracts endpoints, namespaceSelector, selector with Service dependency
- **PodMonitor** processor (`monitoring.coreos.com/v1`): extracts podMetricsEndpoints, jobLabel, selector
- **PrometheusRule** processor (`monitoring.coreos.com/v1`): extracts alert/record rule groups with expressions
- **GrafanaDashboard** processor: detects ConfigMaps with `grafana_dashboard: "1"` label (priority 110, overrides ConfigMapProcessor)

#### Gateway API (Phase 2.2)
- **HTTPRoute** processor (`gateway.networking.k8s.io/v1`): extracts parentRefs, hostnames, rules with Gateway dependency tracking
- **Gateway** processor (`gateway.networking.k8s.io/v1`): extracts gatewayClassName, listeners with TLS support

#### KEDA Autoscaling (Phase 2.3)
- **ScaledObject** processor (`keda.sh/v1alpha1`): extracts scaleTargetRef, triggers, min/maxReplicaCount with scale-to-zero detection
- **TriggerAuthentication** processor (`keda.sh/v1alpha1`): extracts secretTargetRef, env, podIdentity

#### cert-manager (Phase 2.4)
- **Certificate** processor (`cert-manager.io/v1`): extracts dnsNames, issuerRef, secretName, duration, renewBefore
- **ClusterIssuer** processor (`cert-manager.io/v1`): extracts ACME config, selfSigned, CA settings

#### Modern Patterns (Phase 2.5)
- **Argo Rollouts** processor (`argoproj.io/v1alpha1 Rollout`): extracts canary/blueGreen strategy, pod template
- **ExternalDNS detection**: auto-detects `external-dns.alpha.kubernetes.io/hostname` annotation on Services and Ingresses
- **TopologySpreadConstraints**: extraction from Deployment pod specs

#### Relationship Types
- `gateway_route`: HTTPRoute → Gateway parent reference
- `scale_target`: ScaledObject → Deployment/StatefulSet target

#### Testing
- 97 new unit tests across all processors (TDD: tests first → implement)
- 4 new integration tests: MonitoringStack, GatewayAPI, KEDA, Regression
- Test coverage: processor/k8s 89.2%, detector 92.5%

### Changed
- `processor.Registry`: now registers 32 processors (up from 18 in v0.3.0)
- README.md: updated coverage badge to 89%, expanded capabilities section

---

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
