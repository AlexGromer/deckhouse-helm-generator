# Deckhouse Helm Generator — Implementation Plan v4.0

**Version**: 4.0.0 (Goal-Result-Criteria Framework)
**Created**: 2026-02-18
**Status**: Pending
**Methodology**: TDD + Atomic Decomposition + Velocity-Calibrated Estimation
**Predecessor**: IMPLEMENTATION_PLAN_v3.md (v0.3.0 — COMPLETED 2026-02-18)

---

## Velocity Calibration

### v0.3.0 Actuals

| Metric | Value |
|--------|-------|
| Tasks completed | 15/15 |
| Estimated (solo) | 141h |
| Actual (AI-assisted) | ~4.95h (~approx) |
| Average velocity | ~28.5x (~approx) |
| Sessions used | 3 |

### Estimation Model for v0.4.0

Все оценки используют **dual format**:
- **Solo (human)**: сложность задачи по паттернам из v0.2.0/v0.3.0
- **AI-assisted**: `solo_estimate / 25` (используем v0.3.0 velocity ~28.5x, консервативно 25x)

**Note**: Deckhouse CRD processors — паттерн аналогичен v0.2.0 K8s processors (~3-4h solo каждый).
Module scaffold и OpenAPI generation — новые паттерны, может быть медленнее (~1.5-2x overhead).

### Time Tracking Protocol (ОБЯЗАТЕЛЬНО)

**Каждая задача** — выполнить **до первой подзадачи** и **после финального `go test ./...`**:

```bash
date -u +"%Y-%m-%d %H:%M:%S UTC"
```

Записать оба значения в `docs/velocity_v0.4.0.md`. Duration = end - start.

**Формат строки:**

```
| 1.1 | ModuleConfig + IngressNginxController | 10.0h | 0.40h | <actual>h | <velocity>x | <START_UTC> | <END_UTC> |
```

---

## Phase 1: Deckhouse Core CRDs (Tasks 1.1–1.5)

**Тема**: Полная поддержка Deckhouse-специфичных CRD ресурсов
**Prerequisite**: v0.3.0 COMPLETED

---

### Task 1.1: ModuleConfig + IngressNginxController Processors

**Goal**: Добавить поддержку двух наиболее используемых Deckhouse CRD
**Result**: 2 процессора + тесты + интеграция в пайплайн
**Criteria** (FROZEN):
- [ ] Все 8 подзадач выполнены
- [ ] `ModuleConfigProcessor` обрабатывает `deckhouse.io/v1alpha1 ModuleConfig`
- [ ] Извлекает: `enabled`, `version`, `settings` → шаблонизируется как values
- [ ] `IngressNginxControllerProcessor` обрабатывает `deckhouse.io/v1 IngressNginxController`
- [ ] Извлекает: `inlet`, `controllerVersion`, `customErrors`, `resourcesRequests`
- [ ] Оба процессора зарегистрированы в `DefaultProcessorRegistry()`
- [ ] Coverage ≥80% для новых файлов
- [ ] `go test ./...` — все тесты прошли

**Subtasks** (atomic):

1. **Написать тесты ModuleConfigProcessor** (TDD: сначала тесты)
   - `TestModuleConfigProcessor_Kind` → "ModuleConfig"
   - `TestModuleConfigProcessor_Enabled` → `enabled: true/false` в values
   - `TestModuleConfigProcessor_Version` → `version: "1.2.0"` → значение
   - `TestModuleConfigProcessor_Settings` → `settings` map → values вложенная структура
   - `TestModuleConfigProcessor_EmptySettings` → graceful nil handling
   - Файл: `pkg/processor/k8s/moduleconfig_test.go`

2. **Написать тесты IngressNginxControllerProcessor** (TDD)
   - `TestIngressNginxControllerProcessor_Kind` → "IngressNginxController"
   - `TestIngressNginxControllerProcessor_Inlet` → LoadBalancer/HostPort/HostWithFailover
   - `TestIngressNginxControllerProcessor_ControllerVersion` → версия контроллера
   - `TestIngressNginxControllerProcessor_CustomErrors` → configmap ссылки
   - `TestIngressNginxControllerProcessor_ResourcesRequests` → cpu/memory запросы
   - Файл: `pkg/processor/k8s/ingressnginxcontroller_test.go`

