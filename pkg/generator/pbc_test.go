package generator

import (
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"reflect"
	"testing"
)

func TestParsePBCMapping(t *testing.T) {
	tests := []struct {
		name    string
		specs   []string
		want    map[string]string
		wantErr bool
	}{
		{
			name:  "valid mapping single svc",
			specs: []string{"auth=keycloak"},
			want:  map[string]string{"keycloak": "auth"},
		},
		{
			name:  "valid mapping multiple svcs",
			specs: []string{"auth=keycloak,dex"},
			want:  map[string]string{"keycloak": "auth", "dex": "auth"},
		},
		{
			name:  "multiple valid mappings",
			specs: []string{"auth=keycloak,dex", "shop=orders,payments"},
			want:  map[string]string{"keycloak": "auth", "dex": "auth", "orders": "shop", "payments": "shop"},
		},
		{
			name:    "invalid format",
			specs:   []string{"authkeycloak"},
			wantErr: true,
		},
		{
			name:    "empty PBC",
			specs:   []string{"=keycloak"},
			wantErr: true,
		},
		{
			name:    "service mapped to multiple PBCs",
			specs:   []string{"auth=keycloak", "shop=keycloak"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePBCMapping(tt.specs)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePBCMapping() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParsePBCMapping() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveTopology(t *testing.T) {
	g := types.NewResourceGraph()
	g.AddGroup(&types.ResourceGroup{Name: "svc1"})
	explicitMap := map[string]string{"svc1": "pbc1"}

	topology := ResolveTopology(g, explicitMap)
	if topology.ServicePBC["svc1"] != "pbc1" {
		t.Errorf("Expected 'pbc1', got %s", topology.ServicePBC["svc1"])
	}
}

func TestGeneratePBCUmbrellaChart(t *testing.T) {
	topology := &types.ServiceTopology{
		PBCToServices: map[string][]string{"pbc1": {"svc1"}},
	}
	res := GeneratePBCUmbrellaChart(topology, "out")
	if len(res) == 0 {
		t.Errorf("Expected non-empty result")
	}
}

func TestGenerateSeparateLayout(t *testing.T) {
	topology := &types.ServiceTopology{
		PBCToServices: map[string][]string{"pbc1": {"svc1"}},
	}
	res := GenerateSeparateLayout(topology, "out")
	if len(res) == 0 {
		t.Errorf("Expected non-empty result")
	}
}

func TestGenerateLibraryLayout(t *testing.T) {
	topology := &types.ServiceTopology{
		PBCToServices: map[string][]string{"pbc1": {"svc1"}},
	}
	res := GenerateLibraryLayout(topology, "out")
	if len(res) == 0 {
		t.Errorf("Expected non-empty result")
	}
}
