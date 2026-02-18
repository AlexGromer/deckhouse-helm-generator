# Deckhouse Helm Generator — Implementation Plan v4.0

**Version**: 4.0.0 (Goal-Result-Criteria Framework)
**Created**: 2026-02-18
**Status**: Pending
**Methodology**: TDD + Atomic Decomposition + Velocity-Calibrated Estimation
**Predecessor**: IMPLEMENTATION_PLAN_v3.md (v0.3.0 — COMPLETED 2026-02-18)

---

## Velocity Calibration

### Historical Data

| Version | Tasks | Solo Est | Actual (AI) | Velocity |
|---------|-------|----------|-------------|----------|
| v0.2.0 | 24 | 290.5h | 21.55h | 15.6x |
| v0.3.0 | 15 | 141.0h | ~4.95h (~approx) | ~28.5x (~approx) |

### Estimation Model for v0.4.0

- **Solo (human)**: сложность задачи по паттернам (процессор v0.2.0 ≈ 3-4h solo; новый generator ≈ 14-18h solo)
- **AI-assisted**: `solo_midpoint / 20` (консервативно — v0.3.0 дал ~28x но новые CRD могут быть сложнее)
- **v0.3.0 approx caveat**: velocity данные v0.3.0 неточные (нет реальных timestamps), поэтому берём 20x как безопасный коэффициент

### Time Tracking Protocol (ОБЯЗАТЕЛЬНО)

**ПЕРЕД первой подзадачей задачи:**
```bash
echo "=== TASK X.Y START ===" && date -u +"%Y-%m-%d %H:%M:%S UTC"
```

**ПОСЛЕ финального `go test ./...` задачи:**
```bash
echo "=== TASK X.Y END ===" && date -u +"%Y-%m-%d %H:%M:%S UTC"
```

**Записать** оба значения в `docs/velocity_v0.4.0.md` немедленно.

**Вычислить duration:**
```bash
python3 -c "
from datetime import datetime
s=datetime.strptime('YYYY-MM-DD HH:MM:SS','%Y-%m-%d %H:%M:%S')
e=datetime.strptime('YYYY-MM-DD HH:MM:SS','%Y-%m-%d %H:%M:%S')
d=(e-s).total_seconds()/3600
print(f'Duration: {d:.2f}h | Velocity: {SOLO_MID/d:.1f}x')
"
```

---

## Codebase Reference (из ревизии 2026-02-18)

> Ниже — зафиксированные факты о кодовой базе. НЕ МЕНЯТЬ эту секцию — обновлять только при расхождении с реальностью.

### Processor Interface (`pkg/processor/processor.go`)

```go
type Processor interface {
    Process(ctx Context, obj *unstructured.Unstructured) (*Result, error)
    Supports() []schema.GroupVersionKind
    Priority() int
    Name() string
}
```

- Базовый: `processor.BaseProcessor` (embed, через `NewBaseProcessor(name, priority, gvks...)`)
- Регистрация: `processor.NewRegistry()` → `k8s.RegisterAll(registry)` — НЕТ DefaultProcessorRegistry()
- Dispatch: `registry.Process(ctx, obj)` — ищет по GVK, fallback `processGeneric()`
- Test helper: `newTestProcessorContext()` — определена в `deployment_test.go`, доступна всем тестам пакета

### Result struct

```go
type Result struct {
    Processed       bool
    ServiceName     string
    TemplatePath    string
    TemplateContent string
    ValuesPath      string
    Values          map[string]interface{}
    Dependencies    []types.ResourceKey
    ExternalFiles   []*value.ExternalFile
    Metadata        map[string]interface{}
}
```

### Detector Interface (`pkg/analyzer/analyzer.go`)

```go
type Detector interface {
    Detect(ctx context.Context, resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship
    Name() string
    Priority() int
}
```

Нет BaseDetector — каждый имплементирует напрямую.

### CLI (`cmd/dhg/main.go`)

- `--mode`: на `generate`. Значения: `universal`, `separate`, `library`. **Umbrella НЕ экспонирован** (bug/TODO).
- `--output-format`: на `analyze` (text/json/markdown). **НЕ на `generate`.**
- `--env-values`: **НЕ экспонирован** в CLI (добавлено в Options, не в cobra flags).

### Existing Processors (18 шт)

Deployment, StatefulSet, DaemonSet, Service, Ingress, NetworkPolicy, ConfigMap, Secret, PVC, HPA, PDB, CronJob, Job, ServiceAccount, Role, ClusterRole, RoleBinding, ClusterRoleBinding.

### Test file calibration

| Test file | Functions |
|-----------|-----------|
| deployment_test.go | 22 |
| service_test.go | 13 |
| hpa_test.go | 12 |
| pdb_test.go | 10 |
| **Avg per processor** | **~12** |

---

## Phase 0: CLI Fixes (Task 0.1)

> Исправления обнаруженных расхождений перед новыми фичами.

---

### Task 0.1: CLI Flag Cleanup — Expose `umbrella` + `--env-values`

**Goal**: Все фичи v0.3.0 доступны из CLI без правки кода
**Result**: `--mode umbrella` и `--env-values` работают из командной строки
**Criteria** (FROZEN):
- [ ] Все 7 подзадач выполнены
- [ ] `dhg generate --mode umbrella` → работает (UmbrellaGenerator)
- [ ] `dhg generate --env-values` → генерирует values-dev/staging/prod.yaml
- [ ] `dhg generate --help` показывает: `--mode universal|separate|library|umbrella`
- [ ] `dhg generate --help` показывает: `--env-values`
- [ ] Integration test: `TestCLI_UmbrellaMode` → exit 0
- [ ] Integration test: `TestCLI_EnvValues` → 3 файла сгенерированы
- [ ] `go test ./...` pass

