# EPIC-05: спецификация

## 1. Термины

| Термин | Значение |
|---|---|
| DKP | Deckhouse Kubernetes Platform. |
| PSS | Pod Security Standards — три уровня ограничений pod'а Kubernetes: `Privileged` (без ограничений), `Baseline` (запрет известных путей повышения привилегий), `Restricted` (ужесточённый). Источник — документация Kubernetes ([§9](#9-проверенные-факты-и-источники), F14). |
| Контроль PSS | Одна строка таблицы стандарта: поле(я) pod'а и допустимые значения (например, «Host Namespaces»: `spec.hostNetwork|hostPID|hostIPC` ∈ {не задано, `false`}). |
| `admission-policy-engine` | Модуль DKP, применяющий PSS и операционные политики через Gatekeeper (admission-контроллер; не встроенный Pod Security Admission Kubernetes). |
| Метка namespace (watcher label) | Метка `Namespace`, по которой Prometheus DKP решает, читать ли объекты мониторинга из namespace: `prometheus.deckhouse.io/monitor-watcher-enabled`, `…/rules-watcher-enabled`, `…/scrape-configs-watcher-enabled`, `…/probe-watcher-enabled`. |
| Платформенный объект | Cluster-scoped объект платформенной команды (граница проекта). |
| Workload | Объект с pod-шаблоном: Deployment, StatefulSet, DaemonSet, ReplicaSet, Job, CronJob, Pod, Argo `Rollout` (`argoproj.io/v1alpha1`, `spec.template`). |
| Отчёт Deckhouse | Файл `docs/deckhouse-report.md` в каталоге chart'а, который пишет возможность `deckhouse-report` (по образцу `docs/resource-report.md` возможности `resource-report`). |

## 2. Требования

### PSS

- **R1.** Пакет `pkg/compliance/pss` по pod spec'у (map, как в unstructured) вычисляет наивысший уровень PSS (`privileged` < `baseline` < `restricted`), которому pod соответствует, и список нарушений для каждого уровня. Контроли — ровно таблица F14 (Kubernetes PSS, версия страницы на 2026-10-07): Baseline — HostProcess, Host Namespaces, Privileged Containers, Capabilities (add ⊆ 13 разрешённых), HostPath Volumes, Host Ports (только не задано или `0`), Host Probes/Lifecycle Hooks (`host` пусто), AppArmor, SELinux, `/proc` Mount Type, Seccomp (не `Unconfined`), Sysctls (безопасный список); Restricted — всё из Baseline + Volume Types, Privilege Escalation (`allowPrivilegeEscalation: false` обязательно), Running as Non-root (pod или все контейнеры `runAsNonRoot: true`), Running as Non-root user (`runAsUser` ≠ 0), Seccomp (`RuntimeDefault`/`Localhost` задан на уровне pod или всех контейнеров), Capabilities (`drop` содержит `ALL`, `add` ⊆ {`NET_BIND_SERVICE`}). Контейнеры: `containers`, `initContainers`, `ephemeralContainers`. Контроли, помеченные «Linux only» (Privilege Escalation, Seccomp, Capabilities уровня Restricted), не применяются при `spec.os.name: windows`.
- **R2.** `readOnlyRootFilesystem` **не** является контролем PSS и не влияет на уровень.
- **R3.** Возможность `deckhouse-report` (`--with deckhouse-report`) пишет `docs/deckhouse-report.md` для каждого chart'а (кроме library): по одной строке на каждый workload chart'а — уровень PSS и нарушения целевого уровня (параметр `pss-level`, по умолчанию `baseline`). Оценивается pod spec **входного** объекта (по ADR-053 он совпадает с рендером значений по умолчанию).
- **R4.** Отчёт рекомендует значение метки `security.deckhouse.io/pod-policy` — минимальный из уровней workload'ов chart'а (строчными буквами: `privileged`/`baseline`/`restricted`, как в документации DKP).
- **R5.** Отчёт перечисляет workload'ы, у которых на верхнем уровне `metadata.labels` нет, а в pod-шаблоне есть `security.deckhouse.io/skip-pss-check` (DKP проверит контроллер, исключение сработает только для pod'а — F13).

