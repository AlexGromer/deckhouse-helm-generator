# DHG v1.0.0 — Production Release

**Дата выпуска:** 30 марта 2026 г.
**Репозиторий:** https://github.com/AlexGromer/deckhouse-helm-generator
**Тег:** `v1.0.0`

---

## Обзор

**DHG v1.0.0** — первый стабильный производственный релиз Deckhouse Helm Generator.

Начиная с v0.7.3, проект прошёл три крупных этапа разработки (Phase 5.5–5.9, Phase 5.10, Phase 6), которые вывели инструмент на уровень production-ready: интеллектуальный анализ манифестов, расширенная шаблонизация, управление секретами, сервисные меши, наблюдаемость, облачные паттерны и полный цикл производственной полировки.

**Итог:** 90+ генераторов, 50+ процессоров, 2372 теста, 86%+ покрытие кода, поддержка 6 платформ, ~35 000 строк кода, полноценный plugin system.

---

## Что нового с v0.7.3

### Phase 5.5–5.9: Smart Analysis, Advanced Templating, Secret Management, Service Mesh, Observability

_+17 500 LOC, +380 тестов (33 задачи)_

#### Phase 5.5 — Smart Analysis Engine
- **Умный анализатор зависимостей**: автоматическое обнаружение скрытых зависимостей между ресурсами (env-refs, volume-mounts, RBAC-bindings)
- **Cost estimation**: оценка стоимости ресурсов на основе requests/limits с поддержкой AWS/GCP/Azure тарифов
- **Compliance checker**: встроенная проверка на соответствие CIS Kubernetes Benchmark, NSA Hardening Guide
- **Drift detection**: сравнение live-состояния кластера с Helm-values для выявления расхождений

#### Phase 5.6 — Advanced Templating
- **Template inheritance**: поддержка базовых шаблонов с переопределением через overlay-файлы
- **Conditional blocks**: умная генерация `{{- if }}` блоков на основе анализа паттернов использования
- **Helper functions**: расширенная библиотека `_helpers.tpl` (30+ функций: `dhg.fullname`, `dhg.labels`, `dhg.annotations`, `dhg.resources`, `dhg.probes`)
- **Schema validation**: автогенерация `values.schema.json` с полным описанием всех values

#### Phase 5.7 — Secret Management
- **External Secrets Operator**: автогенерация `ExternalSecret` и `SecretStore` для AWS SM, GCP SM, Azure KV, HashiCorp Vault
- **Sealed Secrets**: поддержка генерации `SealedSecret` манифестов с kubeseal интеграцией
- **Secret rotation templates**: шаблоны для автоматической ротации секретов через CronJob
- **Secret scanning**: детектирование жёстко прописанных секретов в исходных манифестах

#### Phase 5.8 — Service Mesh
- **Istio integration**: генерация `VirtualService`, `DestinationRule`, `ServiceEntry`, `PeerAuthentication`, `AuthorizationPolicy`
- **Istio sidecar processor**: извлечение конфигурации sidecar injection из аннотаций и labels
- **Traffic management**: автогенерация retry policies, circuit breakers, timeout configurations
- **mTLS enforcement**: шаблоны для включения Strict/Permissive mTLS на уровне namespace и workload

#### Phase 5.9 — Observability
- **OpenTelemetry**: генерация `OpenTelemetryCollector` и `Instrumentation` ресурсов (operator.opentelemetry.io)
- **Prometheus annotations processor**: извлечение `prometheus.io/scrape`, `prometheus.io/path`, `prometheus.io/port`
- **Loki integration**: генерация `PodLogs` ресурсов для Grafana Loki
- **SLO/SLI templates**: шаблоны PrometheusRule для определения Service Level Objectives

---

### Phase 5.10: Cloud-Native Patterns

_+2 700 LOC, +55 тестов (5 задач)_

#### Workload Identity
- **Workload Identity processor**: поддержка GKE Workload Identity (`iam.gke.io/gcp-service-account`), EKS IRSA (`eks.amazonaws.com/role-arn`), AKS Workload Identity
- **ServiceAccount annotations**: автогенерация аннотаций для cloud IAM интеграции
- **Projected volumes**: шаблоны для `serviceAccountToken` projected volumes

