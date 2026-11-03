package generator

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// sharedObject is a ConfigMap-like object of a group with a template of its own.
func sharedObject(kind, name, namespace, app string) *types.ProcessedResource {
	var labels map[string]string
	if app != "" {
		labels = map[string]string{"app.kubernetes.io/name": app}
	}
	r := makeProcessedResource(kind, name, namespace, labels)
	r.ServiceName = app
	r.TemplatePath = "templates/" + strings.ToLower(kind) + "-" + name + ".yaml"
	r.TemplateContent = "apiVersion: v1\nkind: " + kind + "\nmetadata:\n  name: " + name + "\n"
	return r
}

// sharedWorkload is a Deployment of group app that references the given objects.
func sharedWorkload(name, namespace, app string, refs ...*types.ProcessedResource) *types.ProcessedResource {
	r := sharedObject("Deployment", name, namespace, app)
	for _, ref := range refs {
		r.Dependencies = append(r.Dependencies, ref.Original.ResourceKey())
	}
	return r
}

func groupOf(name string, resources ...*types.ProcessedResource) *ServiceGroup {
	return &ServiceGroup{Name: name, Resources: resources}
}

func names(resources []*types.ProcessedResource) []string {
	var out []string
	for _, r := range resources {
		out = append(out, r.Original.GVK.Kind+"/"+r.Original.Object.GetName())
	}
	return out
}

func sharedNames(split *SharedSplit) []string {
	var out []string
	for _, o := range split.Shared {
		out = append(out, o.Resource.Original.GVK.Kind+"/"+o.Resource.Original.Object.GetName())
	}
	return out
}

func TestSplitShared_ReferencedByTwoGroupsMoves(t *testing.T) {
	common := sharedObject("ConfigMap", "common", "ns", "")
	own := sharedObject("ConfigMap", "web-own", "ns", "")
	web := sharedWorkload("web", "ns", "web", common, own)
	api := sharedWorkload("api", "ns", "api", common)
	groups := []*ServiceGroup{groupOf("api", api), groupOf("web", web, common, own)}

	split := SplitShared(groups)

	if got, want := sharedNames(split), []string{"ConfigMap/common"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shared = %v, want %v", got, want)
	}
	if got, want := split.Shared[0].UsedBy, []string{"api", "web"}; !reflect.DeepEqual(got, want) {
		t.Errorf("UsedBy = %v, want %v", got, want)
	}
	web2 := findGroupByName(split.Groups, "web")
	if got, want := names(web2.Resources), []string{"Deployment/web", "ConfigMap/web-own"}; !reflect.DeepEqual(got, want) {
		t.Errorf("web group = %v, want %v (own ConfigMap stays)", got, want)
	}
}

func findGroupByName(groups []*ServiceGroup, name string) *ServiceGroup {
	for _, g := range groups {
		if g.Name == name {
			return g
		}
	}
	return nil
}

func TestSplitShared_ReferencedByOneGroupStays(t *testing.T) {
	cm := sharedObject("ConfigMap", "cfg", "ns", "")
	secret := sharedObject("Secret", "tok", "ns", "")
	// Two workloads of one group reference both: still one group.
	a := sharedWorkload("a", "ns", "app", cm, secret)
	b := sharedWorkload("b", "ns", "app", cm)
	split := SplitShared([]*ServiceGroup{groupOf("app", a, b, cm, secret)})
	if len(split.Shared) != 0 {
		t.Fatalf("shared = %v, want none", sharedNames(split))
	}
	if got := len(split.Groups[0].Resources); got != 4 {
		t.Errorf("group has %d resources, want 4", got)
	}
}

