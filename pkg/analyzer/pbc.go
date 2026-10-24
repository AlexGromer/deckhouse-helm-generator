package analyzer

import (
	"encoding/json"
	"fmt"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"strings"
)

// RenderServiceGraphDOT outputs a DOT graph string traversing nodes and connections at the service level.
func RenderServiceGraphDOT(graph *types.ResourceGraph) string {
	var sb strings.Builder
	sb.WriteString("digraph ServiceGraph {\n")
	sb.WriteString("  rankdir=LR;\n")

	for _, group := range graph.Groups {
		fmt.Fprintf(&sb, "  \"%s\" [label=\"%s\", shape=box];\n", group.Name, group.Name)
	}

	if graph != nil {
		for _, conn := range graph.Connections {
			fromSvc := ""
			fromRes, ok := graph.Resources[conn.From]
			if ok {
				fromSvc = fromRes.ServiceName
			}

			toSvc := ""
			if conn.To.Resource != nil {
				toRes, ok := graph.Resources[*conn.To.Resource]
				if ok {
					toSvc = toRes.ServiceName
				}
			} else if conn.To.Node != "" {
				toSvc = string(conn.To.Node)
			}

			if fromSvc != "" && toSvc != "" {
				fmt.Fprintf(&sb, "  \"%s\" -> \"%s\";\n", fromSvc, toSvc)
			}
		}
	}

	sb.WriteString("}\n")
	return sb.String()
}

// RenderPBCGraphDOT outputs a DOT graph string traversing nodes and connections at the PBC level.
func RenderPBCGraphDOT(graph *types.ResourceGraph, topology *types.ServiceTopology) string {
	var sb strings.Builder
	sb.WriteString("digraph PBCGraph {\n")
	sb.WriteString("  rankdir=LR;\n")

	if topology != nil {
		for pbc := range topology.PBCToServices {
			fmt.Fprintf(&sb, "  \"%s\" [label=\"%s\", shape=folder];\n", pbc, pbc)
		}

		if graph != nil {
			for _, conn := range graph.Connections {
				fromSvc := ""
				fromRes, ok := graph.Resources[conn.From]
				if ok {
					fromSvc = fromRes.ServiceName
				}

				toSvc := ""
				if conn.To.Resource != nil {
					toRes, ok := graph.Resources[*conn.To.Resource]
					if ok {
						toSvc = toRes.ServiceName
					}
				} else if conn.To.Node != "" {
					toSvc = string(conn.To.Node)
				}

				if fromSvc != "" && toSvc != "" {
					fromPBC := topology.ServicePBC[fromSvc]
					toPBC := topology.ServicePBC[toSvc]
					if toPBC == "" {
						toPBC = "external" // Group external nodes for PBC graph
					}

					if fromPBC != "" && fromPBC != toPBC {
						fmt.Fprintf(&sb, "  \"%s\" -> \"%s\";\n", fromPBC, toPBC)
					}
				}
			}
		}
	}

	sb.WriteString("}\n")
	return sb.String()
}

// RenderServiceGraphMermaid outputs a Mermaid graph string traversing nodes and connections at the service level.
func RenderServiceGraphMermaid(graph *types.ResourceGraph) string {
	var sb strings.Builder
	sb.WriteString("graph LR\n")

	for _, group := range graph.Groups {
		fmt.Fprintf(&sb, "  %s[%s]\n", sanitizeMermaid(group.Name), group.Name)
	}

	if graph != nil {
		for _, conn := range graph.Connections {
			fromSvc := ""
			fromRes, ok := graph.Resources[conn.From]
			if ok {
				fromSvc = fromRes.ServiceName
			}

			toSvc := ""
			if conn.To.Resource != nil {
				toRes, ok := graph.Resources[*conn.To.Resource]
				if ok {
					toSvc = toRes.ServiceName
				}
			} else if conn.To.Node != "" {
				toSvc = string(conn.To.Node)
			}

			if fromSvc != "" && toSvc != "" {
				fmt.Fprintf(&sb, "  %s --> %s\n", sanitizeMermaid(fromSvc), sanitizeMermaid(toSvc))
			}
		}
	}

	return sb.String()
}

// RenderPBCGraphMermaid outputs a Mermaid graph string traversing nodes and connections at the PBC level.
func RenderPBCGraphMermaid(graph *types.ResourceGraph, topology *types.ServiceTopology) string {
	var sb strings.Builder
	sb.WriteString("graph LR\n")

	if topology != nil {
		for pbc := range topology.PBCToServices {
			fmt.Fprintf(&sb, "  %s[%s]\n", sanitizeMermaid(pbc), pbc)
		}

		if graph != nil {
			for _, conn := range graph.Connections {
				fromSvc := ""
				fromRes, ok := graph.Resources[conn.From]
				if ok {
					fromSvc = fromRes.ServiceName
				}

				toSvc := ""
				if conn.To.Resource != nil {
					toRes, ok := graph.Resources[*conn.To.Resource]
					if ok {
						toSvc = toRes.ServiceName
					}
				} else if conn.To.Node != "" {
					toSvc = string(conn.To.Node)
				}

				if fromSvc != "" && toSvc != "" {
					fromPBC := topology.ServicePBC[fromSvc]
					toPBC := topology.ServicePBC[toSvc]
					if toPBC == "" {
						toPBC = "external"
					}

					if fromPBC != "" && fromPBC != toPBC {
						fmt.Fprintf(&sb, "  %s --> %s\n", sanitizeMermaid(fromPBC), sanitizeMermaid(toPBC))
					}
				}
			}
		}
	}

	return sb.String()
}

