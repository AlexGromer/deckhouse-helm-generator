package generator

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

var hostRegex = regexp.MustCompile(`(?m)^\s*-\s*host:\s*(.+)`)

// InjectTLSConfig adds TLS configuration to every Ingress template in the chart.
// For each Ingress template it:
//   - Adds spec.tls section with secretName derived from host
//   - Adds cert-manager.io/cluster-issuer annotation
//   - Adds force-ssl-redirect annotation
//
// Returns a new chart (copy-on-write). If chart is nil, nil is returned.
func InjectTLSConfig(chart *types.GeneratedChart, issuer string) *types.GeneratedChart {
	if chart == nil {
		return nil
	}

	if issuer == "" {
		issuer = "letsencrypt-prod"
	}

	newTemplates := make(map[string]string, len(chart.Templates))
	for path, content := range chart.Templates {
		if extractKind(content) == "Ingress" {
			content = addTLSAnnotations(content, issuer)
			content = addTLSSection(content)
		}
		newTemplates[path] = content
	}

	return &types.GeneratedChart{
		Name:          chart.Name,
		Path:          chart.Path,
		ChartYAML:     chart.ChartYAML,
		ValuesYAML:    chart.ValuesYAML,
		Templates:     newTemplates,
		Helpers:       chart.Helpers,
		Notes:         chart.Notes,
		ValuesSchema:  chart.ValuesSchema,
		ExternalFiles: chart.ExternalFiles,
	}
}

// addTLSAnnotations injects cert-manager and ssl-redirect annotations into Ingress YAML.
func addTLSAnnotations(content, issuer string) string {
	tlsAnnotations := map[string]string{
		"cert-manager.io/cluster-issuer":                 issuer,
		"nginx.ingress.kubernetes.io/force-ssl-redirect": "true",
	}

	return injectAnnotationsIntoTemplate(content, tlsAnnotations)
}

// addTLSSection appends a spec.tls block to the Ingress YAML based on detected hosts.
func addTLSSection(content string) string {
	// Already has tls section
	if strings.Contains(content, "tls:") {
		return content
	}

	hosts := extractHosts(content)
	if len(hosts) == 0 {
		return content
	}

	// Build TLS section
	var tlsBlock strings.Builder
	tlsBlock.WriteString("  tls:\n")
	for _, host := range hosts {
		secretName := hostToSecretName(host)
		tlsBlock.WriteString(fmt.Sprintf("    - secretName: %s\n", secretName))
		tlsBlock.WriteString("      hosts:\n")
		tlsBlock.WriteString(fmt.Sprintf("        - %s\n", host))
	}

	// Insert before the end of spec (after rules section)
	// Find last line that starts with "  rules:" or insert at end
	lines := strings.Split(content, "\n")
	var result []string
	inserted := false

	for i, line := range lines {
		result = append(result, line)
		// Insert after the rules block ends (next line at spec level or EOF)
		if !inserted && strings.TrimSpace(line) == "" && i > 0 {
			prevTrimmed := strings.TrimSpace(lines[i-1])
			// Check if we just finished a block under spec
			if prevTrimmed != "" && !strings.HasPrefix(prevTrimmed, "#") {
				// Continue looking
			}
		}
	}

	if !inserted {
		// Append at the end
		result = append(result, tlsBlock.String())
	}

	return strings.Join(result, "\n")
}

// extractHosts finds all host values from Ingress rules.
func extractHosts(content string) []string {
	matches := hostRegex.FindAllStringSubmatch(content, -1)
	var hosts []string
	seen := make(map[string]bool)

	for _, m := range matches {
		if len(m) >= 2 {
			host := strings.TrimSpace(m[1])
			host = strings.Trim(host, "\"'")
			if host != "" && !seen[host] {
				seen[host] = true
				hosts = append(hosts, host)
			}
		}
	}

	return hosts
}

// hostToSecretName converts a hostname to a TLS secret name.
// e.g., "app.example.com" → "app-example-com-tls"
func hostToSecretName(host string) string {
	name := strings.ReplaceAll(host, ".", "-")
	name = strings.ReplaceAll(name, "*", "wildcard")
	return name + "-tls"
}
