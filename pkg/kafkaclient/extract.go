package kafkaclient

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"regexp"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

var (
	passwordRedactRe = regexp.MustCompile(`(password=)(['"]?)[^'"\s]+(['"]?)`)
)

func redactJAAS(config string) (string, bool) {
	redacted := passwordRedactRe.ReplaceAllString(config, "${1}${2}***REDACTED***${3}")
	return redacted, redacted != config
}

func parseBootstrapServers(val string) []string {
	var servers []string
	for _, part := range strings.Split(val, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			servers = append(servers, part)
		}
	}
	return servers
}

func hasStrimzi(graph *types.ResourceGraph) bool {
	for key := range graph.Resources {
		if key.GVK.Kind == "KafkaTopic" || key.GVK.Kind == "KafkaUser" {
			return true
		}
	}
	return false
}

func isKafkaWorkload(key types.ResourceKey, res *types.ProcessedResource) bool {
	kind := key.GVK.Kind
	if kind == "" && res != nil && res.Original != nil && res.Original.Object != nil {
		kind = res.Original.Object.GetKind()
	}
	return kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet" || kind == "Job" || kind == "CronJob"
}

// Extract facts from graph.
func ExtractFromGraph(graph *types.ResourceGraph) *KafkaFacts {
	facts := &KafkaFacts{
		Mode: KafkaModeExternal,
	}

	if hasStrimzi(graph) {
		facts.Mode = KafkaModeStrimzi
	}

	// Iterate over graph resources
	for key, res := range graph.Resources {
		if !isKafkaWorkload(key, res) {
			continue
		}

		containers, _, _ := unstructured.NestedSlice(res.Original.Object.Object, "spec", "template", "spec", "containers")

		for _, c := range containers {
			envList, _, _ := unstructured.NestedSlice(c.(map[string]interface{}), "env")
			env := make(map[string]string)
			for _, e := range envList {
				envMap := e.(map[string]interface{})
				if name, ok := envMap["name"].(string); ok {
					if val, ok := envMap["value"].(string); ok {
						env[name] = val
					}
				}
			}

			clientFact := KafkaClientFact{
				WorkloadKey:   key,
				ContainerName: c.(map[string]interface{})["name"].(string),
			}

			hasKafkaVars := false

			for k, v := range env {
				kUpper := strings.ToUpper(k)

				// Handle Quarkus and MP Messaging
				if strings.HasPrefix(k, "mp.messaging.") || strings.HasPrefix(k, "quarkus.kafka.") {
					hasKafkaVars = true
					if strings.Contains(k, "topic") {
						clientFact.Topics = append(clientFact.Topics, KafkaTopicFact{Name: v, Op: KafkaTopicOpUnknown})
					}
				}

				// Handle Bootstrap Servers
				if strings.Contains(kUpper, "KAFKA") && (strings.HasSuffix(kUpper, "_BROKERS") || strings.HasSuffix(kUpper, "_BOOTSTRAP") || strings.HasSuffix(kUpper, "_SERVERS") || strings.HasSuffix(kUpper, "_HOSTS")) {
					clientFact.BootstrapServers = parseBootstrapServers(v)
					hasKafkaVars = true
				}
				if k == "spring.kafka.bootstrap-servers" || k == "kafka.bootstrap.servers" {
					clientFact.BootstrapServers = parseBootstrapServers(v)
					hasKafkaVars = true
				}

				// Handle Security Protocol
				if strings.Contains(kUpper, "SECURITY_PROTOCOL") || k == "spring.kafka.security.protocol" || k == "kafka.security.protocol" {
					clientFact.SecurityProtocol = v
					hasKafkaVars = true
				}

				// Handle SASL Mechanism
				if strings.Contains(kUpper, "SASL_MECHANISM") || strings.Contains(k, "sasl.mechanism") {
					clientFact.SaslMechanism = v
					hasKafkaVars = true
				}

				// Handle JAAS config
				if strings.Contains(kUpper, "JAAS_CONFIG") || strings.Contains(k, "jaas.config") {
					redacted, changed := redactJAAS(v)
					clientFact.SaslJaasConfig = redacted
					if changed {
						facts.Warnings = append(facts.Warnings, "Redacted password in SASL JAAS config for container "+c.(map[string]interface{})["name"].(string))
					}
					hasKafkaVars = true
				}

				// Handle Group ID
				if strings.Contains(kUpper, "GROUP_ID") || strings.Contains(k, "group.id") || strings.HasSuffix(kUpper, "_CONSUMER_GROUP") {
					clientFact.GroupID = v
					hasKafkaVars = true
				}

				// Handle Topics
				if strings.HasSuffix(kUpper, "_TOPIC") || strings.HasSuffix(kUpper, "_TOPICS") || strings.Contains(k, "topic") {
					for _, t := range parseBootstrapServers(v) {
						clientFact.Topics = append(clientFact.Topics, KafkaTopicFact{Name: t, Op: KafkaTopicOpUnknown})
					}
					hasKafkaVars = true
				}
			}

			if hasKafkaVars && len(clientFact.BootstrapServers) > 0 {
				facts.Clients = append(facts.Clients, clientFact)

				for _, bs := range clientFact.BootstrapServers {
					if strings.Contains(bs, "-kafka-bootstrap") {
						facts.Mode = KafkaModeStrimzi
					}
				}
			}
		}
	}

	return facts
}

