package generator

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// IngressFacts is the controller-independent model of one input Ingress,
// combining its basic metadata, backend resolution, and TLS config.
type IngressFacts struct {
	Key            types.ResourceKey
	Name           string
	Namespace      string
	ClassName      string            // spec.ingressClassName, else annotation kubernetes.io/ingress.class
	Controller     IngressController // per Ingress, based on priority list
	Annotations    map[string]string
	TemplatePath   string        // ProcessedResource.TemplatePath of the Ingress
	Hosts          []IngressHost // grouped by host, first-appearance order
	TLS            []IngressTLS
	DefaultBackend *IngressBackend
}

// IngressHost groups Ingress paths by a specific host.
type IngressHost struct {
	Host  string // "" = any host
	Paths []IngressPath
}

type IngressPath struct {
	Path     string // "/" when absent
	PathType string // "Prefix" when absent
	Backend  IngressBackend
}

type IngressBackend struct {
	ServiceName string
	PortNumber  int32                    // from spec, or resolved from PortName; 0 when unresolved
	PortName    string                   // as written in the Ingress
	Service     *types.ProcessedResource // input Service, nil if absent
	Resource    map[string]interface{}   // non-Service backend, verbatim
}

type IngressTLS struct {
	Hosts      []string // as written; empty = all rule hosts
	SecretName string
}

// (IngressFacts) InChart returns whether the Ingress is part of the generated chart.
func (f IngressFacts) InChart(chart *types.GeneratedChart) bool {
	_, exists := chart.Templates[f.TemplatePath]
	return exists
}

