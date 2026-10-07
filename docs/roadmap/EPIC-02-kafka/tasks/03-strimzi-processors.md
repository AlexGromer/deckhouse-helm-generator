# EPIC-02 / 03: Процессоры `KafkaTopic` и `KafkaUser`

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | M (2–3 дня) |
| Зависит от | EPIC-02/01 |
| Требования | R3, R4, R5, R6, R7, R8 |

## Контекст

`KafkaTopic` и `KafkaUser` (группа `kafka.strimzi.io`) сейчас обрабатывает `Registry.processGeneric` (`pkg/processor/registry.go`):
- путь values — `services.<svc>.kafkaTopic` (`kindToValuesKey`), поэтому два топика одного сервиса делят путь и разводятся `ResolveCollisions` по разным сервисам;
- метки входа, включая `strimzi.io/cluster`, пишутся литералами;
- namespace — namespace релиза без возможности переопределения.

Strimzi 1.0.0 удалил `v1beta2` из CRD `KafkaTopic`/`KafkaUser`: единственная обслуживаемая версия — `v1` (`packaging/install/cluster-operator/043-Crd-kafkatopic.yaml`, `044-Crd-kafkauser.yaml`). Кластеры на Strimzi 0.49–0.51 обслуживают и `v1`, и `v1beta2`. Topic/User Operator следит только за namespace кластера Kafka, если Cluster Operator не запущен с `STRIMZI_ENTITY_OPERATOR_WATCHED_NAMESPACE_ENABLED=true` (с 1.0.1).

Образец процессора с полным `spec` — `pkg/processor/k8s/servicemonitor.go` (`processor.SpecOverlay`). Образец values по имени объекта — `pkg/processor/k8s/configmap.go` (`services.<svc>.configMaps.<sanitizeName(name)>`).

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/processor/k8s/strimzi.go` (новый) | `KafkaTopicProcessor`, `KafkaUserProcessor` и общая `processStrimziEntity` (design.md §2.3, §4.1). GVK: `{Group: "kafka.strimzi.io", Version: "v1", Kind: "KafkaTopic"}`, то же с `Version: "v1beta2"`; аналогично `KafkaUser`. Приоритет 70. `ServiceName` — `processor.ServiceNameFromResource(obj)`. Values: `enabled: true`, `cluster: <метка или "">`, `namespace: ""`, `spec: <копия spec>` (`runtime.DeepCopyJSONValue`), `annotations` — только если у входа есть аннотации. `ValuesPath`: `services.<svc>.kafkaTopics.<sanitizeName(name)>` / `services.<svc>.kafkaUsers.<sanitizeName(name)>`. `TemplatePath`: `templates/<svc>-kafkatopic-<name>.yaml` / `templates/<svc>-kafkauser-<name>.yaml`. Overlay-ключи: `topicName`, `partitions`, `replicas`, `config` / `authentication`, `authorization`, `quotas`, `template`. `Dependencies` у `KafkaUser`: Secret из `spec.authentication.password.valueFrom.secretKeyRef.name` (GVK `{Version: "v1", Kind: "Secret"}`, namespace объекта) |
| `pkg/processor/k8s/strimzi.go` | Заметки (`Result.Notes`, задача 01), тексты дословно: (1) `v1beta2`: `"<Kind> <ns>/<name> uses kafka.strimzi.io/v1beta2, removed in Strimzi 1.0.0; convert it to v1 (Strimzi v1 API Conversion Tool)"`; (2) ACL с ключом `operation`: `"KafkaUser <ns>/<name>: spec.authorization.acls[].operation was removed in kafka.strimzi.io/v1; use operations"`; (3) нет метки: `"<Kind> <ns>/<name> has no strimzi.io/cluster label; set <ValuesPath>.cluster, the Strimzi operator ignores objects without it"`; (4) namespace входа отличается от namespace хотя бы одного workload'а входа (`ctx.AllResources`, kind'ы Deployment, StatefulSet, DaemonSet, Job, CronJob): `"<Kind> <ns>/<name> is rendered into the release namespace; the Topic/User Operator watches only its own namespace (by default the Kafka cluster's): set <ValuesPath>.namespace"` |
| `pkg/processor/k8s/registry.go` | В `RegisterAll` раздел `// Strimzi`: `r.Register(NewKafkaTopicProcessor())`, `r.Register(NewKafkaUserProcessor())` |
| `tests/integration/fixtures/kafka-app/` (новый) | Фикстура (ниже) |
| `README.md` | Раздел о поддерживаемых CR: строка «Strimzi `KafkaTopic`, `KafkaUser` (`kafka.strimzi.io/v1`, `v1beta2`): кластер и namespace — в values» |
| `docs/RELEASE_NEXT.md` | Новые процессоры; изменение путей values для этих kind'ов (design.md §7) |

