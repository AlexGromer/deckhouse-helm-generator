package k8s

import (
	"testing"
	"text/template"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/processor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKafkaTopicProcessor_V1(t *testing.T) {
	p := NewKafkaTopicProcessor()
	ctx := processor.Context{ChartName: "test-chart"}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kafka.strimzi.io/v1",
			"kind":       "KafkaTopic",
			"metadata": map[string]interface{}{
				"name":      "orders-created",
				"namespace": "kafka",
				"labels": map[string]interface{}{
					"strimzi.io/cluster":     "my-cluster",
					"app.kubernetes.io/name": "orders",
				},
			},
			"spec": map[string]interface{}{
				"partitions": int64(6),
				"replicas":   int64(3),
				"config": map[string]interface{}{
					"retention.ms": "604800000",
				},
			},
		},
	}

	res, err := p.Process(ctx, obj)
	require.NoError(t, err)

	assert.Equal(t, "orders", res.ServiceName)
	assert.Equal(t, "templates/orders-kafkatopic-orders-created.yaml", res.TemplatePath)
	assert.Equal(t, "services.orders.kafkaTopics.ordersCreated", res.ValuesPath)
	assert.Equal(t, "my-cluster", res.Values["cluster"])
	assert.Empty(t, res.Notes)

	require.Contains(t, res.Values, "spec") // it should be there
	assert.Equal(t, int64(6), res.Values["partitions"])

	require.Contains(t, res.TemplateContent, "apiVersion: kafka.strimzi.io/v1")
	require.Contains(t, res.TemplateContent, `{{- with $entity.cluster }}`)
	require.NotContains(t, res.TemplateContent, `strimzi.io/cluster: my-cluster`)
	require.Contains(t, res.TemplateContent, `"app.kubernetes.io/name" "orders"`)

	// Test template parsing
	tmpl, err := template.New("test").Option("missingkey=zero").Funcs(template.FuncMap{
		"nindent":  func(i int, s string) string { return s },
		"fromYaml": func(s string) interface{} { return nil },
		"toYaml":   func(v interface{}) string { return "" },
		"include":  func(s string, v interface{}) string { return "" },
		"set":      func(d interface{}, k string, v interface{}) string { return "" },
		"merge":    func(d interface{}, s interface{}) interface{} { return nil },
		"default":  func(d interface{}, v interface{}) interface{} { return v },
		"dict":     func(v ...interface{}) map[string]interface{} { return map[string]interface{}{} },
		"deepCopy": func(v interface{}) interface{} { return v },
		"hasKey":   func(v interface{}, k string) bool { return false },
		"list":     func(v ...interface{}) []interface{} { return []interface{}{} },
	}).Parse(res.TemplateContent)
	require.NoError(t, err)
	require.NotNil(t, tmpl)
}

func TestKafkaTopicProcessor_V1beta2Note(t *testing.T) {
	p := NewKafkaTopicProcessor()
	ctx := processor.Context{ChartName: "test-chart"}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kafka.strimzi.io/v1beta2",
			"kind":       "KafkaTopic",
			"metadata": map[string]interface{}{
				"name":      "payments-legacy",
				"namespace": "kafka",
				"labels": map[string]interface{}{
					"strimzi.io/cluster": "my-cluster",
				},
			},
			"spec": map[string]interface{}{
				"topicName": "Payments.Legacy",
			},
		},
	}

	res, err := p.Process(ctx, obj)
	require.NoError(t, err)

	assert.Equal(t, "paymentsLegacy", res.ServiceName)
	require.Len(t, res.Notes, 1)
	assert.Contains(t, res.Notes[0], "uses kafka.strimzi.io/v1beta2, removed in Strimzi 1.0.0")
	assert.Equal(t, "Payments.Legacy", res.Values["topicName"])
}

func TestKafkaTopicProcessor_NoClusterLabel(t *testing.T) {
	p := NewKafkaTopicProcessor()
	ctx := processor.Context{ChartName: "test-chart"}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kafka.strimzi.io/v1",
			"kind":       "KafkaTopic",
			"metadata": map[string]interface{}{
				"name": "orders-created",
			},
		},
	}

	res, err := p.Process(ctx, obj)
	require.NoError(t, err)

	assert.Equal(t, "", res.Values["cluster"])
	require.Len(t, res.Notes, 1)
	assert.Contains(t, res.Notes[0], "has no strimzi.io/cluster label")
}

