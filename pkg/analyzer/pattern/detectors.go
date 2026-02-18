package pattern

import (
	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// MicroservicesDetector detects microservices architecture.
type MicroservicesDetector struct{}

func NewMicroservicesDetector() *MicroservicesDetector {
	return &MicroservicesDetector{}
}

func (d *MicroservicesDetector) Name() string {
	return "microservices"
}

func (d *MicroservicesDetector) Detect(graph *types.ResourceGraph) []ArchitecturePattern {
	patterns := make([]ArchitecturePattern, 0)

	// Heuristics for microservices:
	// - Multiple independent services (>= 3)
	// - Each service has its own deployment
	// - Low coupling between services
	// - Services have their own ingresses/endpoints

	if len(graph.Groups) < 3 {
		return patterns
	}

	// Count services with deployments
	servicesWithDeployments := 0
	servicesWithIngress := 0

	for _, group := range graph.Groups {
		hasDeployment := false
		hasIngress := false

		for _, resource := range group.Resources {
			kind := resource.Original.GVK.Kind
			if kind == "Deployment" || kind == "StatefulSet" {
				hasDeployment = true
			}
			if kind == "Ingress" {
				hasIngress = true
			}
		}

		if hasDeployment {
			servicesWithDeployments++
		}
		if hasIngress {
			servicesWithIngress++
		}
	}

	// Microservices if most services are independent
	if servicesWithDeployments >= 3 {
		patterns = append(patterns, PatternMicroservices)
	}

	// Stateless if no persistent storage
	hasPVC := false
	hasStatefulSet := false
	for key := range graph.Resources {
		if key.GVK.Kind == "PersistentVolumeClaim" {
			hasPVC = true
		}
		if key.GVK.Kind == "StatefulSet" {
			hasStatefulSet = true
		}
	}

	if !hasPVC && !hasStatefulSet {
		patterns = append(patterns, PatternStateless)
	}

	return patterns
}

// StatefulDetector detects stateful applications.
type StatefulDetector struct{}

func NewStatefulDetector() *StatefulDetector {
	return &StatefulDetector{}
}

func (d *StatefulDetector) Name() string {
	return "stateful"
}

func (d *StatefulDetector) Detect(graph *types.ResourceGraph) []ArchitecturePattern {
	patterns := make([]ArchitecturePattern, 0)

	hasPVC := false
	hasStatefulSet := false
	hasDaemonSet := false

	for key := range graph.Resources {
		kind := key.GVK.Kind
		if kind == "PersistentVolumeClaim" {
			hasPVC = true
		}
		if kind == "StatefulSet" {
			hasStatefulSet = true
		}
		if kind == "DaemonSet" {
			hasDaemonSet = true
		}
	}

	if hasPVC || hasStatefulSet {
		patterns = append(patterns, PatternStateful)
	}

	if hasDaemonSet {
		patterns = append(patterns, PatternDaemonSet)
	}

	return patterns
}

// DeckhouseDetector detects Deckhouse-specific resources.
type DeckhouseDetector struct{}

func NewDeckhouseDetector() *DeckhouseDetector {
	return &DeckhouseDetector{}
}

func (d *DeckhouseDetector) Name() string {
	return "deckhouse"
}

func (d *DeckhouseDetector) Detect(graph *types.ResourceGraph) []ArchitecturePattern {
	patterns := make([]ArchitecturePattern, 0)

	deckhouseResourceCount := 0

	for key := range graph.Resources {
		if key.GVK.Group == "deckhouse.io" {
			deckhouseResourceCount++
		}
	}

	if deckhouseResourceCount > 0 {
		patterns = append(patterns, PatternDeckhouse)
	}

	// Check for sidecar pattern (multiple containers in deployments)
	for _, resource := range graph.Resources {
		if resource.Original.GVK.Kind != "Deployment" {
			continue
		}

		// Check number of containers
		if containers := resource.Values["containers"]; containers != nil {
			if containerList, ok := containers.([]map[string]interface{}); ok {
				if len(containerList) > 1 {
					patterns = append(patterns, PatternSidecar)
					break
				}
			}
		}
	}

	return patterns
}
