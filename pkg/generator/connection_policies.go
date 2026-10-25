package generator

import (
	"fmt"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"strings"
)

func init() {
	RegisterFeature(Feature{
		Name:        "connection-policies",
		Description: "Generates targeted NetworkPolicy (egress/ingress) based on connection graphs",
		Params:      map[string]string{},
		Apply:       applyConnectionPolicies,
	})
}

func applyConnectionPolicies(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	result := &types.GeneratedChart{
		Name:          chart.Name,
		Path:          chart.Path,
		ChartYAML:     chart.ChartYAML,
		ValuesYAML:    chart.ValuesYAML,
		Templates:     make(map[string]string),
		Helpers:       chart.Helpers,
		Notes:         chart.Notes,
		ValuesSchema:  chart.ValuesSchema,
		ExternalFiles: chart.ExternalFiles,
	}
	for k, v := range chart.Templates {
		result.Templates[k] = v
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata:\n  name: %s-connections\nspec:\n", chart.Name)
	sb.WriteString("  podSelector:\n    matchLabels:\n")
	fmt.Fprintf(&sb, "      app: %s\n", chart.Name)
	sb.WriteString("  policyTypes:\n  - Ingress\n  - Egress\n")

	// Egress block
	sb.WriteString("  egress:\n")
	// Allow DNS
	sb.WriteString("  - ports:\n    - port: 53\n      protocol: UDP\n    - port: 53\n      protocol: TCP\n")

	// Collect target connections where From matches the chart name
	hasIngress := false
	if fc.Graph != nil {
		for _, conn := range fc.Graph.Connections {
			// Find group for From
			fromResource, ok := fc.Graph.Resources[conn.From]
			if ok && fromResource.ServiceName == chart.Name {
				// We have an egress connection from this chart
				if conn.To.Resource != nil {
					toResource, ok := fc.Graph.Resources[*conn.To.Resource]
					if ok {
						sb.WriteString("  - to:\n    - namespaceSelector: {}\n      podSelector:\n        matchLabels:\n")
						fmt.Fprintf(&sb, "          app: %s\n", toResource.ServiceName)
						if conn.Port > 0 {
							sb.WriteString("    ports:\n")
							fmt.Fprintf(&sb, "    - port: %d\n", conn.Port)
						}
					}
				} else if conn.To.Node != "" {
					// External host
					node, ok := fc.Graph.Nodes[conn.To.Node]
					if ok && (node.Kind == types.NodeExternalHost || node.Kind == types.NodeExternalIP) {
						fmt.Fprintf(&sb, "  - to:\n    - ipBlock: # %s\n        cidr: 0.0.0.0/0\n", node.Name)
						if conn.Port > 0 {
							sb.WriteString("    ports:\n")
							fmt.Fprintf(&sb, "    - port: %d\n", conn.Port)
						}
					}
				}
			}

			// Ingress connection (To matches chart)
			if conn.To.Resource != nil {
				toResource, ok := fc.Graph.Resources[*conn.To.Resource]
				if ok && toResource.ServiceName == chart.Name {
					if !hasIngress {
						sb.WriteString("  ingress:\n")
						hasIngress = true
					}
					fromResource, ok := fc.Graph.Resources[conn.From]
					if ok {
						sb.WriteString("  - from:\n    - namespaceSelector: {}\n      podSelector:\n        matchLabels:\n")
						fmt.Fprintf(&sb, "          app: %s\n", fromResource.ServiceName)
					}
				}
			}
		}
	}

	result.Templates["templates/connection-policy.yaml"] = sb.String()

	return result, nil
}
