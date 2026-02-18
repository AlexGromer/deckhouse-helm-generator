# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
