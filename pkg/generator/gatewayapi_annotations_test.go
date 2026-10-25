package generator

import (
	"testing"
)

func TestParseNginxSize(t *testing.T) {
	cases := []struct {
		in  string
		out int64
		ok  bool
	}{
		{"0", 0, false},
		{"100", 100, true},
		{"1k", 1024, true},
		{"2M", 2097152, true},
		{"1G", 1073741824, true},
		{"invalid", 0, false},
	}

	for _, c := range cases {
		out, ok := parseNginxSize(c.in)
		if ok != c.ok || out != c.out {
			t.Errorf("parseNginxSize(%q) = %d, %v; want %d, %v", c.in, out, ok, c.out, c.ok)
		}
	}
}

func TestNginxAnnotations(t *testing.T) {
	facts := IngressFacts{
		Controller: ControllerNginx,
		Annotations: map[string]string{
			"nginx.ingress.kubernetes.io/ssl-redirect":       "false",
			"nginx.ingress.kubernetes.io/rewrite-target":     "/$2",
			"nginx.ingress.kubernetes.io/use-regex":          "true",
			"nginx.ingress.kubernetes.io/enable-cors":        "true",
			"nginx.ingress.kubernetes.io/backend-protocol":   "GRPC",
			"nginx.ingress.kubernetes.io/app-root":           "/home",
			"nginx.ingress.kubernetes.io/temporal-redirect":  "https://example.com/redir",
		},
	}
	opts := GatewayAPIOptions{
		ClassName: "alb",
		CORS:      true,
	}

	notes := []string{}
	noteFunc := func(format string, a ...interface{}) {
		notes = append(notes, format)
	}

	res := nginxAnnotations(facts, opts, noteFunc)

	if res.SSLRedirect == nil || *res.SSLRedirect != false {
		t.Errorf("Expected SSLRedirect to be false, got %v", res.SSLRedirect)
	}

	if res.ForceRegex != true {
		t.Errorf("Expected ForceRegex to be true")
	}
	if res.GRPC != true {
		t.Errorf("Expected GRPC to be true")
	}

	if len(res.ExtraRules) != 2 {
		t.Errorf("Expected 2 ExtraRules (app-root, temporal-redirect), got %d", len(res.ExtraRules))
	}
}

func TestNginxAnnotationsRedirect(t *testing.T) {
	facts := IngressFacts{
		Controller: ControllerNginx,
		Annotations: map[string]string{
			"nginx.ingress.kubernetes.io/ssl-redirect": "true",
			"nginx.ingress.kubernetes.io/canary":       "true",
			"nginx.ingress.kubernetes.io/canary-weight": "50",
			"nginx.ingress.kubernetes.io/canary-by-header": "X-Canary",
		},
	}
	opts := GatewayAPIOptions{
		ClassName: "alb",
	}

	res := nginxAnnotations(facts, opts, func(format string, a ...interface{}) {})

	if res.SSLRedirect == nil || *res.SSLRedirect != true {
		t.Errorf("Expected SSLRedirect to be true")
	}
}

func TestGatewayAPICanaryAndRedirect(t *testing.T) {
	fact := IngressFacts{
		Name:      "test-ingress",
		Namespace: "test-ns",
		Controller: ControllerNginx,
		Hosts: []IngressHost{
			{
				Host: "example.com",
				Paths: []IngressPath{
					{Path: "/", PathType: "Prefix", Backend: IngressBackend{ServiceName: "web", PortNumber: 80}},
				},
			},
		},
		Annotations: map[string]string{
			"nginx.ingress.kubernetes.io/ssl-redirect": "true",
			"nginx.ingress.kubernetes.io/canary":       "true",
			"nginx.ingress.kubernetes.io/canary-weight": "50",
			"nginx.ingress.kubernetes.io/canary-by-header": "X-Canary",
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

	// Should have main route and redirect route
	if len(result.HTTPRoutes) != 2 {
		t.Errorf("Expected 2 HTTPRoutes (main and redirect), got %d", len(result.HTTPRoutes))
	}

	hasRedirect := false
	for _, r := range result.HTTPRoutes {
		if r["name"] == "test-ingress-redirect" {
			hasRedirect = true
			rules := r["spec"].(map[string]interface{})["rules"].([]interface{})
			if len(rules) != 1 {
				t.Errorf("Redirect route should have 1 rule, got %d", len(rules))
			}
			filter := rules[0].(map[string]interface{})["filters"].([]interface{})[0].(map[string]interface{})
			if filter["type"] != "RequestRedirect" {
				t.Errorf("Expected RequestRedirect filter")
			}
		}
	}
	if !hasRedirect {
		t.Errorf("Missing redirect route")
	}
}
