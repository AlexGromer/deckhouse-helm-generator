# EPIC-02: дизайн

## 1. Обзор архитектуры

```
extract ──► process ─────────────────────────► analyze ──────────────────────────► generate ──► features (--with) ──► write
             │ KafkaTopicProcessor (03)          │ StrimziDetector (04)                          │ kafka-client (06)
             │ KafkaUserProcessor  (03)          │ KafkaTopicDetector (07, EPIC-03 типы)          │ strimzi (08)
             │ Result.Notes (01)                 │   └─ pkg/kafkaclient.Extract (05)              │ istio-egress (09)
             ▼                                   ▼                                                ▼
        ProcessedResource.Notes ──────────► Note: / SYNTHESIS.md ◄──────────── FeatureContext.AddNote (01)
                                                                                 │
                       podTemplate (pkg/processor/k8s/podtemplate.go) ◄── <key>.inject в values (02)
```

- **Процессоры** (этап 2) превращают `KafkaTopic`/`KafkaUser` из входа в шаблоны с `SpecOverlay`. Метку кластера и namespace они выносят в values.
- **Детекторы** (этап 3) строят связи. `StrimziDetector` работает с объектами Kubernetes и использует существующий тип `name_reference`. Детектор топиков (задача 07) использует новые типы EPIC-03 и логические узлы топиков.
- **`pkg/kafkaclient`** — чистые функции без зависимостей от конвейера: каталог ключей (spec.md §3.2) и извлечение фактов из плоского набора ключей.
- **Features** (этап 4) читают граф (`FeatureContext.Graph`) и факты. Они добавляют свои шаблоны и values, а в workload'ы попадают только через инъекцию (задача 02).
- **`Note:`** из процессоров и features собираются CLI. При синтетическом источнике они дописываются в `SYNTHESIS.md`.

## 2. Компоненты и изменения

### 2.1 Канал заметок (задача 01)

| Файл | Изменение |
|---|---|
| `pkg/processor/processor.go` | `Result.Notes []string` — заметки процессора об этом объекте |
| `pkg/types/resource.go` | `ProcessedResource.Notes []string` |
| `cmd/dhg/pipeline.go:runPipeline` | копирует `result.Notes` в `ProcessedResource`; после `ResolveCollisions` печатает заметки всех ресурсов (`Note: %s`) без повторов; `pipelineResult.notes []string` хранит напечатанные |
| `pkg/generator/features.go` | `FeatureContext.AddNote func(format string, args ...interface{})`; новая `ApplyFeaturesWithNotes(charts, names, options, graph) ([]*types.GeneratedChart, []string, error)` возвращает уникальные заметки в порядке появления; `ApplyFeatures` сохраняет сигнатуру и вызывает её, отбрасывая заметки (25 вызовов в тестах не меняются) |
| `cmd/dhg/main.go` | печатает заметки features; `writeSynthesisReport` получает `extra []string` (заметки процессоров и features) и дописывает их к `p.synthesis.Notes()` |

`FeatureContext.AddNote` никогда не `nil`: `ApplyFeatures` всегда его задаёт. Тесты, которые строят `FeatureContext` вручную, используют метод `fc.note(...)`, безопасный при `nil`.

### 2.2 Инъекция в workload'ы (задача 02)

| Файл | Изменение |
|---|---|
| `pkg/processor/k8s/podtemplate.go` | `podTemplate(chartName string, indent int, restartPolicy, workloadKey string)`; в начале вывода — блок вычисления `$dhgInjVolumes`/`$dhgInjContainers` (§4.2); `volumes` пода — `concat (.volumes \| default list) $dhgInjVolumes`; в `containerTemplate` — `$dhgInj` и `concat` для `env`, `envFrom`, `volumeMounts` |
| `pkg/processor/k8s/deployment.go`, `common.go` (StatefulSet, DaemonSet), `job.go`, `cronjob.go` | передают `workloadKey = "<Kind>/<metadata.name>"` |
| `pkg/generator/inject.go` (новый) | `type Injection struct{ Volumes []interface{}; Containers map[string]ContainerInjection }`, `type ContainerInjection struct{ Env, EnvFrom, VolumeMounts []interface{} }`, `func injectionValues(inj map[string]Injection) map[string]interface{}` — сериализует в форму values (spec.md §4.2); `func workloadKey(w secWorkload) string` |

