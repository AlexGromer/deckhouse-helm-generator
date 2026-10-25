# EPIC-01: спецификация

## 1. Термины

- **Gateway API** — набор API Kubernetes для маршрутизации входящего трафика (группа `gateway.networking.k8s.io`), преемник Ingress API.
- **Канал (channel)** — набор CRD Gateway API: `standard` (стабильные ресурсы и поля) и `experimental` (дополнительно экспериментальные). Здесь используется только standard.
- **Уровень поддержки (support level)** — `Core` (обязаны поддерживать все реализации), `Extended` (поддержка необязательна, поведение переносимо), `Implementation-specific` (поведение зависит от реализации). Указан в комментариях типов `apis/v1/*.go` Gateway API.
- **`parentRef`** — ссылка маршрута (`HTTPRoute`) на родителя — `Gateway` или `ListenerSet`, к слушателю которого маршрут привязывается; `sectionName` выбирает слушатель по имени.
- **Слушатель (listener)** — порт, протокол, hostname и TLS точки входа.
- **`ListenerSet`** — namespaced-ресурс со слушателями, присоединяемый к `Gateway` через `spec.parentRef` (standard с v1.5). Слушатели и сертификаты принадлежат namespace приложения.
- **`ReferenceGrant`** — объект в namespace *цели* ссылки, разрешающий ссылку из другого namespace.
- **Реализация (implementation)** — контроллер, обслуживающий Gateway API (в Deckhouse — модуль `alb` на Envoy Proxy, `GatewayClass` `d8-alb`).
- **Фильтр (filter)** — действие правила маршрута: `RequestRedirect`, `URLRewrite`, `RequestHeaderModifier`, `CORS` и др.
- **Факт** — значение, прочитанное из входа (ADR-059). **`Note:`** — строка stderr о невыводимом.

## 2. Требования

Требования R1–R14 — ядро (задачи 05–06), R15–R18 — аннотации и gRPC (07–08), R19 — профиль `alb` (09), R20 — canary (10, draft), R21–R23 — сквозные, R24 — процессоры входа (11), R25 — общая модель (02).

