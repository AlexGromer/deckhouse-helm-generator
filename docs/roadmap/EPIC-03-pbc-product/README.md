# EPIC-03: модель «сервис → PBC → продукт»

| Поле | Значение |
|---|---|
| Статус | draft (задачи 01–14 — `ready`, 15–16 — `draft`) |
| Приоритет | P1 |
| Размер | XL (3 × S, 10 × M, 3 × L) |
| Зависит от | — (задача 15 — от слияния работы над Quarkus/Micronaut в `pkg/synth`) |
| Связанные ADR | ADR-003, ADR-019, ADR-046, ADR-047, ADR-049, ADR-053, ADR-056, ADR-058, ADR-059; ADR-кандидаты — `design.md` §6 |
| Владелец общих контрактов | типы соединений и логические узлы (`design.md` §3.1–3.5), парсер адресов `pkg/connect` (§3.7, §4.2–4.9), модель PBC и вложенный umbrella (§3.6, §4.11–4.13) |

Документы: [spec.md](spec.md) — что и как проверить; [design.md](design.md) — как устроено и общие контракты.

## Проблема

dhg видит приложение как набор объектов Kubernetes и **структурные** связи между ними (селектор Service → pod'ы, ссылки на ConfigMap/Secret, Ingress → Service). Связей «кто кого вызывает» и «кто пишет и читает какой топик» в модели нет. Факты (код на 2026-10-07, коммит `a4d01f2`):

- `types.ResourceGraph` (`pkg/types/relationship.go`) содержит только объекты входа; `Relationship.To` — `ResourceKey`, поэтому внешний хост или топик Kafka выразить нельзя.
- Адреса из env разбираются в трёх местах по-разному и без общего источника:
  - `egressEndpoint`, `egressIsExternal`, `egressDetectHosts` (`pkg/generator/egresspolicies.go`, feature `istio-egress`) — только литералы `containers[].env[].value`;
  - `envPortMapping` и `extractEnvBasedPorts` (`pkg/generator/networkpolicy.go`) — порт по **имени** переменной (`REDIS_HOST` → 6379), значение не читается;
  - `knownDependencies` (`pkg/generator/autodeps.go`) — префиксы имён env и подстроки значений → Bitnami-зависимости.
- Ни один из них не читает `envFrom`/`valueFrom` ConfigMap. Синтетические приложения (`pkg/synth/manifests.go`) кладут все переменные в ConfigMap `<name>-env` через `envFrom`, поэтому для `-s source`/`-s compose` `istio-egress` не находит ни одного хоста.
- NetworkPolicy `--namespace-resources` (`generateNetworkPolicy`) разрешает ingress от любого pod'а namespace (`podSelector: {}`), egress — на порты, угаданные по именам env.
- Группировка — только «сервис → chart» (`generator.GroupResources`: метки `app.kubernetes.io/name` → `instance` → `app` → `name`, затем связи, затем namespace). Umbrella — один уровень (`pkg/generator/umbrella.go`). Метка `app.kubernetes.io/part-of` в коде не используется (поиск по `pkg/`, `cmd/`), хотя есть во входах `examples/06–09` и `tests/integration/fixtures/{fidelity,library-app}`.
- `dhg graph` (`cmd/dhg/graph.go`) читает только `-s file`, группирует по `graph.Groups` (группировка анализатора по `ServiceName`, отличная от группировки chart'ов) и не показывает вызовов.

Для продукта из десятков Java Spring микросервисов (Kafka, Keycloak, PostgreSQL) это значит: нельзя собрать PBC в один umbrella, увидеть контракты между PBC, построить минимальные NetworkPolicy, ACL Kafka (EPIC-02) и CiliumNetworkPolicy (EPIC-04).

## Цель и результат

1. Граф соединений: workload → Service входа, Service кластера вне входа, внешний хост/IP, топик (produce/consume/use) — с протоколом, портом, путём, доказательством и уверенностью.
2. Общий парсер адресов `pkg/connect` для манифестов и `pkg/synth`.
3. Модель «сервис → PBC → продукт»: `app.kubernetes.io/part-of` или `--pbc`; вложенный umbrella; раскладка separate/library по PBC.
4. Граница PBC: отчёт межPBC-контрактов (синхронных и асинхронных), граф по PBC, минимальные NetworkPolicy.

```bash
dhg generate -f ./repos/orders -f ./repos/payments -f ./repos/notifier \
  --chart-name shop --mode umbrella --group-by pbc --with connection-policies -o ./out
# Note: connections: 15 (10 high, 5 medium, 0 low), 1 values skipped; details: dhg graph --format report
```

```
out/shop/Chart.yaml                         # dependencies: notifier, orders, payments
out/shop/charts/orders/charts/order-api/    # chart сервиса
out/shop/charts/payments/charts/payment-api/
out/PBC.md                                  # контракты между PBC
```

```bash
dhg graph -f ./repos --level service --group-by pbc --format mermaid
dhg graph -f ./repos --format json --product shop > connections.json   # вход для EPIC-02/04 и внешних инструментов
```

## Scope

- **В scope:** соединения из env, ConfigMap/Secret, JVM-опций, аргументов, `SPRING_APPLICATION_JSON` и смонтированных `application*.yml/.properties`; каталог ключей Spring Boot / Spring Cloud / Quarkus / MicroProfile / Micronaut / OpenTelemetry и общих env; разрешение хостов DNS Kubernetes; топики Spring Cloud Stream / SmallRye / Spring Kafka и сопоставление со Strimzi `KafkaTopic`; PBC и продукт; вложенный umbrella; separate/library по PBC; `dhg graph` (уровни, кластеры, JSON, отчёт); `PBC.md`; feature `connection-policies`; перевод `istio-egress` на общий парсер.
- **Вне scope:**
  - чтение исходного кода Java (аннотации `@KafkaListener`, `@FeignClient`) — нужен разбор Java, отдельный эпик (design §10);
  - ACL Kafka, `KafkaUser`, `KafkaTopic` как выход — EPIC-02 (использует `TopicUsages`);
  - `CiliumNetworkPolicy`, `toFQDNs`, L7 — EPIC-04;
  - клиенты и realm Keycloak — EPIC-08; операторы БД — EPIC-09;
  - смешение источников разных типов в одном запуске — ADR-016 (Deferred), вопрос владельцу;
  - RabbitMQ как exchange/queue/routing key — только адрес брокера и `address` SmallRye AMQP;
  - изменение `--namespace-resources` — остаётся как есть (вопрос владельцу).

## Задачи и порядок

| # | Задача | Размер | Зависит от | Статус |
|---|---|---|---|---|
| 01 | [Типы соединений, логических узлов и топологии](tasks/01-connection-types.md) | S | — | ready |
| 02 | [Разбор адресов `pkg/connect`](tasks/02-address-parser.md) | M | — | ready |
| 03 | [Каталог ключей и `Extract`](tasks/03-key-catalog.md) | M | 01, 02 | ready |
| 04 | [Разрешение хоста в Service](tasks/04-service-resolver.md) | M | 01, 02 | ready |
| 05 | [Проход `connections` в анализаторе и фикстура `pbc-shop`](tasks/05-connection-pass.md) | L | 03, 04 | ready |
| 06 | [Конфигурационные файлы, JVM-опции, аргументы](tasks/06-config-sources.md) | M | 05 | ready |
| 07 | [Топики и брокер](tasks/07-topics.md) | M | 06 | ready |
| 08 | [`istio-egress` на общем парсере](tasks/08-istio-egress-unify.md) | S | 05 | ready |
| 09 | [Топология PBC: `--pbc`, `--pbc-label`](tasks/09-pbc-topology.md) | M | 01 | ready |
| 10 | [Вложенный umbrella, `--group-by pbc`](tasks/10-nested-umbrella.md) | L | 09 | ready |
| 11 | [separate и library по PBC](tasks/11-separate-library-pbc.md) | M | 10 | ready |
| 12 | [`dhg graph`: соединения, уровни, PBC, JSON](tasks/12-graph-views.md) | M | 07, 09 | ready |
| 13 | [Отчёт межPBC-контрактов `PBC.md`](tasks/13-pbc-report.md) | M | 12 | ready |
| 14 | [Feature `connection-policies`](tasks/14-connection-policies.md) | L | 07, 09 | ready |
| 15 | [Соединения из свойств синтетических проектов](tasks/15-synth-connections.md) | M | 07, внешняя | draft |
| 16 | [Вход масштаба продукта: `--git-path` ×N, источники `dhg graph`](tasks/16-product-inputs.md) | S | 12 | draft |

Порядок слияния: 01 и 02 параллельно → 03, 04 → 05 → 06 → 07; 08 после 05; 09 после 01 (параллельно с 02–07); 10 → 11; 12 после 07 и 09 → 13; 14 после 07 и 09. Каждая задача сливается отдельно и не меняет вывод без нового флага.

## Риски и открытые вопросы

| Вопрос | Варианты | Рекомендация | Кто решает |
|---|---|---|---|
| Q1. Включать раскладку по PBC автоматически, если во входе есть `part-of`? | (a) opt-in `--group-by pbc`; (b) автоматически | (a): иначе меняется вывод `examples/06–09` (`part-of: myapp`) и chart'ы, уже сгенерированные пользователями | владелец |
| Q2. Как задаётся продукт? | (a) `--chart-name` = продукт, один продукт на запуск; (b) метка `dhg.deckhouse.io/product` в объектах; (c) аннотация | (a): стандартной метки нет; (b)/(c) требуют правки манифестов во всех репозиториях и выдуманы dhg | владелец |
| Q3. Что в вашем ландшафте значит `part-of`: PBC или продукт (у Diasoft/других поставщиков)? | (a) PBC (умолчание); (b) продукт → PBC задаётся другой меткой через `--pbc-label` или `--pbc` | (a) + проверить на реальных манифестах 2–3 поставщиков до слияния задачи 09 | владелец |
| Q4. Где сервисы без PBC в umbrella? | (a) прямые subchart'ы продукта; (b) синтетический PBC `unassigned` | (a): не выдумывать PBC (ADR-059) | владелец |
| Q5. separate/library по PBC — вложенные каталоги `<pbc>/<svc>`? | (a) да; (b) плоско, PBC только в отчёте | (a): раскладка соответствует GitOps-приложениям Argo CD/Flux по PBC | владелец |
| Q6. Умолчания `connection-policies` там, где факта нет (Ingress-контроллер, мониторинг, FQDN) | (a) не ограничивать (`restrict*: false`) + `Note:`; (b) ограничивать, пользователь открывает через values | (a): (b) ломает трафик сразу после установки | владелец |
| Q7. `--namespace-resources` (широкие NetworkPolicy) | (a) оставить как есть; (b) объявить устаревшим в пользу `connection-policies` | (a) в этом эпике; (b) — после опыта эксплуатации | владелец |
| Q8. Вход масштаба продукта (задача 16) | (a) CI выкачивает репозитории, `-s file -f …` ×N (работает сейчас); (b) повторяемый `--git-path`; (c) несколько `--git-repo`; (d) смешение источников (`--input <source>=<arg>`) | (a) основной путь; (b) — если GitOps-монорепозиторий по PBC; (c), (d) — отдельным решением (ADR-016) | владелец |
| Q9. Compose: имя проекта (`name:` верхнего уровня) как PBC? | (a) нет, только `--pbc`; (b) да, как метка `part-of` синтетических объектов | (a): (b) меняет рендер синтетических chart'ов; задача 15 | владелец |
| Q10. Чтение исходного кода Java (аннотации) | (a) отдельный эпик; (b) в EPIC-03 | (a): нужен разбор Java, высокий риск ложных срабатываний | владелец |
| Q11. `dhg graph` и `.dhg.yaml` | (a) не читать (как сейчас); (b) читать только известные `graph` ключи | (a) в этом эпике | владелец |
| Риск: ложные срабатывания (значение похоже на хост) | уровни уверенности, пороги, `Skip`-причины, отчёт | политики только из ≥ `medium` | — |
| Риск: правила каталога «не проверено» (design §4.3) | проверка при реализации задач 03/07 | непроверенное правило удаляется, не угадывается | реализатор |
| Риск: работа над Quarkus/Micronaut в `pkg/synth` идёт параллельно | задача 15 — `draft`; `EnvNameMP` совпадает с `envName` из `frameworks.go` | слить списки свойств в задаче 15 | реализатор |
