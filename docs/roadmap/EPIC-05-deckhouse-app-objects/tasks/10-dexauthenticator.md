# EPIC-05 / 10: DexAuthenticator — `v2alpha1`, связь с Ingress, проверки

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | M (2–3 дня) |
| Зависит от | EPIC-05/02 |
| Требования | R13, R14, R15 (из [spec.md](../spec.md)) |

## Контекст

- Процессор `DexAuthenticatorProcessor` (`pkg/processor/k8s/dexauthenticator.go:24`) поддерживает только `deckhouse.io/v1`; шаблон жёстко пишет `apiVersion: deckhouse.io/v1`. CRD обслуживает `v1` (storage), `v2alpha1` (served) и устаревший `v1alpha1` (F11). Объект `v2alpha1` (`spec.applications[]`) уходит в generic fallback.
- Детектор `AnnotationDetector` (`pkg/analyzer/detector/annotation.go:173–191`) связывает объект с DexAuthenticator только по аннотациям `deckhouse.io/*`, содержащим `auth`/`dex`. DKP документирует подключение приложения аннотациями `nginx.ingress.kubernetes.io/auth-url|auth-signin|auth-response-headers` на Ingress или `alb.network.deckhouse.io/auth-*` на HTTPRoute (F12). Реальная связь не находится, и dhg не может сказать, что Ingress защищён или что DexAuthenticator «висит» без приложения.
- DexAuthenticator работает только по HTTPS, Secret сертификата должен быть в том же namespace (F11).

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/processor/k8s/dexauthenticator.go` | `Supports`: `deckhouse.io/v1` и `deckhouse.io/v2alpha1`; шаблон пишет `apiVersion` входа (`obj.GetAPIVersion()`); ключи удобства: для `v1` — как сейчас, для `v2alpha1` — `applications`, `sendAuthorizationHeader`, `allowedGroups`; ссылки `Dependencies` на Secret'ы TLS (`applicationIngressCertificateSecretName`, `additionalApplications[].ingressSecretName`, `applications[].ingressSecretName`) того же namespace |
| `pkg/processor/k8s/dexauthenticator_test.go` | Тесты `v2alpha1` и зависимостей |
| `pkg/analyzer/detector/annotation.go` | `dexAuthenticatorFromAuthURL(raw string) (name, ns string, ok bool)`; связь от Ingress (`nginx.ingress.kubernetes.io/auth-url`) и HTTPRoute (`alb.network.deckhouse.io/auth-url`) к DexAuthenticator |
| `pkg/analyzer/detector/annotation_test.go` | Тесты |
| `pkg/generator/features_deckhouse.go` | Раздел `## DexAuthenticator` отчёта (R15) |
| `pkg/generator/features_deckhouse_test.go` | Тесты раздела |
| `tests/integration/fixtures/deckhouse-observability/{ingress,dexauthenticator,dexauthenticator-admin}.yaml` (новые) | Объекты фикстуры |
| `tests/golden/deckhouse_test.go` | `TestDexAuthenticator` |
| `tests/golden/integrity_test.go` | `references`: `case "DexAuthenticator"` → `Secret/<applicationIngressCertificateSecretName>`, `Secret/<additionalApplications[].ingressSecretName>`, `Secret/<applications[].ingressSecretName>` (проверяются, только если Secret был во входе — существующая логика `checkIntegrity`) |
| `docs/RELEASE_NEXT.md` | `v2alpha1` обрабатывается процессором (design §7) |

## Шаги

