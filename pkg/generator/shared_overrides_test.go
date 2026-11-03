package generator

import (
	"context"
	"reflect"
	"strings"
	"testing"

	yamlv3 "gopkg.in/yaml.v3"
	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestValidateSharedOverrides(t *testing.T) {
	cases := []struct {
		name          string
		keep, exclude []string
		wantErr       string
	}{
		{name: "empty"},
		{name: "fine", keep: []string{"ConfigMap/common", "secret/tls.cert"}, exclude: []string{"PersistentVolumeClaim/data"}},
		{name: "no slash", keep: []string{"common"}, wantErr: "--shared-keep"},
		{name: "empty name", exclude: []string{"ConfigMap/"}, wantErr: "--shared-exclude"},
		{name: "empty kind", keep: []string{"/common"}, wantErr: "Kind/name"},
		{name: "two slashes", keep: []string{"ConfigMap/a/b"}, wantErr: "Kind/name"},
		{name: "space in name", keep: []string{"ConfigMap/a b"}, wantErr: "Kind/name"},
		{name: "both", keep: []string{"ConfigMap/common"}, exclude: []string{"configmap/common"}, wantErr: "both --shared-keep and --shared-exclude"},
	}
	for _, tc := range cases {
		err := ValidateSharedOverrides(tc.keep, tc.exclude)
		if tc.wantErr == "" && err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
		if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
			t.Errorf("%s: error = %v, want one containing %q", tc.name, err, tc.wantErr)
		}
	}
}

// overrideGroups: web and api reference common (it sits in web); spare is
// referenced by nobody (it sits in the group ns); web-own is web's only.
func overrideGroups() []*ServiceGroup {
	common := sharedObject("ConfigMap", "common", "ns", "")
	own := sharedObject("ConfigMap", "web-own", "ns", "")
	spare := sharedObject("Secret", "spare", "ns", "")
	web := sharedWorkload("web", "ns", "web", common, own)
	api := sharedWorkload("api", "ns", "api", common)
	return []*ServiceGroup{groupOf("api", api), groupOf("ns", spare), groupOf("web", web, common, own)}
}

func TestSplitSharedWith_NoOverridesIsSplitShared(t *testing.T) {
	a := SplitShared(overrideGroups())
	b := SplitSharedWith(overrideGroups(), SharedOverrides{})
	if !reflect.DeepEqual(sharedNames(a), sharedNames(b)) || len(b.Notes) != 0 {
		t.Errorf("shared = %v / %v, notes = %v", sharedNames(a), sharedNames(b), b.Notes)
	}
	if want := []string{"ConfigMap/common", "Secret/spare"}; !reflect.DeepEqual(sharedNames(b), want) {
		t.Errorf("shared = %v, want %v", sharedNames(b), want)
	}
}

func TestSplitSharedWith_Exclude(t *testing.T) {
	split := SplitSharedWith(overrideGroups(), SharedOverrides{Exclude: []string{"configmap/common", "Secret/spare"}})
	if len(split.Shared) != 0 {
		t.Fatalf("shared = %v, want none", sharedNames(split))
	}
	if len(split.Notes) != 0 {
		t.Errorf("notes = %v", split.Notes)
	}
	// Where grouping put them.
	if got, want := names(findGroupByName(split.Groups, "web").Resources), []string{"Deployment/web", "ConfigMap/common", "ConfigMap/web-own"}; !reflect.DeepEqual(got, want) {
		t.Errorf("web group = %v, want %v", got, want)
	}
	if g := findGroupByName(split.Groups, "ns"); g == nil || !reflect.DeepEqual(names(g.Resources), []string{"Secret/spare"}) {
		t.Errorf("groups = %v", groupNames(split.Groups))
	}
}

