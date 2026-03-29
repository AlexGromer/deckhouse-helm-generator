package k8s

// ============================================================
// Test Plan: Istio Sidecar Injection Detection Processor (Task 5.8.1)
// ============================================================
//
// | #  | Test Name                                                      | Category    | Input                                                       | Expected Output                                                              |
// |----|----------------------------------------------------------------|-------------|-------------------------------------------------------------|------------------------------------------------------------------------------|
// |  1 | TestIstioSidecarProcessor_NamespaceAnnotation                  | happy       | Deployment + namespace label istio-injection=enabled        | istio.enabled=true in Values                                                 |
// |  2 | TestIstioSidecarProcessor_PodAnnotation                       | happy       | Deployment + pod annotation sidecar.istio.io/inject=true    | istio.sidecarInject=true in Values                                           |
// |  3 | TestIstioSidecarProcessor_ProxyContainerDetected               | happy       | Deployment with istio-proxy container in spec               | istio.enabled=true in Values, proxyConfig populated                          |
// |  4 | TestIstioSidecarProcessor_NoIstioMarkers                      | happy       | Deployment with no Istio markers                            | istio.enabled=false in Values                                                |
// |  5 | TestIstioSidecarProcessor_NilObject                           | error       | nil unstructured object                                     | error returned, no panic                                                     |
// |  6 | TestIstioSidecarProcessor_SupportsGVKs                        | happy       | call Supports()                                             | contains Deployment and StatefulSet GVKs                                     |
// |  7 | TestIstioSidecarProcessor_ValuesStructure                     | happy       | Deployment with proxy container                             | Values has "istio" key with enabled, sidecarInject, proxyConfig sub-keys     |
// |  8 | TestIstioSidecarProcessor_ProcessedTrue                       | happy       | valid Deployment                                            | Result.Processed == true                                                     |
// |  9 | TestIstioSidecarProcessor_ProxyConfigPresent                  | happy       | Deployment with istio-proxy container with resources        | istio.proxyConfig is map, not nil                                            |
// | 10 | TestIstioSidecarProcessor_NameAndPriority                     | happy       | constructor                                                 | Name()="istiosidecar", Priority()>0                                          |

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Helpers
// ============================================================

// makeDeploymentForIstio creates an unstructured Deployment with optional
// pod template annotations and container list.
func makeDeploymentForIstio(
	name, namespace string,
	podAnnotations map[string]interface{},
	containers []interface{},
) *unstructured.Unstructured {
	templateMetadata := map[string]interface{}{
		"labels": map[string]interface{}{"app": name},
	}
	if len(podAnnotations) > 0 {
		templateMetadata["annotations"] = podAnnotations
	}

	podSpec := map[string]interface{}{
		"containers": containers,
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels":    map[string]interface{}{"app": name},
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"metadata": templateMetadata,
					"spec":     podSpec,
				},
			},
		},
	}
}

// makeStatefulSetForIstio creates an unstructured StatefulSet.
func makeStatefulSetForIstio(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "StatefulSet",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{},
					"spec": map[string]interface{}{
						"containers": []interface{}{},
					},
				},
			},
		},
	}
}

// istioProxyContainer returns a container map representing the istio-proxy sidecar.
func istioProxyContainer() interface{} {
	return map[string]interface{}{
		"name":  "istio-proxy",
		"image": "docker.io/istio/proxyv2:1.20.0",
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"cpu":    "100m",
				"memory": "128Mi",
			},
		},
	}
}

// appContainer returns a plain application container map.
func appContainer(name string) interface{} {
	return map[string]interface{}{
		"name":  name,
		"image": name + ":latest",
	}
}

// getIstioValues extracts the "istio" key from the Result.Values map.
func getIstioValues(t *testing.T, result *processor.Result) map[string]interface{} {
	t.Helper()
	if result == nil {
		t.Fatal("result is nil")
	}
	istio, ok := result.Values["istio"]
	if !ok {
		t.Fatal("result.Values missing 'istio' key")
	}
	istioMap, ok := istio.(map[string]interface{})
	if !ok {
		t.Fatalf("result.Values['istio'] is not map[string]interface{}, got %T", istio)
	}
	return istioMap
}

// ============================================================
// Test 1: Namespace label istio-injection=enabled → istio.enabled=true
// ============================================================

func TestIstioSidecarProcessor_NamespaceAnnotation(t *testing.T) {
	proc := NewIstioSidecarProcessor()
	ctx := processor.Context{
		ChartName: "test-chart",
		Options: map[string]interface{}{
			// Signal that the namespace has istio-injection=enabled label
			"namespace.labels": map[string]string{
				"istio-injection": "enabled",
			},
		},
	}
	obj := makeDeploymentForIstio("myapp", "default", nil, []interface{}{appContainer("myapp")})

	result, err := proc.Process(ctx, obj)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	istioVals := getIstioValues(t, result)
	enabled, _ := istioVals["enabled"].(bool)
	if !enabled {
		t.Errorf("expected istio.enabled=true when namespace has istio-injection=enabled label")
	}
}

// ============================================================
// Test 2: Pod annotation sidecar.istio.io/inject=true → istio.sidecarInject=true
// ============================================================

