package synth

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// inferDeploymentYAML is one Deployment of the fixture platform: a name
// prefix "qa-", an init container shared by all services, a shared and a
// per-service envFrom, a build label that differs per Deployment and the
// platform's probes, resources and rollout settings.
func inferDeploymentYAML(svc, initTag, branch string) string {
	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: qa-%[1]s
  namespace: shop
  labels:
    app: %[1]s
    tier: backend
    helm.sh/chart: shop-1.0.0
spec:
  replicas: 2
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  selector:
    matchLabels:
      app: %[1]s
  template:
    metadata:
      labels:
        app: %[1]s
        team: platform
        branch: %[3]s
        pod-template-hash: abc%[1]s
    spec:
      imagePullSecrets:
        - name: regcred
      nodeSelector:
        pool: apps
      securityContext: {}
      initContainers:
        - name: wait-for-db
          image: registry.example.com/tools/wait:%[2]s
          args: ["--service", "%[1]s", "--timeout=30", "config.yaml"]
          env:
            - name: DB_HOST
              value: 10.1.2.3
            - name: SERVICE
              value: %[1]s
            - name: JDBC_URL
              value: jdbc:postgresql://db.stand.example.org:5432/%[1]s
          terminationMessagePath: /dev/termination-log
          terminationMessagePolicy: File
          resources: {}
          volumeMounts:
            - name: scripts
              mountPath: /scripts
      containers:
        - name: %[1]s
          image: registry.example.com/shop/%[1]s:2.0.0
          imagePullPolicy: IfNotPresent
          ports:
            - name: http
              containerPort: 8080
          envFrom:
            - configMapRef:
                name: qa-common-config
            - secretRef:
                name: qa-%[1]s-secrets
            - configMapRef:
                name: %[1]s-env
          securityContext:
            runAsNonRoot: true
          readinessProbe:
            httpGet:
              path: /health/ready
              port: http
            periodSeconds: 10
            timeoutSeconds: 1
            failureThreshold: 3
            successThreshold: 1
            initialDelaySeconds: 20
          livenessProbe:
            tcpSocket:
              port: 8080
            periodSeconds: 15
          resources:
            requests:
              cpu: 100m
              memory: 256Mi
            limits:
              memory: 512Mi
          volumeMounts:
            - name: scripts
              mountPath: /scripts
            - name: data-%[1]s
              mountPath: /data
      volumes:
        - name: scripts
          configMap:
            name: qa-scripts
        - name: data-%[1]s
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: qa-%[1]s
  namespace: shop
spec:
  type: NodePort
  selector:
    app: %[1]s
  ports:
    - name: http
      port: 80
      targetPort: http
    - name: metrics
      port: 9090
      targetPort: 9090
`, svc, initTag, branch)
}

func inferFixture(t *testing.T, services []string, tags map[string]string) []*unstructured.Unstructured {
	t.Helper()
	var objs []*unstructured.Unstructured
	for i, svc := range services {
		tag := "1.2"
		if v, ok := tags[svc]; ok {
			tag = v
		}
		objs = append(objs, parseInferObjects(t, inferDeploymentYAML(svc, tag, fmt.Sprintf("feature-%d", i)))...)
	}
	return objs
}

func parseInferObjects(t *testing.T, text string) []*unstructured.Unstructured {
	t.Helper()
	var out []*unstructured.Unstructured
	for _, doc := range strings.Split(text, "\n---\n") {
		if strings.TrimSpace(doc) == "" {
			continue
		}
		m := map[string]interface{}{}
		if err := yaml.Unmarshal([]byte(doc), &m); err != nil {
			t.Fatalf("fixture: %v", err)
		}
		out = append(out, &unstructured.Unstructured{Object: m})
	}
	return out
}

// parseInferred parses an inferred profile with ParseProfile. The keys that
// other changes of this series add to Profile (naming, selector, service,
// strategy) are removed while Profile does not know them; the test then
// still proves that the rest of the output is a valid profile.
func parseInferred(t *testing.T, data []byte) (*Profile, map[string]interface{}) {
	t.Helper()
	raw := map[string]interface{}{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("profile is not YAML: %v\n%s", err, data)
	}
	known := map[string]bool{}
	pt := reflect.TypeOf(Profile{})
	for i := 0; i < pt.NumField(); i++ {
		known[strings.Split(pt.Field(i).Tag.Get("json"), ",")[0]] = true
	}
	strip := map[string]bool{"naming": true, "selector": true, "service": true, "strategy": true}
	trimmed := map[string]interface{}{}
	for k, v := range raw {
		if known[k] {
			trimmed[k] = v
		} else if !strip[k] {
			t.Fatalf("inferred profile has the key %q, which Profile does not have", k)
		}
	}
	out, err := yaml.Marshal(trimmed)
	if err != nil {
		t.Fatal(err)
	}
	p, err := ParseProfile(out, "inferred")
	if err != nil {
		t.Fatalf("ParseProfile: %v\n%s", err, data)
	}
	return p, raw
}

func TestInferProfile(t *testing.T) {
	objs := inferFixture(t, []string{"orders", "billing", "users", "search"}, map[string]string{"search": "1.3"})
	res, err := InferProfile(objs, InferOptions{MinShare: 0.9, Source: "cluster, namespace shop"})
	if err != nil {
		t.Fatalf("InferProfile: %v", err)
	}
	text := string(res.Profile)
	if !strings.HasPrefix(text, "# Generated by dhg profile infer (source: cluster, namespace shop, min-share: 0.9)") {
		t.Errorf("header:\n%s", text)
	}
	p, raw := parseInferred(t, res.Profile)

	if p.Vars["prefix"] != "qa-" {
		t.Errorf("vars.prefix = %q, want qa-", p.Vars["prefix"])
	}

	// Init container: image is the most frequent one, the services' own name
	// became a template, the stand's addresses became variables.
	if len(p.InitContainers) != 1 {
		t.Fatalf("initContainers = %v", p.InitContainers)
	}
	ic := p.InitContainers[0]
	if ic["name"] != "wait-for-db" || ic["image"] != "registry.example.com/tools/wait:1.2" {
		t.Errorf("init container = %v", ic)
	}
	for _, k := range []string{"terminationMessagePath", "terminationMessagePolicy", "resources"} {
		if _, ok := ic[k]; ok {
			t.Errorf("init container keeps the default %s", k)
		}
	}
	args, _ := ic["args"].([]interface{})
	wantArgs := []interface{}{"--service", "{{ .Name }}", "--timeout=30", "config.yaml"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("args = %v, want %v", args, wantArgs)
	}
	env, _ := ic["env"].([]interface{})
	gotEnv := map[string]string{}
	for _, e := range env {
		m := e.(map[string]interface{})
		gotEnv[m["name"].(string)], _ = m["value"].(string)
	}
	wantEnv := map[string]string{"DB_HOST": "{{ .Vars.dbHost }}", "SERVICE": "{{ .Name }}", "JDBC_URL": "jdbc:postgresql://{{ .Vars.jdbcUrlHost }}:5432/{{ .Name }}"}
	if !reflect.DeepEqual(gotEnv, wantEnv) {
		t.Errorf("env = %v, want %v", gotEnv, wantEnv)
	}
	if v, ok := p.Vars["dbHost"]; !ok || v != "" {
		t.Errorf("vars.dbHost = %q (present %v), want an empty default", v, ok)
	}
	vals, err := ParseProfile(res.Values, "values")
	if err != nil {
		t.Fatalf("values file is not a profile: %v\n%s", err, res.Values)
	}
	if vals.Vars["dbHost"] != "10.1.2.3" {
		t.Errorf("values dbHost = %q", vals.Vars["dbHost"])
	}
	if vals.Vars["jdbcUrlHost"] != "db.stand.example.org" {
		t.Errorf("values jdbcUrlHost = %q", vals.Vars["jdbcUrlHost"])
	}
	if strings.Contains(text, "10.1.2.3") || strings.Contains(text, "stand.example.org") {
		t.Errorf("the stand's values are in the profile:\n%s", text)
	}

	// envFrom: shared and per-service entries; dhg's own <name>-env is not repeated.
	wantEnvFrom := []ProfileEnvFrom{
		{ConfigMapRef: "{{ .Vars.prefix }}common-config"},
		{SecretRef: "{{ .Vars.prefix }}{{ .Name }}-secrets"},
	}
	if !reflect.DeepEqual(p.EnvFrom, wantEnvFrom) {
		t.Errorf("envFrom = %v, want %v", p.EnvFrom, wantEnvFrom)
	}

	// Labels: agreeing ones only; selector keys, hash and Helm labels are not labels of the platform.
	if want := map[string]string{"team": "platform"}; !reflect.DeepEqual(p.PodLabels, want) {
		t.Errorf("podLabels = %v, want %v (branch differs per Deployment)", p.PodLabels, want)
	}
	if want := map[string]string{"tier": "backend"}; !reflect.DeepEqual(p.DeploymentLabels, want) {
		t.Errorf("deploymentLabels = %v, want %v", p.DeploymentLabels, want)
	}
	if !containsNote(res.Notes, "branch") {
		t.Errorf("no note about the label that differs: %v", res.Notes)
	}

	// Volumes of the main container and of init containers.
	var mountNames, volumeNames []string
	for _, m := range p.VolumeMounts {
		mountNames = append(mountNames, m["name"].(string))
	}
	for _, v := range p.Volumes {
		volumeNames = append(volumeNames, v["name"].(string))
	}
	if want := []string{"scripts", "data-{{ .Name }}"}; !reflect.DeepEqual(mountNames, want) {
		t.Errorf("volumeMounts = %v, want %v", mountNames, want)
	}
	if want := []string{"data-{{ .Name }}", "scripts"}; !reflect.DeepEqual(volumeNames, want) {
		t.Errorf("volumes = %v, want %v", volumeNames, want)
	}
	if cm, _ := p.Volumes[1]["configMap"].(map[string]interface{}); cm["name"] != "{{ .Vars.prefix }}scripts" {
		t.Errorf("volume scripts = %v", p.Volumes[1])
	}

	// defaults
	if p.Defaults.Resources == nil || p.Defaults.Resources.Requests["cpu"] != "100m" || p.Defaults.Resources.Limits["memory"] != "512Mi" {
		t.Errorf("resources = %+v", p.Defaults.Resources)
	}
	if r := p.Defaults.Probes.Readiness; r == nil || r.Path != "/health/ready" || r.Port != 0 || r.InitialDelaySeconds != 20 || r.PeriodSeconds != 10 {
		t.Errorf("readiness = %+v", r)
	}
	if l := p.Defaults.Probes.Liveness; l == nil || !l.TCP || l.PeriodSeconds != 15 {
		t.Errorf("liveness = %+v", l)
	}
	if p.Defaults.Probes.Startup != nil {
		t.Errorf("startup = %+v, want none", p.Defaults.Probes.Startup)
	}

	// Container and pod patches.
	if want := map[string]interface{}{"imagePullPolicy": "IfNotPresent", "securityContext": map[string]interface{}{"runAsNonRoot": true}}; !reflect.DeepEqual(p.Container, want) {
		t.Errorf("container = %v, want %v", p.Container, want)
	}
	if want := map[string]interface{}{
		"imagePullSecrets": []interface{}{map[string]interface{}{"name": "regcred"}},
		"nodeSelector":     map[string]interface{}{"pool": "apps"},
	}; !reflect.DeepEqual(p.Pod, want) {
		t.Errorf("pod = %v, want %v", p.Pod, want)
	}

	// Keys of the identity (naming, selector, service, strategy).
	if want := map[string]interface{}{"deployment": "{{ .Vars.prefix }}{{ .Name }}", "service": "{{ .Vars.prefix }}{{ .Name }}"}; !reflect.DeepEqual(raw["naming"], want) {
		t.Errorf("naming = %v, want %v", raw["naming"], want)
	}
	if want := map[string]interface{}{"app": "{{ .Name }}"}; !reflect.DeepEqual(raw["selector"], want) {
		t.Errorf("selector = %v, want %v", raw["selector"], want)
	}
	wantService := map[string]interface{}{
		"type":       "NodePort",
		"extraPorts": []interface{}{map[string]interface{}{"name": "metrics", "port": float64(9090)}},
	}
	if !reflect.DeepEqual(raw["service"], wantService) {
		t.Errorf("service = %v, want %v", raw["service"], wantService)
	}
	wantStrategy := map[string]interface{}{
		"type":          "RollingUpdate",
		"rollingUpdate": map[string]interface{}{"maxSurge": float64(1), "maxUnavailable": float64(0)},
	}
	if !reflect.DeepEqual(raw["strategy"], wantStrategy) {
		t.Errorf("strategy = %v, want %v", raw["strategy"], wantStrategy)
	}
}

func containsNote(notes []string, sub string) bool {
	for _, n := range notes {
		if strings.Contains(n, sub) {
			return true
		}
	}
	return false
}

func TestInferProfileDeterministic(t *testing.T) {
	services := []string{"orders", "billing", "users", "search", "mail"}
	tags := map[string]string{"search": "1.3", "mail": "1.3"}
	first, err := InferProfile(inferFixture(t, services, tags), InferOptions{MinShare: 0.6})
	if err != nil {
		t.Fatal(err)
	}
	for seed := int64(1); seed <= 8; seed++ {
		// The order of Deployments and Services in the input, and of the
		// containers' env, must not matter; branches stay with their service.
		shuffled := append([]string(nil), services...)
		rand.New(rand.NewSource(seed)).Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		var objs []*unstructured.Unstructured
		for _, svc := range shuffled {
			tag := "1.2"
			if v, ok := tags[svc]; ok {
				tag = v
			}
			branch := 0
			for i, s := range services {
				if s == svc {
					branch = i
				}
			}
			objs = append(objs, parseInferObjects(t, inferDeploymentYAML(svc, tag, fmt.Sprintf("feature-%d", branch)))...)
		}
		rand.New(rand.NewSource(seed)).Shuffle(len(objs), func(i, j int) { objs[i], objs[j] = objs[j], objs[i] })
		got, err := InferProfile(objs, InferOptions{MinShare: 0.6})
		if err != nil {
			t.Fatal(err)
		}
		if string(got.Profile) != string(first.Profile) {
			t.Fatalf("seed %d: profile differs\n--- first\n%s\n--- got\n%s", seed, first.Profile, got.Profile)
		}
		if string(got.Values) != string(first.Values) {
			t.Fatalf("seed %d: values differ\n%s\n%s", seed, first.Values, got.Values)
		}
		if !reflect.DeepEqual(got.Notes, first.Notes) {
			t.Fatalf("seed %d: notes differ\n%v\n%v", seed, first.Notes, got.Notes)
		}
	}
}

func TestInferProfileMinShare(t *testing.T) {
	// The init container is in 3 of 4 Deployments.
	objs := inferFixture(t, []string{"orders", "billing", "users", "search"}, nil)
	for _, o := range objs {
		if o.GetKind() == "Deployment" && o.GetName() == "qa-search" {
			unstructured.RemoveNestedField(o.Object, "spec", "template", "spec", "initContainers")
		}
	}
	strict, err := InferProfile(objs, InferOptions{MinShare: 0.9})
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := parseInferred(t, strict.Profile); len(p.InitContainers) != 0 {
		t.Errorf("min-share 0.9 kept the init container: %v", p.InitContainers)
	}
	if !containsNote(strict.Notes, "wait-for-db") {
		t.Errorf("no note about the skipped init container: %v", strict.Notes)
	}
	loose, err := InferProfile(objs, InferOptions{MinShare: 0.7})
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := parseInferred(t, loose.Profile); len(p.InitContainers) != 1 {
		t.Errorf("min-share 0.7 dropped the init container: %v", p.InitContainers)
	}
}

func TestInferProfileKeepValues(t *testing.T) {
	objs := inferFixture(t, []string{"orders", "billing"}, nil)
	res, err := InferProfile(objs, InferOptions{MinShare: 0.9, KeepValues: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Values != nil {
		t.Errorf("values written with KeepValues: %s", res.Values)
	}
	p, _ := parseInferred(t, res.Profile)
	if p.Vars["dbHost"] != "10.1.2.3" {
		t.Errorf("vars.dbHost = %q, want the stand's value", p.Vars["dbHost"])
	}
}

func TestInferProfileErrors(t *testing.T) {
	objs := inferFixture(t, []string{"orders"}, nil)
	for _, share := range []float64{0, -0.1, 1.1} {
		if _, err := InferProfile(objs, InferOptions{MinShare: share}); err == nil || !strings.Contains(err.Error(), "min-share") {
			t.Errorf("min-share %v: err = %v", share, err)
		}
	}
	if _, err := InferProfile(nil, InferOptions{MinShare: 0.9}); err == nil || !strings.Contains(err.Error(), "Deployment") {
		t.Errorf("no input: err = %v", err)
	}
	svcOnly := parseInferObjects(t, "apiVersion: v1\nkind: Service\nmetadata:\n  name: a\nspec:\n  selector:\n    app: a\n")
	if _, err := InferProfile(svcOnly, InferOptions{MinShare: 0.9}); err == nil {
		t.Error("input without Deployments: want an error")
	}
}

func TestInferProfileNoPrefix(t *testing.T) {
	// Names equal to the service: no prefix, no naming.
	text := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: orders
  labels:
    app: orders
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: orders
  template:
    metadata:
      labels:
        app.kubernetes.io/name: orders
    spec:
      containers:
        - name: orders
          image: r.example.com/orders:1
`
	res, err := InferProfile(parseInferObjects(t, text), InferOptions{MinShare: 0.9})
	if err != nil {
		t.Fatal(err)
	}
	p, raw := parseInferred(t, res.Profile)
	if _, ok := p.Vars["prefix"]; ok {
		t.Errorf("vars.prefix set without a prefix: %v", p.Vars)
	}
	for _, k := range []string{"naming", "selector", "service", "strategy"} {
		if _, ok := raw[k]; ok {
			t.Errorf("%s = %v, want it absent (the defaults)", k, raw[k])
		}
	}
	if res.Values != nil {
		t.Errorf("values = %s, want none", res.Values)
	}
}

