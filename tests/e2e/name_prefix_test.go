package e2e

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// namePrefixInput is a small application: a workload with in-chart and
// external references, and the objects that point at it. Names that only
// exist outside the chart (ext-*) must stay literal.
const namePrefixInput = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  labels:
    app: web
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      serviceAccountName: web-sa
      containers:
        - name: web
          image: web:1.0
          envFrom:
            - configMapRef:
                name: app-cfg
            - secretRef:
                name: ext-secret
          env:
            - name: TOKEN
              valueFrom:
                secretKeyRef:
                  name: app-token
                  key: token
          volumeMounts:
            - name: cfg
              mountPath: /etc/cfg
            - name: data
              mountPath: /data
            - name: ext
              mountPath: /ext
      volumes:
        - name: cfg
          configMap:
            name: app-cfg
        - name: data
          persistentVolumeClaim:
            claimName: web-data
        - name: ext
          configMap:
            name: ext-cfg
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-cfg
data:
  k: v
---
apiVersion: v1
kind: Secret
metadata:
  name: app-token
type: Opaque
stringData:
  token: abc
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: web-sa
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: web-data
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 1Gi
---
apiVersion: v1
kind: Service
metadata:
  name: web
spec:
  selector:
    app: web
  ports:
    - port: 80
      targetPort: 8080
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
spec:
  tls:
    - hosts: [web.example.com]
      secretName: web-tls
  rules:
    - host: web.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: web
                port:
                  number: 80
          - path: /ext
            pathType: Prefix
            backend:
              service:
                name: ext-svc
                port:
                  number: 80
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: web
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: web
  minReplicas: 1
  maxReplicas: 3
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: web-role
rules:
  - apiGroups: [""]
    resources: [configmaps]
    verbs: [get]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: web-rb
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: web-role
subjects:
  - kind: ServiceAccount
    name: web-sa
  - kind: ServiceAccount
    name: ext-sa
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: web
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: web
`

type renderedObject struct {
	Kind string
	Name string
	Doc  map[string]interface{}
}

// parseRendered splits helm template output into objects.
func parseRendered(t *testing.T, out string) []renderedObject {
	t.Helper()
	var objs []renderedObject
	for _, doc := range strings.Split(out, "\n---") {
		if strings.TrimSpace(doc) == "" {
			continue
		}
		m := map[string]interface{}{}
		if err := yaml.Unmarshal([]byte(doc), &m); err != nil {
			t.Fatalf("rendered document is not YAML: %v\n%s", err, doc)
		}
		if len(m) == 0 {
			continue
		}
		kind, _ := m["kind"].(string)
		meta, _ := m["metadata"].(map[string]interface{})
		name, _ := meta["name"].(string)
		objs = append(objs, renderedObject{Kind: kind, Name: name, Doc: m})
	}
	return objs
}

func objectNames(objs []renderedObject) []string {
	var out []string
	for _, o := range objs {
		out = append(out, o.Kind+"/"+o.Name)
	}
	sort.Strings(out)
	return out
}

func find(t *testing.T, objs []renderedObject, kind, name string) map[string]interface{} {
	t.Helper()
	for _, o := range objs {
		if o.Kind == kind && o.Name == name {
			return o.Doc
		}
	}
	t.Fatalf("%s/%s not found in %v", kind, name, objectNames(objs))
	return nil
}

// path walks maps and lists: "spec.template.spec.volumes.0.configMap.name".
func path(t *testing.T, doc map[string]interface{}, p string) string {
	t.Helper()
	var cur interface{} = doc
	for _, part := range splitPath(p) {
		switch c := cur.(type) {
		case map[string]interface{}:
			cur = c[part]
		case []interface{}:
			var i int
			if _, err := fmt.Sscanf(part, "%d", &i); err != nil || i >= len(c) {
				t.Fatalf("bad index %q in %s", part, p)
			}
			cur = c[i]
		default:
			t.Fatalf("cannot descend into %v at %q of %s", cur, part, p)
		}
	}
	s, _ := cur.(string)
	return s
}

func TestNamePrefix_TwoReleasesOneNamespace(t *testing.T) {
	testNamePrefixTwoReleases(t, types.OutputModeUniversal)
}

// The Deployment, the Ingress and the HorizontalPodAutoscaler land in
// different subcharts: a reference across them gets the same prefix, which the
// parent's global.namePrefix hands to every subchart.
func TestNamePrefix_TwoReleasesOneNamespace_Umbrella(t *testing.T) {
	testNamePrefixTwoReleases(t, types.OutputModeUmbrella)
}

func testNamePrefixTwoReleases(t *testing.T, mode types.OutputMode) {
	h := NewE2ETestHarness(t)
	h.Mode = mode
	h.WriteInput("app.yaml", namePrefixInput)
	h.GenerateChart("prefixapp")

	render := func(prefix string) []renderedObject {
		values := "global:\n  namePrefix: \"" + prefix + "\"\n"
		return parseRendered(t, h.AssertTemplateSuccess("rel"+strings.TrimSuffix(prefix, "-"), WithTemplateValues(h.WriteCustomValues(values))))
	}

	plain := render("")
	a := render("a-")
	b := render("b-")

	// Default: names exactly as in the input.
	for _, want := range []string{"Deployment/web", "ConfigMap/app-cfg", "Secret/app-token", "ServiceAccount/web-sa",
		"PersistentVolumeClaim/web-data", "Service/web", "Ingress/web", "HorizontalPodAutoscaler/web",
		"Role/web-role", "RoleBinding/web-rb", "PodDisruptionBudget/web"} {
		found := false
		for _, n := range objectNames(plain) {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Errorf("default render lacks %s: %v", want, objectNames(plain))
		}
	}
	dep := find(t, plain, "Deployment", "web")
	if got := path(t, dep, "spec.template.spec.volumes.0.configMap.name"); got != "app-cfg" {
		t.Errorf("default volume configMap = %q, want app-cfg", got)
	}

	// Two releases in one namespace: no (kind, name) twice.
	seen := map[string]string{}
	for rel, objs := range map[string][]renderedObject{"a": a, "b": b} {
		for _, n := range objectNames(objs) {
			if other, dup := seen[n]; dup {
				t.Errorf("%s exists in release %s and %s", n, other, rel)
			}
			seen[n] = rel
		}
	}
	if len(a) != len(plain) || len(b) != len(plain) {
		t.Fatalf("object counts differ: plain %d, a %d, b %d", len(plain), len(a), len(b))
	}

	for _, p := range []string{"a-", "b-"} {
		objs := a
		if p == "b-" {
			objs = b
		}
		dep := find(t, objs, "Deployment", p+"web")
		check := func(what, got, want string) {
			t.Helper()
			if got != want {
				t.Errorf("[%s] %s = %q, want %q", p, what, got, want)
			}
		}
		spec := "spec.template.spec."
		check("serviceAccountName", path(t, dep, spec+"serviceAccountName"), p+"web-sa")
		check("envFrom configMapRef", path(t, dep, spec+"containers.0.envFrom.0.configMapRef.name"), p+"app-cfg")
		check("envFrom external secretRef", path(t, dep, spec+"containers.0.envFrom.1.secretRef.name"), "ext-secret")
		check("env secretKeyRef", path(t, dep, spec+"containers.0.env.0.valueFrom.secretKeyRef.name"), p+"app-token")
		check("volume configMap", path(t, dep, spec+"volumes.0.configMap.name"), p+"app-cfg")
		check("volume pvc", path(t, dep, spec+"volumes.1.persistentVolumeClaim.claimName"), p+"web-data")
		check("volume external configMap", path(t, dep, spec+"volumes.2.configMap.name"), "ext-cfg")

		// Service selects by the labels of the input plus the release, so
		// that it does not select the pods of another release.
		svc := find(t, objs, "Service", p+"web")
		check("service selector", path(t, svc, "spec.selector.app"), "web")
		check("service selector release", path(t, svc, "spec.selector.app\\.kubernetes\\.io/instance"), "rel"+strings.TrimSuffix(p, "-"))

		ing := find(t, objs, "Ingress", p+"web")
		check("ingress backend", path(t, ing, "spec.rules.0.http.paths.0.backend.service.name"), p+"web")
		check("ingress external backend", path(t, ing, "spec.rules.0.http.paths.1.backend.service.name"), "ext-svc")

		hpa := find(t, objs, "HorizontalPodAutoscaler", p+"web")
		check("hpa scaleTargetRef", path(t, hpa, "spec.scaleTargetRef.name"), p+"web")

		rb := find(t, objs, "RoleBinding", p+"web-rb")
		check("roleRef", path(t, rb, "roleRef.name"), p+"web-role")
		check("subject in chart", path(t, rb, "subjects.0.name"), p+"web-sa")
		check("subject external", path(t, rb, "subjects.1.name"), "ext-sa")

		pdb := find(t, objs, "PodDisruptionBudget", p+"web")
		check("pdb selector", path(t, pdb, "spec.selector.matchLabels.app"), "web")
		check("pdb selector release", path(t, pdb, "spec.selector.matchLabels.app\\.kubernetes\\.io/instance"), "rel"+strings.TrimSuffix(p, "-"))
	}

	// The selectors of one release must not select the pods of the other,
	// and must select its own.
	podLabels := func(objs []renderedObject, prefix string) map[string]interface{} {
		dep := find(t, objs, "Deployment", prefix+"web")
		return mapAt(t, dep, "spec.template.metadata.labels")
	}
	for _, tc := range []struct {
		name         string
		mine, theirs []renderedObject
		prefix       string
		other        string
	}{{"a", a, b, "a-", "b-"}, {"b", b, a, "b-", "a-"}} {
		own, foreign := podLabels(tc.mine, tc.prefix), podLabels(tc.theirs, tc.other)
		svc := mapAt(t, find(t, tc.mine, "Service", tc.prefix+"web"), "spec.selector")
		pdb := mapAt(t, find(t, tc.mine, "PodDisruptionBudget", tc.prefix+"web"), "spec.selector.matchLabels")
		for what, sel := range map[string]map[string]interface{}{"Service": svc, "PodDisruptionBudget": pdb} {
			if !selects(sel, own) {
				t.Errorf("[%s] %s selector %v does not select its own pods %v", tc.name, what, sel, own)
			}
			if selects(sel, foreign) {
				t.Errorf("[%s] %s selector %v selects the pods of the other release %v", tc.name, what, sel, foreign)
			}
		}
	}
}

// mapAt walks to the map at the dotted path ("\\." is a dot inside a key).
func mapAt(t *testing.T, doc map[string]interface{}, p string) map[string]interface{} {
	t.Helper()
	var cur interface{} = doc
	for _, part := range splitPath(p) {
		m, ok := cur.(map[string]interface{})
		if !ok {
			t.Fatalf("cannot descend into %v at %q of %s", cur, part, p)
		}
		cur = m[part]
	}
	m, ok := cur.(map[string]interface{})
	if !ok {
		t.Fatalf("%s is not a map: %v", p, cur)
	}
	return m
}

// splitPath splits on dots, except those escaped with a backslash.
func splitPath(p string) []string {
	var parts []string
	var cur strings.Builder
	for i := 0; i < len(p); i++ {
		switch {
		case p[i] == '\\' && i+1 < len(p) && p[i+1] == '.':
			cur.WriteByte('.')
			i++
		case p[i] == '.':
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(p[i])
		}
	}
	return append(parts, cur.String())
}

// selects reports whether a label selector (matchLabels form) matches labels.
func selects(selector, labels map[string]interface{}) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// References inside the spec of an object without a processor of its own
// (the generic spec overlay) are prefixed too.
func TestNamePrefix_SpecReferences(t *testing.T) {
	h := NewE2ETestHarness(t)
	h.WriteInput("app.yaml", `
