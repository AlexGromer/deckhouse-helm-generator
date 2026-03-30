# Deckhouse Helm Generator (DHG)

![Version](https://img.shields.io/badge/version-v1.0.0-brightgreen)
![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)
[![codecov](https://codecov.io/gh/AlexGromer/deckhouse-helm-generator/graph/badge.svg)](https://codecov.io/gh/AlexGromer/deckhouse-helm-generator)
![License](https://img.shields.io/badge/license-Apache--2.0-blue)
![Platforms](https://img.shields.io/badge/platforms-linux%20%7C%20darwin%20%7C%20windows-lightgrey)
![K8s](https://img.shields.io/badge/Kubernetes-1.27--1.32-326CE5?logo=kubernetes&logoColor=white)
![Tests](https://img.shields.io/badge/tests-2372-success)
![Coverage](https://img.shields.io/badge/coverage-86%25+-brightgreen)

CLI-инструмент для автоматической генерации Helm charts из манифестов Kubernetes и Deckhouse. Анализирует связи между ресурсами, поддерживает 90+ генераторов и 50+ процессоров ресурсов, работает с живым кластером, файлами и GitOps-репозиториями.

---

## Содержание

- [Ключевые возможности](#ключевые-возможности)
- [Установка](#установка)
- [Быстрый старт](#быстрый-старт)
- [CLI Reference](#cli-reference)
- [Режимы вывода](#режимы-вывода)
- [Расширенные возможности](#расширенные-возможности)
- [Примеры](#примеры)
- [Структура проекта](#структура-проекта)
- [Статистика](#статистика)
- [Дорожная карта](#дорожная-карта)
- [Участие в разработке](#участие-в-разработке)
- [Лицензия и авторы](#лицензия-и-авторы)

---

## Ключевые возможности

### Извлечение и анализ ресурсов

- Извлечение из YAML-файлов, директорий, живого кластера (client-go) и GitOps-репозиториев (ArgoCD, Flux)
- Интеллектуальный граф связей: LabelSelector, NameReference, VolumeMount, EnvFrom, Annotation, ServiceAccount, ImagePullSecret
- Дедупликация и разрешение конфликтов при объединении нескольких источников
- Рекурсивный обход директорий, фильтрация по namespace, label selector, типу ресурса

### Генерация Helm charts

- 4 режима вывода: `universal`, `separate`, `library`, `umbrella`
- Автоматические `values.yaml`, `_helpers.tpl`, `NOTES.txt`, `.helmignore`, `Chart.yaml`
- JSON Schema (`values.schema.json`) для валидации values
- Environment overlays: `values-dev.yaml`, `values-staging.yaml`, `values-prod.yaml`
- Поддержка Deckhouse Module Scaffold (`helm_lib`, OpenAPI schemas, `images/`, `hooks/`)

### Стандартные Kubernetes ресурсы (22+ процессора)

- Рабочие нагрузки: Deployment, StatefulSet, DaemonSet, Job, CronJob
- Сеть: Service, Ingress, NetworkPolicy
- Конфигурация: ConfigMap, Secret
- Хранилище: PersistentVolumeClaim
- Автомасштабирование: HPA, VPA, KEDA (ScaledObject, TriggerAuthentication)
- Политики: PDB, PriorityClass, LimitRange, ResourceQuota
- RBAC: ServiceAccount, Role, ClusterRole, RoleBinding, ClusterRoleBinding

### Deckhouse CRD (8 процессоров)

- ModuleConfig, IngressNginxController, NodeGroup, DexAuthenticator
- User, Group, ClusterAuthorizationRule, InstanceClass

### Экосистема Kubernetes

- **Мониторинг**: ServiceMonitor, PodMonitor, PrometheusRule, GrafanaDashboard
- **Gateway API**: HTTPRoute, Gateway, GRPCRoute, TLSRoute
- **cert-manager**: Certificate, ClusterIssuer
- **Argo Rollouts / Flagger**: Rollout, Canary (progressive delivery)
- **Service Mesh**: Istio (VirtualService, DestinationRule, AuthorizationPolicy, multi-cluster, egress), Linkerd
- **Secret Management**: ESO, Sealed Secrets, Vault CSI, Vault Agent, Reloader, SOPS
- **Observability**: OpenTelemetry, Prometheus annotations, SLO (Sloth), distributed tracing
- **Cloud-Native**: Workload Identity (IRSA/GKE WI/Azure WI), GPU/TPU, Windows containers, Velero

### Инструменты разработчика

- `dhg analyze` — анализ ресурсов без генерации
- `dhg validate` — валидация через kubeconform, conftest, pluto; матрица K8s 1.27–1.32
- `dhg diff` — сравнение двух chart-версий
- `dhg fix` — автоматическое исправление нарушений best practices
- `dhg graph` — граф зависимостей в формате DOT / Mermaid
- `dhg migrate` — миграция между версиями API
- Плагинная система: `.dhg.yaml`, `--template-dir`, внешние процессоры

---

## Установка

### Homebrew (macOS / Linux)

```bash
brew install AlexGromer/tap/dhg
```

### Бинарный релиз

```bash
# Linux AMD64
VERSION=v1.0.0
curl -LO "https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/${VERSION}/dhg_${VERSION#v}_linux_amd64.tar.gz"
tar xzf "dhg_${VERSION#v}_linux_amd64.tar.gz"
sudo mv dhg /usr/local/bin/

# macOS ARM64
curl -LO "https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/${VERSION}/dhg_${VERSION#v}_darwin_arm64.tar.gz"
tar xzf "dhg_${VERSION#v}_darwin_arm64.tar.gz"
sudo mv dhg /usr/local/bin/

# Windows AMD64
curl -LO "https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/${VERSION}/dhg_${VERSION#v}_windows_amd64.zip"
Expand-Archive "dhg_${VERSION#v}_windows_amd64.zip" -DestinationPath .
```

### go install

```bash
go install github.com/AlexGromer/deckhouse-helm-generator/cmd/dhg@v1.0.0
```

### Docker

```bash
docker pull ghcr.io/alexgromer/dhg:v1.0.0
docker run --rm -v $(pwd):/work ghcr.io/alexgromer/dhg:v1.0.0 \
  generate -f /work/manifests -o /work/chart --chart-name myapp
```

### Пакетные менеджеры (DEB / RPM / APK)

```bash
# Debian / Ubuntu
curl -LO "https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/v1.0.0/dhg_1.0.0_linux_amd64.deb"
sudo dpkg -i dhg_1.0.0_linux_amd64.deb

# RHEL / Fedora / CentOS
curl -LO "https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/v1.0.0/dhg_1.0.0_linux_amd64.rpm"
sudo rpm -i dhg_1.0.0_linux_amd64.rpm

# Alpine Linux
curl -LO "https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/v1.0.0/dhg_1.0.0_linux_amd64.apk"
sudo apk add --allow-untrusted dhg_1.0.0_linux_amd64.apk
```

### Сборка из исходников

```bash
git clone https://github.com/AlexGromer/deckhouse-helm-generator.git
cd deckhouse-helm-generator
make build
sudo cp bin/dhg /usr/local/bin/
```

---

## Быстрый старт

### Из YAML-файлов

```bash
# Universal mode (по умолчанию) — один chart для всех ресурсов
dhg generate -f ./manifests -o ./my-chart --chart-name myapp

# С environment overlays (dev/staging/prod)
dhg generate -f ./manifests -o ./my-chart --chart-name myapp --env-values

# С JSON Schema для values
dhg generate -f ./manifests -o ./my-chart --chart-name myapp --include-schema
```

### Из живого кластера

```bash
dhg generate -s cluster -n production --chart-name prod-app -o ./charts/production \
  --kubeconfig ~/.kube/config --context prod-cluster
```

### Из GitOps-репозитория

```bash
dhg generate -s gitops --repo https://github.com/myorg/k8s-config \
  --path apps/production --chart-name myapp -o ./charts/myapp
```

---

## CLI Reference

### generate

Генерация Helm chart из ресурсов.

```
dhg generate [flags]

Flags:
  -f, --file strings             Пути к YAML-файлам или директориям
  -o, --output string            Директория вывода (default "./chart")
      --chart-name string        Имя chart (обязательно)
      --chart-version string     Версия chart (default "0.1.0")
      --app-version string       Версия приложения (default "1.0.0")
      --mode string              Режим вывода: universal|separate|library|umbrella (default "universal")
      --env-values               Генерировать values-dev/staging/prod.yaml
      --deckhouse-module         Scaffold Deckhouse-модуля (helm_lib, openapi/, images/, hooks/)
  -s, --source string            Источник: file|cluster|gitops (default "file")
  -n, --namespace string         Фильтр по namespace
      --namespaces strings       Фильтр по нескольким namespace
  -l, --selector string          Фильтр по label selector
      --include-kinds strings    Включить только эти типы ресурсов
      --exclude-kinds strings    Исключить эти типы ресурсов
  -r, --recursive                Рекурсивный обход директорий (default true)
      --kubeconfig string        Путь к kubeconfig
      --context string           Контекст kubeconfig
      --include-tests            Генерировать тестовые шаблоны
      --include-readme           Генерировать README.md (default true)
      --include-schema           Генерировать values.schema.json
      --template-dir string      Директория с пользовательскими шаблонами
  -v, --verbose                  Подробный вывод
```

### analyze

Анализ ресурсов и вывод графа связей без генерации chart.

```
dhg analyze [flags]

Flags:
  -f, --file strings    Пути к YAML-файлам или директориям
  -s, --source string   Источник: file|cluster|gitops (default "file")
  -n, --namespace string
      --output-format   Формат: table|json|yaml (default "table")
  -v, --verbose
```

### validate

Валидация Helm chart против схем Kubernetes.

```
dhg validate [flags]

Flags:
      --chart string              Путь к chart (default ".")
      --k8s-version string        Версия Kubernetes (default "1.30")
      --k8s-versions strings      Матрица версий: 1.27,1.28,1.29,1.30,1.31,1.32
      --kubeconform               Запустить kubeconform (default true)
      --conftest                  Запустить conftest OPA policy
      --pluto                     Проверить устаревшие API (pluto)
      --strict                    Строгий режим (fail on warnings)
```

### diff

Сравнение двух версий chart.

```
dhg diff <chart-v1> <chart-v2> [flags]

Flags:
      --output-format   Формат: unified|json|summary (default "unified")
      --values string   Дополнительный values-файл для render
```

### fix

Автоматическое исправление нарушений best practices.

```
dhg fix [flags]

Flags:
  -f, --file strings    Пути к YAML-файлам (in-place fix)
      --chart string    Путь к chart
      --dry-run         Показать изменения без применения
      --rules strings   Список правил: pss,resources,probes,labels,all (default "all")
```

### graph

Генерация графа зависимостей ресурсов.

```
dhg graph [flags]

Flags:
  -f, --file strings       Пути к YAML-файлам
  -s, --source string      Источник: file|cluster (default "file")
  -n, --namespace string
      --format string      Формат: dot|mermaid|json (default "dot")
  -o, --output string      Файл вывода (default stdout)
```

### migrate

Миграция манифестов между версиями Kubernetes API.

```
dhg migrate [flags]

Flags:
  -f, --file strings         Пути к YAML-файлам
      --from-version string  Исходная версия K8s (например, "1.25")
      --to-version string    Целевая версия K8s (например, "1.30")
      --dry-run              Показать изменения без применения
  -o, --output string        Директория для результатов
```

### version

```
dhg version
```

---

## Режимы вывода

| Режим | Описание | Когда использовать |
|-------|----------|--------------------|
| `universal` | Один chart для всех сервисов | Монолитное приложение, простая структура |
| `separate` | Отдельный chart на каждый сервис | Независимые деплои, разные версии релизов |
| `library` | Библиотечный chart + wrapper charts | DRY-шаблоны, максимальное переиспользование |
| `umbrella` | Родительский chart + subcharts | Helmfile-стиль, условное включение сервисов |

### Universal (по умолчанию)

```bash
dhg generate -f ./manifests -o ./my-chart --chart-name myapp
```

Все сервисы в одном `values.yaml`:

```yaml
services:
  frontend:
    enabled: true
    replicaCount: 1
    image:
      repository: nginx
      tag: "1.27"
  backend:
    enabled: true
    replicaCount: 2
```

### Separate

```bash
dhg generate -f ./manifests -o ./charts --chart-name myapp --mode separate
```

```
charts/
├── frontend/
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
└── backend/
    ├── Chart.yaml
    ├── values.yaml
    └── templates/
```

### Library

```bash
dhg generate -f ./manifests -o ./charts --chart-name myapp --mode library
```

```
charts/
├── myapp/               # type: library
│   ├── Chart.yaml
│   └── templates/
│       ├── _deployment.tpl
│       ├── _service.tpl
│       └── _helpers.tpl
├── frontend/            # вызывает шаблоны library
│   ├── Chart.yaml       # зависимость на myapp
│   └── values.yaml
└── backend/
    ├── Chart.yaml
    └── values.yaml
```

### Umbrella

```bash
dhg generate -f ./manifests -o ./charts --chart-name myapp --mode umbrella
```

```
charts/
└── myapp/
    ├── Chart.yaml       # dependencies: [frontend, backend, database]
    ├── values.yaml
    └── charts/
        ├── frontend/
        ├── backend/
        └── database/
```

```bash
# Деплой без database
helm upgrade --install myapp ./charts/myapp --set database.enabled=false
```

---

## Расширенные возможности

### Environment Overlays

```bash
dhg generate -f ./manifests -o ./my-chart --chart-name myapp --env-values
```

Создаёт три файла с профилями для каждого окружения:

- `values-dev.yaml` — `replicaCount: 1`, `logLevel: debug`, PDB отключён
- `values-staging.yaml` — `replicaCount: 2`, `logLevel: info`, PDB `minAvailable: 1`
- `values-prod.yaml` — `replicaCount: 3`, `logLevel: warn`, PDB `minAvailable: 2`, resource limits, anti-affinity

```bash
helm upgrade --install myapp ./my-chart -f ./my-chart/values-prod.yaml
```

### Security (PSS, RBAC, Resource Limits)

DHG анализирует безопасность ресурсов и генерирует рекомендации:

- **Pod Security Standards (PSS)**: проверка `securityContext`, `privileged`, `hostNetwork`, `hostPID`
- **RBAC least privilege**: анализ Role/ClusterRole на избыточные права, генерация минимального набора
- **Resource limits**: обнаружение отсутствующих `resources.limits`, автогенерация значений на основе requests
- **Image security**: проверка `imagePullPolicy`, отсутствие `latest`-тегов, digest-pinning
- **TLS**: проверка наличия Certificate/ClusterIssuer для Ingress с HTTPS

```bash
dhg fix -f ./manifests --rules pss,resources --dry-run
```

### Secret Management

Поддержка всех основных подходов к хранению секретов:

| Инструмент | Описание |
|------------|----------|
| **External Secrets Operator** | ExternalSecret + SecretStore/ClusterSecretStore (AWS, GCP, Vault, Azure) |
| **Sealed Secrets** | SealedSecret (`bitnami.com/sealed-secrets`) с шифрованием публичным ключом |
| **Vault CSI Provider** | SecretProviderClass с монтированием через CSI volume |
| **Vault Agent Injector** | Аннотации `vault.hashicorp.com/agent-inject-secret-*` в pod template |
| **Reloader** | Rolling restart при изменении ConfigMap/Secret |
| **SOPS** | `.sops.yaml` + шифрование age/GPG/KMS |

### Service Mesh (Istio / Linkerd)

```bash
dhg generate -f ./manifests --chart-name webapp -o ./webapp-chart
```

**Istio:**

- VirtualService / DestinationRule: traffic splitting, retries, timeouts, circuit breaker
- Canary: прогрессивный сдвиг трафика 10% → 50% → 100% с автоматическим rollback
- AuthorizationPolicy: `ALLOW`/`DENY` правила по JWT, namespace, source principal
- Multi-cluster: ServiceEntry + WorkloadEntry для cross-cluster service discovery
- Egress: EgressGateway + ServiceEntry для управления исходящим трафиком

**Linkerd:**

- Аннотации `linkerd.io/inject: enabled`
- ServiceProfile для traffic metrics и retries
- TrafficSplit для canary deployments

### Observability

- **OpenTelemetry**: OTelCollector, Instrumentation CR, auto-instrumentation аннотации
- **Prometheus**: автоинъекция аннотаций `prometheus.io/scrape`, `port`, `path`
- **SLO (Sloth)**: `PrometheusServiceLevel` с burn-rate alerts (page / ticket)
- **Distributed Tracing**: Jaeger / Zipkin / Tempo через OTEL Collector pipeline
- **Recording Rules**: вычисленные метрики и ratio-функции

### Cloud-Native Patterns

- **Workload Identity**: IRSA (AWS), GKE Workload Identity, Azure Workload Identity
- **GPU/TPU**: автообнаружение `nvidia.com/gpu`, `cloud-tpu`, генерация resource requests
- **Windows containers**: nodeSelector `kubernetes.io/os: windows`, tolerations
- **Velero**: аннотации backup для PVC, pre/post хуки

### Валидация

```bash
# Валидация против K8s 1.30
dhg validate --chart ./my-chart --k8s-version 1.30

# Матрица версий (CI/CD)
dhg validate --chart ./my-chart --k8s-versions 1.27,1.28,1.29,1.30,1.31,1.32

# Полная проверка с OPA policies
dhg validate --chart ./my-chart --kubeconform --conftest --pluto
```

### Плагинная система

Конфигурационный файл `.dhg.yaml` в корне проекта:

```yaml
# .dhg.yaml
version: "1.0"
processors:
  external:
    - name: my-processor
      cmd: ./scripts/my-processor
      kinds: ["MyCustomResource.mygroup.io/v1"]
template_dir: ./templates/custom
hooks:
  pre_generate: ./scripts/pre-generate.sh
  post_generate: ./scripts/post-generate.sh
```

```bash
# Использование пользовательских шаблонов
dhg generate -f ./manifests -o ./chart --chart-name myapp \
  --template-dir ./templates/custom
```

---

## Примеры

### Пример 1: Простой веб-сервис

```bash
dhg generate -f ./k8s -o ./nginx-chart --chart-name nginx --verbose
```

Входные ресурсы: `Deployment` + `Service` + `ConfigMap`. Результат:

```
nginx-chart/
├── Chart.yaml
├── values.yaml
├── .helmignore
└── templates/
    ├── _helpers.tpl
    ├── NOTES.txt
    ├── nginx-deployment.yaml
    ├── nginx-service.yaml
    └── nginx-configmap-nginx-config.yaml
```

Сгенерированный `values.yaml`:

```yaml
global:
  imageRegistry: ""
  imagePullSecrets: []

services:
  nginx:
    enabled: true
    deployment:
      replicas: 2
      containers:
        - name: nginx
          image:
            repository: nginx
            tag: "1.25"
          ports:
            - containerPort: 80
    service:
      type: ClusterIP
      ports:
        - port: 80
          targetPort: 80
    configMaps:
      nginx-config:
        enabled: true
```

### Пример 2: Production-окружение с security

```bash
dhg generate -f ./manifests --chart-name webapp \
  --include-kinds Deployment,Service,Ingress,Certificate,ServiceMonitor,PrometheusRule \
  --env-values --include-schema \
  -o ./webapp-chart
```

Дополнительно сгенерирует:

- `values.schema.json` — JSON Schema для `helm lint` и IDE-подсказок
- `values-prod.yaml` — resource limits, anti-affinity, PDB `minAvailable: 2`
- Зависимости: `Certificate` → `ClusterIssuer`, `ServiceMonitor` → `Service`

### Пример 3: Deckhouse-модуль

```bash
dhg generate -f ./manifests -o ./my-module \
  --chart-name ingress-nginx --deckhouse-module --verbose
```

Структура результата:

```
my-module/
├── Chart.yaml              # dependency: helm_lib "*"
├── values.yaml
├── openapi/
│   ├── config-values.yaml  # публичные настройки (OpenAPI schema)
│   └── values.yaml         # internal values schema
├── images/                 # Dockerfile для образов модуля
├── hooks/                  # Go/Shell хуки
└── templates/
    ├── _helpers.tpl         # helm_lib_module_labels, helm_lib_module_image
    └── ...
```

---

## Структура проекта

```
.
├── cmd/
│   └── dhg/                  # Точка входа CLI (cobra)
├── pkg/
│   ├── extractor/            # Извлечение ресурсов (file, cluster, gitops)
│   ├── analyzer/             # Анализ графа связей
│   ├── processor/            # 50+ процессоров ресурсов
│   │   └── k8s/              # Все процессоры по группам (core, deckhouse,
│   │                         # monitoring, gateway, keda, certmanager,
│   │                         # argo, istio, linkerd, secrets, otel, cloud)
│   ├── generator/            # Генерация charts (90+ генераторов)
│   │   ├── arch/             # Архитектурные генераторы (12)
│   │   ├── security/         # Генераторы безопасности (10)
│   │   └── cloud/            # Cloud-native генераторы
│   ├── helm/                 # Утилиты Helm (render, lint, diff)
│   ├── fix/                  # Auto-Fix Engine
│   ├── graph/                # DOT/Mermaid граф зависимостей
│   ├── migrate/              # Миграция API версий
│   ├── validate/             # Валидация (kubeconform, conftest, pluto)
│   └── types/                # Общие типы и интерфейсы
├── tests/
│   ├── integration/          # Интеграционные тесты
│   └── e2e/                  # End-to-end тесты
├── testdata/                 # Тестовые YAML-манифесты
├── Makefile
└── README.md
```

---

## Статистика

| Показатель | Значение |
|------------|----------|
| Генераторы | 90+ |
| Процессоры ресурсов | 50+ |
| Тесты | 2372 |
| Покрытие кода | 86%+ |
| Платформы | 6 (linux/darwin/windows × amd64/arm64) |
| Поддержка K8s | 1.27 – 1.32 |
| ADR (Architecture Decision Records) | 50 |
| Строк кода | ~35 000 |
| Фазы разработки | Phase 1–6 (100% выполнено) |

---

## Дорожная карта

### Выполнено (Phase 1–6)

| Фаза | Описание | Статус |
|------|----------|--------|
| Phase 1 | Core pipeline, 50+ процессоров, CLI (`validate`, `diff`) | ✅ 100% |
| Phase 2 | 12 архитектурных генераторов (infrastructure, detection, advanced) | ✅ 100% |
| Phase 2.5 | 10 генераторов безопасности (PSS, RBAC, resource limits, image, TLS, audit, admission, supply chain) | ✅ 100% |
| Phase 3 | Deckhouse CRD (InstanceClass, GRPCRoute, TLSRoute, Canary), module scaffold, compatibility | ✅ 100% |
| Phase 4 | Cluster Extractor (client-go), GitOps Extractor (ArgoCD/Flux), Multi-Source Merge | ✅ 100% |
| Phase 5.1–5.4 | Auto-Fix Engine (`dhg fix`), Generic CRD, DOT Graph (`dhg graph`), Migration (`dhg migrate`) | ✅ 100% |
| Phase 5.5 | Smart Analysis: cost estimation, right-sizing, PV best practices, compliance-as-code, policy-as-code | ✅ 100% |
| Phase 5.6 | Advanced Templating: Kustomize post-renderer, Operator scaffold | ✅ 100% |
| Phase 5.7 | Secret Management: ESO, Sealed Secrets, Vault CSI, Vault Agent, Reloader, SOPS | ✅ 100% |
| Phase 5.8 | Service Mesh: Istio (traffic, canary, AuthzPolicy, multi-cluster, egress), Linkerd | ✅ 100% |
| Phase 5.9 | Observability: OpenTelemetry, Prometheus annotations, SLO (Sloth), distributed tracing, recording rules | ✅ 100% |
| Phase 5.10 | Cloud-Native: Workload Identity, GPU/TPU, Windows, Velero, Flux postBuild | ✅ 100% |

### Планируется (Phase 7–13)

| Фаза | Направление | Описание |
|------|-------------|----------|
| Phase 7 | Performance | Параллельная обработка (goroutines), memory optimization, benchmarks |
| Phase 8 | Database Operators | CloudNativePG, Percona, Redis Enterprise, ClickHouse |
| Phase 9 | AI/ML Workloads | Kubeflow, KServe, GPU scheduling, distributed training |
| Phase 10 | Multi-Cluster | Federation, fleet management, cross-cluster networking |
| Phase 11 | IDE Integration | LSP server, VS Code extension, schema autocomplete |
| Phase 12 | SaaS / Web UI | Веб-интерфейс для генерации и визуализации charts |
| Phase 13 | Plugin Registry | Реестр пользовательских процессоров и шаблонов |

---

## Участие в разработке

### Требования

- Go 1.26+
- make
- (опционально) Helm 3.x для тестирования результатов

### Сборка и тестирование

```bash
# Сборка
make build

# Тесты
make test

# Lint
make lint

# Сборка для всех платформ
make build-all
```

### Добавление нового процессора

```go
package k8s

import (
    "github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

type MyResourceProcessor struct {
    processor.BaseProcessor
}

func NewMyResourceProcessor() *MyResourceProcessor {
    return &MyResourceProcessor{
        BaseProcessor: processor.NewBaseProcessor(
            "myresource",
            100, // priority
            schema.GroupVersionKind{Group: "my.group", Version: "v1", Kind: "MyResource"},
        ),
    }
}

func (p *MyResourceProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
    return &processor.Result{
        Processed:       true,
        ServiceName:     "myservice",
        TemplatePath:    "templates/myresource.yaml",
        TemplateContent: generateTemplate(obj),
        Values:          extractValues(obj),
    }, nil
}
```

Зарегистрируйте процессор в `pkg/processor/k8s/registry.go`:

```go
func RegisterAll(r *processor.Registry) {
    // ...
    r.Register(NewMyResourceProcessor())
}
```

### Процесс

1. Сделайте fork репозитория
2. Создайте feature-ветку: `git checkout -b feature/amazing-feature`
3. Напишите тесты для новой функциональности
4. Зафиксируйте изменения: `git commit -m 'feat: add amazing feature'`
5. Отправьте ветку: `git push origin feature/amazing-feature`
6. Откройте Pull Request

---

## Лицензия и авторы

Apache License 2.0 — см. [LICENSE](LICENSE).

**Alex Gromer** — System Architect, End-to-End Engineer

- DevOps/Infrastructure: Deckhouse (K8s), Astra Linux, Java Spring microservices
- Systems programming: Go, Rust, C
- [GitHub](https://github.com/AlexGromer)

---

## Ссылки

- [Документация Deckhouse](https://deckhouse.io/documentation/)
- [Документация Helm](https://helm.sh/docs/)
- [Справочник Kubernetes API](https://kubernetes.io/docs/reference/kubernetes-api/)
- [Gateway API](https://gateway-api.sigs.k8s.io/)
- [OpenTelemetry](https://opentelemetry.io/docs/)
