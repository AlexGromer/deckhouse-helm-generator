# EPIC-01: дизайн

## 1. Обзор архитектуры

Новое появляется на этапе features (`cmd/dhg/main.go`, шаг `[4k/5]`, `generator.ApplyFeatures`) и в процессорах (задача 11). Конвейер не меняется:

```
extract → process (IngressProcessor, Service…) → analyze (graph) → generate (charts)
   → post-processing flags (--detect-ingress, --namespace-resources …)
   → features (--with … gateway-api …)          ← EPIC-01
         │
         ├─ CollectIngressFacts(graph)            pkg/generator/ingressmodel.go   (общая модель, задача 02)
         │     IngressFacts: хосты, пути, бэкенды с разрешённым портом, TLS, аннотации, контроллер
         ├─ translateIngress(facts, opts)         pkg/generator/gatewayapi_translate.go (05, 06)
         │     └─ applyNginxAnnotations(...)      pkg/generator/gatewayapi_annotations.go (07, 09)
         ├─ values: gatewayAPI.ingresses.<имя>    appendTopLevelValues
         ├─ шаблон templates/gateway-api-<Ingress>.yaml   (рендер из values)
         ├─ правка шаблона Ingress: guard disableIngress
         └─ fc.Note(...) → stderr "Note: …"       (задача 01)
```

`--with istio-ingress` (параллельная работа на `main`) после задачи 03 строит VirtualService/Gateway Istio из того же `IngressFacts`.

Почему перевод выполняется при генерации, а не в Helm-шаблоне: правила (разрешение имени порта через Service, группировка правил, лимиты CRD, разбор аннотаций) требуют фактов из графа и тестируются unit-тестами на Go. Результат — данные в схеме Gateway API в values: пользователь меняет их как обычные values, шаблон только сериализует (`toYaml`).

## 2. Компоненты и изменения

### 2.1 `pkg/generator/features.go` (задача 01)

```go
// FeatureContext gains a note sink shared by all charts of one ApplyFeatures call.
type FeatureContext struct {
    Graph  *types.ResourceGraph
    Params map[string]string
    notes  *noteSink // nil-safe
}

// Note records a line printed by `dhg generate` as "Note: <line>".
// Identical lines from several charts (separate/umbrella modes) are printed once.
func (fc FeatureContext) Note(format string, args ...interface{})

type noteSink struct{ seen map[string]bool; lines []string }

// ApplyFeatures now also returns the notes in first-recorded order.
func ApplyFeatures(charts []*types.GeneratedChart, names []string,
    options map[string]map[string]string, graph *types.ResourceGraph,
) ([]*types.GeneratedChart, []string, error)
```

`cmd/dhg/main.go` (блок `// Apply optional features (--with)`) печатает каждую строку как `fmt.Fprintf(os.Stderr, "Note: %s\n", n)` и при `--dry-run`. Если синтетический источник (`pipeline.synthesis`) активен, заметки features дописываются в `SYNTHESIS.md` через существующий `synth.Report(..., notes)` (аргумент `notes` объединяется).

### 2.2 `pkg/generator/ingressmodel.go` (задача 02, новый файл)

