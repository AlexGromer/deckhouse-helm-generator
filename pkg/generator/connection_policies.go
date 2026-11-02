package generator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
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

	// Rules are collected apart from the head of the policy: a policy type is
	// only listed when rules for it are known, because a listed type with no
	// rules denies all of that traffic.
	var egressRules, ingressRules []string
	seenEgress := make(map[string]bool)
	seenIngress := make(map[string]bool)
	addUnique := func(rules *[]string, seen map[string]bool, rule string) {
		if !seen[rule] {
			seen[rule] = true
			*rules = append(*rules, rule)
		}
	}

	if fc.Graph != nil {
		for _, conn := range fc.Graph.Connections {
			// Find group for From
			fromResource, fromOK := fc.Graph.Resources[conn.From]
			if fromOK && fromResource.ServiceName == chart.Name {
				// We have an egress connection from this chart
				if conn.To.Resource != nil {
					toResource, ok := fc.Graph.Resources[*conn.To.Resource]
					if ok {
						var sb strings.Builder
						sb.WriteString("  - to:\n    - namespaceSelector: {}\n      podSelector:\n        matchLabels:\n")
						writeSelectorLabels(&sb, peerSelectorLabels(fc.Graph, toResource.ServiceName), 10)
						if conn.Port > 0 {
							sb.WriteString("    ports:\n")
							fmt.Fprintf(&sb, "    - port: %d\n", conn.Port)
						}
						addUnique(&egressRules, seenEgress, sb.String())
					}
				} else if conn.To.Node != "" {
					// External host
					node, ok := fc.Graph.Nodes[conn.To.Node]
					if ok && (node.Kind == types.NodeExternalHost || node.Kind == types.NodeExternalIP) {
						var sb strings.Builder
						fmt.Fprintf(&sb, "  - to:\n    - ipBlock: # %s\n        cidr: 0.0.0.0/0\n", node.Name)
						if conn.Port > 0 {
							sb.WriteString("    ports:\n")
							fmt.Fprintf(&sb, "    - port: %d\n", conn.Port)
						}
						addUnique(&egressRules, seenEgress, sb.String())
					}
				}
			}

			// Ingress connection (To matches chart)
			if conn.To.Resource != nil {
				toResource, ok := fc.Graph.Resources[*conn.To.Resource]
				if ok && toResource.ServiceName == chart.Name && fromOK {
					var sb strings.Builder
					sb.WriteString("  - from:\n    - namespaceSelector: {}\n      podSelector:\n        matchLabels:\n")
					writeSelectorLabels(&sb, peerSelectorLabels(fc.Graph, fromResource.ServiceName), 10)
					addUnique(&ingressRules, seenIngress, sb.String())
				}
			}
		}
	}

	hasEgress := len(egressRules) > 0
	hasIngress := len(ingressRules) > 0
	hasEdges := hasEgress || hasIngress

	if !hasEdges {
		fc.Note(fmt.Sprintf("connection-policies: no connections known for %s; the NetworkPolicy is disabled by default (connectionPolicies.enabled=false)", chart.Name))
	} else if !hasIngress {
		fc.Note(fmt.Sprintf("connection-policies: no ingress sources known for %s; ingress is not restricted (Ingress/NodePort traffic keeps working)", chart.Name))
	} else if !hasEgress {
		fc.Note(fmt.Sprintf("connection-policies: no egress targets known for %s; egress is not restricted (calls to external APIs, object storage and the like keep working)", chart.Name))
	}

	prefix := opsHelperPrefix(chart)
	selector := serviceSelectorLabels(fc.Graph, chart.Name)

	var sb strings.Builder
	sb.WriteString("{{- if (.Values.connectionPolicies).enabled }}\n")
	fmt.Fprintf(&sb, "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata:\n  name: %s-connections\nspec:\n", chart.Name)
	sb.WriteString("  podSelector:\n    matchLabels:\n")
	if len(selector) > 0 {
		writeSelectorLabels(&sb, selector, 6)
	} else {
		fmt.Fprintf(&sb, "      {{- include %q . | nindent 6 }}\n", prefix+".selectorLabels")
	}

	// A direction is only denied when the peers for it are known; otherwise it
	// is left open. With no edges at all the policy is an opt-in egress
	// allowlist (DNS only), disabled by default.
	listEgress := hasEgress || !hasIngress
	sb.WriteString("  policyTypes:\n")
	if hasIngress {
		sb.WriteString("  - Ingress\n")
	}
	if listEgress {
		sb.WriteString("  - Egress\n")
		sb.WriteString("  egress:\n")
		// Allow DNS
		sb.WriteString("  - ports:\n    - port: 53\n      protocol: UDP\n    - port: 53\n      protocol: TCP\n")
		for _, r := range egressRules {
			sb.WriteString(r)
		}
	}
	if hasIngress {
		sb.WriteString("  ingress:\n")
		for _, r := range ingressRules {
			sb.WriteString(r)
		}
	}
	sb.WriteString("{{- end }}\n")

	result.Templates["templates/connection-policy.yaml"] = sb.String()

	newValues, err := appendTopLevelValues(result.ValuesYAML, "connectionPolicies", map[string]interface{}{"enabled": hasEdges})
	if err != nil {
		return nil, fmt.Errorf("failed to append connectionPolicies values: %w", err)
	}
	result.ValuesYAML = newValues

	return result, nil
}

// serviceSelectorLabels returns the pod selector labels of the workloads
// (Deployment, StatefulSet, DaemonSet) the graph holds for serviceName: the
// labels every one of them selects on. It is nil when no workload carries a
// selector, e.g. the graph is missing.
func serviceSelectorLabels(g *types.ResourceGraph, serviceName string) map[string]string {
	if g == nil {
		return nil
	}
	var common map[string]string
	for _, res := range g.Resources {
		if res == nil || res.ServiceName != serviceName {
			continue
		}
		labels := workloadMatchLabels(res)
		if len(labels) == 0 {
			continue
		}
		if common == nil {
			common = labels
			continue
		}
		for k, v := range common {
			if labels[k] != v {
				delete(common, k)
			}
		}
	}
	if len(common) == 0 {
		return nil
	}
	return common
}

// peerSelectorLabels is serviceSelectorLabels for the other end of a
// connection; a service without a known selector falls back to app=<service>
// so the peer is never an empty (match-everything) selector.
func peerSelectorLabels(g *types.ResourceGraph, serviceName string) map[string]string {
	if labels := serviceSelectorLabels(g, serviceName); len(labels) > 0 {
		return labels
	}
	return map[string]string{"app": serviceName}
}

// workloadMatchLabels reads spec.selector.matchLabels of a processed
// workload from its extracted values.
func workloadMatchLabels(res *types.ProcessedResource) map[string]string {
	selector, ok := res.Values["selector"].(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]string)
	switch ml := selector["matchLabels"].(type) {
	case map[string]interface{}:
		for k, v := range ml {
			out[k] = fmt.Sprint(v)
		}
	case map[string]string:
		for k, v := range ml {
			out[k] = v
		}
	}
	return out
}

// writeSelectorLabels writes labels as sorted, quoted YAML map entries
// indented by indent spaces.
func writeSelectorLabels(sb *strings.Builder, labels map[string]string, indent int) {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pad := strings.Repeat(" ", indent)
	for _, k := range keys {
		fmt.Fprintf(sb, "%s%s: %q\n", pad, k, labels[k])
	}
}
