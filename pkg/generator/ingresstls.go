package generator

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

const ingressTLSValuesKey = "ingressTLS"

// IngressTLSOptions configures the defaults written to values.yaml
// (ingressTLS.*). All of them can be changed at install time.
type IngressTLSOptions struct {
	// Issuer is the cert-manager issuer name (default "letsencrypt-prod").
	Issuer string
	// IssuerKind is "ClusterIssuer" (default) or "Issuer"; it selects the
	// cert-manager.io/cluster-issuer or cert-manager.io/issuer annotation.
	IssuerKind string
}

var (
	// ingressAnnotationsBlockRe matches the values-driven metadata
	// annotations block of the Ingress processor's template.
	ingressAnnotationsBlockRe = regexp.MustCompile(`(?m)^  \{\{- with \.annotations \}\}\n  annotations:\n    \{\{- toYaml \. \| nindent 4 \}\}\n  \{\{- end \}\}\n`)
	ingressNameRe             = regexp.MustCompile(`(?m)^metadata:\n  name: (.+)$`)
)

// InjectIngressTLS enables TLS on every Ingress template whose values do not
// already define a tls section: it adds a tls entry covering all rule hosts
// with a per-Ingress certificate secret ("<ingress name>-tls") and the
// cert-manager issuer annotation, so cert-manager's ingress-shim issues the
// certificate. Ingresses that already have tls are left as they are (their
// certificates may be managed differently), as are user-set annotations.
// Everything is controlled by ingressTLS.* in values.yaml.
//
// It returns the new chart and the template paths that were changed.
func InjectIngressTLS(chart *types.GeneratedChart, opts IngressTLSOptions) (*types.GeneratedChart, []string, error) {
	if chart == nil {
		return nil, nil, nil
	}
	if opts.Issuer == "" {
		opts.Issuer = "letsencrypt-prod"
	}
	if opts.IssuerKind == "" {
		opts.IssuerKind = "ClusterIssuer"
	}
	if opts.IssuerKind != "ClusterIssuer" && opts.IssuerKind != "Issuer" {
		return nil, nil, fmt.Errorf("invalid issuer kind %q (want ClusterIssuer or Issuer)", opts.IssuerKind)
	}

	out := cloneChart(chart)
	var changed []string
	for _, path := range opsSortedTemplatePaths(chart) {
		content := chart.Templates[path]
		if opsTemplateKind(content) != "Ingress" || strings.Contains(content, "dhgIngressTLS") {
			continue
		}
		updated, ok := injectIngressTLSBlocks(content)
		if !ok {
			continue
		}
		out.Templates[path] = updated
		changed = append(changed, path)
	}
	if len(changed) == 0 {
		return chart, nil, nil
	}
	values, err := appendTopLevelValues(out.ValuesYAML, ingressTLSValuesKey, map[string]interface{}{
		"enabled":    true,
		"issuer":     opts.Issuer,
		"issuerKind": opts.IssuerKind,
	})
	if err != nil {
		return nil, nil, err
	}
	out.ValuesYAML = values
	return out, changed, nil
}

// injectIngressTLSBlocks rewrites the annotations block and extends the tls
// block of an Ingress template. It returns false if the template does not
// have the expected shape.
func injectIngressTLSBlocks(content string) (string, bool) {
	nameMatch := ingressNameRe.FindStringSubmatch(content)
	loc := ingressAnnotationsBlockRe.FindStringIndex(content)
	if nameMatch == nil || loc == nil {
		return content, false
	}
	secretName := suffixedName(strings.TrimSpace(nameMatch[1]), "-tls")

	annotations := strings.Join([]string{
		`  {{- $dhgIngressTLS := $.Values.ingressTLS | default dict }}`,
		`  {{- $dhgTLSHosts := list }}`,
		`  {{- range .rules }}{{ if .host }}{{ $dhgTLSHosts = append $dhgTLSHosts .host }}{{ end }}{{ end }}`,
		`  {{- $dhgTLSHosts = $dhgTLSHosts | uniq }}`,
		`  {{- $dhgAutoTLS := and $dhgIngressTLS.enabled (not .tls) (gt (len $dhgTLSHosts) 0) }}`,
		`  {{- $dhgAnnotations := .annotations | default dict }}`,
		`  {{- if and $dhgAutoTLS $dhgIngressTLS.issuer }}`,
		`  {{- $dhgIssuerKey := ternary "cert-manager.io/issuer" "cert-manager.io/cluster-issuer" (eq ($dhgIngressTLS.issuerKind | default "ClusterIssuer") "Issuer") }}`,
		`  {{- $dhgAnnotations = merge (dict) $dhgAnnotations (dict $dhgIssuerKey $dhgIngressTLS.issuer) }}`,
		`  {{- end }}`,
		`  {{- with $dhgAnnotations }}`,
		`  annotations:`,
		`    {{- toYaml . | nindent 4 }}`,
		`  {{- end }}`,
		``,
	}, "\n")
	content = content[:loc[0]] + annotations + content[loc[1]:]

	tls := strings.Join([]string{
		`  {{- if $dhgAutoTLS }}`,
		`  tls:`,
		`    - hosts:`,
		`        {{- range $dhgTLSHosts }}`,
		`        - {{ . | quote }}`,
		`        {{- end }}`,
		`      secretName: ` + secretName,
		`  {{- end }}`,
	}, "\n")

	// The generated tls block renders only when the values have no tls, so it
	// never collides with the template's own `{{- if .tls }}` block.
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if line == "spec:" {
			out := append([]string{}, lines[:i+1]...)
			out = append(out, tls)
			out = append(out, lines[i+1:]...)
			return strings.Join(out, "\n"), true
		}
	}
	return content, false
}