### 2.3 Процессоры Strimzi (задача 03)

`pkg/processor/k8s/strimzi.go`:

```go
// KafkaTopicProcessor processes Strimzi KafkaTopic resources.
type KafkaTopicProcessor struct{ processor.BaseProcessor }
func NewKafkaTopicProcessor() *KafkaTopicProcessor // name "kafkatopic", priority 70,
	// GVKs kafka.strimzi.io/v1 KafkaTopic, kafka.strimzi.io/v1beta2 KafkaTopic
func (p *KafkaTopicProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error)

// KafkaUserProcessor processes Strimzi KafkaUser resources.
type KafkaUserProcessor struct{ processor.BaseProcessor }
func NewKafkaUserProcessor() *KafkaUserProcessor   // name "kafkauser", priority 70, те же версии

// strimziEntity is the shared implementation: values, template and notes.
func processStrimziEntity(ctx processor.Context, obj *unstructured.Unstructured,
	valuesKey, fileKind string, overlayKeys []string) (*processor.Result, error)
```

`RegisterAll` (`pkg/processor/k8s/registry.go`) получает раздел `// Strimzi` с двумя регистрациями.

### 2.4 Детектор Strimzi (задача 04)

`pkg/analyzer/detector/strimzi.go`: `StrimziDetector` (имя `strimzi`, приоритет 85), регистрируется в `pkg/analyzer/detector/registry.go:RegisterAll`. Связи — `types.RelationNameReference`, поле `Field` указывает на источник (`spec.authorization.acls`, `spec.template.spec.containers[].env[].valueFrom.secretKeyRef`, `metadata.labels[strimzi.io/cluster]`, `spec.authentication.password.valueFrom.secretKeyRef`), `Details["strimzi"] = "true"`.

### 2.5 Факты клиента (задача 05)

Новый пакет `pkg/kafkaclient` (без зависимостей от `pkg/generator` и `pkg/processor`):

```go
type Certainty string // "fact", "convention", "guess"
type Direction string // "produce", "consume", "unknown"
type Framework string // "spring", "quarkus", "micronaut", "plain"

// Property is one configuration key of a container.
type Property struct {
	Key        string // property form ("spring.kafka.consumer.group-id") or env name
	Value      string // "" when FromSecret
	FromSecret bool   // value comes from a Secret (name known, value not)
	Origin     string // human-readable source, e.g. "env SPRING_KAFKA_CONSUMER_GROUP_ID"
}

type Value struct {
	Value      string
	Certainty  Certainty
	Origin     string
	Unresolved string // names of placeholders without default, "" if none
}

type Topic struct {
	Name      string
	Direction Direction
	Value          // certainty and origin of the name
}

type Facts struct {
	Framework       Framework
	Bootstrap       []Value
	SecurityProtocol *Value
	SASLMechanism   *Value
	JAASFromSecret  bool
	Topics          []Topic   // deduplicated by (Name, Direction)
	Groups          []Value
	TransactionalIDs []Value  // exact ids
	TransactionalPrefixes []Value
	StreamsAppID    *Value
	Patterns        []Value   // topic regexes (Quarkus pattern=true): no literal ACL
	Unresolved      []Value   // values with placeholders without default (Note:)
}

func NormalizeKey(key string) string                 // lower, without '.', '-', '_'
func DetectFramework(props []Property) Framework
func Extract(props []Property, framework Framework) Facts
func FromContainer(container map[string]interface{}, configMaps map[string]map[string]string) []Property
	// временный адаптер для задачи 06: env (value, valueFrom.secretKeyRef, valueFrom.configMapKeyRef),
	// envFrom.configMapRef; заменяется источником конфигурации EPIC-03 в задаче 07
```