3. **Запустить тесты → ожидаем FAIL** (нет реализации)

4. **Реализовать ModuleConfigProcessor**
   - Файл: `pkg/processor/k8s/moduleconfig.go`
   - Структура: `type ModuleConfigProcessor struct{}`
   - Методы: `Kind() string`, `Process(*unstructured.Unstructured) (*types.ProcessedResource, error)`
   - Settings: рекурсивный map → Helm values

5. **Реализовать IngressNginxControllerProcessor**
   - Файл: `pkg/processor/k8s/ingressnginxcontroller.go`
   - Inlet enum: LoadBalancer, HostPort, HostWithFailover
   - Шаблон: генерировать условные values (`{{ if eq .values.inlet "LoadBalancer" }}...`)

6. **Зарегистрировать оба процессора**
   - Добавить в `DefaultProcessorRegistry()` в `pkg/processor/k8s/registry.go`

7. **Запустить тесты → ожидаем PASS**
   - `go test ./pkg/processor/k8s/ -run TestModuleConfig -v`
   - `go test ./pkg/processor/k8s/ -run TestIngressNginx -v`

8. **Верифицировать coverage + полный регресс**
   - `go test ./pkg/processor/k8s/ -cover` → ≥80%
   - `go test ./...`

**Time Estimate**:
- **Solo**: 9-11h
- **AI-assisted**: 0.36-0.44h

**TDD Workflow**: Subtasks 1-2 (тесты) → Subtask 3 (ожидаем FAIL) → Subtasks 4-6 (реализация) → Subtask 7 (PASS) → Subtask 8 (верификация)

---

### Task 1.2: ClusterAuthorizationRule + NodeGroup Processors

**Goal**: Добавить поддержку RBAC и управления узлами Deckhouse
**Result**: 2 процессора + тесты
**Criteria** (FROZEN):
- [ ] Все 8 подзадач выполнены
- [ ] `ClusterAuthorizationRuleProcessor` извлекает: `subjects`, `accessLevel`, `namespaces`, `allowScale`
- [ ] `accessLevel` enum: User/PrivilegedUser/Editor/Admin/ClusterEditor/ClusterAdmin
- [ ] `NodeGroupProcessor` извлекает: `nodeType`, `disruptions`, `kubelet`, `cloudInstances`
- [ ] `nodeType` enum: CloudEphemeral/CloudPermanent/Static
- [ ] `cloudInstances.minPerZone`, `cloudInstances.maxPerZone` в values
- [ ] Coverage ≥80%, `go test ./...` pass

**Subtasks** (atomic, аналогичны Task 1.1):

1. Тесты ClusterAuthorizationRuleProcessor: Kind, Subjects, AccessLevel enum (6 уровней), Namespaces, AllowScale
2. Тесты NodeGroupProcessor: Kind, NodeType enum (3 типа), Disruptions (automatic/manual), CloudInstances min/max
3. Запустить → FAIL
4. Реализовать ClusterAuthorizationRuleProcessor (`pkg/processor/k8s/clusterauthorizationrule.go`)
5. Реализовать NodeGroupProcessor (`pkg/processor/k8s/nodegroup.go`)
6. Зарегистрировать оба процессора
7. Запустить → PASS
8. Coverage + регресс

**Time Estimate**:
- **Solo**: 9-11h
- **AI-assisted**: 0.36-0.44h

---

### Task 1.3: DexAuthenticator + User/Group Processors

**Goal**: Добавить SSO и identity management ресурсы Deckhouse
**Result**: 3 процессора (DexAuthenticator, User, Group) + тесты
**Criteria** (FROZEN):
- [ ] Все 8 подзадач выполнены
- [ ] `DexAuthenticatorProcessor`: `applicationDomain`, `sendAuthorizationHeader`, `applicationIngressClassName`
- [ ] Шаблон: аннотации Ingress для nginx-authentik интеграции
- [ ] `UserProcessor`: `email`, `password` (bcrypt hash → _не_ в values, в Secret ref), `groups`, `ttl`
- [ ] `GroupProcessor`: `members` list
- [ ] Sensitive data (password hash) → `values.userPasswordHash` с Helm секрет-паттерном
- [ ] Coverage ≥80%, `go test ./...` pass