// common is referenced by api and web, grouping put it in web: kept, it goes
// to the first of the groups that reference it, api, and the Note says so.
func TestSplitSharedWith_KeepWithFirstReferencingGroup(t *testing.T) {
	split := SplitSharedWith(overrideGroups(), SharedOverrides{Keep: []string{"ConfigMap/common"}})
	if want := []string{"Secret/spare"}; !reflect.DeepEqual(sharedNames(split), want) {
		t.Fatalf("shared = %v, want %v", sharedNames(split), want)
	}
	if got, want := names(findGroupByName(split.Groups, "api").Resources), []string{"Deployment/api", "ConfigMap/common"}; !reflect.DeepEqual(got, want) {
		t.Errorf("api group = %v, want %v", got, want)
	}
	if got, want := names(findGroupByName(split.Groups, "web").Resources), []string{"Deployment/web", "ConfigMap/web-own"}; !reflect.DeepEqual(got, want) {
		t.Errorf("web group = %v, want %v", got, want)
	}
	if len(split.Notes) != 1 || !strings.Contains(split.Notes[0], "ConfigMap/common stays in chart api") || !strings.Contains(split.Notes[0], "api, web") {
		t.Errorf("notes = %q", split.Notes)
	}
}

// Already in the first group: stays, nothing is rearranged.
func TestSplitSharedWith_KeepAlreadyInFirstGroup(t *testing.T) {
	common := sharedObject("ConfigMap", "common", "ns", "")
	api := sharedWorkload("api", "ns", "api", common)
	web := sharedWorkload("web", "ns", "web", common)
	split := SplitSharedWith([]*ServiceGroup{groupOf("api", api, common), groupOf("web", web)}, SharedOverrides{Keep: []string{"ConfigMap/common"}})
	if len(split.Shared) != 0 || split.relocated {
		t.Errorf("shared = %v, relocated = %v", sharedNames(split), split.relocated)
	}
	if got, want := names(findGroupByName(split.Groups, "api").Resources), []string{"Deployment/api", "ConfigMap/common"}; !reflect.DeepEqual(got, want) {
		t.Errorf("api group = %v, want %v", got, want)
	}
}

// A group that held only the kept object's slot does not disappear and one
// that is left empty does.
func TestSplitSharedWith_KeepUnreferencedStaysInItsGroup(t *testing.T) {
	split := SplitSharedWith(overrideGroups(), SharedOverrides{Keep: []string{"Secret/spare"}})
	if want := []string{"ConfigMap/common"}; !reflect.DeepEqual(sharedNames(split), want) {
		t.Fatalf("shared = %v, want %v", sharedNames(split), want)
	}
	if g := findGroupByName(split.Groups, "ns"); g == nil || !reflect.DeepEqual(names(g.Resources), []string{"Secret/spare"}) {
		t.Errorf("groups = %v", groupNames(split.Groups))
	}
	// Nothing references it: kept, it makes a chart of its own, and the user
	// is told.
	want := []string{"--shared-keep: Secret/spare is referenced by no workload; it stays in chart ns, which has no workload"}
	if !reflect.DeepEqual(split.Notes, want) {
		t.Errorf("notes = %q, want %q", split.Notes, want)
	}
}

func TestSplitSharedWith_KeepOfAnObjectThatStaysAnyway(t *testing.T) {
	split := SplitSharedWith(overrideGroups(), SharedOverrides{Keep: []string{"ConfigMap/web-own"}})
	if len(split.Notes) != 0 || split.relocated {
		t.Errorf("notes = %v, relocated = %v", split.Notes, split.relocated)
	}
}

func TestSplitSharedWith_NotesForUnknownEntries(t *testing.T) {
	split := SplitSharedWith(overrideGroups(), SharedOverrides{
		Keep:    []string{"ConfigMap/nope", "Deployment/web", "oops"},
		Exclude: []string{"Secret/missing"},
	})
	want := []string{
		"--shared-exclude: no object Secret/missing in the input, ignored",
		"--shared-keep: \"oops\" is not Kind/name (e.g. ConfigMap/common), ignored",
		"--shared-keep: Deployment/web is a workload, a CRD or has no template of its own: it is never moved, the entry has no effect",
		"--shared-keep: no object ConfigMap/nope in the input, ignored",
	}
	if !reflect.DeepEqual(split.Notes, want) {
		t.Errorf("notes:\n%s\nwant:\n%s", strings.Join(split.Notes, "\n"), strings.Join(want, "\n"))
	}
	// The unknown entries changed nothing.
	if want := []string{"ConfigMap/common", "Secret/spare"}; !reflect.DeepEqual(sharedNames(split), want) {
		t.Errorf("shared = %v, want %v", sharedNames(split), want)
	}
}

