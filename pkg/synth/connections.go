package synth

import (
	"fmt"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/connect"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// ExtractSynthConnections extracts connections from synthesized project properties.
func ExtractSynthConnections(props map[string]string, fromKey types.ResourceKey, containerName string, idx *connect.ClusterIndex, graph *types.ResourceGraph) ([]types.Connection, []string) {
	var conns []types.Connection
	var notes []string

	for k, v := range props {
		if !connect.MatchProperty(k) && !connect.MatchEnv(k) {
			continue
		}

		if strings.Contains(v, "${") {
			notes = append(notes, fmt.Sprintf("Note: %s is unparseable placeholder %q", k, v))
			continue
		}

		eps := connect.ParseValue(v)
		for _, ep := range eps {
			redacted, _ := connect.Redact(v)
			evidence := types.Evidence{
				Source: types.EvidenceSynthesized,
				Key:    k,
				Value:  redacted,
			}

			conn := types.Connection{
				From:     fromKey,
				Evidence: []types.Evidence{evidence},
				Port:     int32(ep.Port),
			}

			if ep.Database != "" {
				if conn.Attributes == nil {
					conn.Attributes = make(map[string]string)
				}
				conn.Attributes["database"] = ep.Database
			}
			if ep.Path != "" {
				if conn.Attributes == nil {
					conn.Attributes = make(map[string]string)
				}
				conn.Attributes["Path"] = ep.Path
			}

			if idx != nil {
				resolved := idx.Resolve(ep, fromKey.Namespace)
				conn.To = resolved.Target
				conn.Type = types.RelationshipType(resolved.Role)
			} else {
				conn.To = types.ConnectionTarget{Node: types.NodeID("external-host:" + ep.Host)}
				conn.Type = "external"
			}

			conns = append(conns, conn)

			if graph != nil {
				graph.Connections = append(graph.Connections, conn)
			}
		}
	}

	return conns, notes
}
