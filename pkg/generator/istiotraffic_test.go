package generator

import (
	"reflect"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

func TestIstioFeature(t *testing.T) {
	out := applyObsFeatures(t, []string{"istio"}, nil)

	content, ok := out.Templates["templates/istio-web-service.yaml"]
	if !ok {
		t.Fatalf("istio template missing; templates: %v", keysOfMap(out.Templates))
	}
	// Wrapped like the Service: global toggle, then the Service's own wrapper.
	if !strings.HasPrefix(content, "{{- if $.Values.istio.enabled }}\n{{- $svc := .Values.services.web -}}\n{{- if $svc.enabled }}\n{{- with $svc.service }}\n") {
		t.Errorf("unexpected wrapper:\n%s", content)
	}
	for _, want := range []string{
		"apiVersion: networking.istio.io/v1\nkind: VirtualService",
		"apiVersion: networking.istio.io/v1\nkind: DestinationRule",
		"apiVersion: security.istio.io/v1\nkind: PeerAuthentication",
		"apiVersion: security.istio.io/v1\nkind: AuthorizationPolicy",
		`    - {{ include "app.fullname" $ }}-web`,
		`            host: {{ include "app.fullname" $ }}-web`,
		`  host: {{ include "app.fullname" $ }}-web`,
		// Service selector re-indented under matchLabels.
		"    matchLabels:\n      {{- include \"app.selectorLabels\" $ | nindent 6 }}\n      app.kubernetes.io/component: web",
		"{{- if $.Values.istio.authorizationPolicy.rules }}",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("istio template misses %q:\n%s", want, content)
		}
	}

	want := map[string]interface{}{
		"enabled": true,
		"virtualService": map[string]interface{}{
			"enabled": true,
			"timeout": "30s",
			"retries": map[string]interface{}{"attempts": float64(3), "perTryTimeout": "10s"},
		},
		"destinationRule": map[string]interface{}{
			"enabled": true,
			"trafficPolicy": map[string]interface{}{"outlierDetection": map[string]interface{}{
				"consecutive5xxErrors": float64(5), "interval": "30s", "baseEjectionTime": "30s", "maxEjectionPercent": float64(50),
			}},
		},
		"peerAuthentication":  map[string]interface{}{"mtlsMode": "STRICT"},
		"authorizationPolicy": map[string]interface{}{"action": "ALLOW", "rules": []interface{}{}},
	}
	if got := valuesOf(t, out)["istio"]; !reflect.DeepEqual(got, want) {
		t.Errorf("values.istio =\n%v\nwant\n%v", got, want)
	}
}

func TestIstioFeature_TrafficPolicyParams(t *testing.T) {
	out := applyObsFeatures(t, []string{"istio"}, map[string]map[string]string{"istio": {
		"hash-header": "x-user", "max-connections": "100", "max-pending-requests": "10",
		"outlier-5xx": "0", "retries": "", "timeout": "", "mtls-mode": "",
	}})
	istio := valuesOf(t, out)["istio"].(map[string]interface{})
	policy := istio["destinationRule"].(map[string]interface{})["trafficPolicy"]
	want := map[string]interface{}{
		"loadBalancer":   map[string]interface{}{"consistentHash": map[string]interface{}{"httpHeaderName": "x-user"}},
		"connectionPool": map[string]interface{}{"tcp": map[string]interface{}{"maxConnections": float64(100)}, "http": map[string]interface{}{"http1MaxPendingRequests": float64(10)}},
	}
	if !reflect.DeepEqual(policy, want) {
		t.Errorf("trafficPolicy = %v, want %v", policy, want)
	}
	if vs := istio["virtualService"]; !reflect.DeepEqual(vs, map[string]interface{}{"enabled": true}) {
		t.Errorf("virtualService = %v, want no timeout/retries", vs)
	}

	out = applyObsFeatures(t, []string{"istio"}, map[string]map[string]string{"istio": {"lb-policy": "LEAST_REQUEST"}})
	if !strings.Contains(out.ValuesYAML, "simple: LEAST_REQUEST") {
		t.Errorf("lb-policy not reflected:\n%s", out.ValuesYAML)
	}
}

func TestIstioFeature_InvalidParams(t *testing.T) {
	for _, opts := range []map[string]string{
		{"lb-policy": "FASTEST"},
		{"lb-policy": "RANDOM", "hash-header": "x-user"},
		{"mtls-mode": "ON"},
		{"timeout": "30"},
		{"retries": "-1"},
		{"max-connections": "many"},
	} {
		_, err := ApplyFeatures([]*types.GeneratedChart{obsTestChart()}, []string{"istio"},
			map[string]map[string]string{"istio": opts}, obsTestGraph())
		if err == nil {
			t.Errorf("options %v must be rejected", opts)
		}
	}
}

func TestIstioFeature_NoServices(t *testing.T) {
	chart := obsTestChart()
	delete(chart.Templates, obsServicePath)
	charts, err := ApplyFeatures([]*types.GeneratedChart{chart}, []string{"istio"}, nil, obsTestGraph())
	if err != nil {
		t.Fatal(err)
	}
	if charts[0] != chart {
		t.Error("chart without Services must be returned unchanged")
	}
}

func keysOfMap(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
