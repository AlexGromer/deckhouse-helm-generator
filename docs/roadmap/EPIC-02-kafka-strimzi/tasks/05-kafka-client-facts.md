# EPIC-02 / 05: Каталог ключей Kafka-клиента и извлечение фактов (`pkg/kafkaclient`)

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | M (2–3 дня) |
| Зависит от | — |
| Требования | R10, R11 |

## Контекст

Кто из workload'ов — клиент Kafka, какие у него топики, группы и параметры безопасности, сейчас не знает ни один пакет:
- `pkg/generator/networkpolicy.go:envPortMapping` знает только `KAFKA_HOST`/`KAFKA_URL` → порт 9092;
- `pkg/generator/autodeps.go` — префикс `KAFKA_`, образ `kafka`, порт 9092;
- `pkg/synth/source.go:applySpring` переносит один ключ `spring.kafka.bootstrap-servers`.

Задача создаёт чистый пакет без зависимостей от конвейера. Он используется в задачах 06 (`kafka-client`), 07 (связи) и 10 (исходники). Каталог ключей и уровни достоверности — spec.md §3.2. Нормализация — design.md §4.3.

Разбор адресов (bootstrap-списки на хосты и порты) — контракт EPIC-03. Эта задача только **собирает** bootstrap-значения как строки и делит списки по запятой, не разбирая `host:port`.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/kafkaclient/doc.go` | `// Package kafkaclient recognises Apache Kafka client configuration …` — назначение, ссылка на `docs/roadmap/EPIC-02-kafka/spec.md` §3.2 |
| `pkg/kafkaclient/types.go` | Типы `Certainty`, `Direction`, `Framework`, `Property`, `Value`, `Topic`, `Facts` (design.md §2.5); в `Facts` дополнительно `Unresolved []Value` |
| `pkg/kafkaclient/normalize.go` | `NormalizeKey(key string) string`; `isEnvForm(key string) bool` (только `[A-Z0-9_]`, есть буква) |
| `pkg/kafkaclient/catalog.go` | Каталог как данные: `var fixedKeys = []fixedKey{{norm: "springkafkabootstrapservers", kind: kindBootstrap, frameworks: …, certainty: fact}, …}` и `var patternKeys = []patternKey{{prefix: "spring.cloud.stream.bindings.", suffix: ".destination", …}, …}` — строки spec.md §3.2 один к одному. Для каждого шаблона две формы разбора исходного ключа: property (по `.`) и env (`SPRING_CLOUD_STREAM_BINDINGS_` + `<b>` + `_DESTINATION`) |
| `pkg/kafkaclient/extract.go` | `DetectFramework(props []Property) Framework`; `Extract(props []Property, framework Framework) Facts`. Правила design.md §4.3: placeholder'ы, деление по запятой, направление binding'ов, дедупликация с приоритетом достоверности. Догадки (свой ключ с `topic`, env `*_TOPIC`) не создаются для ключей, уже распознанных как факт или соглашение |
| `pkg/kafkaclient/container.go` | `FromContainer(container map[string]interface{}, configMaps map[string]map[string]string) []Property`. Обходит `env[]`: `value` → `Property{Key: name, Value: value, Origin: "env <name>"}`; `valueFrom.secretKeyRef` → `FromSecret: true`; `valueFrom.configMapKeyRef` → значение из `configMaps[name][key]`, если есть, иначе свойство не создаётся. Обходит `envFrom[].configMapRef.name` → все ключи ConfigMap с `prefix` (`envFrom[].prefix`), `Origin: "configMap <name> key <k>"`. Порядок: env контейнера после envFrom (env перекрывает, как в Kubernetes). Комментарий: временный адаптер, заменяется источником конфигурации EPIC-03 (задача 07) |
| `pkg/synth/source.go` | Переименовать `resolvePlaceholders` → `ResolvePlaceholders` (экспорт, без изменения поведения); обновить вызовы и тесты `pkg/synth`. `pkg/kafkaclient` использует её |

`DetectFramework` по spec.md §3.2: нормализованные ключи с префиксом `spring` → `spring`; `quarkus`/`mpmessaging` → `quarkus`; `micronaut` → `micronaut`; приоритет при смешении: spring > quarkus > micronaut > plain.

## Шаги

1. Типы и нормализация, тесты нормализации.
2. Каталог как таблица данных и табличный тест «каждая строка каталога → ожидаемый факт».
3. `Extract`, `DetectFramework`, `FromContainer`.
4. Экспорт `ResolvePlaceholders`.

## Тесты

Все тесты — `pkg/kafkaclient/*_test.go`, табличные.

