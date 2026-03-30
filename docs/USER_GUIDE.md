# Руководство пользователя — DHG (Deckhouse Helm Generator)

> **Тип:** Руководство + Справочник
> **Аудитория:** разработчики и platform engineers, использующие DHG для генерации Helm chart
> **Последнее обновление:** 2026-03-30
> **Связанные документы:** [DEVELOPER.md](DEVELOPER.md), [ADR.md](ADR.md)

## Обзор

DHG — это CLI-инструмент, генерирующий production-ready Helm chart из Kubernetes YAML-манифестов. Он извлекает ваши ресурсы, обнаруживает связи между ними, группирует их в сервисы и создаёт полный chart — включая `values.yaml`, templates, `_helpers.tpl` и дополнительные компоненты: environment overlays, security policies и scaffold для Deckhouse modules.

---

## 1. Установка

### Homebrew (macOS и Linux)

```bash
brew install AlexGromer/tap/dhg
```

### Готовый бинарный файл (Linux)

```bash
VERSION=$(curl -s https://api.github.com/repos/AlexGromer/deckhouse-helm-generator/releases/latest \
  | grep tag_name | cut -d '"' -f4)

# Linux AMD64
curl -LO "https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/${VERSION}/dhg_${VERSION#v}_linux_amd64.tar.gz"
tar xzf "dhg_${VERSION#v}_linux_amd64.tar.gz"
sudo mv dhg /usr/local/bin/

# Linux ARM64
curl -LO "https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/${VERSION}/dhg_${VERSION#v}_linux_arm64.tar.gz"
tar xzf "dhg_${VERSION#v}_linux_arm64.tar.gz"
sudo mv dhg /usr/local/bin/
```

### Готовый бинарный файл (macOS)

```bash
VERSION=$(curl -s https://api.github.com/repos/AlexGromer/deckhouse-helm-generator/releases/latest \
  | grep tag_name | cut -d '"' -f4)

# macOS ARM64 (Apple Silicon)
curl -LO "https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/${VERSION}/dhg_${VERSION#v}_darwin_arm64.tar.gz"
tar xzf "dhg_${VERSION#v}_darwin_arm64.tar.gz"
sudo mv dhg /usr/local/bin/

# macOS AMD64 (Intel)
curl -LO "https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/${VERSION}/dhg_${VERSION#v}_darwin_amd64.tar.gz"
tar xzf "dhg_${VERSION#v}_darwin_amd64.tar.gz"
sudo mv dhg /usr/local/bin/
```

### go install

```bash
go install github.com/AlexGromer/deckhouse-helm-generator/cmd/dhg@latest
```

### Docker

```bash
docker pull ghcr.io/alexgromer/dhg:latest

# Запуск с файлами из текущей директории
docker run --rm -v $(pwd):/work \
  ghcr.io/alexgromer/dhg:latest \
  generate -f /work/manifests -o /work/chart --chart-name myapp
```

### Сборка из исходного кода

```bash
git clone https://github.com/AlexGromer/deckhouse-helm-generator.git
cd deckhouse-helm-generator
make build
sudo cp bin/dhg /usr/local/bin/
```

### Проверка

```bash
dhg version
# dhg version v0.7.3 (built: ...)
```

---

## 2. Быстрый старт

**Время выполнения:** ~5 минут

**Предварительные требования:** установленный `dhg`, директория с Kubernetes YAML-манифестами

### Шаг 1: Подготовьте манифесты

Разместите Kubernetes YAML-файлы в директории:

```
manifests/
  deployment.yaml
  service.yaml
  ingress.yaml
  configmap.yaml
```

### Шаг 2: Сгенерируйте chart

```bash
dhg generate -f ./manifests -o ./my-chart --chart-name myapp
```

Ожидаемый вывод:

```
[1/5] Extracting resources from source...
[2/5] Processing resources...
[3/5] Analyzing relationships...
[4/5] Generating Helm chart...
[5/5] Writing charts to disk...

Successfully generated 1 chart(s) in ./my-chart

To install the chart, run:
  helm install my-release ./my-chart/myapp
```

