# Deckhouse Helm Generator (DHG)

![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)
[![codecov](https://codecov.io/gh/AlexGromer/deckhouse-helm-generator/graph/badge.svg)](https://codecov.io/gh/AlexGromer/deckhouse-helm-generator)
![License](https://img.shields.io/badge/license-Apache--2.0-blue)
![Platforms](https://img.shields.io/badge/platforms-linux%20%7C%20darwin%20%7C%20windows-lightgrey)

CLI, превращающий манифесты Kubernetes и Deckhouse в Helm charts. Читает YAML-файлы, живой кластер или Git-репозиторий, определяет связи между ресурсами (селекторы, ссылки по имени, тома, env, ServiceAccount), группирует их в сервисы и генерирует chart в одном из четырёх режимов.

Каждый chart, который генерирует dhg, проверяется в CI настоящим Helm: `helm lint --strict`, `helm template`, проверка, что не пропал ни один входной объект, что ссылки между объектами и селекторы продолжают работать, что селекторы workload'ов совпадают с метками pod'ов, плюс `helm unittest` и post-renderer end-to-end (см. [Как проверяется результат](#как-проверяется-результат)).

---

## Содержание

- [Установка](#установка)
- [Быстрый старт](#быстрый-старт)
- [Источники ресурсов](#источники-ресурсов)
- [Режимы вывода](#режимы-вывода)
- [Что генерируется](#что-генерируется)
- [Флаги generate](#флаги-generate)
- [Опциональные возможности (`--with`)](#опциональные-возможности---with)
- [Расширение: конфиг, плагины, переопределение шаблонов](#расширение-конфиг-плагины-переопределение-шаблонов)
- [Остальные команды](#остальные-команды)
- [Как проверяется результат](#как-проверяется-результат)
- [Ограничения](#ограничения)
- [Разработка](#разработка)

---

## Установка

### Бинарный релиз

Архивы для linux/darwin/windows (amd64/arm64), пакеты DEB/RPM/APK и образ `ghcr.io/alexgromer/dhg` публикуются на странице [Releases](https://github.com/AlexGromer/deckhouse-helm-generator/releases) (GoReleaser, подпись cosign, SBOM).

### Из исходников

```bash
git clone https://github.com/AlexGromer/deckhouse-helm-generator.git
cd deckhouse-helm-generator
make build            # ./bin/dhg
```

### Через `go install`

```bash
go install github.com/AlexGromer/deckhouse-helm-generator/cmd/dhg@main
```

`@latest` заработает со следующего релиза: в тегах до `v1.0.0` включительно `go.mod` объявляет прежний путь `github.com/deckhouse/deckhouse-helm-generator`, и Go отклоняет несовпадение.

---

## Быстрый старт

```bash
# Один chart для всех ресурсов
dhg generate -f ./manifests -o ./charts --chart-name myapp

# Проверить результат
helm lint ./charts/myapp
helm template my-release ./charts/myapp
dhg validate -f ./charts/myapp            # синтаксис шаблонов + совместимость API с K8s 1.27–1.32

# Схема values, тесты helm-unittest, оверлеи окружений
dhg generate -f ./manifests -o ./charts --chart-name myapp \
  --include-schema --include-tests --env-values
```

---

## Источники ресурсов

| Источник | Флаги | Что делает |
|---|---|---|
| `file` (по умолчанию) | `-f` (файлы/каталоги, можно несколько), `-r` | Читает YAML (многодокументные файлы, рекурсивно) |
| `cluster` | `--kubeconfig`, `--context`, `-n`, `--cluster-secrets skip\|mask\|include` | Выгружает объекты через API-сервер (kubeconfig: CA, client-cert, bearer token) |
| `gitops` | `--git-repo`, `--git-branch`, `--git-path`, `--ssh-key` | `git clone --depth 1` во временный каталог, далее как `file` |

Общие фильтры: `-n/--namespace`, `--namespaces`, `-l/--selector` (синтаксис kubectl), `--include-kinds`, `--exclude-kinds`. Одинаковый объект в нескольких файлах берётся один раз (первое определение), с предупреждением.

При выгрузке из кластера dhg пропускает то, что создаёт control plane, а не chart:
- объекты с `ownerReferences` (pod'ы и ReplicaSet'ы Deployment'а, Job'ы CronJob'а);
- Events, Endpoints, EndpointSlices, Leases, Nodes, Namespaces;
- `kube-root-ca.crt`, ServiceAccount `default`, токены ServiceAccount;
- системные namespace `kube-*` и `d8-*` (Deckhouse), если namespace не задан явно.

Кроме того, удаляются поля, проставленные сервером: `uid`, `resourceVersion`, `managedFields`, `status`, `last-applied-configuration`, `clusterIP`. Secrets по умолчанию не выгружаются.

```bash
dhg generate -s cluster -n shop --chart-name shop -o ./charts
dhg generate -s gitops --git-repo https://github.com/org/manifests --git-path apps/web --chart-name web
```

Входные манифесты с устаревшими или удалёнными API (например, `policy/v1beta1` PodDisruptionBudget) дают предупреждение с заменой.

---

## Режимы вывода

| Режим | Результат | Когда использовать |
|---|---|---|
| `universal` | Один chart, values сервисов в `services.<имя>` | Приложение деплоится целиком |
| `separate` | По chart на сервисную группу, values плоские | Сервисы релизятся независимо |
| `library` | Library chart `library` с общими helpers и chart на каждую группу, использующий `library.*` | Один набор helpers на много chart'ов |
| `umbrella` | Родительский chart, группы — subcharts в `charts/` с условием `<группа>.enabled` | Условное включение компонентов |

```bash
dhg generate -f ./manifests -o ./charts --chart-name shop --mode umbrella
helm upgrade --install shop ./charts/shop --set database.enabled=false
```

Для `library`-режима зависимость подключается как обычно: `helm dependency build ./charts/<chart>`.

---

## Что генерируется

- `Chart.yaml`, `values.yaml`, `templates/` (по шаблону на объект), `_helpers.tpl`, `NOTES.txt`, `.helmignore`, `README.md` с таблицей параметров (пути в синтаксисе `--set`).
- **Значения**: поля спецификаций выносятся в values по пути, который читает шаблон; каждый сервис включается и выключается через `enabled`.
- **Неизвестные kinds и custom resources** сохраняют все поля верхнего уровня (`spec`, `data`, `rules`, …).
- **CustomResourceDefinitions** пишутся без шаблонизации в `crds/`: Helm устанавливает их до шаблонов и никогда не обновляет и не удаляет.
- `--include-schema`: `values.schema.json` (JSON Schema draft-07), выведенная из values по принципу «структура строго, скаляры мягко». Объект остаётся объектом, список — списком, boolean — boolean; строка и число взаимозаменяемы. Неизвестные ключи разрешены.
- `--include-tests`: тесты [helm-unittest](https://github.com/helm-unittest/helm-unittest) (вид ресурса, выключение через `enabled`, snapshot-тесты).
- `--hooks`: Job-хуки `pre-upgrade`/`post-install`/`pre-delete` (образ — `hooks.image`).
- `--deckhouse-module`: структура модуля Deckhouse — зависимость `helm_lib`, `openapi/config-values.yaml` (схема из values), `openapi/values.yaml`, `images/`, `hooks/`.

---

## Флаги generate

Полный список — `dhg generate --help`. Пост-обработка:

| Флаг | Что добавляет |
|---|---|
| `--env-values` | `values-dev.yaml`, `values-staging.yaml`, `values-prod.yaml` с профилями по типу нагрузки |
| `--namespace-resources` | ResourceQuota, LimitRange, NetworkPolicy по умолчанию и NetworkPolicy по связям сервисов (переключатели `namespace.*`) |
| `--multi-tenant`, `--tenant-count` | Оверлей с изоляцией арендаторов |
| `--feature-flags` | Переключатели monitoring/ingress/autoscaling/security/storage/rbac |
| `--cloud-provider aws\|gcp\|azure`, `--cloud-internal` | Аннотации балансировщика для Service (сливаются с аннотациями из values) |
| `--detect-ingress` | Аннотации для обнаруженного ingress-контроллера |
| `--spot`, `--spot-grace-period` | Tolerations для spot-узлов, `terminationGracePeriodSeconds` и PDB на каждый workload (`spot.enabled`) |
| `--auto-deps` | Bitnami-зависимости (PostgreSQL, Redis, …), найденные по env, выключенные по умолчанию |
| `--airgap-registry` | `images.txt`, `mirror-images.sh`, `values-airgap.yaml` |
| `--kustomize` | `<chart>/kustomize/`: base из входных манифестов и overlay'и dev/staging/prod (replicas 1/2/3 для каждого Deployment/StatefulSet) — путь доставки без Helm |
| `--post-renderer` | `post-renderer/kustomize.sh` — post-renderer для Helm с оверлеями dev/staging/prod: `helm install … --post-renderer ./post-renderer/kustomize.sh --post-renderer-args prod` |
| `--monorepo` | Makefile, `ct.yaml` для chart-testing |
| `--values-flat` | Комментарии с путями `--set` в `values.yaml` |

---

## Опциональные возможности (`--with`)

Возможности подключаются по имени и настраиваются параметрами со значениями по умолчанию. Неизвестный параметр — ошибка, а не молчаливое игнорирование. Всё, что добавляется в chart, выключается через values.

```bash
dhg features                                   # список и параметры
dhg generate -f ./manifests --chart-name app \
  --with istio,prometheus-rules,reloader \
  --feature-opt istio.timeout=10s --feature-opt prometheus-rules.severity=critical
```

| Возможность | Что добавляет |
|---|---|
| `anti-affinity` | podAntiAffinity (и опционально распределение по зонам) для Deployment/StatefulSet |
| `argo-rollouts` | Canary `Rollout` (`workloadRef`) для каждого Deployment |
| `config-checksums` | Аннотации `checksum/*`: перезапуск pod'ов при изменении их ConfigMap/Secret |
| `external-secrets` | `ExternalSecret` (External Secrets Operator) для используемых Secrets из существующего (Cluster)SecretStore |
| `flux` | `flux/helmrelease.yaml` — Flux `HelmRelease` для этого chart |
| `ingress-tls` | TLS для Ingress без `tls`: аннотация issuer cert-manager и секрет сертификата |
| `istio` | VirtualService, DestinationRule, PeerAuthentication, AuthorizationPolicy из values |
| `istio-egress` | ServiceEntry для внешних хостов, найденных в env |
| `linkerd` | Аннотация инъекции прокси |
| `otel` | `Instrumentation` (OpenTelemetry Operator) и аннотации автоинструментирования |
| `policies` | Политики безопасности workload'ов: Kyverno `Policy` и/или Rego для conftest в `policy/` |
| `prometheus-rules` | `PrometheusRule`: crash loop, OOM, рестарты, not ready; опционально SLO burn-rate |
| `reloader` | Аннотации Stakater Reloader |
| `resource-report` | `docs/resource-report.md`: оценка стоимости, right-sizing, находки по томам |
| `vault-agent` | Аннотации HashiCorp Vault Agent для workload'ов с Secrets |
| `velero-backup` | `Schedule` Velero (≥ 1.10) для chart'ов с томами: объекты релиза, pod'ы его workload'ов и их PVC (`orLabelSelectors`, `veleroBackup.labelSelectors`) |

Все 16 возможностей проверяются в CI и по отдельности, и все вместе, во всех режимах.

---

## Расширение: конфиг, плагины, переопределение шаблонов

**Конфиг.** `.dhg.yaml` в рабочем каталоге (или `--config path`) — ключи совпадают с именами флагов `generate`, флаги командной строки важнее:

```yaml
chart-name: shop
mode: umbrella
include-schema: true
with: [istio, prometheus-rules]
feature-opt: ["istio.timeout=10s"]
plugin: ["example.com/v1/Widget=./bin/widget-plugin"]
template-dir: ./chart-overrides
```

**Плагины.** `--plugin <apiVersion>/<Kind>[,…]=<исполняемый файл>` — внешний процессор для указанных GVK с приоритетом над встроенными. Протокол JSON:
- в stdin плагин получает `{"name","namespace","kind","apiVersion","chartName","object"}`;
- в stdout отдаёт `{"serviceName","templatePath","templateContent","valuesPath","values"}`;
- пустой `templateContent` означает «не обработал», и ресурс уходит следующему процессору;
- таймаут — 30 секунд.

**Переопределение шаблонов.** `--template-dir dir` добавляет файлы каталога в каждый chart: `dir/x.yaml` → `templates/x.yaml`; `_helpers.tpl` и `NOTES.txt` тоже разрешены. `--template-strategy override|append|prepend` задаёт способ слияния.

---

## Остальные команды

| Команда | Назначение |
|---|---|
| `dhg validate -f <chart> [--kube-versions 1.27-1.32]` | Chart.yaml и values.yaml; синтаксис всех шаблонов (парсер Go-шаблонов, функции Helm разрешены); API, удалённые или ещё недоступные в целевых версиях K8s, — ошибки, устаревшие — предупреждения. Кластер не нужен |
| `dhg analyze -f <path> [--output-format text\|json\|markdown]` | Связи, найденные паттерны и рекомендации без генерации |
| `dhg graph -f <path> [--format dot\|mermaid]` | Граф зависимостей в stdout; в stderr — связность групп и циклы. `dhg graph -f m \| dot -Tsvg > g.svg` |
| `dhg diff <dir1> <dir2>` | Построчное сравнение двух chart'ов |
| `dhg fix -f <path>` | Генерирует chart и применяет исправления best practices: securityContext (PSS restricted), resources по профилю `--workload-type`, probes, PDB |
| `dhg migrate --from <chart> -f <manifests> --chart-name n` | Drift между существующим chart'ом и сгенерированным заново, план миграции |
| `dhg features` | Список возможностей для `--with` |

---

## Как проверяется результат

`tests/golden` собирает настоящий `dhg` и прогоняет его на всех примерах и фикстурах репозитория во всех режимах, со всеми флагами пост-обработки и со всеми возможностями `--with`. Каждый результат проверяется так:

1. `helm lint --strict` и `helm template` (с `--include-crds`), включая каждый `values-*.yaml`.
2. Число отрендеренных объектов не меньше числа входных: ничего не потерялось молча.
3. Селекторы Deployment/StatefulSet/DaemonSet совпадают с метками pod-шаблона (иначе API-сервер отклонит объект).
4. **Целостность.** Ссылка, которая разрешалась во входных манифестах, разрешается и после рендеринга:
   - ConfigMap, Secret, PVC и ServiceAccount в pod'ах;
   - backend'ы Ingress, `serviceName` у StatefulSet, `roleRef`, цель HPA.

   Service, PDB и NetworkPolicy, выбиравшие pod'ы, продолжают их выбирать.
5. **Точность (fidelity).** В режимах universal/separate/library/umbrella рендер со значениями по умолчанию содержит каждое поле входных манифестов с тем же значением: метки, данные ConfigMap байт в байт, нулевые значения (`replicas: 0`, `enabled: false`), image digest, весь `spec` custom resources.
6. `helm unittest` для сгенерированных тестов (если установлен плагин), запуск post-renderer'а через `helm template --post-renderer` и `kustomize build` каждого overlay `--kustomize` (если есть kustomize или kubectl).
7. **Источники.** Генерация из fake API-сервера с «шумом» живого кластера и из локального git-репозитория.

```bash
DHG_REQUIRE_HELM=1 go test ./tests/golden/   # без helm набор пропускается; с DHG_REQUIRE_HELM=1 — падает
```

---

## Ограничения

- **Аутентификация в кластере**: client-cert, `token`/`tokenFile` и exec-плагины kubeconfig (`client.authentication.k8s.io/v1`, `v1beta1`: kubelogin для OIDC/Dex, облачные CLI). Устаревший `auth-provider` не поддержан (как и в kubectl). Токен plugin'а запрашивается один раз на запуск.
- **Один namespace на релиз.** Объекты рендерятся в namespace релиза; исключение — одноимённые объекты из разных namespace, они сохраняют исходный (dhg печатает `Note:`).
- **Secrets** попадают в `values.yaml` в base64; для хранения в Git используйте `--with external-secrets` или `vault-agent`.

---

## Разработка

Требования: Go 1.26+, Helm 3.x (для `tests/golden`), опционально kustomize и плагин helm-unittest.

```bash
make build          # ./bin/dhg
make test           # все тесты; tests/golden пропускается без helm
make golden         # golden-набор, helm обязателен
make lint           # golangci-lint v2
```

Архитектура конвейера (`cmd/dhg/pipeline.go`):

```
extractor (file | cluster | gitops) → дедупликация
  → processor.Registry (процессоры по GVK + плагины; общий fallback для неизвестных kinds)
  → analyzer (детекторы связей → граф → сервисные группы)
  → generator (universal | separate | library | umbrella)
  → пост-обработка (флаги generate, features --with, --template-dir) → запись на диск
```

Новый вид ресурса — процессор в `pkg/processor/k8s`, регистрация в `RegisterAll`. Новая опциональная возможность — `RegisterFeature` в `pkg/generator/features_*.go`: golden-набор автоматически прогонит её на всех входах.

---

## Лицензия

Apache License 2.0 — см. [LICENSE](LICENSE). Автор — [Alex Gromer](https://github.com/AlexGromer).
