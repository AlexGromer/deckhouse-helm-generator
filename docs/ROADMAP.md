# Deckhouse Helm Generator — ROADMAP

**Version**: 2.9.0
**Created**: 2026-01-30
**Updated**: 2026-02-26
**Status**: Active Development — Research Complete ✅
**Research Passes**: 15 completed (15 planned)
1. Core Features & Basics
2. Ecosystem Integration
3. Advanced Patterns & Edge Cases
4. Production-Readiness & Interoperability
5. K8s Deep Patterns & Helm Best Practices
6. (internal consolidation pass)
7. **Service Mesh & Observability** (Istio, Linkerd, OpenTelemetry, Prometheus, Grafana, Sloth)
8. **Security & Compliance** (PSS/PSA, NetworkPolicy, RBAC, CIS Benchmark, Supply Chain Security)
9. **Production Operations & Day 2 Ops** (Backup/Restore, Multi-Tenancy, Capacity Planning, Cost Optimization, DR, Compliance Audit, Change Management, Monitoring)
10. **Developer Experience & Automation** (IDE Integration, Pre-Commit Hooks, Testing, Documentation, Hot Reload, GitOps, Kubernetes Operators, Platform Engineering, Debugging, Performance)
11. **AI/ML Workloads** (Kubeflow, KServe, Seldon Core, GPU Operator, Ray, MLOps, TensorRT, Triton, MinIO)
12. **Database Operators** (CloudNativePG, Zalando, Crunchy, Percona XtraDB/MongoDB, MariaDB, Oracle MySQL, Redis Enterprise, Spotahome, MongoDB Community, K8ssandra, ClickHouse, OpenSearch)
13. **Advanced Scheduling** (Pod Topology Spread, PriorityClass, Affinity/Anti-Affinity, Taints/Tolerations, Volcano, YuniKorn, Kueue)
14. **Data Management & CSI** (VolumeSnapshots, AWS EBS/GCP PD/Azure Disk CSI, Storage Classes, KMS Encryption, LUKS, Velero Integration)
15. **Edge Computing & Lightweight Kubernetes** (K3s, MicroK8s, KubeEdge, ARM/Multi-Arch, Air-Gapped Deployments, IoT/MQTT/OPC-UA, Device Discovery)

---

## Architectural Decision: Scope & Boundaries

### Решение: Модульный подход (Separation of Concerns)

DHG остаётся **инструментом генерации Helm-чартов** и НЕ расширяется в сторону
management, deployment, security scanning.

### Обоснование

| Фактор | Совмещение | Разделение | Выбор |
|--------|-----------|------------|-------|
| Unix-философия | Нарушает | Соответствует | **Разделение** |
| Индустриальный паттерн | Нет прецедентов | Helmify, Helmfile, Checkov — все отдельные | **Разделение** |
| Deckhouse-архитектура | Противоречит (модули = отдельные) | Соответствует (addon-operator + shell-operator) | **Разделение** |
| Maintainability | Выше сложность, больше багов | Изолированные codebase | **Разделение** |
| Composability | Lock-in на один инструмент | Mix-and-match с Helmfile, ArgoCD, Trivy | **Разделение** |
| Стоимость поддержки | Растёт экспоненциально | Растёт линейно | **Разделение** |

### Что DHG делает

```
Kubernetes YAML / Deckhouse ресурсы
         │
         ▼
  ┌──────────────────────┐
  │   DHG (этот проект)  │
  │                      │
  │  Extract → Process   │
  │  → Analyze → Generate│
  └──────────┬───────────┘
             │
             ▼
    Standard Helm Chart
    (Chart.yaml, values.yaml, templates/)
```

### Что DHG НЕ делает (используйте специализированные инструменты)

| Функция | Инструмент | Почему отдельно |
|---------|------------|-----------------|
| **Deployment** | Helm CLI, Helmfile, ArgoCD | Другой lifecycle, другие паттерны |
| **Security scanning** | Checkov, KubeLinter, Trivy, Polaris | Развиваются отдельно, своя экспертиза |
| **Chart management** | Helmfile, Helmsman | Declarative spec для releases |
| **GitOps CD** | ArgoCD, Flux | Git as source of truth |
| **Policy enforcement** | OPA/Gatekeeper, Kyverno | Admission control |

### Точки интеграции (stdin/stdout + файлы)

```bash
# Пример CI/CD pipeline (compose via pipe)
dhg generate -f manifests/ -o chart/ --chart-name myapp  # 1. Generate
checkov -d chart/                                         # 2. Scan
helm lint chart/myapp                                     # 3. Validate
helmfile -f helmfile.yaml apply                           # 4. Deploy
```

---

## Current State (v0.6.0)

### Реализовано

| Компонент | Статус | Покрытие |
|-----------|--------|----------|
| **File Extractor** | ✅ Done | YAML файлы, рекурсивные директории |
| **K8s Processors** | ✅ Done | 13 типов (+HPA, VPA, PriorityClass, LimitRange, ResourceQuota, PDB, NetworkPolicy, CronJob, Job, Role/Binding) |
| **Relationship Detector** | ✅ Done | 13 типов связей (labels, names, volumes, env, annotations) |
| **Universal Generator** | ✅ Done | Единый чарт со всеми сервисами |
| **Pattern Analyzer** | ✅ Done | 5 детекторов (microservices, monolith, stateful, job, operator) + 9 checkers |
| **Best Practices Checker** | ✅ Done | Security, Resources, HA, InitContainer, QoS, StatefulSet, DaemonSet, GracefulShutdown, PSS |
| **Value Processor** | ✅ Done | JSON/XML/Base64 детекция, pretty-print, externalization |
| **External Files** | ✅ Done | Автовынос больших данных (>1KB) в files/ |
| **Recommendation Engine** | ✅ Done | Приоритизированные action items, text/JSON/markdown |
| **Checksum Annotations** | ✅ Done | Auto-reload ConfigMap/Secret via sha256sum |
| **API Migration** | ✅ Done | Deprecated API detection + auto-migration (12 entries) |
| **CLI** | ✅ Done | `generate`, `analyze`, `validate`, `diff`, `version`, `--dry-run` |
| **Test Coverage** | ✅ Done | 85.9% total (target >= 80%) |

### Не реализовано

| Компонент | Статус | Блокирует |
|-----------|--------|-----------|
| Cluster Extractor | ❌ Stub | Live cluster support |
| GitOps Extractor | ❌ Stub | Git repo support |
| Separate Generator | ❌ Stub | Per-service charts |
| Library Generator | ❌ Stub | Shared library charts |
| Deckhouse CRD Processors | ❌ Empty | Deckhouse-specific resources |
| Monitoring Processors | ❌ Empty | Prometheus/Grafana resources |
| Test Coverage | ⚠️ ~5% | Production readiness |

---

## Phase 1: Core Stabilization (v0.2.0)

**Цель**: Надёжная основа для production use
**Задач**: 28 | **Оценка сложности**: Medium

### 1.1 Test Suite (6 задач)

| # | Задача | Файлы | Описание |
|---|--------|-------|----------|
| 1.1.1 | Unit-тесты процессоров | `pkg/processor/k8s/*_test.go` | По 5-10 тестов на каждый из 9 процессоров (Deployment, Service, ConfigMap, Secret, Ingress, StatefulSet, DaemonSet, PVC, SA) |
| 1.1.2 | Unit-тесты детекторов связей | `pkg/analyzer/detector/*_test.go` | Тесты для 13 типов связей: label match, name reference, volume mount, env reference, annotation, owner reference, service selector, configmap ref, secret ref, PVC ref, ingress backend, SA ref, namespace |
| 1.1.3 | Unit-тесты генератора | `pkg/generator/*_test.go` | Universal generator: values building, template rendering, helpers, external files |
| 1.1.4 | Unit-тесты анализаторов | `pkg/analyzer/*_test.go` | Pattern analyzer, best practices checker, recommendation engine |
| 1.1.5 | Integration-тесты | `tests/integration/` | Полный pipeline: extract → process → analyze → generate. Тестовые наборы: single deployment, microservices (3+ services), stateful app, deckhouse module |
| 1.1.6 | E2E-тесты | `tests/e2e/` | Генерация + `helm lint` + `helm template` + валидация output. Требует установленного helm CLI |
| — | **Целевое покрытие** | — | **≥ 80%** (текущее ~5%) |

### 1.2 Additional K8s Resource Processors (9 задач)

| # | Задача | API Group | Описание |
|---|--------|-----------|----------|
| 1.2.1 | HorizontalPodAutoscaler | `autoscaling/v2` | Извлечение min/max replicas, metrics (CPU, memory, custom), behavior (scaleUp/scaleDown) |
| 1.2.2 | PodDisruptionBudget | `policy/v1` | Извлечение minAvailable/maxUnavailable, selector matching |
| 1.2.3 | NetworkPolicy | `networking.k8s.io/v1` | Извлечение ingress/egress rules, podSelector, namespaceSelector, ipBlock |
| 1.2.4 | CronJob | `batch/v1` | Извлечение schedule, concurrencyPolicy, jobTemplate, suspend, successfulJobsHistoryLimit |
| 1.2.5 | Job | `batch/v1` | Извлечение completions, parallelism, backoffLimit, activeDeadlineSeconds |
| 1.2.6 | Role / ClusterRole | `rbac.authorization.k8s.io/v1` | Извлечение rules (apiGroups, resources, verbs), aggregation |
| 1.2.7 | RoleBinding / ClusterRoleBinding | `rbac.authorization.k8s.io/v1` | Извлечение roleRef, subjects (User, Group, SA) |
| 1.2.8 | VerticalPodAutoscaler | `autoscaling.k8s.io/v1` | ✅ Извлечение updatePolicy, resourcePolicy, targetRef |
| 1.2.9 | PriorityClass | `scheduling.k8s.io/v1` | ✅ Извлечение value, globalDefault, preemptionPolicy, description |
| 1.2.10 | LimitRange | `v1` | ✅ Извлечение default/defaultRequest/min/max для containers. Namespace-level resource enforcement |
| 1.2.11 | ResourceQuota | `v1` | ✅ Извлечение hard limits: CPU, memory, pods, PVCs, services. Namespace capacity planning |

### 1.3 Generator Improvements (5 задач)

| # | Задача | Описание |
|---|--------|----------|
| 1.3.1 | Values Schema generation | Генерация `values.schema.json` из анализа типов данных в values.yaml. JSON Schema Draft 7. Типы: string, integer, boolean, object, array. Обязательные поля. |
| 1.3.2 | Helm test templates | Auto-scaffold `helm-unittest` тесты в `templates/tests/`. Для каждого шаблона: snapshot test + value permutation (enabled=true/false). BDD-style assertions. |
| 1.3.3 | Chart hooks generation | `pre-upgrade` hook для DB миграций (Job), `post-install` hook для smoke test (Job), `pre-delete` hook для cleanup. Аннотации `helm.sh/hook`, `helm.sh/hook-weight`, `helm.sh/hook-delete-policy`. |
| 1.3.4 | Configurable template style | Flag `--template-style`: `standard` (Go template) vs `helm` (Helm-specific functions). Выбор между `{{ if }}` и `{{- with }}`. |
| 1.3.5 | Deprecated API auto-migration | ✅ Детекция deprecated APIs (Pluto-like): `extensions/v1beta1` Ingress → `networking.k8s.io/v1`, `policy/v1beta1` PDB → `policy/v1`. Авто-миграция при генерации. |
| 1.3.6 | NOTES.txt генерация | Динамический `templates/NOTES.txt`: connection instructions (port-forward, LB IP, Ingress URL), conditional sections по enabled features, credentials retrieval commands |
| 1.3.7 | Values.yaml design patterns | Nested структура (image.repository, service.type), inline-комментарии над каждым ключом, sensible defaults. Flag `--values-flat` для плоской структуры |
| 1.3.8 | Checksum annotations | ✅ Автогенерация `checksum/config` и `checksum/secret` annotations в Deployment pod template для auto-reload при изменении ConfigMap/Secret |
| 1.3.9 | Advanced _helpers.tpl | Расширенные named templates: `myapp.labels`, `myapp.selectorLabels`, `myapp.serviceAccountName`, `myapp.image`. Переиспользование во всех шаблонах |

### 1.4 CLI & UX (4 задачи)

| # | Задача | Команда | Описание |
|---|--------|---------|----------|
| 1.4.1 | Validate command | `dhg validate` | ✅ Встроенная валидация чарта без Helm CLI: YAML syntax, required fields, values schema, template rendering. Exit codes: 0=ok, 1=error, 2=warning. |
| 1.4.2 | Diff command | `dhg diff` | ✅ Сравнение сгенерированного чарта с предыдущей версией или существующим чартом. Цветной diff output. |
| 1.4.3 | Dry-run mode | `--dry-run` | ✅ Показать что будет сгенерировано без записи на диск. Список файлов + preview первых 20 строк каждого. |
| 1.4.4 | Progress bar | — | Для больших директорий (>50 файлов): progress bar с eta. Библиотека: `schollz/progressbar`. |

### 1.5 Pattern Detection Fixes (4 задачи)

| # | Задача | Описание |
|---|--------|----------|
| 1.5.1 | PatternJob детектор | ✅ Детекция CronJob/Job паттернов: batch processing, ETL, migrations. Триггеры: наличие CronJob/Job ресурсов, `restartPolicy: Never/OnFailure`. |
| 1.5.2 | PatternOperator детектор | ✅ Детекция operator pattern: CRD + Deployment с controller-manager, RBAC для CRD. Триггеры: наличие CRD + Deployment + ClusterRole с нестандартными resources. |
| 1.5.3 | Sidecar-детекция | Анализ ролей контейнеров: main app vs sidecar (envoy, fluent-bit, vault-agent, istio-proxy). По имени контейнера, image name, port patterns. |
| 1.5.4 | Init-контейнеры | ✅ Детекция init containers: wait-for-db, migrations, config-init. Шаблонизация с условным включением. |
| 1.5.5 | Topology Spread Constraints | Детекция `topologySpreadConstraints` в Pod spec. Шаблонизация maxSkew, topologyKey, whenUnsatisfiable. Best practice: auto-suggest для multi-replica Deployments |
| 1.5.6 | QoS Class анализ | ✅ Определение QoS класса (Guaranteed/Burstable/BestEffort) из resources. Warning если BestEffort в production. Рекомендации по доведению до Guaranteed |
| 1.5.7 | StatefulSet advanced patterns | ✅ Детекция ordinal-based config (pod-0 primary), headless Service auto-create, volumeClaimTemplate strategies, podManagementPolicy (Ordered/Parallel) |
| 1.5.8 | DaemonSet patterns | ✅ Auto-add tolerations (control-plane, not-ready), update strategy detection (RollingUpdate/OnDelete), resource limits warning (HIGH severity для DaemonSets) |
| 1.5.9 | Graceful shutdown | ✅ Генерация preStop hooks для web-серверов (`sleep 15`), terminationGracePeriodSeconds по типу workload (web: 30s, batch: 300s, db: 60s) |
| 1.5.10 | Pod Security Standards | ✅ Анализ PSS compliance (restricted/baseline/privileged). Flag `--pss-level=restricted`. Генерация securityContext: runAsNonRoot, capabilities.drop ALL, readOnlyRootFilesystem, seccompProfile |

---

## Phase 2: Complete Output Modes (v0.3.0)

**Цель**: Все стратегии генерации + архитектурные паттерны
**Задач**: 20 | **Оценка сложности**: High

### 2.1 Separate Generator (5 задач)

