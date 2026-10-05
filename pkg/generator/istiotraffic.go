package generator

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// Istio integration (`dhg generate --with istio`).
//
// For every generated Service the feature adds one template with:
//   - a VirtualService (mesh traffic) with timeout and retries;
//   - a DestinationRule with load balancing, connection pool limits and
//     outlier detection (circuit breaking);
//   - a PeerAuthentication enforcing the mTLS mode on the Service's pods;
//   - an AuthorizationPolicy for the Service's pods, rendered only when
//     `.Values.istio.authorizationPolicy.rules` is set (rules cannot be
//     derived from manifests, so they are left to the chart user).
//
// All resources reuse the Service's own name, labels and selector and are
// toggled through `.Values.istio`.

var (
	istioLBPolicies = map[string]bool{"": true, "LEAST_REQUEST": true, "ROUND_ROBIN": true, "RANDOM": true, "PASSTHROUGH": true}
	istioMTLSModes  = map[string]bool{"": true, "STRICT": true, "PERMISSIVE": true, "DISABLE": true}
	reDuration      = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?(ms|s|m|h)$`)
)

// istioValues builds the `istio` values block from the feature parameters.
func istioValues(fc FeatureContext) (map[string]interface{}, error) {
	for _, key := range []string{"timeout", "per-try-timeout"} {
		if v := fc.Param(key); v != "" && !reDuration.MatchString(v) {
			return nil, fmt.Errorf("%s must be a duration such as 30s; got %q", key, v)
		}
	}
	ints := map[string]int{}
	for _, key := range []string{"retries", "outlier-5xx", "max-connections", "max-pending-requests"} {
		v := fc.Param(key)
		if v == "" {
			ints[key] = -1
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("%s must be a non-negative integer; got %q", key, v)
		}
		ints[key] = n
	}
	lb, hashHeader, mtls := fc.Param("lb-policy"), fc.Param("hash-header"), fc.Param("mtls-mode")
	if !istioLBPolicies[lb] {
		return nil, fmt.Errorf("lb-policy must be one of LEAST_REQUEST, ROUND_ROBIN, RANDOM, PASSTHROUGH; got %q", lb)
	}
	if lb != "" && hashHeader != "" {
		return nil, fmt.Errorf("lb-policy and hash-header are mutually exclusive")
	}
	if !istioMTLSModes[mtls] {
		return nil, fmt.Errorf("mtls-mode must be one of STRICT, PERMISSIVE, DISABLE (or empty); got %q", mtls)
	}

	vs := map[string]interface{}{"enabled": true}
	if t := fc.Param("timeout"); t != "" {
		vs["timeout"] = t
	}
	if n := ints["retries"]; n >= 0 {
		retries := map[string]interface{}{"attempts": n}
		if t := fc.Param("per-try-timeout"); t != "" && n > 0 {
			retries["perTryTimeout"] = t
		}
		vs["retries"] = retries
	}

	policy := map[string]interface{}{}
	switch {
	case hashHeader != "":
		policy["loadBalancer"] = map[string]interface{}{
			"consistentHash": map[string]interface{}{"httpHeaderName": hashHeader},
		}
	case lb != "":
		policy["loadBalancer"] = map[string]interface{}{"simple": lb}
	}
	pool := map[string]interface{}{}
	if n := ints["max-connections"]; n > 0 {
		pool["tcp"] = map[string]interface{}{"maxConnections": n}
	}
	if n := ints["max-pending-requests"]; n > 0 {
		pool["http"] = map[string]interface{}{"http1MaxPendingRequests": n}
	}
	if len(pool) > 0 {
		policy["connectionPool"] = pool
	}
	if n := ints["outlier-5xx"]; n > 0 {
		policy["outlierDetection"] = map[string]interface{}{
			"consecutive5xxErrors": n,
			"interval":             "30s",
			"baseEjectionTime":     "30s",
			"maxEjectionPercent":   50,
		}
	}

	return map[string]interface{}{
		"enabled":            true,
		"virtualService":     vs,
		"destinationRule":    map[string]interface{}{"enabled": true, "trafficPolicy": policy},
		"peerAuthentication": map[string]interface{}{"mtlsMode": mtls},
		"authorizationPolicy": map[string]interface{}{
			"action": "ALLOW",
			"rules":  []interface{}{},
		},
	}, nil
}

// istioTemplateBody builds the Istio resources for one Service template.
func istioTemplateBody(svc *resourceTemplate) []string {
	name := svc.name
	meta := func(kind, apiVersion string) []string {
		lines := []string{
			"---",
			"apiVersion: " + apiVersion,
			"kind: " + kind,
			"metadata:",
			"  name: " + name,
			"  namespace: {{ $.Release.Namespace }}",
		}
		return append(lines, svc.labels()...)
	}

	var b []string
	b = append(b, "{{- if $.Values.istio.virtualService.enabled }}")
	b = append(b, meta("VirtualService", "networking.istio.io/v1")...)
	b = append(b,
		"spec:",
		"  hosts:",
		"    - "+name,
		"  http:",
		"    - route:",
		"        - destination:",
		"            host: "+name,
		"      {{- with $.Values.istio.virtualService.timeout }}",
		"      timeout: {{ . }}",
		"      {{- end }}",
		"      {{- with $.Values.istio.virtualService.retries }}",
		"      retries:",
		"        {{- toYaml . | nindent 8 }}",
		"      {{- end }}",
		"{{- end }}",
		"{{- if $.Values.istio.destinationRule.enabled }}",
	)
	b = append(b, meta("DestinationRule", "networking.istio.io/v1")...)
	b = append(b,
		"spec:",
		"  host: "+name,
		"  {{- with $.Values.istio.destinationRule.trafficPolicy }}",
		"  trafficPolicy:",
		"    {{- toYaml . | nindent 4 }}",
		"  {{- end }}",
		"{{- end }}",
	)

	selector := svc.selector()
	if len(selector) < 2 || !balancedControl(selector[1:]) {
		return b // no pod selector: workload-scoped policies are not possible
	}
	matchLabels := append([]string{"  selector:", "    matchLabels:"}, reindent(selector[1:], 2)...)

	b = append(b, "{{- if $.Values.istio.peerAuthentication.mtlsMode }}")
	b = append(b, meta("PeerAuthentication", "security.istio.io/v1")...)
	b = append(b, "spec:")
	b = append(b, matchLabels...)
	b = append(b,
		"  mtls:",
		"    mode: {{ $.Values.istio.peerAuthentication.mtlsMode }}",
		"{{- end }}",
		"{{- if $.Values.istio.authorizationPolicy.rules }}",
	)
	b = append(b, meta("AuthorizationPolicy", "security.istio.io/v1")...)
	b = append(b, "spec:")
	b = append(b, matchLabels...)
	b = append(b,
		`  action: {{ $.Values.istio.authorizationPolicy.action | default "ALLOW" }}`,
		"  rules:",
		"    {{- toYaml $.Values.istio.authorizationPolicy.rules | nindent 4 }}",
		"{{- end }}",
	)
	return b
}

func applyIstioFeature(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
	values, err := istioValues(fc)
	if err != nil {
		return nil, err
	}
	services := resourceTemplates(chart, isKind("Service"))
	if len(services) == 0 {
		return chart, nil
	}
	out := cloneChart(chart)
	for _, svc := range services {
		if labels := svc.labels(); labels != nil && !balancedControl(labels[1:]) {
			continue
		}
		path := featureTemplatePath("istio", svc.path)
		if err := addTemplate(out, path, svc.wrap("$.Values.istio.enabled", istioTemplateBody(svc))); err != nil {
			return nil, err
		}
	}
	if err := addFeatureValues(out, "istio", values); err != nil {
		return nil, err
	}
	return out, nil
}
