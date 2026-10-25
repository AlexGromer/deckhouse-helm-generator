package generator

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type annotationResult struct {
	RuleFilters      []map[string]interface{} // appended to every rule that has backendRefs
	PathRewrite      func(p IngressPath) (match map[string]interface{}, filters []map[string]interface{}, ok bool)
	ExtraRules       []map[string]interface{} // app-root, redirects (first in the route)
	ForceRegex       bool                     // use-regex
	GRPC             bool                     // backend-protocol GRPC/GRPCS
	SSLRedirect      *bool                    // nil = controller default
	RouteAnnotations map[string]string        // implementation=alb
	Extended         []string                 // Extended features used, for the Note
}

// parseNginxSize parses an nginx size string ("100", "1k", "2M", "1G") into bytes.
// Returns (0, false) for "0", negative values, or invalid strings.
func parseNginxSize(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, false
	}
	mult := int64(1)
	last := s[len(s)-1]
	switch last {
	case 'k', 'K':
		mult = 1024
		s = s[:len(s)-1]
	case 'm', 'M':
		mult = 1024 * 1024
		s = s[:len(s)-1]
	case 'g', 'G':
		mult = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n * mult, true
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func corsFilter(a map[string]string) map[string]interface{} {
	origins := splitCSV(a["nginx.ingress.kubernetes.io/cors-allow-origin"])
	if len(origins) == 0 {
		origins = []string{"*"}
	}
	methods := splitCSV(a["nginx.ingress.kubernetes.io/cors-allow-methods"])
	if len(methods) == 0 {
		methods = []string{"GET", "PUT", "POST", "DELETE", "PATCH", "OPTIONS"}
	}
	headers := splitCSV(a["nginx.ingress.kubernetes.io/cors-allow-headers"])
	if len(headers) == 0 {
		headers = []string{"DNT", "Keep-Alive", "User-Agent", "X-Requested-With", "If-Modified-Since", "Cache-Control", "Content-Type", "Range", "Authorization"}
	}
	expose := splitCSV(a["nginx.ingress.kubernetes.io/cors-expose-headers"])

	allowCreds := true
	if raw, ok := a["nginx.ingress.kubernetes.io/cors-allow-credentials"]; ok {
		allowCreds = strings.EqualFold(strings.TrimSpace(raw), "true")
	}
	maxAge := 1728000
	if raw, ok := a["nginx.ingress.kubernetes.io/cors-max-age"]; ok {
		if v, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && v >= 0 {
			maxAge = v
		}
	}

	corsMap := map[string]interface{}{
		"allowOrigins":     origins,
		"allowMethods":     methods,
		"allowHeaders":     headers,
		"allowCredentials": allowCreds,
		"maxAge":           maxAge,
	}
	if len(expose) > 0 {
		corsMap["exposeHeaders"] = expose
	}
	return map[string]interface{}{
		"type": "CORS",
		"cors": corsMap,
	}
}

func redirectFilter(rawURL string, code int) (map[string]interface{}, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme == "" && u.Host == "" && u.Path == "") {
		return nil, fmt.Errorf("invalid redirect URL %q", rawURL)
	}
	rr := map[string]interface{}{
		"statusCode": code,
	}
	if u.Scheme == "http" || u.Scheme == "https" {
		rr["scheme"] = u.Scheme
	}
	if h := u.Hostname(); h != "" {
		rr["hostname"] = h
	}
	if p := u.Port(); p != "" {
		if portNum, err := strconv.Atoi(p); err == nil && portNum > 0 && portNum <= 65535 {
			rr["port"] = portNum
		}
	}
	if u.Path != "" {
		rr["path"] = map[string]interface{}{
			"type":            "ReplaceFullPath",
			"replaceFullPath": u.Path,
		}
	}
	return map[string]interface{}{
		"type":            "RequestRedirect",
		"requestRedirect": rr,
	}, nil
}

var rewriteCaptureRe = regexp.MustCompile(`\$\d+`)
var rewritePrefixRegexRe = regexp.MustCompile(`^(/[^()*+?\\[\]{}|^$]+)\(/\|\$\)\(\.\*\)$`)

