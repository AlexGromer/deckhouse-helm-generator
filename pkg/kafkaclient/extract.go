package kafkaclient

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"regexp"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

var (
	passwordRedactRe = regexp.MustCompile(`(password=)(['"]?)[^'"\s]+(['"]?)`)
)

func redactJAAS(config string) (string, bool) {
	redacted := passwordRedactRe.ReplaceAllString(config, "${1}${2}***REDACTED***${3}")
	return redacted, redacted != config
}

func parseBootstrapServers(val string) []string {
	var servers []string
	for _, part := range strings.Split(val, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			servers = append(servers, part)
		}
	}
	return servers
}

func hasStrimzi(graph *types.ResourceGraph) bool {
	for key := range graph.Resources {
		if key.GVK.Kind == "KafkaTopic" || key.GVK.Kind == "KafkaUser" {
			return true
		}
	}
	return false
}

func isKafkaWorkload(key types.ResourceKey, res *types.ProcessedResource) bool {
	kind := key.GVK.Kind
	if kind == "" && res != nil && res.Original != nil && res.Original.Object != nil {
		kind = res.Original.Object.GetKind()
	}
	return kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet" || kind == "Job" || kind == "CronJob"
}

// Extract facts from graph.
func Extract(graph *types.ResourceGraph) *KafkaFacts {
	facts := &KafkaFacts{
		Mode: KafkaModeExternal,
	}

	if hasStrimzi(graph) {
		facts.Mode = KafkaModeStrimzi
	}

	// Iterate over graph resources
	for key, res := range graph.Resources {
		if !isKafkaWorkload(key, res) {
			continue
		}

		containers, _, _ := unstructured.NestedSlice(res.Original.Object.Object, "spec", "template", "spec", "containers")

		for _, c := range containers {
			envList, _, _ := unstructured.NestedSlice(c.(map[string]interface{}), "env")
			env := make(map[string]string)
			for _, e := range envList {
				envMap := e.(map[string]interface{})
				if name, ok := envMap["name"].(string); ok {
					if val, ok := envMap["value"].(string); ok {
						env[name] = val
					}
				}
			}

			clientFact := KafkaClientFact{
				WorkloadKey:   key,
				ContainerName: c.(map[string]interface{})["name"].(string),
			}

			hasKafkaVars := false

			for k, v := range env {
				kUpper := strings.ToUpper(k)

				// Handle Quarkus and MP Messaging
				if strings.HasPrefix(k, "mp.messaging.") || strings.HasPrefix(k, "quarkus.kafka.") {
					hasKafkaVars = true
					if strings.Contains(k, "topic") {
						clientFact.Topics = append(clientFact.Topics, KafkaTopicFact{Name: v, Op: KafkaTopicOpUnknown})
					}
				}

				// Handle Bootstrap Servers
				if strings.Contains(kUpper, "KAFKA") && (strings.HasSuffix(kUpper, "_BROKERS") || strings.HasSuffix(kUpper, "_BOOTSTRAP") || strings.HasSuffix(kUpper, "_SERVERS") || strings.HasSuffix(kUpper, "_HOSTS")) {
					clientFact.BootstrapServers = parseBootstrapServers(v)
					hasKafkaVars = true
				}
				if k == "spring.kafka.bootstrap-servers" || k == "kafka.bootstrap.servers" {
					clientFact.BootstrapServers = parseBootstrapServers(v)
					hasKafkaVars = true
				}

				// Handle Security Protocol
				if strings.Contains(kUpper, "SECURITY_PROTOCOL") || k == "spring.kafka.security.protocol" || k == "kafka.security.protocol" {
					clientFact.SecurityProtocol = v
					hasKafkaVars = true
				}

				// Handle SASL Mechanism
				if strings.Contains(kUpper, "SASL_MECHANISM") || strings.Contains(k, "sasl.mechanism") {
					clientFact.SaslMechanism = v
					hasKafkaVars = true
				}

				// Handle JAAS config
				if strings.Contains(kUpper, "JAAS_CONFIG") || strings.Contains(k, "jaas.config") {
					redacted, changed := redactJAAS(v)
					clientFact.SaslJaasConfig = redacted
					if changed {
						facts.Warnings = append(facts.Warnings, "Redacted password in SASL JAAS config for container "+c.(map[string]interface{})["name"].(string))
					}
					hasKafkaVars = true
				}

				// Handle Group ID
				if strings.Contains(kUpper, "GROUP_ID") || strings.Contains(k, "group.id") || strings.HasSuffix(kUpper, "_CONSUMER_GROUP") {
					clientFact.GroupID = v
					hasKafkaVars = true
				}

				// Handle Topics
				if strings.HasSuffix(kUpper, "_TOPIC") || strings.HasSuffix(kUpper, "_TOPICS") || strings.Contains(k, "topic") {
					for _, t := range parseBootstrapServers(v) {
						clientFact.Topics = append(clientFact.Topics, KafkaTopicFact{Name: t, Op: KafkaTopicOpUnknown})
					}
					hasKafkaVars = true
				}
			}

			if hasKafkaVars && len(clientFact.BootstrapServers) > 0 {
				facts.Clients = append(facts.Clients, clientFact)

				for _, bs := range clientFact.BootstrapServers {
					if strings.Contains(bs, "-kafka-bootstrap") {
						facts.Mode = KafkaModeStrimzi
					}
				}
			}
		}
	}

	return facts
}

func ExtractFromChart(chart *types.GeneratedChart, graph *types.ResourceGraph) *KafkaFacts {
	return Extract(graph)
}
