package k8s

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// IstioSidecarProcessor detects Istio sidecar injection markers in Deployments and StatefulSets.
// It checks for:
//   - namespace label istio-injection=enabled (via Context.Options["namespace.labels"])
//   - pod annotation sidecar.istio.io/inject=true
//   - istio-proxy container in pod spec
//
// Values produced: istio.enabled, istio.sidecarInject, istio.proxyConfig
type IstioSidecarProcessor struct {
	processor.BaseProcessor
}

// NewIstioSidecarProcessor creates a new Istio sidecar injection processor.
func NewIstioSidecarProcessor() *IstioSidecarProcessor {
	return &IstioSidecarProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"istiosidecar",
			50,
			schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"},
		),
	}
}

// Process inspects the object for Istio markers and returns values.
func (p *IstioSidecarProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, fmt.Errorf("istiosidecar: object is nil")
	}

	istioEnabled := false
	sidecarInject := false
	var proxyConfig map[string]interface{}

	// Check 1: namespace label istio-injection=enabled via context options
	if ctx.Options != nil {
		if nsLabels, ok := ctx.Options["namespace.labels"]; ok {
			switch labels := nsLabels.(type) {
			case map[string]string:
				if labels["istio-injection"] == "enabled" {
					istioEnabled = true
				}
			case map[string]interface{}:
				if v, ok := labels["istio-injection"]; ok && fmt.Sprintf("%v", v) == "enabled" {
					istioEnabled = true
				}
			}
		}
	}

	// Check 2: pod template annotation sidecar.istio.io/inject=true
	podAnnotations, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "annotations")
	if podAnnotations != nil {
		if val, ok := podAnnotations["sidecar.istio.io/inject"]; ok && val == "true" {
			istioEnabled = true
			sidecarInject = true
		}
	}

	// Check 3: istio-proxy container already present in pod spec
	containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	for _, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := container["name"].(string)
		if name == "istio-proxy" {
			istioEnabled = true
			// Extract proxy config from container resources
			proxyConfig = map[string]interface{}{}
			if resources, ok := container["resources"].(map[string]interface{}); ok {
				proxyConfig["resources"] = resources
			}
			if image, ok := container["image"].(string); ok {
				proxyConfig["image"] = image
			}
			break
		}
	}

	// proxyConfig must always be present as a map (may be empty)
	if proxyConfig == nil {
		proxyConfig = map[string]interface{}{}
	}

	values := map[string]interface{}{
		"istio": map[string]interface{}{
			"enabled":       istioEnabled,
			"sidecarInject": sidecarInject,
			"proxyConfig":   proxyConfig,
		},
	}

	return &processor.Result{
		Processed:   true,
		ServiceName: processor.SanitizeServiceName(processor.ServiceNameFromResource(obj)),
		Values:      values,
	}, nil
}
