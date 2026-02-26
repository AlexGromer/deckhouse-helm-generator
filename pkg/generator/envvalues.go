package generator

import (
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
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

// ============================================================
// Workload-Aware Environment Profiles
// ============================================================

// WorkloadType represents the detected workload category for a service group.
type WorkloadType string

const (
	WorkloadWeb      WorkloadType = "web"
	WorkloadWorker   WorkloadType = "worker"
	WorkloadDatabase WorkloadType = "database"
	WorkloadBatch    WorkloadType = "batch"
	WorkloadCache    WorkloadType = "cache"
)

// extractContainers returns the list of container maps from a ProcessedResource
// whose spec follows the Pod template pattern (Deployment, StatefulSet, DaemonSet, Job, CronJob).
func extractContainers(resource *types.ProcessedResource) []map[string]interface{} {
	obj := resource.Original.Object.Object
	spec, ok := obj["spec"].(map[string]interface{})
	if !ok {
		return nil
	}
	// For Deployment/StatefulSet/DaemonSet: spec.template.spec.containers
	template, ok := spec["template"].(map[string]interface{})
	if !ok {
		return nil
	}
	tplSpec, ok := template["spec"].(map[string]interface{})
	if !ok {
		return nil
	}
	containers, ok := tplSpec["containers"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(containers))
	for _, c := range containers {
		if cm, ok := c.(map[string]interface{}); ok {
			result = append(result, cm)
		}
	}
	return result
}

// DetectWorkloadType infers the workload category from the resources in a ServiceGroup.
// Detection priority: Batch > Database > Cache > Web > Worker > default(Web).
func DetectWorkloadType(group *ServiceGroup) WorkloadType {
	var (
		hasService     bool
		hasIngress     bool
		hasStatefulSet bool
		hasPVC         bool
		hasJob         bool
		hasCronJob     bool
		hasDeployment  bool
	)

	// Flags for image/env-based heuristics.
	hasCacheImage    := false
	hasDBImage       := false
	hasWebPort       := false
	hasAMQPKafkaEnv  := false

	for _, res := range group.Resources {
		kind := res.Original.GVK.Kind

		switch kind {
		case "Service":
			hasService = true
		case "Ingress", "HTTPRoute":
			hasIngress = true
		case "StatefulSet":
			hasStatefulSet = true
		case "PersistentVolumeClaim":
			hasPVC = true
		case "Job":
			hasJob = true
		case "CronJob":
			hasCronJob = true
		case "Deployment":
			hasDeployment = true
		}

		// Inspect containers for image names, ports, and env vars.
		for _, container := range extractContainers(res) {
			// Image-based detection.
			image, _ := container["image"].(string)
			imageLower := strings.ToLower(image)
			if strings.Contains(imageLower, "postgres") ||
				strings.Contains(imageLower, "mysql") ||
				strings.Contains(imageLower, "mongo") {
				hasDBImage = true
			}
			if strings.Contains(imageLower, "redis") ||
				strings.Contains(imageLower, "memcached") {
				hasCacheImage = true
			}

			// Port-based detection.
			if ports, ok := container["ports"].([]interface{}); ok {
				for _, p := range ports {
					if pm, ok := p.(map[string]interface{}); ok {
						var portNum int64
						switch v := pm["containerPort"].(type) {
						case int64:
							portNum = v
						case float64:
							portNum = int64(v)
						case int:
							portNum = int64(v)
						}
						if portNum == 80 || portNum == 443 || portNum == 8080 {
							hasWebPort = true
						}
					}
				}
			}

			// Env-var-based detection.
			if envList, ok := container["env"].([]interface{}); ok {
				for _, e := range envList {
					if em, ok := e.(map[string]interface{}); ok {
						name, _ := em["name"].(string)
						if strings.HasPrefix(name, "AMQP_") ||
							strings.HasPrefix(name, "KAFKA_") {
							hasAMQPKafkaEnv = true
						}
					}
				}
			}
		}
	}

	// Priority 1: Batch — any Job or CronJob in the group.
	if hasJob || hasCronJob {
		return WorkloadBatch
	}

	// Priority 2: Database — StatefulSet + PVC, or DB-specific image (with PVC).
	if (hasStatefulSet && hasPVC) || (hasDBImage && hasPVC) {
		return WorkloadDatabase
	}

	// Priority 3: Database by image alone (no PVC required for image-based DB detection
	// when there is no competing cache signal).
	if hasDBImage && !hasCacheImage {
		return WorkloadDatabase
	}

	// Priority 4: Cache — cache image (redis/memcached) without PVC.
	if hasCacheImage && !hasPVC {
		return WorkloadCache
	}

	// Priority 5: Web — has Ingress/HTTPRoute, or has Service + web port, or any web port.
	if hasIngress || hasWebPort {
		return WorkloadWeb
	}
	if hasService {
		return WorkloadWeb
	}

	// Priority 6: Worker — Deployment without Service, or AMQP/Kafka env vars.
	if hasDeployment || hasAMQPKafkaEnv {
		return WorkloadWorker
	}

	// Default.
	return WorkloadWeb
}

// GenerateEnvValuesForWorkload generates environment-specific values files with
// workload-aware profiles. Returns a map with keys "values-dev.yaml",
// "values-staging.yaml", and "values-prod.yaml".
func GenerateEnvValuesForWorkload(baseValues map[string]interface{}, workloadType WorkloadType) map[string][]byte {
	result := make(map[string][]byte, 3)

	devProfile := buildWorkloadProfile(workloadType, "dev")
	stagingProfile := buildWorkloadProfile(workloadType, "staging")
	prodProfile := buildWorkloadProfile(workloadType, "prod")

	devData, _ := yaml.Marshal(devProfile)
	stagingData, _ := yaml.Marshal(stagingProfile)
	prodData, _ := yaml.Marshal(prodProfile)

	result["values-dev.yaml"] = devData
	result["values-staging.yaml"] = stagingData
	result["values-prod.yaml"] = prodData

	return result
}

// buildWorkloadProfile returns the values map for a given workload type and environment.
func buildWorkloadProfile(workloadType WorkloadType, env string) map[string]interface{} {
	switch workloadType {
	case WorkloadWeb:
		return buildWebProfile(env)
	case WorkloadWorker:
		return buildWorkerProfile(env)
	case WorkloadDatabase:
		return buildDatabaseProfile(env)
	case WorkloadBatch:
		return buildBatchProfile(env)
	case WorkloadCache:
		return buildCacheProfile(env)
	default:
		return buildWebProfile(env)
	}
}

func buildWebProfile(env string) map[string]interface{} {
	switch env {
	case "dev":
		return map[string]interface{}{
			"replicaCount": 1,
			"logLevel":     "debug",
			"autoscaling": map[string]interface{}{
				"enabled": false,
			},
		}
	case "staging":
		return map[string]interface{}{
			"replicaCount": 2,
			"logLevel":     "info",
		}
	default: // prod
		return map[string]interface{}{
			"replicaCount": 3,
			"logLevel":     "warn",
			"autoscaling": map[string]interface{}{
				"enabled":     true,
				"minReplicas": 3,
				"maxReplicas": 10,
			},
			"podDisruptionBudget": map[string]interface{}{
				"enabled":      true,
				"minAvailable": 2,
			},
		}
	}
}

func buildWorkerProfile(env string) map[string]interface{} {
	switch env {
	case "dev":
		return map[string]interface{}{
			"replicaCount": 1,
			"logLevel":     "debug",
		}
	case "staging":
		return map[string]interface{}{
			"replicaCount": 2,
		}
	default: // prod
		return map[string]interface{}{
			"replicaCount": 3,
			"podDisruptionBudget": map[string]interface{}{
				"enabled":      true,
				"minAvailable": 1,
			},
		}
	}
}

func buildDatabaseProfile(env string) map[string]interface{} {
	switch env {
	case "dev":
		return map[string]interface{}{
			"replicaCount": 1,
			"logLevel":     "debug",
		}
	case "staging":
		return map[string]interface{}{
			"replicaCount": 2,
		}
	default: // prod
		return map[string]interface{}{
			"replicaCount": 3,
			"affinity": map[string]interface{}{
				"podAntiAffinity": map[string]interface{}{
					"preferredDuringSchedulingIgnoredDuringExecution": []interface{}{
						map[string]interface{}{
							"weight": 100,
							"podAffinityTerm": map[string]interface{}{
								"topologyKey": "kubernetes.io/hostname",
							},
						},
					},
				},
			},
			"resources": map[string]interface{}{
				"limits": map[string]interface{}{
					"cpu":    "2",
					"memory": "4Gi",
				},
				"requests": map[string]interface{}{
					"cpu":    "500m",
					"memory": "1Gi",
				},
			},
			"podDisruptionBudget": map[string]interface{}{
				"enabled":      true,
				"minAvailable": 2,
			},
		}
	}
}

func buildBatchProfile(env string) map[string]interface{} {
	switch env {
	case "dev":
		return map[string]interface{}{
			"backoffLimit": 1,
			"logLevel":     "debug",
		}
	case "staging":
		return map[string]interface{}{
			"backoffLimit": 2,
		}
	default: // prod
		return map[string]interface{}{
			"backoffLimit": 3,
			"resources": map[string]interface{}{
				"limits": map[string]interface{}{
					"cpu":    "1",
					"memory": "2Gi",
				},
				"requests": map[string]interface{}{
					"cpu":    "250m",
					"memory": "512Mi",
				},
			},
		}
	}
}

func buildCacheProfile(env string) map[string]interface{} {
	switch env {
	case "dev":
		return map[string]interface{}{
			"replicaCount": 1,
			"logLevel":     "debug",
		}
	case "staging":
		return map[string]interface{}{
			"replicaCount": 2,
		}
	default: // prod
		return map[string]interface{}{
			"replicaCount": 2,
			"resources": map[string]interface{}{
				"limits": map[string]interface{}{
					"memory": "512Mi",
				},
				"requests": map[string]interface{}{
					"memory": "256Mi",
				},
			},
			"podDisruptionBudget": map[string]interface{}{
				"enabled":      true,
				"minAvailable": 1,
			},
		}
	}
}

// MergeEnvProfiles performs a deep merge of two values maps.
// For each key in overrides:
//   - If both base[key] and overrides[key] are maps, the maps are recursively merged.
//   - Otherwise, overrides[key] wins.
//
// Keys present only in base are preserved in the result.
func MergeEnvProfiles(base map[string]interface{}, overrides map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(base)+len(overrides))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overrides {
		if baseMap, ok := result[k].(map[string]interface{}); ok {
			if overrideMap, ok := v.(map[string]interface{}); ok {
				result[k] = MergeEnvProfiles(baseMap, overrideMap)
				continue
			}
		}
		result[k] = v
	}
	return result
}