// FormatGraphJSON returns a JSON representation of the graph.
func FormatGraphJSON(graph *types.ResourceGraph, topology *types.ServiceTopology) string {
	data := struct {
		Groups      []string            `json:"groups"`
		Topology    map[string][]string `json:"topology,omitempty"`
		Connections []map[string]string `json:"connections"`
	}{
		Groups:      make([]string, 0),
		Connections: make([]map[string]string, 0),
	}

	for _, group := range graph.Groups {
		data.Groups = append(data.Groups, group.Name)
	}

	if topology != nil {
		data.Topology = topology.PBCToServices
	}

	if graph != nil {
		for _, conn := range graph.Connections {
			fromSvc := ""
			fromRes, ok := graph.Resources[conn.From]
			if ok {
				fromSvc = fromRes.ServiceName
			}

			toSvc := ""
			if conn.To.Resource != nil {
				toRes, ok := graph.Resources[*conn.To.Resource]
				if ok {
					toSvc = toRes.ServiceName
				}
			} else if conn.To.Node != "" {
				toSvc = string(conn.To.Node)
			}

			if fromSvc != "" && toSvc != "" {
				c := map[string]string{
					"from":     fromSvc,
					"to":       toSvc,
					"protocol": conn.Protocol,
					"type":     string(conn.Type),
				}
				data.Connections = append(data.Connections, c)
			}
		}
	}

	b, _ := json.MarshalIndent(data, "", "  ")
	return string(b)
}

func sanitizeMermaid(s string) string {
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, ":", "_")
	return s
}

// GeneratePBCReport outputs a markdown table of PBC summary, internal connections list, cross-PBC contract matrix, external dependencies, and architectural warnings block.
func GeneratePBCReport(graph *types.ResourceGraph, topology *types.ServiceTopology) string {
	var sb strings.Builder

	sb.WriteString("# PBC Report\n\n")

	if topology == nil {
		sb.WriteString("No topology provided.\n")
		return sb.String()
	}

	sb.WriteString("## PBC Summary\n\n")
	sb.WriteString("| PBC | Services |\n")
	sb.WriteString("|---|---|\n")
	for pbc, svcs := range topology.PBCToServices {
		fmt.Fprintf(&sb, "| %s | %d (%s) |\n", pbc, len(svcs), strings.Join(svcs, ", "))
	}

	if len(topology.PlatformServices) > 0 {
		fmt.Fprintf(&sb, "| platform | %d (%s) |\n", len(topology.PlatformServices), strings.Join(topology.PlatformServices, ", "))
	}

	sb.WriteString("\n## Internal Connections\n\n")
	sb.WriteString("| From | To | Protocol | Port | Type |\n")
	sb.WriteString("|---|---|---|---|---|\n")

	matrix := make(map[string]map[string]int)
	external := make(map[string][]string) // target -> [protocol, source]

	if graph != nil {
		for _, conn := range graph.Connections {
			fromSvc := ""
			fromRes, ok := graph.Resources[conn.From]
			if ok {
				fromSvc = fromRes.ServiceName
			}

			toStr := ""
			toSvc := ""
			if conn.To.Resource != nil {
				toRes, ok := graph.Resources[*conn.To.Resource]
				if ok {
					toStr = toRes.ServiceName
					toSvc = toRes.ServiceName
				}
			} else if conn.To.Node != "" {
				node, ok := graph.Nodes[conn.To.Node]
				if ok {
					toStr = string(conn.To.Node)
					if node.Kind == types.NodeExternalHost || node.Kind == types.NodeExternalIP {
						external[toStr] = []string{conn.Protocol, fromSvc}
					}
				}
			}

			if fromSvc != "" && toStr != "" {
				fmt.Fprintf(&sb, "| %s | %s | %s | %d | %s |\n", fromSvc, toStr, conn.Protocol, conn.Port, conn.Type)
			}

			if fromSvc != "" && toSvc != "" {
				fromPBC := topology.ServicePBC[fromSvc]
				toPBC := topology.ServicePBC[toSvc]
				if fromPBC != toPBC && fromPBC != "" && toPBC != "" {
					if matrix[fromPBC] == nil {
						matrix[fromPBC] = make(map[string]int)
					}
					matrix[fromPBC][toPBC]++
				}
			}
		}
	}

	sb.WriteString("\n## Cross-PBC Contract Matrix\n\n")
	sb.WriteString("| Source PBC | Target PBC | Connection Count |\n")
	sb.WriteString("|---|---|---|\n")

	for src, targets := range matrix {
		for target, count := range targets {
			fmt.Fprintf(&sb, "| %s | %s | %d |\n", src, target, count)
		}
	}

	sb.WriteString("\n## External Dependencies\n\n")
	sb.WriteString("| Target | Protocol | Source |\n")
	sb.WriteString("|---|---|---|\n")
	for target, info := range external {
		fmt.Fprintf(&sb, "| %s | %s | %s |\n", target, info[0], info[1])
	}

	sb.WriteString("\n## Architectural Warnings\n\n")
	if len(topology.PlatformServices) > 0 {
		sb.WriteString("⚠️ **Warning**: Shared databases/brokers found in the 'platform' PBC. This is an anti-pattern. Consider isolating databases per service.\n")
	} else {
		sb.WriteString("No warnings.\n")
	}

	return sb.String()
}
