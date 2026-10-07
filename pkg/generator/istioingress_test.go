package generator

import (
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

const istioIngressManifests = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: shop
  namespace: default
  labels:
    app.kubernetes.io/name: shop
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: shop
  template:
    metadata:
      labels:
        app.kubernetes.io/name: shop
    spec:
      containers:
        - name: shop
          image: shop:1.0
---
apiVersion: v1
kind: Service
metadata:
  name: shop
  namespace: default
  labels:
    app.kubernetes.io/name: shop
spec:
  selector:
    app.kubernetes.io/name: shop
  ports:
    - name: http
      port: 80
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: shop
  namespace: default
  labels:
    app.kubernetes.io/name: shop
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
    kubernetes.io/ingress.class: istio
spec:
  tls:
    - hosts: [shop.example.com]
      secretName: shop-tls
  rules:
    - host: shop.example.com
      http:
        paths:
          - path: /api/
            pathType: Prefix
            backend:
              service:
                name: shop
                port:
                  name: http
          - path: /healthz
            pathType: Exact
            backend:
              service:
                name: shop
                port:
                  number: 80
    - host: plain.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: shop
                port:
                  number: 80
`

func istioIngressFC(t *testing.T, graph *types.ResourceGraph, params map[string]string) FeatureContext {
	t.Helper()
	f, ok := LookupFeature("istio-ingress")
	if !ok {
		t.Fatal("istio-ingress is not registered")
	}
	merged := map[string]string{}
	for k, v := range f.Params {
		merged[k] = v
	}
	for k, v := range params {
		merged[k] = v
	}
	return FeatureContext{Graph: graph, Params: merged}
}

func applyIstioIngressTo(t *testing.T, mode types.OutputMode, params map[string]string) *types.GeneratedChart {
	t.Helper()
	charts, graph := generateChartsFromManifests(t, istioIngressManifests, mode)
	for _, chart := range charts {
		if !hasIngressTemplate(chart) {
			continue
		}
		out, err := applyIstioIngressFeature(chart, istioIngressFC(t, graph, params))
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateChart(out); err != nil {
			t.Fatalf("chart invalid: %v", err)
		}
		return out
	}
	t.Fatal("no chart with an Ingress")
	return nil
}

func hasIngressTemplate(chart *types.GeneratedChart) bool {
	return len(resourceTemplates(chart, isKind("Ingress"))) > 0
}

func templateWithPrefix(t *testing.T, chart *types.GeneratedChart, prefix string) string {
	t.Helper()
	for path, content := range chart.Templates {
		if strings.HasPrefix(baseName(path), prefix) {
			return content
		}
	}
	t.Fatalf("no template %s* in %v", prefix, keys(chart.Templates))
	return ""
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestIstioIngress_TemplatesAndValues(t *testing.T) {
	chart := applyIstioIngressTo(t, types.OutputModeUniversal, nil)
	tmpl := templateWithPrefix(t, chart, "istio-ingress-")
	for _, want := range []string{
		"{{- if $.Values.istioIngress.enabled }}",
		"{{- if $svc.enabled }}",
		`{{- with index $.Values.istioIngress.ingresses "shop" }}`,
		"apiVersion: networking.istio.io/v1\nkind: Gateway",
		"kind: VirtualService",
		"    - shop\n", // gateways reference
		"Ingress annotations are not translated: nginx.ingress.kubernetes.io/rewrite-target",
		"istio-system",
	} {
		if !strings.Contains(tmpl, want) {
			t.Errorf("template lacks %q:\n%s", want, tmpl)
		}
	}
	if strings.Contains(tmpl, "with $svc.ingress") {
		t.Errorf("Istio resources must not depend on the Ingress values toggle:\n%s", tmpl)
	}

	var values map[string]interface{}
	if err := yaml.Unmarshal([]byte(chart.ValuesYAML), &values); err != nil {
		t.Fatal(err)
	}
	ii, _ := values["istioIngress"].(map[string]interface{})
	if ii["enabled"] != true || ii["replaceIngress"] != false {
		t.Errorf("toggles = %v", ii)
	}
	if !reflect.DeepEqual(ii["gatewaySelector"], map[string]interface{}{"istio": "ingressgateway"}) {
		t.Errorf("gatewaySelector = %v", ii["gatewaySelector"])
	}
	shop, _ := ii["ingresses"].(map[string]interface{})["shop"].(map[string]interface{})
	servers, _ := shop["servers"].([]interface{})
	if len(servers) != 3 {
		t.Fatalf("servers = %v, want https + redirect + plain http", servers)
	}
	vss, _ := shop["virtualServices"].([]interface{})
	if len(vss) != 2 {
		t.Fatalf("virtualServices = %v, want one per host", vss)
	}
	first, _ := vss[0].(map[string]interface{})
	if first["name"] != "shop-ingress-0" {
		t.Errorf("first VirtualService name = %v", first["name"])
	}
	http, _ := first["http"].([]interface{})
	if len(http) != 2 {
		t.Fatalf("routes = %v", http)
	}
	// Exact before Prefix; the named port is resolved from the Service.
	wantFirst := map[string]interface{}{
		"match": []interface{}{map[string]interface{}{"uri": map[string]interface{}{"exact": "/healthz"}}},
		"route": []interface{}{map[string]interface{}{"destination": map[string]interface{}{"host": "shop", "port": map[string]interface{}{"number": float64(80)}}}},
	}
	if !reflect.DeepEqual(http[0], wantFirst) {
		t.Errorf("first route = %v, want %v", http[0], wantFirst)
	}
	wantSecond := map[string]interface{}{
		"match": []interface{}{
			map[string]interface{}{"uri": map[string]interface{}{"exact": "/api"}},
			map[string]interface{}{"uri": map[string]interface{}{"prefix": "/api/"}},
		},
		"route": []interface{}{map[string]interface{}{"destination": map[string]interface{}{"host": "shop", "port": map[string]interface{}{"number": float64(80)}}}},
	}
	if !reflect.DeepEqual(http[1], wantSecond) {
		t.Errorf("second route = %v, want %v", http[1], wantSecond)
	}

	ingress := templateWithPrefix(t, chart, "shop-ingress")
	if !strings.HasPrefix(ingress, istioIngressReplaceGuard+"\n") || !strings.HasSuffix(ingress, "{{- end }}\n{{- end }}\n") {
		t.Errorf("Ingress is not guarded by replaceIngress:\n%s", ingress)
	}
}

func TestIstioIngress_Params(t *testing.T) {
	chart := applyIstioIngressTo(t, types.OutputModeSeparate, map[string]string{
		"gateway-selector": "app=edge, istio=gw",
		"https-redirect":   "false",
		"replace-ingress":  "true",
	})
	var values map[string]interface{}
	if err := yaml.Unmarshal([]byte(chart.ValuesYAML), &values); err != nil {
		t.Fatal(err)
	}
	ii, _ := values["istioIngress"].(map[string]interface{})
	if ii["replaceIngress"] != true {
		t.Errorf("replaceIngress = %v", ii["replaceIngress"])
	}
	if !reflect.DeepEqual(ii["gatewaySelector"], map[string]interface{}{"app": "edge", "istio": "gw"}) {
		t.Errorf("gatewaySelector = %v", ii["gatewaySelector"])
	}
	if strings.Contains(chart.ValuesYAML, "httpsRedirect") {
		t.Errorf("https-redirect=false must not redirect:\n%s", chart.ValuesYAML)
	}
}

func TestIstioIngress_InvalidSelector(t *testing.T) {
	charts, graph := generateChartsFromManifests(t, istioIngressManifests, types.OutputModeUniversal)
	_, err := applyIstioIngressFeature(charts[0], istioIngressFC(t, graph, map[string]string{"gateway-selector": "istio"}))
	if err == nil || !strings.Contains(err.Error(), "gateway-selector") {
		t.Errorf("err = %v, want gateway-selector error", err)
	}
}

func TestIstioIngress_NoIngressLeavesChart(t *testing.T) {
	chart := &types.GeneratedChart{Name: "x", Templates: map[string]string{}, ValuesYAML: "a: 1\n"}
	out, err := applyIstioIngressFeature(chart, istioIngressFC(t, nil, nil))
	if err != nil || out != chart {
		t.Errorf("out = %v, err = %v; want the chart unchanged", out, err)
	}
}

func TestIstioIngress_EveryMode(t *testing.T) {
	for _, mode := range []types.OutputMode{types.OutputModeUniversal, types.OutputModeSeparate, types.OutputModeLibrary, types.OutputModeUmbrella} {
		charts, graph := generateChartsFromManifests(t, istioIngressManifests, mode)
		out, err := ApplyFeatures(charts, []string{"istio-ingress"}, nil, graph)
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		found := false
		for _, chart := range out {
			if err := ValidateChart(chart); err != nil {
				t.Errorf("%s: chart %s invalid: %v", mode, chart.Name, err)
			}
			for path := range chart.Templates {
				found = found || strings.Contains(path, "istio-ingress-")
			}
		}
		if !found {
			t.Errorf("%s: no istio-ingress template", mode)
		}
	}
}

func ingressObject(t *testing.T, manifest string) *unstructured.Unstructured {
	t.Helper()
	var obj map[string]interface{}
	if err := yaml.Unmarshal([]byte(manifest), &obj); err != nil {
		t.Fatal(err)
	}
	return &unstructured.Unstructured{Object: obj}
}

func TestBuildIstioIngress_EdgeCases(t *testing.T) {
	ing := ingressObject(t, `
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
  namespace: default
spec:
  tls:
    - hosts: [nocert.example.com]
  defaultBackend:
    service:
      name: fallback
      port:
        name: web
  rules:
    - http:
        paths:
          - path: /legacy
            pathType: ImplementationSpecific
            backend:
              service:
                name: legacy
                port:
                  name: grpc
          - path: /bucket
            pathType: Prefix
            backend:
              resource:
                apiGroup: k8s.example.com
                kind: Bucket
                name: static
`)
	v := buildIstioIngress(ing, nil, "web-ingress", true)
	notes := strings.Join(v.notes, "\n")
	for _, want := range []string{
		"spec.tls[0] has no secretName",
		"ImplementationSpecific",
		`port "grpc" of Service legacy`,
		"resource backend",
		"defaultBackend",
	} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes lack %q:\n%s", want, notes)
		}
	}
	if len(v.Servers) != 1 {
		t.Fatalf("servers = %v, want only plain HTTP", v.Servers)
	}
	plain := v.Servers[0].(map[string]interface{})
	if !reflect.DeepEqual(plain["hosts"], []interface{}{"*"}) {
		t.Errorf("host-less rule must be served for every host: %v", plain["hosts"])
	}
	if len(v.VirtualServices) != 1 {
		t.Fatalf("virtualServices = %v", v.VirtualServices)
	}
	vs := v.VirtualServices[0].(map[string]interface{})
	if vs["name"] != "web-ingress" {
		t.Errorf("single-host VirtualService name = %v", vs["name"])
	}
	http := vs["http"].([]interface{})
	// legacy route (port omitted) + default backend; the resource backend is dropped.
	if len(http) != 2 {
		t.Fatalf("routes = %v", http)
	}
	legacy := http[0].(map[string]interface{})
	if !reflect.DeepEqual(legacy["match"], []interface{}{map[string]interface{}{"uri": map[string]interface{}{"prefix": "/legacy"}}}) {
		t.Errorf("ImplementationSpecific match = %v", legacy["match"])
	}
	dest := legacy["route"].([]interface{})[0].(map[string]interface{})["destination"].(map[string]interface{})
	if _, ok := dest["port"]; ok {
		t.Errorf("unresolved named port must be omitted: %v", dest)
	}
	if _, ok := http[1].(map[string]interface{})["match"]; ok {
		t.Errorf("default backend route must have no match: %v", http[1])
	}
}

func TestBuildIstioIngress_DefaultBackendOnly(t *testing.T) {
	ing := ingressObject(t, `
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
  namespace: default
spec:
  defaultBackend:
    service:
      name: web
      port:
        number: 8080
`)
	v := buildIstioIngress(ing, nil, "web-ingress", true)
	if len(v.VirtualServices) != 1 || len(v.Servers) != 1 {
		t.Fatalf("servers = %v, virtualServices = %v", v.Servers, v.VirtualServices)
	}
	vs := v.VirtualServices[0].(map[string]interface{})
	if !reflect.DeepEqual(vs["hosts"], []interface{}{"*"}) {
		t.Errorf("hosts = %v", vs["hosts"])
	}
}

func TestBuildIstioIngress_TLSWithoutRedirect(t *testing.T) {
	ing := ingressObject(t, `
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
  namespace: default
spec:
  tls:
    - hosts: ["*.example.com"]
      secretName: wildcard
  rules:
    - host: a.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: web
                port:
                  number: 80
`)
	v := buildIstioIngress(ing, nil, "web-ingress", false)
	// HTTPS + HTTP for the TLS hosts; a.example.com is covered by the wildcard.
	if len(v.Servers) != 2 {
		t.Fatalf("servers = %v", v.Servers)
	}
	http := v.Servers[1].(map[string]interface{})
	if _, ok := http["tls"]; ok {
		t.Errorf("https-redirect=false must not set tls on the HTTP server: %v", http)
	}
}

func TestIstioPathMatch(t *testing.T) {
	cases := []struct {
		path, pathType string
		want           []string // kind=value
	}{
		{"/", "Prefix", []string{"prefix=/"}},
		{"", "Prefix", []string{"prefix=/"}},
		{"/a/b/", "Prefix", []string{"exact=/a/b", "prefix=/a/b/"}},
		{"/a", "", []string{"exact=/a", "prefix=/a/"}},
		{"/a/", "Exact", []string{"exact=/a/"}},
		{"/a", "ImplementationSpecific", []string{"prefix=/a"}},
	}
	for _, tc := range cases {
		matches, _ := istioPathMatch(tc.path, tc.pathType)
		var got []string
		for _, m := range matches {
			for k, v := range m.(map[string]interface{})["uri"].(map[string]interface{}) {
				got = append(got, k+"="+v.(string))
			}
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("istioPathMatch(%q, %q) = %v, want %v", tc.path, tc.pathType, got, tc.want)
		}
	}
}

func TestSortIstioPaths(t *testing.T) {
	paths := []istioPath{
		{path: "/", pathType: "Prefix", index: 0},
		{path: "/api/v1", pathType: "Prefix", index: 1},
		{path: "/x", pathType: "Exact", index: 2},
		{path: "/api", pathType: "Prefix", index: 3},
		{path: "/abc", pathType: "Prefix", index: 4},
	}
	sortIstioPaths(paths)
	var got []string
	for _, p := range paths {
		got = append(got, p.path)
	}
	want := []string{"/x", "/api/v1", "/api", "/abc", "/"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestHostCovered(t *testing.T) {
	cases := []struct {
		host string
		tls  []string
		want bool
	}{
		{"a.example.com", []string{"a.example.com"}, true},
		{"a.example.com", []string{"*.example.com"}, true},
		{"a.b.example.com", []string{"*.example.com"}, false},
		{"a.example.com", []string{"*"}, true},
		{"a.example.com", []string{"b.example.com"}, false},
	}
	for _, tc := range cases {
		if got := hostCovered(tc.host, tc.tls); got != tc.want {
			t.Errorf("hostCovered(%q, %v) = %v", tc.host, tc.tls, got)
		}
	}
}

func TestParseGatewaySelector(t *testing.T) {
	got, err := parseGatewaySelector("istio=ingressgateway, ,app = edge")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]interface{}{"istio": "ingressgateway", "app": "edge"}) {
		t.Errorf("selector = %v", got)
	}
	if _, err := parseGatewaySelector("=x"); err == nil {
		t.Error("empty key must be rejected")
	}
	if got, err := parseGatewaySelector(""); err != nil || len(got) != 0 {
		t.Errorf("empty selector = %v, %v", got, err)
	}
}

func TestIstioIngressGate_WithoutWith(t *testing.T) {
	rt := &resourceTemplate{prefix: []string{"{{- if .Values.x }}"}}
	gate, opened := istioIngressGate(rt)
	if len(gate) != 1 || opened != 1 {
		t.Errorf("gate = %v, opened = %d", gate, opened)
	}
}