**Subtasks** (atomic):

0. **`date -u` → записать START в `docs/velocity_v0.4.0.md`**

1. **Прочитать `cmd/dhg/main.go`** — найти определение `--mode` и generate command
2. **Написать тест `TestCLI_UmbrellaMode`** — запуск binary с `--mode umbrella`
   - Файл: `tests/e2e/cli_modes_test.go`
3. **Написать тест `TestCLI_EnvValues`** — запуск с `--env-values`
4. **Запустить тесты → FAIL**
5. **Добавить `umbrella` в допустимые значения `--mode`** в `cmd/dhg/main.go`
6. **Добавить `--env-values` flag** в generate command → проброс в `Options.EnvValues`
7. **Запустить тесты → PASS + `go test ./...`**

**`date -u` → записать END в `docs/velocity_v0.4.0.md`**

**Time Estimate**:
- **Solo**: 3-4h
- **AI-assisted**: 0.15-0.20h

---

## Phase 1: Deckhouse Core CRDs (Tasks 1.1–1.5)

**Тема**: Полная поддержка Deckhouse-специфичных CRD ресурсов
**Prerequisite**: Task 0.1 COMPLETED

---

### Task 1.1: ModuleConfig + IngressNginxController Processors

**Goal**: 2 наиболее используемых Deckhouse CRD
**Result**: 2 процессора, зарегистрированы, тесты, coverage ≥80%
**Criteria** (FROZEN):
- [ ] Все 10 подзадач выполнены
- [ ] `ModuleConfigProcessor` реализует `processor.Processor` через `processor.BaseProcessor`
- [ ] `Supports()` → `deckhouse.io/v1alpha1 ModuleConfig`
- [ ] Извлекает: `spec.enabled` → `values.enabled`, `spec.version` → `values.version`, `spec.settings.*` → развёрнутые values
- [ ] Template: условный блок `{{ if .Values.enabled }}`
- [ ] `IngressNginxControllerProcessor` реализует `processor.Processor`
- [ ] `Supports()` → `deckhouse.io/v1 IngressNginxController`
- [ ] Извлекает: `spec.ingressClass`, `spec.inlet` (enum: LoadBalancer/HostPort/HostWithFailover), `spec.controllerVersion`, `spec.resourcesRequests`
- [ ] Оба зарегистрированы в `k8s.RegisterAll()`
- [ ] Coverage ≥80% для обоих файлов
- [ ] `go test ./...` pass

**Subtasks** (atomic):

0. **`date -u` → записать START**

1. **Написать `pkg/processor/k8s/moduleconfig_test.go`** (TDD)
   - `TestModuleConfigProcessor_Name` → `"moduleconfig"`
   - `TestModuleConfigProcessor_Supports` → GVK `deckhouse.io/v1alpha1 ModuleConfig`
   - `TestModuleConfigProcessor_Enabled_True` → `spec.enabled: true` → `values["enabled"] = true`
   - `TestModuleConfigProcessor_Enabled_False` → `spec.enabled: false` → `values["enabled"] = false`
   - `TestModuleConfigProcessor_Version` → `spec.version: "1.61"` → `values["version"] = "1.61"`
   - `TestModuleConfigProcessor_Settings_Flat` → `spec.settings.logLevel: "debug"` → `values["logLevel"] = "debug"`
   - `TestModuleConfigProcessor_Settings_Nested` → `spec.settings.auth.type: "dex"` → `values["auth"]["type"] = "dex"`
   - `TestModuleConfigProcessor_Settings_Empty` → `spec.settings: nil` → no panic, empty values
   - `TestModuleConfigProcessor_Template` → template content contains `{{ if .Values.enabled }}`
   - `TestModuleConfigProcessor_ServiceName` → result.ServiceName = metadata.name
   - **10 test functions**

2. **Написать `pkg/processor/k8s/ingressnginxcontroller_test.go`** (TDD)
   - `TestIngressNginxControllerProcessor_Name` → `"ingressnginxcontroller"`
   - `TestIngressNginxControllerProcessor_Supports` → GVK
   - `TestIngressNginxControllerProcessor_IngressClass` → `spec.ingressClass: "nginx"` → values
   - `TestIngressNginxControllerProcessor_Inlet_LoadBalancer` → `values["inlet"] = "LoadBalancer"`
   - `TestIngressNginxControllerProcessor_Inlet_HostPort` → `values["inlet"] = "HostPort"`
   - `TestIngressNginxControllerProcessor_Inlet_HostWithFailover` → values
   - `TestIngressNginxControllerProcessor_ControllerVersion` → `spec.controllerVersion: "1.6"` → values
   - `TestIngressNginxControllerProcessor_ResourcesRequests` → cpu/memory → values
   - `TestIngressNginxControllerProcessor_Template` → template content
   - `TestIngressNginxControllerProcessor_ServiceName` → metadata.name
   - **10 test functions**

3. **Запустить тесты → FAIL** (нет реализации)
   - `go test ./pkg/processor/k8s/ -run TestModuleConfig -v`
   - `go test ./pkg/processor/k8s/ -run TestIngressNginxController -v`

4. **Реализовать `pkg/processor/k8s/moduleconfig.go`**
   - Embed `processor.BaseProcessor`
   - `NewModuleConfigProcessor()` → `NewBaseProcessor("moduleconfig", 50, GVK{...})`
   - `Process()`: extract `spec.enabled`, `spec.version`, flatten `spec.settings` → values
   - Template: conditional `{{ if .Values.enabled }}`

