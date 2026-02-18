package generator

import (
	"sigs.k8s.io/yaml"
)

// GenerateEnvValues creates environment-specific values override files from base values.
// It returns a map of filename → YAML bytes for values-dev.yaml, values-staging.yaml,
// and values-prod.yaml. Each file contains only the overrides relevant to that environment
// (override-only principle — no full copy of base values).
func GenerateEnvValues(baseValues map[string]interface{}) map[string][]byte {
	result := make(map[string][]byte, 3)

	devYAML, _ := yaml.Marshal(devProfile())
	stagingYAML, _ := yaml.Marshal(stagingProfile())
	prodYAML, _ := yaml.Marshal(prodProfile())

	result["values-dev.yaml"] = append([]byte("# Dev environment overrides — relaxed settings for local development\n"), devYAML...)
	result["values-staging.yaml"] = append([]byte("# Staging environment overrides — mirrors prod at reduced scale\n"), stagingYAML...)
	result["values-prod.yaml"] = append([]byte("# Production environment overrides — hardened settings\n"), prodYAML...)

	return result
}

// devProfile returns override values for the dev environment.
// Dev: single replica, debug logging, no PDB, no resource limits.
func devProfile() map[string]interface{} {
	return map[string]interface{}{
		"replicaCount": 1,
		"logLevel":     "debug",
		"podDisruptionBudget": map[string]interface{}{
			"enabled": false,
		},
	}
}

// stagingProfile returns override values for the staging environment.
// Staging: 2 replicas, info logging, PDB with minAvailable=1.
func stagingProfile() map[string]interface{} {
	return map[string]interface{}{
		"replicaCount": 2,
		"logLevel":     "info",
		"podDisruptionBudget": map[string]interface{}{
			"enabled":      true,
			"minAvailable": 1,
		},
	}
}

// prodProfile returns override values for the production environment.
// Prod: >=3 replicas, warn logging, PDB minAvailable=2, resource limits, affinity.
func prodProfile() map[string]interface{} {
	return map[string]interface{}{
		"replicaCount": 3,
		"logLevel":     "warn",
		"podDisruptionBudget": map[string]interface{}{
			"enabled":      true,
			"minAvailable": 2,
		},
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"cpu":    "100m",
				"memory": "128Mi",
			},
			"limits": map[string]interface{}{
				"cpu":    "500m",
				"memory": "512Mi",
			},
		},
		"affinity": map[string]interface{}{
			"podAntiAffinity": map[string]interface{}{
				"preferredDuringSchedulingIgnoredDuringExecution": []interface{}{
					map[string]interface{}{
						"weight": 100,
						"podAffinityTerm": map[string]interface{}{
							"labelSelector": map[string]interface{}{
								"matchExpressions": []interface{}{
									map[string]interface{}{
										"key":      "app.kubernetes.io/name",
										"operator": "Exists",
									},
								},
							},
							"topologyKey": "kubernetes.io/hostname",
						},
					},
				},
			},
		},
	}
}