func ExtractFromChart(chart *types.GeneratedChart, graph *types.ResourceGraph) *KafkaFacts {
	return ExtractFromGraph(graph)
}

func DetectFramework(props []Property) Framework {
	counts := map[Framework]int{}

	for _, p := range props {
		norm := NormalizeKey(p.Key)
		if strings.HasPrefix(norm, "spring") {
			counts[FrameworkSpring]++
		}
		if strings.HasPrefix(norm, "quarkus") || strings.HasPrefix(norm, "mpmessaging") {
			counts[FrameworkQuarkus]++
		}
		if strings.HasPrefix(norm, "micronaut") {
			counts[FrameworkMicronaut]++
		}
	}

	if counts[FrameworkSpring] > 0 {
		return FrameworkSpring
	}
	if counts[FrameworkQuarkus] > 0 {
		return FrameworkQuarkus
	}
	if counts[FrameworkMicronaut] > 0 {
		return FrameworkMicronaut
	}
	return FrameworkPlain
}

func ExtractFacts(props []Property, framework Framework) Facts {
	facts := Facts{Framework: framework}
	var topics []Topic

	// Two passes: facts/conventions, then guesses.
	// We keep track of which normal keys have been matched by facts/conventions.
	matchedNorms := make(map[string]bool)

	for _, p := range props {
		norm := NormalizeKey(p.Key)
		valStr := p.Value

		// If secret, we don't know the exact value to parse for topics/servers,
		// but we still process the key to detect JAAS config etc.
		if p.FromSecret {
			if strings.Contains(norm, "jaasconfig") {
				facts.JAASFromSecret = true
			}
			continue
		}

		if valStr == "" {
			continue
		}

		// Handle placeholders
		// Use regex from pkg/synth if available, or basic implementation
		var unresolved []string

		placeholderRe := regexp.MustCompile(`\$\{([^}:]+)(?::([^}]*))?\}`)
		resolvedVal := placeholderRe.ReplaceAllStringFunc(valStr, func(m string) string {
			sm := placeholderRe.FindStringSubmatch(m)
			if strings.Contains(m, ":") {
				return sm[2] // has default
			}
			unresolved = append(unresolved, sm[1])
			return ""
		})

		if len(unresolved) > 0 {
			for _, u := range unresolved {
				facts.Unresolved = append(facts.Unresolved, Value{Value: u, Unresolved: u, Origin: p.Origin})
			}
			continue
		}

		valStr = resolvedVal
		if valStr == "" {
			continue
		}

		matched := false

		// 1. Check fixed keys
		for _, fk := range fixedKeys {
			if fk.norm == norm {
				validFw := false
				for _, fw := range fk.frameworks {
					if fw == framework || fw == FrameworkPlain { // plain matches all in fallback
						validFw = true
						break
					}
				}
				if !validFw {
					continue
				}

				matched = true
				switch fk.kind {
				case kindBootstrap:
					for _, s := range parseBootstrapServersList(valStr) {
						facts.Bootstrap = append(facts.Bootstrap, Value{Value: s, Certainty: fk.certainty, Origin: p.Origin})
					}
				case kindSecurityProtocol:
					v := Value{Value: valStr, Certainty: fk.certainty, Origin: p.Origin}
					facts.SecurityProtocol = &v
				case kindSASLMechanism:
					v := Value{Value: valStr, Certainty: fk.certainty, Origin: p.Origin}
					facts.SASLMechanism = &v
				case kindTopic:
					for _, s := range parseBootstrapServersList(valStr) {
						topics = append(topics, Topic{Name: s, Direction: fk.direction, Value: Value{Certainty: fk.certainty, Origin: p.Origin}})
					}
				case kindGroup:
					facts.Groups = append(facts.Groups, Value{Value: valStr, Certainty: fk.certainty, Origin: p.Origin})
				case kindTransactionalIDPrefix:
					facts.TransactionalPrefixes = append(facts.TransactionalPrefixes, Value{Value: valStr, Certainty: fk.certainty, Origin: p.Origin})
				case kindStreamsAppID:
					v := Value{Value: valStr, Certainty: fk.certainty, Origin: p.Origin}
					facts.StreamsAppID = &v
				}
			}
		}

		// 2. Check pattern keys
		for _, pk := range patternKeys {
			if strings.HasPrefix(norm, pk.prefix) && strings.HasSuffix(norm, pk.suffix) {
				validFw := false
				for _, fw := range pk.frameworks {
					if fw == framework {
						validFw = true
						break
					}
				}
				if !validFw {
					continue
				}

				matched = true
				switch pk.kind {
				case kindBootstrap:
					for _, s := range parseBootstrapServersList(valStr) {
						facts.Bootstrap = append(facts.Bootstrap, Value{Value: s, Certainty: pk.certainty, Origin: p.Origin})
					}
				case kindTopic:
					dir := pk.direction
					if pk.direction == DirectionUnknown {
						// Spring cloud stream direction
						if strings.Contains(norm, "in0") || strings.Contains(norm, "in1") || strings.Contains(norm, "in2") {
							dir = DirectionConsume
						} else if strings.Contains(norm, "out0") || strings.Contains(norm, "out1") || strings.Contains(norm, "out2") {
							dir = DirectionProduce
						}
					}

					// If this is a connector=smallrye-kafka, the value is not the topic.
					// We need to parse the channel name.
					if strings.HasSuffix(pk.suffix, "connector") {
						if valStr == "smallrye-kafka" {
							channel := strings.TrimPrefix(norm, pk.prefix)
							channel = strings.TrimSuffix(channel, pk.suffix)
							// We can't know the exact case of channel here, but let's assume it's smallrye convention.
							// Usually, it's better to process all properties and use the original key.
							// For simplicity, we just add the channel name as lower case.
							topics = append(topics, Topic{Name: channel, Direction: dir, Value: Value{Certainty: pk.certainty, Origin: p.Origin}})
						}
					} else {
						for _, s := range parseBootstrapServersList(valStr) {
							topics = append(topics, Topic{Name: s, Direction: dir, Value: Value{Certainty: pk.certainty, Origin: p.Origin}})
						}
					}
				case kindGroup:
					facts.Groups = append(facts.Groups, Value{Value: valStr, Certainty: pk.certainty, Origin: p.Origin})
				case kindQuarkusPattern:
					if valStr == "true" {
						facts.Patterns = append(facts.Patterns, Value{Value: valStr, Certainty: pk.certainty, Origin: p.Origin})
					}
				case kindTransactionalID:
					facts.TransactionalIDs = append(facts.TransactionalIDs, Value{Value: valStr, Certainty: pk.certainty, Origin: p.Origin})
				case kindMicronautGroup:
					// Just mark as existing group
					group := strings.TrimPrefix(norm, pk.prefix)
					facts.Groups = append(facts.Groups, Value{Value: group, Certainty: pk.certainty, Origin: p.Origin})
				}
			}
		}

		if matched {
			matchedNorms[norm] = true
		}
	}

	// 3. Guesses pass (only for properties that didn't match facts/conventions)
	for _, p := range props {
		norm := NormalizeKey(p.Key)
		if matchedNorms[norm] {
			continue
		}

		valStr := p.Value
		if valStr == "" || p.FromSecret {
			continue
		}

		// Unresolved again just in case (already handled in pass 1 actually)
		if strings.Contains(valStr, "${") {
			continue
		}

		isEnv := isEnvForm(p.Key)

		// Guess bootstrap
		if isEnv && strings.Contains(p.Key, "KAFKA") && (strings.HasSuffix(p.Key, "_BROKERS") || strings.HasSuffix(p.Key, "_BOOTSTRAP") || strings.HasSuffix(p.Key, "_SERVERS") || strings.HasSuffix(p.Key, "_HOSTS")) {
			for _, s := range parseBootstrapServersList(valStr) {
				facts.Bootstrap = append(facts.Bootstrap, Value{Value: s, Certainty: CertaintyGuess, Origin: p.Origin})
			}
		}

		// Guess topic
		if strings.HasSuffix(norm, "topic") || strings.HasSuffix(norm, "topics") || (isEnv && (strings.HasSuffix(p.Key, "_TOPIC") || strings.HasSuffix(p.Key, "_TOPICS"))) {
			for _, s := range parseBootstrapServersList(valStr) {
				topics = append(topics, Topic{Name: s, Direction: DirectionUnknown, Value: Value{Certainty: CertaintyGuess, Origin: p.Origin}})
			}
		}

		// Guess group
		if isEnv && strings.Contains(p.Key, "KAFKA") && (strings.HasSuffix(p.Key, "_GROUP_ID") || strings.HasSuffix(p.Key, "_CONSUMER_GROUP")) {
			facts.Groups = append(facts.Groups, Value{Value: valStr, Certainty: CertaintyGuess, Origin: p.Origin})
		}
	}

	// Post processing
	facts.Topics = DeduplicateTopics(topics)

	// Deduplicate bootstrap servers, groups, etc if necessary

	return facts
}
