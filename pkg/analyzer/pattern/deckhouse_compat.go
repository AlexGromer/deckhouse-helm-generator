package pattern

import (
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// DeckhouseCompatChecker validates CRD apiVersions against Deckhouse 1.57+ and
// warns on deprecated fields.
type DeckhouseCompatChecker struct{}

// NewDeckhouseCompatChecker creates a new Deckhouse compatibility checker.
func NewDeckhouseCompatChecker() *DeckhouseCompatChecker {
	return &DeckhouseCompatChecker{}
}

func (c *DeckhouseCompatChecker) Name() string {
	return "deckhouse-compat"
}

func (c *DeckhouseCompatChecker) Category() string {
	return "Deckhouse Compatibility"
}

// deprecatedDeckhouseFields maps Kind to deprecated field paths (valid before 1.57).
var deprecatedDeckhouseFields = map[string][]string{
	"IngressNginxController": {"spec.inlet"},
	"ClusterAuthorizationRule": {"spec.accessLevel"},
	"ModuleConfig": {"spec.version"},
}

// validDeckhouseAPIVersions maps Kind to the expected apiVersion for Deckhouse 1.57+.
var validDeckhouseAPIVersions = map[string]string{
	"ModuleConfig":           "deckhouse.io/v1alpha1",
	"IngressNginxController": "deckhouse.io/v1",
	"ClusterAuthorizationRule": "deckhouse.io/v1",
	"NodeGroup":              "deckhouse.io/v1",
	"DexAuthenticator":       "deckhouse.io/v1",
	"User":                   "deckhouse.io/v1",
	"Group":                  "deckhouse.io/v1",
}

// deckhouseKinds is the set of Deckhouse CRD kinds we check.
var deckhouseKinds = map[string]bool{
	"ModuleConfig":             true,
	"IngressNginxController":   true,
	"ClusterAuthorizationRule": true,
	"NodeGroup":                true,
	"DexAuthenticator":         true,
	"User":                     true,
	"Group":                    true,
}

func (c *DeckhouseCompatChecker) Check(graph *types.ResourceGraph) []BestPractice {
	practices := make([]BestPractice, 0)

	invalidVersion := make([]types.ResourceKey, 0)
	deprecatedField := make([]types.ResourceKey, 0)

	for key, resource := range graph.Resources {
		kind := key.GVK.Kind
		if !deckhouseKinds[kind] {
			continue
		}

		// Check apiVersion compatibility
		if expectedAPI, ok := validDeckhouseAPIVersions[kind]; ok {
			actualGroup := key.GVK.Group
			actualVersion := key.GVK.Version
			actualAPI := actualGroup + "/" + actualVersion
			if actualGroup == "" {
				actualAPI = actualVersion
			}
			if actualAPI != expectedAPI {
				invalidVersion = append(invalidVersion, key)
			}
		}

		// Check deprecated fields
		if deprecatedPaths, ok := deprecatedDeckhouseFields[kind]; ok {
			for _, path := range deprecatedPaths {
				if hasFieldByDotPath(resource.Values, path) {
					deprecatedField = append(deprecatedField, key)
					break
				}
			}
		}
	}

	if len(invalidVersion) > 0 {
		practices = append(practices, BestPractice{
			ID:          "BP-DH-001",
			Title:       "Incompatible Deckhouse CRD apiVersion",
			Description: "CRD resources use apiVersion that may not be compatible with Deckhouse 1.57+",
			Category:    c.Category(),
			Severity:    SeverityWarning,
			Compliant:   false,
			Recommendations: []string{
				"Update apiVersion to the version supported by Deckhouse 1.57+",
				"Check Deckhouse documentation for CRD migration guides",
			},
			AffectedResources: invalidVersion,
			AutoFixable:       false,
		})
	}

	if len(deprecatedField) > 0 {
		practices = append(practices, BestPractice{
			ID:          "BP-DH-002",
			Title:       "Deprecated Deckhouse CRD Fields",
			Description: "CRD resources use fields deprecated in Deckhouse 1.57+",
			Category:    c.Category(),
			Severity:    SeverityWarning,
			Compliant:   false,
			Recommendations: []string{
				"Remove or replace deprecated fields per Deckhouse migration guide",
				"Test manifests against target Deckhouse version before deployment",
			},
			AffectedResources: deprecatedField,
			AutoFixable:       false,
		})
	}

	return practices
}

// hasFieldByDotPath checks if a dot-separated path exists in values.
// e.g. "spec.inlet" checks values["spec"]["inlet"].
func hasFieldByDotPath(values map[string]interface{}, path string) bool {
	current := values
	parts := splitDotPath(path)
	for i, part := range parts {
		val, ok := current[part]
		if !ok {
			return false
		}
		if i == len(parts)-1 {
			return true
		}
		next, ok := val.(map[string]interface{})
		if !ok {
			return false
		}
		current = next
	}
	return false
}

// splitDotPath splits "spec.inlet" into ["spec", "inlet"].
func splitDotPath(path string) []string {
	parts := make([]string, 0, 4)
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '.' {
			if i > start {
				parts = append(parts, path[start:i])
			}
			start = i + 1
		}
	}
	if start < len(path) {
		parts = append(parts, path[start:])
	}
	return parts
}
