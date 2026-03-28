package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// AutoFixResult tracks which fixes were applied to the chart.
type AutoFixResult struct {
	SecurityContextInjected int
	ResourcesInjected       int
	HealthProbesInjected    int
	PDBsGenerated           int
	PSSRestrictedApplied    int
	GracefulShutdownAdded   int
}

// copyChartTemplates returns a shallow copy of the chart with a cloned Templates map.
func copyChartTemplates(chart *types.GeneratedChart) *types.GeneratedChart {
	templates := make(map[string]string, len(chart.Templates))
	for k, v := range chart.Templates {
		templates[k] = v
	}
	return &types.GeneratedChart{
		Name:          chart.Name,
		Path:          chart.Path,
		ChartYAML:     chart.ChartYAML,
		ValuesYAML:    chart.ValuesYAML,
		Templates:     templates,
		Helpers:       chart.Helpers,
		Notes:         chart.Notes,
		ValuesSchema:  chart.ValuesSchema,
		ExternalFiles: chart.ExternalFiles,
	}
}

// InjectSecurityContext adds container-level securityContext to workload templates
// that are missing it: runAsNonRoot, readOnlyRootFilesystem, allowPrivilegeEscalation=false,
// capabilities drop ALL. Uses copy-on-write.
func InjectSecurityContext(chart *types.GeneratedChart) (*types.GeneratedChart, int) {
	result := copyChartTemplates(chart)
	count := 0

	for path, content := range result.Templates {
		if !isWorkloadTemplate(content) {
			continue
		}
		if strings.Contains(content, "securityContext:") {
			continue
		}
		result.Templates[path] = injectSecurityContext(content, "restricted")
		count++
	}

	return result, count
}

// InjectResourceDefaults adds resource requests/limits to workload templates that lack them.
// Uses the profile from resourceProfiles based on workloadType. Copy-on-write.
func InjectResourceDefaults(chart *types.GeneratedChart, workloadType WorkloadType) (*types.GeneratedChart, int) {
	profile, ok := resourceProfiles[workloadType]
	if !ok {
		profile = resourceProfiles[WorkloadWeb]
	}

	result := copyChartTemplates(chart)
	count := 0

	for path, content := range result.Templates {
		if !isWorkloadTemplate(content) {
			continue
		}
		if strings.Contains(content, "resources:") {
			continue
		}
		result.Templates[path] = injectResources(content, profile)
		count++
	}

	return result, count
}

// httpProbePorts are ports that get HTTP health probes (httpGet /healthz).
var httpProbePorts = map[string]bool{
	"80": true, "8080": true, "3000": true,
}

// InjectHealthProbes adds liveness, readiness, and startup probes to workload templates
// that are missing them. HTTP probes for ports 80/8080/3000, TCP probes for others.
// Copy-on-write.
func InjectHealthProbes(chart *types.GeneratedChart) (*types.GeneratedChart, int) {
	result := copyChartTemplates(chart)
	count := 0

	for path, content := range result.Templates {
		if !isWorkloadTemplate(content) {
			continue
		}
		if strings.Contains(content, "livenessProbe:") || strings.Contains(content, "readinessProbe:") {
			continue
		}

		port := detectContainerPort(content)
		probeBlock := buildProbeBlock(port)

		lines := strings.Split(content, "\n")
		var out []string
		injected := false

		for _, line := range lines {
			out = append(out, line)
			if !injected && strings.Contains(line, "image:") {
				indent := leadingSpaces(line)
				for _, pl := range strings.Split(probeBlock, "\n") {
					if pl == "" {
						continue
					}
					out = append(out, indent+pl)
				}
				injected = true
			}
		}

		if injected {
			result.Templates[path] = strings.Join(out, "\n")
			count++
		}
	}

	return result, count
}

// detectContainerPort extracts the first containerPort value from the template content.
// Returns "8080" as default if no port is found.
func detectContainerPort(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		// Handle both "containerPort: 8080" and "- containerPort: 8080"
		trimmed = strings.TrimPrefix(trimmed, "- ")
		if strings.HasPrefix(trimmed, "containerPort:") {
			port := strings.TrimSpace(strings.TrimPrefix(trimmed, "containerPort:"))
			if port != "" {
				return port
			}
		}
	}
	return "8080"
}

// buildProbeBlock generates probe YAML for the given port.
func buildProbeBlock(port string) string {
	if httpProbePorts[port] {
		return fmt.Sprintf(`livenessProbe:
  httpGet:
    path: /healthz
    port: %s
  initialDelaySeconds: 10
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /healthz
    port: %s
  initialDelaySeconds: 5
  periodSeconds: 5
startupProbe:
  httpGet:
    path: /healthz
    port: %s
  failureThreshold: 30
  periodSeconds: 10`, port, port, port)
	}

	return fmt.Sprintf(`livenessProbe:
  tcpSocket:
    port: %s
  initialDelaySeconds: 10
  periodSeconds: 10
readinessProbe:
  tcpSocket:
    port: %s
  initialDelaySeconds: 5
  periodSeconds: 5
startupProbe:
  tcpSocket:
    port: %s
  failureThreshold: 30
  periodSeconds: 10`, port, port, port)
}

