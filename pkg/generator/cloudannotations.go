package generator

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// CloudProvider identifies a cloud infrastructure provider.
type CloudProvider string

const (
	CloudAWS   CloudProvider = "aws"
	CloudGCP   CloudProvider = "gcp"
	CloudAzure CloudProvider = "azure"
)

// CloudAnnotationConfig holds the configuration for cloud-specific annotation generation.
type CloudAnnotationConfig struct {
	Provider CloudProvider
	Internal bool
	Scheme   string // "internet-facing" or "internal" for AWS
}

// metadataNameRegex matches the metadata block through the name field, capturing
// the entire "metadata:\n  name: <value>" section for replacement.
var metadataNameRegex = regexp.MustCompile(`(metadata:\s*\n\s+name:\s*\S+)`)

// GenerateCloudAnnotations returns provider-specific Kubernetes Service annotations.
// Returns an empty (non-nil) map for unknown or empty providers.
func GenerateCloudAnnotations(config CloudAnnotationConfig) map[string]string {
	annotations := make(map[string]string)

	switch config.Provider {
	case CloudAWS:
		scheme := config.Scheme
		if scheme == "" {
			scheme = "internet-facing"
		}
		annotations["service.beta.kubernetes.io/aws-load-balancer-type"] = "nlb"
		annotations["service.beta.kubernetes.io/aws-load-balancer-scheme"] = scheme
		annotations["service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled"] = "true"

	case CloudGCP:
		annotations["cloud.google.com/neg"] = `{"ingress": true}`
		if config.Internal {
			annotations["cloud.google.com/load-balancer-type"] = "Internal"
		}

	case CloudAzure:
		annotations["service.beta.kubernetes.io/azure-load-balancer-health-probe-request-path"] = "/healthz"
		if config.Internal {
			annotations["service.beta.kubernetes.io/azure-load-balancer-internal"] = "true"
		}

	default:
		// Unknown or empty provider — return empty map without panicking.
	}

	return annotations
}

// InjectCloudAnnotations injects provider-specific annotations into all Service (and Ingress
// for AWS) templates in the chart. Returns nil if chart is nil. The original chart is not
// mutated; a new chart with updated templates is returned.
func InjectCloudAnnotations(chart *types.GeneratedChart, config CloudAnnotationConfig) *types.GeneratedChart {
	if chart == nil {
		return nil
	}

	// Copy templates map — do not mutate the original.
	templates := make(map[string]string, len(chart.Templates))
	for k, v := range chart.Templates {
		templates[k] = v
	}

	for name, content := range templates {
		if strings.Contains(content, "kind: Service") {
			svcAnnotations := GenerateCloudAnnotations(config)
			if len(svcAnnotations) > 0 {
				templates[name] = injectAnnotationsIntoTemplate(content, svcAnnotations)
			}
		}

		if strings.Contains(content, "kind: Ingress") && config.Provider == CloudAWS {
			scheme := config.Scheme
			if scheme == "" {
				scheme = "internet-facing"
			}
			albAnnotations := map[string]string{
				"alb.ingress.kubernetes.io/scheme":      scheme,
				"alb.ingress.kubernetes.io/target-type": "ip",
			}
			templates[name] = injectAnnotationsIntoTemplate(content, albAnnotations)
		}
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

// generateCloudValues produces a values map describing the cloud load-balancer configuration.
// Intended for inclusion in a chart's values.yaml alongside cloud-annotated Service templates.
func generateCloudValues(config CloudAnnotationConfig) map[string]interface{} {
	return map[string]interface{}{
		"cloud": map[string]interface{}{
			"provider": string(config.Provider),
			"loadBalancer": map[string]interface{}{
				"internal": config.Internal,
				"scheme":   config.Scheme,
			},
		},
	}
}

// injectAnnotationsIntoTemplate inserts an annotations block immediately after the
// "metadata:\n  name: <value>" section in a YAML template string.
// Annotation keys are sorted alphabetically for deterministic output.
// This shared helper is used by both cloudannotations and ingressdetect injection paths.
func injectAnnotationsIntoTemplate(template string, annotations map[string]string) string {
	if len(annotations) == 0 {
		return template
	}

	// Sort keys for deterministic output.
	keys := make([]string, 0, len(annotations))
	for k := range annotations {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var lines []string
	lines = append(lines, "  annotations:")
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("    %s: %s", k, annotations[k]))
	}
	annotationsBlock := strings.Join(lines, "\n")

	return metadataNameRegex.ReplaceAllString(template, "$1\n"+annotationsBlock)
}
