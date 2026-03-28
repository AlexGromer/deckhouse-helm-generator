package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── Test helpers ────────────────────────────────────────────────────────────

// fakeKubeAPIServer sets up an httptest server that mimics K8s API endpoints.
type fakeKubeAPIServer struct {
	server    *httptest.Server
	mux       *http.ServeMux
	resources map[string]interface{} // path -> JSON response
}

func newFakeKubeAPIServer() *fakeKubeAPIServer {
	f := &fakeKubeAPIServer{
		mux:       http.NewServeMux(),
		resources: make(map[string]interface{}),
	}
	f.server = httptest.NewServer(f.mux)
	f.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Strip query params for lookup.
		path := r.URL.Path
		resp, ok := f.resources[path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	return f
}

func (f *fakeKubeAPIServer) close() {
	f.server.Close()
}

func (f *fakeKubeAPIServer) setResponse(path string, response interface{}) {
	f.resources[path] = response
}

func (f *fakeKubeAPIServer) client() *clusterClient {
	return newClusterClientFromHTTP(f.server.Client(), f.server.URL)
}

// writeTestKubeconfig writes a minimal kubeconfig YAML to a temp file.
func writeTestKubeconfig(t *testing.T, serverURL string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	content := fmt.Sprintf(`apiVersion: v1
kind: Config
current-context: test
clusters:
- name: test-cluster
  cluster:
    server: %s
    insecure-skip-tls-verify: true
contexts:
- name: test
  context:
    cluster: test-cluster
    user: test-user
users:
- name: test-user
  user:
    token: test-token
`, serverURL)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// coreResourceList returns a standard /api/v1 resource list response.
func coreResourceList(resources ...k8sResourceEntry) map[string]interface{} {
	entries := make([]map[string]interface{}, len(resources))
	for i, r := range resources {
		entries[i] = map[string]interface{}{
			"name":       r.Name,
			"kind":       r.Kind,
			"namespaced": r.Namespaced,
			"verbs":      r.Verbs,
		}
	}
	return map[string]interface{}{
		"kind":       "APIResourceList",
		"apiVersion": "v1",
		"resources":  entries,
	}
}

func emptyGroupList() map[string]interface{} {
	return map[string]interface{}{
		"kind":       "APIGroupList",
		"apiVersion": "v1",
		"groups":     []interface{}{},
	}
}

func itemList(items ...map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"kind":       "List",
		"apiVersion": "v1",
		"metadata":   map[string]interface{}{},
		"items":      items,
	}
}

func itemListWithContinue(token string, items ...map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"kind":       "List",
		"apiVersion": "v1",
		"metadata":   map[string]interface{}{"continue": token},
		"items":      items,
	}
}

func configMapItem(name, namespace string) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"data": map[string]interface{}{"key": "value"},
	}
}

func secretItem(name, namespace string, data map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"type": "Opaque",
		"data": data,
	}
}

// ── 4.1.1: K8s REST client tests ───────────────────────────────────────────

func TestClusterClient_DiscoverResources(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.setResponse("/api/v1", coreResourceList(
		k8sResourceEntry{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: []string{"get", "list"}},
		k8sResourceEntry{Name: "pods", Kind: "Pod", Namespaced: true, Verbs: []string{"get", "list"}},
		k8sResourceEntry{Name: "pods/log", Kind: "Pod", Namespaced: true, Verbs: []string{"get"}}, // subresource
		k8sResourceEntry{Name: "namespaces", Kind: "Namespace", Namespaced: false, Verbs: []string{"get", "list"}},
	))
	fake.setResponse("/apis", emptyGroupList())

	client := fake.client()
	resources, err := client.discoverResources(context.Background())
	if err != nil {
		t.Fatalf("discoverResources() error: %v", err)
	}

	// Should have 3 resources (pods/log is subresource, skipped).
	if len(resources) != 3 {
		t.Fatalf("got %d resources; want 3", len(resources))
	}

	kinds := make(map[string]bool)
	for _, r := range resources {
		kinds[r.Kind] = true
	}
	for _, want := range []string{"ConfigMap", "Pod", "Namespace"} {
		if !kinds[want] {
			t.Errorf("missing resource kind %q", want)
		}
	}
}

