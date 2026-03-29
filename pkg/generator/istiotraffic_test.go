package generator

// ============================================================
// Test Plan: Istio Traffic Management Generator (Task 5.8.2)
// ============================================================
//
// | #  | Test Name                                              | Category    | Input                                         | Expected Output                                                      |
// |----|--------------------------------------------------------|-------------|-----------------------------------------------|----------------------------------------------------------------------|
// |  1 | TestGenerateIstioTraffic_VirtualServiceGenerated       | happy       | graph with Service, opts default              | Templates contains VirtualService YAML                               |
// |  2 | TestGenerateIstioTraffic_DestinationRuleGenerated      | happy       | graph with Service, opts default              | Templates contains DestinationRule YAML                              |
// |  3 | TestGenerateIstioTraffic_PeerAuthMTLSEnabled           | happy       | EnableMTLS=true                               | PeerAuthentication YAML with mode: STRICT present                    |
// |  4 | TestGenerateIstioTraffic_TimeoutInVirtualService       | happy       | DefaultTimeout="10s"                          | VirtualService YAML contains "10s"                                   |
// |  5 | TestGenerateIstioTraffic_RetriesInVirtualService       | happy       | RetryAttempts=3                               | VirtualService YAML contains retry config with "3"                   |
// |  6 | TestGenerateIstioTraffic_EmptyGraph                    | edge        | empty graph                                   | Templates map empty, no panic                                        |
// |  7 | TestInjectIstioTraffic_NilChart                        | error       | nil chart                                     | returns (nil, 0), no panic                                           |
// |  8 | TestInjectIstioTraffic_CopyOnWrite                     | happy       | chart with templates + result                 | original chart.Templates unchanged after inject                      |
// |  9 | TestInjectIstioTraffic_Idempotent                      | happy       | inject twice same result                      | second inject count=0, no duplicate keys                             |
// | 10 | TestGenerateIstioTraffic_NOTESTxtContainsIstio         | happy       | any non-empty result                          | NOTESTxt mentions "istio" usage instructions                         |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── 1: VirtualService generated for Service in graph ─────────────────────────

func TestGenerateIstioTraffic_VirtualServiceGenerated(t *testing.T) {
	svc := makeProcessedResource("Service", "myapp", "default", nil)
	graph := buildGraph([]*types.ProcessedResource{svc}, nil)

	opts := IstioTrafficOptions{
		EnableMTLS:     false,
		DefaultTimeout: "5s",
		RetryAttempts:  2,
	}

	result := GenerateIstioTraffic(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil IstioTrafficResult")
	}
	if len(result.Templates) == 0 {
		t.Fatal("expected at least one template for VirtualService")
	}

	found := false
	for _, yaml := range result.Templates {
		if strings.Contains(yaml, "VirtualService") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected VirtualService in Templates, got keys: %v", keysOfMap(result.Templates))
	}
}

// ── 2: DestinationRule generated for Service in graph ────────────────────────

func TestGenerateIstioTraffic_DestinationRuleGenerated(t *testing.T) {
	svc := makeProcessedResource("Service", "myapp", "default", nil)
	graph := buildGraph([]*types.ProcessedResource{svc}, nil)

	opts := IstioTrafficOptions{
		EnableMTLS:     false,
		DefaultTimeout: "5s",
		RetryAttempts:  2,
	}

	result := GenerateIstioTraffic(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil IstioTrafficResult")
	}

	found := false
	for _, yaml := range result.Templates {
		if strings.Contains(yaml, "DestinationRule") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected DestinationRule in Templates, got keys: %v", keysOfMap(result.Templates))
	}
}

// ── 3: PeerAuthentication with mTLS STRICT when EnableMTLS=true ──────────────

