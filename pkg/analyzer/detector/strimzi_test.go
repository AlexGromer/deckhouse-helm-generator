package detector

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

type strimziDetAssert struct{}

var assert = strimziDetAssert{}

func (strimziDetAssert) Len(t *testing.T, v interface{}, n int) {
	t.Helper()
	rv := reflect.ValueOf(v)
	if rv.Len() != n {
		t.Fatalf("expected len %d, got %d (%v)", n, rv.Len(), v)
	}
}

func (strimziDetAssert) Equal(t *testing.T, want, got interface{}) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func (strimziDetAssert) Contains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected %q to contain %q", s, substr)
	}
}

func TestStrimziDetector_KafkaUserToTopic(t *testing.T) {
	detector := NewStrimziDetector()

	kafkaUser := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kafka.strimzi.io/v1",
					"kind":       "KafkaUser",
					"metadata": map[string]interface{}{
						"name":      "orders",
						"namespace": "kafka",
					},
					"spec": map[string]interface{}{
						"authorization": map[string]interface{}{
							"acls": []interface{}{
								map[string]interface{}{
									"resource": map[string]interface{}{
										"type":        "topic",
										"name":        "orders-created",
										"patternType": "literal",
									},
								},
							},
						},
					},
				},
			},
			GVK: schema.GroupVersionKind{Group: "kafka.strimzi.io", Version: "v1", Kind: "KafkaUser"},
		},
	}

	kafkaTopic := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kafka.strimzi.io/v1",
					"kind":       "KafkaTopic",
					"metadata": map[string]interface{}{
						"name":      "orders-created",
						"namespace": "kafka",
					},
				},
			},
			GVK: schema.GroupVersionKind{Group: "kafka.strimzi.io", Version: "v1", Kind: "KafkaTopic"},
		},
	}

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		kafkaTopic.Original.ResourceKey(): kafkaTopic,
		kafkaUser.Original.ResourceKey():  kafkaUser,
	}

	rels := detector.Detect(context.Background(), kafkaUser, allResources)
	assert.Len(t, rels, 1)
	assert.Equal(t, types.RelationNameReference, rels[0].Type)
	assert.Equal(t, kafkaTopic.Original.ResourceKey(), rels[0].To)
	assert.Equal(t, "spec.authorization.acls", rels[0].Field)
	assert.Equal(t, "true", rels[0].Details["strimzi"])
}

func TestStrimziDetector_KafkaUserToSecret(t *testing.T) {
	detector := NewStrimziDetector()

	kafkaUser := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kafka.strimzi.io/v1",
					"kind":       "KafkaUser",
					"metadata": map[string]interface{}{
						"name":      "orders",
						"namespace": "kafka",
					},
					"spec": map[string]interface{}{
						"authentication": map[string]interface{}{
							"password": map[string]interface{}{
								"valueFrom": map[string]interface{}{
									"secretKeyRef": map[string]interface{}{
										"name": "orders-pw",
									},
								},
							},
						},
					},
				},
			},
			GVK: schema.GroupVersionKind{Group: "kafka.strimzi.io", Version: "v1", Kind: "KafkaUser"},
		},
	}

	secret := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"metadata": map[string]interface{}{
						"name":      "orders-pw",
						"namespace": "kafka",
					},
				},
			},
			GVK: schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"},
		},
	}

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		secret.Original.ResourceKey():    secret,
		kafkaUser.Original.ResourceKey(): kafkaUser,
	}

	rels := detector.Detect(context.Background(), kafkaUser, allResources)
	assert.Len(t, rels, 1)
	assert.Equal(t, secret.Original.ResourceKey(), rels[0].To)
	assert.Equal(t, "spec.authentication.password", rels[0].Field)
}

func TestStrimziDetector_WorkloadToKafkaUser(t *testing.T) {
	detector := NewStrimziDetector()

	deployment := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "orders",
						"namespace": "kafka",
					},
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"env": []interface{}{
											map[string]interface{}{
												"valueFrom": map[string]interface{}{
													"secretKeyRef": map[string]interface{}{
														"name": "orders",
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			GVK: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		},
	}

	kafkaUser := &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "kafka.strimzi.io/v1",
					"kind":       "KafkaUser",
					"metadata": map[string]interface{}{
						"name":      "orders",
						"namespace": "kafka",
					},
				},
			},
			GVK: schema.GroupVersionKind{Group: "kafka.strimzi.io", Version: "v1", Kind: "KafkaUser"},
		},
	}

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
		kafkaUser.Original.ResourceKey():  kafkaUser,
	}

	rels := detector.Detect(context.Background(), deployment, allResources)
	assert.Len(t, rels, 1)
	assert.Equal(t, types.RelationNameReference, rels[0].Type)
	assert.Equal(t, kafkaUser.Original.ResourceKey(), rels[0].To)
	assert.Contains(t, rels[0].Field, "secretKeyRef")
}