func TestClusterClient_DiscoverResources_SkipsNonListable(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.setResponse("/api/v1", coreResourceList(
		k8sResourceEntry{Name: "events", Kind: "Event", Namespaced: true, Verbs: []string{"get"}}, // no list
		k8sResourceEntry{Name: "pods", Kind: "Pod", Namespaced: true, Verbs: []string{"get", "list"}},
	))
	fake.setResponse("/apis", emptyGroupList())

	client := fake.client()
	resources, err := client.discoverResources(context.Background())
	if err != nil {
		t.Fatalf("discoverResources() error: %v", err)
	}

	if len(resources) != 1 {
		t.Fatalf("got %d resources; want 1 (events non-listable)", len(resources))
	}
	if resources[0].Kind != "Pod" {
		t.Errorf("Kind = %q; want Pod", resources[0].Kind)
	}
}

func TestClusterClient_DiscoverGroupResources(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.setResponse("/api/v1", coreResourceList())
	fake.setResponse("/apis", map[string]interface{}{
		"kind":       "APIGroupList",
		"apiVersion": "v1",
		"groups": []interface{}{
			map[string]interface{}{
				"name": "apps",
				"versions": []interface{}{
					map[string]interface{}{"groupVersion": "apps/v1", "version": "v1"},
				},
				"preferredVersion": map[string]interface{}{"groupVersion": "apps/v1", "version": "v1"},
			},
		},
	})
	fake.setResponse("/apis/apps/v1", coreResourceList(
		k8sResourceEntry{Name: "deployments", Kind: "Deployment", Namespaced: true, Verbs: []string{"get", "list"}},
	))

	client := fake.client()
	resources, err := client.discoverResources(context.Background())
	if err != nil {
		t.Fatalf("discoverResources() error: %v", err)
	}

	if len(resources) != 1 {
		t.Fatalf("got %d resources; want 1", len(resources))
	}
	if resources[0].Kind != "Deployment" || resources[0].Group != "apps" {
		t.Errorf("resource = %+v; want apps/Deployment", resources[0])
	}
}

func TestClusterClient_ListResources_Basic(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.setResponse("/api/v1/configmaps", itemList(
		configMapItem("cm1", "default"),
		configMapItem("cm2", "default"),
	))

	client := fake.client()
	ar := apiResource{Group: "", Version: "v1", Kind: "ConfigMap", Name: "configmaps", Namespaced: true}

	var items []string
	err := client.listResources(context.Background(), ar, "", "", DefaultPaginationLimit, func(obj *unstructured.Unstructured) {
		items = append(items, obj.GetName())
	})
	if err != nil {
		t.Fatalf("listResources() error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items; want 2", len(items))
	}
}

func TestClusterClient_ListResources_Namespaced(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.setResponse("/api/v1/namespaces/prod/configmaps", itemList(
		configMapItem("cm-prod", "prod"),
	))

	client := fake.client()
	ar := apiResource{Group: "", Version: "v1", Kind: "ConfigMap", Name: "configmaps", Namespaced: true}

	var items []string
	err := client.listResources(context.Background(), ar, "prod", "", DefaultPaginationLimit, func(obj *unstructured.Unstructured) {
		items = append(items, obj.GetName())
	})
	if err != nil {
		t.Fatalf("listResources() error: %v", err)
	}
	if len(items) != 1 || items[0] != "cm-prod" {
		t.Errorf("items = %v; want [cm-prod]", items)
	}
}

func TestClusterClient_ListResources_GroupAPI(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.setResponse("/apis/apps/v1/deployments", itemList(
		map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "web", "namespace": "default"},
		},
	))

	client := fake.client()
	ar := apiResource{Group: "apps", Version: "v1", Kind: "Deployment", Name: "deployments", Namespaced: true}

	var items []string
	err := client.listResources(context.Background(), ar, "", "", DefaultPaginationLimit, func(obj *unstructured.Unstructured) {
		items = append(items, obj.GetName())
	})
	if err != nil {
		t.Fatalf("listResources() error: %v", err)
	}
	if len(items) != 1 || items[0] != "web" {
		t.Errorf("items = %v; want [web]", items)
	}
}

func TestClusterClient_ListResources_SetsGVK(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	// Items without apiVersion/kind (as K8s API sometimes returns).
	fake.setResponse("/api/v1/configmaps", itemList(
		map[string]interface{}{
			"metadata": map[string]interface{}{"name": "bare", "namespace": "default"},
			"data":     map[string]interface{}{},
		},
	))

	client := fake.client()
	ar := apiResource{Group: "", Version: "v1", Kind: "ConfigMap", Name: "configmaps", Namespaced: true}

	err := client.listResources(context.Background(), ar, "", "", DefaultPaginationLimit, func(obj *unstructured.Unstructured) {
		if obj.GetAPIVersion() != "v1" {
			t.Errorf("apiVersion = %q; want v1", obj.GetAPIVersion())
		}
		if obj.GetKind() != "ConfigMap" {
			t.Errorf("kind = %q; want ConfigMap", obj.GetKind())
		}
	})
	if err != nil {
		t.Fatalf("listResources() error: %v", err)
	}
}