### 2.6 Feature `kafka-client` (задача 06)

`pkg/generator/features_kafka.go` (регистрация в `init()`) и `pkg/generator/kafkaclient.go` (`applyKafkaClientFeature`, построение values и инъекций). Ключ values — `kafkaClient`. Шаблонов feature не добавляет: всё, что она меняет, — env и тома workload'ов через инъекцию.

### 2.7 Feature `strimzi` (задача 08, draft)

`pkg/generator/strimzifeature.go`: ключ values `strimzi`, шаблон `templates/strimzi-entities.yaml` (`range` по `strimzi.topics`, `strimzi.users`, `strimzi.access`). Инъекция переменных подключения — в тот же `strimzi.inject`.

### 2.8 Egress (задача 09, draft)

`pkg/generator/egresspolicies.go:egressDetectHosts` разбирает bootstrap-списки через парсер адресов EPIC-03, а не отбрасывает значения с запятой. Хосты из `kafkaClient.brokers` добавляются в `istioEgress.serviceEntries`, если включены обе features.

## 3. Модель данных

Values `KafkaTopic` (задача 03):

```yaml
services:
  <svc>:
    kafkaTopics:
      <key>:                # sanitizeName(metadata.name)
        enabled: true
        cluster: <label>    # "" если метки нет
        namespace: ""
        spec: {...}         # весь spec входа
        # необязательные переопределения: topicName, partitions, replicas, config
```

Values `KafkaUser` — `kafkaUsers.<key>` с теми же `enabled`, `cluster`, `namespace`, `spec` и переопределениями `authentication`, `authorization`, `quotas`, `template`.

Values `strimzi` (задача 08, draft):

```yaml
strimzi:
  enabled: true
  cluster: ""
  namespace: ""
  topics:
    <key>: {enabled: true, topicName: <kafka name>, partitions: null, replicas: null, config: {}, certainty: fact, origin: "<Kind/name>: <key>"}
  users:
    <workload key>: {enabled: true, name: <workload name>, authentication: scram-sha-512, acls: [...], quotas: {}}
  access:
    mode: same-namespace     # или kafka-access
    listener: ""
  inject: {...}
```

## 4. Алгоритмы

### 4.1 Шаблон Strimzi-объекта (задача 03)

Стандартный блок меток (`include "<chart>.labels" $ | nindent 4`) здесь не подходит. `processor.PreserveObjectLabels` сливает **все** метки входа литералами поверх меток chart'а, и тогда метка `strimzi.io/cluster` не переопределяется из values. Поэтому процессор строит блок меток сам, в форме, которую регулярное выражение `metadataLabelsBlock` не распознаёт. `PreserveObjectLabels` тогда возвращает шаблон без изменений.

```
{{- $svc := .Values.services.<svc> -}}
{{- if $svc.enabled }}
{{- with index $svc.kafkaTopics "<key>" }}
{{- if .enabled }}
{{- $dhgLabels := dict "<k1>" "<v1>" … }}                     {{/* метки входа без strimzi.io/cluster, отсортированы */}}
{{- with .cluster }}{{- $_ := set $dhgLabels "strimzi.io/cluster" . }}{{- end }}
apiVersion: <apiVersion входа>
kind: KafkaTopic
metadata:
  name: <processor.ObjectName(name)>
  namespace: {{ .namespace | default $.Release.Namespace }}
  labels:
    {{- toYaml (merge $dhgLabels (include "<chart>.labels" $ | fromYaml)) | nindent 4 }}
  {{- with .annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
<processor.SpecOverlay(".", "topicName", "partitions", "replicas", "config")>
{{- end }}
{{- end }}
{{- end }}
```

- Ключи и значения меток экранируются через `strconv.Quote`, как в `PreserveObjectLabels.dictOf`.
- Аннотации входа сохраняются в values `annotations`, если они есть.
- Заметки процессора: `v1beta2` (R8); `operation` в ACL (R8); нет метки `strimzi.io/cluster` (R5).

