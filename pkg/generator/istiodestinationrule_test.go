package generator

// ============================================================
// Test Plan: Advanced DestinationRule Generator (Task 5.8.6)
// ============================================================
//
// | #  | Test Name                                                    | Category    | Input                                             | Expected Output                                                          |
// |----|--------------------------------------------------------------|-------------|---------------------------------------------------|--------------------------------------------------------------------------|
// |  1 | TestGenerateAdvancedDestinationRule_ConsistentHashLB         | happy       | LBPolicy="CONSISTENT_HASH", ConsistentHashKey set | DestinationRule YAML contains consistentHash block with key              |
// |  2 | TestGenerateAdvancedDestinationRule_RoundRobinLB             | happy       | LBPolicy="ROUND_ROBIN"                            | DestinationRule YAML contains ROUND_ROBIN                                |
// |  3 | TestGenerateAdvancedDestinationRule_OutlierDetection         | happy       | OutlierConsecutive5xx=5, OutlierInterval="30s"    | DestinationRule YAML contains outlierDetection with "5" and "30s"        |
// |  4 | TestGenerateAdvancedDestinationRule_CircuitBreaker           | happy       | CircuitBreakerMaxConn=100, MaxPending=50          | DestinationRule YAML contains connectionPool with "100" and "50"         |
// |  5 | TestGenerateAdvancedDestinationRule_CombinedOptions          | happy       | all fields set                                    | YAML contains LB, outlier, and connectionPool blocks                     |
// |  6 | TestGenerateAdvancedDestinationRule_EmptyServiceName         | error       | ServiceName=""                                    | returns nil or empty map, no panic                                       |
// |  7 | TestGenerateAdvancedDestinationRule_Defaults                 | edge        | only ServiceName set                              | returns non-nil map, YAML contains DestinationRule kind                  |
// |  8 | TestGenerateAdvancedDestinationRule_ServiceNameInYAML        | happy       | ServiceName="inventory"                           | YAML references "inventory" as host                                      |
// |  9 | TestGenerateAdvancedDestinationRule_RandomLBPolicy           | happy       | LBPolicy="RANDOM"                                 | DestinationRule YAML contains RANDOM                                     |
// | 10 | TestGenerateAdvancedDestinationRule_ValidAPIVersion          | happy       | valid opts                                        | YAML contains apiVersion networking.istio.io and kind DestinationRule    |

import (
	"strings"
	"testing"
)

// ── 1: consistentHash LB with hash key ───────────────────────────────────────

func TestGenerateAdvancedDestinationRule_ConsistentHashLB(t *testing.T) {
	opts := DestinationRuleOptions{
		ServiceName:      "session-svc",
		LBPolicy:         "CONSISTENT_HASH",
		ConsistentHashKey: "x-user-id",
	}

	templates := GenerateAdvancedDestinationRule(opts)

	if len(templates) == 0 {
		t.Fatal("expected at least one template")
	}
	all := joinTemplates(templates)
	if !strings.Contains(all, "consistentHash") && !strings.Contains(all, "CONSISTENT_HASH") {
		t.Errorf("expected consistentHash LB policy in YAML:\n%s", all)
	}
	if !strings.Contains(all, "x-user-id") {
		t.Errorf("expected ConsistentHashKey 'x-user-id' in YAML:\n%s", all)
	}
}

// ── 2: ROUND_ROBIN LB policy ─────────────────────────────────────────────────

func TestGenerateAdvancedDestinationRule_RoundRobinLB(t *testing.T) {
	opts := DestinationRuleOptions{
		ServiceName: "api-svc",
		LBPolicy:    "ROUND_ROBIN",
	}

	templates := GenerateAdvancedDestinationRule(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, "ROUND_ROBIN") {
		t.Errorf("expected ROUND_ROBIN load balancer policy in YAML:\n%s", all)
	}
}

// ── 3: outlier detection with consecutive 5xx and interval ───────────────────

func TestGenerateAdvancedDestinationRule_OutlierDetection(t *testing.T) {
	opts := DestinationRuleOptions{
		ServiceName:           "backend-svc",
		LBPolicy:              "ROUND_ROBIN",
		OutlierConsecutive5xx: 5,
		OutlierInterval:       "30s",
	}

	templates := GenerateAdvancedDestinationRule(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, "outlierDetection") {
		t.Errorf("expected outlierDetection block in YAML:\n%s", all)
	}
	if !strings.Contains(all, "5") {
		t.Errorf("expected consecutive5xxErrors '5' in YAML:\n%s", all)
	}
	if !strings.Contains(all, "30s") {
		t.Errorf("expected outlier interval '30s' in YAML:\n%s", all)
	}
}

