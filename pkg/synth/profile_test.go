package synth

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const sampleProfile = `
vars:
  prefix: acme-dev-
  utils: registry.example.com/platform/utils:1.2
envFrom:
  - secretRef: "{{ .Vars.prefix }}system-secret"
  - configMapRef: "{{ .Vars.prefix }}system-config"
  - configMapRef: "{{ .Vars.prefix }}{{ .Name }}-config"
initContainers:
  - name: init-utils
    image: "{{ .Vars.utils }}"
    command: [/bin/sh]
    args: ["-c", "cp -R /opt/utils/. /home/utils; echo {{ .Name }}"]
    volumeMounts:
      - {name: utils, mountPath: /home/utils}
podLabels:
  app: "{{ .Name }}"
  wave: "1000"
podAnnotations:
  platform.example.com/managed: "true"
deploymentLabels:
  app: "{{ .Name }}"
defaults:
  resources:
    requests: {cpu: 500m, memory: 1Gi}
    limits: {cpu: 5, memory: 9Gi}
  port: 8080
  probes:
    startup: {tcp: true, periodSeconds: 30, failureThreshold: 40, timeoutSeconds: 5, successThreshold: 1}
    liveness: {path: "/{{ .Name }}/actuator/health", periodSeconds: 240, initialDelaySeconds: 1200}
    readiness: {tcp: true, periodSeconds: 30}
volumes:
  - {name: utils, emptyDir: {}}
  - name: shared
    configMap: {name: shared, defaultMode: 511}
volumeMounts:
  - {name: utils, mountPath: /share/scripts}
jobs:
  - when: liquibase
    name: "database-{{ .Name }}"
    manifest:
      spec:
        backoffLimit: 0
        template:
          spec:
            restartPolicy: Never
            containers:
              - name: migrate
                image: "registry.example.com/db/{{ .Name }}:latest"
  - when: always
    name: "smoke-{{ .Name }}"
    manifest: |
      apiVersion: batch/v1
      kind: Job
      metadata:
        annotations:
          helm.sh/hook: post-install
      spec:
        template:
          spec:
            restartPolicy: Never
            containers:
              - {name: smoke, image: "busybox:{{ .Vars.prefix }}"}
`

