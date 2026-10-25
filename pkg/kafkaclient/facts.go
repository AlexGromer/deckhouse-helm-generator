package kafkaclient

import "github.com/AlexGromer/deckhouse-helm-generator/pkg/types"

type KafkaMode string

const (
	KafkaModeStrimzi  KafkaMode = "strimzi"
	KafkaModeExternal KafkaMode = "external"
)

type KafkaAuth string

const (
	KafkaAuthNone        KafkaAuth = "none"
	KafkaAuthScramSha512 KafkaAuth = "scram-sha-512"
	KafkaAuthScramSha256 KafkaAuth = "scram-sha-256"
	KafkaAuthTLS         KafkaAuth = "tls"
	KafkaAuthPlain       KafkaAuth = "plain"
	KafkaAuthOauth       KafkaAuth = "oauth"
)

type KafkaSecretRef struct {
	Name string
	Key  string
}

type KafkaTLS struct {
	TruststoreSecret KafkaSecretRef
	KeystoreSecret   KafkaSecretRef
}

type KafkaTopicOp string

const (
	KafkaTopicOpProduce KafkaTopicOp = "produce"
	KafkaTopicOpConsume KafkaTopicOp = "consume"
	KafkaTopicOpUnknown KafkaTopicOp = "unknown"
)

type KafkaTopicFact struct {
	Name string
	Op   KafkaTopicOp
}

type KafkaClientFact struct {
	WorkloadKey      types.ResourceKey
	ContainerName    string
	BootstrapServers []string
	SecurityProtocol string
	SaslMechanism    string
	SaslJaasConfig   string
	GroupID          string
	Topics           []KafkaTopicFact
}

type KafkaFacts struct {
	Mode     KafkaMode
	Auth     KafkaAuth
	TLS      KafkaTLS
	Clients  []KafkaClientFact
	Warnings []string
}

type Certainty string

const (
	CertaintyFact       Certainty = "fact"
	CertaintyConvention Certainty = "convention"
	CertaintyGuess      Certainty = "guess"
)

type Direction string

const (
	DirectionProduce Direction = "produce"
	DirectionConsume Direction = "consume"
	DirectionUnknown Direction = "unknown"
)

type Framework string

const (
	FrameworkSpring    Framework = "spring"
	FrameworkQuarkus   Framework = "quarkus"
	FrameworkMicronaut Framework = "micronaut"
	FrameworkPlain     Framework = "plain"
)

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
	Value     // certainty and origin of the name
}

type Facts struct {
	Framework             Framework
	Bootstrap             []Value
	SecurityProtocol      *Value
	SASLMechanism         *Value
	JAASFromSecret        bool
	Topics                []Topic // deduplicated by (Name, Direction)
	Groups                []Value
	TransactionalIDs      []Value // exact ids
	TransactionalPrefixes []Value
	StreamsAppID          *Value
	Patterns              []Value // topic regexes (Quarkus pattern=true): no literal ACL
	Unresolved            []Value // values with placeholders without default (Note:)
}
