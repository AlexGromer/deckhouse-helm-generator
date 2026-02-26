package generator

import (
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