### Мониторинг

- **R6.** Отчёт перечисляет метки namespace, без которых объекты chart'а не будут прочитаны Prometheus DKP: `monitor-watcher-enabled` — если в chart'е есть `ServiceMonitor` или `PodMonitor`; `rules-watcher-enabled` — `PrometheusRule`; `scrape-configs-watcher-enabled` — `ScrapeConfig`; `probe-watcher-enabled` — `Probe`. Объекты — и входные, и добавленные возможностями (шаблоны chart'а).
- **R7.** Отчёт перечисляет `ServiceMonitor`, `PodMonitor`, `Probe`, `ScrapeConfig` без метки `prometheus: main` и `PrometheusRule` без пары `prometheus: main` + `component: rules` (метки берутся из входа; для шаблонов возможностей — из values по умолчанию).
- **R8.** У возможности `prometheus-rules` появляется параметр `labels` (`k=v[,k=v…]`); значения записываются в `prometheusRules.labels`. Пустое значение — поведение не меняется.
- **R9.** Отчёт перечисляет ConfigMap с меткой `grafana_dashboard: "1"` с пояснением: механизм sidecar kube-prometheus-stack, в DKP документирован не он, а `ObservabilityDashboard` (или legacy `GrafanaDashboardDefinition`).

### Логи

- **R10.** Процессор `PodLoggingConfig` (`deckhouse.io/v1alpha1`) рендерит весь `spec` (`processor.SpecOverlay`) с ключом удобства `clusterDestinationRefs`, namespace — namespace релиза. Сервисная группа выводится из `spec.labelSelector.matchLabels` теми же ключами, что `processor.ServiceNameFromLabels` (`app.kubernetes.io/name`, `app.kubernetes.io/instance`, `app`, `name`, `component`), иначе — как у generic-процессора.
- **R11.** Детектор меток создаёт связь `label_selector` от `PodLoggingConfig` к workload'ам того же namespace, метки pod-шаблона которых удовлетворяют `spec.labelSelector` (`matchLabels` и `matchExpressions`, через `metav1.LabelSelectorAsSelector`). `PodLoggingConfig` без `labelSelector` выбирает все pod'ы namespace — связь со всеми workload'ами не строится (это не связь «приложение → объект»), отчёт пишет «selects every pod in the namespace».
- **R12.** Возможность `deckhouse-logging` создаёт по одному `PodLoggingConfig` на каждый workload chart'а, у которого `spec.selector.matchLabels` не пуст и который ещё не выбран входным `PodLoggingConfig`: `metadata.name` = имя workload'а, `spec.labelSelector.matchLabels` = `spec.selector.matchLabels` workload'а, `spec.clusterDestinationRefs` = `deckhouseLogging.clusterDestinationRefs`. При пустом списке назначений шаблон ничего не рендерит (CRD требует `minItems: 1`).

### DexAuthenticator

- **R13.** Процессор `DexAuthenticator` поддерживает `deckhouse.io/v1` и `deckhouse.io/v2alpha1` (оба версии CRD `served: true`, F11). Шаблон рендерит `apiVersion` входа.
- **R14.** Детектор аннотаций связывает Ingress (`networking.k8s.io/v1`) с `DexAuthenticator` того же namespace, если хост URL в аннотации `nginx.ingress.kubernetes.io/auth-url` равен `<metadata.name DexAuthenticator>-dex-authenticator.<namespace>.svc[.<домен>]`; аналогично для HTTPRoute и аннотации `alb.network.deckhouse.io/auth-url`.
- **R15.** Отчёт Deckhouse для каждого DexAuthenticator сообщает: связанные Ingress/HTTPRoute (или «не найдено — приложение не защищено»), отсутствие во входе Secret из `applicationIngressCertificateSecretName` (или `applications[].ingressSecretName` для `v2alpha1`), несовпадение `applicationDomain` с `spec.rules[].host` связанного Ingress, отсутствие `spec.tls` у связанного Ingress (DexAuthenticator работает только по HTTPS, F11).

### Исключения и политики

- **R16.** Детектор ссылок создаёт связь `name_reference` от workload'а к `SecurityPolicyException` того же namespace по меткам pod-шаблона `security.deckhouse.io/security-policy-exception` и `security.deckhouse.io/security-policy-exception.container.<имя>`. Golden-проверка целостности считает эту ссылку обязательной к разрешению.
- **R17.** (draft) Отчёт проверяет workload'ы против `OperationPolicy` из файла (`deckhouse-report.policies`): поля `allowedRepos`, `requiredResources`, `disallowedImageTags`, `requiredProbes`, `requiredLabels`, `maxRevisionHistoryLimit`, `imagePullPolicy`, `priorityClassNames`, `ingressClassNames`, `storageClassNames`, `replicaLimits` с учётом `spec.match.labelSelector`.

### Значения по умолчанию securityContext

- **R18.** Шаблон pod'а dhg объединяет `securityContext` входа с `global.securityContextDefaults.pod`, а `securityContext` каждого контейнера (включая init) — с `global.securityContextDefaults.container`; значения входа побеждают (Sprig `merge`, глубокое слияние map'ов). Без этих ключей рендер побайтно совпадает с текущим.
- **R19.** `dhg fix` устанавливает `global.securityContextDefaults` вместо текстовой вставки после `image:`; все контейнеры отрендеренного Deployment'а `examples/01-simple-web` получают `securityContext`.
- **R20.** (draft) Возможность `pss-restricted` записывает в `global.securityContextDefaults` значения, закрывающие контроли Restricted, которые не зависят от образа: pod — `seccompProfile.type: RuntimeDefault`; контейнер — `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`. `runAsNonRoot` не ставится (зависит от пользователя образа); отчёт R3 показывает оставшиеся нарушения.