- **`TestNormalizeKey`:** `spring.kafka.consumer.group-id`, `SPRING_KAFKA_CONSUMER_GROUP_ID`, `spring.kafka.consumer.groupId` → `springkafkaconsumergroupid`.
- **`TestCatalogFacts`:** для каждой строки spec.md §3.2 — вход в property-форме и в env-форме (если env-форма определена), ожидаемые `Facts`. Минимальный набор строк:

  | Вход | Ожидаемо |
  |---|---|
  | `spring.kafka.bootstrap-servers=b1:9093,b2:9093` | `Bootstrap` = [`b1:9093`, `b2:9093`], fact |
  | `SPRING_KAFKA_BOOTSTRAP_SERVERS=b1:9093` | `Bootstrap` [`b1:9093`], fact, `Framework` spring |
  | `spring.cloud.stream.bindings.process-in-0.destination=orders,payments` + `…process-in-0.group=billing` | `Topics` [`orders` consume, `payments` consume] (fact, направление — convention), `Groups` [`billing`] |
  | `SPRING_CLOUD_STREAM_BINDINGS_PROCESS_OUT_0_DESTINATION=invoices` | `Topics` [`invoices` produce] |
  | `spring.cloud.stream.bindings.input.destination=x` | `Topics` [`x` unknown] |
  | `mp.messaging.incoming.prices.connector=smallrye-kafka` (без `topic`) + `quarkus.application.name=pricing` | `Topics` [`prices` consume, convention], `Groups` [`pricing`, convention], `Framework` quarkus |
  | `mp.messaging.incoming.prices.topic=price-updates` + `…group.id=g1` | `Topics` [`price-updates` consume, fact], `Groups` [`g1`, fact] |
  | `mp.messaging.incoming.all.topic=orders-.*` + `…all.pattern=true` | `Topics` пуст; `Unresolved` пуст; один элемент в `Facts.Patterns` (добавить поле `Patterns []Value`) |
  | `mp.messaging.outgoing.out.topic=t` + `…out.transactional.id=tx-1` | `Topics` [`t` produce], `TransactionalIDs` [`tx-1`] |
  | `spring.kafka.producer.transaction-id-prefix=tx-` | `TransactionalPrefixes` [`tx-`] |
  | `spring.kafka.streams.application-id=wordcount` | `StreamsAppID` `wordcount` |
  | `spring.kafka.template.default-topic=${ORDERS_TOPIC}` | `Topics` пуст; `Unresolved` [{Value: `${ORDERS_TOPIC}`, Unresolved: `ORDERS_TOPIC`}] |
  | `spring.kafka.template.default-topic=${ORDERS_TOPIC:orders}` | `Topics` [`orders` produce, fact] |
  | `APP_ORDERS_TOPIC=orders` (plain) | `Topics` [`orders` unknown, guess] |
  | `KAFKA_BROKERS=k:9092` | `Bootstrap` [`k:9092`, guess] |
  | `KAFKA_BOOTSTRAP_SERVERS=k:9092` без других ключей | `Framework` plain, `Bootstrap` convention |
  | `spring.kafka.properties.sasl.mechanism=SCRAM-SHA-512`, `spring.kafka.security.protocol=SASL_SSL` | `SASLMechanism`, `SecurityProtocol` |
  | `SPRING_KAFKA_PROPERTIES_SASL_JAAS_CONFIG` с `FromSecret: true` | `JAASFromSecret: true` |
- **`TestDedupPrefersFact`:** один топик как guess (`APP_X_TOPIC=t`) и как fact (`spring.kafka.template.default-topic=t`) → один `Topic` с `Certainty` fact.
- **`TestFromContainer`:** контейнер с `env` (value, secretKeyRef, configMapKeyRef на существующий и отсутствующий ConfigMap) и `envFrom` с `prefix: APP_` → ожидаемый список `Property` и порядок.
- **`pkg/synth`:** существующие тесты зелёные после переименования.

## Критерии приёмки

- [ ] Каждая строка каталога spec.md §3.2 покрыта тестом в property- и (где применимо) env-форме.
- [ ] Значения с неразрешёнными placeholder'ами не становятся фактами и возвращаются в `Unresolved`.
- [ ] `pkg/kafkaclient` не импортирует `pkg/generator`, `pkg/processor`, `pkg/analyzer`.
- [ ] Покрытие пакета ≥ 90 %.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

- Разбор `host:port` и классификация «внутренний/внешний» (EPIC-03).
- Сбор ключей из конфигов проекта для `-s source` (задача 10).
- Построение связей (задача 07).
