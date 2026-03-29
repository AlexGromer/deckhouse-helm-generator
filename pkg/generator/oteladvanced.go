package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// AdvancedOTELOptions configures advanced OpenTelemetry auto-instrumentation generation.
type AdvancedOTELOptions struct {
	SamplingStrategy   string
	SamplingRate       float64
	ResourceAttributes map[string]string
	Propagators        []string
}

// AdvancedOTELResult holds generated advanced OTEL templates.
type AdvancedOTELResult struct {
	// Instrumentations maps workload name → Instrumentation YAML.
	Instrumentations map[string]string
	// SamplingConfig maps config name → YAML content.
	SamplingConfig map[string]string
	NOTESTxt       string
}

// GenerateAdvancedOTEL generates advanced OTEL Instrumentation CRDs and sampling config.
func GenerateAdvancedOTEL(graph *types.ResourceGraph, opts AdvancedOTELOptions) *AdvancedOTELResult {
	result := &AdvancedOTELResult{
		Instrumentations: make(map[string]string),
		SamplingConfig:   make(map[string]string),
	}

	strategy := opts.SamplingStrategy
	if strategy == "" {
		strategy = "parent-based"
	}

	// Sampling config
	var scSB strings.Builder
	fmt.Fprintf(&scSB, "samplingStrategy: %s\n", strategy)
	fmt.Fprintf(&scSB, "samplingRate: %v\n", opts.SamplingRate)
	result.SamplingConfig["sampling"] = scSB.String()

	if graph == nil || len(graph.Resources) == 0 {
		result.NOTESTxt = "Advanced OTEL instrumentation configuration generated.\n"
		return result
	}

	for _, r := range graph.Resources {
		if r == nil || r.Original == nil {
			continue
		}
		if r.Original.GVK.Kind != "Deployment" {
			continue
		}
		name := r.Original.Object.GetName()

		var sb strings.Builder
		sb.WriteString("apiVersion: opentelemetry.io/v1alpha1\n")
		sb.WriteString("kind: Instrumentation\n")
		sb.WriteString("metadata:\n")
		fmt.Fprintf(&sb, "  name: %s-instrumentation\n", name)
		sb.WriteString("spec:\n")
		sb.WriteString("  propagators:\n")
		for _, p := range opts.Propagators {
			fmt.Fprintf(&sb, "  - %s\n", p)
		}
		sb.WriteString("  sampler:\n")
		fmt.Fprintf(&sb, "    type: %s\n", strategy)
		fmt.Fprintf(&sb, "    argument: \"%v\"\n", opts.SamplingRate)
		if len(opts.ResourceAttributes) > 0 {
			sb.WriteString("  resource:\n    attributes:\n")
			for k, v := range opts.ResourceAttributes {
				fmt.Fprintf(&sb, "      %s: %s\n", k, v)
			}
		}
		result.Instrumentations[name] = sb.String()
	}

	result.NOTESTxt = "Advanced OTEL instrumentation configuration generated.\n"
	return result
}
