package pattern

import (
	"strings"

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

// JobDetector detects batch/job processing pattern.
type JobDetector struct{}

func NewJobDetector() *JobDetector {
	return &JobDetector{}
}

func (d *JobDetector) Name() string {
	return "job"
}

func (d *JobDetector) Detect(graph *types.ResourceGraph) []ArchitecturePattern {
	patterns := make([]ArchitecturePattern, 0)

	for key := range graph.Resources {
		kind := key.GVK.Kind
		if kind == "CronJob" || kind == "Job" {
			patterns = append(patterns, PatternJob)
			return patterns
		}
	}

	return patterns
}

// OperatorDetector detects Kubernetes operator pattern.
type OperatorDetector struct{}

func NewOperatorDetector() *OperatorDetector {
	return &OperatorDetector{}
}

func (d *OperatorDetector) Name() string {
	return "operator"
}

func (d *OperatorDetector) Detect(graph *types.ResourceGraph) []ArchitecturePattern {
	patterns := make([]ArchitecturePattern, 0)

	hasCRD := false
	hasControllerDeployment := false

	for key, resource := range graph.Resources {
		if key.GVK.Kind == "CustomResourceDefinition" {
			hasCRD = true
		}

		if key.GVK.Kind == "Deployment" {
			name := key.Name
			// Check labels on the resource
			if resource.Original != nil && resource.Original.Object != nil {
				labels := resource.Original.Object.GetLabels()
				for k, v := range labels {
					if strings.Contains(k, "control-plane") || strings.Contains(v, "control-plane") {
						hasControllerDeployment = true
					}
				}
			}
			// Check name contains controller or operator
			if strings.Contains(name, "controller") || strings.Contains(name, "operator") {
				hasControllerDeployment = true
			}
		}
	}

	if hasCRD && hasControllerDeployment {
		patterns = append(patterns, PatternOperator)
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

// SidecarDetector detects known sidecar containers and recommends injection.
type SidecarDetector struct{}

func NewSidecarDetector() *SidecarDetector {
	return &SidecarDetector{}
}

func (d *SidecarDetector) Name() string {
	return "sidecar"
}

// sidecarSignature maps container name/image substrings to sidecar type.
var sidecarSignatures = map[string]string{
	"envoy":                "service mesh",
	"istio-proxy":          "service mesh",
	"fluent-bit":           "logging",
	"fluentd":              "logging",
	"filebeat":             "logging",
	"vault-agent":          "secrets",
	"datadog-agent":        "monitoring",
	"prometheus-exporter":  "monitoring",
}

func (d *SidecarDetector) Detect(graph *types.ResourceGraph) []ArchitecturePattern {
	patterns := make([]ArchitecturePattern, 0)

	workloadKinds := map[string]bool{
		"Deployment":  true,
		"StatefulSet": true,
		"DaemonSet":   true,
	}

	for key, resource := range graph.Resources {
		if !workloadKinds[key.GVK.Kind] {
			continue
		}

		containers, ok := resource.Values["containers"]
		if !ok || containers == nil {
			continue
		}

		containerList, ok := containers.([]map[string]interface{})
		if !ok {
			continue
		}

		for _, container := range containerList {
			name, _ := container["name"].(string)
			image, _ := container["image"].(string)

			for sig := range sidecarSignatures {
				if strings.Contains(strings.ToLower(name), sig) || strings.Contains(strings.ToLower(image), sig) {
					patterns = append(patterns, PatternSidecar)
					return patterns
				}
			}
		}
	}

	return patterns
}