func TestIstioSidecarProcessor_PodAnnotation(t *testing.T) {
	proc := NewIstioSidecarProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentForIstio(
		"annotated-app", "default",
		map[string]interface{}{
			"sidecar.istio.io/inject": "true",
		},
		[]interface{}{appContainer("annotated-app")},
	)

	result, err := proc.Process(ctx, obj)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	istioVals := getIstioValues(t, result)

	enabled, _ := istioVals["enabled"].(bool)
	if !enabled {
		t.Errorf("expected istio.enabled=true when pod has sidecar.istio.io/inject=true")
	}
	sidecarInject, _ := istioVals["sidecarInject"].(bool)
	if !sidecarInject {
		t.Errorf("expected istio.sidecarInject=true when pod annotation is set")
	}
}

// ============================================================
// Test 3: Existing istio-proxy container detected
// ============================================================

func TestIstioSidecarProcessor_ProxyContainerDetected(t *testing.T) {
	proc := NewIstioSidecarProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentForIstio(
		"proxy-app", "istio-system",
		nil,
		[]interface{}{appContainer("app"), istioProxyContainer()},
	)

	result, err := proc.Process(ctx, obj)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	istioVals := getIstioValues(t, result)

	enabled, _ := istioVals["enabled"].(bool)
	if !enabled {
		t.Errorf("expected istio.enabled=true when istio-proxy container is present")
	}
}

// ============================================================
// Test 4: No Istio markers → istio.enabled=false
// ============================================================

func TestIstioSidecarProcessor_NoIstioMarkers(t *testing.T) {
	proc := NewIstioSidecarProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentForIstio(
		"plain-app", "default",
		nil,
		[]interface{}{appContainer("plain-app")},
	)

	result, err := proc.Process(ctx, obj)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	istioVals := getIstioValues(t, result)

	enabled, _ := istioVals["enabled"].(bool)
	if enabled {
		t.Errorf("expected istio.enabled=false when no Istio markers present")
	}
}

// ============================================================
// Test 5: nil object → error returned, no panic
// ============================================================

func TestIstioSidecarProcessor_NilObject(t *testing.T) {
	proc := NewIstioSidecarProcessor()
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
// Test 6: Supports() contains Deployment and StatefulSet GVKs
// ============================================================

func TestIstioSidecarProcessor_SupportsGVKs(t *testing.T) {
	proc := NewIstioSidecarProcessor()
	gvks := proc.Supports()

	if len(gvks) < 2 {
		t.Fatalf("expected at least 2 supported GVKs, got %d", len(gvks))
	}

	deploymentGVK := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	statefulSetGVK := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}

	hasDeployment := false
	hasStatefulSet := false
	for _, gvk := range gvks {
		if gvk == deploymentGVK {
			hasDeployment = true
		}
		if gvk == statefulSetGVK {
			hasStatefulSet = true
		}
	}

	if !hasDeployment {
		t.Errorf("expected Supports() to include Deployment GVK %v, got %v", deploymentGVK, gvks)
	}
	if !hasStatefulSet {
		t.Errorf("expected Supports() to include StatefulSet GVK %v, got %v", statefulSetGVK, gvks)
	}
}

// ============================================================
// Test 7: Values structure has required sub-keys
// ============================================================

func TestIstioSidecarProcessor_ValuesStructure(t *testing.T) {
	proc := NewIstioSidecarProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentForIstio(
		"structured-app", "default",
		nil,
		[]interface{}{appContainer("structured-app"), istioProxyContainer()},
	)

	result, err := proc.Process(ctx, obj)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	istioVals := getIstioValues(t, result)

	requiredKeys := []string{"enabled", "sidecarInject", "proxyConfig"}
	for _, key := range requiredKeys {
		if _, ok := istioVals[key]; !ok {
			t.Errorf("expected istio.%s key to be present in Values", key)
		}
	}
}

// ============================================================
// Test 8: Result.Processed == true for valid Deployment
// ============================================================

func TestIstioSidecarProcessor_ProcessedTrue(t *testing.T) {
	proc := NewIstioSidecarProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentForIstio(
		"my-deployment", "default",
		nil,
		[]interface{}{appContainer("app")},
	)

	result, err := proc.Process(ctx, obj)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Processed {
		t.Error("expected Result.Processed=true for valid Deployment")
	}
}

// ============================================================
// Test 9: istio.proxyConfig is non-nil map when proxy container present
// ============================================================

func TestIstioSidecarProcessor_ProxyConfigPresent(t *testing.T) {
	proc := NewIstioSidecarProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentForIstio(
		"proxy-config-app", "default",
		nil,
		[]interface{}{appContainer("app"), istioProxyContainer()},
	)

	result, err := proc.Process(ctx, obj)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	istioVals := getIstioValues(t, result)

	proxyConfig, ok := istioVals["proxyConfig"]
	if !ok {
		t.Fatal("expected istio.proxyConfig key to exist")
	}
	if proxyConfig == nil {
		t.Error("expected istio.proxyConfig to be non-nil when istio-proxy container is present")
	}
}

// ============================================================
// Test 10: Name() and Priority()
// ============================================================

func TestIstioSidecarProcessor_NameAndPriority(t *testing.T) {
	proc := NewIstioSidecarProcessor()

	testutil.AssertEqual(t, "istiosidecar", proc.Name(), "processor name")

	if proc.Priority() <= 0 {
		t.Errorf("expected Priority() > 0, got %d", proc.Priority())
	}
}
