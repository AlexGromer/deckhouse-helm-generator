package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/extractor"
	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func extracted(apiVersion, kind, name string) *types.ExtractedResource {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	obj.SetAPIVersion(apiVersion)
	obj.SetKind(kind)
	obj.SetName(name)
	obj.SetNamespace("app")
	return &types.ExtractedResource{Object: obj, GVK: obj.GroupVersionKind()}
}

func depKey(kind, name string) types.ResourceKey {
	return types.ResourceKey{GVK: schema.GroupVersionKind{Version: "v1", Kind: kind}, Namespace: "app", Name: name}
}

func processedWith(r *types.ExtractedResource, deps ...types.ResourceKey) *types.ProcessedResource {
	return &types.ProcessedResource{Original: r, Dependencies: deps}
}

func TestMissingReferenceNotes(t *testing.T) {
	web := extracted("apps/v1", "Deployment", "web")
	cfg := extracted("v1", "ConfigMap", "cfg")
	processed := []*types.ProcessedResource{
		processedWith(web,
			depKey("ConfigMap", "cfg"),              // in the chart
			depKey("ConfigMap", "missing-cm"),       // not extracted
			depKey("Secret", "db"),                  // not extracted
			depKey("ServiceAccount", "web"),         // not extracted
			depKey("ServiceAccount", "default"),     // exists in every namespace
			depKey("ConfigMap", "kube-root-ca.crt"), // exists in every namespace
			depKey("Service", "other"),              // not a workload reference kind
			depKey("Secret", "db"),                  // duplicate
		),
		processedWith(cfg),
	}
	all := map[types.ResourceKey]*types.ExtractedResource{web.ResourceKey(): web, cfg.ResourceKey(): cfg}

	got := missingReferenceNotes(processed, all, types.SourceFile, "", false)
	want := []string{
		"ConfigMap missing-cm is referenced by Deployment web but is not in the chart (create it before install)",
		"Secret db is referenced by Deployment web but is not in the chart (create it before install)",
		"ServiceAccount web is referenced by Deployment web but is not in the chart (create it before install)",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("notes:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestMissingReferenceNotes_ClusterSecretsSkipped(t *testing.T) {
	job := extracted("batch/v1", "Job", "migrate")
	processed := []*types.ProcessedResource{processedWith(job, depKey("ConfigMap", "cfg"), depKey("Secret", "db"))}
	all := map[types.ResourceKey]*types.ExtractedResource{job.ResourceKey(): job}

	for _, mode := range []string{"", "skip"} {
		got := missingReferenceNotes(processed, all, types.SourceCluster, mode, false)
		if len(got) != 2 {
			t.Fatalf("mode %q: got %v", mode, got)
		}
		if !strings.Contains(got[1], "Secret db") || !strings.Contains(got[1], "skipped by --cluster-secrets") {
			t.Errorf("mode %q: Secret note must say it was skipped: %q", mode, got[1])
		}
		if strings.Contains(got[0], "--cluster-secrets") {
			t.Errorf("mode %q: ConfigMap note must not mention secrets: %q", mode, got[0])
		}
	}
	if got := missingReferenceNotes(processed, all, types.SourceCluster, "mask", false); strings.Contains(strings.Join(got, ""), "--cluster-secrets") {
		t.Errorf("mask mode extracts Secrets, nothing was skipped: %v", got)
	}
}

func TestMissingReferenceNotes_OneNotePerObject(t *testing.T) {
	a := extracted("apps/v1", "Deployment", "a")
	b := extracted("batch/v1", "Job", "b")
	processed := []*types.ProcessedResource{
		processedWith(a, depKey("Secret", "db"), depKey("ConfigMap", "shared")),
		processedWith(b, depKey("Secret", "db")),
	}
	all := map[types.ResourceKey]*types.ExtractedResource{a.ResourceKey(): a, b.ResourceKey(): b}

	got := missingReferenceNotes(processed, all, types.SourceCluster, "skip", false)
	want := []string{
		"Secret db is referenced by Deployment a, Job b but is not in the chart (create it before install; Secrets are skipped by --cluster-secrets skip)",
		"ConfigMap shared is referenced by Deployment a but is not in the chart (create it before install)",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("notes:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestMissingReferenceNotes_ManyWorkloadsCollapsed(t *testing.T) {
	var processed []*types.ProcessedResource
	all := map[types.ResourceKey]*types.ExtractedResource{}
	for _, name := range []string{"w1", "w2", "w3", "w4", "w5"} {
		w := extracted("apps/v1", "Deployment", name)
		all[w.ResourceKey()] = w
		processed = append(processed, processedWith(w, depKey("ConfigMap", "common")))
	}

	got := missingReferenceNotes(processed, all, types.SourceFile, "", false)
	want := "ConfigMap common is referenced by Deployment w1, Deployment w2, Deployment w3 +2 more but is not in the chart (create it before install)"
	if len(got) != 1 || got[0] != want {
		t.Errorf("notes = %q; want [%q]", got, want)
	}

	// Under -v every workload is listed, nothing is collapsed.
	verbose := missingReferenceNotes(processed, all, types.SourceFile, "", true)
	if len(verbose) != 1 || !strings.Contains(verbose[0], "Deployment w5") || strings.Contains(verbose[0], "+2 more") {
		t.Errorf("verbose notes = %q; want all five workloads and no '+2 more'", verbose)
	}
}

func TestMissingReferenceNotes_SeveralNamespaces(t *testing.T) {
	inNS := func(ns, kind, name string) *types.ExtractedResource {
		r := extracted("apps/v1", kind, name)
		r.Object.SetNamespace(ns)
		r.GVK = r.Object.GroupVersionKind()
		return r
	}
	dep := func(ns, kind, name string) types.ResourceKey {
		k := depKey(kind, name)
		k.Namespace = ns
		return k
	}
	prodWeb := inNS("prod", "Deployment", "web")
	stagingWeb := inNS("staging", "Deployment", "web")
	processed := []*types.ProcessedResource{
		processedWith(prodWeb, dep("prod", "Secret", "db")),
		processedWith(stagingWeb, dep("staging", "Secret", "db")),
	}
	all := map[types.ResourceKey]*types.ExtractedResource{prodWeb.ResourceKey(): prodWeb, stagingWeb.ResourceKey(): stagingWeb}

	got := missingReferenceNotes(processed, all, types.SourceFile, "", false)
	want := []string{
		"Secret prod/db is referenced by Deployment prod/web but is not in the chart (create it before install)",
		"Secret staging/db is referenced by Deployment staging/web but is not in the chart (create it before install)",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("notes:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestMissingReferenceNotes_ManyObjectsCapped(t *testing.T) {
	web := extracted("apps/v1", "Deployment", "web")
	var deps []types.ResourceKey
	for i := 0; i < 25; i++ {
		deps = append(deps, depKey("ConfigMap", fmt.Sprintf("cm-%02d", i)))
	}
	processed := []*types.ProcessedResource{processedWith(web, deps...)}
	all := map[types.ResourceKey]*types.ExtractedResource{web.ResourceKey(): web}

	got := missingReferenceNotes(processed, all, types.SourceFile, "", false)
	if len(got) != 21 {
		t.Fatalf("got %d notes; want 20 objects and one summary", len(got))
	}
	if !strings.Contains(got[19], "cm-19") {
		t.Errorf("note 20 = %q", got[19])
	}
	if got[20] != "... and 5 more missing objects (use -v to list all)" {
		t.Errorf("summary = %q", got[20])
	}

	if verbose := missingReferenceNotes(processed, all, types.SourceFile, "", true); len(verbose) != 25 {
		t.Errorf("verbose: got %d notes; want all 25", len(verbose))
	}
}

func TestMissingReferenceNotes_NamespaceLessDefinition(t *testing.T) {
	web := extracted("apps/v1", "Deployment", "web")
	cfg := extracted("v1", "ConfigMap", "cfg")
	cfg.Object.SetNamespace("")
	processed := []*types.ProcessedResource{processedWith(web, depKey("ConfigMap", "cfg"))}
	all := map[types.ResourceKey]*types.ExtractedResource{web.ResourceKey(): web, cfg.ResourceKey(): cfg}
	if got := missingReferenceNotes(processed, all, types.SourceFile, "", false); len(got) != 0 {
		t.Errorf("a ConfigMap defined without namespace satisfies the reference: %v", got)
	}
}

func TestPrintExtractWarning(t *testing.T) {
	forbidden := &extractor.ForbiddenKindsError{
		Namespace: "prod",
		Kinds:     []string{"Service", "Ingress"},
		Details:   []string{"error listing Service: HTTP 403 ...", "error listing Ingress: HTTP 403 ..."},
	}
	tests := []struct {
		name    string
		err     error
		verbose bool
		want    string
	}{
		{"note", &extractor.NoteError{Msg: "Job x has already finished"}, false, "Note: Job x has already finished\n"},
		{"forbidden summary", forbidden, false,
			"Warning: no permission to list 2 kinds in prod: Service, Ingress (use -v for details)\n"},
		{"forbidden verbose", forbidden, true,
			"Warning: no permission to list 2 kinds in prod: Service, Ingress\n  error listing Service: HTTP 403 ...\n  error listing Ingress: HTTP 403 ...\n"},
		{"plain error", errTest("boom"), false, "Warning: boom\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printExtractWarning(&buf, tt.err, tt.verbose)
			if buf.String() != tt.want {
				t.Errorf("got %q; want %q", buf.String(), tt.want)
			}
		})
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