### Observability (draft)

- **R21.** (draft) Возможность `deckhouse-dashboards` для каждого входного дашборда (ключ `data` ConfigMap с `grafana_dashboard: "1"`; `spec.definition` входного `GrafanaDashboardDefinition`) добавляет `ObservabilityDashboard` (`observability.deckhouse.io/v1alpha1`) в namespace релиза.
- **R22.** (draft) Параметр `kind` возможности `prometheus-rules`: `PrometheusRule` (по умолчанию) или `ObservabilityMetricsRulesGroup` (по объекту на группу правил, `spec` = группа без `name`).

## 3. Входы

| Источник | Факт | Правило |
|---|---|---|
| pod spec workload'а (`spec.template.spec`, у CronJob `spec.jobTemplate.spec.template.spec`, у Pod `spec`) | поля контролей PSS | R1 |
| `metadata.labels` и `spec.template.metadata.labels` workload'а | `security.deckhouse.io/skip-pss-check`, `security.deckhouse.io/security-policy-exception*` | R5, R16 |
| `metadata.labels` объектов `monitoring.coreos.com` | `prometheus`, `component` | R7 |
| Шаблоны chart'а (после возможностей) | kind'ы `ServiceMonitor`, `PodMonitor`, `PrometheusRule`, `ScrapeConfig`, `Probe` | R6 |
| `PodLoggingConfig.spec.labelSelector.matchLabels` | выбор pod'ов | R10, R11 |
| `spec.selector.matchLabels` workload'а | метки для `PodLoggingConfig` | R12 |
| `DexAuthenticator`: `metadata.name`, `metadata.namespace`, `spec.applicationDomain`, `spec.applicationIngressCertificateSecretName`, `spec.applications[]` (`v2alpha1`) | связь и проверки | R13–R15 |
| Ingress: аннотация `nginx.ingress.kubernetes.io/auth-url`, `spec.rules[].host`, `spec.tls`; HTTPRoute: `alb.network.deckhouse.io/auth-url` | связь с DexAuthenticator | R14, R15 |
| ConfigMap с `grafana_dashboard: "1"`, `GrafanaDashboardDefinition.spec.{folder,definition}` | JSON дашборда | R9, R21 |
| Файл `OperationPolicy` (draft) | политики | R17 |