// ── 4.1.2: Authentication tests ────────────────────────────────────────────

func TestKubeconfigParsing(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	path := writeTestKubeconfig(t, fake.server.URL)

	kc, err := parseKubeconfig(path)
	if err != nil {
		t.Fatalf("parseKubeconfig() error: %v", err)
	}

	if kc.CurrentContext != "test" {
		t.Errorf("CurrentContext = %q; want test", kc.CurrentContext)
	}
	if len(kc.Clusters) != 1 {
		t.Fatalf("Clusters count = %d; want 1", len(kc.Clusters))
	}
	if kc.Clusters[0].Cluster.Server != fake.server.URL {
		t.Errorf("Server = %q; want %q", kc.Clusters[0].Cluster.Server, fake.server.URL)
	}
}

func TestKubeconfigResolveContext_Default(t *testing.T) {
	kc := &kubeconfigFile{
		CurrentContext: "default-ctx",
		Contexts: []kubeconfigContext{
			{Name: "default-ctx", Context: kubeconfigContextDetail{Cluster: "c1", User: "u1"}},
		},
	}
	ctx := kc.resolveContext("")
	if ctx == nil {
		t.Fatal("resolveContext('') returned nil")
	}
	if ctx.Name != "default-ctx" {
		t.Errorf("context name = %q; want default-ctx", ctx.Name)
	}
}

func TestKubeconfigResolveContext_Named(t *testing.T) {
	kc := &kubeconfigFile{
		CurrentContext: "default-ctx",
		Contexts: []kubeconfigContext{
			{Name: "default-ctx", Context: kubeconfigContextDetail{Cluster: "c1", User: "u1"}},
			{Name: "prod", Context: kubeconfigContextDetail{Cluster: "c2", User: "u2"}},
		},
	}
	ctx := kc.resolveContext("prod")
	if ctx == nil {
		t.Fatal("resolveContext('prod') returned nil")
	}
	if ctx.Context.Cluster != "c2" {
		t.Errorf("cluster = %q; want c2", ctx.Context.Cluster)
	}
}

func TestKubeconfigResolveContext_NotFound(t *testing.T) {
	kc := &kubeconfigFile{
		CurrentContext: "default-ctx",
		Contexts:       []kubeconfigContext{},
	}
	ctx := kc.resolveContext("nonexistent")
	if ctx != nil {
		t.Error("expected nil for nonexistent context")
	}
}

func TestNewClusterClient_BearerToken(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	// Verify that the token is sent in requests.
	var gotAuth string
	fake.mux.HandleFunc("/check-auth", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	kubeconfigPath := writeTestKubeconfig(t, fake.server.URL)
	client, err := newClusterClient(kubeconfigPath, "test")
	if err != nil {
		t.Fatalf("newClusterClient() error: %v", err)
	}

	_, err = client.doGet(context.Background(), "/check-auth")
	if err != nil {
		t.Fatalf("doGet() error: %v", err)
	}

	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q; want 'Bearer test-token'", gotAuth)
	}
}

func TestNewClusterClient_InvalidKubeconfig(t *testing.T) {
	_, err := newClusterClient("/nonexistent/kubeconfig", "")
	if err == nil {
		t.Error("expected error for nonexistent kubeconfig")
	}
}

func TestNewClusterClient_MissingContext(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	path := writeTestKubeconfig(t, fake.server.URL)
	_, err := newClusterClient(path, "nonexistent-context")
	if err == nil {
		t.Error("expected error for nonexistent context")
	}
}

func TestLoadKubeconfig_ExplicitPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(path, []byte("apiVersion: v1\nkind: Config\n"), 0600); err != nil {
		t.Fatal(err)
	}

	result, err := LoadKubeconfig(path, "my-ctx")
	if err != nil {
		t.Fatalf("LoadKubeconfig() error: %v", err)
	}
	if result.Path != path {
		t.Errorf("Path = %q; want %q", result.Path, path)
	}
	if result.Context != "my-ctx" {
		t.Errorf("Context = %q; want my-ctx", result.Context)
	}
}

