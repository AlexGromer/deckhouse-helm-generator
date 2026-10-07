# EPIC-03: дизайн

Документ определяет **общие контракты** эпика (раздел 3 `docs/roadmap/README.md`): типы связей и логические узлы графа, общий парсер адресов, модель «сервис → PBC → продукт». EPIC-02, EPIC-04, EPIC-08, EPIC-09 ссылаются на разделы 2–4 и не переопределяют их. Расширения запрашиваются в своём `design.md` (раздел «Запросы к другим эпикам»).

Термины — в `spec.md`, раздел 1.

## Содержание

1. [Обзор архитектуры](#1-обзор-архитектуры)
2. [Компоненты и изменения](#2-компоненты-и-изменения)
3. [Модель данных (контракт)](#3-модель-данных-контракт)
4. [Алгоритмы](#4-алгоритмы)
5. [Альтернативы](#5-альтернативы)
6. [ADR-кандидаты](#6-adr-кандидаты)
7. [Влияние на существующее поведение](#7-влияние-на-существующее-поведение)
8. [Тестирование](#8-тестирование)
9. [Запросы к другим эпикам и контракты для них](#9-запросы-к-другим-эпикам-и-контракты-для-них)
10. [Масштабирование и развитие](#10-масштабирование-и-развитие)

---

## 1. Обзор архитектуры

### 1.1. Где появляется новое

```
[1] Extract ─► []ExtractedResource
[2] Process ─► []ProcessedResource
[3] Analyze ─► ResourceGraph
      detectors (как сейчас) ─► Relationships, Groups            (не меняется)
      NEW graph pass "connections" ─► Connections, Nodes, Notes  (pkg/analyzer/detector/connection.go)
            └─ pkg/connect: Extract (ключи → находки), Parse (значения → адреса), Index.Resolve (хост → Service)
[3b] NEW Topology ─► graph.Topology (PBC каждого сервиса)          (pkg/generator/pbc.go, вызывается из cmd/dhg/pipeline.go)
[4] Generate ─► []GeneratedChart
      umbrella/separate/library: NEW раскладка по PBC при --group-by pbc
[4k] features (--with): NEW connection-policies (NetworkPolicy из Connections + Topology)
[5] Write; NEW PBC.md при --group-by pbc
dhg graph: NEW рёбра соединений, --level service|pbc, --format json|report
```

### 1.2. Поток данных одного соединения

```
Deployment/shop/order-api
  containers[app].env[PAYMENTS_URL] = "http://payment-api:8080/api/v1"
        │  connect.MatchEnv("PAYMENTS_URL") → generic suffix _URL, RoleAddress, base=medium
        │  connect.ParseValue(..., FormatAuto) → Endpoint{Protocol:http, Host:payment-api, Port:8080, Path:/api/v1}
        │  Index.Resolve("payment-api", 8080, "shop") → input-service Service/shop/payment-api, targetPort "8080", PortMatched
        │  confidence: medium → high (порт совпал с портом Service из входа)
        ▼
types.Connection{From: Deployment/shop/order-api, Container: app, Type: service_call,
                 To: {Resource: Service/shop/payment-api}, Protocol: http, Port: 8080, TargetPort: "8080",
                 Path: /api/v1, Confidence: high, Evidence: [{env, Deployment/shop/order-api, ..., PAYMENTS_URL, "http://payment-api:8080/api/v1"}]}
        │  Topology: order-api ∈ PBC orders, payment-api ∈ PBC payments → межPBC-контракт
        ▼
dhg graph (рёбра), PBC.md (контракты), --with connection-policies (NetworkPolicy), EPIC-04 (CNP), EPIC-02 (ACL)
```

### 1.3. Ключевое архитектурное решение

Соединения времени выполнения (runtime connections — вызовы по сети и обмен через брокер) хранятся **отдельно** от структурных связей (`ResourceGraph.Relationships`). Причина — факты кода (проверено 2026-10-07):

| Потребитель `graph.Relationships` | Что сломалось бы при смешивании |
|---|---|
| `generator.GroupResources`, проход 2 (`pkg/generator/grouping.go`) | BFS по всем связям объединил бы ресурсы разных микросервисов без меток в одну группу (chart) |
| `analyzer.DefaultAnalyzer.groupResources` → `findRelatedService` (`pkg/analyzer/analyzer.go`) | ресурс без `ServiceName` получил бы имя сервиса, который его вызывает |
| `analyzer.DetectCircularDependencies` (`pkg/analyzer/graph.go`) | взаимные вызовы A↔B — нормальны, но выдавались бы как цикл зависимостей |
| `buildCrossNamespaceIndex` (`pkg/generator/networkpolicy.go`) | `--namespace-resources` открыл бы ingress из namespace каждого вызывающего |
| `analyzer.AnalyzeDecomposition` | коэффициент связности групп изменился бы |

Поэтому: типы связей — новые константы `RelationshipType` (контракт README §3), но записи — в новом срезе `ResourceGraph.Connections`. Существующий код, читающий `Relationships`, не меняется и не меняет результат.

---

## 2. Компоненты и изменения

| Пакет / файл | Новое или изменение | Задача |
|---|---|---|
| `pkg/types/connection.go` (новый) | типы соединений, логических узлов, доказательств, уверенности, топологии PBC | 01 |
| `pkg/types/relationship.go` | поля `Connections`, `Nodes`, `Topology` в `ResourceGraph`; методы `AddConnection`, `AddNode`, `ConnectionsFrom`, `ConnectionsTo`, `Node` | 01 |
| `pkg/connect/parse.go` (новый пакет) | разбор значений: URL, JDBC, R2DBC, списки `host:port`, Mongo, Redis, AMQP, OIDC issuer; `Redact` | 02 |
| `pkg/connect/catalog.go`, `extract.go` | каталог ключей (Spring Boot, Quarkus, Micronaut, MicroProfile, общие env), сопоставление env ↔ свойство, `Extract` | 03, 07 |
| `pkg/connect/resolve.go` | индекс Service входа, разрешение хоста и порта | 04 |
| `pkg/connect/config.go` | разбор конфигурационных файлов (`application*.yml/.yaml/.properties`), `-Dkey=value`, `--key=value`, `SPRING_APPLICATION_JSON` | 06 |
| `pkg/analyzer/analyzer.go` | интерфейс `GraphPass`, `DefaultAnalyzer.AddPass`, `DefaultAnalyzer.Notes` | 05 |
| `pkg/analyzer/detector/connection.go` (новый) | `ConnectionPass`: сбор фактов из pod spec, ConfigMap, Secret; построение `Connections` | 05, 06, 07 |
| `pkg/analyzer/pbc.go` (новый) | запросы к графу для отчётов и других эпиков: `CrossPBCConnections`, `TopicUsages`, `ServiceOf` | 13 |
| `pkg/analyzer/graph.go` | DOT/Mermaid: рёбра соединений, логические узлы, уровни `service`/`pbc`, кластеры PBC; `GenerateJSONGraph`, `GenerateReport` | 12, 13 |
| `pkg/generator/pbc.go` (новый) | `ParsePBCMapping`, `ResolveTopology` | 09 |
| `pkg/generator/umbrella.go` | вложенный umbrella «продукт → PBC → сервис» | 10 |
| `pkg/generator/separate.go`, `library.go`, `generator.go` | раскладка каталогов `<pbc>/<svc>`; `Options.GroupBy`; экспорт `ChartNameOf` | 10, 11 |
| `pkg/generator/egresspolicies.go` | `istio-egress` берёт хосты из `graph.Connections` | 08 |
| `pkg/generator/features_connections.go` (новый) | feature `connection-policies` | 14 |
| `cmd/dhg/pipeline.go`, `main.go`, `graph.go` | флаги `--cluster-domain`, `--pbc`, `--pbc-label`, `--group-by`; `graph --level/--format json/report`; запись `PBC.md` | 05, 09–13 |

Новых внешних зависимостей нет: только стандартная библиотека (`net/url`, `net`, `net/netip`, `regexp`, `encoding/json`, `encoding/base64`) и уже подключённые `sigs.k8s.io/yaml`, `k8s.io/apimachinery`.

Граф зависимостей пакетов (циклов нет): `pkg/connect → pkg/types`; `pkg/analyzer/detector → pkg/connect, pkg/analyzer, pkg/types`; `pkg/generator → pkg/types, pkg/synth (DNSName)`; `pkg/synth → pkg/connect` (только задача 15).

---

## 3. Модель данных (контракт)

Все идентификаторы ниже — **контракт**. Переименование — только через этот документ.

### 3.1. Типы связей (`pkg/types/connection.go`)

```go
// Runtime connections. They are stored in ResourceGraph.Connections, never in
// ResourceGraph.Relationships (structural links between objects).
const (
	// RelationServiceCall: a workload calls a Service — one from the input
	// (To.Resource) or one of the cluster that is not in the input
	// (To.Node of kind NodeClusterService).
	RelationServiceCall RelationshipType = "service_call"
	// RelationExternalCall: a workload calls a host outside the cluster
	// (To.Node of kind NodeExternalHost or NodeExternalIP).
	RelationExternalCall RelationshipType = "external_call"
	// RelationTopicProduce / RelationTopicConsume: a workload writes to / reads
	// from a broker destination (To.Node of kind NodeTopic).
	RelationTopicProduce RelationshipType = "topic_produce"
	RelationTopicConsume RelationshipType = "topic_consume"
	// RelationTopicUse: the input names the destination but not the direction.
	RelationTopicUse RelationshipType = "topic_use"
)

// IsConnection reports whether t is one of the runtime connection types above.
func (t RelationshipType) IsConnection() bool
```

Соединение с брокером (bootstrap servers, AMQP addresses) — обычный `service_call`/`external_call` с `Role: "broker"`. Отдельного типа «broker» нет: брокер — это адрес, топик — логический узел.

### 3.2. Уверенность (confidence)

```go
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// AtLeast reports whether c >= min (high > medium > low). An empty
// Confidence is treated as low.
func (c Confidence) AtLeast(min Confidence) bool
```

Пороги потребителей (контракт):

| Потребитель | Минимум |
|---|---|
| `dhg graph` | все; `low` — пунктирные рёбра |
| `PBC.md`, `--format report`, `--format json` | все, с колонкой confidence |
| `connection-policies` (EPIC-03), `CiliumNetworkPolicy` (EPIC-04), ACL Kafka (EPIC-02) | `medium` и выше; `low` — только в отчёт |

### 3.3. Логические узлы

```go
type NodeKind string

const (
	NodeExternalHost   NodeKind = "external-host"   // DNS name outside the cluster
	NodeExternalIP     NodeKind = "external-ip"     // IPv4/IPv6 literal
	NodeClusterService NodeKind = "cluster-service" // <svc>.<ns> of the cluster, absent from the input
	NodeTopic          NodeKind = "topic"           // broker destination (Kafka topic, AMQP address)
)

// NodeID is the stable identifier of a logical node:
//   external-host:<host>                e.g. external-host:api.bank.example.com
//   external-ip:<ip>                    e.g. external-ip:10.20.0.5
//   cluster-service:<namespace>/<name>  e.g. cluster-service:kafka/my-cluster-kafka-bootstrap
//   topic:<brokerID>/<name>             e.g. topic:cluster:kafka/my-cluster-kafka-bootstrap:9092/shop.orders.created
// Hosts are lower case without a trailing dot. brokerID is defined in design §4.7 ("" when unknown).
type NodeID string

type LogicalNode struct {
	ID        NodeID
	Kind      NodeKind
	Name      string       // host, IP, Service name or destination name
	Namespace string       // NodeClusterService only
	Broker    string       // NodeTopic only: brokerID ("" = unknown)
	Protocol  string       // NodeTopic only: "kafka", "amqp" or "" (unknown)
	Resource  *ResourceKey // NodeTopic: the KafkaTopic of the input it matches, if any
}
```

Service из входа логическим узлом **не** становится: ребро ведёт к его `ResourceKey`. Так EPIC-04 и NetworkPolicy получают селектор pod'ов Service из входа.

### 3.4. Соединение и доказательства

```go
// ConnectionTarget: exactly one of Resource and Node is set.
type ConnectionTarget struct {
	Resource *ResourceKey // a Service of the input (RelationServiceCall)
	Node     NodeID       // a logical node otherwise
}

// String: Resource.String() or string(Node). Used for sorting and as a map key.
func (t ConnectionTarget) String() string

type Connection struct {
	From       ResourceKey      // workload: Deployment, StatefulSet, DaemonSet, ReplicaSet, Job, CronJob, Pod
	Container  string           // container or init container name
	Type       RelationshipType // one of the five connection types
	To         ConnectionTarget
	Protocol   string // connect.Proto* ("http", "postgresql", "kafka", …); "tcp" when unknown; "" for topics of unknown broker
	TLS        bool   // scheme says TLS (https, rediss, amqps, ldaps, SSL/SASL_SSL listener, ssl=true in JDBC)
	Role       string // connect.Role* service role: "http-api", "database", "cache", "broker", "oidc-issuer", …
	Port       int32  // port as addressed (Service port for Services); 0 = unknown
	TargetPort string // in-input Service only: resolved pod port, decimal number or port name; "" = unknown
	Path       string // URL path without query ("/api/v1"); "" when none
	Attributes map[string]string // documented keys only, see below
	Confidence Confidence
	Evidence   []Evidence // at least one; sorted by (Object, Field, Key)
}

type EvidenceSource string

const (
	EvidenceEnv          EvidenceSource = "env"           // containers[].env[].value
	EvidenceConfigMapEnv EvidenceSource = "configmap-env" // env valueFrom / envFrom of a ConfigMap
	EvidenceSecretEnv    EvidenceSource = "secret-env"    // env valueFrom / envFrom of a Secret (value never copied)
	EvidenceConfigFile   EvidenceSource = "config-file"   // property in a mounted ConfigMap file (application.yml, …)
	EvidenceArgs         EvidenceSource = "args"          // --key=value in command/args
	EvidenceJavaOpts     EvidenceSource = "java-opts"     // -Dkey=value in JAVA_OPTS, JAVA_TOOL_OPTIONS, JDK_JAVA_OPTIONS
	EvidenceAppJSON      EvidenceSource = "application-json" // SPRING_APPLICATION_JSON
	EvidenceSynthesized  EvidenceSource = "synthesized"   // project property read by pkg/synth (task 15)
)

type Evidence struct {
	Source EvidenceSource
	Object ResourceKey // object that holds the value: workload, ConfigMap or Secret
	Field  string      // path inside Object, see the format below
	Key    string      // env var name or property key
	Value  string      // connect.Redact(value); "" for EvidenceSecretEnv
}
```

Формат `Evidence.Field` (контракт, по нему пишутся golden-проверки):

| Источник | `Field` |
|---|---|
| env Deployment/StatefulSet/DaemonSet/ReplicaSet/Job | `spec.template.spec.containers[<container>].env[<VAR>]` (`initContainers[...]` для init-контейнеров) |
| env CronJob | `spec.jobTemplate.spec.template.spec.containers[<container>].env[<VAR>]` |
| env Pod | `spec.containers[<container>].env[<VAR>]` |
| ConfigMap/Secret (env, envFrom) | `data[<key>]` или `stringData[<key>]` (Object — ConfigMap/Secret) |
| файл конфигурации | `data[<file>]#<property>` (Object — ConfigMap) |
| args/command | `<путь к контейнеру>.args[<i>]` / `.command[<i>]` |
| JAVA_OPTS | путь env-переменной, как для env |

Ключи `Attributes` (контракт; других ключей не добавлять без правки этого документа):

| Ключ | Когда | Значение |
|---|---|---|
| `database` | JDBC/R2DBC/Mongo/Redis с номером БД | имя БД из пути (`orders`), номер для Redis |
| `issuer` | `Role == oidc-issuer` | URL issuer без query |
| `realm` | issuer Keycloak вида `…/realms/<realm>` или `…/auth/realms/<realm>` | `<realm>` |
| `group` | топик, consume | consumer group (`spring.cloud.stream.bindings.<b>.group`, `spring.kafka.consumer.group-id`, `mp.messaging.incoming.<c>.group.id`) |
| `binding` | Spring Cloud Stream | имя binding (`orderCreated-out-0`) |
| `channel` | SmallRye Reactive Messaging | имя канала |
| `directionHint` | `topic_use` | `produce`/`consume`, если имя env содержит токен направления (раздел 4.6); направление **не** выставляется |
| `via` | хост пришёл через Service `type: ExternalName` | `Service/<ns>/<name>` |
| `clientName` | Feign/REST client/HTTP service | имя клиента из ключа (`payments`) |

### 3.5. Индексы и методы `ResourceGraph`

```go
type ResourceGraph struct {
	// ... existing fields unchanged ...

	// Connections are the runtime connections of the workloads (design §1.3).
	Connections []Connection
	// Nodes are the logical nodes the connections lead to.
	Nodes map[NodeID]*LogicalNode
	// Topology is the PBC layer over the service groups; nil until computed.
	Topology *Topology

	connFrom map[ResourceKey][]int // indexes into Connections
	connTo   map[string][]int      // keyed by ConnectionTarget.String()
	connKey  map[string]int        // dedup key -> index
}

// AddConnection appends c, or merges it into an existing connection with the
// same From, Container, Type, To, Protocol, Port and Path: evidence is
// unioned (deduplicated, sorted) and the higher confidence kept. Maps are
// created lazily, so a zero ResourceGraph works.
func (g *ResourceGraph) AddConnection(c Connection)
// AddNode stores n unless a node with the same ID exists; returns the stored node.
func (g *ResourceGraph) AddNode(n LogicalNode) *LogicalNode
func (g *ResourceGraph) Node(id NodeID) (*LogicalNode, bool)
// ConnectionsFrom / ConnectionsTo return copies in insertion order.
func (g *ResourceGraph) ConnectionsFrom(key ResourceKey) []Connection
func (g *ResourceGraph) ConnectionsTo(t ConnectionTarget) []Connection
```

`NewResourceGraph` создаёт `Nodes` и индексы. `AddRelationship` и прочие методы не меняются.

### 3.6. Топология PBC

```go
type MembershipSource string

const (
	MembershipMapping MembershipSource = "mapping" // --pbc
	MembershipLabel   MembershipSource = "label"   // --pbc-label (default app.kubernetes.io/part-of)
)

type PBC struct {
	Name     string           // DNS-1123 label: chart name, values key, directory
	RawName  string           // label value before sanitizing ("" for mapping)
	Source   MembershipSource // how the PBC was formed (mapping wins over label)
	Services []string         // service group names (generator.ServiceGroup.Name), sorted
}

type Topology struct {
	Product     string                 // --chart-name
	Label       string                 // label key used; "" when the label source is off
	PBCs        []PBC                  // sorted by Name
	ServicePBC  map[string]string      // service group name -> PBC name; "" = unassigned
	ResourcePBC map[ResourceKey]string // every grouped resource -> PBC name; "" = unassigned
}

// PBCOf returns the PBC of a resource ("" when unassigned or t is nil).
func (t *Topology) PBCOf(key ResourceKey) string
// PBC returns the PBC by name.
func (t *Topology) PBC(name string) (PBC, bool)
// Unassigned returns the service group names without a PBC, sorted.
func (t *Topology) Unassigned() []string
```

Сервис = группа `generator.GroupResources` (`ServiceGroup.Name`), то есть единица chart'а в режимах separate/library/umbrella. Это сознательно: PBC группирует chart'ы.

### 3.7. Пакет `pkg/connect` (контракт парсера)

```go
package connect

// Protocol families (Connection.Protocol). TLS is a separate flag.
const (
	ProtoHTTP = "http"; ProtoGRPC = "grpc"; ProtoWS = "ws"
	ProtoPostgreSQL = "postgresql"; ProtoMySQL = "mysql"; ProtoMariaDB = "mariadb"
	ProtoOracle = "oracle"; ProtoSQLServer = "sqlserver"; ProtoMongoDB = "mongodb"
	ProtoRedis = "redis"; ProtoAMQP = "amqp"; ProtoKafka = "kafka"; ProtoNATS = "nats"
	ProtoLDAP = "ldap"; ProtoSMTP = "smtp"; ProtoTCP = "tcp"
)

// Service roles (Connection.Role).
const (
	RoleHTTPAPI = "http-api"; RoleDatabase = "database"; RoleCache = "cache"; RoleBroker = "broker"
	RoleOIDCIssuer = "oidc-issuer"; RoleTelemetry = "telemetry"; RoleServiceRegistry = "service-registry"
	RoleConfigServer = "config-server"; RoleMail = "mail"; RoleDirectory = "directory"; RoleSearch = "search"
	RoleUnknown = "unknown"
)

type Format int

const (
	FormatAuto         Format = iota // URL if it has "://" or a known prefix (jdbc:, r2dbc:, configserver:), else host[:port]
	FormatURL                        // one URL
	FormatJDBC                       // jdbc:… (also r2dbc:…, vertx-reactive:…)
	FormatHostPortList               // "h1:9092,h2:9092"; each item may carry a scheme or a Kafka listener prefix
	FormatURLList                    // "http://a:9200,http://b:9200"
	FormatHost                       // host only; the port comes from a paired key
)

type Endpoint struct {
	Scheme        string // lower-case scheme as written, without jdbc:/r2dbc:/pool:/vertx-reactive: ("postgresql", "https", "")
	Protocol      string // Proto*
	TLS           bool
	Host          string // lower case; IPv6 without brackets
	Port          int    // 0 = unknown
	PortDefaulted bool   // Port is the scheme default, not written in the value
	Path          string // URL path ("/api/v1"); "" for none or "/"
	Database      string // JDBC/R2DBC/Mongo database, Redis db number
	SRV           bool   // mongodb+srv: Host is an SRV name, Port is 0
}

// ParseValue parses one configuration value into endpoints. defaultProtocol
// applies to values without a scheme. It returns an error (never a partial
// result) for values it cannot parse; Extract turns errors into skips.
func ParseValue(raw string, f Format, defaultProtocol string) ([]Endpoint, error)

// Redact returns raw without userinfo passwords and without query parameters
// named password, pwd, secret, token, apikey, api-key, access_token (case-insensitive):
// their values become "REDACTED".
func Redact(raw string) string

// IsLoopback: localhost, *.localhost, 127.0.0.0/8, ::1, 0.0.0.0, ::.
func IsLoopback(host string) bool

// --- catalog (catalog.go) ---

type KeyRole string

const (
	KeyAddress       KeyRole = "address"        // value is a full address or a list of them
	KeyHost          KeyRole = "host"           // host only; Rule.PortKey names the paired port key
	KeyPort          KeyRole = "port"
	KeyTopic         KeyRole = "topic"          // destination name(s)
	KeyConsumerGroup KeyRole = "consumer-group"
	KeyConnector     KeyRole = "connector"      // SmallRye connector name
)

type Direction string

const (
	DirectionProduce Direction = "produce"
	DirectionConsume Direction = "consume"
	DirectionUnknown Direction = ""
)

// Rule is one catalog entry. Pattern is a property key in dot form; a
// segment "{name}" captures one segment, "{name...}" one or more segments
// (dot-joined), e.g. "spring.cloud.stream.bindings.{binding}.destination".
type Rule struct {
	ID         string // == Pattern for property rules; "env:<SUFFIX>" for generic env rules
	Framework  string // "spring", "spring-cloud", "quarkus", "microprofile", "micronaut", "opentelemetry", "generic"
	Pattern    string
	Role       KeyRole
	Format     Format
	Protocol   string // protocol when the value has no scheme
	Service    string // Role* (service role)
	PortKey    string // KeyHost only: pattern of the paired port key, same captures
	Direction  Direction
	Confidence types.Confidence // base confidence
}

type KeyForm string

const (
	FormProperty  KeyForm = "property"   // spring.datasource.url
	FormEnvMP     KeyForm = "env-mp"     // SPRING_DATASOURCE_URL: every non-alphanumeric -> "_", upper case
	FormEnvSpring KeyForm = "env-spring" // SPRING_KAFKA_BOOTSTRAPSERVERS: "." -> "_", "-" removed, upper case
	FormEnvSuffix KeyForm = "env-suffix" // generic suffix rule (_URL, _HOST, …)
)

type KeyMatch struct {
	Rule     *Rule
	Form     KeyForm
	Captures map[string]string // values of {name} segments (in env form: upper case, as written)
}

func MatchProperty(key string) (KeyMatch, bool) // strips a leading "%<profile>." (Quarkus) and reports it via Captures["profile"]
func MatchEnv(name string) (KeyMatch, bool)     // catalog env forms first, then generic suffix rules
func EnvNameMP(property string) string
func EnvNameSpring(property string) string
func Rules() []Rule // copy of the catalog, for documentation and tests

// --- extraction (extract.go) ---

type Fact struct {
	Key      string // env var name (IsEnv) or property key
	Value    string
	IsEnv    bool
	Secret   bool           // value comes from a Secret: parsed, never echoed
	Evidence types.Evidence // Source, Object, Field, Key; Value is filled by Extract (redacted)
}

type FindingKind string

const (
	FindingAddress FindingKind = "address"
	FindingTopic   FindingKind = "topic"
)

type Finding struct {
	Kind       FindingKind
	Endpoints  []Endpoint        // FindingAddress
	Topic      string            // FindingTopic
	Direction  Direction         // FindingTopic
	Protocol   string            // address: from endpoint or rule; topic: "kafka", "amqp" or ""
	Service    string            // Role* (service role)
	BrokerKeys []string          // FindingTopic: rule IDs of the broker addresses this topic uses (channel-specific first)
	Attributes map[string]string // documented keys of types.Connection.Attributes
	Confidence types.Confidence  // base confidence before resolution
	Evidence   []types.Evidence
}

type Skip struct {
	Evidence types.Evidence
	Reason   string // fixed reasons, see design §4.5
}

// Extract classifies the facts of one container, pairs host/port keys,
// parses values and derives topics. Deterministic: findings sorted by
// (Kind, first evidence Field, Key).
func Extract(facts []Fact) (findings []Finding, skipped []Skip)

// --- resolution (resolve.go) ---

type TargetKind string

const (
	TargetInputService   TargetKind = "input-service"
	TargetClusterService TargetKind = "cluster-service"
	TargetExternalHost   TargetKind = "external-host"
	TargetExternalIP     TargetKind = "external-ip"
	TargetLoopback       TargetKind = "loopback"
)

type Resolution struct {
	Kind        TargetKind
	Service     *types.ResourceKey // TargetInputService
	Namespace   string             // TargetInputService, TargetClusterService
	Name        string             // Service name
	Host        string             // external host / IP (after ExternalName)
	ViaService  *types.ResourceKey // Service of type ExternalName the host came through
	Port        int                // port as addressed (0 = unknown)
	TargetPort  string             // TargetInputService: resolved pod port ("8080" or "http"); "" unknown
	PortMatched bool               // TargetInputService: the Service exposes Port (or has exactly one port and Port was 0)
	Ambiguous   bool               // host could also be a Service of a namespace absent from the input
	Reason      string             // human-readable explanation for notes
}

type Index struct{ /* unexported */ }

// NewIndex indexes Services (ClusterIP, headless, ExternalName), their ports,
// pod selectors, namespaces and KafkaTopics of the input.
func NewIndex(resources map[types.ResourceKey]*types.ProcessedResource, clusterDomain string) *Index
func (ix *Index) Resolve(host string, port int, fromNamespace string) Resolution
// SelectsWorkload reports whether Service svc selects the pods of workload w (self-call detection).
func (ix *Index) SelectsWorkload(svc, w types.ResourceKey) bool
// KafkaTopic finds a KafkaTopic of the input by topic name (spec.topicName, else metadata.name)
// and, when clusterName != "", by label strimzi.io/cluster.
func (ix *Index) KafkaTopic(topic, clusterName string) (*types.ResourceKey, bool)

// --- configuration sources (config.go, task 06) ---

// ParseConfigFile flattens application*.yml/.yaml (all YAML documents without
// spring.config.activate.on-profile / spring.profiles) or *.properties into dot keys.
func ParseConfigFile(name, content string) (map[string]string, error)
// JavaOpts returns -Dkey=value pairs of a JVM options string, in order.
func JavaOpts(value string) [][2]string
// CommandLineProperties returns --key=value pairs of container args (Spring Boot command-line properties).
func CommandLineProperties(args []string) [][2]string
// ApplicationJSON flattens SPRING_APPLICATION_JSON.
func ApplicationJSON(value string) (map[string]string, error)
```

---

## 4. Алгоритмы

### 4.1. Сбор фактов контейнера (`ConnectionPass`, задачи 05–06)

Для каждого workload'а (виды: Deployment, StatefulSet, DaemonSet, ReplicaSet, Job, CronJob, Pod; путь к pod spec — как `secPodSpec` в `pkg/generator/features_security.go`), в порядке сортировки `ResourceKey.String()`, для каждого контейнера (`initContainers`, затем `containers`):

1. **Эффективное окружение** — по семантике Kubernetes (`EnvFromSource`: при совпадении ключей побеждает последний источник; `env` перекрывает `envFrom`):
   - `envFrom[]` по порядку: `configMapRef.name` → ConfigMap того же namespace во входе, все ключи `data` (и `binaryData` пропускается); `secretRef.name` → Secret: `stringData[k]`, иначе base64-декодированный `data[k]`. Ключ env = `prefix + key`. Ключ, не являющийся допустимым именем env, пропускается (как kubelet).
   - `env[]` по порядку: `value` (литерал); `valueFrom.configMapKeyRef{name,key}`; `valueFrom.secretKeyRef{name,key}`; `fieldRef`/`resourceFieldRef` — пропуск без заметки.
   - Ссылки `$(VAR)` в `value` раскрываются по уже определённым переменным того же контейнера (правило Kubernetes: только объявленные **раньше** в `env` и все из `envFrom`); `$$(VAR)` → литерал `$(VAR)`. Нераскрытая ссылка → `Skip{Reason: "unresolved $(VAR)"}` для этого значения.
   - ConfigMap/Secret отсутствует во входе → одна заметка на объект: `<workload>: ConfigMap <name> is not in the input; its variables are not inspected`.
   - Значение Secret, равное `REDACTED` (режим `--cluster-secrets mask`, `pkg/extractor/cluster.go`), — `Skip{Reason: "masked secret"}`.
2. **Свойства из дополнительных источников** (задача 06):
   - env `JAVA_OPTS`, `JAVA_TOOL_OPTIONS`, `JDK_JAVA_OPTIONS` → `connect.JavaOpts` → факты-свойства (`EvidenceJavaOpts`).
   - `command`/`args` → `connect.CommandLineProperties` (`--key=value`, только ключи, найденные `MatchProperty`) → `EvidenceArgs`.
   - env `SPRING_APPLICATION_JSON` → `connect.ApplicationJSON` → `EvidenceAppJSON`.
   - тома `configMap` контейнера (`volumeMounts` ↔ `volumes[].configMap.name`, с учётом `items[].path` и `subPath`): ключи ConfigMap с именем файла `application.yml`, `application.yaml`, `application.properties`, `application-<profile>.(yml|yaml|properties)`, `bootstrap.yml`, `bootstrap.yaml`, `bootstrap.properties` → `connect.ParseConfigFile` → `EvidenceConfigFile`. Базовая уверенность находок из файла: `high`, если каталог монтирования — известное место (`/config/`, `<workingDir>/config/`, `<workingDir>/`, каталог из `SPRING_CONFIG_LOCATION`/`SPRING_CONFIG_ADDITIONAL_LOCATION`/`--spring.config.location`/`--spring.config.additional-location`, для Quarkus — `<workingDir>/config/`); иначе на уровень ниже правила каталога (`medium` вместо `high`), причина в заметке. Файлы профилей (`application-<profile>.*`) дают факты с `Captures["profile"]`; находки профилей получают уверенность на уровень ниже и атрибут-заметку (профиль активен не всегда).
3. **Приоритет при совпадении** свойства из нескольких источников (как в Spring Boot: аргументы > `SPRING_APPLICATION_JSON` > JVM `-D` > env > файл): в находку идёт значение источника с наивысшим приоритетом, остальные — дополнительными `Evidence`, если значения совпадают, иначе заметка `<workload>: <key> differs between <src1> and <src2>; using <src1>`.

### 4.2. Классификация ключей

1. `MatchProperty(key)`: снять префикс профиля Quarkus `%<profile>.`; сопоставить с шаблонами каталога (раздел 4.3) посегментно. Ключи Quarkus в кавычках (`quarkus.rest-client."org.acme.Client".url`) — сегмент в кавычках считается одним сегментом.
2. `MatchEnv(name)`: для каждого правила каталога сравнить имя с `EnvNameMP(Pattern)` и `EnvNameSpring(Pattern)`, где `{name}` → `(.+?)`, `{name...}` → `(.+?)` (регулярное выражение, якоря `^…$`, сначала правила без захватов). Если совпадений нет — общие суффиксные правила (таблица 4.3.2), самое длинное совпадение суффикса побеждает.
3. Ключ, не найденный ни там, ни там: если значение содержит `://` с известной схемой (таблица 4.4.1) — находка `FindingAddress` с уверенностью `low` и `Rule.ID = "value-scheme"`; иначе факт игнорируется без заметки.

`EnvNameMP` совпадает с `envName` из `pkg/synth/frameworks.go` (ветка main, работа над Quarkus/Micronaut): буква/цифра → верхний регистр, остальное → `_`. `EnvNameSpring` — правило Spring Boot relaxed binding: `.` → `_`, `-` удаляется, верхний регистр (`spring.main.log-startup-info` → `SPRING_MAIN_LOGSTARTUPINFO`, docs Spring Boot, проверено).

### 4.3. Каталог ключей (`pkg/connect/catalog.go`)

#### 4.3.1. Свойства фреймворков

Уверенность всех правил таблицы — `high`, кроме отмеченных. «Пара» — ключ порта для `KeyHost`.

| Pattern | Role | Format | Protocol | Service | Пара / направление | Проверено |
|---|---|---|---|---|---|---|
| `spring.datasource.url`, `spring.datasource.hikari.jdbc-url`, `spring.flyway.url`, `spring.liquibase.url` | address | JDBC | — | database | | url — проверено (SPEC_SYNTHESIS, `source.go`); hikari/flyway/liquibase — не проверено |
| `spring.r2dbc.url` | address | JDBC | — | database | | не проверено |
| `spring.data.redis.url` | address | URL | redis | cache | | проверено (`RedisProperties`, Boot 3.5.0) |
| `spring.data.redis.host` | host | Host | redis | cache | `spring.data.redis.port` (умолч. 6379 — `PortDefaulted`) | проверено |
| `spring.data.redis.cluster.nodes`, `spring.data.redis.sentinel.nodes` | address | HostPortList | redis | cache | | проверено |
| `spring.redis.url`, `spring.redis.host` (+`spring.redis.port`) | как выше | | redis | cache | Boot 2.x | не проверено |
| `spring.data.mongodb.uri` | address | URL | mongodb | database | | не проверено |
| `spring.data.mongodb.host` | host | Host | mongodb | database | `spring.data.mongodb.port` | не проверено |
| `spring.rabbitmq.addresses` | address | HostPortList | amqp | broker | | проверено (`RabbitProperties`) |
| `spring.rabbitmq.host` | host | Host | amqp | broker | `spring.rabbitmq.port` (умолч. 5672) | проверено |
| `spring.kafka.bootstrap-servers`, `spring.kafka.consumer.bootstrap-servers`, `spring.kafka.producer.bootstrap-servers`, `spring.kafka.streams.bootstrap-servers`, `spring.kafka.admin.bootstrap-servers` | address | HostPortList | kafka | broker | | общий и consumer/producer — проверено (`KafkaProperties`); streams/admin — не проверено |
| `spring.cloud.stream.kafka.binder.brokers` | address | HostPortList | kafka | broker | | не проверено |
| `spring.elasticsearch.uris` | address | URLList | http | search | | не проверено |
| `spring.ldap.urls` | address | URLList | ldap | directory | | не проверено |
| `spring.mail.host` | host | Host | smtp | mail | `spring.mail.port` | не проверено |
| `spring.security.oauth2.resourceserver.jwt.issuer-uri`, `spring.security.oauth2.client.provider.{provider}.issuer-uri` | address | URL | http | oidc-issuer | | resourceserver — проверено (SPEC_SYNTHESIS); client.provider — не проверено |
| `spring.security.oauth2.resourceserver.jwt.jwk-set-uri` | address | URL | http | oidc-issuer | | не проверено |
| `spring.cloud.openfeign.client.config.{client}.url`, `feign.client.config.{client}.url` | address | URL | http | http-api | `clientName` | первый — проверено (docs spring-cloud-openfeign main); второй (до 4.0) — не проверено |
| `eureka.client.service-url.{zone}` | address | URLList | http | service-registry | | не проверено |
| `spring.cloud.config.uri` | address | URLList | http | config-server | | не проверено |
| `spring.config.import` (элементы `configserver:<url>`) | address | URL (префикс `configserver:` снимается; `optional:` снимается) | http | config-server | | не проверено |
| `management.zipkin.tracing.endpoint`, `management.otlp.tracing.endpoint`, `management.otlp.metrics.export.url` | address | URL | http | telemetry | | не проверено |
| `spring.kafka.template.default-topic` | topic | — | kafka | broker | produce | проверено (`KafkaProperties.Template.defaultTopic`) |
| `spring.kafka.consumer.group-id` | consumer-group | — | kafka | | | проверено (`KafkaProperties`) |
| `spring.cloud.stream.bindings.{binding}.destination` | topic | — | из binder'а (4.6) | broker | по имени binding (4.6) | проверено (docs spring-cloud-stream, functional-binding-names) |
| `spring.cloud.stream.bindings.{binding}.group` | consumer-group | | | | | не проверено |
| `spring.cloud.stream.function.bindings.{binding}` (значение — новое имя) | алиас binding'а (4.6) | | | | | проверено |
| `quarkus.datasource.jdbc.url`, `quarkus.datasource."{name}".jdbc.url` | address | JDBC | — | database | | первый — проверено (`frameworks.go`, main); именованный — не проверено |
| `quarkus.datasource.reactive.url`, `quarkus.datasource."{name}".reactive.url` | address | JDBC (снимается `vertx-reactive:`) | — | database | | первый — проверено (`frameworks.go`) |
| `quarkus.redis.hosts`, `quarkus.redis."{name}".hosts` | address | URLList | redis | cache | | не проверено |
| `quarkus.mongodb.connection-string` | address | URL | mongodb | database | | не проверено |
| `quarkus.oidc.auth-server-url` | address | URL | http | oidc-issuer | | проверено (`frameworks.go`) |
| `quarkus.rest-client.{client}.url`, `quarkus.rest-client."{client}".url` | address | URL | http | http-api | `clientName` | проверено (docs quarkus rest-client) |
| `{client}/mp-rest/url` | address | URL | http | http-api | `clientName`; MicroProfile Rest Client | не проверено |
| `kafka.bootstrap.servers` | address | HostPortList | kafka | broker | Quarkus/Micronaut | проверено (docs quarkus kafka; `frameworks.go`) |
| `mp.messaging.{dir}.{channel}.bootstrap.servers` (`{dir}` — `incoming` или `outgoing`: в каталоге по два правила) | address | HostPortList | kafka | broker | канальный broker | проверено (docs quarkus kafka) |
| `mp.messaging.{dir}.{channel}.topic` | topic | | kafka* | broker | `dir`: incoming → consume, outgoing → produce | проверено |
| `mp.messaging.{dir}.{channel}.connector` | connector | | | | `smallrye-kafka` → kafka; `smallrye-amqp`, `smallrye-rabbitmq` → amqp | проверено (docs quarkus kafka) |
| `mp.messaging.incoming.{channel}.group.id` | consumer-group | | | | | не проверено |
| `mp.messaging.{dir}.{channel}.address` | topic | | amqp | broker | для `smallrye-amqp` | не проверено |
| `datasources.{name}.url` | address | JDBC | — | database | Micronaut | `default` — проверено (`frameworks.go`) |
| `redis.uri`, `redis.servers.{name}.uri` | address | URL | redis | cache | Micronaut | не проверено |
| `micronaut.http.services.{client}.url` | address | URL | http | http-api | `clientName` | не проверено |
| `micronaut.http.services.{client}.urls` | address | URLList | http | http-api | `clientName` | не проверено |
| `micronaut.security.oauth2.clients.{client}.openid.issuer` | address | URL | http | oidc-issuer | | проверено (`frameworks.go`) |
| env `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`, `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` (Framework `opentelemetry`, только env-форма) | address | URL | http | telemetry | | не проверено |

\* Если `connector` не задан и канал есть только в коде (`@Incoming`), конфигурация канала отсутствует: такой канал не виден, это перечисляется в «Невыводимое» (`spec.md` §6).

Правила с пометкой «не проверено» реализатор проверяет по документации фреймворка до реализации (порядок работы, README §4 шаг 3); неподтверждённое правило удаляется из каталога, а не угадывается.

#### 4.3.2. Общие env-правила (Framework `generic`)

Применяются, только если имя не совпало с каталогом. Уверенность — `medium`, Service — `unknown`, если не указано.

| Суффикс / имя | Role | Format | Protocol | Пара |
|---|---|---|---|---|
| `_URL`, `_URI`, `_ENDPOINT`, `_DSN` | address | Auto | tcp | |
| `_ADDR`, `_ADDRESS` | address | Auto | tcp | |
| `_HOST`, `_HOSTNAME` | host | Host | tcp | `<prefix>_PORT` |
| `_SERVERS`, `_BROKERS`, `_BOOTSTRAP_SERVERS`, `_NODES` | address | HostPortList | tcp; `kafka`, если имя содержит токен `KAFKA` | |
| `PGHOST` | host | Host | postgresql | `PGPORT` (умолч. 5432) |
| `_TOPIC`, `_TOPICS`, `_TOPIC_NAME` | topic | список через запятую | `kafka`, если у контейнера есть kafka-брокер (4.7), иначе `""` | направление неизвестно |

Набор `_URL, _URI, _ENDPOINT, _ADDR, _ADDRESS, _HOST` — тот же, что `egressEnvSuffixes` в `pkg/generator/egresspolicies.go` (сохраняется совместимость `istio-egress`, задача 08).

### 4.4. Разбор значений (`ParseValue`)

#### 4.4.1. Схемы и порты по умолчанию

Порты по умолчанию — из `egressSchemePorts` (`pkg/generator/egresspolicies.go`), дополненные; `PortDefaulted = true`.

| Схема | Protocol | TLS | Порт |
|---|---|---|---|
| `http` / `https` | http | нет / да | 80 / 443 |
| `ws` / `wss` | ws | нет / да | 80 / 443 |
| `grpc` / `grpcs` | grpc | нет / да | 443 / 443 (как в `egressSchemePorts`, сохраняется для совместимости `istio-egress`) |
| `postgres`, `postgresql` | postgresql | | 5432 |
| `mysql` / `mariadb` | mysql / mariadb | | 3306 |
| `oracle` (JDBC) | oracle | | 1521 |
| `sqlserver` (JDBC) | sqlserver | | 1433 |
| `mongodb` / `mongodb+srv` | mongodb | | 27017 / 0 (`SRV = true`) |
| `redis` / `rediss` | redis | нет / да | 6379 |
| `amqp` / `amqps` | amqp | нет / да | 5672 / 5671 |
| `kafka` | kafka | | 9092 |
| `nats` | nats | | 4222 |
| `ldap` / `ldaps` | ldap | нет / да | 389 / 636 |
| `smtp` / `smtps` | smtp | нет / да | 25 / 465 |
| Kafka listener-префиксы `PLAINTEXT://`, `SSL://`, `SASL_PLAINTEXT://`, `SASL_SSL://` | kafka | `SSL`, `SASL_SSL` — да | нет умолчания |

Игнорируемые схемы и формы (возвращается пустой результат без ошибки, `Extract` пишет `Skip{Reason: "not a network address"}`): `file:`, `classpath:`, `jar:`, `data:`, `mailto:`, `jdbc:h2:`, `jdbc:hsqldb:`, `jdbc:derby:`, `jdbc:sqlite:`, `jdbc:tc:` (Testcontainers).

#### 4.4.2. Форматы

- **URL** (`net/url`): хост в нижнем регистре; порт 1–65535, иначе ошибка; `Path` без `/` в конце (кроме пустого); `Database` = первый сегмент пути для postgresql/mysql/mariadb/mongodb, номер для redis (`/0`).
- **JDBC** (после снятия `jdbc:`, `r2dbc:`, `pool:`, `vertx-reactive:`):
  - `postgresql://h1[:p1][,h2[:p2]]/db?…` — несколько хостов (pgJDBC multi-host); `ssl=true`/`sslmode=require|verify-ca|verify-full` → TLS;
  - `mysql[:loadbalance|:replication]://h1[:p],h2[:p]/db`, `mariadb://…`;
  - `oracle:thin:@host:port:SID`, `oracle:thin:@//host:port/service`, `oracle:thin:@(DESCRIPTION=…)` — все пары `(HOST=…)(PORT=…)` регулярным выражением без учёта регистра; `Database` = SID/service;
  - `sqlserver://host[\instance][:port];databaseName=db;…` — `\instance` отбрасывается из хоста; `encrypt=true` → TLS.
- **HostPortList**: разделители `,` и `;`, пробелы обрезаются; элемент — `[scheme://]host[:port]`; IPv6 — в `[]`.
- **Host**: одно имя без `/` и `:`; порт — из парного ключа (`Extract`), иначе `Rule.Protocol`-умолчание из 4.4.1 с `PortDefaulted`, иначе 0.
- **Auto**: `FormatURL`, если есть `://` или префикс `jdbc:`/`r2dbc:`/`configserver:`; иначе `FormatHostPortList`.

Ошибка (`Skip{Reason: "unparsable"}`): пробельные символы внутри значения, пустой хост, порт вне диапазона, хост с символами вне `[a-z0-9.-]` (кроме IPv6 в скобках), значение длиннее 2048 байт.

### 4.5. `Extract`: правила и отсечение ложных срабатываний

1. Значения с плейсхолдерами `${…}`: Spring/Quarkus `${NAME:default}` → подставляется default (как `resolvePlaceholders` в `pkg/synth/source.go`); `${NAME}` без default → `Skip{Reason: "placeholder ${NAME}"}`.
2. Пары host/port: для `KeyHost` ищется факт с ключом `PortKey` (с теми же захватами); порт — целое 1–65535, иначе порт = 0 и заметка. `_PORT` без `_HOST` ничего не даёт (это собственный порт приложения: `SERVER_PORT`, `PORT`).
3. Локальные адреса (`IsLoopback`) → `Skip{Reason: "loopback"}`; sidecar'ы и сам pod NetworkPolicy не нужны.
4. Секреты: факт с `Secret = true` разбирается, `Evidence.Value = ""`. Значения остальных фактов — через `Redact`.
5. Фиксированные причины `Skip.Reason` (контракт, их печатает отчёт): `placeholder ${X}`, `unresolved $(X)`, `masked secret`, `loopback`, `not a network address`, `unparsable`, `self`, `duplicate`.
6. Значения из одного ключа, повторяющие уже найденный адрес того же контейнера, сливаются (одна находка, несколько `Evidence`).

### 4.6. Топики и направление (задача 07)

**Spring Cloud Stream** (docs `functional-binding-names.adoc`, проверено): имя binding'а `<function>-in-<index>` — вход (consume), `<function>-out-<index>` — выход (produce).

1. Алиасы: `spring.cloud.stream.function.bindings.<orig>=<alias>` — направление `<alias>` берётся из `<orig>`.
2. Направление binding'а `b`: `-in-\d+$` → consume; `-out-\d+$` → produce; `input` → consume и `output` → produce с уверенностью `medium` (устаревшая модель `Sink.INPUT`/`Source.OUTPUT`); иначе `topic_use`.
3. В env-форме (`SPRING_CLOUD_STREAM_BINDINGS_<B>_DESTINATION`) имя binding'а необратимо (регистр и `-` потеряны); направление — по суффиксу `_IN_\d+$`/`_OUT_\d+$` захвата `<B>`; `Attributes["binding"]` = захват как есть.
4. `destination` может быть списком через запятую — каждый элемент даёт свою находку.
5. Binding упомянут (например, есть только `.group`), но `destination` нет → топик = имя binding'а (документированное умолчание), уверенность `medium`.
6. Protocol: `kafka`, если у контейнера есть находка-брокер kafka или ключи `spring.cloud.stream.kafka.*`; `amqp` при ключах `spring.cloud.stream.rabbit.*` или `spring.rabbitmq.*` без kafka; иначе `""`.

**SmallRye Reactive Messaging** (Quarkus, docs `kafka.adoc`, проверено): `mp.messaging.incoming.<c>.*` → consume, `outgoing` → produce; топик = `.topic`, иначе имя канала (уверенность `medium`); `connector`: `smallrye-kafka` → kafka. Env-форма `MP_MESSAGING_(INCOMING|OUTGOING)_<C>_TOPIC` — направление однозначно.

**Spring Kafka**: `spring.kafka.template.default-topic` → produce. `spring.kafka.consumer.group-id` → `Attributes["group"]` всех consume-находок контейнера без своего group. Если group задан, а consume-находок нет — заметка `<workload>: consumer group <g> is configured; its topics come from code (@KafkaListener) and are not detected`.

**Общие env** `_TOPIC`: `topic_use`; если имя содержит токен (разбиение по `_`) `PRODUCER`, `PRODUCE`, `PUBLISH`, `OUTPUT`, `OUT` → `Attributes["directionHint"]="produce"`; `CONSUMER`, `CONSUME`, `SUBSCRIBE`, `INPUT`, `IN`, `LISTEN` → `"consume"`. Тип связи при этом остаётся `topic_use`: имя переменной — не факт направления.

### 4.7. Брокер топика и `brokerID`

1. Брокер находки-топика — адреса kafka контейнера: сначала канальные (`mp.messaging.<dir>.<c>.bootstrap.servers`), затем `spring.kafka.(consumer|producer).bootstrap-servers` по направлению, затем общие (`spring.kafka.bootstrap-servers`, `spring.cloud.stream.kafka.binder.brokers`, `kafka.bootstrap.servers`, generic `*_BOOTSTRAP_SERVERS`/`*_BROKERS` с токеном `KAFKA`).
2. Каждый адрес брокера разрешается (4.8). Элемент `brokerID`: `svc:<ns>/<name>:<port>` (Service входа), `cluster:<ns>/<name>:<port>`, `ext:<host>:<port>`, `ip:<ip>:<port>`. `brokerID` = отсортированные уникальные элементы через `,`; нет брокера → `""`.
3. Слияние: топик с `brokerID == ""` и тем же именем, что ровно у одного узла-топика с известным брокером, присоединяется к нему (заметка `topic <name>: broker of <workload> unknown, assumed <brokerID>`). При нескольких кандидатах — отдельный узел и заметка.
4. KafkaTopic входа (`kafka.strimzi.io/v1` — Strimzi ≥ 0.49, единственная в 1.0+; `kafka.strimzi.io/v1beta2` — до 0.51): имя топика = `spec.topicName`, иначе `metadata.name`; кластер = метка `strimzi.io/cluster`. Сопоставление с узлом: имя совпадает и (кластер не задан, либо хотя бы один элемент `brokerID` вида `svc:`/`cluster:` имеет имя Service `<cluster>-kafka-bootstrap` — соглашение Strimzi об имени bootstrap Service, docs Red Hat Streams for Apache Kafka 2.7 через поиск 2026-10-07). Найдено → `LogicalNode.Resource`.

### 4.8. Разрешение хоста (`Index.Resolve`)

Вход: `host` (нижний регистр, без завершающей точки), `port` (0 = неизвестен), `fromNamespace` (namespace workload'а; `""` — объект без namespace = namespace релиза). `clusterDomain` — флаг `--cluster-domain`, по умолчанию `cluster.local` (значение по умолчанию `clusterDomain` в Deckhouse `ClusterConfiguration` — `cluster.local`, проверено по docs deckhouse.io через поиск 2026-10-07).

| Шаг | Условие | Результат |
|---|---|---|
| 1 | `IsLoopback(host)` | `loopback` |
| 2 | IP-литерал | `external-ip` |
| 3 | суффикс `.svc.<clusterDomain>` или `.svc` | остаток: `<svc>.<ns>` или `<pod>.<svc>.<ns>` (DNS pod'а StatefulSet за headless Service) → (svc, ns); иное число меток → `external-host`, `Ambiguous` |
| 4 | содержит метку `svc`, но домен ≠ `clusterDomain` (`a.b.svc.corp.local`) | как шаг 3, `Reason: "cluster domain differs from --cluster-domain"`, уверенность −1 |
| 5 | одна метка | (host, `fromNamespace`) — путь поиска DNS pod'а |
| 6 | две метки `a.b`, и `b` — namespace входа или во входе есть Service `b/a` | (a, b) |
| 7 | две метки иначе | `external-host`, `Ambiguous = true`, `Reason: "may be Service a in namespace b outside the input"` |
| 8 | три метки `p.s.n`, `n` — namespace входа, `s` — headless Service в `n` | (s, n) |
| 9 | иначе | `external-host` |

Для (svc, ns): Service `ns/svc` во входе (`Kind: Service`, группа `""`, любая версия) →

- `spec.type: ExternalName` → `external-host` с `Host = spec.externalName`, `ViaService = Service`;
- иначе `input-service`. Порт: `spec.ports[]` с `port == Port` → `PortMatched`; `Port == 0` и ровно один порт → он, `PortMatched`; `TargetPort` = `targetPort` (int → десятичная строка, string → имя), отсутствует → равен `port`;
- Service нет во входе → `cluster-service` (ns, svc), `Port` как адресован.

### 4.9. Уверенность итоговая

Начальная — `Rule.Confidence` (каталог `high`, generic `medium`, `value-scheme` `low`). Затем по порядку (одна ступень = high↔medium↔low, не выше high и не ниже low):

| Условие | Изменение |
|---|---|
| `input-service`, `PortMatched` | +1 |
| `input-service`, `Port > 0`, не `PortMatched` | −1, заметка `port N is not exposed by Service X` |
| шаг 5 таблицы 4.8 и Service нет во входе (`cluster-service` по короткому имени) | = low |
| `Ambiguous` | = low |
| шаг 4 таблицы 4.8 | −1 |
| факт из файла вне известного места или файла профиля (4.1) | −1 (до шагов выше) |

### 4.10. Построение соединений

Для каждой находки-адреса и каждого `Endpoint`:

- `loopback` → skip; `input-service`, выбирающий этот же workload (`SelectsWorkload`) → `Skip{Reason: "self"}`.
- Type: `input-service`, `cluster-service` → `service_call`; `external-host`, `external-ip` → `external_call`.
- To: `Resource` = Service для `input-service`; иначе `AddNode` с ID из 3.3.
- Остальные поля — из `Endpoint`, `Resolution`, `Finding`.

Для находки-топика: `To.Node = topic:<brokerID>/<name>`; Type по направлению; `Protocol` из 4.6; `Port = 0`.

### 4.11. Топология PBC (`ResolveTopology`, задача 09)

```go
// ParsePBCMapping parses --pbc values "<pbc>=<svc>[,<svc>...]".
func ParsePBCMapping(values []string) (map[string][]string, error)

type TopologyOptions struct {
	Product string              // --chart-name
	Label   string              // --pbc-label; "" disables the label source
	Mapping map[string][]string // ParsePBCMapping result
}

// ResolveTopology assigns every service group to a PBC. Notes describe
// conflicts and sanitized names; errors are for invalid mappings only.
func ResolveTopology(groups []*ServiceGroup, opts TopologyOptions) (*types.Topology, []string, error)
```

1. Разбор `--pbc`: имя PBC — DNS-1123 label (`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`, ≤ 63), иначе ошибка; список сервисов не пуст; сервис в двух PBC — ошибка; повтор PBC — объединение.
2. Сервис из `--pbc`, которого нет среди групп, — ошибка с перечнем групп (отсортированы, не больше 20 имён).
3. Для каждой группы (по имени): есть в `--pbc` → `MembershipMapping`; если метка указывает другое — заметка. Иначе метка `Label`: значения у workload'ов группы (виды с pod template); если ни у одного нет — у всех ресурсов группы; выбирается самое частое, при равенстве — лексикографически меньшее; разные значения → заметка. Имя PBC = `synth.DNSName(значение)`; отличается от значения → заметка; пусто → без PBC.
4. `ResourcePBC` заполняется для каждого ресурса группы.

### 4.12. Вложенный umbrella (задача 10)

Проверено на Helm v3.19.0 (собран из `helm.sh/helm/v3@v3.19.0`, 2026-10-07) на модели product → PBC → service:

- вложенные subchart'ы в `charts/<pbc>/charts/<svc>/` без `repository` в `dependencies` проходят `helm lint --strict` и `helm template`;
- `condition: <svc>.enabled` в `Chart.yaml` PBC вычисляется по пути `<pbc>.<svc>.enabled` значений верхнего chart'а (`processDependencyConditions(..., cpath)`, `cpath = path + <subchart name> + "."` — `pkg/chartutil/dependencies.go` v3.19.0 и `pkg/chart/v2/util/dependencies.go` в main/Helm 4): `orders.order-worker.enabled: false` в values продукта отключает сервис;
- `global` верхнего chart'а доходит до сервисов второго уровня (`coalesceDeps` → `coalesceGlobals` рекурсивно, `pkg/chartutil/coalesce.go` v3.19.0);
- PBC и сервис с одинаковым именем (`orders/charts/orders`) рендерятся; **но** именованные шаблоны глобальны: определение `"orders.fullname"` в chart'е PBC перекрыло одноимённое в сервисе (`name: PBC-HELPER` в эксперименте). Отсюда требование: `_helpers.tpl` chart'ов PBC и продукта генерируется той же функцией `helm.GenerateHelpers(name)`, что и сервисов, — одноимённые определения тогда текстуально идентичны и перекрытие безвредно (проверяется unit-тестом).

Раскладка:

```
<out>/<product>/                       Chart.yaml: dependencies = PBC и сервисы без PBC (по имени), condition <name>.enabled
  values.yaml                          global + <pbc>: {enabled, <svc>: {плоские values сервиса, enabled}} + <svc без PBC>: {...}
  charts/<pbc>/                        Chart.yaml: dependencies = сервисы PBC, condition <svc>.enabled; type: application
    values.yaml                        global: {} + <svc>: {плоские values, enabled}
    charts/<svc>/                      chart сервиса — как сейчас (SeparateGenerator.generateChartForGroup)
  charts/<svc без PBC>/
```

- Имена chart'ов: продукт — `--chart-name`; PBC — `PBC.Name`; сервис — `ServiceGroup.Name` (как сейчас).
- Коллизия имени PBC и сервиса без PBC на уровне продукта — ошибка `PBC %q and service %q share the name under umbrella %q; rename the PBC with --pbc`.
- Порядок `dependencies` и ключей values — по имени (детерминированный вывод).
- `NOTES.txt` продукта перечисляет PBC и их сервисы: `{{ if (index .Values "orders" "order-api").enabled }}`; у chart'ов PBC `Notes` пустой (Helm показывает NOTES только верхнего chart'а).
- Values дублируются на трёх уровнях так же, как сейчас родитель umbrella дублирует values subchart'ов (`UmbrellaGenerator.Generate`, `flatVals`): переопределение работает с любого уровня.

### 4.13. separate и library по PBC (задача 11)

- separate: `chart.Name = "<pbc>/<svc>"` (без PBC — `"<svc>"`), каталог `<out>/<pbc>/<svc>/`; имя в `Chart.yaml` — `<svc>`.
- library: библиотека в `<out>/library/`; chart'ы сервисов — `<out>/<pbc>/<svc>/` с `repository: file://../../library` (без PBC — `file://../library`, как сейчас).
- universal: раскладка не меняется (один chart; values-пути задаёт процессор, ADR-047).
- `cmd/dhg/main.go`: сопоставление chart → группа для `--env-values` (`groupsByName[chart.Name]`, строка 753) переходит на `generator.ChartNameOf(chart)` (экспорт `chartNameOf` из `pkg/generator/namespace.go`).

### 4.14. Граница PBC и межPBC-контракты (задача 13)

```go
// pkg/analyzer/pbc.go
type ContractEdge struct {
	FromService, FromPBC string // caller service group and its PBC ("" = unassigned)
	ToService, ToPBC     string // callee service group (input Service's group) and PBC
	Connection           types.Connection
}
// CrossPBCConnections: service_call connections whose caller and callee PBC differ
// (unassigned counts as its own PBC named after the service). Sorted.
func CrossPBCConnections(g *types.ResourceGraph) []ContractEdge

type TopicUsage struct {
	Node      types.LogicalNode
	Producers []types.Connection
	Consumers []types.Connection
	Unknown   []types.Connection // topic_use
	OwnerPBC  string             // design §4.14 rule; "" = shared/unknown
	PBCs      []string           // all PBCs touching the topic, sorted
}
func TopicUsages(g *types.ResourceGraph) []TopicUsage
// ServiceOf returns the service group name of a resource ("" if none).
func ServiceOf(g *types.ResourceGraph, key types.ResourceKey) string
```

Владелец топика (OwnerPBC): (1) PBC KafkaTopic входа (`Topology.PBCOf(Node.Resource)`); (2) иначе единственный PBC среди producer'ов; (3) иначе `""`. Топик межPBC (асинхронный контракт), если `len(PBCs) > 1`.

`ServiceOf` использует `Topology` (группа → ресурсы); без `Topology` — `ProcessedResource.ServiceName`.

### 4.15. Feature `connection-policies` (задача 14)

Регистрация (`pkg/generator/features_connections.go`):

| Параметр | По умолчанию | Смысл |
|---|---|---|
| `dns-namespace` | `kube-system` | namespace DNS-сервера для egress на 53/UDP и 53/TCP (как `generateNetworkPolicy` в `networkpolicy.go`) |
| `min-confidence` | `medium` | минимальная уверенность используемых соединений (`high` или `medium`) |

Для каждого workload'а chart'а с селектором pod'ов (`secChartWorkloads` + `workloadPodSelector`):

- **Ingress**: правило на каждого вызывающего workload'а `C` (соединения `service_call` к Service, выбирающему этот workload): `from.podSelector` = селектор `C`; `namespaceSelector` (`kubernetes.io/metadata.name`) — из `peerNamespaces[<PBC вызывающего или его сервис>]`, иначе namespace `C` во входе, если он отличается; `ports` = `TargetPort` (число или имя порта — `NetworkPolicyPort.port` принимает «numerical or named port on a pod», `k8s.io/api` v0.35.3 `networking/v1/types.go`, проверено) и протокол порта Service.
- **Egress**: DNS; на каждое соединение ≥ `min-confidence`: Service входа → `podSelector` селектора Service (+ `namespaceSelector` по правилу выше) и `TargetPort`; `cluster-service` → `namespaceSelector` по namespace без `ports` (порт pod'а неизвестен: адресуется порт Service); `external-ip` → `ipBlock: {cidr: <ip>/32 или /128}` и порт; `external-host` — не выражается в NetworkPolicy (нет FQDN-селекторов; docs Kubernetes «What you can't do with network policies», проверено).
- `restrictIngress` по умолчанию `true`, только если: workload не backend `Ingress`/`HTTPRoute`/`GRPCRoute` входа, не выбран `ServiceMonitor`/`PodMonitor` входа, нет Service `type: NodePort|LoadBalancer`, и (есть хотя бы один вызывающий во входе или у workload'а нет Service). Иначе `false` и заметка с причиной.
- `restrictEgress` по умолчанию `true`, только если у workload'а нет соединений с `external-host`, нет соединений ниже `min-confidence` и нет пропусков `placeholder`/`unresolved`. Иначе `false` и заметка.
- Объект не рендерится, если оба флага `false` (NetworkPolicy без `policyTypes` по умолчанию ограничивает Ingress).
- Комментарий над каждым правилом: `# <caller|callee> <ResourceKey> (PBC <name>[, cross-PBC])`.

Values (верхний ключ chart'а):

```yaml
connectionPolicies:
  enabled: true
  dnsNamespace: kube-system
  peerNamespaces: {}        # <PBC или сервис без PBC>: <namespace>; пусто — namespace из входа или namespace релиза
  ingressFrom: []           # NetworkPolicyPeer[] для backend'ов Ingress/Gateway (контроллер — платформа, во входе его нет)
  monitoringFrom: []        # NetworkPolicyPeer[] для целей ServiceMonitor/PodMonitor
  workloads:
    order-api:
      enabled: true
      restrictIngress: false   # backend of Ingress shop: set ingressFrom, then true
      restrictEgress: false    # calls external host api.bank.example.com: add extraEgress, then true
      extraIngress: []
      extraEgress: []
```

Ключ workload'а — имя; при совпадении имён в одном chart'е — `<kind в нижнем регистре>-<namespace>-<name>`.

---

## 5. Альтернативы

| Вариант | Плюсы | Минусы | Решение |
|---|---|---|---|
| Соединения в `Relationships` с фильтрацией по типу во всех потребителях | один список | 5 существующих потребителей меняют поведение при любой ошибке фильтра; `ResourceKey` не умеет логические узлы | отклонено (1.3) |
| Логические узлы как псевдо-`ResourceKey` (`GVK{Group: "logical.dhg"}`) | граф DOT/Mermaid работает без изменений | псевдо-объекты попадают в `Resources`-подобные обходы (golden integrity, процессоры) | отклонено |
| Отдельный `ConnectionTarget` + `Nodes` | явная модель, ноль влияния на старое | новые методы и индексы | **принято** |
| Парсер в `pkg/generator` (рядом с `egressEndpoint`) | меньше файлов | `pkg/synth` и `pkg/analyzer` не могут импортировать generator без циклов | отклонено → `pkg/connect` |
| Направление топика по имени env (`*_PRODUCER_TOPIC`) | больше направленных рёбер | имя переменной — не факт (ADR-059) | только `directionHint` |
| PBC по `app.kubernetes.io/part-of` | стандартная рекомендуемая метка Kubernetes («The name of a higher level application this one is part of») | семантика свободная: у одних это продукт, у других PBC | **принято** + `--pbc-label` для другой метки + `--pbc` для явного соответствия |
| Продукт по метке `dhg.deckhouse.io/product` | продукт виден в объектах | метка выдумана dhg, требует правки манифестов во всех репозиториях; один запуск = один umbrella и так | отклонено: продукт = `--chart-name` |
| Продукт по `.dhg.yaml`-карте отдельной структурой | одна карта для всего | `.dhg.yaml` принимает только имена флагов (`cmd/dhg/config.go`, неизвестный ключ — ошибка) | `--pbc` как повторяемый флаг; в `.dhg.yaml` — список `pbc: [...]` |
| PBC-раскладка по умолчанию при наличии `part-of` | меньше флагов | меняет вывод `examples/06–09` (у них `part-of: myapp`) и уже сгенерированных chart'ов пользователей | отклонено: opt-in `--group-by pbc` |
| NetworkPolicy через расширение `--namespace-resources` | один флаг | меняет вывод существующего флага; его ingress `podSelector: {}` и порты по имени env несовместимы с минимальными правилами | отдельная feature `connection-policies` (ADR-046) |
| Запрет egress при внешнем хосте (deny + values ipBlock) | минимально по определению | ломает работу приложения сразу после установки | `restrictEgress: false` + заметка; FQDN — EPIC-04 (`toFQDNs`) |

## 6. ADR-кандидаты

**ADR-кандидат: соединения времени выполнения хранятся отдельно от структурных связей.** Контекст: 1.3. Решение: `ResourceGraph.Connections` + `Nodes`; типы — константы `RelationshipType`, `IsConnection()`. Последствия: существующие группировка, циклы, NetworkPolicy `--namespace-resources` не меняются; новые потребители (graph, отчёт, features, EPIC-02/04/08/09) читают `Connections`.

**ADR-кандидат: общий парсер адресов `pkg/connect` без внешних зависимостей.** Контекст: адреса разбираются в трёх местах (`egresspolicies.go`, `networkpolicy.go` по именам env, `autodeps.go` по префиксам). Решение: один каталог ключей и один парсер, используемые analyzer'ом, features и `pkg/synth`. Последствия: `istio-egress` видит env из ConfigMap (синтетические приложения кладут env в ConfigMap `<name>-env`); каталог ключей — документированный контракт.

**ADR-кандидат: уровни уверенности и пороги потребителей.** Контекст: значение env может выглядеть как адрес и не быть им. Решение: `high/medium/low` по таблицам 4.9; политики используют ≥ `medium`; `low` — только отчёт и пунктир в графе. Последствия: ложное срабатывание не открывает трафик молча.

**ADR-кандидат: PBC = `app.kubernetes.io/part-of` (или `--pbc-label`), явное соответствие `--pbc` побеждает; продукт = `--chart-name`; раскладка по PBC — opt-in `--group-by pbc`.** Контекст: 5. Последствия: вывод без нового флага не меняется.

**ADR-кандидат: вложенный umbrella «продукт → PBC → сервис» без `repository` и с идентичными helpers.** Контекст: 4.12 (проверено на Helm 3.19.0). Последствия: `condition` и `global` работают на всех уровнях; одноимённые PBC и сервис допустимы.

**ADR-кандидат: `connection-policies` не ограничивает направление, которое нельзя выразить фактами.** Контекст: Ingress-контроллер, мониторинг, внешние FQDN не описаны во входе. Решение: `restrictIngress`/`restrictEgress` по умолчанию `false` в этих случаях с заметкой. Последствия: установка не ломает трафик; минимальность достигается заполнением values.

## 7. Влияние на существующее поведение

| Что | Изменение | Миграция |
|---|---|---|
| Все chart'ы без `--group-by pbc` и без `--with connection-policies` | **нет** (golden сравнение рендера до/после — задача 05 AC) | — |
| `dhg graph` | новые рёбра соединений и логические узлы на уровне `resource`; выключаются `--connections=false` | при необходимости старого вывода — `--connections=false` |
| `dhg generate` stderr | одна строка `Note: connections: …` при наличии соединений; подробности — с `-v` | — |
| `--with istio-egress` | хосты берутся из `Connections`: добавляются хосты из env ConfigMap/Secret, конфигурационных файлов, JDBC; уходят хосты из значений с плейсхолдерами | заметка в `RELEASE_NEXT.md` (задача 08) |
| `--env-values` в umbrella | subchart'ы получают профили по типу workload'а (раньше — статические, т.к. `chart.Name` = путь) | заметка в `RELEASE_NEXT.md` (задача 11) |
| `ResourceGraph` | новые поля; литералы `types.ResourceGraph{}` продолжают работать (ленивая инициализация) | — |

## 8. Тестирование

### 8.1. Фикстура `tests/integration/fixtures/pbc-shop/` (создаётся в задаче 05)

Все объекты — `namespace: shop`. Метки: `app.kubernetes.io/name: <service>`; `app.kubernetes.io/part-of` — как в таблице.

| Файл | Объекты | part-of | Ключевые факты |
|---|---|---|---|
| `order-api.yaml` | Deployment `order-api` (контейнер `app`, порт `http` 8080, `workingDir: /workspace`), Service `order-api` (port 8080, `targetPort: http`), ConfigMap `order-api-config` (ключ `application.yml`), Ingress `shop` → Service `order-api:8080` | orders | env: `SPRING_DATASOURCE_URL=jdbc:postgresql://orders-db.shop.svc.cluster.local:5432/orders`; `PAYMENTS_URL=http://payment-api:8080/api/v1`; `SPRING_KAFKA_BOOTSTRAP_SERVERS=my-cluster-kafka-bootstrap.kafka.svc:9092`; `SPRING_SECURITY_OAUTH2_RESOURCESERVER_JWT_ISSUER_URI=https://sso.example.com/realms/shop`; `SPRING_CONFIG_ADDITIONAL_LOCATION=optional:file:/config/`; `LOG_LEVEL=info`; `CALLBACK_URL=${CALLBACK_BASE}/cb`. ConfigMap смонтирован в `/config`; `application.yml`: `spring.cloud.stream.bindings.orderCreated-out-0.destination: shop.orders.created`, `spring.cloud.stream.bindings.paymentCompleted-in-0.destination: shop.payments.completed`, `spring.cloud.stream.bindings.paymentCompleted-in-0.group: order-api` |
| `order-worker.yaml` | Deployment `order-worker` (контейнер `worker`), без Service | orders | env: `KAFKA_BOOTSTRAP_SERVERS=my-cluster-kafka-bootstrap.kafka.svc:9092`; `ORDERS_TOPIC=shop.orders.created`; `ORDER_API_HOST=order-api`; `ORDER_API_PORT=8080` |
| `orders-db.yaml` | StatefulSet `orders-db` (порт 5432 `postgres`), Service `orders-db` (port 5432) | orders | — |
| `payment-api.yaml` | Deployment `payment-api` (контейнер `app`, порт 8080), Service `payment-api` (port 8080, targetPort 8080), ConfigMap `payment-api-env` (через `envFrom`), Secret `payment-api-secrets` | payments | ConfigMap: `BANK_API_URL=https://api.bank.example.com/v1`, `KAFKA_BOOTSTRAP_SERVERS=my-cluster-kafka-bootstrap.kafka.svc:9092`, `MP_MESSAGING_OUTGOING_PAYMENT_COMPLETED_TOPIC=shop.payments.completed`, `MP_MESSAGING_OUTGOING_PAYMENT_COMPLETED_CONNECTOR=smallrye-kafka`; Secret (`stringData`): `LEDGER_URL=postgresql://ledger:s3cret@10.20.0.5:5432/ledger` (env `valueFrom.secretKeyRef`) |
| `notifier.yaml` | Deployment `notifier` (контейнер `app`) | — | env: `NOTIFY_SMTP_HOST=smtp.example.com`, `NOTIFY_SMTP_PORT=587`, `PAYMENTS_EVENTS_TOPIC=shop.payments.completed` |
| `topics.yaml` | KafkaTopic `shop-orders-created` (`kafka.strimzi.io/v1`, `spec.topicName: shop.orders.created`, метки `strimzi.io/cluster: my-cluster`, `app.kubernetes.io/name: order-api`) | orders | — |

### 8.2. Ожидаемые соединения фикстуры (контракт для unit- и golden-тестов)

| # | From | Type | To | Protocol / порт | TargetPort | Confidence | Задача |
|---|---|---|---|---|---|---|---|
| C1 | Deployment/shop/order-api | service_call | Service/shop/orders-db | postgresql / 5432, `database=orders` | 5432 | high | 05 |
| C2 | Deployment/shop/order-api | service_call | Service/shop/payment-api | http / 8080, `Path=/api/v1` | 8080 | high (generic medium +1) | 05 |
| C3 | Deployment/shop/order-api | service_call | cluster-service:kafka/my-cluster-kafka-bootstrap | kafka / 9092, Role broker | — | high | 05 |
| C4 | Deployment/shop/order-api | external_call | external-host:sso.example.com | http TLS / 443 (default), Role oidc-issuer, `realm=shop` | — | high | 05 |
| C5 | Deployment/shop/order-worker | service_call | cluster-service:kafka/my-cluster-kafka-bootstrap | kafka / 9092 (правило каталога `kafka.bootstrap.servers`, env-форма) | — | high | 05 |
| C6 | Deployment/shop/order-worker | service_call | Service/shop/order-api | tcp / 8080 | `http` | high (generic medium +1) | 05 |
| C7 | Deployment/shop/payment-api | external_call | external-host:api.bank.example.com | http TLS / 443, Path `/v1` | — | medium | 05 |
| C8 | Deployment/shop/payment-api | service_call | cluster-service:kafka/my-cluster-kafka-bootstrap | kafka / 9092 (ConfigMap через `envFrom`) | — | high | 05 |
| C9 | Deployment/shop/payment-api | external_call | external-ip:10.20.0.5 | postgresql / 5432; Evidence.Value `""` (Secret) | — | medium | 05 |
| C10 | Deployment/shop/notifier | external_call | external-host:smtp.example.com | tcp / 587 | — | medium | 05 |
| T1 | Deployment/shop/order-api | topic_produce | topic:cluster:kafka/my-cluster-kafka-bootstrap:9092/shop.orders.created (`Resource` = KafkaTopic/shop/shop-orders-created) | kafka, `binding=orderCreated-out-0` | — | high | 06+07 |
| T2 | Deployment/shop/order-api | topic_consume | topic:…/shop.payments.completed | kafka, `group=order-api` | — | high | 06+07 |
| T3 | Deployment/shop/order-worker | topic_use | topic:…/shop.orders.created | kafka | — | medium | 07 |
| T4 | Deployment/shop/payment-api | topic_produce | topic:…/shop.payments.completed | kafka, `channel=PAYMENT_COMPLETED` | — | high | 07 |
| T5 | Deployment/shop/notifier | topic_use | topic:/shop.payments.completed → слит с узлом T4 (4.7 п.3) | `""`→kafka узла | — | medium | 07 |

Пропуски: `CALLBACK_URL` — `placeholder ${CALLBACK_BASE}`. Не классифицированы и молча игнорируются: `LOG_LEVEL`, `SPRING_CONFIG_ADDITIONAL_LOCATION` (нет правила, нет `://`).

Ожидаемые значения `connection-policies` (задача 14): `restrictIngress` — `false` у order-api (backend Ingress `shop`), `true` у остальных; `restrictEgress` — `false` у order-api (внешний issuer, пропуск-плейсхолдер), payment-api и notifier (внешние хосты), `true` у order-worker и orders-db.

Топология (задача 09): `orders` = {order-api, order-worker, orders-db}; `payments` = {payment-api}; без PBC — notifier. МежPBC: C2 (orders → payments, синхронный); топик `shop.payments.completed` (producer payments, consumer orders, use notifier) — асинхронный контракт, OwnerPBC `payments`; `shop.orders.created` — внутренний для orders (OwnerPBC `orders` по KafkaTopic).

### 8.3. Unit-тесты

- `pkg/types`: `AddConnection` слияние и индексы; `Confidence.AtLeast`; `Topology.PBCOf`.
- `pkg/connect`: табличные тесты на каждую строку таблиц 4.3–4.9 (не меньше одного кейса на правило каталога; отрицательные кейсы из 4.4.2 и 4.5); `Redact` не возвращает пароль ни в одном кейсе; fuzz-тест `FuzzParseValue` (без паник, вывод `Redact` не содержит подстроку пароля из userinfo).
- `pkg/analyzer/detector`: `ConnectionPass` на фикстуре (`testutil`-загрузка YAML) — ровно соединения таблицы 8.2 (по задачам).
- `pkg/generator`: `ResolveTopology`, umbrella/separate/library по PBC (структура `GeneratedChart`), `connection-policies` (шаблон и values).
- Производительность: `BenchmarkConnectionPass` на 1000 синтетических workload'ах × 30 env; тест-ограничитель `TestConnectionPassScales`: 1000 workload'ов обрабатываются < 2 с на runner'е CI (линейная сложность; индексы `Index` — map).

### 8.4. Golden

- Все существующие сценарии на всех входах + фикстура `pbc-shop` (подхватывается автоматически `inputs()`).
- Новые сценарии `scenarios` в `tests/golden/golden_test.go`: `umbrella-pbc` (`--mode umbrella --group-by pbc --include-schema`, fidelity), `separate-pbc`, `library-pbc` (задачи 10–11).
- `tests/golden/pbc_test.go`: факты рендера на `pbc-shop` — структура каталогов, `condition`, отключение сервиса через values продукта, NetworkPolicy `connection-policies` (задача 14).
- `featureInputs` дополняется `fixtures/pbc-shop` (задача 14).

## 9. Запросы к другим эпикам и контракты для них

| Эпик | Что использует | Что должен соблюдать |
|---|---|---|
| EPIC-02 (Kafka) | `analyzer.TopicUsages` (producer/consumer, `group`, `brokerID`, KafkaTopic входа, OwnerPBC); `Connection` с `Role: broker`, `TLS` | ACL строит только из `topic_produce`/`topic_consume` ≥ `medium`; `topic_use` → заметка, не ACL Read+Write. Новые ключи Kafka (например, `spring.kafka.properties.security.protocol`) добавляет правилом каталога 4.3.1 через правку этого документа. Детекция из `KafkaUser` ACL (если будет) — новый `EvidenceSource`, добавляется в 3.4 |
| EPIC-04 (CiliumNetworkPolicy) | `Connections` ≥ `medium`, `external-host` → `toFQDNs`; `Path`, `Protocol == http && !TLS` → L7 правила; `Topology` и `CrossPBCConnections` для границ | правила по `Ambiguous`/`low` не создавать; семантика `restrictIngress/restrictEgress` — как в 4.15, чтобы два вида политик вели себя одинаково |
| EPIC-08 (Keycloak) | `Connection{Role: oidc-issuer}`, `Attributes["issuer"]`, `["realm"]` | issuer'ы сравнивать по `Attributes["issuer"]`, не по `Evidence` |
| EPIC-09 (операторы данных) | `Connection{Role: database|cache}`, `Protocol`, `Attributes["database"]`, `To` (`cluster-service`/`input-service`) | замена `autodeps` (`knownDependencies` по префиксам env) — на `Connections`; эвристики по именам env не возвращать |
| Работа над Quarkus/Micronaut (ветка main, `pkg/synth/project.go`, `frameworks.go`) | `connect.EnvNameMP` = `synth.envName` | при появлении новых свойств-адресов в `frameworks.go` — добавить правило каталога (задача 15 сводит списки) |

## 10. Масштабирование и развитие

- **Сложность.** Сбор фактов — O(сумма env, ключей ConfigMap, свойств), разрешение — O(1) на адрес (map по `ns/name`), слияние — map по ключу соединения. Для 100+ микросервисов (≈ 3000 фактов) время пренебрежимо по сравнению с процессорами; тест-ограничитель — 8.3.
- **Объём вывода.** На уровне `resource` граф 100+ сервисов нечитаем; основной вид для продукта — `--level service --group-by pbc` или `--level pbc`.
- **Развитие:** чтение исходного кода (`@KafkaListener(topics=…)`, `@FeignClient(url=…)`) — отдельный эпик (нужен разбор Java; при литералах в аннотациях это факт); профили Spring как values-оверлеи; RabbitMQ (exchange/queue/routing key) как отдельные виды узлов; `KafkaUser` ACL как источник направления (EPIC-02); импорт PBC-карты из архитектурного репозитория (Structurizr DSL) — после решения владельца (README, вопросы).