// InjectPDB generates a PodDisruptionBudget template for workloads with replicas >= 2.
// Reuses GenerateSpotPDBHelm logic. Copy-on-write.
func InjectPDB(chart *types.GeneratedChart) (*types.GeneratedChart, int) {
	result := copyChartTemplates(chart)
	count := 0

	for name, content := range chart.Templates {
		if !strings.Contains(content, "kind: Deployment") && !strings.Contains(content, "kind: StatefulSet") {
			continue
		}

		replicas := extractReplicas(content, 1)
		if replicas < 2 {
			continue
		}

		pdbKey := strings.TrimSuffix(name, ".yaml") + "-pdb.yaml"
		if _, exists := result.Templates[pdbKey]; exists {
			continue
		}

		result.Templates[pdbKey] = GenerateSpotPDBHelm(chart.Name, replicas)
		count++
	}

	return result, count
}

// InjectPSSRestricted wraps InjectPSSDefaults with level=restricted.
// Returns the patched chart and the count of modified templates.
func InjectPSSRestricted(chart *types.GeneratedChart) (*types.GeneratedChart, int) {
	before := countTemplatesWithField(chart, "seccompProfile")
	result := InjectPSSDefaults(chart, "restricted")
	after := countTemplatesWithField(result, "seccompProfile")
	return result, after - before
}

// countTemplatesWithField counts how many workload templates contain a given field.
func countTemplatesWithField(chart *types.GeneratedChart, field string) int {
	count := 0
	for _, content := range chart.Templates {
		if isWorkloadTemplate(content) && strings.Contains(content, field) {
			count++
		}
	}
	return count
}

// gracePeriodByKind maps workload kinds to their terminationGracePeriodSeconds defaults.
var gracePeriodByKind = map[string]int{
	"Deployment":  30,
	"StatefulSet": 60,
	"DaemonSet":   30,
	"Job":         30,
	"CronJob":     30,
}

// InjectGracefulShutdown adds preStop lifecycle hooks and terminationGracePeriodSeconds
// to workload templates that lack them. Copy-on-write.
func InjectGracefulShutdown(chart *types.GeneratedChart) (*types.GeneratedChart, int) {
	result := copyChartTemplates(chart)
	count := 0

	for path, content := range result.Templates {
		if !isWorkloadTemplate(content) {
			continue
		}
		if strings.Contains(content, "preStop:") || strings.Contains(content, "lifecycle:") {
			continue
		}

		kind := detectWorkloadKind(content)
		gracePeriod := gracePeriodByKind[kind]
		if gracePeriod == 0 {
			gracePeriod = 30
		}

		shutdownBlock := fmt.Sprintf(`lifecycle:
  preStop:
    exec:
      command: ["sh", "-c", "sleep %d"]
terminationGracePeriodSeconds: %d`, gracePeriod/3, gracePeriod)

		lines := strings.Split(content, "\n")
		var out []string
		injected := false

		for _, line := range lines {
			out = append(out, line)
			if !injected && strings.Contains(line, "image:") {
				indent := leadingSpaces(line)
				for _, bl := range strings.Split(shutdownBlock, "\n") {
					if bl == "" {
						continue
					}
					out = append(out, indent+bl)
				}
				injected = true
			}
		}

		if injected {
			result.Templates[path] = strings.Join(out, "\n")
			count++
		}
	}

	return result, count
}

// detectWorkloadKind returns the Kubernetes kind from a template content string.
func detectWorkloadKind(content string) string {
	for _, kind := range pssWorkloadKinds {
		if strings.Contains(content, "kind: "+kind) {
			return kind
		}
	}
	return "Deployment"
}

// ApplyAllFixes runs all auto-fix functions on the chart and returns the result.
func ApplyAllFixes(chart *types.GeneratedChart, workloadType WorkloadType) (*types.GeneratedChart, *AutoFixResult) {
	result := &AutoFixResult{}
	current := chart

	var n int
	current, n = InjectSecurityContext(current)
	result.SecurityContextInjected = n

	current, n = InjectResourceDefaults(current, workloadType)
	result.ResourcesInjected = n

	current, n = InjectHealthProbes(current)
	result.HealthProbesInjected = n

	current, n = InjectPDB(current)
	result.PDBsGenerated = n

	current, n = InjectPSSRestricted(current)
	result.PSSRestrictedApplied = n

	current, n = InjectGracefulShutdown(current)
	result.GracefulShutdownAdded = n

	return current, result
}