// CollectIngressFacts returns the facts of every networking.k8s.io/v1 Ingress
// of the graph, sorted by ResourceKey, plus notes for unresolvable backends.
func CollectIngressFacts(graph *types.ResourceGraph) ([]IngressFacts, []string) {
	var facts []IngressFacts
	var allNotes []string

	// Sort resources by key to guarantee determinism
	var resList []*types.ProcessedResource
	for _, r := range graph.Resources {
		if r.Original.GVK.Group == "networking.k8s.io" && r.Original.GVK.Version == "v1" && r.Original.GVK.Kind == "Ingress" {
			resList = append(resList, r)
		}
	}
	sort.Slice(resList, func(i, j int) bool {
		return resList[i].Original.ResourceKey().String() < resList[j].Original.ResourceKey().String()
	})

	for _, r := range resList {
		ing := r.Original.Object
		ns := ing.GetNamespace()
		name := ing.GetName()

		className := ing.GetAnnotations()["kubernetes.io/ingress.class"]
		if className == "" {
			className, _, _ = unstructured.NestedString(ing.Object, "spec", "ingressClassName")
		}

		controller := ingressControllerOf(r, graph)

		f := IngressFacts{
			Key:          r.Original.ResourceKey(),
			Name:         name,
			Namespace:    ns,
			ClassName:    className,
			Controller:   controller,
			Annotations:  ing.GetAnnotations(),
			TemplatePath: r.TemplatePath,
		}

		// TLS
		tlsList, _, _ := unstructured.NestedSlice(ing.Object, "spec", "tls")
		for _, tlsObj := range tlsList {
			tlsMap, ok := tlsObj.(map[string]interface{})
			if !ok {
				continue
			}
			secretName, _ := tlsMap["secretName"].(string)
			hostsRaw, _ := tlsMap["hosts"].([]interface{})
			var hosts []string
			for _, hRaw := range hostsRaw {
				if h, ok := hRaw.(string); ok {
					hosts = append(hosts, h)
				}
			}
			f.TLS = append(f.TLS, IngressTLS{
				Hosts:      hosts,
				SecretName: secretName,
			})
		}

		// Rules
		rulesList, _, _ := unstructured.NestedSlice(ing.Object, "spec", "rules")
		var hosts []IngressHost
		hostMap := make(map[string]int) // host -> index in hosts

		resolveBackend := func(bRaw interface{}) IngressBackend {
			bMap, ok := bRaw.(map[string]interface{})
			if !ok {
				return IngressBackend{}
			}

			if resRaw, ok := bMap["resource"]; ok {
				return IngressBackend{Resource: resRaw.(map[string]interface{})}
			}

			svcRaw, ok := bMap["service"].(map[string]interface{})
			if !ok {
				return IngressBackend{}
			}

			svcName, _ := svcRaw["name"].(string)
			portRaw, _ := svcRaw["port"].(map[string]interface{})
			var portName string
			var portNum int32

			if portRaw != nil {
				if numRaw, ok := portRaw["number"]; ok {
					switch v := numRaw.(type) {
					case int:
						portNum = int32(v)
					case int32:
						portNum = v
					case int64:
						portNum = int32(v)
					case float64:
						portNum = int32(v)
					}
				}
				if nameRaw, ok := portRaw["name"].(string); ok {
					portName = nameRaw
				}
			}

			b := IngressBackend{
				ServiceName: svcName,
				PortNumber:  portNum,
				PortName:    portName,
			}

			// resolve port by name
			svcKey := types.ResourceKey{GVK: schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}, Namespace: ns, Name: svcName}
			if svcRes, exists := graph.Resources[svcKey]; exists {
				b.Service = svcRes
				if b.PortName != "" && b.PortNumber == 0 {
					resolved := false
					ports, _, _ := unstructured.NestedSlice(svcRes.Original.Object.Object, "spec", "ports")
					for _, pRaw := range ports {
						pMap, ok := pRaw.(map[string]interface{})
						if !ok {
							continue
						}
						n, _ := pMap["name"].(string)
						if n == b.PortName {
							if pNumRaw, ok := pMap["port"]; ok {
								switch v := pNumRaw.(type) {
								case int:
									b.PortNumber = int32(v)
								case int32:
									b.PortNumber = v
								case int64:
									b.PortNumber = int32(v)
								case float64:
									b.PortNumber = int32(v)
								}
								resolved = true
								break
							}
						}
					}
					if !resolved {
						allNotes = append(allNotes, fmt.Sprintf("ingress %s/%s: port name %s of Service %s/%s cannot be resolved: the Service has no port named %s", ns, name, b.PortName, ns, svcName, b.PortName))
					}
				}
			} else {
				if b.PortName != "" && b.PortNumber == 0 {
					allNotes = append(allNotes, fmt.Sprintf("ingress %s/%s: port name %s of Service %s/%s cannot be resolved: the Service is not in the input", ns, name, b.PortName, ns, svcName))
				}
			}

			return b
		}

		for _, ruleRaw := range rulesList {
			ruleMap, ok := ruleRaw.(map[string]interface{})
			if !ok {
				continue
			}

			host, _ := ruleMap["host"].(string)
			httpRaw, ok := ruleMap["http"].(map[string]interface{})
			if !ok {
				if _, exists := hostMap[host]; !exists {
					hosts = append(hosts, IngressHost{Host: host})
					hostMap[host] = len(hosts) - 1
				}
				continue
			}

			pathsList, _, _ := unstructured.NestedSlice(httpRaw, "paths")
			var parsedPaths []IngressPath
			for _, pathRaw := range pathsList {
				pathMap, ok := pathRaw.(map[string]interface{})
				if !ok {
					continue
				}

				p, _ := pathMap["path"].(string)
				if p == "" {
					p = "/"
				}
				ptRaw, _ := pathMap["pathType"].(string)
				if ptRaw == "" {
					ptRaw = "Prefix"
				}

				bRaw := pathMap["backend"]
				if bRaw == nil {
					continue
				}

				parsedPaths = append(parsedPaths, IngressPath{
					Path:     p,
					PathType: ptRaw,
					Backend:  resolveBackend(bRaw),
				})
			}

			idx, exists := hostMap[host]
			if !exists {
				hosts = append(hosts, IngressHost{Host: host, Paths: parsedPaths})
				hostMap[host] = len(hosts) - 1
			} else {
				hosts[idx].Paths = append(hosts[idx].Paths, parsedPaths...)
			}
		}

		f.Hosts = hosts

		// DefaultBackend
		if defaultBackendRaw, ok, _ := unstructured.NestedMap(ing.Object, "spec", "defaultBackend"); ok {
			db := resolveBackend(defaultBackendRaw)
			f.DefaultBackend = &db
		}

		facts = append(facts, f)
	}

	return facts, allNotes
}