### Шаг 3: Проверьте результат

```
my-chart/
└── myapp/
    ├── Chart.yaml
    ├── values.yaml
    ├── README.md
    └── templates/
        ├── _helpers.tpl
        ├── myapp-deployment.yaml
        ├── myapp-service.yaml
        ├── myapp-ingress.yaml
        └── myapp-configmap.yaml
```

### Шаг 4: Установите с помощью Helm

```bash
helm install my-release ./my-chart/myapp
# или с кастомными values
helm install my-release ./my-chart/myapp --set myapp.deployment.replicas=3
```

---

## 3. Справочник по CLI

### Глобальные команды

| Команда | Описание |
|---------|----------|
| `dhg generate` | Генерировать Helm chart из Kubernetes-ресурсов |
| `dhg analyze` | Анализировать ресурсы и выдать архитектурные рекомендации |
| `dhg validate` | Проверить структуру Helm chart и синтаксис шаблонов |
| `dhg diff` | Показать различия между двумя директориями chart |
| `dhg fix` | Автоматически исправить манифесты с учётом security best practices |
| `dhg migrate` | Обнаружить расхождения и сформировать план миграции |
| `dhg version` | Вывести информацию о версии |

---

### `dhg generate`

Генерирует Helm chart из файлов Kubernetes-ресурсов.

```
dhg generate [flags]
```

**Обязательные флаги:**

| Флаг | Описание |
|------|----------|
| `-f, --file strings` | Путь(и) к YAML-файлам или директориям |
| `--chart-name string` | Имя chart |

**Основные флаги:**

| Флаг | По умолчанию | Описание |
|------|-------------|----------|
| `-o, --output string` | `./chart` | Выходная директория |
| `--chart-version string` | `0.1.0` | Версия Helm chart |
| `--app-version string` | `1.0.0` | Версия приложения |
| `--mode string` | `universal` | Режим вывода: `universal`, `separate`, `library`, `umbrella` |
| `-r, --recursive` | `true` | Рекурсивный обход директорий |
| `-v, --verbose` | `false` | Подробный вывод |
| `--dry-run` | `false` | Вывести chart в stdout, не записывать на диск |

**Флаги фильтрации:**

| Флаг | Описание |
|------|----------|
| `-n, --namespace string` | Фильтровать ресурсы по namespace |
| `--namespaces strings` | Фильтр по нескольким namespace |
| `-l, --selector string` | Фильтр по label selector (например, `app=myapp`) |
| `--include-kinds strings` | Включить только указанные типы ресурсов |
| `--exclude-kinds strings` | Исключить указанные типы ресурсов |

**Флаги вывода:**

| Флаг | По умолчанию | Описание |
|------|-------------|----------|
| `--include-tests` | `false` | Генерировать шаблоны тестов для helm-unittest |
| `--include-readme` | `true` | Генерировать README.md |
| `--include-schema` | `false` | Генерировать `values.schema.json` |
| `--template-style string` | `standard` | Стиль вывода шаблонов: `standard` или `helm` |
| `--values-flat` | `false` | Добавить комментарии с dot-notation путями в values.yaml для использования с `--set` |

**Флаги окружения и инфраструктуры:**

| Флаг | Описание |
|------|----------|
| `--env-values` | Генерировать `values-dev.yaml`, `values-staging.yaml`, `values-prod.yaml` |
| `--namespace-resources` | Генерировать ResourceQuota, LimitRange, NetworkPolicy |
| `--feature-flags` | Добавить feature flag guards (monitoring, ingress, autoscaling, security, storage, rbac) |
| `--cloud-provider string` | Провайдер облака для аннотаций Service: `aws`, `gcp`, `azure` |
| `--cloud-internal` | Использовать internal load balancer (по умолчанию: internet-facing) |
| `--detect-ingress` | Автоматически определить ingress controller и добавить соответствующие аннотации |
| `--airgap-registry string` | Генерировать air-gap артефакты с указанием целевого registry |
| `--auto-deps` | Автоматически обнаружить инфраструктурные зависимости (PostgreSQL, Redis и др.) |

