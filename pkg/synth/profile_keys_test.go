package synth

import (
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const identityProfile = `
vars:
  prefix: shop-
  tier: backend
naming:
  deployment: "{{ .Vars.prefix }}{{ .Name }}-app"
  container: "main"
  service: "{{ .Name }}-svc"
selector:
  app: "{{ .Name }}"
  tier: "{{ .Vars.tier }}"
podLabels:
  app: "overridden-by-selector"
  wave: "1000"
service:
  type: NodePort
  extraPorts:
    - {name: metrics, port: 9090}
    - {name: debug, port: 5005, targetPort: 5006, protocol: UDP}
    - {name: admin, port: 8081, targetPort: admin-http}
strategy:
  type: RollingUpdate
  rollingUpdate: {maxSurge: 1, maxUnavailable: "25%"}
`

func ordersWithPort(t *testing.T) App {
	t.Helper()
	app := App{Name: "orders", Image: "orders:1"}
	app.AddNamedPort("http", 8080, "TCP")
	app.SetEnv("LOG", "debug")
	return app
}

func nested(t *testing.T, o *unstructured.Unstructured, fields ...string) interface{} {
	t.Helper()
	v, found, err := unstructured.NestedFieldNoCopy(o.Object, fields...)
	if err != nil || !found {
		t.Fatalf("%s %s: no %v (err %v)", o.GetKind(), o.GetName(), fields, err)
	}
	return v
}

func TestProfileNamingOverridesObjectNames(t *testing.T) {
	prof := mustProfile(t, identityProfile)
	app := ordersWithPort(t)
	if err := ApplyProfile(&app, prof); err != nil {
		t.Fatal(err)
	}
	objs := Manifests(app)
	dep := findObj(objs, "Deployment", "shop-orders-app")
	if dep == nil {
		t.Fatalf("no Deployment shop-orders-app in %d objects", len(objs))
	}
	if findObj(objs, "Deployment", "orders") != nil {
		t.Error("the Deployment must not keep the default name")
	}
	containers := nested(t, dep, "spec", "template", "spec", "containers").([]interface{})
	if got := containers[0].(map[string]interface{})["name"]; got != "main" {
		t.Errorf("container name = %v, want main", got)
	}
	svc := findObj(objs, "Service", "orders-svc")
	if svc == nil {
		t.Fatalf("no Service orders-svc in %d objects", len(objs))
	}
	// Everything not named by the profile keeps the application's name.
	if findObj(objs, "ConfigMap", "orders-env") == nil {
		t.Error("ConfigMap orders-env must keep the application's name")
	}
	if got := dep.GetLabels()[nameLabel]; got != "orders" {
		t.Errorf("name label = %q, want orders", got)
	}
}

func TestProfileWithoutNamingKeepsDefaultNames(t *testing.T) {
	prof := mustProfile(t, "podLabels: {a: b}\n")
	app := ordersWithPort(t)
	if err := ApplyProfile(&app, prof); err != nil {
		t.Fatal(err)
	}
	objs := Manifests(app)
	dep := findObj(objs, "Deployment", "orders")
	if dep == nil || findObj(objs, "Service", "orders") == nil {
		t.Fatalf("default names expected, got %d objects", len(objs))
	}
	containers := nested(t, dep, "spec", "template", "spec", "containers").([]interface{})
	if got := containers[0].(map[string]interface{})["name"]; got != "orders" {
		t.Errorf("container name = %v, want orders", got)
	}
	sel := nested(t, dep, "spec", "selector", "matchLabels").(map[string]interface{})
	if !reflect.DeepEqual(sel, map[string]interface{}{nameLabel: "orders"}) {
		t.Errorf("default selector = %v", sel)
	}
	if nested(t, findObj(objs, "Service", "orders"), "spec") == nil {
		t.Fatal("no spec")
	}
	if _, ok := findObj(objs, "Service", "orders").Object["spec"].(map[string]interface{})["type"]; ok {
		t.Error("the Service type must not be set unless the profile sets it")
	}
	if _, ok := dep.Object["spec"].(map[string]interface{})["strategy"]; ok {
		t.Error("the strategy must not be set unless the profile sets it")
	}
}

func TestProfileSelectorReplacesTheDefaultEverywhere(t *testing.T) {
	prof := mustProfile(t, identityProfile)
	app := ordersWithPort(t)
	if err := ApplyProfile(&app, prof); err != nil {
		t.Fatal(err)
	}
	objs := Manifests(app)
	dep := findObj(objs, "Deployment", "shop-orders-app")
	want := map[string]interface{}{"app": "orders", "tier": "backend"}

	match := nested(t, dep, "spec", "selector", "matchLabels").(map[string]interface{})
	if !reflect.DeepEqual(match, want) {
		t.Errorf("matchLabels = %v, want %v", match, want)
	}
	svcSel := nested(t, findObj(objs, "Service", "orders-svc"), "spec", "selector").(map[string]interface{})
	if !reflect.DeepEqual(svcSel, want) {
		t.Errorf("Service selector = %v, want %v", svcSel, want)
	}
	podLabels := nested(t, dep, "spec", "template", "metadata", "labels").(map[string]interface{})
	for k, v := range want {
		if podLabels[k] != v {
			t.Errorf("pod label %s = %v, want %v (the selector must win over podLabels): %v", k, podLabels[k], v, podLabels)
		}
	}
	if podLabels["wave"] != "1000" {
		t.Errorf("other podLabels must stay: %v", podLabels)
	}
	if _, ok := match[nameLabel]; ok {
		t.Errorf("the default selector must be replaced, not extended: %v", match)
	}
}

func TestProfileServiceTypeAndExtraPorts(t *testing.T) {
	prof := mustProfile(t, identityProfile)
	app := ordersWithPort(t)
	if err := ApplyProfile(&app, prof); err != nil {
		t.Fatal(err)
	}
	svc := findObj(Manifests(app), "Service", "orders-svc")
	if got := nested(t, svc, "spec", "type"); got != "NodePort" {
		t.Errorf("type = %v", got)
	}
	ports := nested(t, svc, "spec", "ports").([]interface{})
	want := []interface{}{
		map[string]interface{}{"name": "http", "port": int64(8080), "targetPort": int64(8080), "protocol": "TCP"},
		map[string]interface{}{"name": "metrics", "port": int64(9090), "targetPort": int64(9090), "protocol": "TCP"},
		map[string]interface{}{"name": "debug", "port": int64(5005), "targetPort": int64(5006), "protocol": "UDP"},
		map[string]interface{}{"name": "admin", "port": int64(8081), "targetPort": "admin-http", "protocol": "TCP"},
	}
	if !reflect.DeepEqual(ports, want) {
		t.Errorf("ports = %v\nwant    %v", ports, want)
	}
	for _, p := range ports {
		if _, ok := p.(map[string]interface{})["nodePort"]; ok {
			t.Errorf("nodePort must never be set: %v", p)
		}
	}
	// The container keeps only its own ports.
	dep := findObj(Manifests(app), "Deployment", "shop-orders-app")
	cports := nested(t, dep, "spec", "template", "spec", "containers").([]interface{})[0].(map[string]interface{})["ports"].([]interface{})
	if len(cports) != 1 {
		t.Errorf("container ports = %v, extraPorts belong to the Service only", cports)
	}
}

func TestProfileExtraPortsWithoutAppPortsMakeAService(t *testing.T) {
	prof := mustProfile(t, "service:\n  type: LoadBalancer\n  extraPorts:\n    - {name: metrics, port: 9090}\n")
	app := App{Name: "worker", Image: "w:1"}
	if err := ApplyProfile(&app, prof); err != nil {
		t.Fatal(err)
	}
	svc := findObj(Manifests(app), "Service", "worker")
	if svc == nil {
		t.Fatal("extraPorts must make a Service even if the application has no ports")
	}
	if got := nested(t, svc, "spec", "type"); got != "LoadBalancer" {
		t.Errorf("type = %v", got)
	}
}

func TestProfileServiceTypeWithoutPortsMakesNoService(t *testing.T) {
	prof := mustProfile(t, "service:\n  type: NodePort\n")
	app := App{Name: "worker", Image: "w:1"}
	if err := ApplyProfile(&app, prof); err != nil {
		t.Fatal(err)
	}
	if findObj(Manifests(app), "Service", "worker") != nil {
		t.Error("a Service without ports must not be created")
	}
}

func TestProfileStrategy(t *testing.T) {
	cases := map[string]struct {
		yaml string
		want map[string]interface{}
	}{
		"rolling with int and percent": {
			"strategy:\n  type: RollingUpdate\n  rollingUpdate: {maxSurge: 2, maxUnavailable: 0}\n",
			map[string]interface{}{"type": "RollingUpdate", "rollingUpdate": map[string]interface{}{"maxSurge": int64(2), "maxUnavailable": int64(0)}},
		},
		"percent": {
			"strategy:\n  rollingUpdate: {maxUnavailable: '30%'}\n",
			map[string]interface{}{"type": "RollingUpdate", "rollingUpdate": map[string]interface{}{"maxUnavailable": "30%"}},
		},
		"recreate": {
			"strategy: {type: Recreate}\n",
			map[string]interface{}{"type": "Recreate"},
		},
	}
	for name, c := range cases {
		app := ordersWithPort(t)
		if err := ApplyProfile(&app, mustProfile(t, c.yaml)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		dep := findObj(Manifests(app), "Deployment", "orders")
		if got := nested(t, dep, "spec", "strategy"); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: strategy = %#v, want %#v", name, got, c.want)
		}
		// The object must survive a deep copy (JSON-typed values only).
		_ = dep.DeepCopy()
	}
}

func TestProfileIdentityKeysAreDeterministic(t *testing.T) {
	render := func() string {
		app := ordersWithPort(t)
		if err := ApplyProfile(&app, mustProfile(t, identityProfile)); err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		for _, o := range Manifests(app) {
			y, err := o.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			b.Write(y)
			b.WriteByte('\n')
		}
		return b.String()
	}
	first := render()
	for i := 0; i < 5; i++ {
		if got := render(); got != first {
			t.Fatalf("output differs between runs:\n%s\n%s", first, got)
		}
	}
}

func TestProfileIdentityKeyErrors(t *testing.T) {
	cases := map[string]struct{ yaml, want string }{
		"unknown naming key":     {"naming: {pod: x}\n", "pod"},
		"unknown service key":    {"service: {clusterIP: None}\n", "clusterIP"},
		"unknown strategy key":   {"strategy: {kind: Recreate}\n", "kind"},
		"naming syntax":          {"naming: {deployment: '{{ .Name '}\n", "naming.deployment"},
		"selector syntax":        {"selector: {app: '{{ .Name '}\n", "selector.app"},
		"selector bad key":       {"selector: {'bad key': x}\n", "selector"},
		"selector bad value":     {"selector: {app: 'a b'}\n", "selector.app"},
		"selector empty value":   {"selector: {app: ''}\n", "selector.app"},
		"naming not dns":         {"naming: {deployment: Orders_App}\n", "naming.deployment"},
		"naming container long":  {"naming: {container: " + strings.Repeat("a", 64) + "}\n", "naming.container"},
		"service type":           {"service: {type: ExternalName}\n", "service.type"},
		"service type lowercase": {"service: {type: nodeport}\n", "service.type"},
		"extra port no name":     {"service:\n  extraPorts:\n    - {port: 9090}\n", "extraPorts[0].name"},
		"extra port bad name":    {"service:\n  extraPorts:\n    - {name: Bad_Name, port: 9090}\n", "extraPorts[0].name"},
		"extra port zero":        {"service:\n  extraPorts:\n    - {name: a, port: 0}\n", "extraPorts[0].port"},
		"extra port high":        {"service:\n  extraPorts:\n    - {name: a, port: 70000}\n", "extraPorts[0].port"},
		"extra targetPort high":  {"service:\n  extraPorts:\n    - {name: a, port: 1, targetPort: 70000}\n", "extraPorts[0].targetPort"},
		"extra targetPort name":  {"service:\n  extraPorts:\n    - {name: a, port: 1, targetPort: Bad_Name}\n", "extraPorts[0].targetPort"},
		"extra protocol":         {"service:\n  extraPorts:\n    - {name: a, port: 1, protocol: HTTP}\n", "extraPorts[0].protocol"},
		"extra duplicate name":   {"service:\n  extraPorts:\n    - {name: a, port: 1}\n    - {name: a, port: 2}\n", "extraPorts[1].name"},
		"extra duplicate port":   {"service:\n  extraPorts:\n    - {name: a, port: 1}\n    - {name: b, port: 1}\n", "extraPorts[1].port"},
		"extra nodePort":         {"service:\n  extraPorts:\n    - {name: a, port: 1, nodePort: 30001}\n", "nodePort"},
		"strategy type":          {"strategy: {type: BlueGreen}\n", "strategy.type"},
		"recreate with rolling":  {"strategy: {type: Recreate, rollingUpdate: {maxSurge: 1}}\n", "strategy.rollingUpdate"},
		"negative surge":         {"strategy: {rollingUpdate: {maxSurge: -1}}\n", "maxSurge"},
		"bad percent":            {"strategy: {rollingUpdate: {maxSurge: 'lots'}}\n", "maxSurge"},
		"percent no sign":        {"strategy: {rollingUpdate: {maxUnavailable: '50'}}\n", "maxUnavailable"},
		"unavailable over 100":   {"strategy: {rollingUpdate: {maxUnavailable: '101%'}}\n", "maxUnavailable"},
		"both zero":              {"strategy: {rollingUpdate: {maxSurge: 0, maxUnavailable: '0%'}}\n", "maxSurge"},
	}
	for name, c := range cases {
		_, err := ParseProfile([]byte(c.yaml), "p.yaml")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, c.want)
		}
	}
}

