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

// The notes name Kafka properties only, and come in the same order on every
// run.
func TestExtractKafkaFactsNotes(t *testing.T) {
	props := map[string]string{
		"spring.kafka.bootstrap-servers":      "${KAFKA_URL}",
		"spring.kafka.template.default-topic": "${TOPIC_A}",
		"spring.kafka.consumer.group-id":      "${GROUP}",
		"mp.messaging.incoming.in.topic":      "${TOPIC_B}",
		"server.port":                         "${PORT}",
		"spring.datasource.url":               "${DB_URL}",
		"app.name":                            "${APP}",
	}
	var first []string
	for i := 0; i < 20; i++ {
		_, notes := ExtractKafkaFacts(props, "file ")
		if first == nil {
			first = notes
			continue
		}
		if strings.Join(notes, "\n") != strings.Join(first, "\n") {
			t.Fatalf("run %d: notes differ:\n%v\n%v", i, notes, first)
		}
	}
	if len(first) != 4 {
		t.Errorf("want 4 notes (the Kafka properties), got %v", first)
	}
	for _, n := range first {
		if strings.Contains(n, "server.port") || strings.Contains(n, "datasource") || strings.Contains(n, "app.name") {
			t.Errorf("note about a property that is not Kafka's: %s", n)
		}
	}
	for i := 1; i < len(first); i++ {
		if first[i-1] > first[i] {
			t.Errorf("notes are not sorted: %v", first)
		}
	}
}