## 4. Выходы

### 4.1. `docs/deckhouse-report.md` (R3–R7, R9, R15)

Структура (Markdown, детерминированный порядок: разделы как ниже, строки — по `Kind/name`):

```markdown
# Deckhouse report: <chart>

Generated by dhg from the input manifests. Nothing here is applied to the cluster; ...

## Pod Security Standards (target: baseline)
| Workload | Level | Violations of the target level |
## Namespace labels this chart needs
| Label | Why | Objects |
## Monitoring objects Prometheus will not select
| Object | Missing labels |
## Logging
(PodLoggingConfig без labelSelector; clusterDestinationRefs и их владелец)
## Platform objects in this chart
| Object | Scope | Namespaced alternative |
## DexAuthenticator
| DexAuthenticator | Protected routes | Findings |
## Grafana sidecar dashboards
```

Пустой раздел не выводится. Отчёт на английском, как `docs/resource-report.md` и `SYNTHESIS.md`.

Таблица «Platform objects» (R3, вне scope, только сообщение):

| Входной kind | Namespaced альтернатива для команды приложения |
|---|---|
| `GrafanaDashboardDefinition` | `ObservabilityDashboard` (модуль `observability`) |
| `CustomPrometheusRules` | `ObservabilityMetricsRulesGroup` или `PrometheusRule` с метками `prometheus: main`, `component: rules` |
| `ClusterLoggingConfig`, `ClusterLogDestination` | `PodLoggingConfig` (назначение создаёт платформа) |
| `OperationPolicy`, `SecurityPolicy` | `SecurityPolicyException` для точечных исключений |
| `ModuleConfig`, `NodeGroup`, `IngressNginxController`, `ClusterAuthorizationRule`, `User`, `Group`, `*InstanceClass` | нет: платформенный GitOps или модуль (`--deckhouse-module`) |

### 4.2. Values

| Ключ | Тип | По умолчанию | Кто пишет |
|---|---|---|---|
| `global.securityContextDefaults.pod` | map | отсутствует | `dhg fix` (R19), `pss-restricted` (R20) |
| `global.securityContextDefaults.container` | map | отсутствует | то же |
| `deckhouseLogging.enabled` | bool | `true` | `deckhouse-logging` |
| `deckhouseLogging.clusterDestinationRefs` | list(string) | из параметра `destinations`, иначе `[]` | `deckhouse-logging` |
| `deckhouseLogging.multilineParser` | map | отсутствует; `{type: <param>}` при параметре | `deckhouse-logging` |
| `deckhouseLogging.workloads.<имя>.matchLabels` | map | селектор workload'а | `deckhouse-logging` |
| `prometheusRules.labels` | map | из параметра `labels` | `prometheus-rules` (R8) |
| `services.<svc>.podLoggingConfig.{spec,clusterDestinationRefs}` | map | из входа | процессор (R10) |

### 4.3. Пример рендера `deckhouse-logging`

Вход: Deployment `orders` (`spec.selector.matchLabels: {app: orders}`), `--feature-opt deckhouse-logging.destinations=loki-storage`, релиз `r1` в namespace `shop`:

```yaml
apiVersion: deckhouse.io/v1alpha1
kind: PodLoggingConfig
metadata:
  name: orders
  namespace: shop
  labels:
    app.kubernetes.io/managed-by: Helm
    # ... метки chart'а
spec:
  clusterDestinationRefs:
    - loki-storage
  labelSelector:
    matchLabels:
      app: orders
```

## 5. Интерфейс

| Возможность | Параметры (умолчание) | Добавляет |
|---|---|---|
| `deckhouse-report` | `pss-level` (`baseline`; `baseline`/`restricted`), `policies` (draft, `""`) | `docs/deckhouse-report.md` |
| `deckhouse-logging` | `destinations` (`""`, список через запятую), `multiline-parser` (`""`; `None`/`General`/`Backslash`/`LogWithTime`/`MultilineJSON`) | `templates/deckhouse-logging.yaml`, `deckhouseLogging.*`, строка в `NOTES.txt` при пустом `destinations` |
| `prometheus-rules` (существующая) | + `labels` (`""`), + `kind` (draft, `PrometheusRule`) | метки в `prometheusRules.labels` |
| `pss-restricted` (draft) | — | `global.securityContextDefaults` |
| `deckhouse-dashboards` (draft) | `category` (`""`), `replace-legacy` (`false`) | `templates/deckhouse-dashboards.yaml` |

Флаги CLI не добавляются (ADR-046). `--deckhouse-module` не меняется: он про модуль Deckhouse (платформа), а не про приложение. Ключи `.dhg.yaml` — те же (`with`, `feature-opt`). Совместимость: существующие chart'ы без новых `--with` рендерятся побайтно так же, кроме изменения R18 (шаблон pod'а), которое без ключей `global.securityContextDefaults` даёт тот же рендер.

## 6. Невыводимое

| Что | Как показывается |
|---|---|
| Имена `ClusterLogDestination` | параметр `destinations` / `deckhouseLogging.clusterDestinationRefs`; пустой список → объект не рендерится, строка в `NOTES.txt` |
| Метки namespace | раздел «Namespace labels» отчёта |
| Включён ли модуль `observability`, `monitoring-custom`, `log-shipper` | отчёт пишет «requires module X» рядом с объектом |
| Значения `securityContext` для `Restricted`, зависящие от образа (`runAsNonRoot`, `runAsUser`) | только нарушения в отчёте |
| Домен кластера (`clusterDomain`) в URL `auth-url` | сравнивается только префикс `<name>-dex-authenticator.<ns>.svc` |

## 7. Ошибки и граничные случаи

- Неизвестное значение `pss-level`, `multiline-parser`, `kind` → ошибка `feature <name> on chart <chart>: …` (как у существующих возможностей).
- `labels` с неверным ключом или значением метки Kubernetes (`k8s.io/apimachinery/pkg/util/validation.IsQualifiedName`, `IsValidLabelValue`) → ошибка с именем параметра.
- Workload с `selector.matchExpressions` без `matchLabels` → `deckhouse-logging` его пропускает и перечисляет в `NOTES.txt`.
- Pod spec с `spec.os.name: windows` → Linux-only контроли не проверяются (R1).
- `DexAuthenticator` `v2alpha1` с несколькими `applications[]` → проверки R15 для каждого домена.
- Library-режим: возможности пропускают library chart (как `ApplyFeatures`).
- Отчёт для chart'а без workload'ов и объектов мониторинга → файл с одним заголовком и строкой «No findings.».

## 8. Критерии приёмки эпика

