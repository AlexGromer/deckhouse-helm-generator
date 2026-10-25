package connect

import (
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"testing"
)

func TestClusterIndexResolve(t *testing.T) {
	svcObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      "mysvc",
				"namespace": "myns",
			},
			"spec": map[string]interface{}{
				"clusterIP": "10.0.0.1",
				"selector": map[string]interface{}{
					"app": "my-app",
				},
			},
		},
	}

	depObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "mydep",
				"namespace": "myns",
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"app": "my-app",
						},
					},
				},
			},
		},
	}

	ingressObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "Ingress",
			"metadata": map[string]interface{}{
				"name":      "myingress",
				"namespace": "myns",
			},
			"spec": map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{
						"host": "my-app.example.com",
					},
				},
			},
		},
	}

	rSvc := &types.ExtractedResource{Object: svcObj, GVK: svcObj.GroupVersionKind()}
	pSvc := &types.ProcessedResource{Original: rSvc}

	rDep := &types.ExtractedResource{Object: depObj, GVK: depObj.GroupVersionKind()}
	pDep := &types.ProcessedResource{Original: rDep}

	rIngress := &types.ExtractedResource{Object: ingressObj, GVK: ingressObj.GroupVersionKind()}
	pIngress := &types.ProcessedResource{Original: rIngress}

	idx := NewIndex([]*types.ProcessedResource{pSvc, pDep, pIngress})

	// Test IP
	res := idx.Resolve(types.ConnectionEndpoint{Host: "10.0.0.1"}, "myns")
	if res.Role != "service" || res.Target.Resource == nil || res.Target.Resource.Name != "mysvc" {
		t.Errorf("expected mysvc by IP, got %v", res)
	}

	// Test short name
	res = idx.Resolve(types.ConnectionEndpoint{Host: "mysvc"}, "myns")
	if res.Role != "service" || res.Target.Resource == nil || res.Target.Resource.Name != "mysvc" {
		t.Errorf("expected mysvc by short name, got %v", res)
	}

	// Test FQDN
	res = idx.Resolve(types.ConnectionEndpoint{Host: "mysvc.myns.svc.cluster.local"}, "otherns")
	if res.Role != "service" || res.Target.Resource == nil || res.Target.Resource.Name != "mysvc" {
		t.Errorf("expected mysvc by FQDN, got %v", res)
	}

	// Test headless pod FQDN
	res = idx.Resolve(types.ConnectionEndpoint{Host: "mypod.mysvc.myns.svc"}, "otherns")
	if res.Role != "service" || res.Target.Resource == nil || res.Target.Resource.Name != "mysvc" {
		t.Errorf("expected mysvc by headless FQDN, got %v", res)
	}

	// Test Ingress
	res = idx.Resolve(types.ConnectionEndpoint{Host: "my-app.example.com"}, "myns")
	if res.Role != "ingress" || res.Target.Resource == nil || res.Target.Resource.Name != "myingress" {
		t.Errorf("expected myingress by host, got %v", res)
	}

	// Test SelectsWorkload
	workloads := idx.SelectsWorkload(*idx.Resolve(types.ConnectionEndpoint{Host: "mysvc"}, "myns").Target.Resource)
	if len(workloads) != 1 || workloads[0].Name != "mydep" {
		t.Errorf("expected mydep selected, got %v", workloads)
	}

	// Test external
	res = idx.Resolve(types.ConnectionEndpoint{Host: "api.github.com"}, "myns")
	if res.Role != "external" {
		t.Errorf("expected external, got %v", res)
	}
}

func TestClusterIndexKafkaTopics(t *testing.T) {
	topicObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kafka.strimzi.io/v1beta2",
			"kind":       "KafkaTopic",
			"metadata": map[string]interface{}{
				"name":      "my-topic-cr",
				"namespace": "myns",
				"labels": map[string]interface{}{
					"strimzi.io/cluster": "my-cluster",
				},
			},
			"spec": map[string]interface{}{
				"topicName": "actual.topic.name",
			},
		},
	}

	kafkaObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kafka.strimzi.io/v1beta2",
			"kind":       "Kafka",
			"metadata": map[string]interface{}{
				"name":      "my-cluster",
				"namespace": "myns",
			},
		},
	}

	rTopic := &types.ExtractedResource{Object: topicObj, GVK: topicObj.GroupVersionKind()}
	pTopic := &types.ProcessedResource{Original: rTopic}

	rKafka := &types.ExtractedResource{Object: kafkaObj, GVK: kafkaObj.GroupVersionKind()}
	pKafka := &types.ProcessedResource{Original: rKafka}

	idx := NewIndex([]*types.ProcessedResource{pTopic, pKafka})

	key, ok := idx.KafkaTopic("", "actual.topic.name", "myns")
	if !ok || key.Name != "my-topic-cr" {
		t.Errorf("expected to find my-topic-cr, got %v %v", key, ok)
	}

	res := BindTopic(idx, "actual.topic.name", "my-cluster-kafka-bootstrap.myns.svc:9092", "myns")
	if res.Target.Resource == nil || res.Target.Resource.Name != "my-topic-cr" {
		t.Errorf("BindTopic failed, got %v", res)
	}

	// Test Kafka Broker Resolution
	resBroker := idx.Resolve(types.ConnectionEndpoint{Host: "my-cluster-kafka-brokers.myns.svc:9092"}, "myns")
	if resBroker.Role != "broker" || resBroker.Target.Resource == nil || resBroker.Target.Resource.Name != "my-cluster" {
		t.Errorf("expected Kafka broker my-cluster, got %v", resBroker)
	}

	res2 := BindTopic(idx, "unknown.topic", "my-cluster-kafka-bootstrap", "myns")
	if string(res2.Target.Node) != "topic:cluster:my-cluster/unknown.topic" {
		t.Errorf("BindTopic logical node failed, got %v", res2)
	}
}
func TestResolveOther(t *testing.T) {
	idx := NewIndex(nil)
	idx.Resolve(types.ConnectionEndpoint{}, "")
	idx.SelectsWorkload(types.ResourceKey{})
	idx.KafkaTopic("", "", "")
}