func TestSplitShared_UnreferencedMoves(t *testing.T) {
	orphan := sharedObject("ConfigMap", "orphan", "ns", "")
	orphanSA := sharedObject("ServiceAccount", "unused", "ns", "")
	orphanPVC := sharedObject("PersistentVolumeClaim", "scratch", "ns", "")
	svc := sharedObject("Service", "web", "ns", "web")
	web := sharedWorkload("web", "ns", "web")
	split := SplitShared([]*ServiceGroup{groupOf("web", web, svc), groupOf("ns", orphan, orphanSA, orphanPVC)})

	want := []string{"ConfigMap/orphan", "PersistentVolumeClaim/scratch", "ServiceAccount/unused"}
	if got := sharedNames(split); !reflect.DeepEqual(got, want) {
		t.Fatalf("shared = %v, want %v", got, want)
	}
	for _, o := range split.Shared {
		if len(o.UsedBy) != 0 {
			t.Errorf("%s UsedBy = %v, want none", o.Resource.Original.Object.GetName(), o.UsedBy)
		}
	}
	// The group that only held unreferenced objects disappears; the Service
	// (a kind nothing references from a pod) stays with its workload.
	if len(split.Groups) != 1 || split.Groups[0].Name != "web" {
		t.Fatalf("groups = %v, want only web", groupNames(split.Groups))
	}
	if got, want := names(split.Groups[0].Resources), []string{"Deployment/web", "Service/web"}; !reflect.DeepEqual(got, want) {
		t.Errorf("web group = %v, want %v", got, want)
	}
}

func groupNames(groups []*ServiceGroup) []string {
	var out []string
	for _, g := range groups {
		out = append(out, g.Name)
	}
	return out
}

func TestSplitShared_ServiceAccountAndPVCMove(t *testing.T) {
	sa := sharedObject("ServiceAccount", "runner", "ns", "")
	pvc := sharedObject("PersistentVolumeClaim", "data", "ns", "")
	a := sharedWorkload("a", "ns", "a", sa, pvc)
	b := sharedWorkload("b", "ns", "b", sa, pvc)
	split := SplitShared([]*ServiceGroup{groupOf("a", a), groupOf("b", b, sa, pvc)})
	want := []string{"PersistentVolumeClaim/data", "ServiceAccount/runner"}
	if got := sharedNames(split); !reflect.DeepEqual(got, want) {
		t.Fatalf("shared = %v, want %v", got, want)
	}
}

// A definition written without a namespace applies to the namespace of the workload.
func TestSplitShared_UnscopedObjectResolves(t *testing.T) {
	cm := sharedObject("ConfigMap", "common", "", "")
	a := sharedWorkload("a", "ns", "a")
	b := sharedWorkload("b", "ns", "b")
	for _, w := range []*types.ProcessedResource{a, b} {
		key := cm.Original.ResourceKey()
		key.Namespace = "ns"
		w.Dependencies = append(w.Dependencies, key)
	}
	split := SplitShared([]*ServiceGroup{groupOf("a", a, cm), groupOf("b", b)})
	if got, want := sharedNames(split), []string{"ConfigMap/common"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shared = %v, want %v", got, want)
	}
}

// The references of any object of the group count, not only those of pod
// templates: an Ingress to its TLS Secret, a RoleBinding to its ServiceAccount
// and an Argo Rollout (a workload CRD with a pod template) to its ConfigMap.
func TestSplitShared_ReferencesFromNonPodOwnersKeepObjectInGroup(t *testing.T) {
	tlsSecret := sharedObject("Secret", "web-tls", "ns", "")
	sa := sharedObject("ServiceAccount", "web-sa", "ns", "")
	workerCfg := sharedObject("ConfigMap", "worker-cfg", "ns", "")
	spare := sharedObject("Secret", "spare", "ns", "")

	web := sharedWorkload("web", "ns", "web")
	ingress := sharedObject("Ingress", "web", "ns", "web")
	ingress.Dependencies = []types.ResourceKey{tlsSecret.Original.ResourceKey()}
	binding := sharedObject("RoleBinding", "web-reader", "ns", "web")
	// The RoleBinding processor writes its ServiceAccount keys without a version.
	saKey := sa.Original.ResourceKey()
	saKey.GVK.Version = ""
	binding.Dependencies = []types.ResourceKey{saKey}
	rollout := sharedObject("Rollout", "worker", "ns", "worker")
	rollout.Dependencies = []types.ResourceKey{workerCfg.Original.ResourceKey()}

	split := SplitShared([]*ServiceGroup{
		groupOf("web", web, ingress, binding, tlsSecret, sa),
		groupOf("worker", rollout, workerCfg),
		groupOf("misc", spare),
	})

	if got, want := sharedNames(split), []string{"Secret/spare"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shared = %v, want %v", got, want)
	}
	if got, want := names(findGroupByName(split.Groups, "web").Resources),
		[]string{"Deployment/web", "Ingress/web", "RoleBinding/web-reader", "Secret/web-tls", "ServiceAccount/web-sa"}; !reflect.DeepEqual(got, want) {
		t.Errorf("web group = %v, want %v", got, want)
	}
	if got, want := names(findGroupByName(split.Groups, "worker").Resources), []string{"Rollout/worker", "ConfigMap/worker-cfg"}; !reflect.DeepEqual(got, want) {
		t.Errorf("worker group = %v, want %v", got, want)
	}
}