- **AC1** (R1, R2): unit-тесты `pkg/compliance/pss` покрывают каждый контроль F14 минимум одним нарушающим и одним допустимым примером.
- **AC2** (R3–R7, R9): golden `TestDeckhouseReport` на фикстуре `tests/integration/fixtures/deckhouse-observability` (задача 02) находит в `docs/deckhouse-report.md`: `Deployment/orders | restricted`, `Deployment/legacy | privileged`, нарушение `hostPath`, метки `monitor-watcher-enabled` и `rules-watcher-enabled`, `PrometheusRule/orders` без `component: rules`, ConfigMap `orders-dashboard` в разделе sidecar.
- **AC3** (R10, R11): в universal-режиме `templates/podloggingconfig-orders-logs.yaml` попадает в группу `orders`; рендер содержит `spec.clusterDestinationRefs: [loki-storage]` и все поля входа (fidelity).
- **AC4** (R12): `--with deckhouse-logging --feature-opt deckhouse-logging.destinations=loki-storage` на `examples/01-simple-web` рендерит `PodLoggingConfig/nginx` с `labelSelector.matchLabels` = селектору Deployment'а; без параметра — ни одного `PodLoggingConfig`, `helm lint --strict` проходит.
- **AC5** (R8): `--feature-opt prometheus-rules.labels=prometheus=main,component=rules` → у `PrometheusRule` обе метки.
- **AC6** (R13–R15): связь Ingress `orders` → DexAuthenticator `orders` есть в `dhg graph` фикстуры; DexAuthenticator `v2alpha1` обрабатывается процессором, а не fallback'ом.
- **AC7** (R16): удаление `SecurityPolicyException` из входа фикстуры ломает golden-проверку целостности (тест на детектор подтверждает связь).
- **AC8** (R18, R19): `dhg fix -f examples/01-simple-web` → у контейнера `nginx` в рендере `securityContext.allowPrivilegeEscalation: false` и `seccompProfile` у pod'а; рендер всех golden-входов без `global.securityContextDefaults` не меняется.
- **AC9**: все сценарии `tests/golden` проходят, включая `all/<mode>/fixtures/deckhouse-observability`.

## 9. Проверенные факты и источники

Дата проверки всех строк — 2026-10-07. `deckhouse/deckhouse@main` — коммит `89db706` (после v1.77.2); модули `log-shipper`, `prometheus`, `monitoring-custom`, `operator-prometheus`, `observability` в `main` отсутствуют (внешние модули), поэтому их CRD проверены по тегу `v1.69.0`.