**Subtasks** (аналогичны 1.1):

1. Тесты DexAuthenticatorProcessor: Kind, Domain, SendAuthorizationHeader, IngressClassName
2. Тесты UserProcessor: Kind, Email, Password(secret), Groups, TTL
3. Тесты GroupProcessor: Kind, Members list
4. Запустить → FAIL
5. Реализовать все 3 процессора
6. Зарегистрировать
7. Запустить → PASS
8. Coverage + регресс

**Time Estimate**:
- **Solo**: 10-12h
- **AI-assisted**: 0.40-0.48h

---

### Task 1.4: Deckhouse Module Structure (Scaffold + lib-helm + OpenAPI)

**Goal**: Флаг `--output-format deckhouse-module` генерирует валидную структуру Deckhouse модуля
**Result**: Новый output формат с `openapi/`, `images/`, `hooks/` placeholder, lib-helm dependency
**Criteria** (FROZEN):
- [ ] Все 10 подзадач выполнены
- [ ] `--output-format deckhouse-module` флаг добавлен в CLI
- [ ] Генерирует `openapi/config-values.yaml` и `openapi/values.yaml` (Deckhouse JSON Schema формат)
- [ ] Добавляет `charts/helm_lib/` как dependency placeholder в `Chart.yaml`
- [ ] Использует helm_lib хелперы в templates: `helm_lib_module_labels`, `helm_lib_module_image`
- [ ] Генерирует `images/` директорию (placeholder)
- [ ] Генерирует `hooks/` директорию с README placeholder
- [ ] `helm lint` проходит на сгенерированной структуре
- [ ] Coverage ≥80%
- [ ] `go test ./...` pass

**Subtasks** (atomic):

1. **Тесты: ModuleScaffoldGenerator** (TDD)
   - `TestModuleScaffold_ChartYAML_Type` → `type: library` для lib-helm совместимости
   - `TestModuleScaffold_OpenAPIDir` → наличие `openapi/config-values.yaml`
   - `TestModuleScaffold_ImagesDir` → наличие `images/` placeholder
   - `TestModuleScaffold_HooksDir` → наличие `hooks/README.md`
   - `TestModuleScaffold_HelmLibDependency` → `charts/helm_lib` в `Chart.yaml`

2. **Тесты: OpenAPI Schema Generator** (TDD)
   - `TestOpenAPISchema_ConfigValues_Structure` → корректная Deckhouse JSON Schema структура
   - `TestOpenAPISchema_ValuesMapping` → Go values map → OpenAPI properties
   - `TestOpenAPISchema_RequiredFields` → `x-doc-required` аннотации

3. **Тесты: helm_lib template helpers**
   - `TestHelmLibHelpers_ModuleLabels` → `{{ include "helm_lib_module_labels" . }}` в templates
   - `TestHelmLibHelpers_ModuleImage` → `{{ include "helm_lib_module_image" . "component" }}` паттерн

4. **Запустить → FAIL**

5. **Реализовать ModuleScaffoldGenerator** (`pkg/generator/modulescaffold.go`)

6. **Реализовать OpenAPI Schema Generator** (`pkg/generator/openapi.go`)

7. **Обновить helm_lib template patterns** в Library generator

8. **Добавить `--output-format` CLI flag** (`cmd/dhg/`)

9. **Запустить → PASS**

10. **`helm lint` + coverage + полный регресс**

**Time Estimate**:
- **Solo**: 14-18h (новый паттерн: OpenAPI генерация)
- **AI-assisted**: 0.56-0.72h

---

### Task 1.5: Deckhouse Pattern Detection