### 4.2 Инъекция (задача 02)

В начале вывода `podTemplate`, до строки `template:`, добавляется блок. Все действия обрезаны с двух сторон и не дают вывода:

```
{{- $dhgInjVolumes := list }}
{{- $dhgInjContainers := dict }}
{{- range $dhgKey, $dhgBlock := $.Values }}
{{- if and (kindIs "map" $dhgBlock) $dhgBlock.enabled (kindIs "map" $dhgBlock.inject) }}
{{- with index $dhgBlock.inject "<workloadKey>" }}
{{- $dhgW := tpl (toYaml .) $ | fromYaml }}
{{- $dhgInjVolumes = concat $dhgInjVolumes ($dhgW.volumes | default list) }}
{{- range $dhgC, $dhgS := ($dhgW.containers | default dict) }}
{{- $dhgP := get $dhgInjContainers $dhgC | default dict }}
{{- $_ := set $dhgInjContainers $dhgC (dict
      "env" (concat ($dhgP.env | default list) ($dhgS.env | default list))
      "envFrom" (concat ($dhgP.envFrom | default list) ($dhgS.envFrom | default list))
      "volumeMounts" (concat ($dhgP.volumeMounts | default list) ($dhgS.volumeMounts | default list))) }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}
```

- `range` по `$.Values` идёт в порядке сортировки ключей, поэтому рендер детерминирован.
- Блок `services` не имеет поля `enabled` на верхнем уровне и пропускается.
- `tpl` позволяет записывать в `inject` ссылки на высокоуровневые values feature (`'{{ .Values.kafkaClient.securityProtocol }}'`). Изменение такого значения меняет рендер без повторной генерации.
- В `containerTemplate` перед `- name:` добавляется `{{- $dhgInj := get $dhgInjContainers .name | default dict }}`, а блоки `env`, `envFrom`, `volumeMounts` рендерятся из `concat (.<field> | default list) ($dhgInj.<field> | default list)`.
- Без инъекций `concat` возвращает исходный список: вывод не меняется (AC4).
- Инъекции для init-контейнеров не поддерживаются: `$dhgInj` пуст, если такого имени нет в `containers` блока.

### 4.3 Нормализация ключей и извлечение фактов (задача 05)

1. `NormalizeKey`: нижний регистр, удалить `.`, `-`, `_`. Каталог (spec.md §3.2) хранится в двух формах:
   - фиксированные ключи — множество нормализованных имён;
   - шаблоны с переменным сегментом — префикс и суффикс в нормализованной форме (`springcloudstreambindings` + `<b>` + `destination`).

   Для шаблонов исходная форма ключа разбирается отдельно, чтобы восстановить `<b>`/`<ch>`:
   - property-форма: сегменты между префиксом и суффиксом;
   - env-форма: части между `_`.
2. Направление binding'а Spring Cloud Stream: `<b>` в исходной форме оканчивается на `-in-<n>` или (env) `_IN_<n>` → consume; `-out-<n>` / `_OUT_<n>` → produce.
3. Значение проходит `resolvePlaceholders` (та же семантика, что `pkg/synth/source.go:resolvePlaceholders`: `${NAME:default}` → `default`). Неразрешённые имена записываются в `Value.Unresolved`. Такое значение не попадает в `Topics`/`Bootstrap`, а возвращается в `Facts.Unresolved []Value` для `Note:`.
4. Список через запятую разбивается для `destination`, `topics`, bootstrap.
5. Дедупликация: топик — по (`Name`, `Direction`). При совпадении остаётся значение с большей достоверностью (`fact` > `convention` > `guess`).

### 4.4 Feature `kafka-client` (задача 06)

