package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// envPortMapping maps environment variable name patterns to service ports.
var envPortMapping = map[string]int{
	"DATABASE_URL":  5432,
	"POSTGRES_HOST": 5432,
	"POSTGRES_URL":  5432,
	"PGHOST":        5432,
	"REDIS_HOST":    6379,
	"REDIS_URL":     6379,
	"MYSQL_HOST":    3306,
	"MYSQL_URL":     3306,
	"MONGO_HOST":    27017,
	"MONGO_URL":     27017,
	"MONGODB_HOST":  27017,
	"AMQP_URL":      5672,
	"RABBITMQ_HOST": 5672,
	"KAFKA_HOST":    9092,
	"KAFKA_URL":     9092,
	"ES_HOST":       9200,
	"ELASTIC_HOST":  9200,
}

// GenerateAutoNetworkPolicies creates fine-grained NetworkPolicies from service relationship analysis.
// Returns map of template path → template content.
func GenerateAutoNetworkPolicies(graph *types.ResourceGraph, groups []*ServiceGroup) map[string]string {
	if len(groups) == 0 {
		return make(map[string]string)
	}

	result := make(map[string]string)

	// Build cross-namespace relationship index
	crossNS := buildCrossNamespaceIndex(graph, groups)

	for _, group := range groups {
		// Extract Service ports from group resources
		ingressPorts := extractServicePorts(group)

		// Extract egress ports from env var analysis
		egressPorts := extractEnvBasedPorts(group)

		// Check if this group has cross-namespace relationships
		crossNamespaces := crossNS[group.Name]

		path := fmt.Sprintf("templates/%s-networkpolicy.yaml", group.Name)
		result[path] = generateNetworkPolicy(group, ingressPorts, egressPorts, crossNamespaces)
	}

	return result
}

// portInfo holds port number and protocol.
type portInfo struct {
	Port     int
	Protocol string
}

// extractServicePorts extracts ports from Service resources in the group.
func extractServicePorts(group *ServiceGroup) []portInfo {
	var ports []portInfo
	seen := make(map[int]bool)

	for _, r := range group.Resources {
		if r.Original.GVK.Kind != "Service" {
			continue
		}

		spec, ok := r.Original.Object.Object["spec"].(map[string]interface{})
		if !ok {
			continue
		}

		portList, ok := spec["ports"].([]interface{})
		if !ok {
			continue
		}

		for _, p := range portList {
			portMap, ok := p.(map[string]interface{})
			if !ok {
				continue
			}

			var portNum int
			switch v := portMap["port"].(type) {
			case int64:
				portNum = int(v)
			case float64:
				portNum = int(v)
			case int:
				portNum = v
			}

			if portNum > 0 && !seen[portNum] {
				seen[portNum] = true
				protocol := "TCP"
				if p, ok := portMap["protocol"].(string); ok {
					protocol = p
				}
				ports = append(ports, portInfo{Port: portNum, Protocol: protocol})
			}
		}
	}

	return ports
}

// extractEnvBasedPorts detects egress ports from environment variables in workloads.
func extractEnvBasedPorts(group *ServiceGroup) []portInfo {
	var ports []portInfo
	seen := make(map[int]bool)

	for _, r := range group.Resources {
		kind := r.Original.GVK.Kind
		if kind != "Deployment" && kind != "StatefulSet" && kind != "DaemonSet" {
			continue
		}

		envVars := extractEnvVarsFromWorkload(r)
		for envName := range envVars {
			if port, ok := envPortMapping[envName]; ok {
				if !seen[port] {
					seen[port] = true
					ports = append(ports, portInfo{Port: port, Protocol: "TCP"})
				}
			}
		}
	}

	return ports
}

// extractEnvVarsFromWorkload extracts env var names from a workload resource.
func extractEnvVarsFromWorkload(r *types.ProcessedResource) map[string]string {
	result := make(map[string]string)

	spec, ok := r.Original.Object.Object["spec"].(map[string]interface{})
	if !ok {
		return result
	}

	template, ok := spec["template"].(map[string]interface{})
	if !ok {
		return result
	}

	podSpec, ok := template["spec"].(map[string]interface{})
	if !ok {
		return result
	}

	// Check containers and initContainers
	for _, containerKey := range []string{"containers", "initContainers"} {
		containers, ok := podSpec[containerKey].([]interface{})
		if !ok {
			continue
		}

		for _, c := range containers {
			container, ok := c.(map[string]interface{})
			if !ok {
				continue
			}

			envList, ok := container["env"].([]interface{})
			if !ok {
				continue
			}

			for _, e := range envList {
				envMap, ok := e.(map[string]interface{})
				if !ok {
					continue
				}
				name, _ := envMap["name"].(string)
				value, _ := envMap["value"].(string)
				if name != "" {
					result[name] = value
				}
			}
		}
	}

	return result
}