// Two objects with one name in different namespaces: a reference with a
// namespace finds its own.
func TestSplitShared_SameNameInTwoNamespaces(t *testing.T) {
	cmA := sharedObject("ConfigMap", "cfg", "a", "")
	cmB := sharedObject("ConfigMap", "cfg", "b", "")
	a := sharedWorkload("a", "a", "a", cmA)
	b := sharedWorkload("b", "b", "b", cmB)
	split := SplitShared([]*ServiceGroup{groupOf("a", a, cmA), groupOf("b", b, cmB)})
	if len(split.Shared) != 0 {
		t.Fatalf("shared = %v, want none (each group uses its own cfg)", sharedNames(split))
	}
}

// A Rollout is a workload: it never moves, and its references count like those
// of a Deployment, also when two groups use the same object.
func TestSplitShared_RolloutReferencesDecideSharing(t *testing.T) {
	cm := sharedObject("ConfigMap", "common", "ns", "")
	a := sharedObject("Rollout", "a", "ns", "a")
	a.Dependencies = []types.ResourceKey{cm.Original.ResourceKey()}
	b := sharedWorkload("b", "ns", "b", cm)
	split := SplitShared([]*ServiceGroup{groupOf("a", a, cm), groupOf("b", b)})
	if got, want := sharedNames(split), []string{"ConfigMap/common"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shared = %v, want %v", got, want)
	}
	if got, want := split.Shared[0].UsedBy, []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("UsedBy = %v, want %v", got, want)
	}
}

// Non-workload references of two groups make an object shared too.
func TestSplitShared_NonWorkloadReferencesFromTwoGroupsMove(t *testing.T) {
	tlsSecret := sharedObject("Secret", "wild-tls", "ns", "")
	a := sharedObject("Ingress", "a", "ns", "a")
	a.Dependencies = []types.ResourceKey{tlsSecret.Original.ResourceKey()}
	b := sharedObject("Ingress", "b", "ns", "b")
	b.Dependencies = []types.ResourceKey{tlsSecret.Original.ResourceKey()}
	split := SplitShared([]*ServiceGroup{groupOf("a", sharedWorkload("a", "ns", "a"), a, tlsSecret), groupOf("b", sharedWorkload("b", "ns", "b"), b)})
	if got, want := sharedNames(split), []string{"Secret/wild-tls"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shared = %v, want %v", got, want)
	}
	if got, want := split.Shared[0].UsedBy, []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("UsedBy = %v, want %v", got, want)
	}
}

// The namespace of the input does not take part in the match: every template
// renders into .Release.Namespace.
func TestSplitShared_MixedNamespacesStillMatch(t *testing.T) {
	cm := sharedObject("ConfigMap", "web-cfg", "prod", "")
	web := sharedWorkload("web", "", "web")
	key := cm.Original.ResourceKey()
	key.Namespace = ""
	web.Dependencies = []types.ResourceKey{key}
	split := SplitShared([]*ServiceGroup{groupOf("web", web, cm)})
	if len(split.Shared) != 0 {
		t.Fatalf("shared = %v, want none (web uses web-cfg)", sharedNames(split))
	}

	// Different namespaces on both sides.
	other := sharedObject("ConfigMap", "api-cfg", "prod", "")
	api := sharedWorkload("api", "staging", "api")
	k2 := other.Original.ResourceKey()
	k2.Namespace = "staging"
	api.Dependencies = []types.ResourceKey{k2}
	b := sharedWorkload("b", "other", "b")
	k3 := other.Original.ResourceKey()
	k3.Namespace = "other"
	b.Dependencies = []types.ResourceKey{k3}
	split = SplitShared([]*ServiceGroup{groupOf("api", api, other), groupOf("b", b)})
	if got, want := sharedNames(split), []string{"ConfigMap/api-cfg"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shared = %v, want %v (used by api and b)", got, want)
	}
}

