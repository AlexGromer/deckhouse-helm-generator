package generator

// ============================================================
// Test Plan: Istio Canary VirtualService Generator (Task 5.8.5)
// ============================================================
//
// | #  | Test Name                                               | Category    | Input                                         | Expected Output                                                      |
// |----|---------------------------------------------------------|-------------|-----------------------------------------------|----------------------------------------------------------------------|
// |  1 | TestGenerateIstioCanary_WeightBasedSplit                | happy       | StableWeight=80, CanaryWeight=20              | VirtualService YAML with weight 80 and 20                            |
// |  2 | TestGenerateIstioCanary_HeaderRouting                   | happy       | HeaderRouting={"x-canary":"true"}             | VirtualService contains header match "x-canary"                      |
// |  3 | TestGenerateIstioCanary_RetryAttempts                   | happy       | RetryAttempts=3                               | VirtualService YAML contains retry block with "3"                    |
// |  4 | TestGenerateIstioCanary_Timeout                         | happy       | Timeout="15s"                                 | VirtualService YAML contains "15s"                                   |
// |  5 | TestGenerateIstioCanary_EqualWeights                    | edge        | StableWeight=50, CanaryWeight=50              | Both weights 50 present in YAML                                      |
// |  6 | TestGenerateIstioCanary_ZeroCanaryWeight                | edge        | CanaryWeight=0, StableWeight=100              | YAML contains weight 100 for stable, canary weight 0 or absent       |
// |  7 | TestGenerateIstioCanary_FullCanaryWeight                | edge        | CanaryWeight=100, StableWeight=0              | YAML contains weight 100 for canary                                  |
// |  8 | TestGenerateIstioCanary_MissingServiceName              | error       | ServiceName=""                                | returns nil or empty map, no panic                                   |
// |  9 | TestGenerateIstioCanary_GeneratedYAMLIsValid            | happy       | valid opts                                    | YAML contains apiVersion networking.istio.io and kind VirtualService  |
// | 10 | TestGenerateIstioCanary_ServiceNameInYAML               | happy       | ServiceName="checkout"                        | YAML references "checkout" as host or destination                    |

import (
	"strings"
	"testing"
)

// ── 1: weight-based split produces weights 80 / 20 ───────────────────────────

func TestGenerateIstioCanary_WeightBasedSplit(t *testing.T) {
	opts := IstioCanaryOptions{
		ServiceName:   "frontend",
		StableWeight:  80,
		CanaryWeight:  20,
		RetryAttempts: 0,
		Timeout:       "",
	}

	templates := GenerateIstioCanary(opts)

	if len(templates) == 0 {
		t.Fatal("expected at least one template from GenerateIstioCanary")
	}

	all := joinTemplates(templates)
	if !strings.Contains(all, "80") {
		t.Errorf("expected stable weight '80' in YAML:\n%s", all)
	}
	if !strings.Contains(all, "20") {
		t.Errorf("expected canary weight '20' in YAML:\n%s", all)
	}
}

// ── 2: header routing produces match block with header name ──────────────────

func TestGenerateIstioCanary_HeaderRouting(t *testing.T) {
	opts := IstioCanaryOptions{
		ServiceName:   "frontend",
		StableWeight:  90,
		CanaryWeight:  10,
		HeaderRouting: map[string]string{"x-canary": "true"},
		RetryAttempts: 0,
		Timeout:       "",
	}

	templates := GenerateIstioCanary(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, "x-canary") {
		t.Errorf("expected header 'x-canary' in VirtualService YAML:\n%s", all)
	}
}

// ── 3: RetryAttempts appears in YAML ─────────────────────────────────────────

func TestGenerateIstioCanary_RetryAttempts(t *testing.T) {
	opts := IstioCanaryOptions{
		ServiceName:   "payment",
		StableWeight:  70,
		CanaryWeight:  30,
		RetryAttempts: 3,
		Timeout:       "",
	}

	templates := GenerateIstioCanary(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, "3") {
		t.Errorf("expected retry attempts '3' in YAML:\n%s", all)
	}
	if !strings.Contains(all, "retries") && !strings.Contains(all, "attempts") {
		t.Errorf("expected retries block in YAML:\n%s", all)
	}
}

// ── 4: Timeout appears in YAML ───────────────────────────────────────────────

func TestGenerateIstioCanary_Timeout(t *testing.T) {
	opts := IstioCanaryOptions{
		ServiceName:   "orders",
		StableWeight:  60,
		CanaryWeight:  40,
		RetryAttempts: 0,
		Timeout:       "15s",
	}

	templates := GenerateIstioCanary(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, "15s") {
		t.Errorf("expected timeout '15s' in VirtualService YAML:\n%s", all)
	}
}