- **R1.** В реестре features есть `gateway-api` (`dhg features` показывает описание и параметры из §5). Неизвестный параметр отклоняется существующей логикой `ApplyFeatures`.
- **R2.** Для каждого входного `Ingress` (`networking.k8s.io/v1`), шаблон которого есть в chart'е, feature добавляет один шаблон `templates/gateway-api-<basename шаблона Ingress>` (функция `featureTemplatePath`). Chart без Ingress не меняется (ни шаблонов, ни ключа values).
- **R3.** Правила Ingress группируются по `host`: одна группа — один `HTTPRoute` с `hostnames: [<host>]`. Правило без `host` → маршрут без `hostnames`. Порядок групп — порядок первого появления хоста в `spec.rules`.
- **R4.** Тип пути: `Prefix` → `PathPrefix`; `Exact` → `Exact`; `ImplementationSpecific` → по контроллеру Ingress (§3.2): ingress-nginx с `use-regex: "true"` → `RegularExpression`; ingress-nginx без `use-regex` → `PathPrefix` + `Note:`; другой или неизвестный контроллер → путь пропускается + `Note:`. Отсутствующий `pathType` трактуется как `Prefix` (так рендерит шаблон `IngressProcessor`).
- **R5.** Значение пути `Exact`/`PathPrefix` проверяется правилами CEL CRD `HTTPRoute` v1.6.3 (§9): начинается с `/`; не содержит `//`, `/./`, `/../`, `%2f`, `%2F`, `#`; не оканчивается на `/..`, `/.`; символы — `^(?:[-A-Za-z0-9/._~!$&'()*+,;=:@]|[%][0-9a-fA-F]{2})+$`; длина ≤ 1024. Нарушение → путь пропускается + `Note:`. Отсутствующий `path` → `/`.
- **R6.** Бэкенд `service.port.number` переносится как `backendRefs[].port`. `service.port.name` разрешается в номер по `spec.ports[].name` Service `<namespace Ingress>/<service.name>` из входа; Service нет во входе или имя порта не найдено → путь пропускается + `Note:` (CRD требует `port` для Service: «Must have port for Service reference»). Бэкенд `resource` → путь пропускается + `Note:`.
- **R7.** Пути группы с одинаковыми `backendRefs` и фильтрами объединяются в одно правило с несколькими `matches` (порядок — первое появление). Ограничения CRD: ≤ 64 `matches` в правиле, ≤ 16 правил в маршруте, ≤ 128 `matches` суммарно в маршруте, ≤ 16 `hostnames`. Превышение → маршрут делится: `<имя>`, `<имя>-part2`, `<имя>-part3`, …
- **R8.** `spec.defaultBackend` → маршрут `<ingress>-default` с одним правилом без `matches` (по умолчанию CRD — `PathPrefix /`). В режиме `route` у него нет `hostnames`; в режиме `listenerset` он привязывается к HTTPS-слушателям (или HTTP при отсутствии TLS) хостов этого Ingress. `Note:` — приоритет маршрута без hostnames относительно других маршрутов определяется правилами Gateway API (§9), а не порядком nginx.
- **R9.** Имена объектов: первый маршрут группы хостов — имя Ingress; следующие — `<ingress>-2`, `<ingress>-3` (по порядку групп); маршрут перенаправления — `<маршрут>-redirect`; `ListenerSet` — имя Ingress; `Certificate` и его Secret — `<secretName>-gw`; `ReferenceGrant` — `<ingress>-gateway-secrets`. `metadata.namespace: {{ $.Release.Namespace }}`; `metadata.labels` — копия блока `metadata.labels` шаблона Ingress (`resourceTemplate.labels()`, как в `istioTemplateBody`), поэтому во всех режимах используются правильные helpers (`<chart>.labels` или `library.labels`).
- **R10.** Значения хранятся в values под ключом `gatewayAPI` (§4.2). Шаблон рендерит объекты, только если `.Values.gatewayAPI.enabled` и `enabled` сервиса Ingress (`$svc.enabled`). Состояние `ingress.enabled` сервиса на маршруты не влияет.
- **R11.** Шаблон Ingress (из `IngressProcessor`) получает дополнительное условие: при `.Values.gatewayAPI.disableIngress: true` Ingress не рендерится. Значение по умолчанию — из параметра `ingress` (`keep` → `false`, `disable` → `true`).
- **R12.** Режим `route` (`mode=route`): `parentRefs` каждого маршрута строятся при рендере из `gatewayAPI.parent` (`name`, `namespace`) и `gatewayAPI.sections` (`http`, `https`): `{group: gateway.networking.k8s.io, kind: Gateway, name, namespace?, sectionName?}`. Пустой `parent.name` → `parentRefs` не рендерится (маршрут не привязан) + `Note:` при генерации.
- **R13.** Режим `listenerset` (по умолчанию): для каждого Ingress — `ListenerSet` со слушателями: для каждого хоста правил — HTTP (`<slug>-http`, порт `http-port`), для хоста с TLS — ещё HTTPS (`<slug>-https`, порт `https-port`, `tls.mode: Terminate`, `certificateRefs: [{name: <secret>}]`). `<slug>` — хост в нижнем регистре, `*` → `wildcard`, `.` → `-`. Хост без `host` (пустой) → слушатель без `hostname` + `Note:` (может конфликтовать с другими `ListenerSet` того же `Gateway`). `spec.parentRef` — из `gatewayAPI.parent`; при пустом `parent.name` `ListenerSet` не рендерится + `Note:`. Маршруты ссылаются на `ListenerSet` (`kind: ListenerSet`, `sectionName` — нужный слушатель). Больше 64 слушателей → `ListenerSet` делится (`<имя>-2`, …).
- **R14.** TLS: запись `spec.tls[]` с `hosts` даёт HTTPS для этих хостов; запись без `hosts` — для всех хостов правил Ingress (+ `Note:`). Хост TLS, которого нет в правилах, → `Note:`, слушатель не создаётся. Если у Ingress есть аннотация `cert-manager.io/cluster-issuer` или `cert-manager.io/issuer` и `certificates=separate`, для каждого Secret создаётся `Certificate` (`cert-manager.io/v1`) `<secret>-gw`: `secretName: <secret>-gw`, `dnsNames` — хосты записи TLS, `issuerRef: {name, kind: ClusterIssuer|Issuer, group: cert-manager.io}`; слушатели ссылаются на `<secret>-gw`. Иначе слушатели ссылаются на исходный Secret.
- **R15.** HTTP→HTTPS: для хоста с TLS у Ingress контроллера ingress-nginx, если аннотация `nginx.ingress.kubernetes.io/ssl-redirect` не равна `"false"` или `force-ssl-redirect: "true"`, основной маршрут привязывается только к HTTPS-слушателю, а маршрут `<маршрут>-redirect` — к HTTP-слушателю, с теми же `matches` и фильтром `RequestRedirect {scheme: https, statusCode: 308}` без `backendRefs`. При `ssl-redirect: "false"` основной маршрут привязывается к обоим слушателям. Для другого контроллера перенаправление не создаётся, основной маршрут привязывается к обоим слушателям + `Note:`. В режиме `route` маршрут перенаправления рендерится только при непустом `gatewayAPI.sections.http`; при пустом `sections.https` основной маршрут привязывается ко всему `Gateway` + `Note:` (HTTPS-only не воспроизводится).
- **R16.** Аннотации ingress-nginx переносятся по таблице §3.3; каждая аннотация `nginx.ingress.kubernetes.io/*`, которая не перенесена, даёт ровно одну `Note:` на Ingress.
- **R17.** `nginx.ingress.kubernetes.io/backend-protocol: GRPC` или `GRPCS` → пути Ingress переносятся в `GRPCRoute` (§3.4) вместо `HTTPRoute`; `GRPCS` и `HTTPS` дополнительно дают `Note:` о `BackendTLSPolicy`.
- **R18.** `ReferenceGrant` (параметр `reference-grant=true`, только режим `route`): рендерится, если `gatewayAPI.parent.namespace` не пуст и не равен `.Release.Namespace`; `from: [{group: gateway.networking.k8s.io, kind: Gateway, namespace: <parent.namespace>}]`, `to: [{group: "", kind: Secret, name: <secret>}]` — по одному `to` на Secret TLS этого Ingress. `apiVersion` — параметр `reference-grant-api-version` (по умолчанию `gateway.networking.k8s.io/v1beta1`).
- **R19.** `implementation=alb`: аннотации без стандартного эквивалента переносятся в `metadata.annotations` маршрутов как `alb.network.deckhouse.io/*` по таблице §3.3 (столбец «alb»); `Note:` для них не выводится.
- **R20.** (draft, задача 10) Canary-Ingress (`nginx.ingress.kubernetes.io/canary: "true"`) объединяется с основным Ingress того же хоста и пути: `canary-weight` → `backendRefs[].weight`, `canary-by-header` → отдельное правило с `headers`.
- **R21.** Ни одно поле, которого нет в схеме CRD standard-канала Gateway API v1.6.3 (или `cert-manager.io/v1` `Certificate`), не рендерится (проверяется задачей 04).
- **R22.** Feature комбинируется со всеми остальными (`--with <все>`) во всех режимах (`universal`, `separate`, `library`, `umbrella`) и с `--detect-ingress`, `ingress-tls`: `helm lint --strict` и `helm template` проходят.
- **R23.** Новых зависимостей в `go.mod` нет: объекты строятся как `map[string]interface{}` и сериализуются `sigs.k8s.io/yaml`.
- **R24.** Процессоры входа: `TLSRoute` `gateway.networking.k8s.io/v1` (в дополнение к `v1alpha2`), `ListenerSet` `v1`, `ReferenceGrant` `v1` и `v1beta1` — с сохранением всего `spec` (`processor.SpecOverlay`, ADR-054).
- **R25.** Модель Ingress (задача 02) — единственный разбор Ingress для `gateway-api` и `istio-ingress`: хосты, пути, типы путей, бэкенды с разрешённым портом, TLS, аннотации, контроллер.

