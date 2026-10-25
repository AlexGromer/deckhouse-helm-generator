package generator

import (
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestKafkaNetpol_WithClients(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: make(map[string]string),
	}

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

	fc := &FeatureContext{
		Graph: graph,
		Params: map[string]string{
			"broker-namespace": "kafka",
			"broker-pod-label": "strimzi.io/kind=Kafka",
			"ports":            "9092,9093",
		},
	}

	chart, err := applyKafkaNetpolFeature(chart, *fc)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if chart.ValuesYAML == "" {
		t.Fatal("kafkaNetworkPolicy values not generated")
	}

	if _, ok := chart.Templates["templates/kafka-networkpolicy.yaml"]; !ok {
		t.Fatal("kafka-networkpolicy.yaml not generated for kafka clients")
	}
}

func TestKafkaNetpol_WithoutClients(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: make(map[string]string),
	}

	graph := types.NewResourceGraph()
	// Empty graph, no clients

	fc := &FeatureContext{
		Graph: graph,
		Params: map[string]string{
			"broker-namespace": "kafka",
			"broker-pod-label": "strimzi.io/kind=Kafka",
			"ports":            "9092,9093",
		},
	}

	chart, err := applyKafkaNetpolFeature(chart, *fc)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if chart.ValuesYAML != "" {
		t.Fatal("kafkaNetworkPolicy values should not be generated")
	}

	if _, ok := chart.Templates["templates/kafka-networkpolicy.yaml"]; ok {
		t.Fatal("kafka-networkpolicy.yaml should not be generated without kafka clients")
	}
}
