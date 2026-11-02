package extractor

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// drainCluster runs Extract and returns everything it produced.
func drainCluster(t *testing.T, fake *fakeKubeAPIServer, cfg ClusterExtractorConfig, opts Options) ([]*types.ExtractedResource, []error) {
	t.Helper()
	ce := NewClusterExtractorWithConfig(cfg)
	ce.SetClient(fake.client())
	resCh, errCh := ce.Extract(context.Background(), opts)
	var resources []*types.ExtractedResource
	var errs []error
	for resCh != nil || errCh != nil {
		select {
		case r, ok := <-resCh:
			if !ok {
				resCh = nil
				continue
			}
			resources = append(resources, r)
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			errs = append(errs, err)
		}
	}
	return resources, errs
}

func scopedFake() *fakeKubeAPIServer {
	fake := newFakeKubeAPIServer()
	fake.setResponse("/api/v1", coreResourceList(
		k8sResourceEntry{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: []string{"get", "list"}},
		k8sResourceEntry{Name: "persistentvolumes", Kind: "PersistentVolume", Namespaced: false, Verbs: []string{"get", "list"}},
	))
	fake.setResponse("/apis", emptyGroupList())
	fake.setResponse("/api/v1/namespaces/prod/configmaps", itemList(configMapItem("cm", "prod")))
	fake.setResponse("/api/v1/persistentvolumes", itemList(map[string]interface{}{
		"apiVersion": "v1", "kind": "PersistentVolume", "metadata": map[string]interface{}{"name": "pv1"},
	}))
	return fake
}

func TestClusterExtractor_Extract_NamespaceSkipsClusterScoped(t *testing.T) {
	fake := scopedFake()
	defer fake.close()

	resources, errs := drainCluster(t, fake, ClusterExtractorConfig{Namespace: "prod"}, Options{})
	if len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	if len(resources) != 1 || resources[0].Object.GetKind() != "ConfigMap" {
		t.Fatalf("got %d resources; want only the ConfigMap", len(resources))
	}
}

func TestClusterExtractor_Extract_NamespaceKeepsExplicitClusterScopedKind(t *testing.T) {
	fake := scopedFake()
	defer fake.close()

	resources, _ := drainCluster(t, fake, ClusterExtractorConfig{Namespace: "prod"}, Options{IncludeKinds: []string{"PersistentVolume"}})
	if len(resources) != 1 || resources[0].Object.GetKind() != "PersistentVolume" {
		t.Fatalf("got %d resources; want the explicitly requested PersistentVolume", len(resources))
	}
}

func TestClusterExtractor_Extract_NoNamespaceListsClusterScoped(t *testing.T) {
	fake := scopedFake()
	defer fake.close()
	fake.setResponse("/api/v1/configmaps", itemList(configMapItem("cm", "prod")))

	resources, _ := drainCluster(t, fake, ClusterExtractorConfig{}, Options{})
	if len(resources) != 2 {
		t.Fatalf("got %d resources; want 2 (cluster-wide extraction keeps cluster-scoped kinds)", len(resources))
	}
}

func multiNamespaceFake() *fakeKubeAPIServer {
	fake := scopedFake()
	fake.setResponse("/api/v1/namespaces/staging/configmaps", itemList(configMapItem("cm-staging", "staging")))
	fake.setResponse("/api/v1/namespaces/kube-extra/configmaps", itemList(configMapItem("cm-system", "kube-extra")))
	return fake
}

func kindNames(resources []*types.ExtractedResource) []string {
	var names []string
	for _, r := range resources {
		names = append(names, r.Object.GetKind()+"/"+r.Object.GetName())
	}
	sort.Strings(names)
	return names
}

func TestClusterExtractor_Extract_NamespacesListsEach(t *testing.T) {
	fake := multiNamespaceFake()
	defer fake.close()

	resources, errs := drainCluster(t, fake, ClusterExtractorConfig{}, Options{Namespaces: []string{"prod", "staging"}})
	if len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	// The cluster-scoped PersistentVolume is skipped like with -n.
	want := []string{"ConfigMap/cm", "ConfigMap/cm-staging"}
	if got := kindNames(resources); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}
}

func TestClusterExtractor_Extract_NamespacesCombineWithNamespace(t *testing.T) {
	fake := multiNamespaceFake()
	defer fake.close()

	// A repeated namespace is listed once.
	resources, _ := drainCluster(t, fake, ClusterExtractorConfig{}, Options{Namespace: "prod", Namespaces: []string{"staging", "prod"}})
	want := []string{"ConfigMap/cm", "ConfigMap/cm-staging"}
	if got := kindNames(resources); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}
}

