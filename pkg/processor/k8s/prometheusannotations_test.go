package k8s

// ============================================================
// Test Plan: Prometheus Scrape Annotations Processor (Task 5.9.2)
// ============================================================
//
// | #  | Test Name                                                       | Category    | Input                                                     | Expected Output                                                             |
// |----|------------------------------------------------------------------|-------------|-----------------------------------------------------------|-----------------------------------------------------------------------------|
// |  1 | TestPrometheusAnnotationsProcessor_MetricsPortAddsAnnotations  | happy       | Service with port named "metrics"                         | prometheus.io/scrape, prometheus.io/port, prometheus.io/path in Values      |
// |  2 | TestPrometheusAnnotationsProcessor_NoMetricsPort               | happy       | Service with only port 80                                 | prometheus.io/scrape defaults only (no custom port)                         |
// |  3 | TestPrometheusAnnotationsProcessor_PortNameMetrics             | happy       | Service with port name="metrics", port=2112               | prometheus.io/port="2112" in Values                                         |
// |  4 | TestPrometheusAnnotationsProcessor_Port9090                    | happy       | Service with unnamed port 9090                            | prometheus.io/scrape added, prometheus.io/port="9090"                       |
// |  5 | TestPrometheusAnnotationsProcessor_Port8080                    | happy       | Service with unnamed port 8080                            | prometheus.io/scrape added, prometheus.io/port="8080"                       |
// |  6 | TestPrometheusAnnotationsProcessor_NilObject                  | error       | nil unstructured object                                   | error returned, no panic                                                    |
// |  7 | TestPrometheusAnnotationsProcessor_NonServiceNotProcessed     | happy       | Deployment object passed                                  | Result.Processed=false or skipped without error                             |
// |  8 | TestPrometheusAnnotationsProcessor_ValuesContainAnnotations   | happy       | Service with metrics port                                 | Values["annotations"]["prometheus.io/scrape"] == "true"                     |
// |  9 | TestPrometheusAnnotationsProcessor_SupportsOnlyService        | happy       | call Supports()                                           | only Service GVK in list                                                    |
// | 10 | TestPrometheusAnnotationsProcessor_NameAndPriority            | happy       | constructor                                               | Name()="prometheusannotations", Priority()>0                                |

import (
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helpers
// ============================================================

// makeServiceObj creates an unstructured Service with the given named ports.
// Each entry in ports is {name, port}.
func makeServiceObj(name, namespace string, ports []map[string]interface{}) *unstructured.Unstructured {
	portList := make([]interface{}, 0, len(ports))
	for _, p := range ports {
		entry := map[string]interface{}{}
		for k, v := range p {
			entry[k] = v
		}
		portList = append(portList, entry)
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app": name,
				},
			},
			"spec": map[string]interface{}{
				"ports": portList,
			},
		},
	}
}

// getAnnotationsFromValues retrieves the nested annotations map from Result.Values.
// It tries both Values["annotations"] (flat) and Values["service"]["annotations"] patterns.
func getAnnotationsFromValues(t *testing.T, result *processor.Result) map[string]interface{} {
	t.Helper()
	if result == nil || result.Values == nil {
		t.Fatal("result or result.Values is nil")
	}

	// Try direct annotations key
	if ann, ok := result.Values["annotations"]; ok {
		if annMap, ok := ann.(map[string]interface{}); ok {
			return annMap
		}
	}

	// Try nested under "service"
	if svc, ok := result.Values["service"]; ok {
		if svcMap, ok := svc.(map[string]interface{}); ok {
			if ann, ok := svcMap["annotations"]; ok {
				if annMap, ok := ann.(map[string]interface{}); ok {
					return annMap
				}
			}
		}
	}

	// Try nested under "prometheusAnnotations"
	if pa, ok := result.Values["prometheusAnnotations"]; ok {
		if paMap, ok := pa.(map[string]interface{}); ok {
			return paMap
		}
	}

	t.Logf("Values keys available: %v", func() []string {
		keys := make([]string, 0, len(result.Values))
		for k := range result.Values {
			keys = append(keys, k)
		}
		return keys
	}())
	t.Fatal("could not find annotations in result.Values")
	return nil
}