1. Если в chart'е нет workload'ов — chart возвращается без изменений (как `istio-egress`).
2. Workload'ы chart'а — `secChartWorkloads(chart, fc.Graph)`. Для каждого контейнера: `kafkaclient.FromContainer` → `DetectFramework` (или параметр `framework`) → `Extract`. Клиент — контейнер с непустым `Bootstrap` (любой достоверности) или указанный в `workloads`.
3. Для каждого клиента строится список env по spec.md §4.3. Значения — ссылки `tpl` на `kafkaClient.*`. Секреты — `valueFrom.secretKeyRef` с `name: '{{ .Values.kafkaClient.sasl.jaasConfigSecret.name }}'`. Части без заданного Secret'а пропускаются.
4. Переменная, имя которой уже есть в `env` контейнера во входе, не добавляется. Выводится `Note: <Kind/name>: env <NAME> is set by the input; kafka-client keeps it`.
5. Truststore-файл: том `dhg-kafka-truststore` (`secret.secretName` из values, `items: [{key, path: <key>}]`) и монтирование `readOnly` в `mountPath`.
6. Фреймворк `plain`: env не добавляются, `Note:` перечисляет нужные свойства клиента.
7. `GSSAPI`: только `sasl.mechanism` + `Note:` (keytab и `krb5.conf` — вне scope). `OAUTHBEARER`: mechanism, token endpoint URL и JAAS из Secret. Класс `sasl.login.callback.handler.class` зависит от версии клиента (в Kafka 4.x он перенесён из пакета `…oauthbearer.secured`), поэтому — `Note:`.

### 4.5 ACL и имена (задача 08, draft)

- `KafkaUser` на каждый workload-клиент: `metadata.name` = имя workload'а. ACL по таблице spec.md §4.4 объединяются по (`resource.type`, `name`, `patternType`), операции сортируются.
- Имя `KafkaTopic` из имени топика: нижний регистр; символы вне `[a-z0-9.-]` → `-`; повторы `-` схлопываются; обрезка до 253 символов. Если результат отличается от имени топика — задаётся `spec.topicName`.
- Топик, уже описанный `KafkaTopic` во входе (по `spec.topicName` или `metadata.name`), не генерируется.

## 5. Альтернативы

| Вариант | Плюсы | Минусы | Решение |
|---|---|---|---|
| Strimzi-объекты через generic fallback + правка `PreserveObjectLabels` (исключать «управляемые» метки) | Нет нового процессора | Правка общего механизма меток ради одной метки; нет связей и заметок | Отклонено: типизированный процессор (ADR-003, ADR-054) |
| `KafkaTopic` в namespace входа литералом | Совпадает с watched namespace оператора, если вход взят из кластера | Ломает перенос между окружениями; противоречит остальному chart'у | Отклонено: values `namespace` (R6) |
| Feature меняет values workload'а (`services.<svc>.deployment.containers[i].env`) | Нет изменения шаблонов | Правка вложенного YAML values текстом хрупка; несколько features конфликтуют; в separate/umbrella values плоские | Отклонено: инъекция через top-level ключ feature (ADR-кандидат 2) |
| Инъекция через named template, который переопределяет feature | Меньше кода в шаблоне pod'а | Helm не гарантирует порядок переопределения одноимённых `define`; две features не совмещаются | Отклонено |
| Разбор `@KafkaListener(topics=…)` / `@Topic` в Java-исходниках | Находит топики Spring Kafka и Micronaut | Значения обычно — placeholder'ы `${…}`, SpEL `#{…}` или константы. Разбор Java регулярными выражениями хрупок, и это догадка по коду, а не факт конфигурации | Отклонено для v1; повторно — по данным `SYNTHESIS.md` |
| Генерировать standalone NetworkPolicy `ipBlock` для Kafka | Работает без Cilium/Istio | CIDR брокеров не выводятся; Egress-политика запрещает весь остальной исходящий трафик pod'а | Отклонено (spec.md §7); FQDN — через EPIC-04 |
| Chart создаёт Secret с JAAS из values | «Работает из коробки» | Пароль в values и в git — нарушение ADR-059 | Отклонено: только ссылки на Secret'ы (R13) |

