package connect

import (
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"strings"
)

// ExtractTopic checks if a given key indicates a topic and returns its value.
func ExtractTopic(key string, value string) (string, bool) {
	lowerKey := strings.ToLower(key)

	if strings.HasSuffix(lowerKey, ".topic") ||
		strings.HasSuffix(lowerKey, ".topics") ||
		strings.HasSuffix(lowerKey, "_topic") ||
		strings.HasSuffix(lowerKey, "_topics") ||
		strings.HasSuffix(lowerKey, "_queue") {
		return value, true
	}

	if strings.HasPrefix(key, "mp.messaging.incoming.") && strings.HasSuffix(key, ".topic") {
		return value, true
	}

	if strings.HasPrefix(key, "mp.messaging.outgoing.") && strings.HasSuffix(key, ".topic") {
		return value, true
	}

	if strings.HasPrefix(key, "spring.cloud.stream.bindings.") && strings.HasSuffix(key, ".destination") {
		return value, true
	}

	return "", false
}

// CleanTopicName filters out template placeholders from topic names.
func CleanTopicName(raw string) string {
	if strings.Contains(raw, "${") && strings.Contains(raw, "}") {
		return ""
	}
	if strings.Contains(raw, "{{") && strings.Contains(raw, "}}") {
		return ""
	}
	return strings.TrimSpace(raw)
}

// BindTopic binds a topic string to a Kafka broker and optionally a KafkaTopic CR.
func BindTopic(idx *ClusterIndex, topicName string, bootstrapServers string, sourceNamespace string) types.ResolvedTarget {
	var target types.ConnectionTarget

	// First check if a KafkaTopic CR exists
	clusterName := ""
	if bootstrapServers != "" {
		if strings.Contains(bootstrapServers, "-kafka-bootstrap") {
			clusterName = strings.TrimSuffix(strings.Split(bootstrapServers, ".")[0], "-kafka-bootstrap")
		}
	}

	if key, ok := idx.KafkaTopic(clusterName, topicName, sourceNamespace); ok {
		target = types.ConnectionTarget{Resource: &key}
		return types.ResolvedTarget{
			Target: target,
			Role:   "topic",
		}
	}

	// If no CR exists, represent it as a logical node bound to the broker
	nodeID := "topic:" + topicName
	if bootstrapServers != "" {
		nodeID = "topic:cluster:" + clusterName + "/" + topicName
	}

	target = types.ConnectionTarget{Node: types.NodeID(nodeID)}
	return types.ResolvedTarget{
		Target: target,
		Role:   "logical-topic",
	}
}