func TestLoadKubeconfig_NonexistentExplicit(t *testing.T) {
	_, err := LoadKubeconfig("/nonexistent/kubeconfig", "")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

// ── 4.1.3: Resource extraction integration test ────────────────────────────

func TestClusterExtractor_Extract_WithMockServer(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.setResponse("/api/v1", coreResourceList(
		k8sResourceEntry{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: []string{"get", "list"}},
	))
	fake.setResponse("/apis", emptyGroupList())
	fake.setResponse("/api/v1/configmaps", itemList(
		configMapItem("app-config", "default"),
		configMapItem("db-config", "prod"),
	))

	ce := NewClusterExtractorWithConfig(ClusterExtractorConfig{
		SecretStrategy: "mask",
	})
	ce.SetClient(fake.client())

	resCh, errCh := ce.Extract(context.Background(), Options{})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	var errors []error
	for e := range errCh {
		errors = append(errors, e)
	}

	if len(errors) > 0 {
		t.Fatalf("unexpected errors: %v", errors)
	}
	if len(resources) != 2 {
		t.Fatalf("got %d resources; want 2", len(resources))
	}

	names := make(map[string]bool)
	for _, r := range resources {
		names[r.Object.GetName()] = true
		if r.Source != types.SourceCluster {
			t.Errorf("Source = %q; want cluster", r.Source)
		}
		if r.GVK.Kind != "ConfigMap" {
			t.Errorf("Kind = %q; want ConfigMap", r.GVK.Kind)
		}
	}
	if !names["app-config"] || !names["db-config"] {
		t.Errorf("names = %v; want app-config and db-config", names)
	}
}

func TestClusterExtractor_Validate_WithMockServer(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.setResponse("/api/v1", coreResourceList(
		k8sResourceEntry{Name: "pods", Kind: "Pod", Namespaced: true, Verbs: []string{"get", "list"}},
	))

	ce := NewClusterExtractorWithConfig(ClusterExtractorConfig{})
	ce.SetClient(fake.client())

	err := ce.Validate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Validate() error: %v", err)
	}
}

func TestClusterExtractor_Validate_ServerDown(t *testing.T) {
	// Create a server and close it immediately.
	fake := newFakeKubeAPIServer()
	client := fake.client()
	fake.close()

	ce := NewClusterExtractorWithConfig(ClusterExtractorConfig{})
	ce.SetClient(client)

	err := ce.Validate(context.Background(), Options{})
	if err == nil {
		t.Error("expected error when server is down")
	}
}

// ── 4.1.4: Filtering tests ────────────────────────────────────────────────

func TestClusterExtractor_Extract_NamespaceFilter(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.setResponse("/api/v1", coreResourceList(
		k8sResourceEntry{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: []string{"get", "list"}},
	))
	fake.setResponse("/apis", emptyGroupList())
	// When namespace is set, the URL should target that namespace.
	fake.setResponse("/api/v1/namespaces/prod/configmaps", itemList(
		configMapItem("cm-prod", "prod"),
	))

	ce := NewClusterExtractorWithConfig(ClusterExtractorConfig{
		Namespace: "prod",
	})
	ce.SetClient(fake.client())

	resCh, errCh := ce.Extract(context.Background(), Options{})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 1 {
		t.Fatalf("got %d resources; want 1", len(resources))
	}
	if resources[0].Object.GetName() != "cm-prod" {
		t.Errorf("Name = %q; want cm-prod", resources[0].Object.GetName())
	}
}

func TestClusterExtractor_Extract_ExcludeNamespaces(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.setResponse("/api/v1", coreResourceList(
		k8sResourceEntry{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: []string{"get", "list"}},
	))
	fake.setResponse("/apis", emptyGroupList())
	fake.setResponse("/api/v1/configmaps", itemList(
		configMapItem("cm-default", "default"),
		configMapItem("cm-kube-system", "kube-system"),
		configMapItem("cm-kube-public", "kube-public"),
		configMapItem("cm-prod", "prod"),
	))

	ce := NewClusterExtractorWithConfig(ClusterExtractorConfig{
		ExcludeNamespaces: []string{"kube-system", "kube-public"},
	})
	ce.SetClient(fake.client())

	resCh, errCh := ce.Extract(context.Background(), Options{})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 2 {
		t.Fatalf("got %d resources; want 2 (kube-system and kube-public excluded)", len(resources))
	}

	for _, r := range resources {
		ns := r.Object.GetNamespace()
		if ns == "kube-system" || ns == "kube-public" {
			t.Errorf("should have excluded namespace %q", ns)
		}
	}
}

