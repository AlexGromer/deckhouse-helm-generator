package generator

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// Istio egress (feature "istio-egress").
//
// In meshes with outboundTrafficPolicy REGISTRY_ONLY, calls to hosts outside
// the mesh need a ServiceEntry. The feature detects external hosts from the
// workloads' literal env values (URLs, and *_URL/_URI/_ENDPOINT/_ADDR/_HOST
// variables) and renders one MESH_EXTERNAL ServiceEntry per host from values,
// where more hosts can be added. Kubernetes NetworkPolicies cannot select by
// host name; namespace-level NetworkPolicies are produced by
// --namespace-resources.

// egressEnvSuffixes are env var name suffixes that indicate an address.
var egressEnvSuffixes = []string{"_URL", "_URI", "_ENDPOINT", "_ADDR", "_ADDRESS", "_HOST"}

// egressSchemePorts are default ports of URL schemes.
var egressSchemePorts = map[string]int{
	"http": 80, "https": 443, "ws": 80, "wss": 443, "grpc": 443, "grpcs": 443,
	"postgres": 5432, "postgresql": 5432, "mysql": 3306, "redis": 6379, "rediss": 6379,
	"mongodb": 27017, "amqp": 5672, "amqps": 5671, "nats": 4222, "kafka": 9092,
	"ldap": 389, "ldaps": 636, "smtp": 25, "smtps": 465,
}

// egressPort is one port of a ServiceEntry.
type egressPort struct {
	Number   int
	Protocol string
}

// egressHost is one external host with the ports used to reach it.
type egressHost struct {
	Host  string
	Ports []egressPort
}

// egressEndpoint parses an address ("https://h/p", "h:5432", "h") into host
// and port. scheme-less addresses without a port yield port 0.
func egressEndpoint(raw string) (host string, port egressPort, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \t\n,;") {
		return "", port, false
	}
	scheme := ""
	if i := strings.Index(raw, "://"); i > 0 {
		scheme = strings.ToLower(raw[:i])
	} else {
		raw = "tcp://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", port, false
	}
	host = strings.ToLower(u.Hostname())
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 || n > 65535 {
			return "", port, false
		}
		port.Number = n
	} else {
		port.Number = egressSchemePorts[scheme]
	}
	switch {
	case scheme == "http" || scheme == "ws":
		port.Protocol = "HTTP"
	case scheme == "https" || scheme == "wss" || scheme == "grpcs":
		port.Protocol = "TLS"
	case scheme == "grpc":
		port.Protocol = "GRPC"
	case port.Number == 443:
		port.Protocol = "TLS"
	default:
		port.Protocol = "TCP"
	}
	return host, port, true
}

// egressIsExternal reports whether host is outside the cluster: a DNS name
// with a dot that is not a cluster-local name or "<service>.<namespace>".
func egressIsExternal(host string, services, namespaces map[string]bool) bool {
	if host == "" || !strings.Contains(host, ".") || net.ParseIP(host) != nil {
		return false
	}
	if strings.Contains(host, ".svc.") {
		return false
	}
	for _, suffix := range []string{".svc", ".local", ".localhost", ".internal"} {
		if strings.HasSuffix(host, suffix) {
			return false
		}
	}
	labels := strings.Split(host, ".")
	if len(labels) == 2 && (namespaces[labels[1]] || services[labels[0]]) {
		return false
	}
	return true
}

// egressDetectHosts returns the external hosts used by the given workloads.
func egressDetectHosts(workloads []secWorkload, graph *types.ResourceGraph) map[string]map[egressPort]bool {
	services := map[string]bool{}
	namespaces := map[string]bool{}
	if graph != nil {
		for _, r := range graph.Resources {
			if r == nil || r.Original == nil || r.Original.Object == nil {
				continue
			}
			if ns := r.Original.Object.GetNamespace(); ns != "" {
				namespaces[ns] = true
			}
			if r.Original.Object.GetKind() == "Service" {
				services[r.Original.Object.GetName()] = true
			}
		}
	}

	hosts := map[string]map[egressPort]bool{}
	for _, w := range workloads {
		for _, c := range secContainers(w.podSpec) {
			env := map[string]string{}
			envList, _ := c["env"].([]interface{})
			for _, e := range envList {
				em, _ := e.(map[string]interface{})
				name, _ := em["name"].(string)
				value, _ := em["value"].(string)
				if name != "" && value != "" {
					env[name] = value
				}
			}
			for name, value := range env {
				upper := strings.ToUpper(name)
				hasSuffix := false
				for _, s := range egressEnvSuffixes {
					if strings.HasSuffix(upper, s) {
						hasSuffix = true
						break
					}
				}
				if !hasSuffix && !strings.Contains(value, "://") {
					continue
				}
				host, port, ok := egressEndpoint(value)
				if !ok || !egressIsExternal(host, services, namespaces) {
					continue
				}
				if port.Number == 0 && strings.HasSuffix(upper, "_HOST") {
					// FOO_HOST=db.example.com with FOO_PORT=5432
					if p, err := strconv.Atoi(env[name[:len(name)-len("_HOST")]+"_PORT"]); err == nil && p > 0 && p <= 65535 {
						port = egressPort{Number: p, Protocol: "TCP"}
						if p == 443 {
							port.Protocol = "TLS"
						}
					}
				}
				if port.Number == 0 {
					continue // no way to know the port
				}
				if hosts[host] == nil {
					hosts[host] = map[egressPort]bool{}
				}
				hosts[host][port] = true
			}
		}
	}
	return hosts
}