### Шаблон

Ровно по design.md §4.1:
- метки входа без `strimzi.io/cluster`, отсортированные по ключу, — литеральный `dict` (`strconv.Quote` для ключей и значений);
- `with .cluster` добавляет метку;
- `merge $dhgLabels (include "<chart>.labels" $ | fromYaml)` — метки входа побеждают;
- `namespace: {{ .namespace | default $.Release.Namespace }}`;
- `apiVersion` — из входа;
- `metadata.name` — `processor.ObjectName(name)`;
- тело — `processor.SpecOverlay(".", <overlay-ключи>)`;
- если у входа нет `spec`, строки `spec:` нет (тогда values не содержат `spec`, а `SpecOverlay` не вызывается).

Блок меток не должен совпадать с `metadataLabelsBlock` из `pkg/processor/registry.go`: `Registry.Process` вызывает `PreserveObjectLabels`, и при совпадении метка кластера снова станет литералом.

### Фикстура `tests/integration/fixtures/kafka-app/`

`deployment.yaml` — Deployment `orders` в namespace `shop`:
- метки и селектор `app.kubernetes.io/name: orders`;
- контейнер `orders`, образ `registry.example.com/shop/orders:1.4.2`;
- env:
  - `SPRING_KAFKA_BOOTSTRAP_SERVERS=my-cluster-kafka-bootstrap.kafka:9093`;
  - `SPRING_KAFKA_SECURITY_PROTOCOL=SASL_SSL`;
  - `SPRING_KAFKA_PROPERTIES_SASL_MECHANISM=SCRAM-SHA-512`;
  - `SPRING_KAFKA_TEMPLATE_DEFAULT_TOPIC=orders-created`;
  - `SPRING_KAFKA_CONSUMER_GROUP_ID=orders`;
  - `SPRING_KAFKA_PROPERTIES_SASL_JAAS_CONFIG` из `secretKeyRef {name: orders, key: sasl.jaas.config}`.

`topics.yaml`:

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
    cleanup.policy: delete
---
apiVersion: kafka.strimzi.io/v1beta2
kind: KafkaTopic
metadata:
  name: payments-legacy
  namespace: kafka
  labels:
    strimzi.io/cluster: my-cluster
spec:
  topicName: Payments.Legacy
  partitions: 3
  replicas: 3
```

`users.yaml`:

```yaml
apiVersion: kafka.strimzi.io/v1
kind: KafkaUser
metadata:
  name: orders
  namespace: kafka
  labels:
    strimzi.io/cluster: my-cluster
    app.kubernetes.io/name: orders
spec:
  authentication:
    type: scram-sha-512
  authorization:
    type: simple
    acls:
      - resource: {type: topic, name: orders-created, patternType: literal}
        operations: [Write, Describe]
      - resource: {type: group, name: orders, patternType: literal}
        operations: [Read]
  quotas:
    producerByteRate: 1048576
