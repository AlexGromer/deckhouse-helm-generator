# EPIC-05 / 06: Возможность `deckhouse-logging`

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | M (2–3 дня) |
| Зависит от | EPIC-05/05 |
| Требования | R12 (из [spec.md](../spec.md)) |

## Контекст

Чтобы логи приложения попали в хранилище DKP, команде нужен `PodLoggingConfig` в своём namespace (F3); хранилище (`ClusterLogDestination`) создаёт платформа, его имя команда получает от администратора. Фактом входа является только то, **какие pod'ы** принадлежат workload'у (`spec.selector`), а имя назначения — нет. Возможность создаёт `PodLoggingConfig` на каждый workload с селектором из входа; список назначений — параметр/values; при пустом списке ничего не рендерится (CRD требует `minItems: 1`, F1).

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/generator/features_deckhouse.go` | `RegisterFeature` `deckhouse-logging`, параметры `destinations` (`""`), `multiline-parser` (`""`); `applyDeckhouseLoggingFeature` |
| `pkg/generator/features_deckhouse_test.go` | Unit-тесты |
| `tests/golden/deckhouse_test.go` | `TestDeckhouseLogging` |
| `README.md`, `docs/RELEASE_NEXT.md` | Строка возможности |

## Шаги

1. Отбор workload'ов — алгоритм [design.md §4.5](../design.md#45-deckhouse-logging-r12): `opsChartResources(chart, fc.Graph)` (`pkg/generator/features_ops.go`), kind ∈ {Deployment, StatefulSet, DaemonSet, Job, CronJob, `argoproj.io` Rollout}; селектор: `spec.selector.matchLabels` (CronJob — `spec.jobTemplate.spec.selector.matchLabels`, при отсутствии — метки `spec.jobTemplate.spec.template.metadata.labels`; Job без селектора — метки `spec.template.metadata.labels`). Пропуск, если входной `PodLoggingConfig` того же namespace выбирает этот workload (связь `label_selector` из задачи 05 в `fc.Graph.Relationships`).
2. Параметры: `destinations` — `fc.ListParam`; каждое имя проверяется `validation.IsDNS1123Subdomain` (имя cluster-scoped объекта), ошибка `deckhouse-logging: destination "X": …`. `multiline-parser` ∈ {`""`, `None`, `General`, `Backslash`, `LogWithTime`, `MultilineJSON`} (`Custom` требует regex — не поддерживается параметром, задаётся в values вручную), иначе ошибка.
3. Values (`addFeatureValues(out, "deckhouseLogging", …)`):
   ```yaml
   deckhouseLogging:
     enabled: true
     clusterDestinationRefs: [loki-storage]   # [] без параметра
     multilineParser: {type: MultilineJSON}    # только при параметре
     workloads:
       orders:
         matchLabels: {app: orders}
   ```
4. Шаблон `templates/deckhouse-logging.yaml` (helpers chart'а — `chartHelperPrefix(chart)`):
   ```
   {{- if and .Values.deckhouseLogging.enabled .Values.deckhouseLogging.clusterDestinationRefs }}
   {{- range $name, $w := .Values.deckhouseLogging.workloads }}
   ---
   apiVersion: deckhouse.io/v1alpha1
   kind: PodLoggingConfig
   metadata:
     name: {{ $name }}
     namespace: {{ $.Release.Namespace }}
     labels:
       {{- include "<prefix>.labels" $ | nindent 4 }}
   spec:
     clusterDestinationRefs:
       {{- toYaml $.Values.deckhouseLogging.clusterDestinationRefs | nindent 4 }}
     labelSelector:
       matchLabels:
         {{- toYaml $w.matchLabels | nindent 6 }}
     {{- with $.Values.deckhouseLogging.multilineParser }}
     multilineParser:
       {{- toYaml . | nindent 4 }}
     {{- end }}
   {{- end }}
   {{- end }}
   ```
   Если у chart'а нет helper'а меток (`chartHasHelper`), блок `labels` не выводится (как в `buildPrometheusRulesTemplate`).
5. При пустом `destinations` добавить в `out.Notes` (NOTES.txt) строку: `deckhouse-logging: set deckhouseLogging.clusterDestinationRefs to the ClusterLogDestination names given by the platform team; no PodLoggingConfig is rendered until then.` Workload'ы, пропущенные из-за отсутствия селектора, — строкой `deckhouse-logging: <Kind>/<name> skipped: no matchLabels selector`.
6. Chart без подходящих workload'ов — без изменений.

## Тесты

- Unit: `examples/01-simple-web`-подобный граф (Deployment `nginx`, selector `{app.kubernetes.io/name: nginx}`) → `workloads.nginx.matchLabels`; `destinations=loki-storage,loki-archive` → оба в values; неверное имя `Loki_Storage` → ошибка; `multiline-parser=Custom` → ошибка; workload, выбранный входным `PodLoggingConfig` → пропущен; CronJob без селектора → метки pod-шаблона; NOTES.txt содержит строку при пустом `destinations`.
- Golden (`TestDeckhouseLogging`):
  - `examples/01-simple-web`, `--with deckhouse-logging --feature-opt deckhouse-logging.destinations=loki-storage`, режимы universal/separate/umbrella: в рендере `PodLoggingConfig/nginx`, `spec.clusterDestinationRefs == [loki-storage]`, `spec.labelSelector.matchLabels` равен `spec.selector.matchLabels` отрендеренного Deployment'а `nginx`;
  - без параметра: `PodLoggingConfig` в рендере нет, `helm lint --strict` проходит;
  - фикстура `deckhouse-observability` с параметром: `PodLoggingConfig/legacy` есть, `PodLoggingConfig/orders` (от возможности) нет — `orders` уже выбран входным `orders-logs`.
- `TestFeaturesPassHelm` автоматически прогоняет возможность с параметрами по умолчанию на 7 входах.

## Критерии приёмки

- [ ] AC4 из spec.md выполнен.
- [ ] Без `destinations` вывод — валидный chart без `PodLoggingConfig` и с подсказкой в NOTES.txt.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

`labelFilter`/`logFilter`/`keepDeletedFilesOpenedFor` — задаются вручную в шаблоне/values (не факты). `ClusterLogDestination` — платформа.
