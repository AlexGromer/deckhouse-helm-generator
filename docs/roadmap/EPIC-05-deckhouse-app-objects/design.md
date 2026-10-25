# EPIC-05: дизайн

## 1. Обзор архитектуры

```
extractor ──► processors ─────────────► analyzer ──────────────► generator ──► features (--with) ──► запись
              + PodLoggingConfig (05)   + label: PodLoggingConfig   шаблон pod'а:     deckhouse-report (02)
              ~ DexAuthenticator v2 (10)  → workload (05)           securityContext   deckhouse-logging (06)
                                        + annotation: auth-url →    defaults (03)     prometheus-rules +labels (07)
                                          DexAuthenticator (10)                       pss-restricted (04, draft)
                                        + reference: pod label →                      deckhouse-dashboards (08, draft)
                                          SecurityPolicyException (11)
                       pkg/compliance/pss (01) ◄── используется deckhouse-report и dhg fix
```

Решение «процессор или возможность» принято по правилу:

- **процессор** — если объект пришёл во входе и его надо правильно сгруппировать и шаблонизировать (`PodLoggingConfig`, `DexAuthenticator`);
- **детектор** — если нужно знать связь между объектами входа (Ingress → DexAuthenticator, workload → `SecurityPolicyException`, `PodLoggingConfig` → workload);
- **возможность `--with`** — если dhg добавляет то, чего во входе нет (отчёт, `PodLoggingConfig` для workload'ов, метки правил, ужесточение). Возможность выключается в values и проверяется golden-набором автоматически (ADR-046);
- `--deckhouse-module` не трогаем: это каркас **модуля** Deckhouse (`pkg/generator/modulescaffold.go`), то есть платформенный путь поставки, а не app-chart.

## 2. Компоненты и изменения

### 2.1. `pkg/compliance/pss` (новый пакет, задача 01)

```go
package pss

type Level int
const (
    Privileged Level = iota
    Baseline
    Restricted
)
func (l Level) String() string            // "privileged" | "baseline" | "restricted"
func ParseLevel(s string) (Level, error)  // регистронезависимо

// Violation — нарушение одного контроля.
type Violation struct {
    Level   Level  // уровень, к которому относится контроль (Baseline или Restricted)
    Control string // имя контроля как в стандарте: "Host Namespaces", "Seccomp", ...
    Field   string // путь: "spec.containers[0].securityContext.privileged"
    Value   string // фактическое значение (fmt %v) или "<unset>"
}

// Result — оценка одного pod spec'а.
type Result struct {
    Level      Level       // наивысший выполненный уровень
    Violations []Violation // все нарушения Baseline и Restricted, по порядку Field
}

// Evaluate оценивает pod spec (map из unstructured) и аннотации pod'а
// (контроль AppArmor читает container.apparmor.security.beta.kubernetes.io/*).
// Никогда не паникует на неожиданных типах: поле неверного типа считается
// заданным и недопустимым.
func Evaluate(podSpec map[string]interface{}, podAnnotations map[string]string) Result

// PodSpec возвращает pod spec workload'а, аннотации pod-шаблона и путь к spec
// ("spec.template.spec"). ok=false для kind'ов без pod-шаблона.
func PodSpec(obj *unstructured.Unstructured) (spec map[string]interface{}, annotations map[string]string, path string, ok bool)

// ViolationsOf возвращает нарушения, мешающие уровню target.
func (r Result) ViolationsOf(target Level) []Violation
```

Пакет не зависит от `generator` (без циклов) и используется в `pkg/generator` (`deckhouse-report`, `dhg fix`), а позже — `dhg analyze` (EPIC-11 профиль `deckhouse`).

### 2.2. `pkg/generator/features_deckhouse.go` (новый файл)

Регистрирует `deckhouse-report` (02), `deckhouse-logging` (06), позже `pss-restricted` (04), `deckhouse-dashboards` (08). Отчёт — по образцу `ResourceReport` (`pkg/generator/features_ops.go:214`): тип `DeckhouseReport` с методом `Markdown() string`, путь `const DeckhouseReportPath = "docs/deckhouse-report.md"`, ресурсы chart'а — `opsChartResources(chart, fc.Graph)` (`features_ops.go`), шаблоны мониторинга — `resourceTemplates(chart, isKind(...))` (`features_observability.go:310`).

```go
type DeckhouseReport struct {
    ChartName  string
    Target     pss.Level
    Workloads  []WorkloadPSS     // Kind/name, Result
    NSLabels   []NamespaceLabel  // Label, Why, Objects
    Monitoring []MonitoringIssue // Object, Missing []string
    Platform   []PlatformObject  // Object, Scope, Alternative
    Dex        []DexFinding      // задача 10
    Sidecar    []string          // ConfigMap names
}
func (r DeckhouseReport) Markdown() string
func buildDeckhouseReport(chart *types.GeneratedChart, fc FeatureContext) (DeckhouseReport, error)
```

### 2.3. `pkg/processor/k8s/podloggingconfig.go` (новый, задача 05)

`PodLoggingConfigProcessor`, GVK `deckhouse.io/v1alpha1 PodLoggingConfig`, приоритет 50 (как остальные процессоры Deckhouse), регистрация в `RegisterAll` в блоке «Deckhouse CRDs».

### 2.4. Детекторы

| Файл | Изменение | Задача |
|---|---|---|
| `pkg/analyzer/detector/label.go` | ветка `case "PodLoggingConfig"`: `spec.labelSelector` (`metav1.LabelSelectorAsSelector`) → workload'ы того же namespace, `RelationLabelSelector`, `Field: "spec.labelSelector"` | 05 |
| `pkg/analyzer/detector/annotation.go` | функция `dexAuthenticatorFromAuthURL(url string) (name, namespace string, ok bool)`; связь `RelationDeckhouse`, `Field: "metadata.annotations[nginx.ingress.kubernetes.io/auth-url]"`; эвристика `deckhouse.io/*auth*` остаётся | 10 |
| `pkg/analyzer/detector/reference.go` | ссылки из меток pod-шаблона на `SecurityPolicyException` (`RelationNameReference`) | 11 |

### 2.5. Шаблон pod'а (задача 03)

`pkg/processor/k8s/podtemplate.go:podTemplate` и `containerTemplate`: блоки `securityContext` меняются с

```
{{- with .podSecurityContext }}
securityContext:
  {{- toYaml . | nindent 8 }}
{{- end }}
```

на

```
{{- with merge (deepCopy (.podSecurityContext | default dict)) (deepCopy ((($.Values.global).securityContextDefaults).pod | default dict)) }}
securityContext:
  {{- toYaml . | nindent 8 }}
{{- end }}
```

(для контейнера — `.securityContext` и `…securityContextDefaults).container`). Sprig `merge $dst $src` — глубокое слияние, значения `$dst` (входа) побеждают; `deepCopy` защищает values от мутации. Конструкция `(($.Values.global).securityContextDefaults)` не падает, если ключа нет. Пустой результат `merge` = пустой dict → `with` ложен → блок не рендерится, как сейчас.

### 2.6. Запись `global.*` (задача 03)

Возможности сейчас добавляют только новые ключи верхнего уровня (`appendTopLevelValues`, `features.go`). Для `global.securityContextDefaults` нужен `setGlobalValue(valuesYAML, key string, value interface{}) (string, error)` в `pkg/generator/features.go`: разбор через `go.yaml.in/yaml/v3` (`yaml.Node`, уже используется в `pkg/synth/compose.go`), вставка ключа в map `global` с сохранением комментариев; ошибка, если ключ уже есть. Почему `global`: в umbrella-режиме subchart'ы видят только `global` родителя, а шаблон pod'а одинаков во всех режимах.

## 3. Модель данных

- Связи: используются существующие `types.RelationLabelSelector`, `types.RelationDeckhouse`, `types.RelationNameReference` (`pkg/types/relationship.go`); новых типов связей эпик не вводит, поэтому контракт EPIC-03 не нужен.
- Values: см. [spec.md §4.2](spec.md#42-values).
- Отчёт: `DeckhouseReport` (§2.2), рендерится в Markdown, в values не попадает.

## 4. Алгоритмы

### 4.1. Оценка PSS (R1)

1. Собрать контейнеры: `containers`, `initContainers`, `ephemeralContainers` (с индексом и именем списка для `Field`).
2. Baseline — проверить каждый контроль F14, записывая нарушения с `Level: Baseline`. Значения сравниваются как строки после `fmt.Sprint` для скаляров; булевы — `true`/`false` (строка `"true"` из YAML тоже считается true: unstructured хранит тип YAML).
3. Restricted — то же для контролей Restricted. Правила «поле контейнера может быть не задано, если задано на уровне pod'а» (Running as Non-root, Seccomp) реализуются так: нарушение только если `pod.securityContext.X` не удовлетворяет **и** хотя бы один контейнер не задаёт допустимое значение. Для `runAsNonRoot` явное `false` у контейнера — нарушение независимо от pod'а.
4. `Level` = `Restricted`, если нарушений нет; `Baseline`, если нет нарушений Baseline; иначе `Privileged`.
5. `spec.os.name == "windows"` → пропустить Linux-only контроли (Privilege Escalation, Seccomp Restricted, Capabilities Restricted).

### 4.2. Метки namespace (R6)

`kinds := {ServiceMonitor, PodMonitor → monitor-watcher-enabled; PrometheusRule → rules-watcher-enabled; ScrapeConfig → scrape-configs-watcher-enabled; Probe → probe-watcher-enabled}` по шаблонам chart'а (`resourceTemplates`), группа API — `monitoring.coreos.com`. Плюс строка `security.deckhouse.io/pod-policy: <min level>` (R4), если в chart'е есть хотя бы один workload.

### 4.3. Метки объектов мониторинга (R7)

Для объектов из графа — `obj.GetLabels()` входа. Для шаблона `templates/prometheus-rules.yaml` (возможность) — `prometheusRules.labels` из values chart'а (`yaml.Unmarshal(chart.ValuesYAML)`, в separate/umbrella — values subchart'а).

### 4.4. Ссылка Ingress → DexAuthenticator (R14)

```
u := url.Parse(annotation["nginx.ingress.kubernetes.io/auth-url"])
host := u.Hostname()                     // "orders-dex-authenticator.shop.svc.cluster.local"
labels := strings.Split(host, ".")
if len(labels) >= 3 && labels[2] == "svc" && strings.HasSuffix(labels[0], "-dex-authenticator"):
    name := strings.TrimSuffix(labels[0], "-dex-authenticator"); ns := labels[1]
    связь с DexAuthenticator{name, ns}, если он есть в allResources
```

Ограничение (F12): длинное имя Service может быть усечено Deckhouse — тогда префикс не совпадёт, связь не строится, а отчёт пишет «protected routes: not found». Регистрация связи не меняет группировку (группы назначаются по `ServiceName` процессоров, `pkg/analyzer/analyzer.go:groupResources`), она нужна графу (`dhg graph`), отчёту и проверке целостности.

### 4.5. `deckhouse-logging` (R12)

```
для каждого r из opsChartResources(chart, graph), r — workload (Deployment|StatefulSet|DaemonSet|Job|CronJob|Rollout):
    sel := spec.selector.matchLabels (CronJob: spec.jobTemplate.spec.selector.matchLabels; Job без селектора — метки pod-шаблона)
    если sel пуст → пропустить, записать в NOTES
    если в графе есть PodLoggingConfig того же namespace, чей labelSelector.matchLabels ⊆ меток pod-шаблона r → пропустить
    workloads[r.name] = {matchLabels: sel}
values.deckhouseLogging = {enabled: true, clusterDestinationRefs: params.destinations, workloads: ...}
```

Шаблон — один файл, `range` по `workloads`; рендер только при `enabled` и непустом `clusterDestinationRefs`.

## 5. Альтернативы

| Вариант | Плюсы | Минусы | Решение |
|---|---|---|---|
| Вычислять PSS по отрендеренному YAML (`helm template` внутри dhg) | учитывает все values | dhg не вызывает Helm; новая зависимость | нет: по ADR-053 рендер значений по умолчанию = вход, оцениваем вход |
| Встроить проверку PSS в `--with policies` (Kyverno) | уже есть | Kyverno не используется в DKP (там Gatekeeper); это политика в кластере, а не отчёт | нет; отчёт отдельно |
| Генерировать `Namespace` с метками DKP | одно место | namespace — объект платформы/проекта (граница), один namespace на релиз | нет; только отчёт |
| Конвертировать `GrafanaDashboardDefinition` во входе автоматически | меньше ручной работы | меняет семантику и область видимости, нарушает fidelity | только opt-in (задача 08, `replace-legacy`) |
| Ужесточение `securityContext` текстовой вставкой (как `pss.go`) | без изменения шаблона pod'а | хрупко (баг F19), дублирующиеся ключи YAML | нет; слияние с `global.securityContextDefaults` (задача 03) |
| Отдельная команда `dhg check deckhouse` вместо возможности | не трогает chart | второй конвейер; golden не прогоняет автоматически | нет; возможность `deckhouse-report`, позже — вывод в `dhg analyze` |

## 6. ADR-кандидаты

- **ADR-кандидат: Объекты Deckhouse приложения поддерживаются процессорами (вход) и возможностями `--with` (добавление); cluster-scoped объекты Deckhouse dhg не создаёт, а называет namespaced-альтернативу в `docs/deckhouse-report.md`.** Контекст: граница проекта; ранее Deckhouse-поддержка была только платформенной. Последствия: новые kind'ы платформы не получают процессоров-генераторов.
- **ADR-кандидат: Уровень PSS вычисляется собственной реализацией контролей Kubernetes PSS (`pkg/compliance/pss`) без зависимостей.** Контекст: `k8s.io/pod-security-admission` тянет `k8s.io/client-go` и др. (ADR-051 — минимум зависимостей); DKP применяет PSS через Gatekeeper, а не PSA. Последствия: таблица контролей сопровождается вручную, сверка с F14 при обновлении Kubernetes.
- **ADR-кандидат: Значения `securityContext`, не являющиеся фактами, задаются только через `global.securityContextDefaults` и сливаются с входом так, что вход побеждает.** Контекст: ADR-059 (только факты), баг `pss.go`. Последствия: шаблон pod'а меняется один раз; любые ужесточения видны в values и выключаются удалением ключа.

## 7. Влияние на существующее поведение

- Задача 03 меняет текст шаблона pod'а всех workload'ов. Без `global.securityContextDefaults` рендер идентичен (AC8 проверяет на всех golden-входах). Пользовательские `--template-dir`, переопределяющие шаблоны workload'ов, не затронуты.
- `dhg fix` перестаёт вставлять блок после `image:`; результат меняется (становится корректным). `readOnlyRootFilesystem: true` и `runAsNonRoot: true` `dhg fix` продолжает ставить (это его контракт «best practices», а не PSS), но через defaults.
- Процессор `PodLoggingConfig`: путь шаблона тот же, что у generic fallback (`processor.TemplatePathForResource` → `templates/podloggingconfig-<name>.yaml`), ключ values тот же по форме (`services.<svc>.podLoggingConfig.spec`, `processor.kindToValuesKey`), но (а) `<svc>` теперь выводится из `spec.labelSelector.matchLabels`, а не из меток/имени самого объекта; (б) исчезает ключ `services.<svc>.podLoggingConfig.enabled` generic'а (включение — через `services.<svc>.enabled`, как у `PrometheusRule`). Пользователи `--set` по старому пути должны его сменить — отметить в `RELEASE_NEXT.md`.
- DexAuthenticator `v2alpha1` переходит с generic fallback на процессор: шаблон `templates/dexauthenticator-<name>.yaml` (совпадает), группа — `metadata.name` вместо `processor.ServiceNameFromResource` (метки или имя), исчезает `services.<svc>.dexAuthenticator.enabled` generic'а — отметить в `RELEASE_NEXT.md`.

## 8. Тестирование

- **Unit:** `pkg/compliance/pss/pss_test.go` (табличные тесты по контролям); `pkg/generator/features_deckhouse_test.go` (сборка отчёта, параметры, ошибки); `pkg/processor/k8s/podloggingconfig_test.go`; `pkg/analyzer/detector/{label,annotation,reference}_test.go` — новые случаи; `pkg/processor/k8s/podtemplate_test.go` (новый) — слияние defaults.
- **Golden:** новая фикстура `tests/integration/fixtures/deckhouse-observability/` (задача 02) автоматически попадает во все сценарии (`inputs()` в `tests/golden/golden_test.go:62`) и в `TestFeaturesPassHelm` «all/<mode>»; новый тест `tests/golden/deckhouse_test.go` с проверками фактов рендера и отчёта (AC2–AC8).
- **Integration:** `tests/integration/pipeline_deckhouse_test.go` — новый случай с фикстурой (группы сервисов: `PodLoggingConfig` и `ServiceMonitor` в группе `orders`).

## 9. Запросы к другим эпикам

- **EPIC-11:** профиль `deckhouse` = `with: [deckhouse-report, deckhouse-logging?, …]` и `prometheus-rules.labels=prometheus=main,component=rules`; профиль должен ставить `deckhouse-report` **последним** в `with` (возможности применяются по порядку, отчёт видит только уже добавленные шаблоны).
- **EPIC-08:** отчёт R15 переиспользует связь Ingress → DexAuthenticator; `DexClient` (EPIC-08) — отдельный объект, общих типов нет.
- **EPIC-04:** при генерации `CiliumNetworkPolicy` учесть, что pod'ы DexAuthenticator (`<NAME>-dex-authenticator`) обращаются к Dex и Redis — это не объекты chart'а; запрос информационный.

## 10. Масштабирование и развитие

- Те же контроли PSS можно выводить в `dhg analyze` (`pkg/analyzer/pattern`) без генерации chart'а.
- Когда модуль `observability` станет единственным механизмом (документация: `GrafanaDashboardDefinition` «will be removed»), `deckhouse-dashboards` и `prometheus-rules.kind=ObservabilityMetricsRulesGroup` станут умолчанием профиля `deckhouse` (EPIC-11).
- Проверка `OperationPolicy` (задача 12) расширяется на `SecurityPolicy` теми же средствами (`pkg/compliance`).