apiVersion: v1
kind: Service
metadata:
  name: web
spec:
  selector:
    app: web
  ports:
    - port: 80
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: edge
spec:
  gatewayClassName: nginx
  listeners:
    - name: http
      port: 80
      protocol: HTTP
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: web
spec:
  parentRefs:
    - name: edge
    - name: shared-gw
  rules:
    - backendRefs:
        - name: web
          port: 80
        - name: ext-svc
          port: 80
---
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: ca
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: web
spec:
  secretName: web-cert
  dnsNames: [web.example.com]
  issuerRef:
    kind: ClusterIssuer
    name: ca
`)
	h.GenerateChart("specrefs")
	values := h.WriteCustomValues("global:\n  namePrefix: \"a-\"\n")
	objs := parseRendered(t, h.AssertTemplateSuccess("rel", WithTemplateValues(values)))

	route := find(t, objs, "HTTPRoute", "a-web")
	if got := path(t, route, "spec.parentRefs.0.name"); got != "a-edge" {
		t.Errorf("parentRef in chart = %q, want a-edge", got)
	}
	if got := path(t, route, "spec.parentRefs.1.name"); got != "shared-gw" {
		t.Errorf("parentRef outside the chart = %q, want shared-gw", got)
	}
	if got := path(t, route, "spec.rules.0.backendRefs.0.name"); got != "a-web" {
		t.Errorf("backendRef in chart = %q, want a-web", got)
	}
	if got := path(t, route, "spec.rules.0.backendRefs.1.name"); got != "ext-svc" {
		t.Errorf("backendRef outside the chart = %q, want ext-svc", got)
	}
	cert := find(t, objs, "Certificate", "a-web")
	if got := path(t, cert, "spec.issuerRef.name"); got != "a-ca" {
		t.Errorf("issuerRef = %q, want a-ca", got)
	}
	find(t, objs, "ClusterIssuer", "a-ca")
}

// References that are not in a pod spec: the class of an Ingress, the class
// of a claim, the service a webhook calls and the service account a token
// Secret belongs to.
func TestNamePrefix_ClassWebhookAndTokenReferences(t *testing.T) {
	h := NewE2ETestHarness(t)
	h.WriteInput("app.yaml", `
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata:
  name: myclass
