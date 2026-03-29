package generator

import (
	"fmt"
	"strings"
)

// IstioCanaryOptions configures Istio canary VirtualService generation.
type IstioCanaryOptions struct {
	// ServiceName is the base Kubernetes service name.
	ServiceName string
	// StableWeight is the percentage of traffic routed to the stable version (0-100).
	StableWeight int
	// CanaryWeight is the percentage of traffic routed to the canary version (0-100).
	CanaryWeight int
	// HeaderRouting maps header names to expected values for header-based canary routing.
	HeaderRouting map[string]string
	// RetryAttempts sets the number of retry attempts (0 = disabled).
	RetryAttempts int
	// Timeout sets the per-request timeout (e.g. "15s"). Empty = no timeout.
	Timeout string
}

// GenerateIstioCanary generates a VirtualService YAML map for canary traffic splitting.
// Returns nil if ServiceName is empty.
func GenerateIstioCanary(opts IstioCanaryOptions) map[string]string {
	if opts.ServiceName == "" {
		return nil
	}

	templates := make(map[string]string)
	yaml := buildCanaryVirtualServiceYAML(opts)
	key := fmt.Sprintf("templates/istio-canary-%s.yaml", opts.ServiceName)
	templates[key] = yaml
	return templates
}

// buildCanaryVirtualServiceYAML constructs the VirtualService YAML string.
func buildCanaryVirtualServiceYAML(opts IstioCanaryOptions) string {
	var sb strings.Builder

	sb.WriteString("apiVersion: networking.istio.io/v1beta1\n")
	sb.WriteString("kind: VirtualService\n")
	sb.WriteString("metadata:\n")
	fmt.Fprintf(&sb, "  name: %s-canary\n", opts.ServiceName)
	sb.WriteString("spec:\n")
	fmt.Fprintf(&sb, "  hosts:\n  - %s\n", opts.ServiceName)
	sb.WriteString("  http:\n")

	// Header-based routing block (takes priority, routes to canary)
	if len(opts.HeaderRouting) > 0 {
		sb.WriteString("  - match:\n")
		for headerName, headerValue := range opts.HeaderRouting {
			sb.WriteString("    - headers:\n")
			fmt.Fprintf(&sb, "        %s:\n", headerName)
			fmt.Fprintf(&sb, "          exact: %s\n", headerValue)
		}
		sb.WriteString("    route:\n")
		sb.WriteString("    - destination:\n")
		fmt.Fprintf(&sb, "        host: %s-canary\n", opts.ServiceName)
		fmt.Fprintf(&sb, "      weight: %d\n", opts.CanaryWeight)
	}

	// Weight-based routing block
	sb.WriteString("  - route:\n")
	sb.WriteString("    - destination:\n")
	fmt.Fprintf(&sb, "        host: %s\n", opts.ServiceName)
	fmt.Fprintf(&sb, "      weight: %d\n", opts.StableWeight)
	sb.WriteString("    - destination:\n")
	fmt.Fprintf(&sb, "        host: %s-canary\n", opts.ServiceName)
	fmt.Fprintf(&sb, "      weight: %d\n", opts.CanaryWeight)

	// Timeout
	if opts.Timeout != "" {
		fmt.Fprintf(&sb, "    timeout: %s\n", opts.Timeout)
	}

	// Retries
	if opts.RetryAttempts > 0 {
		sb.WriteString("    retries:\n")
		fmt.Fprintf(&sb, "      attempts: %d\n", opts.RetryAttempts)
		sb.WriteString("      perTryTimeout: 5s\n")
	}

	return sb.String()
}