```

## Шаги

1. Создать фикстуру и убедиться, что текущий `main` проходит на ней golden (базовая линия generic-процессора).
2. Реализовать процессоры, зарегистрировать.
3. Unit-тесты, затем golden.
4. README, `docs/RELEASE_NEXT.md`.

## Тесты

- **Unit `pkg/processor/k8s/strimzi_test.go`:**
  - `TestKafkaTopicProcessor_V1`: объект `orders-created` выше. `ValuesPath` = `services.orders.kafkaTopics.ordersCreated`, `TemplatePath` = `templates/orders-kafkatopic-orders-created.yaml`, `Values["cluster"] = "my-cluster"`, `Values["spec"]["partitions"] = int64(6)`, шаблон содержит `apiVersion: kafka.strimzi.io/v1`, `{{- with .cluster }}`, `"app.kubernetes.io/name" "orders"`, не содержит литерала `strimzi.io/cluster: my-cluster`, `Notes` пуст;
  - `TestKafkaTopicProcessor_V1beta2Note`: `payments-legacy` → одна заметка с `removed in Strimzi 1.0.0`, `ServiceName` = `payments-legacy`;
  - `TestKafkaTopicProcessor_NoClusterLabel`: без метки → `Values["cluster"] = ""` и заметка с `has no strimzi.io/cluster label`;
  - `TestKafkaUserProcessor_OperationNote`: `v1beta2` `KafkaUser` с `acls: [{resource: {type: topic, name: t}, operation: Read}]` → две заметки (v1beta2 и `operation`);
  - `TestKafkaUserProcessor_PasswordSecretDependency`: `authentication.password.valueFrom.secretKeyRef.name: orders-pw` → `Dependencies` содержит Secret `kafka/orders-pw`;
  - `TestStrimziTemplateNotRewrittenByPreserveObjectLabels`: `processor.PreserveObjectLabels(tpl, labels)` возвращает `tpl` без изменений;
  - `TestStrimziNamespaceNote`: `ctx.AllResources` с Deployment в `shop`, объект в `kafka` → заметка `watches only its own namespace`; без Deployment — нет заметки;
  - шаблоны разбираются `text/template/parse` с `parse.SkipFuncCheck`.
- **Golden:**
  - фикстура `kafka-app` автоматически проходит все сценарии `TestGeneratedChartsPassHelm`, включая fidelity в режимах universal/separate/library/umbrella;
  - новый `tests/golden/kafka_test.go` `TestStrimziEntities`: `dhg generate -f tests/integration/fixtures/kafka-app --chart-name app --mode universal`. Затем `helm template golden <chart> --namespace shop`: `KafkaTopic/orders-created` имеет `metadata.namespace: shop`, `metadata.labels["strimzi.io/cluster"] = my-cluster`, `spec.partitions = 6`, `spec.config["retention.ms"] = 604800000`. Повторный рендер с `--set services.orders.kafkaTopics.ordersCreated.cluster=prod --set services.orders.kafkaTopics.ordersCreated.namespace=kafka` → метка `prod`, namespace `kafka`;
  - stderr `dhg generate` содержит заметку `uses kafka.strimzi.io/v1beta2` для `payments-legacy` ровно один раз (AC3).

## Критерии приёмки

- [ ] `KafkaTopic` и `KafkaUser` версий `v1` и `v1beta2` рендерятся типизированными процессорами с полным `spec`.
- [ ] Метка `strimzi.io/cluster` и namespace переопределяются из values; остальные метки входа сохранены.
- [ ] Заметки R5, R6, R8 выводятся по условиям выше.
- [ ] Фикстура `kafka-app` проходит весь golden-набор.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

- Связи (задача 04).
- `kafka.strimzi.io/v1alpha1` и другие kind'ы Strimzi (`Kafka`, `KafkaNodePool`, `KafkaConnect`, …) остаются у generic fallback.
- Конвертация `v1beta2` → `v1`.