## 3. Входы

### 3.1 Поля Ingress

| Источник | Факт | Правило |
|---|---|---|
| `metadata.name`, `metadata.namespace` | имя, namespace | имена объектов (R9); разрешение Service (R6) |
| `spec.ingressClassName`, аннотация `kubernetes.io/ingress.class` | класс | контроллер (§3.2) |
| `spec.rules[].host` | хост | `hostnames`, слушатели (R3, R13) |
| `spec.rules[].http.paths[].path`, `.pathType` | путь, тип | `matches[].path` (R4, R5) |
| `…backend.service.name`, `.port.number`, `.port.name` | бэкенд | `backendRefs` (R6) |
| `…backend.resource` | не-Service бэкенд | пропуск + `Note:` |
| `spec.defaultBackend` | бэкенд по умолчанию | R8 |
| `spec.tls[].hosts`, `.secretName` | TLS | R13, R14 |
| `metadata.annotations` `cert-manager.io/cluster-issuer`, `cert-manager.io/issuer` | issuer | `Certificate` (R14) |
| `metadata.annotations` `nginx.ingress.kubernetes.io/*` | поведение ingress-nginx | §3.3 |
| Service из входа: `spec.ports[].name`, `.port` | номер порта по имени | R6 |
| `IngressClass` из входа: `spec.controller` | контроллер | §3.2 |

### 3.2 Определение контроллера Ingress

Контроллер определяется **на каждый Ingress** (а не на весь вход, как `DetectIngressController`): (1) `IngressClass` с именем `spec.ingressClassName` во входе — по `spec.controller` (`k8s.io/ingress-nginx` → nginx); (2) имя класса (`ingressClassName` или аннотация `kubernetes.io/ingress.class`) содержит `nginx` → nginx; (3) есть аннотация с префиксом `nginx.ingress.kubernetes.io/` → nginx; (4) иначе — `unknown`. В DKP класс по умолчанию у `IngressNginxController` — `nginx` (документация `MIGRATION_RU.md`, §9). Для `unknown` аннотации ingress-nginx не интерпретируются, перенаправление не создаётся (R15), `ImplementationSpecific` пропускается (R4).

### 3.3 Аннотации ingress-nginx

Префикс `nginx.ingress.kubernetes.io/` опущен. «Std» — результат в режиме `implementation=standard`, «alb» — дополнительно в режиме `implementation=alb` (аннотация маршрута `alb.network.deckhouse.io/<ключ>`). Уровень — уровень поддержки Gateway API.