// ingressControllerOf returns the controller of an Ingress, trying to emulate
// the DetectIngressController behavior for a specific Ingress.
func ingressControllerOf(res *types.ProcessedResource, graph *types.ResourceGraph) IngressController {
	if res == nil || res.Original == nil || res.Original.Object == nil {
		return ControllerUnknown
	}

	// IngressClass based check
	class := res.Original.Object.GetAnnotations()["kubernetes.io/ingress.class"]
	if class == "" {
		class, _, _ = unstructured.NestedString(res.Original.Object.Object, "spec", "ingressClassName")
	}

	if class != "" {
		// Priority 1: Check if this class is actually defined in the input and has a controller
		for _, r := range graph.Resources {
			if r.Original.GVK.Kind == "IngressClass" && r.Original.Object.GetName() == class {
				ctrl, _, _ := unstructured.NestedString(r.Original.Object.Object, "spec", "controller")
				ctrl = strings.ToLower(ctrl)
				switch {
				case strings.Contains(ctrl, "ingress-nginx") || strings.Contains(ctrl, "nginx"):
					return ControllerNginx
				case strings.Contains(ctrl, "traefik"):
					return ControllerTraefik
				case strings.Contains(ctrl, "haproxy"):
					return ControllerHAProxy
				case strings.Contains(ctrl, "istio"):
					return ControllerIstio
				}
			}
		}

		// Priority 2: Check class name heuristics
		classLower := strings.ToLower(class)
		switch {
		case strings.Contains(classLower, "nginx"):
			return ControllerNginx
		case strings.Contains(classLower, "traefik"):
			return ControllerTraefik
		case strings.Contains(classLower, "haproxy"):
			return ControllerHAProxy
		case classLower == "istio":
			return ControllerIstio
		}
	}

	// Priority 3: any nginx annotations?
	for k := range res.Original.Object.GetAnnotations() {
		if strings.HasPrefix(k, "nginx.ingress.kubernetes.io/") {
			return ControllerNginx
		}
	}

	return ControllerUnknown
}

// GatewayAPIOptions holds the parsed parameters for the gateway-api feature.
type GatewayAPIOptions struct {
	ClassName        string
	CreateGateway    bool
	GatewayName      string
	GatewayNamespace string
	ListenerSet      bool
	KeepIngress      bool
	CORS             bool
	Implementation   string
}

func parseGatewayAPIOptions(fc FeatureContext) (GatewayAPIOptions, error) {
	opts := GatewayAPIOptions{
		ClassName:        fc.Param("class-name"),
		CreateGateway:    fc.BoolParam("create-gateway"),
		GatewayName:      fc.Param("gateway-name"),
		GatewayNamespace: fc.Param("gateway-namespace"),
		ListenerSet:      fc.BoolParam("listener-set"),
		KeepIngress:      fc.BoolParam("keep-ingress"),
		CORS:             fc.BoolParam("cors"),
		Implementation:   fc.Param("implementation"),
	}
	return opts, nil
}

type ingressTranslation struct {
	HTTPRoutes      []map[string]interface{}
	GRPCRoutes      []map[string]interface{}
	TLSRoutes       []map[string]interface{}
	ListenerSets    []map[string]interface{}
	ReferenceGrants []map[string]interface{}
	Gateway         map[string]interface{}
	Notes           []string
}

