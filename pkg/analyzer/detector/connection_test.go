package detector

import (
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestConnectionDetector_Run(t *testing.T) {
	graph := types.NewResourceGraph()

	// Add Service
	svcObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "my-service",
				"namespace": "default",
			},
		},
	}
	svcRes := &types.ProcessedResource{
		Original: &types.ExtractedResource{Object: svcObj, GVK: schema.GroupVersionKind{Kind: "Service"}},
	}
	graph.AddResource(svcRes)

	// Add Deployment
	depObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "my-app",
				"namespace": "default",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name": "app",
								"env": []interface{}{
									map[string]interface{}{
										"name":  "DB_HOST",
										"value": "my-service",
									},
									map[string]interface{}{
										"name":  "API_URL",
										"value": "https://api.external.com/v1",
									},
									map[string]interface{}{
										"name":  "KAFKA_BOOTSTRAP_SERVERS",
										"value": "kafka.internal:9092",
									},
									map[string]interface{}{
										"name":  "KAFKA_TOPIC_PRODUCE",
										"value": "my.events",
									},
									map[string]interface{}{
										"name":  "POSTGRES_DB",
										"value": "10.0.0.1",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	depRes := &types.ProcessedResource{
		Original: &types.ExtractedResource{Object: depObj, GVK: schema.GroupVersionKind{Kind: "Deployment"}},
	}
	graph.AddResource(depRes)

	detector := NewConnectionDetector()

	err := detector.Run(graph)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(graph.Connections) != 5 {
		t.Fatalf("Expected 5 connections, got %d", len(graph.Connections))
	}

	foundSvc := false
	foundExternal := false
	foundKafka := false
	foundTopic := false
	foundPg := false

	for _, c := range graph.Connections {
		if c.To.Resource != nil && c.To.Resource.Name == "my-service" {
			foundSvc = true
			if c.Type != types.RelationServiceCall {
				t.Errorf("Expected service_call type, got %v", c.Type)
			}
		} else if c.Type == types.RelationExternalCall && string(c.To.Node) == "external-host:api.external.com" {
			foundExternal = true
			if c.Protocol != "https" || !c.TLS {
				t.Errorf("Expected https / TLS, got %s / %v", c.Protocol, c.TLS)
			}
		} else if c.Type == types.RelationServiceCall && string(c.To.Node) == "cluster-service:kafka/kafka.internal" {
			foundKafka = true
			if c.Protocol != "kafka" && c.Protocol != "tcp" {
				t.Errorf("Expected kafka protocol/9092 port, got %s / %d", c.Protocol, c.Port)
			}
		} else if string(c.To.Node) == "topic::my.events" {
			foundTopic = true
			if c.Protocol != "kafka" {
				t.Errorf("Expected kafka protocol for topic, got %s", c.Protocol)
			}
		} else if c.Type == types.RelationExternalCall && string(c.To.Node) == "external-host:10.0.0.1" {
			foundPg = true
			if c.Protocol != "postgresql" {
				t.Errorf("Expected postgresql protocol for fallback, got %s", c.Protocol)
			}
		}
	}

	if !foundSvc {
		t.Errorf("Internal service connection not found")
	}
	if !foundExternal {
		t.Errorf("External HTTPS connection not found")
	}
	if !foundKafka {
		t.Errorf("Kafka broker connection not found")
	}
	if !foundTopic {
		t.Errorf("Kafka topic connection not found")
	}
	if !foundPg {
		t.Errorf("Postgres fallback connection not found")
	}
}
