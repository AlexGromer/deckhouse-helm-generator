# Deckhouse Helm Generator — Implementation Plan v3.0

**Version**: 3.0.0 (Goal-Result-Criteria Framework)
**Created**: 2026-02-18
**Status**: Active
**Methodology**: TDD + Atomic Decomposition + Velocity-Calibrated Estimation
**Predecessor**: IMPLEMENTATION_PLAN_v2.md (v0.2.0 — COMPLETED 2026-02-18)

---

## Velocity Calibration

### v0.2.0 Actuals

| Metric | Value |
|--------|-------|
| Tasks completed | 24/24 |
| Estimated (solo) | 290.5h |
| Actual (AI-assisted) | 21.55h |
| Average velocity | **15.6x** |
| Sessions used | 3 |

### Estimation Model for v0.3.0

All estimates use **dual format**:
- **Solo (human)**: Based on task complexity, adjusted by v0.2.0 velocity data
- **AI-assisted**: `solo_estimate / 10` (conservative, accounts for increased complexity)

**Note**: v0.3.0 tasks are MORE complex than v0.2.0 (new algorithms vs patterns). Expected AI velocity: 8-12x (vs 15x for v0.2.0).

---

## v0.3.0 — Complete Output Modes (Q3 2026)

**Release Goal**: Implement all three Helm chart generation modes + environment-aware configuration
**Release Result**: v0.3.0 with Separate, Library, and Umbrella generators + env-specific values
**Release Criteria** (FROZEN):
- [ ] All three output modes functional (`--mode separate|library|umbrella`)
- [ ] Separate Generator: per-service charts with inter-chart dependencies
- [ ] Library Generator: base library + thin wrapper charts
- [ ] Umbrella Generator: parent chart with conditional subcharts
- [ ] Environment-specific values generation (`values-{env}.yaml`)
- [ ] Test coverage ≥80% for all new code
- [ ] All existing tests still pass (regression)
- [ ] Documentation updated (README, CHANGELOG)
- [ ] Performance: <10s for 100 resources in any mode

**Scope** (from ROADMAP Phase 2):
- Section 2.1: Separate Generator (5 tasks)
- Section 2.2: Library Generator (4 tasks)
- Section 2.3: Umbrella Generator (3 tasks)
- Section 2.4.3: Environment-specific values (1 task)
- Testing & Documentation (3 tasks)

**Out of scope** (deferred to v0.3.5 or v0.4.0):
- Architecture-specific generation (2.4.1, 2.4.2, 2.4.4-2.4.10)
- Namespace management (2.5)
- Cluster/GitOps extractors (ROADMAP Phase 4)
- Security & Compliance (ROADMAP Phase 2.5)

---

## Week 1-2: Separate Generator (Days 1-10)

### Task 1.1: Service Grouping Algorithm

**Goal**: Implement algorithm to group resources into logical services for per-chart generation
**Result**: `pkg/generator/grouping.go` with service grouping by labels, namespace, and relationship graph
**Criteria** (FROZEN):
- [ ] All 5 subtasks completed
- [ ] Groups resources by `app.kubernetes.io/name` label (primary)
- [ ] Falls back to namespace grouping when labels absent
- [ ] Connected component detection from relationship graph
- [ ] Test coverage ≥80%
- [ ] All tests PASS

**Subtasks** (atomic):

1. **Write test: Group by app.kubernetes.io/name label**
   - Input: 6 resources, 2 apps (3 each: Deployment+Service+ConfigMap)
   - Expected: 2 groups, 3 resources each
   - Lines: ~50

2. **Write test: Group by namespace when labels absent**
   - Input: 4 resources across 2 namespaces, no standard labels
   - Expected: 2 groups by namespace
   - Lines: ~40

3. **Write test: Connected components from relationship graph**
   - Input: 5 resources with 3 relationships forming 2 components
   - Expected: 2 groups matching connected components
   - Lines: ~60

4. **Write test: Edge cases**
   - Test: Single resource → single group
   - Test: Resources with conflicting labels (different label values)
   - Test: Orphan resources (no labels, no relationships) → own group
   - Lines: ~50

5. **Implement grouping algorithm**
   - File: `pkg/generator/grouping.go`
   - Strategy priority: label > relationship > namespace > individual
   - Lines: ~200

**Time Estimate**:
- **Solo**: 10-12h
- **AI-assisted**: 1.0-1.2h

**Dependencies**: None (uses existing types)
**Blocks**: Tasks 1.2-1.5

---

### Task 1.2: Separate Generator — Per-Service Chart Generation

