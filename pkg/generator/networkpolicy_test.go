package generator

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Test Helpers
// ============================================================

// makeDeploymentWithEnv creates a Deployment ProcessedResource with env vars.
func makeDeploymentWithEnv(name, namespace string, envVars map[string]string) *types.ProcessedResource {
	r := makeProcessedResource("Deployment", name, namespace, nil)
	envList := make([]interface{}, 0, len(envVars))
	for k, v := range envVars {
		envList = append(envList, map[string]interface{}{
			"name":  k,
			"value": v,
		})
	}
	r.Original.Object.Object["spec"] = map[string]interface{}{
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "app",
						"image": name + ":latest",
						"env":   envList,
					},
				},
			},
		},
	}
	return r
}

// makeServiceWithPorts creates a Service ProcessedResource with specified ports.
func makeServiceWithPorts(name, namespace string, ports []int64) *types.ProcessedResource {
	r := makeProcessedResource("Service", name, namespace, nil)
	portList := make([]interface{}, 0, len(ports))
	for _, p := range ports {
		portList = append(portList, map[string]interface{}{
			"port":     p,
			"protocol": "TCP",
		})
	}
	r.Original.Object.Object["spec"] = map[string]interface{}{
		"ports": portList,
	}
	return r
}

// ============================================================
// Subtask 1: Auto-NetworkPolicy — no services
// ============================================================

func TestAutoNP_NoServices(t *testing.T) {
	deploy := makeDeploymentWithEnv("myapp", "default", nil)
	group := makeGroup("myapp", "default", []*types.ProcessedResource{deploy})
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result := GenerateAutoNetworkPolicies(graph, []*ServiceGroup{group})

	if len(result) == 0 {
		t.Fatal("expected at least 1 NetworkPolicy even without Services (deny-all + allow-dns)")
	}

	for _, content := range result {
		if !strings.Contains(content, "kind: NetworkPolicy") {
			t.Error("generated content must contain 'kind: NetworkPolicy'")
		}
	}
}

// ============================================================
// Subtask 2: Auto-NetworkPolicy — single port ingress
// ============================================================

func TestAutoNP_SinglePort(t *testing.T) {
	deploy := makeDeploymentWithEnv("myapp", "default", nil)
	svc := makeServiceWithPorts("myapp-svc", "default", []int64{8080})
	group := makeGroup("myapp", "default", []*types.ProcessedResource{deploy, svc})
	graph := buildGraph([]*types.ProcessedResource{deploy, svc}, nil)

	result := GenerateAutoNetworkPolicies(graph, []*ServiceGroup{group})

	found := false
	for _, content := range result {
		if strings.Contains(content, "8080") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ingress rule allowing port 8080")
	}
}

func TestAutoNP_MultiPort(t *testing.T) {
	deploy := makeDeploymentWithEnv("myapp", "default", nil)
	svc := makeServiceWithPorts("myapp-svc", "default", []int64{8080, 8443})
	group := makeGroup("myapp", "default", []*types.ProcessedResource{deploy, svc})
	graph := buildGraph([]*types.ProcessedResource{deploy, svc}, nil)

	result := GenerateAutoNetworkPolicies(graph, []*ServiceGroup{group})

	has8080, has8443 := false, false
	for _, content := range result {
		if strings.Contains(content, "8080") {
			has8080 = true
		}
		if strings.Contains(content, "8443") {
			has8443 = true
		}
	}
	if !has8080 || !has8443 {
		t.Errorf("expected both ports 8080 and 8443 in ingress rules (got 8080=%v, 8443=%v)", has8080, has8443)
	}
}

// ============================================================
// Subtask 3: Auto-NetworkPolicy — env-based egress
// ============================================================

func TestAutoNP_EnvBasedEgress_Postgres(t *testing.T) {
	deploy := makeDeploymentWithEnv("myapp", "default", map[string]string{
		"DATABASE_URL": "postgres://db:5432/mydb",
	})
	group := makeGroup("myapp", "default", []*types.ProcessedResource{deploy})
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result := GenerateAutoNetworkPolicies(graph, []*ServiceGroup{group})

	found := false
	for _, content := range result {
		if strings.Contains(content, "5432") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected egress rule for PostgreSQL port 5432 based on DATABASE_URL env var")
	}
}

