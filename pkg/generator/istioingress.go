package generator

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// Istio ingress (`dhg generate --with istio-ingress`).
//
// Istio serves a plain Kubernetes Ingress with `ingressClassName: istio` (or
// the deprecated `kubernetes.io/ingress.class: istio` annotation), but only
// the Ingress spec: controller annotations (ingress-nginx `ssl-redirect`,
// `rewrite-target`, ...) are not interpreted, there is no HTTP→HTTPS
// redirect, and the TLS secret must live in the namespace of the ingress
// gateway deployment. The Istio documentation recommends a Gateway instead
// of Ingress for the full feature set
// (istio.io/latest/docs/tasks/traffic-management/ingress/kubernetes-ingress/).
//
// For every Ingress of the chart this feature adds one template with an
// Istio Gateway and one VirtualService per host (networking.istio.io/v1,
// served since Istio 1.22), built from the input Ingress:
//   - hosts and paths: Exact → `exact`; Prefix → `exact: /p` or
//     `prefix: /p/` (Kubernetes Prefix matches whole path elements, an Istio
//     prefix matches characters); ImplementationSpecific → `prefix` (what
//     ingress-nginx does; noted in the template). Routes are ordered as the
//     Ingress spec ranks them (Exact first, then longer paths), because
//     Istio uses the first matching route;
//   - backends → route.destination.host/port.number; a named port is
//     resolved from the Service in the input, else the port is omitted
//     (valid when the Service has a single port; noted);
//   - spec.tls → an HTTPS server with credentialName = secretName and an
//     HTTP server with httpsRedirect for the same hosts.
//
// The derived configuration is written to values (istioIngress.ingresses)
// and rendered from there, so it can be adjusted without editing templates;
// it does not follow later edits of the Ingress values. Toggles:
// istioIngress.enabled renders the Istio resources, istioIngress.replaceIngress
// additionally stops rendering the Ingress (default false: the chart keeps
// reproducing its input).

// istioIngressValues is the per-Ingress values block.
type istioIngressValues struct {
	Servers         []interface{}
	VirtualServices []interface{}
	// notes are rendered as comments in the template, not stored in values.
	notes []string
}

// istioPath is one path of an Ingress rule.
type istioPath struct {
	path, pathType string
	backend        map[string]interface{}
	index          int // position in the input, for stable ordering
}