// ============================================================
// Test 1: Port named "metrics" → all three annotations present
// ============================================================

func TestPrometheusAnnotationsProcessor_MetricsPortAddsAnnotations(t *testing.T) {
	proc := NewPrometheusAnnotationsProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeServiceObj("myapp", "default", []map[string]interface{}{
		{"name": "metrics", "port": int64(2112), "protocol": "TCP"},
	})

	result, err := proc.Process(ctx, obj)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	annotations := getAnnotationsFromValues(t, result)

	requiredKeys := []string{
		"prometheus.io/scrape",
		"prometheus.io/port",
		"prometheus.io/path",
	}
	for _, key := range requiredKeys {
		if _, ok := annotations[key]; !ok {
			t.Errorf("expected annotation %q to be present in Values", key)
		}
	}
}

// ============================================================
// Test 2: Service with only port 80 → default scrape only, no custom port
// ============================================================

func TestPrometheusAnnotationsProcessor_NoMetricsPort(t *testing.T) {
	proc := NewPrometheusAnnotationsProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeServiceObj("plain-svc", "default", []map[string]interface{}{
		{"name": "http", "port": int64(80), "protocol": "TCP"},
	})

	result, err := proc.Process(ctx, obj)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// No custom metrics port — prometheus.io/port should not be set to 80
	// (only scrape should be present as a default or the result should indicate no scrape)
	annotations := getAnnotationsFromValues(t, result)
	if port, ok := annotations["prometheus.io/port"]; ok {
		if port == "80" || port == int64(80) {
			t.Errorf("port 80 is not a metrics port and should not trigger prometheus.io/port annotation")
		}
	}
}

// ============================================================
// Test 3: Port named "metrics" with port number 2112 → prometheus.io/port="2112"
// ============================================================

func TestPrometheusAnnotationsProcessor_PortNameMetrics(t *testing.T) {
	proc := NewPrometheusAnnotationsProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeServiceObj("custom-metrics-svc", "default", []map[string]interface{}{
		{"name": "metrics", "port": int64(2112), "protocol": "TCP"},
	})

	result, err := proc.Process(ctx, obj)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	annotations := getAnnotationsFromValues(t, result)
	portVal, ok := annotations["prometheus.io/port"]
	if !ok {
		t.Fatal("expected prometheus.io/port to be set for port named 'metrics'")
	}
	portStr := fmt.Sprintf("%v", portVal)
	if portStr != "2112" {
		t.Errorf("expected prometheus.io/port='2112', got '%s'", portStr)
	}
}

// ============================================================
// Test 4: Port 9090 → prometheus.io/scrape and prometheus.io/port="9090"
// ============================================================

func TestPrometheusAnnotationsProcessor_Port9090(t *testing.T) {
	proc := NewPrometheusAnnotationsProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeServiceObj("prom-svc", "default", []map[string]interface{}{
		{"name": "web", "port": int64(9090), "protocol": "TCP"},
	})

	result, err := proc.Process(ctx, obj)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	annotations := getAnnotationsFromValues(t, result)

	scrapeVal, ok := annotations["prometheus.io/scrape"]
	if !ok {
		t.Fatal("expected prometheus.io/scrape to be set for port 9090")
	}
	if fmt.Sprintf("%v", scrapeVal) != "true" {
		t.Errorf("expected prometheus.io/scrape='true', got '%v'", scrapeVal)
	}

	portVal, ok := annotations["prometheus.io/port"]
	if !ok {
		t.Fatal("expected prometheus.io/port to be set for port 9090")
	}
	if fmt.Sprintf("%v", portVal) != "9090" {
		t.Errorf("expected prometheus.io/port='9090', got '%v'", portVal)
	}
}

// ============================================================
// Test 5: Port 8080 → prometheus.io/scrape and prometheus.io/port="8080"
// ============================================================