#### GPU/TPU Support
- **GPU processor**: извлечение `nvidia.com/gpu`, `amd.com/gpu`, `google.com/tpu` resource requests
- **Node selectors**: автогенерация `nodeSelector` и tolerations для GPU/TPU узлов
- **RuntimeClass**: генерация шаблонов с `runtimeClassName: nvidia`

#### Windows Workloads
- **Windows processor**: поддержка `windows` OS selector (`.spec.os.name: windows`)
- **OS-specific tolerations**: автогенерация toleration `kubernetes.io/os=windows:NoSchedule`
- **Windows security context**: генерация `windowsOptions` в securityContext

#### Velero Backup
- **Velero annotations**: детектирование и генерация `backup.velero.io/backup-volumes` аннотаций
- **Schedule templates**: генерация `Schedule` ресурсов Velero для periodic backup
- **Restore hooks**: шаблоны для pre/post restore hooks

#### Flux CD Integration
- **Flux processor**: поддержка `HelmRelease`, `HelmRepository`, `Kustomization` ресурсов (fluxcd.io)
- **GitOps-ready values**: генерация values файлов, совместимых с Flux substitution vars

---

### Phase 6: Production Polish

_+8 400 LOC, +151 тест (26 задач)_

#### Phase 6.1 — Validation & Testing
- **Schema validation framework**: полная валидация всех генерируемых ресурсов против Kubernetes OpenAPI schemas
- **E2E test suite**: расширенный набор end-to-end тестов с реальными Helm chart сценариями
- **Benchmark suite**: benchmarks для всех критических путей (extract/process/generate)
- **Fuzz testing**: fuzzing входных YAML манифестов для выявления edge cases

#### Phase 6.2 — Supply Chain Security
- **cosign signing**: подписание всех бинарных артефактов через Sigstore/cosign
- **SBOM generation**: автоматическая генерация Software Bill of Materials через Syft (SPDX + CycloneDX форматы)
- **SLSA provenance**: генерация SLSA provenance attestation для каждого релиза
- **Vulnerability scanning**: интеграция Grype/Trivy в release pipeline

#### Phase 6.3 — Performance
- **Параллельная обработка**: worker pool для параллельного процессинга независимых ресурсов
- **Кэширование**: кэш схем и обработанных ресурсов с инвалидацией по содержимому
- **Streaming parser**: потоковый YAML-парсер для обработки больших манифестов (>100MB) без загрузки в память
- **Benchmarks**: P50/P95/P99 задокументированы для стандартных сценариев

#### Phase 6.4 — Distribution
- **nfpm packages**: генерация DEB/RPM/APK пакетов через nfpm (GoReleaser интеграция)
- **Multi-arch Docker**: образы для `linux/amd64`, `linux/arm64`, `linux/arm/v7`
- **Homebrew formula**: автоматическое обновление tap при каждом релизе
- **Chocolatey**: пакет для Windows Package Manager

#### Phase 6.5 — Documentation
- **ADR.md**: полный журнал архитектурных решений (50 записей, ADR-001..ADR-050)
- **DEVELOPER.md**: руководство разработчика — setup, testing, contribution workflow, ADR процесс
- **USER_GUIDE.md**: подробное руководство пользователя с примерами для каждого режима
- **Architecture diagrams**: C4-диаграммы (Context, Container, Component) в Mermaid

#### Phase 6.6 — Plugin System
- **External processors**: поддержка пользовательских processors через plugin interface
- **.dhg.yaml config**: глобальный конфигурационный файл (поиск плагинов, настройки генератора, defaults)
- **Template overrides**: механизм переопределения встроенных шаблонов пользовательскими
- **Plugin registry**: команда `dhg plugin list/install/remove` для управления плагинами

---

## Новые команды CLI

| Команда | Описание |
|---------|----------|
| `dhg fix` | Автоматическое исправление обнаруженных проблем в манифестах (deprecated APIs, security violations) |
| `dhg graph` | Визуализация графа зависимостей ресурсов (DOT/Mermaid/JSON форматы) |
| `dhg migrate` | Миграция Helm chart между версиями API Kubernetes (e.g. extensions/v1beta1 → networking/v1) |
| `dhg plugin` | Управление плагинами: `list`, `install`, `remove`, `info` |
| `dhg scan` | Сканирование манифестов на секреты, уязвимости, compliance нарушения |

