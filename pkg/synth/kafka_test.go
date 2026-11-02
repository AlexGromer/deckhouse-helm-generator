package synth

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/connect"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/kafkaclient"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
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

func TestSynthNotes_NoNotePrefix(t *testing.T) {
	// The caller prints "Note: <msg>"; a prefix at the source would double it.
	_, kafkaNotes := ExtractKafkaFacts(map[string]string{"spring.kafka.bootstrap-servers": "${KAFKA_URL}"}, "file ")
	_, connNotes := ExtractSynthConnections(map[string]string{"spring.datasource.url": "${DB_URL}"},
		types.ResourceKey{}, "app", connect.NewIndex(nil), types.NewResourceGraph())
	for name, notes := range map[string][]string{"kafka": kafkaNotes, "connections": connNotes} {
		if len(notes) == 0 {
			t.Errorf("%s: no notes produced", name)
		}
		for _, n := range notes {
			if strings.HasPrefix(n, "Note:") {
				t.Errorf("%s: note carries its own prefix, the caller prints it: %q", name, n)
			}
		}
	}
}
