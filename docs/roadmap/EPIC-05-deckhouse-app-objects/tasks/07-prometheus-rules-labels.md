# EPIC-05 / 07: Параметр `labels` у возможности `prometheus-rules`

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | S (≤ 1 дня) |
| Зависит от | — |
| Требования | R8 (из [spec.md](../spec.md)) |

## Контекст

Prometheus DKP выбирает `PrometheusRule` по меткам `prometheus: main` и `component: rules` (F5, `ruleSelector`) в namespace с `prometheus.deckhouse.io/rules-watcher-enabled: "true"` (F7). Возможность `prometheus-rules` (`pkg/generator/features_observability.go:69`, `pkg/generator/alertingrules.go:applyPrometheusRulesFeature`) рендерит метки chart'а плюс `prometheusRules.labels`, который всегда пуст (`"labels": map[string]interface{}{}`), поэтому в DKP её правила не загружаются (F19). Сейчас обход — `--set prometheusRules.labels.prometheus=main …` при каждой установке.

Параметр общий (не Deckhouse-специфичный): так же задаются метки для `ruleSelector` kube-prometheus-stack (`release: <имя>`). Профиль `deckhouse` (EPIC-11) задаёт его значение.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/generator/features_observability.go` | В `Params` возможности `prometheus-rules` добавить `"labels": ""` |
| `pkg/generator/alertingrules.go` | `applyPrometheusRulesFeature`: разобрать `labels` (`k=v[,k=v…]`) в map и положить в `prometheusRules.labels` |
| `pkg/generator/features.go` | `func (fc FeatureContext) MapParam(key string) (map[string]string, error)` — разбор `k=v` с проверкой `validation.IsQualifiedName(k)` и `validation.IsValidLabelValue(v)` (`k8s.io/apimachinery/pkg/util/validation`) |
| `pkg/generator/alertingrules_test.go`, `features_test.go` | Тесты |
| `tests/golden/deckhouse_test.go` | `TestPrometheusRulesLabels` |
| `README.md` | В таблице `--with`: «`prometheus-rules` … (`labels=` — метки для `ruleSelector`, в Deckhouse — `prometheus=main,component=rules`)» |

## Шаги

1. `MapParam`: пустая строка → пустой map; элемент без `=` → ошибка `parameter labels: "x" is not key=value`; дубликат ключа → ошибка; ошибки валидации — с текстом из `validation`.
2. В `applyPrometheusRulesFeature` значение `labels` записывается в values как `map[string]interface{}` (порядок ключей в YAML — алфавитный, как у `yaml.Marshal`).
3. Ошибки возвращаются как `prometheus-rules: …` (обёртка `ApplyFeatures` добавит имя chart'а).

## Тесты

- Unit: `labels=prometheus=main,component=rules` → `prometheusRules.labels == {component: rules, prometheus: main}`; `labels=` → `{}`; `labels=bad key=x` → ошибка; `labels=a=b,a=c` → ошибка; `labels=app.kubernetes.io/part-of=shop` → допустимо.
- Golden (`TestPrometheusRulesLabels`): `examples/11-monitoring-stack`, `--with prometheus-rules --feature-opt prometheus-rules.labels=prometheus=main,component=rules`, режимы universal/separate/umbrella → у отрендеренного `PrometheusRule` с именем `*-rules` есть обе метки и метки chart'а (`app.kubernetes.io/managed-by: Helm`).

## Критерии приёмки

- [ ] AC5 из spec.md выполнен.
- [ ] `dhg features` показывает параметр `labels=`.
- [ ] Без параметра values и рендер не меняются.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

Метки namespace (только отчёт, задача 02); `ObservabilityMetricsRulesGroup` (задача 09).