Команды, добавленные ранее и стабилизированные в v1.0.0:
- `dhg validate` — валидация Chart.yaml, values.yaml, template syntax
- `dhg diff` — unified diff между chart директориями

---

## Новые генераторы

### Phase 2 (стабилизированы в v1.0.0)
- Air-gapped support, Namespace governance, Auto-NetworkPolicy, Multi-tenant overlay
- Feature flags, Cloud annotations, Ingress controller detection, Workload-aware env profiles
- Monorepo layout, Spot/preemptible, Kustomize overlays, Auto dependency detection

### Phase 2.5 — Security Generators (10 генераторов)
- Pod Security Standards (PSS) generator
- RBAC generator (Role/ClusterRole/Binding)
- Secrets management generator (External Secrets, Sealed Secrets)
- Image policy generator (ImagePolicyWebhook, Kyverno)
- TLS generator (cert-manager Certificate/Issuer/ClusterIssuer)
- Audit policy generator (AuditPolicy, AuditSink)
- Admission webhook generator (ValidatingWebhookConfiguration, MutatingWebhookConfiguration)
- Supply chain security generator (cosign, SBOM, policy)
- Network security generator (NetworkPolicy, GlobalNetworkPolicy)
- Compliance generator (CIS, NSA, NIST profiles)

### Phase 3 — Deckhouse-Specific (8 генераторов)
- Deckhouse Module scaffold generator
- InstanceClass generator (AWS, GCP, Azure, vSphere, OpenStack)
- GRPCRoute generator (gateway.networking.k8s.io/v1alpha2)
- TLSRoute generator (gateway.networking.k8s.io/v1alpha2)
- Canary deployment generator (Flagger/Argo Rollouts)
- CRD validation generator (OpenAPI v3 schema)
- Deckhouse compatibility checker generator
- Module documentation generator

### Phase 5.5–5.9 (новые в v1.0.0)
- Smart dependency analyzer
- Cost estimation generator
- Compliance report generator
- Template inheritance engine
- values.schema.json generator
- External Secrets generator (AWS SM, GCP SM, Azure KV, Vault)
- Sealed Secrets generator
- Istio traffic management generator (VirtualService, DestinationRule)
- mTLS enforcement generator
- OpenTelemetry collector generator
- SLO/SLI PrometheusRule generator
- Loki PodLogs generator

### Phase 5.10 (новые в v1.0.0)
- Workload Identity generator (GKE, EKS, AKS)
- GPU/TPU workload generator
- Windows workload generator
- Velero backup schedule generator
- Flux HelmRelease generator

### Phase 6 (новые в v1.0.0)
- Plugin scaffold generator (`dhg plugin new`)
- SBOM manifest generator
- Architecture diagram generator (Mermaid C4)

**Итого: 90+ генераторов**

---

## Новые процессоры

### Процессоры, добавленные в Phase 5.5–5.10 и Phase 6

| Процессор | API Group | Описание |
|-----------|-----------|----------|
| `IstioSidecarProcessor` | `networking.istio.io/v1beta1` | Sidecar injection конфигурация, egress/ingress hosts |
| `VirtualServiceProcessor` | `networking.istio.io/v1beta1` | HTTP/TCP маршрутизация, retries, timeouts |
| `DestinationRuleProcessor` | `networking.istio.io/v1beta1` | Load balancing, circuit breaker, mTLS |
| `PeerAuthenticationProcessor` | `security.istio.io/v1beta1` | mTLS режим на уровне namespace/workload |
| `AuthorizationPolicyProcessor` | `security.istio.io/v1beta1` | RBAC политики Istio |
| `PrometheusAnnotationsProcessor` | `v1/Pod,Service` | Аннотации prometheus.io/scrape, path, port |
| `OpenTelemetryCollectorProcessor` | `opentelemetry.io/v1alpha1` | OTel collector конфигурация |
| `InstrumentationProcessor` | `opentelemetry.io/v1alpha1` | Auto-instrumentation настройки |
| `WorkloadIdentityProcessor` | `v1/ServiceAccount` | GKE/EKS/AKS IAM аннотации |
| `GPUProcessor` | `v1/Pod` | nvidia.com/gpu, amd.com/gpu, google.com/tpu ресурсы |
| `WindowsProcessor` | `v1/Pod` | Windows OS selector, windowsOptions |
| `ExternalSecretProcessor` | `external-secrets.io/v1beta1` | ExternalSecret, SecretStore маппинг |
| `SealedSecretProcessor` | `bitnami.com/v1alpha1` | SealedSecret извлечение |
| `VeleroScheduleProcessor` | `velero.io/v1` | Backup schedule конфигурация |
| `FluxHelmReleaseProcessor` | `helm.toolkit.fluxcd.io/v2beta1` | HelmRelease values, interval, timeout |
| `FluxKustomizationProcessor` | `kustomize.toolkit.fluxcd.io/v1` | Kustomization source, path, prune |

