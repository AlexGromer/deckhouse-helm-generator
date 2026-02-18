# Deckhouse Helm Generator — Implementation Plan v3.0

**Version**: 3.0.0 (Goal-Result-Criteria Framework)
**Created**: 2026-02-18
**Completed**: 2026-02-18
**Status**: ✅ COMPLETED — all 15 tasks, 131 subtasks, git tag v0.3.0
**Methodology**: TDD + Atomic Decomposition + Velocity-Calibrated Estimation
**Predecessor**: IMPLEMENTATION_PLAN_v2.md (v0.2.0 — COMPLETED 2026-02-18)
**Velocity**: 28.5x (solo 141h / actual ~4.95h) — see docs/velocity_v0.3.0.md

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

### Time Tracking Protocol

**Механизм**: Каждая задача трекается через timestamps в `docs/velocity_v0.3.0.md`.

**Процедура для каждой задачи:**
1. **START**: Перед первой подзадачей записать `start_time` (UTC timestamp)
2. **END**: После финального "Run tests -> Verify coverage" записать `end_time`
3. **DURATION**: `end_time - start_time` = actual time
4. **VELOCITY**: `solo_estimate_midpoint / actual_time`
5. **RECORD**: Добавить строку в таблицу `docs/velocity_v0.3.0.md`

**Формат записи в velocity файле:**
```
| Task | Est (solo) | Est (AI) | Actual | Velocity | Start | End | Notes |
```

**Running average**: После каждой задачи пересчитывать `avg_velocity` и `remaining_estimate`:
```
remaining_estimate = sum(remaining_solo_estimates) / avg_velocity
```

**Автоматизация**: В начале каждой задачи — вывести `[TIMER START] Task X.Y — <timestamp>`.
В конце — `[TIMER END] Task X.Y — <timestamp> — Duration: Xm`.

---

## v0.3.0 — Complete Output Modes (Q3 2026)

**Release Goal**: Implement all three Helm chart generation modes + environment-aware configuration
**Release Result**: v0.3.0 with Separate, Library, and Umbrella generators + env-specific values
**Release Criteria** (FROZEN):
- [x] All three output modes functional (`--mode separate|library|umbrella`)
- [x] Separate Generator: per-service charts with inter-chart dependencies
- [x] Library Generator: base library + thin wrapper charts
- [x] Umbrella Generator: parent chart with conditional subcharts
- [x] Environment-specific values generation (`values-{env}.yaml`)
- [x] Test coverage >=80% for all new code
- [x] All existing tests still pass (regression)
- [x] Documentation updated (README, CHANGELOG)
- [x] Performance: <10s for 100 resources in any mode

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
- [x] All 9 subtasks completed
- [x] Groups resources by `app.kubernetes.io/name` label (primary)
- [x] Falls back to namespace grouping when labels absent
- [x] Connected component detection from relationship graph
- [x] Handles orphan resources (no labels, no relationships)
- [x] Test coverage >=80%
- [x] All tests PASS

**Subtasks** (atomic):

1. **Write test: Group by app.kubernetes.io/name label (single app)**
   - Test case: `TestGroupResources_ByLabel_SingleApp`
     - Input: 3 resources (Deployment+Service+ConfigMap) all with label `app.kubernetes.io/name: myapp`
     - Expected: 1 group named "myapp" with 3 resources
   - Lines: ~40

2. **Write test: Group by app.kubernetes.io/name label (multiple apps)**
   - Test case: `TestGroupResources_ByLabel_MultipleApps`
     - Input: 6 resources, 2 apps ("frontend": Deployment+Service+Ingress, "backend": Deployment+Service+ConfigMap)
     - Expected: 2 groups, 3 resources each, group names = "frontend", "backend"
   - Test case: `TestGroupResources_ByLabel_AppWithSingleResource`
     - Input: 1 Deployment with label `app.kubernetes.io/name: worker`
     - Expected: 1 group "worker" with 1 resource
   - Lines: ~60

3. **Write test: Group by namespace when labels absent**
   - Test case: `TestGroupResources_ByNamespace_NoLabels`
     - Input: 4 resources across 2 namespaces (ns-a: Deployment+Service, ns-b: ConfigMap+Secret), no standard labels
     - Expected: 2 groups named "ns-a" and "ns-b"
   - Test case: `TestGroupResources_ByNamespace_DefaultNamespace`
     - Input: 2 resources in "default" namespace, no labels
     - Expected: 1 group named "default"
   - Lines: ~50

4. **Write test: Connected components from relationship graph**
   - Test case: `TestGroupResources_ByRelationship_ConnectedComponents`
     - Input: 5 resources with relationships forming 2 connected components:
       - Component 1: Deployment-A → Service-A → Ingress-A (label selector + name reference)
       - Component 2: Deployment-B → ConfigMap-B (volume mount)
     - Expected: 2 groups, first with 3 resources, second with 2
   - Test case: `TestGroupResources_ByRelationship_ChainedDependencies`
     - Input: 4 resources: Ingress → Service → Deployment → Secret (chain)
     - Expected: 1 group with all 4 resources (transitive closure)
   - Lines: ~70

5. **Write test: Strategy priority (label > relationship > namespace > individual)**
   - Test case: `TestGroupResources_StrategyPriority_LabelOverNamespace`
     - Input: 3 resources in same namespace but different app labels
     - Expected: Grouped by label, NOT by namespace
   - Test case: `TestGroupResources_StrategyPriority_RelationshipOverNamespace`
     - Input: 2 resources in different namespaces but connected by relationship
     - Expected: 1 group (relationship wins)
   - Lines: ~50

6. **Write test: Edge cases**
   - Test case: `TestGroupResources_Edge_SingleResource`
     - Input: 1 Deployment, no labels, no relationships
     - Expected: 1 group with 1 resource
   - Test case: `TestGroupResources_Edge_EmptyInput`
     - Input: empty resource list
     - Expected: 0 groups, no error
   - Test case: `TestGroupResources_Edge_OrphanResources`
     - Input: 3 resources: 2 with label "myapp", 1 orphan (no labels, no relationships)
     - Expected: 2 groups: "myapp" (2 resources), orphan in its own group
   - Test case: `TestGroupResources_Edge_ConflictingLabels`
     - Input: Resource with `app.kubernetes.io/name: A` and `app: B`
     - Expected: Grouped by `app.kubernetes.io/name` (standard label priority)
   - Lines: ~60

7. **Write test: Group naming**
   - Test case: `TestGroupResources_GroupNaming_FromLabel`
     - Expected: Group name = value of `app.kubernetes.io/name` label
   - Test case: `TestGroupResources_GroupNaming_FromNamespace`
     - Expected: Group name = namespace
   - Test case: `TestGroupResources_GroupNaming_FromResourceName`
     - Expected: For orphans, group name = first resource's name
   - Lines: ~40

8. **Implement grouping algorithm**
   - File: `pkg/generator/grouping.go`
   - Types: `ServiceGroup`, `GroupingStrategy`, `GroupingResult`
   - Functions: `GroupResources(graph *types.ResourceGraph) (*GroupingResult, error)`
   - Strategy priority: label > relationship > namespace > individual
   - Uses `types.ResourceGraph` for relationship traversal
   - Connected components via BFS/DFS on relationship graph
   - Lines: ~250

9. **Run tests -> Fix code -> Verify coverage**
   - Step 1: Run `go test -v ./pkg/generator/ -run TestGroup`
   - Step 2: Analyze failures (expected at this stage)
   - Step 3: Fix `grouping.go` to pass tests (NOT change tests)
   - Step 4: Verify coverage: `go test -cover ./pkg/generator/` >= 80%
   - Step 5: Re-run tests -> all PASS

**Time Estimate**:
- **Solo**: 10-12h (1.25-1.5 days)
  - Subtasks 1-7 (write tests): 7-8h
  - Subtask 8 (implement): 2-3h
  - Subtask 9 (fix/verify): 1h
- **AI-assisted**: 1.0-1.2h