```go
// IngressFacts is the controller-independent model of one input Ingress,
// shared by the gateway-api and istio-ingress features.
type IngressFacts struct {
    Key          types.ResourceKey
    Name         string
    Namespace    string
    ClassName    string            // spec.ingressClassName, else annotation kubernetes.io/ingress.class
    Controller   IngressController // per Ingress, §3.2 of spec.md
    Annotations  map[string]string
    TemplatePath string            // ProcessedResource.TemplatePath of the Ingress
    Hosts        []IngressHost     // grouped by host, first-appearance order
    TLS          []IngressTLS
    DefaultBackend *IngressBackend
}

type IngressHost struct {
    Host  string // "" = any host
    Paths []IngressPath
}

type IngressPath struct {
    Path     string // "/" when absent
    PathType string // "Prefix" when absent
    Backend  IngressBackend
}

type IngressBackend struct {
    ServiceName string
    PortNumber  int32  // from spec, or resolved from PortName; 0 when unresolved
    PortName    string // as written in the Ingress
    Service     *types.ProcessedResource // input Service, nil if absent
    Resource    map[string]interface{}   // non-Service backend, verbatim
}

type IngressTLS struct {
    Hosts      []string // as written; empty = all rule hosts (consumer decides)
    SecretName string
}

// CollectIngressFacts returns the facts of every networking.k8s.io/v1 Ingress
// of the graph, sorted by ResourceKey, plus notes for unresolvable backends.
func CollectIngressFacts(graph *types.ResourceGraph) ([]IngressFacts, []string)

// ingressControllerOf applies spec.md §3.2 to one Ingress.
func ingressControllerOf(ing *types.ProcessedResource, graph *types.ResourceGraph) IngressController

// InChart reports whether the Ingress is rendered by chart (its TemplatePath is in chart.Templates).
func (f IngressFacts) InChart(chart *types.GeneratedChart) bool
```

Факты читаются из `ProcessedResource.Original.Object` (вход), а не из values: `--detect-ingress` и `ingress-tls` меняют шаблон, но не вход. Константы `ControllerNginx`, `ControllerUnknown` берутся из `ingressdetect.go`.

### 2.3 `pkg/generator/features_gatewayapi.go` (задача 05, новый файл)

```go
func init() { RegisterFeature(Feature{Name: "gateway-api", Description: "...", Params: {...§5 spec}, Apply: applyGatewayAPIFeature}) }

type gatewayAPIOptions struct {
    Mode                     string // "listenerset" | "route"
    ParentNamespace, ParentName string
    HTTPSection, HTTPSSection   string
    HTTPPort, HTTPSPort         int
    DisableIngress           bool
    Certificates             string // "separate" | "reuse"
    ReferenceGrant           bool
    ReferenceGrantAPIVersion string
    CORS                     bool
    Implementation           string // "standard" | "alb"
}

func parseGatewayAPIOptions(fc FeatureContext) (gatewayAPIOptions, error)
func applyGatewayAPIFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error)
```

`applyGatewayAPIFeature`: (1) `CollectIngressFacts(fc.Graph)`, отфильтровать `InChart`; пусто → вернуть `chart` без изменений; (2) для каждого Ingress найти шаблон (`resourceTemplates(chart, isKind("Ingress"))` с `rt.path == facts.TemplatePath`), не распознан → `Note`; (3) `translateIngress`; (4) `cloneChart`, `addTemplate(featureTemplatePath("gateway-api", rt.path), gatewayAPITemplate(rt, name))`; (5) `guardIngressTemplate` для шаблона Ingress; (6) `addFeatureValues(out, "gatewayAPI", values)`.

### 2.4 `pkg/generator/gatewayapi_translate.go` (задачи 05, 06)

```go
type ingressTranslation struct {
    ListenerSet     map[string]interface{}   // nil in route mode
    Certificates    []map[string]interface{}
    ReferenceGrants []map[string]interface{}
    HTTPRoutes      []routeValues
    GRPCRoutes      []routeValues
}

type routeValues struct {
    Name        string
    Annotations map[string]string        // implementation=alb only
    Attach      []map[string]interface{} // each {listenerSet, sectionName} | {listener}
    Hostnames   []string
    Rules       []map[string]interface{} // Gateway API schema
}

func (t ingressTranslation) values() map[string]interface{}

func translateIngress(f IngressFacts, opts gatewayAPIOptions, note func(string, ...interface{})) ingressTranslation
func pathMatch(p IngressPath, f IngressFacts, note ...) (map[string]interface{}, bool)   // R4, R5
func backendRef(b IngressBackend, f IngressFacts, note ...) (map[string]interface{}, bool) // R6
func groupRules(paths []translatedPath) []map[string]interface{}                         // R7 grouping
func splitRoute(r routeValues) []routeValues                                              // R7 limits
func hostSlug(host string) string                                                         // R13
func boundedName(name, suffix string) string                                              // spec §7
func listenersFor(f IngressFacts, opts gatewayAPIOptions, note ...) (http, https map[string]string /*host→listener*/, listeners []interface{})
func certificatesFor(f IngressFacts, opts gatewayAPIOptions) ([]map[string]interface{}, map[string]string /*secret→ref*/)
func redirectRoute(main routeValues, httpAttach map[string]interface{}) routeValues      // R15
```

