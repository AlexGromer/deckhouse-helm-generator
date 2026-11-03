package synth

import (
	"sort"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/kafkaclient"
)

// ExtractKafkaFacts converts project configuration (Spring, Quarkus, Micronaut)
// into Kafka facts.
func ExtractKafkaFacts(props map[string]string, originPrefix string) (kafkaclient.Facts, []string) {
	var kProps []kafkaclient.Property

	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := props[k]
		// Use original key for parsing, but resolve placeholders for value

		kProps = append(kProps, kafkaclient.Property{
			Key:    k,
			Value:  v, // we pass the raw value, kafkaclient will resolve it and capture unresolved ones
			Origin: originPrefix + k,
		})
	}

	fw := kafkaclient.DetectFramework(kProps)
	facts := kafkaclient.ExtractFacts(kProps, fw)

	var notes []string
	for _, unresolved := range facts.Unresolved {
		if !isKafkaKey(strings.TrimPrefix(unresolved.Origin, originPrefix)) {
			continue
		}
		notes = append(notes, "unresolved placeholder in Kafka property "+unresolved.Origin+": "+unresolved.Unresolved)
	}
	sort.Strings(notes)

	return facts, notes
}

// isKafkaKey reports whether a property key is Kafka's: kafka.*,
// spring.kafka.*, quarkus.kafka.*, micronaut.*.kafka.* and mp.messaging.*.
func isKafkaKey(key string) bool {
	key = strings.ToLower(key)
	return strings.HasPrefix(key, "kafka.") || strings.Contains(key, ".kafka.") || strings.HasPrefix(key, "mp.messaging.")
}