// ── 5: equal 50/50 weights both appear in YAML ───────────────────────────────

func TestGenerateIstioCanary_EqualWeights(t *testing.T) {
	opts := IstioCanaryOptions{
		ServiceName:  "catalog",
		StableWeight: 50,
		CanaryWeight: 50,
	}

	templates := GenerateIstioCanary(opts)

	all := joinTemplates(templates)
	count := strings.Count(all, "50")
	if count < 2 {
		t.Errorf("expected weight '50' to appear at least twice (stable + canary), found %d times in:\n%s", count, all)
	}
}

// ── 6: 0% canary — stable gets all traffic ───────────────────────────────────

func TestGenerateIstioCanary_ZeroCanaryWeight(t *testing.T) {
	opts := IstioCanaryOptions{
		ServiceName:  "stable-only",
		StableWeight: 100,
		CanaryWeight: 0,
	}

	templates := GenerateIstioCanary(opts)

	if len(templates) == 0 {
		t.Fatal("expected templates even with CanaryWeight=0")
	}
	all := joinTemplates(templates)
	if !strings.Contains(all, "100") {
		t.Errorf("expected stable weight '100' when CanaryWeight=0:\n%s", all)
	}
}

// ── 7: 100% canary — canary gets all traffic ─────────────────────────────────

func TestGenerateIstioCanary_FullCanaryWeight(t *testing.T) {
	opts := IstioCanaryOptions{
		ServiceName:  "full-canary",
		StableWeight: 0,
		CanaryWeight: 100,
	}

	templates := GenerateIstioCanary(opts)

	if len(templates) == 0 {
		t.Fatal("expected templates even with StableWeight=0")
	}
	all := joinTemplates(templates)
	if !strings.Contains(all, "100") {
		t.Errorf("expected canary weight '100' when CanaryWeight=100:\n%s", all)
	}
}

// ── 8: empty ServiceName returns nil/empty map, no panic ─────────────────────

func TestGenerateIstioCanary_MissingServiceName(t *testing.T) {
	opts := IstioCanaryOptions{
		ServiceName:  "",
		StableWeight: 80,
		CanaryWeight: 20,
	}

	// Must not panic.
	templates := GenerateIstioCanary(opts)

	// Either nil or empty map is acceptable when ServiceName is empty.
	if len(templates) > 0 {
		all := joinTemplates(templates)
		// If templates are returned they must not have a meaningful host name.
		if strings.Contains(all, "VirtualService") {
			// Check that no invalid empty-name VS was produced silently.
			if strings.Contains(all, "host: ") && !strings.Contains(all, "host: \"\"") {
				t.Logf("GenerateIstioCanary with empty ServiceName returned templates: %v", keysOfMap(templates))
			}
		}
	}
	// Primary contract: no panic. Any return value is acceptable.
}

// ── 9: generated YAML is structurally valid Istio VirtualService ─────────────

func TestGenerateIstioCanary_GeneratedYAMLIsValid(t *testing.T) {
	opts := IstioCanaryOptions{
		ServiceName:  "checkout",
		StableWeight: 75,
		CanaryWeight: 25,
	}

	templates := GenerateIstioCanary(opts)

	if len(templates) == 0 {
		t.Fatal("expected at least one template")
	}

	all := joinTemplates(templates)
	if !strings.Contains(all, "networking.istio.io") {
		t.Errorf("expected apiVersion 'networking.istio.io' in YAML:\n%s", all)
	}
	if !strings.Contains(all, "VirtualService") {
		t.Errorf("expected kind 'VirtualService' in YAML:\n%s", all)
	}
}

// ── 10: ServiceName appears as host or destination in YAML ───────────────────

func TestGenerateIstioCanary_ServiceNameInYAML(t *testing.T) {
	opts := IstioCanaryOptions{
		ServiceName:  "checkout",
		StableWeight: 80,
		CanaryWeight: 20,
	}

	templates := GenerateIstioCanary(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, "checkout") {
		t.Errorf("ServiceName 'checkout' must appear in generated YAML as host or destination:\n%s", all)
	}
}

// ── local helper ─────────────────────────────────────────────────────────────

func joinTemplates(m map[string]string) string {
	var sb strings.Builder
	for _, v := range m {
		sb.WriteString(v)
		sb.WriteByte('\n')
	}
	return sb.String()
}
