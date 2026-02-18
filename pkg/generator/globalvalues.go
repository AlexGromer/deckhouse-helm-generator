package generator

import (
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ExtractGlobalValues analyzes service groups and extracts values that are common
// across >=2 groups. These become global values in a parent values.yaml.
// Returns a map of global values (e.g., "imageRegistry", "env", "labels").
func ExtractGlobalValues(groups []*ServiceGroup) map[string]interface{} {
	if len(groups) < 2 {
		return make(map[string]interface{})
	}

	globalValues := make(map[string]interface{})

	// Extract common image registry.
	if registry := extractCommonImageRegistry(groups); registry != "" {
		globalValues["imageRegistry"] = registry
	}

	// Extract common environment variables.
	if env := extractCommonEnvVars(groups); len(env) > 0 {
		globalValues["env"] = env
	}

	// Extract common labels.
	if labels := extractCommonLabels(groups); len(labels) > 0 {
		globalValues["labels"] = labels
	}

	return globalValues
}

// extractCommonImageRegistry finds a common image registry across all groups.
func extractCommonImageRegistry(groups []*ServiceGroup) string {
	registries := make(map[string]int)

	for _, group := range groups {
		groupRegistry := ""
		for _, r := range group.Resources {
			reg := extractImageRegistryFromResource(r)
			if reg != "" {
				groupRegistry = reg
				break
			}
		}
		if groupRegistry != "" {
			registries[groupRegistry]++
		}
	}

	// Find registry present in all groups.
	for reg, count := range registries {
		if count == len(groups) {
			return reg
		}
	}
	return ""
}

// extractImageRegistryFromResource extracts image registry from resource values.
func extractImageRegistryFromResource(r *types.ProcessedResource) string {
	if imageVal, ok := r.Values["image"]; ok {
		if imageMap, ok := imageVal.(map[string]interface{}); ok {
			if reg, ok := imageMap["registry"]; ok {
				if regStr, ok := reg.(string); ok {
					return regStr
				}
			}
		}
	}
	return ""
}

// extractCommonEnvVars finds environment variables common to >=2 groups.
func extractCommonEnvVars(groups []*ServiceGroup) map[string]interface{} {
	// Count occurrences of each env var value across groups.
	type envEntry struct {
		value string
		count int
	}
	envCounts := make(map[string]*envEntry)

	for _, group := range groups {
		groupEnv := make(map[string]string)
		for _, r := range group.Resources {
			if envVal, ok := r.Values["env"]; ok {
				if envMap, ok := envVal.(map[string]interface{}); ok {
					for k, v := range envMap {
						if vStr, ok := v.(string); ok {
							groupEnv[k] = vStr
						}
					}
				}
			}
		}
		for k, v := range groupEnv {
			key := k + "=" + v
			if _, ok := envCounts[key]; !ok {
				envCounts[key] = &envEntry{value: v, count: 0}
			}
			envCounts[key].count++
		}
	}

	// Include env vars present in >=2 groups with same value.
	result := make(map[string]interface{})
	seen := make(map[string]bool)
	for key, entry := range envCounts {
		if entry.count >= 2 {
			// Extract env var name from key format "NAME=value"
			envName := key[:len(key)-len("=")-len(entry.value)+1]
			// Recalculate: key is "NAME=value", extract NAME
			for i, c := range key {
				if c == '=' {
					envName = key[:i]
					break
				}
			}
			if !seen[envName] {
				result[envName] = entry.value
				seen[envName] = true
			}
		}
	}

	return result
}

// extractCommonLabels finds labels common to >=2 groups.
func extractCommonLabels(groups []*ServiceGroup) map[string]interface{} {
	type labelEntry struct {
		value string
		count int
	}
	labelCounts := make(map[string]*labelEntry)

	for _, group := range groups {
		groupLabels := make(map[string]string)
		for _, r := range group.Resources {
			if labelsVal, ok := r.Values["commonLabels"]; ok {
				if labelsMap, ok := labelsVal.(map[string]interface{}); ok {
					for k, v := range labelsMap {
						if vStr, ok := v.(string); ok {
							groupLabels[k] = vStr
						}
					}
				}
			}
		}
		for k, v := range groupLabels {
			key := k + "=" + v
			if _, ok := labelCounts[key]; !ok {
				labelCounts[key] = &labelEntry{value: v, count: 0}
			}
			labelCounts[key].count++
		}
	}

	result := make(map[string]interface{})
	seen := make(map[string]bool)
	for key, entry := range labelCounts {
		if entry.count >= 2 {
			var labelName string
			for i, c := range key {
				if c == '=' {
					labelName = key[:i]
					break
				}
			}
			if !seen[labelName] {
				result[labelName] = entry.value
				seen[labelName] = true
			}
		}
	}

	return result
}