5. **Реализовать `pkg/processor/k8s/ingressnginxcontroller.go`**
   - `NewIngressNginxControllerProcessor()` → priority 50
   - `Process()`: extract `spec.ingressClass`, `spec.inlet`, `spec.controllerVersion`, `spec.resourcesRequests`
   - Template: inlet switch `{{ if eq .Values.inlet "LoadBalancer" }}`

6. **Зарегистрировать оба в `k8s.RegisterAll()`**
   - `r.Register(NewModuleConfigProcessor())`
   - `r.Register(NewIngressNginxControllerProcessor())`

7. **Запустить тесты → PASS**
   - `go test ./pkg/processor/k8s/ -run TestModuleConfig -v`
   - `go test ./pkg/processor/k8s/ -run TestIngressNginxController -v`

8. **Coverage check**
   - `go test ./pkg/processor/k8s/ -cover -run "TestModuleConfig|TestIngressNginx"`

9. **Полный регресс: `go test ./...`**

**`date -u` → записать END**

**Time Estimate**:
- **Solo**: 8-10h (2 процессора × ~4h + рег + регресс)
- **AI-assisted**: 0.40-0.50h

**TDD Workflow**: [0: date] → [1-2: тесты] → [3: FAIL] → [4-6: реализация] → [7: PASS] → [8-9: coverage+регресс] → [date]

---

### Task 1.2: ClusterAuthorizationRule + NodeGroup Processors

**Goal**: Deckhouse RBAC и управление узлами
**Result**: 2 процессора, зарегистрированы, тесты, coverage ≥80%
**Criteria** (FROZEN):
- [ ] Все 10 подзадач выполнены
- [ ] `ClusterAuthorizationRuleProcessor`: `Supports()` → `deckhouse.io/v1 ClusterAuthorizationRule`
- [ ] Извлекает: `spec.subjects` (list of {kind, name}), `spec.accessLevel` (enum 6 значений), `spec.limitNamespaces` (list), `spec.allowScale` (bool)
- [ ] `accessLevel` enum: User, PrivilegedUser, Editor, Admin, ClusterEditor, ClusterAdmin
- [ ] `NodeGroupProcessor`: `Supports()` → `deckhouse.io/v1 NodeGroup`
- [ ] Извлекает: `spec.nodeType` (enum: CloudEphemeral/CloudPermanent/Static), `spec.disruptions.approvalMode`, `spec.kubelet.*`, `spec.cloudInstances.minPerZone`, `spec.cloudInstances.maxPerZone`, `spec.cloudInstances.zones`
- [ ] Оба в `k8s.RegisterAll()`
- [ ] Coverage ≥80%, `go test ./...` pass

**Subtasks** (atomic):

0. **`date -u` → START**

1. **Тесты ClusterAuthorizationRuleProcessor** (10 функций)
   - Name, Supports, Subjects, AccessLevel×3 (User, Admin, ClusterAdmin), LimitNamespaces, AllowScale, Template, ServiceName

2. **Тесты NodeGroupProcessor** (12 функций)
   - Name, Supports, NodeType×3 (CloudEphemeral, CloudPermanent, Static), Disruptions_ApprovalMode, Kubelet_MaxPods, CloudInstances_MinMax, CloudInstances_Zones, Template, ServiceName, EmptyCloudInstances

3. **Запустить → FAIL**
4. **Реализовать ClusterAuthorizationRuleProcessor** (`clusterauthorizationrule.go`)
5. **Реализовать NodeGroupProcessor** (`nodegroup.go`)
6. **Зарегистрировать в `RegisterAll()`**
7. **Запустить → PASS**
8. **Coverage check**
9. **Полный регресс**

**`date -u` → END**

**Time Estimate**:
- **Solo**: 8-10h
- **AI-assisted**: 0.40-0.50h

---

### Task 1.3: DexAuthenticator + User + Group Processors

**Goal**: SSO и identity management
**Result**: 3 процессора + secret-паттерн для паролей
**Criteria** (FROZEN):
- [ ] Все 10 подзадач выполнены
- [ ] `DexAuthenticatorProcessor`: `Supports()` → `deckhouse.io/v1 DexAuthenticator`
- [ ] Извлекает: `spec.applicationDomain`, `spec.sendAuthorizationHeader` (bool), `spec.applicationIngressClassName`, `spec.allowedGroups`
- [ ] Template: аннотации Nginx для auth-proxy (`nginx.ingress.kubernetes.io/auth-url`, `auth-signin`)
- [ ] `UserProcessor`: `Supports()` → `deckhouse.io/v1 User`
- [ ] Извлекает: `spec.email`, `spec.groups` (list), `spec.ttl`
- [ ] `spec.password` → НЕ в values напрямую, в result.Metadata["sensitive_fields"] для Secret ref
- [ ] `GroupProcessor`: `Supports()` → `deckhouse.io/v1 Group`
- [ ] Извлекает: `spec.members` (list)
- [ ] Все 3 в `k8s.RegisterAll()`
- [ ] Coverage ≥80%, `go test ./...` pass

**Subtasks** (atomic):

0. **`date -u` → START**

1. **Тесты DexAuthenticatorProcessor** (8 функций)
   - Name, Supports, ApplicationDomain, SendAuthorizationHeader, IngressClassName, AllowedGroups, Template_AuthAnnotations, ServiceName

2. **Тесты UserProcessor** (8 функций)
   - Name, Supports, Email, Groups, TTL, Password_NotInValues, Password_InMetadata, ServiceName

3. **Тесты GroupProcessor** (5 функций)
   - Name, Supports, Members, EmptyMembers, ServiceName

