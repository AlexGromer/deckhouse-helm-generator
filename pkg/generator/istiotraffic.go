package generator

import (
	"fmt"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// IstioTrafficOptions configures Istio traffic management generation.
type IstioTrafficOptions struct {
	EnableMTLS     bool
	DefaultTimeout string
	RetryAttempts  int
}

// IstioTrafficResult holds generated Istio traffic management templates.
type IstioTrafficResult struct {
	Templates map[string]string
	NOTESTxt  string
}

// GenerateIstioTraffic generates VirtualService, DestinationRule, and optionally
// PeerAuthentication templates for each Service found in the resource graph.
func GenerateIstioTraffic(graph *types.ResourceGraph, opts IstioTrafficOptions) *IstioTrafficResult {
	result := &IstioTrafficResult{
		Templates: make(map[string]string),
	}

	if graph == nil || len(graph.Resources) == 0 {
		result.NOTESTxt = buildIstioTrafficNOTESTxt()
		return result
	}

	for _, r := range graph.Resources {
		if r == nil || r.Original == nil || r.Original.GVK.Kind != "Service" {
			continue
		}
		name := r.Original.Object.GetName()
		ns := r.Original.Object.GetNamespace()
		if ns == "" {
			ns = "default"
		}

		// VirtualService
		vsYAML := generateVirtualServiceYAML(name, ns, opts)
		result.Templates[fmt.Sprintf("templates/istio-vs-%s.yaml", name)] = vsYAML

		// DestinationRule
		drYAML := generateIstioDestinationRuleYAML(name, ns)
		result.Templates[fmt.Sprintf("templates/istio-dr-%s.yaml", name)] = drYAML

		// PeerAuthentication (only when mTLS enabled)
		if opts.EnableMTLS {
			paYAML := generatePeerAuthenticationYAML(name, ns)
			result.Templates[fmt.Sprintf("templates/istio-pa-%s.yaml", name)] = paYAML
		}
	}

	result.NOTESTxt = buildIstioTrafficNOTESTxt()
	return result
}

// InjectIstioTraffic merges IstioTrafficResult templates into an existing chart.
// It is copy-on-write (original chart is not modified) and idempotent.
func InjectIstioTraffic(chart *types.GeneratedChart, result *IstioTrafficResult) (*types.GeneratedChart, int) {
	if chart == nil {
		return nil, 0
	}

	newChart := copyChartTemplates(chart)
	count := 0

	if result != nil {
		for k, v := range result.Templates {
			if _, exists := newChart.Templates[k]; !exists {
				newChart.Templates[k] = v
				count++
			}
		}
	}

	return newChart, count
}

// generateVirtualServiceYAML builds a VirtualService YAML for the given service.
func generateVirtualServiceYAML(name, namespace string, opts IstioTrafficOptions) string {
	timeout := opts.DefaultTimeout
	if timeout == "" {
		timeout = "30s"
	}

	var sb strings.Builder
	sb.WriteString("apiVersion: networking.istio.io/v1beta1\n")
	sb.WriteString("kind: VirtualService\n")
	sb.WriteString("metadata:\n")
	fmt.Fprintf(&sb, "  name: %s\n", name)
	fmt.Fprintf(&sb, "  namespace: %s\n", namespace)
	sb.WriteString("spec:\n")
	fmt.Fprintf(&sb, "  hosts:\n  - %s\n", name)
	sb.WriteString("  http:\n  - route:\n    - destination:\n")
	fmt.Fprintf(&sb, "        host: %s\n", name)
	fmt.Fprintf(&sb, "    timeout: %s\n", timeout)

	if opts.RetryAttempts > 0 {
		sb.WriteString("    retries:\n")
		fmt.Fprintf(&sb, "      attempts: %d\n", opts.RetryAttempts)
		sb.WriteString("      perTryTimeout: 5s\n")
	}

	return sb.String()
}

// generateIstioDestinationRuleYAML builds a simple DestinationRule YAML.
func generateIstioDestinationRuleYAML(name, namespace string) string {
	return fmt.Sprintf(`apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: %s
  namespace: %s
spec:
  host: %s
  trafficPolicy:
    tls:
      mode: ISTIO_MUTUAL
`, name, namespace, name)
}

// generatePeerAuthenticationYAML builds a PeerAuthentication YAML with STRICT mTLS.
func generatePeerAuthenticationYAML(name, namespace string) string {
	return fmt.Sprintf(`apiVersion: security.istio.io/v1beta1
kind: PeerAuthentication
metadata:
  name: %s-mtls
  namespace: %s
spec:
  mtls:
    mode: STRICT
`, name, namespace)
}

// buildIstioTrafficNOTESTxt returns usage instructions for generated Istio resources.
func buildIstioTrafficNOTESTxt() string {
	return `Istio Traffic Management
========================
Generated templates include VirtualService and DestinationRule resources.
When EnableMTLS=true, PeerAuthentication (mode: STRICT) is also generated.

Usage:
  - Install Istio before applying these templates.
  - Verify istio-injection label is present on the namespace.
  - Adjust timeout and retry values in VirtualService to match your SLOs.
`
}
