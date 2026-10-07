# EPIC-01 / 03: `--with istio-ingress` на общей модели Ingress

| Поле | Значение |
|---|---|
| Статус | draft |
| Размер | S |
| Зависит от | EPIC-01/02; слияние в `main` работы `--with istio-ingress` и исправления ветки Istio в `--detect-ingress` |
| Требования | R25 |

## Контекст

Параллельно с проработкой этого эпика на `main` добавляется feature `istio-ingress`: Istio `Gateway` + `VirtualService` из каждого Ingress, и исправляется ветка `ControllerIstio` в `GenerateIngressAnnotations` (`pkg/generator/ingressdetect.go`, сейчас `// TODO: Generate VirtualService templates from Ingress resources.`). Её код на момент проработки не опубликован, поэтому имена файлов и функций ниже — **не проверено**; их нужно сверить с `main` перед реализацией.

Цель: `istio-ingress` получает хосты, пути, бэкенды (с портом, разрешённым через Service), TLS и аннотации из `CollectIngressFacts` (задача 02), а не из собственного разбора Ingress. Поведение `istio-ingress` не меняется.

## Открытый вопрос (блокирует `ready`)

1. Где в `istio-ingress` разбирается Ingress (файл, функция) и совпадает ли его трактовка `pathType`, пустого хоста, `defaultBackend` и имени порта с моделью задачи 02? Расхождения: (a) выровнять модель под `istio-ingress`; (b) выровнять `istio-ingress` под модель (меняет вывод — нужен отдельный пункт в `docs/RELEASE_NEXT.md`). Решает владелец после чтения кода.

## Изменения (предварительно)

| Файл | Что сделать |
|---|---|
| файл feature `istio-ingress` в `pkg/generator/` (имя — сверить) | Заменить разбор `unstructured` Ingress на `CollectIngressFacts(fc.Graph)` + `InChart(chart)`; соответствие полей: `IngressHost.Host` → `VirtualService.spec.hosts`, `IngressPath` → `http[].match[].uri` (`Exact` → `exact`, `Prefix` → `prefix`), `IngressBackend.ServiceName`/`PortNumber` → `route[].destination.host`/`.port.number`, `IngressTLS.SecretName` → `Gateway.spec.servers[].tls.credentialName` |
| тесты `istio-ingress` | Существующие тесты должны проходить без изменения ожиданий (кроме согласованных в открытом вопросе) |

## Шаги

1. Прочитать код `istio-ingress` на `main`; заполнить таблицу соответствия; решить открытый вопрос.
2. Перевести разбор на модель; удалить ставший мёртвым код (`deadcode -test ./...`).

## Тесты

- Unit: существующие тесты `istio-ingress`; новый тест — Ingress с `port.name`, разрешаемым через Service, даёт `destination.port.number` (если `istio-ingress` раньше не разрешал имя — согласовать в открытом вопросе).
- Golden: существующие проверки `TestFeaturesPassHelm` для `istio-ingress`; если у `istio-ingress` есть свой golden-тест фактов — без изменений ожиданий.

## Критерии приёмки

- [ ] Ingress разбирается в одном месте (`ingressmodel.go`) для `gateway-api` и `istio-ingress`.
- [ ] Вывод `istio-ingress` на всех golden-входах не изменился (сравнить `helm template` до и после на `examples/*` и `tests/integration/fixtures/*`).
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

Новые возможности `istio-ingress`; аннотации ingress-nginx для Istio (таблица §3.3 spec относится к Gateway API).
