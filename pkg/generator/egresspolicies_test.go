package generator

// ============================================================
// Test Plan: Egress Policies Generator (Task 5.8.4)
// ============================================================
//
// | #  | Test Name                                              | Category    | Input                                         | Expected Output                                                      |
// |----|--------------------------------------------------------|-------------|-----------------------------------------------|----------------------------------------------------------------------|
// |  1 | TestGenerateEgressPolicies_DatabaseURLDetected         | happy       | Deployment with DATABASE_URL env var          | ServiceEntries contains entry for DATABASE_URL host                  |
// |  2 | TestGenerateEgressPolicies_APIURLDetected              | happy       | Deployment with API_URL env var               | ServiceEntries contains entry for API_URL host                       |
// |  3 | TestGenerateEgressPolicies_AllowedHostsInServiceEntry  | happy       | AllowedHosts=["api.example.com"]              | ServiceEntry YAML contains "api.example.com"                         |
// |  4 | TestGenerateEgressPolicies_NetworkPolicyEgressGenerated| happy       | graph with Deployment                         | NetworkPolicies map non-empty with egress rules                      |
// |  5 | TestGenerateEgressPolicies_EmptyGraph                  | edge        | empty graph, AllowedHosts empty               | ServiceEntries empty, NetworkPolicies empty, no panic                |
// |  6 | TestInjectEgressPolicies_NilChart                      | error       | nil chart                                     | returns (nil, 0), no panic                                           |
// |  7 | TestInjectEgressPolicies_CopyOnWrite                   | happy       | chart with templates + result                 | original chart.Templates unchanged after inject                      |
// |  8 | TestGenerateEgressPolicies_MultipleURLs                | happy       | Deployment with DATABASE_URL and REDIS_URL    | DetectedURLs contains both hosts                                     |
// |  9 | TestGenerateEgressPolicies_NOTESTxtPresent             | happy       | any non-empty result                          | NOTESTxt non-empty, mentions egress                                  |
// | 10 | TestGenerateEgressPolicies_DetectFromEnvFalse           | happy       | DetectFromEnv=false, AllowedHosts provided    | ServiceEntries built from AllowedHosts only, no auto-detection       |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── 1: DATABASE_URL env var triggers ServiceEntry detection ──────────────────

func TestGenerateEgressPolicies_DatabaseURLDetected(t *testing.T) {
	deploy := makeDeploymentWithEnv("db-client", "default", map[string]string{
		"DATABASE_URL": "postgres://db.internal:5432/mydb",
	})
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	opts := EgressOptions{
		DetectFromEnv: true,
		AllowedHosts:  nil,
	}

	result := GenerateEgressPolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil EgressResult")
	}

	found := false
	for _, entry := range result.ServiceEntries {
		if strings.Contains(entry, "db.internal") || strings.Contains(entry, "postgres") {
			found = true
			break
		}
	}
	// Also check DetectedURLs
	if !found {
		for _, url := range result.DetectedURLs {
			if strings.Contains(url, "db.internal") || strings.Contains(url, "postgres") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("expected ServiceEntry or DetectedURL for DATABASE_URL host 'db.internal', got entries: %v, urls: %v",
			keysOfMap(result.ServiceEntries), result.DetectedURLs)
	}
}

// ── 2: API_URL env var triggers ServiceEntry detection ───────────────────────

func TestGenerateEgressPolicies_APIURLDetected(t *testing.T) {
	deploy := makeDeploymentWithEnv("api-client", "default", map[string]string{
		"API_URL": "https://api.example.com/v1",
	})
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	opts := EgressOptions{
		DetectFromEnv: true,
		AllowedHosts:  nil,
	}

	result := GenerateEgressPolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil EgressResult")
	}

	found := false
	for _, entry := range result.ServiceEntries {
		if strings.Contains(entry, "api.example.com") {
			found = true
			break
		}
	}
	if !found {
		for _, url := range result.DetectedURLs {
			if strings.Contains(url, "api.example.com") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("expected ServiceEntry for API_URL host 'api.example.com', got entries: %v, urls: %v",
			keysOfMap(result.ServiceEntries), result.DetectedURLs)
	}
}

