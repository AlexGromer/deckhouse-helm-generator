# EPIC-02 / 06: Feature `kafka-client` — безопасность клиента внешнего Kafka

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | M (2–3 дня) |
| Зависит от | EPIC-02/01, EPIC-02/02, EPIC-02/05 |
| Требования | R12, R13, R14 |

## Контекст

Чтобы приложение подключилось к Kafka по `SASL_SSL`, ему нужны:
- `security.protocol` и `sasl.mechanism`;
- `sasl.jaas.config` из Secret'а;
- доверенный CA брокеров (truststore, хранилище доверенных сертификатов);
- для mTLS — клиентский сертификат.

Имена переменных окружения зависят от фреймворка (spec.md §4.3). dhg сейчас ничего из этого не генерирует.

Опоры:
- факты клиента — `pkg/kafkaclient` (задача 05);
- подключение к workload'у — инъекция (задача 02);
- заметки — `fc.note` (задача 01);
- образец feature с обнаружением по env и values — `istio-egress` (`pkg/generator/egresspolicies.go:applyIstioEgressFeature`);
- регистрация — `pkg/generator/features_security.go:init`.

Формат Secret'ов совместим со Strimzi:
- Secret `KafkaUser` — ключи `sasl.jaas.config`, `password`, `user.crt`, `user.key`, `user.p12`, `user.password`;
- CA кластера — `<cluster>-cluster-ca-cert` с ключами `ca.crt`, `ca.p12`, `ca.password`.

Значения по умолчанию в таблицах ниже совпадают с этими ключами.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/generator/features_kafka.go` (новый) | `init()`: `RegisterFeature(Feature{Name: "kafka-client", Description: "Configure Kafka client security (security.protocol, SASL, truststore/keystore from existing Secrets) for workloads that use Kafka", Params: …, Apply: applyKafkaClientFeature})`. Параметры и умолчания — таблица ниже |
| `pkg/generator/kafkaclient.go` (новый) | `applyKafkaClientFeature(chart, fc)`; `kafkaClientWorkloads(chart, graph, params) []kafkaWorkload` (workload, контейнер, фреймворк); `kafkaClientEnv(framework, params, inputEnv map[string]bool) ([]interface{}, []string /*skipped*/)`; `kafkaClientVolumes(params)`. Значения env — строки с `tpl`-ссылками на `.Values.kafkaClient.*` (design.md §4.2, §4.4) |
| `pkg/generator/kafkaclient_test.go` (новый) | Unit-тесты (ниже) |
| `tests/golden/kafka_test.go` | `TestKafkaClientFeature` (ниже) |
| `README.md` | Строка в таблице «Опциональные возможности»: `kafka-client` — «Безопасность клиента Kafka (SASL/SSL, truststore/keystore из существующих Secret'ов) для workload'ов с Kafka-конфигурацией»; число возможностей «Все N возможностей» увеличить |
| `docs/RELEASE_NEXT.md` | Новая feature |

### Параметры

| Параметр | По умолчанию | Допустимо |
|---|---|---|
| `security-protocol` | `""` (не задавать) | `""`, `PLAINTEXT`, `SSL`, `SASL_PLAINTEXT`, `SASL_SSL` |
| `sasl-mechanism` | `""` | `""`, `PLAIN`, `SCRAM-SHA-256`, `SCRAM-SHA-512`, `OAUTHBEARER`, `GSSAPI` |
| `jaas-secret` | `""` | имя Secret'а; ключ — `sasl.jaas.config` (values `sasl.jaasConfigSecret.key`) |
| `truststore-secret` | `""` | имя Secret'а; ключ по умолчанию `ca.crt` |
| `truststore-type` | `PEM` | `PEM`, `PKCS12`, `JKS` |
| `keystore-secret` | `""` | имя Secret'а |
| `keystore-type` | `PEM` | `PEM`, `PKCS12`, `JKS` |
| `workloads` | `""` | `Kind/name` через запятую; пусто — workload'ы с bootstrap-фактом |
| `framework` | `auto` | `auto`, `spring`, `quarkus`, `micronaut`, `plain` |

Пустые `security-protocol` и `sasl-mechanism` по умолчанию сделаны сознательно: feature с параметрами по умолчанию (так её запускает golden `TestFeaturesPassHelm`) не меняет поведение приложения, а только выводит заметки и values для заполнения.

### Values `kafkaClient`

Добавляются, только если в chart'е есть хотя бы один клиент:

```yaml
kafkaClient:
  enabled: true
  securityProtocol: SASL_SSL
  sasl:
    mechanism: SCRAM-SHA-512
    jaasConfigSecret: {name: orders-kafka, key: sasl.jaas.config}
    oauthbearer: {tokenEndpointUrl: ""}
  tls:
    truststore: {secretName: kafka-ca, key: ca.crt, type: PEM, passwordKey: "", mountPath: /etc/kafka/truststore}
    keystore: {secretName: "", type: PEM, certKey: user.crt, keyKey: user.key, storeKey: user.p12, passwordKey: user.password, mountPath: /etc/kafka/keystore}
  brokers: []          # адреса брокеров за bootstrap — для egress (задача 09)
  workloads:
    Deployment/orders: {container: orders, framework: spring}
  inject:
    Deployment/orders: {…}
