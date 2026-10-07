# EPIC-05 / 09: Правила — `ObservabilityMetricsRulesGroup` в `prometheus-rules`

| Поле | Значение |
|---|---|
| Статус | draft — открытый вопрос Q2 ([README](../README.md#риски-и-открытые-вопросы)); схема CRD не проверена |
| Размер | S (≤ 1 дня) |
| Зависит от | EPIC-05/07 |
| Требования | R22 (из [spec.md](../spec.md)) |

## Контекст

Документация DKP называет рекомендуемым способом пользовательских правил `ObservabilityMetricsRulesGroup` (namespaced) модуля `observability`, а `CustomPrometheusRules` — устаревшим (F10, F9). `deckhouse_lib_helm` 1.72.24 при включённом `observability` превращает каждую группу `PrometheusRule` в объект `…MetricsRulesGroup` со `spec` = группа без поля `name` (F8). Для команды приложения работает и обычный `PrometheusRule` с метками (задача 07), поэтому новый вид — только по выбору.

**Почему draft:** схема `ObservabilityMetricsRulesGroup` известна только по документации и `helm_lib` (поля `interval`, `rules`); CRD внешнего модуля в публичном репозитории нет; нужно подтверждение, что модуль включён у владельца (Q2).

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/generator/features_observability.go` | Параметр `kind` (`PrometheusRule` / `ObservabilityMetricsRulesGroup`) у `prometheus-rules` |
| `pkg/generator/alertingrules.go` | Вторая ветка шаблона: по объекту на группу (`<fullname>-workloads`, `<fullname>-slo`), `apiVersion: observability.deckhouse.io/v1alpha1`, `spec: {interval?, rules: [...]}` без `name` |
| `pkg/generator/alertingrules_test.go` | Тесты |
| `tests/golden/deckhouse_test.go` | Рендер с `kind=ObservabilityMetricsRulesGroup` |

## Шаги

1. Параметр проверяется: иное значение → ошибка `prometheus-rules: kind must be PrometheusRule or ObservabilityMetricsRulesGroup`.
2. Метки `labels` (задача 07) ставятся и на новый вид (DKP-селекторы для него не документированы — оставить пользователю).
3. `deckhouse-report` для `ObservabilityMetricsRulesGroup` не требует `rules-watcher-enabled` (механизм модуля `observability`), но пишет «requires module observability».
4. Перед реализацией выполнить в кластере владельца `kubectl explain observabilitymetricsrulesgroup.spec --recursive` и сверить поля; при расхождении — остановиться и описать в этой задаче.

## Тесты

- Unit: `kind=ObservabilityMetricsRulesGroup` → шаблон содержит `kind: ObservabilityMetricsRulesGroup`, не содержит `groups:` и `- name:` на уровне spec; неверный `kind` → ошибка.
- Golden: `examples/11-monitoring-stack`, `--with prometheus-rules --feature-opt prometheus-rules.kind=ObservabilityMetricsRulesGroup` → объект `…-workloads` с `spec.rules[*].alert` ⊇ {`PodCrashLooping`, `ContainerOOMKilled`, `PodFrequentlyRestarting`, `PodNotReady`, `ContainerWaiting`} (`workloadAlerts`, `alertingrules.go:29`), `helm lint --strict` проходит (CRD не нужен для `helm template`).

## Критерии приёмки

- [ ] Схема CRD подтверждена в кластере; Q2 решён.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

`ClusterObservabilityMetricsRulesGroup` (cluster-scoped); конвертация входных `PrometheusRule`.