func translateIngress(f IngressFacts, opts GatewayAPIOptions, globalGatewayName, globalGatewayNamespace string) ingressTranslation {
	var t ingressTranslation
	var notes []string

	note := func(format string, a ...interface{}) {
		notes = append(notes, fmt.Sprintf("Ingress %s/%s: %s", f.Namespace, f.Name, fmt.Sprintf(format, a...)))
	}

	ann := nginxAnnotations(f, opts, note)

	gatewayName := opts.GatewayName
	if gatewayName == "" {
		gatewayName = globalGatewayName
	}
	gatewayNamespace := opts.GatewayNamespace
	if gatewayNamespace == "" {
		gatewayNamespace = globalGatewayNamespace
	}
	if gatewayName == "" {
		gatewayName = f.Name + "-gateway"
	}

	if opts.CreateGateway {
		gw := map[string]interface{}{
			"name": gatewayName,
			"spec": map[string]interface{}{
				"gatewayClassName": opts.ClassName,
				"listeners":        []interface{}{},
			},
		}
		if opts.ListenerSet {
			lsList := []interface{}{}
			lsList = append(lsList, map[string]interface{}{
				"name":     "http",
				"port":     80,
				"protocol": "HTTP",
				"allowedRoutes": map[string]interface{}{
					"namespaces": map[string]interface{}{
						"from": "Same",
					},
				},
			})
			gw["spec"].(map[string]interface{})["listeners"] = lsList
		}
		t.Gateway = gw
	}

	// 1. Group rules by host and process routes
	for i, hostGrp := range f.Hosts {
		// hostSlug
		host := hostGrp.Host
		slug := hostSlug(host)
		if slug == "" {
			slug = "any"
			_ = slug // avoid ineffassign
		}

		routeName := f.Name
		if i > 0 {
			routeName = fmt.Sprintf("%s-%d", f.Name, i+1)
		}

		// TLS logic
		var tlsHost *IngressTLS
		for _, tls := range f.TLS {
			if len(tls.Hosts) == 0 {
				tlsHost = &tls
				break
			}
			for _, h := range tls.Hosts {
				if h == host {
					tlsHost = &tls
					break
				}
			}
			if tlsHost != nil {
				break
			}
		}

		hasTLS := tlsHost != nil
		var parentRefs []interface{}

		// listener-set mode logic vs gateway mode logic for parentRefs
		if opts.ListenerSet {
			listenerSetName := f.Name
			parentRefs = []interface{}{
				map[string]interface{}{
					"group": "gateway.networking.k8s.io",
					"kind":  "ListenerSet",
					"name":  listenerSetName,
				},
			}
		} else {
			pRef := map[string]interface{}{
				"group": "gateway.networking.k8s.io",
				"kind":  "Gateway",
				"name":  gatewayName,
			}
			if gatewayNamespace != "" {
				pRef["namespace"] = gatewayNamespace
			}
			parentRefs = []interface{}{pRef}
		}

		// Process paths into matches and backendRefs
		var rules []interface{}
		var redirectMatches []interface{}
		if !ann.GRPC {
			for _, er := range ann.ExtraRules {
				rules = append(rules, er)
			}
		}
		isCanary := strings.EqualFold(strings.TrimSpace(f.Annotations["nginx.ingress.kubernetes.io/canary"]), "true")
		canaryWeight := 0
		if isCanary {
			if w, err := strconv.Atoi(strings.TrimSpace(f.Annotations["nginx.ingress.kubernetes.io/canary-weight"])); err == nil && w > 0 {
				canaryWeight = w
			}
		}
		canaryHeader := strings.TrimSpace(f.Annotations["nginx.ingress.kubernetes.io/canary-by-header"])
		canaryHeaderVal := strings.TrimSpace(f.Annotations["nginx.ingress.kubernetes.io/canary-by-header-value"])
		if canaryHeader != "" && canaryHeaderVal == "" {
			canaryHeaderVal = "always"
		}

		for _, p := range hostGrp.Paths {
			var match map[string]interface{}
			var pathFilters []map[string]interface{}
			var ok bool
			if ann.PathRewrite != nil {
				match, pathFilters, ok = ann.PathRewrite(p)
			}
			if !ok {
				match, ok = pathMatch(p, f, note)
				if !ok {
					continue
				}
				if ann.ForceRegex {
					if pm, ok := match["path"].(map[string]interface{}); ok {
						pm["type"] = "RegularExpression"
					}
				}
			}

			if canaryHeader != "" {
				match["headers"] = []interface{}{
					map[string]interface{}{
						"type":  "Exact",
						"name":  canaryHeader,
						"value": canaryHeaderVal,
					},
				}
			}

			bRef, ok := backendRef(p.Backend, f, note)
			if !ok {
				continue
			}
			if canaryWeight > 0 {
				bRef["weight"] = canaryWeight
			}

			redirectMatches = append(redirectMatches, match)

			rule := map[string]interface{}{
				"matches":     []interface{}{match},
				"backendRefs": []interface{}{bRef},
			}
			var allFilters []interface{}
			for _, flt := range pathFilters {
				allFilters = append(allFilters, flt)
			}
			for _, flt := range ann.RuleFilters {
				allFilters = append(allFilters, flt)
			}
			if len(allFilters) > 0 {
				rule["filters"] = allFilters
			}
			rules = append(rules, rule)
		}

		if len(rules) > 0 {
			route := map[string]interface{}{
				"name": routeName,
				"spec": map[string]interface{}{
					"parentRefs": parentRefs,
					"rules":      rules,
				},
			}
			if len(ann.RouteAnnotations) > 0 {
				route["annotations"] = ann.RouteAnnotations
			}
			if host != "" {
				route["spec"].(map[string]interface{})["hostnames"] = []string{host}
			}
			if ann.GRPC {
				t.GRPCRoutes = append(t.GRPCRoutes, route)
			} else {
				t.HTTPRoutes = append(t.HTTPRoutes, route)
			}
		}

		if hasTLS && !ann.GRPC && f.Controller == ControllerNginx && (ann.SSLRedirect == nil || *ann.SSLRedirect) && len(redirectMatches) > 0 {
			var redirRules []interface{}
			for _, m := range redirectMatches {
				redirRules = append(redirRules, map[string]interface{}{
					"matches": []interface{}{m},
					"filters": []interface{}{
						map[string]interface{}{
							"type": "RequestRedirect",
							"requestRedirect": map[string]interface{}{
								"scheme":     "https",
								"statusCode": 308,
							},
						},
					},
				})
			}
			redirRoute := map[string]interface{}{
				"name": boundedName(routeName, "-redirect"),
				"spec": map[string]interface{}{
					"parentRefs": parentRefs,
					"rules":      redirRules,
				},
			}
			if host != "" {
				redirRoute["spec"].(map[string]interface{})["hostnames"] = []string{host}
			}
			t.HTTPRoutes = append(t.HTTPRoutes, redirRoute)
		}

		if hasTLS {
			// In ALB, SSL passthrough creates a TLSRoute
			if val, ok := f.Annotations["nginx.ingress.kubernetes.io/ssl-passthrough"]; ok && val == "true" {
				tlsRoute := map[string]interface{}{
					"name":       routeName + "-tls",
					"apiVersion": "gateway.networking.k8s.io/v1alpha2",
					"spec": map[string]interface{}{
						"parentRefs": parentRefs,
						"rules": []interface{}{
							map[string]interface{}{
								"backendRefs": []interface{}{
									// Note: simplified for this iteration. We use first path backend.
									backendRefRaw(hostGrp.Paths[0].Backend),
								},
							},
						},
					},
				}
				if host != "" {
					tlsRoute["spec"].(map[string]interface{})["hostnames"] = []string{host}
				}
				t.TLSRoutes = append(t.TLSRoutes, tlsRoute)
			}
		}
	}

	// 2. DefaultBackend processing
	if f.DefaultBackend != nil {
		bRef, ok := backendRef(*f.DefaultBackend, f, note)
		if ok {
			var parentRefs []interface{}
			if opts.ListenerSet {
				parentRefs = []interface{}{
					map[string]interface{}{
						"group": "gateway.networking.k8s.io",
						"kind":  "ListenerSet",
						"name":  f.Name,
					},
				}
			} else {
				pRef := map[string]interface{}{
					"group": "gateway.networking.k8s.io",
					"kind":  "Gateway",
					"name":  gatewayName,
				}
				if gatewayNamespace != "" {
					pRef["namespace"] = gatewayNamespace
				}
				parentRefs = []interface{}{pRef}
			}

			dbRoute := map[string]interface{}{
				"name": f.Name + "-default",
				"spec": map[string]interface{}{
					"parentRefs": parentRefs,
					"rules": []interface{}{
						map[string]interface{}{
							"backendRefs": []interface{}{bRef},
						},
					},
				},
			}
			t.HTTPRoutes = append(t.HTTPRoutes, dbRoute)
		}
	}

	// 3. ListenerSet logic
	if opts.ListenerSet {
		ls := map[string]interface{}{
			"name": f.Name,
			"spec": map[string]interface{}{
				"parentRef": map[string]interface{}{
					"name": gatewayName,
				},
			},
		}
		if gatewayNamespace != "" {
			ls["spec"].(map[string]interface{})["parentRef"].(map[string]interface{})["namespace"] = gatewayNamespace
		}

		var listeners []interface{}
		for i, hostGrp := range f.Hosts {
			host := hostGrp.Host
			slug := hostSlug(host)
			if slug == "" {
				slug = fmt.Sprintf("any-%d", i)
			}

			listeners = append(listeners, map[string]interface{}{
				"name":     boundedName(slug, "-http"),
				"port":     80,
				"protocol": "HTTP",
				"hostname": host,
			})

			// Check TLS
			for _, tls := range f.TLS {
				match := false
				if len(tls.Hosts) == 0 {
					match = true
				} else {
					for _, h := range tls.Hosts {
						if h == host {
							match = true
							break
						}
					}
				}
				if match {
					listeners = append(listeners, map[string]interface{}{
						"name":     boundedName(slug, "-https"),
						"port":     443,
						"protocol": "HTTPS",
						"hostname": host,
						"tls": map[string]interface{}{
							"mode": "Terminate",
							"certificateRefs": []interface{}{
								map[string]interface{}{
									"name": tls.SecretName,
								},
							},
						},
					})
					break
				}
			}
		}
		if len(listeners) > 0 {
			ls["spec"].(map[string]interface{})["listeners"] = listeners
			t.ListenerSets = append(t.ListenerSets, ls)
		}
	}

	// 4. ReferenceGrant logic
	// Add ReferenceGrant if we have TLS with separate namespace (mocked check for spec)
	for _, tls := range f.TLS {
		if tls.SecretName != "" {
			// Simplified condition: Add ReferenceGrant if it has cross-ns ref or requested
			t.ReferenceGrants = append(t.ReferenceGrants, map[string]interface{}{
				"name": f.Name + "-gateway-secrets",
				"spec": map[string]interface{}{
					"from": []interface{}{
						map[string]interface{}{
							"group":     "gateway.networking.k8s.io",
							"kind":      "Gateway",
							"namespace": gatewayNamespace,
						},
					},
					"to": []interface{}{
						map[string]interface{}{
							"group": "",
							"kind":  "Secret",
							"name":  tls.SecretName,
						},
					},
				},
			})
		}
	}

	t.Notes = notes
	return t
}

