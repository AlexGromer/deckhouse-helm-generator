# EPIC-03 / 01: типы соединений, логических узлов и топологии

| Поле | Значение |
|---|---|
| Статус | ready |
| Размер | S |
| Зависит от | — |
| Требования | R1, R6, R19 (контракт для всех остальных задач) |

## Контекст

`types.ResourceGraph` (`pkg/types/relationship.go`) хранит только объекты входа и структурные связи (`Relationships`). Новые соединения должны храниться отдельно (design §1.3): `generator.GroupResources` (проход 2), `DefaultAnalyzer.groupResources`, `DetectCircularDependencies`, `buildCrossNamespaceIndex`, `AnalyzeDecomposition` читают `Relationships` и изменили бы результат. Задача добавляет только типы и методы — контракт design §3.1–3.6. Поведение dhg не меняется.

## Изменения

| Файл | Что сделать |
|---|---|
| `pkg/types/connection.go` (новый) | константы `RelationServiceCall`, `RelationExternalCall`, `RelationTopicProduce`, `RelationTopicConsume`, `RelationTopicUse`; метод `RelationshipType.IsConnection`; `Confidence` (+ `AtLeast`), `NodeKind`, `NodeID`, `LogicalNode`, `ConnectionTarget` (+ `String`), `Connection`, `EvidenceSource` (8 констант), `Evidence`; `MembershipSource`, `PBC`, `Topology` (+ `PBCOf`, `PBC`, `Unassigned`) — дословно по design §3 |
| `pkg/types/relationship.go` | поля `Connections`, `Nodes`, `Topology` и неэкспортируемые индексы `connFrom`, `connTo`, `connKey` в `ResourceGraph`; инициализация в `NewResourceGraph`; методы `AddConnection`, `AddNode`, `Node`, `ConnectionsFrom`, `ConnectionsTo` |
| `pkg/types/connection_test.go` (новый) | unit-тесты |

## Шаги

1. Создать `connection.go` с типами и godoc-комментариями из design §3 (комментарии на английском, как в остальном коде).
2. `Confidence.AtLeast`: порядок `high(3) > medium(2) > low(1)`, пустое и неизвестное значение = `low`.
3. `ConnectionTarget.String()`: `Resource.String()`, если `Resource != nil`, иначе `string(Node)`.
4. Ключ слияния `AddConnection`: `From.String() | Container | Type | To.String() | Protocol | Port | Path` (разделитель `\x00`). Новое соединение: `Evidence` сортируется по `(Object.String(), Field, Key)`, дубликаты (все поля равны) удаляются, `Attributes` копируется. Существующее: доказательства объединяются, сортируются, дубликаты удаляются; `Confidence` = большая; `TLS` = логическое ИЛИ; `TargetPort` — непустое из двух (первое непустое); `Attributes` — объединение, при конфликте значение первого и не перезаписывается.
5. Ленивая инициализация: каждый метод создаёт nil-карты, поэтому `(&types.ResourceGraph{}).AddConnection(...)` работает.
6. `ConnectionsFrom`/`ConnectionsTo` возвращают **копии** в порядке добавления (изменение результата не меняет граф).
7. `AddNode`: если ID уже есть — вернуть существующий узел без изменений; иначе сохранить копию и вернуть указатель на неё.
8. `Topology.PBCOf` на nil-получателе возвращает `""`; `Unassigned` — имена из `ServicePBC` со значением `""`, сортировка.

## Тесты

- Unit (`pkg/types/connection_test.go`):
  - `IsConnection`: true для 5 новых типов, false для всех 19 существующих констант;
  - `AtLeast`: таблица 3×3 + пустое значение;
  - `AddConnection`: два соединения с одинаковым ключом и разными `Evidence`/`Confidence` → одно, 2 доказательства, `high`; разный `Port` → два; `ConnectionsFrom`/`ConnectionsTo` после слияния возвращают по одному элементу; изменение возвращённого среза не влияет на `g.Connections`;
  - нулевой `ResourceGraph{}`: `AddConnection`, `AddNode`, `Node` без паники;
  - `AddNode` идемпотентен; `Node` находит;
  - `Topology`: `PBCOf` для известного/неизвестного ключа и nil; `Unassigned` сортирован.
- Golden: не требуется (вывод не меняется).

## Критерии приёмки

- [ ] Имена типов, констант, полей и методов совпадают с design §3 (контракт для EPIC-02/04/08/09).
- [ ] Существующие тесты проходят без изменений; `deadcode -test ./...` не показывает новых недостижимых функций (методы покрыты тестами).
- [ ] Покрытие `pkg/types` новым кодом ≥ 86 %.
- [ ] Проверки DoD из docs/roadmap/README.md проходят.

## Вне задачи

Заполнение `Connections` — задачи 05–07; `Topology` — задача 09; рендер — 12–14.
