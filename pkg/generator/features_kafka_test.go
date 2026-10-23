package generator

import (
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"testing"
)

func TestApplyKafkaClient(t *testing.T) {
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
	}

	chart, err := applyKafkaClientFeature(chart, *fc)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if chart.ValuesYAML == "" {
		t.Fatal("kafkaClient values not generated")
	}

	if _, ok := chart.Templates["templates/kafka-secret.yaml"]; !ok {
		t.Fatal("kafka-secret.yaml not generated for external mode")
	}
}

func TestApplyKafkaClientStrimzi(t *testing.T) {
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
										"value": "cluster-kafka-bootstrap:9092",
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
	}

	chart, err := applyKafkaClientFeature(chart, *fc)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if chart.ValuesYAML == "" {
		t.Fatal("kafkaClient values not generated")
	}

	if _, ok := chart.Templates["templates/kafka-topic.yaml"]; !ok {
		t.Fatal("kafka-topic.yaml not generated for strimzi mode")
	}
	if _, ok := chart.Templates["templates/kafka-user.yaml"]; !ok {
		t.Fatal("kafka-user.yaml not generated for strimzi mode")
	}
}