func TestProfileIdentityKeyApplyErrors(t *testing.T) {
	cases := map[string]struct{ yaml, want string }{
		"missing variable in naming": {"naming: {service: '{{ .Vars.nope }}'}\n", "naming.service"},
		"naming renders empty":       {"vars: {x: ''}\nnaming: {deployment: '{{ .Vars.x }}'}\n", "naming.deployment"},
		"naming renders invalid":     {"vars: {x: 'Not DNS'}\nnaming: {container: '{{ .Vars.x }}'}\n", "naming.container"},
		"selector renders invalid":   {"vars: {x: 'a b'}\nselector: {app: '{{ .Vars.x }}'}\n", "selector.app"},
		"extra port name collides":   {"service:\n  extraPorts:\n    - {name: http, port: 9090}\n", "extraPorts[0].name"},
		"extra port collides":        {"service:\n  extraPorts:\n    - {name: other, port: 8080}\n", "extraPorts[0].port"},
	}
	for name, c := range cases {
		prof, err := ParseProfile([]byte(c.yaml), "p.yaml")
		if err != nil {
			t.Errorf("%s: the profile itself must load: %v", name, err)
			continue
		}
		app := ordersWithPort(t)
		err = ApplyProfile(&app, prof)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, c.want)
		}
	}
}

func TestProfileExtraPortSameNumberOtherProtocolIsFine(t *testing.T) {
	prof := mustProfile(t, "service:\n  extraPorts:\n    - {name: dns-udp, port: 8080, protocol: UDP}\n")
	app := ordersWithPort(t)
	if err := ApplyProfile(&app, prof); err != nil {
		t.Fatal(err)
	}
}
