# Deckhouse Helm Generator (DHG)

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)
![Coverage](https://img.shields.io/badge/coverage-89%25-brightgreen)
![License](https://img.shields.io/badge/license-Apache--2.0-blue)

CLI-инструмент для генерации Helm charts из Kubernetes/Deckhouse ресурсов с автоматическим обнаружением связей между ресурсами.

## Возможности

- 📦 **Автоматическое извлечение ресурсов** из YAML файлов, кластера Kubernetes или GitOps репозиториев
- 🔍 **Интеллектуальное обнаружение связей** между ресурсами (Service → Deployment, Ingress → Service, Volume mounts и т.д.)
- 🎯 **Группировка ресурсов** в логические сервисы на основе labels и dependencies
- 📝 **Генерация готовых Helm charts** с values.yaml, templates и _helpers.tpl
- 🔧 **Поддержка Deckhouse CRDs** (IngressNginxController, ModuleConfig, DexAuthenticator)
- 🎨 **Несколько режимов вывода**: Universal (один chart), Separate (chart на сервис), Library (библиотечный chart)

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
# Простейший пример
dhg generate -f ./manifests -o ./my-chart --chart-name myapp

# С verbose выводом
dhg generate -f ./manifests -o ./my-chart --chart-name myapp --verbose

# Рекурсивное сканирование директорий
dhg generate -f ./k8s --recursive -o ./chart --chart-name web-app
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

## Поддерживаемые ресурсы

### Standard Kubernetes (18 processors)

- ✅ **Core Workloads**: Deployment, StatefulSet, DaemonSet
- ✅ **Services & Networking**: Service, Ingress, NetworkPolicy
- ✅ **Configuration**: ConfigMap, Secret
- ✅ **Storage**: PersistentVolumeClaim
- ✅ **Autoscaling**: HorizontalPodAutoscaler (HPA)
- ✅ **Disruption Budget**: PodDisruptionBudget (PDB)
- ✅ **Batch Workloads**: CronJob, Job
- ✅ **RBAC & Identity**: ServiceAccount, Role, ClusterRole, RoleBinding, ClusterRoleBinding

### Deckhouse CRDs

- 🚧 IngressNginxController (deckhouse.io/v1)
- 🚧 ModuleConfig (deckhouse.io/v1alpha1)
- 🚧 DexAuthenticator, DexProvider, DexClient
- 🚧 NodeGroup

### Monitoring (Prometheus Operator)

- 🚧 PrometheusRule
- 🚧 ServiceMonitor

### cert-manager

- 🚧 Certificate
- 🚧 ClusterIssuer

_Примечание: ✅ = реализовано, 🚧 = в разработке_

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

### Universal (по умолчанию)

Один chart, все сервисы в `values.yaml`:

```yaml
services:
  frontend:
    enabled: true
    deployment: {...}
    service: {...}
  backend:
    enabled: true
    deployment: {...}
```

### Separate

Отдельный chart для каждого сервиса:

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

Библиотечный chart + тонкие обертки (в разработке).

## Опции CLI

### generate

```
Flags:
  -f, --file strings            Path(s) to YAML files or directories
  -o, --output string           Output directory (default "./chart")
      --chart-name string       Chart name (required)
      --chart-version string    Chart version (default "0.1.0")
      --app-version string      App version (default "1.0.0")
      --mode string             Output mode: universal|separate|library (default "universal")
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
│   ├── processor/        # Обработка ресурсов
│   │   ├── k8s/          # Стандартные K8s процессоры
│   │   ├── deckhouse/    # Deckhouse CRD процессоры
│   │   └── monitoring/   # Prometheus Operator процессоры
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
- Deckhouse CRD процессоры в разработке
- Separate и Library режимы в разработке

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Лицензия

MIT

## Авторы

- Ваша команда Deckhouse

## Ссылки

- [Deckhouse Documentation](https://deckhouse.io/documentation/)
- [Helm Documentation](https://helm.sh/docs/)
- [Kubernetes API Reference](https://kubernetes.io/docs/reference/kubernetes-api/)
