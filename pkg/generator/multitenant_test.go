package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"

	"sigs.k8s.io/yaml"
)

// ============================================================
// Test Helpers
// ============================================================

func makeBaseChart(name string) *types.GeneratedChart {
	return &types.GeneratedChart{
		Name:       name,
		ChartYAML:  "apiVersion: v2\nname: " + name + "\nversion: 0.1.0\n",
		ValuesYAML: "replicaCount: 1\n",
		Templates: map[string]string{
			"templates/deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: " + name + "\n",
			"templates/service.yaml":    "apiVersion: v1\nkind: Service\nmetadata:\n  name: " + name + "\n",
		},
	}
}

// ============================================================
// Subtask 1: Default two tenants
// ============================================================

func TestMultiTenant_DefaultTwoTenants(t *testing.T) {
	chart := makeBaseChart("myapp")
	result := GenerateMultiTenantOverlay(chart, 2)

	var vals map[string]interface{}
	if err := yaml.Unmarshal([]byte(result.ValuesYAML), &vals); err != nil {
		t.Fatalf("failed to parse ValuesYAML: %v", err)
	}

	tenants, ok := vals["tenants"].([]interface{})
	if !ok {
		t.Fatal("expected 'tenants' array in values")
	}
	if len(tenants) != 2 {
		t.Errorf("expected 2 tenants, got %d", len(tenants))
	}
}

// ============================================================
// Subtask 2: Single tenant
// ============================================================

func TestMultiTenant_SingleTenant(t *testing.T) {
	chart := makeBaseChart("myapp")
	result := GenerateMultiTenantOverlay(chart, 1)

	var vals map[string]interface{}
	if err := yaml.Unmarshal([]byte(result.ValuesYAML), &vals); err != nil {
		t.Fatalf("failed to parse ValuesYAML: %v", err)
	}

	tenants, ok := vals["tenants"].([]interface{})
	if !ok {
		t.Fatal("expected 'tenants' array in values")
	}
	if len(tenants) != 1 {
		t.Errorf("expected 1 tenant, got %d", len(tenants))
	}
}

// ============================================================
// Subtask 3: Zero tenants — returns chart unchanged
// ============================================================

func TestMultiTenant_ZeroTenants(t *testing.T) {
	chart := makeBaseChart("myapp")
	originalValues := chart.ValuesYAML
	result := GenerateMultiTenantOverlay(chart, 0)

	if result.ValuesYAML != originalValues {
		t.Error("expected chart unchanged when tenantCount=0")
	}

	// Should not have tenant templates
	for path := range result.Templates {
		if strings.Contains(path, "tenant") {
			t.Errorf("unexpected tenant template '%s' when tenantCount=0", path)
		}
	}
}

// ============================================================
// Subtask 4: Tenant names
// ============================================================

func TestMultiTenant_TenantNames(t *testing.T) {
	chart := makeBaseChart("myapp")
	result := GenerateMultiTenantOverlay(chart, 2)

	var vals map[string]interface{}
	if err := yaml.Unmarshal([]byte(result.ValuesYAML), &vals); err != nil {
		t.Fatalf("failed to parse ValuesYAML: %v", err)
	}

	tenants, ok := vals["tenants"].([]interface{})
	if !ok {
		t.Fatal("expected 'tenants' array in values")
	}

	for i, tenant := range tenants {
		tenantMap, ok := tenant.(map[string]interface{})
		if !ok {
			t.Fatalf("tenant %d is not a map", i)
		}
		name, ok := tenantMap["name"].(string)
		if !ok {
			t.Fatalf("tenant %d missing 'name' field", i)
		}
		expectedName := "tenant-" + strings.TrimPrefix(name, "tenant-")
		if name != expectedName {
			t.Errorf("tenant %d: expected name pattern 'tenant-N', got '%s'", i, name)
		}
	}
}

// ============================================================
// Subtask 5: Range loop in templates
// ============================================================

func TestMultiTenant_RangeLoop(t *testing.T) {
	chart := makeBaseChart("myapp")
	result := GenerateMultiTenantOverlay(chart, 2)

	hasRange := false
	for _, content := range result.Templates {
		if strings.Contains(content, "range .Values.tenants") {
			hasRange = true
			break
		}
	}
	if !hasRange {
		t.Error("expected at least one template with '{{ range .Values.tenants }}'")
	}
}