func TestClusterExtractor_Extract_LabelSelector(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.setResponse("/api/v1", coreResourceList(
		k8sResourceEntry{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: []string{"get", "list"}},
	))
	fake.setResponse("/apis", emptyGroupList())

	// The fake server doesn't actually filter by selector, but we verify the URL contains it.
	var gotPath string
	fake.mux.HandleFunc("/api/v1/configmaps", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(itemList(configMapItem("web", "default")))
	})

	ce := NewClusterExtractorWithConfig(ClusterExtractorConfig{
		Selector: "app=web",
	})
	ce.SetClient(fake.client())

	resCh, errCh := ce.Extract(context.Background(), Options{})
	for range resCh {
	}
	for range errCh {
	}

	if !strings.Contains(gotPath, "labelSelector=app%3Dweb") && !strings.Contains(gotPath, "labelSelector=app=web") {
		t.Errorf("request path %q does not contain label selector", gotPath)
	}
}

func TestClusterExtractor_Extract_OptsLabelSelector(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.setResponse("/api/v1", coreResourceList(
		k8sResourceEntry{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: []string{"get", "list"}},
	))
	fake.setResponse("/apis", emptyGroupList())

	var gotPath string
	fake.mux.HandleFunc("/api/v1/configmaps", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(itemList())
	})

	ce := NewClusterExtractorWithConfig(ClusterExtractorConfig{})
	ce.SetClient(fake.client())

	resCh, errCh := ce.Extract(context.Background(), Options{LabelSelector: "env=staging"})
	for range resCh {
	}
	for range errCh {
	}

	if !strings.Contains(gotPath, "labelSelector=env") {
		t.Errorf("request path %q does not contain label selector from opts", gotPath)
	}
}

// ── 4.1.5: Secret handling tests ───────────────────────────────────────────

func TestClusterExtractor_Extract_SecretsExcludedByDefault(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.setResponse("/api/v1", coreResourceList(
		k8sResourceEntry{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: []string{"get", "list"}},
		k8sResourceEntry{Name: "secrets", Kind: "Secret", Namespaced: true, Verbs: []string{"get", "list"}},
	))
	fake.setResponse("/apis", emptyGroupList())
	fake.setResponse("/api/v1/configmaps", itemList(configMapItem("cfg", "default")))
	fake.setResponse("/api/v1/secrets", itemList(
		secretItem("db-creds", "default", map[string]interface{}{"password": "c2VjcmV0"}),
	))

	ce := NewClusterExtractorWithConfig(ClusterExtractorConfig{
		IncludeSecrets: false, // default
	})
	ce.SetClient(fake.client())

	resCh, errCh := ce.Extract(context.Background(), Options{})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 1 {
		t.Fatalf("got %d resources; want 1 (secrets excluded)", len(resources))
	}
	if resources[0].Object.GetKind() != "ConfigMap" {
		t.Errorf("Kind = %q; want ConfigMap", resources[0].Object.GetKind())
	}
}

func TestClusterExtractor_Extract_SecretsIncluded_MaskStrategy(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.setResponse("/api/v1", coreResourceList(
		k8sResourceEntry{Name: "secrets", Kind: "Secret", Namespaced: true, Verbs: []string{"get", "list"}},
	))
	fake.setResponse("/apis", emptyGroupList())
	fake.setResponse("/api/v1/secrets", itemList(
		secretItem("db-creds", "default", map[string]interface{}{
			"username": "dXNlcg==",
			"password": "cGFzcw==",
		}),
	))

	ce := NewClusterExtractorWithConfig(ClusterExtractorConfig{
		IncludeSecrets: true,
		SecretStrategy: "mask",
	})
	ce.SetClient(fake.client())

	resCh, errCh := ce.Extract(context.Background(), Options{})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 1 {
		t.Fatalf("got %d resources; want 1", len(resources))
	}

	data, _, _ := unstructuredNestedMap(resources[0].Object.Object, "data")
	for k, v := range data {
		if v != "REDACTED" {
			t.Errorf("data[%q] = %q; want REDACTED", k, v)
		}
	}
}

