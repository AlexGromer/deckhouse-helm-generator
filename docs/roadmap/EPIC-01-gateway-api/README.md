# EPIC-01: миграция Ingress → Gateway API (`HTTPRoute`, `GRPCRoute`, `ListenerSet`)

| Поле | Значение |
|---|---|
| Статус | draft (часть задач `ready`, см. таблицу; эпик остаётся `draft`, пока не закрыты открытые вопросы владельца) |
| Приоритет | P1 |
| Размер | XL (сумма задач: 3×S, 5×M, 3×L) |
| Зависит от | Внешнее: работа `--with istio-ingress` на `main` (общая модель Ingress, задача 03). Контракты EPIC-03 не нужны |
| Связанные ADR | ADR-046 (реестр `--with`), ADR-049 (идентичность объектов), ADR-051 (минимум зависимостей), ADR-053 (fidelity), ADR-054 (`SpecOverlay`), ADR-059 (только факты). ADR-кандидаты — в [design.md](design.md#6-adr-кандидаты) |

## Проблема

1. **ingress-nginx выведен из эксплуатации.** В README репозитория `kubernetes/ingress-nginx` (ветка `main`, проверено 2026-10-07) со ссылкой на блог SIG Network от 2025-11-11 (<https://kubernetes.io/blog/2025/11/11/ingress-nginx-retirement/>): «Best-effort maintenance will continue until March 2026. Afterward, there will be no further releases, no bugfixes, and no updates to resolve any security vulnerabilities». Там же: «If you are not already using ingress-nginx, you should not be deploying it… identify a Gateway API implementation and use it». Существующие установки продолжают работать, но без исправлений уязвимостей.
2. **dhg генерирует только `Ingress`.** `pkg/processor/k8s/ingress.go` (`IngressProcessor`) воспроизводит Ingress из входа; аннотации ingress-nginx переносятся как есть (`values.services.<svc>.ingress.annotations`). Gateway API dhg только *читает* из входа (`gateway.go`, `httproute.go`, `grpcroute.go`, `tlsroute.go`), но не умеет строить маршруты из Ingress.
3. **Отставание процессоров от Gateway API.** `TLSRouteProcessor` (`pkg/processor/k8s/tlsroute.go`) зарегистрирован только на `gateway.networking.k8s.io/v1alpha2`. В Gateway API v1.5+ `TLSRoute` — `v1` (standard), а `v1alpha2` в CRD standard-канала v1.6.3 **не обслуживается** (`served: false`). Процессоров `ListenerSet` и `ReferenceGrant` нет: они уходят в generic fallback.
4. **Deckhouse.** В документации `deckhouse/deckhouse` (ветка `main`, коммит `89db706`, 2026-10-07) описан модуль `alb` — реализация Gateway API на Envoy Proxy: администратор создаёт `ClusterALBInstance`/`ALBInstance` (управляемый `Gateway`, `GatewayClass` `d8-alb`, для кластерного шлюза — namespace `d8-alb`), приложение публикуется объектами **`ListenerSet` + `HTTPRoute` в своём namespace**; слушатели `d8-http`/`d8-https` служебные, привязывать к ним маршруты приложений документация запрещает. Там же — таблица соответствия аннотаций ingress-nginx аннотациям `alb.network.deckhouse.io/*`. С какой версии DKP доступен модуль `alb` — **не проверено**.

## Цель и результат

`dhg generate --with gateway-api` для каждого входного `Ingress` добавляет в chart эквивалентные объекты Gateway API, управляемые через values, и оставляет исходный Ingress (переключение — значением values). Всё, что не переносится 1:1, перечисляется в `Note:`.

```bash
dhg generate -f ./manifests -o ./out --chart-name shop \
  --with gateway-api --feature-opt gateway-api.parent=d8-alb/public-gw
```

Фрагмент результата (вход — `Ingress shop` с TLS и двумя путями, см. [spec.md §4](spec.md#4-выходы)):

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: ListenerSet
metadata: {name: shop, namespace: shop}
spec:
  parentRef: {name: public-gw, namespace: d8-alb}
  listeners:
    - {name: shop-example-com-http,  port: 80,  protocol: HTTP,  hostname: shop.example.com}
    - name: shop-example-com-https
      port: 443
      protocol: HTTPS
      hostname: shop.example.com
      tls: {mode: Terminate, certificateRefs: [{name: shop-tls-gw}]}
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata: {name: shop, namespace: shop}
spec:
  parentRefs:
    - {group: gateway.networking.k8s.io, kind: ListenerSet, name: shop, sectionName: shop-example-com-https}
  hostnames: [shop.example.com]
  rules:
    - matches: [{path: {type: PathPrefix, value: /api}}]
      backendRefs: [{name: orders, port: 8080}]
    - matches: [{path: {type: PathPrefix, value: /}}]
      backendRefs: [{name: web, port: 80}]
```

```
Note: gateway-api: Ingress shop/shop: nginx.ingress.kubernetes.io/proxy-read-timeout="120" has no Gateway API equivalent (TCP idle timeout ≠ HTTPRoute timeouts.request); not converted
```

## Scope

- В scope:
  - `HTTPRoute`/`GRPCRoute` из каждого `Ingress` входа; `ListenerSet` (namespaced, в namespace приложения) со слушателями HTTP/HTTPS по хостам и TLS-сертификатам Ingress;
  - перенаправление HTTP→HTTPS, rewrite, redirect, CORS и прочие аннотации ingress-nginx — по таблице в [spec.md §3.3](spec.md#33-аннотации-ingress-nginx);
  - `ReferenceGrant` в namespace приложения (разрешение чужому `Gateway` читать Secret приложения), `Certificate` cert-manager для отдельного Secret пути Gateway API;
  - профиль `implementation=alb` — аннотации `alb.network.deckhouse.io/*` модуля Deckhouse `alb`;
  - переключатели values: Ingress выключить, маршруты включить;
  - общая с `--with istio-ingress` модель «Ingress → маршруты» (одно внутреннее представление);
  - процессоры входных `TLSRoute v1`, `ListenerSet`, `ReferenceGrant`.
- Вне scope:
  - `Gateway`, `GatewayClass`, `ClusterALBInstance`/`ALBInstance` — инфраструктура платформы (в DKP управляемый `Gateway` создаёт модуль `alb`, «ручная модификация … не допускается»). Приложение только ссылается на шлюз через `parentRef`. Решение — открытый вопрос Q2 ниже;
  - `BackendTLSPolicy` (backend-protocol `HTTPS`/`GRPCS`): CA и SNI бэкенда не выводятся из Ingress — только `Note:`;
  - `TLSRoute` для `ssl-passthrough`, `TCPRoute`/`UDPRoute` — нет фактов во входе Ingress (ssl-passthrough → `Note:`);
  - experimental-канал Gateway API (`ExternalAuth`, `retry`, `sessionPersistence`): в standard v1.6.3 этих полей нет, генерировать их нельзя;
  - установка CRD Gateway API и выбор реализации.

## Задачи и порядок

| # | Задача | Размер | Зависит от | Статус |
|---|---|---|---|---|
| 01 | [Канал заметок для features (`FeatureContext.Note`)](tasks/01-feature-notes.md) | S | — | ready |
| 02 | [Общая модель «Ingress → маршруты»](tasks/02-ingress-model.md) | M | 01 | ready |
| 03 | [`--with istio-ingress` на общей модели](tasks/03-istio-ingress-on-model.md) | S | 02, слияние `istio-ingress` в `main` | draft |
| 04 | [Golden: проверка полей CR по схемам CRD](tasks/04-crd-schema-check.md) | M | — | ready |
| 05 | [`--with gateway-api`: HTTPRoute, режим `route`, переключатели values](tasks/05-httproute-core.md) | L | 01, 02, 04 | ready |
| 06 | [Режим `listenerset`: ListenerSet, TLS, HTTP→HTTPS, Certificate, ReferenceGrant](tasks/06-listenerset-tls.md) | L | 05 | ready |
| 07 | [Аннотации ingress-nginx → фильтры Gateway API](tasks/07-nginx-annotations.md) | L | 05 | ready |
| 08 | [`backend-protocol: GRPC` → `GRPCRoute`](tasks/08-grpcroute.md) | M | 05 | ready |
| 09 | [Профиль `implementation=alb` (Deckhouse)](tasks/09-alb-profile.md) | M | 07 | ready |
| 10 | [Canary-Ingress → веса и заголовки в маршруте](tasks/10-canary.md) | M | 07 | draft |
| 11 | [Процессоры `TLSRoute v1`, `ListenerSet`, `ReferenceGrant`](tasks/11-route-processors.md) | S | — | ready |

Порядок: 01 → 02 → (04, 11 параллельно) → 05 → (06, 07, 08) → 09 → 10. Задача 03 — после слияния `istio-ingress`.

## Риски и открытые вопросы

| Вопрос | Варианты | Рекомендация | Кто решает |
|---|---|---|---|
| Q1. Режим по умолчанию | (a) `listenerset` — приложение приносит свой `ListenerSet` (хосты, TLS) и маршруты, к шлюзу платформы привязывается через `parentRef`; (b) `route` — только маршруты к существующим слушателям `Gateway` | (a): это модель Deckhouse `alb` («для публикации приложений используйте ListenerSet»), сертификат остаётся в namespace приложения без `ReferenceGrant`. (b) остаётся параметром для кластеров, где слушатели заводит платформа или реализация не поддерживает `ListenerSet` (standard только с v1.5) | владелец |
| Q2. Генерировать ли собственный `Gateway` приложения | (a) нет — платформа; (b) режим `gateway` с параметром `gateway-class` | (a). `Gateway` определяет точку входа и инфраструктуру (в DKP его создаёт модуль). Нужен — отдельная задача после решения владельца | владелец |
| Q3. Что делать с Ingress после генерации | (a) `ingress=keep`: оба объекта, переключение `gatewayAPI.disableIngress`; (b) `ingress=disable` по умолчанию | (a): параллельная работа на время миграции (так же описывает миграцию DKP). Значение по умолчанию параметра — `keep` | владелец |
| Q4. Сертификаты cert-manager | (a) `certificates=separate`: отдельный `Certificate` `<secret>-gw` и Secret для пути Gateway API; (b) `reuse`: слушатель ссылается на Secret Ingress | (a): DKP предупреждает о конфликтах HTTP-01 при общих Secret у `ingress-nginx` и `alb`; при (b) и выключенном Ingress ingress-shim перестаёт продлевать сертификат | владелец |
| Q5. Таймауты `proxy-*-timeout` | (a) только `Note:`; (b) эвристика ingress2gateway: `timeouts.request = max(connect, read, send) × 10` | (a) для `standard`; в профиле `alb` — `alb.network.deckhouse.io/idle-timeout` (DKP: аналог `proxy-read-timeout`/`proxy-send-timeout`). Эвристика выдумывает значение (ADR-059) | владелец |
| Q6. `pathType: ImplementationSpecific` у ingress-nginx без `use-regex` | (a) `PathPrefix` + `Note:` о семантике; (b) пропуск пути | (a), как ingress2gateway v1.2.0 (`implementationSpecificPathMatch`: «treats ImplementationSpecific as Prefix by default»). Точная семантика nginx (строковый префикс `location /foo` совпадает с `/foobar`) — не проверено на контроллере | владелец |
| Q7. CORS | (a) фильтр `CORS` (standard с Gateway API v1.6.0); (b) только `Note:` | (a) с параметром `cors=false` для кластеров с CRD < v1.6 | владелец |
| Q8. Canary (задача 10) | (a) объединять canary-Ingress в маршрут основного; (b) только `Note:` | Решить до перевода задачи 10 в `ready`: объединение меняет владение объектом (один маршрут на два Ingress) | владелец |
| Q9. `TLSRoute v1alpha2` во входе | (a) сохранять `apiVersion` входа (fidelity) + `Note:`; (b) повышать до `v1` | (a): ADR-053 требует воспроизведения входа; `Note:` сообщает, что `v1alpha2` не обслуживается CRD ≥ v1.5 | владелец |
| R1. Реализации различаются в Extended-возможностях (`URLRewrite`, `CORS`, `RequestRedirect.scheme`, `parentRef.port`) | — | В `Note:` перечислять использованные Extended-возможности маршрута | — |
| R2. Параллельная работа над `istio-ingress` меняет те же файлы | — | Задача 02 вводит модель без изменения `istio-ingress`; перевод — задача 03 после слияния | — |