| Аннотация | Std (Gateway API) | Уровень | alb | Задача |
|---|---|---|---|---|
| `ssl-redirect`, `force-ssl-redirect` | R15: маршрут `-redirect`, `RequestRedirect {scheme: https, statusCode: 308}` | `RequestRedirect` Core; `scheme` и код 308 — Extended | — | 06 |
| `permanent-redirect` (+ `permanent-redirect-code`) | правило без `backendRefs`: `RequestRedirect` с `scheme`, `hostname`, `port`, `path {type: ReplaceFullPath}` из URL аннотации; код по умолчанию 301, допустимы 301/302/303/307/308, иное → 301 + `Note:` | Core (301/302), Extended (прочее) | — | 07 |
| `temporal-redirect` (+ `temporal-redirect-code`) | то же, код по умолчанию 302; приоритет над `permanent-redirect` | как выше | — | 07 |
| `app-root` | правило `Exact /` с `RequestRedirect {path: {type: ReplaceFullPath, replaceFullPath: <app-root>}, statusCode: 302}` первым в маршруте | Core/Extended | — | 07 |
| `rewrite-target` без `$N`, `use-regex` не `"true"` | `URLRewrite {path: {type: ReplaceFullPath, replaceFullPath: <значение>}}` | Extended | — | 07 |
| `rewrite-target: /$2` при пути вида `<p>(/|$)(.*)` (`<p>` без символов regex) | `PathPrefix <p>` + `URLRewrite {path: {type: ReplacePrefixMatch, replacePrefixMatch: /}}` (эквивалент примера ingress-nginx `docs/examples/rewrite`) | Extended | — | 07 |
| `rewrite-target` с иными `$N` | Std: правило без rewrite + `Note:` | — | `rewrite-target` (значение с заменой `$N` → `\N`), путь `RegularExpression` | 07, 09 |
| `use-regex: "true"` | `RegularExpression` для всех путей Ingress (R4) + `Note:`: регистронезависимость `~*` ingress-nginx не переносится | Implementation-specific | — | 07 |
| `upstream-vhost` | `URLRewrite {hostname: <значение>}` | Extended | — | 07 |
| `x-forwarded-prefix` (вместе с `rewrite-target`) | `RequestHeaderModifier {set: [{name: X-Forwarded-Prefix, value}]}` | Core | — | 07 |
| `enable-cors: "true"` + `cors-allow-origin`, `-methods`, `-headers`, `-expose-headers`, `-credentials`, `-max-age` | фильтр `CORS {allowOrigins, allowMethods, allowHeaders, exposeHeaders, allowCredentials, maxAge}`; списки — разбиение по `,` с trim; отсутствующие аннотации — значения по умолчанию ingress-nginx (§9): origins `*`, methods `GET, PUT, POST, DELETE, PATCH, OPTIONS`, headers `DNT,Keep-Alive,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Range,Authorization`, credentials `true`, maxAge `1728000`. Параметр `cors=false` → `Note:` | Extended (standard с v1.6.0) | — | 07 |
| `backend-protocol: GRPC` / `GRPCS` | `GRPCRoute` (§3.4); `GRPCS` + `Note:` (BackendTLSPolicy) | Core | — | 08 |
| `backend-protocol: HTTPS` | `HTTPRoute` + `Note:` (BackendTLSPolicy не выводится) | — | `backend-tls-settings` не генерируется (CA/SNI неизвестны) | 07 |
| `backend-protocol: AUTO_HTTP`, `FCGI` | `Note:` | — | — | 07 |
| `proxy-connect-timeout`, `proxy-read-timeout`, `proxy-send-timeout` | `Note:` (Q5) | — | `idle-timeout: <max(read, send)>` (секунды) | 07, 09 |
| `whitelist-source-range` | `Note:` | — | `whitelist-source-range` (значение как есть) | 07, 09 |
| `limit-rps` | `Note:` | — | `limit-rps` | 07, 09 |
| `proxy-body-size` | `Note:` | — | `buffer-max-request-bytes` = размер в байтах (`k`/`K`×1024, `m`/`M`×1024², `g`/`G`×1024³; `0` → не переносится) | 07, 09 |
| `proxy-buffer-size` | `Note:` | — | `proxy-buffer-size` (значение как есть) | 07, 09 |
| `affinity: cookie` (+ `session-cookie-*`) | `Note:` (`sessionPersistence` — experimental) | — | `session-affinity: {"mode":"cookie", ...}` из `session-cookie-name`→`cookieName`, `session-cookie-path`→`path` | 07, 09 |
| `auth-url`, `auth-signin` | `Note:` (`ExternalAuth` — experimental) | — | `auth-url`, `auth-signin` | 07, 09 |
| `auth-response-headers` | `Note:` | — | `auth-response-headers` | 07, 09 |
| `auth-type: basic` + `auth-secret` | `Note:` | — | `basic-auth-secret: <ns>/<secret>` (namespace Secret — namespace Ingress, если в `auth-secret` его нет) | 07, 09 |
| `service-upstream: "true"` | `Note:` | — | `service-upstream: "true"` | 07, 09 |
| `canary*` | `Note:` (до задачи 10) | — | — | 07, 10 |
| `ssl-passthrough` | `Note:` (TLSRoute не генерируется) | — | — | 07 |
| `configuration-snippet`, `server-snippet`, `custom-headers`, `proxy-redirect-*` и прочие | `Note:` «no Gateway API equivalent» | — | — | 07 |