## 6. ADR-кандидаты

- **ADR-кандидат: Strimzi `KafkaTopic`/`KafkaUser` — типизированные процессоры; кластер и namespace — в values.**
  - Контекст: метка `strimzi.io/cluster` и watched namespace различаются между окружениями, а generic fallback пишет их литералами.
  - Решение: процессоры для `v1` и `v1beta2` без конвертации версий; метки строятся процессором (§4.1); `namespace` в values, по умолчанию — namespace релиза.
  - Последствия: `PreserveObjectLabels` не применяется к этим двум kind'ам.
- **ADR-кандидат: features добавляют env и тома в workload'ы через `<key>.inject` в values.**
  - Контекст: features (Kafka, базы данных, Keycloak) должны подключать приложение к Secret'ам, а единственная точка расширения — подмена `podAnnotations` (vault-agent).
  - Решение: шаблон pod'а конкатенирует инъекции всех включённых top-level блоков values. Значения проходят через `tpl`.
  - Последствия: шаблон pod'а длиннее на ~15 строк; инъекции видны и редактируемы в values; без инъекций рендер не меняется.
- **ADR-кандидат: процессоры и features сообщают невыводимое через `Note:`.**
  - Контекст: ADR-059 требует перечислять невыводимое, но канал был только у синтетических источников.
  - Решение: `Result.Notes`, `FeatureContext.AddNote`; заметки печатаются один раз и попадают в `SYNTHESIS.md`.
- **ADR-кандидат: chart ссылается на Secret'ы Kafka по имени и никогда не содержит учётных данных.** Развитие ADR-059 для `sasl.jaas.config`, паролей хранилищ и ключей.
- **ADR-кандидат: ACL генерируются только из фактов и соглашений. Догадки рендерятся выключенными.**

## 7. Влияние на существующее поведение

| Изменение | Влияние | Миграция |
|---|---|---|
| Процессоры Strimzi (03) | Для входов с `KafkaTopic`/`KafkaUser` меняются путь values (`services.<svc>.kafkaTopic` → `services.<svc>.kafkaTopics.<key>`, `kafkaUser` → `kafkaUsers.<key>`) и файл шаблона (`templates/kafkatopic-<name>.yaml` → `templates/<svc>-kafkatopic-<name>.yaml`). Рендер с values по умолчанию совпадает с прежним (namespace — по-прежнему namespace релиза) | Пользователи, переопределявшие values прежнего пути, переносят их; запись в `docs/RELEASE_NEXT.md` |
| Инъекция (02) | Текст шаблонов workload'ов меняется, рендер — нет | Нет |
| `ApplyFeaturesWithNotes` (01) | Внутренний API `pkg/generator`, `ApplyFeatures` не меняется | `cmd/dhg/main.go` вызывает новую функцию |
| `istio-egress` разбирает bootstrap-списки (09) | В chart'ах с Kafka появляются новые ServiceEntry | Запись в `docs/RELEASE_NEXT.md` |

## 8. Тестирование

- **Unit:**
  - `pkg/processor/k8s/strimzi_test.go` — версии, метки, namespace, overlay, заметки;
  - `pkg/analyzer/detector/strimzi_test.go`;
  - `pkg/kafkaclient/*_test.go` — табличные тесты каталога: каждая строка spec.md §3.2 в property- и env-форме;
  - `pkg/processor/k8s/podtemplate_test.go` — шаблон с инъекциями рендерится движком Helm-совместимых функций (`helm template` в golden) и через `text/template` + `sprig` (как в `pkg/generator/templateparse_test.go`);
  - `pkg/generator/kafkaclient_test.go`.
- **Golden:**
  - новая фикстура `tests/integration/fixtures/kafka-app` (задача 03) попадает во все сценарии `TestGeneratedChartsPassHelm` (в т. ч. fidelity);
  - `kafka-client` и `strimzi` попадают в `TestFeaturesPassHelm` автоматически (реестр features);
  - отдельный тест `TestKafkaClientFeature` в `tests/golden/kafka_test.go` проверяет отрендеренные env и тома (AC5).
