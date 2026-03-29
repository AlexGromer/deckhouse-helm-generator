package generator

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// EgressOptions configures egress policy generation.
type EgressOptions struct {
	// DetectFromEnv controls whether URL env vars are parsed for hosts.
	DetectFromEnv bool
	// AllowedHosts lists explicit external hosts to permit.
	AllowedHosts []string
}

// EgressResult holds generated egress policy templates.
type EgressResult struct {
	// ServiceEntries maps filename → Istio ServiceEntry YAML.
	ServiceEntries map[string]string
	// NetworkPolicies maps filename → Kubernetes NetworkPolicy YAML.
	NetworkPolicies map[string]string
	// DetectedURLs lists raw URL strings extracted from env vars.
	DetectedURLs []string
	NOTESTxt     string
}

// urlEnvVarSuffixes are env var name fragments that indicate a URL value.
var urlEnvVarSuffixes = []string{"_URL", "_ADDR", "_HOST", "_ENDPOINT"}

// GenerateEgressPolicies generates Istio ServiceEntry and Kubernetes NetworkPolicy
// egress rules based on the resource graph.
func GenerateEgressPolicies(graph *types.ResourceGraph, opts EgressOptions) *EgressResult {
	result := &EgressResult{
		ServiceEntries:  make(map[string]string),
		NetworkPolicies: make(map[string]string),
		DetectedURLs:    []string{},
	}

	// Collect hosts to allow
	seenHosts := make(map[string]bool)
	deploymentNames := []string{}

	// Step 1: detect from env vars
	if opts.DetectFromEnv && graph != nil {
		for _, r := range graph.Resources {
			if r == nil || r.Original == nil {
				continue
			}
			if r.Original.GVK.Kind != "Deployment" {
				continue
			}
			name := r.Original.Object.GetName()
			deploymentNames = append(deploymentNames, name)

			envVars := extractEgressEnvVars(r)
			for envName, envVal := range envVars {
				if isURLEnvVar(envName) {
					host := extractHostFromURL(envVal)
					if host != "" && !seenHosts[host] {
						seenHosts[host] = true
						result.DetectedURLs = append(result.DetectedURLs, envVal)
					}
				}
			}
		}
	} else if graph != nil {
		// Collect deployment names even when DetectFromEnv=false
		for _, r := range graph.Resources {
			if r == nil || r.Original == nil {
				continue
			}
			if r.Original.GVK.Kind == "Deployment" {
				deploymentNames = append(deploymentNames, r.Original.Object.GetName())
			}
		}
	}

	// Step 2: add explicit AllowedHosts
	for _, h := range opts.AllowedHosts {
		if !seenHosts[h] {
			seenHosts[h] = true
		}
	}

	// Step 3: build ServiceEntries for all known hosts
	hostList := make([]string, 0)
	for h := range seenHosts {
		hostList = append(hostList, h)
	}
	// Also add AllowedHosts that may not have been added above
	for _, h := range opts.AllowedHosts {
		alreadyIn := false
		for _, existing := range hostList {
			if existing == h {
				alreadyIn = true
				break
			}
		}
		if !alreadyIn {
			hostList = append(hostList, h)
		}
	}

	for _, host := range hostList {
		yaml := generateServiceEntryYAML(host)
		key := fmt.Sprintf("templates/istio-se-%s.yaml", sanitizeHostForFilename(host))
		result.ServiceEntries[key] = yaml
	}

	// Step 4: generate NetworkPolicy with egress for each Deployment
	for _, depName := range deploymentNames {
		np := generateEgressNetworkPolicyYAML(depName)
		key := fmt.Sprintf("templates/netpol-egress-%s.yaml", depName)
		result.NetworkPolicies[key] = np
	}

	// Build NOTESTxt only when there is something to report
	if len(result.ServiceEntries) > 0 || len(result.NetworkPolicies) > 0 || len(result.DetectedURLs) > 0 {
		result.NOTESTxt = buildEgressNOTESTxt(result)
	}

	return result
}