**Goal**: Автодетекция Deckhouse-кластера по CRD и умные рекомендации
**Result**: `DeckhouseDetector` + рекомендации в output + version compatibility check
**Criteria** (FROZEN):
- [ ] Все 6 подзадач выполнены
- [ ] Детектор находит Deckhouse если есть ModuleConfig или IngressNginxController в input
- [ ] При детекции: рекомендует `--output-format deckhouse-module`
- [ ] Проверяет `apiVersion` против поддерживаемых версий Deckhouse (1.57+)
- [ ] Warning при deprecated CRD fields
- [ ] Coverage ≥80%, `go test ./...` pass

**Subtasks** (atomic):

1. Тесты: `TestDeckhouseDetector_Detected` → ModuleConfig input → is_deckhouse=true
2. Тесты: `TestDeckhouseDetector_NotDetected` → only K8s resources → is_deckhouse=false
3. Тесты: `TestDeckhouseDetector_VersionCheck` → старый apiVersion → warning
4. Запустить → FAIL
5. Реализовать `DeckhouseDetector` (`pkg/analyzer/detector/deckhouse.go`)
6. Запустить → PASS + coverage + регресс

**Time Estimate**:
- **Solo**: 7-9h
- **AI-assisted**: 0.28-0.36h

---

## Phase 2: Monitoring & Modern K8s (Tasks 2.1–2.5)

**Тема**: Prometheus Operator + Gateway API + KEDA + cert-manager

---

### Task 2.1: Monitoring Processors (ServiceMonitor + PodMonitor + PrometheusRule + GrafanaDashboard)

**Goal**: Полная поддержка Prometheus Operator CRDs
**Result**: 4 процессора + тесты
**Criteria** (FROZEN):
- [ ] Все 8 подзадач выполнены
- [ ] `ServiceMonitorProcessor`: `endpoints` (path, interval, scrapeTimeout), `namespaceSelector`, `selector`
- [ ] `PodMonitorProcessor`: аналогично ServiceMonitor, но для подов
- [ ] `PrometheusRuleProcessor`: `groups`, `rules` (alert, expr, for, labels, annotations), шаблонизация `expr`
- [ ] `GrafanaDashboardProcessor`: ConfigMap с label `grafana_dashboard: "1"` → `files/` директория
- [ ] Coverage ≥80%, `go test ./...` pass

**Subtasks** (TDD-аналог задач 1.x):

1. Тесты ServiceMonitorProcessor (Kind, Endpoints, NamespaceSelector, Selector)
2. Тесты PodMonitorProcessor (Kind, PodMetricsEndpoints, JobLabel)
3. Тесты PrometheusRuleProcessor (Kind, Groups, Rules expr шаблонизация)
4. Тесты GrafanaDashboardProcessor (Kind, Label detection, files/ output)
5. Запустить → FAIL
6. Реализовать все 4 процессора
7. Запустить → PASS
8. Coverage + регресс

**Time Estimate**:
- **Solo**: 12-14h
- **AI-assisted**: 0.48-0.56h

---

### Task 2.2: Gateway API Processors (HTTPRoute + Gateway)

**Goal**: Поддержка Gateway API как замены Ingress
**Result**: 2 процессора + шаблоны + детекция связей
**Criteria** (FROZEN):
- [ ] Все 7 подзадач выполнены
- [ ] `HTTPRouteProcessor`: `parentRefs`, `hostnames`, `rules` (matches, backendRefs, filters)
- [ ] `GatewayProcessor`: `gatewayClassName`, `listeners` (port, protocol, hostname, tls)
- [ ] Детекция связей: HTTPRoute → Gateway (через parentRef)
- [ ] Условные шаблоны: `{{ if .values.gatewayAPI.enabled }}` vs Ingress
- [ ] Coverage ≥80%, `go test ./...` pass

**Subtasks** (TDD):

1. Тесты HTTPRouteProcessor (Kind, ParentRefs, Hostnames, Rules)
2. Тесты GatewayProcessor (Kind, GatewayClassName, Listeners)
3. Тесты связи HTTPRoute→Gateway
4. Запустить → FAIL
5. Реализовать оба процессора
6. Зарегистрировать + связи
7. Запустить → PASS + coverage + регресс

