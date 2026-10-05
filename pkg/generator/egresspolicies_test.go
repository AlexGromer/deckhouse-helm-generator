package generator

import "testing"

func TestEgressEndpoint(t *testing.T) {
	cases := []struct {
		raw      string
		host     string
		port     int
		protocol string
		ok       bool
	}{
		{"https://api.stripe.com/v1", "api.stripe.com", 443, "TLS", true},
		{"http://Example.COM", "example.com", 80, "HTTP", true},
		{"grpc://grpc.example.com:9000", "grpc.example.com", 9000, "GRPC", true},
		{"postgres://user:pw@db.example.com/app", "db.example.com", 5432, "TCP", true},
		{"cache.example.com:6380", "cache.example.com", 6380, "TCP", true},
		{"mail.example.com:443", "mail.example.com", 443, "TLS", true},
		{"smtp.example.com", "smtp.example.com", 0, "TCP", true},
		{"not a url", "", 0, "", false},
		{"https://", "", 0, "", false},
		{"host.example.com:99999", "", 0, "", false},
	}
	for _, c := range cases {
		host, port, ok := egressEndpoint(c.raw)
		if ok != c.ok || host != c.host || (ok && (port.Number != c.port || port.Protocol != c.protocol)) {
			t.Errorf("egressEndpoint(%q) = %q %+v %v, want %q %d/%s %v", c.raw, host, port, ok, c.host, c.port, c.protocol, c.ok)
		}
	}
}

func TestEgressIsExternal(t *testing.T) {
	services := map[string]bool{"redis": true}
	namespaces := map[string]bool{"shop": true}
	for host, want := range map[string]bool{
		"api.stripe.com":                 true,
		"example.org":                    true,
		"redis":                          false, // no dot: in-cluster short name
		"redis.cache":                    false, // <service>.<namespace>
		"orders.shop":                    false, // <service>.<namespace>
		"orders.shop.svc":                false,
		"orders.shop.svc.cluster.local":  false,
		"printer.local":                  false,
		"10.0.0.1":                       false,
		"metadata.google.internal":       false,
		"s3.eu-west-1.amazonaws.com":     true,
		"storage.googleapis.com":         true,
		"vault.platform.svc.example.com": false,
	} {
		if got := egressIsExternal(host, services, namespaces); got != want {
			t.Errorf("egressIsExternal(%q) = %v, want %v", host, got, want)
		}
	}
}