**Топологические флаги:**

| Флаг | По умолчанию | Описание |
|------|-------------|----------|
| `--monorepo` | `false` | Генерировать monorepo-структуру (Makefile, .helmignore, ct.yaml) |
| `--kustomize` | `false` | Генерировать Kustomize-структуру с base и overlays для dev/staging/prod |
| `--post-renderer` | `false` | Генерировать Kustomize overlays, совместимые с Flux CD `postBuild` |
| `--multi-tenant` | `false` | Генерировать multi-tenant overlay с изоляцией на уровне tenant |
| `--tenant-count int` | `2` | Количество примеров tenant для scaffold |
| `--spot` | `false` | Добавить tolerations и PDB для spot/preemptible инстансов |
| `--spot-grace-period int` | `15` | Время ожидания (секунды) для preStop hook при освобождении spot-инстанса |

**Флаги Deckhouse:**

| Флаг | Описание |
|------|----------|
| `--deckhouse-module` | Генерировать scaffold Deckhouse module (helm_lib, openapi/, images/, hooks/) |
| `--hooks` | Генерировать шаблоны Helm lifecycle hook Job (pre-upgrade, post-install, pre-delete) |

> Примечание: `--monorepo` и `--kustomize` взаимоисключающие флаги.

---

### `dhg analyze`

Анализирует ресурсы на предмет архитектурных паттернов, best practices и рекомендаций по группировке сервисов.

```
dhg analyze -f ./manifests [flags]
```

| Флаг | По умолчанию | Описание |
|------|-------------|----------|
| `-f, --file strings` | обязательный | Путь(и) к YAML-файлам или директориям |
| `--output-format string` | `text` | Формат вывода: `text`, `json`, `markdown` |
| `-o, --output string` | stdout | Выходной файл |
| `--summary` | `false` | Показать только раздел сводки |
| `--color` | `true` | Включить цветной вывод |
| `-r, --recursive` | `true` | Рекурсивный обход директорий |
| `-n, --namespace string` | | Фильтр по namespace |

**Примеры:**

```bash
# Вывести рекомендации в stdout
dhg analyze -f ./manifests

# Экспортировать как Markdown-отчёт
dhg analyze -f ./manifests --output-format markdown -o analysis.md
```

---

### `dhg validate`

Проверяет существующий Helm chart на структурные проблемы и синтаксические ошибки в шаблонах.

```
dhg validate -f ./chart/myapp [flags]
```

| Флаг | По умолчанию | Описание |
|------|-------------|----------|
| `-f, --file strings` | `.` | Путь(и) к директориям chart |
| `-v, --verbose` | `false` | Подробный вывод |

Выполняемые проверки:
- Наличие `Chart.yaml` и обязательных полей (`apiVersion`, `name`, `version`)
- Наличие `values.yaml` и корректность YAML
- Синтаксис шаблонов: сбалансированные разделители `{{ }}`

**Пример:**

```bash
dhg validate -f ./chart/myapp -v
```

Ожидаемый вывод для корректного chart:

```
Validating chart at: ./chart/myapp
  OK: Chart.yaml found (142 bytes)
  OK: values.yaml valid (520 bytes)
  OK: deployment.yaml (12 template expressions)
  Templates: 5 files checked

Validation complete: 0 error(s), 0 warning(s)
```

---

### `dhg diff`

Показывает различия между двумя директориями chart.

```
dhg diff <dir1> <dir2> [flags]
```

| Флаг | По умолчанию | Описание |
|------|-------------|----------|
| `--color` | `true` | Включить цветной вывод |

**Пример:**

```bash
# Сравнить chart до и после регенерации
dhg diff ./chart-v1 ./chart-v2
```

Вывод включает:
- Файлы, присутствующие только в одной директории
- Построчные различия для изменённых файлов

---

### `dhg fix`

