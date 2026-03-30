package k8s

// ============================================================
// Test Plan: Windows Container Support Processor (Task 5.10.4)
// ============================================================
//
// | #  | Test Name                                                   | Category    | Input                                                     | Expected Output                                                        |
// |----|-------------------------------------------------------------|-------------|-----------------------------------------------------------|------------------------------------------------------------------------|
// |  1 | TestWindowsContainerProcessor_NanoserverImageDetected       | happy       | Deployment with "nanoserver" image                        | windows.enabled=true                                                   |
// |  2 | TestWindowsContainerProcessor_ServercoreImageDetected       | happy       | Deployment with "servercore" image                        | windows.enabled=true                                                   |
// |  3 | TestWindowsContainerProcessor_ExistingNodeSelector          | happy       | Deployment with kubernetes.io/os: windows nodeSelector    | windows.enabled=true                                                   |
// |  4 | TestWindowsContainerProcessor_DHGAnnotation                 | happy       | Deployment with dhg.io/os: windows annotation             | windows.enabled=true                                                   |
// |  5 | TestWindowsContainerProcessor_LinuxImage_Disabled           | happy       | Deployment with linux-only image                          | windows.enabled=false                                                  |
// |  6 | TestWindowsContainerProcessor_NilObject                     | error       | nil object                                                | error returned, no panic                                               |
// |  7 | TestWindowsContainerProcessor_Supports3GVKs                 | happy       | call Supports()                                           | Deployment, StatefulSet, DaemonSet GVKs present                        |
// |  8 | TestWindowsContainerProcessor_ValuesStructure               | happy       | Deployment with windows image                             | windows.enabled, windows.nodeSelector, windows.tolerations all present |
// |  9 | TestWindowsContainerProcessor_NameAndPriority               | happy       | constructor                                               | Name()="windowscontainer", Priority()>0                                |
// | 10 | TestWindowsContainerProcessor_MultiContainerAnyWindows      | edge        | Deployment with one linux + one nanoserver container      | windows.enabled=true (any windows container triggers flag)             |

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

// makeDeploymentWithContainersAndMeta builds a Deployment with the given
// containers, nodeSelector, and pod-level annotations for Windows tests.
func makeDeploymentWithContainersAndMeta(
	name, namespace string,
	containers []interface{},
	nodeSelector map[string]interface{},
	podAnnotations map[string]interface{},
) *unstructured.Unstructured {
	podSpec := map[string]interface{}{
		"containers": containers,
	}
	if len(nodeSelector) > 0 {
		podSpec["nodeSelector"] = nodeSelector
	}

	templateMeta := map[string]interface{}{
		"labels": map[string]interface{}{"app": name},
	}
	if len(podAnnotations) > 0 {
		templateMeta["annotations"] = podAnnotations
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
					"metadata": templateMeta,
					"spec":     podSpec,
				},
			},
		},
	}
}

// windowsContainerWithImage returns a container map using the supplied image name.
func windowsContainerWithImage(name, image string) map[string]interface{} {
	return map[string]interface{}{
		"name":  name,
		"image": image,
	}
}

// getWindowsValues extracts the "windows" sub-map from Result.Values.
func getWindowsValues(t *testing.T, result *processor.Result) map[string]interface{} {
	t.Helper()
	if result == nil {
		t.Fatal("result is nil")
	}
	win, ok := result.Values["windows"]
	if !ok {
		t.Fatal("result.Values missing 'windows' key")
	}
	winMap, ok := win.(map[string]interface{})
	if !ok {
		t.Fatalf("result.Values['windows'] is not map[string]interface{}, got %T", win)
	}
	return winMap
}

// ============================================================
// Test 1: Nanoserver image → windows.enabled=true
// ============================================================

func TestWindowsContainerProcessor_NanoserverImageDetected(t *testing.T) {
	proc := NewWindowsContainerProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentWithContainersAndMeta(
		"nano-app", "default",
		[]interface{}{
			windowsContainerWithImage("app", "mcr.microsoft.com/windows/nanoserver:ltsc2022"),
		},
		nil, nil,
	)

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	winVals := getWindowsValues(t, result)

	enabled, _ := winVals["enabled"].(bool)
	if !enabled {
		t.Error("expected windows.enabled=true for nanoserver image")
	}
}

// ============================================================
// Test 2: Servercore image → windows.enabled=true
// ============================================================

func TestWindowsContainerProcessor_ServercoreImageDetected(t *testing.T) {
	proc := NewWindowsContainerProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentWithContainersAndMeta(
		"core-app", "default",
		[]interface{}{
			windowsContainerWithImage("app", "mcr.microsoft.com/windows/servercore:ltsc2019"),
		},
		nil, nil,
	)

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	winVals := getWindowsValues(t, result)

	enabled, _ := winVals["enabled"].(bool)
	if !enabled {
		t.Error("expected windows.enabled=true for servercore image")
	}
}

// ============================================================
// Test 3: Existing nodeSelector kubernetes.io/os: windows → enabled=true
// ============================================================

func TestWindowsContainerProcessor_ExistingNodeSelector(t *testing.T) {
	proc := NewWindowsContainerProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentWithContainersAndMeta(
		"selector-app", "default",
		[]interface{}{
			windowsContainerWithImage("app", "myregistry.io/myapp:latest"),
		},
		map[string]interface{}{
			"kubernetes.io/os": "windows",
		},
		nil,
	)

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	winVals := getWindowsValues(t, result)

	enabled, _ := winVals["enabled"].(bool)
	if !enabled {
		t.Error("expected windows.enabled=true when nodeSelector has kubernetes.io/os=windows")
	}
}