Аннотации `cert-manager.io/*` — R14; `kubernetes.io/ingress.class` — §3.2; прочие (не `nginx.ingress.kubernetes.io/`) не интерпретируются и не переносятся на маршруты.

### 3.4 Пути gRPC

Для Ingress с `backend-protocol: GRPC|GRPCS` каждый путь переводится в `GRPCRoute.rules[].matches[]`:

| Путь Ingress | `GRPCRoute` match |
|---|---|
| `/` (`Prefix`) или отсутствует | правило без `matches` (все методы) |
| `/<service>` или `/<service>/` (`Prefix`), `<service>` соответствует `^(?i)\.?[a-z_][a-z_0-9]*(\.[a-z_][a-z_0-9]*)*$` | `{method: {type: Exact, service: <service>}}` |
| `/<service>/<Method>` (`Exact`), `<Method>` соответствует `^[A-Za-z_][A-Za-z_0-9]*$` | `{method: {type: Exact, service, method}}` |
| иное | путь пропускается + `Note:` |

## 4. Выходы

### 4.1 Объекты

| Объект | apiVersion | Когда |
|---|---|---|
| `HTTPRoute` | `gateway.networking.k8s.io/v1` | на группу хостов Ingress (R3), `-redirect` (R15), `-default` (R8), `-partN` (R7) |
| `GRPCRoute` | `gateway.networking.k8s.io/v1` | R17 |
| `ListenerSet` | `gateway.networking.k8s.io/v1` | режим `listenerset`, непустой `parent.name` (R13) |
| `Certificate` | `cert-manager.io/v1` | R14 |
| `ReferenceGrant` | параметр, по умолчанию `gateway.networking.k8s.io/v1beta1` | R18 |

### 4.2 Values

```yaml
gatewayAPI:
  enabled: true                 # bool; false — объекты Gateway API не рендерятся
  disableIngress: false         # bool; true — Ingress chart'а не рендерятся (R11); из параметра ingress
  parent:                       # родительский Gateway (не выводится из входа)
    name: ""                    # string; из параметра parent
    namespace: ""               # string; из параметра parent
  sections:                     # только режим route: имена слушателей родительского Gateway
    http: ""
    https: ""
  ingresses:                    # map: имя Ingress → результат перевода (заполняется при генерации)
    shop:
      listenerSet:              # режим listenerset; отсутствует в режиме route
        name: shop
        listeners: []           # list: слушатели Gateway API (поля name/port/protocol/hostname/tls)
      certificates: []          # list: {name, secretName, dnsNames, issuerRef}
      referenceGrants: []       # list: {name, secrets: []}
      httpRoutes:               # list
        - name: shop
          annotations: {}       # map: только implementation=alb
          attach:               # list: слушатели, к которым привязан маршрут (по одному parentRef)
            - listenerSet: shop                    # режим listenerset: имя ListenerSet …
              sectionName: shop-example-com-https  # … и имя слушателя
            # режим route вместо этого: - listener: https | http | ""  (ключ в gatewayAPI.sections)
          hostnames: [shop.example.com]
          rules: []             # list: правила HTTPRoute в схеме Gateway API, рендерятся toYaml
      grpcRoutes: []            # та же структура, rules в схеме GRPCRoute
```

Правила рендера `parentRefs` — по одному элементу на запись `attach`: запись с `listenerSet` → `{group: gateway.networking.k8s.io, kind: ListenerSet, name: <listenerSet>, sectionName: <sectionName>}`; запись с `listener` при непустом `gatewayAPI.parent.name` → `{group: gateway.networking.k8s.io, kind: Gateway, name: <parent.name>, namespace: <parent.namespace, если не пуст>, sectionName: <sections[listener], если не пуст>}`; запись с `listener` при пустом `parent.name` не даёт элемента. Ни одного элемента → ключ `parentRefs` не рендерится. Gateway API требует различимых `parentRefs` (документация `CommonRouteSpec.ParentRefs`: если один элемент задаёт `sectionName`, все элементы с тем же родителем тоже должны его задавать). Поэтому в режиме `route` шаблон рендерит элементы так: оба `sections` непусты → по элементу на запись; хотя бы одно `sections[listener]` пусто → один элемент без `sectionName` (маршрут привязывается ко всему `Gateway`, `Note:` при генерации, R15).

### 4.3 Пример

Вход (`tests/integration/fixtures/ingress-nginx-app/`, задача 05/06):

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: shop
  namespace: shop
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt
    nginx.ingress.kubernetes.io/proxy-read-timeout: "120"
