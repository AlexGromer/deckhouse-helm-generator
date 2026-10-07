# EPIC-05 / 02: Возможность `deckhouse-report` и фикстура `deckhouse-observability`

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | M (2–3 дня) |
| Зависит от | EPIC-05/01 |
| Требования | R3–R7, R9 (из [spec.md](../spec.md)) |

## Контекст

Отчёт — единственное место, где dhg сообщает команде приложения то, чего не может сделать сам: метки namespace, уровень PSS, метки объектов мониторинга, cluster-scoped объекты во входе. Образец реализации — возможность `resource-report` (`pkg/generator/features_ops.go:164` `applyResourceReportFeature`, путь `ResourceReportPath = "docs/resource-report.md"`, ресурсы chart'а — `opsChartResources`). Шаблоны chart'а по kind'ам — `resourceTemplates(chart, isKind(...))` (`pkg/generator/features_observability.go:310`).

Задача также создаёт фикстуру, на которой проверяются задачи 02–11. Golden-набор подхватывает новый каталог `tests/integration/fixtures/*` автоматически (`tests/golden/golden_test.go:62 inputs`).

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/generator/features_deckhouse.go` (новый) | `init()` с `RegisterFeature` для `deckhouse-report` (параметры: `pss-level` = `baseline`); `applyDeckhouseReportFeature`; `DeckhouseReport`, `buildDeckhouseReport`, `Markdown`; `const DeckhouseReportPath = "docs/deckhouse-report.md"` |
| `pkg/generator/features_deckhouse_test.go` (новый) | Unit-тесты |
| `tests/integration/fixtures/deckhouse-observability/*.yaml` (новые) | Фикстура (ниже) |
| `tests/golden/deckhouse_test.go` (новый) | `TestDeckhouseReport` |
| `README.md` | Строка `deckhouse-report` в таблице «Опциональные возможности (`--with`)»; число возможностей «Все 16» → актуальное |
| `docs/RELEASE_NEXT.md` | Запись о новой возможности |

## Шаги

1. Фикстура `tests/integration/fixtures/deckhouse-observability/` (namespace `shop` во всех объектах):
   - `deployment-orders.yaml` — Deployment `orders`: `metadata.labels {app: orders}`, `spec.replicas: 2`, `selector.matchLabels {app: orders}`, pod-метки `{app: orders}`; pod `securityContext {runAsNonRoot: true, runAsUser: 10001, seccompProfile: {type: RuntimeDefault}}`; контейнер `orders`, образ `registry.example.com/shop/orders:2.1.0`, порт `{name: http, containerPort: 8080}`, `securityContext {allowPrivilegeEscalation: false, capabilities: {drop: [ALL]}}`, `resources.requests {cpu: 100m, memory: 256Mi}`.
   - `deployment-legacy.yaml` — Deployment `legacy`: метки `{app: legacy}`, селектор и pod-метки `{app: legacy}`, том `{name: data, hostPath: {path: /var/lib/legacy}}`, контейнер `legacy`, образ `registry.example.com/shop/legacy:1.0`, `volumeMounts [{name: data, mountPath: /data}]`.
   - `service.yaml` — Service `orders`: селектор `{app: orders}`, порт `{name: http, port: 8080, targetPort: http}`.
   - `servicemonitor.yaml` — ServiceMonitor `orders`: метки `{app: orders, prometheus: main}`, `selector.matchLabels {app: orders}`, `endpoints [{port: http, path: /actuator/prometheus, interval: 30s}]`.
   - `prometheusrule.yaml` — PrometheusRule `orders`: метки `{app: orders, prometheus: main}` (без `component`), группа `orders.rules`, правило `alert: OrdersHighErrorRate`, `expr: rate(http_server_requests_seconds_count{status=~"5.."}[5m]) > 1`, `for: 5m`, `labels {severity: warning}`.
   - `configmap-dashboard.yaml` — ConfigMap `orders-dashboard`: метки `{app: orders, grafana_dashboard: "1"}`, `data {orders.json: '{"title":"Orders","panels":[]}'}`.
2. `buildDeckhouseReport` (алгоритмы [design.md §4.1–4.3](../design.md#4-алгоритмы)):
   - workload'ы: `opsChartResources(chart, fc.Graph)` → `pss.PodSpec` → `pss.Evaluate`; строка `| Deployment/legacy | privileged | HostPath Volumes: spec.volumes[0].hostPath |` (нарушения через `; `, только `ViolationsOf(target)`);
   - R4: минимальный уровень → строка `security.deckhouse.io/pod-policy: <level>`;
   - R5: метка `security.deckhouse.io/skip-pss-check` только в pod-шаблоне;
   - R6: по шаблонам chart'а kind ∈ {ServiceMonitor, PodMonitor, PrometheusRule, ScrapeConfig, Probe};
   - R7: метки входных объектов `monitoring.coreos.com`; для `templates/prometheus-rules.yaml` — `prometheusRules.labels` из values chart'а;
   - «Platform objects»: входные объекты из таблицы [spec.md §4.1](../spec.md#41-docsdeckhouse-reportmd-r3r7-r9-r15) по kind'у;
   - R9: ConfigMap с `grafana_dashboard: "1"` (строка `ConfigMap/<name>: Grafana sidecar convention (kube-prometheus-stack); Deckhouse uses ObservabilityDashboard (module observability) or legacy GrafanaDashboardDefinition`).
3. Детерминированность: сортировка строк по `Kind/name`; без времени и путей файловой системы.
4. Пустой отчёт: заголовок + `No findings.`.
5. Ошибка при неверном `pss-level`: `deckhouse-report: pss-level must be baseline or restricted, got "x"`.
6. В описании возможности (`Description`) указать: «list it last in --with: it reports templates added by the features before it».

## Тесты

- Unit (`pkg/generator/features_deckhouse_test.go`):
  - chart из графа с Deployment `restricted`/`privileged` (объекты строятся в тесте) → строки таблицы PSS и рекомендация `privileged`;
  - `pss-level=restricted` → у `restricted` workload'а нет нарушений, у workload'а без `seccompProfile` — «Seccomp»;
  - шаблон `kind: PodMonitor` в chart'е → строка `prometheus.deckhouse.io/monitor-watcher-enabled`;
  - PrometheusRule во входе с метками `{prometheus: main}` → `Missing: component: rules`;
  - chart с `templates/prometheus-rules.yaml` и values `prometheusRules.labels: {prometheus: main, component: rules}` → строки нет;
  - вход `GrafanaDashboardDefinition` → раздел Platform с альтернативой `ObservabilityDashboard`;
  - неверный `pss-level` → ошибка; library chart не трогается (через `ApplyFeatures`);
  - вход без объектов → `No findings.`; повторный вызов даёт байт-в-байт тот же текст.
- Golden (`tests/golden/deckhouse_test.go`, `TestDeckhouseReport`): `dhg generate -f tests/integration/fixtures/deckhouse-observability --chart-name app --with deckhouse-report` в режимах universal, separate, umbrella; `checkCharts` (Helm lint/template); затем в `docs/deckhouse-report.md` соответствующего chart'а (в separate/umbrella — chart'а группы `orders`/`legacy`) есть подстроки: `Deployment/orders | restricted`, `Deployment/legacy | privileged`, `HostPath Volumes: spec.volumes[0].hostPath`, `prometheus.deckhouse.io/monitor-watcher-enabled`, `prometheus.deckhouse.io/rules-watcher-enabled`, `PrometheusRule/orders`, `component: rules`, `ConfigMap/orders-dashboard`, `security.deckhouse.io/pod-policy`.
- Существующие golden-сценарии на новой фикстуре (все режимы, fidelity) проходят без изменений кода, кроме возможности.

## Критерии приёмки

- [ ] `dhg features` показывает `deckhouse-report` с параметром `pss-level=baseline`.
- [ ] AC2 из spec.md выполнен.
- [ ] Фикстура проходит все сценарии `TestGeneratedChartsPassHelm` и `TestFeaturesPassHelm` (включая `all/<mode>`).
- [ ] Отчёт детерминирован (два запуска — одинаковые файлы).
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

Раздел DexAuthenticator (задача 10), проверка `OperationPolicy` (задача 12), метки `prometheus-rules` (задача 07).
