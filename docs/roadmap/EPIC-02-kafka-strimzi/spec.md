# EPIC-02: спецификация

## 1. Термины

| Термин | Значение |
|---|---|
| Strimzi | Оператор Apache Kafka для Kubernetes (CNCF). Cluster Operator управляет кластером. Topic Operator и User Operator (вместе — Entity Operator) управляют `KafkaTopic` и `KafkaUser` |
| `KafkaTopic` | CR Strimzi: топик Kafka. Поля `spec`: `topicName`, `partitions`, `replicas`, `config` |
| `KafkaUser` | CR Strimzi: пользователь Kafka. Поля `spec`: `authentication`, `authorization`, `quotas`, `template`. User Operator создаёт Secret **с тем же именем**, что у `KafkaUser`, в namespace `KafkaUser` |
| ACL (access control list) | Список прав Kafka: тип ресурса (`topic`, `group`, `cluster`, `transactionalId`), имя, `patternType` (`literal`/`prefix`), операции |
| Consumer group (группа потребителей) | Имя `group.id`: потребители с одним `group.id` делят партиции топика |
| `transactional.id` | Идентификатор транзакционного продюсера. ACL выдаются на ресурс `transactionalId` |
| Bootstrap servers | Список `host:port`, к которому клиент подключается первым. Дальнейшие соединения идут на адреса брокеров, которые объявляет кластер (advertised listeners) |
| Watched namespace | Namespace, в котором Topic/User Operator ищет свои CR. По умолчанию — namespace кластера Kafka |
| Факт / соглашение / догадка | Уровни достоверности значения: **факт** — явный ключ конфигурации; **соглашение** — документированное значение по умолчанию фреймворка; **догадка** — вывод по имени ключа. См. §3 |
| Источник конфигурации | Плоский набор пар «ключ → значение» workload'а (env, ConfigMap, конфиги проекта). Контракт EPIC-03 |
| Инъекция | Добавление env, `envFrom`, томов и монтирований в workload через values (задача 02) |

## 2. Требования

**Общие механизмы**

- **R1.** Процессор может вернуть заметки (`processor.Result.Notes`), а feature — добавить заметку через `FeatureContext`. CLI печатает каждую уникальную заметку один раз в stderr как `Note: <текст>`. При синтетическом источнике заметки features и процессоров дописываются в раздел «Notes» файла `SYNTHESIS.md`.
- **R2.** Feature может добавить env, `envFrom`, `volumeMounts` контейнеру и `volumes` pod'у любого workload'а (Deployment, StatefulSet, DaemonSet, Job, CronJob) chart'а. Добавление объявляется в values feature (`<key>.inject`) и действует только при `<key>.enabled: true`. Если инъекций нет, рендер workload'а не меняется (golden fidelity).

**Strimzi-объекты из входа**

- **R3.** `KafkaTopic` и `KafkaUser` с `apiVersion` `kafka.strimzi.io/v1` или `kafka.strimzi.io/v1beta2` обрабатываются типизированными процессорами. Рендер повторяет `apiVersion` входа: dhg не конвертирует версии. Другие версии (`v1alpha1`) остаются у generic fallback.
- **R4.** Рендер содержит весь `spec` входа (ADR-054). Отдельные ключи values переопределяют поля `spec`: у `KafkaTopic` — `topicName`, `partitions`, `replicas`, `config`; у `KafkaUser` — `authentication`, `authorization`, `quotas`, `template`.
- **R5.** Значение метки `strimzi.io/cluster` берётся из values (`cluster`), по умолчанию — из входа. Остальные метки входа сохраняются (ADR-049). Если `cluster` пуст, метка не рендерится и выводится `Note:`: без метки оператор объект не обрабатывает.
- **R6.** `metadata.namespace` = `namespace` из values, а если он пуст — namespace релиза. Для объектов из входа выводится `Note:` о watched namespace оператора, если namespace входа отличается от namespace остальных объектов входа.
- **R7.** Values объекта лежат по `services.<svc>.kafkaTopics.<key>` / `services.<svc>.kafkaUsers.<key>`, где `<key>` — имя объекта, приведённое функцией `sanitizeName` пакета `k8s`. Несколько топиков одного сервиса не конфликтуют.
- **R8.** Для `v1beta2` выводится `Note:`: версия удалена в Strimzi 1.0.0, нужна конвертация (v1 API Conversion Tool). Для `KafkaUser` с полем `spec.authorization.acls[].operation` (единственное число) выводится `Note:`: поле удалено в `v1`, нужно `operations`.
- **R9.** Определяются связи (тип `name_reference`, `types.RelationNameReference`):
  - `KafkaUser` → `KafkaTopic` для ACL `type: topic`, `patternType: literal` (или без `patternType`), имя которого совпадает с `spec.topicName` или `metadata.name` топика из входа;
  - workload → `KafkaUser`, если pod ссылается на Secret с именем `KafkaUser` (`secretKeyRef`, `envFrom.secretRef`, том `secret`/`projected`) в том же namespace;
  - `KafkaUser`/`KafkaTopic` → CR `Kafka` с именем из `strimzi.io/cluster`, если `Kafka` есть во входе;
  - `KafkaUser` → Secret из `spec.authentication.password.valueFrom.secretKeyRef.name`.

