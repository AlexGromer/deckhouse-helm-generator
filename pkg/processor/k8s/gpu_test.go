package k8s

// ============================================================
// Test Plan: GPU/TPU Workload Detection Processor (Task 5.10.2)
// ============================================================
//
// | #  | Test Name                                              | Category    | Input                                                    | Expected Output                                                      |
// |----|--------------------------------------------------------|-------------|----------------------------------------------------------|----------------------------------------------------------------------|
// |  1 | TestGPUProcessor_NvidiaGPUDetected                     | happy       | Deployment with nvidia.com/gpu in limits                 | gpu.enabled=true, type="nvidia", count≥1                             |
// |  2 | TestGPUProcessor_AMDGPUDetected                        | happy       | Deployment with amd.com/gpu in limits                    | gpu.enabled=true, type="amd"                                         |
// |  3 | TestGPUProcessor_MIGProfileDetected                    | happy       | Deployment with nvidia.com/mig-1g.5gb in limits          | gpu.enabled=true, type="nvidia-mig", migProfile="1g.5gb"             |
// |  4 | TestGPUProcessor_NoGPUResources_Disabled               | happy       | Deployment with only cpu/memory resources                | gpu.enabled=false                                                    |
// |  5 | TestGPUProcessor_GPUCountExtraction                    | happy       | Deployment with nvidia.com/gpu: "4"                      | gpu.count=4                                                          |
// |  6 | TestGPUProcessor_TolerationsAdded                      | happy       | Deployment with nvidia GPU                               | gpu.tolerations contains nvidia.com/gpu toleration                   |
// |  7 | TestGPUProcessor_NilObject                             | error       | nil object                                               | error returned, no panic                                             |
// |  8 | TestGPUProcessor_Supports4GVKs                         | happy       | call Supports()                                          | Deployment, StatefulSet, Job, DaemonSet GVKs present                 |
// |  9 | TestGPUProcessor_MultipleContainersSumGPU              | edge        | Deployment with 2 containers each requesting 2 GPUs      | gpu.count=4 (sum across containers)                                  |
// | 10 | TestGPUProcessor_NameAndPriority                       | happy       | constructor                                              | Name()="gpu", Priority()>0                                           |

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

// makeDeploymentWithGPU constructs a Deployment whose pod spec contains
// the supplied containers slice.
func makeDeploymentWithGPU(name, namespace string, containers []interface{}) *unstructured.Unstructured {
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
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": name},
					},
					"spec": map[string]interface{}{
						"containers": containers,
					},
				},
			},
		},
	}
}

// gpuContainer returns a container map that requests/limits the given GPU resource.
// resourceName examples: "nvidia.com/gpu", "amd.com/gpu", "nvidia.com/mig-1g.5gb"
// count is a string (Kubernetes resource quantity format).
func gpuContainer(name, resourceName, count string) map[string]interface{} {
	return map[string]interface{}{
		"name":  name,
		"image": name + ":latest",
		"resources": map[string]interface{}{
			"limits": map[string]interface{}{
				resourceName: count,
			},
			"requests": map[string]interface{}{
				resourceName: count,
			},
		},
	}
}

// cpuOnlyContainer returns a container with only CPU/memory resources — no GPU.
func cpuOnlyContainer(name string) map[string]interface{} {
	return map[string]interface{}{
		"name":  name,
		"image": name + ":latest",
		"resources": map[string]interface{}{
			"limits": map[string]interface{}{
				"cpu":    "500m",
				"memory": "256Mi",
			},
		},
	}
}

// getGPUValues extracts the "gpu" sub-map from Result.Values.
func getGPUValues(t *testing.T, result *processor.Result) map[string]interface{} {
	t.Helper()
	if result == nil {
		t.Fatal("result is nil")
	}
	gpu, ok := result.Values["gpu"]
	if !ok {
		t.Fatal("result.Values missing 'gpu' key")
	}
	gpuMap, ok := gpu.(map[string]interface{})
	if !ok {
		t.Fatalf("result.Values['gpu'] is not map[string]interface{}, got %T", gpu)
	}
	return gpuMap
}

// ============================================================
// Test 1: Nvidia GPU detected
// ============================================================

func TestGPUProcessor_NvidiaGPUDetected(t *testing.T) {
	proc := NewGPUProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentWithGPU("gpu-app", "default", []interface{}{
		gpuContainer("trainer", "nvidia.com/gpu", "1"),
	})

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	gpuVals := getGPUValues(t, result)

	enabled, _ := gpuVals["enabled"].(bool)
	if !enabled {
		t.Error("expected gpu.enabled=true for nvidia.com/gpu resource")
	}

	gpuType, _ := gpuVals["type"].(string)
	testutil.AssertEqual(t, "nvidia", gpuType, "gpu.type")

	count, _ := gpuVals["count"].(int64)
	if count < 1 {
		t.Errorf("expected gpu.count >= 1, got %d", count)
	}
}

// ============================================================
// Test 2: AMD GPU detected
// ============================================================

