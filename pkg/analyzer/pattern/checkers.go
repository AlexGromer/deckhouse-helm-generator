package pattern

import (
	"strings"

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

// InitContainerChecker checks for init containers in workloads.
type InitContainerChecker struct{}

func NewInitContainerChecker() *InitContainerChecker {
	return &InitContainerChecker{}
}

func (c *InitContainerChecker) Name() string {
	return "init-containers"
}

func (c *InitContainerChecker) Category() string {
	return "Patterns"
}

func (c *InitContainerChecker) Check(graph *types.ResourceGraph) []BestPractice {
	practices := make([]BestPractice, 0)

	workloadKinds := map[string]bool{
		"Deployment":  true,
		"StatefulSet": true,
		"DaemonSet":   true,
	}

	initContainerResources := make([]types.ResourceKey, 0)

	for key, resource := range graph.Resources {
		if !workloadKinds[key.GVK.Kind] {
			continue
		}

		if initContainers, ok := resource.Values["initContainers"]; ok && initContainers != nil {
			initContainerResources = append(initContainerResources, key)
		}
	}

	if len(initContainerResources) > 0 {
		practices = append(practices, BestPractice{
			ID:          "BP-PAT-001",
			Title:       "Init Containers Detected",
			Description: "Workloads use init containers for initialization tasks",
			Category:    c.Category(),
			Severity:    SeverityInfo,
			Compliant:   true,
			Recommendations: []string{
				"Document the purpose of each init container",
				"Ensure proper resource limits are set on init containers",
			},
			AffectedResources: initContainerResources,
			AutoFixable:       false,
		})
	}

	return practices
}

// QoSClassChecker checks for QoS classification of workloads.
type QoSClassChecker struct{}

func NewQoSClassChecker() *QoSClassChecker {
	return &QoSClassChecker{}
}

func (c *QoSClassChecker) Name() string {
	return "qos-class"
}

func (c *QoSClassChecker) Category() string {
	return "Resource Management"
}

func (c *QoSClassChecker) Check(graph *types.ResourceGraph) []BestPractice {
	practices := make([]BestPractice, 0)

	workloadKinds := map[string]bool{
		"Deployment":  true,
		"StatefulSet": true,
		"DaemonSet":   true,
	}

	bestEffortResources := make([]types.ResourceKey, 0)
	guaranteedResources := make([]types.ResourceKey, 0)

	for key, resource := range graph.Resources {
		if !workloadKinds[key.GVK.Kind] {
			continue
		}

		containers, ok := resource.Values["containers"]
		if !ok || containers == nil {
			bestEffortResources = append(bestEffortResources, key)
			continue
		}

		containerList, ok := containers.([]map[string]interface{})
		if !ok {
			continue
		}

		if len(containerList) == 0 {
			bestEffortResources = append(bestEffortResources, key)
			continue
		}

		anyHasResources := false
		allGuaranteed := true

		for _, container := range containerList {
			resourcesRaw, hasResources := container["resources"]
			if !hasResources || resourcesRaw == nil {
				allGuaranteed = false
				continue
			}

			resources, ok := resourcesRaw.(map[string]interface{})
			if !ok {
				allGuaranteed = false
				continue
			}

			limits, hasLimits := resources["limits"].(map[string]interface{})
			requests, hasRequests := resources["requests"].(map[string]interface{})

			if hasLimits || hasRequests {
				anyHasResources = true
			}

			// Guaranteed: equal requests and limits for both cpu and memory
			if hasLimits && hasRequests {
				limCPU, limMem := limits["cpu"], limits["memory"]
				reqCPU, reqMem := requests["cpu"], requests["memory"]
				if limCPU == nil || limMem == nil || reqCPU == nil || reqMem == nil {
					allGuaranteed = false
				} else if limCPU != reqCPU || limMem != reqMem {
					allGuaranteed = false
				}
			} else {
				allGuaranteed = false
			}
		}

		if !anyHasResources {
			bestEffortResources = append(bestEffortResources, key)
		} else if allGuaranteed {
			guaranteedResources = append(guaranteedResources, key)
		}
	}

	if len(bestEffortResources) > 0 {
		practices = append(practices, BestPractice{
			ID:          "BP-QOS-001",
			Title:       "BestEffort QoS Class Detected",
			Description: "Workloads with no resource requests or limits get BestEffort QoS and are first to be evicted under pressure",
			Category:    c.Category(),
			Severity:    SeverityWarning,
			Compliant:   false,
			Recommendations: []string{
				"Add resource requests and limits to all containers",
				"Use Guaranteed QoS for critical workloads by setting equal requests and limits",
				"Use Burstable QoS for workloads with variable resource needs",
			},
			AffectedResources: bestEffortResources,
			AutoFixable:       false,
		})
	}

	if len(guaranteedResources) > 0 {
		practices = append(practices, BestPractice{
			ID:          "BP-QOS-002",
			Title:       "Guaranteed QoS Class Configured",
			Description: "Workloads have equal resource requests and limits, providing Guaranteed QoS",
			Category:    c.Category(),
			Severity:    SeverityInfo,
			Compliant:   true,
			Recommendations: []string{
				"Continue monitoring resource usage to ensure limits are appropriate",
				"Consider using VPA for automatic resource adjustment",
			},
			AffectedResources: guaranteedResources,
			AutoFixable:       false,
		})
	}

	return practices
}

// StatefulSetPatternChecker checks for StatefulSet best practices.
type StatefulSetPatternChecker struct{}

func NewStatefulSetPatternChecker() *StatefulSetPatternChecker {
	return &StatefulSetPatternChecker{}
}

func (c *StatefulSetPatternChecker) Name() string {
	return "statefulset-patterns"
}

func (c *StatefulSetPatternChecker) Category() string {
	return "Patterns"
}

func (c *StatefulSetPatternChecker) Check(graph *types.ResourceGraph) []BestPractice {
	practices := make([]BestPractice, 0)

	for key, resource := range graph.Resources {
		if key.GVK.Kind != "StatefulSet" {
			continue
		}

		missingItems := make([]string, 0)

		// Check for headless service (serviceName field)
		if serviceName, ok := resource.Values["serviceName"]; !ok || serviceName == nil || serviceName == "" {
			missingItems = append(missingItems, "headless service (serviceName not configured)")
		}

		// Check for podManagementPolicy
		if _, ok := resource.Values["podManagementPolicy"]; !ok {
			missingItems = append(missingItems, "podManagementPolicy not set (defaults to OrderedReady)")
		}

		// Check for updateStrategy
		if _, ok := resource.Values["updateStrategy"]; !ok {
			missingItems = append(missingItems, "updateStrategy not configured")
		}

		if len(missingItems) > 0 {
			recommendations := make([]string, 0, len(missingItems)+1)
			for _, item := range missingItems {
				recommendations = append(recommendations, "Configure "+item)
			}
			recommendations = append(recommendations, "Review StatefulSet documentation for production best practices")

			practices = append(practices, BestPractice{
				ID:          "BP-SS-001",
				Title:       "StatefulSet Best Practices",
				Description: "StatefulSet is missing recommended configuration items",
				Category:    c.Category(),
				Severity:    SeverityWarning,
				Compliant:   false,
				Recommendations: recommendations,
				AffectedResources: []types.ResourceKey{key},
				AutoFixable:       false,
			})
		}
	}

	return practices
}

// DaemonSetPatternChecker checks for DaemonSet best practices.
type DaemonSetPatternChecker struct{}

func NewDaemonSetPatternChecker() *DaemonSetPatternChecker {
	return &DaemonSetPatternChecker{}
}

func (c *DaemonSetPatternChecker) Name() string {
	return "daemonset-patterns"
}

func (c *DaemonSetPatternChecker) Category() string {
	return "Patterns"
}

func (c *DaemonSetPatternChecker) Check(graph *types.ResourceGraph) []BestPractice {
	practices := make([]BestPractice, 0)

	for key, resource := range graph.Resources {
		if key.GVK.Kind != "DaemonSet" {
			continue
		}

		missingItems := make([]string, 0)

		// Check for tolerations
		if tolerations, ok := resource.Values["tolerations"]; !ok || tolerations == nil {
			missingItems = append(missingItems, "tolerations not configured (DaemonSet may not run on all nodes)")
		}

		// Check for updateStrategy
		if _, ok := resource.Values["updateStrategy"]; !ok {
			missingItems = append(missingItems, "updateStrategy not configured")
		}

		// Check for resource limits on containers
		containers, ok := resource.Values["containers"]
		if !ok || containers == nil {
			missingItems = append(missingItems, "resource limits not set (DaemonSet runs on every node!)")
		} else if containerList, ok := containers.([]map[string]interface{}); ok {
			hasLimits := false
			for _, container := range containerList {
				if resourcesRaw, ok := container["resources"]; ok {
					if resources, ok := resourcesRaw.(map[string]interface{}); ok {
						if _, ok := resources["limits"]; ok {
							hasLimits = true
						}
					}
				}
			}
			if !hasLimits {
				missingItems = append(missingItems, "resource limits not set (DaemonSet runs on every node!)")
			}
		}

		if len(missingItems) > 0 {
			recommendations := make([]string, 0, len(missingItems)+1)
			for _, item := range missingItems {
				recommendations = append(recommendations, "Configure "+item)
			}
			recommendations = append(recommendations, "DaemonSets run on every node — resource limits are critical")

			practices = append(practices, BestPractice{
				ID:          "BP-DS-001",
				Title:       "DaemonSet Best Practices",
				Description: "DaemonSet is missing recommended configuration items",
				Category:    c.Category(),
				Severity:    SeverityWarning,
				Compliant:   false,
				Recommendations: recommendations,
				AffectedResources: []types.ResourceKey{key},
				AutoFixable:       false,
			})
		}
	}

	return practices
}

// GracefulShutdownChecker checks for graceful shutdown configuration.
type GracefulShutdownChecker struct{}

func NewGracefulShutdownChecker() *GracefulShutdownChecker {
	return &GracefulShutdownChecker{}
}

func (c *GracefulShutdownChecker) Name() string {
	return "graceful-shutdown"
}

func (c *GracefulShutdownChecker) Category() string {
	return "Reliability"
}

func (c *GracefulShutdownChecker) Check(graph *types.ResourceGraph) []BestPractice {
	practices := make([]BestPractice, 0)

	workloadKinds := map[string]bool{
		"Deployment":  true,
		"StatefulSet": true,
		"DaemonSet":   true,
	}

	missingGraceful := make([]types.ResourceKey, 0)

	for key, resource := range graph.Resources {
		if !workloadKinds[key.GVK.Kind] {
			continue
		}

		hasPreStop := false
		hasTerminationGracePeriod := false

		// Check terminationGracePeriodSeconds in values
		if _, ok := resource.Values["terminationGracePeriodSeconds"]; ok {
			hasTerminationGracePeriod = true
		}

		// Check preStop hooks in containers
		containers, ok := resource.Values["containers"]
		if ok && containers != nil {
			if containerList, ok := containers.([]map[string]interface{}); ok {
				for _, container := range containerList {
					if lifecycle, ok := container["lifecycle"].(map[string]interface{}); ok {
						if _, ok := lifecycle["preStop"]; ok {
							hasPreStop = true
						}
					}
				}
			}
		}

		if !hasPreStop && !hasTerminationGracePeriod {
			missingGraceful = append(missingGraceful, key)
		}
	}

	if len(missingGraceful) > 0 {
		practices = append(practices, BestPractice{
			ID:          "BP-GS-001",
			Title:       "Graceful Shutdown Not Configured",
			Description: "Workloads lack graceful shutdown configuration (preStop hooks or terminationGracePeriodSeconds)",
			Category:    c.Category(),
			Severity:    SeverityWarning,
			Compliant:   false,
			Recommendations: []string{
				"Add lifecycle.preStop hook to containers to handle shutdown gracefully",
				"Set terminationGracePeriodSeconds to allow time for graceful shutdown",
				"Ensure application handles SIGTERM signal correctly",
			},
			AffectedResources: missingGraceful,
			AutoFixable:       false,
		})
	}

	return practices
}

// PodSecurityStandardsChecker checks for Pod Security Standards compliance.
type PodSecurityStandardsChecker struct{}

func NewPodSecurityStandardsChecker() *PodSecurityStandardsChecker {
	return &PodSecurityStandardsChecker{}
}

func (c *PodSecurityStandardsChecker) Name() string {
	return "pod-security-standards"
}

func (c *PodSecurityStandardsChecker) Category() string {
	return "Security"
}

// pssLevel represents PSS compliance level.
type pssLevel string

const (
	pssPrivileged pssLevel = "privileged"
	pssBaseline   pssLevel = "baseline"
	pssRestricted pssLevel = "restricted"
)

func (c *PodSecurityStandardsChecker) Check(graph *types.ResourceGraph) []BestPractice {
	practices := make([]BestPractice, 0)

	workloadKinds := map[string]bool{
		"Deployment":  true,
		"StatefulSet": true,
		"DaemonSet":   true,
	}

	privilegedResources := make([]types.ResourceKey, 0)

	for key, resource := range graph.Resources {
		if !workloadKinds[key.GVK.Kind] {
			continue
		}

		level := c.classifyPSSLevel(resource)

		if level == pssPrivileged {
			privilegedResources = append(privilegedResources, key)
		}
	}

	if len(privilegedResources) > 0 {
		practices = append(practices, BestPractice{
			ID:          "BP-PSS-001",
			Title:       "Pod Security Standards: Privileged Level Workloads",
			Description: "Workloads are running at the privileged PSS level, bypassing all security restrictions",
			Category:    c.Category(),
			Severity:    SeverityCritical,
			Compliant:   false,
			Recommendations: []string{
				"Migrate workloads to baseline or restricted PSS level",
				"Remove privileged:true from securityContext",
				"Remove hostNetwork, hostPID, hostIPC settings",
				"Set runAsNonRoot:true and drop ALL capabilities",
				"Configure seccompProfile for restricted compliance",
			},
			AffectedResources: privilegedResources,
			AutoFixable:       false,
		})
	}

	return practices
}

// classifyPSSLevel determines the PSS level for a workload resource.
func (c *PodSecurityStandardsChecker) classifyPSSLevel(resource *types.ProcessedResource) pssLevel {
	// Check pod-level security context for baseline violations
	if hostNetwork, ok := resource.Values["hostNetwork"].(bool); ok && hostNetwork {
		return pssPrivileged
	}
	if hostPID, ok := resource.Values["hostPID"].(bool); ok && hostPID {
		return pssPrivileged
	}
	if hostIPC, ok := resource.Values["hostIPC"].(bool); ok && hostIPC {
		return pssPrivileged
	}

	// Check container-level security context
	containers, ok := resource.Values["containers"]
	if !ok || containers == nil {
		// No containers info — cannot determine, treat as baseline
		return pssBaseline
	}

	containerList, ok := containers.([]map[string]interface{})
	if !ok {
		return pssBaseline
	}

	for _, container := range containerList {
		secCtx, ok := container["securityContext"].(map[string]interface{})
		if !ok {
			// No securityContext — restricted requirements not met
			return pssBaseline
		}

		// Baseline check: no privileged
		if privileged, ok := secCtx["privileged"].(bool); ok && privileged {
			return pssPrivileged
		}

		// Restricted checks
		runAsNonRoot, _ := secCtx["runAsNonRoot"].(bool)
		if !runAsNonRoot {
			return pssBaseline
		}

		// Check drop ALL capabilities
		capabilities, _ := secCtx["capabilities"].(map[string]interface{})
		if capabilities == nil {
			return pssBaseline
		}
		dropList, _ := capabilities["drop"].([]interface{})
		hasDropAll := false
		for _, d := range dropList {
			if s, ok := d.(string); ok && strings.ToUpper(s) == "ALL" {
				hasDropAll = true
			}
		}
		if !hasDropAll {
			return pssBaseline
		}

		// Check seccompProfile
		if _, ok := secCtx["seccompProfile"]; !ok {
			return pssBaseline
		}
	}

	return pssRestricted
}