func TestClusterExtractor_Extract_NamespacesKeepExplicitClusterScopedKind(t *testing.T) {
	fake := multiNamespaceFake()
	defer fake.close()

	opts := Options{Namespaces: []string{"prod", "staging"}, IncludeKinds: []string{"ConfigMap", "PersistentVolume"}}
	resources, _ := drainCluster(t, fake, ClusterExtractorConfig{}, opts)
	// The PersistentVolume is listed once, not once per namespace.
	want := []string{"ConfigMap/cm", "ConfigMap/cm-staging", "PersistentVolume/pv1"}
	if got := kindNames(resources); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}
}

func TestClusterExtractor_Extract_NamespacesBypassDefaultExclusion(t *testing.T) {
	fake := multiNamespaceFake()
	defer fake.close()

	resources, _ := drainCluster(t, fake, ClusterExtractorConfig{}, Options{Namespaces: []string{"prod", "kube-extra"}})
	want := []string{"ConfigMap/cm", "ConfigMap/cm-system"}
	if got := kindNames(resources); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v; want %v (an explicitly requested kube-* namespace is kept)", got, want)
	}
}

func TestClusterExtractor_Extract_NamespacesNamedInForbidden(t *testing.T) {
	fake := multiNamespaceFake()
	defer fake.close()
	fake.setResponse("/api/v1", coreResourceList(
		k8sResourceEntry{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: []string{"get", "list"}},
		k8sResourceEntry{Name: "services", Kind: "Service", Namespaced: true, Verbs: []string{"get", "list"}},
	))
	for _, ns := range []string{"prod", "staging"} {
		fake.mux.HandleFunc("/api/v1/namespaces/"+ns+"/services", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"forbidden: User cannot list resource"}`, http.StatusForbidden)
		})
	}

	resources, errs := drainCluster(t, fake, ClusterExtractorConfig{}, Options{Namespaces: []string{"prod", "staging"}})
	if len(resources) != 2 {
		t.Fatalf("got %d resources; want the two ConfigMaps", len(resources))
	}
	var forbidden *ForbiddenKindsError
	for _, err := range errs {
		errors.As(err, &forbidden)
	}
	if forbidden == nil {
		t.Fatalf("no ForbiddenKindsError in %v", errs)
	}
	if got := forbidden.Error(); got != "no permission to list 1 kinds in namespaces prod, staging: Service" {
		t.Errorf("summary = %q", got)
	}
	if len(forbidden.Details) != 2 || !strings.Contains(forbidden.Details[0], "prod") || !strings.Contains(forbidden.Details[1], "staging") {
		t.Errorf("details must name the namespace of each refusal: %v", forbidden.Details)
	}
}

func TestClusterExtractor_Extract_ForbiddenSummaryNamesOnlyRefusedNamespaces(t *testing.T) {
	fake := multiNamespaceFake()
	defer fake.close()
	fake.setResponse("/api/v1", coreResourceList(
		k8sResourceEntry{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: []string{"get", "list"}},
		k8sResourceEntry{Name: "services", Kind: "Service", Namespaced: true, Verbs: []string{"get", "list"}},
	))
	fake.setResponse("/api/v1/namespaces/prod/services", itemList())
	fake.mux.HandleFunc("/api/v1/namespaces/staging/services", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"forbidden: User cannot list resource"}`, http.StatusForbidden)
	})

	_, errs := drainCluster(t, fake, ClusterExtractorConfig{}, Options{Namespaces: []string{"prod", "staging"}})
	var forbidden *ForbiddenKindsError
	for _, err := range errs {
		errors.As(err, &forbidden)
	}
	if forbidden == nil {
		t.Fatalf("no ForbiddenKindsError in %v", errs)
	}
	if got := forbidden.Error(); got != "no permission to list 1 kinds in staging: Service" {
		t.Errorf("summary = %q; prod was listed fine and must not be named", got)
	}
}

func TestClusterExtractor_Extract_ListErrorNamesNamespace(t *testing.T) {
	fake := multiNamespaceFake()
	defer fake.close()
	fake.mux.HandleFunc("/api/v1/namespaces/staging/configmaps", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
	})

	_, errs := drainCluster(t, fake, ClusterExtractorConfig{}, Options{Namespaces: []string{"prod", "staging"}})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "error listing ConfigMap in staging:") {
		t.Fatalf("a failure in one of several namespaces must name it, got %v", errs)
	}

	// A single namespace is already known to the user: no repetition.
	fake2 := scopedFake()
	defer fake2.close()
	fake2.mux.HandleFunc("/api/v1/namespaces/prod/configmaps", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
	})
	_, errs = drainCluster(t, fake2, ClusterExtractorConfig{Namespace: "prod"}, Options{})
	if len(errs) != 1 || strings.Contains(errs[0].Error(), " in prod") {
		t.Fatalf("single namespace error = %v", errs)
	}
}