spec:
  controller: example.com/ingress-controller
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
spec:
  ingressClassName: myclass
  rules:
    - host: web.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: web
                port:
                  number: 80
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: other
spec:
  ingressClassName: nginx
  rules:
    - host: other.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: web
                port:
                  number: 80
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: fast
provisioner: example.com/disk
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: web-data
spec:
  storageClassName: fast
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 1Gi
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ext-data
spec:
  storageClassName: standard
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 1Gi
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: db
spec:
  serviceName: db
  replicas: 1
  selector:
    matchLabels:
      app: db
  template:
    metadata:
      labels:
        app: db
    spec:
      containers:
        - name: db
          image: db:1.0
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        storageClassName: fast
        accessModes: [ReadWriteOnce]
        resources:
          requests:
            storage: 1Gi
---
apiVersion: v1
kind: Service
metadata:
  name: web
spec:
  selector:
    app: web
  ports:
    - port: 443
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: checks
webhooks:
  - name: checks.example.com
    admissionReviewVersions: [v1]
    sideEffects: None
    clientConfig:
      service:
        name: web
        namespace: default
        path: /validate
  - name: ext.example.com
    admissionReviewVersions: [v1]
    sideEffects: None
    clientConfig:
      service:
        name: ext-hook
        namespace: default
        path: /validate
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: web
---
apiVersion: v1
kind: Secret
metadata:
  name: web-token
  annotations:
    kubernetes.io/service-account.name: web