func TestClusterExtractor_Extract_SecretsIncluded_IncludeStrategy(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.setResponse("/api/v1", coreResourceList(
		k8sResourceEntry{Name: "secrets", Kind: "Secret", Namespaced: true, Verbs: []string{"get", "list"}},
	))
	fake.setResponse("/apis", emptyGroupList())
	fake.setResponse("/api/v1/secrets", itemList(
		secretItem("db-creds", "default", map[string]interface{}{
			"password": "cGFzcw==",
		}),
	))

	ce := NewClusterExtractorWithConfig(ClusterExtractorConfig{
		IncludeSecrets: true,
		SecretStrategy: "include",
	})
	ce.SetClient(fake.client())

	resCh, errCh := ce.Extract(context.Background(), Options{})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 1 {
		t.Fatalf("got %d resources; want 1", len(resources))
	}

	data, _, _ := unstructuredNestedMap(resources[0].Object.Object, "data")
	if data["password"] != "cGFzcw==" {
		t.Errorf("password should be unmasked; got %q", data["password"])
	}
}

func TestClusterExtractor_Extract_SecretsIncluded_ExternalSecretStrategy(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.setResponse("/api/v1", coreResourceList(
		k8sResourceEntry{Name: "secrets", Kind: "Secret", Namespaced: true, Verbs: []string{"get", "list"}},
	))
	fake.setResponse("/apis", emptyGroupList())
	fake.setResponse("/api/v1/secrets", itemList(
		secretItem("creds", "default", map[string]interface{}{
			"api-key": "c2VjcmV0",
		}),
	))

	ce := NewClusterExtractorWithConfig(ClusterExtractorConfig{
		IncludeSecrets: true,
		SecretStrategy: "external-secret",
	})
	ce.SetClient(fake.client())

	resCh, errCh := ce.Extract(context.Background(), Options{})

	var resources []*types.ExtractedResource
	for r := range resCh {
		resources = append(resources, r)
	}
	for range errCh {
	}

	if len(resources) != 1 {
		t.Fatalf("got %d resources; want 1", len(resources))
	}

	data, _, _ := unstructuredNestedMap(resources[0].Object.Object, "data")
	if data["api-key"] != "EXTERNAL_SECRET_REF" {
		t.Errorf("api-key = %q; want EXTERNAL_SECRET_REF", data["api-key"])
	}
}

// ── 4.1.6: Pagination tests ───────────────────────────────────────────────