**Time Estimate**:
- **Solo**: 10-12h
- **AI-assisted**: 0.40-0.48h

---

### Task 2.3: KEDA Processors (ScaledObject + TriggerAuthentication)

**Goal**: Поддержка event-driven autoscaling KEDA
**Result**: 2 процессора + связи с Deployment/StatefulSet
**Criteria** (FROZEN):
- [ ] Все 7 подзадач выполнены
- [ ] `ScaledObjectProcessor`: `scaleTargetRef`, `triggers` (type, metadata), `minReplicaCount` (scale-to-zero!), `maxReplicaCount`
- [ ] `TriggerAuthenticationProcessor`: `secretTargetRef`, `env`, `podIdentity`
- [ ] Связь: ScaledObject → Deployment/StatefulSet через `scaleTargetRef`
- [ ] scale-to-zero: `minReplicaCount: 0` → добавляет аннотации в template
- [ ] Coverage ≥80%, `go test ./...` pass

**Subtasks** (TDD, аналог):

1. Тесты ScaledObjectProcessor (Kind, ScaleTargetRef, Triggers, MinZero)
2. Тесты TriggerAuthenticationProcessor (Kind, SecretTargetRef, PodIdentity)
3. Тесты связи ScaledObject→Deployment
4. Запустить → FAIL
5. Реализовать оба процессора
6. Связи + регистрация
7. Запустить → PASS + coverage + регресс

**Time Estimate**:
- **Solo**: 9-11h
- **AI-assisted**: 0.36-0.44h

---

### Task 2.4: cert-manager Processors (Certificate + ClusterIssuer)

**Goal**: Поддержка cert-manager TLS automation
**Result**: 2 процессора + детекция аннотаций в Ingress
**Criteria** (FROZEN):
- [ ] Все 7 подзадач выполнены
- [ ] `CertificateProcessor`: `dnsNames`, `issuerRef` (name, kind), `secretName`, `duration`
- [ ] `ClusterIssuerProcessor`: `acme` (server, email, solvers), `selfSigned`
- [ ] Детекция Ingress аннотации `cert-manager.io/cluster-issuer` → связь с ClusterIssuer
- [ ] Шаблонизация: условный TLS блок в Ingress template
- [ ] Coverage ≥80%, `go test ./...` pass

**Subtasks** (TDD, аналог):

1. Тесты CertificateProcessor (Kind, DNSNames, IssuerRef, SecretName)
2. Тесты ClusterIssuerProcessor (Kind, ACME, SelfSigned)
3. Тесты Ingress-аннотации детекция
4. Запустить → FAIL
5. Реализовать оба процессора
6. Детекция связей + регистрация
7. Запустить → PASS + coverage + регресс

**Time Estimate**:
- **Solo**: 9-11h
- **AI-assisted**: 0.36-0.44h

---

### Task 2.5: Modern Patterns — TopologySpreadConstraints + ExternalDNS + Argo Rollouts

**Goal**: Поддержка продвинутых паттернов scheduling, DNS, и canary deployments
**Result**: Детекция в pod spec + 1 процессор Rollout + values шаблонизация
**Criteria** (FROZEN):
- [ ] Все 8 подзадач выполнены
- [ ] `TopologySpreadConstraints` детектируется из pod spec → `topologySpreadConstraints` block в values
- [ ] ExternalDNS аннотации детектируются из Ingress/Service → `externalDNS.enabled/provider` values
- [ ] `RolloutProcessor`: `strategy` (canary/blueGreen), `steps` (canary), `autoPromotionEnabled` (blueGreen)
- [ ] Rollout → Deployment преобразование с сохранением pod spec
- [ ] Coverage ≥80%, `go test ./...` pass

**Subtasks** (TDD):

1. Тесты TopologySpreadConstraints (detector из pod spec, maxSkew, topologyKey, whenUnsatisfiable)
2. Тесты ExternalDNS (аннотации детекция, hostname extraction, provider detection)
3. Тесты RolloutProcessor (Kind, Strategy, Steps, BlueGreen config)
4. Запустить → FAIL
5. Реализовать TopologySpread + ExternalDNS детекторы
6. Реализовать RolloutProcessor
7. Запустить → PASS
8. Coverage + регресс

