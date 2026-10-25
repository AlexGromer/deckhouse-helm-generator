package kafkaclient

import "strings"

type catalogKind int

const (
	kindBootstrap catalogKind = iota
	kindSecurityProtocol
	kindSASLMechanism
	kindTrustStoreType
	kindKeyStoreType
	kindTopic
	kindGroup
	kindTransactionalIDPrefix
	kindStreamsAppID
	kindQuarkusPattern
	kindTransactionalID
	kindMicronautGroup
)

type fixedKey struct {
	norm       string
	kind       catalogKind
	frameworks []Framework
	certainty  Certainty
	direction  Direction // for topic kind
}

type patternKey struct {
	prefix     string
	suffix     string
	kind       catalogKind
	frameworks []Framework
	certainty  Certainty
	direction  Direction
}

var allFrameworks = []Framework{FrameworkSpring, FrameworkQuarkus, FrameworkMicronaut, FrameworkPlain}
var qmFrameworks = []Framework{FrameworkQuarkus, FrameworkMicronaut}

var fixedKeys = []fixedKey{
	{norm: "springkafkabootstrapservers", kind: kindBootstrap, frameworks: []Framework{FrameworkSpring}, certainty: CertaintyFact},
	{norm: "springkafkaconsumerbootstrapservers", kind: kindBootstrap, frameworks: []Framework{FrameworkSpring}, certainty: CertaintyFact},
	{norm: "springkafkaproducerbootstrapservers", kind: kindBootstrap, frameworks: []Framework{FrameworkSpring}, certainty: CertaintyFact},
	{norm: "springkafkaadminbootstrapservers", kind: kindBootstrap, frameworks: []Framework{FrameworkSpring}, certainty: CertaintyFact},
	{norm: "springkafkastreamsbootstrapservers", kind: kindBootstrap, frameworks: []Framework{FrameworkSpring}, certainty: CertaintyFact},
	{norm: "springcloudstreamkafkabinderbrokers", kind: kindBootstrap, frameworks: []Framework{FrameworkSpring}, certainty: CertaintyFact},
	{norm: "kafkabootstrapservers", kind: kindBootstrap, frameworks: qmFrameworks, certainty: CertaintyFact},
	{norm: "mpmessagingconnectorsmallryekafkabootstrapservers", kind: kindBootstrap, frameworks: []Framework{FrameworkQuarkus}, certainty: CertaintyFact},
	{norm: "kafkabootstrapservers", kind: kindBootstrap, frameworks: []Framework{FrameworkPlain}, certainty: CertaintyConvention},

	{norm: "springkafkasecurityprotocol", kind: kindSecurityProtocol, frameworks: allFrameworks, certainty: CertaintyFact},
	{norm: "springkafkapropertiessecurityprotocol", kind: kindSecurityProtocol, frameworks: allFrameworks, certainty: CertaintyFact},
	{norm: "springcloudstreamkafkabinderconfigurationsecurityprotocol", kind: kindSecurityProtocol, frameworks: allFrameworks, certainty: CertaintyFact},
	{norm: "kafkasecurityprotocol", kind: kindSecurityProtocol, frameworks: allFrameworks, certainty: CertaintyFact},
	{norm: "mpmessagingconnectorsmallryekafkasecurityprotocol", kind: kindSecurityProtocol, frameworks: allFrameworks, certainty: CertaintyFact},

	{norm: "springkafkapropertiessaslmechanism", kind: kindSASLMechanism, frameworks: allFrameworks, certainty: CertaintyFact},
	{norm: "springcloudstreamkafkabinderconfigurationsaslmechanism", kind: kindSASLMechanism, frameworks: allFrameworks, certainty: CertaintyFact},
	{norm: "kafkasaslmechanism", kind: kindSASLMechanism, frameworks: allFrameworks, certainty: CertaintyFact},
	{norm: "mpmessagingconnectorsmallryekafkasaslmechanism", kind: kindSASLMechanism, frameworks: allFrameworks, certainty: CertaintyFact},

	{norm: "springkafkassltruststoretype", kind: kindTrustStoreType, frameworks: allFrameworks, certainty: CertaintyFact},
	{norm: "kafkassltruststoretype", kind: kindTrustStoreType, frameworks: allFrameworks, certainty: CertaintyFact},
	{norm: "springkafkasslkeystoretype", kind: kindKeyStoreType, frameworks: allFrameworks, certainty: CertaintyFact},
	{norm: "kafkasslkeystoretype", kind: kindKeyStoreType, frameworks: allFrameworks, certainty: CertaintyFact},

	{norm: "springkafkatemplatedefaulttopic", kind: kindTopic, frameworks: []Framework{FrameworkSpring}, certainty: CertaintyFact, direction: DirectionProduce},
	{norm: "springkafkaconsumergroupid", kind: kindGroup, frameworks: []Framework{FrameworkSpring}, certainty: CertaintyFact},
	{norm: "springkafkaproducertransactionidprefix", kind: kindTransactionalIDPrefix, frameworks: []Framework{FrameworkSpring}, certainty: CertaintyFact},
	{norm: "springkafkatemplatetransactionidprefix", kind: kindTransactionalIDPrefix, frameworks: []Framework{FrameworkSpring}, certainty: CertaintyFact},
	{norm: "springkafkastreamsapplicationid", kind: kindStreamsAppID, frameworks: []Framework{FrameworkSpring}, certainty: CertaintyFact},

	// Quarkus default fallback
	{norm: "quarkusapplicationname", kind: kindGroup, frameworks: []Framework{FrameworkQuarkus}, certainty: CertaintyConvention},
}