// InjectEgressPolicies merges EgressResult templates into an existing chart.
// It is copy-on-write (original chart is not modified).
func InjectEgressPolicies(chart *types.GeneratedChart, result *EgressResult) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}

	newChart := copyChartTemplates(chart)
	count := 0

	if result != nil {
		for k, v := range result.ServiceEntries {
			if _, exists := newChart.Templates[k]; !exists {
				newChart.Templates[k] = v
				count++
			}
		}
		for k, v := range result.NetworkPolicies {
			if _, exists := newChart.Templates[k]; !exists {
				newChart.Templates[k] = v
				count++
			}
		}
	}

	return newChart, count
}

// extractEgressEnvVars returns env var name→value map from a Deployment-style ProcessedResource.
func extractEgressEnvVars(r *types.ProcessedResource) map[string]string {
	result := make(map[string]string)
	if r == nil || r.Original == nil || r.Original.Object == nil {
		return result
	}
	spec, ok := r.Original.Object.Object["spec"].(map[string]interface{})
	if !ok {
		return result
	}
	template, ok := spec["template"].(map[string]interface{})
	if !ok {
		return result
	}
	podSpec, ok := template["spec"].(map[string]interface{})
	if !ok {
		return result
	}
	containers, ok := podSpec["containers"].([]interface{})
	if !ok {
		return result
	}
	for _, c := range containers {
		cMap, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		envList, ok := cMap["env"].([]interface{})
		if !ok {
			continue
		}
		for _, e := range envList {
			eMap, ok := e.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := eMap["name"].(string)
			value, _ := eMap["value"].(string)
			if name != "" && value != "" {
				result[name] = value
			}
		}
	}
	return result
}

// isURLEnvVar returns true if the env var name suggests it holds a URL/address.
func isURLEnvVar(name string) bool {
	upper := strings.ToUpper(name)
	for _, suffix := range urlEnvVarSuffixes {
		if strings.HasSuffix(upper, suffix) {
			return true
		}
	}
	return false
}

// extractHostFromURL parses a raw URL string and returns only the hostname.
func extractHostFromURL(raw string) string {
	// Handle scheme-less values like "hostname:port"
	if !strings.Contains(raw, "://") {
		// Try adding a dummy scheme
		raw = "dummy://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	return u.Hostname()
}

// sanitizeHostForFilename replaces dots and colons with dashes.
func sanitizeHostForFilename(host string) string {
	r := strings.NewReplacer(".", "-", ":", "-")
	return r.Replace(host)
}

// generateServiceEntryYAML builds a minimal Istio ServiceEntry YAML for a host.
func generateServiceEntryYAML(host string) string {
	return fmt.Sprintf(`apiVersion: networking.istio.io/v1beta1
kind: ServiceEntry
metadata:
  name: egress-%s
spec:
  hosts:
  - %s
  location: MESH_EXTERNAL
  resolution: DNS
  ports:
  - number: 443
    name: https
    protocol: HTTPS
  - number: 80
    name: http
    protocol: HTTP
`, sanitizeHostForFilename(host), host)
}

// generateEgressNetworkPolicyYAML builds a NetworkPolicy with egress rules.
func generateEgressNetworkPolicyYAML(name string) string {
	return fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: %s-egress
spec:
  podSelector:
    matchLabels:
      app: %s
  policyTypes:
  - Egress
  egress:
  - {}
`, name, name)
}

// buildEgressNOTESTxt returns usage instructions for generated egress policies.
func buildEgressNOTESTxt(result *EgressResult) string {
	var sb strings.Builder
	sb.WriteString("Egress Policies\n")
	sb.WriteString("===============\n")
	fmt.Fprintf(&sb, "Generated %d ServiceEntry resource(s) and %d NetworkPolicy resource(s).\n",
		len(result.ServiceEntries), len(result.NetworkPolicies))
	if len(result.DetectedURLs) > 0 {
		sb.WriteString("\nDetected external URLs from env vars:\n")
		for _, u := range result.DetectedURLs {
			fmt.Fprintf(&sb, "  - %s\n", u)
		}
	}
	sb.WriteString(`
Apply these templates to allow egress traffic to detected external services.
Review each ServiceEntry and NetworkPolicy before applying to production.
`)
	return sb.String()
}