**Goal**: Generate independent Helm chart for each service group
**Result**: `pkg/generator/separate.go` implementing `Generator` interface
**Criteria** (FROZEN):
- [ ] All 6 subtasks completed
- [ ] Each service group produces a complete Helm chart (Chart.yaml, values.yaml, templates/)
- [ ] Chart name derived from service name
- [ ] Templates only include resources for that service
- [ ] Values scoped to single service (no nesting)
- [ ] Test coverage ≥80%

**Subtasks** (atomic):

1. **Write test: Single service chart generation**
   - Input: 1 group (Deployment + Service)
   - Expected: 1 chart with 2 templates, flat values
   - Lines: ~50

2. **Write test: Multiple service charts**
   - Input: 3 groups (frontend, backend, database)
   - Expected: 3 independent charts
   - Lines: ~60

3. **Write test: Chart.yaml per service**
   - Validation: name = service name, version, apiVersion, dependencies
   - Lines: ~40

4. **Write test: Values scoped to service**
   - Expected: No `services.frontend.*` nesting, just `replicaCount`, `image`, etc.
   - Lines: ~40

5. **Write test: Helpers per service**
   - Expected: _helpers.tpl with chart-specific fullname, labels
   - Lines: ~30

6. **Implement SeparateGenerator**
   - File: `pkg/generator/separate.go`
   - Register in `DefaultRegistry()` for `OutputModeSeparate`
   - Lines: ~250

**Time Estimate**:
- **Solo**: 12-14h
- **AI-assisted**: 1.2-1.4h

**Dependencies**: Task 1.1 (grouping algorithm)

---

### Task 1.3: Inter-Chart Dependencies

**Goal**: Generate `Chart.yaml` dependencies between related service charts
**Result**: Dependency detection from relationship graph + Chart.yaml generation with dependencies section
**Criteria** (FROZEN):
- [ ] All 4 subtasks completed
- [ ] Cross-service relationships → Chart.yaml dependencies
- [ ] `file://` repository for local subcharts
- [ ] Condition field for optional dependencies
- [ ] Test coverage ≥80%

**Subtasks** (atomic):

1. **Write test: Detect cross-chart dependencies**
   - Input: Frontend → Backend relationship (Service→Deployment)
   - Expected: Frontend Chart.yaml has dependency on backend
   - Lines: ~50

2. **Write test: file:// repository references**
   - Expected: `repository: file://../backend` in dependency
   - Lines: ~30

3. **Write test: Conditional dependencies**
   - Expected: `condition: backend.enabled` in dependency
   - Lines: ~35

4. **Implement dependency generation**
   - Extend SeparateGenerator to analyze cross-group relationships
   - Lines: ~120

**Time Estimate**:
- **Solo**: 8-10h
- **AI-assisted**: 0.8-1.0h

**Dependencies**: Task 1.2

---

### Task 1.4: Shared Values (Global)

**Goal**: Generate parent values.yaml with global settings shared across service charts
**Result**: Global values propagation to child charts via `.Values.global.*`
**Criteria** (FROZEN):
- [ ] All 4 subtasks completed
- [ ] Global values extracted (image registry, environment, common labels)
- [ ] Child charts reference `{{ .Values.global.imageRegistry }}`
- [ ] Parent values.yaml has global section
- [ ] Test coverage ≥80%

**Subtasks** (atomic):

1. **Write test: Extract global values from multiple services**
   - Input: 3 services sharing same image registry
   - Expected: `global.imageRegistry` in parent values
   - Lines: ~50

2. **Write test: Global environment variables**
   - Input: Common env vars across services (LOG_LEVEL, ENV)
   - Expected: `global.env` section
   - Lines: ~40

3. **Write test: Child chart references global**
   - Expected: Template uses `{{ .Values.global.imageRegistry }}`
   - Lines: ~40

4. **Implement global values extraction**
   - Detect common values across services
   - Generate parent values.yaml with global section
   - Lines: ~150

**Time Estimate**:
- **Solo**: 8-10h
- **AI-assisted**: 0.8-1.0h

**Dependencies**: Task 1.2

---

### Task 1.5: Integration Tests — Separate Generator

**Goal**: End-to-end tests for Separate generation mode
**Result**: `tests/integration/pipeline_separate_test.go`
**Criteria** (FROZEN):
- [ ] All 4 subtasks completed
- [ ] Full pipeline test: YAML → Separate charts
- [ ] Multi-service scenarios validated
- [ ] Generated charts pass `helm lint`
- [ ] All tests PASS