**Time Estimate**:
- **Solo**: 10-12h
- **AI-assisted**: 0.40-0.48h

---

## Phase 3: Integration + Release (Tasks 3.1–3.3)

---

### Task 3.1: Integration Tests — Deckhouse Pipeline

**Goal**: End-to-end тесты полного пайплайна с Deckhouse ресурсами
**Result**: `tests/integration/pipeline_deckhouse_test.go` + fixtures
**Criteria** (FROZEN):
- [ ] Все 8 подзадач выполнены
- [ ] Fixture: `tests/integration/fixtures/deckhouse-app/` (ModuleConfig + IngressNginxController + Deployment + ServiceMonitor)
- [ ] `TestPipelineDeckhouse_DetectsDeckhouseCluster` → detector finds Deckhouse
- [ ] `TestPipelineDeckhouse_ModuleScaffold` → `--output-format deckhouse-module` генерирует корректную структуру
- [ ] `TestPipelineDeckhouse_AllProcessors` → все новые процессоры обрабатывают input без ошибок
- [ ] `TestPipelineDeckhouse_MonitoringStack` → ServiceMonitor + PrometheusRule + GrafanaDashboard в одном пайплайне
- [ ] `TestPipelineDeckhouse_GatewayAPI` → HTTPRoute + Gateway → корректные templates
- [ ] `TestPipelineDeckhouse_HelmLint` → helm lint проходит (если helm в PATH)
- [ ] `go test ./...` — все тесты проходят

**Subtasks** (TDD):

1. Создать fixtures/deckhouse-app/: ModuleConfig, IngressNginxController, Deployment, ServiceMonitor, PrometheusRule
2. Написать 7 тестов
3. Запустить → FAIL (процессоры ещё не интегрированы)
4. Убедиться что все процессоры зарегистрированы и pipeline их обрабатывает
5. Добавить `TestPipelineDeckhouse_UniversalRegression` (предыдущие режимы не сломаны)
6. Запустить → PASS
7. Верифицировать: 0 failed, 0 skipped (кроме helm-зависимых)
8. `go test ./...` финальный регресс

**Time Estimate**:
- **Solo**: 8-10h
- **AI-assisted**: 0.32-0.40h

---

### Task 3.2: Documentation & Release Prep

**Goal**: Обновить всю документацию для v0.4.0 фич
**Result**: README, CHANGELOG, examples обновлены
**Criteria** (FROZEN):
- [ ] Все 8 подзадач выполнены
- [ ] README: новая секция "## Deckhouse Integration" с примером `--output-format deckhouse-module`
- [ ] README: таблица новых процессоров (CRD, API group, что извлекается)
- [ ] README: секция "## Monitoring Stack" (ServiceMonitor + PrometheusRule + GrafanaDashboard)
- [ ] README: секция "## Modern K8s Patterns" (Gateway API, KEDA, cert-manager)
- [ ] CHANGELOG: v0.4.0 секция
- [ ] examples/10-deckhouse-module/ создана
- [ ] examples/11-monitoring-stack/ создана
- [ ] examples/12-gateway-api/ создана

**Subtasks**:
1. README — Deckhouse Integration section
2. README — новые процессоры таблица
3. README — Monitoring Stack section
4. README — Modern K8s Patterns section
5. CHANGELOG v0.4.0 section
6. examples/10-deckhouse-module/ (input + README)
7. examples/11-monitoring-stack/ (input + README)
8. examples/12-gateway-api/ (input + README)

**Time Estimate**:
- **Solo**: 5-7h
- **AI-assisted**: 0.20-0.28h

---

### Task 3.3: Release v0.4.0

