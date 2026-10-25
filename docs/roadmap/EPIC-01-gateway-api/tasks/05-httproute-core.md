# EPIC-01 / 05: `--with gateway-api` — HTTPRoute из Ingress, режим `route`, переключатели values

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | L |
| Зависит от | EPIC-01/01, EPIC-01/02, EPIC-01/04 |
| Требования | R1–R12, R21–R23 |

## Контекст

Модель `IngressFacts` (задача 02) даёт хосты, пути и бэкенды Ingress. Задача регистрирует feature `gateway-api` и реализует ядро: `HTTPRoute` на группу хостов, перевод путей и бэкендов, лимиты CRD, `defaultBackend`, values `gatewayAPI`, шаблон, правку шаблона Ingress (`disableIngress`) и режим привязки `route` (к существующему `Gateway` платформы). Режим `listenerset`, TLS и HTTP→HTTPS — задача 06 (там же значение `mode` по умолчанию меняется на `listenerset`); аннотации — 07.

Образцы в коде: регистрация и параметры — `features_observability.go` (`istio`), шаблоны на основе `resourceTemplate` — `istiotraffic.go` (`applyIstioFeature`, `svc.wrap`, `svc.labels()`), правка шаблона Ingress регулярным выражением — `ingresstls.go` (`injectIngressTLSBlocks`), values — `addFeatureValues`, путь шаблона — `featureTemplatePath`.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/generator/features_gatewayapi.go` (новый) | `init()` → `RegisterFeature{Name: "gateway-api", Description: "Gateway API: HTTPRoute (and ListenerSet/GRPCRoute) for every Ingress, Ingress switchable off via values", Params: {"mode": "route", "parent": "", "http-section": "", "https-section": "", "ingress": "keep"}}`; `gatewayAPIOptions`, `parseGatewayAPIOptions`, `applyGatewayAPIFeature` (design §2.3) |
| `pkg/generator/gatewayapi_translate.go` (новый) | `translateIngress` (без TLS-логики и аннотаций), `pathMatch`, `backendRef`, `groupRules`, `splitRoute`, `boundedName`, тип `routeValues`, `ingressTranslation.values()` |
| `pkg/generator/gatewayapi_template.go` (новый) | `gatewayAPITemplate(rt, ingressName, opts)` (HTTPRoute; блоки ListenerSet/Certificate/ReferenceGrant/GRPCRoute добавят задачи 06–08), `guardIngressTemplate(content)` |
| `pkg/generator/features_gatewayapi_test.go`, `gatewayapi_translate_test.go`, `gatewayapi_template_test.go` (новые) | Unit-тесты |
| `tests/integration/fixtures/ingress-nginx-app/` (новый) | `ingress.yaml`, `services.yaml`, `deployments.yaml` (ниже) |
| `tests/golden/gatewayapi_test.go` (новый) | `TestGatewayAPIFeature` |
| `README.md` | Строка `gateway-api` в таблице раздела «Опциональные возможности (`--with`)»; абзац: values `gatewayAPI.*`, переключение Ingress, `parent` не выводится |
| `docs/RELEASE_NEXT.md` | Пункт о новой feature |

## Шаги

1. Параметры: `mode` ∈ {`route`} (в этой задаче; `listenerset` добавит 06), `parent` — `<ns>/<name>` или `<name>` (больше одного `/` → ошибка), `ingress` ∈ {`keep`, `disable`}; ошибки — `fmt.Errorf("gateway-api: …")`.
2. `applyGatewayAPIFeature`: (а) `facts, notes := CollectIngressFacts(fc.Graph)`; заметки модели — `fc.Note("gateway-api: %s", n)`; (б) оставить `InChart(chart)`; нет — вернуть `chart`; (в) шаблоны Ingress: `resourceTemplates(chart, isKind("Ingress"))`, сопоставить по `rt.path == f.TemplatePath`; не найден → `Note` и пропуск; (г) `translateIngress`; (д) `out := cloneChart(chart)`, `addTemplate(out, featureTemplatePath("gateway-api", rt.path), gatewayAPITemplate(...))`, `out.Templates[rt.path], ok = guardIngressTemplate(...)` (не `ok` → `Note`, шаблон Ingress не трогать); (е) `addFeatureValues(out, "gatewayAPI", {enabled: true, disableIngress: opts.ingress == "disable", parent: {name, namespace}, sections: {http, https}, ingresses: {...}})`.
3. `translateIngress` (режим `route`): по группе хостов — маршрут (`Name` по R9), `Attach`: `[{listener: http}]` для хоста без TLS, `[{listener: http}, {listener: https}]` для хоста из `spec.tls` (перенаправление — задача 06); `pathMatch` (R4, R5; `ImplementationSpecific`: контроллер nginx → `PathPrefix` + `Note`, иначе пропуск + `Note`; ветка `use-regex` — задача 07); `backendRef` (R6); `groupRules` и `splitRoute` (R7, design §4.4); `defaultBackend` → `<ingress>-default` с `Attach` как у первого маршрута и без `hostnames` (R8) + `Note`; пустой `parent` → `Note: gateway-api: Ingress <ns>/<name>: parent Gateway is not set (gatewayAPI.parent); HTTPRoutes are rendered without parentRefs`.
4. `boundedName(name, suffix)`: если `len(name+suffix) ≤ 253` → `name+suffix`; иначе `name[:253-len(suffix)-9] + "-" + %08x(FNV-32a(name)) + suffix` + `Note`.
5. Шаблон: схема design §2.6 (HTTPRoute и рендер `parentRefs` из `attach`), labels — `rt.labels()`, первая строка — `rt.prefix[0]` (`$svc := …`); проверить `balancedControl(labels[1:])`, как `applyIstioFeature`.
6. `guardIngressTemplate`: design §4.5.
7. Fixture и golden-тест (ниже); README, RELEASE_NEXT.

## Тесты

- Unit `gatewayapi_translate_test.go` (вход — `IngressFacts`, построенные в тесте):
  - `TestTranslate_PrefixExact`: `Prefix /api` → `{type: PathPrefix, value: /api}`; `Exact /x` → `Exact`.
  - `TestTranslate_ImplementationSpecific`: nginx → `PathPrefix` + 1 заметка; unknown → путь пропущен + 1 заметка.
  - `TestTranslate_InvalidPath`: `Prefix /a//b`, `/a#b`, `/a/..` → пропуск + заметка с причиной (R5).
  - `TestTranslate_PortName`: `PortNumber 8080` (разрешён моделью) → `{name: orders, port: 8080}`; `PortNumber 0` → пропуск, заметка.
  - `TestTranslate_ResourceBackend`: → пропуск, заметка.
  - `TestTranslate_GroupRules`: пути `/a`→web:80, `/b`→orders:8080, `/c`→web:80 → правила `[{matches:[/a,/c], web}, {matches:[/b], orders}]`.
  - `TestTranslate_SplitRules`: 17 различных бэкендов → маршруты `shop` (16 правил) и `shop-part2` (1 правило).
  - `TestTranslate_SplitMatches`: 129 путей на один бэкенд → правила по 64, 64, 1; маршрут делится по сумме 128 → `shop` (2 правила, 128 matches), `shop-part2` (1 правило).
  - `TestTranslate_HostGroups`: два хоста → `admin`, `admin-2`; пустой хост → маршрут без `hostnames`.
  - `TestTranslate_DefaultBackend`: → `admin-default`, одно правило без `matches`.
  - `TestBoundedName`: 250-символьное имя + `-redirect` → длина 253, суффикс сохранён, детерминированно.