func TestPrometheusAnnotationsProcessor_Port8080(t *testing.T) {
	proc := NewPrometheusAnnotationsProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeServiceObj("app-svc", "default", []map[string]interface{}{
		{"name": "http", "port": int64(8080), "protocol": "TCP"},
	})

	result, err := proc.Process(ctx, obj)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	annotations := getAnnotationsFromValues(t, result)

	scrapeVal, ok := annotations["prometheus.io/scrape"]
	if !ok {
		t.Fatal("expected prometheus.io/scrape to be set for port 8080")
	}
	if fmt.Sprintf("%v", scrapeVal) != "true" {
		t.Errorf("expected prometheus.io/scrape='true', got '%v'", scrapeVal)
	}

	portVal, ok := annotations["prometheus.io/port"]
	if !ok {
		t.Fatal("expected prometheus.io/port to be set for port 8080")
	}
	if fmt.Sprintf("%v", portVal) != "8080" {
		t.Errorf("expected prometheus.io/port='8080', got '%v'", portVal)
	}
}

// ============================================================
// Test 6: nil object → error, no panic
// ============================================================

func TestPrometheusAnnotationsProcessor_NilObject(t *testing.T) {
	proc := NewPrometheusAnnotationsProcessor()
	ctx := processor.Context{ChartName: "test-chart"}

	result, err := proc.Process(ctx, nil)

	if err == nil {
		t.Error("expected error for nil object")
	}
	if result != nil {
		t.Errorf("expected nil result for nil object, got %+v", result)
	}
}

// ============================================================
// Test 7: Non-Service resource → not processed (Processed=false or skipped)
// ============================================================

func TestPrometheusAnnotationsProcessor_NonServiceNotProcessed(t *testing.T) {
	proc := NewPrometheusAnnotationsProcessor()
	ctx := processor.Context{ChartName: "test-chart"}

	// Pass a Deployment to a Service-only processor
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "my-deployment",
				"namespace": "default",
			},
			"spec": map[string]interface{}{},
		},
	}

	result, err := proc.Process(ctx, obj)

	if err != nil {
		t.Fatalf("unexpected error for non-Service: %v", err)
	}
	if result != nil && result.Processed {
		t.Error("expected Processed=false for non-Service resource")
	}
}

// ============================================================
// Test 8: Values contain prometheus.io/scrape="true"
// ============================================================

func TestPrometheusAnnotationsProcessor_ValuesContainAnnotations(t *testing.T) {
	proc := NewPrometheusAnnotationsProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeServiceObj("annotated-svc", "default", []map[string]interface{}{
		{"name": "metrics", "port": int64(9090), "protocol": "TCP"},
	})

	result, err := proc.Process(ctx, obj)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	annotations := getAnnotationsFromValues(t, result)

	scrapeVal, ok := annotations["prometheus.io/scrape"]
	if !ok {
		t.Fatal("expected prometheus.io/scrape in Values annotations")
	}
	if fmt.Sprintf("%v", scrapeVal) != "true" {
		t.Errorf("expected prometheus.io/scrape='true', got '%v'", scrapeVal)
	}
}

// ============================================================
// Test 9: Supports() returns only Service GVK
// ============================================================

func TestPrometheusAnnotationsProcessor_SupportsOnlyService(t *testing.T) {
	proc := NewPrometheusAnnotationsProcessor()
	gvks := proc.Supports()

	if len(gvks) == 0 {
		t.Fatal("expected at least one supported GVK")
	}

	serviceGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}

	found := false
	for _, gvk := range gvks {
		if gvk == serviceGVK {
			found = true
		}
		// Processor should be Service-only — no workload GVKs
		if gvk.Kind == "Deployment" || gvk.Kind == "StatefulSet" {
			t.Errorf("unexpected GVK %v in Supports() for a Service-only processor", gvk)
		}
	}

	if !found {
		t.Errorf("expected Service GVK %v in Supports(), got %v", serviceGVK, gvks)
	}
}

// ============================================================
// Test 10: Name() and Priority()
// ============================================================

func TestPrometheusAnnotationsProcessor_NameAndPriority(t *testing.T) {
	proc := NewPrometheusAnnotationsProcessor()

	testutil.AssertEqual(t, "prometheusannotations", proc.Name(), "processor name")

	if proc.Priority() <= 0 {
		t.Errorf("expected Priority() > 0, got %d", proc.Priority())
	}
}