// ============================================================
// Subtask 6: Namespace template per tenant
// ============================================================

func TestMultiTenant_NamespaceTemplate(t *testing.T) {
	chart := makeBaseChart("myapp")
	result := GenerateMultiTenantOverlay(chart, 2)

	hasNamespace := false
	for _, content := range result.Templates {
		if strings.Contains(content, "kind: Namespace") && strings.Contains(content, "range .Values.tenants") {
			hasNamespace = true
			break
		}
	}
	if !hasNamespace {
		t.Error("expected tenant Namespace template with range loop")
	}
}

// ============================================================
// Subtask 7: ResourceQuota per tenant
// ============================================================

func TestMultiTenant_ResourceQuotaPerTenant(t *testing.T) {
	chart := makeBaseChart("myapp")
	result := GenerateMultiTenantOverlay(chart, 2)

	hasRQ := false
	for _, content := range result.Templates {
		if strings.Contains(content, "kind: ResourceQuota") && strings.Contains(content, "range .Values.tenants") {
			hasRQ = true
			break
		}
	}
	if !hasRQ {
		t.Error("expected tenant ResourceQuota template with range loop")
	}
}

// ============================================================
// Subtask 8: NetworkPolicy isolation
// ============================================================

func TestMultiTenant_NetworkPolicyIsolation(t *testing.T) {
	chart := makeBaseChart("myapp")
	result := GenerateMultiTenantOverlay(chart, 2)

	hasNP := false
	for _, content := range result.Templates {
		if strings.Contains(content, "kind: NetworkPolicy") && strings.Contains(content, "range .Values.tenants") {
			hasNP = true
			break
		}
	}
	if !hasNP {
		t.Error("expected tenant NetworkPolicy template for cross-tenant isolation")
	}
}

// ============================================================
// Subtask 9: Values structure
// ============================================================

func TestMultiTenant_ValuesStructure(t *testing.T) {
	chart := makeBaseChart("myapp")
	result := GenerateMultiTenantOverlay(chart, 2)

	var vals map[string]interface{}
	if err := yaml.Unmarshal([]byte(result.ValuesYAML), &vals); err != nil {
		t.Fatalf("failed to parse ValuesYAML: %v", err)
	}

	tenants, ok := vals["tenants"].([]interface{})
	if !ok {
		t.Fatal("expected 'tenants' array in values")
	}

	for i, tenant := range tenants {
		tenantMap, ok := tenant.(map[string]interface{})
		if !ok {
			t.Fatalf("tenant %d is not a map", i)
		}

		// Must have name
		if _, ok := tenantMap["name"]; !ok {
			t.Errorf("tenant %d missing 'name'", i)
		}

		// Must have namespace
		if _, ok := tenantMap["namespace"]; !ok {
			t.Errorf("tenant %d missing 'namespace'", i)
		}

		// Must have resources
		resources, ok := tenantMap["resources"].(map[string]interface{})
		if !ok {
			t.Errorf("tenant %d missing 'resources' map", i)
			continue
		}

		if _, ok := resources["cpu"]; !ok {
			t.Errorf("tenant %d resources missing 'cpu'", i)
		}
		if _, ok := resources["memory"]; !ok {
			t.Errorf("tenant %d resources missing 'memory'", i)
		}
	}
}

// ============================================================
// Subtask 10: Preserves existing templates
// ============================================================

func TestMultiTenant_PreservesExistingTemplates(t *testing.T) {
	chart := makeBaseChart("myapp")
	originalTemplateCount := len(chart.Templates)

	result := GenerateMultiTenantOverlay(chart, 2)

	// Must have at least the original templates plus tenant templates
	if len(result.Templates) < originalTemplateCount {
		t.Errorf("expected at least %d templates (original), got %d", originalTemplateCount, len(result.Templates))
	}

	// Original templates must still be present
	if _, ok := result.Templates["templates/deployment.yaml"]; !ok {
		t.Error("original deployment.yaml template was removed")
	}
	if _, ok := result.Templates["templates/service.yaml"]; !ok {
		t.Error("original service.yaml template was removed")
	}
}