func TestSplitShared_IgnoresWorkloadTargetsCRDsAndMissing(t *testing.T) {
	crd := sharedObject("CustomResourceDefinition", "widgets.example.com", "", "")
	a := sharedWorkload("a", "ns", "a")
	b := sharedWorkload("b", "ns", "b", a) // a workload referencing a workload is not a shared object
	missing := sharedObject("ConfigMap", "gone", "ns", "")
	a.Dependencies = append(a.Dependencies, missing.Original.ResourceKey())
	b.Dependencies = append(b.Dependencies, missing.Original.ResourceKey())
	split := SplitShared([]*ServiceGroup{groupOf("a", a, crd), groupOf("b", b)})
	if len(split.Shared) != 0 {
		t.Fatalf("shared = %v, want none", sharedNames(split))
	}
	if got := len(findGroupByName(split.Groups, "a").Resources); got != 2 {
		t.Errorf("group a has %d resources, want 2 (CRD stays)", got)
	}
}

func TestSplitShared_ObjectsWithoutTemplateStay(t *testing.T) {
	cm := sharedObject("ConfigMap", "common", "ns", "")
	cm.TemplateContent = ""
	a := sharedWorkload("a", "ns", "a", cm)
	b := sharedWorkload("b", "ns", "b", cm)
	split := SplitShared([]*ServiceGroup{groupOf("a", a, cm), groupOf("b", b)})
	if len(split.Shared) != 0 {
		t.Fatalf("shared = %v, want none (nothing to write)", sharedNames(split))
	}
}

func TestSplitShared_Deterministic(t *testing.T) {
	build := func() []*ServiceGroup {
		x := sharedObject("ConfigMap", "x", "ns", "")
		y := sharedObject("Secret", "y", "ns", "")
		z := sharedObject("ConfigMap", "a-last", "ns", "")
		a := sharedWorkload("a", "ns", "a", x, y, z)
		b := sharedWorkload("b", "ns", "b", z, y, x)
		return []*ServiceGroup{groupOf("a", a, y), groupOf("b", b, x, z)}
	}
	first := sharedNames(SplitShared(build()))
	for i := 0; i < 20; i++ {
		if got := sharedNames(SplitShared(build())); !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d: %v, want %v", i, got, first)
		}
	}
	if want := []string{"ConfigMap/a-last", "ConfigMap/x", "Secret/y"}; !reflect.DeepEqual(first, want) {
		t.Errorf("order = %v, want %v", first, want)
	}
}

func TestSplitShared_DoesNotModifyInput(t *testing.T) {
	cm := sharedObject("ConfigMap", "common", "ns", "")
	a := sharedWorkload("a", "ns", "a", cm)
	b := sharedWorkload("b", "ns", "b", cm)
	groups := []*ServiceGroup{groupOf("a", a, cm), groupOf("b", b)}
	SplitShared(groups)
	if len(groups[0].Resources) != 2 {
		t.Errorf("input group changed: %v", names(groups[0].Resources))
	}
}

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

func sharedGraph() (*types.ResourceGraph, map[string]*types.ProcessedResource) {
	common := sharedObject("ConfigMap", "common", "ns", "")
	own := sharedObject("ConfigMap", "web-own", "ns", "web")
	web := sharedWorkload("web", "ns", "web", common, own)
	api := sharedWorkload("api", "ns", "api", common)
	orphan := sharedObject("Secret", "spare", "ns", "")
	all := map[string]*types.ProcessedResource{"common": common, "own": own, "web": web, "api": api, "orphan": orphan}
	return buildGraph([]*types.ProcessedResource{common, own, web, api, orphan}, nil), all
}

