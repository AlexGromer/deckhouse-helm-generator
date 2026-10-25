# EPIC-05 / 05: Процессор `PodLoggingConfig`

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | S (≤ 1 дня) |
| Зависит от | EPIC-05/02 (фикстура) |
| Требования | R10, R11 (из [spec.md](../spec.md)) |

## Контекст

`PodLoggingConfig` (`deckhouse.io/v1alpha1`, namespaced; F1) сейчас обрабатывает generic fallback (`pkg/processor/registry.go:processGeneric`): поля сохраняются (`spec` → `services.<svc>.podLoggingConfig.spec`), но сервисная группа выводится из меток/имени самого объекта (`processor.ServiceNameFromResource`), а не из выбираемых pod'ов, и в графе нет связи с workload'ом. В separate/umbrella-режимах объект попадает в отдельный chart.

Перед реализацией (Q7 README): сверить поля CRD в кластере владельца — `kubectl explain podloggingconfig.spec --recursive`; схема в этом документе — по тегу `v1.69.0`. Процессор рендерит весь `spec`, поэтому новые поля CRD не теряются.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/processor/k8s/podloggingconfig.go` (новый) | `PodLoggingConfigProcessor` (ниже) |
| `pkg/processor/k8s/podloggingconfig_test.go` (новый) | Unit-тесты |
| `pkg/processor/k8s/registry.go` | `r.Register(NewPodLoggingConfigProcessor())` в блоке «Deckhouse CRDs» |
| `pkg/processor/k8s/registry_test.go` | Ожидаемое число/список процессоров |
| `pkg/analyzer/detector/label.go` | `case "PodLoggingConfig": detectPodLoggingConfigToWorkload` |
| `pkg/analyzer/detector/label_test.go` | Тесты связи |
| `pkg/generator/features_deckhouse.go`, `features_deckhouse_test.go` | Раздел `## Logging` отчёта (шаг 3) |
| `tests/integration/fixtures/deckhouse-observability/podloggingconfig.yaml` (новый) | Объект фикстуры |
| `tests/golden/deckhouse_test.go` | `TestPodLoggingConfig` |
| `tests/golden/integrity_test.go` | `selectorOf`: `case "PodLoggingConfig": sel = get(m, "spec", "labelSelector", "matchLabels")` — проверка «селектор продолжает выбирать pod'ы» начинает покрывать `PodLoggingConfig` на всех входах |
| `docs/RELEASE_NEXT.md` | Изменение группы и пути values (design §7) |

## Шаги

1. Процессор (по образцу `prometheusrule.go`):
   - `NewBaseProcessor("podloggingconfig", 50, schema.GroupVersionKind{Group: "deckhouse.io", Version: "v1alpha1", Kind: "PodLoggingConfig"})`;
   - `serviceName`: из `spec.labelSelector.matchLabels` по ключам `app.kubernetes.io/name`, `app.kubernetes.io/instance`, `app`, `name`, `component` (первый непустой — тот же порядок, что `processor.ServiceNameFromLabels`; вынести список ключей в экспортируемую переменную `processor.ServiceNameLabelKeys` и использовать в обоих местах); иначе `processor.ServiceNameFromResource(obj)`;
   - `TemplatePath: fmt.Sprintf("templates/podloggingconfig-%s.yaml", name)`, `ValuesPath: fmt.Sprintf("services.%s.podLoggingConfig", serviceName)`;
   - values: `spec` (весь), `clusterDestinationRefs` (если есть);
   - шаблон:
     ```
     {{- $svc := .Values.services.<sanitized> -}}
     {{- if $svc.enabled }}
     {{- with $svc.podLoggingConfig }}
     apiVersion: deckhouse.io/v1alpha1
     kind: PodLoggingConfig
     metadata:
       name: <processor.ObjectName(name)>
       namespace: {{ $.Release.Namespace }}
       labels:
         {{- include "<chart>.labels" $ | nindent 4 }}
     <processor.SpecOverlay(".", "clusterDestinationRefs")>{{- end }}
     {{- end }}
     ```
2. Детектор: `metav1.LabelSelectorAsSelector` по `spec.labelSelector` (unstructured → `metav1.LabelSelector` через `runtime.DefaultUnstructuredConverter.FromUnstructured`); кандидаты — workload'ы того же namespace: Deployment, StatefulSet, DaemonSet, ReplicaSet (метки `spec.template.metadata.labels`), Job (`spec.template…`), CronJob (`spec.jobTemplate.spec.template.metadata.labels`), Pod (`metadata.labels`); связь `RelationLabelSelector`, `Field: "spec.labelSelector"`, `Details["selector"] = selector.String()`. Без `labelSelector` — связей нет.
3. `pkg/generator/features_deckhouse.go` (`buildDeckhouseReport`): раздел `## Logging` — для каждого входного `PodLoggingConfig` без `spec.labelSelector` строка `PodLoggingConfig/<name>: selects every pod in the namespace, including pods of other releases`; для каждого — список `clusterDestinationRefs` с пометкой `ClusterLogDestination is created by the platform team`.
4. Фикстура `podloggingconfig.yaml`:
   ```yaml
   apiVersion: deckhouse.io/v1alpha1
   kind: PodLoggingConfig
   metadata:
     name: orders-logs
     namespace: shop
   spec:
     clusterDestinationRefs: [loki-storage]
     labelSelector:
       matchLabels: {app: orders}
     multilineParser:
       type: MultilineJSON
   ```

## Тесты

- Unit (`podloggingconfig_test.go`): `ServiceName == "orders"` для `matchLabels {app: orders}`; приоритет `app.kubernetes.io/name` над `app`; без селектора — имя объекта; `Values["spec"]` содержит `multilineParser`; шаблон содержит `kind: PodLoggingConfig`, `namespace: {{ $.Release.Namespace }}`, `$dhgSpec`; `nil` → ошибка.
- Unit (`label_test.go`): связь с Deployment `orders` (совпадение `matchLabels`), нет связи с Deployment `legacy`; `matchExpressions: [{key: app, operator: In, values: [orders]}]` — связь есть; другой namespace — связи нет; без `labelSelector` — нет связей.
- Golden (`TestPodLoggingConfig`): фикстура `deckhouse-observability`, режимы universal/separate/umbrella; в universal шаблон `templates/podloggingconfig-orders-logs.yaml` есть в chart'е и значения лежат в `services.orders.podLoggingConfig`; в separate объект в chart'е группы `orders` (тот же, где Deployment `orders`); рендер: `spec.clusterDestinationRefs == [loki-storage]`, `spec.multilineParser.type == MultilineJSON`, `metadata.namespace` = namespace релиза; `--set services.orders.podLoggingConfig.clusterDestinationRefs={loki-a,loki-b}` (universal) → в рендере оба значения. Fidelity-сценарии проходят.

## Критерии приёмки

- [ ] AC3 из spec.md выполнен.
- [ ] `dhg graph -f tests/integration/fixtures/deckhouse-observability` показывает ребро `PodLoggingConfig/orders-logs → Deployment/orders`.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

Создание `PodLoggingConfig` для workload'ов без него (задача 06); `ClusterLoggingConfig`/`ClusterLogDestination` (платформа).