func TestAutoNP_EnvBasedEgress_Redis(t *testing.T) {
	deploy := makeDeploymentWithEnv("myapp", "default", map[string]string{
		"REDIS_HOST": "redis.default.svc",
	})
	group := makeGroup("myapp", "default", []*types.ProcessedResource{deploy})
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result := GenerateAutoNetworkPolicies(graph, []*ServiceGroup{group})

	found := false
	for _, content := range result {
		if strings.Contains(content, "6379") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected egress rule for Redis port 6379 based on REDIS_HOST env var")
	}
}

func TestAutoNP_EnvBasedEgress_Multiple(t *testing.T) {
	deploy := makeDeploymentWithEnv("myapp", "default", map[string]string{
		"POSTGRES_HOST": "pg.default.svc",
		"REDIS_HOST":    "redis.default.svc",
	})
	group := makeGroup("myapp", "default", []*types.ProcessedResource{deploy})
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result := GenerateAutoNetworkPolicies(graph, []*ServiceGroup{group})

	hasPG, hasRedis := false, false
	for _, content := range result {
		if strings.Contains(content, "5432") {
			hasPG = true
		}
		if strings.Contains(content, "6379") {
			hasRedis = true
		}
	}
	if !hasPG || !hasRedis {
		t.Errorf("expected egress for both 5432 and 6379 (got pg=%v, redis=%v)", hasPG, hasRedis)
	}
}

// ============================================================
// Subtask 4: Auto-NetworkPolicy — structure validation
// ============================================================

func TestAutoNP_DenyAllPresent(t *testing.T) {
	deploy := makeDeploymentWithEnv("myapp", "default", nil)
	group := makeGroup("myapp", "default", []*types.ProcessedResource{deploy})
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result := GenerateAutoNetworkPolicies(graph, []*ServiceGroup{group})

	for _, content := range result {
		if !strings.Contains(content, "policyTypes") {
			t.Error("every NetworkPolicy must specify policyTypes for deny-all base")
		}
	}
}

func TestAutoNP_AllowDNSPresent(t *testing.T) {
	deploy := makeDeploymentWithEnv("myapp", "default", nil)
	group := makeGroup("myapp", "default", []*types.ProcessedResource{deploy})
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result := GenerateAutoNetworkPolicies(graph, []*ServiceGroup{group})

	hasDNS := false
	for _, content := range result {
		if strings.Contains(content, "53") && strings.Contains(content, "UDP") {
			hasDNS = true
			break
		}
	}
	if !hasDNS {
		t.Error("every NetworkPolicy must include DNS egress (UDP port 53)")
	}
}

// ============================================================
// Subtask 5: Auto-NetworkPolicy — multiple groups
// ============================================================

func TestAutoNP_OnePerGroup(t *testing.T) {
	deploy1 := makeDeploymentWithEnv("frontend", "default", nil)
	deploy2 := makeDeploymentWithEnv("backend", "default", nil)
	deploy3 := makeDeploymentWithEnv("worker", "default", nil)

	groups := []*ServiceGroup{
		makeGroup("frontend", "default", []*types.ProcessedResource{deploy1}),
		makeGroup("backend", "default", []*types.ProcessedResource{deploy2}),
		makeGroup("worker", "default", []*types.ProcessedResource{deploy3}),
	}

	graph := buildGraph([]*types.ProcessedResource{deploy1, deploy2, deploy3}, nil)

	result := GenerateAutoNetworkPolicies(graph, groups)

	if len(result) < 3 {
		t.Errorf("expected at least 3 NetworkPolicy templates (one per group), got %d", len(result))
	}
}

func TestAutoNP_EmptyGraph(t *testing.T) {
	graph := buildGraph(nil, nil)
	result := GenerateAutoNetworkPolicies(graph, nil)

	if len(result) != 0 {
		t.Errorf("expected 0 templates for empty graph, got %d", len(result))
	}
}

// ============================================================
// HC-5: Port validation in extractServicePorts
// ============================================================