spec:
  ingressClassName: nginx
  tls:
    - hosts: [shop.example.com]
      secretName: shop-tls
  rules:
    - host: shop.example.com
      http:
        paths:
          - {path: /api, pathType: Prefix, backend: {service: {name: orders, port: {name: http}}}}
          - {path: /,    pathType: Prefix, backend: {service: {name: web,    port: {number: 80}}}}
---
apiVersion: v1
kind: Service
metadata: {name: orders, namespace: shop}
spec:
  selector: {app: orders}
  ports: [{name: http, port: 8080, targetPort: 8080}]
```

Выход `helm template` (параметр `parent=d8-alb/public-gw`, остальное по умолчанию), кроме `ListenerSet` и `HTTPRoute shop` из [README](README.md#цель-и-результат):

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata: {name: shop-redirect, namespace: <release ns>}
spec:
  parentRefs:
    - {group: gateway.networking.k8s.io, kind: ListenerSet, name: shop, sectionName: shop-example-com-http}
  hostnames: [shop.example.com]
  rules:
    - matches: [{path: {type: PathPrefix, value: /api}}]
      filters: [{type: RequestRedirect, requestRedirect: {scheme: https, statusCode: 308}}]
    - matches: [{path: {type: PathPrefix, value: /}}]
      filters: [{type: RequestRedirect, requestRedirect: {scheme: https, statusCode: 308}}]
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata: {name: shop-tls-gw}
spec:
  secretName: shop-tls-gw
  dnsNames: [shop.example.com]
  issuerRef: {name: letsencrypt, kind: ClusterIssuer, group: cert-manager.io}
```

### 4.4 Записи отчёта

Формат `Note:`: `gateway-api: Ingress <namespace>/<name>: <факт>; <что сделано>`. Перечень: пропущенный путь (R4–R6) с причиной; неперенесённая аннотация (R16); пустой `parent` (R12, R13); TLS без хостов или с хостом вне правил (R14); перенаправление не создано для не-nginx контроллера (R15); `defaultBackend` (R8); деление маршрута (R7); Extended-возможности, использованные маршрутом (`URLRewrite`, `CORS`, `RequestRedirect.scheme`, код ≠ 301/302) — одной строкой на Ingress.

## 5. Интерфейс

`dhg generate --with gateway-api [--feature-opt gateway-api.<ключ>=<значение>]`; в `.dhg.yaml` — `with: [gateway-api]`, `feature-opt: [...]` (существующий механизм).

| Параметр | По умолчанию | Значения | Описание |
|---|---|---|---|
| `mode` | `listenerset` | `listenerset`, `route` | R12, R13 (Q1 README) |
| `parent` | `""` | `<namespace>/<name>` или `<name>` | `gatewayAPI.parent` |
| `http-section`, `https-section` | `""` | имя слушателя | `gatewayAPI.sections` (режим `route`) |
| `http-port`, `https-port` | `80`, `443` | 1–65535 | порты слушателей `ListenerSet` |
| `ingress` | `keep` | `keep`, `disable` | `gatewayAPI.disableIngress` (R11) |
| `certificates` | `separate` | `separate`, `reuse` | R14 |
| `reference-grant` | `false` | bool | R18 |
| `reference-grant-api-version` | `gateway.networking.k8s.io/v1beta1` | `gateway.networking.k8s.io/v1`, `…/v1beta1` | R18 |
| `cors` | `true` | bool | §3.3 |
| `implementation` | `standard` | `standard`, `alb` | R19 |

Ошибки параметров (неверное значение перечисления, порт вне диапазона, `parent` с более чем одним `/`) — ошибка `dhg generate` с текстом `gateway-api: <param> …`. Совместимость: `--detect-ingress` (аннотации в шаблон Ingress) и `ingress-tls` (TLS в шаблон Ingress) работают с шаблоном Ingress; `gateway-api` читает факты из графа (входа), а не из этих добавлений — `Note:` не нужна, поведение описано в README проекта.

## 6. Невыводимое

| Что | Почему | Как показано |
|---|---|---|
| Родительский `Gateway` (имя, namespace), имена его слушателей | инфраструктура платформы, во входе Ingress его нет | пустые `gatewayAPI.parent`, `gatewayAPI.sections` + `Note:` |
| Поддержка Extended-возможностей реализацией | зависит от реализации | `Note:` со списком использованных возможностей |
| Таймауты, лимиты, auth, IP-фильтры, session affinity (standard) | нет полей в standard-канале | `Note:` по аннотации |
| CA/SNI бэкенда для `backend-protocol: HTTPS/GRPCS` | не во входе | `Note:` |
| Настройка solver'а HTTP-01 для Gateway API у issuer'а | объект платформы (ClusterIssuer) | `Note:` при генерации `Certificate` |

## 7. Ошибки и граничные случаи