// ============================================================
// Test 4: dhg.io/os: windows annotation → enabled=true
// ============================================================

func TestWindowsContainerProcessor_DHGAnnotation(t *testing.T) {
	proc := NewWindowsContainerProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentWithContainersAndMeta(
		"annotated-win-app", "default",
		[]interface{}{
			windowsContainerWithImage("app", "myregistry.io/myapp:v1"),
		},
		nil,
		map[string]interface{}{
			"dhg.io/os": "windows",
		},
	)

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	winVals := getWindowsValues(t, result)

	enabled, _ := winVals["enabled"].(bool)
	if !enabled {
		t.Error("expected windows.enabled=true when dhg.io/os=windows annotation is set")
	}
}

// ============================================================
// Test 5: Linux-only image → windows.enabled=false
// ============================================================

func TestWindowsContainerProcessor_LinuxImage_Disabled(t *testing.T) {
	proc := NewWindowsContainerProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentWithContainersAndMeta(
		"linux-app", "default",
		[]interface{}{
			windowsContainerWithImage("app", "nginx:1.25-alpine"),
		},
		nil, nil,
	)

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	winVals := getWindowsValues(t, result)

	enabled, _ := winVals["enabled"].(bool)
	if enabled {
		t.Error("expected windows.enabled=false for linux-only image with no windows markers")
	}
}

// ============================================================
// Test 6: nil object → error, no panic
// ============================================================

func TestWindowsContainerProcessor_NilObject(t *testing.T) {
	proc := NewWindowsContainerProcessor()
	ctx := processor.Context{ChartName: "test-chart"}

	result, err := proc.Process(ctx, nil)

	if err == nil {
		t.Error("expected error for nil object, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result for nil object, got %+v", result)
	}
}

// ============================================================
// Test 7: Supports() returns Deployment, StatefulSet, DaemonSet (exactly 3)
// ============================================================

func TestWindowsContainerProcessor_Supports3GVKs(t *testing.T) {
	proc := NewWindowsContainerProcessor()
	gvks := proc.Supports()

	required := []schema.GroupVersionKind{
		{Group: "apps", Version: "v1", Kind: "Deployment"},
		{Group: "apps", Version: "v1", Kind: "StatefulSet"},
		{Group: "apps", Version: "v1", Kind: "DaemonSet"},
	}

	for _, want := range required {
		found := false
		for _, gvk := range gvks {
			if gvk == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected Supports() to include GVK %v, got %v", want, gvks)
		}
	}
}

// ============================================================
// Test 8: Values structure — required keys present
// ============================================================

func TestWindowsContainerProcessor_ValuesStructure(t *testing.T) {
	proc := NewWindowsContainerProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentWithContainersAndMeta(
		"struct-app", "default",
		[]interface{}{
			windowsContainerWithImage("app", "mcr.microsoft.com/windows/nanoserver:ltsc2022"),
		},
		nil, nil,
	)

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	if result == nil {
		t.Fatal("result is nil")
	}
	if !result.Processed {
		t.Error("expected Result.Processed=true")
	}

	winVals := getWindowsValues(t, result)

	for _, key := range []string{"enabled", "nodeSelector", "tolerations"} {
		if _, ok := winVals[key]; !ok {
			t.Errorf("expected windows.%s key to be present in Values", key)
		}
	}

	// nodeSelector must contain kubernetes.io/os=windows
	nsMap, ok := winVals["nodeSelector"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected windows.nodeSelector to be map[string]interface{}, got %T", winVals["nodeSelector"])
	}
	osVal, _ := nsMap["kubernetes.io/os"].(string)
	testutil.AssertEqual(t, "windows", osVal, "windows.nodeSelector[kubernetes.io/os]")

	// tolerations must be non-empty slice
	tolerations, ok := winVals["tolerations"].([]interface{})
	if !ok || len(tolerations) == 0 {
		t.Error("expected windows.tolerations to be a non-empty slice")
	}
}

// ============================================================
// Test 9: Name() and Priority()
// ============================================================

func TestWindowsContainerProcessor_NameAndPriority(t *testing.T) {
	proc := NewWindowsContainerProcessor()

	testutil.AssertEqual(t, "windowscontainer", proc.Name(), "processor name")
	if proc.Priority() <= 0 {
		t.Errorf("expected Priority() > 0, got %d", proc.Priority())
	}
}

// ============================================================
// Test 10: Any windows container among multiple triggers enabled=true
// ============================================================

func TestWindowsContainerProcessor_MultiContainerAnyWindows(t *testing.T) {
	proc := NewWindowsContainerProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentWithContainersAndMeta(
		"mixed-app", "default",
		[]interface{}{
			// First container is a standard linux image.
			windowsContainerWithImage("sidecar", "busybox:1.36"),
			// Second container uses a windows image — should trigger detection.
			windowsContainerWithImage("app", "mcr.microsoft.com/windows/nanoserver:ltsc2022"),
		},
		nil, nil,
	)

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	winVals := getWindowsValues(t, result)

	enabled, _ := winVals["enabled"].(bool)
	if !enabled {
		t.Error("expected windows.enabled=true when at least one container uses a windows image")
	}
}