### 2.5 `pkg/generator/gatewayapi_annotations.go` (задачи 07, 09)

```go
// annotationResult is what the ingress-nginx annotations of one Ingress
// contribute to the translation.
type annotationResult struct {
    RuleFilters   []map[string]interface{} // appended to every rule that has backendRefs
    PathRewrite   func(p IngressPath) (match map[string]interface{}, filters []map[string]interface{}, ok bool)
    ExtraRules    []map[string]interface{} // app-root, redirects (first in the route)
    ForceRegex    bool                     // use-regex
    GRPC          bool                     // backend-protocol GRPC/GRPCS
    SSLRedirect   *bool                    // nil = controller default
    RouteAnnotations map[string]string     // implementation=alb
    Extended      []string                 // Extended features used, for the Note
}

func nginxAnnotations(f IngressFacts, opts gatewayAPIOptions, note func(string, ...interface{})) annotationResult
func parseNginxSize(s string) (int64, bool)  // proxy-body-size → bytes (alb)
func corsFilter(a map[string]string) map[string]interface{}
func redirectFilter(rawURL string, code int) (map[string]interface{}, error)
```

Таблица соответствия — `var nginxAnnotationTable = []nginxAnnotationRule{...}` (ключ, обработчик, «ключ alb»), чтобы каждая аннотация имела ровно одну строку и тест мог проверить, что неизвестные дают `Note`.

### 2.6 `pkg/generator/gatewayapi_template.go` (задача 05)

```go
// gatewayAPITemplate renders all Gateway API objects of one Ingress from
// .Values.gatewayAPI.ingresses.<name>. rt is the Ingress template: its first
// prefix line ($svc := ...) and metadata.labels are reused.
func gatewayAPITemplate(rt *resourceTemplate, ingressName string, opts gatewayAPIOptions) string

// guardIngressTemplate adds `(not $.Values.gatewayAPI.disableIngress)` to the
// `{{- if .enabled }}` line that follows `{{- with $svc.ingress }}` in the
// IngressProcessor template. Returns false for any other shape.
func guardIngressTemplate(content string) (string, bool)
```

Схема шаблона (фрагмент; `$svc`-строка берётся из `rt.prefix[0]`):

```
{{- /* Generated by dhg feature "gateway-api" from Ingress shop. */}}
{{- $svc := .Values.services.shop -}}
{{- $gw := .Values.gatewayAPI }}
{{- if and $svc.enabled $gw.enabled }}
{{- $ing := index $gw.ingresses "shop" }}
{{- range $ing.httpRoutes }}
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: {{ .name }}
  namespace: {{ $.Release.Namespace }}
<labels Ingress-шаблона>
  {{- with .annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  {{- $refs := list }}
  {{- $allSections := true }}
  {{- range .attach }}
  {{- if .listenerSet }}
  {{- $refs = append $refs (dict "group" "gateway.networking.k8s.io" "kind" "ListenerSet" "name" .listenerSet "sectionName" .sectionName) }}
  {{- else if not (index ($gw.sections | default dict) (.listener | default "-")) }}
  {{- $allSections = false }}
  {{- end }}
  {{- end }}
  {{- if $gw.parent.name }}
  {{- range .attach }}
  {{- if and .listener $allSections }}
  {{- $ref := dict "group" "gateway.networking.k8s.io" "kind" "Gateway" "name" $gw.parent.name "sectionName" (index $gw.sections .listener) }}
  {{- with $gw.parent.namespace }}{{ $_ := set $ref "namespace" . }}{{ end }}
  {{- $refs = append $refs $ref }}
  {{- end }}
  {{- end }}
  {{- if and (not $allSections) (not (first .attach).listenerSet) }}
  {{- $ref := dict "group" "gateway.networking.k8s.io" "kind" "Gateway" "name" $gw.parent.name }}
  {{- with $gw.parent.namespace }}{{ $_ := set $ref "namespace" . }}{{ end }}
  {{- $refs = append $refs $ref }}
  {{- end }}
  {{- end }}
  {{- with $refs }}
  parentRefs:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  {{- with .hostnames }}
  hostnames:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  rules:
    {{- toYaml .rules | nindent 4 }}
{{- end }}
… аналогично grpcRoutes, listenerSet (если $gw.parent.name), certificates, referenceGrants (условие R18) …
{{- end }}
```

