# Deckhouse Helm Generator — Implementation Plan v2.0

**Version**: 2.0.0 (Goal-Result-Criteria Framework)
**Created**: 2026-02-16
**Status**: Active
**Methodology**: TDD + Atomic Decomposition + Dual Estimation

**Current Implementation Status**: ~55% Phase 1 complete (Tasks 1.1 ✅, 1.2 ✅)
**Critical Gap**: Test coverage 27.2% package-wide vs 80% target (deployment.go: 96.5% ✅)

---

## Estimation Methodology

### Dual Tracking (Solo vs Team)

All tasks include **dual estimates** for benchmarking and velocity comparison:

| Context | Description | Usage |
|---------|-------------|-------|
| **Solo Developer** | Single person, part-time (4-6h/day effective coding) | Current reality, baseline |
| **Team (FTE)** | 1-2 developers, full-time (7-8h/day effective coding) | Future scaling, comparison |

**Velocity Multipliers:**
- Solo: 1.0x (baseline, includes context switching, learning)
- Team (1 FTE): 1.5-2x (dedicated focus, pair programming)
- Team (2+ FTE): 2.5-3x (parallel work, specialization)

**Tracking Metrics:**
- After each task: Record **actual time** vs **estimated time**
- Calculate velocity: `velocity = estimated_hours / actual_hours`
- Adjust future estimates: `new_estimate = base_estimate / avg_velocity`

---

## v0.2.0 — Stabilization (Q2 2026, 8 weeks)

**Release Goal**: Achieve production-ready test coverage (80%) + critical K8s processors
**Release Result**: v0.2.0 with 14/20 K8s processors, ≥80% test coverage, CI/CD pipeline
**Release Criteria** (FROZEN):
- [ ] Test coverage ≥80% (unit + integration + E2E)
- [ ] 14/20 K8s processors implemented and tested
- [ ] CI/CD pipeline operational (GitHub Actions)
- [ ] Zero P0/P1 bugs in test suite
- [ ] Documentation updated (README, CHANGELOG)
- [ ] Performance benchmark: <5s for 100 resources

---

## Week 1-2: Test Infrastructure (Days 1-10)

### Task 1.1: Test Utilities & Framework ✅ COMPLETED

**Goal**: Create reusable test infrastructure for consistent testing across all processors
**Result**: `pkg/testutil/` package with fixtures, mocks, assertions, coverage helpers
**Criteria** (FROZEN):
- [x] All 8 subtasks completed
- [x] Test utilities package builds without errors
- [x] Sample usage in at least 1 processor test (test_utils_test.go)
- [x] Documentation: godoc comments on all exported functions
- [x] Code review passed

**Actual Time**: ~2.5 hours (estimate: 16-20h solo)
**Velocity Factor**: 7.2x faster than estimate
**Notes**:
- Created complete testutil infrastructure
- 5 files: test_utils.go, factory.go, mock_processor.go, coverage.go, README.md
- 14 helper functions + resource factories + mock processor
- 5 YAML fixtures (deployment, statefulset, service, configmap, secret)
- All tests passing (13/13)
- TDD violation detected and fixed (GAP-ENG-007)

**Subtasks** (atomic):

1. **Create test utilities package structure**
   - File: `pkg/testutil/testutil.go`
   - Functions: `LoadFixture()`, `MustLoadFixture()`, `CompareYAML()`
   - Lines: ~100

2. **Implement resource factory helpers**
   - File: `pkg/testutil/factory.go`
   - Functions: `NewDeployment()`, `NewService()`, `NewConfigMap()` (factory pattern)
   - Lines: ~150

3. **Create mock processor interface**
   - File: `pkg/testutil/mock_processor.go`
   - Mock implementation of `processor.Processor` interface
   - Lines: ~80

4. **Implement coverage helpers**
   - File: `pkg/testutil/coverage.go`
   - Functions: `AssertCoverage(t *testing.T, pkg string, minCoverage float64)`
   - Lines: ~50

5. **Create test fixtures directory**
   - Path: `pkg/testutil/fixtures/`
   - Fixtures: `deployment.yaml`, `service.yaml`, `configmap.yaml`, `secret.yaml` (10 files)
   - Lines: ~300 (YAML)

6. **Implement assertion helpers**
   - File: `pkg/testutil/assert.go`
   - Functions: `AssertMapContains()`, `AssertSliceContains()`, `AssertStructEqual()`
   - Lines: ~120

7. **Write testutil tests**
   - File: `pkg/testutil/testutil_test.go`
   - Test coverage for testutil package itself (meta-testing)
   - Lines: ~200

8. **Document test utilities**
   - File: `pkg/testutil/README.md`
   - Usage examples, best practices
   - Lines: ~150

**Time Estimate**:
- **Solo**: 16-20 hours (2-2.5 days)
- **Team (1 FTE)**: 10-12 hours (1.25-1.5 days)
- **Velocity tracking**: Record actual time in `docs/velocity.md`

**TDD Workflow**:
- Write tests for testutil (subtask 7) BEFORE implementing utilities
- Run tests → expect failures
- Implement utilities → tests pass
- Use utilities in first processor test (validation)

**Dependencies**: None (foundational task)
**Blocks**: All processor tests (1.2-1.8)

---

### Task 1.2: Unit Tests — Deployment Processor ✅ COMPLETED

**Goal**: Establish test-first contract for Deployment processor expected behavior
**Result**: `deployment_test.go` with 15+ test cases, 80%+ coverage, all tests passing
**Criteria** (FROZEN):
- [x] All 13 subtasks completed
- [x] 15+ test cases covering all `ProcessDeployment()` code paths (49 test cases, 35 functions)
- [x] Test coverage ≥80% for `deployment.go` (measured via `go test -cover`) — **96.5%**
- [x] All tests PASS (no skipped tests) — 49/49
- [x] Code fixed to match test expectations (NOT tests changed to match code) — 5 fixes
- [x] No flaky tests (tests deterministic, no time/randomness dependencies)
- [x] Test execution time <1s total — **0.009s**

**Actual Time**: ~1.5 hours (estimate: 12-16h solo)
**Velocity Factor**: 9.3x faster than estimate
**Notes**:
- TDD workflow followed strictly: tests written first, 5 code fixes applied
- Code fixes: nil guard, default replicas, podSecurityContext, container securityContext, startupProbe
- Template updated for podSecurityContext and startupProbe support
- Testutil path resolution fixed for 3-level-deep packages

**Subtasks** (atomic):

1. **Write test: Extract replicas field**
   - Test case: `TestProcessDeployment_ExtractsReplicas_Valid`
     - Input: Deployment with `spec.replicas: 3`
     - Expected: `values["replicaCount"] == 3`
   - Test case: `TestProcessDeployment_ExtractsReplicas_Nil`
     - Input: Deployment with `spec.replicas: nil`
     - Expected: `values["replicaCount"] == 1` (default)
   - Lines: ~40

2. **Write test: Extract container image**
   - Test case: `TestProcessDeployment_ExtractsImage_SingleContainer`
     - Input: 1 container with `image: nginx:1.21`
     - Expected: `values["image"]["repository"] == "nginx"`, `values["image"]["tag"] == "1.21"`
   - Test case: `TestProcessDeployment_ExtractsImage_MultipleContainers`
     - Input: 2+ containers
     - Expected: `values["containers"][0]["image"]`, `values["containers"][1]["image"]`
   - Test case: `TestProcessDeployment_ExtractsImage_NoTag`
     - Input: `image: nginx` (no tag)
     - Expected: `values["image"]["tag"] == "latest"` (default)
   - Lines: ~60

3. **Write test: Extract resource limits/requests**
   - Test case: `TestProcessDeployment_ExtractsResources_LimitsAndRequests`
     - Input: `resources: {limits: {cpu: 1, memory: 1Gi}, requests: {cpu: 500m, memory: 512Mi}}`
     - Expected: `values["resources"]["limits"]["cpu"] == "1"`, etc.
   - Test case: `TestProcessDeployment_ExtractsResources_MissingLimits`
     - Input: Only `requests` present
     - Expected: `values["resources"]["limits"]` absent or empty
   - Lines: ~50

4. **Write test: Extract pod labels**
   - Test case: `TestProcessDeployment_ExtractsPodLabels`
     - Input: `spec.template.metadata.labels: {app: myapp, version: v1}`
     - Expected: `values["podLabels"]["app"] == "myapp"`
   - Lines: ~30

5. **Write test: Extract pod annotations**
   - Test case: `TestProcessDeployment_ExtractsPodAnnotations`
     - Input: `spec.template.metadata.annotations: {prometheus.io/scrape: "true"}`
     - Expected: `values["podAnnotations"]["prometheus.io/scrape"] == "true"`
   - Lines: ~30

6. **Write test: Extract node affinity**
   - Test case: `TestProcessDeployment_ExtractsAffinity_NodeAffinity`
     - Input: `spec.template.spec.affinity.nodeAffinity`
     - Expected: `values["affinity"]["nodeAffinity"]` structure preserved
   - Lines: ~40

7. **Write test: Extract pod affinity/anti-affinity**
   - Test case: `TestProcessDeployment_ExtractsAffinity_PodAntiAffinity`
     - Input: `spec.template.spec.affinity.podAntiAffinity`
     - Expected: `values["affinity"]["podAntiAffinity"]` structure preserved
   - Lines: ~40