**Факты Kafka-клиента**

- **R10.** Функция извлечения (`pkg/kafkaclient`) по плоскому набору ключей одного контейнера возвращает:
  - фреймворк;
  - bootstrap-значения;
  - параметры безопасности;
  - топики с направлением (produce/consume/unknown);
  - группы;
  - `transactional.id` и их префиксы;
  - `application.id` Kafka Streams.

  Каждое значение несёт ключ-источник и уровень достоверности. Каталог ключей — §3.2.
- **R11.** Значения с неразрешённым placeholder'ом (`${NAME}` без значения по умолчанию) не считаются фактом. Они попадают в результат с пометкой `unresolved` и порождают `Note:`.

**Внешний Kafka (feature `kafka-client`)**

- **R12.** `--with kafka-client` для каждого workload'а chart'а, у которого есть bootstrap-факт (или который перечислен в параметре `workloads`), добавляет через инъекцию (R2) настройки безопасности под фреймворк контейнера (§4.3).
- **R13.** Учётные данные и сертификаты chart **никогда не создаёт и не кладёт в values**. Он ссылается на существующие Secret'ы по имени, как ADR-059 для паролей. Пустое имя Secret'а — инъекция для этой части не рендерится, выводится `Note:`.
- **R14.** Truststore по умолчанию — PEM-файл из Secret (`ssl.truststore.type=PEM`, Kafka ≥ 2.7.0). PKCS12 и JKS поддерживаются с ключом пароля из того же Secret'а.

**Связи и генерация для Strimzi (draft, после EPIC-03)**

- **R15.** По фактам клиента строятся связи EPIC-03 «workload публикует топик» / «workload потребляет топик» с логическим узлом топика.
- **R16.** `--with strimzi` рендерит:
  - `KafkaTopic` для каждого топика-факта, которого нет во входе;
  - `KafkaUser` на каждый workload-клиент с ACL минимальных прав (§4.4);
  - опционально `KafkaAccess` (Strimzi Access Operator) для доступа из другого namespace.

  Топики-догадки и ACL по ним рендерятся выключенными (`enabled: false`).
- **R17.** `partitions` и `replicas` генерируемых топиков не задаются: Strimzi берёт `num.partitions` и `default.replication.factor` брокера. Values содержат ключи со значением `null`, `Note:` требует их заполнить.

**Egress (draft)**

- **R18.** Хосты bootstrap-списка внешнего Kafka и явно заданные адреса брокеров (`kafkaClient.brokers`) попадают в ServiceEntry `istio-egress` и передаются EPIC-04 для `toFQDNs`. Standalone NetworkPolicy только для Kafka не генерируется (§7).

**Исходники (draft)**

- **R19.** Для `-s source` ключи Kafka из конфигов проекта (Spring, Quarkus, Micronaut) становятся фактами в том же виде, что и для манифестов, через источник конфигурации EPIC-03.

## 3. Входы

### 3.1 Strimzi CR

| Источник | Факт | Правило |
|---|---|---|
| `KafkaTopic.metadata.labels["strimzi.io/cluster"]` | кластер Kafka | → values `cluster` |
| `KafkaTopic.metadata.namespace` | watched namespace оператора (вероятно) | не копируется в шаблон (namespace релиза), см. R6 |
| `KafkaTopic.spec.*` | топик | весь `spec` → values `spec`; поля из R4 — переопределения |
| `KafkaUser.spec.authentication.type` | `tls` / `tls-external` / `scram-sha-512` | в `spec` |
| `KafkaUser.spec.authorization` | `type: simple`, `acls[]` (`resource.type` ∈ `topic`, `group`, `cluster`, `transactionalId`; `resource.patternType` ∈ `literal`, `prefix`; `operations[]` ∈ `Read`, `Write`, `Create`, `Delete`, `Alter`, `Describe`, `ClusterAction`, `AlterConfigs`, `DescribeConfigs`, `IdempotentWrite`, `All`; `type` ∈ `allow`, `deny`; `host`) | в `spec`, связи R9 |
| `KafkaUser.spec.quotas` | `producerByteRate`, `consumerByteRate`, `requestPercentage`, `controllerMutationRate` | в `spec` |
| Pod spec workload'а: ссылки на Secret | потребитель учётных данных `KafkaUser` | связь R9 |