// egressOwnedHosts returns the hosts the chart declares. A host used by
// workloads of several charts (separate/umbrella mode) is declared, with the
// union of its ports, by the chart of the first such workload only, so that
// one release never duplicates another's entries.
func egressOwnedHosts(chart *types.GeneratedChart, graph *types.ResourceGraph) []egressHost {
	mine := map[string]bool{} // host -> owned by this chart
	ports := map[string]map[egressPort]bool{}
	for _, w := range secAllWorkloads(graph) {
		for host, hostPorts := range egressDetectHosts([]secWorkload{w}, graph) {
			if ports[host] == nil {
				ports[host] = map[egressPort]bool{}
				mine[host] = w.inChart(chart)
			}
			for p := range hostPorts {
				ports[host][p] = true
			}
		}
	}
	owned := map[string]map[egressPort]bool{}
	for host, ok := range mine {
		if ok {
			owned[host] = ports[host]
		}
	}
	return egressSortHosts(owned)
}

func egressSortHosts(m map[string]map[egressPort]bool) []egressHost {
	out := make([]egressHost, 0, len(m))
	for host, ports := range m {
		h := egressHost{Host: host}
		for p := range ports {
			h.Ports = append(h.Ports, p)
		}
		sort.Slice(h.Ports, func(i, j int) bool {
			if h.Ports[i].Number != h.Ports[j].Number {
				return h.Ports[i].Number < h.Ports[j].Number
			}
			return h.Ports[i].Protocol < h.Ports[j].Protocol
		})
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out
}

func applyIstioEgressFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	apiVersion := fc.Param("api-version")
	if !strings.HasPrefix(apiVersion, "networking.istio.io/") {
		return nil, fmt.Errorf("api-version must be in the networking.istio.io group, got %q", apiVersion)
	}
	// Explicit hosts: "host", "host:port" or "scheme://host[:port]".
	explicit := map[string]map[egressPort]bool{}
	for _, raw := range fc.ListParam("hosts") {
		host, port, ok := egressEndpoint(raw)
		if !ok {
			return nil, fmt.Errorf("invalid host %q in hosts", raw)
		}
		if port.Number == 0 {
			port = egressPort{Number: 443, Protocol: "TLS"}
		}
		if explicit[host] == nil {
			explicit[host] = map[egressPort]bool{}
		}
		explicit[host][port] = true
	}

	// Charts without workloads (e.g. an umbrella parent) make no calls.
	if len(secChartWorkloads(chart, fc.Graph)) == 0 && !secHasWorkloadTemplate(chart) {
		return chart, nil
	}

	var hosts []egressHost
	if fc.BoolParam("detect") {
		hosts = egressOwnedHosts(chart, fc.Graph)
	}
	merged := map[string]map[egressPort]bool{}
	for _, h := range hosts {
		merged[h.Host] = map[egressPort]bool{}
		for _, p := range h.Ports {
			merged[h.Host][p] = true
		}
	}
	for host, ports := range explicit {
		if merged[host] == nil {
			merged[host] = map[egressPort]bool{}
		}
		for p := range ports {
			merged[host][p] = true
		}
	}

	entries := make([]interface{}, 0, len(merged))
	for _, h := range egressSortHosts(merged) {
		ports := make([]interface{}, 0, len(h.Ports))
		for _, p := range h.Ports {
			ports = append(ports, map[string]interface{}{
				"number":   p.Number,
				"name":     fmt.Sprintf("%s-%d", strings.ToLower(p.Protocol), p.Number),
				"protocol": p.Protocol,
			})
		}
		entry := map[string]interface{}{"host": h.Host, "ports": ports}
		if strings.HasPrefix(h.Host, "*.") {
			entry["resolution"] = "NONE"
		}
		entries = append(entries, entry)
	}
	exportTo := make([]interface{}, 0)
	for _, e := range fc.ListParam("export-to") {
		exportTo = append(exportTo, e)
	}

	out := cloneChart(chart)
	if err := secAddValues(out, "istioEgress",
		"# Istio ServiceEntries for external hosts (dhg feature: istio-egress).\n"+
			"# Each entry: host, ports (number/name/protocol), optional resolution (default DNS).\n",
		map[string]interface{}{
			"enabled":        true,
			"exportTo":       exportTo,
			"serviceEntries": entries,
		}); err != nil {
		return nil, err
	}
	if err := secAddTemplate(out, "templates/istio-egress.yaml", istioEgressTemplate(newSecChartHelpers(chart), apiVersion)); err != nil {
		return nil, err
	}
	return out, nil
}

func istioEgressTemplate(h secChartHelpers, apiVersion string) string {
	return `{{- /* Generated by dhg feature "istio-egress". Requires Istio. */}}
{{- $eg := .Values.istioEgress }}
{{- if and $eg $eg.enabled }}
{{- range $eg.serviceEntries }}
---
apiVersion: ` + apiVersion + `
kind: ServiceEntry
metadata:
  name: ` + h.name(`(printf "egress-%s" (.host | replace "*." "wildcard-" | replace "." "-"))`) + `
  namespace: {{ $.Release.Namespace }}
` + h.labels(4) + `spec:
  hosts:
    - {{ .host | quote }}
  {{- with $eg.exportTo }}
  exportTo:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  location: MESH_EXTERNAL
  resolution: {{ .resolution | default "DNS" }}
  ports:
    {{- toYaml .ports | nindent 4 }}
{{- end }}
{{- end }}
`
}
