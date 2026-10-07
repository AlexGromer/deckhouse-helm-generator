# EPIC-05 / 08: Дашборды — `ObservabilityDashboard` из входных дашбордов

| Поле | Значение |
|---|---|
| Статус | draft — открытые вопросы Q2, Q3, Q4 ([README](../README.md#риски-и-открытые-вопросы)) |
| Размер | M (2–3 дня) |
| Зависит от | EPIC-05/02 |
| Требования | R21 (из [spec.md](../spec.md)) |

## Контекст

Команда приложения в DKP поставляет дашборд как namespaced `ObservabilityDashboard` (`observability.deckhouse.io/v1alpha1`, модуль `observability`; F10) — «recommended»; `GrafanaDashboardDefinition` (cluster-scoped) — «legacy, will be removed» (F9). dhg сейчас:

- ConfigMap с меткой `grafana_dashboard: "1"` обрабатывает `GrafanaDashboardProcessor` (`pkg/processor/k8s/grafanadashboard.go`) — соглашение sidecar'а kube-prometheus-stack (F18);
- `GrafanaDashboardDefinition` во входе уходит в generic fallback (cluster-scoped объект в app-chart'е).

Факт для генерации — JSON дашборда (значение ключа `data` ConfigMap'а или `spec.definition` `GrafanaDashboardDefinition`) и папка (`spec.folder`). Всё остальное (наличие модуля, категория) — не факт.

**Почему draft:** (Q2) неизвестно, включён ли `observability` у владельца (не входит в CSE Lite); (Q3) расхождение ключей аннотаций в документации DKP (`observability.deckhouse.io/*` в таблице миграции, `metadata.deckhouse.io/*` в примерах и в `deckhouse_lib_helm` 1.72.24); схема CRD не публикуется в `deckhouse/deckhouse` (модуль внешний) — проверить `kubectl explain observabilitydashboard --recursive` в кластере владельца; (Q4) заменять ли входной `GrafanaDashboardDefinition`.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/generator/features_deckhouse.go` | `deckhouse-dashboards`: параметры `category` (`""`), `replace-legacy` (`false`) |
| `pkg/generator/features_deckhouse_test.go` | Unit-тесты |
| `tests/golden/deckhouse_test.go` | `TestDeckhouseDashboards` |
| `README.md`, `docs/RELEASE_NEXT.md` | Строка возможности |

## Шаги

1. Источники дашбордов: `opsChartResources` → ConfigMap с `grafana_dashboard: "1"` (каждый ключ `data` = один дашборд; имя объекта = `<configmap>-<ключ без .json>` в DNS-форме) и `GrafanaDashboardDefinition` (`spec.definition`; категория = `spec.folder`).
2. Values: `deckhouseDashboards: {enabled: true, dashboards: {<имя>: {category: <folder|param>, title: <title из JSON, если есть>, definition: <JSON строкой>}}}`. JSON не переформатируется.
3. Шаблон `templates/deckhouse-dashboards.yaml`: `range` по `dashboards`, объект
   ```yaml
   apiVersion: observability.deckhouse.io/v1alpha1
   kind: ObservabilityDashboard
   metadata:
     name: {{ $name }}
     namespace: {{ $.Release.Namespace }}
     annotations:
       metadata.deckhouse.io/category: {{ $d.category | quote }}   # только если не пусто
       metadata.deckhouse.io/title: {{ $d.title | quote }}         # только если не пусто
   spec:
     definition: {{ $d.definition | toJson }}
   ```
   (ключ аннотаций — по решению Q3).
4. `replace-legacy=true`: шаблоны входных `GrafanaDashboardDefinition` оборачиваются в `{{- if not .Values.deckhouseDashboards.enabled }}` (видно и обратимо в values). По умолчанию — не трогать (fidelity).
5. Отчёт (`deckhouse-report`) при наличии возможности пишет «requires module observability».

## Тесты

- Unit: ConfigMap с двумя ключами → два дашборда; `GrafanaDashboardDefinition` с `folder: Apps` → `category: Apps`; невалидный JSON → дашборд создаётся (JSON — данные), но `title` пуст; `replace-legacy=true` → шаблон legacy-объекта обёрнут.
- Golden: фикстура `deckhouse-observability` (`ConfigMap/orders-dashboard`) с `--with deckhouse-dashboards` → в рендере `ObservabilityDashboard/orders-dashboard-orders` в namespace релиза, `spec.definition` равен `{"title":"Orders","panels":[]}` (сравнение после `json.Unmarshal`), аннотация `metadata.deckhouse.io/title: Orders`.

## Критерии приёмки

- [ ] Q2–Q4 решены и отражены в README эпика; ключ аннотаций проверен на кластере.
- [ ] Без `--with deckhouse-dashboards` вывод не меняется.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

`ClusterObservabilityDashboard`, `ClusterObservabilityPropagatedDashboard` (cluster-scoped, платформа); RBAC на `observabilitydashboards` (выдаёт платформа).