func TestKafkaUserProcessor_OperationNote(t *testing.T) {
	p := NewKafkaUserProcessor()
	ctx := processor.Context{ChartName: "test-chart"}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kafka.strimzi.io/v1beta2",
			"kind":       "KafkaUser",
			"metadata": map[string]interface{}{
				"name":      "orders",
				"namespace": "kafka",
				"labels": map[string]interface{}{
					"strimzi.io/cluster": "my-cluster",
				},
			},
			"spec": map[string]interface{}{
				"authorization": map[string]interface{}{
					"acls": []interface{}{
						map[string]interface{}{
							"resource": map[string]interface{}{
								"type": "topic",
								"name": "t",
							},
							"operation": "Read",
						},
					},
				},
			},
		},
	}

	res, err := p.Process(ctx, obj)
	require.NoError(t, err)

	require.Len(t, res.Notes, 2)
	assert.Contains(t, res.Notes[0], "uses kafka.strimzi.io/v1beta2")
	assert.Contains(t, res.Notes[1], "spec.authorization.acls[].operation was removed")
}

func TestKafkaUserProcessor_PasswordSecretDependency(t *testing.T) {
	p := NewKafkaUserProcessor()
	ctx := processor.Context{ChartName: "test-chart"}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kafka.strimzi.io/v1",
			"kind":       "KafkaUser",
			"metadata": map[string]interface{}{
				"name":      "orders",
				"namespace": "kafka",
				"labels": map[string]interface{}{
					"strimzi.io/cluster": "my-cluster",
				},
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
	}

	res, err := p.Process(ctx, obj)
	require.NoError(t, err)

	require.Len(t, res.Dependencies, 1)
	assert.Equal(t, "Secret", res.Dependencies[0].GVK.Kind)
	assert.Equal(t, "orders-pw", res.Dependencies[0].Name)
	assert.Equal(t, "kafka", res.Dependencies[0].Namespace)
}

func TestStrimziNamespaceNote(t *testing.T) {
	p := NewKafkaTopicProcessor()

	deployment := &types.ExtractedResource{
		Object: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      "orders",
					"namespace": "shop",
				},
			},
		},
		GVK: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
	}

	ctx := processor.Context{
		ChartName: "test-chart",
		AllResources: map[types.ResourceKey]*types.ExtractedResource{
			deployment.ResourceKey(): deployment,
		},
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kafka.strimzi.io/v1",
			"kind":       "KafkaTopic",
			"metadata": map[string]interface{}{
				"name":      "orders-created",
				"namespace": "kafka",
				"labels": map[string]interface{}{
					"strimzi.io/cluster": "my-cluster",
				},
			},
		},
	}

	res, err := p.Process(ctx, obj)
	require.NoError(t, err)

	require.Len(t, res.Notes, 1)
	assert.Contains(t, res.Notes[0], "is rendered into the release namespace; the Topic/User Operator watches only its own namespace")
}

func TestStrimziTemplateNotRewrittenByPreserveObjectLabels(t *testing.T) {
	tpl := `apiVersion: kafka.strimzi.io/v1
kind: KafkaTopic
metadata:
  labels:
    {{- $dhgLabels := dict "app.kubernetes.io/name" "orders" }}
    {{- with $entity.cluster }}
    {{- $_ := set $dhgLabels "strimzi.io/cluster" . }}
    {{- end }}
    {{- merge $dhgLabels (include "app.labels" $ | fromYaml) | toYaml | nindent 4 }}`

	labels := map[string]string{
		"app.kubernetes.io/name": "orders",
		"strimzi.io/cluster":     "my-cluster",
	}

	res := processor.PreserveObjectLabels(tpl, labels)
	assert.Equal(t, tpl, res, "Template should not be rewritten")
}

func TestStrimziSanitizeName(t *testing.T) {
	assert.Equal(t, "ordersCreated", sanitizeName("orders-created"))
	assert.Equal(t, "paymentsLEgacy", sanitizeName("Payments.Legacy"))
}

func TestProcessStrimziEntityEmptyLabelsDict(t *testing.T) {
	p := NewKafkaTopicProcessor()
	ctx := processor.Context{ChartName: "test-chart"}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kafka.strimzi.io/v1",
			"kind":       "KafkaTopic",
			"metadata": map[string]interface{}{
				"name": "t",
			},
		},
	}

	res, err := p.Process(ctx, obj)
	require.NoError(t, err)

	assert.Contains(t, res.TemplateContent, `{{- $dhgLabels := dict }}`)
}
