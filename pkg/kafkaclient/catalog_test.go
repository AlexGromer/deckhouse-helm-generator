package kafkaclient

import (
	"reflect"
	"testing"
)

func TestCatalogFacts(t *testing.T) {
	tests := []struct {
		name      string
		props     []Property
		fw        Framework
		wantFacts Facts
	}{
		{
			name: "bootstrap_fact",
			props: []Property{
				{Key: "spring.kafka.bootstrap-servers", Value: "b1:9093,b2:9093", Origin: "prop"},
			},
			fw: FrameworkSpring,
			wantFacts: Facts{
				Framework: FrameworkSpring,
				Bootstrap: []Value{
					{Value: "b1:9093", Certainty: CertaintyFact, Origin: "prop"},
					{Value: "b2:9093", Certainty: CertaintyFact, Origin: "prop"},
				},
			},
		},
		{
			name: "bootstrap_env_fact",
			props: []Property{
				{Key: "SPRING_KAFKA_BOOTSTRAP_SERVERS", Value: "b1:9093", Origin: "env"},
			},
			fw: FrameworkSpring,
			wantFacts: Facts{
				Framework: FrameworkSpring,
				Bootstrap: []Value{
					{Value: "b1:9093", Certainty: CertaintyFact, Origin: "env"},
				},
			},
		},
		{
			name: "spring_cloud_stream",
			props: []Property{
				{Key: "spring.cloud.stream.bindings.process-in-0.destination", Value: "orders,payments", Origin: "p1"},
				{Key: "spring.cloud.stream.bindings.process-in-0.group", Value: "billing", Origin: "p2"},
			},
			fw: FrameworkSpring,
			wantFacts: Facts{
				Framework: FrameworkSpring,
				Topics: []Topic{
					{Name: "orders", Direction: DirectionConsume, Value: Value{Certainty: CertaintyFact, Origin: "p1"}},
					{Name: "payments", Direction: DirectionConsume, Value: Value{Certainty: CertaintyFact, Origin: "p1"}},
				},
				Groups: []Value{
					{Value: "billing", Certainty: CertaintyFact, Origin: "p2"},
				},
			},
		},
		{
			name: "quarkus_connector",
			props: []Property{
				{Key: "mp.messaging.incoming.prices.connector", Value: "smallrye-kafka", Origin: "p1"},
				{Key: "quarkus.application.name", Value: "pricing", Origin: "p2"},
			},
			fw: FrameworkQuarkus,
			wantFacts: Facts{
				Framework: FrameworkQuarkus,
				Topics: []Topic{
					{Name: "prices", Direction: DirectionConsume, Value: Value{Certainty: CertaintyConvention, Origin: "p1"}},
				},
				Groups: []Value{
					{Value: "pricing", Certainty: CertaintyConvention, Origin: "p2"},
				},
			},
		},
		{
			name: "quarkus_pattern",
			props: []Property{
				{Key: "mp.messaging.incoming.all.topic", Value: "orders-.*", Origin: "p1"},
				{Key: "mp.messaging.incoming.all.pattern", Value: "true", Origin: "p2"},
			},
			fw: FrameworkQuarkus,
			wantFacts: Facts{
				Framework: FrameworkQuarkus,
				Topics: []Topic{
					{Name: "orders-.*", Direction: DirectionConsume, Value: Value{Certainty: CertaintyFact, Origin: "p1"}},
				},
				Patterns: []Value{
					{Value: "true", Certainty: CertaintyFact, Origin: "p2"},
				},
			},
		},
		{
			name: "unresolved_placeholder",
			props: []Property{
				{Key: "spring.kafka.template.default-topic", Value: "${ORDERS_TOPIC}", Origin: "p1"},
			},
			fw: FrameworkSpring,
			wantFacts: Facts{
				Framework: FrameworkSpring,
				Unresolved: []Value{
					{Value: "ORDERS_TOPIC", Unresolved: "ORDERS_TOPIC", Origin: "p1"},
				},
			},
		},
		{
			name: "resolved_placeholder",
			props: []Property{
				{Key: "spring.kafka.template.default-topic", Value: "${ORDERS_TOPIC:orders}", Origin: "p1"},
			},
			fw: FrameworkSpring,
			wantFacts: Facts{
				Framework: FrameworkSpring,
				Topics: []Topic{
					{Name: "orders", Direction: DirectionProduce, Value: Value{Certainty: CertaintyFact, Origin: "p1"}},
				},
			},
		},
		{
			name: "guess_topic",
			props: []Property{
				{Key: "APP_ORDERS_TOPIC", Value: "orders", Origin: "p1"},
			},
			fw: FrameworkPlain,
			wantFacts: Facts{
				Framework: FrameworkPlain,
				Topics: []Topic{
					{Name: "orders", Direction: DirectionUnknown, Value: Value{Certainty: CertaintyGuess, Origin: "p1"}},
				},
			},
		},
		{
			name: "security_protocol",
			props: []Property{
				{Key: "spring.kafka.properties.sasl.mechanism", Value: "SCRAM-SHA-512", Origin: "p1"},
				{Key: "spring.kafka.security.protocol", Value: "SASL_SSL", Origin: "p2"},
			},
			fw: FrameworkSpring,
			wantFacts: Facts{
				Framework:        FrameworkSpring,
				SASLMechanism:    &Value{Value: "SCRAM-SHA-512", Certainty: CertaintyFact, Origin: "p1"},
				SecurityProtocol: &Value{Value: "SASL_SSL", Certainty: CertaintyFact, Origin: "p2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractFacts(tt.props, tt.fw)
			if !reflect.DeepEqual(got, tt.wantFacts) {
				t.Errorf("ExtractFacts() = %+v, want %+v", got, tt.wantFacts)
			}
		})
	}
}

func TestDedupPrefersFact(t *testing.T) {
	topics := []Topic{
		{Name: "orders", Direction: DirectionProduce, Value: Value{Certainty: CertaintyGuess, Origin: "guess"}},
		{Name: "orders", Direction: DirectionProduce, Value: Value{Certainty: CertaintyFact, Origin: "fact"}},
	}
	deduped := DeduplicateTopics(topics)
	if len(deduped) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(deduped))
	}
	if deduped[0].Certainty != CertaintyFact {
		t.Errorf("expected fact certainty, got %s", deduped[0].Certainty)
	}
}