| # | Факт | Источник | Статус |
|---|---|---|---|
| F1 | `PodLoggingConfig`: группа `deckhouse.io`, версия `v1alpha1` (served, storage), `scope: Namespaced`; `spec.clusterDestinationRefs` обязателен, `minItems: 1`; поля `keepDeletedFilesOpenedFor` (pattern `^([0-9]+h([0-9]+m)?|[0-9]+m)$`), `labelSelector` (`matchLabels`/`matchExpressions`), `labelFilter`, `logFilter`, `multilineParser.type` ∈ {`None`,`General`,`Backslash`,`LogWithTime`,`MultilineJSON`,`Custom`} | `github.com/deckhouse/deckhouse/blob/v1.69.0/modules/460-log-shipper/crds/pod-logging-config.yaml` | проверено (v1.69.0); актуальная версия — не проверено |
| F2 | `ClusterLogDestination`: `deckhouse.io/v1alpha1`, `scope: Cluster` | `…/v1.69.0/modules/460-log-shipper/crds/cluster-log-destination.yaml` | проверено (v1.69.0) |
| F3 | Пользовательская инструкция по логам: `PodLoggingConfig` в своём namespace, имя хранилища для `clusterDestinationRefs` выдаёт администратор | `deckhouse/deckhouse@main docs/documentation/pages/user/LOGGING.md` | проверено |
| F4 | Роль `d8:namespace:manager` разрешает `PodLoggingConfig`, `DexAuthenticator`, `Certificate`; не даёт `ClusterLoggingConfig`, `ClusterLogDestination`, `OperationPolicy` | `@main modules/140-user-authz/docs/README.md`, `templates/rbacv2/global/namespace/roles/manager.yaml` | проверено |
| F5 | Prometheus DKP: `serviceMonitorSelector`, `podMonitorSelector`, `probeSelector`, `scrapeConfigSelector` = `prometheus: main`; `ruleSelector` = `prometheus: main` + `component: rules`; namespace-селекторы `prometheus.deckhouse.io/monitor-watcher-enabled`, `rules-watcher-enabled`, `scrape-configs-watcher-enabled`, `probe-watcher-enabled` = `"true"` | `…/v1.69.0/modules/300-prometheus/templates/prometheus/prometheus.yaml` (строки 178–219) | проверено (v1.69.0) |
| F6 | Для `ServiceMonitor`/`PodMonitor` нужна метка `prometheus: main` и метка namespace `monitor-watcher-enabled`; для `ScrapeConfig` — `scrape-configs-watcher-enabled`; для `Probe` — `probe-watcher-enabled`; `PodMonitor` рекомендуется | `@main docs/documentation/pages/user/monitoring/APP.md` | проверено |
| F7 | `PrometheusRule`: метка namespace `prometheus.deckhouse.io/rules-watcher-enabled: "true"` | `…/v1.69.0/modules/300-prometheus/docs/FAQ.md` («How do I set up a PrometheusRules…») | проверено (v1.69.0) |
| F8 | `deckhouse_lib_helm` 1.72.24 ставит на `PrometheusRule` метки `prometheus: main`, `component: rules`; при модуле `observability` вместо них создаёт `ClusterObservabilityMetricsRulesGroup` со `spec` = группа правил без `name` | `@main helm_lib/charts/deckhouse_lib_helm/templates/_monitoring_prometheus_rules.tpl` | проверено |
| F9 | `GrafanaDashboardDefinition` и `CustomPrometheusRules`: `deckhouse.io`, версии `v1alpha1` (served) и `v1` (storage), `scope: Cluster`. `GrafanaDashboardDefinition` — «legacy, will be removed», `CustomPrometheusRules` — «outdated» | `…/v1.69.0/modules/300-prometheus/crds/{grafanadashboarddefinition,customprometheusrules}.yaml`; `@main docs/documentation/pages/user/monitoring/{DASHBOARDS,OVERVIEW}.md` | проверено |
| F10 | `ObservabilityDashboard` (`observability.deckhouse.io/v1alpha1`) — namespaced, `spec.definition` (строка JSON), аннотации `metadata.deckhouse.io/category`, `metadata.deckhouse.io/title`; `ClusterObservabilityDashboard`, `ClusterObservabilityPropagatedDashboard` — cluster. `ObservabilityMetricsRulesGroup` — namespaced правила, `ClusterObservabilityMetricsRulesGroup` — cluster. Модуль `observability`: редакции CE, Core, BE, SE, SE+, EE, CSE Pro; внешний | `@main docs/documentation/pages/user/monitoring/DASHBOARDS.md`, `…/OVERVIEW.md`, `docs/documentation/pages/admin/configuration/app-scaling/SCALING_BY_METRICS.md`, `helm_lib/…/_monitoring_grafana_dashboards.tpl`, `docs/documentation/_data/modules/modules-addition.json` | проверено по документации; схема CRD — не проверено (CRD не публикуется в репозитории) |
| F11 | `DexAuthenticator`: `deckhouse.io`, `v1alpha1` (deprecated), `v2alpha1` (served), `v1` (storage); `scope: Namespaced`; создаёт Deployment (oauth2-proxy + Redis), Service, Ingress, опционально HTTPRoute, Secrets; только HTTPS; поля `v1`: `applicationDomain`, `sendAuthorizationHeader`, `applicationIngressCertificateSecretName`, `applicationIngressClassName`, `gatewayAPI`, `signOutURL`, `keepUsersLoggedInFor`, `allowedEmails`, `allowedGroups`, `whitelistSourceRanges`, `additionalApplications`, `podMetadata`, `nodeSelector`, `tolerations`, `highAvailability`, `resources`; `v2alpha1`: `applications[]` (`domain`, `ingressSecretName`, `ingressClassName`, …) | `@main modules/150-user-authn/crds/dex-authenticator.yaml` | проверено |
| F12 | Подключение приложения: аннотации Ingress `nginx.ingress.kubernetes.io/auth-signin: https://$host/dex-authenticator/sign_in`, `…/auth-url: https://<NAME>-dex-authenticator.<NS>.svc.<C_DOMAIN>/dex-authenticator/auth`, `…/auth-response-headers: X-Auth-Request-User,X-Auth-Request-Email`; имя Service может быть усечено, его находят по метке `deckhouse.io/dex-authenticator-for=<NAME>`; для ALB — аннотации `alb.network.deckhouse.io/auth-*` на HTTPRoute | `@main modules/150-user-authn/docs/FAQ.md` | проверено |
| F13 | PSS в DKP: метка namespace `security.deckhouse.io/pod-policy=<policy>` (пример: `restricted`), режим — `security.deckhouse.io/pod-policy-action=<deny|warn|dryrun>`; политика по умолчанию `Baseline` для установок с v1.55, `Privileged` — для более ранних; `enforcementAction` по умолчанию `Deny`; реализовано Gatekeeper'ом, не Pod Security Admission; контроллеры проверяются отдельно (`controllerValidation`); метка `security.deckhouse.io/skip-pss-check` читается с верхнего уровня `metadata.labels` объекта | `@main modules/015-admission-policy-engine/docs/README.md`, `openapi/config-values.yaml` | проверено |
| F14 | Контроли PSS Baseline и Restricted (поля и допустимые значения, включая «Host Probes / Lifecycle Hooks (v1.34+)», список sysctls, `container_engine_t` с 1.31); `readOnlyRootFilesystem` в стандарт не входит | `github.com/kubernetes/website/blob/main/content/en/docs/concepts/security/pod-security-standards.md` | проверено |
| F15 | `SecurityPolicyException`: `deckhouse.io/v1alpha1`, `scope: Namespaced`; ссылки — метки pod-шаблона `security.deckhouse.io/security-policy-exception: <имя>` и `…security-policy-exception.container.<контейнер>: <имя>`; `OperationPolicy`, `SecurityPolicy` — `scope: Cluster` | `@main modules/015-admission-policy-engine/crds/{security-policy-exception,operation-policy,security-policy}.yaml`, `docs/README.md` | проверено |
| F16 | `OperationPolicy.spec.policies`: `allowedRepos`, `requiredResources`, `disallowedImageTags`, `disallowedTolerations`, `requiredLabels`, `requiredAnnotations`, `requiredProbes`, `maxRevisionHistoryLimit`, `priorityClassNames`, `ingressClassNames`, `storageClassNames`, `imagePullPolicy`, `checkHostNetworkDNSPolicy`, `checkContainerDuplicates`, `replicaLimits`, `gpuResourceRestriction`; `spec.match.{namespaceSelector,labelSelector}` | `@main modules/015-admission-policy-engine/crds/operation-policy.yaml` | проверено |
| F17 | `monitoring-custom`: метка `prometheus.deckhouse.io/custom-target` на Service/Pod, порты `http-metrics`/`https-metrics`, аннотации `prometheus.deckhouse.io/{port,path,tls,sample-limit,…}` | `@main docs/documentation/pages/user/monitoring/APP.md` | проверено |
| F18 | Механизм ConfigMap с меткой `grafana_dashboard: "1"` в документации DKP не описан | поиск по `@main docs/` | проверено (отсутствие в документации); работает ли он в DKP — не проверено |
| F19 | Поведение dhg: `dhg fix` не добавляет `securityContext` основному контейнеру `examples/01-simple-web`; `--with prometheus-rules` не ставит `prometheus: main`/`component: rules` | запуск `dhg` из коммита `a4d01f2` + `helm template` (Helm в `$HOME/go/bin/helm`) | проверено |