- Unit `gatewayapi_template_test.go`:
  - `TestGuardIngressTemplate`: шаблон Ingress получить из `k8s.NewIngressProcessor().Process(processor.Context{ChartName: "app"}, obj)` (пакет `pkg/processor/k8s` не импортирует `pkg/generator`, цикла нет) — строка `{{- if .enabled }}` после `{{- with $svc.ingress }}` заменена; тот же шаблон с первой строкой `{{- $svc := .Values -}}` (форма режима separate) — заменена; повторный вызов не меняет; шаблон Service → `false`.
  - `TestGatewayAPITemplate`: `parseResourceTemplate` шаблона Ingress из процессора → `gatewayAPITemplate` содержит первую строку `$svc := .Values.services.<svc>`, `kind: HTTPRoute`, `index $gw.ingresses "shop"` и блок labels Ingress (рендер проверяет golden).
- Unit `features_gatewayapi_test.go`: параметры (неверный `mode`, `parent=a/b/c`, `ingress=x` → ошибка); chart без Ingress не меняется (`reflect.DeepEqual` с входом); values содержат `gatewayAPI.ingresses.shop.httpRoutes[0].name == "shop"`.
- Fixture `tests/integration/fixtures/ingress-nginx-app/` (namespace `shop`):
  - `deployments.yaml`: Deployment `orders` (метки `app: orders`, контейнер `orders`, образ `registry.example.com/shop/orders:1.0`, порт 8080), Deployment `web` (`app: web`, `registry.example.com/shop/web:1.0`, порт 80);
  - `services.yaml`: Service `orders` (`selector app: orders`, `ports: [{name: http, port: 8080, targetPort: 8080}]`), Service `web` (`selector app: web`, `ports: [{name: http, port: 80, targetPort: 80}]`);
  - `ingress.yaml`: Ingress `shop` — как spec §4.3; Ingress `admin` (`ingressClassName: nginx`): правило `admin.example.com` с путями `/exact` (`Exact`) → `web:80` и `/reports` (`Prefix`) → `reports` `port.name: http` (Service `reports` во входе нет); правило `api.example.com` `/` (`Prefix`) → `orders` `port.number: 8080`; `defaultBackend` → `web` `port.number: 80`.