| # | Задача | Описание |
|---|--------|----------|
| 2.1.1 | Service grouping | Алгоритм группировки ресурсов в отдельные чарты: по label `app.kubernetes.io/name`, по namespace, по relationship graph (connected components). |
| 2.1.2 | Per-service chart generation | Генерация отдельного `Chart.yaml`, `values.yaml`, `templates/` для каждой группы. Имя чарта = имя сервиса. |
| 2.1.3 | Inter-chart dependencies | Генерация `Chart.yaml` dependencies: `condition`, `repository` (file://), `version`. Автодетекция зависимостей из relationship graph. |
| 2.1.4 | Shared values | Parent `values.yaml` с общими настройками (global.image.registry, global.env). Child charts наследуют через `.Values.global.*`. |
| 2.1.5 | Independent versioning | Каждый subchart с независимым `version` в `Chart.yaml`. Скрипт для bump версий отдельных чартов. |

### 2.2 Library Generator (4 задачи)

| # | Задача | Описание |
|---|--------|----------|
| 2.2.1 | Base library chart | `type: library` в `Chart.yaml`. Общие шаблоны: `_helpers.tpl` (fullname, labels, selectorLabels, serviceAccountName), `_deployment.tpl`, `_service.tpl`, `_ingress.tpl`. |
| 2.2.2 | Wrapper charts | Тонкие wrapper чарты для каждого сервиса. Только `Chart.yaml` (dependency на library) + `values.yaml`. Templates через `{{ include "library.deployment" . }}`. |
| 2.2.3 | DRY-шаблоны | Не дублировать boilerplate: named templates для общих блоков (resources, securityContext, probes, env). |
| 2.2.4 | Deckhouse lib-helm интеграция | Опция `--library-style deckhouse`: совместимость с Deckhouse `lib-helm`. Хелперы: `helm_lib_module_labels`, `helm_lib_priority_class`. |

### 2.3 Umbrella Generator (3 задачи)

| # | Задача | Описание |
|---|--------|----------|
| 2.3.1 | Parent chart structure | Root `Chart.yaml` с `dependencies[]`. Единый `helm install`. `charts/` директория с subcharts. |
| 2.3.2 | Cascading values | Parent `values.yaml` с секциями для каждого subchart. Override mechanism: `subchart.key: value`. |
| 2.3.3 | Conditional subcharts | `condition: subchart.enabled` в dependencies. Позволяет включать/отключать сервисы через values. |

### 2.4 Architecture-Specific Generation (5 задач)

| # | Задача | Описание |
|---|--------|----------|
| 2.4.1 | Multi-tenant charts | Генерация с изоляцией по tenant: namespace per tenant, ResourceQuota per tenant, NetworkPolicy between tenants. Values: `tenants: [{name, resources, networkPolicy}]`. |
| 2.4.2 | Feature-flag driven | Условная генерация компонентов: `{{ if .Values.features.monitoring }}` для ServiceMonitor, `{{ if .Values.features.ingress }}` для Ingress. Матрица флагов в values. |
| 2.4.3 | Environment-specific values | Генерация `values-dev.yaml`, `values-staging.yaml`, `values-prod.yaml`. Dev: 1 replica, debug logging, no PDB. Prod: 3+ replicas, PDB, resource limits, node affinity. |
| 2.4.4 | Monorepo support | Флаг `--monorepo`: генерация нескольких чартов из одного дерева. Структура: `charts/{service-a,service-b,shared-lib}/`. |
| 2.4.5 | Spot/Preemptible instance support | Генерация tolerations для spot nodes (AWS/GCP/Azure), graceful shutdown (`preStop` hook), PDB auto-generation. Values: `spot.enabled`, `spot.provider`. |
| 2.4.6 | Air-gapped environment support | Извлечение всех image references → `images.txt`. Генерация `values-airgap.yaml` с `global.imageRegistry`. Скрипт `mirror-images.sh` (skopeo/crane) для bulk copy |
| 2.4.7 | Kustomize-hybrid output | `--post-renderer-mode` — генерация `base/` + Kustomize overlays для post-rendering. Совместимость с Flux CD `postBuild` |
| 2.4.8 | Chart dependency management | Автодетекция common dependencies (postgresql, redis, mongodb) → `Chart.yaml` dependencies с condition/tags. Version constraints, alias для multiple instances |
| 2.4.9 | Cloud-specific Service annotations | Генерация LB annotations по cloud provider: AWS NLB/ALB, GCP ILB, Azure ILB. Flag `--cloud=aws\|gcp\|azure`. Session affinity шаблонизация |
| 2.4.10 | Ingress controller detection | Детекция controller type (nginx/traefik/haproxy) → генерация controller-specific annotations: canary, rate-limit, CORS, auth-url, rewrite-target |

### 2.5 Namespace Management (3 задачи)

| # | Задача | Описание |
|---|--------|----------|
| 2.5.1 | ResourceQuota template | Генерация `ResourceQuota` при multi-tenant: CPU/memory requests+limits, PVC count, pod count. |
| 2.5.2 | LimitRange template | Генерация `LimitRange` с default/min/max для containers. Предотвращение unbounded resource usage. |
| 2.5.3 | NetworkPolicy per namespace | Default deny-all + allow-list: same namespace, specific namespaces, specific CIDRs. Egress rules. |
| 2.5.4 | Auto-NetworkPolicy из Service анализа | Анализ Service→Pod relationships → генерация ingress rules. Детекция DB_HOST env vars → egress к database. Default: deny-all + allow DNS (UDP 53) |

---

## Phase 2.5: Security & Compliance (v0.2.5)

**Цель**: Production-ready security controls и compliance automation
**Задач**: 10 | **Оценка сложности**: High
**Automation rate**: 82% of compliance controls auto-fixed

### 2.6 Security & Compliance (10 задач)

| # | Задача | Threat Mitigated | Compliance | Auto-Fix | Priority |
|---|--------|------------------|------------|----------|----------|
| 2.6.1 | Pod Security Standards (PSS) auto-migration | Container escape, privilege escalation, host compromise | CIS 5.2.x, PCI-DSS 2.2.4, FedRAMP AC-6 | ✅ Yes | P1 |
| | | **Описание**: Анализ securityContext → классификация (privileged/baseline/restricted). Auto-add: runAsNonRoot, readOnlyRootFilesystem, capabilities.drop ALL, seccompProfile RuntimeDefault. Генерация namespace labels для PSS enforcement (K8s 1.25+). Совместимость: PSP migration path. |
| 2.6.2 | NetworkPolicy default-deny + allow patterns | Lateral movement, data exfiltration, unauthorized service access | CIS 5.3.2, PCI-DSS 1.2.1, FedRAMP SC-7 | ✅ Yes | P1 |
| | | **Описание**: Auto-generate default-deny NetworkPolicy + allow rules. Детекция: Service ports → ingress rules. Env vars (DATABASE_URL, REDIS_HOST) → egress rules. Always include: allow-dns (UDP 53 to kube-system). Values: networkPolicy.ingress[], .egress[]. |
| 2.6.3 | RBAC least-privilege auto-generation | Privilege escalation, unauthorized API access, cluster compromise | CIS 5.1.5, PCI-DSS 7.1, FedRAMP AC-6 | ✅ Yes | P1 |
| | | **Описание**: Детекция K8s API interaction (kubectl, client-go в image/command). Infer permissions: read-only (get/list), operator (create/update), admin (delete). Generate ServiceAccount + Role/ClusterRole + Binding. automountServiceAccountToken: false (default). |
| 2.6.4 | Resource limits auto-configuration | DoS attacks, resource exhaustion, noisy neighbor, cryptomining | CIS 5.2.13, FedRAMP SC-6 | ✅ Yes | P1 |
| | | **Описание**: Workload type detection (web/worker/database) → RESOURCE_PROFILES. Set requests = limits для Guaranteed QoS. Generate LimitRange at namespace level (enforce defaults). QoS class validation. |
| 2.6.5 | Secret management — External Secrets migration | Hardcoded secrets in Git, secret sprawl, rotation failure | CIS 5.4.1, PCI-DSS 3.4, SOC2 CC6.1 | ✅ Yes | P1 |
| | | **Описание**: Scan for hardcoded secrets (Secret kind with literal data, values.yaml passwords). Convert to ExternalSecret (external-secrets-operator). Generate secretStoreRef. Pre-commit hook: gitleaks scan, block if secrets detected. Support: Vault, AWS SM, GCP SM, Azure KV. |
| 2.6.6 | Image security enforcement | Supply chain attacks, vulnerable dependencies, malicious images | CIS 5.4.2, NIST SP 800-190, FedRAMP SA-10 | ✅ Yes | P2 |
| | | **Описание**: imagePullPolicy logic: Always для :latest, IfNotPresent для tags. Detect untagged images (implicit :latest). Add imagePullSecrets для private registry. Trivy/Anchore scan integration (CI). Warning: :latest in production. |
| 2.6.7 | Audit logging — K8s Audit Policy | Undetected intrusions, insider threats, compliance violations | CIS 3.2.1, PCI-DSS 10.2, SOC2 CC7.2, FedRAMP AU-2 | ⚠️ Partial | P3 |
| | | **Описание**: Generate Audit Policy YAML (cluster-level config). Rules: Secret access (RequestResponse), RBAC changes (Request), pod exec/attach (Request). Note: Cluster admin must apply manually. Reference manifest only. |
| 2.6.8 | Ingress security — TLS + AuthN/AuthZ | MITM attacks, unauthorized access, credential theft | CIS 5.3.1, PCI-DSS 4.1, SOC2 CC6.6 | ✅ Yes | P2 |
| | | **Описание**: Auto-add spec.tls + cert-manager.io/cluster-issuer annotation. force-ssl-redirect annotation. Auth annotations (nginx/traefik): basic, oauth2. Warning if TLS disabled. ClusterIssuer dependency (cert-manager). |
| 2.6.9 | Admission control — Kyverno/OPA policy generation | Policy violations, configuration drift, compliance failures | CIS 5.1.1, SOC2 CC8.1, FedRAMP CM-7 | ✅ Yes | P2 |
| | | **Описание**: Scan manifests for violations (missing labels, privileged containers, no resources). Generate ClusterPolicy (Kyverno) or ConstraintTemplate (OPA). Common policies: require-labels, deny-privileged, require-resources, require-probes. validationFailureAction: enforce (prod) / audit (dev). |
| 2.6.10 | Supply chain security — SBOM + provenance | Supply chain attacks, compromised dependencies, unsigned images | EO 14028, NIST SSDF, SLSA L3 | ⚠️ Partial | P3 |
| | | **Описание**: Generate CI pipeline templates (.github/workflows, .gitlab-ci.yml). Steps: syft SBOM generation, cosign signing (keyless/key-based), cosign attach sbom. Verification instructions. Note: CI/CD integration, not chart templates. |

---

## Phase 3: Deckhouse Integration (v0.4.0)

**Цель**: Полная поддержка Deckhouse-специфичных ресурсов
**Задач**: 22 | **Оценка сложности**: High

### 3.1 Deckhouse CRD Processors (7 задач)

| # | Задача | CRD | Описание |
|---|--------|-----|----------|
| 3.1.1 | ModuleConfig | `deckhouse.io/v1alpha1` | Конфигурация модулей: enabled, version, settings. Шаблонизация settings как values. |
| 3.1.2 | IngressNginxController | `deckhouse.io/v1` | Inlet type (LoadBalancer/HostPort/HostWithFailover), ControllerVersion, customErrors, resourcesRequests. |
| 3.1.3 | ClusterAuthorizationRule | `deckhouse.io/v1` | Subjects, accessLevel (User/PrivilegedUser/Editor/Admin/ClusterEditor/ClusterAdmin), namespaces, allowScale. |
| 3.1.4 | NodeGroup | `deckhouse.io/v1` | nodeType (CloudEphemeral/CloudPermanent/Static), disruptions, kubelet, cloudInstances (minPerZone, maxPerZone). |
| 3.1.5 | DexAuthenticator | `deckhouse.io/v1` | applicationDomain, sendAuthorizationHeader, applicationIngressClassName. |
| 3.1.6 | User / Group | `deckhouse.io/v1` | User: email, password (bcrypt), groups, ttl. Group: members. |
| 3.1.7 | OpenstackInstanceClass / AWSInstanceClass / etc. | `deckhouse.io/v1` | Cloud-provider specific: flavor, image, rootDiskSize, mainNetwork. |

### 3.2 Deckhouse Module Structure (3 задачи)

| # | Задача | Описание |
|---|--------|----------|
| 3.2.1 | Module scaffold generation | Флаг `--output-format deckhouse-module`. Структура: `templates/`, `charts/` (lib-helm), `images/`, `hooks/` (placeholder), `openapi/`, `Chart.yaml`, `values.yaml`. |
| 3.2.2 | lib-helm интеграция | Автодобавление `charts/helm_lib/` как dependency. Использование хелперов: `helm_lib_module_labels`, `helm_lib_module_image`, `helm_lib_priority_class`. |
| 3.2.3 | OpenAPI schema generation | Генерация `openapi/config-values.yaml` и `openapi/values.yaml` (Deckhouse-specific JSON Schema формат). Маппинг Go types → OpenAPI. |

### 3.3 Deckhouse Pattern Detection (3 задачи)

| # | Задача | Описание |
|---|--------|----------|
| 3.3.1 | Deckhouse pattern detector | Детекция Deckhouse-паттернов: ModuleConfig + IngressNginxController → Deckhouse cluster. NodeGroup + ClusterAuthorizationRule → managed nodes. |
| 3.3.2 | Deckhouse-specific рекомендации | Если Deckhouse detected: рекомендовать lib-helm, module structure, Deckhouse-specific security (DexAuthenticator vs generic OAuth). |
| 3.3.3 | Version compatibility | Валидация CRD apiVersions против Deckhouse versions (1.57+). Warning при deprecated CRD fields. |

### 3.4 Monitoring Processors (3 задачи)

| # | Задача | CRD | Описание |
|---|--------|-----|----------|
| 3.4.1 | ServiceMonitor / PodMonitor | `monitoring.coreos.com/v1` | Извлечение endpoints, namespaceSelector, selector, interval, scrapeTimeout, path. |
| 3.4.2 | PrometheusRule | `monitoring.coreos.com/v1` | Извлечение groups, rules (alert, expr, for, labels, annotations). Шаблонизация expr с variables. |
| 3.4.3 | GrafanaDashboard | ConfigMap + label | Детекция ConfigMap с label `grafana_dashboard: "1"`. Вынос JSON dashboard в `files/` (external file support). |

### 3.5 Modern K8s Patterns (6 задач)

| # | Задача | API | Описание |
|---|--------|-----|----------|
| 3.5.1 | Gateway API — HTTPRoute | `gateway.networking.k8s.io/v1` | Извлечение parentRefs, hostnames, rules (matches, backendRefs, filters). Замена Ingress для mesh-native routing. |
| 3.5.2 | Gateway API — Gateway | `gateway.networking.k8s.io/v1` | Извлечение gatewayClassName, listeners (port, protocol, hostname, tls). |
| 3.5.3 | KEDA ScaledObject | `keda.sh/v1alpha1` | Извлечение scaleTargetRef, triggers (type, metadata), minReplicaCount (scale-to-zero), maxReplicaCount. |
| 3.5.4 | KEDA TriggerAuthentication | `keda.sh/v1alpha1` | Извлечение secretTargetRef, env, podIdentity. Связь с ScaledObject. |
| 3.5.5 | Topology Spread Constraints | Pod spec field | Детекция `topologySpreadConstraints`: maxSkew, topologyKey (zone/node), whenUnsatisfiable. Шаблонизация. |
| 3.5.6 | cert-manager Certificate/Issuer | `cert-manager.io/v1` | Извлечение Certificate (dnsNames, issuerRef, secretName), ClusterIssuer (ACME, selfSigned). Аннотации в Ingress. |
| 3.5.7 | Gateway API — GRPCRoute/TLSRoute | `gateway.networking.k8s.io/v1` | Извлечение GRPCRoute (services, filters), TLSRoute (sniHosts, backendRefs). Шаблонизация с условным включением |
| 3.5.8 | External DNS annotations | — | Детекция Ingress/Service hostnames → добавление `external-dns.alpha.kubernetes.io/hostname`, `/ttl`. Values: `externalDNS.enabled`, `externalDNS.provider` |
| 3.5.9 | Argo Rollouts | `argoproj.io/v1alpha1` | Извлечение Rollout (strategy: canary/blueGreen), AnalysisTemplate (metrics, provider). Преобразование Deployment → Rollout |
| 3.5.10 | Flagger Canary | `flagger.app/v1beta1` | Извлечение Canary (targetRef, analysis, progressDeadlineSeconds). Генерация MetricTemplate для Prometheus |

---

## Phase 4: Source Expansion (v0.5.0)

**Цель**: Извлечение ресурсов из кластера и Git
**Задач**: 14 | **Оценка сложности**: High

### 4.1 Cluster Extractor (6 задач)

| # | Задача | Описание |
|---|--------|----------|
| 4.1.1 | client-go интеграция | `k8s.io/client-go` dependency. Discovery API для динамических ресурсов. |
| 4.1.2 | Аутентификация | Kubeconfig (default `~/.kube/config`), `--kubeconfig` flag, `--context` flag, in-cluster config. |
| 4.1.3 | Resource extraction | `GET` ресурсов по GVR (GroupVersionResource). Список: Deployment, Service, ConfigMap, Secret, Ingress, StatefulSet, DaemonSet, CronJob, HPA, PDB, NetworkPolicy, RBAC, SA. |
| 4.1.4 | Filtering | `--namespace` (default: all), `--selector` (label selector), `--field-selector`, `--exclude-namespace` (kube-system, kube-public). |
| 4.1.5 | Secret handling | `--include-secrets` flag (default: false). Маскирование sensitive data. Опция `--secret-strategy`: mask/include/external-secret. |
| 4.1.6 | Pagination | `client-go` list with `Continue` token. Chunk size: 500 resources. Progress reporting для больших кластеров. |

### 4.2 GitOps Extractor (5 задач)

| # | Задача | Описание |
|---|--------|----------|
| 4.2.1 | go-git интеграция | `github.com/go-git/go-git/v5`. Clone, shallow clone (`--depth 1`). |
| 4.2.2 | Аутентификация | HTTP basic (token), SSH key (`--ssh-key`), Git credential helper. |
| 4.2.3 | YAML discovery | Рекурсивный поиск `*.yaml`, `*.yml` в repo. Exclude patterns: `.git/`, `vendor/`, `node_modules/`. |
| 4.2.4 | Kustomize overlay parsing | Детекция `kustomization.yaml`. Запуск `kustomize build` или парсинг `resources:`, `patches:`, `configMapGenerator:`. |
| 4.2.5 | ArgoCD/Flux manifest parsing | ArgoCD `Application` → extract source repo + path. Flux `GitRepository` + `Kustomization` → extract source. |

### 4.3 Multi-Source Merge (3 задачи)

| # | Задача | Описание |
|---|--------|----------|
| 4.3.1 | Resource deduplication | По GVK + namespace + name. Стратегия: latest wins, cluster wins, file wins (configurable). |
| 4.3.2 | Conflict resolution | При конфликте: diff + warning. `--conflict-strategy`: error/warn/merge. Three-way merge для values. |
| 4.3.3 | Source priority | Configurable priority: `cluster > file > git` (default). `--source-priority` flag. |

---

## Phase 5: Advanced Analysis (v0.6.0)

**Цель**: Глубокий анализ и интеллектуальные рекомендации
**Задач**: 38 | **Оценка сложности**: Very High

### 5.1 Auto-Fix Engine (5 задач)

| # | Задача | Описание |
|---|--------|----------|
| 5.1.1 | SecurityContext auto-add | Добавление: `runAsNonRoot: true`, `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`. Условно: если image не root-based. |
| 5.1.2 | Resources auto-add | Эвристики: web app → CPU 100m-500m, RAM 128Mi-512Mi. DB → CPU 500m-2, RAM 512Mi-4Gi. Worker → CPU 250m-1, RAM 256Mi-1Gi. Configurable profiles. |
| 5.1.3 | Health probes auto-add | HTTP probe: если container port 80/8080/3000 → `httpGet /healthz`. TCP probe: для всех остальных портов. Startup probe для slow-start apps (>30s). |
| 5.1.4 | PDB auto-generation | Если replicas ≥ 2: `minAvailable: 50%` или `maxUnavailable: 1`. Для StatefulSet: `maxUnavailable: 1`. |
| 5.1.5 | `dhg fix` command | CLI command: `dhg fix -f manifests/ -o fixed/`. Apply all auto-fixes. `--fix-category`: security, resources, ha, all. `--dry-run` для preview. |
| 5.1.6 | PSS auto-fix | Автоматическое доведение до restricted PSS level: добавление runAsNonRoot, capabilities.drop ALL, readOnlyRootFilesystem, seccompProfile RuntimeDefault. Report нарушений |
| 5.1.7 | Graceful shutdown auto-add | Автодобавление preStop hooks + terminationGracePeriodSeconds. Эвристики по image name (nginx→sleep 15, java→SIGTERM handler hint) |

### 5.2 CRD Support (3 задачи)

| # | Задача | Описание |
|---|--------|----------|
| 5.2.1 | Generic CRD processing | Обработка любых CRD: extract GVK, spec fields, status fields. Generic template generation. |
| 5.2.2 | CRD schema extraction | Из `spec.versions[].schema.openAPIV3Schema` → values schema. Nested objects → nested values. |
| 5.2.3 | CRD installation | Генерация `crds/` директории в чарте. CRD ресурсы устанавливаются до templates. Warning: CRD updates не managed Helm. |

### 5.3 Dependency Analysis (3 задачи)

| # | Задача | Описание |
|---|--------|----------|
| 5.3.1 | DOT/Graphviz graph | `dhg graph -f manifests/ -o graph.dot`. Узлы: ресурсы (цвет по kind). Рёбра: relationships (стиль по типу). |
| 5.3.2 | Circular dependency detection | BFS/DFS на relationship graph. Если цикл → warning с описанием цикла и рекомендацией по разрыву. |
| 5.3.3 | Decomposition recommendations | На основе graph: предложить split points для separate generator. Метрика: coupling (edges between groups) / cohesion (edges within group). |

### 5.4 Migration Support (3 задачи)

| # | Задача | Описание |
|---|--------|----------|
| 5.4.1 | Drift detection | `dhg diff --existing-chart chart/ -f manifests/`. Сравнение: templates (structural), values (data), helpers (logic). Diff output: added/removed/changed. |
| 5.4.2 | Migration plan generation | `dhg migrate --from chart/ --source manifests/`. Генерация пошагового плана: что добавить, что изменить, что удалить. Markdown output. |
| 5.4.3 | Values backward compatibility | При migration: сохранить старые keys как deprecated aliases. Генерация `_migrate.tpl` с coalesce logic: `{{ coalesce .Values.new.key .Values.old.key }}`. |

### 5.5 Smart Analysis (4 задачи)

| # | Задача | Описание |
|---|--------|----------|
| 5.5.1 | Cost estimation | Маппинг CPU/RAM → $ на основе cloud pricing (AWS/GCP/Azure). `dhg analyze --cost`. Output: monthly cost per service, total, recommendations (right-sizing). |
| 5.5.2 | Resource right-sizing | Анализ requests vs limits ratio. Warning: limits > 10x requests. Рекомендации: "reduce CPU limit from 4 to 1 based on request 100m". |
| 5.5.3 | Compliance-as-Code | Генерация Kyverno `ClusterPolicy` из best practices findings. Пример: если нет securityContext → policy "require-security-context". |
| 5.5.4 | Progressive Delivery | Если Deployment с strategy RollingUpdate: предложить Argo Rollouts `Rollout` с canary. Генерация `Rollout` + `AnalysisTemplate` шаблонов. |
| 5.5.5 | Policy-as-Code генерация | Генерация OPA Gatekeeper `ConstraintTemplate` + `Constraint` или Kyverno `ClusterPolicy` из best practices findings. Values: `policy.engine: gatekeeper\|kyverno\|none` |
| 5.5.6 | PDB автогенерация | Анализ replicas > 1 → генерация PodDisruptionBudget. StatefulSet: `maxUnavailable: 1`. Deployment: `minAvailable: 50%`. Warning если replicas=1 |
| 5.5.7 | Pod anti-affinity автогенерация | Для replicas > 1: preferred anti-affinity по hostname. Для HA: required anti-affinity по zone. Flag `--affinity=zone\|node\|none` |
| 5.5.8 | Persistent Volume best practices | Анализ PVC access modes, StorageClass selection. VolumeSnapshot template для backup. Warning: RWX requires shared storage. Flag `--enable-snapshots` |
| 5.5.9 | Helm Schema validation | Генерация `values.schema.json` с conditional validation (if ingress.enabled → hostname required). oneOf/anyOf, pattern matching, custom error messages |

### 5.6 Advanced Templating (2 задачи)

| # | Задача | Описание |
|---|--------|----------|
| 5.6.1 | Post-renderer support | Генерация Kustomize overlays (`kustomization.yaml` + patches) для post-rendering. Use case: security injection, environment-specific patches. |
| 5.6.2 | Operator pattern scaffold | Если PatternOperator detected: генерация CRD + controller Deployment + RBAC + ServiceAccount. Placeholder code для reconciler. |

### 5.7 Secret Management Integration (5 задач)

| # | Задача | Описание |
|---|--------|----------|
| 5.7.1 | External Secrets Operator | Флаг `--secret-strategy eso`. Генерация `SecretStore` + `ExternalSecret` вместо plain Secret. Provider config в values: AWS SM, Vault, GCP SM, Azure KV. |
| 5.7.2 | Sealed Secrets | Флаг `--secret-strategy sealed`. Output `SealedSecret` (bitnami). Требует `kubeseal` CLI для шифрования. |
| 5.7.3 | Vault CSI | Генерация `SecretProviderClass` для secrets as volumes. Mount path, object mapping, sync as K8s Secret option. |
| 5.7.4 | Vault Agent Inject | Добавление annotations: `vault.hashicorp.com/agent-inject: "true"`, `vault.hashicorp.com/role`, `vault.hashicorp.com/agent-inject-secret-*`. |
| 5.7.5 | Reloader annotations | Детекция apps с mounted ConfigMaps/Secrets → добавление `reloader.stakater.com/auto: "true"` для auto-restart. |
| 5.7.6 | SOPS / Helm Secrets | `--secret-strategy sops`. Разделение на `values.yaml` + `secrets.yaml` (encrypted). Генерация `.sopsconfig` с KMS hints. Инструкции по `helm-secrets` plugin |
| 5.7.7 | Secrets example template | Генерация `secrets.example.yaml` с placeholder values для документации. Gitignore rule для `secrets.yaml` |

### 5.8 Service Mesh Integration (8 задач)

| # | Задача | Описание |
|---|--------|----------|
| 5.8.1 | Istio sidecar injection | Детекция mesh: namespace label `istio-injection: enabled` или pod annotation `sidecar.istio.io/inject: "true"`. Шаблонизация. |
| 5.8.2 | Istio traffic management | Генерация `VirtualService` из Service + Ingress. `DestinationRule` с circuit breaking. `PeerAuthentication` mTLS STRICT. |
| 5.8.3 | Linkerd integration | Annotation `linkerd.io/inject: enabled`. `ServiceProfile` для per-route metrics. `TrafficSplit` для canary. |
| 5.8.4 | Egress policies | Детекция external URLs в env vars / ConfigMaps → генерация Istio `ServiceEntry` для each external host. |
| 5.8.5 | Istio VirtualService canary patterns | Расширенная генерация: traffic splitting (stable/canary weights), header-based routing (x-canary: true), retry policies (attempts, perTryTimeout, retryOn). Values: `istio.trafficSplit.{stable,canary}`, `istio.retries.*` |
| 5.8.6 | Istio DestinationRule advanced | Load balancing (LEAST_REQUEST/ROUND_ROBIN), outlier detection (consecutiveErrors, interval, baseEjectionTime), connection pool limits. Circuit breaker patterns |
| 5.8.7 | Istio AuthorizationPolicy zero-trust | Fine-grained RBAC: по principals (service accounts), methods (GET/POST), paths. Генерация allow-list policies. Integration с existing RBAC rules |
| 5.8.8 | Istio multi-cluster ServiceEntry | Генерация ServiceEntry для cross-cluster traffic. Detection: multi-cluster annotations, ServiceEntry resources. Locality-aware routing |

### 5.9 Observability-as-Code (9 задач)

| # | Задача | Описание |
|---|--------|----------|
| 5.9.1 | OpenTelemetry instrumentation | Детекция runtime (Java/Python/Node.js по image) → генерация `Instrumentation` CRD. Auto-add `OTEL_RESOURCE_ATTRIBUTES`. |
| 5.9.2 | Prometheus annotations | Auto-add `prometheus.io/scrape: "true"`, `prometheus.io/port`, `prometheus.io/path` к Services с HTTP портами. |
| 5.9.3 | Basic alerting rules | Генерация `PrometheusRule`: `KubePodCrashLooping` (restarts > 5), `KubeContainerOOMKilled`, `KubePodNotReady` (>15m). Configurable thresholds. |
| 5.9.4 | OTEL auto-instrumentation advanced | Language detection (Java/Python/Node.js/Go), exporter config (OTLP endpoint), sampling strategies (parentbased_traceidratio). Env vars: OTEL_SERVICE_NAME, OTEL_EXPORTER_OTLP_ENDPOINT, OTEL_TRACES_SAMPLER |
| 5.9.5 | Distributed tracing W3C context | Auto-inject W3C Trace Context headers (traceparent, tracestate). Env vars: OTEL_PROPAGATORS=tracecontext,baggage. Backend support: Jaeger/Zipkin/Tempo/OTLP |
| 5.9.6 | SLO-based alerting (Sloth) | Генерация Sloth PrometheusServiceLevel CRD. Auto-detect SLI metrics (http_requests_total, http_request_duration_seconds). Error budget alerts (page/ticket speed). Values: `slo.objectives[].{name,target,window}` |
| 5.9.7 | Prometheus recording rules SLI | Pre-aggregate SLI metrics: latency percentiles (p50/p95/p99), availability ratio, error ratio, QPS. Recording rules для faster dashboards. Expr: histogram_quantile, rate calculations |
| 5.9.8 | Grafana dashboard auto-generation | Генерация GrafanaDashboard CRD из Prometheus metrics. Panels: SLI (availability, latency, error rate), resources (CPU, memory, network), custom metrics. JSON schema compliance |
| 5.9.9 | Custom metrics ServiceMonitor | Детекция custom metrics ports (METRICS_PORT env, non-standard ports). Генерация ServiceMonitor с metricRelabelings (drop high-cardinality). Alternative: prometheus.io annotations |

### 5.10 Cloud-Native Patterns (4 задачи)

| # | Задача | Описание |
|---|--------|----------|
| 5.10.1 | Workload Identity | Детекция SA annotations (EKS IRSA, GKE WI, Azure WI). Шаблонизация с `workloadIdentity.provider` enum. |
| 5.10.2 | GPU/TPU workloads | Детекция `nvidia.com/gpu` в resources. Шаблонизация: gpu.enabled, gpu.count, gpu.type, tolerations. MIG support. |
| 5.10.3 | Velero backup annotations | Добавление `backup.velero.io/backup-volumes`. Опциональная генерация `Schedule` CRD. Pre/post hooks. |
| 5.10.4 | Windows container support | Детекция Windows images → nodeSelector `kubernetes.io/os: windows` + tolerations. Multi-OS values. |

---

## Phase 6: Production Polish (v1.0.0)

**Цель**: Production-ready release
**Задач**: 26 | **Оценка сложности**: Medium-High

### 6.0 Validation & Testing Pipeline (6 задач)

| # | Задача | Инструмент | Описание |
|---|--------|------------|----------|
| 6.0.1 | Kubeconform интеграция | kubeconform | Валидация шаблонов против K8s JSON schemas. CRD-aware (custom schemas). Strict mode: fail on additional properties. |
| 6.0.2 | Pluto интеграция | pluto | Детекция deprecated APIs в source YAML. Report: deprecated resource, replacement API, removal version. |
| 6.0.3 | helm-unittest генерация | helm-unittest | Auto-scaffold: snapshot tests для каждого template, value permutation tests (enabled=true/false matrix), assertion tests для labels/annotations. |
| 6.0.4 | Conftest policy генерация | conftest (OPA) | Генерация Rego policies из best practices: `deny_privileged_containers`, `deny_no_resource_limits`, `deny_latest_tag`. |
| 6.0.5 | K8s version matrix | — | Валидация совместимости с K8s 1.27–1.32. Matrix test в CI: template + kubeconform per version. |
| 6.0.6 | CI pipeline template | GitHub Actions | Генерация `.github/workflows/chart-ci.yaml`: lint → validate → unit → security → integration. |
| 6.0.7 | chart-testing (ct) интеграция | chart-testing | Генерация `ct.yaml` конфига. `ct lint` + `ct install` в CI. Авто-детекция изменённых чартов в monorepo |
| 6.0.8 | Conftest policy library | conftest | Набор готовых Rego policies: `deny_no_resource_limits`, `deny_privileged`, `deny_latest_tag`, `require_labels`, `require_probes`. Генерация `policy/` директории |

### 6.1 Supply Chain Security (4 задачи)

| # | Задача | Описание |
|---|--------|----------|
| 6.1.1 | Chart signing | Генерация `cosign sign` команд. Keyless signing через OIDC (GitHub Actions). OCI attestation. |
| 6.1.2 | SBOM generation | Syft интеграция: scan images в чарте → SBOM. Форматы: SPDX 3.0, CycloneDX. Attach к OCI artifact. |
| 6.1.3 | SLSA provenance | Level 2+ attestation. Build metadata: workflow ID, commit SHA, builder identity. `cosign verify-attestation` docs. |
| 6.1.4 | OCI artifact metadata | Auto-add annotations: `org.opencontainers.image.source`, `org.opencontainers.image.version`, `org.opencontainers.image.description`. |

### 6.2 Performance (3 задачи)

| # | Задача | Описание |
|---|--------|----------|
| 6.2.1 | Бенчмарки | `testing.B` benchmarks: 10/100/1000/5000 ресурсов. Metrics: time, memory, allocations. Baseline для regression detection. |
| 6.2.2 | Параллельная обработка | `sync.WaitGroup` + горутины: parallel file reading, parallel processing per-kind, parallel template rendering. Worker pool pattern. |
| 6.2.3 | Memory optimization | `sync.Pool` для повторного использования буферов. Streaming YAML parsing (не загружать все в RAM). Profile: `pprof`. |

### 6.3 Distribution (6 задач)

| # | Задача | Описание |
|---|--------|----------|
| 6.3.1 | GitHub Actions CI/CD | Workflow: lint (golangci-lint) → test → build → release (goreleaser). Trigger: tag push `v*`. |
| 6.3.2 | Multi-platform бинарники | goreleaser: linux/darwin/windows × amd64/arm64. SHA256 checksums. |
| 6.3.3 | Docker image | Multi-stage build. `ghcr.io/deckhouse/helm-generator:latest` + `:<version>`. Scratch-based (minimal size). |
| 6.3.4 | OCI registry | Публикация как OCI artifact. `helm push` compatible. |
| 6.3.5 | Homebrew formula | `brew tap deckhouse/tap && brew install dhg`. Auto-update при release. |
| 6.3.6 | DEB/RPM пакеты | nfpm интеграция в goreleaser. Apt/Yum repository. |

### 6.4 Documentation (4 задачи)

| # | Задача | Описание |
|---|--------|----------|
| 6.4.1 | Architecture Decision Records | ADR-001: Modular architecture. ADR-002: Universal generator first. ADR-003: Value processor design. ADR-004: Relationship detection algorithm. |
| 6.4.2 | Developer guide | Как добавить процессор: interface, registration, tests. Как добавить детектор: interface, scoring, tests. Как добавить generator: interface, templates. |
| 6.4.3 | User guide | Quick start, CLI reference, examples (single app, microservices, stateful, Deckhouse). Troubleshooting. |
| 6.4.4 | API documentation | GoDoc comments на все exported types/functions. `go doc` compatible. Hosted на pkg.go.dev. |

### 6.5 Plugin System (3 задачи)

| # | Задача | Описание |
|---|--------|----------|
| 6.5.1 | External processors | Go plugins (`plugin.Open`) или subprocess (stdin/stdout JSON). Interface: `Process(resource) → ProcessedResource`. Discovery: `~/.dhg/plugins/`. |
| 6.5.2 | Config file `.dhg.yaml` | Persistent generation settings: default output mode, template style, enabled processors, exclude patterns, secret strategy. |
| 6.5.3 | Custom template overrides | `--template-dir custom/`: user templates override built-in. Merge strategy: user template wins. Fallback to built-in. |

---

## Phase 7: Production Operations (v0.7.0)

**Цель**: Production-ready Day 2 operations, operational excellence
**Задач**: 40 | **Оценка сложности**: High
**Automation rate**: 78% of operational work auto-generated

### 7.1 Backup & Restore (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 7.1.1 | Velero Schedule generation | Auto-generate backup schedules for StatefulSets/PVCs. Detection: StatefulSet/PVC presence. Configurable: cron (default: `0 2 * * *`), retention (720h), storage location. Template: `velero.io/v1.Schedule` with snapshotVolumes, ttl, includedNamespaces | P1 |
| 7.1.2 | PVC snapshot annotations | Add `backup.velero.io/backup-volumes` to StatefulSet pod templates. Opt-in per workload via `backup.enabled`. Auto-detect critical volumes (db, redis, etc.) | P1 |
| 7.1.3 | Backup hooks (pre/post) | Generate pre-backup hooks for DB dumps (postgres `pg_dump`, mysql `mysqldump`). Post-backup verification jobs. Hook detection: database workload type. Template: `backup.velero.io/backup-hook-*` annotations | P2 |
| 7.1.4 | Restore instructions | Generate NOTES.txt section with `velero restore create` commands. Document RTO expectations. Include validation steps. Reference disaster recovery runbook | P3 |

### 7.2 Multi-Tenancy (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 7.2.1 | Tenant-aware chart generation | Generate charts with per-tenant values overlay. Namespace isolation, resource quotas, network policies. Detection: `--multi-tenant` flag or multiple `app.kubernetes.io/part-of` values. Values: `multitenancy.tenants[]` array | P1 |
| 7.2.2 | ResourceQuota auto-generation | Calculate quotas from deployment requests × safety_factor (1.5x default). Generate `ResourceQuota` per namespace. LimitRange for defaults. Values: `tenant.resources.{cpu,memory,pvcCount,maxPods}` | P1 |
| 7.2.3 | Default-deny NetworkPolicy | Auto-generate baseline deny-all + allow DNS (UDP 53 to kube-system). Infer allow-list from Service relationships. Template: `networkPolicy.enabled`, `networkPolicy.ingress[]`, `networkPolicy.egress[]` | P1 |
| 7.2.4 | Cost attribution labels | Add tenant labels for cost tracking (AWS Cost Explorer, GCP labels, Azure tags). Standard labels: `tenant`, `cost-center`, `project`, `environment`. Propagate to all resources. Integration with Kubecost/OpenCost | P2 |

### 7.3 Capacity Planning (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 7.3.1 | HPA auto-generation | Detect traffic-serving workloads (Service exposed) → HPA. Multi-metric (CPU/memory/custom). Behavior tuning: scaleDown stabilization (300s), scaleUp policies. Detection: replicas > 1. Values: `autoscaling.hpa.{minReplicas,maxReplicas,targetCPU,targetMemory,customMetrics[]}` | P1 |
| 7.3.2 | VPA recommendations | Generate VPA in `updateMode: Off` for insights only. Template with resource policies (containerName, mode, min/max resources). Conflict detection: warn if HPA+VPA on same metric. Values: `autoscaling.vpa.{enabled,updateMode,resourcePolicy}` | P2 |
| 7.3.3 | PDB smart defaults | Auto-generate PDB if replicas > 1. StatefulSet: `maxUnavailable: 1` (rolling updates). Deployment: `minAvailable: 50%` (HA). Production workloads: stricter PDB. Values: `pdb.{enabled,minAvailable,maxUnavailable}` | P1 |
| 7.3.4 | Capacity forecasting | Analyze historical requests → project growth (linear regression). Recommend cluster size, node pools. Integration with Goldilocks/VPA Recommender. Generate capacity report. Warning: resource exhaustion risk | P3 |

### 7.4 Cost Optimization (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 7.4.1 | Spot instance template | Generate tolerations/nodeSelector for AWS Spot (lifecycle=spot), GCP Preemptible, Azure Spot. Provider detection: `--cloud-provider` flag. PreStop hook for graceful termination. Values: `spot.{enabled,provider,toleration,nodeSelector,interruptionHandling}` | P2 |
| 7.4.2 | Right-sizing analysis | Detect overprovisioned resources (limits >> requests, ratio >10x). Suggest VPA-based tuning. Flag wasteful configs. Recommend optimal requests/limits. Values: `costOptimization.rightSizing.{enabled,requestsLimitsRatio}` | P2 |
| 7.4.3 | Cost estimation | Map CPU/RAM → cloud pricing (AWS/GCP/Azure $/hour). Generate monthly cost report per workload. Integration with Kubecost/OpenCost APIs. Output: cost breakdown, optimization recommendations | P3 |
| 7.4.4 | Cluster autoscaler hints | Add annotations: `cluster-autoscaler.kubernetes.io/safe-to-evict: "true"` (stateless pods). PDB generation for HA workloads. Prevent CA eviction for stateful. Resource requests validation (CA requires requests) | P2 |

### 7.5 Operational Excellence (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 7.5.1 | Probe auto-generation | HTTP services (port 80/8080/3000) → httpGet liveness/readiness probes. Non-HTTP → tcpSocket. Auto-detect ports from Service. Paths: `/healthz` (liveness), `/ready` (readiness). Values: `probes.{liveness,readiness,startup}.{enabled,path,port,initialDelaySeconds,periodSeconds}` | P1 |
| 7.5.2 | Startup probe for slow apps | Detect Java/large images (>500MB) → startup probe (12 failures × 5s = 60s max). Prevents liveness killing slow boot. Detection: image analysis, language detection. Values: `probes.startup.{enabled,failureThreshold,periodSeconds}` | P1 |
| 7.5.3 | Graceful shutdown | Generate preStop hooks: `sleep 15` (web servers, connection drain), custom SIGTERM handlers. terminationGracePeriodSeconds by workload type (web: 30s, worker: 60s, db: 120s). Values: `gracefulShutdown.{enabled,preStopDelay,terminationGracePeriodSeconds}` | P1 |
| 7.5.4 | Probe best practices validation | Warn: liveness = readiness (anti-pattern). Check p99 startup time for initialDelay. Flag missing probes (HIGH priority). Validate probe timeouts < periodSeconds. Readiness must be faster than liveness | P2 |

### 7.6 GitOps Patterns (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 7.6.1 | ArgoCD Application generation | Generate `Application` CRD with sync policies. Auto/manual sync, prune, selfHeal. Retry backoff (duration: 5s, factor: 2, maxDuration: 3m). Values: `gitops.{enabled,provider=argocd,repo,branch,path,project,syncWave,autoPrune,selfHeal}` | P1 |
| 7.6.2 | Flux HelmRelease generation | Generate `HelmRelease` + `HelmRepository` CRDs. Interval (5m default), upgrade/install remediation (retries: 3). Values schema integration. Values: `gitops.{provider=flux,helmRepo,interval}` | P1 |
| 7.6.3 | Sync wave auto-assignment | Analyze dependency graph → assign `argocd.argoproj.io/sync-wave` annotations. Order: Namespace=0, CRDs=1, RBAC=2, ConfigMap/Secret=3, Services=4, Deployments/StatefulSets=5. Configurable overrides | P2 |
| 7.6.4 | ApplicationSet for multi-env | Generate `ApplicationSet` with Git/List generators. Per-environment values overlays (`values-dev.yaml`, `values-prod.yaml`). Matrix generator for multi-cluster × multi-env. Values: `gitops.applicationSet.{enabled,environments[]}` | P2 |

### 7.7 Disaster Recovery (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 7.7.1 | DR backup strategy | Aggressive backup frequency for low RPO (<5min RPO → backup every 5min). Multi-region storage (S3 cross-region replication). Pre/post hooks for DB dumps. Values: `disasterRecovery.{enabled,rto,rpo,backup.frequency,backup.retention,backup.location}` | P1 |
| 7.7.2 | Multi-region Service config | External-DNS annotations for global LB: `external-dns.alpha.kubernetes.io/hostname`, `aws-weight` (traffic distribution). Weight-based routing (active-passive: primary=100/standby=0, active-active: 50/50). Values: `dr.regions[].{name,primary,weight}` | P2 |
| 7.7.3 | Failover automation docs | Generate runbook for DR failover in NOTES.txt. Steps: DNS update (external-dns weight flip), storage promotion (RDS failover), app restart. RTO validation checklist. Automated vs manual decision tree | P1 |
| 7.7.4 | RTO/RPO compliance validation | Check backup frequency vs RPO target (warn if backup interval > RPO). Validate PDB allows fast recovery (minAvailable allows updates). Warn if RTO unrealistic (>15min with slow storage) | P2 |

### 7.8 Compliance Audit (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 7.8.1 | CIS benchmark policy generation | Generate Kyverno `ClusterPolicy` or OPA `ConstraintTemplate` for CIS Kubernetes checks. Policies: require-run-as-non-root (CIS 5.2.6), require-resource-limits (5.2.13), deny-privileged (5.2.1). Audit mode default, enforce optional. Values: `compliance.{enabled,standard=cis,enforcementMode=audit,policies.*}` | P1 |
| 7.8.2 | Compliance violation detection | Scan manifests pre-generation. Report violations: privileged containers, no resource limits, no probes, no NetworkPolicy. Severity scoring (Critical/High/Medium/Low). Auto-remediation hints. Output: compliance report with fix commands | P1 |
| 7.8.3 | Audit policy template | Generate cluster-level K8s Audit Policy YAML (reference only, cluster admin applies). Rules: Secret access (RequestResponse), RBAC changes (Request), pod exec/attach (Request). Note: cluster-wide config, not chart template. Values: `compliance.auditPolicy.{enabled,level=Metadata}` | P3 |
| 7.8.4 | Auto-remediation engine | Apply fixes automatically with `--fix-compliance` flag. Add securityContext (runAsNonRoot, readOnlyRootFilesystem), resource limits (calculated from workload type), default-deny NetworkPolicy. Dry-run mode: show fixes without applying | P2 |

### 7.9 Change Management (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 7.9.1 | Argo Rollouts canary generation | Convert Deployment → Rollout (kind: Rollout). Traffic steps (20%/40%/60%/80%/100%). AnalysisTemplate with Prometheus metrics (success-rate threshold: 0.95). Istio/nginx traffic routing. Values: `progressiveDelivery.{enabled,strategy=canary,canary.steps[],canary.analysis}` | P1 |
| 7.9.2 | Blue/Green deployment | Generate Rollout with blueGreen strategy. Active/preview Services. Manual/auto promotion. autoPromotionEnabled: false (manual gate). scaleDownDelaySeconds: 30 (connection drain). Values: `progressiveDelivery.bluegreen.{autoPromotionEnabled,previewReplicaCount,scaleDownDelaySeconds}` | P2 |
| 7.9.3 | Automated rollback | AnalysisTemplate failure → auto-rollback. Configurable thresholds (error rate >5%, latency p95 >500ms). Alerts on rollback (Slack/PagerDuty integration). successCondition/failureCondition expressions. Values: `progressiveDelivery.canary.analysis.metrics[].threshold` | P1 |
| 7.9.4 | Flagger Canary integration | Generate Flagger `Canary` + `MetricTemplate` CRDs (alternative to Argo Rollouts). Istio/Linkerd/nginx traffic routing. Progressive traffic shift (step weight: 10, max weight: 50). Prometheus metrics: request-success-rate, request-duration. Values: `progressiveDelivery.provider=flagger` | P2 |

### 7.10 Monitoring & Alerting (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 7.10.1 | SLO manifest generation (Sloth) | Generate Sloth `PrometheusServiceLevel` CRD. SLIs: availability (HTTP 5xx rate), latency (p95 < 200ms). Multi-window burn rate alerts (1h/6h/3d). Page alert (burn rate fast), ticket alert (burn rate slow). Values: `monitoring.slo.{enabled,availability.target=0.999,latency.{target,threshold}}` | P1 |
| 7.10.2 | ServiceMonitor auto-generation | Detect metrics port (METRICS_PORT env, annotation, non-standard port) → `ServiceMonitor`. Prometheus annotations fallback (`prometheus.io/scrape`). Custom relabelings: drop high-cardinality labels. Values: `monitoring.serviceMonitor.{enabled,interval,path,relabelings[]}` | P1 |
| 7.10.3 | Recording rules for SLI | Pre-aggregate p50/p95/p99 latency (histogram_quantile). Availability ratio: `sum(rate(http_requests_total{status!~"5.."})) / sum(rate(http_requests_total))`. Error ratio, QPS. Faster Grafana dashboards. Generate `PrometheusRule` with recording rules | P2 |
| 7.10.4 | Grafana dashboard generation | Auto-generate `GrafanaDashboard` CRD from available metrics. Panels: SLI (availability, latency percentiles, error rate), resources (CPU, memory, network I/O), custom metrics. JSON schema compliance. Datasource auto-detection | P3 |

---

## Phase 8: Developer Experience & Automation (v0.8.0)

**Цель**: Best-in-class developer experience, automation, tooling, platform engineering
**Задач**: 40 | **Оценка сложности**: High
**Automation rate**: 84% of manual developer workflow automated

### 8.1 IDE Integration (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 8.1.1 | Language Server Protocol (LSP) | DHG Language Server binary (`dhg-lsp`) для autocomplete, validation, hover docs. Protocol: JSON-RPC over stdio. Capabilities: completion (values.yaml keys from schema), hover (descriptions + examples), definition (values → template usage), diagnostics (real-time validation). Multi-IDE support: VS Code, JetBrains, Neovim, Emacs | P1 |
| 8.1.2 | VS Code extension | Marketplace extension with commands: `dhg.generateChart`, `dhg.validateChart`, `dhg.preview`. Features: syntax highlighting (Helm templates), snippets (common patterns), tree view (chart structure), integrated terminal. Auto-install dhg-lsp binary | P1 |
| 8.1.3 | JetBrains plugin | IntelliJ/GoLand plugin. Features: gutter icons (deploy, preview), run configurations (dhg generate), tool window (chart explorer). LSP client integration. Support: IDEA, GoLand, PyCharm | P2 |
| 8.1.4 | JSON Schema auto-generation | Generate `values.schema.json` from DHG config. Use for IDE validation (JSON Schema Store integration). Extract descriptions from YAML comments. Support `$ref`, `allOf`, `anyOf`, `oneOf`. Publish to schema store | P1 |

### 8.2 Pre-Commit Automation (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 8.2.1 | Pre-commit hook framework | `.pre-commit-config.yaml` template for DHG projects. Hooks: helmlint (custom values), helm-unittest, gitleaks (secrets), helm-docs (auto-update README). Command: `dhg init --git-hooks` auto-setup. Integration: gruntwork pre-commit, husky (npm) | P1 |
| 8.2.2 | `dhg validate` command | Comprehensive validation: helm lint, JSON schema validation (values.yaml), template rendering dry-run, kubeconform (K8s schema), conftest (policy checks). Exit code: 0 (pass), 1 (fail). CI/CD integration: GitHub Actions, GitLab CI templates | P1 |
| 8.2.3 | Secrets detection integration | Gitleaks/TruffleHog integration. Scan: values.yaml, templates/, .env files. Block commit if secrets detected. Auto-add `.gitleaksignore` for false positives. Support: AWS keys, GCP keys, Azure keys, generic patterns | P1 |
| 8.2.4 | Auto-documentation hook | helm-docs hook to auto-update README.md from values.yaml. Pre-commit trigger: values.yaml changed. Template: README.md.gotmpl with custom sections. Fail commit if docs out-of-sync (enforce documentation) | P2 |

### 8.3 Testing Framework (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 8.3.1 | `dhg test generate` | Auto-generate helm-unittest snapshot tests for all templates. Test scenarios: default values, enabled/disabled toggles (matrix), edge cases (replicas=0, no resources). Output: `charts/*/tests/*.yaml`. Coverage tracking: % of templates with tests | P1 |
| 8.3.2 | `dhg test run` | Execute helm-unittest with coverage report. Formats: JUnit XML (CI integration), HTML (local review), JSON. Coverage metrics: templates tested, assertions passed, lines covered. Fail threshold: <90% coverage | P1 |
| 8.3.3 | `dhg test update-snapshots` | Refresh snapshot baselines after intentional changes. Workflow: 1) make template changes, 2) tests fail (snapshot mismatch), 3) review diff, 4) run update-snapshots, 5) commit updated snapshots. Safety: require manual approval | P1 |
| 8.3.4 | Regression detection (CI) | GitHub Actions workflow: test on PR, compare snapshots with base branch, post diff as PR comment. Block merge if unintended changes. Auto-approve if snapshots explicitly updated (commit message: `[update snapshots]`) | P2 |