Ранее добавленные и стабилизированные процессоры (всего 50+):
- Deployment, StatefulSet, DaemonSet, Service, Ingress, ConfigMap, Secret, PVC, ServiceAccount
- HPA, PDB, NetworkPolicy, CronJob, Job, Role, ClusterRole, RoleBinding, ClusterRoleBinding
- VPA, PriorityClass, LimitRange, ResourceQuota
- ServiceMonitor, PodMonitor, PrometheusRule, GrafanaDashboard
- HTTPRoute, Gateway, ScaledObject, TriggerAuthentication
- Certificate, ClusterIssuer, Rollout (Argo)
- ModuleConfig, IngressNginxController, ClusterAuthorizationRule, NodeGroup, DexAuthenticator, User, Group

---

## Инфраструктура и CI/CD

### GoReleaser (обновлено в v1.0.0)
- **cosign**: подписание всех бинарных артефактов (`.sig` файлы в GitHub Release)
- **SBOM (Syft)**: автоматическая генерация `sbom-*.json` в форматах SPDX и CycloneDX
- **nfpm**: пакеты для основных Linux дистрибутивов:
  - `.deb` (Debian, Ubuntu)
  - `.rpm` (RHEL, Fedora, openSUSE)
  - `.apk` (Alpine Linux)
- **Docker multi-arch**: образы для `linux/amd64`, `linux/arm64`, `linux/arm/v7`
- **SLSA provenance**: attestation для каждого артефакта

### CI Workflows (GitHub Actions)
| Workflow | Jobs | Описание |
|----------|------|----------|
| `test.yml` | Unit Tests (Go 1.25 + 1.26), Integration, E2E | Matrix build + coverage gate 80% |
| `release.yml` | Build, Sign, SBOM, Docker, Packages | Полный release pipeline |
| `codeql.yml` | CodeQL Analysis | Static security analysis |
| `benchmark.yml` | Benchmarks | Регрессионные тесты производительности |
| `security.yml` | Trivy, gitleaks | Container и secrets scan |
| `auto-approve.yml` | Auto-merge | Автоматический merge owner PRs |
| `dependabot.yml` | Dependency updates | Go modules, Actions, Docker |

**Итого: 7+ CI jobs**

### Branch Protection
Обязательные проверки для merge в `main`:
- Unit Tests (Go 1.26)
- Lint Code
- Security Scan
- Build Binary

---

## Документация

### Новые файлы (Phase 6.5)
| Файл | Описание |
|------|----------|
| `docs/ADR.md` | Architecture Decision Records — 50 записей (ADR-001..ADR-050) |
| `docs/DEVELOPER.md` | Руководство разработчика: setup, testing, contribution, ADR процесс |
| `docs/USER_GUIDE.md` | Руководство пользователя: все команды, режимы, примеры, FAQ |
| `docs/ARCHITECTURE_DIAGRAMS.md` | C4-диаграммы в Mermaid (Context, Container, Component) |

### Обновлённые файлы
- `README.md` — полная переработка с примерами для каждого режима, badges, quickstart
- `CONTRIBUTING.md` — обновлён ADR процесс, test requirements
- `SECURITY.md` — добавлена supply chain security секция

---

## Статистика релиза