**Subtasks** (atomic):

1. **Write test: Simple 2-service separation**
   - Input: Frontend (Deployment+Service+Ingress) + Backend (Deployment+Service+ConfigMap)
   - Expected: 2 charts, correct templates in each
   - Lines: ~80

2. **Write test: 3-tier app with dependencies**
   - Input: Frontend → Backend → Database
   - Expected: 3 charts, dependency chain in Chart.yaml
   - Lines: ~100

3. **Write test: Shared ConfigMap across services**
   - Input: 2 services referencing same ConfigMap
   - Expected: ConfigMap in one chart or duplicated with warning
   - Lines: ~70

4. **Write test: Helm lint all generated charts**
   - Lint each generated chart
   - Lines: ~40

**Time Estimate**:
- **Solo**: 8-10h
- **AI-assisted**: 0.8-1.0h

**Dependencies**: Tasks 1.2-1.4

---

## Week 3-4: Library Generator (Days 11-20)

### Task 2.1: Base Library Chart

**Goal**: Generate a `type: library` Helm chart with reusable named templates
**Result**: `pkg/generator/library.go` producing base library chart
**Criteria** (FROZEN):
- [ ] All 5 subtasks completed
- [ ] Chart.yaml with `type: library`
- [ ] Named templates for: deployment, service, ingress, configmap, secret, statefulset
- [ ] Templates parameterized via dictionary/context pattern
- [ ] Test coverage ≥80%

**Subtasks** (atomic):

1. **Write test: Library Chart.yaml**
   - Expected: `type: library` in Chart.yaml
   - Lines: ~30

2. **Write test: Named deployment template**
   - Expected: `{{- define "library.deployment" -}}` in _deployment.tpl
   - Input: Context with `.replicaCount`, `.image`, `.resources`
   - Lines: ~50

3. **Write test: Named service template**
   - Expected: `{{- define "library.service" -}}`
   - Lines: ~40

4. **Write test: Named templates for all resource types**
   - Expected: Templates for all 18 supported K8s resources
   - Lines: ~60

5. **Implement LibraryGenerator (base chart)**
   - File: `pkg/generator/library.go`
   - Generate library chart with named templates
   - Lines: ~350

**Time Estimate**:
- **Solo**: 14-16h
- **AI-assisted**: 1.4-1.6h

**Dependencies**: None (new generator)

---

### Task 2.2: Wrapper Charts

**Goal**: Generate thin wrapper charts that depend on the library chart
**Result**: Per-service wrapper charts using `{{ include "library.deployment" }}`
**Criteria** (FROZEN):
- [ ] All 4 subtasks completed
- [ ] Each service gets thin wrapper chart
- [ ] Wrapper only has Chart.yaml (dependency) + values.yaml
- [ ] Templates use `{{ include "library.<kind>" . }}`
- [ ] Test coverage ≥80%

**Subtasks** (atomic):

1. **Write test: Wrapper Chart.yaml with library dependency**
   - Expected: `dependencies: [{name: library, version, repository}]`
   - Lines: ~40

2. **Write test: Wrapper template calls library**
   - Expected: `{{ include "library.deployment" (dict "context" . "values" .Values) }}`
   - Lines: ~50

3. **Write test: Wrapper values are flat**
   - Expected: `replicaCount: 3`, not `services.frontend.replicaCount: 3`
   - Lines: ~35

4. **Implement wrapper chart generation**
   - Extend LibraryGenerator to produce wrappers
   - Lines: ~200

**Time Estimate**:
- **Solo**: 10-12h
- **AI-assisted**: 1.0-1.2h

**Dependencies**: Task 2.1

---

### Task 2.3: DRY Named Templates

**Goal**: Ensure library templates eliminate all boilerplate duplication
**Result**: Named templates for common blocks (resources, securityContext, probes, env)
**Criteria** (FROZEN):
- [ ] All 3 subtasks completed
- [ ] Common blocks extracted: resources, securityContext, probes, env, volumeMounts
- [ ] No boilerplate duplication across resource types
- [ ] Test coverage ≥80%

**Subtasks** (atomic):

1. **Write test: Shared resources block**
   - Expected: `{{- define "library.resources" -}}` reused in deployment, statefulset, daemonset
   - Lines: ~40

2. **Write test: Shared probes block**
   - Expected: `{{- define "library.probes" -}}` with liveness, readiness, startup
   - Lines: ~40

3. **Implement DRY templates**
   - Extract common blocks from each resource template
   - Lines: ~200

