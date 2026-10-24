package analyzer

import (
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"strings"
	"testing"
)

func TestRenderServiceGraphDOT(t *testing.T) {
	g := types.NewResourceGraph()
	g.AddGroup(&types.ResourceGroup{Name: "svc1"})
	dot := RenderServiceGraphDOT(g)
	if !strings.Contains(dot, "svc1") {
		t.Errorf("Expected 'svc1' in DOT, got: %s", dot)
	}
}

func TestRenderPBCGraphDOT(t *testing.T) {
	g := types.NewResourceGraph()
	topology := &types.ServiceTopology{
		PBCToServices: map[string][]string{"pbc1": {"svc1"}},
	}
	dot := RenderPBCGraphDOT(g, topology)
	if !strings.Contains(dot, "pbc1") {
		t.Errorf("Expected 'pbc1' in DOT, got: %s", dot)
	}
}

func TestRenderServiceGraphMermaid(t *testing.T) {
	g := types.NewResourceGraph()
	g.AddGroup(&types.ResourceGroup{Name: "svc-1"})
	mermaid := RenderServiceGraphMermaid(g)
	if !strings.Contains(mermaid, "svc_1[svc-1]") {
		t.Errorf("Expected 'svc_1[svc-1]' in Mermaid, got: %s", mermaid)
	}
}

func TestRenderPBCGraphMermaid(t *testing.T) {
	g := types.NewResourceGraph()
	topology := &types.ServiceTopology{
		PBCToServices: map[string][]string{"pbc-1": {"svc1"}},
	}
	mermaid := RenderPBCGraphMermaid(g, topology)
	if !strings.Contains(mermaid, "pbc_1[pbc-1]") {
		t.Errorf("Expected 'pbc_1[pbc-1]' in Mermaid, got: %s", mermaid)
	}
}

func TestFormatGraphJSON(t *testing.T) {
	g := types.NewResourceGraph()
	g.AddGroup(&types.ResourceGroup{Name: "svc1"})
	topology := &types.ServiceTopology{
		PBCToServices: map[string][]string{"pbc1": {"svc1"}},
	}
	jsonStr := FormatGraphJSON(g, topology)
	if !strings.Contains(jsonStr, "\"svc1\"") {
		t.Errorf("Expected 'svc1' in JSON, got: %s", jsonStr)
	}
}

func TestGeneratePBCReport(t *testing.T) {
	g := types.NewResourceGraph()
	topology := &types.ServiceTopology{
		PBCToServices:    map[string][]string{"pbc1": {"svc1"}},
		PlatformServices: []string{"db1"},
	}
	report := GeneratePBCReport(g, topology)
	if !strings.Contains(report, "pbc1") || !strings.Contains(report, "platform") {
		t.Errorf("Expected 'pbc1' and 'platform' in report, got: %s", report)
	}
}
