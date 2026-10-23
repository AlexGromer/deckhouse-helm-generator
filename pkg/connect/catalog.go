package connect

import (
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"strings"
)

type KeyValue struct {
	Key   string
	Value string
}

type Candidate struct {
	Key       string
	Value     string
	Endpoints []types.ConnectionEndpoint
}

func MatchProperty(key string) bool {
	if strings.Contains(key, "datasource.url") || strings.Contains(key, "redis.url") || strings.Contains(key, "mongodb.uri") || strings.Contains(key, "kafka.bootstrap") || strings.Contains(key, "amqp") {
		return true
	}
	return false
}

func MatchEnv(name string) bool {
	suffixes := []string{"_URL", "_URI", "_DSN", "_HOST", "_PORT", "_BOOTSTRAP_SERVERS", "_BROKERS", "_ADDR", "_ENDPOINT"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}