4. **Запустить → FAIL**
5. **Реализовать DexAuthenticatorProcessor** (`dexauthenticator.go`)
6. **Реализовать UserProcessor** (`user.go`) — sensitive field handling
7. **Реализовать GroupProcessor** (`group.go`)
8. **Зарегистрировать в `RegisterAll()`**
9. **Запустить → PASS + coverage**
10. **Полный регресс**

**`date -u` → END**

**Time Estimate**:
- **Solo**: 9-11h (3 процессора; sensitive field — новый паттерн)
- **AI-assisted**: 0.45-0.55h

---

### Task 1.4: Deckhouse Module Scaffold Generator

**Goal**: `--deckhouse-module` flag генерирует валидную структуру Deckhouse модуля
**Result**: Новый generator + OpenAPI schema generation + `--deckhouse-module` bool flag на `generate`
**Criteria** (FROZEN):
- [ ] Все 12 подзадач выполнены
- [ ] CLI: `dhg generate -f manifests/ -o output/ --chart-name mymodule --deckhouse-module` работает
- [ ] Генерирует: `openapi/config-values.yaml` (Deckhouse JSON Schema)
- [ ] Генерирует: `openapi/values.yaml` (internal values schema)
- [ ] `Chart.yaml` содержит dependency `helm_lib` (version: "*")
- [ ] Templates используют: `helm_lib_module_labels`, `helm_lib_module_image` (include-строки)
- [ ] Генерирует `images/` с placeholder `README.md`
- [ ] Генерирует `hooks/` с placeholder `README.md`
- [ ] `helm lint` проходит (опционально, если helm в PATH)
- [ ] Options: `DeckhouseModule bool` field
- [ ] Coverage ≥80%, `go test ./...` pass

**Subtasks** (atomic):

0. **`date -u` → START**

1. **Тесты `pkg/generator/modulescaffold_test.go`** (TDD, 10 функций)
   - `TestModuleScaffold_ChartYAML_HasHelmLibDep` → dependency `helm_lib`
   - `TestModuleScaffold_ChartYAML_APIVersion` → `apiVersion: v2`
   - `TestModuleScaffold_OpenAPI_ConfigValues` → ExternalFiles содержит `openapi/config-values.yaml`
   - `TestModuleScaffold_OpenAPI_Values` → ExternalFiles содержит `openapi/values.yaml`
   - `TestModuleScaffold_OpenAPI_Structure` → YAML содержит `type: object`, `properties:`
   - `TestModuleScaffold_ImagesDir` → ExternalFiles содержит `images/README.md`
   - `TestModuleScaffold_HooksDir` → ExternalFiles содержит `hooks/README.md`
   - `TestModuleScaffold_Templates_HelmLib` → templates содержат `helm_lib_module_labels`
   - `TestModuleScaffold_Templates_ModuleImage` → templates содержат `helm_lib_module_image`
   - `TestModuleScaffold_DefaultDisabled` → `Options{}.DeckhouseModule == false`

2. **Тесты `pkg/generator/openapi_test.go`** (TDD, 6 функций)
   - `TestOpenAPIFromValues_EmptyMap` → `type: object` с пустыми properties
   - `TestOpenAPIFromValues_StringField` → `{type: string}`
   - `TestOpenAPIFromValues_IntField` → `{type: integer}`
   - `TestOpenAPIFromValues_BoolField` → `{type: boolean}`
   - `TestOpenAPIFromValues_NestedMap` → nested `{type: object, properties: ...}`
   - `TestOpenAPIFromValues_Array` → `{type: array, items: ...}`

3. **Запустить → FAIL**

4. **Реализовать `pkg/generator/openapi.go`** — `GenerateOpenAPISchema(values map[string]interface{}) string`

