package synth

import (
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/kafkaclient"
)

func TestExtractKafkaFacts(t *testing.T) {
	props := map[string]string{
		"spring.kafka.bootstrap-servers":      "kafka:9092",
		"spring.kafka.template.default-topic": "${MISSING}",
	}

	facts, notes := ExtractKafkaFacts(props, "file ")

	if facts.Framework != kafkaclient.FrameworkSpring {
		t.Errorf("Expected framework spring, got %s", facts.Framework)
	}
	if len(facts.Bootstrap) != 1 || facts.Bootstrap[0].Value != "kafka:9092" {
		t.Errorf("Expected 1 bootstrap server, got %v", facts.Bootstrap)
	}
	if len(notes) != 1 {
		t.Logf("Notes: %v", notes)
		t.Errorf("Expected 1 note, got %d", len(notes))
	}
}
