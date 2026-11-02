package processor

import (
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func collisionResource(kind, namespace, name, service, valuesKey string) *types.ProcessedResource {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	obj.SetAPIVersion("v1")
	obj.SetKind(kind)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return &types.ProcessedResource{
		Original:     &types.ExtractedResource{Object: obj, GVK: obj.GroupVersionKind()},
		ServiceName:  service,
		TemplatePath: "templates/" + service + "-" + strings.ToLower(kind) + ".yaml",
		ValuesPath:   "services." + service + "." + valuesKey,
		TemplateContent: "{{- $svc := .Values.services." + service + " -}}\n" +
			"metadata:\n  name: " + name + "\n  namespace: {{ $.Release.Namespace }}\n" +
			"other: {{ .Values.services." + service + "Extra.x }}\n",
	}
}

func TestResolveCollisions_SameServiceMovesToOwnService(t *testing.T) {
	api := collisionResource("Deployment", "default", "shop-api", "shop", "deployment")
	worker := collisionResource("Deployment", "default", "shop-worker", "shop", "deployment")

	notes := ResolveCollisions([]*types.ProcessedResource{api, worker})

	if api.ServiceName != "shop" || api.TemplatePath != "templates/shop-deployment.yaml" {
		t.Errorf("first resource must keep its paths, got %s %s", api.ServiceName, api.TemplatePath)
	}
	if worker.ServiceName != "shopWorker" || worker.ValuesPath != "services.shopWorker.deployment" ||
		worker.TemplatePath != "templates/shop-deployment-worker.yaml" {
		t.Errorf("worker moved to %s %s %s", worker.ServiceName, worker.ValuesPath, worker.TemplatePath)
	}
	if !strings.Contains(worker.TemplateContent, "$svc := .Values.services.shopWorker -}}") {
		t.Errorf("template still reads the old service:\n%s", worker.TemplateContent)
	}
	if !strings.Contains(worker.TemplateContent, ".Values.services.shopExtra.x") {
		t.Errorf("a longer service name sharing the prefix must not be rewritten:\n%s", worker.TemplateContent)
	}
	if !strings.Contains(worker.TemplateContent, "namespace: {{ $.Release.Namespace }}") {
		t.Errorf("distinct names need no literal namespace:\n%s", worker.TemplateContent)
	}
	if len(notes) != 1 {
		t.Errorf("notes = %v", notes)
	}
}

func TestResolveCollisions_SameNameInTwoNamespaces(t *testing.T) {
	a := collisionResource("ConfigMap", "team-a", "settings", "settings", "configMaps.settings")
	b := collisionResource("ConfigMap", "team-b", "settings", "settings", "configMaps.settings")

	ResolveCollisions([]*types.ProcessedResource{a, b})

	for _, r := range []*types.ProcessedResource{a, b} {
		want := `namespace: "` + r.Original.Object.GetNamespace() + `"`
		if !strings.Contains(r.TemplateContent, want) {
			t.Errorf("%s: template lacks %s:\n%s", r.Original.ResourceKey(), want, r.TemplateContent)
		}
	}
	if a.TemplatePath == b.TemplatePath || a.ValuesPath == b.ValuesPath {
		t.Errorf("paths still collide: %s %s / %s %s", a.TemplatePath, a.ValuesPath, b.TemplatePath, b.ValuesPath)
	}
	if b.ServiceName != "settingsTeamB" {
		t.Errorf("second ConfigMap moved to %q, want settingsTeamB", b.ServiceName)
	}
}

func TestResolveCollisions_NoCollisionNoChange(t *testing.T) {
	a := collisionResource("Deployment", "default", "a", "a", "deployment")
	b := collisionResource("Deployment", "default", "b", "b", "deployment")
	before := [4]string{b.ServiceName, b.TemplatePath, b.ValuesPath, b.TemplateContent}
	if notes := ResolveCollisions([]*types.ProcessedResource{a, b}); len(notes) != 0 {
		t.Errorf("unexpected notes %v", notes)
	}
	if after := [4]string{b.ServiceName, b.TemplatePath, b.ValuesPath, b.TemplateContent}; after != before {
		t.Errorf("resource changed without a collision")
	}
}

// collisionJob is collisionResource with labels, for the naming tests.
func collisionJob(namespace, name, service string, labels map[string]string) *types.ProcessedResource {
	r := collisionResource("Job", namespace, name, service, "job")
	r.Original.Object.SetLabels(labels)
	return r
}

func TestResolveCollisions_NamesDropNamespaceGroupAndRunIDs(t *testing.T) {
	const group = "orders"
	first := collisionJob("staging", "staging-dev-database-orders-dbfix", group, nil)
	run := collisionJob("staging", "staging-dev-database-orders-dbfix-240101", group, nil)
	other := collisionJob("staging", "staging-dev-database-orders-dbfix2-240102", group, nil)

	ResolveCollisions([]*types.ProcessedResource{first, run, other})

	// first keeps the group; the others get readable, stable names.
	if first.ServiceName != group {
		t.Errorf("first moved to %q", first.ServiceName)
	}
	if want := "ordersDevDatabaseDbfix"; run.ServiceName != want {
		t.Errorf("run job = %q, want %q", run.ServiceName, want)
	}
	if want := "ordersDevDatabaseDbfix2"; other.ServiceName != want {
		t.Errorf("other job = %q, want %q", other.ServiceName, want)
	}
}

func TestResolveCollisions_NumericRunIDsOnlyNamesFallBack(t *testing.T) {
	a := collisionJob("ns1", "550001", "batch", nil)
	b := collisionJob("ns1", "550001-2", "batch", nil)
	c := collisionJob("ns1", "550001-3", "batch", nil)

	ResolveCollisions([]*types.ProcessedResource{a, b, c})

	// Nothing readable is left of the names: namespace, then kind.
	if b.ServiceName != "batchNs1" || c.ServiceName != "batchJob" {
		t.Errorf("fallback names = %q, %q; want batchNs1, batchJob", b.ServiceName, c.ServiceName)
	}
}

func TestResolveCollisions_PrefersAppLabelOverOpaqueCounter(t *testing.T) {
	// Name equal to the service: nothing to take from the name.
	a := collisionJob("shop", "shop", "shop", map[string]string{"app": "shop"})
	b := collisionJob("shop", "shop", "shop", map[string]string{"app": "billing-sync"})

	ResolveCollisions([]*types.ProcessedResource{a, b})

	if b.ServiceName != "shopBillingSync" {
		t.Errorf("service = %q, want shopBillingSync", b.ServiceName)
	}
}

func TestResolveCollisions_RecommendedNameLabel(t *testing.T) {
	a := collisionJob("shop", "shop", "shop", nil)
	b := collisionJob("shop", "shop", "shop", map[string]string{"app.kubernetes.io/name": "reports"})

	ResolveCollisions([]*types.ProcessedResource{a, b})

	if b.ServiceName != "shopReports" {
		t.Errorf("service = %q, want shopReports", b.ServiceName)
	}
}

func TestResolveCollisions_DeterministicWhateverTheInputOrder(t *testing.T) {
	build := func() []*types.ProcessedResource {
		return []*types.ProcessedResource{
			collisionJob("ns", "7", "batch", nil),
			collisionJob("ns", "8", "batch", nil),
			collisionJob("ns", "9", "batch", nil),
			collisionJob("ns", "10", "batch", nil),
		}
	}
	names := func(rs []*types.ProcessedResource) map[string]string {
		m := map[string]string{}
		for _, r := range rs {
			m[r.Original.Object.GetName()] = r.ServiceName
		}
		return m
	}
	forward := build()
	ResolveCollisions(forward)

	reversed := build()
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	ResolveCollisions(reversed)

	if f, r := names(forward), names(reversed); !reflect.DeepEqual(f, r) {
		t.Errorf("names depend on input order:\nforward  %v\nreversed %v", f, r)
	}
}

// A run-id object (a one-off Job copy such as job-runner-550001-2) never takes
// the shared service from a stable object, whatever the input order: worker-1
// keeps "worker", as it did before collisions were sorted.
func TestResolveCollisions_StableObjectKeepsSharedService(t *testing.T) {
	build := func() []*types.ProcessedResource {
		return []*types.ProcessedResource{
			collisionJob("prod", "worker-1", "worker", nil),
			collisionJob("prod", "worker-2", "worker", nil),
			collisionJob("prod", "job-runner-550001-2", "worker", nil),
		}
	}
	for _, reverse := range []bool{false, true} {
		rs := build()
		if reverse {
			rs[0], rs[2] = rs[2], rs[0]
		}
		ResolveCollisions(rs)
		for _, r := range rs {
			if r.Original.Object.GetName() == "worker-1" && r.ServiceName != "worker" {
				t.Errorf("reverse=%v: worker-1 service = %q, want worker", reverse, r.ServiceName)
			}
		}
	}
}

// Short ordinals distinguish objects; only run ids (5+ digits) are noise.
func TestResolveCollisions_KeepsShortOrdinals(t *testing.T) {
	a := collisionJob("prod", "worker-1", "app", nil)
	b := collisionJob("prod", "worker-2", "app", nil)
	c := collisionJob("prod", "worker-3", "app", nil)
	d := collisionJob("prod", "worker-4", "app", nil)

	ResolveCollisions([]*types.ProcessedResource{a, b, c, d})

	if a.ServiceName != "app" {
		t.Errorf("first = %q, want app", a.ServiceName)
	}
	for r, want := range map[*types.ProcessedResource]string{b: "appWorker2", c: "appWorker3", d: "appWorker4"} {
		if r.ServiceName != want {
			t.Errorf("%s = %q, want %q", r.Original.Object.GetName(), r.ServiceName, want)
		}
	}
}

func TestReadableSuffix_DropsOnlyRunIDs(t *testing.T) {
	for in, want := range map[string]string{
		"worker-1":             "worker-1",
		"worker-12-3":          "worker-12-3",
		"worker-240101":        "worker",
		"worker-240101-2":      "worker",
		"job-1700000000":       "job",
		"550001":               "",
		"550001-2":             "",
		"7":                    "7",
		"dbfix2-240102":        "dbfix2",
		"shop-worker-2-240101": "shop-worker",
	} {
		if got := readableSuffix(in); got != want {
			t.Errorf("readableSuffix(%q) = %q, want %q", in, got, want)
		}
	}
}