5. **Реализовать `pkg/generator/modulescaffold.go`** — `GenerateDeckhouseModule(chart *types.GeneratedChart, values map[string]interface{}) *types.GeneratedChart`
   - Модифицирует Chart.yaml (добавляет helm_lib dep)
   - Добавляет ExternalFiles: openapi/*, images/*, hooks/*
   - Добавляет helm_lib includes в Templates

6. **Добавить `DeckhouseModule bool` в `Options`** (`generator.go`)

7. **Добавить `--deckhouse-module` flag** в `cmd/dhg/main.go`

8. **Запустить → PASS**

9. **Coverage: `go test ./pkg/generator/ -cover -run "TestModuleScaffold|TestOpenAPI"`**

10. **`helm lint` test (если helm доступен)**

11. **Полный регресс: `go test ./...`**

**`date -u` → END**

**Time Estimate**:
- **Solo**: 14-18h (OpenAPI generation — новый паттерн, helm_lib integration — нужен research)
- **AI-assisted**: 0.70-0.90h

---

### Task 1.5: Deckhouse Pattern Detection

**Goal**: Автодетекция Deckhouse в input ресурсах
**Result**: `DeckhouseDetector` реализует `analyzer.Detector`
**Criteria** (FROZEN):
- [ ] Все 8 подзадач выполнены
- [ ] `DeckhouseDetector` реализует `Detector` interface (Detect, Name, Priority)
- [ ] Детектирует Deckhouse если есть ресурсы с group `deckhouse.io`
- [ ] Создаёт `types.Relationship{Type: "deckhouse"}` между Deckhouse CRDs
- [ ] Добавляет metadata tag `deckhouse_detected: true` к результату анализа
- [ ] Зарегистрирован в `detector.RegisterAll()`
- [ ] Тесты: detected (ModuleConfig input), not detected (only k8s), mixed input
- [ ] Coverage ≥80%, `go test ./...` pass

**Subtasks** (atomic):

0. **`date -u` → START**

1. **Тесты `pkg/analyzer/detector/deckhouse_test.go`** (8 функций)
   - `TestDeckhouseDetector_Name` → `"deckhouse"`
   - `TestDeckhouseDetector_Priority` → number > 0
   - `TestDeckhouseDetector_Detected_ModuleConfig` → deckhouse.io resource → relationship created
   - `TestDeckhouseDetector_Detected_IngressNginx` → deckhouse.io resource → detected
   - `TestDeckhouseDetector_NotDetected_K8sOnly` → only apps/v1 → no relationship
   - `TestDeckhouseDetector_MixedResources` → k8s + deckhouse → detected for deckhouse only
   - `TestDeckhouseDetector_MultipleDeckhouseCRDs` → creates relationships between them
   - `TestDeckhouseDetector_EmptyInput` → no panic, no relationships

2. **Запустить → FAIL**
3. **Реализовать `pkg/analyzer/detector/deckhouse.go`**
4. **Зарегистрировать в `detector.RegisterAll()`**
5. **Запустить → PASS**
6. **Coverage: `go test ./pkg/analyzer/detector/ -cover`**
7. **Полный регресс**

**`date -u` → END**

**Time Estimate**:
- **Solo**: 5-7h (паттерн аналогичен существующим детекторам)
- **AI-assisted**: 0.25-0.35h

---

## Phase 2: Monitoring & Modern K8s (Tasks 2.1–2.5)

**Тема**: Prometheus Operator + Gateway API + KEDA + cert-manager
**Prerequisite**: Phase 1 не обязательна (процессоры независимы), но Phase 0 обязательна

---

### Task 2.1: Monitoring Processors (ServiceMonitor + PodMonitor + PrometheusRule + GrafanaDashboard)

**Goal**: 4 процессора Prometheus Operator stack
**Result**: Процессоры зарегистрированы, тесты, coverage ≥80%
**Criteria** (FROZEN):
- [ ] Все 11 подзадач выполнены
- [ ] `ServiceMonitorProcessor`: `Supports()` → `monitoring.coreos.com/v1 ServiceMonitor`
- [ ] Извлекает: `spec.endpoints[]` (port, path, interval, scrapeTimeout), `spec.namespaceSelector`, `spec.selector.matchLabels`
- [ ] Dependencies: ServiceMonitor → Service (через selector)
- [ ] `PodMonitorProcessor`: `Supports()` → `monitoring.coreos.com/v1 PodMonitor`
- [ ] Извлекает: `spec.podMetricsEndpoints[]` (port, path, interval), `spec.jobLabel`, `spec.selector`
- [ ] `PrometheusRuleProcessor`: `Supports()` → `monitoring.coreos.com/v1 PrometheusRule`
- [ ] Извлекает: `spec.groups[]` → rules с alert name, expr (шаблонизация threshold), for, labels, annotations
- [ ] `GrafanaDashboardProcessor`: ConfigMap с label `grafana_dashboard: "1"` → JSON dashboard в ExternalFiles (`files/dashboards/`)
- [ ] Все 4 в `k8s.RegisterAll()`
- [ ] Coverage ≥80%, `go test ./...` pass

**Subtasks** (atomic):

0. **`date -u` → START**

1. **Тесты ServiceMonitorProcessor** (10 функций)
   - Name, Supports, Endpoints_Single, Endpoints_Multi, NamespaceSelector, Selector, Interval, ScrapeTimeout, Dependency_ToService, Template

2. **Тесты PodMonitorProcessor** (7 функций)
   - Name, Supports, PodMetricsEndpoints, JobLabel, Selector, Template, ServiceName

3. **Тесты PrometheusRuleProcessor** (8 функций)
   - Name, Supports, SingleGroup, MultiGroup, AlertRule_Fields (alert, expr, for, labels, annotations), RecordRule, ExprTemplating, Template

4. **Тесты GrafanaDashboardProcessor** (6 функций)
   - Name, Supports_ConfigMapWithLabel, NotSupports_ConfigMapWithoutLabel, Dashboard_ExternalFile, Dashboard_FilePath, Template

5. **Запустить → FAIL**
6. **Реализовать ServiceMonitorProcessor** (`servicemonitor.go`)
7. **Реализовать PodMonitorProcessor** (`podmonitor.go`)
8. **Реализовать PrometheusRuleProcessor** (`prometheusrule.go`)
9. **Реализовать GrafanaDashboardProcessor** (`grafanadashboard.go`)
10. **Зарегистрировать все 4 + coverage**
11. **Полный регресс**

**`date -u` → END**

**Time Estimate**:
- **Solo**: 14-16h (4 процессора; PrometheusRule expr шаблонизация — сложная; GrafanaDashboard — ExternalFiles паттерн)
- **AI-assisted**: 0.70-0.80h

---

### Task 2.2: Gateway API Processors (HTTPRoute + Gateway)

**Goal**: Gateway API — замена Ingress
**Result**: 2 процессора + relationship HTTPRoute→Gateway
**Criteria** (FROZEN):
- [ ] Все 9 подзадач выполнены
- [ ] `HTTPRouteProcessor`: `Supports()` → `gateway.networking.k8s.io/v1 HTTPRoute`
- [ ] Извлекает: `spec.parentRefs[]` (name, namespace, sectionName), `spec.hostnames[]`, `spec.rules[]` (matches: path/headers/queryParams, backendRefs: name/port/weight, filters: requestRedirect/urlRewrite)
- [ ] `GatewayProcessor`: `Supports()` → `gateway.networking.k8s.io/v1 Gateway`
- [ ] Извлекает: `spec.gatewayClassName`, `spec.listeners[]` (name, port, protocol, hostname, tls.mode, tls.certificateRefs)
- [ ] Relationship: HTTPRoute → Gateway через `parentRefs[].name`
- [ ] Relationship type: добавить `RelationGatewayRoute` в `types/relationship.go`
- [ ] Coverage ≥80%, `go test ./...` pass

**Subtasks** (atomic):

0. **`date -u` → START**

1. **Тесты HTTPRouteProcessor** (10 функций)
   - Name, Supports, ParentRefs, Hostnames, Rules_PathMatch, Rules_BackendRefs, Rules_Filters, Dependency_ToGateway, Template, ServiceName

2. **Тесты GatewayProcessor** (8 функций)
   - Name, Supports, GatewayClassName, Listeners_HTTP, Listeners_HTTPS_TLS, Listeners_Multi, Template, ServiceName

3. **Запустить → FAIL**
4. **Добавить `RelationGatewayRoute` в `types/relationship.go`**
5. **Реализовать HTTPRouteProcessor** (`httproute.go`)
6. **Реализовать GatewayProcessor** (`gateway.go`)
7. **Зарегистрировать + coverage**
8. **Полный регресс**

**`date -u` → END**

**Time Estimate**:
- **Solo**: 10-12h
- **AI-assisted**: 0.50-0.60h

---

### Task 2.3: KEDA Processors (ScaledObject + TriggerAuthentication)

**Goal**: Event-driven autoscaling
**Result**: 2 процессора + relationship ScaledObject→Deployment
**Criteria** (FROZEN):
- [ ] Все 9 подзадач выполнены
- [ ] `ScaledObjectProcessor`: `Supports()` → `keda.sh/v1alpha1 ScaledObject`
- [ ] Извлекает: `spec.scaleTargetRef` (name, kind), `spec.minReplicaCount` (0=scale-to-zero), `spec.maxReplicaCount`, `spec.triggers[]` (type, metadata, authenticationRef)
- [ ] `TriggerAuthenticationProcessor`: `Supports()` → `keda.sh/v1alpha1 TriggerAuthentication`
- [ ] Извлекает: `spec.secretTargetRef[]` (parameter, name, key), `spec.env[]`, `spec.podIdentity`
- [ ] Relationship: ScaledObject → target Deployment/StatefulSet через `scaleTargetRef.name`
- [ ] Relationship type: добавить `RelationScaleTarget` в `types/relationship.go`
- [ ] scale-to-zero handling: `minReplicaCount: 0` → values + template аннотация
- [ ] Coverage ≥80%, `go test ./...` pass

**Subtasks** (atomic):

0. **`date -u` → START**

1. **Тесты ScaledObjectProcessor** (10 функций)
   - Name, Supports, ScaleTargetRef, MinReplicaCount_Normal, MinReplicaCount_Zero, MaxReplicaCount, Triggers_Single, Triggers_Multi_WithAuth, Dependency_ToDeployment, Template

2. **Тесты TriggerAuthenticationProcessor** (6 функций)
   - Name, Supports, SecretTargetRef, Env, PodIdentity, Template

3. **Запустить → FAIL**
4. **Добавить `RelationScaleTarget` в `types/relationship.go`**
5. **Реализовать ScaledObjectProcessor** (`scaledobject.go`)
6. **Реализовать TriggerAuthenticationProcessor** (`triggerauthentication.go`)
7. **Зарегистрировать + coverage**
8. **Полный регресс**

**`date -u` → END**

**Time Estimate**:
- **Solo**: 9-11h
- **AI-assisted**: 0.45-0.55h

---

### Task 2.4: cert-manager Processors (Certificate + ClusterIssuer)

**Goal**: TLS automation
**Result**: 2 процессора + детекция cert-manager аннотации в Ingress
**Criteria** (FROZEN):
- [ ] Все 9 подзадач выполнены
- [ ] `CertificateProcessor`: `Supports()` → `cert-manager.io/v1 Certificate`
- [ ] Извлекает: `spec.dnsNames[]`, `spec.issuerRef` (name, kind, group), `spec.secretName`, `spec.duration`, `spec.renewBefore`
- [ ] `ClusterIssuerProcessor`: `Supports()` → `cert-manager.io/v1 ClusterIssuer`
- [ ] Извлекает: `spec.acme` (server, email, privateKeySecretRef, solvers[]), `spec.selfSigned`, `spec.ca`
- [ ] Детекция: Ingress с аннотацией `cert-manager.io/cluster-issuer` → relationship к ClusterIssuer
- [ ] Relationship type: уже есть `RelationAnnotation` в annotation detector — расширить
- [ ] Coverage ≥80%, `go test ./...` pass

**Subtasks** (atomic):

0. **`date -u` → START**

1. **Тесты CertificateProcessor** (8 функций)
   - Name, Supports, DNSNames, IssuerRef, SecretName, Duration, Template, ServiceName

2. **Тесты ClusterIssuerProcessor** (7 функций)
   - Name, Supports, ACME_Server, ACME_Email, ACME_Solvers, SelfSigned, Template

3. **Тесты Ingress→ClusterIssuer детекция** (3 функции)
   - `TestAnnotationDetector_CertManager_Detected` → annotation present → relationship
   - `TestAnnotationDetector_CertManager_NotDetected` → no annotation → no relationship
   - `TestAnnotationDetector_CertManager_IssuerRef` → correct target

4. **Запустить → FAIL**
5. **Реализовать CertificateProcessor** (`certificate.go`)
6. **Реализовать ClusterIssuerProcessor** (`clusterissuer.go`)
7. **Расширить AnnotationDetector** для cert-manager (если не покрыто)
8. **Зарегистрировать + coverage**
9. **Полный регресс**

**`date -u` → END**

**Time Estimate**:
- **Solo**: 9-11h
- **AI-assisted**: 0.45-0.55h

---

### Task 2.5: TopologySpread + ExternalDNS + Argo Rollouts

**Goal**: Продвинутые паттерны
**Result**: Pod spec детекторы + RolloutProcessor
**Criteria** (FROZEN):
- [ ] Все 10 подзадач выполнены
- [ ] TopologySpreadConstraints: детектируется из `pod.spec.topologySpreadConstraints[]` → `values.topologySpreadConstraints` block
- [ ] Извлекает: `maxSkew`, `topologyKey`, `whenUnsatisfiable`, `labelSelector`
- [ ] ExternalDNS: детектируется из аннотаций `external-dns.alpha.kubernetes.io/hostname` → `values.externalDNS.enabled`, `.hostname`
- [ ] `RolloutProcessor`: `Supports()` → `argoproj.io/v1alpha1 Rollout`
- [ ] Извлекает: `spec.strategy.canary` (steps[], maxSurge, maxUnavailable) ИЛИ `spec.strategy.blueGreen` (autoPromotionEnabled, prePromotionAnalysis)
- [ ] Rollout template preserves pod spec from `spec.template`
- [ ] Coverage ≥80%, `go test ./...` pass

**Subtasks** (atomic):

0. **`date -u` → START**

1. **Тесты TopologySpread** (5 функций)
   - Detect_Present, Detect_Absent, MaxSkew_Value, TopologyKey_Zone, TopologyKey_Node

2. **Тесты ExternalDNS** (4 функции)
   - Detect_Ingress_WithAnnotation, Detect_Service_WithAnnotation, Detect_NoAnnotation, Hostname_Extraction

3. **Тесты RolloutProcessor** (8 функций)
   - Name, Supports, Strategy_Canary_Steps, Strategy_Canary_MaxSurge, Strategy_BlueGreen_AutoPromotion, Strategy_BlueGreen_PreAnalysis, PodSpec_Preserved, Template

4. **Запустить → FAIL**
5. **Реализовать TopologySpread detection** (в существующем Deployment/StatefulSet processor или отдельный helper)
6. **Реализовать ExternalDNS detection** (расширить AnnotationDetector или отдельный helper)
7. **Реализовать RolloutProcessor** (`rollout.go`)
8. **Зарегистрировать + coverage**
9. **Полный регресс**

**`date -u` → END**

**Time Estimate**:
- **Solo**: 10-12h (3 разных паттерна; Rollout — argo CRD)
- **AI-assisted**: 0.50-0.60h

---

## Phase 3: Integration + Release (Tasks 3.1–3.3)

**Prerequisite**: Phases 1 и 2 COMPLETED

---

### Task 3.1: Integration Tests — Full Deckhouse Pipeline

**Goal**: E2E тесты полного пайплайна с новыми ресурсами
**Result**: `tests/integration/pipeline_deckhouse_test.go` + fixtures
**Criteria** (FROZEN):
- [ ] Все 10 подзадач выполнены
- [ ] `tests/integration/fixtures/deckhouse-app/`: ModuleConfig + IngressNginxController + Deployment + Service + ServiceMonitor + PrometheusRule
- [ ] `tests/integration/fixtures/gateway-app/`: Gateway + HTTPRoute + Service + Deployment
- [ ] `TestPipelineDeckhouse_AllProcessors` → все 20 новых процессоров обрабатывают input
- [ ] `TestPipelineDeckhouse_ModuleScaffold` → `--deckhouse-module` output валиден
- [ ] `TestPipelineDeckhouse_MonitoringStack` → ServiceMonitor + PrometheusRule корректны
- [ ] `TestPipelineDeckhouse_GatewayAPI` → HTTPRoute + Gateway templates
- [ ] `TestPipelineDeckhouse_KEDA` → ScaledObject + target relationship
- [ ] `TestPipelineDeckhouse_HelmLint` → helm lint pass
- [ ] `TestPipelineDeckhouse_Regression` → v0.3.0 modes не сломаны
- [ ] `go test ./...` pass

**Subtasks** (atomic):

0. **`date -u` → START**

1. **Создать `tests/integration/fixtures/deckhouse-app/`** — 5 YAML файлов
2. **Создать `tests/integration/fixtures/gateway-app/`** — 4 YAML файла
3. **Написать 8 тестов** в `pipeline_deckhouse_test.go`
4. **Написать regression тест** — v0.3.0 modes
5. **Запустить → FAIL (ожидаемо — fixtures могут обнаружить gap)**
6. **Исправить обнаруженные проблемы**
7. **Запустить → PASS**
8. **Verify: 0 failed, 0 skipped (кроме helm-зависимых)**
9. **`go test ./...` финальный**

**`date -u` → END**

**Time Estimate**:
- **Solo**: 8-10h
- **AI-assisted**: 0.40-0.50h

---

### Task 3.2: Documentation & Release Prep

**Goal**: README, CHANGELOG, examples для v0.4.0
**Result**: Документация обновлена
**Criteria** (FROZEN):
- [ ] Все 10 подзадач выполнены
- [ ] README: "## Deckhouse Integration" section с `--deckhouse-module` примером
- [ ] README: таблица 20 новых процессоров (Kind, API group, что извлекается)
- [ ] README: "## Monitoring Stack" section (ServiceMonitor + PrometheusRule + GrafanaDashboard)
- [ ] README: "## Modern K8s Patterns" section (Gateway API, KEDA, cert-manager, Argo Rollouts)
- [ ] README: CLI flags обновлены (`--mode umbrella`, `--env-values`, `--deckhouse-module`)
- [ ] README: coverage badge обновлён
- [ ] CHANGELOG: v0.4.0 section
- [ ] `examples/10-deckhouse-module/` (input + README)
- [ ] `examples/11-monitoring-stack/` (input + README)
- [ ] `examples/12-gateway-api/` (input + README)

**Subtasks** (atomic):

0. **`date -u` → START**

1. README — Deckhouse Integration section
2. README — таблица 20 процессоров
3. README — Monitoring Stack section
4. README — Modern K8s Patterns section
5. README — CLI flags update
6. README — coverage badge: `go test -cover ./pkg/...`
7. CHANGELOG v0.4.0 section
8. `examples/10-deckhouse-module/`
9. `examples/11-monitoring-stack/`
10. `examples/12-gateway-api/`

**`date -u` → END**

**Time Estimate**:
- **Solo**: 5-7h
- **AI-assisted**: 0.25-0.35h

---

### Task 3.3: Release v0.4.0

**Goal**: Tag, test, release
**Result**: git tag v0.4.0, Docker image, release notes
**Criteria** (FROZEN):
- [ ] Все 9 подзадач выполнены
- [ ] `go test ./... -count=1` → ALL PASS, 0 failures
- [ ] `go test -cover ./pkg/...` → ≥80% для всех новых файлов
- [ ] `go vet ./...` → 0 warnings
- [ ] `gitleaks detect` → 0 новых secrets
- [ ] `.claude/` в `.gitignore`
- [ ] `make bench` → результаты задокументированы, сравнены с v0.3.0
- [ ] `docker build -t dhg:v0.4.0` → success; `docker run --rm dhg:v0.4.0 --help` → OK
- [ ] `docs/RELEASE_v0.4.0.md` создан
- [ ] `git tag -a v0.4.0 -m "Release v0.4.0: Deckhouse Integration + Modern K8s"`

**Subtasks** (atomic):

0. **`date -u` → START**

1. `go test ./... -count=1 -v` + verify 0 failures
2. `go test -cover ./pkg/...` → записать coverage
3. `go vet ./...`
4. `gitleaks detect --source . --verbose`
5. `make bench` → сравнить с v0.3.0 baseline
6. `docker build + docker run --help`
7. Написать `docs/RELEASE_v0.4.0.md`
8. Git commit + tag
9. Финальный `go test ./...` после tag

**`date -u` → END**

**Time Estimate**:
- **Solo**: 4-5h
- **AI-assisted**: 0.20-0.25h

---

## v0.4.0 Summary

### Total Tasks: 14 (включая 0.1)

| Phase | Tasks | Theme | Solo Est (mid) | AI Est (mid) |
|-------|-------|-------|---------------|--------------|
| CLI Fixes | 0.1 | umbrella + env-values exposure | 3.5h | 0.18h |
| Deckhouse Core | 1.1-1.5 | 7 CRD + Module Scaffold + Detection | 49.0h | 2.45h |
| Monitoring + Modern K8s | 2.1-2.5 | 12 processors (Prom/Gateway/KEDA/cert/Argo) | 55.0h | 2.75h |
| Integration + Release | 3.1-3.3 | E2E tests + Docs + Release | 19.5h | 0.98h |
| **Total** | **14** | **20 new processors + scaffold + detection** | **127.0h** | **6.35h** |

### Total Subtasks: 135

| Phase | Tasks | Subtasks |
|-------|-------|----------|
| 0.x | 1 | 8 (вкл. date) |
| 1.x | 5 | 52 |
| 2.x | 5 | 51 |
| 3.x | 3 | 31 |
| **Total** | **14** | **142** (вкл. date subtasks) |

### Dependency Graph

```
Task 0.1 (CLI Fixes) ──────────────────────────────────────────────────┐
                                                                        │
Task 1.1 (ModuleConfig+IngressNginx) ──┐                               │
Task 1.2 (ClusterAuthRule+NodeGroup)   ├─> Task 1.5 (Detection) ──┐    │
Task 1.3 (DexAuth+User+Group)         │                            │    │
Task 1.4 (Module Scaffold)  ───────────┘                            │    │
                                                                     │    │
Task 2.1 (Monitoring)     ──┐                                       │    │
Task 2.2 (Gateway API)    ──┤                                       │    │
Task 2.3 (KEDA)           ──┼─> Task 3.1 (IntTests) ←──────────────┘    │
Task 2.4 (cert-manager)   ──┤         │                                  │
Task 2.5 (Modern Patterns)──┘         ↓                                  │
                               Task 3.2 (Docs) ←────────────────────────┘
                                       │
                                       ↓
                               Task 3.3 (Release)
```

### New Processors Count

| Category | Processors | CRD / API |
|----------|-----------|-----------|
| Deckhouse CRDs | 7 | ModuleConfig, IngressNginxController, ClusterAuthorizationRule, NodeGroup, DexAuthenticator, User, Group |
| Monitoring | 4 | ServiceMonitor, PodMonitor, PrometheusRule, GrafanaDashboard |
| Gateway API | 2 | HTTPRoute, Gateway |
| KEDA | 2 | ScaledObject, TriggerAuthentication |
| cert-manager | 2 | Certificate, ClusterIssuer |
| Argo CD | 1 | Rollout |
| Detectors | 2 | TopologySpread, ExternalDNS (extensions) |
| **Total** | **20** | |

---

## Velocity Tracker Template

`docs/velocity_v0.4.0.md` — обновлять **в реальном времени** при каждой задаче.

Subtask 0 каждой задачи — `date -u START`.
Последний subtask каждой задачи — `date -u END`.
Сразу после END — записать в velocity tracker.