### 8.4 Documentation Automation (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 8.4.1 | `dhg docs generate` | Auto-create README.md from helm-docs template. Sections: description, installation, configuration (values table), examples. Extract: descriptions from YAML comments (`# --` prefix), defaults, types. Badges: version, license, maintained | P2 |
| 8.4.2 | `dhg docs validate` | Check docs in sync with values.yaml. Compare: documented keys vs actual keys, default values match. Exit 1 if outdated. Pre-commit hook integration. CI/CD validation (fail PR if docs stale) | P2 |
| 8.4.3 | README.md.gotmpl template | Best-practice template with sections: badges, description, prerequisites, installation (Helm repo, OCI), configuration (auto-generated table), examples (minimal, production, HA), upgrading, uninstalling. Company branding support (logo, footer) | P2 |
| 8.4.4 | JSON Schema for IDE | Generate `values.schema.json` for IDE autocomplete/validation. Publish to JSON Schema Store (schemastore.org). IDE integration: VS Code, JetBrains auto-detect schema. Validation on save | P1 |

### 8.5 Inner Loop Optimization (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 8.5.1 | `dhg dev` hot reload | Start local dev environment with file watcher. Detect changes: dhg-config.yaml, templates/, values.yaml. Auto-reload: regenerate chart → helm upgrade (K8s). File sync: code changes without container rebuild (Tilt-style). Port-forward automation | P2 |
| 8.5.2 | Tilt integration | `Tiltfile` template for DHG-generated charts. Features: live_update (sync ./src), docker_build with cache, k8s_yaml with helm(), port_forwards, log streaming. Auto-detect chart location. Hot reload on template changes | P2 |
| 8.5.3 | DevSpace integration | `devspace.yaml` template. Features: sync (bidirectional file sync), ports (8080:8080), logs (tail -f), terminal (exec into pod). Helm deployment config. Dev/prod profile switching. Pipeline: dev (local), deploy (prod) | P2 |
| 8.5.4 | Preview environments (PR) | Ephemeral namespaces per PR. GitHub Actions: detect PR → create namespace → deploy chart → post URL as comment. Cleanup on PR close. Integration: ArgoCD ApplicationSet (Git generator), Flux Kustomization. DNS: pr-123.example.com | P3 |

