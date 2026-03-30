package k8s

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// WindowsContainerProcessor detects Windows-based container workloads.
// Detection signals (any one is sufficient):
//   - Container image contains "nanoserver", "servercore", "ltsc", or "windows"
//   - Pod nodeSelector has kubernetes.io/os=windows
//   - Pod annotation dhg.io/os=windows
//
// Values produced: windows.enabled, .nodeSelector, .tolerations
type WindowsContainerProcessor struct {
	processor.BaseProcessor
}

// NewWindowsContainerProcessor creates a new WindowsContainerProcessor.
func NewWindowsContainerProcessor() *WindowsContainerProcessor {
	return &WindowsContainerProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"windowscontainer",
			50,
			schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"},
			schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DaemonSet"},
		),
	}
}

// Process inspects the object for Windows workload markers and returns values.
func (p *WindowsContainerProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, fmt.Errorf("windowscontainer: object is nil")
	}

	windowsDetected := false

	// Check 1: Pod nodeSelector kubernetes.io/os=windows
	nodeSelector, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "spec", "nodeSelector")
	if nodeSelector["kubernetes.io/os"] == "windows" {
		windowsDetected = true
	}

	// Check 2: Pod-level annotation dhg.io/os=windows
	if !windowsDetected {
		podAnnotations, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "annotations")
		if podAnnotations["dhg.io/os"] == "windows" {
			windowsDetected = true
		}
	}

	// Check 3: Container images containing Windows-specific keywords
	if !windowsDetected {
		containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
		for _, c := range containers {
			container, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			image, _ := container["image"].(string)
			if isWindowsImage(image) {
				windowsDetected = true
				break
			}
		}
	}

	winValues := map[string]interface{}{
		"enabled":      windowsDetected,
		"nodeSelector": map[string]interface{}{},
		"tolerations":  []interface{}{},
	}

	if windowsDetected {
		winValues["nodeSelector"] = map[string]interface{}{
			"kubernetes.io/os": "windows",
		}
		winValues["tolerations"] = []interface{}{
			map[string]interface{}{
				"key":      "os",
				"value":    "windows",
				"operator": "Equal",
				"effect":   "NoSchedule",
			},
		}
	}

	values := map[string]interface{}{
		"windows": winValues,
	}

	return &processor.Result{
		Processed:   true,
		ServiceName: processor.SanitizeServiceName(processor.ServiceNameFromResource(obj)),
		Values:      values,
	}, nil
}

// windowsKeywords are the image name substrings that indicate a Windows-based image.
var windowsKeywords = []string{"nanoserver", "servercore", "ltsc", "windows"}

// isWindowsImage returns true if the image name contains a Windows-specific keyword.
func isWindowsImage(image string) bool {
	lower := strings.ToLower(image)
	for _, kw := range windowsKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
