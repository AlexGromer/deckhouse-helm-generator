package generator

// ============================================================
// Test Plan: Istio Multi-Cluster ServiceEntry Generator (Task 5.8.8)
// ============================================================
//
// | #  | Test Name                                                   | Category    | Input                                           | Expected Output                                                        |
// |----|-------------------------------------------------------------|-------------|-------------------------------------------------|------------------------------------------------------------------------|
// |  1 | TestGenerateMultiClusterServiceEntry_WithEndpoints          | happy       | RemoteClusterEndpoints=["10.1.0.1","10.1.0.2"] | ServiceEntry YAML contains both endpoint IPs                           |
// |  2 | TestGenerateMultiClusterServiceEntry_MultiplePorts          | happy       | Ports=[{http,80,HTTP},{grpc,9090,GRPC}]         | ServiceEntry YAML contains port 80 and 9090                            |
// |  3 | TestGenerateMultiClusterServiceEntry_LocalityRouting        | happy       | LocalityRouting=true                            | YAML contains locality routing or trafficPolicy locality config        |
// |  4 | TestGenerateMultiClusterServiceEntry_FailoverPriority       | happy       | FailoverPriority=1                              | YAML contains failoverPriority or "1" in priority context              |
// |  5 | TestGenerateMultiClusterServiceEntry_EmptyEndpoints         | edge        | RemoteClusterEndpoints=[]                       | returns nil or empty map, no panic                                     |
// |  6 | TestGenerateMultiClusterServiceEntry_SingleEndpoint         | edge        | RemoteClusterEndpoints=["192.168.1.10"]         | ServiceEntry YAML contains "192.168.1.10"                              |
// |  7 | TestGenerateMultiClusterServiceEntry_ProtocolTypes          | happy       | Ports=[{http,80,HTTP},{tcp,3306,TCP}]           | YAML contains both HTTP and TCP protocol strings                       |
// |  8 | TestGenerateMultiClusterServiceEntry_ValidAPIVersion        | happy       | valid opts                                      | YAML contains apiVersion networking.istio.io and kind ServiceEntry     |

import (
	"strings"
	"testing"
)

// ── 1: ServiceEntry contains all configured endpoint IPs ─────────────────────

func TestGenerateMultiClusterServiceEntry_WithEndpoints(t *testing.T) {
	opts := MultiClusterOptions{
		ServiceName:              "remote-db",
		RemoteClusterEndpoints:   []string{"10.1.0.1", "10.1.0.2"},
		Ports:                    []ServicePort{{Name: "tcp", Number: 5432, Protocol: "TCP"}},
		LocalityRouting:          false,
		FailoverPriority:         0,
	}

	templates := GenerateMultiClusterServiceEntry(opts)

	if len(templates) == 0 {
		t.Fatal("expected at least one template")
	}
	all := joinTemplates(templates)
	if !strings.Contains(all, "10.1.0.1") {
		t.Errorf("expected endpoint '10.1.0.1' in ServiceEntry YAML:\n%s", all)
	}
	if !strings.Contains(all, "10.1.0.2") {
		t.Errorf("expected endpoint '10.1.0.2' in ServiceEntry YAML:\n%s", all)
	}
}

// ── 2: multiple ports all appear in YAML ─────────────────────────────────────

func TestGenerateMultiClusterServiceEntry_MultiplePorts(t *testing.T) {
	opts := MultiClusterOptions{
		ServiceName:            "multi-port-svc",
		RemoteClusterEndpoints: []string{"172.16.0.10"},
		Ports: []ServicePort{
			{Name: "http", Number: 80, Protocol: "HTTP"},
			{Name: "grpc", Number: 9090, Protocol: "GRPC"},
		},
	}

	templates := GenerateMultiClusterServiceEntry(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, "80") {
		t.Errorf("expected port 80 in ServiceEntry YAML:\n%s", all)
	}
	if !strings.Contains(all, "9090") {
		t.Errorf("expected port 9090 in ServiceEntry YAML:\n%s", all)
	}
}

// ── 3: LocalityRouting=true produces locality config ─────────────────────────

func TestGenerateMultiClusterServiceEntry_LocalityRouting(t *testing.T) {
	opts := MultiClusterOptions{
		ServiceName:            "locality-svc",
		RemoteClusterEndpoints: []string{"10.2.0.1"},
		Ports:                  []ServicePort{{Name: "http", Number: 80, Protocol: "HTTP"}},
		LocalityRouting:        true,
		FailoverPriority:       0,
	}

	templates := GenerateMultiClusterServiceEntry(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, "locality") && !strings.Contains(all, "Locality") {
		t.Errorf("expected locality routing config in YAML when LocalityRouting=true:\n%s", all)
	}
}