func TestInferPrefix(t *testing.T) {
	mk := func(pairs ...string) []*inferDeployment {
		var out []*inferDeployment
		for i := 0; i < len(pairs); i += 2 {
			out = append(out, &inferDeployment{name: pairs[i], svc: pairs[i+1]})
		}
		return out
	}
	cases := []struct {
		name  string
		deps  []*inferDeployment
		share float64
		want  string
	}{
		{"all", mk("qa-a", "a", "qa-b", "b"), 0.9, "qa-"},
		{"none", mk("a", "a", "b", "b"), 0.9, ""},
		{"below share", mk("qa-a", "a", "b", "b"), 0.9, ""},
		{"at share", mk("qa-a", "a", "qa-b", "b", "c", "c"), 0.6, "qa-"},
		{"longest", mk("qa-x-a", "a", "qa-x-b", "b", "qa-c", "c"), 0.6, "qa-x-"},
		{"single", mk("qa-a", "a"), 0.9, "qa-"},
	}
	for _, c := range cases {
		if got := inferPrefix(c.deps, inferNeed(c.share, len(c.deps))); got != c.want {
			t.Errorf("%s: prefix = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestInferNormalize(t *testing.T) {
	n := inferNormalizer{prefix: "qa-", svc: "orders"}
	cases := map[string]string{
		"qa-orders":                  "{{ .Vars.prefix }}{{ .Name }}",
		"qa-orders-secrets":          "{{ .Vars.prefix }}{{ .Name }}-secrets",
		"qa-common-config":           "{{ .Vars.prefix }}common-config",
		"orders-env":                 "{{ .Name }}-env",
		"registry/orders:1.0":        "registry/{{ .Name }}:1.0",
		"preorders":                  "preorders",
		"ordersx":                    "ordersx",
		"data-qa-x":                  "data-{{ .Vars.prefix }}x",
		"aqa-x":                      "aqa-x",
		"nothing":                    "nothing",
		"literal {{ braces }}":       `literal {{ "{{" }} braces }}`,
		"--svc=orders --p=qa-orders": "--svc={{ .Name }} --p={{ .Vars.prefix }}{{ .Name }}",
	}
	for in, want := range cases {
		if got := n.str(in); got != want {
			t.Errorf("str(%q) = %q, want %q", in, got, want)
		}
	}
	if got := (inferNormalizer{svc: "orders"}).str("qa-orders"); got != "qa-{{ .Name }}" {
		t.Errorf("without a prefix: %q", got)
	}
}

func TestInferEnvSpecific(t *testing.T) {
	yes := []string{
		"10.0.0.5", "10.0.0.5:5432", "db.example.org", "db.example.org:5432",
		"jdbc:postgresql://db.example.org:5432/app", "http://svc.other.example:8080/x", "https://api.example.org",
		"jdbc:oracle:thin:@db.example.org:1521/xe",
	}
	no := []string{
		"", "app", "config.yaml", "app.jar", "1.2.3", "v1.2.3", "/etc/ssl", "registry.example.com/shop/app:1.0",
		"http://localhost:8080", "{{ .Vars.x }}.example.org", "IfNotPresent", "10.0.0", "300.1.1.1", "--timeout=30",
	}
	for _, s := range yes {
		if _, v, ok := inferEnvSpecific(s); !ok || v != s {
			t.Errorf("%q: specific = %v (value %q), want the whole string", s, ok, v)
		}
	}
	for _, s := range no {
		if _, _, ok := inferEnvSpecific(s); ok {
			t.Errorf("%q: reported as environment-specific", s)
		}
	}
	if p, v, ok := inferEnvSpecific("--host=db.example.org"); !ok || p != "--host=" || v != "db.example.org" {
		t.Errorf("flag: %q %q %v", p, v, ok)
	}
	if p, v, ok := inferEnvSpecific("HOST=10.1.1.1"); !ok || p != "HOST=" || v != "10.1.1.1" {
		t.Errorf("assignment: %q %q %v", p, v, ok)
	}
	// A URL that holds a template keeps it; only the host becomes a variable.
	if h, host, tail, ok := inferTemplatedHost("jdbc:postgresql://db.example.org:5432/{{ .Name }}"); !ok || h != "jdbc:postgresql://" || host != "db.example.org" || tail != ":5432/{{ .Name }}" {
		t.Errorf("templated URL: %q %q %q %v", h, host, tail, ok)
	}
	if _, _, _, ok := inferTemplatedHost("http://{{ .Vars.host }}/x"); ok {
		t.Error("templated host reported as environment-specific")
	}
	if _, _, _, ok := inferTemplatedHost("{{ .Name }}-env"); ok {
		t.Error("plain template reported as environment-specific")
	}
}

// Credentials in a URL (user:password@host) must never reach the profile or
// the values file: the userinfo is dropped and a note says so.
func TestInferURLCredentialsRedacted(t *testing.T) {
	objs := inferFixture(t, []string{"orders", "billing", "users", "search"}, nil)
	for _, o := range objs {
		if o.GetKind() != "Deployment" {
			continue
		}
		svc := o.GetLabels()["app"]
		ics, _, _ := unstructured.NestedSlice(o.Object, "spec", "template", "spec", "initContainers")
		ic := ics[0].(map[string]interface{})
		env := ic["env"].([]interface{})
		for _, e := range env {
			m := e.(map[string]interface{})
			if m["name"] == "JDBC_URL" {
				m["value"] = "jdbc:postgresql://dbuser:S3cretPw@db.stand.example.com:5432/" + svc
			}
		}
		env = append(env, map[string]interface{}{"name": "CB", "value": "http://admin:hunter2@cb.stand.example.com:8080/x"})
		ic["env"] = env
		ics[0] = ic
		if err := unstructured.SetNestedSlice(o.Object, ics, "spec", "template", "spec", "initContainers"); err != nil {
			t.Fatal(err)
		}
	}
	for _, keep := range []bool{false, true} {
		res, err := InferProfile(objs, InferOptions{MinShare: 0.9, KeepValues: keep})
		if err != nil {
			t.Fatal(err)
		}
		all := string(res.Profile) + string(res.Values) + strings.Join(res.Notes, "\n")
		for _, secret := range []string{"S3cretPw", "hunter2", "dbuser", "admin:"} {
			if strings.Contains(all, secret) {
				t.Errorf("keep=%v: %q leaked:\n%s", keep, secret, all)
			}
		}
		if !strings.Contains(string(res.Profile), "jdbc:postgresql://{{ .Vars.jdbcUrlHost }}:5432/{{ .Name }}") {
			t.Errorf("keep=%v: templated URL lost its host variable:\n%s", keep, res.Profile)
		}
		found := 0
		for _, n := range res.Notes {
			if strings.Contains(n, "credentials in URL") {
				found++
			}
		}
		if found != 2 {
			t.Errorf("keep=%v: want a note per URL with credentials (2), got %d: %v", keep, found, res.Notes)
		}
	}
	// URLs without credentials say nothing.
	if _, red := inferRedactURL("jdbc:postgresql://db.example.org:5432/app"); red {
		t.Error("URL without userinfo reported as redacted")
	}
	if s, red := inferRedactURL("--url=http://u:p@h.example.org/x"); !red || s != "--url=http://h.example.org/x" {
		t.Errorf("assignment: %q %v", s, red)
	}
	if _, red := inferRedactURL("http://{{ .Vars.u }}@h.example.org/x"); red {
		t.Error("templated userinfo is not a secret")
	}
}

// An address inside a shell command (not the whole string, not name=value)
// is made a variable too.
func TestInferAddressInCommand(t *testing.T) {
	objs := inferFixture(t, []string{"orders", "billing", "users", "search"}, nil)
	for _, o := range objs {
		if o.GetKind() != "Deployment" {
			continue
		}
		ics, _, _ := unstructured.NestedSlice(o.Object, "spec", "template", "spec", "initContainers")
		ic := ics[0].(map[string]interface{})
		ic["command"] = []interface{}{"sh", "-c", "until nc -z 10.1.2.3 5432; do sleep 1; done; curl -s 'http://api.stand.example.org:8080/ready'; ls config.yaml; wget http://bob:pw123@files.stand.example.org/y"}
		ics[0] = ic
		if err := unstructured.SetNestedSlice(o.Object, ics, "spec", "template", "spec", "initContainers"); err != nil {
			t.Fatal(err)
		}
	}
	res, err := InferProfile(objs, InferOptions{MinShare: 0.9})
	if err != nil {
		t.Fatal(err)
	}
	text := string(res.Profile)
	if strings.Contains(text, "10.1.2.3") || strings.Contains(text, "stand.example.org") {
		t.Errorf("literal address in the profile:\n%s", text)
	}
	if strings.Contains(text+string(res.Values), "pw123") || strings.Contains(text, "bob") {
		t.Errorf("credentials of a URL in a command leaked:\n%s", text)
	}
	if !strings.Contains(text, "nc -z {{ .Vars.") || !strings.Contains(text, "config.yaml") {
		t.Errorf("command:\n%s", text)
	}
	if !strings.Contains(string(res.Values), "10.1.2.3") || !strings.Contains(string(res.Values), "http://api.stand.example.org:8080/ready") {
		t.Errorf("values:\n%s", res.Values)
	}
}

// A prefix shared by most, but not min-share, of the Deployments is still
// vars.prefix: naming does not get the literal prefix of the stand.
func TestInferFrequentPrefix(t *testing.T) {
	var objs []*unstructured.Unstructured
	for i, svc := range []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "b1", "b2", "b3"} {
		text := inferDeploymentYAML(svc, "1.2", fmt.Sprintf("f%d", i))
		if i < 7 {
			text = strings.ReplaceAll(text, "qa-", "shop-dev-")
		} else {
			text = strings.ReplaceAll(text, "qa-", "")
		}
		objs = append(objs, parseInferObjects(t, text)...)
	}
	res, err := InferProfile(objs, InferOptions{MinShare: 0.9})
	if err != nil {
		t.Fatal(err)
	}
	p, raw := parseInferred(t, res.Profile)
	if p.Vars["prefix"] != "shop-dev-" {
		t.Errorf("vars.prefix = %q, want shop-dev-\n%s", p.Vars["prefix"], res.Profile)
	}
	naming, _ := raw["naming"].(map[string]interface{})
	if naming["deployment"] != "{{ .Vars.prefix }}{{ .Name }}" {
		t.Errorf("naming = %v", naming)
	}
	if strings.Contains(string(res.Profile), "shop-dev-") && strings.Count(string(res.Profile), "shop-dev-") != 1 {
		t.Errorf("the literal prefix is in the profile outside vars.prefix:\n%s", res.Profile)
	}
	notes := strings.Join(res.Notes, "\n")
	if !strings.Contains(notes, "7 of 10") || !strings.Contains(notes, "namespace") {
		t.Errorf("want notes about the share and the namespace in the prefix:\n%s", notes)
	}
}

// A Service port that the application's port serves, but under another
// number, is lost in the profile: a note says the chart exposes the
// application's port.
func TestInferShiftedServicePort(t *testing.T) {
	objs := inferFixture(t, []string{"orders", "billing", "users", "search"}, nil)
	res, err := InferProfile(objs, InferOptions{MinShare: 0.9})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, n := range res.Notes {
		if strings.Contains(n, "Service port 80 -> 8080") {
			got = append(got, n)
		}
	}
	if len(got) != 1 || !strings.Contains(got[0], "4 of 4") || !strings.Contains(got[0], "qa-billing") {
		t.Errorf("want one note on Service port 80 -> 8080 for 4 of 4 Deployments, got %v\nall: %v", got, res.Notes)
	}
	// Same port and targetPort: nothing to say.
	for _, o := range objs {
		if o.GetKind() == "Service" {
			ports, _, _ := unstructured.NestedSlice(o.Object, "spec", "ports")
			p := ports[0].(map[string]interface{})
			p["port"] = int64(8080)
			if err := unstructured.SetNestedSlice(o.Object, ports, "spec", "ports"); err != nil {
				t.Fatal(err)
			}
		}
	}
	res, err = InferProfile(objs, InferOptions{MinShare: 0.9})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range res.Notes {
		if strings.Contains(n, "Service port") {
			t.Errorf("unexpected note: %s", n)
		}
	}
}
