package k8s

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// GPUProcessor detects GPU/TPU workloads by inspecting container resource requests/limits.
// It supports:
//   - nvidia.com/gpu  → type="nvidia"
//   - amd.com/gpu     → type="amd"
//   - nvidia.com/mig-<profile> → type="nvidia-mig", migProfile=<profile>
//
// Values produced: gpu.enabled, .type, .count, .migProfile, .tolerations
type GPUProcessor struct {
	processor.BaseProcessor
}

// NewGPUProcessor creates a new GPUProcessor.
func NewGPUProcessor() *GPUProcessor {
	return &GPUProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"gpu",
			50,
			schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"},
			schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"},
			schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"},
		),
	}
}

// Process inspects pod spec containers for GPU resource requests and returns values.
func (p *GPUProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, fmt.Errorf("gpu: object is nil")
	}

	containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")

	gpuEnabled := false
	gpuType := ""
	migProfile := ""
	var totalCount int64

	for _, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		// Inspect limits first, then requests for GPU resources.
		for _, resourcesField := range []string{"limits", "requests"} {
			resources, _, _ := unstructured.NestedMap(container, "resources", resourcesField)
			for resKey, resVal := range resources {
				detectedType, detectedMIG, count := classifyGPUResource(resKey, resVal)
				if detectedType == "" {
					continue
				}
				gpuEnabled = true
				// First-container-wins policy: GPU type is determined by the first
				// container that declares a GPU resource. Mixed-GPU pods are rare
				// and the first container is the primary workload container.
				if gpuType == "" {
					gpuType = detectedType
					migProfile = detectedMIG
				}
				totalCount += count
			}
			// Per-container: prefer limits over requests for counting.
			// Once a GPU is found in limits, skip requests for this container.
			// Counts from separate containers still accumulate independently.
			if gpuEnabled {
				break
			}
		}
	}

	gpuValues := map[string]interface{}{
		"enabled": gpuEnabled,
		"type":    gpuType,
		"count":   totalCount,
	}

	if migProfile != "" {
		gpuValues["migProfile"] = migProfile
	} else {
		gpuValues["migProfile"] = ""
	}

	// Add standard tolerations when GPU is detected.
	if gpuEnabled {
		gpuValues["tolerations"] = buildGPUTolerations(gpuType)
	} else {
		gpuValues["tolerations"] = []interface{}{}
	}

	values := map[string]interface{}{
		"gpu": gpuValues,
	}

	return &processor.Result{
		Processed:   true,
		ServiceName: processor.SanitizeServiceName(processor.ServiceNameFromResource(obj)),
		Values:      values,
	}, nil
}

// classifyGPUResource returns (gpuType, migProfile, count) for a resource key/value pair.
// Returns ("", "", 0) if the resource is not a GPU resource.
func classifyGPUResource(key string, val interface{}) (string, string, int64) {
	count := parseResourceCount(val)

	if key == "nvidia.com/gpu" {
		return "nvidia", "", count
	}
	if key == "amd.com/gpu" {
		return "amd", "", count
	}
	// nvidia MIG profiles: nvidia.com/mig-<profile>
	if strings.HasPrefix(key, "nvidia.com/mig-") {
		profile := strings.TrimPrefix(key, "nvidia.com/mig-")
		return "nvidia-mig", profile, count
	}
	return "", "", 0
}

// parseResourceCount parses a Kubernetes resource quantity string into an int64.
func parseResourceCount(val interface{}) int64 {
	if val == nil {
		return 0
	}
	var s string
	switch v := val.(type) {
	case string:
		s = v
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		s = fmt.Sprintf("%v", v)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// buildGPUTolerations returns the standard tolerations slice for a given GPU type.
func buildGPUTolerations(gpuType string) []interface{} {
	switch gpuType {
	case "nvidia", "nvidia-mig":
		return []interface{}{
			map[string]interface{}{
				"key":      "nvidia.com/gpu",
				"operator": "Exists",
				"effect":   "NoSchedule",
			},
		}
	case "amd":
		return []interface{}{
			map[string]interface{}{
				"key":      "amd.com/gpu",
				"operator": "Exists",
				"effect":   "NoSchedule",
			},
		}
	default:
		return []interface{}{}
	}
}
