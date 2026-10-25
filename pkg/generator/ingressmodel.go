package generator

import (
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

type TLSMode string

const (
	TLSModeTerminate   TLSMode = "Terminate"
	TLSModePassthrough TLSMode = "Passthrough"
	TLSModeSimple      TLSMode = "Simple"
)

type PathMatchKind string

const (
	PathPrefix                 PathMatchKind = "PathPrefix"
	PathExact                  PathMatchKind = "Exact"
	PathRegex                  PathMatchKind = "RegularExpression"
	PathImplementationSpecific PathMatchKind = "ImplementationSpecific"
)

type HeaderMatch struct {
	Name  string
	Value string
	Regex bool
}

type BackendTarget struct {
	ServiceName string
	PortNumber  int64
	PortName    string
	Weight      int32
}

type RewritePolicy struct {
	ReplaceFullPath    string
	ReplacePrefixMatch string
	ReplaceHostname    string
}

type RedirectPolicy struct {
	Scheme          string
	StatusCode      int
	ReplaceFullPath string
}

type RetryPolicy struct {
}

type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           string
}

type PathMatch struct {
	Path     string
	PathType PathMatchKind
}

type RouteRule struct {
	Matches         []PathMatch
	Headers         []HeaderMatch
	BackendTargets  []BackendTarget
	RewritePolicy   *RewritePolicy
	RedirectPolicy  *RedirectPolicy
	CORSConfig      *CORSConfig
	ValuesPathIndex int
}

type TLSBinding struct {
	Hosts      []string
	SecretName string
	Mode       TLSMode
}

type VirtualHostSpec struct {
	Host  string
	Rules []RouteRule
}

type IngressSpec struct {
	SourceKey        string
	Namespace        string
	Name             string
	ServiceGroup     string
	IngressClassName string
	Hosts            []VirtualHostSpec
	TLS              []TLSBinding
	DefaultBackend   *BackendTarget
	Annotations      map[string]string
	ValuesPathPrefix string
	OriginalIndex    int
	Notes            []string
	TemplatePath     string
}

type IngressModel struct {
	Items    []IngressSpec
	Warnings []string
}



// ExtractIngressModel extracts common facts about Ingresses from the graph.
func ExtractIngressModel(graph *types.ResourceGraph) *IngressModel {
	model := &IngressModel{}
	var items []IngressSpec

	for _, r := range opsSortedResources(graph) {
		if r.Original.GVK.Kind != "Ingress" || r.Original.GVK.Group != "networking.k8s.io" || r.Original.GVK.Version != "v1" {
			continue
		}

		ing := r.Original.Object
		spec := IngressSpec{
			SourceKey:    ing.GetName(),
			Namespace:    ing.GetNamespace(),
			Name:         ing.GetName(),
			Annotations:  ing.GetAnnotations(),
			TemplatePath: r.TemplatePath,
		}

		// IngressClassName
		ic, _, _ := unstructured.NestedString(ing.Object, "spec", "ingressClassName")
		if ic != "" {
			spec.IngressClassName = ic
		} else {
			if annIC := spec.Annotations["kubernetes.io/ingress.class"]; annIC != "" {
				spec.IngressClassName = annIC
			}
		}

		// TLS
		tls, _, _ := unstructured.NestedSlice(ing.Object, "spec", "tls")
		for _, t := range tls {
			tm, _ := t.(map[string]interface{})
			secret, _ := tm["secretName"].(string)
			th := stringList(tm["hosts"])
			mode := TLSModeSimple
			if spec.Annotations["nginx.ingress.kubernetes.io/ssl-passthrough"] == "true" {
				mode = TLSModePassthrough
			}
			spec.TLS = append(spec.TLS, TLSBinding{
				Hosts:      th,
				SecretName: secret,
				Mode:       mode,
			})
		}

		// DefaultBackend
		defaultBackend, hasDefault, _ := unstructured.NestedMap(ing.Object, "spec", "defaultBackend")
		if hasDefault {
			spec.DefaultBackend = parseBackend(defaultBackend, graph, spec.Namespace)
		}

		// Hosts and Rules
		rules, _, _ := unstructured.NestedSlice(ing.Object, "spec", "rules")
		hostMap := map[string]*VirtualHostSpec{}
		var hostOrder []string

		for n, ruleIf := range rules {
			rule, _ := ruleIf.(map[string]interface{})
			if rule == nil {
				continue
			}
			host, _ := rule["host"].(string)
			if host == "" {
				host = "*"
			}

			if _, ok := hostMap[host]; !ok {
				hostOrder = append(hostOrder, host)
				hostMap[host] = &VirtualHostSpec{Host: host}
			}
			vhs := hostMap[host]

			list, _, _ := unstructured.NestedSlice(rule, "http", "paths")
			if len(list) == 0 {
				// Add an empty rule to indicate host exists but no paths
				vhs.Rules = append(vhs.Rules, RouteRule{ValuesPathIndex: n})
				continue
			}

			for _, pIf := range list {
				p, _ := pIf.(map[string]interface{})
				if p == nil {
					continue
				}
				path, _ := p["path"].(string)
				if path == "" {
					path = "/"
				}
				pathType, _ := p["pathType"].(string)
				if pathType == "" {
					pathType = "Prefix"
				}

				pt := PathPrefix
				switch pathType {
				case "Exact":
					pt = PathExact
				case "ImplementationSpecific":
					if spec.Annotations["nginx.ingress.kubernetes.io/use-regex"] == "true" {
						pt = PathRegex
					} else {
						pt = PathImplementationSpecific
					}
				case "Prefix":
					pt = PathPrefix
				}

				backend, _ := p["backend"].(map[string]interface{})
				target := parseBackend(backend, graph, spec.Namespace)

				var targets []BackendTarget
				if target != nil {
					targets = append(targets, *target)
				}

				vhs.Rules = append(vhs.Rules, RouteRule{
					Matches:         []PathMatch{{Path: path, PathType: pt}},
					BackendTargets:  targets,
					ValuesPathIndex: n,
				})
			}
		}

		for _, host := range hostOrder {
			spec.Hosts = append(spec.Hosts, *hostMap[host])
		}

		items = append(items, spec)
	}

	// Deterministic sorting
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Namespace != items[j].Namespace {
			return items[i].Namespace < items[j].Namespace
		}
		return items[i].Name < items[j].Name
	})

	for i := range items {
		sortHosts(items[i].Hosts)
		for j := range items[i].Hosts {
			sortRules(items[i].Hosts[j].Rules)
		}
	}

	model.Items = items
	return model
}

