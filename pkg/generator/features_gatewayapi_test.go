package generator

import (
	"testing"
)

func TestGatewayAPI(t *testing.T) {
	// A simple unit test to check translateIngress
	fact := IngressFacts{
		Name:      "test-ingress",
		Namespace: "test-ns",
		Hosts: []IngressHost{
			{
				Host: "example.com",
				Paths: []IngressPath{
					{Path: "/", PathType: "Prefix", Backend: IngressBackend{ServiceName: "web", PortNumber: 80}},
				},
			},
		},
		Annotations: map[string]string{
			"nginx.ingress.kubernetes.io/ssl-passthrough": "true",
		},
		TLS: []IngressTLS{
			{SecretName: "test-tls"},
		},
	}
	opts := GatewayAPIOptions{
		ClassName:   "alb",
		ListenerSet: true,
	}

	result := translateIngress(fact, opts, "gw", "gw-ns")

	if len(result.HTTPRoutes) != 1 {
		t.Errorf("Expected 1 HTTPRoute, got %d", len(result.HTTPRoutes))
	}
	if len(result.ListenerSets) != 1 {
		t.Errorf("Expected 1 ListenerSet, got %d", len(result.ListenerSets))
	}
	if len(result.TLSRoutes) != 1 {
		t.Errorf("Expected 1 TLSRoute, got %d", len(result.TLSRoutes))
	}
}