func nginxAnnotations(f IngressFacts, opts GatewayAPIOptions, note func(string, ...interface{})) annotationResult {
	var res annotationResult
	if len(f.Annotations) == 0 {
		return res
	}
	if f.Controller != ControllerNginx {
		var keys []string
		for k := range f.Annotations {
			if strings.HasPrefix(k, "nginx.ingress.kubernetes.io/") {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			note("annotation %s ignored because controller is %s", k, f.Controller)
		}
		return res
	}

	a := f.Annotations
	handled := map[string]bool{}
	isALB := opts.Implementation == "alb"
	albAnnotations := map[string]string{}

	addExtended := func(feat string) {
		for _, ex := range res.Extended {
			if ex == feat {
				return
			}
		}
		res.Extended = append(res.Extended, feat)
	}

	// ssl-redirect / force-ssl-redirect
	if v, ok := a["nginx.ingress.kubernetes.io/ssl-redirect"]; ok {
		handled["nginx.ingress.kubernetes.io/ssl-redirect"] = true
		b := !strings.EqualFold(strings.TrimSpace(v), "false")
		res.SSLRedirect = &b
	}
	if v, ok := a["nginx.ingress.kubernetes.io/force-ssl-redirect"]; ok {
		handled["nginx.ingress.kubernetes.io/force-ssl-redirect"] = true
		if strings.EqualFold(strings.TrimSpace(v), "true") {
			b := true
			res.SSLRedirect = &b
		}
	}

	// use-regex
	if v, ok := a["nginx.ingress.kubernetes.io/use-regex"]; ok {
		handled["nginx.ingress.kubernetes.io/use-regex"] = true
		if strings.EqualFold(strings.TrimSpace(v), "true") {
			res.ForceRegex = true
			note("nginx.ingress.kubernetes.io/use-regex: case-insensitive regex matching is not portable in Gateway API")
		}
	}

	// backend-protocol
	if v, ok := a["nginx.ingress.kubernetes.io/backend-protocol"]; ok {
		handled["nginx.ingress.kubernetes.io/backend-protocol"] = true
		switch strings.ToUpper(strings.TrimSpace(v)) {
		case "GRPC":
			res.GRPC = true
		case "GRPCS":
			res.GRPC = true
			note("nginx.ingress.kubernetes.io/backend-protocol=GRPCS: BackendTLSPolicy is not generated automatically")
		case "HTTPS":
			note("nginx.ingress.kubernetes.io/backend-protocol=HTTPS: BackendTLSPolicy is not generated automatically")
		case "HTTP":
			// default
		default:
			note("nginx.ingress.kubernetes.io/backend-protocol=%s has no Gateway API equivalent", v)
		}
	}

	// app-root
	if v, ok := a["nginx.ingress.kubernetes.io/app-root"]; ok && strings.TrimSpace(v) != "" {
		handled["nginx.ingress.kubernetes.io/app-root"] = true
		appRoot := strings.TrimSpace(v)
		if strings.HasPrefix(appRoot, "/") {
			res.ExtraRules = append(res.ExtraRules, map[string]interface{}{
				"matches": []interface{}{
					map[string]interface{}{
						"path": map[string]interface{}{
							"type":  "Exact",
							"value": "/",
						},
					},
				},
				"filters": []interface{}{
					map[string]interface{}{
						"type": "RequestRedirect",
						"requestRedirect": map[string]interface{}{
							"path": map[string]interface{}{
								"type":            "ReplaceFullPath",
								"replaceFullPath": appRoot,
							},
							"statusCode": 302,
						},
					},
				},
			})
			addExtended("RequestRedirect.path")
		} else {
			note("nginx.ingress.kubernetes.io/app-root=%q must start with '/'", appRoot)
		}
	}

	// temporal-redirect / permanent-redirect
	redirURL := ""
	redirCode := 0
	if v, ok := a["nginx.ingress.kubernetes.io/temporal-redirect"]; ok && strings.TrimSpace(v) != "" {
		handled["nginx.ingress.kubernetes.io/temporal-redirect"] = true
		handled["nginx.ingress.kubernetes.io/temporal-redirect-code"] = true
		handled["nginx.ingress.kubernetes.io/permanent-redirect"] = true
		handled["nginx.ingress.kubernetes.io/permanent-redirect-code"] = true
		redirURL = strings.TrimSpace(v)
		redirCode = 302
		if cStr, ok := a["nginx.ingress.kubernetes.io/temporal-redirect-code"]; ok {
			if c, err := strconv.Atoi(strings.TrimSpace(cStr)); err == nil && isValidRedirectCode(c) {
				redirCode = c
			} else {
				note("nginx.ingress.kubernetes.io/temporal-redirect-code=%s is invalid; using 302", cStr)
			}
		}
	} else if v, ok := a["nginx.ingress.kubernetes.io/permanent-redirect"]; ok && strings.TrimSpace(v) != "" {
		handled["nginx.ingress.kubernetes.io/permanent-redirect"] = true
		handled["nginx.ingress.kubernetes.io/permanent-redirect-code"] = true
		redirURL = strings.TrimSpace(v)
		redirCode = 301
		if cStr, ok := a["nginx.ingress.kubernetes.io/permanent-redirect-code"]; ok {
			if c, err := strconv.Atoi(strings.TrimSpace(cStr)); err == nil && isValidRedirectCode(c) {
				redirCode = c
			} else {
				note("nginx.ingress.kubernetes.io/permanent-redirect-code=%s is invalid; using 301", cStr)
			}
		}
	}
	if redirURL != "" {
		if flt, err := redirectFilter(redirURL, redirCode); err == nil {
			res.ExtraRules = append(res.ExtraRules, map[string]interface{}{
				"filters": []interface{}{flt},
			})
			addExtended("RequestRedirect")
		} else {
			note("invalid redirect URL %q: %v", redirURL, err)
		}
	}

	// rewrite-target & upstream-vhost & x-forwarded-prefix
	upstreamVhost := strings.TrimSpace(a["nginx.ingress.kubernetes.io/upstream-vhost"])
	if upstreamVhost != "" {
		handled["nginx.ingress.kubernetes.io/upstream-vhost"] = true
	}
	xForwardedPrefix := strings.TrimSpace(a["nginx.ingress.kubernetes.io/x-forwarded-prefix"])
	if xForwardedPrefix != "" {
		handled["nginx.ingress.kubernetes.io/x-forwarded-prefix"] = true
	}
	if rwTarget, ok := a["nginx.ingress.kubernetes.io/rewrite-target"]; ok {
		handled["nginx.ingress.kubernetes.io/rewrite-target"] = true
		rwTarget = strings.TrimSpace(rwTarget)
		hasCaptures := rewriteCaptureRe.MatchString(rwTarget)

		if !hasCaptures && !res.ForceRegex {
			urlRewrite := map[string]interface{}{
				"path": map[string]interface{}{
					"type":            "ReplaceFullPath",
					"replaceFullPath": rwTarget,
				},
			}
			if upstreamVhost != "" {
				urlRewrite["hostname"] = upstreamVhost
				upstreamVhost = ""
			}
			res.RuleFilters = append(res.RuleFilters, map[string]interface{}{
				"type":       "URLRewrite",
				"urlRewrite": urlRewrite,
			})
			addExtended("URLRewrite")
		} else if rwTarget == "/$2" {
			res.PathRewrite = func(p IngressPath) (map[string]interface{}, []map[string]interface{}, bool) {
				if m := rewritePrefixRegexRe.FindStringSubmatch(p.Path); m != nil {
					prefix := m[1]
					if prefix == "" {
						prefix = m[2]
					}
					match := map[string]interface{}{
						"path": map[string]interface{}{
							"type":  "PathPrefix",
							"value": prefix,
						},
					}
					urlRewrite := map[string]interface{}{
						"path": map[string]interface{}{
							"type":               "ReplacePrefixMatch",
							"replacePrefixMatch": "/",
						},
					}
					if upstreamVhost != "" {
						urlRewrite["hostname"] = upstreamVhost
					}
					filters := []map[string]interface{}{
						{
							"type":       "URLRewrite",
							"urlRewrite": urlRewrite,
						},
					}
					return match, filters, true
				}
				return nil, nil, false
			}
			addExtended("URLRewrite")
		} else if isALB {
			albAnnotations["alb.network.deckhouse.io/rewrite-target"] = rewriteCaptureRe.ReplaceAllStringFunc(rwTarget, func(m string) string {
				return `\` + m[1:]
			})
			res.ForceRegex = true
		} else {
			note("nginx.ingress.kubernetes.io/rewrite-target=%q uses capture groups not supported in standard Gateway API", rwTarget)
		}

		if xForwardedPrefix != "" {
			res.RuleFilters = append(res.RuleFilters, map[string]interface{}{
				"type": "RequestHeaderModifier",
				"requestHeaderModifier": map[string]interface{}{
					"set": []interface{}{
						map[string]interface{}{
							"name":  "X-Forwarded-Prefix",
							"value": xForwardedPrefix,
						},
					},
				},
			})
		}
	}
	if upstreamVhost != "" {
		res.RuleFilters = append(res.RuleFilters, map[string]interface{}{
			"type": "URLRewrite",
			"urlRewrite": map[string]interface{}{
				"hostname": upstreamVhost,
			},
		})
		addExtended("URLRewrite")
	}

	// enable-cors
	if v, ok := a["nginx.ingress.kubernetes.io/enable-cors"]; ok {
		handled["nginx.ingress.kubernetes.io/enable-cors"] = true
		for _, k := range []string{
			"nginx.ingress.kubernetes.io/cors-allow-origin",
			"nginx.ingress.kubernetes.io/cors-allow-methods",
			"nginx.ingress.kubernetes.io/cors-allow-headers",
			"nginx.ingress.kubernetes.io/cors-expose-headers",
			"nginx.ingress.kubernetes.io/cors-allow-credentials",
			"nginx.ingress.kubernetes.io/cors-max-age",
		} {
			handled[k] = true
		}
		if strings.EqualFold(strings.TrimSpace(v), "true") {
			if opts.CORS {
				res.RuleFilters = append(res.RuleFilters, corsFilter(a))
				addExtended("CORS")
			} else {
				note("nginx.ingress.kubernetes.io/enable-cors=true skipped because cors=false")
			}
		}
	}

	// Canary annotations are handled by translateIngress
	for _, k := range []string{
		"nginx.ingress.kubernetes.io/canary",
		"nginx.ingress.kubernetes.io/canary-weight",
		"nginx.ingress.kubernetes.io/canary-weight-total",
		"nginx.ingress.kubernetes.io/canary-by-header",
		"nginx.ingress.kubernetes.io/canary-by-header-value",
		"nginx.ingress.kubernetes.io/canary-by-header-pattern",
		"nginx.ingress.kubernetes.io/canary-by-cookie",
	} {
		if _, ok := a[k]; ok {
			handled[k] = true
		}
	}

	// ALB profile annotations vs standard notes
	// Timeouts
	readTimeout, hasRead := a["nginx.ingress.kubernetes.io/proxy-read-timeout"]
	sendTimeout, hasSend := a["nginx.ingress.kubernetes.io/proxy-send-timeout"]
	_, hasConn := a["nginx.ingress.kubernetes.io/proxy-connect-timeout"]
	if hasRead || hasSend || hasConn {
		handled["nginx.ingress.kubernetes.io/proxy-read-timeout"] = true
		handled["nginx.ingress.kubernetes.io/proxy-send-timeout"] = true
		handled["nginx.ingress.kubernetes.io/proxy-connect-timeout"] = true
		if isALB && (hasRead || hasSend) {
			rSec, _ := strconv.Atoi(strings.TrimSpace(readTimeout))
			sSec, _ := strconv.Atoi(strings.TrimSpace(sendTimeout))
			maxSec := rSec
			if sSec > maxSec {
				maxSec = sSec
			}
			if maxSec > 0 {
				albAnnotations["alb.network.deckhouse.io/idle-timeout"] = fmt.Sprintf("%ds", maxSec)
			}
		} else {
			note("proxy-* timeout annotations are not converted automatically")
		}
	}

	// Direct ALB passthroughs
	directALB := map[string]string{
		"nginx.ingress.kubernetes.io/whitelist-source-range": "alb.network.deckhouse.io/whitelist-source-range",
		"nginx.ingress.kubernetes.io/limit-rps":              "alb.network.deckhouse.io/limit-rps",
		"nginx.ingress.kubernetes.io/proxy-buffer-size":      "alb.network.deckhouse.io/proxy-buffer-size",
		"nginx.ingress.kubernetes.io/auth-url":               "alb.network.deckhouse.io/auth-url",
		"nginx.ingress.kubernetes.io/auth-signin":            "alb.network.deckhouse.io/auth-signin",
		"nginx.ingress.kubernetes.io/auth-response-headers":  "alb.network.deckhouse.io/auth-response-headers",
	}
	for ngKey, albKey := range directALB {
		if v, ok := a[ngKey]; ok {
			handled[ngKey] = true
			if isALB {
				albAnnotations[albKey] = v
			} else {
				note("%s has no standard Gateway API equivalent", ngKey)
			}
		}
	}

	if v, ok := a["nginx.ingress.kubernetes.io/service-upstream"]; ok {
		handled["nginx.ingress.kubernetes.io/service-upstream"] = true
		if isALB && strings.EqualFold(strings.TrimSpace(v), "true") {
			albAnnotations["alb.network.deckhouse.io/service-upstream"] = "true"
		} else if !isALB {
			note("nginx.ingress.kubernetes.io/service-upstream has no standard Gateway API equivalent")
		}
	}

	if v, ok := a["nginx.ingress.kubernetes.io/proxy-body-size"]; ok {
		handled["nginx.ingress.kubernetes.io/proxy-body-size"] = true
		if isALB {
			if bytes, ok := parseNginxSize(v); ok {
				albAnnotations["alb.network.deckhouse.io/buffer-max-request-bytes"] = strconv.FormatInt(bytes, 10)
			}
		} else {
			note("nginx.ingress.kubernetes.io/proxy-body-size has no standard Gateway API equivalent")
		}
	}

	if v, ok := a["nginx.ingress.kubernetes.io/affinity"]; ok {
		handled["nginx.ingress.kubernetes.io/affinity"] = true
		handled["nginx.ingress.kubernetes.io/session-cookie-name"] = true
		handled["nginx.ingress.kubernetes.io/session-cookie-path"] = true
		if isALB && strings.EqualFold(strings.TrimSpace(v), "cookie") {
			aff := map[string]string{"mode": "cookie"}
			if cn := strings.TrimSpace(a["nginx.ingress.kubernetes.io/session-cookie-name"]); cn != "" {
				aff["cookieName"] = cn
			}
			if cp := strings.TrimSpace(a["nginx.ingress.kubernetes.io/session-cookie-path"]); cp != "" {
				aff["path"] = cp
			}
			if b, err := json.Marshal(aff); err == nil {
				albAnnotations["alb.network.deckhouse.io/session-affinity"] = string(b)
			}
		} else {
			note("nginx.ingress.kubernetes.io/affinity: sessionPersistence is experimental in Gateway API")
		}
	}

	if v, ok := a["nginx.ingress.kubernetes.io/auth-type"]; ok {
		handled["nginx.ingress.kubernetes.io/auth-type"] = true
		handled["nginx.ingress.kubernetes.io/auth-secret"] = true
		secret := strings.TrimSpace(a["nginx.ingress.kubernetes.io/auth-secret"])
		if isALB && strings.EqualFold(strings.TrimSpace(v), "basic") && secret != "" {
			if !strings.Contains(secret, "/") && f.Namespace != "" {
				secret = f.Namespace + "/" + secret
			}
			albAnnotations["alb.network.deckhouse.io/basic-auth-secret"] = secret
		} else {
			note("nginx.ingress.kubernetes.io/auth-type has no standard Gateway API equivalent")
		}
	}

	if len(albAnnotations) > 0 {
		res.RouteAnnotations = albAnnotations
	}

	// Report any unhandled nginx.ingress.kubernetes.io/* annotations deterministically
	var unhandled []string
	for k := range a {
		if strings.HasPrefix(k, "nginx.ingress.kubernetes.io/") && !handled[k] {
			unhandled = append(unhandled, k)
		}
	}
	sort.Strings(unhandled)
	for _, k := range unhandled {
		note("annotation %s has no Gateway API equivalent", k)
	}

	return res
}

func isValidRedirectCode(c int) bool {
	switch c {
	case 301, 302, 303, 307, 308:
		return true
	}
	return false
}