| Метрика | Значение |
|---------|----------|
| Тестов всего | **2 372** |
| Покрытие кода | **86%+** |
| Поддерживаемых платформ | **6** (linux/darwin/windows × amd64/arm64) |
| Строк кода | **~35 000** |
| Генераторов | **90+** |
| Процессоров | **50+** |
| ADR записей | **50** |
| Выпущенных версий | **11** (v0.1.0 → v1.0.0) |
| Закрытых issues | **29+** |

### Прирост по фазам
| Фаза | LOC | Тестов | Задач |
|------|-----|--------|-------|
| Phase 1 (v0.1–v0.3) | ~5 000 | 200+ | 15 |
| Phase 2 (v0.4–v0.7) | ~8 000 | 350+ | 20 |
| Phase 2.5–3 (v0.7.x) | ~3 500 | 250+ | 18 |
| Phase 4 (стабы) | ~500 | 30+ | 5 |
| Phase 5.1–5.4 | ~7 367 | 380+ | 30 |
| Phase 5.5–5.9 | **+17 500** | **+380** | 33 |
| Phase 5.10 | **+2 700** | **+55** | 5 |
| Phase 6 | **+8 400** | **+151** | 26 |

---

## Установка

### Homebrew (macOS / Linux)
```bash
brew install AlexGromer/tap/dhg
```

### Бинарный файл (все платформы)
```bash
# Linux amd64
curl -sSL https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/v1.0.0/dhg_linux_amd64.tar.gz | tar xz
sudo mv dhg /usr/local/bin/

# macOS arm64
curl -sSL https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/v1.0.0/dhg_darwin_arm64.tar.gz | tar xz
sudo mv dhg /usr/local/bin/
```

### Docker
```bash
# Последний стабильный
docker pull ghcr.io/alexgromer/dhg:v1.0.0

# Запуск
docker run --rm -v $(pwd):/workspace ghcr.io/alexgromer/dhg:v1.0.0 generate --source /workspace/manifests --output /workspace/chart
```

### go install
```bash
go install github.com/AlexGromer/deckhouse-helm-generator/cmd/dhg@v1.0.0
```

### DEB пакет (Debian/Ubuntu)
```bash
curl -sSL https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/v1.0.0/dhg_linux_amd64.deb -o dhg.deb
sudo dpkg -i dhg.deb
```

### RPM пакет (RHEL/Fedora)
```bash
curl -sSL https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/v1.0.0/dhg_linux_amd64.rpm -o dhg.rpm
sudo rpm -i dhg.rpm
```

### APK пакет (Alpine Linux)
```bash
curl -sSL https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/v1.0.0/dhg_linux_amd64.apk -o dhg.apk
sudo apk add --allow-untrusted dhg.apk
```

### Верификация подписи (cosign)
```bash
cosign verify-blob \
  --certificate dhg_linux_amd64.tar.gz.crt \
  --signature dhg_linux_amd64.tar.gz.sig \
  dhg_linux_amd64.tar.gz
```

---

## Обновление с v0.7.x

### Breaking Changes
- Команда `dhg generate` теперь читает `.dhg.yaml` из текущей директории, если он присутствует
- Флаг `--source` теперь поддерживает несколько путей через запятую: `--source dir1,dir2`
- Вывод команды `dhg validate` изменён для машинной читаемости (добавлен `--json` флаг)

### Migration Guide
1. Если используете `.dhg.yaml` — проверьте новые поля в схеме (см. `dhg config schema`)
2. Если есть CI скрипты с парсингом вывода `dhg validate` — добавьте флаг `--json`
3. Внешние плагины (если были) — необходима перекомпиляция против нового plugin interface

---

## Благодарности

Спасибо всем участникам, использовавшим DHG в production сценариях и предоставившим обратную связь.

Особая благодарность:
- Команде Deckhouse за документацию CRD спецификаций
- Maintainer'ам проектов Istio, External Secrets Operator, OpenTelemetry за отличные API
- Сообществу cert-manager за примеры интеграции

---

## Полный Changelog

Подробная история всех изменений: [CHANGELOG.md](../CHANGELOG.md)

Архив Release Notes по версиям: [docs/](.)

---

_DHG v1.0.0 — Production Release | 2026-03-30_
