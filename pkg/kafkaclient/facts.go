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
