package kafkaclient

import (
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"testing"
)

func TestExtract(t *testing.T) {
	graph := types.NewResourceGraph()

	// Create a dummy workload resource
	res := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name": "test-app",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name": "app-container",
								"env": []interface{}{
									map[string]interface{}{
										"name":  "SPRING_KAFKA_BOOTSTRAP_SERVERS",
										"value": "kafka:9092",
									},
									map[string]interface{}{
										"name":  "SPRING_KAFKA_PROPERTIES_SASL_JAAS_CONFIG",
										"value": "org.apache.kafka.common.security.scram.ScramLoginModule required username=\"user\" password=\"secret_pass\";",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	processedRes := &types.ProcessedResource{
		Original: &types.ExtractedResource{Object: res},
	}
	graph.AddResource(processedRes)

	facts := Extract(graph)

	if facts.Mode != KafkaModeExternal {
		t.Errorf("Expected External mode, got %v", facts.Mode)
	}

	if len(facts.Clients) != 1 {
		t.Fatalf("Expected 1 client, got %d", len(facts.Clients))
	}

	client := facts.Clients[0]
	if len(client.BootstrapServers) != 1 || client.BootstrapServers[0] != "kafka:9092" {
		t.Errorf("Unexpected bootstrap servers: %v", client.BootstrapServers)
	}

	if client.SaslJaasConfig != "org.apache.kafka.common.security.scram.ScramLoginModule required username=\"user\" password=\"***REDACTED***\";" {
		t.Errorf("Password was not redacted correctly. Got: %s", client.SaslJaasConfig)
	}
}

func TestExtractFromChart(t *testing.T) {
	graph := types.NewResourceGraph()
	chart := &types.GeneratedChart{}
	facts := ExtractFromChart(chart, graph)
	if facts.Mode != KafkaModeExternal {
		t.Errorf("Expected External mode, got %v", facts.Mode)
	}
}