8. **Write test: Extract tolerations**
   - Test case: `TestProcessDeployment_ExtractsTolerations`
     - Input: `spec.template.spec.tolerations: [{key: node-role, operator: Exists}]`
     - Expected: `values["tolerations"][0]["key"] == "node-role"`
   - Lines: ~35

9. **Write test: Extract securityContext**
   - Test case: `TestProcessDeployment_ExtractsSecurityContext_Pod`
     - Input: `spec.template.spec.securityContext: {runAsUser: 1000}`
     - Expected: `values["podSecurityContext"]["runAsUser"] == 1000`
   - Test case: `TestProcessDeployment_ExtractsSecurityContext_Container`
     - Input: Container-level `securityContext`
     - Expected: `values["securityContext"]` (container level)
   - Lines: ~50

10. **Write test: Extract volumes and volumeMounts**
    - Test case: `TestProcessDeployment_ExtractsVolumes_ConfigMap`
      - Input: Volume from ConfigMap + volumeMount
      - Expected: `values["volumes"][0]["configMap"]["name"]`
    - Test case: `TestProcessDeployment_ExtractsVolumes_Secret`
      - Input: Volume from Secret + volumeMount
      - Expected: `values["volumes"][0]["secret"]["secretName"]`
    - Lines: ~60

11. **Write test: Extract probes (liveness, readiness, startup)**
    - Test case: `TestProcessDeployment_ExtractsProbes`
      - Input: All 3 probe types with httpGet
      - Expected: `values["livenessProbe"]["httpGet"]["path"]`
    - Lines: ~50

12. **Write test: Edge cases**
    - Test case: `TestProcessDeployment_InvalidInput_NilDeployment`
      - Input: `nil` Deployment
      - Expected: Error returned
    - Test case: `TestProcessDeployment_EmptySpec`
      - Input: Deployment with empty spec
      - Expected: Default values applied
    - Lines: ~40

13. **Run tests → Fix code → Verify coverage**
    - Step 1: Run `go test -v ./pkg/processor/k8s/deployment_test.go`
    - Step 2: Analyze failures (expected at this stage)
    - Step 3: Fix `deployment.go` to pass tests (NOT change tests)
    - Step 4: Verify coverage: `go test -cover` → ≥80%
    - Step 5: Re-run tests → all PASS

**Time Estimate**:
- **Solo**: 12-16 hours (1.5-2 days)
  - Subtasks 1-12 (write tests): 10-12h
  - Subtask 13 (fix code): 2-4h
- **Team (1 FTE)**: 8-10 hours (1-1.25 days)
  - Parallel test writing + code fixing