func mustProfile(t *testing.T, y string) *Profile {
	t.Helper()
	p, err := ParseProfile([]byte(y), "test")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func findObj(objs []*unstructured.Unstructured, kind, name string) *unstructured.Unstructured {
	for _, o := range objs {
		if o.GetKind() == kind && o.GetName() == name {
			return o
		}
	}
	return nil
}

func TestApplyProfileBuildsThePlatformPod(t *testing.T) {
	prof := mustProfile(t, sampleProfile)
	app := App{Name: "orders", Image: "orders:1", Migrations: []string{"liquibase"}}
	app.AddNamedPort("http", 9000, "TCP")
	app.Liveness = &Probe{HTTPPath: "/guess", Port: 9000, PeriodSeconds: 10}
	app.SetEnv("LOG", "debug")
	if err := ApplyProfile(&app, prof); err != nil {
		t.Fatal(err)
	}
	objs := Manifests(app)
	dep := findObj(objs, "Deployment", "orders")
	if dep == nil {
		t.Fatal("no Deployment")
	}

	pod, _, _ := unstructured.NestedMap(dep.Object, "spec", "template", "spec")
	containers := pod["containers"].([]interface{})
	main := containers[0].(map[string]interface{})

	var refs []string
	for _, ref := range main["envFrom"].([]interface{}) {
		m := ref.(map[string]interface{})
		for kind, v := range m {
			refs = append(refs, kind+":"+v.(map[string]interface{})["name"].(string))
		}
	}
	want := "secretRef:acme-dev-system-secret configMapRef:acme-dev-system-config configMapRef:acme-dev-orders-config configMapRef:orders-env"
	if strings.Join(refs, " ") != want {
		t.Errorf("envFrom = %v\nwant %s", refs, want)
	}

	inits := pod["initContainers"].([]interface{})
	if len(inits) != 1 {
		t.Fatalf("initContainers = %v", inits)
	}
	init0 := inits[0].(map[string]interface{})
	if init0["image"] != "registry.example.com/platform/utils:1.2" || !strings.Contains(init0["args"].([]interface{})[1].(string), "echo orders") {
		t.Errorf("init container = %v", init0)
	}

	labels, _, _ := unstructured.NestedStringMap(dep.Object, "spec", "template", "metadata", "labels")
	if labels["app"] != "orders" || labels["wave"] != "1000" || labels[nameLabel] != "orders" {
		t.Errorf("pod labels = %v", labels)
	}
	ann, _, _ := unstructured.NestedStringMap(dep.Object, "spec", "template", "metadata", "annotations")
	if ann["platform.example.com/managed"] != "true" {
		t.Errorf("pod annotations = %v", ann)
	}
	if dep.GetLabels()["app"] != "orders" || dep.GetLabels()[nameLabel] != "orders" {
		t.Errorf("deployment labels = %v", dep.GetLabels())
	}
	sel, _, _ := unstructured.NestedStringMap(dep.Object, "spec", "selector", "matchLabels")
	if len(sel) != 1 || sel[nameLabel] != "orders" {
		t.Errorf("selector = %v: profile labels must not enter the selector", sel)
	}

	vols := pod["volumes"].([]interface{})
	if len(vols) != 2 || vols[1].(map[string]interface{})["configMap"].(map[string]interface{})["defaultMode"] != int64(511) {
		t.Errorf("volumes = %v", vols)
	}
	if mounts := main["volumeMounts"].([]interface{}); len(mounts) != 1 || mounts[0].(map[string]interface{})["mountPath"] != "/share/scripts" {
		t.Errorf("mounts = %v", main["volumeMounts"])
	}

	res := main["resources"].(map[string]interface{})
	if res["limits"].(map[string]interface{})["cpu"] != "5" || res["requests"].(map[string]interface{})["memory"] != "1Gi" {
		t.Errorf("resources = %v", res)
	}
	if _, ok := main["startupProbe"].(map[string]interface{})["tcpSocket"]; !ok {
		t.Errorf("startup = %v", main["startupProbe"])
	}
	live := main["livenessProbe"].(map[string]interface{})
	if live["httpGet"].(map[string]interface{})["path"] != "/orders/actuator/health" || live["httpGet"].(map[string]interface{})["port"] != int64(9000) ||
		live["periodSeconds"] != int64(240) || live["initialDelaySeconds"] != int64(1200) {
		t.Errorf("liveness = %v: the profile replaces the derived probe, on the app's port", live)
	}
	ready := main["readinessProbe"].(map[string]interface{})
	if ready["tcpSocket"].(map[string]interface{})["port"] != int64(9000) {
		t.Errorf("readiness = %v", ready)
	}

	job := findObj(objs, "Job", "database-orders")
	if job == nil {
		t.Fatal("no Job for liquibase")
	}
	jann := job.GetAnnotations()
	if jann["helm.sh/hook"] != "pre-install,pre-upgrade" || jann["helm.sh/hook-delete-policy"] != "before-hook-creation" {
		t.Errorf("job hook annotations = %v", jann)
	}
	if job.GetAPIVersion() != "batch/v1" {
		t.Errorf("job = %v", job.Object)
	}
	smoke := findObj(objs, "Job", "smoke-orders")
	if smoke == nil || smoke.GetAnnotations()["helm.sh/hook"] != "post-install" {
		t.Errorf("smoke job = %v: a hook of the manifest is kept", smoke)
	}
	if c, _, _ := unstructured.NestedSlice(smoke.Object, "spec", "template", "spec", "containers"); c[0].(map[string]interface{})["image"] != "busybox:acme-dev-" {
		t.Errorf("job container = %v", c)
	}
}

// container and pod patch what the profile has no dedicated key for.
func TestProfileContainerAndPodPatches(t *testing.T) {
	prof := mustProfile(t, `
vars: {prefix: acme-dev-}
container:
  imagePullPolicy: Always
  workingDir: /share/config
  command: [/bin/sh]
  args: ["-c", "sh /share/run.sh {{ .Name }}"]
  env:
    - {name: ARCH, value: hybrid}
  lifecycle:
    preStop: {exec: {command: [/bin/sh, -c, "sh /share/stop.sh {{ .Name }}"]}}
  securityContext:
    allowPrivilegeEscalation: false
    capabilities: {drop: [ALL]}
pod:
  hostname: "{{ .Vars.prefix }}{{ .Name }}"
  imagePullSecrets: [{name: "{{ .Vars.prefix }}registry"}]
  nodeSelector: {"node-role.kubernetes.io/worker": ""}
  securityContext: {runAsUser: 30001}
  terminationGracePeriodSeconds: 60
`)
	app := App{Name: "orders", Image: "x"}
	app.AddNamedPort("http", 8080, "TCP")
	app.SetUser("10001:10001")
	if err := ApplyProfile(&app, prof); err != nil {
		t.Fatal(err)
	}
	dep := findObj(Manifests(app), "Deployment", "orders")
	pod, _, _ := unstructured.NestedMap(dep.Object, "spec", "template", "spec")
	main := pod["containers"].([]interface{})[0].(map[string]interface{})
	if main["imagePullPolicy"] != "Always" || main["workingDir"] != "/share/config" || main["args"].([]interface{})[1] != "sh /share/run.sh orders" ||
		main["env"].([]interface{})[0].(map[string]interface{})["value"] != "hybrid" || main["image"] != "x" ||
		main["securityContext"].(map[string]interface{})["allowPrivilegeEscalation"] != false {
		t.Errorf("container = %v", main)
	}
	if got := main["lifecycle"].(map[string]interface{})["preStop"].(map[string]interface{})["exec"].(map[string]interface{})["command"].([]interface{}); got[2] != "sh /share/stop.sh orders" {
		t.Errorf("lifecycle = %v", got)
	}
	if pod["hostname"] != "acme-dev-orders" || pod["terminationGracePeriodSeconds"] != int64(60) ||
		pod["imagePullSecrets"].([]interface{})[0].(map[string]interface{})["name"] != "acme-dev-registry" ||
		pod["nodeSelector"].(map[string]interface{})["node-role.kubernetes.io/worker"] != "" {
		t.Errorf("pod = %v", pod)
	}
	// The pod patch is merged into what the sources gave: the user from the
	// Dockerfile is replaced, its group stays.
	sc := pod["securityContext"].(map[string]interface{})
	if sc["runAsUser"] != int64(30001) || sc["runAsGroup"] != int64(10001) || sc["runAsNonRoot"] != true {
		t.Errorf("securityContext = %v", sc)
	}
}

func TestProfileJobsFollowTheMigrations(t *testing.T) {
	prof := mustProfile(t, sampleProfile)
	app := App{Name: "orders", Image: "orders:1"}
	if err := ApplyProfile(&app, prof); err != nil {
		t.Fatal(err)
	}
	if len(app.Jobs) != 1 || app.Jobs[0].Name != "smoke-orders" {
		t.Errorf("without migrations only the always job: %+v", app.Jobs)
	}
	app = App{Name: "orders", Image: "orders:1", Migrations: []string{"flyway"}}
	if err := ApplyProfile(&app, mustProfile(t, strings.Replace(sampleProfile, "when: liquibase", "when: flyway", 1))); err != nil {
		t.Fatal(err)
	}
	if len(app.Jobs) != 2 {
		t.Errorf("flyway job: %+v", app.Jobs)
	}
}

// Defaults fill gaps; they do not overwrite what the sources say.
func TestProfileDefaultsKeepSourceFacts(t *testing.T) {
	prof := mustProfile(t, `
defaults:
  resources: {requests: {cpu: 100m}}
  port: 8080
`)
	app := App{Name: "orders", Image: "x", Resources: &Resources{Requests: map[string]string{"cpu": "2"}}}
	app.AddNamedPort("http", 9000, "TCP")
	if err := ApplyProfile(&app, prof); err != nil {
		t.Fatal(err)
	}
	if app.Resources.Requests["cpu"] != "2" || len(app.Ports) != 1 || app.Ports[0].Port != 9000 {
		t.Errorf("resources = %+v ports = %+v", app.Resources, app.Ports)
	}
	empty := App{Name: "orders", Image: "x"}
	if err := ApplyProfile(&empty, prof); err != nil {
		t.Fatal(err)
	}
	if empty.Resources == nil || empty.Resources.Requests["cpu"] != "100m" || len(empty.Ports) != 1 || empty.Ports[0].Port != 8080 || empty.Ports[0].Name != "http" {
		t.Errorf("resources = %+v ports = %+v", empty.Resources, empty.Ports)
	}
}

func TestProfileSecretPatternsReclassifyEnv(t *testing.T) {
	prof := mustProfile(t, "secretKeyPatterns: ['_PIN$']\n")
	app := App{Name: "x", Image: "x"}
	app.SetEnv("DB_PASSWORD", "p")
	app.SetEnv("CARD_PIN", "1")
	if err := ApplyProfile(&app, prof); err != nil {
		t.Fatal(err)
	}
	if envMap(app.Env)["DB_PASSWORD"] != "p" || envMap(app.SecretEnv)["CARD_PIN"] != "1" || len(app.SecretEnv) != 1 {
		t.Errorf("env = %+v secret = %+v", app.Env, app.SecretEnv)
	}
}

func TestProfileErrors(t *testing.T) {
	cases := map[string]struct{ yaml, want string }{
		"unknown key":      {"initContainer: []\n", "initContainer"},
		"unknown nested":   {"defaults:\n  probe: {}\n", "probe"},
		"bad pattern":      {"secretKeyPatterns: ['(']\n", "secretKeyPatterns"},
		"envFrom both":     {"envFrom:\n  - {configMapRef: a, secretRef: b}\n", "exactly one"},
		"envFrom none":     {"envFrom:\n  - {}\n", "exactly one"},
		"job when":         {"jobs:\n  - {when: sometimes, name: x, manifest: {}}\n", "when"},
		"job no manifest":  {"jobs:\n  - {when: always, name: x}\n", "manifest"},
		"probe no handler": {"defaults:\n  probes:\n    liveness: {periodSeconds: 5}\n", "one of tcp, path, exec"},
		"probe two":        {"defaults:\n  probes:\n    liveness: {tcp: true, path: /x}\n", "one of tcp, path, exec"},
		"template syntax":  {"podLabels:\n  a: \"{{ .Name \"\n", "podLabels"},
		"init no name":     {"initContainers:\n  - {image: x}\n", "name"},
		"mount no path":    {"volumeMounts:\n  - {name: x}\n", "mountPath"},
		"glob outside":     {"dockerfileGlobs: ['../x']\n", "dockerfileGlobs"},
		"bad port":         {"defaults:\n  port: 70000\n", "port"},
		"duplicate key":    {"vars: {a: b}\nvars: {c: d}\n", "vars"},
		"container image":  {"container: {image: x}\n", "container.image"},
		"container probe":  {"container: {livenessProbe: {}}\n", "defaults.probes"},
		"container res":    {"container: {resources: {}}\n", "defaults.resources"},
		"pod containers":   {"pod: {containers: []}\n", "pod.containers"},
		"pod volumes":      {"pod: {volumes: []}\n", "volumes"},
	}
	for name, c := range cases {
		_, err := ParseProfile([]byte(c.yaml), "p.yaml")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, c.want)
		}
	}
}