Внутри `range` контекст `.` — элемент; `$` — корень, поэтому `include "<chart>.labels" $` из скопированного блока labels работает. Инвариант генерации: `attach` каждого маршрута непуст и однороден (все записи — `listenerSet` или все — `listener`), иначе `(first .attach).listenerSet` упал бы на `nil`; unit-тест `translateIngress` проверяет инвариант. `gatewayAPI.sections` записывается в values в обоих режимах (пустые строки), чтобы `index` не получал `nil`.

### 2.7 Процессоры (задача 11)

`pkg/processor/k8s/tlsroute.go`: `Supports()` — `v1alpha2` и `v1`; шаблон использует `apiVersion` входа (`obj.GetAPIVersion()`), а не константу. Новые файлы `listenerset.go` (`ListenerSet` v1, `ValuesPath` `services.<svc>.listenerSet`), `referencegrant.go` (`ReferenceGrant` v1 и v1beta1, `services.<svc>.referenceGrant`) — по образцу `httproute.go` с `processor.SpecOverlay`. Регистрация — `pkg/processor/k8s/registry.go`.

## 3. Модель данных

- **Вход:** `IngressFacts` (§2.2).
- **Промежуточная:** `ingressTranslation`, `routeValues` (§2.4).
- **Values:** ключ `gatewayAPI` (spec §4.2). Ключи `ingresses.<имя Ingress>` — исходные имена (в YAML — строки, в шаблоне — `index`).
- **Объекты:** `map[string]interface{}` → `sigs.k8s.io/yaml` (уже в `go.mod`).

## 4. Алгоритмы

### 4.1 Перевод одного Ingress (`translateIngress`)

1. `ann := nginxAnnotations(f, opts, note)` (только если `f.Controller == ControllerNginx`; иначе каждая `nginx.ingress.kubernetes.io/*` → `Note`).
2. Слушатели (режим `listenerset`): для каждого `IngressHost` с непустым хостом — HTTP; хосты TLS (R14; запись без `hosts` → все хосты) — HTTPS с `certificateRefs`; `certificatesFor` даёт `Certificate` при `separate` и аннотации issuer. Пустой хост → один слушатель без `hostname` на протокол + `Note`.
3. Для каждой группы хостов: перевести пути (`pathMatch`, `backendRef`, переписывания из `ann`), отброшенные — `Note`; сгруппировать (`groupRules`); добавить `ann.ExtraRules` первыми; при `ann.GRPC` — в `GRPCRoutes` (§3.4 spec), иначе `HTTPRoutes`.
4. Привязка (`Attach` — список, по записи на слушатель): хост без TLS → `[http]`; TLS-хост с перенаправлением (R15) → основной маршрут `[https]`, `redirectRoute` — `[http]`; TLS-хост без перенаправления (`ssl-redirect: "false"` или контроллер не nginx) → один маршрут `[http, https]`. В режиме `listenerset` записи — `{listenerSet, sectionName}`, в режиме `route` — `{listener: http|https}`.
5. `splitRoute` для каждого маршрута (R7), имена — `boundedName`.
6. `defaultBackend` → маршрут `<ingress>-default` (R8).
7. `ReferenceGrant` (R18) — режим `route`, `opts.ReferenceGrant`, есть TLS Secret.

### 4.2 Тип пути (`pathMatch`)