func TestSplitSharedWith_Deterministic(t *testing.T) {
	ov := SharedOverrides{Keep: []string{"ConfigMap/common", "ConfigMap/zzz"}, Exclude: []string{"Secret/spare"}}
	first := SplitSharedWith(overrideGroups(), ov)
	for i := 0; i < 20; i++ {
		next := SplitSharedWith(overrideGroups(), ov)
		if !reflect.DeepEqual(next.Notes, first.Notes) || !reflect.DeepEqual(groupNames(next.Groups), groupNames(first.Groups)) {
			t.Fatalf("run %d differs: %v / %v", i, next.Notes, first.Notes)
		}
		for j, g := range next.Groups {
			if !reflect.DeepEqual(names(g.Resources), names(first.Groups[j].Resources)) {
				t.Fatalf("run %d: group %s = %v, want %v", i, g.Name, names(g.Resources), names(first.Groups[j].Resources))
			}
		}
	}
}

func TestSplitSharedWith_DoesNotModifyInput(t *testing.T) {
	groups := overrideGroups()
	SplitSharedWith(groups, SharedOverrides{Keep: []string{"ConfigMap/common"}})
	if got := names(findGroupByName(groups, "web").Resources); len(got) != 3 {
		t.Errorf("input group changed: %v", got)
	}
	if got := names(findGroupByName(groups, "api").Resources); len(got) != 1 {
		t.Errorf("input group changed: %v", got)
	}
}

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

func TestSeparateGenerator_SharedChart_ExcludeAndKeep(t *testing.T) {
	graph, _ := sharedGraph()
	var notes []string
	charts, err := NewSeparateGenerator().Generate(context.Background(), graph, Options{
		ChartName: "app", ChartVersion: "0.1.0", SharedChart: "platform",
		SharedExclude: []string{"Secret/spare"}, SharedKeep: []string{"configmap/common", "ConfigMap/ghost"},
		OnSharedNote: func(n string) { notes = append(notes, n) },
	})
	if err != nil {
		t.Fatal(err)
	}
	// Nothing is left to share: the platform chart is not written.
	if chartByName(charts, "platform") != nil {
		t.Errorf("charts = %v, want no platform chart", chartNamesOf(charts))
	}
	api := chartByName(charts, "api")
	if api == nil || !hasTemplate(api, "templates/configmap-common.yaml") {
		t.Errorf("common stays with the first referencing group, api: %v", templateKeys(api))
	}
	if web := chartByName(charts, "web"); web == nil || hasTemplate(web, "templates/configmap-common.yaml") {
		t.Errorf("web must not carry common: %v", templateKeys(web))
	}
	joined := strings.Join(notes, "\n")
	for _, want := range []string{"ConfigMap/common stays in chart api", "no object ConfigMap/ghost"} {
		if !strings.Contains(joined, want) {
			t.Errorf("notes lack %q:\n%s", want, joined)
		}
	}
}

func TestLibraryGenerator_SharedChart_Exclude(t *testing.T) {
	graph, _ := sharedGraph()
	charts, err := NewLibraryGenerator().Generate(context.Background(), graph, Options{
		ChartName: "app", ChartVersion: "0.1.0", SharedChart: "platform", SharedExclude: []string{"ConfigMap/common"},
	})
	if err != nil {
		t.Fatal(err)
	}
	platform := chartByName(charts, "platform")
	if platform == nil || hasTemplate(platform, "templates/configmap-common.yaml") || !hasTemplate(platform, "templates/secret-spare.yaml") {
		t.Fatalf("platform = %v", templateKeys(platform))
	}
	holders := 0
	for _, c := range charts {
		if c.Name != "platform" && hasTemplate(c, "templates/configmap-common.yaml") {
			holders++
		}
	}
	if holders != 1 {
		t.Errorf("common is in %d service charts, want 1", holders)
	}
}

