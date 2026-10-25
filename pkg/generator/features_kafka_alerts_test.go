package generator

import (
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestKafkaAlerts_WithClients(t *testing.T) {
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
									map[string]interface{}{
										"name":  "SPRING_KAFKA_CONSUMER_GROUP_ID",
										"value": "test-group",
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
			"lag-threshold": "1000",
			"for":           "5m",
			"severity":      "warning",
		},
	}

	chart, err := applyKafkaAlertsFeature(chart, *fc)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if chart.ValuesYAML == "" {
		t.Fatal("kafkaAlerts values not generated")
	}

	if _, ok := chart.Templates["templates/kafka-prometheusrules.yaml"]; !ok {
		t.Fatal("kafka-prometheusrules.yaml not generated for kafka clients")
	}
}

func TestKafkaAlerts_WithoutClients(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: make(map[string]string),
	}

	graph := types.NewResourceGraph()
	// Empty graph, no clients

	fc := &FeatureContext{
		Graph: graph,
		Params: map[string]string{
			"lag-threshold": "1000",
			"for":           "5m",
			"severity":      "warning",
		},
	}

	chart, err := applyKafkaAlertsFeature(chart, *fc)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if chart.ValuesYAML != "" {
		t.Fatal("kafkaAlerts values should not be generated")
	}

	if _, ok := chart.Templates["templates/kafka-prometheusrules.yaml"]; ok {
		t.Fatal("kafka-prometheusrules.yaml should not be generated without kafka clients")
	}
}