- **Integration:** `tests/integration` — `dhg graph` на фикстуре (AC2).

## 9. Запросы к другим эпикам

**EPIC-03 (владелец общих контрактов).** Имена ниже — рабочие. EPIC-03 фиксирует окончательные, после чего задачи 07–10 переходят в `ready`.

| Нужно | Зачем | Минимальные требования |
|---|---|---|
| Типы связей «workload публикует в топик» и «workload потребляет из топика» (`types.RelationshipType`), логический узел топика | Задачи 07, 08 | `Relationship.Details` хранит `topic`, `group` (consume), `transactionalId`, `certainty` (`fact`/`convention`/`guess`), `origin` (ключ и контейнер). Идентичность узла топика включает идентичность брокера: нормализованный bootstrap-список или имя кластера Strimzi. Тогда один топик разных кластеров — разные узлы |
| Источник конфигурации workload'а: плоские ключи контейнера из env (`value`; имена Secret'ов для `secretKeyRef`), `valueFrom.configMapKeyRef` и `envFrom.configMapRef` (если ConfigMap во входе), конфигов проекта для `-s source` | Задача 07 заменяет временный адаптер `kafkaclient.FromContainer`; задача 10 | Каждая запись: ключ в исходной форме (property или env), значение, признак «из Secret», origin, контейнер. Формат совместим с `kafkaclient.Property` или конвертируется без потерь |
| Парсер адресов: bootstrap-списки | Задачи 07, 09 | Формы `host:port[,host:port…]`, элемент с префиксом `<LISTENER>://` (`SASL_SSL://b1:9093`), IPv6 `[::1]:9092`, хост без порта (порт по умолчанию задаёт вызывающий: 9092). Классификация «внутри кластера» (Service во входе, `*.svc`, `*.svc.<domain>`, `<svc>.<ns>`) / «внешний» |
| Группировка «сервис → PBC» (`app.kubernetes.io/part-of`) | Опция ACL с префиксом по PBC (развитие, §10) | Доступ к PBC workload'а по графу |

**EPIC-04.** Для `CiliumNetworkPolicy` `toFQDNs` к внешнему Kafka feature `kafka-client` публикует хосты в values `kafkaClient.brokers` (и bootstrap-хосты из фактов). Просьба: EPIC-04 читает внешние endpoint'ы из графа связей EPIC-03 и дополнительно из `kafkaClient.brokers` (список `host:port`). Протокол — TCP. L7-правил для Kafka в Cilium нет: Kafka-парсер Cilium в рамках этого эпика не используется.

**EPIC-08.** Для `OAUTHBEARER` с Keycloak `sasl.oauthbearer.token.endpoint.url` = `<issuer>/protocol/openid-connect/token`. Если EPIC-08 извлекает issuer, `kafka-client` может подставить URL как соглашение. Просьба: отдать issuer workload'а через граф (связь «вызов внешнего хоста» с `Details["issuer"]`).

**Всем эпикам.** Задачи 01 и 02 — общие механизмы. EPIC-09 зависит от них явно. Владельцу предлагается внести их в таблицу общих контрактов `docs/roadmap/README.md` §3.

## 10. Масштабирование и развитие

- ACL с `patternType: prefix` по границе PBC (`<pbc>.`) — по контракту группировки EPIC-03.
- `KafkaConnect`/`KafkaConnector`: коннекторы приложения — в scope приложения. Процессор `KafkaConnector` — отдельный эпик.
- Модуль Deckhouse `managed-kafka` — после проверки API.
- Квоты `KafkaUser` (`producerByteRate`, `consumerByteRate`) не выводятся из фактов. Их можно задать в values `strimzi.users.<w>.quotas`.
- Разбор аннотаций в коде (§5) — по метрикам `SYNTHESIS.md`.
