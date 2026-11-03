package golden

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// sharedInput: web and api share the ConfigMap common and the ServiceAccount
// runner; web has a ConfigMap of its own, api a Secret of its own; the Secret
// spare is referenced by no workload.
const sharedInput = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  labels:
    app.kubernetes.io/name: web
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
      serviceAccountName: runner
      containers:
        - name: web
          image: web:1.0
          envFrom:
            - configMapRef:
                name: common
            - configMapRef:
                name: web-own
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  labels:
    app.kubernetes.io/name: api
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
      serviceAccountName: runner
      containers:
        - name: api
          image: api:1.0
          envFrom:
            - configMapRef:
                name: common
          env:
            - name: TOKEN
              valueFrom:
                secretKeyRef:
                  name: api-token
                  key: token
---
apiVersion: v1
kind: Service
metadata:
  name: web
  labels:
    app.kubernetes.io/name: web
spec:
  selector:
    app: web
  ports:
    - port: 80
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: common
data:
  LOG_LEVEL: info
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: web-own
  labels:
    app.kubernetes.io/name: web
data:
  MODE: web
---
apiVersion: v1
kind: Secret
metadata:
  name: api-token
  labels:
    app.kubernetes.io/name: api
type: Opaque
stringData:
  token: abc
---
apiVersion: v1
kind: Secret
metadata:
  name: spare
type: Opaque
stringData:
  key: value
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: runner
`

// renderChart renders one chart directory of out.
func renderChart(t *testing.T, helm, out, chart string, extra ...string) []object {
	t.Helper()
	args := append([]string{"template", "golden", filepath.Join(out, chart)}, extra...)
	stream, err := run(helm, args...)
	if err != nil {
		t.Fatalf("helm template %s failed: %v\n%s", chart, err, stream)
	}
	return parseStream(stream)
}

func objectIDs(objs []object) []string {
	var ids []string
	for _, o := range objs {
		ids = append(ids, o.kind()+"/"+o.name())
	}
	sort.Strings(ids)
	return ids
}

func sameIDs(t *testing.T, what string, got []object, want ...string) {
	t.Helper()
	sort.Strings(want)
	if g := strings.Join(objectIDs(got), " "); g != strings.Join(want, " ") {
		t.Errorf("%s: %s\nwant %s", what, g, strings.Join(want, " "))
	}
}

func TestSharedChart(t *testing.T) {
	helm := requireHelm(t)
	for _, mode := range []string{"separate", "library"} {
		for _, viaConfig := range []bool{false, true} {
			name := mode
			if viaConfig {
				name += "-config-file"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				dir := t.TempDir()
				in := filepath.Join(dir, "in")
				if err := os.MkdirAll(in, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(in, "app.yaml"), []byte(sharedInput), 0o644); err != nil {
					t.Fatal(err)
				}
				out := filepath.Join(dir, "out")
				args := []string{"generate", "-f", in, "-o", out, "--chart-name", "app", "--mode", mode, "--include-schema"}
				if viaConfig {
					cfg := filepath.Join(dir, "dhg.yaml")
					if err := os.WriteFile(cfg, []byte("shared-chart: platform\n"), 0o644); err != nil {
						t.Fatal(err)
					}
					args = append(args, "--config", cfg)
				} else {
					args = append(args, "--shared-chart", "platform")
				}
				args = append(args, "-v")
				log := runOK(t, dhgBin, args...)

				for _, want := range []string{"3 shared objects moved to chart platform", "ConfigMap common (used by api, web)", "ServiceAccount runner (used by api, web)", "Secret spare (not referenced by any resource)", "Install platform first"} {
					if !strings.Contains(log, want) {
						t.Errorf("output lacks %q:\n%s", want, log)
					}
				}
				if i, j := strings.Index(log, "helm install platform"), strings.Index(log, "helm install api"); i < 0 || j < 0 || i > j {
					t.Errorf("platform must be installed first:\n%s", log)
				}

				// Every chart passes helm lint --strict and helm template.
				if _, complete := checkCharts(t, helm, out); !complete {
					t.Error("some chart was not rendered")
				}
				// All input objects are rendered by some chart, references resolve.
				checkReleaseIntegrity(t, helm, in, out, true)

				sameIDs(t, "platform", renderChart(t, helm, out, "platform"),
					"ConfigMap/common", "ServiceAccount/runner", "Secret/spare")
				sameIDs(t, "web", renderChart(t, helm, out, "web"),
					"Deployment/web", "Service/web", "ConfigMap/web-own")
				sameIDs(t, "api", renderChart(t, helm, out, "api"),
					"Deployment/api", "Secret/api-token")

				// A name prefix on a service chart never reaches the references
				// to the shared objects: they are literal.
				for _, svc := range []string{"web", "api"} {
					objs := renderChart(t, helm, out, svc, "--set", "global.namePrefix=r1-")
					dep := findObject(objs, "Deployment", "r1-"+svc)
					if dep == nil {
						t.Fatalf("%s: Deployment r1-%s missing in %v", svc, svc, objectIDs(objs))
					}
					spec := get(map[string]interface{}(dep), "spec", "template", "spec")
					if got := str(get(spec, "serviceAccountName")); got != "runner" {
						t.Errorf("%s: serviceAccountName = %q, want the literal runner", svc, got)
					}
					containers := list(get(spec, "containers"))
					first, _ := containers[0].(map[string]interface{})
					if got := str(get(list(first["envFrom"])[0], "configMapRef", "name")); got != "common" {
						t.Errorf("%s: envFrom configMapRef = %q, want the literal common", svc, got)
					}
				}
				// The shared chart refuses a prefix: the references to its objects are
				// literal and would break silently.
				if stream, err := run(helm, "template", "golden", filepath.Join(out, "platform"), "--set", "global.namePrefix=r1-"); err == nil || !strings.Contains(stream, "global.namePrefix must stay empty in the shared chart platform") {
					t.Errorf("platform accepted global.namePrefix (err = %v):\n%s", err, stream)
				}
				// Its own objects are prefixed together with their references.
				web := findObject(renderChart(t, helm, out, "web", "--set", "global.namePrefix=r1-"), "Deployment", "r1-web")
				ref := list(get(list(get(map[string]interface{}(web), "spec", "template", "spec", "containers"))[0], "envFrom"))
				if got := str(get(ref[1], "configMapRef", "name")); got != "r1-web-own" {
					t.Errorf("web-own reference = %q, want r1-web-own", got)
				}
			})
		}
	}
}

// Without anything to share no chart is written and the run still succeeds.
func TestSharedChart_NothingShared(t *testing.T) {
	helm := requireHelm(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "in")
	if err := os.MkdirAll(in, 0o755); err != nil {
		t.Fatal(err)
	}
	single := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: solo
  labels:
    app.kubernetes.io/name: solo
spec:
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
          envFrom:
            - configMapRef:
                name: solo-cfg
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: solo-cfg
  labels:
    app.kubernetes.io/name: solo
data:
  k: v
`
	if err := os.WriteFile(filepath.Join(in, "app.yaml"), []byte(single), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	log := runOK(t, dhgBin, "generate", "-f", in, "-o", out, "--chart-name", "app", "--mode", "separate", "--shared-chart", "platform")
	if !strings.Contains(log, "no shared objects") {
		t.Errorf("no note about the empty shared chart:\n%s", log)
	}
	if fileExists(filepath.Join(out, "platform")) {
		t.Error("an empty platform chart was written")
	}
	checkCharts(t, helm, out)
}