func TestClusterClient_ListResources_Pagination(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	callCount := 0
	fake.mux.HandleFunc("/api/v1/configmaps", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		cont := r.URL.Query().Get("continue")
		var resp interface{}
		switch cont {
		case "":
			// First page.
			resp = itemListWithContinue("page2",
				configMapItem("cm1", "default"),
				configMapItem("cm2", "default"),
			)
		case "page2":
			// Second page.
			resp = itemListWithContinue("page3",
				configMapItem("cm3", "default"),
			)
		case "page3":
			// Last page — no continue token.
			resp = itemList(
				configMapItem("cm4", "default"),
			)
		default:
			http.Error(w, "unexpected continue token", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	client := fake.client()
	ar := apiResource{Group: "", Version: "v1", Kind: "ConfigMap", Name: "configmaps", Namespaced: true}

	var items []string
	err := client.listResources(context.Background(), ar, "", "", DefaultPaginationLimit, func(obj *unstructured.Unstructured) {
		items = append(items, obj.GetName())
	})
	if err != nil {
		t.Fatalf("listResources() error: %v", err)
	}

	if len(items) != 4 {
		t.Fatalf("got %d items; want 4", len(items))
	}
	if callCount != 3 {
		t.Errorf("API calls = %d; want 3 (pagination)", callCount)
	}

	expected := []string{"cm1", "cm2", "cm3", "cm4"}
	for i, name := range expected {
		if items[i] != name {
			t.Errorf("items[%d] = %q; want %q", i, items[i], name)
		}
	}
}

func TestClusterClient_ListResources_PaginationLimit(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	var gotLimit string
	fake.mux.HandleFunc("/api/v1/pods", func(w http.ResponseWriter, r *http.Request) {
		gotLimit = r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(itemList())
	})

	client := fake.client()
	ar := apiResource{Group: "", Version: "v1", Kind: "Pod", Name: "pods", Namespaced: true}

	_ = client.listResources(context.Background(), ar, "", "", 100, func(obj *unstructured.Unstructured) {})

	if gotLimit != "100" {
		t.Errorf("limit = %q; want 100", gotLimit)
	}
}

// ── Context cancellation ───────────────────────────────────────────────────

func TestClusterExtractor_Extract_CancelledContext(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.setResponse("/api/v1", coreResourceList(
		k8sResourceEntry{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: []string{"get", "list"}},
	))
	fake.setResponse("/apis", emptyGroupList())
	fake.setResponse("/api/v1/configmaps", itemList(configMapItem("cm1", "default")))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	ce := NewClusterExtractorWithConfig(ClusterExtractorConfig{})
	ce.SetClient(fake.client())

	resCh, errCh := ce.Extract(ctx, Options{})
	for range resCh {
	}
	for range errCh {
	}
	// No panic is success.
}

// ── Config validation tests ────────────────────────────────────────────────

func TestClusterExtractorConfig_Validate_InvalidStrategy(t *testing.T) {
	cfg := &ClusterExtractorConfig{SecretStrategy: "bogus"}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for invalid strategy")
	}
}

func TestClusterExtractorConfig_Validate_NegativeLimit(t *testing.T) {
	cfg := &ClusterExtractorConfig{Pagination: PaginationConfig{Limit: -1}}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for negative limit")
	}
}

func TestClusterExtractorConfig_Validate_Valid(t *testing.T) {
	cfg := &ClusterExtractorConfig{
		SecretStrategy: "mask",
		Pagination:     NewPaginationConfig(),
	}
	err := cfg.Validate()
	if err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

// ── FilterConfig tests ─────────────────────────────────────────────────────

func TestFilterConfig_MatchesNamespace(t *testing.T) {
	tests := []struct {
		name   string
		filter *FilterConfig
		ns     string
		want   bool
	}{
		{"nil filter", nil, "any", true},
		{"no filter matches all", &FilterConfig{}, "any", true},
		{"specific ns matches", &FilterConfig{Namespace: "prod"}, "prod", true},
		{"specific ns rejects other", &FilterConfig{Namespace: "prod"}, "dev", false},
		{"cluster-scoped included", &FilterConfig{Namespace: "prod"}, "", true},
		{"exclude matches", &FilterConfig{ExcludeNamespaces: []string{"kube-system"}}, "kube-system", false},
		{"exclude passes other", &FilterConfig{ExcludeNamespaces: []string{"kube-system"}}, "default", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.filter.MatchesNamespace(tc.ns); got != tc.want {
				t.Errorf("MatchesNamespace(%q) = %v; want %v", tc.ns, got, tc.want)
			}
		})
	}
}

func TestFilterConfig_Validate_MutuallyExclusive(t *testing.T) {
	f := &FilterConfig{Namespace: "prod", ExcludeNamespaces: []string{"dev"}}
	if err := f.Validate(); err == nil {
		t.Error("expected error for mutually exclusive namespace/exclude_namespaces")
	}
}

// ── MaskSecretData tests ───────────────────────────────────────────────────

func TestMaskSecretData_Masks(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]interface{}{"name": "s1"},
		"data":       map[string]interface{}{"key1": "val1", "key2": "val2"},
		"stringData": map[string]interface{}{"key3": "val3"},
	}}
	res := &types.ExtractedResource{Object: obj}

	masked := MaskSecretData(res)
	if !masked {
		t.Error("MaskSecretData returned false")
	}

	data, _, _ := unstructuredNestedMap(obj.Object, "data")
	for k, v := range data {
		if v != "REDACTED" {
			t.Errorf("data[%q] = %q; want REDACTED", k, v)
		}
	}
	sd, _, _ := unstructuredNestedMap(obj.Object, "stringData")
	for k, v := range sd {
		if v != "REDACTED" {
			t.Errorf("stringData[%q] = %q; want REDACTED", k, v)
		}
	}
}

func TestMaskSecretData_NotSecret(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]interface{}{"name": "cm1"},
		"data":       map[string]interface{}{"key": "value"},
	}}
	res := &types.ExtractedResource{Object: obj}

	masked := MaskSecretData(res)
	if masked {
		t.Error("MaskSecretData should return false for ConfigMap")
	}

	data, _, _ := unstructuredNestedMap(obj.Object, "data")
	if data["key"] != "value" {
		t.Error("ConfigMap data should not be modified")
	}
}

func TestMaskSecretData_NilResource(t *testing.T) {
	if MaskSecretData(nil) {
		t.Error("should return false for nil")
	}
	if MaskSecretData(&types.ExtractedResource{}) {
		t.Error("should return false for nil Object")
	}
}

// ── URL building tests ─────────────────────────────────────────────────────