**Time Estimate**:
- **Solo**: 8-10h
- **AI-assisted**: 0.8-1.0h

**Dependencies**: Task 2.1

---

### Task 2.4: Integration Tests — Library Generator

**Goal**: End-to-end tests for Library generation mode
**Result**: `tests/integration/pipeline_library_test.go`
**Criteria** (FROZEN):
- [ ] All 3 subtasks completed
- [ ] Full pipeline test: YAML → Library + Wrapper charts
- [ ] Wrapper charts use library templates correctly
- [ ] All tests PASS

**Subtasks** (atomic):

1. **Write test: Library + 2 wrappers**
   - Input: Frontend + Backend
   - Expected: 1 library chart + 2 wrapper charts
   - Lines: ~80

2. **Write test: Library template invocation**
   - Verify wrapper templates call library includes
   - Lines: ~60

3. **Write test: Helm lint all charts**
   - Lines: ~40

**Time Estimate**:
- **Solo**: 6-8h
- **AI-assisted**: 0.6-0.8h

**Dependencies**: Tasks 2.1-2.3

---

## Week 5: Umbrella Generator (Days 21-25)

### Task 3.1: Parent Chart with Dependencies

**Goal**: Generate umbrella (parent) chart containing all subcharts as dependencies
**Result**: `pkg/generator/umbrella.go` implementing `Generator` interface
**Criteria** (FROZEN):
- [ ] All 4 subtasks completed
- [ ] Parent Chart.yaml with `dependencies[]` listing all services
- [ ] `charts/` directory with subcharts
- [ ] Parent values.yaml with per-subchart sections
- [ ] Test coverage ≥80%

**Subtasks** (atomic):

1. **Write test: Parent Chart.yaml structure**
   - Expected: `dependencies:` array with name, version, condition for each service
   - Lines: ~50

2. **Write test: Subchart directories**
   - Expected: `charts/frontend/`, `charts/backend/`, each with full chart structure
   - Lines: ~50

3. **Write test: Cascading values**
   - Expected: Parent values with `frontend: {replicaCount: 3}`, `backend: {replicaCount: 2}`
   - Lines: ~40

4. **Implement UmbrellaGenerator**
   - File: `pkg/generator/umbrella.go`
   - Uses grouping algorithm (Task 1.1) for subcharts
   - Register in `DefaultRegistry()`
   - Lines: ~280

**Time Estimate**:
- **Solo**: 12-14h
- **AI-assisted**: 1.2-1.4h

**Dependencies**: Task 1.1 (grouping)

---

### Task 3.2: Conditional Subcharts

**Goal**: Support enabling/disabling subcharts via values
**Result**: `condition` field in dependencies + subchart.enabled pattern
**Criteria** (FROZEN):
- [ ] All 3 subtasks completed
- [ ] Each subchart has `condition: <name>.enabled`
- [ ] Default: all subcharts enabled
- [ ] Disabling a subchart excludes its resources from rendering

**Subtasks** (atomic):

1. **Write test: Condition field in dependencies**
   - Expected: `condition: frontend.enabled` in Chart.yaml dependency
   - Lines: ~35

2. **Write test: Default enabled values**
   - Expected: `frontend: {enabled: true}` in parent values
   - Lines: ~30

3. **Implement conditional subchart support**
   - Add condition field to dependency generation
   - Add enabled flag to parent values
   - Lines: ~80

**Time Estimate**:
- **Solo**: 4-6h
- **AI-assisted**: 0.4-0.6h

**Dependencies**: Task 3.1

---

### Task 3.3: Integration Tests — Umbrella Generator

**Goal**: End-to-end tests for Umbrella generation mode
**Result**: `tests/integration/pipeline_umbrella_test.go`
**Criteria** (FROZEN):
- [ ] All 3 subtasks completed
- [ ] Full pipeline: YAML → Umbrella chart with subcharts
- [ ] Conditional subcharts validated
- [ ] All tests PASS

**Subtasks** (atomic):

1. **Write test: Umbrella with 3 subcharts**
   - Input: Frontend + Backend + Database
   - Expected: 1 parent chart, 3 subcharts in `charts/`
   - Lines: ~100

2. **Write test: Conditional subchart disabling**
   - Values: `database.enabled: false`
   - Expected: Database templates not rendered
   - Lines: ~60

3. **Write test: Helm lint umbrella chart**
   - Lines: ~40

**Time Estimate**:
- **Solo**: 6-8h
- **AI-assisted**: 0.6-0.8h

**Dependencies**: Tasks 3.1-3.2

---