func TestGenerateIstioTraffic_PeerAuthMTLSEnabled(t *testing.T) {
	svc := makeProcessedResource("Service", "secure-app", "default", nil)
	graph := buildGraph([]*types.ProcessedResource{svc}, nil)

	opts := IstioTrafficOptions{
		EnableMTLS:     true,
		DefaultTimeout: "5s",
		RetryAttempts:  0,
	}

	result := GenerateIstioTraffic(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil IstioTrafficResult")
	}

	found := false
	for _, yaml := range result.Templates {
		if strings.Contains(yaml, "PeerAuthentication") {
			if strings.Contains(yaml, "STRICT") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("expected PeerAuthentication with mode: STRICT when EnableMTLS=true, got templates: %v", keysOfMap(result.Templates))
	}
}

// ── 4: DefaultTimeout appears in VirtualService YAML ─────────────────────────

func TestGenerateIstioTraffic_TimeoutInVirtualService(t *testing.T) {
	svc := makeProcessedResource("Service", "timeout-app", "default", nil)
	graph := buildGraph([]*types.ProcessedResource{svc}, nil)

	opts := IstioTrafficOptions{
		EnableMTLS:     false,
		DefaultTimeout: "10s",
		RetryAttempts:  0,
	}

	result := GenerateIstioTraffic(graph, opts)

	for _, yaml := range result.Templates {
		if strings.Contains(yaml, "VirtualService") {
			if !strings.Contains(yaml, "10s") {
				t.Errorf("VirtualService YAML must contain timeout '10s':\n%s", yaml)
			}
			return
		}
	}
	t.Error("no VirtualService template found in result")
}

// ── 5: RetryAttempts appears in VirtualService YAML ──────────────────────────

func TestGenerateIstioTraffic_RetriesInVirtualService(t *testing.T) {
	svc := makeProcessedResource("Service", "retry-app", "default", nil)
	graph := buildGraph([]*types.ProcessedResource{svc}, nil)

	opts := IstioTrafficOptions{
		EnableMTLS:     false,
		DefaultTimeout: "5s",
		RetryAttempts:  3,
	}

	result := GenerateIstioTraffic(graph, opts)

	for _, yaml := range result.Templates {
		if strings.Contains(yaml, "VirtualService") {
			if !strings.Contains(yaml, "3") {
				t.Errorf("VirtualService YAML must contain retry attempts '3':\n%s", yaml)
			}
			// retries block must be present
			if !strings.Contains(yaml, "retries") && !strings.Contains(yaml, "attempts") {
				t.Errorf("VirtualService YAML must contain retries block:\n%s", yaml)
			}
			return
		}
	}
	t.Error("no VirtualService template found in result")
}

// ── 6: empty graph returns empty Templates map without panic ──────────────────

func TestGenerateIstioTraffic_EmptyGraph(t *testing.T) {
	graph := types.NewResourceGraph()

	opts := IstioTrafficOptions{
		EnableMTLS:     true,
		DefaultTimeout: "5s",
		RetryAttempts:  2,
	}

	result := GenerateIstioTraffic(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil IstioTrafficResult even for empty graph")
	}
	if len(result.Templates) != 0 {
		t.Errorf("expected 0 templates for empty graph, got %d: %v", len(result.Templates), keysOfMap(result.Templates))
	}
}

// ── 7: InjectIstioTraffic with nil chart returns (nil, 0) ────────────────────

func TestInjectIstioTraffic_NilChart(t *testing.T) {
	result := &IstioTrafficResult{
		Templates: map[string]string{
			"templates/istio-vs-myapp.yaml": "apiVersion: networking.istio.io/v1beta1\nkind: VirtualService\n",
		},
	}

	newChart, count := InjectIstioTraffic(nil, result)

	if newChart != nil {
		t.Errorf("expected nil chart for nil input, got %+v", newChart)
	}
	if count != 0 {
		t.Errorf("expected count=0 for nil chart, got %d", count)
	}
}

// ── 8: InjectIstioTraffic is copy-on-write ───────────────────────────────────

func TestInjectIstioTraffic_CopyOnWrite(t *testing.T) {
	originalContent := testDeploymentTemplate
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": originalContent,
	})
	result := &IstioTrafficResult{
		Templates: map[string]string{
			"templates/istio-vs-myapp.yaml": "apiVersion: networking.istio.io/v1beta1\nkind: VirtualService\nmetadata:\n  name: myapp\n",
		},
	}

	newChart, _ := InjectIstioTraffic(chart, result)

	if newChart == chart {
		t.Error("InjectIstioTraffic must return a new chart (copy-on-write)")
	}
	if chart.Templates["templates/deployment.yaml"] != originalContent {
		t.Error("original chart template must not be modified by InjectIstioTraffic")
	}
	if len(chart.Templates) != 1 {
		t.Errorf("original chart.Templates must have exactly 1 key, got %d", len(chart.Templates))
	}
}

// ── 9: InjectIstioTraffic is idempotent ──────────────────────────────────────

func TestInjectIstioTraffic_Idempotent(t *testing.T) {
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": testDeploymentTemplate,
	})
	result := &IstioTrafficResult{
		Templates: map[string]string{
			"templates/istio-vs-myapp.yaml": "apiVersion: networking.istio.io/v1beta1\nkind: VirtualService\nmetadata:\n  name: myapp\n",
		},
	}

	firstChart, firstCount := InjectIstioTraffic(chart, result)
	if firstCount == 0 {
		t.Errorf("expected first inject to add templates, got count=%d", firstCount)
	}

	keysBefore := len(firstChart.Templates)
	secondChart, secondCount := InjectIstioTraffic(firstChart, result)

	if secondCount != 0 {
		t.Errorf("second inject should be idempotent (count=0), got count=%d", secondCount)
	}
	if len(secondChart.Templates) != keysBefore {
		t.Errorf("idempotent inject must not add duplicate templates: before=%d after=%d", keysBefore, len(secondChart.Templates))
	}
}

// ── 10: NOTESTxt mentions istio ───────────────────────────────────────────────

func TestGenerateIstioTraffic_NOTESTxtContainsIstio(t *testing.T) {
	svc := makeProcessedResource("Service", "info-svc", "default", nil)
	graph := buildGraph([]*types.ProcessedResource{svc}, nil)

	opts := IstioTrafficOptions{
		EnableMTLS:     true,
		DefaultTimeout: "5s",
		RetryAttempts:  1,
	}

	result := GenerateIstioTraffic(graph, opts)

	lower := strings.ToLower(result.NOTESTxt)
	if !strings.Contains(lower, "istio") {
		t.Errorf("NOTESTxt should contain 'istio' usage instructions, got:\n%s", result.NOTESTxt)
	}
}

// ── local helper (scoped to avoid collision) ─────────────────────────────────

func keysOfMap(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