- Ingress без `spec.rules` и без `defaultBackend` → маршрутов нет, `Note:`.
- Один и тот же хост в двух Ingress → каждый Ingress даёт свой маршрут; в режиме `listenerset` — два `ListenerSet` с одинаковым hostname на одном `Gateway`: `Note:` (DKP `alb` выявляет конфликты одинаковых слушателей).
- Wildcard-хост: Ingress `*.example.com` совпадает с одной меткой; в Gateway API `*.example.com` — суффикс (совпадает и с `a.b.example.com`) → `Note:` о расширении.
- `use-regex` или `rewrite-target` на любом Ingress хоста делает в ingress-nginx все пути хоста регистронезависимыми регулярными выражениями (`ingress-path-matching.md`) → `Note:` для Ingress этого хоста.
- Имя объекта с суффиксом (`-redirect`, `-default`, `-partN`, `-gw`) длиннее 253 символов → новая функция `boundedName(name, suffix string) string` (задача 05): обрезает `name` так, чтобы `name + "-" + 8 hex FNV-32a(name) + suffix` уложилось в 253, и `Note:`. Существующий `suffixedName` (`pkg/generator/workloadpods.go`) только дописывает суффикс и длину не ограничивает. Имена слушателей (`SectionName`, шаблон `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`, ≤ 253) — тем же правилом.
- Повторный запуск feature на chart'е, где шаблон `gateway-api-*` уже есть → ошибка `template … already exists` (существующий `addTemplate`).
- Шаблон Ingress не распознан (`parseResourceTemplate` вернул false, например переписан `--template-dir`) → Ingress пропускается + `Note:`.

## 8. Критерии приёмки эпика

- **AC1** (R1, R22): `dhg features` перечисляет `gateway-api`; `TestFeaturesPassHelm` проходит (feature на 7 входах и `--with <все>` во всех режимах).
- **AC2** (R2–R7, R10, R13, R14, R21): golden `TestGatewayAPIFeature` на `fixtures/ingress-nginx-app` с `parent=d8-alb/public-gw`: рендер содержит `ListenerSet shop` с двумя слушателями (HTTP 80, HTTPS 443 с `certificateRefs[0].name == shop-tls-gw`), `HTTPRoute shop` с `parentRefs[0].sectionName == shop-example-com-https`, двумя правилами, `backendRefs` `orders:8080` и `web:80`; все поля проходят проверку схем CRD (задача 04).
- **AC3** (R15): там же `HTTPRoute shop-redirect` с `RequestRedirect {scheme: https, statusCode: 308}` и без `backendRefs`.
- **AC4** (R11): `helm template --set gatewayAPI.disableIngress=true` не содержит `kind: Ingress`; без `--set` — содержит.
- **AC5** (R12): `--feature-opt gateway-api.mode=route` без `parent` → у `HTTPRoute` нет `parentRefs`, `ListenerSet` нет, stderr содержит `Note: gateway-api: … parent`.
- **AC6** (R16, R17): Ingress fixture'а с `rewrite-target`, `enable-cors`, `backend-protocol: GRPC` дают `URLRewrite`, `CORS`, `GRPCRoute`; неперенесённые аннотации дают по одной `Note:`.
- **AC7** (R19): `implementation=alb` переносит `whitelist-source-range` в аннотацию `alb.network.deckhouse.io/whitelist-source-range` маршрута.
- **AC8** (R24): вход `examples/12-gateway-api` и fixture с `TLSRoute v1`, `ListenerSet`, `ReferenceGrant` проходят fidelity (`render ⊇ input`).
- **AC9** (R23): `go.mod` без новых `require`.

## 9. Проверенные факты и источники

Дата проверки всех строк — 2026-10-07.