func chartByName(charts []*types.GeneratedChart, name string) *types.GeneratedChart {
	for _, c := range charts {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func hasTemplate(c *types.GeneratedChart, path string) bool {
	_, ok := c.Templates[path]
	return ok
}

func TestSeparateGenerator_SharedChart(t *testing.T) {
	graph, _ := sharedGraph()
	var reported []SharedObject
	var reportedChart string
	charts, err := NewSeparateGenerator().Generate(context.Background(), graph, Options{
		ChartName: "app", ChartVersion: "0.1.0", SharedChart: "platform",
		OnShared: func(chart string, moved []SharedObject) { reportedChart, reported = chart, moved },
	})
	if err != nil {
		t.Fatal(err)
	}
	platform := chartByName(charts, "platform")
	if platform == nil {
		t.Fatalf("no platform chart in %v", chartNamesOf(charts))
	}
	for _, p := range []string{"templates/configmap-common.yaml", "templates/secret-spare.yaml"} {
		if !hasTemplate(platform, p) {
			t.Errorf("platform chart lacks %s: %v", p, platform.Templates)
		}
	}
	if hasTemplate(platform, "templates/configmap-web-own.yaml") {
		t.Error("the ConfigMap used by one group moved to the platform chart")
	}
	web := chartByName(charts, "web")
	if web == nil || !hasTemplate(web, "templates/configmap-web-own.yaml") || hasTemplate(web, "templates/configmap-common.yaml") {
		t.Errorf("web chart templates = %v", templateKeys(web))
	}
	api := chartByName(charts, "api")
	if api == nil || hasTemplate(api, "templates/configmap-common.yaml") {
		t.Errorf("api chart templates = %v", templateKeys(api))
	}
	if len(charts) != 3 {
		t.Errorf("charts = %v, want api, web, platform", chartNamesOf(charts))
	}
	if reportedChart != "platform" || len(reported) != 2 {
		t.Errorf("OnShared(%q, %d objects), want (platform, 2)", reportedChart, len(reported))
	}
	// The platform chart is a chart of its own: helpers and plain Chart.yaml.
	if !strings.Contains(platform.Helpers, `define "platform.objname"`) {
		t.Error("platform chart has no helpers of its own")
	}
	if strings.Contains(platform.ChartYAML, "dependencies") {
		t.Errorf("platform chart depends on something:\n%s", platform.ChartYAML)
	}
}

// References to a moved object are literal: the service charts do not list it
// among the objects global.namePrefix applies to.
func TestSeparateGenerator_SharedChart_ServiceChartsDoNotPrefixSharedNames(t *testing.T) {
	graph, _ := sharedGraph()
	charts, err := NewSeparateGenerator().Generate(context.Background(), graph, Options{ChartName: "app", ChartVersion: "0.1.0", SharedChart: "platform"})
	if err != nil {
		t.Fatal(err)
	}
	web := chartByName(charts, "web")
	if !strings.Contains(web.Helpers, `"web-own"`) {
		t.Errorf("web helpers do not list its own ConfigMap:\n%s", web.Helpers)
	}
	if strings.Contains(web.Helpers, `"common"`) || strings.Contains(web.Helpers, `"spare"`) {
		t.Errorf("web helpers list shared objects, a prefix would be put in front of their names:\n%s", web.Helpers)
	}
	if !strings.Contains(web.Helpers, `"web"`) {
		t.Errorf("web helpers lack the workload names:\n%s", web.Helpers)
	}
}

// The service charts refer to the shared objects by their names in the input:
// a global.namePrefix on the shared chart would rename them, so the chart
// refuses it. The service charts keep the prefix.
func TestSharedChart_RefusesNamePrefix(t *testing.T) {
	graph, _ := sharedGraph()
	for name, gen := range map[string]Generator{"separate": NewSeparateGenerator(), "library": NewLibraryGenerator()} {
		charts, err := gen.Generate(context.Background(), graph, Options{ChartName: "app", ChartVersion: "0.1.0", SharedChart: "platform"})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		platform := chartByName(charts, "platform")
		if platform == nil {
			t.Fatalf("%s: no platform chart", name)
		}
		for _, want := range []string{`fail "global.namePrefix must stay empty in the shared chart platform`, `define "platform.namePrefix"`} {
			if !strings.Contains(platform.Helpers, want) {
				t.Errorf("%s: platform helpers lack %q:\n%s", name, want, platform.Helpers)
			}
		}
		for _, c := range charts {
			if c.Name != "platform" && strings.Contains(c.Helpers, "must stay empty") {
				t.Errorf("%s: service chart %s refuses the name prefix", name, c.Name)
			}
		}
	}
}

func TestSeparateGenerator_SharedChart_NothingShared(t *testing.T) {
	cm := sharedObject("ConfigMap", "cfg", "ns", "")
	web := sharedWorkload("web", "ns", "web", cm)
	graph := buildGraph([]*types.ProcessedResource{cm, web}, nil)
	called := false
	charts, err := NewSeparateGenerator().Generate(context.Background(), graph, Options{
		ChartName: "app", ChartVersion: "0.1.0", SharedChart: "platform",
		OnShared: func(chart string, moved []SharedObject) { called = len(moved) == 0 },
	})
	if err != nil {
		t.Fatal(err)
	}
	if chartByName(charts, "platform") != nil {
		t.Error("an empty platform chart was written")
	}
	if !called {
		t.Error("OnShared was not called with an empty list")
	}
}

func TestSeparateGenerator_SharedChart_NameClash(t *testing.T) {
	graph, _ := sharedGraph()
	_, err := NewSeparateGenerator().Generate(context.Background(), graph, Options{ChartName: "app", ChartVersion: "0.1.0", SharedChart: "web"})
	if err == nil || !strings.Contains(err.Error(), "web") {
		t.Fatalf("err = %v, want a clash with the service chart web", err)
	}
}

func TestSeparateGenerator_SharedChart_InvalidName(t *testing.T) {
	graph, _ := sharedGraph()
	for _, name := range []string{"Platform", "a/b", "-x", "a b"} {
		_, err := NewSeparateGenerator().Generate(context.Background(), graph, Options{ChartName: "app", ChartVersion: "0.1.0", SharedChart: name})
		if err == nil {
			t.Errorf("--shared-chart %q was accepted", name)
		}
	}
}

func TestLibraryGenerator_SharedChart(t *testing.T) {
	graph, _ := sharedGraph()
	charts, err := NewLibraryGenerator().Generate(context.Background(), graph, Options{ChartName: "app", ChartVersion: "0.1.0", SharedChart: "platform"})
	if err != nil {
		t.Fatal(err)
	}
	platform := chartByName(charts, "platform")
	if platform == nil || !hasTemplate(platform, "templates/configmap-common.yaml") {
		t.Fatalf("charts = %v, platform = %v", chartNamesOf(charts), platform)
	}
	// A plain chart: it needs no library.
	if strings.Contains(platform.ChartYAML, "library") || strings.Contains(platform.ChartYAML, "dependencies") {
		t.Errorf("platform chart depends on the library:\n%s", platform.ChartYAML)
	}
	if platform.Helpers == "" {
		t.Error("platform chart has no helpers")
	}
	web := chartByName(charts, "web")
	if web == nil || hasTemplate(web, "templates/configmap-common.yaml") {
		t.Fatalf("web chart = %v", templateKeys(web))
	}
	// The service chart lists its own names for the library's ref helper.
	var inChart string
	for _, f := range web.ExternalFiles {
		if f.Path == inChartTemplatePath {
			inChart = f.Content
		}
	}
	if inChart == "" || strings.Contains(inChart, `"common"`) || !strings.Contains(inChart, `"web-own"`) {
		t.Errorf("web inChart template:\n%s", inChart)
	}
	if chartByName(charts, "library") == nil {
		t.Error("library chart is gone")
	}
}

func TestLibraryGenerator_SharedChart_NameOfLibrary(t *testing.T) {
	graph, _ := sharedGraph()
	_, err := NewLibraryGenerator().Generate(context.Background(), graph, Options{ChartName: "app", ChartVersion: "0.1.0", SharedChart: "library"})
	if err == nil {
		t.Fatal("the shared chart may not take the name of the library")
	}
}

func TestOtherGenerators_RejectSharedChart(t *testing.T) {
	graph, _ := sharedGraph()
	opts := Options{ChartName: "app", ChartVersion: "0.1.0", SharedChart: "platform"}
	if _, err := NewUmbrellaGenerator().Generate(context.Background(), graph, opts); err == nil {
		t.Error("umbrella accepted --shared-chart")
	}
	if _, err := NewUniversalGenerator().Generate(context.Background(), graph, opts); err == nil {
		t.Error("universal accepted --shared-chart")
	}
}

func templateKeys(c *types.GeneratedChart) []string {
	if c == nil {
		return nil
	}
	var out []string
	for k := range c.Templates {
		out = append(out, k)
	}
	return out
}
