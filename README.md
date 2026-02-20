# Deckhouse Helm Generator (DHG)

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)
![Coverage](https://img.shields.io/badge/coverage-56%25-yellow)
![License](https://img.shields.io/badge/license-Apache--2.0-blue)

CLI-инструмент для генерации Helm charts из Kubernetes/Deckhouse ресурсов с автоматическим обнаружением связей между ресурсами.

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
git clone https://github.com/deckhouse/deckhouse-helm-generator.git
cd deckhouse-helm-generator
make build
sudo cp bin/dhg /usr/local/bin/
```

### Из бинарных релизов

```bash
# Linux AMD64
curl -LO https://github.com/deckhouse/deckhouse-helm-generator/releases/latest/download/dhg-linux-amd64
chmod +x dhg-linux-amd64
sudo mv dhg-linux-amd64 /usr/local/bin/dhg

# macOS ARM64
curl -LO https://github.com/deckhouse/deckhouse-helm-generator/releases/latest/download/dhg-darwin-arm64
chmod +x dhg-darwin-arm64
sudo mv dhg-darwin-arm64 /usr/local/bin/dhg
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

## Примеры использования

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

Генерация:

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

Полученный `values.yaml`:

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
│   Cluster   │    │    Files    │    │   GitOps    │
│ (client-go) │    │   (YAML)    │    │    (git)    │
└──────┬──────┘    └──────┬──────┘    └──────┬──────┘
       └──────────────────┼──────────────────┘
                          ▼
              ┌───────────────────────┐
              │      Extractor        │
              │  (Unstructured API)   │
              └───────────┬───────────┘
                          ▼
              ┌───────────────────────┐
              │       Analyzer        │
              │  (Relationship Graph) │
              └───────────┬───────────┘
                          ▼
              ┌───────────────────────┐
              │      Processors       │
              │   (GVK → Template)    │
              └───────────┬───────────┘
                          ▼
              ┌───────────────────────┐
              │      Generator        │
              │  (Chart + Values)     │
              └───────────────────────┘
```

## Поддерживаемые ресурсы (36 процессоров)

### Standard Kubernetes (18 processors)

- ✅ **Core Workloads**: Deployment, StatefulSet, DaemonSet
- ✅ **Services & Networking**: Service, Ingress, NetworkPolicy
- ✅ **Configuration**: ConfigMap, Secret
- ✅ **Storage**: PersistentVolumeClaim
- ✅ **Autoscaling**: HorizontalPodAutoscaler (HPA)
- ✅ **Disruption Budget**: PodDisruptionBudget (PDB)
- ✅ **Batch Workloads**: CronJob, Job
- ✅ **RBAC & Identity**: ServiceAccount, Role, ClusterRole, RoleBinding, ClusterRoleBinding

### Deckhouse CRDs (7 processors)

- ✅ **ModuleConfig** (`deckhouse.io/v1alpha1`): настройки модулей с version tracking
- ✅ **IngressNginxController** (`deckhouse.io/v1`): inlet, hostPort/hostWithFailover
- ✅ **ClusterAuthorizationRule** (`deckhouse.io/v1`): subjects, accessLevel
- ✅ **NodeGroup** (`deckhouse.io/v1`): nodeType, disruptions, cloudInstances
- ✅ **DexAuthenticator** (`deckhouse.io/v1`): applicationDomain, allowed groups
- ✅ **User** (`deckhouse.io/v1`): email, groups, ttl
- ✅ **Group** (`deckhouse.io/v1`): members list

### Monitoring (4 processors)

- ✅ **ServiceMonitor** (`monitoring.coreos.com/v1`): endpoints, namespaceSelector, Service dependency
- ✅ **PodMonitor** (`monitoring.coreos.com/v1`): podMetricsEndpoints, jobLabel
- ✅ **PrometheusRule** (`monitoring.coreos.com/v1`): alert/record rule groups
- ✅ **GrafanaDashboard**: ConfigMap с label `grafana_dashboard: "1"`

### Gateway API (2 processors)

- ✅ **HTTPRoute** (`gateway.networking.k8s.io/v1`): parentRefs, hostnames, rules
- ✅ **Gateway** (`gateway.networking.k8s.io/v1`): gatewayClassName, listeners, TLS

### KEDA (2 processors)

- ✅ **ScaledObject** (`keda.sh/v1alpha1`): scaleTargetRef, triggers, scale-to-zero detection
- ✅ **TriggerAuthentication** (`keda.sh/v1alpha1`): secretTargetRef, env, podIdentity

### cert-manager (2 processors)

- ✅ **Certificate** (`cert-manager.io/v1`): dnsNames, issuerRef, secretName
- ✅ **ClusterIssuer** (`cert-manager.io/v1`): ACME, selfSigned, CA

### Argo Rollouts (1 processor)

- ✅ **Rollout** (`argoproj.io/v1alpha1`): canary/blueGreen strategy

## Deckhouse Integration

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

## Monitoring Stack

Полная поддержка Prometheus Operator + Grafana dashboards:

```bash
dhg generate -f ./manifests --include-kinds Deployment,Service,ServiceMonitor,PrometheusRule \
  --chart-name myapp -o ./myapp-chart
```

- **ServiceMonitor** → автоматическая dependency на Service (через selector)
- **PrometheusRule** → alert/record rules с шаблонизацией threshold-ов
- **GrafanaDashboard** → ConfigMap с label `grafana_dashboard: "1"` автоматически детектируется (priority 110)
- **PodMonitor** → для Pod-level метрик без Service

## Modern K8s Patterns

### Gateway API

Замена Ingress для advanced routing:

```bash
dhg generate -f ./gateway-manifests --chart-name webapp -o ./webapp-chart
```

- **Gateway** → `gatewayClassName`, listeners (HTTP/HTTPS/TLS)
- **HTTPRoute** → parentRefs, hostnames, path-based routing с dependency на Gateway

### KEDA

Event-driven autoscaling:

- **ScaledObject** → `scaleTargetRef` → автоматическая dependency на Deployment/StatefulSet
- Scale-to-zero: `minReplicaCount: 0` → флаг в metadata
- **TriggerAuthentication** → секретные ключи для trigger-ов

### cert-manager

- **Certificate** → dnsNames, issuerRef, secretName
- **ClusterIssuer** → ACME (Let's Encrypt), selfSigned, CA
- Ingress с аннотацией `cert-manager.io/cluster-issuer` → dependency на ClusterIssuer

### Argo Rollouts

- **Rollout** → canary/blueGreen strategies
- Pod template preservation для progressive delivery

### ExternalDNS & TopologySpread

- **ExternalDNS**: аннотация `external-dns.alpha.kubernetes.io/hostname` на Service/Ingress → metadata
- **TopologySpreadConstraints**: автоизвлечение из pod spec Deployment

## Обнаружение связей

DHG автоматически обнаруживает следующие типы связей:

| Тип | Описание | Пример |
|-----|----------|--------|
| **LabelSelector** | Селектор по labels | Service → Deployment (по spec.selector) |
| **NameReference** | Прямая ссылка по имени | Ingress → Service (backend.service.name) |
| **VolumeMount** | Монтирование volume | Deployment → ConfigMap/Secret |
| **EnvFrom** | Переменные окружения | Deployment → ConfigMap/Secret (envFrom) |
| **EnvValueFrom** | Отдельная переменная | Deployment → ConfigMap/Secret (valueFrom) |
| **Annotation** | Аннотации | Ingress → ClusterIssuer (cert-manager) |
| **ServiceAccount** | Service Account | Deployment → ServiceAccount |
| **ImagePullSecret** | Image pull secrets | Deployment → Secret |

## Режимы вывода

| Режим | Описание | Когда использовать |
|-------|----------|-------------------|
| `universal` | Один chart для всех сервисов | Монолитное приложение, простая структура |
| `separate` | Отдельный chart на каждый сервис | Независимые деплои, разные версии |
| `library` | Библиотечный chart + тонкие wrapper charts | DRY-шаблоны, максимальное переиспользование |
| `umbrella` | Родительский chart + subcharts | Helmfile-style, условное включение сервисов |

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

## Environment-Specific Values

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

Файлы содержат только **переопределения** (override-only) — не копию всех значений base chart.

## Опции CLI

### generate

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

- Go 1.22+
- make
- (опционально) Helm 3.x для тестирования

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
├── cmd/dhg/              # CLI entrypoint
├── pkg/
│   ├── extractor/        # Извлечение ресурсов
│   ├── analyzer/         # Анализ связей
│   ├── processor/        # Обработка ресурсов (32 процессора)
│   │   └── k8s/          # K8s + Deckhouse + Monitoring + Gateway + KEDA + cert-manager + Argo
│   ├── generator/        # Генерация charts
│   ├── helm/             # Helm утилиты
│   └── types/            # Общие типы
├── testdata/             # Тестовые данные
├── Makefile
└── README.md
```

### Добавление нового процессора

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
    // Ваша логика обработки
    return &processor.Result{
        Processed:       true,
        ServiceName:     "myservice",
        TemplatePath:    "templates/myresource.yaml",
        TemplateContent: generateTemplate(obj),
        Values:          extractValues(obj),
    }, nil
}
```

Затем зарегистрируйте в `pkg/processor/k8s/registry.go`:

```go
func RegisterAll(r *processor.Registry) {
    // ...
    r.Register(NewMyResourceProcessor())
}
```

## Ограничения и известные проблемы

- Cluster extractor (извлечение из live кластера) еще не реализован
- GitOps extractor еще не реализован

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Лицензия

Apache License 2.0 — см. [LICENSE](LICENSE).

## Авторы

- Ваша команда Deckhouse

## Ссылки

- [Deckhouse Documentation](https://deckhouse.io/documentation/)
- [Helm Documentation](https://helm.sh/docs/)
- [Kubernetes API Reference](https://kubernetes.io/docs/reference/kubernetes-api/)