// makeServiceWithRawPort creates a Service with a single port stored as an arbitrary interface{} value.
func makeServiceWithRawPort(name, namespace string, rawPort interface{}) *types.ProcessedResource {
	r := makeProcessedResource("Service", name, namespace, nil)
	r.Original.Object.Object["spec"] = map[string]interface{}{
		"ports": []interface{}{
			map[string]interface{}{
				"port":     rawPort,
				"protocol": "TCP",
			},
		},
	}
	return r
}

func TestExtractServicePorts_RejectsZeroPort(t *testing.T) {
	svc := makeServiceWithRawPort("svc", "default", int64(0))
	group := makeGroup("svc", "default", []*types.ProcessedResource{svc})
	ports := extractServicePorts(group)
	if len(ports) != 0 {
		t.Errorf("port 0 should be rejected, got %v", ports)
	}
}

func TestExtractServicePorts_RejectsNegativePort(t *testing.T) {
	svc := makeServiceWithRawPort("svc", "default", int64(-1))
	group := makeGroup("svc", "default", []*types.ProcessedResource{svc})
	ports := extractServicePorts(group)
	if len(ports) != 0 {
		t.Errorf("negative port should be rejected, got %v", ports)
	}
}

func TestExtractServicePorts_RejectsAbove65535(t *testing.T) {
	svc := makeServiceWithRawPort("svc", "default", int64(65536))
	group := makeGroup("svc", "default", []*types.ProcessedResource{svc})
	ports := extractServicePorts(group)
	if len(ports) != 0 {
		t.Errorf("port 65536 should be rejected, got %v", ports)
	}
}

func TestExtractServicePorts_RejectsFractionalFloat(t *testing.T) {
	svc := makeServiceWithRawPort("svc", "default", float64(80.5))
	group := makeGroup("svc", "default", []*types.ProcessedResource{svc})
	ports := extractServicePorts(group)
	if len(ports) != 0 {
		t.Errorf("fractional float port 80.5 should be rejected, got %v", ports)
	}
}

func TestExtractServicePorts_AcceptsIntegerFloat(t *testing.T) {
	svc := makeServiceWithRawPort("svc", "default", float64(443.0))
	group := makeGroup("svc", "default", []*types.ProcessedResource{svc})
	ports := extractServicePorts(group)
	if len(ports) != 1 || ports[0].Port != 443 {
		t.Errorf("float64(443.0) should be accepted as port 443, got %v", ports)
	}
}

func TestExtractServicePorts_AcceptsBoundary65535(t *testing.T) {
	svc := makeServiceWithRawPort("svc", "default", int64(65535))
	group := makeGroup("svc", "default", []*types.ProcessedResource{svc})
	ports := extractServicePorts(group)
	if len(ports) != 1 || ports[0].Port != 65535 {
		t.Errorf("port 65535 should be accepted, got %v", ports)
	}
}

// ============================================================
// Subtask 6: Auto-NetworkPolicy — cross-namespace
// ============================================================

func TestAutoNP_CrossNamespace(t *testing.T) {
	deploy := makeDeploymentWithEnv("frontend", "web", nil)
	svc := makeServiceWithPorts("backend-svc", "api", []int64{8080})

	groups := []*ServiceGroup{
		makeGroup("frontend", "web", []*types.ProcessedResource{deploy}),
		makeGroup("backend", "api", []*types.ProcessedResource{svc}),
	}

	rel := types.Relationship{
		From: resourceKey(deploy),
		To:   resourceKey(svc),
		Type: types.RelationNameReference,
	}
	graph := buildGraph([]*types.ProcessedResource{deploy, svc}, []types.Relationship{rel})

	result := GenerateAutoNetworkPolicies(graph, groups)

	hasNsSelector := false
	for _, content := range result {
		if strings.Contains(content, "namespaceSelector") {
			hasNsSelector = true
			break
		}
	}
	if !hasNsSelector {
		t.Error("expected namespaceSelector in cross-namespace NetworkPolicy")
	}
}

// ============================================================
// M-5: Namespace uses Helm Release.Namespace template
// ============================================================