**Goal**: Tag, test, release
**Result**: git tag `v0.4.0`, release notes, Docker image
**Criteria** (FROZEN):
- [ ] Все 7 подзадач выполнены
- [ ] `go test ./... -count=1` — ALL PASS
- [ ] Coverage ≥80% для всех новых файлов
- [ ] `go vet ./...` — 0 warnings
- [ ] `gitleaks detect` — 0 новых secrets (pre-existing в secret_test.go — известная false-positive)
- [ ] `docker build -t dhg:v0.4.0 --build-arg VERSION=v0.4.0 .` — success
- [ ] `git tag -a v0.4.0 -m "Release v0.4.0: Deckhouse Integration"`
- [ ] `docs/RELEASE_v0.4.0.md` создан

**Subtasks**:
1. `go test ./... -count=1 -v` + verify 0 failures
2. `go test -cover ./pkg/...` → записать coverage числа
3. `go vet ./...`
4. `make bench` → сравнить с v0.3.0 baseline
5. `docker build + docker run --help`
6. `docs/RELEASE_v0.4.0.md` создать
7. `git tag v0.4.0` на правильном коммите

**Time Estimate**:
- **Solo**: 4-5h
- **AI-assisted**: 0.16-0.20h

---

## v0.4.0 Summary

### Total Tasks: 11

| Phase | Tasks | Theme | Solo Est | AI Est |
|-------|-------|-------|----------|--------|
| Deckhouse Core | 1.1-1.5 | CRDs + Module Scaffold + Pattern Detection | 49-61h | ~2.2h |
| Monitoring + Modern K8s | 2.1-2.5 | Prometheus Operator + Gateway API + KEDA + cert-manager | 50-60h | ~2.2h |
| Integration + Release | 3.1-3.3 | E2E tests + Docs + Release | 17-22h | ~0.7h |
| **Total** | **11** | | **116-143h** | **~5.1h** |

### Dependency Graph

```
Task 1.1 (ModuleConfig + IngressNginx) ──┐
Task 1.2 (ClusterAuthRule + NodeGroup)   ├──> Task 1.5 (Pattern Detection) ──> Task 3.1 (IntTests)
Task 1.3 (DexAuth + User/Group)          │
Task 1.4 (Module Scaffold)  ─────────────┘

Task 2.1 (Monitoring)     ──┐
Task 2.2 (Gateway API)    ──┤
Task 2.3 (KEDA)           ──┼──> Task 3.1 (IntTests) ──> Task 3.2 (Docs) ──> Task 3.3 (Release)
Task 2.4 (cert-manager)   ──┤
Task 2.5 (Modern Patterns)──┘
```

### New Processors Count

| Category | Count | CRD / API |
|----------|-------|-----------|
| Deckhouse CRDs | 7 | deckhouse.io/v1, v1alpha1 |
| Monitoring | 4 | monitoring.coreos.com/v1 |
| Gateway API | 2 | gateway.networking.k8s.io/v1 |
| KEDA | 2 | keda.sh/v1alpha1 |
| cert-manager | 2 | cert-manager.io/v1 |
| Modern Patterns | 3 | argoproj.io/v1alpha1 + annotations |
| **Total new** | **20** | |

---

## Velocity Tracker Reference

Шаблон для `docs/velocity_v0.4.0.md` — заполнять в процессе работы:

```markdown
| Task | Description | Est (solo) | Est (AI) | Actual | Velocity | Start (UTC) | End (UTC) |
|------|-------------|-----------|---------|--------|----------|-------------|-----------|
| 1.1  | ModuleConfig + IngressNginx | 10h | 0.40h | | | | |
```

**Процедура:**
```bash
# ДО первой подзадачи:
echo "START $(date -u '+%Y-%m-%d %H:%M:%S UTC')"

# ПОСЛЕ финального go test ./...:
echo "END $(date -u '+%Y-%m-%d %H:%M:%S UTC')"

# Duration = end - start (вручную или через:)
python3 -c "
from datetime import datetime
start = datetime.strptime('2026-02-18 10:00:00', '%Y-%m-%d %H:%M:%S')
end   = datetime.strptime('2026-02-18 10:25:00', '%Y-%m-%d %H:%M:%S')
print(f'{(end-start).seconds/3600:.2f}h')
"
```
