# EPIC-01 / 02: общая модель «Ingress → маршруты» (`IngressFacts`)

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | M |
| Зависит от | EPIC-01/01 |
| Требования | R25, R3, R4 (разбор), R6 (разрешение порта), §3.2 spec (контроллер) |

## Контекст

Ingress разбирают несколько мест: `IngressProcessor.extractValues` (`pkg/processor/k8s/ingress.go`, для values), `NameReferenceDetector.detectIngressToService` (`pkg/analyzer/detector/reference.go`, связи), `DetectIngressController` (`pkg/generator/ingressdetect.go`, один контроллер на весь вход). Параллельно на `main` добавляется `--with istio-ingress` (Istio Gateway + VirtualService из каждого Ingress), который разбирает Ingress ещё раз. Feature `gateway-api` — третий потребитель. Нужна одна модель фактов Ingress, не зависящая от целевого API: хосты, пути, бэкенды с портом, разрешённым через Service входа, TLS, аннотации, контроллер на Ingress.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/generator/ingressmodel.go` (новый) | Типы `IngressFacts`, `IngressHost`, `IngressPath`, `IngressBackend`, `IngressTLS`; функции `CollectIngressFacts(graph) ([]IngressFacts, []string)`, `ingressControllerOf(res, graph) IngressController`, метод `(IngressFacts) InChart(chart) bool` — сигнатуры в design.md §2.2 |
| `pkg/generator/ingressmodel_test.go` (новый) | Unit-тесты (ниже) |

## Шаги

1. Перебрать `graph.Resources` с `GVK == networking.k8s.io/v1 Ingress` в порядке `ResourceKey.String()`.
2. Хосты: группировать `spec.rules[]` по `host` в порядке первого появления (два правила с одним хостом → одна группа, пути подряд). Правило без `http` → группа без путей.
3. Пути: `path` (по умолчанию `/`), `pathType` (по умолчанию `Prefix`), бэкенд `service.name`, `service.port.number` (через существующий `toInt64` в `pkg/processor/k8s` недоступен — реализовать локально по образцу `extractServicePorts` в `networkpolicy.go`: int64/float64/int/string), `service.port.name`, `resource`.
4. Разрешение порта: `PortName != ""` → найти в графе Service `{Version: v1, Kind: Service, Namespace: ns Ingress, Name}`; в `spec.ports[]` элемент с `name == PortName` → `PortNumber = port`. Нет Service → `Service = nil`, `PortNumber = 0`, заметка `ingress <ns>/<name>: port name <p> of Service <ns>/<svc> cannot be resolved: the Service is not in the input`; Service есть, порта нет → `… has no port named <p>`.
5. TLS: `spec.tls[]` → `IngressTLS{Hosts, SecretName}` как во входе.
6. `defaultBackend` — как бэкенд пути.
7. Контроллер (spec §3.2): `IngressClass` во входе с `metadata.name == ClassName` → `spec.controller` содержит `ingress-nginx` или `nginx` → `ControllerNginx`, `traefik` → `ControllerTraefik`, `haproxy` → `ControllerHAProxy`, `istio` → `ControllerIstio`; иначе имя класса содержит `nginx` → nginx; иначе есть аннотация с префиксом `nginx.ingress.kubernetes.io/` → nginx; иначе `ControllerUnknown`. Константы — из `ingressdetect.go`, `DetectIngressController` не менять.
8. `TemplatePath` — из `ProcessedResource.TemplatePath`; `InChart` — `chart.Templates[TemplatePath]` существует.

## Тесты

- Unit `pkg/generator/ingressmodel_test.go` (графы строятся как в `features_test.go`: `types.NewResourceGraph()`, `AddResource` с `ProcessedResource{Original: &types.ExtractedResource{Object: …, GVK: …}, TemplatePath: …}`):
  - `TestCollectIngressFacts_HostsGrouped`: правила `a.example.com /x`, `b.example.com /`, `a.example.com /y` → `Hosts` = `[a: [/x, /y], b: [/]]`.
  - `TestCollectIngressFacts_Defaults`: путь без `path`/`pathType` → `/`, `Prefix`.
  - `TestCollectIngressFacts_PortNameResolved`: бэкенд `orders` `port.name: http`, Service `orders` с `ports: [{name: http, port: 8080}]` → `PortNumber == 8080`, `Service != nil`.
  - `TestCollectIngressFacts_PortNameUnresolved`: без Service → `PortNumber == 0`, одна заметка с `cannot be resolved`; Service без порта `http` → заметка `has no port named http`.
  - `TestCollectIngressFacts_PortNumberString`: `port.number: "80"` (строка из YAML) → 80.
  - `TestIngressControllerOf`: таблица — IngressClass `controller: k8s.io/ingress-nginx` → nginx; класс `nginx` без IngressClass → nginx; класс `alb`, аннотация `nginx.ingress.kubernetes.io/rewrite-target` → nginx; класс `traefik` → traefik; без класса и аннотаций → unknown.
  - `TestCollectIngressFacts_TLSAndDefaultBackend`, `TestCollectIngressFacts_ResourceBackend`, `TestIngressFactsInChart`.
- Golden: не требуется (модель не меняет chart'ы).

## Критерии приёмки

- [ ] `CollectIngressFacts` детерминирована (повторный вызов даёт равный результат, `reflect.DeepEqual`).
- [ ] Покрытие `ingressmodel.go` ≥ 90 %.
- [ ] `deadcode -test ./...` не сообщает о новых функциях (модель используется тестами; потребитель появится в задаче 05 — если `deadcode` всё же сообщает, объединить PR с задачей 05).
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

Перевод `istio-ingress` на модель — задача 03. Перевод в Gateway API — задачи 05–08.
