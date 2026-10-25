package generator

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestStrimziFeature(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: make(map[string]string),
	}

	graph := types.NewResourceGraph()
	res := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name": "app3",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name": "app3-container",
								"env": []interface{}{
									map[string]interface{}{
										"name":  "SPRING_KAFKA_BOOTSTRAP_SERVERS",
										"value": "b1:9092",
									},
									map[string]interface{}{
										"name":  "SPRING_KAFKA_TEMPLATE_DEFAULT_TOPIC",
										"value": "topic3",
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

	graph.Connections = []types.Connection{
		{
			From: types.ResourceKey{GVK: schema.GroupVersionKind{Kind: "Deployment"}, Name: "app1"},
			To:   types.ConnectionTarget{Resource: &types.ResourceKey{Name: "topic1"}},
			Type: types.RelationTopicProduce,
		},
		{
			From: types.ResourceKey{GVK: schema.GroupVersionKind{Kind: "Deployment"}, Name: "app2"},
			To:   types.ConnectionTarget{Resource: &types.ResourceKey{Name: "topic2"}},
			Type: types.RelationTopicUse, // Should be disabled
		},
	}

	var notes []string
	fc := FeatureContext{
		Graph: graph,
		Params: map[string]string{
			"cluster":   "my-cluster",
			"namespace": "kafka",
			"auth":      "scram-sha-512",
			"access":    "kafka-access",
		},
		Notes: &notes,
	}

	chart, err := applyStrimziFeature(chart, fc)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if _, ok := chart.Templates["templates/strimzi-entities.yaml"]; !ok {
		t.Fatal("templates/strimzi-entities.yaml not generated")
	}

	tmpl := chart.Templates["templates/strimzi-entities.yaml"]
	if !strings.Contains(tmpl, "KafkaTopic") {
		t.Error("KafkaTopic not generated in template")
	}
	if !strings.Contains(tmpl, "KafkaUser") {
		t.Error("KafkaUser not generated in template")
	}
	if !strings.Contains(tmpl, "KafkaAccess") {
		t.Error("KafkaAccess not generated in template")
	}

	hasNote := false
	for _, note := range notes {
		if strings.Contains(note, "adding with enabled: false") {
			hasNote = true
		}
	}

	if !hasNote {
		t.Error("Expected note for topic_use connection")
	}
}
