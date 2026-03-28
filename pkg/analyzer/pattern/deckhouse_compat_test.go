package pattern

import (
	"testing"
)

// ============================================================
// Test: Valid Deckhouse version (no warnings)
// ============================================================

func TestDeckhouseCompat_ValidVersion(t *testing.T) {
	g := makeGraph()

	// Add a ModuleConfig with valid apiVersion deckhouse.io/v1alpha1
	r := addResource(g, "deckhouse.io", "v1alpha1", "ModuleConfig", "prometheus", "default", "prometheus")
	r.Values["spec"] = map[string]interface{}{
		"enabled": true,
	}

	checker := NewDeckhouseCompatChecker()
	results := checker.Check(g)

	// Should not have any incompatible version findings
	for _, bp := range results {
		if bp.ID == "BP-DH-001" {
			t.Errorf("Expected no incompatible version warning for valid apiVersion, got: %s", bp.Title)
		}
	}
}

// ============================================================
// Test: Deprecated field detected
// ============================================================

func TestDeckhouseCompat_DeprecatedField(t *testing.T) {
	g := makeGraph()

	// Add an IngressNginxController with deprecated spec.inlet field
	r := addResource(g, "deckhouse.io", "v1", "IngressNginxController", "main", "d8-ingress-nginx", "main")
	r.Values["spec"] = map[string]interface{}{
		"inlet":          "LoadBalancer",
		"ingressClass":   "nginx",
	}

	checker := NewDeckhouseCompatChecker()
	results := checker.Check(g)

	found := false
	for _, bp := range results {
		if bp.ID == "BP-DH-002" {
			found = true
			if bp.Compliant {
				t.Error("Expected non-compliant for deprecated field")
			}
			if len(bp.AffectedResources) == 0 {
				t.Error("Expected affected resources")
			}
		}
	}
	if !found {
		t.Error("Expected BP-DH-002 finding for deprecated field")
	}
}

// ============================================================
// Test: Invalid apiVersion
// ============================================================

func TestDeckhouseCompat_InvalidVersion(t *testing.T) {
	g := makeGraph()

	// Add a ModuleConfig with wrong apiVersion (v1 instead of v1alpha1)
	r := addResource(g, "deckhouse.io", "v1", "ModuleConfig", "prometheus", "default", "prometheus")
	r.Values["spec"] = map[string]interface{}{
		"enabled": true,
	}

	checker := NewDeckhouseCompatChecker()
	results := checker.Check(g)

	found := false
	for _, bp := range results {
		if bp.ID == "BP-DH-001" {
			found = true
			if bp.Compliant {
				t.Error("Expected non-compliant for invalid apiVersion")
			}
		}
	}
	if !found {
		t.Error("Expected BP-DH-001 finding for incompatible apiVersion")
	}
}

// ============================================================
// Test: Checker name and category
// ============================================================

func TestDeckhouseCompat_NameCategory(t *testing.T) {
	checker := NewDeckhouseCompatChecker()
	if checker.Name() != "deckhouse-compat" {
		t.Errorf("Expected name 'deckhouse-compat', got '%s'", checker.Name())
	}
	if checker.Category() != "Deckhouse Compatibility" {
		t.Errorf("Expected category 'Deckhouse Compatibility', got '%s'", checker.Category())
	}
}