// ── 3: AllowedHosts appear in ServiceEntry YAML ───────────────────────────────

func TestGenerateEgressPolicies_AllowedHostsInServiceEntry(t *testing.T) {
	graph := types.NewResourceGraph()

	opts := EgressOptions{
		DetectFromEnv: false,
		AllowedHosts:  []string{"api.example.com", "metrics.corp.internal"},
	}

	result := GenerateEgressPolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil EgressResult")
	}
	if len(result.ServiceEntries) == 0 {
		t.Fatal("expected at least one ServiceEntry for AllowedHosts")
	}

	for _, host := range opts.AllowedHosts {
		found := false
		for _, yaml := range result.ServiceEntries {
			if strings.Contains(yaml, host) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AllowedHost %q not found in any ServiceEntry YAML", host)
		}
	}
}

// ── 4: NetworkPolicy with egress rules is generated ──────────────────────────

func TestGenerateEgressPolicies_NetworkPolicyEgressGenerated(t *testing.T) {
	deploy := makeDeploymentWithEnv("worker", "default", map[string]string{
		"UPSTREAM_URL": "http://upstream.svc:8080",
	})
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	opts := EgressOptions{
		DetectFromEnv: true,
		AllowedHosts:  nil,
	}

	result := GenerateEgressPolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil EgressResult")
	}
	if len(result.NetworkPolicies) == 0 {
		t.Fatal("expected at least one NetworkPolicy with egress rules")
	}
	for _, yaml := range result.NetworkPolicies {
		if strings.Contains(yaml, "NetworkPolicy") {
			if !strings.Contains(yaml, "egress") {
				t.Errorf("NetworkPolicy YAML must contain egress rules:\n%s", yaml)
			}
			return
		}
	}
	t.Error("no NetworkPolicy kind found in NetworkPolicies map")
}

// ── 5: empty graph returns empty result without panic ─────────────────────────

func TestGenerateEgressPolicies_EmptyGraph(t *testing.T) {
	graph := types.NewResourceGraph()

	opts := EgressOptions{
		DetectFromEnv: true,
		AllowedHosts:  nil,
	}

	result := GenerateEgressPolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil EgressResult even for empty graph")
	}
	if len(result.ServiceEntries) != 0 {
		t.Errorf("expected 0 ServiceEntries for empty graph, got %d", len(result.ServiceEntries))
	}
	if len(result.NetworkPolicies) != 0 {
		t.Errorf("expected 0 NetworkPolicies for empty graph, got %d", len(result.NetworkPolicies))
	}
}

// ── 6: nil chart returns (nil, 0) ─────────────────────────────────────────────

func TestInjectEgressPolicies_NilChart(t *testing.T) {
	result := &EgressResult{
		ServiceEntries: map[string]string{
			"templates/istio-se-api.yaml": "apiVersion: networking.istio.io/v1beta1\nkind: ServiceEntry\n",
		},
		NetworkPolicies: map[string]string{},
	}

	newChart, count := InjectEgressPolicies(nil, result)

	if newChart != nil {
		t.Errorf("expected nil chart for nil input, got %+v", newChart)
	}
	if count != 0 {
		t.Errorf("expected count=0 for nil chart, got %d", count)
	}
}

// ── 7: InjectEgressPolicies is copy-on-write ──────────────────────────────────