Автоматически исправляет Kubernetes-манифесты, добавляя security best practices.

```
dhg fix -f ./manifests -o ./fixed [flags]
```

| Флаг | По умолчанию | Описание |
|------|-------------|----------|
| `-f, --file strings` | обязательный | Путь(и) к YAML-файлам или директориям |
| `-o, --output string` | `./fixed` | Выходная директория для исправленных манифестов |
| `--chart-name string` | `fixed-chart` | Имя выходного chart |
| `--workload-type string` | `web` | Профиль ресурсов: `web`, `worker`, `database`, `batch`, `cache` |
| `-r, --recursive` | `true` | Рекурсивный обход |
| `-v, --verbose` | `false` | Подробный вывод |

Применяемые исправления:
- `SecurityContext` (`runAsNonRoot`, `readOnlyRootFilesystem`, `allowPrivilegeEscalation: false`)
- Resource requests и limits (подбираются по `--workload-type`)
- Liveness, readiness и startup probes
- PodDisruptionBudgets
- PSS Restricted compliance labels
- Graceful shutdown через `preStop` hook

**Пример:**

```bash
dhg fix -f ./manifests -o ./fixed --workload-type web -v
```

---

### `dhg migrate`

Сравнивает существующий chart с манифестами и создаёт отчёт о расхождениях и план миграции.

```
dhg migrate --from ./existing-chart -f ./manifests --chart-name myapp [flags]
```

| Флаг | По умолчанию | Описание |
|------|-------------|----------|
| `--from string` | обязательный | Путь к существующей директории chart |
| `-f, --source strings` | обязательный | Путь(и) к исходным файлам манифестов |
| `--chart-name string` | обязательный | Имя chart |
| `--chart-version string` | `0.1.0` | Версия chart для генерируемого сравнительного chart |
| `--mode string` | `universal` | Режим вывода |
| `-v, --verbose` | `false` | Подробный вывод |

**Пример:**

```bash
dhg migrate --from ./chart/myapp -f ./manifests --chart-name myapp -v
```

Вывод:
- Сводка расхождений (добавленные templates, удалённые templates, изменённые values)
- Пошаговый план миграции
- Шаблон миграции values `_migrate.tpl` (при изменении ключей values)

---

## 4. Режимы вывода

DHG поддерживает четыре режима вывода, задаваемых флагом `--mode`.

### universal (по умолчанию)

Все ресурсы помещаются в один Helm chart. Подходит для простых приложений или когда нужна единственная команда `helm install`.

```bash
dhg generate -f ./manifests -o ./chart --chart-name myapp --mode universal
```

```
chart/
└── myapp/
    ├── Chart.yaml
    ├── values.yaml
    └── templates/
        ├── frontend-deployment.yaml
        ├── backend-deployment.yaml
        └── ...
```

### separate

Отдельный chart для каждого обнаруженного сервиса. Подходит для микросервисов, которые деплоятся независимо.

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

### library

Один library chart с общими шаблонами и тонкий wrapper chart для каждого сервиса. Подходит для организаций, применяющих DRY-шаблоны для множества сервисов.

```bash
dhg generate -f ./manifests -o ./charts --chart-name myapp --mode library
```

### umbrella

Родительский chart, в котором каждый сервис подключён как зависимость-subchart. Подходит для одновременного деплоя всех сервисов с раздельными конфигурациями.

```bash
dhg generate -f ./manifests -o ./charts --chart-name myapp --mode umbrella
```

```
charts/
└── myapp/             # родительский (umbrella) chart
    ├── Chart.yaml     # перечисляет frontend + backend как зависимости
    ├── values.yaml    # глобальные values
    └── charts/
        ├── frontend/
        └── backend/
```

---

## 5. Environment overlays (`--env-values`)

Генерирует environment-специфичные файлы `values-*.yaml` в дополнение к основному `values.yaml`:

```bash
dhg generate -f ./manifests -o ./chart --chart-name myapp --env-values
```

