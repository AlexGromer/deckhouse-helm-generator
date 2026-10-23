package connect

import (
	"testing"
)

func TestParseValue(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected int // number of endpoints
	}{
		{"JDBC URL multi-host", "jdbc:postgresql://h1:5432,h2:5432/db", 2},
		{"JDBC URL oracle", "jdbc:oracle:thin:@//host:1521/svc", 1},
		{"Kafka brokers", "kafka-0:9092,kafka-1:9092", 2},
		{"Standard URI", "https://example.com/api", 1},
		{"Bare host:port", "redis:6379", 1},
		{"Bare host", "mongodb", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseValue(tt.raw)
			if len(got) != tt.expected {
				t.Errorf("ParseValue(%q) returned %d endpoints, want %d", tt.raw, len(got), tt.expected)
			}
		})
	}
}

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"test.localhost", true},
		{"::1", true},
		{"0.0.0.0", true},
		{"example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := IsLoopback(tt.host); got != tt.want {
				t.Errorf("IsLoopback(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestRedact(t *testing.T) {
	tests := []struct {
		raw       string
		want      string
		hasSecret bool
	}{
		{"jdbc:postgresql://user:pass@localhost:5432/db", "jdbc:postgresql://user:REDACTED@localhost:5432/db", true},
		{"https://api.com?token=123", "https://api.com?token=REDACTED", true},
		{"redis://localhost:6379", "redis://localhost:6379", false},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, hadSecret := Redact(tt.raw)
			if got != tt.want {
				t.Errorf("Redact() got = %v, want %v", got, tt.want)
			}
			if hadSecret != tt.hasSecret {
				t.Errorf("Redact() hadSecret = %v, want %v", hadSecret, tt.hasSecret)
			}
		})
	}
}