func TestInjectEgressPolicies_CopyOnWrite(t *testing.T) {
	originalContent := testDeploymentTemplate
	chart := newTestChart(map[string]string{
		"templates/deployment.yaml": originalContent,
	})
	result := &EgressResult{
		ServiceEntries: map[string]string{
			"templates/istio-se-api.yaml": "apiVersion: networking.istio.io/v1beta1\nkind: ServiceEntry\nmetadata:\n  name: api-example\n",
		},
		NetworkPolicies: map[string]string{},
	}

	newChart, _ := InjectEgressPolicies(chart, result)

	if newChart == chart {
		t.Error("InjectEgressPolicies must return a new chart (copy-on-write)")
	}
	if chart.Templates["templates/deployment.yaml"] != originalContent {
		t.Error("original chart template must not be modified by InjectEgressPolicies")
	}
	if len(chart.Templates) != 1 {
		t.Errorf("original chart.Templates must have exactly 1 key, got %d", len(chart.Templates))
	}
}

// ── 8: multiple URL env vars all appear in DetectedURLs ──────────────────────

func TestGenerateEgressPolicies_MultipleURLs(t *testing.T) {
	deploy := makeDeploymentWithEnv("multi-client", "default", map[string]string{
		"DATABASE_URL": "postgres://db.internal:5432/mydb",
		"REDIS_URL":    "redis://cache.internal:6379",
	})
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	opts := EgressOptions{
		DetectFromEnv: true,
		AllowedHosts:  nil,
	}

	result := GenerateEgressPolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil EgressResult")
	}

	allContent := strings.Join(result.DetectedURLs, " ")
	for _, url := range result.ServiceEntries {
		allContent += " " + url
	}

	if !strings.Contains(allContent, "db.internal") && !strings.Contains(allContent, "postgres") {
		t.Errorf("expected DATABASE_URL host in result, got detected: %v", result.DetectedURLs)
	}
	if !strings.Contains(allContent, "cache.internal") && !strings.Contains(allContent, "redis") {
		t.Errorf("expected REDIS_URL host in result, got detected: %v", result.DetectedURLs)
	}
}

// ── 9: NOTESTxt is present and mentions egress ───────────────────────────────

func TestGenerateEgressPolicies_NOTESTxtPresent(t *testing.T) {
	deploy := makeDeploymentWithEnv("notes-client", "default", map[string]string{
		"API_URL": "https://api.example.com/v2",
	})
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	opts := EgressOptions{
		DetectFromEnv: true,
		AllowedHosts:  nil,
	}

	result := GenerateEgressPolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil EgressResult")
	}
	if result.NOTESTxt == "" {
		t.Error("NOTESTxt must not be empty for non-empty result")
	}
	lower := strings.ToLower(result.NOTESTxt)
	if !strings.Contains(lower, "egress") && !strings.Contains(lower, "serviceentry") {
		t.Errorf("NOTESTxt should mention 'egress' or 'ServiceEntry', got:\n%s", result.NOTESTxt)
	}
}

// ── 10: DetectFromEnv=false uses only AllowedHosts ───────────────────────────

func TestGenerateEgressPolicies_DetectFromEnvFalse(t *testing.T) {
	// Deployment has URL env var but DetectFromEnv=false; only AllowedHosts must appear.
	deploy := makeDeploymentWithEnv("env-ignored", "default", map[string]string{
		"SECRET_URL": "https://should-not-appear.internal/api",
	})
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	opts := EgressOptions{
		DetectFromEnv: false,
		AllowedHosts:  []string{"allowed.corp.com"},
	}

	result := GenerateEgressPolicies(graph, opts)

	if result == nil {
		t.Fatal("expected non-nil EgressResult")
	}

	// should-not-appear must not be in any ServiceEntry
	for _, yaml := range result.ServiceEntries {
		if strings.Contains(yaml, "should-not-appear.internal") {
			t.Errorf("host from env var must not appear when DetectFromEnv=false:\n%s", yaml)
		}
	}

	// allowed.corp.com must be present
	found := false
	for _, yaml := range result.ServiceEntries {
		if strings.Contains(yaml, "allowed.corp.com") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("AllowedHost 'allowed.corp.com' not found in ServiceEntries: %v", keysOfMap(result.ServiceEntries))
	}
}
