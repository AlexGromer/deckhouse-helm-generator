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
var metadataNameRegex = regexp.MustCompile(`(metadata:\s*\n\s+name:\s*[^\n]+)`)

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
		if extractKind(content) == "Service" {
			svcAnnotations := GenerateCloudAnnotations(config)
			if len(svcAnnotations) > 0 {
				templates[name] = injectAnnotationsIntoTemplate(content, svcAnnotations)
			}
		}

		if extractKind(content) == "Ingress" && config.Provider == CloudAWS {
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

// annotationsLineRegex matches an existing "  annotations:" line.
var annotationsLineRegex = regexp.MustCompile(`(?m)^  annotations:\s*$`)

// injectAnnotationsIntoTemplate inserts annotation key-value pairs into a YAML
// template. It is idempotent: if an "  annotations:" block already exists right
// after "metadata:\n  name: …", the new keys are merged into that block instead
// of creating a duplicate.
//
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

	// Check whether the template already contains an annotations block after
	// the metadata/name section. We look for the pattern:
	//   metadata:
	//     name: <value>
	//   annotations:
	//     <existing keys>
	// Match existing annotations block (handles both expanded and compact `annotations: {}` forms).
	existingAnnotationsRe := regexp.MustCompile(
		`(metadata:\s*\n\s+name:\s*[^\n]+\n)(  annotations:\s*(?:\{\})?\s*\n(    \S+:.*\n)*)`,
	)

	if loc := existingAnnotationsRe.FindStringIndex(template); loc != nil {
		// An annotations block already exists — append new keys to it.
		match := existingAnnotationsRe.FindStringSubmatch(template)
		existingBlock := match[0]

		var newLines []string
		for _, k := range keys {
			// Only add the key if it is not already present in the block.
			if !strings.Contains(existingBlock, k+":") {
				newLines = append(newLines, fmt.Sprintf("    %s: %s", k, annotations[k]))
			}
		}

		if len(newLines) == 0 {
			return template // all keys already present
		}

		insertion := strings.Join(newLines, "\n") + "\n"
		// Insert the new annotation lines at the end of the existing annotations block.
		return template[:loc[1]] + insertion + template[loc[1]:]
	}

	// No existing annotations block — insert a new one after metadata/name.
	var lines []string
	lines = append(lines, "  annotations:")
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("    %s: %s", k, annotations[k]))
	}
	annotationsBlock := strings.Join(lines, "\n")

	return metadataNameRegex.ReplaceAllString(template, "$1\n"+annotationsBlock)
}
