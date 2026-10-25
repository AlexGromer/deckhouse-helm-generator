package connect

import (
	"testing"
)

func TestExtract(t *testing.T) {
	entries := []KeyValue{
		{Key: "SPRING_DATASOURCE_URL", Value: "jdbc:postgresql://db:5432/orders"},
		{Key: "SOME_VAR", Value: "some_value"}, // this won't match keys
		{Key: "KAFKA_BOOTSTRAP_SERVERS", Value: "kafka-0:9092,kafka-1:9092"},
	}

	candidates := Extract(entries)

	if len(candidates) != 2 {
		t.Errorf("Expected 2 candidates, got %d", len(candidates))
	}

	if candidates[0].Key != "SPRING_DATASOURCE_URL" {
		t.Errorf("Expected first candidate to be SPRING_DATASOURCE_URL, got %s", candidates[0].Key)
	}
}