1. Процессор: `NewBaseProcessor("dexauthenticator", 50, gvkV1, gvkV2alpha1)` (`BaseProcessor` принимает список GVK — проверить сигнатуру `processor.NewBaseProcessor` в `pkg/processor/processor.go`; при необходимости — два вызова `Register`). Зависимости — `types.ResourceKey{GVK: v1 Secret, Namespace, Name}` (golden-проверка целостности потребует их разрешения, только если Secret был во входе).
2. `dexAuthenticatorFromAuthURL` — алгоритм [design.md §4.4](../design.md#44-ссылка-ingress--dexauthenticator-r14). Пример: `https://orders-dex-authenticator.shop.svc.cluster.local/dex-authenticator/auth` → (`orders`, `shop`, true); `https://dex.example.com/auth` → false; хост без `.svc` → false; namespace из URL должен совпасть с namespace Ingress'а **или** DexAuthenticator'а в графе (ищется ключ `{deckhouse.io, DexAuthenticator, ns, name}` в `allResources` для обеих версий GVK).
3. Связь: `types.Relationship{From: Ingress, To: DexAuthenticator, Type: types.RelationDeckhouse, Field: "metadata.annotations[nginx.ingress.kubernetes.io/auth-url]", Details: {"annotation": key, "annotationValue": value}}`. Старую эвристику `deckhouse.io/*` не удалять (обратная совместимость тестов `annotation_test.go:305`).
4. Отчёт R15 (по каждому DexAuthenticator, строки по алфавиту):
   - `Protected routes`: связанные Ingress/HTTPRoute или `none found: no Ingress/HTTPRoute uses its auth-url — the application is not protected`;
   - `Secret <name> (TLS for <domain>) is not in the input` — если Secret не в графе;
   - `applicationDomain <d> does not match any host of Ingress/<name>` — если связанный Ingress не содержит `spec.rules[].host == d`;
   - `Ingress/<name> has no spec.tls: DexAuthenticator works over HTTPS only`.
5. Фикстура (namespace `shop`):
   - `ingress.yaml` — Ingress `orders`: `ingressClassName: nginx`, аннотации `nginx.ingress.kubernetes.io/auth-signin: https://$host/dex-authenticator/sign_in`, `nginx.ingress.kubernetes.io/auth-url: https://orders-dex-authenticator.shop.svc.cluster.local/dex-authenticator/auth`, `nginx.ingress.kubernetes.io/auth-response-headers: X-Auth-Request-User,X-Auth-Request-Email`; `rules: [{host: orders.example.com, http: {paths: [{path: /, pathType: Prefix, backend: {service: {name: orders, port: {name: http}}}}]}}]`; `tls: [{hosts: [orders.example.com], secretName: orders-tls}]`.
   - `dexauthenticator.yaml` — `deckhouse.io/v1` `DexAuthenticator` `orders`: `applicationDomain: orders.example.com`, `applicationIngressClassName: nginx`, `applicationIngressCertificateSecretName: orders-tls`, `sendAuthorizationHeader: false`, `keepUsersLoggedInFor: 24h`, `allowedGroups: [shop-admins]`.
   - `dexauthenticator-admin.yaml` — `deckhouse.io/v2alpha1` `DexAuthenticator` `admin`: `applications: [{domain: admin.example.com, ingressClassName: nginx, ingressSecretName: admin-tls}]`.

## Тесты

- Unit (процессор): `v2alpha1` → `Processed: true`, шаблон содержит `apiVersion: deckhouse.io/v2alpha1`, values содержат `applications`; `v1` — шаблон содержит `apiVersion: deckhouse.io/v1` (как сейчас); `Dependencies` содержат Secret `orders-tls`.
- Unit (детектор): URL из шага 2 (все случаи); Ingress в `shop` + DexAuthenticator `orders` в `shop` → одна связь; DexAuthenticator в другом namespace → нет; HTTPRoute с `alb.network.deckhouse.io/auth-url` → связь.
- Unit (отчёт): все четыре находки R15; DexAuthenticator `admin` → `none found`.
- Golden (`TestDexAuthenticator`): фикстура, режимы universal/separate/umbrella: рендер содержит `DexAuthenticator/admin` с `apiVersion: deckhouse.io/v2alpha1` и `spec.applications[0].domain == admin.example.com`; `dhg graph -f <фикстура> --format dot` содержит ребро Ingress `orders` → DexAuthenticator `orders`; `--with deckhouse-report` → в отчёте `Ingress/orders` в строке `DexAuthenticator/orders`, `Secret orders-tls (TLS for orders.example.com) is not in the input`, `DexAuthenticator/admin` — `none found`.

## Критерии приёмки

- [ ] AC6 из spec.md выполнен.
- [ ] Существующие тесты `annotation_test.go` проходят без изменений.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

Генерация DexAuthenticator и аннотаций Ingress для приложения без него (не факт: решение о защите — за командой; возможный `--with dex-authenticator` описан в EPIC-08 как альтернатива); `DexClient` — EPIC-08.
