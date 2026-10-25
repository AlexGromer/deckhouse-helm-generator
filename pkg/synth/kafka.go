package synth

import (
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/kafkaclient"
)

// ExtractKafkaFacts converts project configuration (Spring, Quarkus, Micronaut)
// into Kafka facts.
func ExtractKafkaFacts(props map[string]string, originPrefix string) (kafkaclient.Facts, []string) {
	var kProps []kafkaclient.Property

	for k, v := range props {
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
		notes = append(notes, "Note: unresolved placeholder in Kafka property "+unresolved.Origin+": "+unresolved.Unresolved)
	}

	return facts, notes
}