func TestGPUProcessor_AMDGPUDetected(t *testing.T) {
	proc := NewGPUProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentWithGPU("amd-gpu-app", "default", []interface{}{
		gpuContainer("worker", "amd.com/gpu", "2"),
	})

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	gpuVals := getGPUValues(t, result)

	enabled, _ := gpuVals["enabled"].(bool)
	if !enabled {
		t.Error("expected gpu.enabled=true for amd.com/gpu resource")
	}

	gpuType, _ := gpuVals["type"].(string)
	testutil.AssertEqual(t, "amd", gpuType, "gpu.type")
}

// ============================================================
// Test 3: Nvidia MIG profile detected
// ============================================================

func TestGPUProcessor_MIGProfileDetected(t *testing.T) {
	proc := NewGPUProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentWithGPU("mig-app", "default", []interface{}{
		gpuContainer("mig-worker", "nvidia.com/mig-1g.5gb", "1"),
	})

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	gpuVals := getGPUValues(t, result)

	enabled, _ := gpuVals["enabled"].(bool)
	if !enabled {
		t.Error("expected gpu.enabled=true for nvidia MIG resource")
	}

	gpuType, _ := gpuVals["type"].(string)
	testutil.AssertEqual(t, "nvidia-mig", gpuType, "gpu.type")

	migProfile, _ := gpuVals["migProfile"].(string)
	testutil.AssertEqual(t, "1g.5gb", migProfile, "gpu.migProfile")
}

// ============================================================
// Test 4: No GPU resources → gpu.enabled=false
// ============================================================

func TestGPUProcessor_NoGPUResources_Disabled(t *testing.T) {
	proc := NewGPUProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentWithGPU("cpu-only-app", "default", []interface{}{
		cpuOnlyContainer("web"),
	})

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	gpuVals := getGPUValues(t, result)

	enabled, _ := gpuVals["enabled"].(bool)
	if enabled {
		t.Error("expected gpu.enabled=false when no GPU resources requested")
	}
}

// ============================================================
// Test 5: GPU count extracted from resource quantity
// ============================================================

func TestGPUProcessor_GPUCountExtraction(t *testing.T) {
	proc := NewGPUProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentWithGPU("multi-gpu-app", "default", []interface{}{
		gpuContainer("trainer", "nvidia.com/gpu", "4"),
	})

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	gpuVals := getGPUValues(t, result)

	count, _ := gpuVals["count"].(int64)
	testutil.AssertEqual(t, int64(4), count, "gpu.count")
}

// ============================================================
// Test 6: Tolerations auto-added for nvidia GPU
// ============================================================

func TestGPUProcessor_TolerationsAdded(t *testing.T) {
	proc := NewGPUProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentWithGPU("toleration-app", "default", []interface{}{
		gpuContainer("gpu-worker", "nvidia.com/gpu", "1"),
	})

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	gpuVals := getGPUValues(t, result)

	tolerations, ok := gpuVals["tolerations"].([]interface{})
	if !ok || len(tolerations) == 0 {
		t.Fatal("expected gpu.tolerations to be a non-empty slice")
	}

	// At least one toleration should reference nvidia.com/gpu.
	found := false
	for _, tol := range tolerations {
		tolMap, ok := tol.(map[string]interface{})
		if !ok {
			continue
		}
		key, _ := tolMap["key"].(string)
		if key == "nvidia.com/gpu" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a toleration with key='nvidia.com/gpu' in gpu.tolerations, got %v", tolerations)
	}
}

// ============================================================
// Test 7: nil object → error, no panic
// ============================================================

func TestGPUProcessor_NilObject(t *testing.T) {
	proc := NewGPUProcessor()
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
// Test 8: Supports() returns Deployment, StatefulSet, Job, DaemonSet
// ============================================================

func TestGPUProcessor_Supports4GVKs(t *testing.T) {
	proc := NewGPUProcessor()
	gvks := proc.Supports()

	required := []schema.GroupVersionKind{
		{Group: "apps", Version: "v1", Kind: "Deployment"},
		{Group: "apps", Version: "v1", Kind: "StatefulSet"},
		{Group: "apps", Version: "v1", Kind: "DaemonSet"},
		{Group: "batch", Version: "v1", Kind: "Job"},
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
// Test 9: Multiple containers — GPU count is summed across all containers
// ============================================================

func TestGPUProcessor_MultipleContainersSumGPU(t *testing.T) {
	proc := NewGPUProcessor()
	ctx := processor.Context{ChartName: "test-chart"}
	obj := makeDeploymentWithGPU("multi-container-app", "default", []interface{}{
		gpuContainer("trainer-a", "nvidia.com/gpu", "2"),
		gpuContainer("trainer-b", "nvidia.com/gpu", "2"),
	})

	result, err := proc.Process(ctx, obj)

	testutil.AssertNoError(t, err)
	gpuVals := getGPUValues(t, result)

	count, _ := gpuVals["count"].(int64)
	testutil.AssertEqual(t, int64(4), count, "gpu.count should sum GPUs across all containers")
}

// ============================================================
// Test 10: Name() and Priority()
// ============================================================

func TestGPUProcessor_NameAndPriority(t *testing.T) {
	proc := NewGPUProcessor()

	testutil.AssertEqual(t, "gpu", proc.Name(), "processor name")
	if proc.Priority() <= 0 {
		t.Errorf("expected Priority() > 0, got %d", proc.Priority())
	}
}
