package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// 2.6.8: Ingress TLS — InjectTLSConfig
// ============================================================

func TestIngressTLS_AddsTLSSection(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/ingress.yaml": `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: myapp-ingress
  namespace: {{ .Release.Namespace }}
spec:
  rules:
    - host: app.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: myapp
                port:
                  number: 80
`,
		},
	}

	result := InjectTLSConfig(chart, "letsencrypt-prod")

	content := result.Templates["templates/ingress.yaml"]

	if !strings.Contains(content, "tls:") {
		t.Error("expected tls: section in Ingress")
	}
	if !strings.Contains(content, "app-example-com-tls") {
		t.Error("expected secretName derived from host: app-example-com-tls")
	}
	if !strings.Contains(content, "app.example.com") {
		t.Error("expected host in tls section")
	}
}

func TestIngressTLS_CertManagerAnnotation(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/ingress.yaml": `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: myapp-ingress
  annotations:
    existing: "true"
spec:
  rules:
    - host: app.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
`,
		},
	}

	result := InjectTLSConfig(chart, "letsencrypt-staging")

	content := result.Templates["templates/ingress.yaml"]

	if !strings.Contains(content, "cert-manager.io/cluster-issuer: letsencrypt-staging") {
		t.Error("expected cert-manager.io/cluster-issuer annotation")
	}
	if !strings.Contains(content, "force-ssl-redirect: \"true\"") && !strings.Contains(content, "force-ssl-redirect: 'true'") {
		// The annotation injection may quote the value
		if !strings.Contains(content, "force-ssl-redirect") {
			t.Error("expected force-ssl-redirect annotation")
		}
	}
}

func TestIngressTLS_DefaultIssuer(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/ingress.yaml": `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: myapp-ingress
spec:
  rules:
    - host: api.example.com
`,
		},
	}

	result := InjectTLSConfig(chart, "")

	content := result.Templates["templates/ingress.yaml"]
	if !strings.Contains(content, "letsencrypt-prod") {
		t.Error("expected default issuer letsencrypt-prod when empty issuer provided")
	}
}

func TestIngressTLS_MultipleHosts(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/ingress.yaml": `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: myapp-ingress
spec:
  rules:
    - host: app.example.com
      http:
        paths:
          - path: /
    - host: api.example.com
      http:
        paths:
          - path: /api
`,
		},
	}

	result := InjectTLSConfig(chart, "letsencrypt-prod")

	content := result.Templates["templates/ingress.yaml"]
	if !strings.Contains(content, "app-example-com-tls") {
		t.Error("expected tls secret for app.example.com")
	}
	if !strings.Contains(content, "api-example-com-tls") {
		t.Error("expected tls secret for api.example.com")
	}
}

func TestIngressTLS_SkipsNonIngress(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/deployment.yaml": `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
`,
		},
	}

	result := InjectTLSConfig(chart, "letsencrypt-prod")

	content := result.Templates["templates/deployment.yaml"]
	if strings.Contains(content, "tls:") {
		t.Error("should not add TLS to non-Ingress resources")
	}
	if strings.Contains(content, "cert-manager") {
		t.Error("should not add cert-manager annotation to non-Ingress resources")
	}
}

func TestIngressTLS_NilChart(t *testing.T) {
	result := InjectTLSConfig(nil, "letsencrypt-prod")
	if result != nil {
		t.Error("expected nil for nil chart")
	}
}

func TestIngressTLS_CopyOnWrite(t *testing.T) {
	original := &types.GeneratedChart{
		Name: "test",
		Templates: map[string]string{
			"templates/ingress.yaml": `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: myapp-ingress
spec:
  rules:
    - host: app.example.com
`,
		},
	}

	result := InjectTLSConfig(original, "letsencrypt-prod")

	if strings.Contains(original.Templates["templates/ingress.yaml"], "tls:") {
		t.Error("original chart must not be modified (copy-on-write violation)")
	}
	if !strings.Contains(result.Templates["templates/ingress.yaml"], "cert-manager") {
		t.Error("result must contain cert-manager annotation")
	}
}

func TestIngressTLS_ExistingTLSSection(t *testing.T) {
	chart := &types.GeneratedChart{
		Templates: map[string]string{
			"templates/ingress.yaml": `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: myapp-ingress
spec:
  tls:
    - secretName: existing-tls
      hosts:
        - app.example.com
  rules:
    - host: app.example.com
`,
		},
	}

	result := InjectTLSConfig(chart, "letsencrypt-prod")

	content := result.Templates["templates/ingress.yaml"]
	// Should still add annotations but not duplicate tls section
	if !strings.Contains(content, "cert-manager") {
		t.Error("should add cert-manager annotation even with existing TLS")
	}
	// Count occurrences of "tls:" — should be exactly 1
	count := strings.Count(content, "tls:")
	if count != 1 {
		t.Errorf("expected exactly 1 tls: section, got %d", count)
	}
}

// ============================================================
// Host extraction and secret name
// ============================================================

func TestHostToSecretName(t *testing.T) {
	tests := []struct {
		host     string
		expected string
	}{
		{"app.example.com", "app-example-com-tls"},
		{"api.example.com", "api-example-com-tls"},
		{"*.example.com", "wildcard-example-com-tls"},
		{"simple", "simple-tls"},
	}

	for _, tt := range tests {
		got := hostToSecretName(tt.host)
		if got != tt.expected {
			t.Errorf("hostToSecretName(%q) = %q, want %q", tt.host, got, tt.expected)
		}
	}
}

func TestExtractHosts(t *testing.T) {
	content := `spec:
  rules:
    - host: app.example.com
    - host: api.example.com
    - host: app.example.com
`
	hosts := extractHosts(content)

	if len(hosts) != 2 {
		t.Errorf("expected 2 unique hosts, got %d: %v", len(hosts), hosts)
	}
}