func TestAutoNP_UsesHelmReleaseNamespace(t *testing.T) {
	deploy := makeDeploymentWithEnv("myapp", "hardcoded-ns", nil)
	group := makeGroup("myapp", "hardcoded-ns", []*types.ProcessedResource{deploy})
	graph := buildGraph([]*types.ProcessedResource{deploy}, nil)

	result := GenerateAutoNetworkPolicies(graph, []*ServiceGroup{group})

	for _, content := range result {
		if !strings.Contains(content, "namespace: {{ .Release.Namespace }}") {
			t.Error("expected namespace to use Helm template {{ .Release.Namespace }}")
		}
		if strings.Contains(content, "namespace: hardcoded-ns") {
			t.Error("namespace must NOT contain literal source namespace 'hardcoded-ns'")
		}
	}
}

// ============================================================
// M-6: Deterministic output of buildCrossNamespaceIndex
// ============================================================

func TestBuildCrossNamespaceIndex_Deterministic(t *testing.T) {
	// Create 4 groups across 4 namespaces with cross-namespace relationships
	deployA := makeDeploymentWithEnv("svc-a", "ns-alpha", nil)
	deployB := makeDeploymentWithEnv("svc-b", "ns-beta", nil)
	deployC := makeDeploymentWithEnv("svc-c", "ns-gamma", nil)
	deployD := makeDeploymentWithEnv("svc-d", "ns-delta", nil)

	groups := []*ServiceGroup{
		makeGroup("svc-a", "ns-alpha", []*types.ProcessedResource{deployA}),
		makeGroup("svc-b", "ns-beta", []*types.ProcessedResource{deployB}),
		makeGroup("svc-c", "ns-gamma", []*types.ProcessedResource{deployC}),
		makeGroup("svc-d", "ns-delta", []*types.ProcessedResource{deployD}),
	}

	// Build relationships: A->B, A->C, A->D, B->C, B->D, C->D
	rels := []types.Relationship{
		{From: resourceKey(deployA), To: resourceKey(deployB), Type: types.RelationNameReference},
		{From: resourceKey(deployA), To: resourceKey(deployC), Type: types.RelationNameReference},
		{From: resourceKey(deployA), To: resourceKey(deployD), Type: types.RelationNameReference},
		{From: resourceKey(deployB), To: resourceKey(deployC), Type: types.RelationNameReference},
		{From: resourceKey(deployB), To: resourceKey(deployD), Type: types.RelationNameReference},
		{From: resourceKey(deployC), To: resourceKey(deployD), Type: types.RelationNameReference},
	}

	graph := buildGraph([]*types.ProcessedResource{deployA, deployB, deployC, deployD}, rels)

	// Call 5 times and verify all results are identical
	var firstResult map[string][]string
	for i := 0; i < 5; i++ {
		result := buildCrossNamespaceIndex(graph, groups)

		// Verify each slice is sorted
		for name, namespaces := range result {
			if !sort.StringsAreSorted(namespaces) {
				t.Errorf("iteration %d: namespaces for %q are not sorted: %v", i, name, namespaces)
			}
		}

		if firstResult == nil {
			firstResult = result
			continue
		}

		if !reflect.DeepEqual(firstResult, result) {
			t.Errorf("iteration %d: result differs from first iteration\nfirst: %v\ngot:   %v", i, firstResult, result)
		}
	}

	// Verify svc-a has 3 cross-namespace entries (ns-beta, ns-gamma, ns-delta) — all sorted
	if got := firstResult["svc-a"]; len(got) != 3 {
		t.Errorf("svc-a: expected 3 cross-namespace entries, got %d: %v", len(got), got)
	} else {
		want := []string{"ns-beta", "ns-delta", "ns-gamma"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("svc-a: expected %v, got %v", want, got)
		}
	}
}

// ============================================================
// M-13: extractServicePorts accepts string port values
// ============================================================

func TestExtractServicePorts_AcceptsStringPort(t *testing.T) {
	r := makeProcessedResource("Service", "web", "default", nil)
	r.Original.Object.Object["spec"] = map[string]interface{}{
		"ports": []interface{}{
			map[string]interface{}{
				"port":     "8080",
				"protocol": "TCP",
			},
		},
	}

	group := makeGroup("web", "default", []*types.ProcessedResource{r})
	ports := extractServicePorts(group)

	if len(ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(ports))
	}
	if ports[0].Port != 8080 {
		t.Errorf("expected port 8080, got %d", ports[0].Port)
	}
	if ports[0].Protocol != "TCP" {
		t.Errorf("expected protocol TCP, got %s", ports[0].Protocol)
	}
}
