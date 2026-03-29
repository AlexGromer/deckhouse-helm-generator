package generator

import (
	"fmt"
	"strings"
)

// ServicePort describes a single port entry in a ServiceEntry.
type ServicePort struct {
	Name     string
	Number   int
	Protocol string
}

// MultiClusterOptions configures multi-cluster ServiceEntry generation.
type MultiClusterOptions struct {
	// ServiceName is the logical service name.
	ServiceName string
	// RemoteClusterEndpoints lists IP addresses of remote cluster endpoints.
	RemoteClusterEndpoints []string
	// Ports lists service port definitions.
	Ports []ServicePort
	// LocalityRouting enables locality-aware routing config when true.
	LocalityRouting bool
	// FailoverPriority sets the failover priority value (0 = not set).
	FailoverPriority int
}

// GenerateMultiClusterServiceEntry generates a ServiceEntry YAML map for
// multi-cluster routing.
// Returns nil if RemoteClusterEndpoints is empty.
func GenerateMultiClusterServiceEntry(opts MultiClusterOptions) map[string]string {
	if len(opts.RemoteClusterEndpoints) == 0 {
		return nil
	}

	templates := make(map[string]string)
	yaml := buildMultiClusterServiceEntryYAML(opts)
	key := fmt.Sprintf("templates/istio-mc-se-%s.yaml", opts.ServiceName)
	templates[key] = yaml
	return templates
}

// buildMultiClusterServiceEntryYAML constructs the ServiceEntry YAML string.
func buildMultiClusterServiceEntryYAML(opts MultiClusterOptions) string {
	var sb strings.Builder

	sb.WriteString("apiVersion: networking.istio.io/v1beta1\n")
	sb.WriteString("kind: ServiceEntry\n")
	sb.WriteString("metadata:\n")
	fmt.Fprintf(&sb, "  name: %s-multicluster\n", opts.ServiceName)
	sb.WriteString("spec:\n")
	fmt.Fprintf(&sb, "  hosts:\n  - %s.global\n", opts.ServiceName)
	sb.WriteString("  location: MESH_INTERNAL\n")
	sb.WriteString("  resolution: STATIC\n")

	// Ports
	if len(opts.Ports) > 0 {
		sb.WriteString("  ports:\n")
		for _, p := range opts.Ports {
			sb.WriteString("  - ")
			fmt.Fprintf(&sb, "name: %s\n", p.Name)
			fmt.Fprintf(&sb, "    number: %d\n", p.Number)
			fmt.Fprintf(&sb, "    protocol: %s\n", p.Protocol)
		}
	}

	// Endpoints
	sb.WriteString("  endpoints:\n")
	for _, ep := range opts.RemoteClusterEndpoints {
		fmt.Fprintf(&sb, "  - address: %s\n", ep)
		if opts.LocalityRouting {
			sb.WriteString("    locality: us-east1/us-east1-a\n")
		}
		if opts.FailoverPriority > 0 {
			fmt.Fprintf(&sb, "    priority: %d\n", opts.FailoverPriority)
		}
	}

	// TrafficPolicy for locality / failover
	if opts.LocalityRouting {
		sb.WriteString("  trafficPolicy:\n")
		sb.WriteString("    outlierDetection:\n")
		sb.WriteString("      consecutive5xxErrors: 5\n")
		sb.WriteString("      interval: 30s\n")
		sb.WriteString("    loadBalancer:\n")
		sb.WriteString("      localityLbSetting:\n")
		sb.WriteString("        enabled: true\n")
		if opts.FailoverPriority > 0 {
			sb.WriteString("        failoverPriority:\n")
			fmt.Fprintf(&sb, "        - topology.kubernetes.io/zone\n")
		}
	}

	return sb.String()
}