## Week 6: Environment Values + Polish (Days 26-30)

### Task 4.1: Environment-Specific Values

**Goal**: Generate environment-specific values files (dev, staging, prod)
**Result**: `values-dev.yaml`, `values-staging.yaml`, `values-prod.yaml`
**Criteria** (FROZEN):
- [ ] All 5 subtasks completed
- [ ] Three environment profiles: dev, staging, prod
- [ ] Dev: 1 replica, debug logging, no PDB, relaxed resources
- [ ] Staging: 2 replicas, info logging, optional PDB
- [ ] Prod: 3+ replicas, warn logging, PDB, full resource limits, node affinity
- [ ] CLI flag: `--env-values` to enable generation

**Subtasks** (atomic):

1. **Write test: Dev values profile**
   - Expected: `replicaCount: 1`, `logLevel: debug`, no PDB
   - Lines: ~40

2. **Write test: Staging values profile**
   - Expected: `replicaCount: 2`, `logLevel: info`
   - Lines: ~40

3. **Write test: Prod values profile**
   - Expected: `replicaCount: 3`, `logLevel: warn`, PDB, resource limits
   - Lines: ~50

4. **Write test: Environment detection from resources**
   - Input: Deployment with 5 replicas → prod-like; 1 replica → dev-like
   - Expected: Base values use original, env overrides differ
   - Lines: ~40

5. **Implement environment values generator**
   - File: `pkg/generator/envvalues.go`
   - Lines: ~200

**Time Estimate**:
- **Solo**: 10-12h
- **AI-assisted**: 1.0-1.2h

**Dependencies**: None (works with any generator mode)

---

### Task 4.2: Documentation & Release Prep

**Goal**: Update documentation for v0.3.0 features
**Result**: README, CHANGELOG, examples updated
**Criteria** (FROZEN):
- [ ] README documents all 3 output modes with usage examples
- [ ] CHANGELOG.md has v0.3.0 section
- [ ] examples/ has mode-specific examples
- [ ] Coverage badge updated

**Time Estimate**:
- **Solo**: 4-6h
- **AI-assisted**: 0.4-0.6h

**Dependencies**: All implementation tasks

---

### Task 4.3: Release v0.3.0

**Goal**: Tag and release v0.3.0
**Result**: Git tag, release notes
**Criteria** (FROZEN):
- [ ] Git tag `v0.3.0` created
- [ ] All tests pass
- [ ] CHANGELOG updated
- [ ] Benchmark comparison with v0.2.0

**Time Estimate**:
- **Solo**: 2-4h
- **AI-assisted**: 0.2-0.4h

**Dependencies**: All tasks

---

## v0.3.0 Summary

### Total Tasks: 16

| Week | Tasks | Theme |
|------|-------|-------|
| 1-2 | 1.1-1.5 | Separate Generator |
| 3-4 | 2.1-2.4 | Library Generator |
| 5 | 3.1-3.3 | Umbrella Generator |
| 6 | 4.1-4.3 | Env Values + Polish |

### Total Time Estimates

| Mode | Estimated | Adjusted (velocity) |
|------|-----------|-------------------|
| Solo (human) | 134-162h (16.75-20.25 days) | — |
| AI-assisted | 11.4-14.0h | ~12h expected |
| AI sessions | 2-3 sessions | Based on v0.2.0 pattern |

### Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| Helm chart dependency resolution complexity | Medium | High | Prototype early, test with real Helm CLI |
| Library template parameterization design | Medium | Medium | Study existing library charts (bitnami/common) |
| Cross-chart value propagation | Low | Medium | Leverage Helm's built-in global values |
| Regression in existing tests | Low | Low | Full test suite runs after each task |

### Key Design Decisions (to be resolved in implementation)

1. **Shared resources** (e.g., ConfigMap used by 2 services): Duplicate to each chart or place in parent?
2. **Library template contract**: Use `dict` pattern or `include` with context?
3. **Umbrella vs Separate**: When same resource appears in multiple groups, which mode handles it better?

---

## Future Releases

| Version | Scope | Estimated (AI) |
|---------|-------|---------------|
| v0.3.5 | Architecture-specific generation (2.4), Namespace management (2.5) | ~10h |
| v0.4.0 | Deckhouse CRD processors, module structure, lib-helm integration | ~15h |
| v0.5.0 | Cluster & GitOps extractors, multi-source merge | ~12h |
| v0.6.0 | Auto-fix engine, advanced analysis, CRD detection | ~20h |
| v1.0.0 | GA: polish, documentation, performance optimization | ~8h |