| Факт | Источник | Статус |
|---|---|---|
| Retirement ingress-nginx: best-effort до марта 2026, далее без релизов и исправлений уязвимостей; рекомендация — Gateway API | README `kubernetes/ingress-nginx` (`main`), цитирует <https://kubernetes.io/blog/2025/11/11/ingress-nginx-retirement/>; сам блог недоступен из среды (egress заблокирован) | проверено (по README) |
| Последний релиз Gateway API — v1.6.3 (тег, коммит `c083613`, 2026-10-06) | `git ls-remote https://github.com/kubernetes-sigs/gateway-api` | проверено |
| Standard-канал v1.6.3: `HTTPRoute` v1 (+v1beta1 served), `GRPCRoute` v1, `TLSRoute` v1 (v1alpha2, v1alpha3 — `served: false`), `ListenerSet` v1, `ReferenceGrant` v1 (served) и v1beta1 (storage), `TCPRoute`/`UDPRoute` v1 | `config/crd/standard/*.yaml` тега v1.6.3 | проверено |
| v1.5: TLSRoute v1, ListenerSet в standard, ReferenceGrant → v1; v1.6: CORS в standard, TCPRoute/UDPRoute GA | `CHANGELOG/1.5-CHANGELOG.md`, `CHANGELOG/1.6-CHANGELOG.md` | проверено |
| Поля правила `HTTPRoute` standard: `matches`, `filters`, `backendRefs`, `timeouts{request, backendRequest}`, `name`; `retry`, `sessionPersistence`, фильтр `ExternalAuth` — только experimental | схема `gateway.networking.k8s.io_httproutes.yaml` standard/experimental v1.6.3 | проверено |
| Типы фильтров standard: `RequestHeaderModifier`, `ResponseHeaderModifier`, `RequestMirror`, `RequestRedirect`, `URLRewrite`, `ExtensionRef`, `CORS`; коды `RequestRedirect` 301, 302, 303, 307, 308; `scheme` — `http`/`https` | там же | проверено |
| Ограничения: 16 правил, 64 `matches` на правило, 128 суммарно, 16 `hostnames`, 64 слушателя, длина пути 1024; CEL: «Must have port for Service reference», «RequestRedirect filter must not be used together with backendRefs», `ReplacePrefixMatch` требует ровно один `PathPrefix` match | там же | проверено |
| Уровни поддержки: `Exact`/`PathPrefix` Core, `RegularExpression` Implementation-specific; `URLRewrite`, `CORS`, `RequestMirror` Extended; `RequestRedirect` Core, его `scheme`/`port`/`path` Extended, коды 301/302 Core | `apis/v1/httproute_types.go` v1.6.3 | проверено |
| `PathPrefix` — поэлементное сравнение, конечный `/` игнорируется; «semantically equivalent to the "Prefix" path type in the Kubernetes Ingress API» | там же | проверено |
| Wildcard-hostname в HTTPRoute — суффиксное совпадение (`*.example.com` совпадает с `foo.test.example.com`); в Ingress wildcard совпадает ровно с одной меткой | `httproute_types.go` v1.6.3; `k8s.io/api/networking/v1/types.go` Kubernetes v1.36.0 | проверено |
| Приоритет совпадений: Exact → самый длинный Prefix → method → headers → query; затем старейший маршрут, затем по алфавиту `ns/name` | `httproute_types.go` v1.6.3 | проверено |
| `ListenerSet` может ссылаться на Secret своего namespace без `ReferenceGrant`; по умолчанию `Gateway.spec.allowedListeners.namespaces.from: None` | `geps/gep-1713/index.md`, схема `gateways` v1.6.3 | проверено |
| ingress2gateway: последний релиз v1.2.0 (2026-07-07), Apache-2.0; ingress-nginx провайдер переносит canary, rewrite-target (без `$N`), app-root, redirects, ssl-redirect (308, отдельный HTTP-маршрут), upstream-vhost, timeouts (×10 → `timeouts.request`), CORS, backend-protocol GRPC→GRPCRoute, use-regex, ssl-passthrough→TLSRoute; `go.mod` тянет client-go, controller-runtime, helm v4, envoy gateway, kgateway, kong, istio api | `git ls-remote`, `CHANGELOG.md`, `pkg/i2gw/providers/ingressnginx/README.md`, `go.mod` | проверено |
| Аннотации ingress-nginx и значения по умолчанию CORS; ssl-redirect 308 по умолчанию при TLS; `backend-protocol` HTTP/HTTPS/AUTO_HTTP/GRPC/GRPCS/FCGI; пример rewrite `/something(/|$)(.*)` → `/$2` | `docs/user-guide/nginx-configuration/annotations.md`, `docs/examples/rewrite/README.md` (`kubernetes/ingress-nginx`, `main`) | проверено |
| `use-regex`/`rewrite-target` на любом Ingress хоста включают регистронезависимые regex-локации для всех путей хоста | `docs/user-guide/ingress-path-matching.md` (`main`) | проверено |
| Семантика `ImplementationSpecific` в ingress-nginx без `use-regex` (строковый префикс nginx) | — | не проверено |
| Deckhouse `alb`: `ClusterALBInstance`/`ALBInstance` создают управляемый `Gateway`; кластерный — обычно в `d8-alb`; `GatewayClass` `d8-alb`; приложение — `ListenerSet` + `HTTPRoute`; слушатели `d8-http`/`d8-https` служебные; список аннотаций `alb.network.deckhouse.io/*`; совместимые версии API: ReferenceGrant v1beta1, ListenerSet/HTTPRoute/GRPCRoute/TLSRoute v1 | `docs/documentation/pages/{admin/configuration,user}/network/ingress/alb/*_RU.md`, `docs/documentation/pages/architecture/network/ALB_RU.md` (`deckhouse/deckhouse`, `main`, `89db706`) | проверено |
| Версия DKP, с которой доступен модуль `alb` | — | не проверено |
| Модуль Deckhouse `istio` поддерживает Gateway API (istiod 1.27, 1.29) | `CHANGELOG/CHANGELOG-v1.77.1.yml` (`deckhouse/deckhouse`) | проверено |
| `TLSRouteProcessor` dhg зарегистрирован только на `v1alpha2` | `pkg/processor/k8s/tlsroute.go` | проверено |
