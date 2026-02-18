package pattern

import (
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ResourceLimitsChecker checks for resource limits and requests.
type ResourceLimitsChecker struct{}

func NewResourceLimitsChecker() *ResourceLimitsChecker {
	return &ResourceLimitsChecker{}
}

func (c *ResourceLimitsChecker) Name() string {
	return "resource-limits"
}

func (c *ResourceLimitsChecker) Category() string {
	return "Resource Management"
}

func (c *ResourceLimitsChecker) Check(graph *types.ResourceGraph) []BestPractice {
	practices := make([]BestPractice, 0)

	workloadKinds := map[string]bool{
		"Deployment":  true,
		"StatefulSet": true,
		"DaemonSet":   true,
	}

	missingLimits := make([]types.ResourceKey, 0)
	missingRequests := make([]types.ResourceKey, 0)

	for key, resource := range graph.Resources {
		if !workloadKinds[key.GVK.Kind] {
			continue
		}

		// Check containers for resource limits/requests
		containers := resource.Values["containers"]
		if containers == nil {
			continue
		}

		containerList, ok := containers.([]map[string]interface{})
		if !ok {
			continue
		}

		hasLimits := false
		hasRequests := false

		for _, container := range containerList {
			if resourcesRaw, ok := container["resources"]; ok {
				if resources, ok := resourcesRaw.(map[string]interface{}); ok {
					if _, ok := resources["limits"]; ok {
						hasLimits = true
					}
					if _, ok := resources["requests"]; ok {
						hasRequests = true
					}
				}
			}
		}

		if !hasLimits {
			missingLimits = append(missingLimits, key)
		}
		if !hasRequests {
			missingRequests = append(missingRequests, key)
		}
	}

	// Resource limits best practice
	if len(missingLimits) > 0 {
		practices = append(practices, BestPractice{
			ID:          "BP-001",
			Title:       "Resource Limits Not Set",
			Description: "Containers should have resource limits to prevent resource exhaustion",
			Category:    c.Category(),
			Severity:    SeverityWarning,
			Compliant:   false,
			Recommendations: []string{
				"Add resources.limits.cpu and resources.limits.memory to all containers",
				"Use reasonable limits based on application requirements",
				"Consider using VPA (Vertical Pod Autoscaler) for automatic recommendations",
			},
			AffectedResources: missingLimits,
			AutoFixable:       false,
		})
	}

	// Resource requests best practice
	if len(missingRequests) > 0 {
		practices = append(practices, BestPractice{
			ID:          "BP-002",
			Title:       "Resource Requests Not Set",
			Description: "Containers should have resource requests for proper scheduling",
			Category:    c.Category(),
			Severity:    SeverityWarning,
			Compliant:   false,
			Recommendations: []string{
				"Add resources.requests.cpu and resources.requests.memory to all containers",
				"Set requests to typical usage, not peak usage",
				"Ensure requests <= limits",
			},
			AffectedResources: missingRequests,
			AutoFixable:       false,
		})
	}

	// If all resources have limits and requests
	if len(missingLimits) == 0 && len(missingRequests) == 0 {
		practices = append(practices, BestPractice{
			ID:          "BP-001",
			Title:       "Resource Limits and Requests Configured",
			Description: "All workloads have proper resource limits and requests",
			Category:    c.Category(),
			Severity:    SeverityInfo,
			Compliant:   true,
			Recommendations: []string{
				"Continue monitoring resource usage",
				"Adjust limits based on actual usage patterns",
			},
			AffectedResources: []types.ResourceKey{},
			AutoFixable:       false,
		})
	}

	return practices
}

// SecurityChecker checks for security best practices.
type SecurityChecker struct{}

func NewSecurityChecker() *SecurityChecker {
	return &SecurityChecker{}
}

func (c *SecurityChecker) Name() string {
	return "security"
}

func (c *SecurityChecker) Category() string {
	return "Security"
}

func (c *SecurityChecker) Check(graph *types.ResourceGraph) []BestPractice {
	practices := make([]BestPractice, 0)

	workloadKinds := map[string]bool{
		"Deployment":  true,
		"StatefulSet": true,
		"DaemonSet":   true,
	}

	runAsNonRoot := make([]types.ResourceKey, 0)
	readOnlyRootFS := make([]types.ResourceKey, 0)
	privilegedContainers := make([]types.ResourceKey, 0)

	for key, resource := range graph.Resources {
		if !workloadKinds[key.GVK.Kind] {
			continue
		}

		// Check securityContext
		containers := resource.Values["containers"]
		if containers == nil {
			runAsNonRoot = append(runAsNonRoot, key)
			readOnlyRootFS = append(readOnlyRootFS, key)
			continue
		}

		containerList, ok := containers.([]map[string]interface{})
		if !ok {
			continue
		}

		for _, container := range containerList {
			secCtx, ok := container["securityContext"].(map[string]interface{})
			if !ok {
				runAsNonRoot = append(runAsNonRoot, key)
				readOnlyRootFS = append(readOnlyRootFS, key)
				continue
			}

			// Check runAsNonRoot
			if runAsNonRootVal, ok := secCtx["runAsNonRoot"].(bool); !ok || !runAsNonRootVal {
				runAsNonRoot = append(runAsNonRoot, key)
			}

			// Check readOnlyRootFilesystem
			if readOnlyVal, ok := secCtx["readOnlyRootFilesystem"].(bool); !ok || !readOnlyVal {
				readOnlyRootFS = append(readOnlyRootFS, key)
			}

			// Check for privileged containers
			if privilegedVal, ok := secCtx["privileged"].(bool); ok && privilegedVal {
				privilegedContainers = append(privilegedContainers, key)
			}
		}
	}

	// Run as non-root
	if len(runAsNonRoot) > 0 {
		practices = append(practices, BestPractice{
			ID:          "BP-SEC-001",
			Title:       "Containers Running as Root",
			Description: "Containers should run as non-root user for security",
			Category:    c.Category(),
			Severity:    SeverityError,
			Compliant:   false,
			Recommendations: []string{
				"Add securityContext.runAsNonRoot: true to containers",
				"Add securityContext.runAsUser: <UID> with non-zero UID",
				"Ensure application supports non-root execution",
			},
			AffectedResources: runAsNonRoot,
			AutoFixable:       true,
		})
	}

	// Read-only root filesystem
	if len(readOnlyRootFS) > 0 {
		practices = append(practices, BestPractice{
			ID:          "BP-SEC-002",
			Title:       "Root Filesystem Not Read-Only",
			Description: "Containers should use read-only root filesystem",
			Category:    c.Category(),
			Severity:    SeverityWarning,
			Compliant:   false,
			Recommendations: []string{
				"Add securityContext.readOnlyRootFilesystem: true",
				"Mount emptyDir volumes for writable directories",
				"Ensure application writes only to mounted volumes",
			},
			AffectedResources: readOnlyRootFS,
			AutoFixable:       true,
		})
	}

	// Privileged containers
	if len(privilegedContainers) > 0 {
		practices = append(practices, BestPractice{
			ID:          "BP-SEC-003",
			Title:       "Privileged Containers Detected",
			Description: "Privileged containers have full host access and should be avoided",
			Category:    c.Category(),
			Severity:    SeverityCritical,
			Compliant:   false,
			Recommendations: []string{
				"Remove securityContext.privileged: true",
				"Use specific capabilities instead of privileged mode",
				"Consider using Pod Security Standards/Policies",
			},
			AffectedResources: privilegedContainers,
			AutoFixable:       false,
		})
	}

	return practices
}

// HighAvailabilityChecker checks for high availability best practices.
type HighAvailabilityChecker struct{}

func NewHighAvailabilityChecker() *HighAvailabilityChecker {
	return &HighAvailabilityChecker{}
}

func (c *HighAvailabilityChecker) Name() string {
	return "high-availability"
}

func (c *HighAvailabilityChecker) Category() string {
	return "High Availability"
}

func (c *HighAvailabilityChecker) Check(graph *types.ResourceGraph) []BestPractice {
	practices := make([]BestPractice, 0)

	singleReplica := make([]types.ResourceKey, 0)
	missingProbes := make([]types.ResourceKey, 0)
	missingPDB := make([]types.ResourceKey, 0)

	deploymentCount := 0

	for key, resource := range graph.Resources {
		if key.GVK.Kind != "Deployment" {
			continue
		}

		deploymentCount++

		// Check replicas
		if replicasRaw, ok := resource.Values["replicas"]; ok {
			if replicas, ok := replicasRaw.(int64); ok && replicas == 1 {
				singleReplica = append(singleReplica, key)
			}
		}

		// Check health probes
		containers := resource.Values["containers"]
		if containers != nil {
			if containerList, ok := containers.([]map[string]interface{}); ok {
				hasProbes := false
				for _, container := range containerList {
					if _, ok := container["livenessProbe"]; ok {
						hasProbes = true
					}
					if _, ok := container["readinessProbe"]; ok {
						hasProbes = true
					}
				}
				if !hasProbes {
					missingProbes = append(missingProbes, key)
				}
			}
		}

		// Check for PodDisruptionBudget
		hasPDB := false
		for pdbKey := range graph.Resources {
			if pdbKey.GVK.Kind == "PodDisruptionBudget" {
				hasPDB = true
				break
			}
		}
		if !hasPDB && deploymentCount > 0 {
			missingPDB = append(missingPDB, key)
		}
	}

	// Single replica warning
	if len(singleReplica) > 0 {
		practices = append(practices, BestPractice{
			ID:          "BP-HA-001",
			Title:       "Single Replica Deployments",
			Description: "Deployments with single replica have no redundancy",
			Category:    c.Category(),
			Severity:    SeverityWarning,
			Compliant:   false,
			Recommendations: []string{
				"Increase replicas to at least 2 for production workloads",
				"Use HorizontalPodAutoscaler for automatic scaling",
				"Consider using pod anti-affinity for availability zones",
			},
			AffectedResources: singleReplica,
			AutoFixable:       true,
		})
	}

	// Missing health probes
	if len(missingProbes) > 0 {
		practices = append(practices, BestPractice{
			ID:          "BP-HA-002",
			Title:       "Missing Health Probes",
			Description: "Containers should have liveness and readiness probes",
			Category:    c.Category(),
			Severity:    SeverityError,
			Compliant:   false,
			Recommendations: []string{
				"Add livenessProbe to detect and restart unhealthy containers",
				"Add readinessProbe to control traffic routing",
				"Use appropriate probe types (HTTP, TCP, exec) for your application",
			},
			AffectedResources: missingProbes,
			AutoFixable:       false,
		})
	}

	// Missing PodDisruptionBudget
	if len(missingPDB) > 0 && deploymentCount > 1 {
		practices = append(practices, BestPractice{
			ID:          "BP-HA-003",
			Title:       "No PodDisruptionBudget Defined",
			Description: "PodDisruptionBudget protects against voluntary disruptions",
			Category:    c.Category(),
			Severity:    SeverityInfo,
			Compliant:   false,
			Recommendations: []string{
				"Create PodDisruptionBudget for critical deployments",
				"Set minAvailable or maxUnavailable based on requirements",
				"Ensure enough replicas to satisfy PDB constraints",
			},
			AffectedResources: missingPDB,
			AutoFixable:       false,
		})
	}

	return practices
}
