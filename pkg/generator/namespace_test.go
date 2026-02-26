package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Test Helpers
// ============================================================

func makeResourceWithValues(kind, name, namespace string, labels map[string]string, values map[string]interface{}) *types.ProcessedResource {
	r := makeProcessedResource(kind, name, namespace, labels)
	r.Values = values
	return r
}

func makeGroup(name, namespace string, resources []*types.ProcessedResource) *ServiceGroup {
	return &ServiceGroup{
		Name:      name,
		Resources: resources,
		Namespace: namespace,
		Strategy:  GroupByLabel,
	}
}

// ============================================================
// Subtask 1: ResourceQuota template generation
// ============================================================

func TestNamespace_ResourceQuota_SingleService(t *testing.T) {
	deploy := makeResourceWithValues("Deployment", "myapp", "default", nil, map[string]interface{}{
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{"cpu": "100m", "memory": "128Mi"},
			"limits":   map[string]interface{}{"cpu": "500m", "memory": "512Mi"},
		},
	})
	group := makeGroup("myapp", "default", []*types.ProcessedResource{deploy})

	tmpl := GenerateResourceQuotaTemplate(group)

	if tmpl == "" {
		t.Fatal("expected non-empty ResourceQuota template")
	}
	if !strings.Contains(tmpl, "kind: ResourceQuota") {
		t.Error("template must contain 'kind: ResourceQuota'")
	}
}

func TestNamespace_ResourceQuota_MultiService(t *testing.T) {
	deploy1 := makeResourceWithValues("Deployment", "frontend", "default", nil, map[string]interface{}{
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{"cpu": "100m", "memory": "128Mi"},
			"limits":   map[string]interface{}{"cpu": "500m", "memory": "512Mi"},
		},
	})
	deploy2 := makeResourceWithValues("Deployment", "backend", "default", nil, map[string]interface{}{
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{"cpu": "200m", "memory": "256Mi"},
			"limits":   map[string]interface{}{"cpu": "1", "memory": "1Gi"},
		},
	})
	deploy3 := makeResourceWithValues("Deployment", "worker", "default", nil, map[string]interface{}{
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{"cpu": "50m", "memory": "64Mi"},
			"limits":   map[string]interface{}{"cpu": "250m", "memory": "256Mi"},
		},
	})
	group := makeGroup("app", "default", []*types.ProcessedResource{deploy1, deploy2, deploy3})

	tmpl := GenerateResourceQuotaTemplate(group)

	if tmpl == "" {
		t.Fatal("expected non-empty ResourceQuota template")
	}
	if !strings.Contains(tmpl, "kind: ResourceQuota") {
		t.Error("template must contain 'kind: ResourceQuota'")
	}
}

func TestNamespace_ResourceQuota_NoResources(t *testing.T) {
	deploy := makeResourceWithValues("Deployment", "myapp", "default", nil, map[string]interface{}{})
	group := makeGroup("myapp", "default", []*types.ProcessedResource{deploy})

	tmpl := GenerateResourceQuotaTemplate(group)

	// Should still generate a template with placeholder values
	if tmpl == "" {
		t.Fatal("expected non-empty ResourceQuota template even without resource values")
	}
	if !strings.Contains(tmpl, "kind: ResourceQuota") {
		t.Error("template must contain 'kind: ResourceQuota'")
	}
}

func TestNamespace_ResourceQuota_HasEnabledGuard(t *testing.T) {
	deploy := makeResourceWithValues("Deployment", "myapp", "default", nil, map[string]interface{}{})
	group := makeGroup("myapp", "default", []*types.ProcessedResource{deploy})

	tmpl := GenerateResourceQuotaTemplate(group)

	if !strings.Contains(tmpl, ".Values.namespace.resourceQuota.enabled") {
		t.Error("ResourceQuota template must have {{- if .Values.namespace.resourceQuota.enabled }} guard")
	}
}

// ============================================================
// Subtask 2: LimitRange template generation
// ============================================================