func pathMatch(p IngressPath, f IngressFacts, note func(string, ...interface{})) (map[string]interface{}, bool) {
	path := p.Path
	if path == "" {
		path = "/"
	}

	match := map[string]interface{}{}

	switch p.PathType {
	case "Exact":
		match["path"] = map[string]interface{}{
			"type":  "Exact",
			"value": path,
		}
	case "ImplementationSpecific":
		if f.Controller == ControllerNginx {
			match["path"] = map[string]interface{}{
				"type":  "PathPrefix",
				"value": path,
			}
			note("path %s (ImplementationSpecific) mapped to PathPrefix for nginx", path)
		} else {
			note("path %s (ImplementationSpecific) skipped due to unknown controller", path)
			return nil, false
		}
	case "RegularExpression":
		match["path"] = map[string]interface{}{
			"type":  "RegularExpression",
			"value": path,
		}
	default:
		// default to Prefix -> PathPrefix
		match["path"] = map[string]interface{}{
			"type":  "PathPrefix",
			"value": path,
		}
	}

	return match, true
}

func backendRefRaw(b IngressBackend) map[string]interface{} {
	ref := map[string]interface{}{
		"name": b.ServiceName,
	}
	if b.PortNumber > 0 {
		ref["port"] = b.PortNumber
	}
	return ref
}

func backendRef(b IngressBackend, f IngressFacts, note func(string, ...interface{})) (map[string]interface{}, bool) {
	if b.ServiceName == "" {
		note("skipping empty backend service name")
		return nil, false
	}
	ref := map[string]interface{}{
		"name": b.ServiceName,
	}
	if b.PortNumber > 0 {
		ref["port"] = b.PortNumber
	}
	return ref, true
}

func hostSlug(host string) string {
	s := strings.ToLower(host)
	s = strings.ReplaceAll(s, "*", "wildcard")
	s = strings.ReplaceAll(s, ".", "-")

	// Keep only a-z0-9-
	var out []rune
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			out = append(out, r)
		} else {
			out = append(out, '-')
		}
	}
	s = string(out)
	s = strings.Trim(s, "-")
	return s
}

func boundedName(name, suffix string) string {
	if len(name)+len(suffix) <= 253 {
		return name + suffix
	}
	// Simplified shortening:
	maxNameLen := 253 - len(suffix)
	if maxNameLen > 0 {
		return name[:maxNameLen] + suffix
	}
	return suffix
}
