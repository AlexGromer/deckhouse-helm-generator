package kafkaclient

import (
	"testing"
)

func TestNormalizeKey(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"spring.kafka.consumer.group-id", "springkafkaconsumergroupid"},
		{"SPRING_KAFKA_CONSUMER_GROUP_ID", "springkafkaconsumergroupid"},
		{"spring.kafka.consumer.groupId", "springkafkaconsumergroupid"},
		{"mp.messaging.incoming.prices.connector", "mpmessagingincomingpricesconnector"},
	}

	for _, tt := range tests {
		got := NormalizeKey(tt.in)
		if got != tt.want {
			t.Errorf("NormalizeKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsEnvForm(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"SPRING_KAFKA_CONSUMER_GROUP_ID", true},
		{"spring.kafka.consumer.group-id", false},
		{"APP_123_VAR", true},
		{"123_456", false},
		{"APP-VAR", false},
	}

	for _, tt := range tests {
		got := isEnvForm(tt.in)
		if got != tt.want {
			t.Errorf("isEnvForm(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
