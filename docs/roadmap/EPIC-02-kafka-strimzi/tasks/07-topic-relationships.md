# EPIC-02 / 07: Связи «публикует/потребляет топик» по фактам клиента

| Поле | Значение |
|---|---|
| Статус | draft |
| Размер | M (2–3 дня) |
| Зависит от | EPIC-02/05; EPIC-03: контракт «типы связей публикации/потребления топика и логические узлы», контракт «источник конфигурации workload'а», контракт «парсер адресов» (bootstrap-списки) |
| Требования | R15 |

**Открытый вопрос (блокирует `ready`):** окончательные имена и форма контрактов EPIC-03:
- константы `types.RelationshipType`;
- тип логического узла топика и его `ResourceKey`;
- API источника конфигурации;
- функция разбора bootstrap-списка.

Ниже они названы рабочими именами в угловых скобках. После публикации `docs/roadmap/EPIC-03-pbc-product/design.md` задачу нужно переписать на точные имена и перевести в `ready`.

## Контекст

`pkg/kafkaclient.Extract` (задача 05) даёт факты одного контейнера. Чтобы features `strimzi` (задача 08) и EPIC-04 (`CiliumNetworkPolicy`) работали с графом, факты нужно превратить в связи:
- «workload → топик (produce)»;
- «workload → топик (consume)».

Топика как объекта Kubernetes может не быть, поэтому нужен логический узел (контракт EPIC-03). Временный адаптер `kafkaclient.FromContainer` заменяется источником конфигурации EPIC-03, который покрывает и манифесты, и `pkg/synth`.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/analyzer/detector/kafkatopic.go` (новый) | `KafkaTopicDetector`: для каждого workload'а и контейнера — свойства из `<источник конфигурации EPIC-03>` → `kafkaclient.Extract` → для каждого `Topic` связь `<RelationTopicProduce>`/`<RelationTopicConsume>` (unknown → обе? — см. вопрос 2) на `<узел топика>` с идентичностью брокера: `strimzi.io/cluster` связанного `KafkaUser` (если связь задачи 04 есть) или нормализованный bootstrap-список из `<парсер адресов EPIC-03>`. `Details`: `topic`, `group` (для consume — первая группа-факт контейнера, иначе пусто), `transactionalId`, `certainty`, `origin` |
| `pkg/analyzer/detector/registry.go` | регистрация |
| `pkg/kafkaclient/container.go` | `FromContainer` удалить или пометить устаревшей; задача 06 переходит на источник EPIC-03 |
| `pkg/generator/kafkaclient.go` | заменить вызов `FromContainer` на источник EPIC-03 |

## Тесты (план)

- Unit: Deployment `orders` с `SPRING_KAFKA_TEMPLATE_DEFAULT_TOPIC=orders-created` и Deployment `billing` с `SPRING_CLOUD_STREAM_BINDINGS_PROCESS_IN_0_DESTINATION=orders-created`, `…_GROUP=billing`, оба с одним bootstrap → две связи на **один** узел топика `orders-created`: produce от `orders`, consume от `billing` с `Details["group"] = "billing"`.
- Unit: тот же топик у двух разных bootstrap-списков → два разных узла.
- Integration: `dhg graph` на `fixtures/kafka-app` показывает узел топика и связи produce/consume.

## Открытые вопросы

1. Имена контрактов EPIC-03 (см. выше).
2. Направление `unknown` (Spring Cloud Stream с legacy-именами binding'ов, догадки): одна связь с `Details["direction"] = "unknown"` или отсутствие связи. Рекомендация: связь с `certainty: guess` и направлением `unknown`; feature `strimzi` выдаёт по ней выключенные ACL (`Read`, `Write`, `Describe`).

## Критерии приёмки (черновик)

- [ ] Производитель и потребитель одного топика одного брокера связаны через один логический узел.
- [ ] `kafkaclient.FromContainer` больше не используется.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

- Генерация `KafkaTopic`/`KafkaUser` (задача 08).