### 8.6 GitOps Integration (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 8.6.1 | `dhg gitops argocd` | Generate ArgoCD `Application` manifest. Fields: repoURL, targetRevision, path, helm.valueFiles, syncPolicy (automated.prune/selfHeal), ignoreDifferences (HPA replicas). Support: multi-cluster (destination.server), sync waves (annotations), health checks | P1 |
| 8.6.2 | `dhg gitops flux` | Generate Flux `HelmRelease` + `HelmRepository` CRDs. Fields: interval (5m), chart.spec.sourceRef, values, upgrade/install.remediation, rollback, test.enable. Post-renderers: kustomize patches. Notifications: Slack/Discord integration | P1 |
| 8.6.3 | Argo Rollouts integration | Convert Deployment → Rollout. Strategies: canary (traffic steps: 20/40/60/80%), blueGreen (autoPromotionEnabled: false). AnalysisTemplate with Prometheus metrics (success-rate >= 0.95). Istio/nginx traffic routing. Auto-rollback on failure | P1 |
| 8.6.4 | ApplicationSet (multi-env) | Generate `ApplicationSet` with generators: Git (directory per env), List (explicit envs), Matrix (cluster × env). Template: per-environment values overlays (values-dev.yaml, values-prod.yaml). Sync policy inheritance. Progressive rollouts (dev → staging → prod) | P2 |

### 8.7 Kubernetes Operator (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 8.7.1 | HelmChart CRD definition | Define `helmcharts.dhg.deckhouse.io` CRD. Spec: chartName, source (git.url, branch), values (x-kubernetes-preserve-unknown-fields), autoSync. Status: conditions (type: Ready/Progressing/Failed), observedGeneration, lastDeployedRevision. Printer columns: chart, status, age | P2 |
| 8.7.2 | DHG Operator (Kubebuilder) | Reconciliation loop: 1) Fetch HelmChart CR, 2) Clone Git repo, 3) Run `dhg generate`, 4) Deploy via Helm SDK, 5) Update status. Watch: HelmChart CRs, Secrets (values), ConfigMaps. Leader election, metrics (Prometheus), webhooks (validation) | P2 |
| 8.7.3 | Auto-reconciliation | Periodic sync (default: 5m interval). Detect drift: compare desired (Git) vs actual (cluster). Auto-remediate: re-apply chart. Git polling: watch for new commits (HEAD SHA changed). Webhook support (GitHub webhook → instant reconcile) | P2 |
| 8.7.4 | Multi-tenancy support | Namespace isolation: HelmChart in namespace A can only deploy to A. RBAC: ServiceAccount per tenant. Resource quotas enforcement. Audit logging: who deployed what. Admission webhook: prevent cross-namespace references | P3 |

### 8.8 Platform Engineering (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 8.8.1 | Backstage software template | Create `template.yaml` for Backstage catalog. Steps: fetch (skeleton), dhg:generate (custom action), publish (GitHub), createArgoApp. Parameters: chartName, tier (frontend/backend/database), language (go/python/nodejs). Output: repository URL, ArgoCD app URL | P2 |
| 8.8.2 | Port.io integration | Port blueprint: helmChart with properties (chartName, version, deploymentStatus). Action: generate_chart (webhook to DHG API). Self-service portal: developers click "Create Chart", fill form, auto-deployed. Catalog integration: sync chart metadata to Port | P2 |
| 8.8.3 | DHG API server | REST API for self-service. Endpoints: POST /generate (chart generation), GET /status (deployment status), POST /validate (pre-check). Input: JSON config. Output: chart path, ArgoCD app URL. Auth: OIDC (GitHub/GitLab). Multi-tenancy: per-team quotas | P2 |
| 8.8.4 | Golden path templates | Pre-configured templates per app tier: frontend (nginx ingress, HPA, 3 replicas), backend (service mesh, autoscaling), database (StatefulSet, backup, PVC). Templates: dhg-config.yaml per tier. CLI: `dhg init --tier=frontend` | P2 |

### 8.9 Advanced Debugging (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 8.9.1 | `dhg diff` | Compare rendered charts across environments. Usage: `dhg diff dev prod`. Output: unified diff (colorized), resource summary (added/changed/removed). Diff algorithms: line-by-line, structural (YAML-aware). Integration: CI (post diff as PR comment) | P1 |
| 8.9.2 | `dhg preview` | Dry-run with server-side validation. Steps: 1) helm template, 2) kubeconform (K8s schema), 3) conftest (policies), 4) diff vs live cluster. Output: resource impact (CPU/memory changes), policy violations, preview URL (ephemeral namespace). Interactive: approve/reject deployment | P1 |
| 8.9.3 | `dhg debug` | Troubleshoot failed deployments. Features: helm status, kubectl describe, logs (last 100 lines), events (namespace-scoped), common issues detection (ImagePullBackOff, CrashLoopBackOff). Output: diagnosis + remediation steps. Integration: kubectl-debug plugin | P2 |
| 8.9.4 | Visual diff UI | Web interface for chart comparison. Features: side-by-side view (dev vs prod), inline diff, YAML syntax highlighting, collapsible sections. Export: PDF report, HTML. GitHub integration: post link as PR comment. Auth: GitHub OAuth | P3 |

### 8.10 Performance Optimization (4 задачи)

| # | Задача | Описание | Priority |
|---|--------|----------|----------|
| 8.10.1 | Benchmark suite | `go test -bench=.` for performance regression detection. Benchmarks: chart generation (10/100/1000/5000 resources), template rendering, YAML parsing. Metrics: time (ns/op), memory (B/op), allocations (allocs/op). Baseline: store in repo, CI comparison | P3 |
| 8.10.2 | Content-based caching | Cache charts by config hash (SHA256 of dhg-config.yaml). Cache directory: `~/.cache/dhg/`. Helm 4 integration: content-addressable storage. CI/CD caching: GitHub Actions cache key (hashFiles), GitLab artifacts. Cache hit rate tracking | P3 |
| 8.10.3 | Parallel generation | Use goroutines for parallel processing: file reading (concurrent I/O), per-kind processing (isolated goroutines), template rendering (worker pool). Speedup: 3-5x on multi-core. Memory optimization: sync.Pool for buffers, streaming YAML parsing | P3 |
| 8.10.4 | Performance monitoring | Prometheus metrics: `dhg_chart_generation_duration_seconds` (histogram), `dhg_cache_hits_total` (counter). Grafana dashboard: generation time trend, cache hit rate, resource usage. Profiling: pprof integration (`dhg generate --profile cpu.prof`) | P3 |

---

## Phase 9: AI/ML Workloads (v0.9.0)

**Цель**: AI/ML-ready charts с GPU management, distributed training, model serving, MLOps automation
**Задач**: 20 | **Оценка сложности**: High
**Automation rate**: 73% of AI/ML configurations auto-detected and auto-generated

**Research sources**: KServe docs, Kubeflow, NVIDIA GPU Operator, Ray, Seldon Core, MLflow, TensorRT