func TestNamespace_LimitRange_DefaultValues(t *testing.T) {
	deploy1 := makeResourceWithValues("Deployment", "frontend", "default", nil, map[string]interface{}{
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{"cpu": "100m", "memory": "128Mi"},
			"limits":   map[string]interface{}{"cpu": "500m", "memory": "512Mi"},
		},
	})
	deploy2 := makeResourceWithValues("Deployment", "backend", "default", nil, map[string]interface{}{
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{"cpu": "200m", "memory": "256Mi"},
			"limits":   map[string]interface{}{"cpu": "1", "memory": "1Gi"},
		},
	})
	group := makeGroup("app", "default", []*types.ProcessedResource{deploy1, deploy2})

	tmpl := GenerateLimitRangeTemplate(group)

	if tmpl == "" {
		t.Fatal("expected non-empty LimitRange template")
	}
	if !strings.Contains(tmpl, "kind: LimitRange") {
		t.Error("template must contain 'kind: LimitRange'")
	}
}

func TestNamespace_LimitRange_SingleWorkload(t *testing.T) {
	deploy := makeResourceWithValues("Deployment", "myapp", "default", nil, map[string]interface{}{
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{"cpu": "100m", "memory": "128Mi"},
			"limits":   map[string]interface{}{"cpu": "500m", "memory": "512Mi"},
		},
	})
	group := makeGroup("myapp", "default", []*types.ProcessedResource{deploy})

	tmpl := GenerateLimitRangeTemplate(group)

	if tmpl == "" {
		t.Fatal("expected non-empty LimitRange template")
	}
	if !strings.Contains(tmpl, "kind: LimitRange") {
		t.Error("template must contain 'kind: LimitRange'")
	}
	if !strings.Contains(tmpl, "type: Container") {
		t.Error("LimitRange must specify type: Container")
	}
}

func TestNamespace_LimitRange_HasEnabledGuard(t *testing.T) {
	deploy := makeResourceWithValues("Deployment", "myapp", "default", nil, map[string]interface{}{})
	group := makeGroup("myapp", "default", []*types.ProcessedResource{deploy})

	tmpl := GenerateLimitRangeTemplate(group)

	if !strings.Contains(tmpl, ".Values.namespace.limitRange.enabled") {
		t.Error("LimitRange template must have {{- if .Values.namespace.limitRange.enabled }} guard")
	}
}

// ============================================================
// Subtask 3: NetworkPolicy template generation
// ============================================================

func TestNamespace_NetworkPolicy_DenyAllBase(t *testing.T) {
	deploy := makeResourceWithValues("Deployment", "myapp", "default", nil, map[string]interface{}{})
	group := makeGroup("myapp", "default", []*types.ProcessedResource{deploy})

	tmpl := GenerateNetworkPolicyTemplate(group)

	if tmpl == "" {
		t.Fatal("expected non-empty NetworkPolicy template")
	}
	if !strings.Contains(tmpl, "kind: NetworkPolicy") {
		t.Error("template must contain 'kind: NetworkPolicy'")
	}
	if !strings.Contains(tmpl, "policyTypes") {
		t.Error("NetworkPolicy must specify policyTypes")
	}
}

func TestNamespace_NetworkPolicy_AllowSameNamespace(t *testing.T) {
	deploy := makeResourceWithValues("Deployment", "myapp", "default", nil, map[string]interface{}{})
	group := makeGroup("myapp", "default", []*types.ProcessedResource{deploy})

	tmpl := GenerateNetworkPolicyTemplate(group)

	if !strings.Contains(tmpl, "namespaceSelector") {
		t.Error("NetworkPolicy must include namespaceSelector for same-namespace access")
	}
}

func TestNamespace_NetworkPolicy_AllowDNSEgress(t *testing.T) {
	deploy := makeResourceWithValues("Deployment", "myapp", "default", nil, map[string]interface{}{})
	group := makeGroup("myapp", "default", []*types.ProcessedResource{deploy})

	tmpl := GenerateNetworkPolicyTemplate(group)

	if !strings.Contains(tmpl, "53") {
		t.Error("NetworkPolicy must include DNS egress rule (port 53)")
	}
	if !strings.Contains(tmpl, "UDP") {
		t.Error("NetworkPolicy DNS egress must use UDP protocol")
	}
}