var patternKeys = []patternKey{
	{prefix: "mpmessagingincoming", suffix: "bootstrapservers", kind: kindBootstrap, frameworks: []Framework{FrameworkQuarkus}, certainty: CertaintyFact},
	{prefix: "mpmessagingoutgoing", suffix: "bootstrapservers", kind: kindBootstrap, frameworks: []Framework{FrameworkQuarkus}, certainty: CertaintyFact},

	{prefix: "springcloudstreambindings", suffix: "destination", kind: kindTopic, frameworks: []Framework{FrameworkSpring}, certainty: CertaintyFact, direction: DirectionUnknown},
	{prefix: "springcloudstreambindings", suffix: "group", kind: kindGroup, frameworks: []Framework{FrameworkSpring}, certainty: CertaintyFact},

	{prefix: "mpmessagingincoming", suffix: "topic", kind: kindTopic, frameworks: []Framework{FrameworkQuarkus}, certainty: CertaintyFact, direction: DirectionConsume},
	{prefix: "mpmessagingincoming", suffix: "topics", kind: kindTopic, frameworks: []Framework{FrameworkQuarkus}, certainty: CertaintyFact, direction: DirectionConsume},
	{prefix: "mpmessagingoutgoing", suffix: "topic", kind: kindTopic, frameworks: []Framework{FrameworkQuarkus}, certainty: CertaintyFact, direction: DirectionProduce},

	{prefix: "mpmessagingincoming", suffix: "pattern", kind: kindQuarkusPattern, frameworks: []Framework{FrameworkQuarkus}, certainty: CertaintyFact},

	{prefix: "mpmessagingincoming", suffix: "connector", kind: kindTopic, frameworks: []Framework{FrameworkQuarkus}, certainty: CertaintyConvention, direction: DirectionConsume},
	{prefix: "mpmessagingoutgoing", suffix: "connector", kind: kindTopic, frameworks: []Framework{FrameworkQuarkus}, certainty: CertaintyConvention, direction: DirectionProduce},

	{prefix: "mpmessagingincoming", suffix: "groupid", kind: kindGroup, frameworks: []Framework{FrameworkQuarkus}, certainty: CertaintyFact},
	{prefix: "mpmessagingoutgoing", suffix: "transactionalid", kind: kindTransactionalID, frameworks: []Framework{FrameworkQuarkus}, certainty: CertaintyFact},

	{prefix: "kafkaconsumers", suffix: "", kind: kindMicronautGroup, frameworks: []Framework{FrameworkMicronaut}, certainty: CertaintyConvention},
}

func parseBootstrapServersList(val string) []string {
	var servers []string
	for _, part := range strings.Split(val, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			servers = append(servers, part)
		}
	}
	return servers
}

func DeduplicateTopics(topics []Topic) []Topic {
	if len(topics) == 0 {
		return nil
	}

	type key struct {
		name string
		dir  Direction
	}

	seen := make(map[key]Topic)
	var orderedKeys []key

	for _, t := range topics {
		k := key{name: t.Name, dir: t.Direction}
		if existing, found := seen[k]; found {
			// keep the one with higher certainty
			if t.Certainty == CertaintyFact && existing.Certainty != CertaintyFact {
				seen[k] = t
			} else if t.Certainty == CertaintyConvention && existing.Certainty == CertaintyGuess {
				seen[k] = t
			}
		} else {
			seen[k] = t
			orderedKeys = append(orderedKeys, k)
		}
	}

	var res []Topic
	for _, k := range orderedKeys {
		res = append(res, seen[k])
	}
	return res
}