func TestSharedChart_WrongMode(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in")
	if err := os.MkdirAll(in, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(in, "app.yaml"), []byte(sharedInput), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"universal", "umbrella"} {
		out, err := run(dhgBin, "generate", "-f", in, "-o", filepath.Join(dir, mode), "--chart-name", "app", "--mode", mode, "--shared-chart", "platform")
		if err == nil || !strings.Contains(out, "--shared-chart applies to --mode separate and --mode library") {
			t.Errorf("%s: err = %v, output:\n%s", mode, err, out)
		}
	}
}

// Objects that only a non-workload resource of one group references stay in
// that group: an Ingress and its TLS Secret, a RoleBinding and its
// ServiceAccount, an Argo Rollout and its ConfigMap, and a ConfigMap whose
// namespace differs from the one of its workload. Only the Secret nothing
// references moves.
const sharedNonWorkloadInput = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  labels:
    app.kubernetes.io/name: web
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
          envFrom:
            - configMapRef:
                name: web-cfg
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: web-cfg
  namespace: prod
  labels:
    app.kubernetes.io/name: web
data:
  k: v
---
apiVersion: v1
kind: Service
metadata:
  name: web
  labels:
    app.kubernetes.io/name: web
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
  labels:
    app.kubernetes.io/name: web
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
---
apiVersion: v1
kind: Secret
metadata:
  name: web-tls
  labels:
    app.kubernetes.io/name: web
type: Opaque
stringData:
  tls: x
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: web-sa
  labels:
    app.kubernetes.io/name: web
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: web-reader
  labels:
    app.kubernetes.io/name: web
rules:
  - apiGroups: [""]
    resources: [pods]
    verbs: [get]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: web-reader
  labels:
    app.kubernetes.io/name: web
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: web-reader
subjects:
  - kind: ServiceAccount
    name: web-sa
---
apiVersion: v1
kind: Secret
metadata:
  name: spare
type: Opaque
stringData:
  key: value
`

func TestSharedChart_NonWorkloadReferencesStay(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in")
	if err := os.MkdirAll(in, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(in, "app.yaml"), []byte(sharedNonWorkloadInput), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"separate", "library"} {
		t.Run(mode, func(t *testing.T) {
			out := filepath.Join(dir, "out-"+mode)
			log := runOK(t, dhgBin, "generate", "-f", in, "-o", out, "--chart-name", "app", "--mode", mode, "--shared-chart", "platform", "-v")
			if !strings.Contains(log, "1 shared objects moved to chart platform") && !strings.Contains(log, "1 shared object moved to chart platform") {
				t.Errorf("only the spare Secret moves:\n%s", log)
			}
			for _, kept := range []string{"web-tls", "web-sa", "web-cfg"} {
				if strings.Contains(log, " "+kept+" (") || strings.Contains(log, "/"+kept+" (") {
					t.Errorf("%s was moved to the shared chart:\n%s", kept, log)
				}
			}
			if !strings.Contains(log, "Secret spare (not referenced by any resource)") {
				t.Errorf("the spare Secret is not reported:\n%s", log)
			}
		})
	}
}
