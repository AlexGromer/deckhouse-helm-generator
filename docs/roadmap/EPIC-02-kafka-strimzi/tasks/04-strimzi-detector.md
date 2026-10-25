# EPIC-02 / 04: Детектор связей Strimzi-объектов

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | S (≤ 1 дня) |
| Зависит от | EPIC-02/03 |
| Требования | R9 |

## Контекст

Связи строят детекторы `pkg/analyzer/detector` (интерфейс `analyzer.Detector`: `Detect(ctx, resource, allResources) []types.Relationship`). Регистрация — `pkg/analyzer/detector/registry.go:RegisterAll`. Strimzi-объекты сейчас ни с чем не связаны:
- `dhg graph` не показывает, какой workload пользуется каким `KafkaUser`;
- в separate-режиме `KafkaUser` без меток не группируется с приложением (`analyzer.groupResources` группирует по `ServiceName`, вторым проходом — по связям).

Связи между объектами Kubernetes не требуют контрактов EPIC-03: это существующий тип `types.RelationNameReference`.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/analyzer/detector/strimzi.go` (новый) | `type StrimziDetector struct{ priority int }`, `NewStrimziDetector()` (приоритет 85), `Name() = "strimzi"`, `Priority()`, `Detect(...)`: (1) для `KafkaUser` (группа `kafka.strimzi.io`): ACL `resource.type == "topic"` с `patternType` `literal` или пустым — связь на `KafkaTopic` того же namespace, у которого `spec.topicName == resource.name`, а если `spec.topicName` пуст — `metadata.name == resource.name`; `Field = "spec.authorization.acls"`. (2) `KafkaUser` → Secret из `spec.authentication.password.valueFrom.secretKeyRef.name`, если Secret во входе; `Field = "spec.authentication.password"`. (3) `KafkaTopic`/`KafkaUser` → `Kafka` (`kafka.strimzi.io`, любая версия) с именем из метки `strimzi.io/cluster` в том же namespace; `Field = "metadata.labels[strimzi.io/cluster]"`. (4) workload (`Deployment`, `StatefulSet`, `DaemonSet`, `Job`, `CronJob`) → `KafkaUser` того же namespace, если pod spec ссылается на Secret с именем `KafkaUser`: `env[].valueFrom.secretKeyRef.name`, `envFrom[].secretRef.name`, `volumes[].secret.secretName`, `volumes[].projected.sources[].secret.name`; `Field` — путь ссылки. Во всех связях `Type = types.RelationNameReference`, `Details = {"strimzi": "true"}` |
| `pkg/analyzer/detector/registry.go` | `a.AddDetector(NewStrimziDetector())` |
| `tests/integration/strimzi_graph_test.go` (новый) | Интеграционный тест графа (ниже) |

Сравнение namespace: пустой namespace считается равным пустому. Поиск идёт по `allResources`, ключ — `types.ResourceKey` (namespace + GVK + имя). Группа и kind сравниваются по `res.Original.GVK`, версия не важна. Pod spec извлекается так же, как в `pkg/generator/features_security.go:secPodSpec`. Логику не копировать, а вынести в `pkg/analyzer/detector` локальную функцию `podSpecOf(kind string, obj map[string]interface{}) map[string]interface{}`: пакет `generator` нельзя импортировать из `analyzer`.

## Шаги

1. Реализовать детектор и зарегистрировать.
2. Unit-тесты на каждое правило, интеграционный тест.
3. Проверить `dhg graph -f tests/integration/fixtures/kafka-app --format mermaid` вручную.

## Тесты

- **Unit `pkg/analyzer/detector/strimzi_test.go`:**
  - `KafkaUser orders` с ACL на `orders-created` + `KafkaTopic orders-created` → одна связь `KafkaUser/orders → KafkaTopic/orders-created`;
  - ACL на `Payments.Legacy` + `KafkaTopic payments-legacy` со `spec.topicName: Payments.Legacy` → связь;
  - ACL `patternType: prefix` → нет связи;
  - ACL на топик из другого namespace → нет связи;
  - Deployment с `secretKeyRef {name: orders}` + `KafkaUser orders` → связь `Deployment/orders → KafkaUser/orders` с `Field`, содержащим `secretKeyRef`;
  - CronJob с томом `secret.secretName: orders` → связь;
  - `KafkaTopic` с меткой `strimzi.io/cluster: my-cluster` + `Kafka my-cluster` во входе → связь; без `Kafka` → нет;
  - `KafkaUser` с `password.valueFrom.secretKeyRef.name: orders-pw` + Secret `orders-pw` → связь.
- **Integration `tests/integration/strimzi_graph_test.go`:** конвейер на `fixtures/kafka-app` (тот же путь, что в `framework_test.go`) → в `graph.Relationships` есть `KafkaUser/orders → KafkaTopic/orders-created` и `Deployment/orders → KafkaUser/orders` (AC2).
- **Golden:** изменений ожиданий нет. Фикстура `kafka-app` проходит.

## Критерии приёмки

- [ ] Четыре правила R9 строят связи типа `name_reference` с `Details["strimzi"] = "true"`.
- [ ] `dhg graph` на фикстуре показывает обе связи из AC2.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

- Связи «workload публикует/потребляет топик» по фактам env (задача 07, типы EPIC-03).
- Связи по ACL с `patternType: prefix`: топики-кандидаты можно перечислить, но это не ссылка по имени.