type: kubernetes.io/service-account-token
`)
	h.GenerateChart("classrefs")
	values := h.WriteCustomValues("global:\n  namePrefix: \"a-\"\n")
	objs := parseRendered(t, h.AssertTemplateSuccess("rel", WithTemplateValues(values)))

	check := func(what, got, want string) {
		t.Helper()
		if got != want {
			t.Errorf("%s = %q, want %q", what, got, want)
		}
	}
	find(t, objs, "IngressClass", "a-myclass")
	find(t, objs, "StorageClass", "a-fast")
	check("ingressClassName in chart", path(t, find(t, objs, "Ingress", "a-web"), "spec.ingressClassName"), "a-myclass")
	check("ingressClassName outside the chart", path(t, find(t, objs, "Ingress", "a-other"), "spec.ingressClassName"), "nginx")
	check("storageClassName in chart", path(t, find(t, objs, "PersistentVolumeClaim", "a-web-data"), "spec.storageClassName"), "a-fast")
	check("storageClassName outside the chart", path(t, find(t, objs, "PersistentVolumeClaim", "a-ext-data"), "spec.storageClassName"), "standard")
	check("volumeClaimTemplates storageClassName", path(t, find(t, objs, "StatefulSet", "a-db"), "spec.volumeClaimTemplates.0.spec.storageClassName"), "a-fast")
	hook := find(t, objs, "ValidatingWebhookConfiguration", "a-checks")
	check("webhook service in chart", path(t, hook, "webhooks.0.clientConfig.service.name"), "a-web")
	check("webhook service outside the chart", path(t, hook, "webhooks.1.clientConfig.service.name"), "ext-hook")
	check("token secret service account", path(t, find(t, objs, "Secret", "a-web-token"), "metadata.annotations.kubernetes\\.io/service-account\\.name"), "a-web")

	// Default: the input, unchanged.
	plain := parseRendered(t, h.AssertTemplateSuccess("rel"))
	check("default ingressClassName", path(t, find(t, plain, "Ingress", "web"), "spec.ingressClassName"), "myclass")
	check("default storageClassName", path(t, find(t, plain, "PersistentVolumeClaim", "web-data"), "spec.storageClassName"), "fast")
	check("default webhook service", path(t, find(t, plain, "ValidatingWebhookConfiguration", "checks"), "webhooks.0.clientConfig.service.name"), "web")
	check("default token secret service account", path(t, find(t, plain, "Secret", "web-token"), "metadata.annotations.kubernetes\\.io/service-account\\.name"), "web")
}

// The objects the optional features add (Gateway API translation, golden
// signals) are named by the feature, not by the input: they get the prefix
// too, and the references among them follow.
func TestNamePrefix_Features(t *testing.T) {
	h := NewE2ETestHarness(t)
	h.Features = []string{"gateway-api", "golden-signals"}
	h.WriteInput("app.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
        - name: web
          image: web:1.0
---
apiVersion: v1
kind: Service
metadata:
  name: web
spec:
  selector:
    app: web
  ports:
    - port: 80
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
spec:
  tls:
    - hosts: [web.example.com]
      secretName: web-tls
  rules:
    - host: web.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: web
                port:
                  number: 80
`)
	h.GenerateChart("featapp")

	render := func(prefix, release string) []renderedObject {
		values := h.WriteCustomValues("global:\n  namePrefix: \"" + prefix + "\"\n")
		return parseRendered(t, h.AssertTemplateSuccess(release, WithTemplateValues(values)))
	}
	plain := parseRendered(t, h.AssertTemplateSuccess("rel"))
	a, b := render("a-", "rela"), render("b-", "relb")

	// Default names are the ones the features always used.
	route := find(t, plain, "HTTPRoute", "web")
	if got := path(t, route, "spec.rules.0.backendRefs.0.name"); got != "web" {
		t.Errorf("default backendRef = %q, want web", got)
	}
	find(t, plain, "Gateway", "web-gateway")
	find(t, plain, "ConfigMap", "golden-signals-dashboard")

	// Two releases in one namespace: no (kind, name) twice.
	seen := map[string]bool{}
	for _, n := range objectNames(a) {
		seen[n] = true
	}
	for _, n := range objectNames(b) {
		if seen[n] {
			t.Errorf("%s exists in both releases", n)
		}
	}
	if len(a) != len(plain) {
		t.Errorf("object counts differ: plain %d, a %d", len(a), len(plain))
	}

	for _, tc := range []struct {
		objs   []renderedObject
		prefix string
	}{{a, "a-"}, {b, "b-"}} {
		p := tc.prefix
		route := find(t, tc.objs, "HTTPRoute", p+"web")
		if got := path(t, route, "spec.rules.0.backendRefs.0.name"); got != p+"web" {
			t.Errorf("[%s] backendRef = %q, want %sweb", p, got, p)
		}
		find(t, tc.objs, "Service", p+"web")
		find(t, tc.objs, "Gateway", p+"web-gateway")
		if got := path(t, route, "spec.parentRefs.0.name"); got != p+"web-gateway" {
			t.Errorf("[%s] parentRef = %q, want %sweb-gateway", p, got, p)
		}
		find(t, tc.objs, "ConfigMap", p+"golden-signals-dashboard")
		find(t, tc.objs, "PrometheusRule", p+"golden-signals-rules")
	}
}