```
pathType Exact            → {type: Exact, value: path}
pathType Prefix | ""      → {type: PathPrefix, value: path}
ImplementationSpecific:
  nginx && ann.ForceRegex → {type: RegularExpression, value: path}
  nginx                   → {type: PathPrefix, value: path} + Note (Q6)
  иначе                   → пропуск + Note
ann.ForceRegex и Prefix/Exact → RegularExpression (ingress-nginx при use-regex трактует все пути хоста как regex)
проверка R5 для Exact/PathPrefix; ошибка → пропуск + Note
```

### 4.3 Порт бэкенда (`backendRef`)

`PortNumber > 0` → `{name, port}`. Иначе `PortName != ""`: `Service` из входа, `spec.ports[i].name == PortName` → `port = spec.ports[i].port`; иначе пропуск + `Note: … port name <x> of Service <ns>/<svc> is not in the input`. `targetPort` не используется: `backendRef.port` — порт Service.

### 4.4 Группировка и лимиты (`groupRules`, `splitRoute`)

Ключ группы — канонический JSON (`backendRefs`, `filters`). Правило = все `matches` группы (порядок первого появления), при > 64 — новое правило с тем же ключом. Затем маршрут делится последовательно: текущая часть принимает правило, пока правил ≤ 16 и сумма `matches` ≤ 128.

### 4.5 Правка шаблона Ingress (`guardIngressTemplate`)

Ищется последовательность строк `{{- with $svc.ingress }}` + `{{- if .enabled }}` (форма из `IngressProcessor.generateTemplate`, одинаковая во всех режимах — проверено на `examples/05-full-stack` в `separate`, `umbrella`, `library`). Вторая строка заменяется на `{{- if and .enabled (not $.Values.gatewayAPI.disableIngress) }}`. Правка не затрагивает блок `{{- with .annotations }}`, который ищут `ingress-tls` (`ingressAnnotationsBlockRe`) и `--detect-ingress`, поэтому порядок features не важен. Повторная правка исключается проверкой подстроки `gatewayAPI.disableIngress`.

## 5. Альтернативы

| Вариант | Плюсы | Минусы | Решение |
|---|---|---|---|
| Импортировать `sigs.k8s.io/ingress2gateway` | готовые правила, поддержка сообщества | `go.mod` ingress2gateway v1.2.0 тянет client-go, controller-runtime, helm v4, envoy gateway, kgateway, kong, istio api, gke — противоречит ADR-051; выход — объекты, а не values/шаблоны; эвристика таймаутов противоречит ADR-059 | отклонено; правила перенесены как спецификация (Apache-2.0 разрешает заимствование подхода; код не копируется) |
| Импортировать `sigs.k8s.io/gateway-api` ради типов | типобезопасность | новая зависимость ради сериализации; dhg везде работает с `unstructured` | отклонено; соответствие полей проверяет задача 04 по CRD |
| Перевод в Helm-шаблоне из `services.<svc>.ingress` | один источник правды (хосты правятся в одном месте) | разрешение портов, лимиты CRD и аннотации на языке шаблонов не тестируются unit-тестами; сложные шаблоны | отклонено; перевод при генерации, результат в values |
| Собственный `Gateway` приложения | самодостаточный chart | платформенный объект (scope, DKP запрещает правку управляемых Gateway) | отложено (Q2) |
| Один маршрут на Ingress (все хосты) | меньше объектов | `hostnames` маршрута общие для всех правил — пути одного хоста начали бы обслуживать другой | отклонено; маршрут на хост |

## 6. ADR-кандидаты

