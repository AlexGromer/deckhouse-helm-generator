# EPIC-01 / 06: режим `listenerset` — ListenerSet, TLS-слушатели, HTTP→HTTPS, Certificate, ReferenceGrant

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | L |
| Зависит от | EPIC-01/05 |
| Требования | R13, R14, R15, R18, R12 (уточнение привязки), R9 |

## Контекст

Задача 05 привязывает маршруты к `Gateway` платформы целиком или к слушателям по `sections`. В Deckhouse модуль `alb` требует другой модели: приложение создаёт в своём namespace `ListenerSet` (хосты, порты 80/443, TLS с Secret того же namespace), привязанный к управляемому `Gateway` (`spec.parentRef`, для `ClusterALBInstance` — namespace `d8-alb`), а маршруты привязываются к слушателям `ListenerSet` (`kind: ListenerSet`, `sectionName`). Документация `deckhouse/deckhouse` (`main`): `docs/documentation/pages/user/network/ingress/alb/GATEWAY_API_RU.md`, раздел «Публикация приложения с ListenerSet и HTTPRoute». В Gateway API v1.6.3 `ListenerSet` — standard v1; «A ListenerSet must be able to reference a secret/backend in the same namespace as itself without a ReferenceGrant» (GEP-1713).

ingress-nginx по умолчанию перенаправляет HTTP→HTTPS кодом 308, если у Ingress есть TLS (`annotations.md`, раздел «Server-side HTTPS enforcement through redirect»); `ssl-redirect: "false"` отключает, `force-ssl-redirect: "true"` включает без TLS. Gateway API: `RequestRedirect` нельзя сочетать с `backendRefs` в одном правиле (CEL CRD), поэтому перенаправление — отдельный маршрут на HTTP-слушателе.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/generator/features_gatewayapi.go` | Параметры: `mode` ∈ {`listenerset`, `route`}, **по умолчанию `listenerset`**; `http-port` (`80`), `https-port` (`443`), `certificates` ∈ {`separate`, `reuse`} (`separate`), `reference-grant` (`false`), `reference-grant-api-version` ∈ {`gateway.networking.k8s.io/v1beta1`, `gateway.networking.k8s.io/v1`} (`…/v1beta1`) |
| `pkg/generator/gatewayapi_translate.go` | `listenersFor`, `hostSlug`, `certificatesFor`, `redirectRoute`; в `translateIngress` — режим `listenerset` (`Attach` → `{listenerSet, sectionName}`), TLS-хосты, R15, `ReferenceGrant` (R18) |
| `pkg/generator/gatewayapi_template.go` | Блоки `ListenerSet` (при непустом `$gw.parent.name`), `Certificate`, `ReferenceGrant` (условие R18 в шаблоне: `and $gw.parent.namespace (ne $gw.parent.namespace $.Release.Namespace)`) |
| тесты `gatewayapi_*_test.go` | Новые случаи (ниже) |
| `tests/golden/gatewayapi_test.go` | Подтесты `listenerset`, `route-tls`, `reference-grant` |
| `README.md`, `docs/RELEASE_NEXT.md` | Режимы `listenerset`/`route`, смена значения по умолчанию `mode`, Certificate `<secret>-gw` |

## Шаги

1. `hostSlug(host)`: нижний регистр; `*` → `wildcard`; `.` → `-`; символы вне `[a-z0-9-]` → `-`; обрезка `-` по краям. Имена слушателей — `boundedName(slug, "-http")`, `boundedName(slug, "-https")`.
2. `listenersFor` (режим `listenerset`): для каждой группы хостов с непустым хостом — `{name: <slug>-http, port: <http-port>, protocol: HTTP, hostname: <host>}`; если хост в TLS (R14) — ещё `{name: <slug>-https, port: <https-port>, protocol: HTTPS, hostname: <host>, tls: {mode: Terminate, certificateRefs: [{name: <secretRef>}]}}`. Пустой хост — слушатели без `hostname` с `slug = "any"` + `Note`. TLS-запись без `hosts` → для всех хостов + `Note`; хост TLS вне правил → `Note`. > 64 слушателей → несколько `ListenerSet` (`<ingress>`, `<ingress>-2`…), маршруты ссылаются на тот, где их слушатель.
3. `certificatesFor`: `certificates=separate` и аннотация `cert-manager.io/cluster-issuer` (kind `ClusterIssuer`) или `cert-manager.io/issuer` (kind `Issuer`) → на каждый `secretName` один `Certificate` `{name: <secret>-gw, secretName: <secret>-gw, dnsNames: <хосты записи TLS или все хосты правил>, issuerRef: {name, kind, group: cert-manager.io}}`; `secretRef = <secret>-gw`. Иначе `secretRef = <secret>`; при `reuse` и аннотации issuer → `Note`: после `disableIngress: true` ingress-shim перестанет продлевать Secret. Всегда при созданном `Certificate` → `Note`: issuer должен поддерживать solver HTTP-01 для Gateway API или DNS-01 (настройка платформы).
4. Перенаправление (R15): контроллер nginx (`IngressFacts.Controller`), хост в TLS, `ssl-redirect` ≠ `"false"` — или `force-ssl-redirect: "true"` (тогда и без TLS) → основной маршрут `Attach: [https]`, `redirectRoute(main)`: имя `boundedName(main.Name, "-redirect")`, те же `hostnames`, правила — `matches` каждого правила основного маршрута + `filters: [{type: RequestRedirect, requestRedirect: {scheme: https, statusCode: 308}}]`, без `backendRefs`, `Attach: [http]`. `force-ssl-redirect` без HTTPS-слушателя → `Note` (перенаправление на внешний TLS-терминатор). Контроллер не nginx → без перенаправления, `Attach: [http, https]` + `Note`. Режим `route`: перенаправление создаётся, но шаблон рендерит `-redirect` только при непустом `gatewayAPI.sections.http` (условие в шаблоне); при пустом `sections.https` — `Note` о HTTPS-only.
5. `ReferenceGrant` (R18): только режим `route`, `reference-grant=true`, у Ingress есть TLS → `{name: <ingress>-gateway-secrets, secrets: [<secret>...]}`; шаблон: `apiVersion` из параметра, `spec.from: [{group: gateway.networking.k8s.io, kind: Gateway, namespace: $gw.parent.namespace}]`, `spec.to` — `{group: "", kind: Secret, name}` на каждый Secret.
6. `ListenerSet`-шаблон: `spec.parentRef: {name: $gw.parent.name, namespace: $gw.parent.namespace (если не пуст)}` (поля `group`/`kind` по умолчанию в CRD — `gateway.networking.k8s.io`/`Gateway`, не рендерить); `listeners: toYaml`. Пустой `parent.name` → не рендерить + `Note` (из 05).

## Тесты

- Unit:
  - `TestHostSlug`: `shop.example.com` → `shop-example-com`; `*.example.com` → `wildcard-example-com`; `Shop.Example.com` → `shop-example-com`.
  - `TestListenersFor_TLS`: Ingress `shop` (spec §4.3) → 2 слушателя с ожидаемыми полями; `certificateRefs[0].name == shop-tls-gw` при `separate`+issuer, `shop-tls` при `reuse`.
  - `TestListenersFor_TLSWithoutHosts`, `TestListenersFor_TLSHostNotInRules`, `TestListenersFor_EmptyHost`, `TestListenersFor_Over64` (33 хоста с TLS → 66 слушателей → 2 `ListenerSet`).
  - `TestCertificatesFor`: `cluster-issuer` → kind `ClusterIssuer`; `issuer` → `Issuer`; без аннотации → нет `Certificate`.
  - `TestRedirect`: nginx + TLS → основной `Attach` только `https`, `-redirect` с 308 и без `backendRefs`; `ssl-redirect: "false"` → один маршрут `[http, https]`; `force-ssl-redirect: "true"` без TLS → `-redirect` + заметка; traefik + TLS → без `-redirect` + заметка.
  - `TestReferenceGrant`: `route` + `reference-grant=true` → один RG c двумя Secret для двух TLS-записей; `listenerset` → нет RG.
  - Инвариант: у каждого маршрута `Attach` непуст и однороден.
- Golden `TestGatewayAPIFeature` (fixture `ingress-nginx-app` из задачи 05):
  - подтест `listenerset` (по умолчанию, `parent=d8-alb/public-gw`): `ListenerSet shop` — `spec.parentRef == {name: public-gw, namespace: d8-alb}`, слушатели `shop-example-com-http` (80, HTTP, `hostname: shop.example.com`) и `shop-example-com-https` (443, HTTPS, `tls.mode: Terminate`, `certificateRefs: [{name: shop-tls-gw}]`); `HTTPRoute shop` — `parentRefs == [{group: gateway.networking.k8s.io, kind: ListenerSet, name: shop, sectionName: shop-example-com-https}]`; `HTTPRoute shop-redirect` — `parentRefs[0].sectionName == shop-example-com-http`, 2 правила с `RequestRedirect {scheme: https, statusCode: 308}`, без `backendRefs`; `Certificate shop-tls-gw` — `secretName: shop-tls-gw`, `dnsNames: [shop.example.com]`, `issuerRef: {name: letsencrypt, kind: ClusterIssuer, group: cert-manager.io}`; `ListenerSet admin` — только HTTP-слушатели `admin-example-com-http`, `api-example-com-http`; `HTTPRoute admin` привязан к `admin-example-com-http`;
  - подтест `listenerset-no-parent`: без `parent` — нет `ListenerSet`, маршруты есть (с `parentRefs` на `ListenerSet`), stderr содержит `parent Gateway is not set`;
  - подтест `route-tls`: `mode=route`, `parent=d8-alb/public-gw`, `http-section=http`, `https-section=https` → `HTTPRoute shop` `parentRefs[0].sectionName == https`, `HTTPRoute shop-redirect` `sectionName == http`; без `http-section` — `shop-redirect` не рендерится, `HTTPRoute shop` с одним `parentRef` без `sectionName`;
  - подтест `reference-grant`: `mode=route`, `parent=platform/gw`, `reference-grant=true` → `ReferenceGrant shop-gateway-secrets`, `apiVersion: gateway.networking.k8s.io/v1beta1`, `from[0].namespace == platform`, `to == [{group: "", kind: Secret, name: shop-tls}]`; с `helm template --namespace platform` — RG не рендерится;
  - все объекты проходят `checkCRSchemas`.

## Критерии приёмки

- [ ] Значение `mode` по умолчанию — `listenerset`; `dhg features` показывает новые параметры.
- [ ] Golden-подтесты выше и `TestFeaturesPassHelm` проходят.
- [ ] README описывает оба режима, требование `Gateway.spec.allowedListeners` для `ListenerSet` из namespace приложения (настройка платформы) и `Certificate <secret>-gw`.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

Собственный `Gateway` приложения (вопрос Q2 README); `BackendTLSPolicy`; `TLSRoute` для `ssl-passthrough`.
