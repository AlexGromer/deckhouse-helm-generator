package generator

import (
	"fmt"
	"strings"
)

// TracingOptions configures distributed tracing config generation.
type TracingOptions struct {
	Backend            string
	CollectorEndpoint  string
	SamplingRate       float64
	ContextPropagation []string
}

// TracingResult holds generated tracing config templates.
type TracingResult struct {
	Templates map[string]string
	NOTESTxt  string
}

// GenerateTracingConfig generates distributed tracing configuration templates.
func GenerateTracingConfig(opts TracingOptions) *TracingResult {
	result := &TracingResult{
		Templates: make(map[string]string),
	}

	backend := opts.Backend
	if backend == "" {
		backend = "jaeger"
	}

	var sb strings.Builder
	sb.WriteString("# Distributed Tracing Configuration\n")
	fmt.Fprintf(&sb, "# backend: %s\n", backend)
	if opts.CollectorEndpoint != "" {
		fmt.Fprintf(&sb, "# collector: %s\n", opts.CollectorEndpoint)
	}
	fmt.Fprintf(&sb, "# samplingRate: %v\n", opts.SamplingRate)
	for _, p := range opts.ContextPropagation {
		fmt.Fprintf(&sb, "# propagator: %s\n", p)
	}
	fmt.Fprintf(&sb, "backend: %s\n", backend)
	if opts.CollectorEndpoint != "" {
		fmt.Fprintf(&sb, "endpoint: %s\n", opts.CollectorEndpoint)
	}
	fmt.Fprintf(&sb, "samplingRate: %v\n", opts.SamplingRate)
	if len(opts.ContextPropagation) > 0 {
		sb.WriteString("propagators:\n")
		for _, p := range opts.ContextPropagation {
			fmt.Fprintf(&sb, "- %s\n", p)
		}
	}

	key := fmt.Sprintf("templates/tracing-%s.yaml", backend)
	result.Templates[key] = sb.String()
	result.NOTESTxt = "Distributed tracing configuration generated. Apply to enable tracing.\n"
	return result
}
