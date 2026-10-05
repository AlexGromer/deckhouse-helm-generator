package generator

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// tlsIngressTemplate has the shape of the Ingress processor's template.
const tlsIngressTemplate = `{{- with .Values.ingress }}
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ include "web.fullname" $ }}-web
  labels:
    {{- include "web.labels" $ | nindent 4 }}
  {{- with .annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  {{- if .tls }}
  tls:
    {{- range .tls }}
    - secretName: {{ .secretName }}
    {{- end }}
  {{- end }}
  rules: []
{{- end }}
`

func TestInjectIngressTLS(t *testing.T) {
	in := &types.GeneratedChart{
		Name:       "web",
		ValuesYAML: "ingress:\n  enabled: true\n",
		Templates: map[string]string{
			"templates/web-ingress.yaml": tlsIngressTemplate,
			"templates/web-service.yaml": "apiVersion: v1\nkind: Service\n",
		},
	}
	out, changed, err := InjectIngressTLS(in, IngressTLSOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(changed, ",") != "templates/web-ingress.yaml" {
		t.Fatalf("changed = %v", changed)
	}
	tpl := out.Templates["templates/web-ingress.yaml"]
	for _, want := range []string{
		// tls is only generated when the values have none and hosts exist.
		`{{- $dhgAutoTLS := and $dhgIngressTLS.enabled (not .tls) (gt (len $dhgTLSHosts) 0) }}`,
		// user annotations win over the issuer annotation.
		`{{- $dhgAnnotations = merge (dict) $dhgAnnotations (dict $dhgIssuerKey $dhgIngressTLS.issuer) }}`,
		`"cert-manager.io/cluster-issuer"`,
		"spec:\n  {{- if $dhgAutoTLS }}\n  tls:\n    - hosts:\n",
		`      secretName: {{ include "web.fullname" $ }}-web-tls`,
		// The template's own tls block is untouched.
		"  {{- if .tls }}\n  tls:\n    {{- range .tls }}",
	} {
		if !strings.Contains(tpl, want) {
			t.Errorf("template misses %q:\n%s", want, tpl)
		}
	}
	if strings.Count(tpl, "  annotations:\n") != 1 {
		t.Errorf("expected exactly one annotations key:\n%s", tpl)
	}
	if !strings.Contains(out.ValuesYAML, "ingressTLS:\n  enabled: true\n  issuer: letsencrypt-prod\n  issuerKind: ClusterIssuer\n") {
		t.Errorf("values:\n%s", out.ValuesYAML)
	}
	if in.Templates["templates/web-ingress.yaml"] != tlsIngressTemplate {
		t.Error("input chart was modified")
	}

	again, changed, err := InjectIngressTLS(out, IngressTLSOptions{})
	if err != nil || again != out || len(changed) != 0 {
		t.Errorf("second application changed the chart (err=%v, changed=%v)", err, changed)
	}
}

func TestInjectIngressTLS_UnknownShapeAndOptions(t *testing.T) {
	static := "apiVersion: networking.k8s.io/v1\nkind: Ingress\nmetadata:\n  name: web\n  annotations:\n    a: b\nspec:\n  rules: []\n"
	in := &types.GeneratedChart{Name: "web", Templates: map[string]string{"templates/ingress.yaml": static}}
	out, changed, err := InjectIngressTLS(in, IngressTLSOptions{})
	if err != nil || out != in || len(changed) != 0 {
		t.Errorf("unrecognised template must be skipped (err=%v, changed=%v)", err, changed)
	}
	if _, _, err := InjectIngressTLS(in, IngressTLSOptions{IssuerKind: "Vault"}); err == nil {
		t.Error("expected an error for an invalid issuer kind")
	}
}
