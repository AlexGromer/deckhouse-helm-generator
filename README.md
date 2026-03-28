# Deckhouse Helm Generator (DHG)

![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)
[![codecov](https://codecov.io/gh/AlexGromer/deckhouse-helm-generator/graph/badge.svg)](https://codecov.io/gh/AlexGromer/deckhouse-helm-generator)
![License](https://img.shields.io/badge/license-Apache--2.0-blue)

CLI-инструмент для генерации Helm charts из ресурсов Kubernetes/Deckhouse с автоматическим обнаружением связей между ресурсами.

## Возможности

- 📦 **Автоматическое извлечение ресурсов** из YAML файлов, кластера Kubernetes или GitOps репозиториев
- 🔍 **Интеллектуальное обнаружение связей** между ресурсами (Service → Deployment, Ingress → Service, Volume mounts и т.д.)
- 🎯 **Группировка ресурсов** в логические сервисы на основе labels и dependencies
- 📝 **Генерация готовых Helm charts** с values.yaml, templates и _helpers.tpl
- 🔧 **Поддержка Deckhouse CRDs** (ModuleConfig, IngressNginxController, NodeGroup, DexAuthenticator, User, Group, ClusterAuthorizationRule)
- 🏗️ **Deckhouse Module Scaffold** (`--deckhouse-module`): helm_lib dependency, OpenAPI schemas, images/ и hooks/ directories
- 📊 **Monitoring Stack**: ServiceMonitor, PodMonitor, PrometheusRule, GrafanaDashboard (Prometheus Operator)
- 🌐 **Modern K8s**: Gateway API (HTTPRoute, Gateway), KEDA (ScaledObject, TriggerAuthentication), cert-manager (Certificate, ClusterIssuer), Argo Rollouts
- 🎨 **4 режима вывода**: Universal (один chart), Separate (chart на сервис), Library (DRY-шаблоны), Umbrella (родительский chart + subcharts)
- 🌍 **Environment-specific values**: автогенерация `values-dev.yaml`, `values-staging.yaml`, `values-prod.yaml` с профилями для каждой среды

## Установка

### Из исходников

```bash
git clone https://github.com/AlexGromer/deckhouse-helm-generator.git
cd deckhouse-helm-generator
make build
sudo cp bin/dhg /usr/local/bin/
```

### Из бинарных релизов

```bash
# Linux AMD64
VERSION=$(curl -s https://api.github.com/repos/AlexGromer/deckhouse-helm-generator/releases/latest | grep tag_name | cut -d '"' -f4)
curl -LO "https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/${VERSION}/dhg_${VERSION#v}_linux_amd64.tar.gz"
tar xzf "dhg_${VERSION#v}_linux_amd64.tar.gz"
sudo mv dhg /usr/local/bin/

# macOS ARM64
curl -LO "https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/${VERSION}/dhg_${VERSION#v}_darwin_arm64.tar.gz"
tar xzf "dhg_${VERSION#v}_darwin_arm64.tar.gz"
sudo mv dhg /usr/local/bin/
```

### Docker

```bash
docker pull ghcr.io/alexgromer/dhg:latest
docker run --rm -v $(pwd):/work ghcr.io/alexgromer/dhg generate -f /work/manifests -o /work/chart --chart-name myapp
```

### Homebrew (macOS/Linux)

```bash
brew install AlexGromer/tap/dhg
```

## Быстрый старт

### Генерация chart из YAML файлов

```bash
# Universal mode (по умолчанию) — один chart
dhg generate -f ./manifests -o ./my-chart --chart-name myapp

# Separate mode — отдельный chart на каждый сервис
dhg generate -f ./manifests -o ./charts --chart-name myapp --mode separate

# Library mode — DRY-шаблоны + wrapper charts
dhg generate -f ./manifests -o ./charts --chart-name myapp --mode library

# Umbrella mode — родительский chart + subcharts
dhg generate -f ./manifests -o ./charts --chart-name myapp --mode umbrella

# С environment-specific values (dev/staging/prod)
dhg generate -f ./manifests -o ./my-chart --chart-name myapp --env-values

# С verbose выводом
dhg generate -f ./manifests -o ./my-chart --chart-name myapp --verbose
```

### Генерация из live кластера

```bash
# Из конкретного namespace
dhg generate -s cluster -n production --chart-name prod-app -o ./charts/production

# С kubeconfig
dhg generate -s cluster --kubeconfig ~/.kube/config --context prod-cluster \
  -n production --chart-name prod-app -o ./charts/production
```

### Фильтрация ресурсов

```bash
# Только определенные типы ресурсов
dhg generate -f ./manifests --include-kinds Deployment,Service,Ingress \
  --chart-name frontend -o ./frontend-chart

# Исключить определенные типы
dhg generate -f ./manifests --exclude-kinds Secret,ConfigMap \
  --chart-name app -o ./app-chart

# По label selector
dhg generate -s cluster -n default -l app=nginx \
  --chart-name nginx -o ./nginx-chart
```

## Примеры

### Пример 1: Простой веб-сервис

Исходные файлы:

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  labels:
    app.kubernetes.io/name: nginx
spec:
  replicas: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: nginx
  template:
    metadata:
      labels:
        app.kubernetes.io/name: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.25
        ports:
        - containerPort: 80
        volumeMounts:
        - name: config
          mountPath: /etc/nginx/conf.d
      volumes:
      - name: config
        configMap:
          name: nginx-config

---
# service.yaml
apiVersion: v1
kind: Service
metadata:
  name: nginx
spec:
  selector:
    app.kubernetes.io/name: nginx
  ports:
  - port: 80
    targetPort: 80

---
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx-config
data:
  default.conf: |
    server {
        listen 80;
        location / {
            root /usr/share/nginx/html;
        }
    }
```

Команда генерации:

```bash
dhg generate -f ./k8s -o ./nginx-chart --chart-name nginx --verbose
```

Результат:

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
        volumeMounts:
        - name: config
          mountPath: /etc/nginx/conf.d
      volumes:
      - name: config
        configMap:
          name: nginx-config
    service:
      type: ClusterIP
      ports:
      - port: 80
        targetPort: 80
    configMaps:
      nginx-config:
        enabled: true
        data:
          default.conf: |
            server {
                listen 80;
                location / {
                    root /usr/share/nginx/html;
                }
            }
```

### Пример 2: Полный стек с Ingress и cert-manager

```bash
dhg generate -f ./full-stack --chart-name webapp \
  --include-kinds Deployment,Service,Ingress,ConfigMap,Secret,Certificate \
  -o ./webapp-chart --include-schema
```

## Архитектура

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Кластер   │    │    Файлы    │    │   GitOps    │
│ (client-go) │    │   (YAML)    │    │    (git)    │
└──────┬──────┘    └──────┬──────┘    └──────┬──────┘
       └──────────────────┼──────────────────┘
                          ▼
              ┌───────────────────────┐
              │     Экстрактор        │
              │  (Unstructured API)   │
              └───────────┬───────────┘
                          ▼
              ┌───────────────────────┐
              │      Анализатор       │
              │    (Граф связей)      │
              └───────────┬───────────┘
                          ▼
              ┌───────────────────────┐
              │      Процессоры       │
              │   (GVK → Template)    │
              └───────────┬───────────┘
                          ▼
              ┌───────────────────────┐
              │      Генератор        │
              │  (Chart + Values)     │
              └───────────────────────┘
```

## Поддерживаемые ресурсы (45+ процессоров)

### Стандартные Kubernetes (22 процессора)

- ✅ **Основные рабочие нагрузки**: Deployment, StatefulSet, DaemonSet
- ✅ **Сервисы и сеть**: Service, Ingress, NetworkPolicy
- ✅ **Конфигурация**: ConfigMap, Secret
- ✅ **Хранилище**: PersistentVolumeClaim
- ✅ **Автомасштабирование**: HorizontalPodAutoscaler (HPA), VPA
- ✅ **Бюджет прерываний**: PodDisruptionBudget (PDB)
- ✅ **Пакетные задачи**: CronJob, Job
- ✅ **RBAC и идентификация**: ServiceAccount, Role, ClusterRole, RoleBinding, ClusterRoleBinding
- ✅ **Политики**: PriorityClass, LimitRange, ResourceQuota

### Deckhouse CRD (8 процессоров)

- ✅ **ModuleConfig** (`deckhouse.io/v1alpha1`): настройки модулей с version tracking
- ✅ **IngressNginxController** (`deckhouse.io/v1`): inlet, hostPort/hostWithFailover
- ✅ **ClusterAuthorizationRule** (`deckhouse.io/v1`): subjects, accessLevel
- ✅ **NodeGroup** (`deckhouse.io/v1`): nodeType, disruptions, cloudInstances
- ✅ **DexAuthenticator** (`deckhouse.io/v1`): applicationDomain, разрешённые группы
- ✅ **User** (`deckhouse.io/v1`): email, groups, ttl
- ✅ **Group** (`deckhouse.io/v1`): список участников
- ✅ **InstanceClass** (`deckhouse.io/v1`): cloud instance specifications

### Мониторинг (4 процессора)

- ✅ **ServiceMonitor** (`monitoring.coreos.com/v1`): endpoints, namespaceSelector, зависимость от Service
- ✅ **PodMonitor** (`monitoring.coreos.com/v1`): podMetricsEndpoints, jobLabel
- ✅ **PrometheusRule** (`monitoring.coreos.com/v1`): группы правил alert/record
- ✅ **GrafanaDashboard**: ConfigMap с label `grafana_dashboard: "1"`

### Gateway API (4 процессора)

- ✅ **HTTPRoute** (`gateway.networking.k8s.io/v1`): parentRefs, hostnames, rules
- ✅ **Gateway** (`gateway.networking.k8s.io/v1`): gatewayClassName, listeners, TLS
- ✅ **GRPCRoute** (`gateway.networking.k8s.io/v1`): gRPC backend routing
- ✅ **TLSRoute** (`gateway.networking.k8s.io/v1alpha2`): TLS passthrough routing

### KEDA (2 процессора)

- ✅ **ScaledObject** (`keda.sh/v1alpha1`): scaleTargetRef, triggers, обнаружение scale-to-zero
- ✅ **TriggerAuthentication** (`keda.sh/v1alpha1`): secretTargetRef, env, podIdentity

### cert-manager (2 процессора)

- ✅ **Certificate** (`cert-manager.io/v1`): dnsNames, issuerRef, secretName
- ✅ **ClusterIssuer** (`cert-manager.io/v1`): ACME, selfSigned, CA

### Argo Rollouts (2 процессора)

- ✅ **Rollout** (`argoproj.io/v1alpha1`): стратегии canary/blueGreen
- ✅ **Canary** (`flagger.app/v1beta1`): progressive delivery с Flagger

## Интеграция с Deckhouse

DHG нативно поддерживает Deckhouse CRDs и может генерировать полноценную структуру модуля Deckhouse:

```bash
# Генерация Deckhouse модуля из существующих ресурсов
dhg generate -f ./manifests -o ./my-module --chart-name ingress-nginx --deckhouse-module
```

Флаг `--deckhouse-module` добавляет:
- **`Chart.yaml`**: dependency на `helm_lib` (`version: "*"`)
- **`openapi/config-values.yaml`**: OpenAPI schema для публичных настроек
- **`openapi/values.yaml`**: internal values schema
- **`images/`**: директория для Dockerfile
- **`hooks/`**: директория для Go/Shell hooks
- **Templates**: автоматическая инъекция `helm_lib_module_labels`, `helm_lib_module_image`

Автодетекция: если во входных ресурсах есть CRDs с group `deckhouse.io`, DHG автоматически распознаёт Deckhouse-контекст.

## Стек мониторинга

Полная поддержка Prometheus Operator + Grafana dashboards:

```bash
dhg generate -f ./manifests --include-kinds Deployment,Service,ServiceMonitor,PrometheusRule \
  --chart-name myapp -o ./myapp-chart
```

- **ServiceMonitor** → автоматическая зависимость от Service (через selector)
- **PrometheusRule** → правила alert/record с шаблонизацией пороговых значений
- **GrafanaDashboard** → ConfigMap с label `grafana_dashboard: "1"` автоматически распознаётся (priority 110)
- **PodMonitor** → для метрик уровня Pod без Service

## Современные паттерны K8s

### Gateway API

Замена Ingress для продвинутой маршрутизации:

```bash
dhg generate -f ./gateway-manifests --chart-name webapp -o ./webapp-chart
```

- **Gateway** → `gatewayClassName`, listeners (HTTP/HTTPS/TLS)
- **HTTPRoute** → parentRefs, hostnames, path-based routing с зависимостью от Gateway

### KEDA

Событийно-управляемое автомасштабирование:

- **ScaledObject** → `scaleTargetRef` → автоматическая зависимость от Deployment/StatefulSet
- Scale-to-zero: `minReplicaCount: 0` → флаг в metadata
- **TriggerAuthentication** → секретные ключи для trigger-ов

### cert-manager

- **Certificate** → dnsNames, issuerRef, secretName
- **ClusterIssuer** → ACME (Let's Encrypt), selfSigned, CA
- Ingress с аннотацией `cert-manager.io/cluster-issuer` → зависимость от ClusterIssuer

### Argo Rollouts

- **Rollout** → стратегии canary/blueGreen
- Сохранение pod template для progressive delivery

### ExternalDNS и TopologySpread

- **ExternalDNS**: аннотация `external-dns.alpha.kubernetes.io/hostname` на Service/Ingress → metadata
- **TopologySpreadConstraints**: автоизвлечение из pod spec Deployment

## Обнаружение связей

DHG автоматически обнаруживает следующие типы связей:

| Тип | Описание | Пример |
|-----|----------|--------|
| **LabelSelector** | Селектор по labels | Service → Deployment (через spec.selector) |
| **NameReference** | Прямая ссылка по имени | Ingress → Service (через backend.service.name) |
| **VolumeMount** | Монтирование тома | Deployment → ConfigMap/Secret |
| **EnvFrom** | Переменные окружения | Deployment → ConfigMap/Secret (через envFrom) |
| **EnvValueFrom** | Отдельная переменная окружения | Deployment → ConfigMap/Secret (через valueFrom) |
| **Annotation** | Аннотации | Ingress → ClusterIssuer (cert-manager) |
| **ServiceAccount** | Сервисный аккаунт | Deployment → ServiceAccount |
| **ImagePullSecret** | Секреты для загрузки образов | Deployment → Secret |

## Режимы вывода

| Режим | Описание | Когда использовать |
|-------|----------|-------------------|
| `universal` | Один chart для всех сервисов | Монолитное приложение, простая структура |
| `separate` | Отдельный chart на каждый сервис | Независимые деплои, разные версии релизов |
| `library` | Библиотечный chart + тонкие wrapper-charts | DRY-шаблоны, максимальное переиспользование |
| `umbrella` | Родительский chart + subcharts | Helmfile-стиль, условное включение сервисов |

### Universal (по умолчанию)

Один chart, все сервисы в `values.yaml`:

```bash
dhg generate -f ./manifests -o ./my-chart --chart-name myapp
# или явно:
dhg generate -f ./manifests -o ./my-chart --chart-name myapp --mode universal
```

```yaml
# values.yaml
services:
  frontend:
    enabled: true
    replicaCount: 1
    image: {repository: nginx, tag: latest}
  backend:
    enabled: true
    replicaCount: 2
```

### Separate

Отдельный chart для каждого сервиса:

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

Библиотечный chart с именованными шаблонами + тонкие wrapper charts для каждого сервиса:

```bash
dhg generate -f ./manifests -o ./charts --chart-name myapp --mode library
```

```
charts/
├── myapp/               # library chart (type: library)
│   ├── Chart.yaml
│   └── templates/
│       ├── _deployment.tpl
│       ├── _service.tpl
│       └── ...
├── frontend/            # wrapper chart (вызывает library templates)
│   ├── Chart.yaml       # зависимость на myapp library
│   ├── values.yaml
│   └── templates/
└── backend/
    ├── Chart.yaml
    ├── values.yaml
    └── templates/
```

### Umbrella

Родительский chart + subcharts в `charts/` директории. Позволяет условно включать/выключать сервисы через `--set <name>.enabled=false`:

```bash
dhg generate -f ./manifests -o ./charts --chart-name myapp --mode umbrella
```

```
charts/
└── myapp/               # родительский umbrella chart
    ├── Chart.yaml       # dependencies: [frontend, backend, database]
    ├── values.yaml      # frontend.enabled: true, backend.enabled: true
    └── charts/
        ├── frontend/    # subchart
        ├── backend/     # subchart
        └── database/    # subchart
```

```bash
# Деплой без database
helm upgrade --install myapp ./charts/myapp --set database.enabled=false
```

## Значения для разных окружений

Флаг `--env-values` генерирует три файла с override-значениями для разных сред:

```bash
dhg generate -f ./manifests -o ./my-chart --chart-name myapp --env-values
```

Создаёт:
- `values-dev.yaml` — расслабленные настройки: `replicaCount: 1`, `logLevel: debug`, без PDB
- `values-staging.yaml` — промежуточные: `replicaCount: 2`, `logLevel: info`, PDB с `minAvailable: 1`
- `values-prod.yaml` — production-ready: `replicaCount: 3`, `logLevel: warn`, PDB `minAvailable: 2`, resource limits, anti-affinity

```bash
# Применить dev профиль
helm upgrade --install myapp ./my-chart -f ./my-chart/values-dev.yaml

# Применить prod профиль
helm upgrade --install myapp ./my-chart -f ./my-chart/values-prod.yaml
```

Файлы содержат только **переопределения** -- не копию всех значений базового chart.

## Параметры CLI

### Команда generate

```
Flags:
  -f, --file strings            Path(s) to YAML files or directories
  -o, --output string           Output directory (default "./chart")
      --chart-name string       Chart name (required)
      --chart-version string    Chart version (default "0.1.0")
      --app-version string      App version (default "1.0.0")
      --mode string             Output mode: universal|separate|library|umbrella (default "universal")
      --env-values              Generate environment-specific value files (dev/staging/prod)
      --deckhouse-module        Generate Deckhouse module scaffold (helm_lib, openapi/, images/, hooks/)
  -s, --source string           Source: file|cluster|gitops (default "file")
  -n, --namespace string        Filter by namespace
      --namespaces strings      Filter by multiple namespaces
  -l, --selector string         Label selector filter
      --include-kinds strings   Include only these kinds
      --exclude-kinds strings   Exclude these kinds
  -r, --recursive               Recursive directory scan (default true)
      --kubeconfig string       Kubeconfig path
      --context string          Kubeconfig context
      --include-tests           Generate test templates
      --include-readme          Generate README.md (default true)
      --include-schema          Generate values.schema.json
  -v, --verbose                 Verbose output
```

## Разработка

### Требования

- Go 1.26+
- make
- (опционально) Helm 3.x для тестирования результатов

### Сборка

```bash
# Сборка бинарника
make build

# Запуск тестов
make test

# Lint
make lint

# Сборка для всех платформ
make build-all
```

### Структура проекта

```
.
├── cmd/dhg/              # Точка входа CLI
├── pkg/
│   ├── extractor/        # Извлечение ресурсов
│   ├── analyzer/         # Анализ связей
│   ├── processor/        # Обработка ресурсов (45+ процессоров)
│   │   └── k8s/          # K8s + Deckhouse + Monitoring + Gateway + KEDA + cert-manager + Argo
│   ├── generator/        # Генерация charts
│   ├── helm/             # Утилиты Helm
│   └── types/            # Общие типы
├── testdata/             # Тестовые данные
├── Makefile
└── README.md
```

### Добавление нового процессора ресурсов

```go
package k8s

import (
    "github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
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
    // Логика обработки ресурса
    return &processor.Result{
        Processed:       true,
        ServiceName:     "myservice",
        TemplatePath:    "templates/myresource.yaml",
        TemplateContent: generateTemplate(obj),
        Values:          extractValues(obj),
    }, nil
}
```

Затем зарегистрируйте процессор в `pkg/processor/k8s/registry.go`:

```go
func RegisterAll(r *processor.Registry) {
    // ...
    r.Register(NewMyResourceProcessor())
}
```

## Ограничения и известные проблемы

- Cluster Extractor (извлечение из live кластера) ещё не реализован
- GitOps Extractor ещё не реализован

## Дорожная карта

### Выполнено
- **Phase 1**: Core pipeline, 45+ процессоров, pattern detectors, CLI (`validate`, `diff`)
- **Phase 2**: 12 архитектурных генераторов (3 tier'а: infrastructure, detection, advanced)
- **Phase 2.5**: 8 генераторов безопасности (PSS, RBAC, resource limits, image security, TLS, audit policy, admission policy, supply chain)
- **Phase 3**: Deckhouse CRD процессоры (InstanceClass, GRPCRoute, TLSRoute, Canary), module scaffold, compatibility
- **Phase 4**: Инфраструктура — hooks генератор, Deckhouse compatibility checker

### Запланировано
| Направление | Описание | Статус |
|-------------|----------|--------|
| Cluster Extractor | Генерация чартов из live K8s кластера через client-go | Планируется |
| GitOps Extractor | Генерация чартов из Git репозиториев (ArgoCD/Flux) | Планируется |
| Auto-Fix Engine | Авто-добавление securityContext, resource limits, health probes, PDB | Планируется |
| CRD Support | Обработка произвольных CRD с извлечением схемы | Планируется |
| Миграция | Обнаружение drift, планы миграции, обратная совместимость values | Планируется |
| Secret Management | External Secrets Operator, Sealed Secrets, Vault CSI/Agent | Планируется |
| Service Mesh | Istio, Linkerd, OpenTelemetry | Исследование |
| Операторы БД | CloudNativePG, Percona, Redis Enterprise | Исследование |
| Compliance Reports | Генерация отчётов соответствия PSS/CIS Benchmark | Планируется |
| Multi-Cluster | Поддержка мультикластерных конфигураций и federation | Исследование |

## Участие в разработке

1. Сделайте fork репозитория
2. Создайте feature-ветку (`git checkout -b feature/amazing-feature`)
3. Зафиксируйте изменения (`git commit -m 'Add amazing feature'`)
4. Отправьте ветку в remote (`git push origin feature/amazing-feature`)
5. Откройте Pull Request

## Лицензия

Apache License 2.0 — см. [LICENSE](LICENSE).

## Авторы

- **Alex Gromer** — System Architect, End-to-End Engineer
  - DevOps/Infrastructure: Deckhouse (K8s), Astra Linux, Java Spring microservices
  - Systems programming: Go, Rust, C
  - [GitHub](https://github.com/AlexGromer)

## Ссылки

- [Документация Deckhouse](https://deckhouse.io/documentation/)
- [Документация Helm](https://helm.sh/docs/)
- [Справочник Kubernetes API](https://kubernetes.io/docs/reference/kubernetes-api/)