// A library chart is published once and shared: charts of two different
// applications render with the library of the first one, and the references
// among their own objects still get the prefix.
func TestNamePrefix_LibraryOfAnotherApplication(t *testing.T) {
	libraryOf := func(manifests string) (*E2ETestHarness, *types.GeneratedChart) {
		h := NewE2ETestHarness(t)
		h.Mode = types.OutputModeLibrary
		h.WriteInput("app.yaml", manifests)
		h.GenerateChart("shop")
		return h, h.Charts[0]
	}

	_, lib1 := libraryOf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: solo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: solo
  template:
    metadata:
      labels:
        app: solo
    spec:
      containers:
        - name: solo
          image: solo:1.0
`)
	h2, lib2 := libraryOf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  replicas: 1
  selector:
    matchLabels:
      app: api
  template:
    metadata:
      labels:
        app: api
    spec:
      containers:
        - name: api
          image: api:1.0
          envFrom:
            - configMapRef:
                name: api-cfg
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: api-cfg
data:
  k: v
`)
	if lib1.Helpers != lib2.Helpers {
		t.Fatal("two applications produce different libraries")
	}

	// Render the second application's chart with the first one's library.
	var service *types.GeneratedChart
	for _, c := range h2.Charts[1:] {
		service = c
	}
	if service == nil {
		t.Fatal("no service chart")
	}
	libHelpers := h2.ChartPathIn(service.Name, "charts", lib2.Name, "templates", "_helpers.tpl")
	if err := os.WriteFile(libHelpers, []byte(lib1.Helpers), 0o644); err != nil {
		t.Fatal(err)
	}
	h2.ChartDir = h2.ChartPathIn(service.Name)
	h2.Chart = service

	values := h2.WriteCustomValues("global:\n  namePrefix: \"a-\"\n")
	objs := parseRendered(t, h2.AssertTemplateSuccess("rel", WithTemplateValues(values)))
	find(t, objs, "ConfigMap", "a-api-cfg")
	dep := find(t, objs, "Deployment", "a-api")
	if got := path(t, dep, "spec.template.spec.containers.0.envFrom.0.configMapRef.name"); got != "a-api-cfg" {
		t.Errorf("envFrom configMapRef = %q, want a-api-cfg", got)
	}
}