func TestClusterExtractor_Extract_AggregatesForbidden(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()
	fake.setResponse("/api/v1", coreResourceList(
		k8sResourceEntry{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: []string{"get", "list"}},
		k8sResourceEntry{Name: "services", Kind: "Service", Namespaced: true, Verbs: []string{"get", "list"}},
		k8sResourceEntry{Name: "resourcequotas", Kind: "ResourceQuota", Namespaced: true, Verbs: []string{"get", "list"}},
		k8sResourceEntry{Name: "limitranges", Kind: "LimitRange", Namespaced: true, Verbs: []string{"get", "list"}},
	))
	fake.setResponse("/apis", emptyGroupList())
	fake.setResponse("/api/v1/namespaces/prod/configmaps", itemList(configMapItem("cm", "prod")))
	for _, p := range []string{"services", "resourcequotas"} {
		fake.mux.HandleFunc("/api/v1/namespaces/prod/"+p, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"forbidden: User cannot list resource"}`, http.StatusForbidden)
		})
	}
	// limitranges is not registered in the fake: 404, a different failure that stays individual.

	resources, errs := drainCluster(t, fake, ClusterExtractorConfig{Namespace: "prod"}, Options{})
	if len(resources) != 1 {
		t.Fatalf("got %d resources; want the ConfigMap", len(resources))
	}
	var forbidden *ForbiddenKindsError
	var others []error
	for _, err := range errs {
		var fe *ForbiddenKindsError
		if errors.As(err, &fe) {
			forbidden = fe
		} else {
			others = append(others, err)
		}
	}
	if forbidden == nil {
		t.Fatalf("no ForbiddenKindsError in %v", errs)
	}
	if got := forbidden.Error(); got != "no permission to list 2 kinds in prod: Service, ResourceQuota" {
		t.Errorf("summary = %q", got)
	}
	if len(forbidden.Details) != 2 || !strings.Contains(forbidden.Details[0], "Service") || !strings.Contains(forbidden.Details[0], "HTTP 403") {
		t.Errorf("details = %v", forbidden.Details)
	}
	if len(others) != 1 || !strings.Contains(others[0].Error(), "LimitRange") {
		t.Errorf("non-403 errors must stay individual, got %v", others)
	}
}

func TestForbiddenKindsError_ClusterWide(t *testing.T) {
	e := &ForbiddenKindsError{Kinds: []string{"Node"}}
	if got := e.Error(); got != "no permission to list 1 kinds in the cluster: Node" {
		t.Errorf("Error() = %q", got)
	}
}

func TestClusterExtractor_Extract_FinishedJobNote(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()
	fake.setResponse("/api/v1", map[string]interface{}{"kind": "APIResourceList", "resources": []interface{}{}})
	fake.setResponse("/apis", map[string]interface{}{"kind": "APIGroupList", "groups": []interface{}{
		map[string]interface{}{
			"name":             "batch",
			"preferredVersion": map[string]interface{}{"groupVersion": "batch/v1", "version": "v1"},
			"versions":         []interface{}{map[string]interface{}{"groupVersion": "batch/v1", "version": "v1"}},
		},
	}})
	fake.setResponse("/apis/batch/v1", map[string]interface{}{"kind": "APIResourceList", "groupVersion": "batch/v1", "resources": []interface{}{
		map[string]interface{}{"name": "jobs", "kind": "Job", "namespaced": true, "verbs": []string{"get", "list"}},
	}})
	done := liveJob("migrate", nil)
	done.SetNamespace("prod")
	done.Object["status"] = map[string]interface{}{"succeeded": int64(1)}
	running := liveJob("worker", nil)
	running.SetNamespace("prod")
	fake.setResponse("/apis/batch/v1/namespaces/prod/jobs", itemList(done.Object, running.Object))

	resources, errs := drainCluster(t, fake, ClusterExtractorConfig{Namespace: "prod"}, Options{})
	if len(resources) != 2 {
		t.Fatalf("got %d resources; the finished Job must be kept", len(resources))
	}
	if len(errs) != 1 {
		t.Fatalf("got %d errors/notes; want one note: %v", len(errs), errs)
	}
	var note *NoteError
	if !errors.As(errs[0], &note) {
		t.Fatalf("not a NoteError: %T", errs[0])
	}
	want := "Job migrate has already finished (succeeded); it is kept in the chart and will run again on install. Consider a Helm hook or excluding it"
	if note.Msg != want {
		t.Errorf("note = %q", note.Msg)
	}
}
