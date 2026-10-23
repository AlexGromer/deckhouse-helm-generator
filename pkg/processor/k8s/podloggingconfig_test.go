package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/testutil"
)

func TestPodLoggingConfigProcessor_Process(t *testing.T) {
	proc := NewPodLoggingConfigProcessor()
	ctx := newTestProcessorContext()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "deckhouse.io/v1alpha1",
			"kind":       "PodLoggingConfig",
			"metadata": map[string]interface{}{
				"name":      "orders-logs",
				"namespace": "shop",
			},
			"spec": map[string]interface{}{
				"clusterDestinationRefs": []interface{}{"loki-storage"},
				"labelSelector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app": "orders",
					},
				},
				"multilineParser": map[string]interface{}{
					"type": "MultilineJSON",
				},
				"logFilter": "filter",
			},
		},
	}

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, "orders", result.ServiceName, "service name from matchLabels")
	testutil.AssertEqual(t, "templates/podloggingconfig-orders-logs.yaml", result.TemplatePath, "template path")

	testutil.AssertContains(t, result.TemplateContent, "kind: PodLoggingConfig", "kind in template")
	testutil.AssertContains(t, result.TemplateContent, "namespace: {{ $.Release.Namespace }}", "namespace templated")
	testutil.AssertContains(t, result.TemplateContent, "$dhgSpec", "spec overlay used")

	spec := result.Values["spec"].(map[string]interface{})
	testutil.AssertEqual(t, "filter", spec["logFilter"], "logFilter preserved in values")

	// Test ClusterLoggingConfigProcessor
	clusterProc := NewClusterLoggingConfigProcessor()
	clusterObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "deckhouse.io/v1alpha1",
			"kind":       "ClusterLoggingConfig",
			"metadata": map[string]interface{}{
				"name": "system-logs",
			},
			"spec": map[string]interface{}{
				"destinationRefs": []interface{}{"loki-system"},
				"labelSelector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app": "system",
					},
				},
			},
		},
	}

	clusterResult, err := clusterProc.Process(ctx, clusterObj)
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, "system", clusterResult.ServiceName, "service name from matchLabels")
	if strings.Contains(clusterResult.TemplateContent, "namespace: ") {
		t.Error("ClusterLoggingConfig template should not contain namespace")
	}
}

func TestPodLoggingConfigProcessor_FallbackName(t *testing.T) {
	proc := NewPodLoggingConfigProcessor()
	ctx := newTestProcessorContext()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "deckhouse.io/v1alpha1",
			"kind":       "PodLoggingConfig",
			"metadata": map[string]interface{}{
				"name":      "orders-logs",
				"namespace": "shop",
			},
			"spec": map[string]interface{}{
				"clusterDestinationRefs": []interface{}{"loki-storage"},
			},
		},
	}

	result, err := proc.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "orders-logs", result.ServiceName, "fallback to object name")
}

func TestPodLoggingConfigProcessor_Nil(t *testing.T) {
	proc := NewPodLoggingConfigProcessor()
	ctx := newTestProcessorContext()

	_, err := proc.Process(ctx, nil)
	if err == nil {
		t.Error("Expected error for nil object")
	}
}
