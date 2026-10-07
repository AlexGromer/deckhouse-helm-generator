# EPIC-05: Объекты Deckhouse для приложений и Pod Security Standards

| Поле | Значение |
|---|---|
| Статус | draft (задачи 01–03, 05–07, 10, 11 — `ready`; 04, 08, 09, 12 — `draft`) |
| Приоритет | P2 |
| Размер | L (сумма задач: 5 × S, 7 × M) |
| Зависит от | — (контракты EPIC-03 не нужны; задача 12 использует только существующий граф) |
| Связанные ADR | ADR-003 (реестр процессоров), ADR-046 (`--with`), ADR-049 (идентичность), ADR-053 (fidelity), ADR-054 (`SpecOverlay`), ADR-059 (только факты); ADR-кандидаты — в [design.md](design.md#6-adr-кандидаты) |

## Проблема

dhg объявлен как генератор с «первоклассной поддержкой Deckhouse», но Deckhouse-часть почти целиком состоит из процессоров **платформенных** cluster-scoped объектов (`ModuleConfig`, `NodeGroup`, `IngressNginxController`, `ClusterAuthorizationRule`, `User`, `Group`, `*InstanceClass`; `pkg/processor/k8s/registry.go:RegisterAll`). Для объектов, которые живут в жизненном цикле **приложения** (namespaced, их создаёт команда приложения), поддержка либо отсутствует, либо работает неверно. Факты (проверено 2026-10-07 на коде этого репозитория, коммит `a4d01f2`):

1. **Логи.** `PodLoggingConfig` (`deckhouse.io/v1alpha1`, namespaced — модуль `log-shipper`) не имеет процессора: объект уходит в generic fallback (`pkg/processor/registry.go:processGeneric`), поля сохраняются, но объект не связывается с workload'ом, который он выбирает (`spec.labelSelector`), и попадает в отдельную сервисную группу. Создать `PodLoggingConfig` для приложения, у которого его нет, нельзя.
2. **Мониторинг.**
   - Prometheus в Deckhouse выбирает `ServiceMonitor`/`PodMonitor` только с меткой `prometheus: main` и только в namespace с меткой `prometheus.deckhouse.io/monitor-watcher-enabled: "true"`; `PrometheusRule` — с метками `prometheus: main` + `component: rules` и в namespace с `prometheus.deckhouse.io/rules-watcher-enabled: "true"` (см. [spec.md §9](spec.md#9-проверенные-факты-и-источники)). Возможность `--with prometheus-rules` (`pkg/generator/alertingrules.go:buildPrometheusRulesTemplate`) ставит только метки chart'а и пустой `prometheusRules.labels`, поэтому её `PrometheusRule` в Deckhouse **молча игнорируется**. dhg нигде не сообщает о нужных метках namespace.
   - `GrafanaDashboardProcessor` (`pkg/processor/k8s/grafanadashboard.go`) обрабатывает ConfigMap с меткой `grafana_dashboard: "1"` — это соглашение sidecar'а kube-prometheus-stack; в документации Deckhouse такого механизма нет. Механизмы Deckhouse: `GrafanaDashboardDefinition` (cluster-scoped, «legacy, will be removed») и namespaced `ObservabilityDashboard` модуля `observability` (рекомендуемый).
3. **DexAuthenticator.** Процессор (`pkg/processor/k8s/dexauthenticator.go`) регистрирует только `deckhouse.io/v1`, хотя CRD обслуживает ещё `v2alpha1`. Связь «Ingress приложения → DexAuthenticator» ищется по аннотациям `deckhouse.io/*`, содержащим `auth`/`dex` (`pkg/analyzer/detector/annotation.go:173–191`), а реальная связь, описанная Deckhouse, — аннотация `nginx.ingress.kubernetes.io/auth-url: https://<NAME>-dex-authenticator.<NS>.svc.<домен>/dex-authenticator/auth`. Она не распознаётся.
4. **Pod Security Standards** (PSS, стандарты безопасности pod'ов Kubernetes). В Deckhouse их применяет модуль `admission-policy-engine` (Gatekeeper), уровень задаётся меткой namespace `security.deckhouse.io/pod-policy`, по умолчанию для установок с v1.55 — `Baseline` в режиме `Deny`. dhg не умеет сказать, какому уровню соответствуют workload'ы chart'а:
   - синтез (`pkg/synth/manifests.go:securityContext`) ставит только `runAsUser`/`runAsNonRoot` — `Baseline` выполняется, `Restricted` нет (нет `seccompProfile`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`);
   - `dhg fix` (`pkg/generator/pss.go:injectSecurityContext`) вставляет блок `securityContext` после **первой** строки `image:` шаблона. В шаблонах dhg первая такая строка лежит в блоке `{{- with .initContainers }}` (`pkg/processor/k8s/podtemplate.go:231`), поэтому основные контейнеры не получают ничего. Проверено: `dhg fix -f examples/01-simple-web` → в `helm template` у Deployment нет `securityContext`. Кроме того, уровень `baseline` в `pss.go` требует `runAsNonRoot` — это контроль уровня `Restricted`, а `readOnlyRootFilesystem` вообще не входит в PSS.
5. **Исключения политик.** `SecurityPolicyException` (`deckhouse.io/v1alpha1`) — namespaced объект приложения; на него ссылаются метки pod-шаблона `security.deckhouse.io/security-policy-exception[.container.<имя>]`. dhg не видит этой ссылки: golden-проверка целостности не заметит, что исключение пропало из chart'а.

## Цель и результат

Команда приложения получает chart, который в кластере Deckhouse **работает с первого раза** в части логов, мониторинга, аутентификации и PSS, а всё, что требует решения платформы (метки namespace, имена `ClusterLogDestination`), перечислено в отчёте, а не выдумано.

```bash
dhg generate -f ./manifests --chart-name orders -o ./charts \
  --with deckhouse-report,deckhouse-logging,prometheus-rules \
  --feature-opt deckhouse-logging.destinations=loki-storage \
  --feature-opt prometheus-rules.labels=prometheus=main,component=rules
```

Фрагмент `charts/orders/docs/deckhouse-report.md`:

```markdown
## Pod Security Standards (цель: baseline)
| Workload | Уровень | Нарушения целевого уровня |
|---|---|---|
| Deployment/orders | restricted | — |
| Deployment/legacy | privileged | hostPath volume `data` (spec.volumes[0].hostPath) |

## Метки namespace, которые нужны chart'у
| Метка | Зачем | Объекты |
|---|---|---|
| prometheus.deckhouse.io/monitor-watcher-enabled: "true" | ServiceMonitor/PodMonitor | ServiceMonitor/orders |
| prometheus.deckhouse.io/rules-watcher-enabled: "true" | PrometheusRule | PrometheusRule/orders-rules |
| security.deckhouse.io/pod-policy: privileged | минимальный уровень, которому соответствуют все workload'ы | Deployment/legacy |
```

## Scope

- В scope:
  - namespaced объекты Deckhouse из жизненного цикла приложения: `PodLoggingConfig`, `DexAuthenticator`, `SecurityPolicyException`, `ObservabilityDashboard`, `ObservabilityMetricsRulesGroup`; метки `ServiceMonitor`/`PodMonitor`/`PrometheusRule`, нужные Prometheus Deckhouse;
  - проверка соответствия workload'ов уровням PSS (`Baseline`/`Restricted`) и отчёт;
  - исправление `dhg fix` (PSS) и opt-in-возможность ужесточения `securityContext` через values;
  - проверка совместимости workload'ов с уже существующими в кластере `OperationPolicy` (только чтение, отчёт).
- Вне scope:
  - `ClusterLogDestination`, `ClusterLoggingConfig`, `GrafanaDashboardDefinition`, `CustomPrometheusRules`, `ClusterObservabilityDashboard`, `ClusterObservabilityMetricsRulesGroup`, `OperationPolicy`, `SecurityPolicy`, `ModuleConfig` — cluster-scoped объекты платформы (граница проекта, [roadmap README §1](../README.md#1-граница-проекта)). dhg их **не создаёт**; если они пришли во входе, существующее поведение сохраняется, а отчёт указывает namespaced-альтернативу;
  - метки `Namespace` (`security.deckhouse.io/pod-policy`, `prometheus.deckhouse.io/*-watcher-enabled`): namespace принадлежит платформе или проекту `multitenancy-manager`; dhg только сообщает, какие метки нужны;
  - `DexClient` и OIDC-клиенты — [EPIC-08](../EPIC-08-keycloak/README.md);
  - профиль генерации «Deckhouse» (набор возможностей одной командой) — [EPIC-11](../EPIC-11-generation-profiles/README.md), он собирается из возможностей этого эпика.

## Задачи и порядок

| # | Задача | Размер | Зависит от | Статус |
|---|---|---|---|---|
| 01 | [Пакет проверки Pod Security Standards](tasks/01-pss-evaluator.md) | M | — | ready |
| 02 | [Возможность `deckhouse-report` и фикстура `deckhouse-observability`](tasks/02-deckhouse-report.md) | M | 01 | ready |
| 03 | [Значения по умолчанию `securityContext` через `global` и исправление `dhg fix`](tasks/03-security-context-defaults.md) | M | 01 | ready |
| 04 | [Возможность `pss-restricted` (opt-in ужесточение)](tasks/04-pss-restricted-feature.md) | S | 03 | draft |
| 05 | [Процессор `PodLoggingConfig`](tasks/05-podloggingconfig-processor.md) | S | 02 | ready |
| 06 | [Возможность `deckhouse-logging`](tasks/06-deckhouse-logging-feature.md) | M | 05 | ready |
| 07 | [Параметр `labels` у `prometheus-rules`](tasks/07-prometheus-rules-labels.md) | S | — | ready |
| 08 | [Дашборды: `ObservabilityDashboard` из входных дашбордов](tasks/08-observability-dashboards.md) | M | 02 | draft |
| 09 | [Правила: `ObservabilityMetricsRulesGroup` в `prometheus-rules`](tasks/09-observability-rules.md) | S | 07 | draft |
| 10 | [DexAuthenticator: `v2alpha1`, связь с Ingress, проверки](tasks/10-dexauthenticator.md) | M | 02 | ready |
| 11 | [Ссылки на `SecurityPolicyException`](tasks/11-security-policy-exception.md) | S | 02 | ready |
| 12 | [Совместимость с `OperationPolicy` кластера](tasks/12-operation-policy-check.md) | M | 02 | draft |

Рекомендуемый порядок: 01 → 02 → (05, 07, 10, 11 параллельно) → 03 → 06 → по решениям владельца 04, 08, 09, 12.

## Риски и открытые вопросы

| Вопрос | Варианты | Рекомендация | Кто решает |
|---|---|---|---|
| Q1. Может ли dhg **добавлять** в chart поля `securityContext`, которых нет во входе (ужесточение до `Restricted`)? Это не факты (ADR-059), а безопасные значения по умолчанию, которые могут сломать приложение (например, бинарь, которому нужна capability). | (а) никогда — только отчёт; (б) opt-in `--with pss-restricted`, значения в `global.securityContextDefaults`, вход побеждает; (в) по умолчанию | (б): ничего не меняется без явного `--with`, всё видно и выключается в values. Задача 04 — `draft` до решения | владелец |
| Q2. Включён ли модуль `observability` в кластерах владельца? Он доступен в редакциях CE, BE, SE, SE+, EE, CSE Pro, но **не** в CSE Lite (`modules-addition.json`), и это внешний модуль — его CRD нет в репозитории `deckhouse/deckhouse`. | (а) да — генерировать `ObservabilityDashboard`/`ObservabilityMetricsRulesGroup`; (б) нет — только `PrometheusRule` с метками и отчёт | Уточнить редакцию и версию DKP; до ответа задачи 08, 09 — `draft` | владелец |
| Q3. Ключ аннотаций категории/заголовка `ObservabilityDashboard`: в таблице миграции документации — `observability.deckhouse.io/category|title`, в примерах той же страницы и в `deckhouse_lib_helm` 1.72.24 — `metadata.deckhouse.io/category|title`. | (а) `metadata.deckhouse.io/*`; (б) оба | (а), как в коде Deckhouse (`helm_lib`); проверить `kubectl explain`/поведение в кластере владельца перед реализацией задачи 08 | исполнитель задачи 08 (проверка) |
| Q4. Входной `GrafanaDashboardDefinition`/`CustomPrometheusRules` (cluster-scoped) — оставлять в chart'е как есть или заменять namespaced-аналогом? | (а) оставлять + отчёт; (б) заменять при `--with`; (в) удалять | (а) по умолчанию (fidelity, ADR-053), (б) — параметр `replace-legacy=true` задачи 08 | владелец |
| Q5. Откуда брать `OperationPolicy` для проверки совместимости (задача 12)? | (а) файл/каталог `--feature-opt deckhouse-report.policies=<путь>`; (б) чтение из кластера при `-s cluster`; (в) оба | (а): работает без доступа к кластеру; (б) позже | владелец |
| Q6. Целевой уровень PSS по умолчанию в отчёте. | `baseline` (умолчание DKP с v1.55) / `restricted` | `baseline`, параметр `pss-level` | владелец |
| Q7. Схемы CRD `PodLoggingConfig`, `GrafanaDashboardDefinition`, `CustomPrometheusRules` проверены по тегу `v1.69.0`: модули `log-shipper` и `prometheus` с тех пор вынесены во внешние модули. Изменились ли поля в актуальной версии DKP владельца — **не проверено**. | — | Перед реализацией задач 05, 06 выполнить `kubectl explain podloggingconfig.spec --recursive` в кластере владельца | исполнитель |