// parseGatewaySelector parses "k=v,k2=v2".
func parseGatewaySelector(raw string) (map[string]interface{}, error) {
	out := map[string]interface{}{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		k, v, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("gateway-selector must be a list of key=value labels; got %q", raw)
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out, nil
}

// istioPathMatch converts an Ingress path to Istio URI matches.
func istioPathMatch(path, pathType string) (matches []interface{}, note string) {
	uri := func(kind, value string) interface{} {
		return map[string]interface{}{"uri": map[string]interface{}{kind: value}}
	}
	if path == "" {
		path = "/"
	}
	switch pathType {
	case "Exact":
		return []interface{}{uri("exact", path)}, ""
	case "ImplementationSpecific":
		return []interface{}{uri("prefix", path)},
			fmt.Sprintf("path %s (ImplementationSpecific) is matched as a character prefix, as ingress-nginx does; check it against the original controller", path)
	default: // Prefix (the generated Ingress template defaults to it too)
		p := strings.TrimRight(path, "/")
		if p == "" {
			return []interface{}{uri("prefix", "/")}, ""
		}
		return []interface{}{uri("exact", p), uri("prefix", p+"/")}, ""
	}
}

// sortIstioPaths orders paths the way the Ingress spec ranks matches: Exact
// before the other types, then longer paths first; input order breaks ties.
func sortIstioPaths(paths []istioPath) {
	sort.SliceStable(paths, func(i, j int) bool {
		ei, ej := paths[i].pathType == "Exact", paths[j].pathType == "Exact"
		if ei != ej {
			return ei
		}
		li, lj := len(strings.TrimRight(paths[i].path, "/")), len(strings.TrimRight(paths[j].path, "/"))
		if li != lj {
			return li > lj
		}
		return paths[i].index < paths[j].index
	})
}

// istioServicePort returns the number of the named port of a Service in the graph.
func istioServicePort(graph *types.ResourceGraph, namespace, service, portName string) (int64, bool) {
	if graph == nil {
		return 0, false
	}
	for _, r := range opsSortedResources(graph) {
		obj := r.Original.Object
		if r.Original.GVK.Kind != "Service" || obj.GetName() != service || obj.GetNamespace() != namespace {
			continue
		}
		ports, _, _ := unstructured.NestedSlice(obj.Object, "spec", "ports")
		for _, p := range ports {
			pm, _ := p.(map[string]interface{})
			if name, _ := pm["name"].(string); name == portName {
				if n, ok := intValue(pm["port"]); ok {
					return n, true
				}
			}
		}
	}
	return 0, false
}

// istioRoute converts an Ingress backend to an Istio HTTP route destination.
func istioRoute(graph *types.ResourceGraph, namespace string, backend map[string]interface{}) (interface{}, string) {
	svc, _ := backend["service"].(map[string]interface{})
	name, _ := svc["name"].(string)
	if name == "" {
		return nil, "a resource backend has no Istio equivalent and is not routed"
	}
	dest := map[string]interface{}{"host": name}
	note := ""
	port, _ := svc["port"].(map[string]interface{})
	if n, ok := intValue(port["number"]); ok {
		dest["port"] = map[string]interface{}{"number": n}
	} else if portName, _ := port["name"].(string); portName != "" {
		if n, ok := istioServicePort(graph, namespace, name, portName); ok {
			dest["port"] = map[string]interface{}{"number": n}
		} else {
			note = fmt.Sprintf("port %q of Service %s is not in the input: destination.port is omitted, which Istio accepts only when the Service has a single port", portName, name)
		}
	}
	return []interface{}{map[string]interface{}{"destination": dest}}, note
}

// hostCovered reports whether host is served by one of the TLS hosts
// (exactly, or by a single-label wildcard as in the Ingress spec).
func hostCovered(host string, tlsHosts []string) bool {
	for _, t := range tlsHosts {
		if t == host || t == "*" {
			return true
		}
		if strings.HasPrefix(t, "*.") && strings.HasSuffix(host, t[1:]) &&
			!strings.Contains(strings.TrimSuffix(host, t[1:]), ".") {
			return true
		}
	}
	return false
}

func stringList(v interface{}) []string {
	list, _ := v.([]interface{})
	var out []string
	for _, item := range list {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func toInterfaces(list []string) []interface{} {
	out := make([]interface{}, len(list))
	for i, s := range list {
		out[i] = s
	}
	return out
}

// buildIstioIngress derives the Gateway servers and VirtualServices of one
// Ingress. vsName is the base name of the VirtualServices.
func buildIstioIngress(ing *unstructured.Unstructured, graph *types.ResourceGraph, vsName string, httpsRedirect bool) istioIngressValues {
	var v istioIngressValues
	ns := ing.GetNamespace()

	var annotations []string
	for k := range ing.GetAnnotations() {
		if k != "kubernetes.io/ingress.class" {
			annotations = append(annotations, k)
		}
	}
	sort.Strings(annotations)
	if len(annotations) > 0 {
		v.notes = append(v.notes, "Ingress annotations are not translated: "+strings.Join(annotations, ", "))
	}

	// Rules, merged by host in order of appearance ("" = every host).
	var hosts []string
	paths := map[string][]istioPath{}
	rules, _, _ := unstructured.NestedSlice(ing.Object, "spec", "rules")
	n := 0
	for _, r := range rules {
		rule, _ := r.(map[string]interface{})
		if rule == nil {
			continue
		}
		host, _ := rule["host"].(string)
		if host == "" {
			host = "*"
		}
		if _, seen := paths[host]; !seen {
			hosts = append(hosts, host)
			paths[host] = nil
		}
		list, _, _ := unstructured.NestedSlice(rule, "http", "paths")
		for _, p := range list {
			pm, _ := p.(map[string]interface{})
			if pm == nil {
				continue
			}
			path, _ := pm["path"].(string)
			pathType, _ := pm["pathType"].(string)
			backend, _ := pm["backend"].(map[string]interface{})
			paths[host] = append(paths[host], istioPath{path: path, pathType: pathType, backend: backend, index: n})
			n++
		}
	}
	defaultBackend, hasDefault, _ := unstructured.NestedMap(ing.Object, "spec", "defaultBackend")
	if len(hosts) == 0 && hasDefault {
		hosts = []string{"*"}
	}
	if hasDefault {
		v.notes = append(v.notes, "spec.defaultBackend is routed for the hosts of the rules only")
	}

	// TLS: one HTTPS server per entry, plus an HTTP server redirecting it.
	var tlsHosts []string
	tls, _, _ := unstructured.NestedSlice(ing.Object, "spec", "tls")
	secretNote := false
	for i, t := range tls {
		tm, _ := t.(map[string]interface{})
		secret, _ := tm["secretName"].(string)
		th := stringList(tm["hosts"])
		if len(th) == 0 {
			th = []string{"*"}
		}
		if secret == "" {
			v.notes = append(v.notes, fmt.Sprintf("spec.tls[%d] has no secretName (the controller's default certificate): no HTTPS server is generated for %s", i, strings.Join(th, ", ")))
			continue
		}
		secretNote = true
		tlsHosts = append(tlsHosts, th...)
		v.Servers = append(v.Servers, map[string]interface{}{
			"port":  map[string]interface{}{"number": 443, "name": fmt.Sprintf("https-%d", i), "protocol": "HTTPS"},
			"hosts": toInterfaces(th),
			"tls":   map[string]interface{}{"mode": "SIMPLE", "credentialName": secret},
		})
		redirect := map[string]interface{}{
			"port":  map[string]interface{}{"number": 80, "name": fmt.Sprintf("http-%d", i), "protocol": "HTTP"},
			"hosts": toInterfaces(th),
		}
		if httpsRedirect {
			redirect["tls"] = map[string]interface{}{"httpsRedirect": true}
		}
		v.Servers = append(v.Servers, redirect)
	}
	if secretNote {
		v.notes = append(v.notes, "credentialName secrets must exist in the namespace of the ingress gateway deployment "+
			"(usually istio-system), not in the release namespace")
	}
	var plain []string
	for _, h := range hosts {
		if !hostCovered(h, tlsHosts) {
			plain = append(plain, h)
		}
	}
	if len(plain) > 0 || len(v.Servers) == 0 {
		if len(plain) == 0 {
			plain = []string{"*"}
		}
		v.Servers = append(v.Servers, map[string]interface{}{
			"port":  map[string]interface{}{"number": 80, "name": "http", "protocol": "HTTP"},
			"hosts": toInterfaces(plain),
		})
	}

	// One VirtualService per host: a VirtualService has one route list for
	// all of its hosts, while Ingress rules give each host its own paths.
	for i, host := range hosts {
		hp := append([]istioPath(nil), paths[host]...)
		sortIstioPaths(hp)
		var http []interface{}
		for _, p := range hp {
			route, note := istioRoute(graph, ns, p.backend)
			if note != "" {
				v.notes = append(v.notes, note)
			}
			if route == nil {
				continue
			}
			match, note := istioPathMatch(p.path, p.pathType)
			if note != "" {
				v.notes = append(v.notes, note)
			}
			http = append(http, map[string]interface{}{"match": match, "route": route})
		}
		if hasDefault {
			route, note := istioRoute(graph, ns, defaultBackend)
			if note != "" {
				v.notes = append(v.notes, note)
			}
			if route != nil {
				http = append(http, map[string]interface{}{"route": route})
			}
		}
		if len(http) == 0 {
			continue // nothing routable for this host
		}
		name := vsName
		if len(hosts) > 1 {
			name = fmt.Sprintf("%s-%d", vsName, i)
		}
		v.VirtualServices = append(v.VirtualServices, map[string]interface{}{
			"name":  name,
			"hosts": []interface{}{host},
			"http":  http,
		})
	}
	v.notes = uniqueStrings(v.notes)
	return v
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// istioIngressGate returns the wrapper lines of an Ingress template that
// follow the service (`$svc := ...`, `if $svc.enabled`), without the lines
// that select and toggle the Ingress itself (`with $svc.ingress`, `if
// .enabled`): the Istio resources do not depend on the Ingress being
// rendered. It also returns the matching number of closing lines.
func istioIngressGate(rt *resourceTemplate) ([]string, int) {
	var gate []string
	opened := 0
	for _, l := range rt.prefix {
		if reWith.MatchString(l) {
			break
		}
		if reOpener.MatchString(l) {
			opened++
		}
		gate = append(gate, l)
	}
	return gate, opened
}

// istioIngressTemplate renders the Gateway and VirtualServices of one Ingress
// from istioIngress.ingresses.<key>.
func istioIngressTemplate(rt *resourceTemplate, key string, notes []string) string {
	labels := rt.labels()
	// The namespace line of the Ingress is reused: objects that keep their
	// input namespace (ADR-056) keep it here too.
	namespace := "  namespace: {{ $.Release.Namespace }}"
	for _, l := range rt.block(rt.topLevel("metadata:")) {
		if strings.HasPrefix(l, "  namespace: ") {
			namespace = l
		}
	}
	meta := func(nameExpr string) []string {
		return append([]string{"metadata:", "  name: " + nameExpr, namespace}, labels...)
	}
	gate, opened := istioIngressGate(rt)

	var b []string
	b = append(b, "{{- if $.Values.istioIngress.enabled }}")
	b = append(b, gate...)
	b = append(b, fmt.Sprintf("{{- with index $.Values.istioIngress.ingresses %q }}", key))
	b = append(b, "apiVersion: networking.istio.io/v1", "kind: Gateway")
	b = append(b, meta(rt.name)...)
	b = append(b, "spec:")
	b = append(b, "  # Generated by dhg from Ingress "+key+" (values: istioIngress.ingresses).")
	for _, n := range notes {
		b = append(b, "  # Note: "+n)
	}
	b = append(b,
		"  selector:",
		"    {{- toYaml $.Values.istioIngress.gatewaySelector | nindent 4 }}",
		"  servers:",
		"    {{- toYaml .servers | nindent 4 }}",
		"{{- range .virtualServices }}",
		"---",
		"apiVersion: networking.istio.io/v1",
		"kind: VirtualService",
	)
	b = append(b, meta("{{ .name }}")...)
	b = append(b,
		"spec:",
		"  hosts:",
		"    {{- toYaml .hosts | nindent 4 }}",
		"  gateways:",
		"    - "+rt.name,
		"  http:",
		"    {{- toYaml .http | nindent 4 }}",
		"{{- end }}",
		"{{- end }}",
	)
	for i := 0; i < opened; i++ {
		b = append(b, "{{- end }}")
	}
	b = append(b, "{{- end }}")
	return strings.Join(b, "\n") + "\n"
}

// istioIngressReplaceGuard is the condition under which the Ingress itself
// is still rendered.
const istioIngressReplaceGuard = "{{- if not (and $.Values.istioIngress.enabled $.Values.istioIngress.replaceIngress) }}"

func applyIstioIngressFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	selector, err := parseGatewaySelector(fc.Param("gateway-selector"))
	if err != nil {
		return nil, err
	}
	ingresses := resourceTemplates(chart, isKind("Ingress"))
	if len(ingresses) == 0 {
		return chart, nil
	}
	out := cloneChart(chart)
	perIngress := map[string]interface{}{}
	for _, rt := range ingresses {
		if labels := rt.labels(); labels != nil && !balancedControl(labels[1:]) {
			continue
		}
		r := graphResourceFor(fc.Graph, rt)
		if r == nil {
			continue // not generated from an input Ingress: nothing to derive from
		}
		ing := r.Original.Object
		key := ing.GetName()
		if _, dup := perIngress[key]; dup {
			key = ing.GetName() + "-" + ing.GetNamespace()
		}
		v := buildIstioIngress(ing, fc.Graph, key+"-ingress", fc.BoolParam("https-redirect"))
		perIngress[key] = map[string]interface{}{"servers": v.Servers, "virtualServices": v.VirtualServices}

		path := featureTemplatePath("istio-ingress", rt.path)
		if err := addTemplate(out, path, istioIngressTemplate(rt, key, v.notes)); err != nil {
			return nil, err
		}
		out.Templates[rt.path] = istioIngressReplaceGuard + "\n" + rt.render() + "{{- end }}\n"
	}
	if len(perIngress) == 0 {
		return chart, nil
	}
	if err := addFeatureValues(out, "istioIngress", map[string]interface{}{
		"enabled":         true,
		"replaceIngress":  fc.BoolParam("replace-ingress"),
		"gatewaySelector": selector,
		"ingresses":       perIngress,
	}); err != nil {
		return nil, err
	}
	return out, nil
}