func TestBuildListPath(t *testing.T) {
	tests := []struct {
		name string
		ar   apiResource
		ns   string
		want string
	}{
		{"core all ns", apiResource{Group: "", Version: "v1", Name: "pods", Namespaced: true}, "", "/api/v1/pods"},
		{"core specific ns", apiResource{Group: "", Version: "v1", Name: "pods", Namespaced: true}, "default", "/api/v1/namespaces/default/pods"},
		{"core cluster-scoped", apiResource{Group: "", Version: "v1", Name: "namespaces", Namespaced: false}, "", "/api/v1/namespaces"},
		{"group all ns", apiResource{Group: "apps", Version: "v1", Name: "deployments", Namespaced: true}, "", "/apis/apps/v1/deployments"},
		{"group specific ns", apiResource{Group: "apps", Version: "v1", Name: "deployments", Namespaced: true}, "prod", "/apis/apps/v1/namespaces/prod/deployments"},
		{"group cluster-scoped", apiResource{Group: "rbac.authorization.k8s.io", Version: "v1", Name: "clusterroles", Namespaced: false}, "", "/apis/rbac.authorization.k8s.io/v1/clusterroles"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildListPath(tc.ar, tc.ns)
			if got != tc.want {
				t.Errorf("buildListPath() = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestBuildListQuery(t *testing.T) {
	tests := []struct {
		name      string
		selector  string
		cont      string
		limit     int64
		wantParts []string
	}{
		{"basic", "", "", 500, []string{"limit=500"}},
		{"with selector", "app=web", "", 500, []string{"limit=500", "labelSelector=app=web"}},
		{"with continue", "", "abc123", 500, []string{"limit=500", "continue=abc123"}},
		{"all params", "env=prod", "token", 100, []string{"limit=100", "labelSelector=env=prod", "continue=token"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildListQuery(tc.selector, tc.cont, tc.limit)
			for _, part := range tc.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("query %q does not contain %q", got, part)
				}
			}
		})
	}
}

// ── Helper function tests ──────────────────────────────────────────────────

func TestContainsVerb(t *testing.T) {
	verbs := []string{"get", "list", "watch"}
	if !containsVerb(verbs, "list") {
		t.Error("should find 'list'")
	}
	if containsVerb(verbs, "delete") {
		t.Error("should not find 'delete'")
	}
	if containsVerb(nil, "get") {
		t.Error("should return false for nil")
	}
}

func TestTruncateStr(t *testing.T) {
	if truncateStr("short", 10) != "short" {
		t.Error("should not truncate short strings")
	}
	got := truncateStr("a very long string that exceeds limit", 10)
	if got != "a very lon..." {
		t.Errorf("truncateStr = %q; want 'a very lon...'", got)
	}
}

func TestIsValidSecretStrategy(t *testing.T) {
	if !IsValidSecretStrategy("mask") {
		t.Error("mask should be valid")
	}
	if !IsValidSecretStrategy("include") {
		t.Error("include should be valid")
	}
	if !IsValidSecretStrategy("external-secret") {
		t.Error("external-secret should be valid")
	}
	if IsValidSecretStrategy("bogus") {
		t.Error("bogus should be invalid")
	}
}

// ── ClusterExtractor Source / Config ───────────────────────────────────────

func TestClusterExtractor_SourceType(t *testing.T) {
	ce := NewClusterExtractor()
	if ce.Source() != types.SourceCluster {
		t.Errorf("Source() = %q; want cluster", ce.Source())
	}
}

func TestClusterExtractor_Config_Defaults(t *testing.T) {
	ce := NewClusterExtractor()
	cfg := ce.Config()
	if cfg.SecretStrategy != "mask" {
		t.Errorf("default SecretStrategy = %q; want mask", cfg.SecretStrategy)
	}
	if cfg.Pagination.Limit != DefaultPaginationLimit {
		t.Errorf("default Pagination.Limit = %d; want %d", cfg.Pagination.Limit, DefaultPaginationLimit)
	}
}

func TestClusterExtractor_ConfigWithDefaults(t *testing.T) {
	ce := NewClusterExtractorWithConfig(ClusterExtractorConfig{
		Kubeconfig: "/custom/path",
	})
	cfg := ce.Config()
	if cfg.Kubeconfig != "/custom/path" {
		t.Errorf("Kubeconfig = %q; want /custom/path", cfg.Kubeconfig)
	}
	if cfg.SecretStrategy != "mask" {
		t.Errorf("default SecretStrategy = %q; want mask", cfg.SecretStrategy)
	}
}

// ── HTTP error handling ────────────────────────────────────────────────────

func TestClusterClient_DoGet_HTTPError(t *testing.T) {
	fake := newFakeKubeAPIServer()
	defer fake.close()

	fake.mux.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
	})

	client := fake.client()
	_, err := client.doGet(context.Background(), "/error")
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "HTTP 403") {
		t.Errorf("error = %q; want to contain 'HTTP 403'", err.Error())
	}
}