- Golden `tests/golden/gatewayapi_test.go`, `TestGatewayAPIFeature` (helpers `requireHelm`, `runOK`, `dhgBin`, `parseStream`, `findCharts`; рендер всех не-library chart'ов вывода, как `checkReleaseIntegrity`), режимы `universal`, `separate`, `umbrella`, аргументы `--with gateway-api --feature-opt gateway-api.parent=d8-alb/public-gw`:
  - есть `HTTPRoute shop`: `parentRefs == [{group: gateway.networking.k8s.io, kind: Gateway, name: public-gw, namespace: d8-alb}]`, `hostnames == [shop.example.com]`, правила `[{matches: [{path: {type: PathPrefix, value: /api}}], backendRefs: [{name: orders, port: 8080}]}, {matches: [{path: {type: PathPrefix, value: /}}], backendRefs: [{name: web, port: 80}]}]`;
  - `HTTPRoute admin`: одно правило `Exact /exact` → `web:80`; `HTTPRoute admin-2`: `hostnames == [api.example.com]`, `PathPrefix /` → `orders:8080`; `HTTPRoute admin-default`: без `hostnames`, правило без `matches` → `web:80`;
  - stderr содержит `Note: gateway-api: ` и `reports`;
  - `Ingress shop` и `Ingress admin` есть; с `--set gatewayAPI.disableIngress=true` (`helm template … --set`) — нет `kind: Ingress`, `HTTPRoute` есть; с `--set gatewayAPI.enabled=false` — нет `HTTPRoute`;
  - без `parent`: у `HTTPRoute` нет ключа `parentRefs`, stderr содержит `parent Gateway is not set`;
  - все объекты проходят `checkCRSchemas` (задача 04).

## Критерии приёмки

- [ ] Golden `TestGatewayAPIFeature` и `TestFeaturesPassHelm` (включая `--with <все>` во всех режимах) проходят.
- [ ] Новый fixture проходит все сценарии `TestGeneratedChartsPassHelm` (fidelity, integrity) без feature.
- [ ] Chart без Ingress не меняется при `--with gateway-api`.
- [ ] README и `docs/RELEASE_NEXT.md` обновлены.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

`ListenerSet`, TLS-слушатели, перенаправление HTTP→HTTPS, `Certificate`, `ReferenceGrant` — задача 06. Аннотации (`use-regex`, rewrite, CORS…) — 07. gRPC — 08.
