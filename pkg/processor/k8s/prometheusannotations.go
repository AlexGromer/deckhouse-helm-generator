package k8s

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/processor"
)

// PrometheusAnnotationsProcessor auto-generates prometheus.io/* annotations for Services
// that expose a metrics endpoint. It detects metrics ports by:
//   - port name == "metrics"
//   - port number == 9090 or 8080
//
// Generated annotations:
//   - prometheus.io/scrape: "true"
//   - prometheus.io/port:   "<port>"
//   - prometheus.io/path:   "/metrics"  (only when a metrics port is detected)
type PrometheusAnnotationsProcessor struct {
	processor.BaseProcessor
}

// NewPrometheusAnnotationsProcessor creates a new Prometheus scrape annotations processor.
func NewPrometheusAnnotationsProcessor() *PrometheusAnnotationsProcessor {
	return &PrometheusAnnotationsProcessor{
		BaseProcessor: processor.NewBaseProcessor(
			"prometheusannotations",
			60,
			schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"},
		),
	}
}

// Process inspects a Service for metrics ports and produces prometheus.io/* annotation values.
func (p *PrometheusAnnotationsProcessor) Process(ctx processor.Context, obj *unstructured.Unstructured) (*processor.Result, error) {
	if obj == nil {
		return nil, fmt.Errorf("prometheusannotations: object is nil")
	}

	// Only handle Service resources
	if obj.GetKind() != "Service" {
		return &processor.Result{Processed: false}, nil
	}

	metricsPort, found := detectMetricsPort(obj)

	annotations := map[string]interface{}{}

	if found {
		annotations["prometheus.io/scrape"] = "true"
		annotations["prometheus.io/port"] = fmt.Sprintf("%d", metricsPort)
		annotations["prometheus.io/path"] = "/metrics"
	}

	values := map[string]interface{}{
		"annotations": annotations,
	}

	return &processor.Result{
		Processed:   true,
		ServiceName: processor.SanitizeServiceName(processor.ServiceNameFromResource(obj)),
		Values:      values,
	}, nil
}

// detectMetricsPort looks for a metrics-relevant port in the Service spec.
// Returns (port, true) if found, (0, false) otherwise.
func detectMetricsPort(obj *unstructured.Unstructured) (int64, bool) {
	ports, _, _ := unstructured.NestedSlice(obj.Object, "spec", "ports")
	for _, p := range ports {
		port, ok := p.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := port["name"].(string)
		portNum, _ := port["port"].(int64)

		// Port named "metrics" always qualifies, regardless of port number
		if name == "metrics" {
			return portNum, true
		}

		// Well-known metrics ports
		if portNum == 9090 || portNum == 8080 {
			return portNum, true
		}
	}
	return 0, false
}
