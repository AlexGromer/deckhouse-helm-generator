package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// CanaryStep describes a single canary rollout step.
type CanaryStep struct {
	Weight int
	Pause  string
}

// ProgressiveDeliveryOptions configures progressive delivery (Argo Rollouts) generation.
type ProgressiveDeliveryOptions struct {
	MinReplicas             int
	CanarySteps             []CanaryStep
	IncludeAnalysisTemplate bool
}

// ProgressiveDeliveryResult holds generated Argo Rollouts manifests.
type ProgressiveDeliveryResult struct {
	// Candidates is the list of eligible workload names.
	Candidates []string
	// Rollouts maps workload name → Rollout YAML.
	Rollouts map[string]string
	// AnalysisTemplates maps workload name → AnalysisTemplate YAML.
	AnalysisTemplates map[string]string
	NOTESTxt          string
}

var defaultCanarySteps = []CanaryStep{
	{Weight: 20, Pause: "1m"},
	{Weight: 50, Pause: "2m"},
	{Weight: 100, Pause: ""},
}

// SuggestProgressiveDelivery scans the graph and generates Argo Rollouts YAML
// for eligible Deployments (replicas >= MinReplicas).
func SuggestProgressiveDelivery(graph *types.ResourceGraph, opts ProgressiveDeliveryOptions) *ProgressiveDeliveryResult {
	result := &ProgressiveDeliveryResult{
		Rollouts:          make(map[string]string),
		AnalysisTemplates: make(map[string]string),
	}

	if graph == nil {
		result.NOTESTxt = buildProgressiveDeliveryNOTESTxt(result)
		return result
	}

	minReplicas := opts.MinReplicas
	if minReplicas == 0 {
		minReplicas = 2
	}

	steps := opts.CanarySteps
	if len(steps) == 0 {
		steps = defaultCanarySteps
	}

	for _, r := range graph.Resources {
		if r.Original.GVK.Kind != "Deployment" {
			continue
		}
		name := r.Original.Object.GetName()
		ns := r.Original.Object.GetNamespace()

		// Check replicas.
		replicas := int64(0)
		spec, ok := r.Original.Object.Object["spec"].(map[string]interface{})
		if ok {
			if rep, ok := spec["replicas"].(int64); ok {
				replicas = rep
			}
		}
		if replicas < int64(minReplicas) {
			continue
		}

		result.Candidates = append(result.Candidates, name)
		result.Rollouts[name] = generateRolloutYAML(name, ns, steps)

		if opts.IncludeAnalysisTemplate {
			result.AnalysisTemplates[name] = generateAnalysisTemplateYAML(name, ns)
		}
	}

	result.NOTESTxt = buildProgressiveDeliveryNOTESTxt(result)
	return result
}

func generateRolloutYAML(name, namespace string, steps []CanaryStep) string {
	var sb strings.Builder
	sb.WriteString("apiVersion: argoproj.io/v1alpha1\n")
	sb.WriteString("kind: Rollout\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s\n", name))
	sb.WriteString(fmt.Sprintf("  namespace: %s\n", namespace))
	sb.WriteString("spec:\n")
	sb.WriteString("  strategy:\n")
	sb.WriteString("    canary:\n")
	sb.WriteString("      steps:\n")
	for _, step := range steps {
		sb.WriteString(fmt.Sprintf("        - setWeight: %d\n", step.Weight))
		if step.Pause != "" {
			sb.WriteString(fmt.Sprintf("        - pause: {duration: %s}\n", step.Pause))
		}
	}
	return sb.String()
}

func generateAnalysisTemplateYAML(name, namespace string) string {
	var sb strings.Builder
	sb.WriteString("apiVersion: argoproj.io/v1alpha1\n")
	sb.WriteString("kind: AnalysisTemplate\n")
	sb.WriteString("metadata:\n")
	sb.WriteString(fmt.Sprintf("  name: %s-analysis\n", name))
	sb.WriteString(fmt.Sprintf("  namespace: %s\n", namespace))
	sb.WriteString("spec:\n")
	sb.WriteString("  metrics:\n")
	sb.WriteString("    - name: success-rate\n")
	sb.WriteString("      interval: 5m\n")
	sb.WriteString("      successCondition: result[0] >= 0.95\n")
	return sb.String()
}

func buildProgressiveDeliveryNOTESTxt(result *ProgressiveDeliveryResult) string {
	candidateStr := strings.Join(result.Candidates, ", ")
	if candidateStr == "" {
		candidateStr = "none"
	}
	return fmt.Sprintf(
		"Progressive delivery candidates: %s. %d Rollout(s) generated. "+
			"Requires Argo Rollouts installed in cluster.",
		candidateStr, len(result.Rollouts),
	)
}

// InjectProgressiveDelivery injects Rollout and AnalysisTemplate YAMLs into a chart.
// Returns (copy, 0) if chart is nil.
func InjectProgressiveDelivery(chart *types.GeneratedChart, result *ProgressiveDeliveryResult) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}

	newChart := copyChartTemplates(chart)
	count := 0

	if result == nil {
		return newChart, 0
	}

	for name, yaml := range result.Rollouts {
		path := fmt.Sprintf("templates/rollout-%s.yaml", strings.ToLower(name))
		if _, exists := newChart.Templates[path]; exists {
			continue
		}
		newChart.Templates[path] = yaml
		count++
	}

	for name, yaml := range result.AnalysisTemplates {
		path := fmt.Sprintf("templates/analysis-%s.yaml", strings.ToLower(name))
		if _, exists := newChart.Templates[path]; exists {
			continue
		}
		newChart.Templates[path] = yaml
		count++
	}

	return newChart, count
}