func TestSharedChart_OverrideErrors(t *testing.T) {
	graph, _ := sharedGraph()
	cases := map[string]Options{
		"keep without chart":    {SharedKeep: []string{"ConfigMap/common"}},
		"exclude without chart": {SharedExclude: []string{"ConfigMap/common"}},
		"bad entry":             {SharedChart: "platform", SharedKeep: []string{"common"}},
		"both":                  {SharedChart: "platform", SharedKeep: []string{"ConfigMap/common"}, SharedExclude: []string{"ConfigMap/common"}},
	}
	for name, opts := range cases {
		opts.ChartName, opts.ChartVersion = "app", "0.1.0"
		if _, err := NewSeparateGenerator().Generate(context.Background(), graph, opts); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// ---------------------------------------------------------------------------
// helm.sh/resource-policy: keep
// ---------------------------------------------------------------------------

const standardAnnotations = "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\n  {{- with $cm.annotations }}\n  annotations:\n    {{- toYaml . | nindent 4 }}\n  {{- end }}\n"

func policyResource(template string, values map[string]interface{}) *types.ProcessedResource {
	r := sharedObject("ConfigMap", "x", "ns", "")
	r.TemplateContent = template
	r.Values = values
	return r
}

func TestWithKeepPolicy_ValuesAnnotations(t *testing.T) {
	cases := []struct {
		name   string
		values map[string]interface{}
		want   int // annotations in the result
	}{
		{"none", map[string]interface{}{"data": map[string]interface{}{"k": "v"}}, 1},
		{"nil values", nil, 1},
		{"string map", map[string]interface{}{"annotations": map[string]string{"team": "a"}}, 2},
		{"interface map", map[string]interface{}{"annotations": map[string]interface{}{"team": "a"}}, 2},
		{"already present", map[string]interface{}{"annotations": map[string]interface{}{"team": "a", keepPolicyKey: "keep"}}, 2},
	}
	for _, tc := range cases {
		r := policyResource(standardAnnotations, tc.values)
		got, problem := withKeepPolicy(r)
		if problem != "" {
			t.Errorf("%s: %s", tc.name, problem)
		}
		ann, _ := got.Values["annotations"].(map[string]interface{})
		if ann[keepPolicyKey] != "keep" || len(ann) != tc.want {
			t.Errorf("%s: annotations = %v, want %d with the policy", tc.name, got.Values["annotations"], tc.want)
		}
		if got.TemplateContent != standardAnnotations {
			t.Errorf("%s: the template changed:\n%s", tc.name, got.TemplateContent)
		}
	}
	// The input resource is left alone.
	r := policyResource(standardAnnotations, map[string]interface{}{"annotations": map[string]string{"team": "a"}})
	withKeepPolicy(r)
	if ann, ok := r.Values["annotations"].(map[string]string); !ok || len(ann) != 1 {
		t.Errorf("input values were modified: %v", r.Values)
	}
}

func TestWithKeepPolicy_ExistingValueIsKept(t *testing.T) {
	r := policyResource(standardAnnotations, map[string]interface{}{"annotations": map[string]interface{}{keepPolicyKey: "delete"}})
	got, _ := withKeepPolicy(r)
	if ann := got.Values["annotations"].(map[string]interface{}); ann[keepPolicyKey] != "delete" {
		t.Errorf("an explicit policy of the object is replaced: %v", ann)
	}
}

func TestWithKeepPolicy_TemplateShapes(t *testing.T) {
	const plain = "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\ndata: {}\n"
	got, problem := withKeepPolicy(policyResource(plain, nil))
	if problem != "" || !strings.Contains(got.TemplateContent, "metadata:\n  annotations:\n    helm.sh/resource-policy: keep\n  name: x\n") {
		t.Errorf("no annotations: %q / %s", got.TemplateContent, problem)
	}
	assertYAMLAnnotations(t, got.TemplateContent, 1)

	const literal = "apiVersion: example.com/v1\nkind: Widget\nmetadata:\n  name: x\n  annotations:\n    example.com/note: hi\nspec: {}\n"
	got, problem = withKeepPolicy(policyResource(literal, nil))
	if problem != "" || !strings.Contains(got.TemplateContent, "  annotations:\n    helm.sh/resource-policy: keep\n    example.com/note: hi\n") {
		t.Errorf("literal annotations: %q / %s", got.TemplateContent, problem)
	}
	assertYAMLAnnotations(t, got.TemplateContent, 2)

	// The template renders the annotation itself: left alone.
	own := literal + "# helm.sh/resource-policy handled above\n"
	if got, problem = withKeepPolicy(policyResource(own, nil)); problem != "" || got.TemplateContent != own {
		t.Errorf("own policy: %q / %s", got.TemplateContent, problem)
	}

	conditional := "apiVersion: v1\nkind: Job\nmetadata:\n  name: x\n  {{- if .hook }}\n  annotations:\n    {{- toYaml .hook | nindent 4 }}\n  {{- end }}\n"
	got, problem = withKeepPolicy(policyResource(conditional, nil))
	if problem == "" || got.TemplateContent != conditional || !strings.Contains(problem, "ConfigMap/x") {
		t.Errorf("conditional annotations: %q / %q", got.TemplateContent, problem)
	}

	if _, problem = withKeepPolicy(policyResource("{{ include \"x\" . }}\n", nil)); problem == "" {
		t.Error("a template without a metadata block is not reported")
	}
}

func assertYAMLAnnotations(t *testing.T, doc string, want int) {
	t.Helper()
	// yaml.v3 refuses a duplicated key.
	var obj struct {
		Metadata struct {
			Annotations map[string]string `yaml:"annotations"`
		} `yaml:"metadata"`
	}
	if err := yamlv3.Unmarshal([]byte(doc), &obj); err != nil {
		t.Errorf("not valid YAML (a duplicated key?): %v\n%s", err, doc)
		return
	}
	if len(obj.Metadata.Annotations) != want || obj.Metadata.Annotations[keepPolicyKey] != "keep" {
		t.Errorf("annotations = %v, want %d with the policy", obj.Metadata.Annotations, want)
	}
}

// The objects of the shared chart carry the annotation; the objects of the
// service charts do not.
func TestSharedChart_ObjectsGetResourcePolicyKeep(t *testing.T) {
	graph, all := sharedGraph()
	// The standard shape: annotations come from the values of the object.
	all["common"].TemplateContent = standardAnnotations
	all["common"].Values = map[string]interface{}{"annotations": map[string]string{"team": "a"}, "data": map[string]interface{}{"LOG": "info"}}
	all["common"].ValuesPath = ""
	charts, err := NewSeparateGenerator().Generate(context.Background(), graph, Options{ChartName: "app", ChartVersion: "0.1.0", SharedChart: "platform"})
	if err != nil {
		t.Fatal(err)
	}
	platform := chartByName(charts, "platform")
	if platform == nil {
		t.Fatal("no platform chart")
	}

	var values struct {
		ConfigMaps map[string]struct {
			Annotations map[string]string      `json:"annotations"`
			Data        map[string]interface{} `json:"data"`
		} `json:"configMaps"`
	}
	if err := yaml.Unmarshal([]byte(platform.ValuesYAML), &values); err != nil {
		t.Fatal(err)
	}
	cm, ok := values.ConfigMaps["common"]
	if !ok {
		t.Fatalf("ConfigMap data is not in values under configMaps.common:\n%s", platform.ValuesYAML)
	}
	if cm.Annotations[keepPolicyKey] != "keep" || cm.Annotations["team"] != "a" || len(cm.Annotations) != 2 {
		t.Errorf("annotations = %v", cm.Annotations)
	}
	if cm.Data["LOG"] != "info" {
		t.Errorf("data = %v", cm.Data)
	}
	if got := platform.Templates["templates/secret-spare.yaml"]; !strings.Contains(got, "metadata:\n  annotations:\n    helm.sh/resource-policy: keep\n") {
		t.Errorf("secret-spare template:\n%s", got)
	}
	for _, name := range []string{"web", "api"} {
		c := chartByName(charts, name)
		for path, content := range c.Templates {
			if strings.Contains(content, keepPolicyKey) {
				t.Errorf("%s/%s carries the policy", name, path)
			}
		}
		if strings.Contains(c.ValuesYAML, keepPolicyKey) {
			t.Errorf("%s values carry the policy", name)
		}
	}
	// The graph's resources are left as they were.
	if _, ok := all["common"].Values["annotations"].(map[string]string); !ok {
		t.Errorf("input values were modified: %v", all["common"].Values)
	}
}