// buildCrossNamespaceIndex maps group name → list of foreign namespaces it communicates with.
func buildCrossNamespaceIndex(graph *types.ResourceGraph, groups []*ServiceGroup) map[string][]string {
	if graph == nil || len(graph.Relationships) == 0 {
		return make(map[string][]string)
	}

	// Build resource key → group mapping
	resourceToGroup := make(map[types.ResourceKey]*ServiceGroup)
	for _, g := range groups {
		for _, r := range g.Resources {
			resourceToGroup[r.Original.ResourceKey()] = g
		}
	}

	crossNS := make(map[string]map[string]bool)
	for _, rel := range graph.Relationships {
		fromGroup := resourceToGroup[rel.From]
		toGroup := resourceToGroup[rel.To]

		if fromGroup == nil || toGroup == nil {
			continue
		}
		if fromGroup.Namespace == toGroup.Namespace {
			continue
		}

		if crossNS[fromGroup.Name] == nil {
			crossNS[fromGroup.Name] = make(map[string]bool)
		}
		crossNS[fromGroup.Name][toGroup.Namespace] = true

		if crossNS[toGroup.Name] == nil {
			crossNS[toGroup.Name] = make(map[string]bool)
		}
		crossNS[toGroup.Name][fromGroup.Namespace] = true
	}

	result := make(map[string][]string)
	for name, nsMap := range crossNS {
		for ns := range nsMap {
			result[name] = append(result[name], ns)
		}
	}
	return result
}

// generateNetworkPolicy builds a NetworkPolicy YAML template.
func generateNetworkPolicy(group *ServiceGroup, ingressPorts, egressPorts []portInfo, crossNamespaces []string) string {
	var sb strings.Builder

	sb.WriteString("apiVersion: networking.k8s.io/v1\n")
	sb.WriteString("kind: NetworkPolicy\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s-netpol\n", group.Name))
	if group.Namespace != "" {
		sb.WriteString(fmt.Sprintf("  namespace: %s\n", group.Namespace))
	}
	sb.WriteString("spec:\n")
	sb.WriteString("  podSelector:\n")
	sb.WriteString("    matchLabels:\n")
	sb.WriteString(fmt.Sprintf("      app.kubernetes.io/name: %s\n", group.Name))
	sb.WriteString("  policyTypes:\n")
	sb.WriteString("    - Ingress\n")
	sb.WriteString("    - Egress\n")

	// Ingress rules
	sb.WriteString("  ingress:\n")
	if len(ingressPorts) > 0 {
		sb.WriteString("    - from:\n")
		sb.WriteString("        - podSelector: {}\n")
		sb.WriteString("      ports:\n")
		for _, p := range ingressPorts {
			sb.WriteString(fmt.Sprintf("        - port: %d\n", p.Port))
			sb.WriteString(fmt.Sprintf("          protocol: %s\n", p.Protocol))
		}
	} else {
		sb.WriteString("    - from:\n")
		sb.WriteString("        - podSelector: {}\n")
	}

	// Cross-namespace ingress
	for _, ns := range crossNamespaces {
		sb.WriteString("    - from:\n")
		sb.WriteString("        - namespaceSelector:\n")
		sb.WriteString("            matchLabels:\n")
		sb.WriteString(fmt.Sprintf("              kubernetes.io/metadata.name: %s\n", ns))
	}

	// Egress rules
	sb.WriteString("  egress:\n")

	// Always allow DNS
	sb.WriteString("    # Allow DNS\n")
	sb.WriteString("    - to:\n")
	sb.WriteString("        - namespaceSelector:\n")
	sb.WriteString("            matchLabels:\n")
	sb.WriteString("              kubernetes.io/metadata.name: kube-system\n")
	sb.WriteString("      ports:\n")
	sb.WriteString("        - port: 53\n")
	sb.WriteString("          protocol: UDP\n")
	sb.WriteString("        - port: 53\n")
	sb.WriteString("          protocol: TCP\n")

	// Env-based egress
	if len(egressPorts) > 0 {
		sb.WriteString("    # Detected service dependencies\n")
		sb.WriteString("    - ports:\n")
		for _, p := range egressPorts {
			sb.WriteString(fmt.Sprintf("        - port: %d\n", p.Port))
			sb.WriteString(fmt.Sprintf("          protocol: %s\n", p.Protocol))
		}
	}

	// Allow same-namespace
	sb.WriteString("    # Allow same-namespace\n")
	sb.WriteString("    - to:\n")
	sb.WriteString("        - podSelector: {}\n")

	return sb.String()
}
