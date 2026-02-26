package generator

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
	"sigs.k8s.io/yaml"
)

// FeatureCategory represents a named group of Kubernetes resource kinds
// that can be toggled on or off via a Helm feature flag.
type FeatureCategory string

const (
	FeatureMonitoring  FeatureCategory = "monitoring"
	FeatureIngress     FeatureCategory = "ingress"
	FeatureAutoscaling FeatureCategory = "autoscaling"
	FeatureSecurity    FeatureCategory = "security"
	FeatureStorage     FeatureCategory = "storage"
	FeatureRBAC        FeatureCategory = "rbac"
)

// FeatureFlagConfig controls which feature categories are enabled and which
// Kubernetes resource kinds are assigned to each category.
type FeatureFlagConfig struct {
	// Categories maps each feature category to its enabled/disabled state.
	Categories map[FeatureCategory]bool

	// KindToCategory maps a Kubernetes resource kind (e.g. "ServiceMonitor") to
	// the feature category that gates it.
	KindToCategory map[string]FeatureCategory
}

// neverWrapKinds lists workload and infrastructure kinds that must NOT be wrapped
// in feature-flag guards by default.  An explicit entry in KindToCategory always
// takes precedence and overrides this protection.
var neverWrapKinds = map[string]bool{
	"Deployment":     true,
	"StatefulSet":    true,
	"DaemonSet":      true,
	"Job":            true,
	"CronJob":        true,
	"Service":        true,
	"ConfigMap":      true,
	"Secret":         true,
	"ServiceAccount": true,
}

// kindRegex extracts the value of the top-level `kind:` field from a YAML document.
var kindRegex = regexp.MustCompile(`(?m)^kind:\s*(\S+)`)

// DefaultFeatureFlagConfig returns a FeatureFlagConfig with all six standard
// categories enabled and the canonical kind-to-category mappings pre-populated.
func DefaultFeatureFlagConfig() *FeatureFlagConfig {
	return &FeatureFlagConfig{
		Categories: map[FeatureCategory]bool{
			FeatureMonitoring:  true,
			FeatureIngress:     true,
			FeatureAutoscaling: true,
			FeatureSecurity:    true,
			FeatureStorage:     true,
			FeatureRBAC:        true,
		},
		KindToCategory: map[string]FeatureCategory{
			// monitoring
			"ServiceMonitor":   FeatureMonitoring,
			"PodMonitor":       FeatureMonitoring,
			"PrometheusRule":   FeatureMonitoring,
			"GrafanaDashboard": FeatureMonitoring,
			// ingress
			"Ingress":   FeatureIngress,
			"HTTPRoute": FeatureIngress,
			"Gateway":   FeatureIngress,
			"GRPCRoute": FeatureIngress,
			// autoscaling
			"HorizontalPodAutoscaler": FeatureAutoscaling,
			"VerticalPodAutoscaler":   FeatureAutoscaling,
			"ScaledObject":            FeatureAutoscaling,
			"TriggerAuthentication":   FeatureAutoscaling,
			// security
			"NetworkPolicy":       FeatureSecurity,
			"PodDisruptionBudget": FeatureSecurity,
			// storage
			"PersistentVolumeClaim": FeatureStorage,
			// rbac
			"Role":               FeatureRBAC,
			"ClusterRole":        FeatureRBAC,
			"RoleBinding":        FeatureRBAC,
			"ClusterRoleBinding": FeatureRBAC,
		},
	}
}

// InjectFeatureFlags scans every template in chart, wraps feature-gated resource
// kinds with a Helm `{{- if .Values.features.<category> }}` guard, and merges a
// `features:` section into chart.ValuesYAML reflecting which categories are in use.
//
// The original chart is not mutated; a shallow copy with a new Templates map is
// returned.  If chart is nil, nil is returned.  If config is nil,
// DefaultFeatureFlagConfig is used.
func InjectFeatureFlags(chart *types.GeneratedChart, config *FeatureFlagConfig) *types.GeneratedChart {
	if chart == nil {
		return nil
	}
	if config == nil {
		config = DefaultFeatureFlagConfig()
	}

	// Build a copy of the chart so we do not mutate the original.
	result := *chart
	result.Templates = make(map[string]string, len(chart.Templates))

	usedCategories := make(map[FeatureCategory]bool)

	for path, content := range chart.Templates {
		kind := extractKind(content)

		// Determine whether this kind should be wrapped.
		// Rule: skip if in neverWrapKinds AND there is no explicit override in
		// KindToCategory (an explicit mapping always wins).
		if _, hasOverride := config.KindToCategory[kind]; neverWrapKinds[kind] && !hasOverride {
			result.Templates[path] = content
			continue
		}

		category, mapped := config.KindToCategory[kind]
		if !mapped {
			result.Templates[path] = content
			continue
		}

		result.Templates[path] = wrapTemplateWithFeatureFlag(content, category)
		usedCategories[category] = true
	}

	// Only inject a features: section when at least one category was actually used.
	if len(usedCategories) > 0 {
		result.ValuesYAML = mergeFeatureValues(result.ValuesYAML, config, usedCategories)
	}

	return &result
}

// wrapTemplateWithFeatureFlag surrounds templateContent with a Helm feature-flag
// conditional guard for the given category.
func wrapTemplateWithFeatureFlag(templateContent string, category FeatureCategory) string {
	return fmt.Sprintf("{{- if .Values.features.%s }}\n%s\n{{- end }}", category, templateContent)
}

// generateFeatureValues returns a flat map[string]interface{} where every key is
// the string form of a category in config.Categories and the value is its bool
// state (true by default).
func generateFeatureValues(config *FeatureFlagConfig) map[string]interface{} {
	result := make(map[string]interface{}, len(config.Categories))
	for cat, enabled := range config.Categories {
		result[string(cat)] = enabled
	}
	return result
}

// mergeFeatureValues parses existingYAML, adds or updates the `features:` key
// with values from config (restricted to usedCategories), and returns the
// re-marshalled YAML string.
func mergeFeatureValues(existingYAML string, config *FeatureFlagConfig, usedCategories map[FeatureCategory]bool) string {
	// Parse existing values into a generic map.
	base := make(map[string]interface{})
	if strings.TrimSpace(existingYAML) != "" {
		_ = yaml.Unmarshal([]byte(existingYAML), &base)
	}

	// Build the features sub-map using only categories present in config.Categories.
	featuresMap := make(map[string]interface{}, len(usedCategories))
	for cat := range usedCategories {
		enabled, ok := config.Categories[cat]
		if !ok {
			enabled = true
		}
		featuresMap[string(cat)] = enabled
	}

	base["features"] = featuresMap

	out, err := yaml.Marshal(base)
	if err != nil {
		// Fallback: return original YAML unchanged.
		return existingYAML
	}
	return string(out)
}

// extractKind returns the value of the top-level `kind:` field found in yamlContent,
// or an empty string if the field is absent.
func extractKind(yamlContent string) string {
	matches := kindRegex.FindStringSubmatch(yamlContent)
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}
