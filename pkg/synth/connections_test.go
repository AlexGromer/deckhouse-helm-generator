package synth

import (
	"reflect"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/connect"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestExtractSynthConnections(t *testing.T) {
	fromKey := types.ResourceKey{Namespace: "test", Name: "app"}
	idx := connect.NewIndex(nil)

	tests := []struct {
		name      string
		props     map[string]string
		wantConns []types.Connection
		wantNotes []string
	}{
		{
			name: "spring jdbc url",
			props: map[string]string{
				"spring.datasource.url": "jdbc:postgresql://db:5432/mydb",
			},
			wantConns: []types.Connection{
				{
					From:       fromKey,
					To:         types.ConnectionTarget{Node: "external-host:db"},
					Type:       "external",
					Port:       5432,
					Attributes: map[string]string{"database": "mydb"},
					Evidence: []types.Evidence{{
						Source: types.EvidenceSynthesized,
						Key:    "spring.datasource.url",
						Value:  "jdbc:postgresql://db:5432/mydb",
					}},
				},
			},
		},
		{
			name: "kafka bootstrap servers",
			props: map[string]string{
				"spring.kafka.bootstrap-servers": "kafka1:9092,kafka2:9092",
			},
			wantConns: []types.Connection{
				{
					From: fromKey,
					To:   types.ConnectionTarget{Node: "external-host:kafka1"},
					Type: "external",
					Port: 9092,
					Evidence: []types.Evidence{{
						Source: types.EvidenceSynthesized,
						Key:    "spring.kafka.bootstrap-servers",
						Value:  "kafka1:9092,kafka2:9092",
					}},
				},
				{
					From: fromKey,
					To:   types.ConnectionTarget{Node: "external-host:kafka2"},
					Type: "external",
					Port: 9092,
					Evidence: []types.Evidence{{
						Source: types.EvidenceSynthesized,
						Key:    "spring.kafka.bootstrap-servers",
						Value:  "kafka1:9092,kafka2:9092",
					}},
				},
			},
		},
		{
			name: "placeholder",
			props: map[string]string{
				"spring.datasource.url": "${DB_URL}",
			},
			wantNotes: []string{
				"Note: spring.datasource.url is unparseable placeholder \"${DB_URL}\"",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := types.NewResourceGraph()
			conns, notes := ExtractSynthConnections(tt.props, fromKey, "app", idx, graph)

			if len(conns) != len(tt.wantConns) {
				t.Errorf("got %d conns, want %d", len(conns), len(tt.wantConns))
			} else {
				for i := range conns {
					if conns[i].From != tt.wantConns[i].From {
						t.Errorf("conn %d From got %v, want %v", i, conns[i].From, tt.wantConns[i].From)
					}
					if conns[i].To != tt.wantConns[i].To {
						t.Errorf("conn %d To got %v, want %v", i, conns[i].To, tt.wantConns[i].To)
					}
					if conns[i].Protocol != tt.wantConns[i].Protocol {
						t.Errorf("conn %d Protocol got %v, want %v", i, conns[i].Protocol, tt.wantConns[i].Protocol)
					}
					if conns[i].Port != tt.wantConns[i].Port {
						t.Errorf("conn %d Port got %v, want %v", i, conns[i].Port, tt.wantConns[i].Port)
					}
					if len(conns[i].Evidence) > 0 && len(tt.wantConns[i].Evidence) > 0 && conns[i].Evidence[0].Key != tt.wantConns[i].Evidence[0].Key {
						t.Errorf("conn %d Evidence.Key got %v, want %v", i, conns[i].Evidence[0].Key, tt.wantConns[i].Evidence[0].Key)
					}
					if !reflect.DeepEqual(conns[i].Attributes, tt.wantConns[i].Attributes) {
						t.Errorf("conn %d Attributes got %v, want %v", i, conns[i].Attributes, tt.wantConns[i].Attributes)
					}
				}
			}

			if !reflect.DeepEqual(notes, tt.wantNotes) {
				t.Errorf("got notes %v, want %v", notes, tt.wantNotes)
			}
		})
	}
}