**TDD Workflow**:
1. Write ALL tests FIRST (subtasks 1-12) — defines contract
2. Run tests → expect FAILURES (current code doesn't match contract)
3. Fix `deployment.go` code (subtask 13) → tests PASS
4. Verify coverage ≥80%
5. NO test changes after initial write (tests are frozen contract)

**Dependencies**: Task 1.1 (testutil package)
**Blocks**: Integration tests (Week 3)

---

### Task 1.3: Unit Tests — Service Processor

**Goal**: Establish test-first contract for Service processor expected behavior
**Result**: `service_test.go` with 10+ test cases, 80%+ coverage, all tests passing
**Criteria** (FROZEN):
- [ ] All 10 subtasks completed
- [ ] 10+ test cases covering all `ProcessService()` code paths
- [ ] Test coverage ≥80% for `service.go`
- [ ] All tests PASS
- [ ] Code fixed to match test expectations

**Subtasks** (atomic):

1. **Write test: Extract service type**
   - Test cases: ClusterIP, NodePort, LoadBalancer, ExternalName
   - Expected: `values["service"]["type"]` matches input
   - Lines: ~40

2. **Write test: Extract ports**
   - Test case: Single port (HTTP 80)
   - Test case: Multiple ports (HTTP 80, HTTPS 443)
   - Test case: Named ports
   - Expected: `values["service"]["ports"]` array with `name`, `port`, `targetPort`, `protocol`
   - Lines: ~60

3. **Write test: Extract selectors**
   - Test case: Standard selectors (`app`, `version`)
   - Expected: `values["service"]["selector"]` map
   - Lines: ~30

4. **Write test: Extract session affinity**
   - Test case: `sessionAffinity: ClientIP`
   - Expected: `values["service"]["sessionAffinity"] == "ClientIP"`
   - Lines: ~25

5. **Write test: Extract loadBalancerIP**
   - Test case: LoadBalancer with specific IP
   - Expected: `values["service"]["loadBalancerIP"]`
   - Lines: ~25

6. **Write test: Extract annotations**
   - Test case: Cloud provider annotations (AWS ELB, GCP, Azure)
   - Expected: `values["service"]["annotations"]` preserved
   - Lines: ~30

7. **Write test: Extract externalTrafficPolicy**
   - Test case: `externalTrafficPolicy: Local`
   - Expected: `values["service"]["externalTrafficPolicy"] == "Local"`
   - Lines: ~25

8. **Write test: Extract healthCheckNodePort**
   - Test case: NodePort service with health check port
   - Expected: `values["service"]["healthCheckNodePort"]`
   - Lines: ~25

9. **Write test: Edge cases**
   - Test case: Headless service (clusterIP: None)
   - Test case: Service without selectors (manual endpoints)
   - Lines: ~40

10. **Run tests → Fix code → Verify coverage**
    - Same TDD workflow as Task 1.2 subtask 13

**Time Estimate**:
- **Solo**: 8-10 hours (1-1.25 days)
- **Team (1 FTE)**: 5-7 hours (0.6-0.9 days)

**Dependencies**: Task 1.1 (testutil)
**TDD Workflow**: Same as Task 1.2

---

### Task 1.4: Unit Tests — ConfigMap Processor

**Goal**: Establish test-first contract for ConfigMap processor expected behavior
**Result**: `configmap_test.go` with 8+ test cases, 80%+ coverage, all tests passing
**Criteria** (FROZEN):
- [ ] All 8 subtasks completed
- [ ] 8+ test cases covering all `ProcessConfigMap()` code paths
- [ ] Test coverage ≥80% for `configmap.go`
- [ ] All tests PASS
- [ ] External file generation tested

**Subtasks** (atomic):

1. **Write test: Extract data (simple key-value)**
   - Test case: `data: {key1: value1, key2: value2}`
   - Expected: `values["configmap"]["data"]["key1"] == "value1"`
   - Lines: ~30

2. **Write test: Extract data (multi-line values)**
   - Test case: `data: {nginx.conf: "...multi-line..."}` (>100 chars)
   - Expected: External file created in `files/configmap-data-nginx.conf`
   - Lines: ~40

3. **Write test: Extract binaryData**
   - Test case: `binaryData: {image.png: <base64>}`
   - Expected: `values["configmap"]["binaryData"]["image.png"]` or external file
   - Lines: ~35

4. **Write test: Detect JSON data**
   - Test case: `data: {config.json: "{\"key\": \"value\"}"}`
   - Expected: Parsed JSON in `values["configmap"]["data"]["config.json"]` (pretty-printed)
   - Lines: ~30

5. **Write test: Detect XML data**
   - Test case: `data: {config.xml: "<root>...</root>"}`
   - Expected: Formatted XML in values
   - Lines: ~30

6. **Write test: Large value externalization**
   - Test case: Data value >1KB
   - Expected: External file in `files/`, reference in values
   - Lines: ~35

7. **Write test: Edge cases**
   - Test case: Empty ConfigMap (no data/binaryData)
   - Test case: ConfigMap with only metadata
   - Lines: ~30

8. **Run tests → Fix code → Verify coverage**

**Time Estimate**:
- **Solo**: 6-8 hours (0.75-1 day)
- **Team (1 FTE)**: 4-5 hours (0.5-0.6 days)

**Dependencies**: Task 1.1
**TDD Workflow**: Same as Task 1.2

---

### Task 1.5: Unit Tests — Secret Processor

**Goal**: Establish test-first contract for Secret processor expected behavior
**Result**: `secret_test.go` with 9+ test cases, 80%+ coverage, all tests passing
**Criteria** (FROZEN):
- [ ] All 9 subtasks completed
- [ ] 9+ test cases covering all `ProcessSecret()` code paths
- [ ] Test coverage ≥80% for `secret.go`
- [ ] All tests PASS
- [ ] Secret value handling tested (base64, externalization)

**Subtasks** (atomic):

1. **Write test: Extract data (Opaque secret)**
   - Test case: `type: Opaque, data: {username: <base64>}`
   - Expected: `values["secret"]["data"]["username"]` (base64 preserved or decoded based on config)
   - Lines: ~35

2. **Write test: Extract stringData**
   - Test case: `stringData: {password: plaintext}`
   - Expected: Converted to base64 in values OR external file
   - Lines: ~30

3. **Write test: Docker registry secret**
   - Test case: `type: kubernetes.io/dockerconfigjson`
   - Expected: `values["secret"]["type"]`, data extracted
   - Lines: ~35

4. **Write test: TLS secret**
   - Test case: `type: kubernetes.io/tls, data: {tls.crt, tls.key}`
   - Expected: Certificate and key in values or external files
   - Lines: ~40

5. **Write test: SSH auth secret**
   - Test case: `type: kubernetes.io/ssh-auth, data: {ssh-privatekey}`
   - Expected: SSH key in external file (security)
   - Lines: ~35

6. **Write test: External file for large secrets**
   - Test case: Secret value >500 bytes
   - Expected: External file in `secrets/`, reference in values
   - Lines: ~35

7. **Write test: Secret strategy detection**
   - Test case: Detect if secret should use External Secrets Operator pattern
   - Expected: Annotation/label detection → recommendation in output
   - Lines: ~30

8. **Write test: Edge cases**
   - Test case: Empty secret (no data)
   - Test case: Secret with immutable: true
   - Lines: ~30

9. **Run tests → Fix code → Verify coverage**

**Time Estimate**:
- **Solo**: 8-10 hours (1-1.25 days)
- **Team (1 FTE)**: 5-6 hours (0.6-0.75 days)

**Dependencies**: Task 1.1
**TDD Workflow**: Same as Task 1.2

---

### Task 1.6: Unit Tests — Ingress Processor

**Goal**: Establish test-first contract for Ingress processor expected behavior
**Result**: `ingress_test.go` with 11+ test cases, 80%+ coverage, all tests passing
**Criteria** (FROZEN):
- [ ] All 11 subtasks completed
- [ ] 11+ test cases covering all `ProcessIngress()` code paths
- [ ] Test coverage ≥80% for `ingress.go`
- [ ] All tests PASS
- [ ] Ingress rules and TLS tested

**Subtasks** (atomic):

1. **Write test: Extract ingressClassName**
   - Test case: `spec.ingressClassName: nginx`
   - Expected: `values["ingress"]["className"] == "nginx"`
   - Lines: ~25

2. **Write test: Extract rules (single host)**
   - Test case: 1 host, 1 path, 1 backend
   - Expected: `values["ingress"]["hosts"][0]["host"]`, `paths`, `backend`
   - Lines: ~40

3. **Write test: Extract rules (multiple hosts)**
   - Test case: 2+ hosts, multiple paths per host
   - Expected: Array of hosts with paths
   - Lines: ~50

4. **Write test: Extract TLS configuration**
   - Test case: `spec.tls: [{hosts: [example.com], secretName: tls-secret}]`
   - Expected: `values["ingress"]["tls"]` array
   - Lines: ~35

5. **Write test: Extract backend service**
   - Test case: Backend with `service.name` and `service.port`
   - Expected: `values["ingress"]["hosts"][0]["paths"][0]["backend"]["service"]`
   - Lines: ~30

6. **Write test: Extract annotations**
   - Test case: NGINX annotations (`nginx.ingress.kubernetes.io/*`)
   - Test case: Cert-manager annotations (`cert-manager.io/cluster-issuer`)
   - Expected: `values["ingress"]["annotations"]` preserved
   - Lines: ~40

7. **Write test: Path type detection**
   - Test case: `pathType: Prefix` vs `Exact` vs `ImplementationSpecific`
   - Expected: `values["ingress"]["hosts"][0]["paths"][0]["pathType"]`
   - Lines: ~30

8. **Write test: Default backend**
   - Test case: `spec.defaultBackend` present
   - Expected: `values["ingress"]["defaultBackend"]`
   - Lines: ~25

9. **Write test: Wildcard hosts**
   - Test case: `host: *.example.com`
   - Expected: Wildcard preserved
   - Lines: ~25

10. **Write test: Edge cases**
    - Test case: Ingress without rules (only defaultBackend)
    - Test case: Empty host (host: "")
    - Lines: ~30

11. **Run tests → Fix code → Verify coverage**

**Time Estimate**:
- **Solo**: 10-12 hours (1.25-1.5 days)
- **Team (1 FTE)**: 6-8 hours (0.75-1 day)

**Dependencies**: Task 1.1
**TDD Workflow**: Same as Task 1.2

---

### Task 1.7: Unit Tests — StatefulSet/DaemonSet/PVC (common.go)

**Goal**: Establish test-first contract for StatefulSet, DaemonSet, PVC processors
**Result**: `common_test.go` with 15+ test cases, 80%+ coverage, all tests passing
**Criteria** (FROZEN):
- [ ] All 15 subtasks completed
- [ ] 15+ test cases covering StatefulSet, DaemonSet, PVC, ServiceAccount
- [ ] Test coverage ≥80% for `common.go`
- [ ] All tests PASS

**Subtasks** (atomic):

1. **Write test: StatefulSet — Extract serviceName**
   - Test case: `spec.serviceName: myapp-headless`
   - Expected: `values["statefulset"]["serviceName"]`
   - Lines: ~25

2. **Write test: StatefulSet — Extract volumeClaimTemplates**
   - Test case: PVC template with storageClassName
   - Expected: `values["statefulset"]["volumeClaimTemplates"]` array
   - Lines: ~45

3. **Write test: StatefulSet — Extract podManagementPolicy**
   - Test case: `podManagementPolicy: Parallel`
   - Expected: `values["statefulset"]["podManagementPolicy"]`
   - Lines: ~25

4. **Write test: StatefulSet — Extract updateStrategy**
   - Test case: `updateStrategy.type: RollingUpdate, partition: 3`
   - Expected: `values["statefulset"]["updateStrategy"]`
   - Lines: ~30

5. **Write test: DaemonSet — Extract updateStrategy**
   - Test case: `updateStrategy.type: RollingUpdate, maxUnavailable: 1`
   - Expected: `values["daemonset"]["updateStrategy"]`
   - Lines: ~30

6. **Write test: DaemonSet — Extract node selector**
   - Test case: `spec.template.spec.nodeSelector: {disktype: ssd}`
   - Expected: `values["daemonset"]["nodeSelector"]`
   - Lines: ~25

7. **Write test: PVC — Extract storageClassName**
   - Test case: `spec.storageClassName: fast`
   - Expected: `values["persistence"]["storageClass"]`
   - Lines: ~25

8. **Write test: PVC — Extract resources**
   - Test case: `spec.resources.requests.storage: 10Gi`
   - Expected: `values["persistence"]["size"] == "10Gi"`
   - Lines: ~25

9. **Write test: PVC — Extract accessModes**
   - Test case: `accessModes: [ReadWriteOnce]`
   - Expected: `values["persistence"]["accessModes"]`
   - Lines: ~25

10. **Write test: PVC — Extract volumeMode**
    - Test case: `volumeMode: Filesystem` vs `Block`
    - Expected: `values["persistence"]["volumeMode"]`
    - Lines: ~25

11. **Write test: PVC — Extract dataSource**
    - Test case: PVC cloned from snapshot
    - Expected: `values["persistence"]["dataSource"]`
    - Lines: ~30

12. **Write test: ServiceAccount — Extract imagePullSecrets**
    - Test case: `imagePullSecrets: [{name: regcred}]`
    - Expected: `values["serviceAccount"]["imagePullSecrets"]`
    - Lines: ~25

13. **Write test: ServiceAccount — Extract automountServiceAccountToken**
    - Test case: `automountServiceAccountToken: false`
    - Expected: `values["serviceAccount"]["automount"]`
    - Lines: ~20

14. **Write test: Edge cases**
    - Test case: StatefulSet without volumeClaimTemplates
    - Test case: PVC with selector (label matching)
    - Lines: ~40

15. **Run tests → Fix code → Verify coverage**

**Time Estimate**:
- **Solo**: 12-14 hours (1.5-1.75 days)
- **Team (1 FTE)**: 8-10 hours (1-1.25 days)

**Dependencies**: Task 1.1
**TDD Workflow**: Same as Task 1.2

---

### Task 1.8: Unit Tests — Relationship Detectors

**Goal**: Establish test-first contract for all relationship detector expected behaviors
**Result**: 4 test files (label, reference, volume, annotation), 80%+ coverage, all tests passing
**Criteria** (FROZEN):
- [ ] All 16 subtasks completed
- [ ] Test coverage ≥80% for all detectors (label.go, reference.go, volume.go, annotation.go)
- [ ] All tests PASS
- [ ] Relationship detection accuracy validated

**Subtasks** (atomic):

1. **Write test: Label detector — Service → Deployment**
   - Test case: Service selector matches Deployment pod labels
   - Expected: Relationship detected, type: `selector`
   - Lines: ~40

2. **Write test: Label detector — Multiple matches**
   - Test case: 1 Service selector matches 2 Deployments
   - Expected: 2 relationships detected
   - Lines: ~35

3. **Write test: Label detector — No match**
   - Test case: Service selector doesn't match any pods
   - Expected: No relationships, warning logged
   - Lines: ~30

4. **Write test: Reference detector — Deployment → ConfigMap**
   - Test case: Deployment references ConfigMap in `envFrom`
   - Expected: Relationship detected, type: `envFrom`
   - Lines: ~35

5. **Write test: Reference detector — Deployment → Secret**
   - Test case: Deployment references Secret in `env.valueFrom`
   - Expected: Relationship detected, type: `env`
   - Lines: ~35

6. **Write test: Reference detector — PVC → StorageClass**
   - Test case: PVC references StorageClass in `spec.storageClassName`
   - Expected: Relationship detected, type: `storageClass`
   - Lines: ~30

7. **Write test: Reference detector — ServiceAccount references**
   - Test case: Pod references ServiceAccount
   - Expected: Relationship detected, type: `serviceAccount`
   - Lines: ~30

8. **Write test: Volume detector — Deployment → ConfigMap volume**
   - Test case: Deployment mounts ConfigMap as volume
   - Expected: Relationship detected, type: `volume`, mountPath extracted
   - Lines: ~40

9. **Write test: Volume detector — Deployment → Secret volume**
   - Test case: Deployment mounts Secret as volume
   - Expected: Relationship detected, type: `volume`
   - Lines: ~35

10. **Write test: Volume detector — Deployment → PVC**
    - Test case: Deployment mounts PVC via volumeClaimName
    - Expected: Relationship detected, type: `persistentVolumeClaim`
    - Lines: ~35

11. **Write test: Volume detector — EmptyDir volume**
    - Test case: Deployment uses emptyDir (no relationship)
    - Expected: No relationship (internal volume)
    - Lines: ~25

12. **Write test: Annotation detector — Ingress → Service**
    - Test case: Ingress with annotation referencing Service
    - Expected: Relationship detected if annotation pattern matches
    - Lines: ~35

13. **Write test: Annotation detector — Custom annotations**
    - Test case: Resource with `dhg.deckhouse.io/depends-on` annotation
    - Expected: Custom dependency relationship
    - Lines: ~30

14. **Write test: Edge cases**
    - Test case: Circular dependencies detection
    - Test case: Missing referenced resource (dangling reference)
    - Lines: ~40

15. **Write test: Relationship graph construction**
    - Test case: Complex app (5 resources, 8 relationships)
    - Expected: Dependency graph correctly built, topological sort possible
    - Lines: ~60

16. **Run tests → Fix code → Verify coverage**

**Time Estimate**:
- **Solo**: 14-16 hours (1.75-2 days)
- **Team (1 FTE)**: 10-12 hours (1.25-1.5 days)

**Dependencies**: Task 1.1
**TDD Workflow**: Same as Task 1.2

---

## Week 1-2 Summary

**Total Time Estimate**:
- **Solo**: 86-106 hours (10.75-13.25 days, ~2.2 weeks @ 5h/day effective)
- **Team (1 FTE)**: 56-68 hours (7-8.5 days, ~1.5 weeks @ 8h/day effective)

**Velocity Tracking**:
- Record actual time for each task in `docs/velocity.md`
- Format: `Task 1.1 | Estimated: 16-20h | Actual: Xh | Velocity: Y`
- Calculate average velocity after Week 2
- Adjust Week 3+ estimates based on actual velocity

**Deliverables**:
- 8 test files (~1,860 lines)
- Test coverage: 5% → 65%
- Test utilities package fully functional
- All existing processors have ≥80% coverage

**Critical Success Factors**:
- TDD discipline: Tests FIRST, code SECOND
- No test changes after initial write (frozen contract)
- Coverage gates enforced (CI fails if <80%)
- Daily velocity tracking (compare estimated vs actual)

---

## Week 3: Integration Tests (Days 11-15)

### Task 2.1: Integration Test Framework

**Goal**: Create reusable integration test framework for end-to-end pipeline validation
**Result**: `tests/integration/framework.go` with test harness, pipeline executor, validation helpers
**Criteria** (FROZEN):
- [ ] All 8 subtasks completed
- [ ] Framework supports full pipeline: extract → process → analyze → generate
- [ ] Test isolation (each test in temp directory)
- [ ] Cleanup after tests (no leftover files)
- [ ] Framework tested with at least 1 sample pipeline

**Subtasks** (atomic):

1. **Create integration test directory structure**
   - Path: `tests/integration/`
   - Files: `framework.go`, `fixtures/`, `testdata/`
   - Lines: ~50

2. **Implement test harness**
   - File: `tests/integration/framework.go`
   - Type: `TestHarness` with methods: `Setup()`, `Run()`, `Cleanup()`
   - Functionality: Temp directory creation, cleanup, assertions
   - Lines: ~150

3. **Implement pipeline executor**
   - Function: `ExecutePipeline(inputDir string, opts GenerateOptions) (*ChartOutput, error)`
   - Executes: Extractor → Processor → Analyzer → Generator
   - Lines: ~120

4. **Implement chart validation helpers**
   - Functions: `ValidateChartStructure()`, `ValidateValues()`, `ValidateTemplates()`
   - Checks: Chart.yaml format, values.yaml syntax, template rendering
   - Lines: ~100

5. **Create integration test fixtures**
   - Path: `tests/integration/fixtures/`
   - Fixtures: `simple-app/` (3 resources), `full-stack/` (10 resources)
   - Lines: ~400 (YAML)

6. **Implement diff helpers**
   - Function: `CompareGeneratedChart(actual, expected *Chart) []string`
   - Compares: Chart structure, values, templates (semantic diff, not byte-by-byte)
   - Lines: ~80

7. **Write framework tests (meta-test)**
   - File: `tests/integration/framework_test.go`
   - Test the framework itself before using it
   - Lines: ~100

8. **Document integration test framework**
   - File: `tests/integration/README.md`
   - Usage examples, writing integration tests guide
   - Lines: ~150

**Time Estimate**:
- **Solo**: 12-14 hours (1.5-1.75 days)
- **Team (1 FTE)**: 8-10 hours (1-1.25 days)

**TDD Workflow**:
- Write framework tests (subtask 7) FIRST
- Implement framework → tests pass
- Use framework in next tasks (2.2-2.5)

**Dependencies**: Tasks 1.2-1.8 (processor tests must pass)
**Blocks**: Tasks 2.2-2.5 (pipeline tests)

---

### Task 2.2: Pipeline Test — Simple Deployment

**Goal**: Validate end-to-end pipeline for simple single-service application
**Result**: `pipeline_simple_test.go` with 5+ test cases, validates chart generation correctness
**Criteria** (FROZEN):
- [ ] All 6 subtasks completed
- [ ] 5+ test cases covering simple app scenarios
- [ ] All tests PASS
- [ ] Generated chart passes `helm lint`
- [ ] Generated chart passes `helm template`

**Subtasks** (atomic):

1. **Write test: Single Deployment + Service**
   - Input: `testdata/simple/deployment.yaml`, `service.yaml`
   - Expected: Chart with `templates/deployment.yaml`, `templates/service.yaml`, `values.yaml` with correct values
   - Validation: Replicas, image, service port extracted
   - Lines: ~50

2. **Write test: Deployment + ConfigMap**
   - Input: Deployment with ConfigMap reference
   - Expected: ConfigMap relationship detected, external file created (`files/configmap-data.txt`)
   - Lines: ~60

3. **Write test: Deployment + Secret**
   - Input: Deployment with Secret reference
   - Expected: Secret external file created (`secrets/secret-data.txt`), reference in values
   - Lines: ~55

4. **Write test: Values.yaml correctness**
   - Validation: All extracted values present, correct types (int, string, map)
   - Validation: No hardcoded values in templates (all parameterized)
   - Lines: ~40

5. **Write test: Helm lint validation**
   - Execute: `helm lint <generated-chart>`
   - Expected: Exit code 0, no errors/warnings
   - Lines: ~30

6. **Write test: Helm template validation**
   - Execute: `helm template <generated-chart>`
   - Expected: Valid YAML output, all templates render without errors
   - Lines: ~35

**Time Estimate**:
- **Solo**: 8-10 hours (1-1.25 days)
- **Team (1 FTE)**: 5-7 hours (0.6-0.9 days)

**TDD Workflow**:
- Write tests defining expected chart structure FIRST
- Run pipeline → compare actual vs expected
- Fix generator/processor code if mismatch

**Dependencies**: Task 2.1 (framework)

---

### Task 2.3: Pipeline Test — Full-Stack Application

**Goal**: Validate end-to-end pipeline for complex multi-tier application
**Result**: `pipeline_fullstack_test.go` with 8+ test cases, validates complex relationships
**Criteria** (FROZEN):
- [ ] All 8 subtasks completed
- [ ] 8+ test cases covering full-stack scenarios (frontend, backend, database, cache)
- [ ] All tests PASS
- [ ] Relationship graph correctly constructed

**Subtasks** (atomic):

1. **Write test: Multi-tier app (3 deployments, 3 services)**
   - Input: Frontend, Backend, Database (each with Deployment + Service)
   - Expected: 3 separate charts OR 1 chart with 6 templates
   - Lines: ~70

2. **Write test: Service dependencies**
   - Input: Frontend → Backend → Database (label-based relationships)
   - Expected: Dependency graph: `frontend depends-on backend depends-on database`
   - Lines: ~60

3. **Write test: Shared ConfigMap**
   - Input: Multiple deployments referencing same ConfigMap
   - Expected: Single ConfigMap template, multiple references in values
   - Lines: ~50

4. **Write test: Ingress + Service + Deployment**
   - Input: Ingress → Service → Deployment chain
   - Expected: All relationships detected, Ingress rules correctly extracted
   - Lines: ~65

5. **Write test: StatefulSet with PVC**
   - Input: Database StatefulSet with volumeClaimTemplate
   - Expected: PVC template generated, StatefulSet references it
   - Lines: ~60

6. **Write test: Job + ConfigMap (init job pattern)**
   - Input: Job that mounts ConfigMap (DB migration pattern)
   - Expected: Job template with ConfigMap reference
   - Lines: ~50

7. **Write test: DaemonSet (logging agent pattern)**
   - Input: DaemonSet with hostPath volumes
   - Expected: DaemonSet template with tolerations, nodeSelector
   - Lines: ~55

8. **Write test: Complex values.yaml structure**
   - Validation: Nested values for multi-tier app
   - Expected: `values.frontend.*`, `values.backend.*`, `values.database.*`
   - Lines: ~45

**Time Estimate**:
- **Solo**: 12-14 hours (1.5-1.75 days)
- **Team (1 FTE)**: 8-10 hours (1-1.25 days)

**Dependencies**: Task 2.1

---

### Task 2.4: Pipeline Test — Deckhouse Module

**Goal**: Validate pipeline for Deckhouse-specific resources (ModuleConfig, etc.)
**Result**: `pipeline_deckhouse_test.go` with 6+ test cases, validates Deckhouse patterns
**Criteria** (FROZEN):
- [ ] All 6 subtasks completed
- [ ] 6+ test cases covering Deckhouse resources
- [ ] All tests PASS
- [ ] Deckhouse module structure validated

**Subtasks** (atomic):

1. **Write test: ModuleConfig extraction**
   - Input: `ModuleConfig` CRD (Deckhouse-specific)
   - Expected: Module config extracted to values
   - Lines: ~50

2. **Write test: IngressNginxController extraction**
   - Input: IngressNginxController CRD
   - Expected: Nginx config extracted
   - Lines: ~50

3. **Write test: NodeGroup extraction (cloud-specific)**
   - Input: NodeGroup for AWS
   - Expected: Cloud provider settings extracted
   - Lines: ~55

4. **Write test: Deckhouse module structure**
   - Validation: Generated chart follows Deckhouse module conventions
   - Expected: `openapi/`, `values.yaml` with OpenAPI schema
   - Lines: ~60

5. **Write test: Helm hooks for Deckhouse**
   - Input: Deckhouse resources requiring pre-install hooks
   - Expected: Helm hooks generated (pre-install, post-upgrade)
   - Lines: ~50

6. **Write test: Edge case — Mixed Deckhouse + vanilla K8s**
   - Input: ModuleConfig + Deployment + Service
   - Expected: All resources processed correctly
   - Lines: ~55

**Time Estimate**:
- **Solo**: 10-12 hours (1.25-1.5 days)
- **Team (1 FTE)**: 7-9 hours (0.9-1.1 days)

**Dependencies**: Task 2.1

---

### Task 2.5: Generator Output Validation

**Goal**: Validate generated chart structure, syntax, and Helm compatibility
**Result**: `validate_test.go` with 10+ validation test cases
**Criteria** (FROZEN):
- [ ] All 10 subtasks completed
- [ ] 10+ test cases covering all validation aspects
- [ ] All tests PASS
- [ ] Charts generated in previous tests (2.2-2.4) all pass validation

**Subtasks** (atomic):

1. **Write test: Chart.yaml structure validation**
   - Validation: Required fields present (name, version, apiVersion)
   - Validation: Semantic versioning (version field)
   - Lines: ~40

2. **Write test: values.yaml syntax validation**
   - Validation: Valid YAML syntax (no tabs, correct indentation)
   - Validation: No duplicate keys
   - Lines: ~35

3. **Write test: Template rendering validation**
   - Validation: All templates render with default values
   - Validation: No Go template syntax errors
   - Lines: ~50

4. **Write test: _helpers.tpl validation**
   - Validation: Helper templates defined and used
   - Validation: Standard helpers present (fullname, labels, selectorLabels)
   - Lines: ~45

5. **Write test: NOTES.txt validation**
   - Validation: NOTES.txt exists (if configured)
   - Validation: Contains useful post-install instructions
   - Lines: ~30

6. **Write test: Chart dependencies validation**
   - Validation: If dependencies exist, Chart.lock present
   - Lines: ~25

7. **Write test: Kubernetes API version validation**
   - Validation: All resources use correct apiVersion (no deprecated APIs)
   - Example: `apiVersion: apps/v1` (not `extensions/v1beta1`)
   - Lines: ~40

8. **Write test: Label consistency validation**
   - Validation: All resources have consistent labels (app.kubernetes.io/*)
   - Lines: ~45

9. **Write test: Security validation**
   - Validation: No hardcoded secrets in templates
   - Validation: SecurityContext recommended (warning if missing)
   - Lines: ~50

10. **Write test: Helm lint (automated)**
    - Execute `helm lint` on all generated charts
    - Expected: All charts pass lint with no errors
    - Lines: ~30

**Time Estimate**:
- **Solo**: 10-12 hours (1.25-1.5 days)
- **Team (1 FTE)**: 7-9 hours (0.9-1.1 days)

**Dependencies**: Tasks 2.2-2.4 (uses their generated charts)

---

## Week 3 Summary

**Total Time Estimate**:
- **Solo**: 52-62 hours (6.5-7.75 days, ~1.3 weeks @ 5h/day effective)
- **Team (1 FTE)**: 35-45 hours (4.4-5.6 days, ~1 week @ 8h/day effective)

**Deliverables**:
- Integration test framework
- 5 integration test files (~1,150 lines)
- Test coverage: 65% → 75%
- All existing pipelines validated

---

## Week 4: E2E Tests & CI/CD (Days 16-20)

### Task 3.1: E2E Test Framework

**Goal**: Create E2E test framework with Helm CLI integration for production-like validation
**Result**: `tests/e2e/framework.go` with Helm CLI wrapper, K8s dry-run support
**Criteria** (FROZEN):
- [ ] All 7 subtasks completed
- [ ] Framework supports `helm lint`, `helm template`, `helm install --dry-run`
- [ ] Framework can run against real/mock K8s cluster
- [ ] All tests isolated (no cluster state pollution)

**Subtasks** (atomic):

1. **Create E2E test directory structure**
   - Path: `tests/e2e/`
   - Files: `framework.go`, `helm.go`, `kubernetes.go`
   - Lines: ~50

2. **Implement Helm CLI wrapper**
   - File: `tests/e2e/helm.go`
   - Functions: `HelmLint()`, `HelmTemplate()`, `HelmInstall()` (exec wrapper)
   - Lines: ~150

3. **Implement Kubernetes dry-run support**
   - File: `tests/e2e/kubernetes.go`
   - Functionality: Fake K8s API server OR connect to real cluster for `--dry-run=server`
   - Lines: ~120

4. **Implement E2E test harness**
   - File: `tests/e2e/framework.go`
   - Type: `E2ETestHarness` with setup/cleanup
   - Lines: ~100

5. **Create E2E test fixtures**
   - Path: `tests/e2e/fixtures/charts/`
   - Fixtures: Pre-generated charts for testing
   - Lines: ~300 (YAML)

6. **Write framework tests**
   - File: `tests/e2e/framework_test.go`
   - Meta-test the E2E framework
   - Lines: ~80

7. **Document E2E test framework**
   - File: `tests/e2e/README.md`
   - Lines: ~120

**Time Estimate**:
- **Solo**: 10-12 hours (1.25-1.5 days)
- **Team (1 FTE)**: 7-9 hours (0.9-1.1 days)

**Dependencies**: Task 2.5 (validated charts available)
**Blocks**: Tasks 3.2-3.4

---

### Task 3.2: Helm Lint Validation

**Goal**: Validate all generated charts pass `helm lint` with no errors
**Result**: `helm_lint_test.go` with 5+ test cases
**Criteria** (FROZEN):
- [ ] All 5 subtasks completed
- [ ] All test charts pass `helm lint`
- [ ] Lint warnings analyzed and addressed (or documented as acceptable)

**Subtasks** (atomic):

1. **Write test: Lint simple chart**
   - Input: Chart from Task 2.2
   - Execute: `helm lint`
   - Expected: Exit code 0, no errors
   - Lines: ~30

2. **Write test: Lint full-stack chart**
   - Input: Chart from Task 2.3
   - Execute: `helm lint`
   - Expected: Exit code 0
   - Lines: ~30

3. **Write test: Lint Deckhouse chart**
   - Input: Chart from Task 2.4
   - Execute: `helm lint`
   - Expected: Exit code 0
   - Lines: ~30

4. **Write test: Lint with values overrides**
   - Execute: `helm lint --values custom-values.yaml`
   - Expected: Chart accepts custom values, no errors
   - Lines: ~35

5. **Write test: Lint error detection**
   - Input: Intentionally broken chart (invalid Chart.yaml)
   - Expected: Lint FAILS, error message validated
   - Lines: ~30

**Time Estimate**:
- **Solo**: 4-5 hours (0.5-0.6 days)
- **Team (1 FTE)**: 3-4 hours (0.4-0.5 days)

**Dependencies**: Task 3.1

---

### Task 3.3: Helm Template Validation

**Goal**: Validate all generated charts render correctly with `helm template`
**Result**: `helm_template_test.go` with 8+ test cases
**Criteria** (FROZEN):
- [ ] All 8 subtasks completed
- [ ] All templates render without errors
- [ ] Rendered YAML validated for correct K8s resources

**Subtasks** (atomic):

1. **Write test: Template rendering (default values)**
   - Execute: `helm template mychart`
   - Expected: Valid YAML output, all resources present
   - Lines: ~40

2. **Write test: Template rendering (custom values)**
   - Execute: `helm template mychart --values custom.yaml`
   - Expected: Custom values applied, rendered correctly
   - Lines: ~45

3. **Write test: Conditional template blocks**
   - Input: Chart with conditional blocks (e.g., `{{ if .Values.ingress.enabled }}`)
   - Test: Render with `ingress.enabled=true` and `ingress.enabled=false`
   - Expected: Ingress present/absent based on value
   - Lines: ~50

4. **Write test: Helper template usage**
   - Validation: _helpers.tpl templates used in main templates
   - Expected: `{{ include "mychart.fullname" . }}` rendered correctly
   - Lines: ~40

5. **Write test: Template variable substitution**
   - Validation: All `{{ .Values.* }}` variables substituted
   - Expected: No `{{ }}` left in rendered output
   - Lines: ~35

6. **Write test: Multi-document YAML output**
   - Validation: `---` separators between resources
   - Expected: Each resource in separate YAML document
   - Lines: ~30

7. **Write test: Template rendering performance**
   - Measure: Time to render chart with 100+ templates
   - Expected: <2 seconds
   - Lines: ~35

8. **Write test: Template error handling**
   - Input: Invalid template syntax
   - Expected: Helm template fails with clear error message
   - Lines: ~30

**Time Estimate**:
- **Solo**: 8-10 hours (1-1.25 days)
- **Team (1 FTE)**: 5-7 hours (0.6-0.9 days)

**Dependencies**: Task 3.1

---

### Task 3.4: Helm Install (Dry-Run) Validation

**Goal**: Validate charts can be installed to K8s cluster (dry-run mode)
**Result**: `helm_install_test.go` with 6+ test cases
**Criteria** (FROZEN):
- [ ] All 6 subtasks completed
- [ ] All charts pass `helm install --dry-run`
- [ ] Server-side validation tested (K8s API validation)

**Subtasks** (atomic):

1. **Write test: Install dry-run (client-side)**
   - Execute: `helm install --dry-run --debug mychart`
   - Expected: Chart simulated install, no errors
   - Lines: ~40

2. **Write test: Install dry-run (server-side)**
   - Execute: `helm install --dry-run=server mychart`
   - Expected: K8s API validates resources, no errors
   - Requires: Mock K8s API or real cluster
   - Lines: ~50

3. **Write test: Install with namespace**
   - Execute: `helm install mychart --namespace mynamespace --dry-run`
   - Expected: Resources scoped to namespace
   - Lines: ~35

4. **Write test: Install with release name**
   - Execute: `helm install myrelease mychart --dry-run`
   - Expected: Release name used in resource names (via `{{ .Release.Name }}`)
   - Lines: ~40

5. **Write test: Install validation errors**
   - Input: Chart with invalid K8s resource (e.g., missing required field)
   - Expected: Server-side dry-run FAILS with K8s validation error
   - Lines: ~45

6. **Write test: Install with dependencies**
   - Input: Chart with dependencies (subcharts)
   - Execute: `helm install --dry-run`
   - Expected: All dependencies resolved and installed
   - Lines: ~50

**Time Estimate**:
- **Solo**: 10-12 hours (1.25-1.5 days)
- **Team (1 FTE)**: 7-9 hours (0.9-1.1 days)

**Dependencies**: Task 3.1

---

### Task 3.5: CI/CD Pipeline Integration

**Goal**: Automate all tests in CI/CD pipeline (GitHub Actions)
**Result**: `.github/workflows/test.yml` with complete test automation
**Criteria** (FROZEN):
- [ ] All 10 subtasks completed
- [ ] CI pipeline runs on every PR
- [ ] All tests (unit, integration, E2E) automated
- [ ] Coverage reporting integrated (codecov.io or similar)
- [ ] Pipeline runtime <10 minutes

**Subtasks** (atomic):

1. **Create GitHub Actions workflow file**
   - File: `.github/workflows/test.yml`
   - Trigger: `on: [push, pull_request]`
   - Lines: ~150

2. **Configure Go environment in CI**
   - Setup: Go version (1.21+), cache modules
   - Lines: ~20 (YAML)

3. **Add unit test job**
   - Job: `unit-tests`
   - Run: `go test -v -race -coverprofile=coverage.txt ./...`
   - Lines: ~30

4. **Add integration test job**
   - Job: `integration-tests`
   - Run: `go test -v ./tests/integration/...`
   - Dependencies: Unit tests pass
   - Lines: ~30

5. **Add E2E test job**
   - Job: `e2e-tests`
   - Setup: Install Helm CLI
   - Run: `go test -v ./tests/e2e/...`
   - Lines: ~40

6. **Add code coverage reporting**
   - Upload: `coverage.txt` to codecov.io
   - Badge: Add coverage badge to README.md
   - Lines: ~25

7. **Add linting job**
   - Job: `lint`
   - Run: `golangci-lint run`
   - Lines: ~25

8. **Add build job**
   - Job: `build`
   - Run: `go build -v ./cmd/dhg/`
   - Lines: ~20

9. **Add coverage gate**
   - Enforce: Fail CI if coverage <80%
   - Tool: `go test -cover` + threshold check
   - Lines: ~15

10. **Optimize CI performance**
    - Cache: Go modules, build cache
    - Parallel: Run jobs in parallel where possible
    - Target: <10 minutes total runtime
    - Lines: ~30 (optimization config)

**Time Estimate**:
- **Solo**: 8-10 hours (1-1.25 days)
- **Team (1 FTE)**: 5-7 hours (0.6-0.9 days)

**Dependencies**: Tasks 3.2-3.4 (all tests written)

---

## Week 4 Summary

**Total Time Estimate**:
- **Solo**: 40-49 hours (5-6.1 days, ~1.2 weeks @ 5h/day effective)
- **Team (1 FTE)**: 27-36 hours (3.4-4.5 days, ~0.9 weeks @ 8h/day effective)

**Deliverables**:
- E2E test framework
- 4 E2E test files (~790 lines)
- CI/CD pipeline fully automated
- Test coverage: 75% → 80%+
- Coverage badge on README

---

## Week 5-6: Critical K8s Processors (Days 21-30)

### Task 4.1: HorizontalPodAutoscaler Processor

**Goal**: Implement HPA processor with test-first approach, extract autoscaling configuration
**Result**: `hpa.go` + `hpa_test.go`, 80%+ coverage, handles HPA v2 API
**Criteria** (FROZEN):
- [ ] All 11 subtasks completed
- [ ] HPA processor extracts all v2 fields (metrics, behavior, scaleTargetRef)
- [ ] Test coverage ≥80%
- [ ] All tests PASS
- [ ] Supports CPU, memory, custom metrics

**Subtasks** (atomic):

1. **Write test: Extract scaleTargetRef**
   - Test case: `scaleTargetRef: {kind: Deployment, name: myapp}`
   - Expected: `values["autoscaling"]["targetRef"]`
   - Lines: ~35

2. **Write test: Extract min/max replicas**
   - Test case: `minReplicas: 2, maxReplicas: 10`
   - Expected: `values["autoscaling"]["minReplicas"]`, `maxReplicas`
   - Lines: ~30

3. **Write test: Extract CPU metric (v2)**
   - Test case: `metrics: [{type: Resource, resource: {name: cpu, target: {type: Utilization, averageUtilization: 80}}}]`
   - Expected: `values["autoscaling"]["metrics"]["cpu"]["targetAverageUtilization"]`
   - Lines: ~50

4. **Write test: Extract memory metric (v2)**
   - Test case: Memory resource metric
   - Expected: `values["autoscaling"]["metrics"]["memory"]`
   - Lines: ~45

5. **Write test: Extract custom metric**
   - Test case: `metrics: [{type: Pods, pods: {metric: {name: http_requests}, target: {type: AverageValue, averageValue: 1000}}}]`
   - Expected: `values["autoscaling"]["metrics"]["custom"]` array
   - Lines: ~60

6. **Write test: Extract external metric**
   - Test case: External metric (e.g., from Prometheus)
   - Expected: `values["autoscaling"]["metrics"]["external"]`
   - Lines: ~55

7. **Write test: Extract behavior (v2)**
   - Test case: `behavior: {scaleDown: {stabilizationWindowSeconds: 300, policies: [...]}}`
   - Expected: `values["autoscaling"]["behavior"]["scaleDown"]`
   - Lines: ~65

8. **Write test: Extract scaleUp behavior**
   - Test case: scaleUp policies (max change rate)
   - Expected: `values["autoscaling"]["behavior"]["scaleUp"]`
   - Lines: ~55

9. **Write test: Edge cases**
   - Test case: HPA without behavior (v1 compatible)
   - Test case: HPA with only CPU metric (simple case)
   - Lines: ~40

10. **Implement HPA processor**
    - File: `pkg/processor/k8s/hpa.go`
    - Implement: `ProcessHPA(hpa *autoscalingv2.HorizontalPodAutoscaler) map[string]interface{}`
    - Lines: ~350

11. **Verify tests pass + coverage**
    - Run tests → all PASS
    - Check coverage ≥80%

**Time Estimate**:
- **Solo**: 12-14 hours (1.5-1.75 days)
- **Team (1 FTE)**: 8-10 hours (1-1.25 days)

**TDD Workflow**:
- Subtasks 1-9: Write tests FIRST
- Subtask 10: Implement processor to pass tests
- Subtask 11: Verify

**Dependencies**: Task 1.1 (testutil)

---

### Task 4.2: PodDisruptionBudget Processor

**Goal**: Implement PDB processor with test-first approach, extract disruption budget config
**Result**: `pdb.go` + `pdb_test.go`, 80%+ coverage
**Criteria** (FROZEN):
- [ ] All 9 subtasks completed
- [ ] PDB processor extracts minAvailable, maxUnavailable, selectors
- [ ] Test coverage ≥80%
- [ ] All tests PASS

**Subtasks** (atomic):

1. **Write test: Extract minAvailable (integer)**
   - Test case: `minAvailable: 2`
   - Expected: `values["podDisruptionBudget"]["minAvailable"]`
   - Lines: ~30

2. **Write test: Extract minAvailable (percentage)**
   - Test case: `minAvailable: "50%"`
   - Expected: `values["podDisruptionBudget"]["minAvailable"] == "50%"`
   - Lines: ~35

3. **Write test: Extract maxUnavailable (integer)**
   - Test case: `maxUnavailable: 1`
   - Expected: `values["podDisruptionBudget"]["maxUnavailable"]`
   - Lines: ~30

4. **Write test: Extract maxUnavailable (percentage)**
   - Test case: `maxUnavailable: "25%"`
   - Expected: `values["podDisruptionBudget"]["maxUnavailable"] == "25%"`
   - Lines: ~35

5. **Write test: Extract selector**
   - Test case: `selector: {matchLabels: {app: myapp}}`
   - Expected: `values["podDisruptionBudget"]["selector"]`
   - Lines: ~40

6. **Write test: Extract unhealthyPodEvictionPolicy**
   - Test case: `unhealthyPodEvictionPolicy: AlwaysAllow`
   - Expected: `values["podDisruptionBudget"]["unhealthyPodEvictionPolicy"]`
   - Lines: ~30

7. **Write test: Edge cases**
   - Test case: PDB with neither minAvailable nor maxUnavailable (invalid)
   - Test case: PDB with both minAvailable AND maxUnavailable (pick one)
   - Lines: ~40

8. **Implement PDB processor**
   - File: `pkg/processor/k8s/pdb.go`
   - Lines: ~280

9. **Verify tests pass + coverage**

**Time Estimate**:
- **Solo**: 8-10 hours (1-1.25 days)
- **Team (1 FTE)**: 5-7 hours (0.6-0.9 days)

**Dependencies**: Task 1.1

---

### Task 4.3: NetworkPolicy Processor

**Goal**: Implement NetworkPolicy processor, extract ingress/egress rules
**Result**: `networkpolicy.go` + `networkpolicy_test.go`, 80%+ coverage
**Criteria** (FROZEN):
- [ ] All 12 subtasks completed
- [ ] NetworkPolicy processor extracts all rule types (ingress, egress)
- [ ] Test coverage ≥80%
- [ ] All tests PASS

**Subtasks** (atomic):

1. **Write test: Extract podSelector**
   - Test case: `podSelector: {matchLabels: {role: db}}`
   - Expected: `values["networkPolicy"]["podSelector"]`
   - Lines: ~35

2. **Write test: Extract policyTypes**
   - Test case: `policyTypes: [Ingress, Egress]`
   - Expected: `values["networkPolicy"]["policyTypes"]`
   - Lines: ~30

3. **Write test: Extract ingress rule (from pods)**
   - Test case: `ingress: [{from: [{podSelector: {matchLabels: {role: frontend}}}], ports: [{protocol: TCP, port: 5432}]}]`
   - Expected: `values["networkPolicy"]["ingress"][0]["from"]`, `ports`
   - Lines: ~60

4. **Write test: Extract ingress rule (from namespaces)**
   - Test case: `from: [{namespaceSelector: {matchLabels: {name: prod}}}]`
   - Expected: `values["networkPolicy"]["ingress"][0]["from"]` with namespace selector
   - Lines: ~50

5. **Write test: Extract ingress rule (from IP blocks)**
   - Test case: `from: [{ipBlock: {cidr: 10.0.0.0/16, except: [10.0.1.0/24]}}]`
   - Expected: `values["networkPolicy"]["ingress"][0]["from"]` with CIDR
   - Lines: ~50

6. **Write test: Extract egress rule (to pods)**
   - Test case: `egress: [{to: [{podSelector: ...}]}]`
   - Expected: `values["networkPolicy"]["egress"][0]["to"]`
   - Lines: ~55

7. **Write test: Extract egress rule (to IP blocks)**
   - Test case: `to: [{ipBlock: {cidr: 0.0.0.0/0}}]` (allow all egress)
   - Expected: `values["networkPolicy"]["egress"][0]["to"]`
   - Lines: ~45

8. **Write test: Extract port specifications**
   - Test case: Multiple ports with protocols (TCP 80, TCP 443, UDP 53)
   - Expected: `values["networkPolicy"]["ingress"][0]["ports"]` array
   - Lines: ~50

9. **Write test: Default-deny policy**
   - Test case: Empty ingress/egress (deny all)
   - Expected: `values["networkPolicy"]["ingress"] == []`
   - Lines: ~35

10. **Write test: Edge cases**
    - Test case: NetworkPolicy without policyTypes (defaults to Ingress)
    - Lines: ~30

11. **Implement NetworkPolicy processor**
    - File: `pkg/processor/k8s/networkpolicy.go`
    - Lines: ~400

12. **Verify tests pass + coverage**

**Time Estimate**:
- **Solo**: 14-16 hours (1.75-2 days)
- **Team (1 FTE)**: 10-12 hours (1.25-1.5 days)

**Dependencies**: Task 1.1

---

### Task 4.4: CronJob Processor

**Goal**: Implement CronJob processor, extract schedule and job template
**Result**: `cronjob.go` + `cronjob_test.go`, 80%+ coverage
**Criteria** (FROZEN):
- [ ] All 11 subtasks completed
- [ ] CronJob processor extracts schedule, jobTemplate, concurrency policy
- [ ] Test coverage ≥80%
- [ ] All tests PASS

**Subtasks** (atomic):

1. **Write test: Extract schedule**
   - Test case: `schedule: "0 2 * * *"` (daily 2 AM)
   - Expected: `values["cronjob"]["schedule"]`
   - Lines: ~30

2. **Write test: Extract timezone (K8s 1.25+)**
   - Test case: `timeZone: "America/New_York"`
   - Expected: `values["cronjob"]["timeZone"]`
   - Lines: ~30

3. **Write test: Extract concurrencyPolicy**
   - Test case: `concurrencyPolicy: Forbid`
   - Expected: `values["cronjob"]["concurrencyPolicy"]`
   - Lines: ~30

4. **Write test: Extract suspend flag**
   - Test case: `suspend: true`
   - Expected: `values["cronjob"]["suspend"]`
   - Lines: ~25

5. **Write test: Extract successfulJobsHistoryLimit**
   - Test case: `successfulJobsHistoryLimit: 3`
   - Expected: `values["cronjob"]["successfulJobsHistoryLimit"]`
   - Lines: ~25

6. **Write test: Extract failedJobsHistoryLimit**
   - Test case: `failedJobsHistoryLimit: 1`
   - Expected: `values["cronjob"]["failedJobsHistoryLimit"]`
   - Lines: ~25

7. **Write test: Extract startingDeadlineSeconds**
   - Test case: `startingDeadlineSeconds: 300`
   - Expected: `values["cronjob"]["startingDeadlineSeconds"]`
   - Lines: ~30

8. **Write test: Extract jobTemplate spec**
   - Test case: `jobTemplate.spec` (completions, parallelism, backoffLimit)
   - Expected: `values["cronjob"]["jobTemplate"]` with Job fields
   - Lines: ~60

9. **Write test: Extract pod template from jobTemplate**
   - Test case: `jobTemplate.spec.template` (container spec)
   - Expected: `values["cronjob"]["podTemplate"]` with image, resources
   - Lines: ~65

10. **Implement CronJob processor**
    - File: `pkg/processor/k8s/cronjob.go`
    - Lines: ~320

11. **Verify tests pass + coverage**

**Time Estimate**:
- **Solo**: 10-12 hours (1.25-1.5 days)
- **Team (1 FTE)**: 7-9 hours (0.9-1.1 days)

**Dependencies**: Task 1.1

---

### Task 4.5: Job Processor

**Goal**: Implement Job processor, extract job completion/parallelism config
**Result**: `job.go` + `job_test.go`, 80%+ coverage
**Criteria** (FROZEN):
- [ ] All 10 subtasks completed
- [ ] Job processor extracts completions, parallelism, backoffLimit, TTL
- [ ] Test coverage ≥80%
- [ ] All tests PASS

**Subtasks** (atomic):

1. **Write test: Extract completions**
   - Test case: `completions: 5`
   - Expected: `values["job"]["completions"]`
   - Lines: ~30

2. **Write test: Extract parallelism**
   - Test case: `parallelism: 3`
   - Expected: `values["job"]["parallelism"]`
   - Lines: ~30

3. **Write test: Extract backoffLimit**
   - Test case: `backoffLimit: 4`
   - Expected: `values["job"]["backoffLimit"]`
   - Lines: ~30

4. **Write test: Extract activeDeadlineSeconds**
   - Test case: `activeDeadlineSeconds: 600` (10 minutes timeout)
   - Expected: `values["job"]["activeDeadlineSeconds"]`
   - Lines: ~30

5. **Write test: Extract ttlSecondsAfterFinished**
   - Test case: `ttlSecondsAfterFinished: 3600` (cleanup after 1 hour)
   - Expected: `values["job"]["ttl"]`
   - Lines: ~30

6. **Write test: Extract completionMode**
   - Test case: `completionMode: Indexed` (vs NonIndexed)
   - Expected: `values["job"]["completionMode"]`
   - Lines: ~30

7. **Write test: Extract suspend flag**
   - Test case: `suspend: true`
   - Expected: `values["job"]["suspend"]`
   - Lines: ~25

8. **Write test: Extract pod template**
   - Test case: `template.spec` with restartPolicy
   - Expected: `values["job"]["podTemplate"]` with restartPolicy
   - Lines: ~60

9. **Write test: Edge cases**
   - Test case: Job without completions (runs until success)
   - Test case: Job with parallelism > completions (warning)
   - Lines: ~35

10. **Implement Job processor**
    - File: `pkg/processor/k8s/job.go`
    - Lines: ~280

**Time Estimate**:
- **Solo**: 8-10 hours (1-1.25 days)
- **Team (1 FTE)**: 5-7 hours (0.6-0.9 days)

**Dependencies**: Task 1.1

---

### Task 4.6: Integration Tests for New Processors

**Goal**: Write integration tests for HPA, PDB, NetworkPolicy, CronJob, Job processors
**Result**: `pipeline_autoscaling_test.go`, `pipeline_security_test.go`, `pipeline_batch_test.go`
**Criteria** (FROZEN):
- [ ] All 6 subtasks completed
- [ ] Integration tests cover new processors in full pipeline
- [ ] All tests PASS

**Subtasks** (atomic):

1. **Write test: Pipeline with HPA**
   - Input: Deployment + HPA
   - Expected: HPA template generated, references Deployment
   - Lines: ~50

2. **Write test: Pipeline with PDB**
   - Input: Deployment + PDB
   - Expected: PDB template generated, selector matches Deployment
   - Lines: ~45

3. **Write test: Pipeline with NetworkPolicy**
   - Input: Deployment + Service + NetworkPolicy
   - Expected: NetworkPolicy template, rules reference Service
   - Lines: ~60

4. **Write test: Pipeline with CronJob**
   - Input: CronJob + ConfigMap
   - Expected: CronJob template, ConfigMap mounted
   - Lines: ~55

5. **Write test: Pipeline with Job**
   - Input: Job + Secret
   - Expected: Job template, Secret reference
   - Lines: ~50

6. **Write test: Complex scenario**
   - Input: Deployment + HPA + PDB + NetworkPolicy (all together)
   - Expected: All templates generated, relationships correct
   - Lines: ~70

**Time Estimate**:
- **Solo**: 8-10 hours (1-1.25 days)
- **Team (1 FTE)**: 5-7 hours (0.6-0.9 days)

**Dependencies**: Tasks 4.1-4.5 (processors implemented)

---

## Week 5-6 Summary

**Total Time Estimate**:
- **Solo**: 68-82 hours (8.5-10.25 days, ~2 weeks @ 5h/day effective)
- **Team (1 FTE)**: 45-57 hours (5.6-7.1 days, ~1.4 weeks @ 8h/day effective)

**Deliverables**:
- 5 new K8s processors (HPA, PDB, NetworkPolicy, CronJob, Job)
- 5 test files (~1,630 lines tests + ~1,630 lines implementation)
- Integration tests for new processors
- Test coverage maintained at 80%+

---

## Week 7: RBAC Processors (Days 31-35)

### Task 5.1-5.4: Role, ClusterRole, RoleBinding, ClusterRoleBinding Processors

**Goal**: Implement all 4 RBAC processors with test-first approach
**Result**: 4 processors + 4 test files, 80%+ coverage each
**Criteria** (FROZEN):
- [ ] All RBAC processors extract rules, subjects, roleRefs
- [ ] Test coverage ≥80% for each
- [ ] All tests PASS
- [ ] Integration test for RBAC chain

**Time Estimate** (all 4 tasks):
- **Solo**: 28-32 hours (3.5-4 days)
- **Team (1 FTE)**: 18-22 hours (2.25-2.75 days)

*(Detailed subtasks omitted for brevity — follow same pattern as Tasks 4.1-4.5)*

---

## Week 8: Polish & Release (Days 36-40)

### Task 6.1: Documentation Updates

**Goal**: Update all documentation to reflect v0.2.0 changes
**Result**: README.md, CHANGELOG.md, examples/ updated
**Criteria** (FROZEN):
- [ ] README.md lists all 14 processors
- [ ] CHANGELOG.md documents all changes
- [ ] examples/ has 5 example YAML sets
- [ ] Coverage badge shows 80%+

**Time Estimate**:
- **Solo**: 6-8 hours (0.75-1 day)
- **Team (1 FTE)**: 4-5 hours (0.5-0.6 days)

---

### Task 6.2: Performance Benchmarks

**Goal**: Create benchmark suite for performance regression detection
**Result**: `benchmark_test.go` with benchmarks for large manifests
**Criteria** (FROZEN):
- [ ] Benchmarks for 10/100/1000 resource generation
- [ ] Baseline performance recorded
- [ ] Benchmark runs in CI (informational)

**Time Estimate**:
- **Solo**: 6-8 hours (0.75-1 day)
- **Team (1 FTE)**: 4-5 hours (0.5-0.6 days)

---

### Task 6.3: Release v0.2.0

**Goal**: Tag and release v0.2.0 with all deliverables
**Result**: GitHub release, Docker image, changelog
**Criteria** (FROZEN):
- [ ] Git tag `v0.2.0` created
- [ ] GitHub release published
- [ ] Docker image published to registry
- [ ] Announcement prepared

**Time Estimate**:
- **Solo**: 4-6 hours (0.5-0.75 days)
- **Team (1 FTE)**: 3-4 hours (0.4-0.5 days)

---

## Week 8 Summary

**Total Time Estimate**:
- **Solo**: 16-22 hours (2-2.75 days, ~0.6 weeks @ 5h/day effective)
- **Team (1 FTE)**: 11-14 hours (1.4-1.75 days, ~0.4 weeks @ 8h/day effective)

---

## v0.2.0 Grand Total

**Total Time Estimate (8 weeks)**:
- **Solo**: 290-353 hours (36.25-44.1 days, ~7.25-8.8 weeks @ 5h/day effective)
- **Team (1 FTE)**: 192-244 hours (24-30.5 days, ~4.8-6.1 weeks @ 8h/day effective)

**Velocity Comparison**:
- Solo vs Team: 1.5x-1.45x speedup (team is 1.5x faster)
- Actual velocity will be tracked and compared post-release

---

## Appendix: Velocity Tracking Template

After each task, record actual time in `docs/velocity.md`:

```markdown
## v0.2.0 Velocity Tracking

| Task | Estimated (Solo) | Actual | Velocity | Notes |
|------|------------------|--------|----------|-------|
| 1.1 | 16-20h | Xh | Y | [Learnings, blockers] |
| 1.2 | 12-16h | Xh | Y | |
| ... | | | | |

**Average Velocity**: Z
**Adjustment Factor for v0.3.0**: 1/Z
```

---

## Future Releases (Summary)

### v0.3.0 - v1.0.0 (Planned)

*(Detailed Goal-Result-Criteria for future releases will be created after v0.2.0 velocity is established and can inform accurate estimates.)*

**v0.3.0** (Q3 2026, 8 weeks): Core expansion — Remaining K8s processors, Cluster/GitOps extractors, Generator modes
**v0.4.0** (Q4 2026, 6 weeks): Deckhouse integration — Deckhouse CRD processors
**v0.5.0** (Q1 2027, 8 weeks): Production patterns — Monitoring, GitOps, CI/CD
**v0.7.0** (Q2-Q3 2027, 12 weeks): Enterprise features — Multi-tenancy, secret strategies
**v0.8.0** (Q4 2027, 8 weeks): Advanced workloads — AI/ML, databases
**v0.9.0** (Q1 2028, 8 weeks): Edge & optimization — Scheduling, CSI, Edge
**v1.0.0** (Q2 2028, 4 weeks): GA release — Polish, documentation, performance

---

## Conclusion

This implementation plan provides:
- **Goal-Result-Criteria framework** for every task
- **Atomic decomposition** (13-16 subtasks per major task)
- **Dual estimation** (solo vs team) for velocity comparison
- **TDD approach** enforced (tests first, code second)
- **Velocity tracking** for continuous improvement

**Next Step**: Begin Task 1.1 (Test Utilities & Framework) — Week 1, Day 1

**Critical Success Factor**: Maintain TDD discipline and velocity tracking throughout v0.2.0 to inform all future estimates.