**TDD Workflow**:
1. Write ALL tests FIRST (subtasks 1-7) — defines contract
2. Run tests -> expect FAILURES (grouping.go doesn't exist yet)
3. Implement `grouping.go` (subtask 8) -> tests PASS
4. Verify coverage >=80%
5. NO test changes after initial write (tests are frozen contract)

**Velocity tracking**: Record actual time in `docs/velocity_v0.3.0.md`
**Dependencies**: None (uses existing types.ResourceGraph)
**Blocks**: Tasks 1.2, 1.3, 1.4, 1.5, 3.1

---

### Task 1.2: Separate Generator — Per-Service Chart Generation

**Goal**: Generate independent Helm chart for each service group
**Result**: `pkg/generator/separate.go` implementing `Generator` interface
**Criteria** (FROZEN):
- [x] All 10 subtasks completed
- [x] Each service group produces a complete Helm chart (Chart.yaml, values.yaml, templates/)
- [x] Chart name derived from service group name
- [x] Templates only include resources for that service
- [x] Values scoped to single service (no nesting by service name)
- [x] Implements Generator interface: `Generate()` and `Mode()` methods
- [x] Registered in `DefaultRegistry()` for `OutputModeSeparate`
- [x] Test coverage >=80%

**Subtasks** (atomic):

1. **Write test: SeparateGenerator implements Generator interface**
   - Test case: `TestSeparateGenerator_ImplementsInterface`
     - Expected: `var _ Generator = (*SeparateGenerator)(nil)` compiles
   - Test case: `TestSeparateGenerator_Mode`
     - Expected: `Mode() == types.OutputModeSeparate`
   - Lines: ~25

2. **Write test: Single service chart generation**
   - Test case: `TestSeparateGenerator_SingleGroup_ChartStructure`
     - Input: 1 group "myapp" (Deployment + Service)
     - Expected: 1 GeneratedChart with Chart.yaml, values.yaml, templates/deployment.yaml, templates/service.yaml
   - Test case: `TestSeparateGenerator_SingleGroup_ChartName`
     - Input: Group named "frontend"
     - Expected: Chart.yaml `name: frontend`
   - Lines: ~60

3. **Write test: Multiple service charts**
   - Test case: `TestSeparateGenerator_MultipleGroups_ChartCount`
     - Input: 3 groups (frontend: Deployment+Service+Ingress, backend: Deployment+Service+ConfigMap, database: StatefulSet+Service+PVC)
     - Expected: 3 GeneratedCharts
   - Test case: `TestSeparateGenerator_MultipleGroups_TemplateIsolation`
     - Input: Same as above
     - Expected: Frontend chart has 3 templates, backend has 3, database has 3
   - Lines: ~80

4. **Write test: Chart.yaml per service**
   - Test case: `TestSeparateGenerator_ChartYAML_RequiredFields`
     - Expected: apiVersion: v2, name, version: "0.1.0", type: application
   - Test case: `TestSeparateGenerator_ChartYAML_Description`
     - Expected: Description auto-generated from service name
   - Lines: ~40

5. **Write test: Values scoped to service (flat, no service prefix)**
   - Test case: `TestSeparateGenerator_Values_FlatStructure`
     - Input: Group with Deployment (replicas:3, image:nginx:1.21)
     - Expected: `values["replicaCount"] == 3`, NOT `values["frontend"]["replicaCount"]`
   - Test case: `TestSeparateGenerator_Values_NoServiceNesting`
     - Expected: No service-name-prefixed keys in values
   - Lines: ~50

6. **Write test: _helpers.tpl per service**
   - Test case: `TestSeparateGenerator_Helpers_Fullname`
     - Expected: `_helpers.tpl` contains `{{- define "<chart>.fullname" -}}`
   - Test case: `TestSeparateGenerator_Helpers_Labels`
     - Expected: `_helpers.tpl` contains `{{- define "<chart>.labels" -}}`
   - Lines: ~40

7. **Write test: Template content correctness**
   - Test case: `TestSeparateGenerator_Templates_DeploymentContent`
     - Input: Group with Deployment
     - Expected: Generated template references `.Values.replicaCount`, `.Values.image`
   - Test case: `TestSeparateGenerator_Templates_ServiceContent`
     - Input: Group with Service
     - Expected: Generated template references `.Values.service.type`, `.Values.service.ports`
   - Lines: ~50

8. **Write test: Edge cases**
   - Test case: `TestSeparateGenerator_Edge_EmptyGraph`
     - Input: Empty resource graph
     - Expected: 0 charts, no error
   - Test case: `TestSeparateGenerator_Edge_SingleResourceGroup`
     - Input: 1 group with 1 ConfigMap
     - Expected: 1 chart with 1 template
   - Lines: ~40

9. **Implement SeparateGenerator**
   - File: `pkg/generator/separate.go`
   - Struct: `SeparateGenerator` embedding `BaseGenerator`
   - Constructor: `NewSeparateGenerator() *SeparateGenerator`
   - Method: `Generate(ctx, graph, opts) -> []*types.GeneratedChart`
   - Uses `GroupResources()` from Task 1.1 for service isolation
   - Reuses processor templates from UniversalGenerator pattern
   - Register in `DefaultRegistry()`: add `r.Register(NewSeparateGenerator())`
   - Lines: ~300

10. **Run tests -> Fix code -> Verify coverage**
    - Step 1: Run `go test -v ./pkg/generator/ -run TestSeparateGenerator`
    - Step 2: Analyze failures
    - Step 3: Fix `separate.go` to pass tests (NOT change tests)
    - Step 4: Verify coverage >=80%
    - Step 5: Re-run all generator tests (regression): `go test ./pkg/generator/...`

**Time Estimate**:
- **Solo**: 12-14h (1.5-1.75 days)
  - Subtasks 1-8 (write tests): 8-10h
  - Subtask 9 (implement): 3-4h
  - Subtask 10 (fix/verify): 1h
- **AI-assisted**: 1.2-1.4h

**TDD Workflow**:
1. Write ALL tests FIRST (subtasks 1-8) — defines contract
2. Run tests -> expect FAILURES (separate.go doesn't exist yet)
3. Implement `separate.go` (subtask 9) -> tests PASS
4. Verify coverage >=80%
5. NO test changes after initial write (tests are frozen contract)

**Velocity tracking**: Record actual time in `docs/velocity_v0.3.0.md`
**Dependencies**: Task 1.1 (grouping algorithm)
**Blocks**: Tasks 1.3, 1.4, 1.5

---

### Task 1.3: Inter-Chart Dependencies

**Goal**: Generate `Chart.yaml` dependencies between related service charts
**Result**: Dependency detection from relationship graph + Chart.yaml generation with dependencies section
**Criteria** (FROZEN):
- [x] All 8 subtasks completed
- [x] Cross-service relationships -> Chart.yaml dependencies
- [x] `file://` repository for local subcharts
- [x] Condition field for optional dependencies
- [x] Circular dependency detection with error
- [x] Test coverage >=80%

**Subtasks** (atomic):

1. **Write test: Detect cross-chart dependencies from relationships**
   - Test case: `TestInterChartDeps_DetectCrossChart_ServiceToDeployment`
     - Input: Frontend group (Ingress) -> Backend group (Service) via relationship
     - Expected: Frontend has dependency on backend
   - Test case: `TestInterChartDeps_DetectCrossChart_MultipleRelationships`
     - Input: Frontend -> Backend, Backend -> Database
     - Expected: Frontend depends on backend; backend depends on database
   - Lines: ~60

2. **Write test: file:// repository references**
   - Test case: `TestInterChartDeps_FileRepository_LocalPath`
     - Expected: `repository: file://../backend` in dependency spec
   - Test case: `TestInterChartDeps_FileRepository_RelativeToChart`
     - Input: Frontend at `output/frontend/`, backend at `output/backend/`
     - Expected: Relative path from frontend to backend
   - Lines: ~40

3. **Write test: Condition field in dependencies**
   - Test case: `TestInterChartDeps_Condition_DefaultPattern`
     - Expected: `condition: backend.enabled` in dependency
   - Test case: `TestInterChartDeps_Condition_PresenceInValues`
     - Expected: `backend: { enabled: true }` in parent values
   - Lines: ~40

4. **Write test: Circular dependency detection**
   - Test case: `TestInterChartDeps_CircularDependency_Detection`
     - Input: A -> B -> C -> A
     - Expected: Error returned containing "circular dependency"
   - Test case: `TestInterChartDeps_CircularDependency_SelfReference`
     - Input: A -> A
     - Expected: Error
   - Lines: ~45

5. **Write test: No cross-chart dependencies (independent charts)**
   - Test case: `TestInterChartDeps_NoCrossDeps_IndependentCharts`
     - Input: 2 groups with no cross-group relationships
     - Expected: No dependencies in either Chart.yaml
   - Lines: ~30

6. **Write test: Edge cases**
   - Test case: `TestInterChartDeps_Edge_SingleChart`
     - Input: 1 group
     - Expected: No dependencies section
   - Test case: `TestInterChartDeps_Edge_EmptyGroups`
     - Input: 0 groups
     - Expected: No error
   - Lines: ~30

7. **Implement inter-chart dependency generation**
   - File: extend `pkg/generator/separate.go` (or `pkg/generator/dependencies.go`)
   - Functions: `DetectCrossChartDeps(groups, graph)`, `GenerateDepSection(deps)`
   - Circular dependency detection via DFS with visited set
   - Lines: ~150

8. **Run tests -> Fix code -> Verify coverage**
   - Step 1: Run `go test -v ./pkg/generator/ -run TestInterChartDeps`
   - Step 2: Fix code to pass tests
   - Step 3: Verify coverage >=80%
   - Step 4: Regression: `go test ./pkg/generator/...`

**Time Estimate**:
- **Solo**: 8-10h (1-1.25 days)
  - Subtasks 1-6 (write tests): 5-6h
  - Subtask 7 (implement): 2-3h
  - Subtask 8 (fix/verify): 1h
- **AI-assisted**: 0.8-1.0h

**TDD Workflow**:
1. Write ALL tests FIRST (subtasks 1-6) — defines contract
2. Run tests -> expect FAILURES
3. Implement dependency detection (subtask 7) -> tests PASS
4. Verify coverage >=80%
5. NO test changes after initial write

**Velocity tracking**: Record actual time in `docs/velocity_v0.3.0.md`
**Dependencies**: Task 1.2 (SeparateGenerator exists)
**Blocks**: Task 1.5

---

### Task 1.4: Shared Values (Global)

**Goal**: Generate parent values.yaml with global settings shared across service charts
**Result**: Global values propagation to child charts via `.Values.global.*`
**Criteria** (FROZEN):
- [x] All 8 subtasks completed
- [x] Global values extracted (image registry, environment, common labels)
- [x] Child charts reference `{{ .Values.global.imageRegistry }}`
- [x] Parent values.yaml has global section
- [x] Only truly shared values promoted to global (>=2 services)
- [x] Test coverage >=80%

**Subtasks** (atomic):

1. **Write test: Extract common image registry**
   - Test case: `TestGlobalValues_ExtractCommon_ImageRegistry`
     - Input: 3 services all using `registry.example.com/` prefix
     - Expected: `global.imageRegistry: registry.example.com` in parent values
   - Test case: `TestGlobalValues_ExtractCommon_NoCommonRegistry`
     - Input: Services using different registries
     - Expected: No `global.imageRegistry`
   - Lines: ~50

2. **Write test: Extract common environment variables**
   - Test case: `TestGlobalValues_ExtractCommon_EnvVars`
     - Input: All services have env `LOG_LEVEL=info` and `ENV=production`
     - Expected: `global.env.LOG_LEVEL: info`, `global.env.ENV: production`
   - Test case: `TestGlobalValues_ExtractCommon_PartialOverlap`
     - Input: 2 of 3 services share `LOG_LEVEL`, only 1 has `DEBUG=true`
     - Expected: `LOG_LEVEL` in global (>=2 services), `DEBUG` stays local
   - Lines: ~60

3. **Write test: Extract common labels**
   - Test case: `TestGlobalValues_ExtractCommon_Labels`
     - Input: All services have `team: platform`, `environment: prod`
     - Expected: `global.labels.team: platform`, `global.labels.environment: prod`
   - Lines: ~40

4. **Write test: Child chart references global values**
   - Test case: `TestGlobalValues_ChildTemplate_UsesGlobal`
     - Expected: Deployment template has `image: {{ .Values.global.imageRegistry }}/{{ .Values.image.repository }}`
   - Test case: `TestGlobalValues_ChildTemplate_FallbackToLocal`
     - Expected: Template uses `{{ .Values.global.imageRegistry | default .Values.image.registry }}`
   - Lines: ~50

5. **Write test: Parent values.yaml structure**
   - Test case: `TestGlobalValues_ParentValues_Structure`
     - Expected: Top-level `global:` section, then per-service sections
   - Test case: `TestGlobalValues_ParentValues_GlobalAtTop`
     - Expected: `global` is first key in parent values
   - Lines: ~35

6. **Write test: Edge cases**
   - Test case: `TestGlobalValues_Edge_NoCommonValues`
     - Input: All services have completely different configurations
     - Expected: Empty `global:` section or no global section
   - Test case: `TestGlobalValues_Edge_SingleService`
     - Input: 1 service
     - Expected: No global extraction (nothing to share)
   - Lines: ~35

7. **Implement global values extraction**
   - File: `pkg/generator/globalvalues.go`
   - Functions: `ExtractGlobalValues(groups []*ServiceGroup) map[string]interface{}`
   - Logic: Find values present in >=2 groups, promote to global
   - Modify child templates to reference global values
   - Lines: ~200

8. **Run tests -> Fix code -> Verify coverage**
   - Step 1: Run `go test -v ./pkg/generator/ -run TestGlobalValues`
   - Step 2: Fix code to pass tests
   - Step 3: Verify coverage >=80%
   - Step 4: Regression: `go test ./pkg/generator/...`

**Time Estimate**:
- **Solo**: 8-10h (1-1.25 days)
  - Subtasks 1-6 (write tests): 5-6h
  - Subtask 7 (implement): 2-3h
  - Subtask 8 (fix/verify): 1h
- **AI-assisted**: 0.8-1.0h

**TDD Workflow**:
1. Write ALL tests FIRST (subtasks 1-6) — defines contract
2. Run tests -> expect FAILURES
3. Implement global values (subtask 7) -> tests PASS
4. Verify coverage >=80%
5. NO test changes after initial write

**Velocity tracking**: Record actual time in `docs/velocity_v0.3.0.md`
**Dependencies**: Task 1.2 (SeparateGenerator)
**Blocks**: Task 1.5

---

### Task 1.5: Integration Tests — Separate Generator

**Goal**: End-to-end tests for Separate generation mode through the full pipeline
**Result**: `tests/integration/pipeline_separate_test.go`
**Criteria** (FROZEN):
- [x] All 8 subtasks completed
- [x] Full pipeline test: YAML -> Separate charts
- [x] Multi-service scenarios validated
- [x] Inter-chart dependencies verified
- [x] Global values propagated
- [x] Generated charts pass `helm lint`
- [x] All tests PASS

**Subtasks** (atomic):

1. **Write test: Simple 2-service separation**
   - Test case: `TestPipelineSeparate_TwoServices`
     - Input: YAML fixtures: Frontend (Deployment+Service+Ingress) + Backend (Deployment+Service+ConfigMap)
     - Expected: 2 chart directories created, each with Chart.yaml + values.yaml + templates/
   - Lines: ~80

2. **Write test: 3-tier app with dependencies**
   - Test case: `TestPipelineSeparate_ThreeTierApp`
     - Input: Frontend -> Backend -> Database (Service references)
     - Expected: 3 charts, dependency chain in Chart.yaml: frontend depends on backend, backend depends on database
   - Lines: ~100

3. **Write test: Shared ConfigMap across services**
   - Test case: `TestPipelineSeparate_SharedConfigMap`
     - Input: 2 services both mounting same ConfigMap
     - Expected: ConfigMap placed in one chart (or duplicated with warning)
   - Lines: ~70

4. **Write test: Global values in parent**
   - Test case: `TestPipelineSeparate_GlobalValues`
     - Input: 3 services with common image registry
     - Expected: Parent values.yaml with `global.imageRegistry`, child templates reference global
   - Lines: ~60

5. **Write test: Single-service input (degenerate case)**
   - Test case: `TestPipelineSeparate_SingleService`
     - Input: 1 Deployment + 1 Service
     - Expected: 1 chart, no dependencies, no global values
   - Lines: ~50

6. **Write test: All 18 resource types distributed across services**
   - Test case: `TestPipelineSeparate_AllResourceTypes`
     - Input: Full-stack with Deployment, StatefulSet, Service, Ingress, ConfigMap, Secret, PVC, HPA, PDB, NetworkPolicy, CronJob, Job, RBAC resources
     - Expected: Resources correctly distributed to service groups
   - Lines: ~120

7. **Write test: Helm lint all generated charts**
   - Test case: `TestPipelineSeparate_HelmLint`
     - Run `helm lint` on each generated chart
     - Expected: 0 errors, 0 warnings (or expected warnings only)
   - Lines: ~40

8. **Write test: Regression — Universal mode still works**
   - Test case: `TestPipelineSeparate_UniversalRegression`
     - Run same input through universal mode
     - Expected: Still produces valid single chart (no regression)
   - Lines: ~40

**Time Estimate**:
- **Solo**: 8-10h (1-1.25 days)
  - Subtasks 1-8 (write tests): 7-9h
  - Fixing issues: 1h
- **AI-assisted**: 0.8-1.0h

**TDD Workflow**:
1. Write ALL integration tests FIRST (subtasks 1-8)
2. Run tests -> expect FAILURES (pipeline doesn't support separate mode yet)
3. Iterate on generator code until all integration tests PASS
4. No test changes — tests define the contract

**Velocity tracking**: Record actual time in `docs/velocity_v0.3.0.md`
**Dependencies**: Tasks 1.1-1.4 (all Separate Generator components)
**Blocks**: Task 4.2 (documentation)

---

## Week 3-4: Library Generator (Days 11-20)

### Task 2.1: Base Library Chart

**Goal**: Generate a `type: library` Helm chart with reusable named templates
**Result**: `pkg/generator/library.go` producing base library chart
**Criteria** (FROZEN):
- [x] All 11 subtasks completed
- [x] Chart.yaml with `type: library`
- [x] Named templates for all 18 supported resource types
- [x] Templates parameterized via dictionary/context pattern
- [x] Named templates follow convention: `{{- define "library.<kind>" -}}`
- [x] Test coverage >=80%

**Subtasks** (atomic):

1. **Write test: LibraryGenerator implements Generator interface**
   - Test case: `TestLibraryGenerator_ImplementsInterface`
     - Expected: `var _ Generator = (*LibraryGenerator)(nil)` compiles
   - Test case: `TestLibraryGenerator_Mode`
     - Expected: `Mode() == types.OutputModeLibrary`
   - Lines: ~25

2. **Write test: Library Chart.yaml**
   - Test case: `TestLibraryGenerator_ChartYAML_Type`
     - Expected: `type: library` in Chart.yaml
   - Test case: `TestLibraryGenerator_ChartYAML_Fields`
     - Expected: apiVersion: v2, name: "library", version present
   - Lines: ~35

3. **Write test: Named deployment template**
   - Test case: `TestLibraryGenerator_NamedTemplate_Deployment`
     - Expected: `_deployment.tpl` contains `{{- define "library.deployment" -}}`
     - Expected: Template accepts context dict with `.name`, `.replicaCount`, `.image`, `.resources`
   - Lines: ~50

4. **Write test: Named service template**
   - Test case: `TestLibraryGenerator_NamedTemplate_Service`
     - Expected: `_service.tpl` contains `{{- define "library.service" -}}`
     - Expected: Template accepts `.serviceName`, `.type`, `.ports`
   - Lines: ~40

5. **Write test: Named statefulset template**
   - Test case: `TestLibraryGenerator_NamedTemplate_StatefulSet`
     - Expected: `_statefulset.tpl` contains `{{- define "library.statefulset" -}}`
   - Lines: ~35

6. **Write test: Named templates for all remaining resource types**
   - Test case: `TestLibraryGenerator_NamedTemplate_AllTypes`
     - Expected: Templates exist for: daemonset, ingress, configmap, secret, pvc, hpa, pdb, networkpolicy, cronjob, job, serviceaccount, role, clusterrole, rolebinding, clusterrolebinding
     - Verification: Each has `{{- define "library.<kind>" -}}`
   - Lines: ~80

7. **Write test: Template parameterization via dict pattern**
   - Test case: `TestLibraryGenerator_Templates_DictPattern`
     - Expected: Templates use `{{ .context }}` and `{{ .values }}` pattern
     - Expected: Caller passes `(dict "context" . "values" .Values.frontend)`
   - Lines: ~40

8. **Write test: Template content for deployment**
   - Test case: `TestLibraryGenerator_TemplateContent_Deployment_Replicas`
     - Expected: Template generates `replicas: {{ .values.replicaCount }}`
   - Test case: `TestLibraryGenerator_TemplateContent_Deployment_Image`
     - Expected: Template generates `image: {{ .values.image.repository }}:{{ .values.image.tag }}`
   - Lines: ~50

9. **Write test: Edge cases**
   - Test case: `TestLibraryGenerator_Edge_EmptyGraph`
     - Input: Empty resource graph
     - Expected: Library chart with all named templates (they're generic)
   - Test case: `TestLibraryGenerator_Edge_SingleResourceType`
     - Input: Only Deployments
     - Expected: Still generates all named templates (library is generic)
   - Lines: ~35

10. **Implement LibraryGenerator (base chart)**
    - File: `pkg/generator/library.go`
    - Struct: `LibraryGenerator` embedding `BaseGenerator`
    - Constructor: `NewLibraryGenerator() *LibraryGenerator`
    - Method: `Generate(ctx, graph, opts) -> []*types.GeneratedChart`
    - Generate library chart with named templates for all 18 K8s types
    - Register in `DefaultRegistry()`: add `r.Register(NewLibraryGenerator())`
    - Lines: ~400

11. **Run tests -> Fix code -> Verify coverage**
    - Step 1: Run `go test -v ./pkg/generator/ -run TestLibraryGenerator`
    - Step 2: Fix code to pass tests
    - Step 3: Verify coverage >=80%
    - Step 4: Regression: `go test ./pkg/generator/...`

**Time Estimate**:
- **Solo**: 14-16h (1.75-2 days)
  - Subtasks 1-9 (write tests): 9-11h
  - Subtask 10 (implement): 4-5h
  - Subtask 11 (fix/verify): 1h
- **AI-assisted**: 1.4-1.6h

**TDD Workflow**:
1. Write ALL tests FIRST (subtasks 1-9) — defines contract
2. Run tests -> expect FAILURES (library.go doesn't exist yet)
3. Implement `library.go` (subtask 10) -> tests PASS
4. Verify coverage >=80%
5. NO test changes after initial write

**Velocity tracking**: Record actual time in `docs/velocity_v0.3.0.md`
**Dependencies**: None (new generator, independent of Separate)
**Blocks**: Tasks 2.2, 2.3, 2.4

---

### Task 2.2: Wrapper Charts

**Goal**: Generate thin wrapper charts that depend on the library chart
**Result**: Per-service wrapper charts using `{{ include "library.deployment" }}`
**Criteria** (FROZEN):
- [x] All 8 subtasks completed
- [x] Each service gets thin wrapper chart
- [x] Wrapper Chart.yaml has library as dependency
- [x] Wrapper templates use `{{ include "library.<kind>" . }}` pattern
- [x] Wrapper values are flat (no service name prefixing)
- [x] Test coverage >=80%

**Subtasks** (atomic):

1. **Write test: Wrapper Chart.yaml with library dependency**
   - Test case: `TestWrapperChart_ChartYAML_LibraryDependency`
     - Expected: `dependencies: [{name: library, version: "0.1.0", repository: "file://../library"}]`
   - Test case: `TestWrapperChart_ChartYAML_ApplicationType`
     - Expected: `type: application` (not library)
   - Lines: ~45

2. **Write test: Wrapper template calls library include**
   - Test case: `TestWrapperChart_Template_DeploymentInclude`
     - Expected: Template contains `{{ include "library.deployment" (dict "context" . "values" .Values) }}`
   - Test case: `TestWrapperChart_Template_ServiceInclude`
     - Expected: Template contains `{{ include "library.service" (dict "context" . "values" .Values) }}`
   - Lines: ~50

3. **Write test: Wrapper values are flat**
   - Test case: `TestWrapperChart_Values_FlatStructure`
     - Expected: `replicaCount: 3`, NOT `frontend.replicaCount: 3`
   - Test case: `TestWrapperChart_Values_AllFields`
     - Input: Service with Deployment + Service + Ingress
     - Expected: Values contain replicaCount, image, service, ingress sections
   - Lines: ~45

4. **Write test: Multiple wrapper generation**
   - Test case: `TestWrapperChart_MultipleWrappers`
     - Input: 3 services (frontend, backend, database)
     - Expected: 1 library chart + 3 wrapper charts
   - Lines: ~60

5. **Write test: Wrapper _helpers.tpl**
   - Test case: `TestWrapperChart_Helpers_Fullname`
     - Expected: `_helpers.tpl` with chart-specific fullname template
   - Lines: ~30

6. **Write test: Edge cases**
   - Test case: `TestWrapperChart_Edge_SingleWrapper`
     - Input: 1 service
     - Expected: 1 library chart + 1 wrapper chart
   - Test case: `TestWrapperChart_Edge_WrapperWithManyResources`
     - Input: Service with 6 resource types
     - Expected: Wrapper has 6 template files, each calling library include
   - Lines: ~40

7. **Implement wrapper chart generation**
   - Extend `pkg/generator/library.go` (or `pkg/generator/wrapper.go`)
   - Functions: `generateWrapperChart(group, libraryName)`, `generateWrapperTemplate(kind)`
   - Uses grouping algorithm (Task 1.1) for service isolation
   - Lines: ~200

8. **Run tests -> Fix code -> Verify coverage**
   - Step 1: Run `go test -v ./pkg/generator/ -run TestWrapperChart`
   - Step 2: Fix code to pass tests
   - Step 3: Verify coverage >=80%
   - Step 4: Regression: `go test ./pkg/generator/...`

**Time Estimate**:
- **Solo**: 10-12h (1.25-1.5 days)
  - Subtasks 1-6 (write tests): 6-8h
  - Subtask 7 (implement): 3-3h
  - Subtask 8 (fix/verify): 1h
- **AI-assisted**: 1.0-1.2h

**TDD Workflow**:
1. Write ALL tests FIRST (subtasks 1-6) — defines contract
2. Run tests -> expect FAILURES
3. Implement wrapper generation (subtask 7) -> tests PASS
4. Verify coverage >=80%
5. NO test changes after initial write

**Velocity tracking**: Record actual time in `docs/velocity_v0.3.0.md`
**Dependencies**: Task 2.1 (LibraryGenerator base chart)
**Blocks**: Task 2.4

---

### Task 2.3: DRY Named Templates

**Goal**: Ensure library templates eliminate all boilerplate duplication via shared sub-templates
**Result**: Named templates for common blocks (resources, securityContext, probes, env, volumeMounts)
**Criteria** (FROZEN):
- [x] All 8 subtasks completed
- [x] Common blocks extracted: resources, securityContext, probes, env, volumeMounts, labels, annotations
- [x] No boilerplate duplication across resource type templates
- [x] Shared templates reused by deployment, statefulset, daemonset, cronjob, job
- [x] Test coverage >=80%

**Subtasks** (atomic):

1. **Write test: Shared resources block**
   - Test case: `TestDRYTemplates_SharedBlock_Resources`
     - Expected: `{{- define "library.resources" -}}` exists
     - Expected: Deployment, StatefulSet, DaemonSet templates all call `{{ include "library.resources" }}`
   - Lines: ~40

2. **Write test: Shared probes block**
   - Test case: `TestDRYTemplates_SharedBlock_Probes`
     - Expected: `{{- define "library.probes" -}}` with liveness, readiness, startup
     - Expected: Deployment, StatefulSet templates include probes block
   - Lines: ~40

3. **Write test: Shared securityContext block**
   - Test case: `TestDRYTemplates_SharedBlock_SecurityContext`
     - Expected: `{{- define "library.securityContext" -}}` for pod-level
     - Expected: `{{- define "library.containerSecurityContext" -}}` for container-level
   - Lines: ~40

4. **Write test: Shared env block**
   - Test case: `TestDRYTemplates_SharedBlock_Env`
     - Expected: `{{- define "library.env" -}}` for environment variables
     - Expected: Used by all workload types (Deployment, StatefulSet, DaemonSet, Job, CronJob)
   - Lines: ~35

5. **Write test: Shared volumeMounts block**
   - Test case: `TestDRYTemplates_SharedBlock_VolumeMounts`
     - Expected: `{{- define "library.volumeMounts" -}}` and `{{- define "library.volumes" -}}`
   - Lines: ~35

6. **Write test: Shared labels and annotations blocks**
   - Test case: `TestDRYTemplates_SharedBlock_Labels`
     - Expected: `{{- define "library.labels" -}}` and `{{- define "library.annotations" -}}`
   - Lines: ~30

7. **Implement DRY templates**
   - File: `pkg/generator/library.go` (extend generateTemplate methods)
   - New file: `pkg/generator/library_helpers.go` (shared template generation)
   - Extract common blocks from each resource template into shared defines
   - Lines: ~250

8. **Run tests -> Fix code -> Verify coverage**
   - Step 1: Run `go test -v ./pkg/generator/ -run TestDRYTemplates`
   - Step 2: Fix code to pass tests
   - Step 3: Verify coverage >=80%
   - Step 4: Regression: `go test ./pkg/generator/...`

**Time Estimate**:
- **Solo**: 8-10h (1-1.25 days)
  - Subtasks 1-6 (write tests): 5-6h
  - Subtask 7 (implement): 2-3h
  - Subtask 8 (fix/verify): 1h
- **AI-assisted**: 0.8-1.0h

**TDD Workflow**:
1. Write ALL tests FIRST (subtasks 1-6) — defines contract
2. Run tests -> expect FAILURES
3. Implement DRY templates (subtask 7) -> tests PASS
4. Verify coverage >=80%
5. NO test changes after initial write

**Velocity tracking**: Record actual time in `docs/velocity_v0.3.0.md`
**Dependencies**: Task 2.1 (LibraryGenerator)
**Blocks**: Task 2.4

---

### Task 2.4: Integration Tests — Library Generator

**Goal**: End-to-end tests for Library generation mode through the full pipeline
**Result**: `tests/integration/pipeline_library_test.go`
**Criteria** (FROZEN):
- [x] All 7 subtasks completed
- [x] Full pipeline test: YAML -> Library + Wrapper charts
- [x] Wrapper charts use library templates correctly
- [x] DRY templates verified (no duplication)
- [x] All tests PASS

**Subtasks** (atomic):

1. **Write test: Library + 2 wrappers**
   - Test case: `TestPipelineLibrary_TwoServices`
     - Input: Frontend (Deployment+Service+Ingress) + Backend (Deployment+Service+ConfigMap)
     - Expected: 1 library chart + 2 wrapper charts
     - Verify: Library Chart.yaml has `type: library`
   - Lines: ~80

2. **Write test: Library template invocation correctness**
   - Test case: `TestPipelineLibrary_WrapperCallsLibrary`
     - Verify: Each wrapper template file calls `{{ include "library.<kind>" }}`
     - Verify: No inline resource definitions in wrapper templates
   - Lines: ~60

3. **Write test: DRY verification — no duplication**
   - Test case: `TestPipelineLibrary_DRYVerification`
     - Count occurrences of common blocks (resources, probes) across all templates
     - Expected: Each block defined ONCE in library, referenced N times in wrappers
   - Lines: ~50

4. **Write test: Full-stack with all resource types**
   - Test case: `TestPipelineLibrary_AllResourceTypes`
     - Input: All 18 resource types distributed across 3 services
     - Expected: Library has 18 named templates, each wrapper references relevant ones
   - Lines: ~100

5. **Write test: Helm lint all charts**
   - Test case: `TestPipelineLibrary_HelmLint`
     - Run `helm lint` on library chart and each wrapper
     - Expected: 0 errors
   - Lines: ~40

6. **Write test: Regression — Universal and Separate modes still work**
   - Test case: `TestPipelineLibrary_UniversalRegression`
   - Test case: `TestPipelineLibrary_SeparateRegression`
   - Lines: ~50

7. **Create test fixtures for library mode**
   - Files: `tests/integration/fixtures/library-app/`
   - Fixtures: frontend.yaml, backend.yaml, database.yaml, shared-config.yaml
   - Lines: ~200 (YAML)

**Time Estimate**:
- **Solo**: 6-8h (0.75-1 day)
  - Subtasks 1-7 (write tests + fixtures): 6-8h
- **AI-assisted**: 0.6-0.8h

**TDD Workflow**:
1. Write ALL integration tests FIRST (subtasks 1-6)
2. Create fixtures (subtask 7)
3. Run tests -> expect FAILURES
4. Iterate on generator code until all integration tests PASS
5. No test changes

**Velocity tracking**: Record actual time in `docs/velocity_v0.3.0.md`
**Dependencies**: Tasks 2.1-2.3 (all Library Generator components)
**Blocks**: Task 4.2 (documentation)

---

## Week 5: Umbrella Generator (Days 21-25)

### Task 3.1: Parent Chart with Subcharts

**Goal**: Generate umbrella (parent) chart containing all subcharts as dependencies
**Result**: `pkg/generator/umbrella.go` implementing `Generator` interface
**Criteria** (FROZEN):
- [x] All 10 subtasks completed
- [x] Parent Chart.yaml with `dependencies[]` listing all services
- [x] `charts/` directory with subcharts (each with full chart structure)
- [x] Parent values.yaml with per-subchart sections
- [x] Implements Generator interface
- [x] Registered in `DefaultRegistry()`
- [x] Test coverage >=80%

**Subtasks** (atomic):

1. **Write test: UmbrellaGenerator implements Generator interface**
   - Test case: `TestUmbrellaGenerator_ImplementsInterface`
     - Expected: `var _ Generator = (*UmbrellaGenerator)(nil)` compiles
   - Test case: `TestUmbrellaGenerator_Mode`
     - Expected: `Mode() == types.OutputModeUmbrella`
     - Note: Need to add `OutputModeUmbrella` to types if not present
   - Lines: ~25

2. **Write test: Parent Chart.yaml structure**
   - Test case: `TestUmbrellaGenerator_ParentChartYAML_Dependencies`
     - Input: 3 groups (frontend, backend, database)
     - Expected: `dependencies:` array with 3 entries, each with name, version, condition
   - Test case: `TestUmbrellaGenerator_ParentChartYAML_RequiredFields`
     - Expected: apiVersion: v2, name: "umbrella", version, type: application
   - Lines: ~55

3. **Write test: Subchart directories**
   - Test case: `TestUmbrellaGenerator_SubchartDirs_Structure`
     - Input: 3 groups
     - Expected: `charts/frontend/`, `charts/backend/`, `charts/database/` directories
   - Test case: `TestUmbrellaGenerator_SubchartDirs_ChartContent`
     - Expected: Each subchart has Chart.yaml + values.yaml + templates/
   - Lines: ~60

4. **Write test: Cascading values (parent -> subchart)**
   - Test case: `TestUmbrellaGenerator_CascadingValues_PerSubchart`
     - Expected: Parent values with `frontend: {replicaCount: 3}`, `backend: {replicaCount: 2}`
   - Test case: `TestUmbrellaGenerator_CascadingValues_Override`
     - Expected: Parent values override subchart defaults via Helm value cascade
   - Lines: ~50

5. **Write test: Subchart Chart.yaml (child)**
   - Test case: `TestUmbrellaGenerator_SubchartChartYAML`
     - Expected: Each subchart has own Chart.yaml with `type: application`
     - Expected: No dependencies section in subcharts (dependencies handled at parent level)
   - Lines: ~35

6. **Write test: Subchart templates correctness**
   - Test case: `TestUmbrellaGenerator_SubchartTemplates_Content`
     - Input: Frontend group (Deployment+Service)
     - Expected: Subchart has deployment.yaml and service.yaml in templates/
   - Lines: ~40

7. **Write test: Global values in umbrella**
   - Test case: `TestUmbrellaGenerator_GlobalValues`
     - Input: 3 services with common image registry
     - Expected: Parent values has `global:` section
   - Lines: ~40

8. **Write test: Edge cases**
   - Test case: `TestUmbrellaGenerator_Edge_SingleSubchart`
     - Input: 1 group
     - Expected: Parent with 1 subchart in charts/
   - Test case: `TestUmbrellaGenerator_Edge_EmptyGraph`
     - Input: Empty resource graph
     - Expected: Error or empty chart
   - Lines: ~35

9. **Implement UmbrellaGenerator**
   - File: `pkg/generator/umbrella.go`
   - Struct: `UmbrellaGenerator` embedding `BaseGenerator`
   - Constructor: `NewUmbrellaGenerator() *UmbrellaGenerator`
   - Method: `Generate(ctx, graph, opts) -> []*types.GeneratedChart`
   - Add `OutputModeUmbrella OutputMode = "umbrella"` to types if not present
   - Uses `GroupResources()` from Task 1.1 for subchart creation
   - Register in `DefaultRegistry()`: add `r.Register(NewUmbrellaGenerator())`
   - Lines: ~300

10. **Run tests -> Fix code -> Verify coverage**
    - Step 1: Run `go test -v ./pkg/generator/ -run TestUmbrellaGenerator`
    - Step 2: Fix code to pass tests
    - Step 3: Verify coverage >=80%
    - Step 4: Regression: `go test ./pkg/generator/...`

**Time Estimate**:
- **Solo**: 12-14h (1.5-1.75 days)
  - Subtasks 1-8 (write tests): 8-9h
  - Subtask 9 (implement): 3-4h
  - Subtask 10 (fix/verify): 1h
- **AI-assisted**: 1.2-1.4h

**TDD Workflow**:
1. Write ALL tests FIRST (subtasks 1-8) — defines contract
2. Run tests -> expect FAILURES (umbrella.go doesn't exist yet)
3. Implement `umbrella.go` (subtask 9) -> tests PASS
4. Verify coverage >=80%
5. NO test changes after initial write

**Velocity tracking**: Record actual time in `docs/velocity_v0.3.0.md`
**Dependencies**: Task 1.1 (grouping algorithm)
**Blocks**: Tasks 3.2, 3.3

---

### Task 3.2: Conditional Subcharts

**Goal**: Support enabling/disabling individual subcharts via parent values
**Result**: `condition` field in dependencies + `<subchart>.enabled` pattern in values
**Criteria** (FROZEN):
- [x] All 7 subtasks completed
- [x] Each subchart has `condition: <name>.enabled` in parent Chart.yaml dependency
- [x] Default: all subcharts enabled (`<name>.enabled: true` in parent values)
- [x] Disabling a subchart excludes its resources from `helm template` rendering
- [x] Test coverage >=80%

**Subtasks** (atomic):

1. **Write test: Condition field in parent Chart.yaml dependencies**
   - Test case: `TestConditionalSubcharts_ConditionField`
     - Input: 3 subcharts (frontend, backend, database)
     - Expected: Each dependency has `condition: <name>.enabled`
   - Lines: ~40

2. **Write test: Default enabled values**
   - Test case: `TestConditionalSubcharts_DefaultEnabled`
     - Expected: Parent values has `frontend: {enabled: true}`, `backend: {enabled: true}`, etc.
   - Lines: ~35

3. **Write test: Subchart disabling**
   - Test case: `TestConditionalSubcharts_DisableSubchart`
     - Input: Set `database.enabled: false`
     - Expected: Database subchart resources NOT rendered by `helm template`
   - Lines: ~50

4. **Write test: Enable/disable doesn't affect other subcharts**
   - Test case: `TestConditionalSubcharts_IsolatedToggle`
     - Input: Disable database, keep frontend and backend enabled
     - Expected: Frontend and backend rendered normally
   - Lines: ~40

5. **Write test: All subcharts disabled**
   - Test case: `TestConditionalSubcharts_AllDisabled`
     - Input: All subcharts disabled
     - Expected: Only parent chart metadata rendered, no resources
   - Lines: ~30

6. **Implement conditional subchart support**
   - Extend `pkg/generator/umbrella.go`
   - Add `condition` field to each dependency in `generateParentChartYAML()`
   - Add `<name>.enabled: true` to parent values for each subchart
   - Lines: ~80

7. **Run tests -> Fix code -> Verify coverage**
   - Step 1: Run `go test -v ./pkg/generator/ -run TestConditionalSubcharts`
   - Step 2: Fix code to pass tests
   - Step 3: Verify coverage >=80%
   - Step 4: Regression: `go test ./pkg/generator/...`

**Time Estimate**:
- **Solo**: 4-6h (0.5-0.75 days)
  - Subtasks 1-5 (write tests): 3-4h
  - Subtask 6 (implement): 1-1.5h
  - Subtask 7 (fix/verify): 0.5h
- **AI-assisted**: 0.4-0.6h

**TDD Workflow**:
1. Write ALL tests FIRST (subtasks 1-5) — defines contract
2. Run tests -> expect FAILURES
3. Implement conditional support (subtask 6) -> tests PASS
4. Verify coverage >=80%
5. NO test changes after initial write

**Velocity tracking**: Record actual time in `docs/velocity_v0.3.0.md`
**Dependencies**: Task 3.1 (UmbrellaGenerator)
**Blocks**: Task 3.3

---

### Task 3.3: Integration Tests — Umbrella Generator

**Goal**: End-to-end tests for Umbrella generation mode through the full pipeline
**Result**: `tests/integration/pipeline_umbrella_test.go`
**Criteria** (FROZEN):
- [x] All 8 subtasks completed
- [x] Full pipeline: YAML -> Umbrella chart with subcharts
- [x] Conditional subcharts validated
- [x] Cascading values verified
- [x] All tests PASS

**Subtasks** (atomic):

1. **Write test: Umbrella with 3 subcharts**
   - Test case: `TestPipelineUmbrella_ThreeSubcharts`
     - Input: Frontend + Backend + Database (YAML fixtures)
     - Expected: 1 parent chart, 3 subcharts in `charts/`
     - Verify: Parent Chart.yaml has 3 dependencies
   - Lines: ~100

2. **Write test: Conditional subchart disabling**
   - Test case: `TestPipelineUmbrella_ConditionalDisable`
     - Values override: `database.enabled: false`
     - Expected: Database templates NOT rendered via `helm template`
   - Lines: ~70

3. **Write test: Cascading values override**
   - Test case: `TestPipelineUmbrella_CascadingValues`
     - Parent values: `frontend: {replicaCount: 5}`
     - Subchart default: `replicaCount: 1`
     - Expected: Rendered template shows 5 replicas
   - Lines: ~60

4. **Write test: Global values propagation**
   - Test case: `TestPipelineUmbrella_GlobalValues`
     - Parent values: `global: {imageRegistry: registry.example.com}`
     - Expected: Subchart templates use global registry
   - Lines: ~50

5. **Write test: All 18 resource types in umbrella**
   - Test case: `TestPipelineUmbrella_AllResourceTypes`
     - Input: Full-stack with all resource types
     - Expected: Resources correctly distributed to subcharts
   - Lines: ~100

6. **Write test: Helm lint umbrella chart**
   - Test case: `TestPipelineUmbrella_HelmLint`
     - Run `helm lint` on parent chart
     - Expected: 0 errors
   - Lines: ~40

7. **Write test: Regression — Other modes still work**
   - Test case: `TestPipelineUmbrella_UniversalRegression`
   - Test case: `TestPipelineUmbrella_SeparateRegression`
   - Test case: `TestPipelineUmbrella_LibraryRegression`
   - Lines: ~60

8. **Create test fixtures for umbrella mode**
   - Files: `tests/integration/fixtures/umbrella-app/`
   - Fixtures: frontend.yaml, backend.yaml, database.yaml
   - Lines: ~150 (YAML)

**Time Estimate**:
- **Solo**: 6-8h (0.75-1 day)
  - Subtasks 1-8 (write tests + fixtures): 6-8h
- **AI-assisted**: 0.6-0.8h

**TDD Workflow**:
1. Write ALL integration tests FIRST (subtasks 1-7)
2. Create fixtures (subtask 8)
3. Run tests -> expect FAILURES
4. Iterate on generator code until all tests PASS
5. No test changes

**Velocity tracking**: Record actual time in `docs/velocity_v0.3.0.md`
**Dependencies**: Tasks 3.1-3.2 (Umbrella Generator components)
**Blocks**: Task 4.2 (documentation)

---

## Week 6: Environment Values + Polish (Days 26-30)

### Task 4.1: Environment-Specific Values

**Goal**: Generate environment-specific values files (dev, staging, prod) with sensible defaults
**Result**: `pkg/generator/envvalues.go` + generated `values-{env}.yaml` files
**Criteria** (FROZEN):
- [x] All 10 subtasks completed
- [x] Three environment profiles: dev, staging, prod
- [x] Dev: 1 replica, debug logging, no PDB, relaxed resources
- [x] Staging: 2 replicas, info logging, optional PDB, moderate resources
- [x] Prod: 3+ replicas, warn logging, PDB, full resource limits, node affinity
- [x] CLI flag: `--env-values` to enable generation
- [x] Works with all three output modes (universal, separate, library, umbrella)
- [x] Test coverage >=80%

**Subtasks** (atomic):

1. **Write test: Dev values profile**
   - Test case: `TestEnvValues_DevProfile_Replicas`
     - Expected: `replicaCount: 1`
   - Test case: `TestEnvValues_DevProfile_LogLevel`
     - Expected: `logLevel: debug`
   - Test case: `TestEnvValues_DevProfile_NoPDB`
     - Expected: `podDisruptionBudget.enabled: false` or PDB section absent
   - Test case: `TestEnvValues_DevProfile_RelaxedResources`
     - Expected: No resource limits, or very generous limits
   - Lines: ~50

2. **Write test: Staging values profile**
   - Test case: `TestEnvValues_StagingProfile_Replicas`
     - Expected: `replicaCount: 2`
   - Test case: `TestEnvValues_StagingProfile_LogLevel`
     - Expected: `logLevel: info`
   - Test case: `TestEnvValues_StagingProfile_OptionalPDB`
     - Expected: `podDisruptionBudget.enabled: true`, `minAvailable: 1`
   - Lines: ~45

3. **Write test: Prod values profile**
   - Test case: `TestEnvValues_ProdProfile_Replicas`
     - Expected: `replicaCount: 3` (minimum)
   - Test case: `TestEnvValues_ProdProfile_LogLevel`
     - Expected: `logLevel: warn`
   - Test case: `TestEnvValues_ProdProfile_PDB`
     - Expected: `podDisruptionBudget.enabled: true`, `minAvailable: 2`
   - Test case: `TestEnvValues_ProdProfile_ResourceLimits`
     - Expected: `resources.limits` and `resources.requests` both set
   - Test case: `TestEnvValues_ProdProfile_Affinity`
     - Expected: Node affinity or topology constraints present
   - Lines: ~60

4. **Write test: Environment detection from source resources**
   - Test case: `TestEnvValues_Detection_HighReplicasIsProdLike`
     - Input: Deployment with 5 replicas
     - Expected: Base values preserve original 5, env overrides scale down for dev/staging
   - Test case: `TestEnvValues_Detection_LowReplicasIsDevLike`
     - Input: Deployment with 1 replica
     - Expected: Env overrides scale UP for staging/prod
   - Lines: ~45

5. **Write test: File naming convention**
   - Test case: `TestEnvValues_FileNaming`
     - Expected: `values-dev.yaml`, `values-staging.yaml`, `values-prod.yaml`
   - Test case: `TestEnvValues_FileNaming_WithOutputDir`
     - Expected: Files placed in same directory as values.yaml
   - Lines: ~30

6. **Write test: Values override structure (only differences from base)**
   - Test case: `TestEnvValues_OverrideOnly`
     - Expected: Env files only contain values that DIFFER from base values.yaml
     - Expected: NOT a full copy of values.yaml
   - Lines: ~40

7. **Write test: CLI flag integration**
   - Test case: `TestEnvValues_CLIFlag_Enabled`
     - Input: `--env-values` flag set
     - Expected: Env files generated alongside regular output
   - Test case: `TestEnvValues_CLIFlag_Disabled`
     - Input: No `--env-values` flag
     - Expected: No env files generated (backward compatible)
   - Lines: ~40

8. **Write test: Works with all output modes**
   - Test case: `TestEnvValues_WithUniversalMode`
   - Test case: `TestEnvValues_WithSeparateMode`
   - Test case: `TestEnvValues_WithUmbrellaMode`
   - Expected: Each mode generates env-specific values correctly
   - Lines: ~60

9. **Implement environment values generator**
   - File: `pkg/generator/envvalues.go`
   - Types: `EnvironmentProfile`, `EnvValuesGenerator`
   - Functions: `GenerateEnvValues(baseValues, profiles) map[string][]byte`
   - Add `--env-values` flag to `cmd/dhg/main.go`
   - Lines: ~250

10. **Run tests -> Fix code -> Verify coverage**
    - Step 1: Run `go test -v ./pkg/generator/ -run TestEnvValues`
    - Step 2: Fix code to pass tests
    - Step 3: Verify coverage >=80%
    - Step 4: Test CLI integration: `go test -v ./cmd/dhg/ -run TestEnvValues`
    - Step 5: Regression: `go test ./...`

**Time Estimate**:
- **Solo**: 10-12h (1.25-1.5 days)
  - Subtasks 1-8 (write tests): 7-8h
  - Subtask 9 (implement): 2-3h
  - Subtask 10 (fix/verify): 1h
- **AI-assisted**: 1.0-1.2h

**TDD Workflow**:
1. Write ALL tests FIRST (subtasks 1-8) — defines contract
2. Run tests -> expect FAILURES
3. Implement envvalues (subtask 9) -> tests PASS
4. Verify coverage >=80%
5. NO test changes after initial write

**Velocity tracking**: Record actual time in `docs/velocity_v0.3.0.md`
**Dependencies**: None (works with any generator mode, but best tested after all modes exist)
**Blocks**: Tasks 4.2, 4.3

---

### Task 4.2: Documentation & Release Prep

**Goal**: Update all documentation for v0.3.0 features
**Result**: README, CHANGELOG, examples updated with all 4 output modes and env values
**Criteria** (FROZEN):
- [x] All 10 subtasks completed
- [x] README documents all 4 output modes (`--mode universal|separate|library|umbrella`)
- [x] README documents `--env-values` flag
- [x] CHANGELOG.md has v0.3.0 section with all changes
- [x] examples/ has mode-specific example directories
- [x] Coverage badge updated to reflect new coverage
- [x] All code examples in docs are tested/verified

**Subtasks** (atomic):

1. **Update README.md — Output Modes section**
   - Add section: "## Output Modes"
   - Document universal mode (existing, default)
   - Document separate mode: `dhg generate -f manifests/ -o charts/ --mode separate`
   - Document library mode: `dhg generate -f manifests/ -o charts/ --mode library`
   - Document umbrella mode: `dhg generate -f manifests/ -o charts/ --mode umbrella`
   - Include comparison table: when to use each mode
   - Lines: ~60

2. **Update README.md — Environment Values section**
   - Add section: "## Environment-Specific Values"
   - Document: `dhg generate -f manifests/ -o chart/ --env-values`
   - Explain dev/staging/prod profiles and their defaults
   - Lines: ~40

3. **Update README.md — Quick Start section**
   - Update existing Quick Start to mention output modes
   - Add examples for each mode
   - Lines: ~30

4. **Update README.md — Coverage badge**
   - Run `go test -cover ./...` to get actual coverage
   - Update badge: `![Coverage](https://img.shields.io/badge/coverage-XX%25-brightgreen)`
   - Lines: ~5

5. **Create CHANGELOG.md v0.3.0 section**
   - Section: "## [0.3.0] - YYYY-MM-DD"
   - Subsections: Added, Changed, Fixed
   - List all new generators, env values, bug fixes
   - Lines: ~40

6. **Create examples/06-separate-mode/ directory**
   - Input manifests: frontend.yaml, backend.yaml
   - Expected output: 2 charts with dependencies
   - README.md explaining the example
   - Lines: ~100 (YAML + markdown)

7. **Create examples/07-library-mode/ directory**
   - Input manifests: same as above
   - Expected output: 1 library + 2 wrappers
   - README.md explaining the example
   - Lines: ~100

8. **Create examples/08-umbrella-mode/ directory**
   - Input manifests: frontend.yaml, backend.yaml, database.yaml
   - Expected output: 1 umbrella + 3 subcharts
   - README.md explaining the example
   - Lines: ~120

9. **Create examples/09-env-values/ directory**
   - Input manifests + `--env-values` flag demo
   - Expected output: values.yaml + values-dev.yaml + values-staging.yaml + values-prod.yaml
   - README.md explaining the example
   - Lines: ~100

10. **Verify all documentation**
    - Run all code examples from README to verify they work
    - Check all links in README
    - Verify CHANGELOG is accurate against actual changes
    - Lines: ~0 (verification only)

**Time Estimate**:
- **Solo**: 6-8h (0.75-1 day)
  - Subtasks 1-5 (README + CHANGELOG): 3-4h
  - Subtasks 6-9 (examples): 3-4h
  - Subtask 10 (verification): 0.5h
- **AI-assisted**: 0.6-0.8h

**TDD Workflow**: N/A (documentation task, but examples are verified against actual generation output)

**Velocity tracking**: Record actual time in `docs/velocity_v0.3.0.md`
**Dependencies**: All implementation tasks (1.1-4.1)
**Blocks**: Task 4.3

---

### Task 4.3: Release v0.3.0

**Goal**: Tag, test, and release v0.3.0
**Result**: Git tag `v0.3.0`, release notes, passing CI, benchmark comparison
**Criteria** (FROZEN):
- [x] All 9 subtasks completed
- [x] All tests pass: `go test ./... -count=1`
- [x] Test coverage >=80% for new code
- [x] No regressions in existing functionality
- [x] Git tag `v0.3.0` created
- [x] CHANGELOG updated with final release date
- [x] Benchmark comparison with v0.2.0 documented
- [x] Release notes created (docs/RELEASE_v0.3.0.md)
- [x] Docker image builds successfully

**Subtasks** (atomic):

1. **Run full test suite**
   - Command: `go test ./... -count=1 -v`
   - Expected: ALL tests PASS (unit + integration + e2e)
   - Verify: 0 failures, 0 skipped

2. **Verify test coverage**
   - Command: `go test -cover ./pkg/generator/...`
   - Expected: >=80% for all new generator code
   - Record exact coverage numbers

3. **Run regression tests**
   - Verify: Universal mode output identical to v0.2.0 for same inputs
   - Verify: All existing integration and e2e tests pass unchanged
   - Command: `go test ./tests/integration/... ./tests/e2e/...`

4. **Run performance benchmarks**
   - Command: `make bench`
   - Compare: v0.3.0 vs v0.2.0 baselines from `docs/benchmark_baseline.md`
   - Expected: <10s for 100 resources in any mode (release criteria)
   - Document: New baselines for separate/library/umbrella modes

5. **Verify Docker build**
   - Command: `docker build -t dhg:v0.3.0 --build-arg VERSION=v0.3.0 .`
   - Expected: Build succeeds
   - Test: `docker run --rm dhg:v0.3.0 --help` works

6. **Update CHANGELOG with release date**
   - Change `[0.3.0] - YYYY-MM-DD` to actual date
   - Final review of all changelog entries

7. **Create release notes**
   - File: `docs/RELEASE_v0.3.0.md`
   - Content: Summary, What's New, Installation, Performance, Upgrade Notes
   - Lines: ~80

8. **Create git tag**
   - Command: `git tag -a v0.3.0 -m "Release v0.3.0: Complete Output Modes"`
   - Verify: Tag on correct commit (after all changes committed)

9. **Final verification**
   - Run: `go vet ./...` (no warnings)
   - Run: `gitleaks detect --source . --verbose` (no secrets)
   - Verify: `.claude/` in `.gitignore`
   - Run: full test suite one more time

**Time Estimate**:
- **Solo**: 4-6h (0.5-0.75 days)
  - Subtasks 1-3 (testing): 1-2h
  - Subtasks 4-5 (benchmarks + docker): 1-1.5h
  - Subtasks 6-9 (release artifacts): 1-2h
- **AI-assisted**: 0.4-0.6h

**TDD Workflow**: N/A (release task — all tests already written and passing)

**Velocity tracking**: Record actual time in `docs/velocity_v0.3.0.md`
**Dependencies**: All tasks (1.1-4.2)
**Blocks**: None (final task)

---

## v0.3.0 Summary

### Total Tasks: 15

| Week | Tasks | Theme | Subtasks |
|------|-------|-------|----------|
| 1-2 | 1.1-1.5 | Separate Generator | 43 |
| 3-4 | 2.1-2.4 | Library Generator | 34 |
| 5 | 3.1-3.3 | Umbrella Generator | 25 |
| 6 | 4.1-4.3 | Env Values + Polish | 29 |
| **Total** | **15** | | **131** |

### Dependency Graph

```
Task 1.1 (Grouping) ──┬──> Task 1.2 (SeparateGen) ──┬──> Task 1.3 (InterChartDeps) ──> Task 1.5 (IntTests)
                       │                              └──> Task 1.4 (GlobalValues)  ────> Task 1.5 (IntTests)
                       └──> Task 3.1 (UmbrellaGen) ──> Task 3.2 (ConditionalSub) ──> Task 3.3 (IntTests)

Task 2.1 (LibraryGen) ──┬──> Task 2.2 (Wrappers) ──> Task 2.4 (IntTests)
                         └──> Task 2.3 (DRY)       ──> Task 2.4 (IntTests)

Task 4.1 (EnvValues) ──> Task 4.2 (Docs) ──> Task 4.3 (Release)
Tasks 1.5, 2.4, 3.3  ──> Task 4.2 (Docs) ──> Task 4.3 (Release)
```

### Critical Path

`Task 1.1 -> Task 1.2 -> Task 1.3 -> Task 1.5 -> Task 4.2 -> Task 4.3`

### Parallel Tracks

| Track | Tasks | Can run in parallel with |
|-------|-------|------------------------|
| Separate | 1.1-1.5 | Library (after 1.1 done) |
| Library | 2.1-2.4 | Separate (independent of 1.2+) |
| Umbrella | 3.1-3.3 | Library (after 1.1 done) |
| EnvValues | 4.1 | Any (independent) |

### Total Time Estimates

| Mode | Estimated | Adjusted (velocity) |
|------|-----------|-------------------|
| Solo (human) | 134-162h (16.75-20.25 days) | -- |
| AI-assisted | 11.4-14.0h | ~12h expected |
| AI sessions | 2-3 sessions | Based on v0.2.0 pattern |

### Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| Helm chart dependency resolution complexity | Medium | High | Prototype early, test with real Helm CLI |
| Library template parameterization design | Medium | Medium | Study existing library charts (bitnami/common) |
| Cross-chart value propagation | Low | Medium | Leverage Helm's built-in global values |
| Regression in existing tests | Low | Low | Full test suite runs after each task |
| OutputModeUmbrella not in types | Low | Low | Add to types.OutputMode in Task 3.1 |

### Key Design Decisions (to be resolved in implementation)

1. **Shared resources** (e.g., ConfigMap used by 2 services): Duplicate to each chart or place in parent?
2. **Library template contract**: Use `dict` pattern or `include` with context?
3. **Umbrella vs Separate**: When same resource appears in multiple groups, which mode handles it better?
4. **Grouping algorithm**: BFS vs DFS for connected components? (Prefer BFS for breadth-first service boundary detection)

---

## Future Releases

| Version | Scope | Estimated (AI) |
|---------|-------|---------------|
| v0.3.5 | Architecture-specific generation (2.4), Namespace management (2.5) | ~10h |
| v0.4.0 | Deckhouse CRD processors, module structure, lib-helm integration | ~15h |
| v0.5.0 | Cluster & GitOps extractors, multi-source merge | ~12h |
| v0.6.0 | Auto-fix engine, advanced analysis, CRD detection | ~20h |
| v1.0.0 | GA: polish, documentation, performance optimization | ~8h |

---

## Appendix: Velocity Tracking Template

After each task, record actual time in `docs/velocity_v0.3.0.md`:

```
[TIMER START] Task X.Y — 2026-MM-DD HH:MM UTC
... work ...
[TIMER END] Task X.Y — 2026-MM-DD HH:MM UTC — Duration: XXm
```

**Velocity table format:**

```markdown
| Task | Est (solo) | Est (AI) | Actual | Velocity | Start | End | Notes |
|------|-----------|---------|--------|----------|-------|-----|-------|
| 1.1 | 10-12h | 1.0-1.2h | Xm | Y | HH:MM | HH:MM | [blockers, learnings] |
| 1.2 | 12-14h | 1.2-1.4h | Xm | Y | | | |
| ... | | | | | | | |

**Running Average Velocity**: Z
**Remaining Estimate**: sum(remaining) / avg_velocity
```

**Velocity formula:**
```
velocity = solo_estimate_midpoint / actual_hours
adjusted_remaining = sum(remaining_solo_midpoints) / running_avg_velocity
```
