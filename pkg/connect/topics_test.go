package connect

import "testing"

func TestExtractTopic(t *testing.T) {
	tests := []struct {
		key      string
		val      string
		expected bool
	}{
		{"my.cool.topic", "events", true},
		{"SOME_TOPICS", "events,logs", true},
		{"mp.messaging.incoming.channel1.topic", "ch1", true},
		{"spring.cloud.stream.bindings.out.destination", "dest1", true},
		{"random.key", "val", false},
	}

	for _, tt := range tests {
		val, ok := ExtractTopic(tt.key, tt.val)
		if ok != tt.expected {
			t.Errorf("ExtractTopic(%q) ok=%v, want %v", tt.key, ok, tt.expected)
		}
		if ok && val != tt.val {
			t.Errorf("ExtractTopic(%q) val=%q, want %q", tt.key, val, tt.val)
		}
	}
}

func TestCleanTopicName(t *testing.T) {
	if CleanTopicName("${SOME_ENV}") != "" {
		t.Error("expected empty string for placeholder")
	}
	if CleanTopicName("topic1") != "topic1" {
		t.Error("expected topic1")
	}
	if CleanTopicName("{{ .Values.topic }}") != "" {
		t.Error("expected empty string for Helm template")
	}
}