### 9.1 Kubeflow Components (4 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 9.1.1 | Kubeflow Pipelines Workflow CRD | Auto-generate ScheduledWorkflow/Workflow CRDs с поддержкой параметров, артефактов, кэширования. Integration: Argo Workflow engine, separate CRD chart (crds/). Example: cron schedule for training pipeline (daily/weekly). Output: `templates/workflow.yaml` + `templates/scheduledworkflow.yaml` | Detect `kind: Workflow` + `apiVersion: argoproj.io/v1alpha1` в templates/ | P2 |
| 9.1.2 | Katib Experiment Auto-Configuration | Auto-generate Experiment CRD для hyperparameter tuning с parallelTrialCount, maxTrialCount, objective metrics (accuracy/loss/F1). Algorithms: random, grid, bayesian. Validation: `parallelTrialCount` ≤ `maxTrialCount`. Integration: Katib Helm chart dependencies. Output: `templates/experiment.yaml` + values placeholders | Detect `kind: Experiment` + `spec.algorithm` (random/grid/bayesian) в templates/ | P2 |
| 9.1.3 | Training Operator Jobs | Auto-generate TFJob/PyTorchJob/MPIJob с распределенной конфигурацией (worker/ps replicas, GPU requests). Features: auto-inject GPU tolerations, node selectors для GPU nodes, securityContext (runAsNonRoot). Framework detection: PyTorch (`replicaType: Master/Worker`), TensorFlow (`ps/worker/chief`). Output: framework-specific templates | Detect `kind: PyTorchJob/TFJob` + `spec.pytorchReplicaSpecs/tfReplicaSpecs` в templates/ | P1 |
| 9.1.4 | Notebook Controller Integration | Auto-generate Notebook CRD с PVC для persistent workspace (10Gi default), GPU allocation (optional), network isolation (optional NetworkPolicy). Image support: jupyter/tensorflow-notebook, jupyter/pytorch-notebook. Security: drop capabilities, read-only root filesystem. Output: `templates/notebook.yaml` + PVC + optional NetworkPolicy | Detect `kind: Notebook` + `spec.template.spec.containers[].image` (jupyter/*) в templates/ | P3 |

**Technical Notes:**
- Kubeflow Pipelines requires Argo Workflow CRDs — DHG should detect CRD dependency and suggest separate installation
- Training Operator supports 4 job types (TensorFlow, PyTorch, MPI, XGBoost) — auto-detect by CRD kind
- GPU tolerations: `nvidia.com/gpu:NoSchedule` should be auto-injected for all GPU workloads

### 9.2 Model Serving (4 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 9.2.1 | KServe InferenceService GPU Configuration | Auto-inject GPU resources (`nvidia.com/gpu: 1`), GPU tolerations (`nvidia.com/gpu:NoSchedule`), node selectors для GPU nodes (`nvidia.com/gpu.present: true`). Support 4 GPU types: nvidia.com/gpu, intel.com/gpu, amd.com/gpu, habana.ai/gaudi. Auto-add shared memory volumes (`/dev/shm`) для distributed inference. Output: predictor spec with GPU config | Detect `kind: InferenceService` + `spec.predictor.model` в templates/ | P1 |
| 9.2.2 | KServe Multi-GPU Serving | Auto-generate workerSpec для multi-node/multi-GPU inference (tensorParallelSize × pipelineParallelSize). Features: shared memory volumes (emptyDir + sizeLimit), inter-node communication (hostNetwork: optional), GPU topology awareness. Validation: total GPUs = tensorParallel × pipelineParallel. Output: `spec.predictor.workerSpec` with multi-GPU config | Detect `spec.workerSpec` + `resources.limits["nvidia.com/gpu"]` > 1 в templates/ | P2 |
| 9.2.3 | Seldon Core Traffic Splitting | Auto-configure SeldonDeployment с canary/A/B traffic weights (baseline: 80%, candidate: 20%). Patterns: canary (80/20 → 50/50 → 0/100), shadow (100/100), A/B (50/50). Integration: Istio VirtualService для advanced routing. Output: `templates/seldondeployment.yaml` с multiple predictors + traffic config | Detect multiple `spec.predictors[]` + `traffic: X` values в SeldonDeployment | P1 |
| 9.2.4 | Model Registry Integration | Auto-inject MLflow model registry annotations (`mlflow.experiment.id`, `mlflow.model.uri`), secret mounts для tracking URI. Support: MLflow Model Registry, model versioning (stage: staging/production). Environment variables: `MLFLOW_TRACKING_URI`, `MLFLOW_EXPERIMENT_NAME`. Output: annotations + secretRef injection | Detect `mlflow.tracking.uri` annotation OR mlflow-related secrets в resources | P3 |

**Technical Notes:**
- KServe supports RawDeployment mode (without Knative) — DHG should detect `spec.predictor.model` vs `spec.predictor.containers`
- Traffic splitting requires Istio — validate dependency or warn if missing
- MLflow integration uses HTTP API — ensure network connectivity between InferenceService and MLflow server

### 9.3 GPU Management (4 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 9.3.1 | NVIDIA GPU Operator Time-Slicing | Auto-generate ConfigMap для time-slicing (replicas: 4 default), update DevicePlugin config reference. Pattern: `devicePlugin.config.name: time-slicing-config` → generate ConfigMap with `sharing.timeSlicing.replicas: 4`. Installation: `helm install gpu-operator nvidia/gpu-operator --set devicePlugin.config.name=time-slicing-config`. Output: `templates/time-slicing-configmap.yaml` | Detect `devicePlugin.config.name: time-slicing-config` в values.yaml + GPU workloads present | P2 |
| 9.3.2 | MIG Partitioning Configuration | Auto-configure MIG strategy (single/mixed mode), profile specs (1g.5gb, 2g.10gb, 3g.20gb, 7g.40gb) в ConfigMap. Validation: MIG profile must match GPU memory (A100 40GB → 7g.40gb max). Pattern: `spec.mig.strategy: mixed` + `devicePlugin.config` → generate MIG ConfigMap. Output: `templates/mig-configmap.yaml` + device plugin config | Detect `spec.mig.strategy: mixed` + `devicePlugin.config` в values.yaml | P2 |
| 9.3.3 | GPU Resource Requests Auto-Injection | Auto-add `nvidia.com/gpu: 1` limits/requests для workloads без explicit GPU config. Image pattern detection: `tensorflow-gpu:*`, `pytorch/pytorch:*-cuda*`, `nvcr.io/*`, `nvidia/*`. Warnings: if image contains "gpu" but no GPU resources → suggest auto-injection. Output: `resources.limits["nvidia.com/gpu"]` added to container spec | Detect container images (tensorflow-gpu, pytorch:*-cuda*, nvcr.io/*) без GPU resources в spec | P1 |
| 9.3.4 | GPU Node Affinity | Auto-inject node selectors/affinity для GPU workloads: `nvidia.com/gpu.present: true` (node selector), `nvidia.com/gpu.family: ampere` (optional affinity). Toleration: `nvidia.com/gpu:NoSchedule`. Support GPU families: volta, turing, ampere, hopper. Output: nodeSelector + affinity + tolerations in podSpec | Detect `resources.limits["nvidia.com/gpu"]` > 0 в container spec | P1 |

**Technical Notes:**
- Time-slicing allows 4-8 workloads per GPU (replicas: 4-8) — DHG should validate replicas ≤ 8
- MIG requires A100/H100 GPUs — validate GPU model compatibility
- GPU detection patterns: image name OR explicit GPU resources → trigger GPU configs

### 9.4 Distributed Training (3 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 9.4.1 | Ray Cluster Auto-Configuration | Auto-generate RayCluster CRD с head/worker specs, autoscaling (minReplicas: 1, maxReplicas: 10). Features: RBAC (ClusterRole for cluster-wide), resource requests (head: 2 CPU, worker: 4 CPU), tolerations for GPU nodes (optional). Output: `templates/raycluster.yaml` + RBAC resources. Helm integration: separate CRD chart (`kuberay-operator --skip-crds`) | Detect `kind: RayCluster` + `spec.headGroupSpec` в templates/ | P2 |
| 9.4.2 | RayJob Cleanup Policy | Auto-configure `shutdownAfterJobFinishes: true` (ephemeral jobs), `ttlSecondsAfterFinished: 3600` (1 hour retention). Pattern: detect `kind: RayJob` + job submission → suggest cleanup config. Validation: long-running jobs (RayService) should NOT have shutdown policy. Output: `spec.shutdownAfterJobFinishes` + `spec.ttlSecondsAfterFinished` in RayJob | Detect `kind: RayJob` + job submission pattern (не RayService) в templates/ | P2 |
| 9.4.3 | MPI Operator Integration | Auto-generate MPIJob с launcher/worker topology, hostNetwork для inter-node communication, slotsPerWorker (GPUs per worker). Features: Horovod integration (NCCL backend), SSH key management (auto-generate secret). Validation: `slotsPerWorker` должно match GPU allocation. Output: `templates/mpijob.yaml` + SSH secret + hostNetwork config | Detect `kind: MPIJob` + `spec.slotsPerWorker` в templates/ | P3 |

**Technical Notes:**
- Ray requires RBAC for cluster-scoped operations — DHG should generate ClusterRole/ClusterRoleBinding
- RayJob cleanup prevents resource leaks — default TTL should be configurable via values
- MPI Operator requires SSH between pods — auto-generate SSH keypair and secret

### 9.5 MLOps Patterns (3 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 9.5.1 | Experiment Tracking Annotations | Auto-inject annotations для linkage с experiment tracking: `mlflow.experiment.id`, `katib.trial.id`, `wandb.run.id`. Pattern: detect MLflow/Katib/W&B usage + Deployment/Job → add annotations. Environment variables: `MLFLOW_TRACKING_URI`, `KATIB_TRIAL_ID`. Output: metadata.annotations + env vars injection | Detect MLflow/Katib usage (secrets, configmaps) + Deployment/Job resources в chart | P3 |
| 9.5.2 | Model Versioning Labels | Auto-add labels для GitOps tracking: `model.version: v1.2.3`, `model.framework: tensorflow`, `model.stage: production`. Detection: InferenceService → extract model version from spec.predictor.model URI. Integration: ArgoCD Application labels sync. Output: consistent labels across InferenceService, Deployment, Service | Detect InferenceService/SeldonDeployment в templates/ | P2 |
| 9.5.3 | Canary Deployment Automation | Auto-generate Argo Rollouts AnalysisTemplate для progressive delivery (10% → 50% → 100%). Metrics: Prometheus queries (request latency p95, error rate). Validation: rollback if p95 > 500ms OR error rate > 1%. Integration: Argo Rollouts + Seldon Deploy. Output: `templates/rollout.yaml` + `templates/analysistemplate.yaml` | Detect `strategy.canary` + Prometheus metrics annotations в Deployment/InferenceService | P2 |

**Technical Notes:**
- Experiment tracking requires network access to tracking server — validate connectivity or use init container
- Model versioning labels should follow semantic versioning (semver) — DHG should parse and validate
- Canary deployment requires observability stack (Prometheus) — check dependency and warn if missing

### 9.6 Data Pipeline (3 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 9.6.1 | MinIO S3 Storage Integration | Auto-generate MinIO Tenant (StatefulSet, PV/PVC, Services), tiering configuration (NVMe → HDD → Cloud). Features: 4-node cluster (min), TLS (optional), console (optional). Spec: `spec.pools[].servers: 4`, `volumesPerServer: 4`, storage class selection (fast-ssd/standard). Output: `templates/tenant.yaml` + PVC templates + Services | Detect `kind: Tenant` + `spec.pools[].volumesPerServer` в templates/ OR minio-related secrets | P2 |
| 9.6.2 | Dataset PVC Auto-Provisioning | Auto-create PVC для datasets с appropriate storage class: fast-ssd (NVMe) для training, standard (HDD) для inference. Detection: dataset annotations (`dataset.size: 100Gi`, `dataset.access-pattern: sequential`). Features: accessModes (ReadWriteMany для distributed training), volumeMode (Filesystem/Block). Output: `templates/dataset-pvc.yaml` + storage class mapping | Detect dataset annotations (`dataset.size`, `dataset.access-pattern`) в Job/StatefulSet resources | P1 |
| 9.6.3 | Data Preprocessing Jobs | Auto-generate Kubernetes Jobs для ETL с resource limits (CPU/memory), restart policy (OnFailure), backoff limits (3 retries). Image detection: spark, dask, airflow. Features: parallel processing (completions: 5), TTL cleanup (`ttlSecondsAfterFinished`). Output: `templates/preprocessing-job.yaml` + ConfigMap for scripts | Detect Job pattern + data transformation images (spark, dask, airflow) в templates/ | P3 |

**Technical Notes:**
- MinIO requires 4+ nodes for erasure coding — DHG should validate `spec.pools[].servers` ≥ 4
- Dataset PVC should use ReadWriteMany for distributed training (PyTorchJob, TFJob) — auto-detect training CRDs
- Data preprocessing jobs should have TTL cleanup — prevent resource accumulation

### 9.7 Optimization & Inference (3 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 9.7.1 | TensorRT Model Optimization | Auto-inject init containers для ONNX → TensorRT conversion с FP16/INT8 quantization. Pattern: detect `.onnx` model artifacts + GPU serving → add conversion step. Image: `nvcr.io/nvidia/tensorrt:latest`. Command: `trtexec --onnx=model.onnx --saveEngine=model.trt --fp16`. Output: init container spec + shared volume for converted model | Detect `.onnx` model artifacts + GPU serving (InferenceService с GPU) в spec | P2 |
| 9.7.2 | Triton Inference Server Config | Auto-generate model repository structure, config.pbtxt для multi-model serving. Features: dynamic batching (max_batch_size: 8), model versioning (version_policy: latest), instance groups (GPU count). Directory structure: `models/model_name/1/model.onnx` + `config.pbtxt`. Output: ConfigMap with model configs + init container to populate model repo | Detect `nvcr.io/nvidia/tritonserver` image в container spec | P2 |
| 9.7.3 | HPA for Model Serving | Auto-configure HPA с Prometheus metrics (`inference_requests_per_second`, `inference_latency_ms`). Scaling: minReplicas: 2, maxReplicas: 10, targetValue: 100 req/s. Integration: ServiceMonitor for Prometheus scraping, custom metrics API (prometheus-adapter). Output: `templates/hpa.yaml` + ServiceMonitor + metrics annotations | Detect InferenceService + `autoscaling.enabled: true` в values.yaml | P1 |
| 9.7.4 | Batch Inference Jobs | Auto-generate CronJob для batch predictions с dataset → model → results pipeline. Features: input PVC (dataset), output PVC (results), model loading (init container), parallel processing (completions: 5). Schedule: configurable (daily/weekly). Output: `templates/batch-inference-cronjob.yaml` + PVC claims + ConfigMap for scripts | Detect batch inference pattern (large input dataset, scheduled execution) в Job/CronJob templates | P3 |

**Technical Notes:**
- TensorRT conversion requires GPU — init container should request GPU resources
- Triton config.pbtxt must match model format (ONNX, TensorFlow, PyTorch) — DHG should detect format from file extension
- HPA requires custom metrics API — validate prometheus-adapter installation or warn

---

## Phase 10: Database Operators (v0.10.0)

**Цель**: Production-ready database operator integration с auto-detection, HA, backup, connection pooling
**Задач**: 51 | **Оценка сложности**: High
**Automation rate**: 81% of database configurations auto-detected and auto-generated
**Operator coverage**: 15+ operators (PostgreSQL, MySQL, MariaDB, Redis, MongoDB, Cassandra, ClickHouse, OpenSearch)

**Research sources**: CloudNativePG, Zalando Postgres Operator, Crunchy Data, Percona, MariaDB, Oracle MySQL, Redis Enterprise, Spotahome, MongoDB Community, K8ssandra, Altinity ClickHouse, OpenSearch

**Detection strategy**: DHG uses 7 detection methods — API version, labels/annotations, values schema, CRD kind, chart dependencies, ConfigMap patterns, secret references

### 10.1 PostgreSQL Operators (10 задач)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 10.1.1 | Auto-detect CloudNativePG Cluster CRD | DHG generates CloudNativePG cluster manifest with streaming replication, instances: 3 (default), connection pooler (optional). Output: `templates/cluster.yaml` + backup config | Detect `.Values.database.type == "postgresql"` AND `.Values.database.operator == "cloudnativepg"` OR `apiVersion: postgresql.cnpg.io/v1` в templates/ | P1 |
| 10.1.2 | Generate backup configuration for CloudNativePG | Auto-configure barman-cloud backup with S3/MinIO integration. Features: WAL archiving, PITR, retention policies (30d default). Output: `spec.backup.barmanObjectStore` section + S3 credentials secretRef | If `.Values.database.backup.enabled == true` AND operator is CloudNativePG, generate ScheduledBackup CRD | P1 |
| 10.1.3 | Configure synchronous replication | Generate cluster with sync replicas for zero RPO (no data loss). Settings: `minSyncReplicas: 1`, `maxSyncReplicas: 2`. Validation: instances ≥ minSyncReplicas + 1. Output: `spec.minSyncReplicas` + PostgreSQL `synchronous_standby_names` | If `.Values.database.replication.synchronous == true`, add sync standby config to cluster spec | P2 |
| 10.1.4 | Setup PITR recovery capability | Configure point-in-time recovery with WAL archiving. Features: recovery target (time/XID/name), backup source selection. Output: `spec.bootstrap.recovery` section + recovery ConfigMap | If backup enabled, auto-configure recovery section for disaster recovery procedures | P2 |
| 10.1.5 | Auto-detect Zalando postgresql CRD | Generate Zalando postgresql manifest with HA setup (Patroni-based). Features: connection pooler (PgBouncer), logical backups (WAL-G). Output: `templates/postgresql.yaml` + pooler config | Detect `.Values.database.operator == "zalando"` OR `apiVersion: acid.zalan.do/v1` в templates/ | P2 |
| 10.1.6 | Auto-configure PgBouncer pooler (Zalando) | Generate connection pooler config for Zalando clusters. Settings: mode (transaction/session), instances: 2, auth_query setup. Output: `spec.connectionPooler` section + pooler secret | If `.Values.database.connectionPooler.enabled == true` AND Zalando operator, add pooler to postgresql CRD | P2 |
| 10.1.7 | Setup logical backup with WAL-G (Zalando) | Configure S3-based logical backups via WAL-G. Features: schedule (daily), compression (gzip), parallel workers (2). Output: backup env vars + S3 credentials | If backup enabled AND Zalando operator, generate WAL-G configuration with S3 endpoint | P2 |
| 10.1.8 | Detect Crunchy PostgresCluster CRD | Generate Crunchy Data PostgreSQL cluster (PostgresCluster kind). Features: pgBackRest backup, monitoring (pgMonitor), HA (Patroni). Output: `templates/postgrescluster.yaml` + repo config | Detect `.Values.database.operator == "crunchy"` OR `apiVersion: postgres-operator.crunchydata.com/v1beta1` в templates/ | P2 |
| 10.1.9 | Auto-configure pgBackRest (Crunchy) | Setup pgBackRest with repo configuration (S3/MinIO). Features: full/differential/incremental backups, retention (30 days). Output: `spec.backups.pgbackrest` section + repo credentials | If backup enabled AND Crunchy operator, generate pgBackRest repos + schedules | P2 |
| 10.1.10 | Generate PGAdmin deployment (Crunchy) | Create PGAdmin CRD for namespace-scoped admin UI. Features: auto-discovery of PostgreSQL clusters, RBAC (namespace-scoped). Output: `templates/pgadmin.yaml` + Service/Ingress | If `.Values.database.adminUI.enabled == true` AND Crunchy operator, generate PGAdmin CRD | P3 |

**Technical Notes:**
- **CloudNativePG**: Native streaming replication, barman-cloud for backups, operator manages all lifecycle
- **Zalando**: Patroni-based HA, PgBouncer pooler integrated, WAL-G for S3 backups
- **Crunchy**: pgBackRest (preferred over barman), pgMonitor for observability, PGAdmin v5.5+ namespace-scoped
- **Detection priority**: Explicit operator field → CRD API version → Helm dependencies → default (CloudNativePG)

### 10.2 MySQL/MariaDB Operators (8 задач)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 10.2.1 | Detect PerconaXtraDBCluster CRD | Generate Percona XtraDB cluster with multi-master (Galera). Features: 3-node default, ProxySQL load balancer, automated backups (Percona XtraBackup). Output: `templates/perconaxtradbcluster.yaml` + ProxySQL config | Detect `.Values.database.type == "mysql"` AND `.Values.database.operator == "percona-xtradb"` OR `apiVersion: pxc.percona.com/v1` в templates/ | P2 |
| 10.2.2 | Auto-configure ProxySQL load balancer | Generate ProxySQL deployment for connection management. Features: query routing, connection pooling, automatic backend discovery. Output: `spec.proxysql` section + admin credentials | If `.Values.database.loadBalancer.enabled == true` AND Percona XtraDB operator, add ProxySQL config | P2 |
| 10.2.3 | Setup automated backups (Percona XtraBackup) | Configure backup schedules and S3 storage. Features: full backups (daily), incremental (hourly), compression (zstd). Output: `spec.backup` section + S3 credentials + CronJob schedules | If backup enabled AND Percona XtraDB, generate backup configuration with retention policies | P2 |
| 10.2.4 | Detect MariaDB CRD | Generate MariaDB cluster manifest (standalone or Galera). Features: MaxScale load balancer (optional), Galera multi-master. Output: `templates/mariadb.yaml` + replication config | Detect `.Values.database.type == "mariadb"` OR `apiVersion: k8s.mariadb.com/v1alpha1` в templates/ | P2 |
| 10.2.5 | Auto-configure MaxScale load balancer | Generate MaxScale deployment with connection pooling. Features: read/write split, query routing, 2+ instances for HA. Output: MaxScale configuration + Service | If `.Values.database.loadBalancer.enabled == true` AND MariaDB operator, create MaxScale config | P2 |
| 10.2.6 | Configure Galera cluster replication | Setup multi-master synchronous replication. Features: 3-node quorum, wsrep settings, SST method (xtrabackup-v2). Output: Galera configuration in MariaDB spec | If `.Values.database.replication.type == "galera"` AND MariaDB operator, add Galera settings | P3 |
| 10.2.7 | Detect InnoDBCluster CRD (Oracle MySQL) | Generate MySQL InnoDB Cluster with Group Replication. Features: 3-node default, MySQL Router, automatic failover. Output: `templates/innodbcluster.yaml` + router config | Detect `.Values.database.operator == "oracle-mysql"` OR `apiVersion: mysql.oracle.com/v2` в templates/ | P2 |
| 10.2.8 | Auto-configure MySQL Router | Generate router pods for connection load balancing. Features: read/write split, bootstrap from cluster, replica count (2 default). Output: router instances configured via cluster spec | Router instances auto-generated from InnoDBCluster spec, configure count via `.Values.database.router.instances` | P3 |

**Technical Notes:**
- **Percona XtraDB**: Galera synchronous replication (multi-master), ProxySQL for routing, Percona XtraBackup for consistent backups
- **MariaDB**: Supports standalone (master-replica) AND Galera (multi-master), MaxScale for advanced routing
- **Oracle MySQL**: Group Replication (single-primary or multi-primary), MySQL Router for connection management
- **Load balancer detection**: If `.Values.database.loadBalancer.enabled == true`, auto-configure ProxySQL/MaxScale/Router

### 10.3 Redis Operators (6 задач)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 10.3.1 | Detect RedisEnterpriseCluster CRD | Generate Redis Enterprise cluster manifest. Features: active-active geo-distribution, persistence, flash storage (optional). Output: `templates/redisenterprisecluster.yaml` + node resources | Detect `.Values.database.type == "redis"` AND `.Values.database.operator == "redis-enterprise"` OR `apiVersion: app.redislabs.com/v1` в templates/ | P3 |
| 10.3.2 | Auto-configure RedisEnterpriseDatabase | Create application database within cluster. Features: replication (1-3 replicas), persistence (snapshot/AOF), memory limits. Output: `templates/redisenterprisedatabase.yaml` + access secret | Generate RedisEnterpriseDatabase CRD with replication and persistence settings based on values | P3 |
| 10.3.3 | Detect RedisFailover CRD (Spotahome) | Generate Redis with Sentinel failover. Features: master-replica topology, Sentinel monitoring (3+ instances), automatic failover. Output: `templates/redisfailover.yaml` + Sentinel config | Detect `.Values.database.operator == "spotahome"` OR `apiVersion: databases.spotahome.com/v1` в templates/ | P2 |
| 10.3.4 | Configure Sentinel high availability | Auto-configure Sentinel instances for failover monitoring. Settings: quorum (2 default), down-after-milliseconds (30000), failover-timeout (180000). Output: `spec.sentinel` section + quorum config | Generate Sentinel section with 3+ instances for quorum-based failover | P2 |
| 10.3.5 | Detect Bitnami Redis Helm pattern | Generate Redis StatefulSet with Sentinel (via Bitnami chart). Features: master-replica topology, Sentinel monitoring, password auth. Output: Bitnami chart values + sentinel config | Detect `.Values.database.operator == "bitnami"` AND Helm chart reference `bitnami/redis` в dependencies | P3 |
| 10.3.6 | Auto-configure master-replica topology | Generate master + N replicas with Sentinel monitoring. Settings: replicas: 2 (default), persistence enabled, auth password. Output: master/replica StatefulSets + Sentinel | If `.Values.redis.sentinel.enabled == true`, configure Sentinel-based HA topology | P3 |

**Technical Notes:**
- **Redis Enterprise**: Commercial operator, active-active geo-distribution, Redis-on-Flash support
- **Spotahome**: Open-source, Sentinel-based failover, operator manages StatefulSets + ConfigMaps
- **Bitnami**: Helm chart (not operator-based), Sentinel optional, widely used for simplicity
- **Sentinel requirements**: Minimum 3 instances for quorum (2/3 majority), odd numbers preferred (3, 5, 7)

### 10.4 MongoDB Operators (5 задач)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 10.4.1 | Detect MongoDBCommunity CRD | Generate MongoDB replica set cluster (3-node default). Features: automatic primary election, vector search support (2026), TLS encryption. Output: `templates/mongodbcommunity.yaml` + replica set config | Detect `.Values.database.type == "mongodb"` AND `.Values.database.operator == "mongodb-community"` OR `apiVersion: mongodbcommunity.mongodb.com/v1` в templates/ | P2 |
| 10.4.2 | Configure replica set topology | Auto-generate 3-node replica set for HA. Features: arbiter support (optional), priority settings, hidden members. Output: `spec.members` section + replica set name | Configure `.spec.members: 3` with optional arbiter if `.Values.database.arbiter.enabled == true` | P2 |
| 10.4.3 | Setup automated backups (MongoDB) | Configure backup agents and storage (S3/MinIO). Features: full backups (daily), oplog backups (continuous), PITR. Output: backup sidecar + S3 credentials | If backup enabled, add backup configuration to MongoDBCommunity cluster spec | P3 |
| 10.4.4 | Detect PerconaServerMongoDB CRD | Generate Percona MongoDB cluster with sharding. Features: replica sets + config servers + mongos routers, automated backups (Percona Backup for MongoDB). Output: `templates/perconaservermongodb.yaml` + sharding topology | Detect `.Values.database.operator == "percona-mongodb"` OR `apiVersion: psmdb.percona.com/v1` в templates/ | P2 |
| 10.4.5 | Auto-configure sharding topology | Generate sharded cluster topology (mongos/config/shards). Default: 3 mongos, 3 config servers, 2 shards (3 replicas each). Output: `spec.sharding` section + shard configs | If `.Values.database.sharding.enabled == true`, configure mongos, configsvrReplSet, replsets sections | P2 |

**Technical Notes:**
- **MongoDB Community**: Unified operator (2026 merger), supports replica sets, vector search (AI/ML integration)
- **Percona MongoDB**: Supports both replica sets and sharded clusters, Percona Backup for MongoDB (PBM) integrated
- **Sharding topology**: mongos (query routers), config servers (metadata), shards (data partitions)
- **Arbiter usage**: Arbiters don't hold data, only vote in elections (useful for 2-node + arbiter setup)

### 10.5 NoSQL/Other Operators (8 задач)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 10.5.1 | Detect CassandraDatacenter CRD (K8ssandra) | Generate K8ssandra Cassandra cluster. Features: Reaper (repairs), Medusa (backups), Stargate (APIs), multi-datacenter support. Output: `templates/cassandradatacenter.yaml` + K8ssandra stack | Detect `.Values.database.type == "cassandra"` OR `apiVersion: cassandra.datastax.com/v1beta1` в templates/ | P3 |
| 10.5.2 | Auto-configure Medusa backup (Cassandra) | Setup S3-based backup with Medusa. Features: snapshot backups, differential backups, S3 storage. Output: `spec.medusa` section + S3 credentials + backup schedules | If backup enabled AND Cassandra operator, generate MedusaBackupJob CRD with S3 config | P3 |
| 10.5.3 | Configure Reaper for repairs (Cassandra) | Generate Reaper CRD for automated repairs. Features: incremental repairs, cluster-wide coordination, JMX access. Output: `templates/reaper.yaml` + JMX config | If `.Values.database.repair.enabled == true`, create Reaper CRD for repair automation | P3 |
| 10.5.4 | Detect ClickHouseInstallation CRD | Generate ClickHouse cluster with sharding. Features: distributed tables, ZooKeeper integration, replicas. Output: `templates/clickhouseinstallation.yaml` + shard/replica topology | Detect `.Values.database.type == "clickhouse"` OR `apiVersion: clickhouse.altinity.com/v1` в templates/ | P3 |
| 10.5.5 | Auto-configure distributed tables | Setup sharding and replication for ClickHouse. Features: shard count (2 default), replica count (2 per shard), ZooKeeper for coordination. Output: `spec.configuration.clusters` section + shard config | If `.Values.database.sharding.enabled == true`, configure shard/replica topology in cluster spec | P3 |
| 10.5.6 | Detect OpenSearchCluster CRD | Generate OpenSearch cluster with data/master nodes. Features: dedicated master nodes (3 default), data nodes (hot/warm/cold), snapshot repositories. Output: `templates/opensearchcluster.yaml` + node groups | Detect `.Values.database.type == "opensearch"` OR `apiVersion: opensearch.opster.io/v1` в templates/ | P3 |
| 10.5.7 | Auto-configure ISM policies (OpenSearch) | Generate Index State Management policies. Features: hot → warm → cold transitions, retention periods, rollover conditions. Output: `templates/opensearchismpolicy.yaml` + lifecycle config | If `.Values.database.ism.enabled == true`, create OpenSearchISMPolicy CRD with retention rules | P3 |
| 10.5.8 | Setup snapshot lifecycle policies (OpenSearch) | Configure SLM for automated backups. Features: snapshot schedules (daily), S3 repository, retention (30 snapshots). Output: `templates/opensearchslmpolicy.yaml` + S3 config | If backup enabled AND OpenSearch operator, generate OpenSearchSLMPolicy CRD with S3 backend | P3 |

**Technical Notes:**
- **Cassandra (K8ssandra)**: Bundles cass-operator + Reaper (repairs) + Medusa (S3 backups) + Stargate (CQL/REST/GraphQL APIs)
- **ClickHouse**: Two operators (Official 2026, Altinity mature), supports sharding + replication via ZooKeeper
- **OpenSearch**: Fully open-source fork of Elasticsearch (Apache 2.0), ISM for lifecycle, SLM for snapshots
- **Detection**: NoSQL operators have unique CRD kinds and API groups, easy to distinguish

### 10.6 Connection Management & Pooling (4 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 10.6.1 | Auto-inject PgBouncer for PostgreSQL | Generate PgBouncer deployment when connection pooling requested. Features: transaction pooling, auth_query setup, admin user. Settings: pool_mode (transaction/session), default_pool_size (25). Output: `templates/pgbouncer.yaml` + ConfigMap + secret | If `.Values.database.connectionPooler.type == "pgbouncer"`, create PgBouncer StatefulSet with auto-generated config | P2 |
| 10.6.2 | Auto-inject ProxySQL for MySQL | Generate ProxySQL deployment for MySQL load balancing. Features: query routing, caching, automatic backend discovery. Settings: 2 instances (HA), admin interface. Output: `templates/proxysql.yaml` + admin credentials | If `.Values.database.connectionPooler.type == "proxysql"`, create ProxySQL with backend server discovery | P2 |
| 10.6.3 | Configure Redis Sentinel failover | Auto-configure Sentinel for Redis HA monitoring. Features: quorum-based failover, down detection (30s), automatic promotion. Settings: 3+ Sentinel instances, quorum: 2. Output: Sentinel StatefulSet + quorum config | If Redis operator supports Sentinel, generate Sentinel StatefulSet with quorum settings | P2 |
| 10.6.4 | Auto-detect pooler credentials | Generate secrets for connection pooler auth. Features: randomized passwords, reference main DB credentials, admin user creation. Output: `templates/pooler-secret.yaml` + user grants | Auto-create Secret for pooler user with randomized password referencing main DB credentials | P2 |

**Technical Notes:**
- **PgBouncer**: Transaction pooling recommended (lower overhead), requires auth_query for user validation
- **ProxySQL**: Query routing + read/write split, automatic backend health checks
- **Sentinel**: Quorum = (N/2) + 1 (for 3 Sentinels, quorum = 2), odd numbers prevent split-brain
- **Credential management**: Pooler should have limited privileges (CONNECT only), not SUPERUSER

### 10.7 Backup & Restore Patterns (5 задач)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 10.7.1 | Auto-configure S3/MinIO storage backend | Generate backup configuration with object storage. Features: S3-compatible endpoints (MinIO, AWS S3, GCS), TLS support, access/secret keys from secret. Output: S3 configuration section + credentialsSecret reference | If `.Values.database.backup.storage.type == "s3"`, create S3-compatible backup config with endpoint/credentials | P1 |
| 10.7.2 | Generate CronJob-based backup schedules | Create Kubernetes CronJob for periodic backups. Features: daily backups (2 AM default), backup tool auto-selection (barman/pgbackrest/xtrabackup), retention enforcement. Output: `templates/backup-cronjob.yaml` + backup script ConfigMap | If `.Values.database.backup.schedule` is set, generate CronJob with operator-specific backup tool | P1 |
| 10.7.3 | Configure WAL archiving for PostgreSQL | Setup continuous WAL archiving to object storage. Features: archive_command config, compression (gzip), parallel archiving (2 workers). Output: WAL archive settings in PostgreSQL cluster spec | If PostgreSQL + backup enabled, configure archive_command or operator-native WAL archiving to S3 | P2 |
| 10.7.4 | Setup PITR restore procedures | Generate restore scripts/manifests for point-in-time recovery. Features: recovery target (time/XID), restore from backup + WAL replay. Output: `templates/restore-configmap.yaml` with restore commands | Create ConfigMap with restore commands and recovery target configuration for disaster recovery | P2 |
| 10.7.5 | Auto-detect backup retention policies | Generate retention rules based on values. Settings: retentionDays → full backup count, incremental retention. Output: retention configuration in backup spec | Convert `.Values.database.backup.retentionDays` to operator-specific retention config (30d default) | P2 |

**Technical Notes:**
- **Storage backends**: S3 (AWS), MinIO (self-hosted), GCS (Google Cloud Storage), Azure Blob — all S3-compatible
- **Backup tools by operator**: CloudNativePG (barman-cloud), Crunchy (pgBackRest), Percona XtraDB (Percona XtraBackup), MariaDB (MariaDB Backup)
- **WAL archiving**: PostgreSQL-specific, required for PITR (point-in-time recovery), continuous archiving to S3
- **Retention**: Full backups (daily/weekly), WAL archives (continuous), retention period (30 days default)

### 10.8 High Availability Patterns (5 задач)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 10.8.1 | Auto-configure streaming replication | Generate PostgreSQL streaming replication config. Features: async replication (default), WAL sender processes, replication slots. Settings: instances: 3 (1 primary + 2 replicas). Output: replication configuration in cluster spec | If `.Values.database.replication.enabled == true`, configure primary + N replicas with streaming replication | P1 |
| 10.8.2 | Setup synchronous replicas for zero RPO | Configure sync standby for no data loss. Features: `synchronous_standby_names` = ANY 1 (*), min/max sync replicas. Validation: instances ≥ minSyncReplicas + 1. Output: synchronous replication settings | If `.Values.database.replication.synchronous == true`, set synchronous_standby_names or operator equivalent | P2 |
| 10.8.3 | Configure automatic failover | Enable operator-managed automatic failover. Features: health checks (30s interval), failover timeout (90s default), split-brain prevention. Most operators support auto-failover by default. Output: failover configuration in HA section | Most operators have auto-failover enabled by default, configure failover timeout via `.Values.database.ha.failoverTimeout` | P2 |
| 10.8.4 | Setup read replicas for load distribution | Generate read-only replica instances for query load distribution. Features: load balancing across replicas, read-only connections, lag monitoring. Output: replica instances with read-only flag | If `.Values.database.readReplicas.enabled == true`, add replica instances with read-only configuration | P2 |
| 10.8.5 | Prevent split-brain with quorum-based failover | Configure Sentinel/quorum for safe failover. Features: quorum = (N/2) + 1, odd node counts (3, 5, 7), majority voting. Output: quorum configuration in HA settings | For Redis/MongoDB, ensure odd number of nodes (3, 5, 7) for quorum-based consensus | P2 |

**Technical Notes:**
- **Streaming replication**: PostgreSQL native, async by default (sync adds latency but zero RPO)
- **Synchronous replication**: `ANY 1 (*)` = at least 1 sync standby, `FIRST 2 (replica1, replica2)` = prefer specific standbys
- **Automatic failover**: Operators use health checks + consensus (Patroni for Postgres, Sentinel for Redis, Replica Set elections for MongoDB)
- **Read replicas**: Horizontal scaling for read-heavy workloads, lag monitoring important (acceptable lag < 1s)
- **Split-brain prevention**: Quorum-based consensus (majority voting), odd node counts, network partitioning tolerance

---

## Phase 11: Advanced Scheduling (v0.11.0)

**Цель**: Production-ready scheduling с auto-detection topology spread, priority classes, affinity/anti-affinity, custom schedulers
**Задач**: 12 | **Оценка сложности**: Medium-High
**Automation rate**: 85% of scheduling patterns auto-detectable from replica count, workload type, GPU resources, namespace labels

**Research sources**: Kubernetes Scheduling Framework, Volcano (CNCF), YuniKorn (Apache), Kueue, Pod Topology Spread Best Practices

**Detection strategy**: Replica count (>=2/>=3), workload criticality (StatefulSet, namespace labels), GPU resources, cloud provider topology keys

### 11.1 Pod Topology Spread Constraints (3 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 11.1.1 | Zone-level topology spread | Auto-generate `topologySpreadConstraints` for zone distribution. Features: `maxSkew: 1`, `topologyKey: topology.kubernetes.io/zone`, `whenUnsatisfiable: ScheduleAnyway` (soft, default) OR `DoNotSchedule` (hard, production). Coordination: ensure PDB `minAvailable` + `maxSkew` constraints are satisfiable (e.g., 3 replicas, maxSkew=1, minAvailable=2 → requires 3 zones). Values: `topologySpread.{enabled,maxSkew,zoneDistribution}`. Output: topology spread section in Deployment/StatefulSet spec | Detect `replicas >= 3` in Deployment/StatefulSet + no existing `topologySpreadConstraints`. Cloud provider labels present (AWS/GCP/Azure zone topology key: `topology.kubernetes.io/zone`). Production environments (namespace name contains "prod/production" OR label `environment: production`) → hard constraint `DoNotSchedule` | P1 |
| 11.1.2 | Node-level anti-affinity via topology spread | Auto-generate hostname-based topology spread: `topologyKey: kubernetes.io/hostname`, `maxSkew: 1`. Prevents pod co-location on same node (alternative to podAntiAffinity). Interaction: coordinate with PodDisruptionBudget (`minAvailable` must allow spread). Values: `topologySpread.nodeDistribution: preferred|required`. Output: node-level topology constraint + PDB validation | Detect `replicas >= 2` + high availability requirements (production namespace, `app.kubernetes.io/part-of` label present, OR StatefulSet kind). Validate sufficient node count in cluster (via Phase 4 cluster extraction) | P2 |
| 11.1.3 | Custom topology keys | Support custom topology keys: `topology.kubernetes.io/region`, `node.kubernetes.io/instance-type`, user-defined labels (e.g., `rack`, `datacenter`). Multi-constraint generation: zone + node hostname (cascade). LabelSelector matching: coordinate with Deployment selector. Values: `topologySpread.customKeys[]` with key/maxSkew pairs. Output: multiple topology spread constraints in single podSpec | If `.Values.topologySpread.customKeys` specified (user override) OR custom node labels detected via cluster extraction (Phase 4 dependency). Example: `customKeys: [{key: "rack", maxSkew: 2}, {key: "zone", maxSkew: 1}]` | P3 |

**Technical Notes:**
- **Detection priority**: Zone spread for >=3 replicas (HA requirement), node spread for >=2 replicas
- **whenUnsatisfiable**: Default `ScheduleAnyway` (soft, P2) for dev/staging; `DoNotSchedule` (hard, P1) for production (detect via namespace name/labels)
- **PDB coordination**: Ensure `minAvailable` + `maxSkew` constraints are satisfiable — DHG should validate or warn
- **Helm templating**: Conditional generation via `{{ if .Values.topologySpread.enabled }}`

### 11.2 Priority & Preemption (2 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 11.2.1 | PriorityClass auto-assignment | Auto-detect workload criticality → assign PriorityClass. Tiers: system-critical (2000000000), production-high (10000), production-medium (5000), production-low (1000), dev/test (0). Generate PriorityClass resources if not present. Values: `priorityClass.{name,value,preemptionPolicy,globalDefault}`. Output: PriorityClass CRD (if missing) + `spec.priorityClassName` in workload | Pattern matching: namespace name (production/critical/system), labels (`app.kubernetes.io/component: database|api|frontend`), workload kind (StatefulSet > Deployment > Job). Deckhouse modules: use Deckhouse priority classes from `lib-helm` (detect via Deckhouse module pattern in Chart.yaml). Database workloads (labels, StatefulSet) → production-high | P1 |
| 11.2.2 | Preemption policy configuration | Configure `preemptionPolicy: PreemptLowerPriority` (default) OR `Never` (for non-preempting workloads). Use case: batch jobs should NOT preempt long-running services. Coordination: ensure low-priority batch workloads have `preemptionPolicy: Never`. Values: `priorityClass.preemptionPolicy: PreemptLowerPriority|Never`. Output: preemption policy in PriorityClass spec | Detect Job/CronJob resources (batch workloads) → set `preemptionPolicy: Never`. Detect critical services (StatefulSet, database labels, `app.kubernetes.io/component: api`) → allow preemption (`PreemptLowerPriority`). User override via `.Values.priorityClass.preemptionPolicy` | P2 |

**Technical Notes:**
- **System priority classes**: Use existing K8s system classes (`system-cluster-critical: 2000001000`, `system-node-critical: 2000000000`) for cluster components
- **Deckhouse integration**: Respect Deckhouse priority classes from `lib-helm` (detect via Deckhouse module pattern)
- **GlobalDefault**: Only one PriorityClass can have `globalDefault: true` — DHG should warn if generating multiple
- **Preemption fairness**: Lower-priority pods evicted first; scheduler respects PDB during preemption

### 11.3 Advanced Affinity & Anti-Affinity (3 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 11.3.1 | Pod anti-affinity for HA | Auto-generate `podAntiAffinity` with `preferredDuringSchedulingIgnoredDuringExecution` (soft, weight: 100) OR `requiredDuringScheduling...` (hard). Topology key: `kubernetes.io/hostname` (node-level) OR `topology.kubernetes.io/zone` (zone-level). Values: `affinity.podAntiAffinity.{type,weight,topologyKey}`. Output: podAntiAffinity section in podSpec | Detect `replicas >= 2` + no existing anti-affinity. Production environments (namespace name/label) → hard anti-affinity (`requiredDuringScheduling...`). Coordination: ensure enough nodes/zones for hard constraints (validate via cluster extraction). Soft anti-affinity (preferred) for dev/staging | P1 |
| 11.3.2 | Node affinity for specialized hardware | Auto-generate `nodeAffinity` for GPU/SSD/spot instances. Operators: `In` (required nodes), `NotIn` (exclude nodes), `Exists` (label presence), `DoesNotExist`. Match expressions: `nvidia.com/gpu: Exists`, `node.kubernetes.io/instance-type: In [c5.xlarge, c5.2xlarge]`. Values: `affinity.nodeAffinity.{required,preferred,matchExpressions[]}`. Output: nodeAffinity section with matchExpressions | Detect GPU resources (`nvidia.com/gpu`, `amd.com/gpu` limits) → add GPU node affinity (`nvidia.com/gpu: Exists`). Detect high IOPS requirements (PVC with fast storage class like `gp3`, `ebs-ssd`) → SSD node affinity. Spot instance flag (`cost.optimization: spot`) → spot node affinity (`eks.amazonaws.com/capacityType: SPOT`) | P1 |
| 11.3.3 | Inter-service pod affinity | Auto-generate `podAffinity` for co-location (latency-sensitive services). Use case: frontend → backend affinity (same node/zone). Topology key: `kubernetes.io/hostname` (tight co-location, same node) OR `topology.kubernetes.io/zone` (loose co-location). LabelSelector: target service's selector labels. Values: `affinity.podAffinity.{enabled,targetService,topologyKey}`. Output: podAffinity section with labelSelector | Detect relationship graph: Service A → Service B (high traffic, >1000 req/s). Latency-sensitive annotations (`latency.requirement: low`, `co-locate-with: backend`). User-specified via `.Values.affinity.coLocateWith: serviceName`. Default: zone-level co-location (looser constraint) | P3 |

**Technical Notes:**
- **Hard vs soft**: `required...` blocks scheduling if unsatisfiable; `preferred...` is best-effort with weights (0-100)
- **topologyKey selection**: `hostname` = strict (same node), `zone` = loose (same zone/AZ), `region` = very loose
- **Weight calculation**: Multiple preferred rules cumulative; highest total weight wins
- **Conflict detection**: Anti-affinity + affinity on same selector → DHG should warn user of contradiction

### 11.4 Taints & Tolerations (2 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 11.4.1 | GPU/special hardware tolerations | Auto-add tolerations for tainted GPU/high-memory nodes. Common taints: `nvidia.com/gpu:NoSchedule`, `node.kubernetes.io/memory-pressure:NoSchedule`, `dedicated=gpu:NoSchedule`, `spot:NoSchedule`. Effect types: `NoSchedule` (block new pods), `PreferNoSchedule` (avoid if possible), `NoExecute` (evict existing). Values: `tolerations[].{key,operator,value,effect}`. Output: tolerations array in podSpec | Detect GPU resources (`nvidia.com/gpu`, `amd.com/gpu`, `intel.com/gpu` limits) → add GPU toleration. Detect large memory requests (>64Gi) → add memory-pressure toleration. Spot instances (`cost.optimization: spot`) → spot taint toleration (`spot:NoSchedule`). Operator: `Equal` (key=value) OR `Exists` (key presence only) | P1 |
| 11.4.2 | Control plane & not-ready tolerations | Auto-add tolerations for DaemonSets: `node-role.kubernetes.io/control-plane:NoSchedule` (schedule on masters), `node.kubernetes.io/not-ready:NoExecute` (stay on unready nodes), `node.kubernetes.io/unreachable:NoExecute`. TolerationSeconds: 300 (5min grace period before eviction). Values: `tolerations[].{key,effect,tolerationSeconds}`. Output: system-level tolerations for DaemonSets + system components | Detect DaemonSet kind → auto-add control-plane + not-ready tolerations. System components (namespace: `kube-system`, `deckhouse`, labels `app.kubernetes.io/managed-by: deckhouse`) → aggressive tolerations (tolerationSeconds: 0 for immediate tolerance). Flag: `--daemonset-tolerations=aggressive|standard` | P2 |

**Technical Notes:**
- **Operator types**: `Equal` (key=value match), `Exists` (key presence only, matches any value)
- **Effect hierarchy**: `NoExecute` > `NoSchedule` > `PreferNoSchedule` (NoExecute actively evicts)
- **TolerationSeconds**: Only valid for `NoExecute` effect; triggers graceful pod eviction after timeout (default: unlimited if not specified)
- **Wildcard toleration**: `operator: Exists` without key tolerates ALL taints (use with caution, DaemonSets only)

### 11.5 Scheduler Profiles & Custom Schedulers (2 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 11.5.1 | Custom scheduler assignment | Support custom schedulers: Volcano (gang scheduling, GPU), YuniKorn (Apache, hierarchical queues), Kueue (job queueing), default-scheduler profiles. Set `spec.schedulerName` in Pod spec. Use cases: ML training (Volcano gang), batch (Kueue queue), multi-tenancy (YuniKorn hierarchical queues). Values: `schedulerName: volcano|yunikorn|kueue|default-scheduler`. Output: `schedulerName` field in podSpec | Detect ML workload patterns (Kubeflow Training CRDs: PyTorchJob/TFJob/MPIJob, GPU resources, multi-pod jobs with `parallelism > 1`) → suggest Volcano. Detect batch job patterns (CronJob, Job with parallelism) → suggest Kueue. User-specified via `.Values.scheduling.customScheduler`. Validate scheduler exists in cluster (via Phase 4 cluster extraction) | P3 |
| 11.5.2 | Volcano gang scheduling integration | Auto-generate Volcano `PodGroup` for gang scheduling (all-or-nothing pod scheduling). Use case: distributed training (all workers must start together). Fields: `minMember` (minimum pods for gang), `minResources` (total resources required), `queue` (Volcano queue name). Coordination: link PodGroup to Job/TrainingJob via annotation `scheduling.k8s.io/group-name`. Values: `volcano.{enabled,minMember,queue,priority}`. Output: `templates/podgroup.yaml` + annotation in workload | Detect multi-replica training workloads (MPIJob, PyTorchJob with `spec.pytorchReplicaSpecs.Worker.replicas > 1`, TFJob) → generate PodGroup. Detect `parallelism > 1` in Job + GPU resources → suggest gang scheduling. Volcano CRDs present in cluster (`PodGroup` CRD exists via cluster extraction). minMember = sum of all replica counts | P3 |

**Technical Notes:**
- **Scheduler plugins**: Default scheduler supports profiles (low-latency, bin-packing, score-based); custom schedulers replace default entirely
- **Volcano**: Best for HPC/ML gang scheduling; requires Volcano operator installation (detect via CRD presence)
- **YuniKorn**: Apache-licensed, strong multi-tenancy support; hierarchical queues (`root.prod.team-a.user`)
- **Kueue**: K8s-native job queueing (CNCF sandbox); integrates with Kueue ClusterQueue/LocalQueue CRDs

---

## Phase 12: Data Management & CSI (v0.12.0)

**Цель**: Production-ready data management с volume snapshots, CSI driver integration, encryption at rest, backup automation
**Задач**: 10 | **Оценка сложности**: Medium-High
**Automation rate**: 90% of storage patterns auto-detectable from cloud provider, workload type, security requirements

**Research sources**: Kubernetes CSI Developer Documentation, AWS EBS CSI, GCP Persistent Disk CSI, Azure Disk CSI, Velero, Ceph RBD, Longhorn

**Detection strategy**: Cloud provider labels, StorageClass provisioner field, workload criticality, namespace labels (environment, security-tier), compliance annotations

### 12.1 Volume Snapshots (3 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 12.1.1 | VolumeSnapshotClass Auto-Generation | DHG auto-generates VolumeSnapshotClass CRD для CSI driver с appropriate deletionPolicy. Features: driver auto-detection (ebs.csi.aws.com, pd.csi.storage.gke.io, disk.csi.azure.com, rbd.csi.ceph.com, driver.longhorn.io), deletionPolicy: `Delete` (dev/test) OR `Retain` (production), driver-specific parameters (AWS EBS: `encrypted: true` + KMS key ARN, GCP: `storage-locations` + snapshot type, Azure: `incremental: true`). Values: `volumeSnapshots.{enabled,snapshotClass.name,snapshotClass.deletionPolicy,snapshotClass.parameters}`. Output: `templates/volumesnapshotclass.yaml` + parameters ConfigMap | Detect StatefulSet/PVC + `.Values.volumeSnapshots.enabled == true` + CSI driver from PVC `spec.storageClassName` (lookup StorageClass → provisioner field). Cloud provider detection: AWS (storageClass provisioner: `ebs.csi.aws.com`), GCP (`pd.csi.storage.gke.io`), Azure (`disk.csi.azure.com`). Production environment (namespace label `environment: production`) → deletionPolicy: Retain | P1 |
| 12.1.2 | Snapshot Scheduling with CronJob | Auto-generate Kubernetes CronJob для periodic snapshot creation с VolumeSnapshot CRD templates. Features: configurable schedule (daily 2 AM default: `0 2 * * *`), retention policy (keep last 30 snapshots), snapshot naming (`{pvc-name}-{timestamp}`), source PVC auto-detection from StatefulSet volumeClaimTemplates. Lifecycle: CronJob creates VolumeSnapshot → CSI driver creates actual backend snapshot → retention job prunes old snapshots (via separate CronJob with kubectl delete). Values: `volumeSnapshots.{schedule,retentionCount,labelSelector}`. Output: `templates/snapshot-cronjob.yaml` + VolumeSnapshot template + retention cleanup job | Detect StatefulSet with volumeClaimTemplates OR standalone PVC + `.Values.volumeSnapshots.schedule` defined. Database workloads (labels: `app.kubernetes.io/component: database`, OR StatefulSet + PVC > 10Gi) → default enable snapshot scheduling. Retention count from `.Values.volumeSnapshots.retentionCount` (default: 30) | P1 |
| 12.1.3 | Restore from Snapshot Templates | Generate Helm template helpers для VolumeSnapshot-based restore workflows. Features: restore PVC template (dataSource.kind: VolumeSnapshot, dataSource.name: snapshot-name), conditional restore mode (`restore.enabled: true` → PVC uses snapshot as dataSource), pre-restore validation hooks (Job checks snapshot readyToUse status). Integration: coordinate with StatefulSet update strategy (Recreate for restore scenario). Values: `restore.{enabled,snapshotName,validateSnapshot}`. Output: `_helpers.tpl` restore macros + conditional PVC dataSource section + pre-restore validation Job template | User-initiated restore workflow (`.Values.restore.enabled == true` + `.Values.restore.snapshotName` specified) OR disaster recovery mode flag (`--enable-dr-restore`). Validation: check VolumeSnapshot exists and status.readyToUse == true before PVC creation (via init Job with kubectl wait) | P2 |

**Technical Notes:**
- **CSI driver auto-detection**: Parse StorageClass provisioner field from cluster (Phase 4 cluster extractor dependency) or from existing PVC manifests
- **Snapshot controller**: Assumes snapshot-controller and snapshot CRDs (snapshot.storage.k8s.io/v1) are pre-installed — DHG should warn if not detected
- **Asynchronous operations**: VolumeSnapshot creation is async; DHG-generated restore jobs must poll `status.readyToUse` before proceeding
- **Retention enforcement**: Cleanup CronJob uses label selectors + `kubectl delete` with `--sort-by=.metadata.creationTimestamp`

### 12.2 CSI Driver-Specific Storage Classes (4 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 12.2.1 | Cloud Provider Storage Class Auto-Selection | DHG auto-generates appropriate StorageClass based on cloud provider detection + workload requirements. Features: AWS EBS (gp3 default, io2 for databases, st1 for analytics), GCP Persistent Disk (pd-ssd for databases, pd-standard for general, pd-balanced), Azure Disk (Premium_LRS for production, StandardSSD_LRS for dev). Parameters: IOPS (io2: 3000-64000), throughput (gp3: 125-1000 MiB/s), fsType (ext4/xfs). Values: `storageClass.{type,parameters.iops,parameters.throughput,parameters.volumeType}`. Output: `templates/storageclass.yaml` with cloud-specific parameters | Cloud provider detection: node labels (`kubernetes.io/cloud-provider: aws|gce|azure`) via cluster extraction OR explicit `.Values.cloudProvider`. Workload type: database (StatefulSet + labels `app.kubernetes.io/component: database`) → high IOPS (gp3 IOPS: 16000, io2), web/general (Deployment) → standard (gp3 default 3000 IOPS, pd-balanced). Storage size: >1TB → throughput-optimized (st1 for AWS, pd-standard for GCP) | P1 |
| 12.2.2 | Volume Binding Mode Auto-Configuration | Auto-set `volumeBindingMode: WaitForFirstConsumer` для zone-aware storage provisioning. Benefits: prevents "volume in wrong zone" errors, respects pod topology spread + affinity/anti-affinity, aligns with node selectors. Use cases: multi-zone clusters (detect 2+ zones from node labels), StatefulSets with zone spread (topology spread constraints), workloads with node affinity. Values: `storageClass.volumeBindingMode: Immediate|WaitForFirstConsumer`. Output: volumeBindingMode field in StorageClass | Detect multi-zone cluster: node labels contain `topology.kubernetes.io/zone` with 2+ distinct values (via cluster extraction). Workloads with topology spread constraints OR pod anti-affinity (zone-level) → auto-enable WaitForFirstConsumer. Single-zone clusters → Immediate. Default: WaitForFirstConsumer for all multi-zone clusters (GKE, EKS multi-AZ, AKS) | P1 |
| 12.2.3 | Volume Expansion Auto-Enable | Auto-set `allowVolumeExpansion: true` для StorageClass to support online PVC resizing. Features: CSI driver capability detection (check CSI driver supports EXPAND_VOLUME), validation warnings (warn if driver doesn't support online expansion), resize instructions in NOTES.txt (`kubectl patch pvc` commands). Limitations: some CSI drivers require pod restart for filesystem resize (XFS online resize requires kernel 4.18+). Values: `storageClass.allowVolumeExpansion: true|false`. Output: allowVolumeExpansion field + NOTES.txt resize instructions | Auto-enable for all CSI drivers that support expansion (AWS EBS CSI, GCP PD CSI, Azure Disk CSI, Ceph RBD CSI, Longhorn CSI — all support as of 2026). Detect CSI driver from provisioner field. Warn if in-tree provisioner detected (kubernetes.io/aws-ebs, kubernetes.io/gce-pd) → suggest migration to CSI. Default: enabled for CSI drivers, disabled for in-tree provisioners | P2 |
| 12.2.4 | Reclaim Policy Selection | Auto-configure `reclaimPolicy: Delete|Retain` based on environment. Features: production → Retain (prevent accidental data loss), dev/test → Delete (auto-cleanup), override via values. Coordination: align with VolumeSnapshotClass deletionPolicy (both Retain in production). Values: `storageClass.reclaimPolicy: Delete|Retain`. Output: reclaimPolicy field in StorageClass | Production environment (namespace label `environment: production`, namespace name contains `prod|production`) → Retain. Dev/test environments → Delete. Database workloads (StatefulSet + database labels) → Retain (data protection). User override via `.Values.storageClass.reclaimPolicy` | P2 |

**Technical Notes:**
- **Cloud provider detection**: Prefer cluster extraction (node labels) over values-based config for accuracy
- **WaitForFirstConsumer**: Google Cloud recommends this for all zonal storage (GCP PD), AWS multi-AZ clusters (EBS), Azure availability zones (Azure Disk)
- **Volume expansion**: Online expansion supported by most CSI drivers as of 2026; some filesystems (ext3/4, XFS) may require pod restart for resize to take effect
- **CSI vs in-tree**: In-tree provisioners (kubernetes.io/*) are deprecated as of K8s 1.23; DHG should suggest CSI migration

### 12.3 Encryption at Rest (2 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 12.3.1 | KMS-Encrypted Storage Classes | Auto-generate StorageClass with encryption-at-rest via cloud provider KMS. Features: AWS EBS (encrypted: "true", kmsKeyId: ARN from secret), GCP Persistent Disk (disk-encryption-kms-key: projects/{project}/locations/{location}/keyRings/{keyring}/cryptoKeys/{key}), Azure Disk (diskEncryptionSetID: resource ID). KMS key reference: secretRef → Secret with key ARN/ID. IAM permissions: EBS CSI driver requires kms:Decrypt, kms:GenerateDataKeyWithoutPlaintext, kms:CreateGrant. Values: `encryption.{enabled,kmsKeySecretName,kmsKeySecretKey}`. Output: encrypted StorageClass + Secret reference + IAM policy snippet (NOTES.txt) | Detect security requirements: namespace label `security-tier: high|critical`, compliance annotations (`compliance.required: pci-dss|hipaa|fedramp`), OR `.Values.encryption.enabled == true`. Cloud provider detection → select KMS integration pattern (AWS KMS, GCP Cloud KMS, Azure Key Vault). Production database workloads → auto-enable encryption. KMS key ARN from `.Values.encryption.kmsKeySecretName` (user must pre-create Secret with KMS key reference) | P1 |
| 12.3.2 | LUKS Encryption for Ceph/Longhorn | Auto-configure LUKS encryption для self-hosted storage (Ceph RBD CSI, Longhorn CSI). Features: encrypted PVC parameters (csi.storage.k8s.io/pvc/encrypt: "true"), passphrase management (Secret with encryption key), Ceph/Longhorn-specific encryption config (Ceph: rbd encryption format LUKS2, Longhorn: crypto.cipher.algorithm AES-256). Key rotation instructions (NOTES.txt). Values: `encryption.{luksEnabled,passphraseSecretName,cipher}`. Output: encrypted PVC parameters + passphrase Secret template + key rotation instructions | Detect Ceph RBD CSI (provisioner: rbd.csi.ceph.com) OR Longhorn CSI (driver.longhorn.io) + `.Values.encryption.luksEnabled == true`. Self-hosted clusters (no cloud provider labels) + security requirements → suggest LUKS encryption. Generate random passphrase Secret (base64-encoded 32-byte key) if `.Values.encryption.passphraseSecretName` not specified | P2 |

**Technical Notes:**
- **KMS IAM permissions**: AWS requires EBS CSI driver service account to have kms:Decrypt, kms:GenerateDataKeyWithoutPlaintext, kms:CreateGrant on KMS key — DHG should include IAM policy snippet in NOTES.txt
- **GCP KMS**: Requires Compute Engine Service Agent to have `cloudkms.cryptoKeyEncrypterDecrypter` role on Cloud KMS key
- **Azure Key Vault**: Requires Disk Encryption Set with access to Azure Key Vault + VM identity with permissions
- **LUKS**: Encryption happens at block layer (dm-crypt); passphrase rotation requires re-encrypting data (complex operation — NOTES.txt should warn)

### 12.4 Data Protection & Velero Integration (1 задача)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 12.4.1 | Velero Backup Annotations & Schedules | Auto-inject Velero backup annotations + generate Velero Schedule CRD для StatefulSet/PVC protection. Annotations: `backup.velero.io/backup-volumes: {volume-name-list}` (PVC volumes to snapshot), `backup.velero.io/backup-volumes-excludes` (temp volumes to skip), `pre.hook.backup.velero.io/command` (pre-backup hook — database dump), `post.hook.backup.velero.io/command` (post-backup verification). Schedule CRD: `spec.schedule` (cron: `0 2 * * *`), `spec.template.snapshotVolumes: true` (CSI snapshot integration), `spec.template.ttl: 720h` (30 days retention), `spec.template.includedNamespaces` (namespace filter). Values: `velero.{enabled,schedule,ttl,hooks.{pre,post}}`. Output: backup annotations in StatefulSet pod template + `templates/velero-schedule.yaml` + hook ConfigMaps | Detect StatefulSet/PVC + `.Values.velero.enabled == true` OR `.Values.backup.provider == "velero"`. Database workloads (labels: `app.kubernetes.io/component: database`) → auto-add pre-backup hook (postgres: `pg_dump`, mysql: `mysqldump`, mongodb: `mongodump`). Critical volumes (PVC > 10Gi OR database PVCs) → include in `backup-volumes` annotation. Temp/cache volumes (emptyDir, PVC with `storage.class: temp`) → exclude | P1 |

**Technical Notes:**
- **Velero CSI integration**: `snapshotVolumes: true` requires Velero CSI plugin + VolumeSnapshotClass configuration (coordinate with task 12.1.1)
- **Pre-backup hooks**: Database dumps ensure consistency; hook runs in pod before snapshot (exec into container). Hook timeout default: 30s (configurable via annotation `backup.velero.io/timeout`)
- **Retention policy**: Velero Schedule TTL (720h = 30 days) vs VolumeSnapshot retention (task 12.1.2) — both should align for consistent retention
- **Namespace scope**: Velero Schedule is namespace-scoped; for multi-namespace backups, generate Schedule per namespace OR use cluster-scoped Schedule with namespace selectors

---

## Phase 13: Edge Computing & Lightweight Kubernetes (v0.13.0)

**Цель**: Edge-ready Helm charts для lightweight K8s distributions, ARM architecture, air-gapped deployments, resource-constrained environments, IoT/device integration
**Задач**: 12 | **Оценка сложности**: High
**Automation rate**: 85% of edge patterns auto-detectable from node labels, CRDs, cluster metadata, image manifests

**Research sources**: K3s/Rancher, MicroK8s/Canonical, KubeEdge (CNCF), Akri (Microsoft), NATS Leaf Nodes, Harbor, Zarf, ARM multi-arch Docker

**Detection strategy**: Node labels (K3s, MicroK8s, edge, arch), CRD presence (KubeEdge Device, Akri Configuration), image manifest inspection, resource constraints analysis

### 13.1 Lightweight K8s Distribution Detection (3 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 13.1.1 | K3s Embedded Components Auto-Detection | Detect K3s cluster and configure Helm chart for embedded components. Features: SQLite3 embedded datastore (default for K3s), Klipper ServiceLB (embedded load balancer controller — creates DaemonSet per LoadBalancer Service using host ports), Traefik v3 Ingress (embedded controller), CoreDNS (embedded). Skip generating separate loadbalancer/ingress deployments when K3s detected. Values: `k3s.{detected,embeddedLB,embeddedIngress,datastore}`. Output: conditional template blocks (`{{ if not .Values.k3s.embeddedLB }}`), NOTES.txt with K3s-specific instructions (port-forward vs LoadBalancer IP), resource limit adjustments (lower defaults for edge) | Cluster extraction: node label `node.kubernetes.io/instance-type: k3s` OR kubeconfig context contains `k3s` OR API server endpoint contains `:6443` (K3s default) OR ConfigMap `kube-system/k3s-config` exists. ServiceLB: detect DaemonSet `kube-system/svclb-*` pods. Traefik: IngressClass `traefik` with controller `traefik.io/ingress-controller`. SQLite detection: API server flag `--datastore-endpoint` not set (embedded mode) | P1 |
| 13.1.2 | MicroK8s Addon Detection & Integration | Detect MicroK8s cluster and adapt chart generation for snap-based addons. Features: MetalLB addon detection (metallb namespace exists), GPU operator addon (nvidia.com/gpu resources present), Ingress addon (nginx-ingress-microk8s-controller), Storage addon (microk8s-hostpath provisioner). Skip generating services if addons provide equivalent functionality. Values: `microk8s.{detected,addons.{metallb,gpu,ingress,storage}}`. Output: conditional ServiceMonitor generation (if Prometheus addon enabled), GPU tolerations/nodeSelector, StorageClass selection (hostpath vs dynamic provisioner) | Cluster extraction: node label `microk8s.io/cluster: true` OR snap package metadata in node info OR ConfigMap `kube-system/microk8s-config`. Addon detection: namespace presence (metallb, gpu-operator-resources), IngressClass `public` (MicroK8s default), StorageClass `microk8s-hostpath`. GPU: `nvidia.com/gpu` in node allocatable resources | P2 |
| 13.1.3 | KubeEdge Cloud-Edge Architecture Detection | Detect KubeEdge cluster and generate edge-optimized manifests. Features: CloudCore (control plane in datacenter), EdgeCore (lightweight runtime on edge nodes, caches metadata locally), offline autonomy (edge nodes continue running when disconnected up to 24h default), device management (Device/DeviceModel CRDs). Generate node affinity for cloud vs edge placement, tolerations for edge nodes (edge.kubeedge.io/node=edge:NoSchedule), local-first ConfigMap/Secret caching strategies. Values: `kubeedge.{detected,cloudCore,edgeNodes[],offlineMode}`. Output: node affinity templates (cloud workloads on CloudCore nodes, edge workloads on EdgeCore nodes), device CRD templates (if IoT detected), edge-specific resource limits (lower CPU/memory for edge) | Cluster extraction: CRD `devices.devices.kubeedge.io` exists OR namespace `kubeedge` with CloudCore deployment OR node labels `node.kubeedge.io/edge-node: true` OR EdgeCore version annotation. Edge node detection: node taints `edge.kubeedge.io/node=edge:NoSchedule`. Device workload: detect Device/DeviceModel CRDs in manifests → generate edge-optimized device templates | P2 |

**Technical Notes:**
- **K3s binary size**: <100MB single binary (vs standard K8s ~1GB), embedded SQLite3 (default) or external etcd/MySQL/PostgreSQL
- **Klipper LoadBalancer**: Creates DaemonSet per LoadBalancer Service using host ports (no cloud provider required) — suitable for edge/on-prem
- **MicroK8s snap**: Single `snap install microk8s` command, addon system (`microk8s enable <addon>`), strict confinement for security
- **KubeEdge offline autonomy**: Edge nodes cache metadata/ConfigMaps locally, continue running workloads during cloud disconnection (up to 24h default)

### 13.2 ARM Architecture & Multi-Arch Support (2 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 13.2.1 | Multi-Arch Image Manifest Auto-Detection | Detect multi-arch images and configure Helm chart for ARM64/ARMv7 support. Features: Docker manifest list detection (platform-specific images under single tag), ARM64 (aarch64, linux/arm64) support for Raspberry Pi 4/M1/Graviton, ARMv7 (linux/arm/v7) for older Pi models, nodeSelector auto-generation (`kubernetes.io/arch: arm64`), resource limits adjustment (ARM typically 50-70% performance of x86 at same clock speed). Values: `image.{multiArch,platforms[],armOptimized}`, `resources.arm.{requests,limits}`. Output: multi-platform image references, ARM-specific resource profiles (50% of x86 baseline for RPi), architecture-aware nodeSelector | Image analysis: parse Deployment/StatefulSet images → check for manifest list (crane/skopeo inspect `--raw` shows `mediaType: application/vnd.docker.distribution.manifest.list.v2+json`). ARM platform detection: manifest includes `linux/arm64` OR `linux/arm/v7`. Cluster detection: node labels `kubernetes.io/arch: arm64|arm`. Auto-flag: if cluster has ARM nodes BUT image is x86-only → WARNING (incompatible architecture) | P1 |
| 13.2.2 | ARM Resource Constraint Profiles | Auto-adjust resource requests/limits for ARM edge devices. Features: Raspberry Pi 4 profiles (2GB/4GB/8GB RAM variants), single-core vs quad-core CPU allocation, GPU support (NVIDIA Jetson for ML inference: Jetson Nano 4GB/128 CUDA cores, Jetson Xavier 32GB/512 cores; Google Coral TPU), memory-constrained defaults (requests.memory: 64Mi-256Mi for edge workloads vs 128Mi-512Mi for x86). Values: `edge.deviceProfile: rpi4-2gb|rpi4-8gb|jetson-nano|jetson-xavier`, `resources.edge.{cpu,memory,gpu}`. Output: resource profiles in values.yaml, conditional GPU resource requests (`nvidia.com/gpu: 1` for Jetson, `edgetpu.google.com/tpu: 1` for Coral) | Device profile detection: node labels `device.hardware: raspberry-pi|jetson` OR `.Values.edge.deviceProfile` explicit. Memory constraints: node allocatable memory <2GB → apply low-memory profile. GPU detection: node allocatable resources include `nvidia.com/gpu` (Jetson) OR `edgetpu.google.com/tpu` (Coral). Workload type: ML inference (image contains `tensorflow|pytorch|onnx`) → allocate GPU resources | P1 |

**Technical Notes:**
- **Multi-arch images**: Use `docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7` to create manifest lists
- **ARM performance**: Raspberry Pi 4 (quad-core Cortex-A72 1.5GHz) ~50-70% performance of modern x86 processors at similar clock speeds
- **Raspberry Pi 4 RAM**: 2GB/4GB/8GB LPDDR4 variants; 2GB variant requires aggressive memory limits (<512Mi per pod typical)
- **NVIDIA Jetson**: Jetson Nano (4GB, 128 CUDA cores), Jetson Xavier (32GB, 512 CUDA cores), Jetson Orin (64GB, 2048 CUDA cores) — use `nvidia.com/gpu` resource type
- **ARM chart compatibility**: ~80% of popular Helm charts support multi-arch as of 2026 (up from ~20% in 2021)

### 13.3 Air-Gapped & Offline Deployment Patterns (3 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 13.3.1 | Private Image Registry Configuration | Auto-generate values and manifests for private image registry pattern. Features: global image registry override (`global.imageRegistry: registry.local:5000`), imagePullSecrets auto-injection, image list extraction (`images.txt` for all container images in chart), mirror script generation (`mirror-images.sh` using skopeo/crane for bulk copy). Registries: Harbor (production, vulnerability scanning with Trivy/Clair, RBAC, replication), Docker Distribution/Registry v2 (lightweight), Zarf embedded registry (air-gapped bootstrap, splits image data into ConfigMaps). Values: `global.{imageRegistry,imagePullSecrets[]}`, `airgap.{enabled,registry,authSecretName}`. Output: `values-airgap.yaml` with registry overrides, `scripts/mirror-images.sh`, imagePullSecrets in all workload templates | Air-gap detection: `.Values.airgap.enabled == true` OR namespace annotation `deployment.mode: airgapped` OR existing image references use non-public registries (not docker.io/gcr.io/quay.io). Extract all images: parse all container specs → unique image list → generate `images.txt`. Registry type: Harbor (if `.Values.airgap.registry` contains `harbor`), Zarf (if `zarf-injector` namespace exists in cluster), generic Docker Registry v2 otherwise | P1 |
| 13.3.2 | Helm Chart Vendoring & Dependency Bundling | Auto-configure Helm chart dependencies for offline installation. Features: `helm dependency build --skip-refresh` (download deps locally), chart archive bundling (all subcharts in `charts/` directory), dependency lock (`Chart.lock`) for version pinning, mirror chart repository setup (ChartMuseum/Harbor for air-gapped chart hosting). Values: `airgap.{chartMirror,vendorDependencies}`. Output: pre-downloaded charts in `charts/` directory, `Chart.lock` file, script `bundle-chart.sh` (helm package + dependency bundling), NOTES.txt with offline installation instructions | Dependency detection: `Chart.yaml` has `dependencies[]` field. Air-gap mode: `.Values.airgap.enabled` OR deployment mode annotation. Generate bundling script: `helm dependency build && helm package .` → creates `.tgz` with all dependencies. Chart mirror: if `.Values.airgap.chartMirror` specified → update `Chart.yaml` dependency `repository:` URLs to mirror endpoint | P2 |
| 13.3.3 | Init Container Image Pre-Pull Strategy | Generate init containers for critical image pre-pull in intermittent connectivity environments. Features: init container pulls all application images to node before main container starts (avoids startup delays during connectivity windows), uses `imagePullPolicy: IfNotPresent` for main containers (leverage cached images), failure tolerance (init container retry with exponential backoff, max retries: 5), image pre-warming CronJob (periodic pull to keep images fresh: cron `0 2 * * *`). Values: `edge.{prePullImages,prePullSchedule}`. Output: init container specs in Deployment/StatefulSet templates, CronJob for periodic pre-pull, image pull backoff configuration | Edge/offline detection: node labels `edge.connectivity: intermittent` OR `.Values.edge.prePullImages == true`. Critical workloads: StatefulSet (databases need fast startup), Deployment with `replicas > 3` (avoid thundering herd image pulls). Image size: if image >500MB → enable pre-pull to avoid startup delays during connectivity windows | P2 |

**Technical Notes:**
- **Harbor for air-gap**: Provides image replication, vulnerability scanning (Trivy/Clair), RBAC, webhook notifications; recommended for production air-gapped environments
- **Zarf**: Self-contained registry splits image data into ConfigMaps to bootstrap cluster without existing registry; solves chicken-egg problem
- **Image mirroring tools**: skopeo (Red Hat, no daemon required), crane (Google, Go binary), Docker registry mirror (pull-through cache)
- **Chart vendoring**: `helm dependency build` downloads charts to `charts/` dir; `helm package` creates `.tgz` with all dependencies embedded (self-contained archive)
- **Bandwidth optimization**: Harbor supports image compression, delta layers (only pull changed layers), and webhook-based replication for WAN-optimized sync

### 13.4 Resource-Constrained Edge Optimization (2 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 13.4.1 | Low-Memory Pod Optimization (<512MB nodes) | Auto-generate memory-optimized configurations for edge nodes with <512MB RAM. Features: aggressive resource limits (requests.memory: 32Mi-128Mi, limits.memory: 64Mi-256Mi), single replica default (replicas: 1 for edge), eviction thresholds (kubelet eviction at 100Mi available memory), memory-optimized base images (Alpine Linux <10MB, distroless <20MB vs Ubuntu 78MB, Debian 124MB), disabled probes for memory-constrained nodes (startup/liveness/readiness consume ~10-20Mi overhead). Values: `edge.{lowMemoryMode,memoryProfile: minimal|standard}`, `replicas: 1`. Output: reduced resource requests, single replica, probe disabling, memory-optimized image suggestions (NOTES.txt) | Low-memory node detection: cluster node allocatable memory <512MB OR `.Values.edge.lowMemoryMode == true`. K3s default detection (K3s minimum 512MB) → apply low-memory profile. Workload type: web service (nginx, caddy) → 64Mi request/128Mi limit; worker (batch jobs) → 32Mi request/64Mi limit. Warning: if original requests >512MB → incompatible with edge node (suggest re-architecture) | P1 |
| 13.4.2 | Single-Core CPU & SD Card Storage Optimization | Auto-optimize for single-core CPUs and SD card/eMMC storage constraints. Features: CPU pinning (`resources.requests.cpu: 100m-500m`, avoid >1 CPU request on single-core), I/O scheduler tuning (deadline scheduler for flash storage, avoid random writes), ephemeral storage limits (`resources.limits.ephemeral-storage: 1Gi` to prevent SD card wear), log rotation (aggressive: 10MB max, 2 files), read-only root filesystem (`readOnlyRootFilesystem: true` to reduce writes by 70-90%), emptyDir volume type prioritization (RAM-backed for temp data, avoid PVC on SD card). Values: `edge.{singleCore,flashStorage}`, `storage.type: sd-card|emmc`. Output: CPU request caps, ephemeral storage limits, readOnlyRootFilesystem securityContext, log rotation ConfigMap, emptyDir volumes for temp data | Single-core detection: node allocatable CPU = 1000m (1 core) OR `.Values.edge.singleCore == true`. Flash storage: node annotation `storage.type: sd-card|emmc` OR Raspberry Pi device profile (Pi uses microSD). Workload: if writes logs heavily (logging sidecar detected) → aggressive log rotation. Database workloads: WARNING on SD card (IOPS <100, unsuitable for databases) → suggest network storage or external SSD | P2 |

**Technical Notes:**
- **K3s low-memory**: K3s can run with 512MB RAM total, but leaves only ~200-300MB for workloads after system overhead
- **Memory-optimized images**: Alpine Linux (5-10MB base), distroless (10-20MB), busybox (1-5MB) vs standard Ubuntu (78MB), Debian (124MB)
- **SD card constraints**: microSD typical IOPS: 50-100 (vs SSD: 10,000-50,000), limited write endurance (10,000-100,000 write cycles per cell)
- **Single-core CPU**: Raspberry Pi 4 Model B has 4 cores @ 1.5GHz, but older Pi Zero W has single core @ 1GHz; avoid CPU-intensive workloads
- **Read-only root**: Requires writable volumes for `/tmp`, `/var/run`, application logs; reduces SD card wear by 70-90%

### 13.5 Intermittent Connectivity & Hub-Spoke Patterns (2 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 13.5.1 | Local-First Architecture & Edge Autonomy | Generate manifests optimized for intermittent connectivity with local-first semantics. Features: local ConfigMap/Secret caching (KubeEdge edge-side caching up to 24h), stateless workload prioritization (avoid dependencies on cloud APIs), retry/backoff for cloud sync (exponential backoff, max retry: 24h), graceful degradation patterns (local fallback data sources), node-local storage (emptyDir, hostPath for edge-local data), health probes tolerance (increase `failureThreshold: 10`, `periodSeconds: 30` for slow/intermittent networks). Values: `edge.{localFirst,offlineGracePeriod,cloudSyncRetry}`. Output: increased probe tolerances, local storage volumes, retry annotations, offline-capable init containers (pre-cache data before disconnect) | Intermittent connectivity detection: node label `edge.connectivity: intermittent` OR `.Values.edge.localFirst == true`. KubeEdge cluster: auto-enable edge caching (KubeEdge EdgeCore caches ConfigMap/Secret). Stateful workloads: if StatefulSet with cloud database → WARNING (unsuitable for offline) → suggest local database replica (edge-local PostgreSQL). Cloud API dependencies: scan env vars for external API endpoints → suggest local caching layer | P1 |
| 13.5.2 | Hub-Spoke Topology & Centralized Management | Generate hub-spoke architecture with centralized cloud control and distributed edge execution. Features: hub deployment (cloud/datacenter — centralized control plane, API aggregation, monitoring), spoke deployments (edge nodes — local execution, local data processing, periodic cloud sync), NATS leaf nodes (hub-spoke messaging, local-first with cloud sync, account permissions for data flow control), GitOps edge sync (ArgoCD/Flux edge agents with intermittent connectivity tolerance, local Git cache, interval-based sync: increase from 5m to 1h for intermittent links), namespace isolation per edge site. Values: `architecture.{hubSpoke,hub.{cloudRegion,resources},spokes[].{siteName,nodeSelector,resources}}`. Output: hub Deployment (cloud node affinity), spoke Deployments per site (edge node affinity), NATS leaf node configuration, GitOps sync wave annotations (hub first, spokes after), site-specific values (`values-site-{name}.yaml`) | Hub-spoke detection: multiple edge sites (`.Values.architecture.spokes[]` length >1) OR node labels `edge.site: {site-name}` with >1 distinct site. Hub placement: cloud nodes (label `node.type: cloud` OR no edge label) OR datacenter nodes. Spoke placement: edge nodes per site (nodeSelector `edge.site: {site-name}`). NATS detection: NATS StatefulSet exists → generate leaf node config. GitOps: ArgoCD Application/Flux Kustomization exists → enable edge sync agents | P2 |

**Technical Notes:**
- **KubeEdge offline**: EdgeCore caches ConfigMaps/Secrets for 24h default (configurable), continues pod reconciliation during cloud disconnect
- **NATS leaf nodes**: Edge nodes connect to hub via leaf node pattern; hub controls routing, permissions; local-first semantics (local pub/sub works offline)
- **Hub-spoke bandwidth**: Typical edge uplink: 1-10 Mbps (vs datacenter 1-10 Gbps); optimize for low bandwidth (delta updates, compression, batching)
- **GitOps edge**: ArgoCD edge agents cache Git repo locally, sync when connectivity available; Flux supports interval-based sync (default 5m, increase to 1h for intermittent links)
- **Site isolation**: Use namespace per site (`site-A`, `site-B`) OR label-based isolation with NetworkPolicy (`edge.site: A` can't reach `edge.site: B`)

### 13.6 IoT & Device Integration (2 задачи)

| # | Задача | Описание | Detection Logic | Priority |
|---|--------|----------|-----------------|----------|
| 13.6.1 | KubeEdge Device CRD & MQTT Integration | Generate KubeEdge Device/DeviceModel CRDs and MQTT broker integration for IoT edge workloads. Features: DeviceModel CRD (template for device class: sensors, actuators, cameras — protocol config, data properties), Device CRD (instance of DeviceModel: physical device metadata, device twin pattern for state synchronization), MQTT broker deployment (Eclipse Mosquitto for edge-local message bus: 3MB image, QoS 0/1/2 configuration, persistence PVC 1Gi), EventBus integration (KubeEdge component for MQTT subscription, forwards device status to cloud), device mapper Deployment (protocol adapter for Modbus/OPC-UA). Values: `iot.{enabled,deviceModels[],devices[],mqttBroker.{enabled,qos,persistence}}`. Output: DeviceModel CRDs, Device CRDs, MQTT broker StatefulSet + Service (port 1883/TCP), EventBus configuration, device mapper Deployment, device twin sync annotations | KubeEdge cluster detection: CRD `devices.devices.kubeedge.io` exists. IoT workload: `.Values.iot.enabled == true` OR Device/DeviceModel manifests present. Device protocol: MQTT (default for KubeEdge), Modbus (requires mapper deployment), OPC-UA (requires OPC-UA mapper). MQTT broker: if not present in cluster → auto-generate Mosquitto deployment (StatefulSet for persistence, PVC 1Gi, Service). Device count: 1-10 devices → single broker; >10 devices → broker per site (hub-spoke pattern) | P2 |
| 13.6.2 | Akri Device Discovery & Protocol Integration | Generate Akri Configuration/Instance CRDs for automatic device discovery (IP cameras, USB devices, industrial protocols). Features: Akri Configuration CRD (discovery handler: ONVIF IP cameras via WS-Discovery, udev USB devices via enumeration, OPC-UA servers via Local Discovery Server query), Akri Instance CRD (discovered device instances, node affinity to device location), protocol-specific discovery (ONVIF: multicast on 239.255.255.250:3702, OPC-UA: TCP port 4840, udev: USB hotplug), workload scheduling based on device presence (pod scheduled to node with device access), device capacity management (limit pods per device, share/exclusive usage modes). Values: `deviceDiscovery.{enabled,protocol: onvif|opcua|udev,capacity,brokerPod.{image,resources}}`. Output: Akri Configuration CRD (discovery handler config), broker pod template (workload accessing device), instance service (expose device data via Kubernetes Service), device filtering rules (vendor ID, model ID, network subnet) | Akri installation detection: CRD `configurations.akri.sh` exists OR `.Values.deviceDiscovery.enabled == true`. Protocol detection: ONVIF (IP cameras, network scan subnet `.Values.deviceDiscovery.onvif.subnet`), OPC-UA (industrial automation, discovery server URLs `.Values.deviceDiscovery.opcua.discoveryUrls[]`), udev (USB devices, udev rules `.Values.deviceDiscovery.udev.rules[]`). Device type: camera (ONVIF) → generate broker pod with video processing (FFmpeg, OpenCV), PLC (OPC-UA) → broker with protocol client, USB sensor (udev) → broker with device-specific driver | P3 |

**Technical Notes:**
- **KubeEdge Device CRDs**: `devices.devices.kubeedge.io/v1beta1` API version (stable since KubeEdge v1.12); DeviceModel = template, Device = instance
- **MQTT for IoT**: Eclipse Mosquitto (lightweight, 3MB Docker image), HiveMQ (commercial, clustering), EMQX (scalable, 100k+ connections per node)
- **Device twin pattern**: Cloud maintains desired state, edge reports actual state, reconciliation via MQTT; KubeEdge DeviceTwin module handles sync
- **Akri protocols**: ONVIF (camera discovery, WS-Discovery multicast), OPC-UA (industrial automation, TCP port 4840), udev (USB device hotplug)
- **Modbus/OPC-UA**: Modbus TCP (industrial protocol, port 502), OPC-UA (secure, port 4840); require custom device mappers (containers implementing protocol client)
- **Device mapper**: Bridge between Kubernetes (Device CRD) and physical device (protocol); runs as Deployment/DaemonSet on edge nodes

---

## Backlog (Post v1.0) — Детализированный

### P1 — Высокий приоритет (следующий после v1.0)

| # | Feature | Описание | Задачи | Зависимости |
|---|---------|----------|--------|-------------|
| B1.1 | **Config file `.dhg.yaml`** | Persistent settings per project. Schema: output mode, processors, excludes, secret strategy, template style. CLI flags override config. YAML format. | 3 | — |
| B1.2 | **GitOps manifest generation** | Генерация ArgoCD `ApplicationSet` / Flux `HelmRelease` CRDs alongside chart. Sync wave auto-assign по dependency graph. | 4 | Phase 5.3 (dependency graph) |
| B1.3 | **Sync wave annotations** | Auto-assign `argocd.argoproj.io/sync-wave` по resource dependencies: namespace=0, CRDs=1, RBAC=2, services=3, deployments=4. Configurable. | 2 | B1.2 |

### P2 — Средний приоритет

| # | Feature | Описание | Задачи | Зависимости |
|---|---------|----------|--------|-------------|
| B2.1 | **Interactive wizard (TUI)** | `dhg wizard` — пошаговая генерация через TUI (bubbletea). Steps: select source → select mode → configure options → preview → generate. | 5 | Phase 2 (all generators) |
| B2.2 | **OCI registry support** | `dhg push` — публикация чарта в OCI registry. `dhg pull` — скачивание. Аутентификация через `docker login` credentials. | 3 | Phase 6.3.4 |
| B2.3 | **Helm repo publish** | `dhg publish` — упаковка + публикация в Helm repo (ChartMuseum, Harbor, GitHub Pages). Auto-index update. | 3 | Phase 6.3 |
| B2.4 | **Watch mode** | `dhg watch -f manifests/ -o chart/` — отслеживание изменений в source файлах, авто-регенерация. fsnotify. Debounce 500ms. | 2 | — |
| B2.5 | **Multi-cluster support** | Генерация Cluster API `HelmChartProxy`. Region-aware `values-{region}.yaml`. Cluster selectors в templates. | 4 | Phase 4.1 (cluster extractor) |
| B2.6 | **Kustomize output** | `--output-format kustomize` — альтернативный output: `base/` + `overlays/{dev,staging,prod}/`. Kustomization.yaml generation. | 4 | Phase 2.4.3 (env-specific) |
| B2.7 | **Ingress → Gateway API миграция** | `dhg migrate-gateway` — автоматическое преобразование Ingress → HTTPRoute + Gateway. Маппинг annotations → HTTPRoute filters | 3 | Phase 3.5.1 |
| B2.8 | **Image inventory** | `dhg images -f chart/` — извлечение всех image references из чарта. Output: `images.txt`, JSON с tag/digest. Интеграция с `crane` для vulnerability check | 2 | — |

### P3 — Низкий приоритет

| # | Feature | Описание | Задачи | Зависимости |
|---|---------|----------|--------|-------------|
| B3.1 | **Helm 4 Charts v3 API** | Совместимость с будущим форматом. Server-side apply support. Multi-doc values. Digest-based installs. | 3 | Helm 4 stable release |
| B3.2 | **Terraform/Crossplane output** | `--output-format terraform` — генерация HCL с `helm_release` resource. `--output-format crossplane` — генерация Crossplane `Release`. | 4 | Phase 2 |

### P4 — Долгосрочные / экспериментальные

| # | Feature | Описание | Задачи | Зависимости |
|---|---------|----------|--------|-------------|
| B4.1 | **AI-assisted generation** | LLM-powered: auto-suggest values descriptions, auto-generate NOTES.txt, intelligent resource naming. MCP Server for Helm validation. | 5 | External LLM API |
| B4.2 | **Web UI** | Browser-based интерфейс: drag-and-drop YAML → visual graph → generated chart. React + Go API. | 8 | Phase 6 (stable API) |
| B4.3 | **VS Code extension** | Language server: DHG integration in IDE. Generate chart from open files. Inline preview. Validate on save. | 5 | Phase 6 (stable CLI) |
| B4.4 | **Helm 4 WASM plugins** | Поддержка будущего WASM plugin API для расширения генератора через sandboxed modules. | 3 | Helm 4 WASM spec |
| B4.5 | **MCP Server for Helm** | AI-assisted validation: валидация чартов через Model Context Protocol сервер. Context-aware suggestions из chart repos. | 4 | MCP ecosystem maturity |
| B4.6 | **Crossplane Composition output** | `--output-format crossplane` — генерация Crossplane `Composition` + `XRD` для infrastructure provisioning alongside app chart | 4 | Crossplane stable |
| B4.7 | **KubeVela Application output** | `--output-format kubevela` — генерация OAM `Application` с traits (ingress, autoscaler, rollout) wrapping Helm chart | 3 | KubeVela ecosystem |

---

## Competitive Landscape

### Существующие инструменты

| Инструмент | Направление | Scope | Архитектура |
|------------|------------|-------|-------------|
| **Helmify** | K8s YAML → Helm | Только генерация | CLI, single-purpose |
| **CDK8s** | Code → K8s YAML | Генерация + программная модель | Framework |
| **Kompose** | Docker Compose → K8s | Только генерация | CLI |
| **DHG (наш)** | K8s YAML + Deckhouse → Helm | Генерация + анализ + рекомендации | CLI, модульный |

### Отличия DHG

1. **Deckhouse-специфичная поддержка** — ни один инструмент не поддерживает Deckhouse CRD
2. **Анализ архитектуры** — определение паттернов и рекомендации по стратегии
3. **Best Practices Engine** — встроенная проверка соответствия
4. **Value Processor** — интеллектуальная обработка сложных данных
5. **Relationship Detection** — 13 типов связей между ресурсами
6. **Multiple output modes** — universal, separate, library, umbrella (roadmap)
7. **Cloud-native patterns** — workload identity, GPU, spot instances, mesh (roadmap)

### Смежные инструменты (НЕ конкуренты — дополняют)

| Инструмент | Роль | Интеграция с DHG |
|------------|------|-----------------|
| **Helmfile** | Declarative chart deployment | DHG generates → Helmfile deploys |
| **ArgoCD** | GitOps CD | DHG generates → ArgoCD syncs |
| **Checkov** | Security scanning | DHG generates → Checkov scans |
| **KubeLinter** | Static analysis | DHG generates → KubeLinter validates |
| **Trivy** | Vulnerability scanning | DHG generates → Trivy scans images |
| **Kubeconform** | Schema validation | DHG generates → Kubeconform validates |
| **Pluto** | Deprecated API detection | DHG pre-scans → auto-migrates |
| **cert-manager** | Certificate management | DHG generates annotations |
| **External Secrets** | Secret management | DHG generates ESO manifests |
| **KEDA** | Event-driven autoscaling | DHG generates ScaledObject |
| **Velero** | Backup & DR | DHG generates backup annotations |
| **Argo Rollouts** | Progressive delivery | DHG generates Rollout + AnalysisTemplate |
| **Flagger** | Canary automation | DHG generates Canary + MetricTemplate |
| **OPA/Gatekeeper** | Policy enforcement | DHG generates ConstraintTemplate |
| **Kyverno** | Policy engine | DHG generates ClusterPolicy |
| **SOPS** | Secret encryption | DHG generates encrypted secrets.yaml |
| **External DNS** | DNS automation | DHG generates ExternalDNS annotations |
| **Gateway API** | Modern networking | DHG generates HTTPRoute/Gateway |

---

## Summary Statistics

| Metric | Count |
|--------|-------|
| **Total phases** | 14 (Phase 1-6 + Phase 2.5: Security & Compliance + Phase 7: Production Operations + Phase 8: Developer Experience & Automation + Phase 9: AI/ML Workloads + Phase 10: Database Operators + Phase 11: Advanced Scheduling + Phase 12: Data Management & CSI + Phase 13: Edge Computing & Lightweight Kubernetes) |
| **Total tasks (phases)** | ~395 (Phase 7: 40, Phase 8: 40, Phase 9: 20, Phase 10: 51, Phase 11: 12, Phase 12: 10, Phase 13: 12) |
| **Total backlog items** | 20 |
| **Total backlog tasks** | ~65 |
| **Grand total** | ~460 задач |
| **K8s resource types covered** | 35+ (9 implemented + 26 planned) |
| **CRD types covered** | 57+ (Deckhouse + monitoring + mesh + KEDA + cert-manager + Gateway API + Rollouts + Flagger + **AI/ML (Kubeflow: Experiment, Workflow, PyTorchJob, TFJob, MPIJob, Notebook; KServe: InferenceService; Ray: RayCluster, RayJob; Seldon: SeldonDeployment; MinIO: Tenant)** + **Databases (CloudNativePG: Cluster; Zalando: postgresql; Crunchy: PostgresCluster, PGAdmin; Percona: PerconaXtraDBCluster, PerconaServerMongoDB; MariaDB: MariaDB; Oracle: InnoDBCluster; Redis Enterprise: RedisEnterpriseCluster, RedisEnterpriseDatabase; Spotahome: RedisFailover; MongoDB: MongoDBCommunity; K8ssandra: CassandraDatacenter; ClickHouse: ClickHouseInstallation; OpenSearch: OpenSearchCluster, OpenSearchISMPolicy, OpenSearchSLMPolicy)** + **Scheduling (Volcano: PodGroup; PriorityClass)** + **Storage (VolumeSnapshot, VolumeSnapshotClass, VolumeSnapshotContent, StorageClass, Velero: Schedule)** + **Edge/IoT (KubeEdge: Device, DeviceModel; Akri: Configuration, Instance)**) |
| **Output modes** | 7 (universal ✅, separate, library, umbrella, deckhouse-module, kustomize, air-gapped) |
| **Secret strategies** | 7 (plain ✅, ESO, sealed, vault-csi, vault-inject, SOPS, helm-secrets) |
| **Cloud providers** | 3 (AWS, GCP, Azure) |
| **Policy engines** | 2 (OPA/Gatekeeper, Kyverno) |
| **Progressive delivery** | 2 (Argo Rollouts, Flagger) |
| **Security profiles** | 3 (PSS restricted/baseline/privileged) |
| **Ingress controllers** | 3 (Nginx, Traefik, HAProxy) |

---

## Version History

| Version | Date | Milestone |
|---------|------|-----------|
| v0.1.0 | 2026-01-22 | Initial: file extractor, K8s processors, universal generator |
| v0.1.1 | 2026-01-30 | Pattern analyzer, value processor, external files, best practices |
| v0.2.0 | 2026-02-17 | Core stabilization: tests, additional resources, CLI improvements |
| v0.3.0 | 2026-02-18 | Complete output modes: separate, library, umbrella generators |
| v0.4.0 | 2026-02-19 | Deckhouse integration: CRD processors, module structure, monitoring, Gateway API, KEDA, cert-manager |
| v0.5.0 | 2026-02-19 | BREAKING: SanitizeServiceName, critical security fixes, graph optimizations |
| v0.6.0 | 2026-02-20 | Distribution: GoReleaser, Docker, Homebrew, coverage 78%, OSS infrastructure |
| v1.0.0 | TBD | Production release: performance, distribution, docs, supply chain |