### 3.2 Каталог ключей Kafka-клиента

Ключи сравниваются в **нормализованной форме**: нижний регистр, символы `.`, `-`, `_` удалены (`spring.kafka.consumer.group-id` ≡ `SPRING_KAFKA_CONSUMER_GROUP_ID` ≡ `springkafkaconsumergroupid`). Это повторяет relaxed binding Spring Boot: `SystemEnvironmentPropertyMapper` сопоставляет свойству и каноническое имя env без дефисов, и «legacy»-имя с `_` вместо `-`. Для ключей с переменным сегментом (`<b>`, `<ch>`) сопоставление идёт по префиксу и суффиксу.

**Фреймворк контейнера** определяется по набору ключей: есть ключи с префиксом `spring.` / env `SPRING_` → `spring`; `quarkus.` / `mp.messaging.` / env `QUARKUS_`, `MP_MESSAGING_` → `quarkus`; `micronaut.` / env `MICRONAUT_` → `micronaut`; иначе `plain`. Синтетический источник передаёт фреймворк явно (Spring Boot из `pom.xml`/`build.gradle`).

| Ключ (property-форма) | Фреймворк | Что | Достоверность |
|---|---|---|---|
| `spring.kafka.bootstrap-servers`, `spring.kafka.{consumer,producer,admin,streams}.bootstrap-servers` | spring | bootstrap | факт |
| `spring.cloud.stream.kafka.binder.brokers` | spring (Spring Cloud Stream) | bootstrap; хост без порта → порт `spring.cloud.stream.kafka.binder.default-broker-port`, по умолчанию 9092 | факт |
| `kafka.bootstrap.servers` | quarkus, micronaut | bootstrap | факт |
| `mp.messaging.connector.smallrye-kafka.bootstrap.servers`, `mp.messaging.{incoming,outgoing}.<ch>.bootstrap.servers` | quarkus | bootstrap | факт |
| env `KAFKA_BOOTSTRAP_SERVERS` | plain | bootstrap | соглашение (так это имя читают Quarkus и Micronaut) |
| env, имя содержит `KAFKA` и оканчивается на `_BROKERS`, `_BOOTSTRAP`, `_SERVERS`, `_HOSTS` | plain | bootstrap | догадка |
| `spring.kafka.security.protocol`, `spring.kafka.properties.security.protocol`, `spring.cloud.stream.kafka.binder.configuration.security.protocol`, `kafka.security.protocol`, `mp.messaging.connector.smallrye-kafka.security.protocol` | все | `security.protocol` | факт |
| `…properties.sasl.mechanism`, `…configuration.sasl.mechanism`, `kafka.sasl.mechanism`, `mp.messaging.connector.smallrye-kafka.sasl.mechanism` | все | `sasl.mechanism` | факт |
| `spring.kafka.ssl.trust-store-type`, `spring.kafka.ssl.key-store-type`, `kafka.ssl.truststore.type`, `kafka.ssl.keystore.type` | все | тип хранилищ | факт |
| `spring.kafka.template.default-topic` | spring | топик, produce | факт |
| `spring.kafka.consumer.group-id` | spring | группа (для listener'ов без своего `groupId`) | факт |
| `spring.kafka.producer.transaction-id-prefix`, `spring.kafka.template.transaction-id-prefix` | spring | префикс `transactional.id` | факт |
| `spring.kafka.streams.application-id` | spring | `application.id` Kafka Streams | факт |
| `spring.cloud.stream.bindings.<b>.destination` | spring | топик(и), значение через запятую. Направление: `<b>` вида `…-in-<n>` → consume, `…-out-<n>` → produce (функциональная модель Spring Cloud Stream); иначе unknown | факт (топик), соглашение (направление) |
| `spring.cloud.stream.bindings.<b>.group` | spring | группа consumer-binding'а | факт |
| `mp.messaging.incoming.<ch>.topic`, `mp.messaging.incoming.<ch>.topics` (через запятую) | quarkus | топик, consume | факт |
| `mp.messaging.incoming.<ch>.pattern=true` | quarkus | `topic` — регулярное выражение | не топик; `Note:` (literal ACL невозможен) |
| `mp.messaging.outgoing.<ch>.topic` | quarkus | топик, produce | факт |
| `mp.messaging.{incoming,outgoing}.<ch>.connector=smallrye-kafka` без `topic` | quarkus | топик = имя канала («By default, the topic name is same as the channel name», Quarkus Kafka guide) | соглашение |
| `mp.messaging.incoming.<ch>.group.id`; если нет — `quarkus.application.name` (Quarkus Kafka guide: «If the group.id attribute is not set, it defaults the quarkus.application.name») | quarkus | группа | факт / соглашение |
| `mp.messaging.outgoing.<ch>.transactional.id` | quarkus | `transactional.id` | факт |
| `kafka.consumers.<group>.*` | micronaut | группа `<group>` существует | соглашение (**не проверено**) |
| свой ключ, последний сегмент `topic`, `topics`, `…-topic`, `…Topic`; env `*_TOPIC`, `*_TOPICS` | все | топик, направление unknown | догадка |
| env с `KAFKA` в имени и окончанием `_GROUP_ID`, `_CONSUMER_GROUP` | plain | группа | догадка |

Значения `spring.cloud.stream.bindings.<b>.destination` и `mp.messaging.incoming.<ch>.topics` разбиваются по запятой с обрезкой пробелов. Пустые элементы отбрасываются.

## 4. Выходы

### 4.1 Шаблон `KafkaTopic` (задача 03)

Вход:

```yaml
apiVersion: kafka.strimzi.io/v1
kind: KafkaTopic
metadata:
  name: orders-created
  namespace: kafka
  labels:
    strimzi.io/cluster: my-cluster
    app.kubernetes.io/name: orders
spec:
  partitions: 6
  replicas: 3
  config:
    retention.ms: 604800000
```

Рендер с values по умолчанию (`helm template r chart -n shop`):

```yaml
apiVersion: kafka.strimzi.io/v1
kind: KafkaTopic
metadata:
  name: orders-created
  namespace: shop                      # namespace релиза; values namespace: kafka → kafka
  labels:
    app.kubernetes.io/name: orders     # метки входа побеждают метки chart'а
    strimzi.io/cluster: my-cluster     # из values cluster
    helm.sh/chart: …
spec:
  config:
    retention.ms: 604800000
  partitions: 6
  replicas: 3
```

Values: `services.orders.kafkaTopics.ordersCreated` = `{enabled: true, cluster: my-cluster, namespace: "", spec: {…}}`. Файл шаблона — `templates/orders-kafkatopic-orders-created.yaml`. `KafkaUser` — аналогично: `kafkaUsers`, `templates/<svc>-kafkauser-<name>.yaml`.

### 4.2 Инъекция (задача 02)

```yaml
kafkaClient:                       # top-level ключ любой feature
  enabled: true
  inject:
    Deployment/orders:             # <Kind>/<metadata.name> workload'а из входа
      volumes:
        - name: dhg-kafka-truststore
          secret: {secretName: kafka-ca, items: [{key: ca.crt, path: ca.crt}]}
      containers:
        orders:                    # имя контейнера
          env:
            - name: SPRING_KAFKA_SECURITY_PROTOCOL
              value: SASL_SSL
          volumeMounts:
            - {name: dhg-kafka-truststore, mountPath: /etc/kafka/truststore, readOnly: true}
```

Списки из всех включённых блоков конкатенируются после собственных списков workload'а в порядке сортировки top-level ключей values.

### 4.3 Feature `kafka-client` (задача 06)

Values (top-level ключ `kafkaClient`):

| Ключ | Тип | По умолчанию | Смысл |
|---|---|---|---|
| `enabled` | bool | `true` | выключатель |
| `securityProtocol` | string | параметр `security-protocol` (`""`) | `PLAINTEXT` / `SSL` / `SASL_PLAINTEXT` / `SASL_SSL`; `""` — не задавать |
| `sasl.mechanism` | string | параметр `sasl-mechanism` (`""`) | `PLAIN` / `SCRAM-SHA-256` / `SCRAM-SHA-512` / `OAUTHBEARER` / `GSSAPI` |
| `sasl.jaasConfigSecret.name` / `.key` | string | параметр `jaas-secret` (`""`) / `sasl.jaas.config` | Secret с готовой строкой JAAS (формат Secret'а `KafkaUser` Strimzi) |
| `sasl.oauthbearer.tokenEndpointUrl` | string | `""` | `sasl.oauthbearer.token.endpoint.url` для `OAUTHBEARER` |
| `tls.truststore.secretName` / `.key` / `.type` / `.passwordKey` / `.mountPath` | string | параметр `truststore-secret` (`""`) / `ca.crt` / `PEM` / `""` / `/etc/kafka/truststore` | доверенные CA |
| `tls.keystore.secretName` / `.type` / `.certKey` / `.keyKey` / `.storeKey` / `.passwordKey` / `.mountPath` | string | параметр `keystore-secret` (`""`) / `PEM` / `user.crt` / `user.key` / `user.p12` / `user.password` / `/etc/kafka/keystore` | клиентский сертификат (mTLS) |
| `workloads` | map | обнаруженные workload'ы | `<Kind>/<name>: {container: <name>, framework: spring\|quarkus\|micronaut\|plain}` |
| `inject` | map | вычисляется feature | §4.2; пользователь может править |

Отображение на переменные окружения (значения `…` берутся из values. `valueFrom` — `secretKeyRef` на Secret из values):

| Свойство клиента | spring | quarkus | micronaut | plain |
|---|---|---|---|---|
| `security.protocol` | `SPRING_KAFKA_SECURITY_PROTOCOL` | `KAFKA_SECURITY_PROTOCOL` | `KAFKA_SECURITY_PROTOCOL` | `Note:` (имена неизвестны) |
| `sasl.mechanism` | `SPRING_KAFKA_PROPERTIES_SASL_MECHANISM` | `KAFKA_SASL_MECHANISM` | `KAFKA_SASL_MECHANISM` | `Note:` |
| `sasl.jaas.config` (Secret) | `SPRING_KAFKA_PROPERTIES_SASL_JAAS_CONFIG` | `KAFKA_SASL_JAAS_CONFIG` | `KAFKA_SASL_JAAS_CONFIG` | `Note:` |
| `sasl.oauthbearer.token.endpoint.url` | `SPRING_KAFKA_PROPERTIES_SASL_OAUTHBEARER_TOKEN_ENDPOINT_URL` | `KAFKA_SASL_OAUTHBEARER_TOKEN_ENDPOINT_URL` | то же | `Note:` |
| `ssl.truststore.type` | `SPRING_KAFKA_SSL_TRUST_STORE_TYPE` | `KAFKA_SSL_TRUSTSTORE_TYPE` | `KAFKA_SSL_TRUSTSTORE_TYPE` | `Note:` |
| `ssl.truststore.location` | `SPRING_KAFKA_SSL_TRUST_STORE_LOCATION` = `file:<mountPath>/<key>` | `KAFKA_SSL_TRUSTSTORE_LOCATION` = `<mountPath>/<key>` | то же | `Note:` |
| `ssl.truststore.password` (Secret, только PKCS12/JKS) | `SPRING_KAFKA_SSL_TRUST_STORE_PASSWORD` | `KAFKA_SSL_TRUSTSTORE_PASSWORD` | то же | `Note:` |
| keystore PEM по значению (Secret) | `SPRING_KAFKA_SSL_KEY_STORE_TYPE=PEM`, `SPRING_KAFKA_SSL_KEY_STORE_CERTIFICATE_CHAIN`, `SPRING_KAFKA_SSL_KEY_STORE_KEY` | `KAFKA_SSL_KEYSTORE_TYPE=PEM`, `KAFKA_SSL_KEYSTORE_CERTIFICATE_CHAIN`, `KAFKA_SSL_KEYSTORE_KEY` | то же | `Note:` |
| keystore PKCS12 файлом | `SPRING_KAFKA_SSL_KEY_STORE_TYPE=PKCS12`, `…_LOCATION=file:…`, `…_PASSWORD` (Secret) | `KAFKA_SSL_KEYSTORE_TYPE`, `…_LOCATION`, `…_PASSWORD` | то же | `Note:` |

Имена Spring опираются на `KafkaProperties` (`spring.kafka.security.protocol`, `spring.kafka.ssl.*`, `spring.kafka.properties.*`) и на relaxed binding env → `Map<String,String>` (ключи карты в нижнем регистре). Имена Quarkus/Micronaut получены по правилу MicroProfile Config / Micronaut (точки → `_`, верхний регистр). Их чтение без объявления ключа в конфиге приложения **не проверено** (README, «Риски»).

### 4.4 Feature `strimzi` (задача 08, draft)

ACL минимальных прав по фактам (источник — таблица «Operations and Resources on Protocols» документации Kafka, `docs/security/authorization-and-acls.md`):

| Факт | ACL `KafkaUser` |
|---|---|
| produce в топик `T` | `topic T literal`: `Write`, `Describe` |
| consume из топика `T` | `topic T literal`: `Read`, `Describe` |
| группа `G` | `group G literal`: `Read` (JoinGroup, SyncGroup, Heartbeat, OffsetCommit, ConsumerGroupHeartbeat требуют `Read` на группу; OffsetFetch и FindCoordinator — `Describe`, который включён в `Read`) |
| `transactional.id` `X` / префикс `P` | `transactionalId X literal` / `P prefix`: `Write`, `Describe` (InitProducerId, AddPartitionsToTxn, EndTxn — `Write`; FindCoordinator — `Describe`). Для `sendOffsetsToTransaction` — `Read` на группу (TxnOffsetCommit, AddOffsetsToTxn) |
| параметр `idempotent-write=true` | `cluster`: `IdempotentWrite` (нужен только брокерам < 2.8, KIP-679) |
| `application.id` `A` Kafka Streams | `group A literal`: `Read`; `topic A- prefix`: внутренние топики (точный набор операций **не проверен**, задача 08 — draft) |

`Describe` указывается явно, хотя `Read`, `Write`, `Delete`, `Alter` его подразумевают (`StandardAuthorizerData.IMPLIES_DESCRIBE` в Kafka). Так ACL читаются однозначно и совпадают с `kafka-acls --producer/--consumer`.

## 5. Интерфейс

| Механизм | Значение |
|---|---|
| Процессоры (задача 03) | Работают всегда, флагов нет |
| `--with kafka-client` | Параметры: `security-protocol` (`""` — не задавать), `sasl-mechanism` (`""`), `jaas-secret` (`""`), `truststore-secret` (`""`), `truststore-type` (`PEM`), `keystore-secret` (`""`), `keystore-type` (`PEM`), `workloads` (`""` — по фактам; иначе список `Kind/name` через запятую), `framework` (`auto`) |
| `--with strimzi` (draft) | Параметры: `cluster` (`""`), `namespace` (`""`), `authentication` (`scram-sha-512`; также `tls`, `tls-external`, `none`), `access` (`same-namespace`; также `kafka-access`), `listener` (`""`), `idempotent-write` (`false`), `guesses` (`disabled`; также `omit`) |
| `.dhg.yaml` | `with: [kafka-client]`, `feature-opt: ["kafka-client.jaas-secret=orders-kafka"]` — ключи совпадают с флагами (`cmd/dhg/config.go`) |

Совместимость: новые флаги CLI не добавляются. Feature `istio-egress` сохраняет параметры, меняется только обнаружение хостов (задача 09).

## 6. Невыводимое

| Что | Как показывается |
|---|---|
| `partitions`, `replicas`, `config` генерируемых топиков | values `null`/`{}` + `Note:` «`<topic>`: partitions/replicas not derived; Strimzi uses broker defaults num.partitions/default.replication.factor — set them in values» |
| Имя кластера Strimzi для генерируемых объектов | параметр `strimzi.cluster`; пусто → объекты не рендерятся + `Note:` |
| Watched namespace Topic/User Operator'а | values `namespace` + `Note:` |
| Пароли, JAAS-строки, сертификаты | только имена Secret'ов (R13); пустое имя → `Note:` |
| Адреса брокеров за bootstrap (advertised listeners) | values `kafkaClient.brokers: []` + `Note:` для egress |
| Группа consumer'а Spring Kafka, заданная в `@KafkaListener(groupId=…)` | не выводится; `Note:` при consume без группы |
| Топики `@KafkaListener`/`@Topic` в коде | не выводятся (design.md §5) |

## 7. Ошибки и граничные случаи

- `KafkaTopic` без `spec` (недопустимо для `v1`) — рендерится без `spec` и с `Note:` (как вход), ошибки нет.
- Имя топика Kafka недопустимо как имя объекта Kubernetes (`Orders_Created`): генерируемый `KafkaTopic` получает `metadata.name` по правилу DNS-1123 (`orders-created`) и `spec.topicName: Orders_Created`. Коллизия имён после приведения разрешается суффиксом `-2`, `-3`… и выводится `Note:`.
- Bootstrap с неразрешённым placeholder'ом — не факт (R11).
- Топики с `pattern=true` (Quarkus) — не создаются, ACL не выдаются, `Note:`.
- Один топик и produce, и consume у одного workload'а — ACL объединяются: `Read`, `Write`, `Describe`.
- `feature-opt kafka-client.workloads` с несуществующим `Kind/name` — ошибка `unknown workload "<Kind/name>" in workloads (chart <name>)`.
- `security-protocol` вне списка, `sasl-mechanism` вне списка, `truststore-type` ∉ {`PEM`, `PKCS12`, `JKS`} — ошибка feature с перечислением допустимых значений.
- Standalone NetworkPolicy для egress к Kafka не генерируется. Политика с `policyTypes: [Egress]` запретила бы workload'у весь остальной исходящий трафик (DNS, другие сервисы), а адреса брокеров (`ipBlock`) не выводятся из входа.

## 8. Критерии приёмки эпика

- **AC1** (R3–R8): фикстура `tests/integration/fixtures/kafka-app` (задача 03) проходит golden во всех режимах со `fidelity: true`. В рендере `KafkaTopic orders-created` имеет метку `strimzi.io/cluster: my-cluster` и `spec.partitions: 6`, а `--set services.orders.kafkaTopics.ordersCreated.cluster=prod` меняет метку.
- **AC2** (R9): `dhg graph -f tests/integration/fixtures/kafka-app` выводит рёбра `KafkaUser/orders → KafkaTopic/orders-created` и `Deployment/orders → KafkaUser/orders`.
- **AC3** (R1): `dhg generate` на фикстуре печатает `Note:` про `v1beta2` ровно один раз.
- **AC4** (R2): golden без `--with` не меняет ни одного отрендеренного объекта (существующий набор зелёный без правки ожиданий).
- **AC5** (R12–R14): `--with kafka-client --feature-opt kafka-client.security-protocol=SASL_SSL --feature-opt kafka-client.sasl-mechanism=SCRAM-SHA-512 --feature-opt kafka-client.jaas-secret=orders-kafka --feature-opt kafka-client.truststore-secret=kafka-ca` на `tests/golden/testdata/synth/orders-service` даёт Deployment `orders` с env `SPRING_KAFKA_SECURITY_PROTOCOL=SASL_SSL`, `SPRING_KAFKA_PROPERTIES_SASL_JAAS_CONFIG` из `secretKeyRef {name: orders-kafka, key: sasl.jaas.config}`, томом `dhg-kafka-truststore` из Secret `kafka-ca` и `SPRING_KAFKA_SSL_TRUST_STORE_LOCATION=file:/etc/kafka/truststore/ca.crt`.
- **AC6**: все features вместе (`TestFeaturesPassHelm`) проходят Helm на всех входах.
- **AC7** (R15–R17, draft): будет уточнён после контрактов EPIC-03.

## 9. Проверенные факты и источники

Дата проверки — 2026-10-07. strimzi.io, kafka.apache.org, deckhouse.io, deckhouse.ru, community.broadcom.com и charts.bitnami.com заблокированы прокси. Факты проверены по исходникам на GitHub (`raw.githubusercontent.com`, `git ls-remote`).

| Факт | Источник | Статус |
|---|---|---|
| Последний GA-релиз Strimzi — 1.2.0; существует тег `1.3.0-rc1` | `git ls-remote --tags https://github.com/strimzi/strimzi-kafka-operator`; блог strimzi.io «what is new in strimzi 1.2.0» от 2026-08-20 (по результату поиска) | проверено |
| Strimzi 0.49.0 ввёл API `v1` и перевёл на него Topic/User Operator; 1.0.0 удалил `v1beta2` (и `v1alpha1`/`v1beta2` у `KafkaTopic`/`KafkaUser`) и хранит `v1` | `CHANGELOG.md` strimzi-kafka-operator (main), разделы 0.49.0 и 1.0.0 | проверено |
| Перед переходом на 0.49 нужно заменить `acls[].operation` на `acls[].operations` | `CHANGELOG.md`, 0.49.0 «Major changes» | проверено |
| CRD `kafkatopics.kafka.strimzi.io` и `kafkausers.kafka.strimzi.io`: единственная версия `v1` (served, storage), `scope: Namespaced`, поля `spec` из §3.1, `partitions`/`replicas` при отсутствии берутся из `num.partitions`/`default.replication.factor` брокера | `packaging/install/cluster-operator/043-Crd-kafkatopic.yaml`, `044-Crd-kafkauser.yaml` (main) | проверено |
| `authentication.type` ∈ `tls`, `tls-external`, `scram-sha-512`; `authorization.type` = `simple`; `resource.type` ∈ `topic`, `group`, `cluster`, `transactionalId`; `patternType` ∈ `literal`, `prefix` | те же CRD | проверено |
| User Operator создаёт Secret с именем `KafkaUser`: для SCRAM — ключи `password`, `sasl.jaas.config`; для mTLS — `ca.crt`, `user.crt`, `user.key`, `user.p12`, `user.password`. CA кластера — Secret `<cluster>-cluster-ca-cert` (`ca.crt`, `ca.p12`, `ca.password`) | `documentation/modules/security/con-securing-client-authentication.adoc`, `ref-certificates-and-secrets.adoc` (main) | проверено |
| Topic Operator по умолчанию следит за namespace кластера Kafka; другой namespace — только с `STRIMZI_ENTITY_OPERATOR_WATCHED_NAMESPACE_ENABLED=true` (с 1.0.1 выключено по умолчанию; исправления CVE-2026-55225/55226) | `documentation/modules/configuring/con-configuring-topic-operator.adoc`; `CHANGELOG.md` 1.0.1 | проверено |
| Документация Strimzi для internal-клиентов: «The client application must be running in the same namespace as the Kafka resource» | `documentation/modules/security/proc-configuring-internal-clients-to-trust-cluster-ca.adoc` | проверено |
| Strimzi 1.3.0 объявил PKCS #12 в Secret'ах CA и пользователей устаревшим | `CHANGELOG.md`, 1.3.0 | проверено (1.3.0 ещё не GA) |
| Strimzi 1.2.0 поддерживает Kafka 4.2.0, 4.2.1, 4.3.0, 4.3.1 (по умолчанию 4.3.1) | `kafka-versions.yaml` тега 1.2.0 | проверено |
| Образы Strimzi собираются на `registry.access.redhat.com/ubi9/ubi-minimal` | `docker-images/base/Dockerfile` тега 1.2.0 | проверено |
| Strimzi Access Operator: `KafkaAccess` `access.strimzi.io/v1alpha1`, `spec.kafka{name,namespace,listener}`, `spec.user{kind,apiGroup,name,namespace}`, `spec.secretName`; binding Secret типа `servicebinding.io/kafka` с ключами `bootstrap.servers`, `security.protocol`, `ssl.truststore.crt`, `username`, `password`, `sasl.jaas.config`, `sasl.mechanism`, `ssl.keystore.crt`, `ssl.keystore.key`; с Strimzi ≥ 1.0 нужен Access Operator ≥ 0.3.0 | `README.md`, `UPGRADING_TO_STRIMZI_1_0_0.md`, `examples/kafka-access-with-user.yaml` (strimzi/kafka-access-operator, main) | проверено |
| Операции ACL по протоколам: Produce — `Write` на topic; Fetch — `Read` на topic; JoinGroup/SyncGroup/Heartbeat/LeaveGroup/OffsetCommit/ConsumerGroupHeartbeat — `Read` на group; OffsetFetch/FindCoordinator — `Describe` на group; InitProducerId — `Write` на transactionalId или `IdempotentWrite` на cluster; AddOffsetsToTxn/TxnOffsetCommit — `Read` на group | `docs/security/authorization-and-acls.md` (apache/kafka, trunk) | проверено |
| `Read`, `Write`, `Delete`, `Alter` подразумевают `Describe`; `AlterConfigs` подразумевает `DescribeConfigs` | `metadata/src/main/java/org/apache/kafka/metadata/authorizer/StandardAuthorizerData.java` (trunk) | проверено |
| KIP-679: с Kafka 2.8 для идемпотентного продюсера достаточно `Write` на любой топик, `IdempotentWrite` устарел с 3.0 | cwiki.apache.org KIP-679 (по результату поиска) | проверено (вторичный источник) |
| PEM для key/trust store поддерживается с Kafka 2.7.0, в том числе файлом (`ssl.truststore.type=PEM`); пароль хранилища для PEM не используется, зашифрованный ключ — `ssl.key.password` | `docs/security/encryption-and-authentication-using-ssl.md` (apache/kafka, trunk) | проверено |
| SASL-механизмы клиента: `GSSAPI`, `PLAIN`, `SCRAM-SHA-256`, `SCRAM-SHA-512`, `OAUTHBEARER`; модули `PlainLoginModule`, `ScramLoginModule`, `OAuthBearerLoginModule` | `docs/security/authentication-using-sasl.md` (trunk) | проверено |
| Spring Boot: `spring.kafka.bootstrap-servers`, `.security.protocol`, `.ssl.{bundle,key-password,key-store-certificate-chain,key-store-key,key-store-location,key-store-password,key-store-type,trust-store-certificates,trust-store-location,trust-store-password,trust-store-type,protocol}`, `.properties.*`, `.consumer.group-id`, `.producer.transaction-id-prefix`, `.template.default-topic`, `.template.transaction-id-prefix`, `.streams.application-id` | `KafkaProperties.java` (spring-boot, main, модуль `spring-boot-kafka`) | проверено |
| Relaxed binding: env-имя сопоставляется в канонической форме и в legacy-форме (`-` → `_`); ключи `Map` из env — в нижнем регистре | `SystemEnvironmentPropertyMapper.java`; `external-config.adoc` («Binding Maps From Environment Variables») | проверено |
| `SPRING_KAFKA_PROPERTIES_SASL_JAAS_CONFIG` даёт ключ карты `sasl.jaas.config` (точки внутри ключа) | поведение `MapBinder` для `Map<String,String>` | **не проверено** (проверить тестом приложения в задаче 06) |
| Quarkus: `kafka.*` — общая конфигурация клиентов; топик по умолчанию = имя канала; `group.id` по умолчанию = `quarkus.application.name`; `transactional.id` по умолчанию `${quarkus.application.name}-${channelName}`; TLS через `kafka.tls-configuration-name` | `docs/src/main/asciidoc/kafka.adoc` (quarkusio/quarkus, main) | проверено |
| Spring Cloud Stream: `spring.cloud.stream.kafka.binder.brokers`, `bindings.<b>.destination`/`.group`, имена `<function>-in-<n>`/`-out-<n>` | docs.spring.io (по результату поиска) | проверено частично |
| Deckhouse: модуль `managed-kafka` (EE, Preview), объекты `Kafka` и `KafkaClass` | deckhouse.io/modules/managed-kafka (по результату поиска; страница заблокирована) | проверено частично; API CR — **не проверено** |
| Ограничения Astra Linux для Strimzi/Kafka-клиентов | — | **не проверено** (сведений не найдено; профиль среды — EPIC-11) |