func TestProfileMissingVariable(t *testing.T) {
	prof := mustProfile(t, "podLabels:\n  a: \"{{ .Vars.prefix }}x\"\n")
	app := App{Name: "orders", Image: "x"}
	err := ApplyProfile(&app, prof)
	if err == nil || !strings.Contains(err.Error(), "prefix") || !strings.Contains(err.Error(), "podLabels") {
		t.Errorf("err = %v: a template that uses an unset variable must fail and name it", err)
	}
}

func TestEmptyProfileChangesNothing(t *testing.T) {
	app := App{Name: "orders", Image: "x"}
	app.AddNamedPort("http", 8080, "TCP")
	before := len(Manifests(app))
	if err := ApplyProfile(&app, &Profile{}); err != nil {
		t.Fatal(err)
	}
	if err := ApplyProfile(&app, nil); err != nil {
		t.Fatal(err)
	}
	if len(Manifests(app)) != before || len(app.Jobs) != 0 || len(app.InitContainers) != 0 {
		t.Errorf("an empty profile must change nothing: %+v", app)
	}
}

// The example profile in examples/ is the documented schema: it must load
// and apply to a project with a Liquibase module.
func TestExampleProfileLoadsAndApplies(t *testing.T) {
	prof, err := LoadProfile("../../examples/synth-profile.yaml")
	if err != nil {
		t.Fatal(err)
	}
	s, notes := readOrders(t, ordersFixture, prof)
	app := s.App("registry.example.com/shop/orders:1", "", notes)
	if err := ApplyProfile(&app, prof); err != nil {
		t.Fatal(err)
	}
	objs := Manifests(app)
	if findObj(objs, "Job", "shop-dev-migrate-orders") == nil {
		t.Errorf("no migration Job in %d objects", len(objs))
	}
	dep := findObj(objs, "Deployment", "shop-dev-orders")
	if dep == nil {
		t.Fatalf("no Deployment shop-dev-orders in %d objects", len(objs))
	}
	pod, _, _ := unstructured.NestedMap(dep.Object, "spec", "template", "spec")
	if len(pod["initContainers"].([]interface{})) != 1 {
		t.Errorf("pod = %v", pod)
	}
	svc := findObj(objs, "Service", "shop-dev-orders")
	if svc == nil {
		t.Fatalf("no Service shop-dev-orders in %d objects", len(objs))
	}
	if ports, _, _ := unstructured.NestedSlice(svc.Object, "spec", "ports"); len(ports) < 3 {
		t.Errorf("the extra ports are missing: %v", ports)
	}
	if typ, _, _ := unstructured.NestedString(dep.Object, "spec", "strategy", "type"); typ != "RollingUpdate" {
		t.Errorf("strategy type = %q", typ)
	}
	if _, ok := envMap(app.SecretEnv)["DB_PASSWORD"]; !ok {
		t.Errorf("secret = %+v", app.SecretEnv)
	}
}

func TestLoadProfileMissingFile(t *testing.T) {
	if _, err := LoadProfile("testdata/none.yaml"); err == nil {
		t.Error("a missing file must fail")
	}
}
