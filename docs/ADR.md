# Архитектурные решения (ADR) — DHG

> **Тип:** Справочник
> **Аудитория:** участники разработки, архитекторы
> **Последнее обновление:** 2026-10-07
> **Связанные документы:** [README.md](../README.md), [DEVELOPER.md](DEVELOPER.md)

## Обзор

В этом файле собраны все Architecture Decision Records проекта deckhouse-helm-generator. Каждая запись документирует значимое техническое решение: контекст, который его потребовал, что именно было решено, и текущий статус.

Статусы: **Accepted** (принято, в действии), **Proposed** (предложено, ещё не реализовано), **Superseded by ADR-NNN** (заменено более поздним решением, указан его номер), **Rejected** (не принято: код пошёл другим путём), **Deferred** (отложено: кода нет, решение остаётся возможным планом). Пометка _Факт: …_ в колонке «Контекст» фиксирует, что показала сверка с кодом.

---

## Фазы 1–3: Основной пайплайн и безопасность (ADR-001 – ADR-013)

| ADR | Дата | Решение | Статус | Контекст |
|-----|------|---------|--------|---------|
| ADR-001 | 2026-02-26 | Go + Cobra CLI framework | Accepted | Дистрибуция в виде одного бинарного файла, богатая stdlib, Cobra — де-факто стандарт для Go CLI |
| ADR-002 | 2026-02-26 | GoReleaser для релизов, GHCR и Homebrew tap | Accepted | Автоматизирует multi-platform сборку, публикацию Docker образа и формулу Homebrew в одной конфигурации |
| ADR-003 | 2026-02-26 | Plugin-style processor registry с маршрутизацией на основе GVK | Accepted | Разделяет обработку ресурсов и оркестрацию пайплайна; позволяет добавлять процессоры без изменения main.go |
| ADR-004 | 2026-02-26 | Test-driven development (TDD) для всех генераторов Phase 2+ | Accepted | Генераторы — чистые функции, удобные для тестирования; TDD предотвращает регрессии по мере роста числа генераторов |
| ADR-005 | 2026-02-27 | Генераторы Phase 2 разбиты на 3 уровня (infrastructure, detection, advanced) | Accepted | Управляет рисками поставки; каждый уровень релизится отдельным PR-батчем |
| ADR-006 | 2026-03-27 | Релиз v0.7.0 с известными некритичными находками (code review #29) | Accepted | 71 находка отслеживается в issue #29; P1/P2 исправлены до релиза, P3+ в последующих патчах |
| ADR-007 | 2026-03-27 | Общий `sanitize.go` для всей валидации пользовательского ввода | Accepted | Централизует `validateShellSafe()` и `validateResourceName()`, используемые в airgap, kustomize и других генераторах |
| ADR-008 | 2026-03-27 | Copy-on-write обязателен для всех мутаций `GeneratedChart` | Accepted | Предотвращает скрытые баги-алиасинги при работе нескольких post-processor'ов с одним struct chart |
| ADR-009 | 2026-03-28 | Scaffold тестов helm-unittest генерируется через `helmtest.go` | Accepted | Позволяет DHG генерировать готовые к запуску файлы тестов chart вместе с шаблонами |
| ADR-010 | 2026-03-28 | Обновление Go 1.24 → 1.26 (Go 1.24 достиг EOL) | Accepted | Go 1.26 — последняя стабильная версия; матрица CI покрывает 1.25 и 1.26 |
| ADR-011 | 2026-03-28 | Security-генераторы Phase 2.5 как pipeline post-processor'ы (copy-on-write, тот же паттерн, что в Phase 2) | Accepted | Сохраняет composable и независимо тестируемую генерацию безопасности; не требует нового этапа пайплайна |
| ADR-012 | 2026-03-28 | Процессоры Deckhouse Phase 3 следуют паттерну plugin registry (ADR-003) | Accepted | Deckhouse CRD — просто процессоры, не требующие специальной обработки в registry или пайплайне |
| ADR-013 | 2026-03-28 | Экстракторы Phase 4 как подключаемые реализации источников (file / cluster / gitops) | Accepted | Интерфейс Extractor использует Go-каналы для стриминга; тип источника — ключ registry, а не switch statement |

---

## Фаза 4: Расширение источников (ADR-014 – ADR-016)

| ADR | Дата | Решение | Статус | Контекст |
|-----|------|---------|--------|---------|
| ADR-014 | 2026-03-28 | Использовать динамический client `k8s.io/client-go` для извлечения из cluster | Superseded by ADR-051 | `k8s.io/apimachinery` уже присутствует; `client-go` разделяет транзитивные зависимости. Встроенная аутентификация (kubeconfig, OIDC, exec, in-cluster), пагинация и обнаружение CRD оправдывают увеличение бинарного файла на ~15–20 МБ. _Факт: `client-go` нет в `go.mod`; кластерный экстрактор обращается к API по REST (`net/http`, `pkg/extractor/cluster.go`)_ |
| ADR-015 | 2026-03-28 | Использовать `go-git/v5` для git-операций; вызов бинарного файла `kustomize` через shell-exec для kustomize build | Superseded by ADR-051 | Pure Go клонирование работает в scratch Docker образах; `kustomize build` в любом случае требует внешний бинарный файл. _Факт: `go-git` нет в `go.mod`; gitops-экстрактор делает `git clone --depth 1` через git CLI и читает YAML из клона (`pkg/extractor/gitops.go`), `kustomize build` не вызывается_ |
| ADR-016 | 2026-03-28 | Multi-source merge как отдельный компонент `merger.go`; стратегия по умолчанию `file-wins`; без three-way merge в v1 | Deferred | Дедупликация по `ResourceKey`; конфликты создают структуры `MergeConflict`. Three-way merge отложен — высокая сложность, низкая ROI для v1. _Факт: `--source` принимает один тип, слияния источников нет. `pkg/extractor/merger.go` содержит только `Deduplicate`: дубликаты по `ResourceKey` внутри одного источника, побеждает первое вхождение, выводится предупреждение; стратегий и `MergeConflict` нет_ |

---

## Фаза 5: Расширенный анализ (ADR-017 – ADR-024)

| ADR | Дата | Решение | Статус | Контекст |
|-----|------|---------|--------|---------|
| ADR-017 | 2026-03-28 | `dhg fix` как самостоятельная Cobra-команда (не `generate --fix`) | Accepted | `fix` мутирует входные манифесты; `generate` создаёт Helm chart — разные I/O-контракты требуют отдельных команд. _Факт: `dhg fix` (`cmd/dhg/main.go`) не изменяет входные файлы: строит chart, применяет `generator.ApplyAllFixes` и пишет результат в `--output` (по умолчанию `./fixed`)_ |
| ADR-018 | 2026-03-28 | `GenericCRDProcessor` как динамический fallback для неизвестных CRD | Accepted | Обходит поля `spec` в runtime; генерирует шаблон `{{ toYaml .Values.<kind>.spec }}`. Типизированные процессоры остаются для известных типов (ADR-003). _Факт: реализован не отдельным типом, а как `Registry.processGeneric` (`pkg/processor/registry.go`): каждое поле верхнего уровня, кроме `apiVersion`/`kind`/`metadata`/`status`, рендерится через `toYaml` из `.Values.services.<svc>.<kind>.<field>`_ |
| ADR-019 | 2026-03-28 | Нативная генерация DOT-текста — без зависимости от библиотеки graphviz | Accepted | DOT — тривиальный текстовый формат; пользователи рендерят через `dot -Tpng`. Исключает CGo-зависимость и сохраняет переносимость бинарного файла. _Факт: `dhg graph --format dot` (по умолчанию; также `mermaid`), `analyzer.GenerateDOTGraph` (`pkg/analyzer/graph.go`); graphviz в `go.mod` нет_ |
| ADR-020 | 2026-03-28 | Один файл на каждый secret provider в `pkg/generator/secrets/`; общий интерфейс `SecretStrategy` | Superseded by ADR-046 | ESO, Sealed Secrets, Vault CSI, Vault Agent, SOPS, Reloader изолированы. Выбор через флаг `--secret-strategy`. _Факт: каталога `pkg/generator/secrets/`, интерфейса `SecretStrategy` в генераторах и флага `--secret-strategy` нет. ESO, Vault Agent и Reloader подключаются как features `--with external-secrets`, `vault-agent`, `reloader`; Sealed Secrets, Vault CSI и SOPS не реализованы_ |
| ADR-021 | 2026-03-28 | Генераторы service mesh (Istio, Linkerd) как pipeline post-processor'ы (copy-on-write) | Accepted | Следует паттерну Phase 2/2.5 — добавляет шаблоны и values в `GeneratedChart`; не требует нового этапа пайплайна. _Факт: features `--with istio` и `--with linkerd` (ADR-046); `Feature.Apply` не изменяет входной chart_ |
| ADR-022 | 2026-03-28 | Плагинная система через subprocess JSON (stdin/stdout); не Go `plugin.Open` и не WASM | Accepted | Go-плагины требуют совпадения версий Go и работают только на Linux/macOS. Subprocess-паттерн проверен (Terraform, Helm, kubectl plugins). _Факт: флаг `--plugin <apiVersion>/<Kind>=<executable>`, `processor.PluginProcessor` (`pkg/processor/plugin.go`): JSON через stdin/stdout, приоритет выше встроенных процессоров_ |
| ADR-023 | 2026-03-28 | Worker pool на каждый этап пайплайна; параллельность в этапах Process и Generate; последовательный Analyze | Deferred | Параллелизм на уровне ресурсов в Stage 2 (процессоры независимы). Построение графа в Stage 3 должно быть последовательным. Ограниченный goroutine pool (`GOMAXPROCS`). _Факт: worker pool нет — `cmd/dhg/pipeline.go` обрабатывает ресурсы последовательным циклом; goroutine есть только в экстракторах для стриминга (ADR-013)_ |
| ADR-024 | 2026-03-28 | GoDoc на `pkg.go.dev` как основная документация API; без отдельного docs-сайта для v1.0.0 | Accepted | Автоматически публикуется из публичного Go-модуля; минимальные затраты на поддержку. Пересмотреть при росте экосистемы плагинов. _Факт: v1.0.0 вышел без docs-сайта; API описан комментариями пакетов (`// Package` нет у `pkg/processor/k8s` и `pkg/testutil`)_ |

---

## Фазы 5.5–5.9: Умный анализ, шаблоны, секреты, mesh, наблюдаемость (ADR-039 – ADR-044)

| ADR | Дата | Решение | Статус | Контекст |
|-----|------|---------|--------|---------|
| ADR-039 | 2026-03-29 | Глубина валидации JSON Schema ограничена 5 уровнями вложенности | Accepted | Предотвращает комбинаторный взрыв при генерации схемы; покрывает все практические структуры Helm values |
| ADR-040 | 2026-03-29 | Типизированный struct `PostRendererEnv` вместо встраивания `KustomizeDir` в генератор post-renderer | Accepted | Исключает aliasing struct между вызовами генератора; соответствует контракту copy-on-write (ADR-008) |
| ADR-041 | 2026-03-29 | Scaffold оператора использует чистые YAML-шаблоны, без маркеров Kubebuilder | Accepted | Маркеры Kubebuilder требуют бинарный файл `controller-gen`; чистый YAML работает везде и проще тестируется |
| ADR-042 | 2026-03-29 | Стратегии секретов (ESO, Sealed Secrets, Vault CSI, SOPS) взаимоисключающие через флаг `--secret-strategy` | Accepted | Несколько стратегий в одном chart создают конфликтующие ссылки на Secret; выбор одной стратегии исключает неоднозначность |
| ADR-043 | 2026-03-29 | Генераторы Istio и Linkerd — независимые модули, без общей абстракции | Accepted | Два mesh-решения достаточно различаются в CRD (VirtualService vs SMI), что вынужденный общий интерфейс создал бы утечку абстракции |
| ADR-044 | 2026-03-29 | Определение языка OpenTelemetry использует эвристику по имени образа, а не runtime-анализ | Accepted | Runtime-анализ требует доступа к кластеру; паттерны имён образов (`*-java`, `*-python` и т.д.) работают в file-only режиме |

---

## Фаза 6: Production Polish / v1.0.0 (ADR-022 – ADR-024)

_См. таблицу Phase 5 выше — ADR-022, ADR-023, ADR-024 затрагивают задачи v1.0.0 (плагинная система, параллелизм, документация API)._

---

## Фаза 7: Production Operations (ADR-025 – ADR-026)

| ADR | Дата | Решение | Статус | Контекст |
|-----|------|---------|--------|---------|
| ADR-025 | 2026-03-28 | Генераторы Phase 7 организованы в доменных поддиректориях внутри `pkg/generator/` (`gitops/`, `monitoring/`, `backup/`, `delivery/`) | Rejected | Плоская структура уже содержит 37 файлов генераторов; 10+ добавлений без группировки сделали бы навигацию непрактичной. _Факт: `pkg/generator/` остался плоским (58 файлов без тестов, поддиректорий нет); опциональные генераторы сгруппированы реестром features (ADR-046) и файлами `features_*.go`_ |
| ADR-026 | 2026-03-28 | Один файл Helm-шаблона на каждый тип CRD (не группировать по инструменту) | Rejected | Следует конвенции Helm; позволяет использовать per-CRD переключатели `enabled` в values. _Факт: шаблоны features группируются по инструменту: файл `istio-*.yaml` на каждый Service содержит VirtualService, DestinationRule, PeerAuthentication и AuthorizationPolicy с отдельными `enabled`; также `external-secrets.yaml`, `prometheus-rules.yaml`. Процессоры пишут файл на объект (`templates/<kind>-<name>.yaml`)_ |

---

## Фаза 8: Опыт разработчика и автоматизация (ADR-027 – ADR-029)

| ADR | Дата | Решение | Статус | Контекст |
|-----|------|---------|--------|---------|
| ADR-027 | 2026-03-28 | LSP-сервер как отдельный бинарный файл `dhg-lsp`; общие импорты `pkg/` | Deferred | LSP — долго живущий процесс с иным жизненным циклом, чем CLI; независимый цикл релизов, более лёгкая установка в IDE. _Факт: бинарного файла `dhg-lsp` нет; GoReleaser собирает только `./cmd/dhg`_ |
| ADR-028 | 2026-03-28 | Kubernetes Operator в отдельном репозитории `dhg-operator`; импортирует `dhg` как библиотеку | Deferred | Сохраняет Unix-философию CLI; оператор опционален и привлекает отдельные feature requests. _Факт: в репозитории нет кода оператора и ссылок на `dhg-operator`_ |
| ADR-029 | 2026-03-28 | Hot reload через встроенный `fsnotify` + генерируемые шаблоны `Tiltfile` / `devspace.yaml` | Deferred | `dhg dev` регенерирует chart при изменении файлов; Tilt/DevSpace управляют деплоем в кластер — чёткое разделение ответственности. _Факт: команды `dhg dev` нет, `fsnotify` нет в `go.mod`, `Tiltfile`/`devspace.yaml` не генерируются_ |

---

## Фаза 9: AI/ML нагрузки (ADR-030 – ADR-031)

| ADR | Дата | Решение | Статус | Контекст |
|-----|------|---------|--------|---------|
| ADR-030 | 2026-03-28 | AI/ML процессоры следуют паттерну plugin registry (ADR-003); организованы в `pkg/processor/k8s/ml/` | Deferred | CRD Kubeflow, KServe, Ray, Seldon регистрируют GVK штатно; группировка в поддиректории для ~8–10 файлов. _Факт: `pkg/processor/k8s/ml/` нет; процессоров Kubeflow, KServe, Ray, Seldon нет_ |
| ADR-031 | 2026-03-28 | Инъекция GPU-конфигурации как copy-on-write post-processor | Deferred | Обнаруживает GPU-паттерны в именах образов; добавляет лимиты `nvidia.com/gpu`, tolerations и emptyDir-том для `/dev/shm`. _Факт: GPU-инъекции (`nvidia.com/gpu`) в коде нет_ |

---

## Фаза 10: Операторы баз данных (ADR-032 – ADR-034)

| ADR | Дата | Решение | Статус | Контекст |
|-----|------|---------|--------|---------|
| ADR-032 | 2026-03-28 | Процессоры баз данных организованы в поддиректориях по семействам операторов (`db/postgres/`, `db/mysql/`, `db/redis/` и т.д.) | Deferred | 51 задача для 12+ операторов; плоская директория `k8s/` неприемлема при таком масштабе. _Факт: процессоров DB-операторов и `pkg/processor/k8s/db/` нет_ |
| ADR-033 | 2026-03-28 | Общие DB-интерфейсы: `BackupConfigurer`, `HAConfigurer`, `PoolerConfigurer` — реализации специфичны для каждого оператора | Deferred | Общие паттерны для всех DB-операторов; вспомогательные функции в `db/common.go` для S3-credentials и PDB-scaffold. _Факт: интерфейсов `BackupConfigurer`/`HAConfigurer`/`PoolerConfigurer` и `db/common.go` нет_ |
| ADR-034 | 2026-03-28 | 7-уровневый каскад обнаружения операторов (явные values → версия API → kind → labels → зависимости chart → паттерны ConfigMap → ссылки Secret) | Deferred | Несколько операторов для одного DB-движка требуют надёжного разграничения до GVK-маршрутизации. _Факт: каскада нет; `OperatorDetector` (`pkg/analyzer/pattern/detectors.go`) только распознаёт паттерн «CRD + controller Deployment»_ |

---

## Фаза 11: Расширенное планирование (ADR-035)

| ADR | Дата | Решение | Статус | Контекст |
|-----|------|---------|--------|---------|
| ADR-035 | 2026-03-28 | Конфигурация планирования через чекеры анализатора + генераторы-post-processor'ы; без нового этапа пайплайна | Accepted | `TopologySpreadChecker`, `PriorityClassChecker`, `AffinityChecker` добавлены в pattern analyzer; генераторы инъецируют конфигурацию после анализа. _Факт: реализовано частично — в анализаторе есть `TopologySpreadChecker`, а `PriorityClassChecker` и `AffinityChecker` нет; генератор — feature `--with anti-affinity` (podAntiAffinity, опционально zone topologySpread)_ |

---

## Фаза 12: Управление данными и CSI (ADR-036)

| ADR | Дата | Решение | Статус | Контекст |
|-----|------|---------|--------|---------|
| ADR-036 | 2026-03-28 | CSI/storage конфигурация: генераторы для автосоздания + процессоры для существующих CRD; values-based обнаружение как основное (cluster-based как будущее улучшение) | Deferred | Исключает жёсткую зависимость от cluster extractor Phase 4; флаг `--cloud-provider` предоставляет контекст хранилища уже сегодня. _Факт: генераторов и процессоров CSI/storage CRD нет, флага `--cloud-provider` нет (провайдер есть только как параметр feature `resource-report` для оценки стоимости)_ |

---

## Фаза 13: Edge Computing (ADR-037 – ADR-038)

| ADR | Дата | Решение | Статус | Контекст |
|-----|------|---------|--------|---------|
| ADR-037 | 2026-03-28 | Обнаружение K8s-дистрибутивов (K3s, MicroK8s, KubeEdge) как pattern detector анализатора | Deferred | Работает в file-only режиме через инспекцию CRD; переходит к cluster metadata при наличии extractor Phase 4. _Факт: детектора K3s/MicroK8s/KubeEdge нет_ |
| ADR-038 | 2026-03-28 | IoT CRD: процессор для существующих ресурсов + генератор для автосоздаваемых (MQTT broker, DaemonSet device mapper) | Deferred | Двойной подход зеркалирует стратегию CSI Phase 12; соответствует существующему разделению пайплайна. _Факт: процессоров и генераторов IoT CRD (MQTT broker, device mapper) нет_ |

---

## Ревизия корректности (ADR-045 – ADR-057)

| ADR | Дата | Решение | Статус | Контекст |
|-----|------|---------|--------|---------|
| ADR-045 | 2026-10-05 | Корректность результата проверяется настоящим Helm на всех входах репозитория (`tests/golden`), а не строковыми проверками | Accepted | Набор из ~2400 unit-тестов был зелёным, при этом umbrella не генерировался, separate рендерил 0 объектов, `values.schema.json` был в YAML, Deployment'ы отклонялись бы API-сервером. Golden-набор проверяет lint/template, сохранность объектов, селекторы, ссылки, unittest, post-renderer |
| ADR-046 | 2026-10-05 | Опциональные возможности — реестр features (`--with`, `--feature-opt`, `dhg features`) вместо флага на каждую | Accepted | ~60% кода генераторов было недостижимо из CLI. Реестр даёт единый механизм подключения, документированные параметры со значениями по умолчанию и автоматическое покрытие golden-набором; неподключаемый код удалён |
| ADR-047 | 2026-10-05 | `ValuesPath` процессора — единственный источник истины размещения values | Accepted | Генераторы угадывали ключ по Kind (`horizontalPodAutoscaler` против `hpa` в шаблоне) и по имени группы — HPA, PDB, Gateway, HTTPRoute молча не рендерились |
| ADR-048 | 2026-10-05 | Library-режим: library chart содержит общие helpers, chart'ы групп используют `library.*` | Accepted | Прежние 18 шаблонов `library.<kind>` читали поля, которых процессоры не производят, и рендерили значения по умолчанию (`nginx:latest`) |
| ADR-049 | 2026-10-05 | Идентичность объектов сохраняется: исходные имена, селекторы и метки pod'ов | Accepted | Переименование в `<release>-<chart>-<name>` ломало ссылки, хранящиеся в values как данные (backend'ы Ingress, ConfigMap в pod'ах, roleRef, serviceName), и селекторы PDB/NetworkPolicy. Сохранение имён также позволяет Helm перенять существующие объекты. Компромисс: два релиза одного chart'а в одном namespace конфликтуют |
| ADR-050 | 2026-10-05 | CustomResourceDefinitions пишутся в `crds/` без шаблонизации | Accepted | CRD-шаблон не существует к моменту валидации custom resources того же релиза; Helm ставит `crds/` первыми и не обновляет их |
| ADR-051 | 2026-10-05 | Источники `cluster` и `gitops` реализованы без client-go/go-git: REST по kubeconfig и `git clone` | Accepted | Минимум зависимостей; кластерный экстрактор пропускает объекты контроллеров и системные namespace (`kube-*`, `d8-*`) и чистит server-side поля. Аутентификация: client-cert, token/tokenFile и exec-плагины ExecCredential v1/v1beta1 (kubelogin для OIDC) |
| ADR-052 | 2026-10-05 | CI: только Go 1.26, golangci-lint v2 без `continue-on-error`, отдельный golden-job | Accepted (заменяет часть ADR-010) | Job «Go 1.25» фактически собирался 1.26 из-за `go 1.26` в go.mod; конфиг линтера v1 не загружался в v2, а ошибка скрывалась |
| ADR-053 | 2026-10-05 | Chart воспроизводит входные данные (fidelity): рендер со значениями по умолчанию содержит каждое поле входа | Accepted | Golden-проверка `render ⊇ input` нашла: перезапись меток, `\n`, добавленный к значениям ConfigMap, потерю нулевых значений (`replicas: 0`, `backoffLimit: 0`), поломку image digest, потерю финального `\n` блочных скаляров в экстракторе. Данные ConfigMap/Secret рендерятся как JSON-строки, нулевые поля — через `hasKey` |
| ADR-054 | 2026-10-05 | Процессоры custom resources рендерят весь `spec` (`processor.SpecOverlay`), отдельные values-ключи переопределяют его поля | Accepted | 20 процессоров рендерили 1–5 поля из белого списка и молча теряли остальные (`resourcesLimits`, `nodeTemplate`, `privateKey`, `cooldownPeriod`…). Переопределение через `set`, а не `mergeOverwrite`: mergo игнорирует нулевые значения (`enabled: false`) |
| ADR-055 | 2026-10-05 | `--kustomize`: base — входные манифесты, overlay'и патчат replicas каждого Deployment/StatefulSet по kind/name/namespace | Accepted | Прежняя раскладка ссылалась на незаписанные файлы-шаблоны Helm (`kustomize build` невозможен) и патчила Deployment с именем chart'а. Патч prod-лимитов (`cpu: 1`, `512Mi`) удалён: он перезаписывал реальные лимиты входа. Связка Helm + Kustomize — это `--post-renderer` |
| ADR-056 | 2026-10-05 | Объекты, совпадающие по пути шаблона или values, разводятся по своим сервисам; одноимённые объекты из разных namespace сохраняют исходный namespace | Accepted | Два Deployment'а с общей меткой `app` или ConfigMap с одним именем в двух namespace молча перезаписывали друг друга (4 объекта на входе → 2 на выходе). В separate/library/umbrella сервис группы, чьи пути пересекаются с основным, остаётся под своим ключом values. Каждое решение печатается как `Note:` |
| ADR-057 | 2026-10-05 | Velero Schedule выбирает объекты через `orLabelSelectors`: метки chart'а + селекторы workload'ов + метки PVC | Accepted | После ADR-049 pod'ы несут исходные метки, а PVC из `volumeClaimTemplates` — метки селектора StatefulSet, поэтому выбор по `selectorLabels` chart'а пропускал именно данные. Требует Velero ≥ 1.10 |

---

## Генерация без манифестов (ADR-058 – ADR-060)

| ADR | Дата | Решение | Статус | Контекст |
|-----|------|---------|--------|---------|
| ADR-058 | 2026-10-07 | Источники `image`, `compose`, `source` строят синтетические манифесты (`pkg/synth`), которые идут в неизменённый конвейер | Accepted | Один путь для всех входов: режимы, `--with`, golden-проверки применяются без особых случаев. Альтернатива — генерировать chart напрямую из модели — дублировала бы генераторы. См. docs/SPEC_SYNTHESIS.md |
| ADR-059 | 2026-10-07 | В chart попадают только факты входа; невыводимое (requests/limits, пароли, размеры томов, Ingress) не подставляется, а перечисляется в `SYNTHESIS.md` | Accepted | Выдуманные значения ресурсов ломают планирование и выглядят как решения. Имя порта `http` — только при известном протоколе (Spring Boot), иначе `tcp-<порт>`: имя порта управляет определением протокола в mesh |
| ADR-060 | 2026-10-07 | Конфигурация образа читается по Docker Registry HTTP API v2 на `net/http` (manifest + config, без слоёв), без `go-containerregistry` | Accepted | Как ADR-051: минимум зависимостей и контроль размера ответов (≤ 4 MiB) и digest. Цена: нет credential helpers и схем, кроме Basic/Bearer — для Docker Hub, Harbor, GitLab, Nexus и Deckhouse registry достаточно. Bearer-токен кэшируется по паре registry+scope и не уходит в другой registry. Compose разбирается по YAML 1.2 (`sigs.k8s.io/yaml/goyaml.v3`, подпакет уже подключённой зависимости), Spring `application.yml` — по YAML 1.1, как их читают сами Docker Compose v2 и SnakeYAML |

---

## Сводка по статусам

| Статус | Количество | ADR |
|--------|-----------|-----|
| Accepted | 42 | 001–013, 017–019, 021, 022, 024, 035, 039–060 |
| Superseded | 3 | 014, 015 (ADR-051), 020 (ADR-046) |
| Rejected | 2 | 025, 026 |
| Deferred | 13 | 016, 023, 027–034, 036–038 |
| Proposed | 0 | — |
| **Итого** | **60** | |

> 2026-10-07: статусы ADR-014 – ADR-038 сверены с кодом (`cmd/dhg`, `pkg/`, `go.mod`). Реализованные решения переведены в Accepted, заменённые — в Superseded, нереализованные — в Rejected или Deferred; расхождения с текстом решения описаны пометкой _Факт_ в колонке «Контекст».
>
> Deferred ADR представляют задокументированные намерения для будущих фаз: кода для них нет, детали могут измениться до принятия.