func TestNamespace_NetworkPolicy_HasEnabledGuard(t *testing.T) {
	deploy := makeResourceWithValues("Deployment", "myapp", "default", nil, map[string]interface{}{})
	group := makeGroup("myapp", "default", []*types.ProcessedResource{deploy})

	tmpl := GenerateNetworkPolicyTemplate(group)

	if !strings.Contains(tmpl, ".Values.namespace.networkPolicy.enabled") {
		t.Error("NetworkPolicy template must have {{- if .Values.namespace.networkPolicy.enabled }} guard")
	}
}

// ============================================================
// Subtask 4: GenerateNamespaceResources integration
// ============================================================

func TestNamespace_GenerateNamespaceResources_AllEnabled(t *testing.T) {
	deploy := makeResourceWithValues("Deployment", "myapp", "default", nil, map[string]interface{}{
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{"cpu": "100m", "memory": "128Mi"},
		},
	})
	groups := []*ServiceGroup{
		makeGroup("myapp", "default", []*types.ProcessedResource{deploy}),
	}

	opts := NamespaceOpts{
		ResourceQuota: true,
		LimitRange:    true,
		NetworkPolicy: true,
	}

	result := GenerateNamespaceResources(groups, opts)

	if len(result) < 3 {
		t.Errorf("expected at least 3 templates when all enabled, got %d", len(result))
	}
}

func TestNamespace_GenerateNamespaceResources_OnlyQuota(t *testing.T) {
	deploy := makeResourceWithValues("Deployment", "myapp", "default", nil, map[string]interface{}{})
	groups := []*ServiceGroup{
		makeGroup("myapp", "default", []*types.ProcessedResource{deploy}),
	}

	opts := NamespaceOpts{
		ResourceQuota: true,
		LimitRange:    false,
		NetworkPolicy: false,
	}

	result := GenerateNamespaceResources(groups, opts)

	if len(result) != 1 {
		t.Errorf("expected 1 template when only ResourceQuota enabled, got %d", len(result))
	}

	// The single template must be a ResourceQuota
	for _, content := range result {
		if !strings.Contains(content, "kind: ResourceQuota") {
			t.Error("expected ResourceQuota template content")
		}
	}
}

func TestNamespace_GenerateNamespaceResources_NoneEnabled(t *testing.T) {
	deploy := makeResourceWithValues("Deployment", "myapp", "default", nil, map[string]interface{}{})
	groups := []*ServiceGroup{
		makeGroup("myapp", "default", []*types.ProcessedResource{deploy}),
	}

	opts := NamespaceOpts{
		ResourceQuota: false,
		LimitRange:    false,
		NetworkPolicy: false,
	}

	result := GenerateNamespaceResources(groups, opts)

	if len(result) != 0 {
		t.Errorf("expected 0 templates when nothing enabled, got %d", len(result))
	}
}

func TestNamespace_GenerateNamespaceResources_EmptyGroups(t *testing.T) {
	opts := NamespaceOpts{
		ResourceQuota: true,
		LimitRange:    true,
		NetworkPolicy: true,
	}

	result := GenerateNamespaceResources(nil, opts)

	if len(result) != 0 {
		t.Errorf("expected 0 templates for nil groups, got %d", len(result))
	}
}

func TestNamespace_GenerateNamespaceResources_MultipleGroups(t *testing.T) {
	deploy1 := makeResourceWithValues("Deployment", "frontend", "default", nil, map[string]interface{}{})
	deploy2 := makeResourceWithValues("Deployment", "backend", "default", nil, map[string]interface{}{})
	groups := []*ServiceGroup{
		makeGroup("frontend", "default", []*types.ProcessedResource{deploy1}),
		makeGroup("backend", "default", []*types.ProcessedResource{deploy2}),
	}

	opts := NamespaceOpts{
		ResourceQuota: true,
		LimitRange:    false,
		NetworkPolicy: false,
	}

	result := GenerateNamespaceResources(groups, opts)

	// Should generate one ResourceQuota per group
	if len(result) < 2 {
		t.Errorf("expected at least 2 templates (one per group), got %d", len(result))
	}
}