func parseBackend(backend map[string]interface{}, graph *types.ResourceGraph, namespace string) *BackendTarget {
	if backend == nil {
		return nil
	}
	svcIf, ok := backend["service"]
	if !ok {
		// resource backend
		return &BackendTarget{}
	}
	svc, _ := svcIf.(map[string]interface{})
	if svc == nil {
		return nil
	}
	svcName, _ := svc["name"].(string)

	target := &BackendTarget{
		ServiceName: svcName,
	}

	portIf, ok := svc["port"]
	if !ok {
		return target
	}
	port, _ := portIf.(map[string]interface{})
	if port == nil {
		return target
	}

	if nIf, ok := port["number"]; ok {
		if n, ok2 := intValue(nIf); ok2 {
			target.PortNumber = n
		} else if s, ok3 := nIf.(string); ok3 {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				target.PortNumber = n
			}
		}
	} else if nameIf, ok := port["name"]; ok {
		name, _ := nameIf.(string)
		target.PortName = name
		if n, resolved := resolveServicePort(graph, namespace, svcName, name); resolved {
			target.PortNumber = n
		}
	}

	return target
}

// resolveServicePort is similar to istioServicePort but for general model
func resolveServicePort(graph *types.ResourceGraph, namespace, service, portName string) (int64, bool) {
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

func sortHosts(hosts []VirtualHostSpec) {
	sort.SliceStable(hosts, func(i, j int) bool {
		hi, hj := hosts[i].Host, hosts[j].Host
		if hi == "*" && hj != "*" {
			return false
		}
		if hj == "*" && hi != "*" {
			return true
		}
		return hi < hj
	})
}

func sortRules(rules []RouteRule) {
	sort.SliceStable(rules, func(i, j int) bool {
		ri, rj := rules[i], rules[j]
		if len(ri.Matches) == 0 && len(rj.Matches) > 0 {
			return false
		}
		if len(rj.Matches) == 0 && len(ri.Matches) > 0 {
			return true
		}
		if len(ri.Matches) == 0 && len(rj.Matches) == 0 {
			return ri.ValuesPathIndex < rj.ValuesPathIndex
		}

		mi, mj := ri.Matches[0], rj.Matches[0]
		ei, ej := mi.PathType == PathExact, mj.PathType == PathExact
		if ei != ej {
			return ei
		}
		li, lj := len(strings.TrimRight(mi.Path, "/")), len(strings.TrimRight(mj.Path, "/"))
		if li != lj {
			return li > lj
		}
		return ri.ValuesPathIndex < rj.ValuesPathIndex
	})
}

// InChart checks if the original Ingress object resulted in a generated chart template
func (f IngressSpec) InChart(chart *types.GeneratedChart) bool {
	_, ok := chart.Templates[f.TemplatePath]
	return ok
}
