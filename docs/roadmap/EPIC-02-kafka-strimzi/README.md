# EPIC-02: Kafka — Strimzi `KafkaTopic`/`KafkaUser`, внешний Kafka, ACL по связям

| Поле | Значение |
|---|---|
| Статус | draft (задачи 01–06 — `ready`, 07–10 — `draft` до фиксации контрактов EPIC-03) |
| Приоритет | P1 |
| Размер | L (сумма задач: 4×S, 5×M, 1×L) |
| Зависит от | EPIC-03: контракты «типы связей публикации/потребления топика и логические узлы топиков», «извлечение адресов и параметров подключения» (подробно — [design.md §9](design.md#9-запросы-к-другим-эпикам)) |
| Связанные ADR | ADR-003 (реестр процессоров), ADR-046 (реестр features `--with`), ADR-047 (`ValuesPath`), ADR-049 (идентичность объектов), ADR-053 (fidelity), ADR-054 (`SpecOverlay`), ADR-058/059 (синтез, только факты); ADR-кандидаты — в [design.md §6](design.md#6-adr-кандидаты) |

## Проблема

1. **Strimzi custom resources (CR, пользовательские ресурсы Kubernetes) обрабатываются только общим fallback-процессором.** Для `KafkaTopic` и `KafkaUser` нет процессора в `pkg/processor/k8s/registry.go:RegisterAll`, поэтому их рендерит `Registry.processGeneric` (`pkg/processor/registry.go`):
   - метка `strimzi.io/cluster` (имя кластера Kafka, к которому относится объект) пишется в шаблон литералом (`writeStringMap`) и не меняется через values, хотя кластер обычно разный в dev/prod;
   - `metadata.namespace` всегда `{{ .Release.Namespace }}`. Topic Operator и User Operator по умолчанию следят только за namespace кластера Kafka, а с Strimzi 1.0.1 слежение за другим namespace выключено по умолчанию (`STRIMZI_ENTITY_OPERATOR_WATCHED_NAMESPACE_ENABLED`). Объект в namespace релиза приложения молча игнорируется;
   - нет связей `KafkaUser` → `KafkaTopic` (ACL) и workload → `KafkaUser` (Secret с учётными данными, который создаёт User Operator);
   - устаревшие версии API (`kafka.strimzi.io/v1beta2`, удалена в Strimzi 1.0.0) проходят без предупреждения.
2. **Нет генерации Kafka-объектов из фактов.** Адрес брокера уже извлекается из исходников (`pkg/synth/source.go:applySpring`, только `spring.kafka.bootstrap-servers` → env `SPRING_KAFKA_BOOTSTRAP_SERVERS`), но топики, consumer groups (группы потребителей), `transactional.id` и параметры безопасности (`security.protocol`, `sasl.*`, `ssl.*`) нигде не читаются. Пользователь пишет `KafkaTopic`, `KafkaUser` и ACL руками.
3. **Нет конфигурации клиента для Kafka вне кластера.** Для `SASL_SSL` нужны Secret с `sasl.jaas.config`, truststore (хранилище доверенных сертификатов) в томе и правильные имена переменных окружения для каждого фреймворка (Spring, Quarkus, Micronaut). dhg этого не генерирует.
4. **Egress (исходящий трафик) к Kafka не описан.** `istio-egress` (`pkg/generator/egresspolicies.go:egressEndpoint`) отбрасывает значения с запятой, то есть список `bootstrap.servers` (`b1:9093,b2:9093`) никогда не превращается в ServiceEntry. Кроме того, bootstrap-адреса — только точка входа: клиент затем подключается к адресам брокеров, которые объявляет кластер (advertised listeners), и их во входе нет.
5. **Нет механизмов, без которых эпик не реализуется:**
   - процессоры и features не умеют выводить `Note:`. `processor.Result` и `types.ProcessedResource` не имеют поля заметок, `Feature.Apply` возвращает только chart и ошибку;
   - feature не может добавить env, том или `volumeMount` в существующий workload. Единственная точка расширения — замена `{{- with .podAnnotations }}` в `pkg/generator/vaultagent.go`.

## Цель и результат

Пользователь получает chart, в котором Kafka-часть приложения описана фактами входа и настраивается через values.

```bash
# Манифесты с KafkaTopic/KafkaUser: процессоры, связи, cluster и namespace в values
dhg generate -f ./manifests --chart-name orders

# Внешний Kafka по SASL_SSL: Secret с JAAS, truststore из Secret, env под фреймворк
dhg generate -s source -f ./orders-service --chart-name orders \
  --with kafka-client \
  --feature-opt kafka-client.security-protocol=SASL_SSL \
  --feature-opt kafka-client.sasl-mechanism=SCRAM-SHA-512 \
  --feature-opt kafka-client.jaas-secret=orders-kafka \
  --feature-opt kafka-client.truststore-secret=kafka-ca

# In-cluster Strimzi: KafkaTopic + KafkaUser с ACL по фактам (задача 08, draft)
dhg generate -f ./manifests --chart-name shop --with strimzi \
  --feature-opt strimzi.cluster=my-cluster --feature-opt strimzi.namespace=kafka
```

Фрагмент `values.yaml` после задачи 03 (вход — `KafkaTopic` `orders-created` с меткой `strimzi.io/cluster: my-cluster`):

```yaml
services:
  orders:
    kafkaTopics:
      ordersCreated:
        enabled: true
        cluster: my-cluster      # метка strimzi.io/cluster
        namespace: ""            # "" — namespace релиза; задать namespace Topic Operator'а
        spec:
          partitions: 6
          replicas: 3
          config:
            retention.ms: 604800000
```

## Scope

- **В scope:**
  - процессоры `KafkaTopic` и `KafkaUser` (`kafka.strimzi.io/v1`, `v1beta2`);
  - связи между Strimzi-объектами и workload'ами;
  - каталог ключей конфигурации Kafka-клиента (Spring Kafka, Spring Cloud Stream, Quarkus/SmallRye, Micronaut, «голые» env) с делением на факты и догадки;
  - feature `kafka-client` — безопасность клиента для любого Kafka;
  - feature `strimzi` — `KafkaTopic`/`KafkaUser`/ACL из связей и, опционально, `KafkaAccess`;
  - egress к брокерам через `istio-egress` и запрос в EPIC-04;
  - два общих механизма: канал заметок и инъекция в workload'ы. Ими пользуется EPIC-09.
- **Вне scope:**
  - CR `Kafka`, `KafkaNodePool`, `KafkaConnect`, `KafkaMirrorMaker2` — это платформа, не жизненный цикл приложения (граница из `docs/roadmap/README.md` §1). Они продолжают идти через generic fallback;
  - число партиций и реплик топиков — не выводятся из входа (только values + `Note:`);
  - разбор аннотаций `@KafkaListener`/`@Topic` в Java-исходниках: решение «не делать в v1» с обоснованием в [design.md §5](design.md#5-альтернативы);
  - Kafka Streams: специальные ACL с префиксом `<application.id>-` описаны, но генерация ACL для Streams — только по `spring.kafka.streams.application-id` (задача 08);
  - модуль Deckhouse `managed-kafka`: его API не удалось проверить (см. «Риски»);
  - Kerberos (`GSSAPI`): keytab и `krb5.conf` не генерируются, только `sasl.mechanism` и Note.

## Задачи и порядок

| # | Задача | Размер | Зависит от | Статус |
|---|---|---|---|---|
| 01 | [Канал заметок для процессоров и features](tasks/01-notes-channel.md) | S | — | ready |
| 02 | [Инъекция env, томов и монтирований в workload'ы из features](tasks/02-workload-injection.md) | M | — | ready |
| 03 | [Процессоры `KafkaTopic` и `KafkaUser`](tasks/03-strimzi-processors.md) | M | 01 | ready |
| 04 | [Детектор связей Strimzi-объектов](tasks/04-strimzi-detector.md) | S | 03 | ready |
| 05 | [Каталог ключей Kafka-клиента и извлечение фактов (`pkg/kafkaclient`)](tasks/05-kafka-client-facts.md) | M | — | ready |
| 06 | [Feature `kafka-client`: безопасность клиента внешнего Kafka](tasks/06-kafka-client-feature.md) | M | 01, 02, 05 | ready |
| 07 | [Связи «публикует/потребляет топик» по фактам клиента](tasks/07-topic-relationships.md) | M | 05, EPIC-03 (типы связей, источник конфигурации, парсер адресов) | draft |
| 08 | [Feature `strimzi`: `KafkaTopic`/`KafkaUser`/ACL из связей](tasks/08-strimzi-feature.md) | L | 02, 03, 06, 07 | draft |
| 09 | [Egress к брокерам Kafka](tasks/09-kafka-egress.md) | S | 06, EPIC-03 (парсер адресов), EPIC-04 | draft |
| 10 | [Kafka-факты из исходников (Spring, Quarkus, Micronaut)](tasks/10-synth-kafka-facts.md) | S | 05, EPIC-03 (источник конфигурации для `pkg/synth`) | draft |

Порядок: 01 и 02 — общие механизмы, их ждёт EPIC-09. 03→04 дают пользу на манифестах без EPIC-03. 05→06 закрывают внешний Kafka. 07–10 начинаются после того, как EPIC-03 зафиксирует имена контрактов.

## Риски и открытые вопросы

| Вопрос | Варианты | Рекомендация | Кто решает |
|---|---|---|---|
| В каком namespace создавать `KafkaTopic`/`KafkaUser` по умолчанию | a) namespace релиза (как сейчас); b) namespace из входного манифеста; c) значение `namespace` в values, по умолчанию пустое (= namespace релиза) | c): ADR-049 и остальной chart используют namespace релиза, а переопределение на namespace Topic/User Operator'а — одна строка values. В `Note:` объясняется, почему это нужно | владелец |
| Как приложение в другом namespace получает Secret `KafkaUser` (Secret создаётся в namespace оператора) | a) деплоить приложение в namespace Kafka; b) Strimzi Access Operator (`KafkaAccess`, `access.strimzi.io/v1alpha1`, версия 0.3.0 нужна для Strimzi ≥ 1.0) — binding Secret в namespace приложения; c) внешняя репликация Secret (reflector и т. п.); d) External Secrets | b) как опция feature `strimzi` (`access=kafka-access`), по умолчанию `same-namespace` с `Note:`. Access Operator — проект Strimzi под Apache-2.0, но API `v1alpha1`: риск изменений | владелец |
| Поддерживать ли `kafka.strimzi.io/v1beta2` | a) только `v1`; b) `v1` и `v1beta2` с `Note:` об удалении в Strimzi 1.0.0 | b): кластеры на Strimzi 0.49–0.51 ещё обслуживают `v1beta2`. dhg не конвертирует версию (Strimzi предлагает v1 API Conversion Tool) | владелец |
| Генерировать ACL из догадок (env `*_TOPIC`, `*.topic` в своих ключах) | a) не генерировать, только `Note:`; b) генерировать, помечая в values `source: guess`; c) генерировать выключенными (`enabled: false`) | c): пользователь видит предложение в values и включает его одной строкой, но по умолчанию права не выдаются | владелец |
| Разбор `@KafkaListener(topics=…)` в исходниках | a) не делать; b) opt-in эвристика только для строковых литералов | a) в v1 (обоснование в design.md §5). Вернуться, если по отчётам `SYNTHESIS.md` окажется, что топики Spring Kafka почти всегда не найдены | владелец |
| Модуль Deckhouse `managed-kafka` (EE, стадия Preview по данным поиска; страница deckhouse.io заблокирована прокси) — поддерживать ли его CR | a) нет; b) процессор и генерация после проверки API | a) до проверки API (поля CR, управление топиками и пользователями). Для топиков и ACL в Deckhouse рекомендуется Strimzi | владелец |
| Ключ `IdempotentWrite` для старых брокеров | a) никогда; b) параметр feature `idempotent-write=true` | b): по умолчанию выключен. С Kafka 2.8 (KIP-679) для `InitProducerId` достаточно `Write` на любой топик. Strimzi 1.2.0 поддерживает только Kafka 4.2.x–4.3.x | владелец |
| Strimzi 1.3.0 объявил PKCS #12 в Secret'ах устаревшим, в будущих версиях останется только PEM | — | В `kafka-client` по умолчанию PEM: truststore — `ssl.truststore.type=PEM`; keystore для mTLS — PEM по значению (Spring) или PKCS12 с `Note:` | — (учтено в дизайне) |
| Quarkus: читаются ли env `KAFKA_SASL_JAAS_CONFIG` и т. п. как `kafka.sasl.jaas.config` без объявления ключа в `application.properties` | — | **не проверено.** В задаче 06 для Quarkus генерируются env по правилу MicroProfile Config; агент проверяет поведение на тестовом приложении до реализации (README §4, шаг 3) | исполнитель задачи 06 |
| Механизмы задач 01 и 02 общие для нескольких эпиков (EPIC-08, EPIC-09) | a) оставить в EPIC-02; b) вынести в таблицу общих контрактов `docs/roadmap/README.md` §3 | b): владелец добавляет строки «Канал заметок» и «Инъекция в workload'ы» с владельцем EPIC-02 | владелец |
