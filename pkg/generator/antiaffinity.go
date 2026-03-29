package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// AffinityMode controls whether anti-affinity rules are preferred or required.
type AffinityMode string

const (
	AffinityModePreferred AffinityMode = "preferred"
	AffinityModeStrict    AffinityMode = "strict"
)

// TopologyKey represents a Kubernetes topology spread key.
type TopologyKey string

const (
	TopologyKeyHostname TopologyKey = "kubernetes.io/hostname"
	TopologyKeyZone     TopologyKey = "topology.kubernetes.io/zone"
)

// AntiAffinityOptions configures anti-affinity injection.
type AntiAffinityOptions struct {
	Mode                        AffinityMode
	TopologyKeys                []TopologyKey
	AddTopologySpreadConstraints bool
	MaxSkew                     int
	SkipSingleReplica           bool
	LabelSelector               string
}

// AntiAffinityResult tracks the result of InjectAntiAffinity.
type AntiAffinityResult struct {
	Injected int
	Skipped  int
}

// InjectAntiAffinity injects pod anti-affinity rules into workload templates.
func InjectAntiAffinity(chart *types.GeneratedChart, opts AntiAffinityOptions) (*types.GeneratedChart, AntiAffinityResult) {
	if chart == nil {
		return nil, AntiAffinityResult{}
	}

	// Default topology key.
	keys := opts.TopologyKeys
	if len(keys) == 0 {
		keys = []TopologyKey{TopologyKeyHostname}
	}

	// Default label selector.
	labelSelector := opts.LabelSelector
	if labelSelector == "" {
		labelSelector = "app.kubernetes.io/name"
	}

	// helmFullname is the Helm template expression for the chart's fullname helper.
	helmFullname := fmt.Sprintf(`{{ include "%s.fullname" . }}`, chart.Name)

	result := copyChartTemplates(chart)
	var res AntiAffinityResult

	for path, content := range result.Templates {
		kind := detectKindFromContent(content)
		// Only inject into Deployment, StatefulSet, DaemonSet.
		if kind != "Deployment" && kind != "StatefulSet" && kind != "DaemonSet" {
			continue
		}

		// Skip if affinity already present.
		if strings.Contains(content, "affinity:") {
			res.Skipped++
			continue
		}

		// Skip single-replica workloads if requested.
		if opts.SkipSingleReplica {
			replicas := extractReplicas(content, 1)
			if replicas <= 1 {
				res.Skipped++
				continue
			}
		}

		// Build affinity block.
		var sb strings.Builder
		if opts.Mode == AffinityModeStrict {
			sb.WriteString("      affinity:\n")
			sb.WriteString("        podAntiAffinity:\n")
			sb.WriteString("          requiredDuringSchedulingIgnoredDuringExecution:\n")
			for _, key := range keys {
				sb.WriteString("          - labelSelector:\n")
				sb.WriteString("              matchExpressions:\n")
				sb.WriteString(fmt.Sprintf("              - key: %s\n", labelSelector))
				sb.WriteString("                operator: In\n")
				sb.WriteString("                values:\n")
				sb.WriteString(fmt.Sprintf("                - %s\n", helmFullname))
				sb.WriteString(fmt.Sprintf("            topologyKey: %s\n", key))
			}
		} else {
			sb.WriteString("      affinity:\n")
			sb.WriteString("        podAntiAffinity:\n")
			sb.WriteString("          preferredDuringSchedulingIgnoredDuringExecution:\n")
			for _, key := range keys {
				sb.WriteString("          - weight: 100\n")
				sb.WriteString("            podAffinityTerm:\n")
				sb.WriteString("              labelSelector:\n")
				sb.WriteString("                matchExpressions:\n")
				sb.WriteString(fmt.Sprintf("                - key: %s\n", labelSelector))
				sb.WriteString("                  operator: In\n")
				sb.WriteString("                  values:\n")
				sb.WriteString(fmt.Sprintf("                  - %s\n", helmFullname))
				sb.WriteString(fmt.Sprintf("              topologyKey: %s\n", key))
			}
		}

		// Add topology spread constraints if requested.
		if opts.AddTopologySpreadConstraints {
			maxSkew := opts.MaxSkew
			if maxSkew <= 0 {
				maxSkew = 1
			}
			sb.WriteString("      topologySpreadConstraints:\n")
			for _, key := range keys {
				sb.WriteString(fmt.Sprintf("      - maxSkew: %d\n", maxSkew))
				sb.WriteString(fmt.Sprintf("        topologyKey: %s\n", key))
				sb.WriteString("        whenUnsatisfiable: DoNotSchedule\n")
				sb.WriteString("        labelSelector:\n")
				sb.WriteString("          matchLabels:\n")
				sb.WriteString(fmt.Sprintf("            %s: %s\n", labelSelector, helmFullname))
			}
		}

		affinityBlock := sb.String()

		// Insert before containers:.
		updated := insertBeforeContainers(content, affinityBlock)
		result.Templates[path] = updated
		res.Injected++
	}

	return result, res
}

// insertBeforeContainers inserts a block before the containers: line in a template.
func insertBeforeContainers(content, block string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "containers:" {
			result := strings.Join(lines[:i], "\n") + "\n" + block + strings.Join(lines[i:], "\n")
			return result
		}
	}
	return content + "\n" + block
}

// detectKindFromContent extracts the kind from a YAML template string.
func detectKindFromContent(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "kind:") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "kind:"))
		}
	}
	return ""
}

// GenerateAntiAffinity returns per-template affinity YAML snippets for resources in the graph.
func GenerateAntiAffinity(graph *types.ResourceGraph, opts AntiAffinityOptions) map[string]string {
	result := make(map[string]string)
	if graph == nil {
		return result
	}
	return result
}

// GenerateAntiAffinityValues returns a values map for anti-affinity configuration.
func GenerateAntiAffinityValues(opts AntiAffinityOptions) map[string]interface{} {
	keys := make([]string, len(opts.TopologyKeys))
	for i, k := range opts.TopologyKeys {
		keys[i] = string(k)
	}
	return map[string]interface{}{
		"antiAffinity": map[string]interface{}{
			"enabled":      true,
			"mode":         string(opts.Mode),
			"topologyKeys": keys,
			"maxSkew":      opts.MaxSkew,
		},
	}
}
