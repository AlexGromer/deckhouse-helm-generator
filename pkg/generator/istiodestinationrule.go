package generator

import (
	"fmt"
	"strings"
)

// DestinationRuleOptions configures advanced DestinationRule generation.
type DestinationRuleOptions struct {
	// ServiceName is the Kubernetes service name (used as host).
	ServiceName string
	// LBPolicy is the load balancing policy (ROUND_ROBIN, LEAST_CONN, RANDOM, CONSISTENT_HASH).
	LBPolicy string
	// ConsistentHashKey is the header/cookie key for CONSISTENT_HASH LB.
	ConsistentHashKey string
	// OutlierConsecutive5xx is the number of consecutive 5xx errors before ejection (0 = disabled).
	OutlierConsecutive5xx int
	// OutlierInterval is the outlier detection sweep interval (e.g. "30s").
	OutlierInterval string
	// CircuitBreakerMaxConn is the max TCP connections (0 = disabled).
	CircuitBreakerMaxConn int
	// CircuitBreakerMaxPending is the max pending HTTP requests (0 = disabled).
	CircuitBreakerMaxPending int
}

// GenerateAdvancedDestinationRule generates a DestinationRule YAML map with
// load balancing, outlier detection, and circuit breaker settings.
// Returns nil if ServiceName is empty.
func GenerateAdvancedDestinationRule(opts DestinationRuleOptions) map[string]string {
	if opts.ServiceName == "" {
		return nil
	}

	templates := make(map[string]string)
	yaml := buildAdvancedDestinationRuleYAML(opts)
	key := fmt.Sprintf("templates/istio-dr-adv-%s.yaml", opts.ServiceName)
	templates[key] = yaml
	return templates
}

// buildAdvancedDestinationRuleYAML builds the DestinationRule YAML string.
func buildAdvancedDestinationRuleYAML(opts DestinationRuleOptions) string {
	var sb strings.Builder

	sb.WriteString("apiVersion: networking.istio.io/v1beta1\n")
	sb.WriteString("kind: DestinationRule\n")
	sb.WriteString("metadata:\n")
	fmt.Fprintf(&sb, "  name: %s\n", opts.ServiceName)
	sb.WriteString("spec:\n")
	fmt.Fprintf(&sb, "  host: %s\n", opts.ServiceName)
	sb.WriteString("  trafficPolicy:\n")

	// Load balancing
	if opts.LBPolicy != "" {
		if opts.LBPolicy == "CONSISTENT_HASH" && opts.ConsistentHashKey != "" {
			sb.WriteString("    loadBalancer:\n")
			sb.WriteString("      consistentHash:\n")
			sb.WriteString("        httpHeaderName:\n")
			fmt.Fprintf(&sb, "          - %s\n", opts.ConsistentHashKey)
		} else {
			sb.WriteString("    loadBalancer:\n")
			fmt.Fprintf(&sb, "      simple: %s\n", opts.LBPolicy)
		}
	}

	// Connection pool (circuit breaker)
	if opts.CircuitBreakerMaxConn > 0 || opts.CircuitBreakerMaxPending > 0 {
		sb.WriteString("    connectionPool:\n")
		if opts.CircuitBreakerMaxConn > 0 {
			sb.WriteString("      tcp:\n")
			fmt.Fprintf(&sb, "        maxConnections: %d\n", opts.CircuitBreakerMaxConn)
		}
		if opts.CircuitBreakerMaxPending > 0 {
			sb.WriteString("      http:\n")
			fmt.Fprintf(&sb, "        http1MaxPendingRequests: %d\n", opts.CircuitBreakerMaxPending)
		}
	}

	// Outlier detection
	if opts.OutlierConsecutive5xx > 0 || opts.OutlierInterval != "" {
		sb.WriteString("    outlierDetection:\n")
		if opts.OutlierConsecutive5xx > 0 {
			fmt.Fprintf(&sb, "      consecutive5xxErrors: %d\n", opts.OutlierConsecutive5xx)
		}
		if opts.OutlierInterval != "" {
			fmt.Fprintf(&sb, "      interval: %s\n", opts.OutlierInterval)
		}
	}

	return sb.String()
}