```

Для `truststore-type` `PKCS12`/`JKS` значения по умолчанию: `key: ca.p12` / `truststore.jks`, `passwordKey: ca.password` / `""`. Пустой `passwordKey` при не-PEM → `Note:`.

### Алгоритм

По design.md §4.4:

1. Нет workload'ов в chart'е → вернуть chart без изменений.
2. Для каждого workload'а `secChartWorkloads(chart, fc.Graph)` и каждого контейнера из `containers`:
   - `kafkaclient.FromContainer(container, configMaps)`, где `configMaps` — data ConfigMap'ов графа из того же namespace;
   - фреймворк: параметр `framework` или `kafkaclient.DetectFramework`;
   - `Extract`;
   - клиент — контейнер с непустым `Bootstrap` или workload из параметра `workloads`. Если в `workloads` указан workload, которого нет в графе, — ошибка `unknown workload "<Kind/name>" in workloads (chart <name>)`.
3. Env по spec.md §4.3. Условия:
   - `security.protocol` — если параметр не пуст;
   - `sasl.mechanism` и JAAS — только при `SASL_*`, JAAS — только при заданном `jaas-secret`;
   - truststore — при `SSL`/`SASL_SSL` и заданном `truststore-secret`;
   - keystore — при `SSL`/`SASL_SSL` и заданном `keystore-secret`;
   - PEM-keystore — по значению (env из `secretKeyRef` на `certKey`, `keyKey`), без тома;
   - PKCS12/JKS — файлом (том `dhg-kafka-keystore`, `items: [{key: storeKey, path: storeKey}]`) и паролем из `passwordKey`.
4. Имена томов — `dhg-kafka-truststore`, `dhg-kafka-keystore`; монтирования `readOnly: true`.
5. Переменная, которая уже есть в `env` контейнера входа, не добавляется. Выводится заметка `"<Kind>/<name>: env <NAME> is set by the input; kafka-client keeps it"`.
6. Заметки:
   - фреймворк `plain`: `"<Kind>/<name>: Kafka client framework unknown; set security.protocol, sasl.mechanism, sasl.jaas.config, ssl.truststore.* in the application configuration"`;
   - `SASL_*` без `jaas-secret`: `"kafka-client: no jaas-secret; set kafkaClient.sasl.jaasConfigSecret.name to a Secret with key sasl.jaas.config"`;
   - `SSL`/`SASL_SSL` без `truststore-secret`: `"kafka-client: no truststore-secret; the JVM default truststore is used"`;
   - `GSSAPI`: `"kafka-client: GSSAPI needs a keytab and krb5.conf, which dhg does not generate"`;
   - `OAUTHBEARER`: `"kafka-client: set sasl.login.callback.handler.class for your Kafka client version"`;
   - для `quarkus`/`micronaut` при первом использовании: `"kafka-client: <framework> reads KAFKA_* environment variables as kafka.* properties only if …"` — точный текст после проверки (см. «Шаги» п. 1);
   - для каждого `kafkaclient.Facts.Unresolved` — значение и имена placeholder'ов.
7. Values: `secAddValues(out, "kafkaClient", "# Kafka client security (dhg feature: kafka-client). Secrets are referenced by name, never stored in values.\n", …)`; `inject` — `injectionValues` (задача 02).

## Шаги

1. **Проверка до реализации** (README §4, шаг 3). На тестовом приложении (Spring Boot 3.x с `spring-kafka`, Quarkus 3.x с `quarkus-messaging-kafka`, Micronaut 4.x с `micronaut-kafka`) проверить, что env из spec.md §4.3 доходят до клиента. Для Spring — в первую очередь `SPRING_KAFKA_PROPERTIES_SASL_JAAS_CONFIG` → ключ `sasl.jaas.config` (точки внутри ключа карты). Результат записать в spec.md §9 (статус «проверено»). Если имя не работает — исправить таблицу spec.md §4.3 до кода.
2. Реализовать параметры, их проверку, алгоритм.
3. Unit-тесты, golden.
4. README, `docs/RELEASE_NEXT.md`.

## Тесты

- **Unit `pkg/generator/kafkaclient_test.go`** (входы через `secTestCharts` из `features_security_test.go`, манифесты — строки в тесте):
  - `TestKafkaClientSpringSaslSsl`: Deployment `orders` (env `SPRING_KAFKA_BOOTSTRAP_SERVERS=k:9093`) + параметры `security-protocol=SASL_SSL`, `sasl-mechanism=SCRAM-SHA-512`, `jaas-secret=orders-kafka`, `truststore-secret=kafka-ca`. В values `kafkaClient.inject["Deployment/orders"].containers.orders.env` — по порядку `SPRING_KAFKA_SECURITY_PROTOCOL`, `SPRING_KAFKA_PROPERTIES_SASL_MECHANISM`, `SPRING_KAFKA_PROPERTIES_SASL_JAAS_CONFIG` (`valueFrom.secretKeyRef.name` = `{{ .Values.kafkaClient.sasl.jaasConfigSecret.name }}`), `SPRING_KAFKA_SSL_TRUST_STORE_TYPE`, `SPRING_KAFKA_SSL_TRUST_STORE_LOCATION`; том `dhg-kafka-truststore`;
  - `TestKafkaClientDefaultsNoop`: параметры по умолчанию → `inject` без env, есть заметки, chart проходит `assertTemplateParses`;
  - `TestKafkaClientQuarkusNames`: контейнер с `MP_MESSAGING_INCOMING_PRICES_CONNECTOR=smallrye-kafka`, `KAFKA_BOOTSTRAP_SERVERS=k:9092` → env `KAFKA_SECURITY_PROTOCOL`, `KAFKA_SASL_MECHANISM`, `KAFKA_SASL_JAAS_CONFIG`;
  - `TestKafkaClientPlainNote`: `KAFKA_BROKERS=k:9092`, без признаков фреймворка → env нет, заметка `framework unknown`;
  - `TestKafkaClientKeepsInputEnv`: вход уже содержит `SPRING_KAFKA_SECURITY_PROTOCOL=SSL` → feature его не добавляет, заметка `is set by the input`;
  - `TestKafkaClientPKCS12Keystore`: `keystore-secret=orders`, `keystore-type=PKCS12` → том `dhg-kafka-keystore` с `items: [{key: user.p12, path: user.p12}]`, env `SPRING_KAFKA_SSL_KEY_STORE_TYPE=PKCS12`, `…_LOCATION=file:/etc/kafka/keystore/user.p12`, `…_PASSWORD` из `secretKeyRef user.password`;
  - `TestKafkaClientInvalidParams`: `security-protocol=TLS`, `sasl-mechanism=SCRAM`, `truststore-type=P12`, `workloads=Deployment/missing` → ошибки с перечислением допустимых значений;
  - `TestKafkaClientNoClients`: вход без Kafka → chart не изменён (`reflect.DeepEqual` values и шаблонов).
- **Golden `tests/golden/kafka_test.go` `TestKafkaClientFeature`:**
  - `dhg generate -s source -f tests/golden/testdata/synth/orders-service --chart-name app --with kafka-client --feature-opt kafka-client.security-protocol=SASL_SSL --feature-opt kafka-client.sasl-mechanism=SCRAM-SHA-512 --feature-opt kafka-client.jaas-secret=orders-kafka --feature-opt kafka-client.truststore-secret=kafka-ca` во всех режимах (`synthesize` из `synth_test.go`);
  - у Deployment `orders` env `SPRING_KAFKA_SECURITY_PROTOCOL=SASL_SSL`, `SPRING_KAFKA_PROPERTIES_SASL_JAAS_CONFIG` с `secretKeyRef {name: orders-kafka, key: sasl.jaas.config}`, `SPRING_KAFKA_SSL_TRUST_STORE_LOCATION=file:/etc/kafka/truststore/ca.crt`, том `dhg-kafka-truststore` из Secret `kafka-ca` и монтирование `/etc/kafka/truststore` (AC5);
  - повторный рендер с `--set kafkaClient.sasl.jaasConfigSecret.name=other` → `secretKeyRef.name: other` (`tpl` работает).
- **Golden `TestFeaturesPassHelm`:** `kafka-client` подхватывается автоматически, все входы зелёные.

## Критерии приёмки

- [ ] Для Spring, Quarkus и Micronaut инъекция даёт имена env из spec.md §4.3, проверенные на тестовых приложениях (шаг 1).
- [ ] Chart не содержит учётных данных: только имена Secret'ов и ключей.
- [ ] С параметрами по умолчанию feature не меняет env workload'ов.
- [ ] AC5 эпика выполнен.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

- Egress к брокерам (задача 09).
- Генерация `KafkaUser` и `KafkaAccess` (задача 08).
- Keytab и `krb5.conf` для `GSSAPI`.
- Обнаружение клиентов через источник конфигурации EPIC-03 (задача 07 заменит `kafkaclient.FromContainer`).