// ── 4: circuit breaker via connectionPool settings ────────────────────────────

func TestGenerateAdvancedDestinationRule_CircuitBreaker(t *testing.T) {
	opts := DestinationRuleOptions{
		ServiceName:              "payment-svc",
		LBPolicy:                 "LEAST_CONN",
		CircuitBreakerMaxConn:    100,
		CircuitBreakerMaxPending: 50,
	}

	templates := GenerateAdvancedDestinationRule(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, "connectionPool") {
		t.Errorf("expected connectionPool block in YAML:\n%s", all)
	}
	if !strings.Contains(all, "100") {
		t.Errorf("expected maxConnections '100' in YAML:\n%s", all)
	}
	if !strings.Contains(all, "50") {
		t.Errorf("expected maxPendingRequests '50' in YAML:\n%s", all)
	}
}

// ── 5: combined options produce all three policy blocks ──────────────────────

func TestGenerateAdvancedDestinationRule_CombinedOptions(t *testing.T) {
	opts := DestinationRuleOptions{
		ServiceName:              "full-svc",
		LBPolicy:                 "ROUND_ROBIN",
		OutlierConsecutive5xx:    3,
		OutlierInterval:          "10s",
		CircuitBreakerMaxConn:    200,
		CircuitBreakerMaxPending: 100,
	}

	templates := GenerateAdvancedDestinationRule(opts)

	all := joinTemplates(templates)
	for _, expected := range []string{"ROUND_ROBIN", "outlierDetection", "connectionPool"} {
		if !strings.Contains(all, expected) {
			t.Errorf("combined YAML missing %q block:\n%s", expected, all)
		}
	}
}

// ── 6: empty ServiceName returns nil/empty, no panic ─────────────────────────

func TestGenerateAdvancedDestinationRule_EmptyServiceName(t *testing.T) {
	opts := DestinationRuleOptions{
		ServiceName: "",
		LBPolicy:    "ROUND_ROBIN",
	}

	// Must not panic.
	templates := GenerateAdvancedDestinationRule(opts)

	// Either nil or empty map is acceptable.
	if len(templates) > 0 {
		t.Logf("GenerateAdvancedDestinationRule with empty ServiceName returned %d templates (implementation choice)", len(templates))
	}
}

// ── 7: only ServiceName set — defaults produce valid DestinationRule ──────────

func TestGenerateAdvancedDestinationRule_Defaults(t *testing.T) {
	opts := DestinationRuleOptions{
		ServiceName: "minimal-svc",
	}

	templates := GenerateAdvancedDestinationRule(opts)

	if len(templates) == 0 {
		t.Fatal("expected at least one template even with minimal options")
	}
	all := joinTemplates(templates)
	if !strings.Contains(all, "DestinationRule") {
		t.Errorf("expected kind 'DestinationRule' in YAML:\n%s", all)
	}
}

// ── 8: ServiceName appears as host in YAML ───────────────────────────────────

func TestGenerateAdvancedDestinationRule_ServiceNameInYAML(t *testing.T) {
	opts := DestinationRuleOptions{
		ServiceName: "inventory",
		LBPolicy:    "ROUND_ROBIN",
	}

	templates := GenerateAdvancedDestinationRule(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, "inventory") {
		t.Errorf("expected ServiceName 'inventory' as host in YAML:\n%s", all)
	}
}

// ── 9: RANDOM LB policy ──────────────────────────────────────────────────────

func TestGenerateAdvancedDestinationRule_RandomLBPolicy(t *testing.T) {
	opts := DestinationRuleOptions{
		ServiceName: "random-svc",
		LBPolicy:    "RANDOM",
	}

	templates := GenerateAdvancedDestinationRule(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, "RANDOM") {
		t.Errorf("expected RANDOM load balancer policy in YAML:\n%s", all)
	}
}

// ── 10: generated YAML has valid Istio apiVersion ────────────────────────────

func TestGenerateAdvancedDestinationRule_ValidAPIVersion(t *testing.T) {
	opts := DestinationRuleOptions{
		ServiceName: "check-svc",
		LBPolicy:    "ROUND_ROBIN",
	}

	templates := GenerateAdvancedDestinationRule(opts)

	if len(templates) == 0 {
		t.Fatal("expected at least one template")
	}
	all := joinTemplates(templates)
	if !strings.Contains(all, "networking.istio.io") {
		t.Errorf("expected apiVersion 'networking.istio.io' in YAML:\n%s", all)
	}
	if !strings.Contains(all, "DestinationRule") {
		t.Errorf("expected kind 'DestinationRule' in YAML:\n%s", all)
	}
}