DHG определяет тип workload (web, worker, database и т.д.) и генерирует профили соответственно:

```
chart/myapp/
├── values.yaml            # базовые values
├── values-dev.yaml        # уменьшенные replicas, смягчённые resource limits
├── values-staging.yaml    # production-подобное окружение, меньший масштаб
└── values-prod.yaml       # полные replicas, жёсткие resource limits, HPA включён
```

Установка с environment overlay:

```bash
helm install myapp ./chart/myapp -f ./chart/myapp/values-prod.yaml
```

---

## 6. Функции безопасности (`--security-mode` и другие)

### Pod Security Standards

DHG может добавлять PSS-метки к namespace и pod во время генерации:

```bash
# Применить PSS baseline ко всем ресурсам
dhg generate -f ./manifests -o ./chart --chart-name myapp
# PSS-метки генерируются post-processor'ом pss.go; активируются через --deckhouse-module или в настройках Phase 2.5 по умолчанию
```

Для доработки существующих манифестов используйте `dhg fix`:

```bash
dhg fix -f ./manifests -o ./fixed --workload-type web
```

### Resource limits

Команда `dhg fix` добавляет CPU и memory requests/limits, подобранные по типу workload:

| `--workload-type` | CPU request | Memory request | Сценарий использования |
|-------------------|-------------|----------------|------------------------|
| `web` | 100m | 128Mi | Stateless HTTP-сервисы |
| `worker` | 500m | 256Mi | Фоновые обработчики задач |
| `database` | 1000m | 512Mi | Stateful хранилища данных |
| `batch` | 200m | 128Mi | Одноразовые Job-нагрузки |
| `cache` | 100m | 256Mi | In-memory кэши (Redis, Memcached) |

### RBAC scaffold

```bash
# RBAC-шаблоны генерируются автоматически при наличии ресурсов ServiceAccount
# Для явной генерации RBAC post-processor rbac.go запускается во время generate
dhg generate -f ./manifests -o ./chart --chart-name myapp
```

### Air-gapped окружения

```bash
dhg generate -f ./manifests -o ./chart --chart-name myapp \
  --airgap-registry registry.internal.example.com/mirror
```

В дополнение к chart генерируется:
- `images.txt` — список всех container images, упомянутых в шаблонах
- `mirror-images.sh` — скрипт для pull и push образов в ваш registry
- `values-airgap.yaml` — переопределение values, указывающее все образы на mirror registry

---

## 7. Стратегии управления секретами (`--secret-strategy`)

> Примечание: `--secret-strategy` введён в Phase 5.7. Проверьте наличие в вашей версии командой `dhg generate --help`.

DHG поддерживает четыре взаимоисключающие стратегии управления секретами (ADR-042):

| Стратегия | Значение флага | Провайдер |
|-----------|---------------|-----------|
| External Secrets Operator | `eso` | ESO `ExternalSecret` CRs |
| Sealed Secrets | `sealed` | Bitnami `SealedSecret` CRs |
| Vault CSI Provider | `vault-csi` | `SecretProviderClass` CRs |
| SOPS / Helm Secrets | `sops` | Зашифрованные values-файлы |

```bash
# Генерировать chart с секретами, управляемыми ESO
dhg generate -f ./manifests -o ./chart --chart-name myapp --secret-strategy eso

# Генерировать chart с Sealed Secrets
dhg generate -f ./manifests -o ./chart --chart-name myapp --secret-strategy sealed
```

При указании стратегии секретов DHG заменяет обычные шаблоны `Secret` на соответствующие provider CRs и добавляет интеграцию с `Reloader` на основе аннотаций для перезапуска pod при ротации секретов.

---

## 8. Генерация Deckhouse Module (`--deckhouse-module`)

Генерирует scaffold, совместимый с Deckhouse module, вместо обычного Helm chart:

```bash
dhg generate -f ./manifests -o ./module --chart-name my-module --deckhouse-module
```

Добавляемая структура:

```
module/my-module/
├── Chart.yaml          # с зависимостью helm_lib
├── values.yaml
├── openapi/
│   └── config-values.yaml   # OpenAPI схема для валидации ModuleConfig
├── images/
│   └── .gitkeep             # placeholder для image build contexts
├── hooks/
│   └── .gitkeep             # placeholder для shell hooks
└── templates/
    ├── _helpers.tpl
    └── ...
```

Схема `openapi/config-values.yaml` генерируется из структуры `values.yaml` и совместима с валидацией CRD `ModuleConfig` в Deckhouse.

---

## 9. Примеры

### Базовый: односервисное приложение

```bash
dhg generate -f ./manifests/nginx -o ./chart --chart-name nginx-app
helm lint ./chart/nginx-app
helm install nginx ./chart/nginx-app
```

### Многосервисное приложение с раздельными chart

```bash
dhg generate -f ./manifests -o ./charts --chart-name shop --mode separate -v
# Просмотреть обнаруженные сервисы
ls ./charts/
# frontend  backend  postgres  redis

# Задеплоить все сервисы
for dir in ./charts/*/; do
  name=$(basename "$dir")
  helm install "$name" "$dir"
done
```

### С security и environment overlays

```bash
dhg generate -f ./manifests \
  -o ./chart \
  --chart-name myapp \
  --env-values \
  --namespace-resources \
  --feature-flags \
  --include-schema \
  --include-tests

# Установить для production
helm install myapp ./chart/myapp \
  -f ./chart/myapp/values-prod.yaml \
  --namespace production
```

### С аннотациями облачного провайдера (AWS)

```bash
dhg generate -f ./manifests \
  -o ./chart \
  --chart-name myapp \
  --cloud-provider aws \
  --detect-ingress

# Services получают аннотации AWS NLB; Ingress получает аннотации nginx/alb controller
```

### Workload на spot-инстансах

```bash
dhg generate -f ./manifests \
  -o ./chart \
  --chart-name batch-jobs \
  --spot \
  --spot-grace-period 30 \
  --cloud-provider gcp
```

### Deckhouse module с секретами

```bash
dhg generate -f ./manifests \
  -o ./module \
  --chart-name my-deckhouse-module \
  --deckhouse-module \
  --secret-strategy eso \
  --env-values
```

### Анализ перед генерацией

```bash
# Сначала разберитесь, что у вас есть
dhg analyze -f ./manifests --output-format markdown -o analysis.md
cat analysis.md

# Затем генерируйте на основе рекомендаций
dhg generate -f ./manifests -o ./chart --chart-name myapp
```

### Исправление, затем генерация

```bash
# Сначала исправьте проблемы безопасности в манифестах
dhg fix -f ./manifests -o ./fixed --workload-type web -v

# Генерировать из исправленных манифестов
dhg generate -f ./fixed -o ./chart --chart-name myapp

# Проверить результат
dhg validate -f ./chart/myapp -v
```

---

## Устранение неполадок

| Симптом | Причина | Решение |
|---------|---------|---------|
| `no resources extracted` | Путь в `-f` не существует или не содержит YAML | Проверьте путь: `ls ./manifests/*.yaml` |
| `invalid mode: umbrella` | Опечатка в значении `--mode` | Допустимые значения: `universal`, `separate`, `library`, `umbrella` |
| `--monorepo and --kustomize are mutually exclusive` | Указаны оба флага | Используйте один из них |
| `unknown cloud provider: "eks"` | Значение `--cloud-provider` не распознано | Допустимые значения: `aws`, `gcp`, `azure` |
| Несбалансированные `{{ }}` в шаблонах | Шаблон вручную отредактирован с синтаксической ошибкой | Запустите `dhg validate -f ./chart/myapp` для определения файла |
| `no extractor available for source type: cluster` | `--source cluster` используется до завершения Phase 4 | Используйте `--source file` (по умолчанию) |
| Ошибка прав доступа Docker | `$(pwd)` некорректно разрешается в Windows | Используйте абсолютные пути: `-v /c/Users/you/project:/work` |