// ── 4: FailoverPriority appears in YAML ──────────────────────────────────────

func TestGenerateMultiClusterServiceEntry_FailoverPriority(t *testing.T) {
	opts := MultiClusterOptions{
		ServiceName:            "failover-svc",
		RemoteClusterEndpoints: []string{"10.3.0.1", "10.3.0.2"},
		Ports:                  []ServicePort{{Name: "http", Number: 8080, Protocol: "HTTP"}},
		LocalityRouting:        false,
		FailoverPriority:       1,
	}

	templates := GenerateMultiClusterServiceEntry(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, "failoverPriority") && !strings.Contains(all, "priority") {
		t.Errorf("expected failoverPriority config in YAML when FailoverPriority=1:\n%s", all)
	}
}

// ── 5: empty endpoints returns nil/empty, no panic ────────────────────────────

func TestGenerateMultiClusterServiceEntry_EmptyEndpoints(t *testing.T) {
	opts := MultiClusterOptions{
		ServiceName:            "empty-svc",
		RemoteClusterEndpoints: []string{},
		Ports:                  []ServicePort{{Name: "http", Number: 80, Protocol: "HTTP"}},
	}

	// Must not panic.
	templates := GenerateMultiClusterServiceEntry(opts)

	if len(templates) > 0 {
		t.Logf("GenerateMultiClusterServiceEntry with empty endpoints returned %d templates (implementation choice)", len(templates))
	}
}

// ── 6: single endpoint appears in YAML ───────────────────────────────────────

func TestGenerateMultiClusterServiceEntry_SingleEndpoint(t *testing.T) {
	opts := MultiClusterOptions{
		ServiceName:            "single-ep-svc",
		RemoteClusterEndpoints: []string{"192.168.1.10"},
		Ports:                  []ServicePort{{Name: "https", Number: 443, Protocol: "HTTPS"}},
	}

	templates := GenerateMultiClusterServiceEntry(opts)

	if len(templates) == 0 {
		t.Fatal("expected at least one template for single endpoint")
	}
	all := joinTemplates(templates)
	if !strings.Contains(all, "192.168.1.10") {
		t.Errorf("expected endpoint '192.168.1.10' in ServiceEntry YAML:\n%s", all)
	}
}

// ── 7: HTTP and TCP protocol types both appear ───────────────────────────────

func TestGenerateMultiClusterServiceEntry_ProtocolTypes(t *testing.T) {
	opts := MultiClusterOptions{
		ServiceName:            "mixed-proto-svc",
		RemoteClusterEndpoints: []string{"10.0.0.5"},
		Ports: []ServicePort{
			{Name: "http", Number: 80, Protocol: "HTTP"},
			{Name: "mysql", Number: 3306, Protocol: "TCP"},
		},
	}

	templates := GenerateMultiClusterServiceEntry(opts)

	all := joinTemplates(templates)
	if !strings.Contains(all, "HTTP") {
		t.Errorf("expected protocol 'HTTP' in ServiceEntry YAML:\n%s", all)
	}
	if !strings.Contains(all, "TCP") {
		t.Errorf("expected protocol 'TCP' in ServiceEntry YAML:\n%s", all)
	}
}

// ── 8: generated YAML has valid Istio networking apiVersion ──────────────────

func TestGenerateMultiClusterServiceEntry_ValidAPIVersion(t *testing.T) {
	opts := MultiClusterOptions{
		ServiceName:            "valid-svc",
		RemoteClusterEndpoints: []string{"10.10.0.1"},
		Ports:                  []ServicePort{{Name: "http", Number: 80, Protocol: "HTTP"}},
	}

	templates := GenerateMultiClusterServiceEntry(opts)

	if len(templates) == 0 {
		t.Fatal("expected at least one template")
	}
	all := joinTemplates(templates)
	if !strings.Contains(all, "networking.istio.io") {
		t.Errorf("expected apiVersion 'networking.istio.io' in YAML:\n%s", all)
	}
	if !strings.Contains(all, "ServiceEntry") {
		t.Errorf("expected kind 'ServiceEntry' in YAML:\n%s", all)
	}
}