- **ADR-кандидат: перевод Ingress → Gateway API выполняется при генерации, результат хранится в values в схеме Gateway API.** Контекст: §5. Решение: `gatewayAPI.ingresses.<имя>` содержит готовые `rules`, `listeners`; шаблон сериализует. Последствия: изменение хостов Ingress в values не меняет маршруты — пользователь правит оба ключа или перегенерирует chart; это указано в README проекта.
- **ADR-кандидат: приложение не создаёт `Gateway`; точка входа — `ListenerSet` в namespace приложения.** Контекст: граница scope, модель DKP `alb`. Последствия: нужен `Gateway` с `allowedListeners`, допускающим namespace приложения (платформа).
- **ADR-кандидат: общая модель Ingress (`IngressFacts`) для всех features, читающих Ingress.** Последствия: изменения разбора Ingress делаются в одном месте; `istio-ingress` переводится на модель (задача 03).
- **ADR-кандидат: `FeatureContext.Note` — канал `Note:` для features.** Последствия: сигнатура `ApplyFeatures` меняется (единственный вызов — `cmd/dhg/main.go`).

## 7. Влияние на существующее поведение

- Без `--with gateway-api` вывод не меняется (кроме задачи 11: входные `TLSRoute v1`, `ListenerSet`, `ReferenceGrant` обрабатываются типизированными процессорами вместо generic fallback — `TemplatePath` меняется с generic на `templates/<svc>-tlsroute.yaml` и т. п.; содержимое рендера то же).
- С feature: шаблон Ingress получает дополнительное условие (рендер по умолчанию не меняется, `disableIngress: false`).
- Сигнатура `ApplyFeatures` (задача 01) — внутренний API, единственный вызов в `cmd/dhg/main.go`; тесты `pkg/generator/features*_test.go` обновляются.

## 8. Тестирование

- **Unit** (`pkg/generator/ingressmodel_test.go`, `gatewayapi_translate_test.go`, `gatewayapi_annotations_test.go`, `features_gatewayapi_test.go`): таблицы «вход Ingress → ожидаемые `rules`/`listeners`/`Note`» для каждого требования R3–R19; `guardIngressTemplate` на шаблонах всех режимов; лимиты (17 путей → 2 маршрута; 65 одинаковых бэкендов → 2 правила).
- **Golden** (`tests/golden/gatewayapi_test.go`, новый): fixture `tests/integration/fixtures/ingress-nginx-app/` (spec §4.3 + дополнительные Ingress: rewrite, CORS, gRPC, app-root, use-regex, defaultBackend, named port без Service); запуск настоящего `dhg` и `helm template`; проверка фактов рендера (AC2–AC7) и схем CRD (задача 04).
- **Golden общий:** `TestFeaturesPassHelm` автоматически включает `gateway-api`; новый fixture автоматически попадает во все сценарии `TestGeneratedChartsPassHelm` (fidelity — для Ingress fixture'а без feature).
- **Сверка с ingress2gateway (ручная, не в CI):** `ingress2gateway print --providers ingress-nginx --input-file <fixture>` для сравнения правил; расхождения описываются в PR.

## 9. Запросы к другим эпикам

- **EPIC-04:** `cilium-network-policy` должна разрешать ingress к бэкендам маршрутов от прокси шлюза. Gateway API не сообщает namespace прокси реализации — EPIC-04 использует факт «Service опубликован через Ingress/HTTPRoute» (отношение `name_reference` Ingress → Service уже есть в графе; для маршрутов, созданных этой feature, — тот же Service). Запрос: EPIC-04 трактует наличие `HTTPRoute`/`GRPCRoute` (входных или от `gateway-api`) как публикацию Service.
- **EPIC-03:** не требуется.
- **`istio-ingress` (main):** использовать `IngressFacts` (задача 03).

## 10. Масштабирование и развитие

- `BackendTLSPolicy` (standard v1) — когда во входе появятся факты о CA бэкенда (например, Secret с `ca.crt`, на который ссылается аннотация `proxy-ssl-secret`).
- `TLSRoute` для `ssl-passthrough` (ingress2gateway v1.1.0 делает это через standard TLSRoute).
- Профили других реализаций (Envoy Gateway, Istio) — по аналогии с `implementation=alb`: таблица `nginxAnnotationTable` получает столбец на реализацию.
- Обратный путь: генерация `HTTPRoute` из входа `image`/`compose`/`source` — нет фактов о хостах (ADR-059), не планируется.
