package generator

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// CloudProvider identifies a cloud infrastructure provider.
type CloudProvider string

const (
	CloudAWS   CloudProvider = "aws"
	CloudGCP   CloudProvider = "gcp"
	CloudAzure CloudProvider = "azure"

	// CloudProviderAWS/GCP/Azure are aliases used by cost estimation.
	CloudProviderAWS   CloudProvider = "aws"
	CloudProviderGCP   CloudProvider = "gcp"
	CloudProviderAzure CloudProvider = "azure"
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

		// Only AWS Ingress gets ALB annotations. GCP and Azure configure
		// load balancers via Service annotations (handled above), not Ingress.
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

// valuesAnnotationsBlock is the metadata.annotations block dhg processors emit:
// annotations come from the resource's values.
var valuesAnnotationsBlock = regexp.MustCompile(
	`(?m)^  \{\{- with \.annotations \}\}\n  annotations:\n    \{\{- toYaml \. \| nindent 4 \}\}\n  \{\{- end \}\}\n`)

// dhgAnnotationsDecl marks a metadata.annotations block already rewritten by
// injectAnnotationsIntoTemplate; further injections add `set` lines after it.
const dhgAnnotationsDecl = "  {{- $dhgAnnotations := dict }}\n"

// injectAnnotationsIntoTemplate adds metadata annotations to a template.
//
// For dhg-generated templates, whose annotations come from values
// (`{{- with .annotations }}`), the block is rewritten so that injected
// annotations are defaults merged under the user's values — values win, and
// there is exactly one annotations key. Repeated injections extend the same
// block. For other templates the annotations are added to (or create) a static
// annotations block after metadata.name. Values are always quoted.
//
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

	var sets strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&sets, "  {{- $_ := set $dhgAnnotations %s %s }}\n", strconv.Quote(k), strconv.Quote(annotations[k]))
	}

	if strings.Contains(template, dhgAnnotationsDecl) {
		return strings.Replace(template, dhgAnnotationsDecl, dhgAnnotationsDecl+sets.String(), 1)
	}
	if loc := valuesAnnotationsBlock.FindStringIndex(template); loc != nil {
		block := dhgAnnotationsDecl + sets.String() +
			"  {{- with .annotations }}{{- $dhgAnnotations = merge (deepCopy .) $dhgAnnotations }}{{- end }}\n" +
			"  annotations:\n" +
			"    {{- toYaml $dhgAnnotations | nindent 4 }}\n"
		return template[:loc[0]] + block + template[loc[1]:]
	}

	// Static annotations block right after metadata.name: merge into it.
	existingAnnotationsRe := regexp.MustCompile(
		`(metadata:\s*\n\s+name:\s*[^\n]+\n)(  annotations:\s*(?:\{\})?\s*\n(    \S+:.*\n)*)`,
	)
	if loc := existingAnnotationsRe.FindStringIndex(template); loc != nil {
		existingBlock := existingAnnotationsRe.FindString(template)
		var newLines []string
		for _, k := range keys {
			// Only add the key if it is not already present in the block.
			if !strings.Contains(existingBlock, k+":") {
				newLines = append(newLines, fmt.Sprintf("    %s: %s", k, strconv.Quote(annotations[k])))
			}
		}
		if len(newLines) == 0 {
			return template // all keys already present
		}
		insertion := strings.Join(newLines, "\n") + "\n"
		return template[:loc[1]] + insertion + template[loc[1]:]
	}

	// No annotations block — insert a new one after metadata/name.
	lines := []string{"  annotations:"}
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("    %s: %s", k, strconv.Quote(annotations[k])))
	}
	return metadataNameRegex.ReplaceAllString(template, "$1\n"+strings.Join(lines, "\n"))
}
