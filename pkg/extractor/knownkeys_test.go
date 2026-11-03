package extractor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

const secretSentinel = "S3NTINEL-value-that-must-not-leak"

// fakeConfigMapAPI serves ConfigMaps and records every request. Secrets are
// served too, with a value that fails the tests when it shows up: dhg must
// not even ask for them.
type fakeConfigMapAPI struct {
	srv      *httptest.Server
	mu       sync.Mutex
	requests []string
}

func newFakeConfigMapAPI(t *testing.T, configMaps map[string]map[string]interface{}) *fakeConfigMapAPI {
	t.Helper()
	f := &fakeConfigMapAPI{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/secrets") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]string{"TOKEN": secretSentinel}})
			return
		}
		const prefix = "/api/v1/namespaces/"
		rest := strings.TrimPrefix(r.URL.Path, prefix)
		parts := strings.Split(rest, "/")
		if r.Method != http.MethodGet || len(parts) != 3 || parts[1] != "configmaps" {
			http.NotFound(w, r)
			return
		}
		if parts[0] == "forbidden" {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		cm, ok := configMaps[parts[0]+"/"+parts[2]]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(cm)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeConfigMapAPI) client() *clusterClient {
	return newClusterClientFromHTTP(f.srv.Client(), f.srv.URL)
}

func (f *fakeConfigMapAPI) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := append([]string(nil), f.requests...)
	sort.Strings(out)
	return out
}

func platformConfigMaps() map[string]map[string]interface{} {
	return map[string]map[string]interface{}{
		"team/acme-dev-system-config": {
			"kind":       "ConfigMap",
			"data":       map[string]string{"DB_URL": secretSentinel, "DB_LOGIN": "x", "KAFKA_HOST": "kafka", "KAFKA_PORT": "9092"},
			"binaryData": map[string]string{"CERT": "AAAA"},
		},
	}
}

func TestConfigMapKeysTakesOnlyKeyNames(t *testing.T) {
	api := newFakeConfigMapAPI(t, platformConfigMaps())
	keys, notes, err := configMapKeys(context.Background(), api.client(), "team", []string{"acme-dev-system-config", "gone", "acme-dev-system-config"})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(keys["acme-dev-system-config"], ",")
	if got != "CERT,DB_LOGIN,DB_URL,KAFKA_HOST,KAFKA_PORT" {
		t.Errorf("keys = %s", got)
	}
	if _, ok := keys["gone"]; ok {
		t.Error("a missing ConfigMap has no keys")
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "ConfigMap gone is not found in namespace team") {
		t.Errorf("notes = %v", notes)
	}
	want := []string{"GET /api/v1/namespaces/team/configmaps/acme-dev-system-config", "GET /api/v1/namespaces/team/configmaps/gone"}
	if strings.Join(api.seen(), "|") != strings.Join(want, "|") {
		t.Errorf("requests = %v, want one GET per ConfigMap and nothing else", api.seen())
	}
}

func TestConfigMapKeysForbiddenIsANoteServerErrorFails(t *testing.T) {
	api := newFakeConfigMapAPI(t, nil)
	_, notes, err := configMapKeys(context.Background(), api.client(), "forbidden", []string{"cm"})
	if err != nil || len(notes) != 1 || !strings.Contains(notes[0], "not allowed to read ConfigMap cm") {
		t.Errorf("notes = %v err = %v", notes, err)
	}
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) }))
	defer broken.Close()
	if _, _, err := configMapKeys(context.Background(), newClusterClientFromHTTP(broken.Client(), broken.URL), "team", []string{"cm"}); err == nil {
		t.Error("a server error must fail")
	}
}

const knownKeysProfile = `
vars: {prefix: acme-dev-}
envFrom:
  - secretRef: "{{ .Vars.prefix }}system-secret"
  - configMapRef: "{{ .Vars.prefix }}system-config"
`

func extractKnown(t *testing.T, opts Options, kube *clusterClient) (*SourceExtractor, string, error) {
	t.Helper()
	s := NewSourceExtractor()
	s.kube = kube
	rs, err := drain(s.Extract(context.Background(), opts))
	var dump strings.Builder
	for _, r := range rs {
		b, _ := json.Marshal(r.Object.Object)
		dump.Write(b)
	}
	return s, dump.String(), err
}

// The placeholders of the project that the profile's ConfigMap provides are
// not emitted; the Secret of the profile is never read.
func TestSourceExtractorKnownKeysFromCluster(t *testing.T) {
	api := newFakeConfigMapAPI(t, platformConfigMaps())
	opts := Options{
		Paths: []string{ordersFixture}, Namespace: "team", KnownKeysFromCluster: true,
		SynthProfile: writeProfile(t, knownKeysProfile),
	}
	s, dump, err := extractKnown(t, opts, api.client())
	if err != nil {
		t.Fatal(err)
	}
	for _, covered := range []string{"DB_URL", "DB_LOGIN", "KAFKA_HOST", "KAFKA_PORT"} {
		if strings.Contains(dump, `"`+covered+`"`) {
			t.Errorf("%s comes from the cluster ConfigMap but is in the chart", covered)
		}
	}
	for _, still := range []string{"KEYCLOAK_URL", "TOKEN_URL"} {
		if !strings.Contains(dump, `"`+still+`"`) {
			t.Errorf("%s is not known and must stay a variable", still)
		}
	}
	if strings.Contains(dump, secretSentinel) || strings.Contains(strings.Join(s.Notes(), "\n"), secretSentinel) {
		t.Error("a value from the cluster reached the output")
	}
	notes := strings.Join(s.Notes(), "\n")
	if !strings.Contains(notes, "4 variables come from acme-dev-system-config (DB_LOGIN, DB_URL, KAFKA_HOST, KAFKA_PORT)") {
		t.Errorf("notes:\n%s", notes)
	}
	if want := []string{"GET /api/v1/namespaces/team/configmaps/acme-dev-system-config"}; strings.Join(api.seen(), "|") != want[0] {
		t.Errorf("requests = %v: only the ConfigMap of envFrom may be read, never a Secret", api.seen())
	}
	if !strings.Contains(strings.Join(s.Inputs(), "\n"), "acme-dev-system-config") {
		t.Errorf("inputs = %v", s.Inputs())
	}
}

func TestSourceExtractorKnownKeysFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "known.txt")
	if err := os.WriteFile(file, []byte("# provided by the platform\nDB_URL\nvault:DB_PASSWORD\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := Options{Paths: []string{ordersFixture}, KnownKeysFile: file}
	s := NewSourceExtractor()
	if err := s.Validate(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	_, dump, err := extractKnown(t, opts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(dump, `"DB_URL"`) || strings.Contains(dump, `"DB_PASSWORD"`) || !strings.Contains(dump, `"DB_LOGIN"`) {
		t.Errorf("dump = %s", dump)
	}
}

func TestSourceExtractorKnownKeysFileAndClusterMerge(t *testing.T) {
	api := newFakeConfigMapAPI(t, platformConfigMaps())
	file := filepath.Join(t.TempDir(), "known.txt")
	if err := os.WriteFile(file, []byte("vault:DB_PASSWORD\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := Options{Paths: []string{ordersFixture}, Namespace: "team", KnownKeysFile: file, KnownKeysFromCluster: true, SynthProfile: writeProfile(t, knownKeysProfile)}
	s, _, err := extractKnown(t, opts, api.client())
	if err != nil {
		t.Fatal(err)
	}
	notes := strings.Join(s.Notes(), "\n")
	for _, want := range []string{"1 variable comes from vault (DB_PASSWORD)", "variables come from acme-dev-system-config"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes lack %q:\n%s", want, notes)
		}
	}
}

func TestKnownKeysValidation(t *testing.T) {
	ctx := context.Background()
	s := NewSourceExtractor()
	if err := s.Validate(ctx, Options{Paths: []string{ordersFixture}, KnownKeysFromCluster: true}); err == nil || !strings.Contains(err.Error(), "--synth-profile") {
		t.Errorf("--known-keys-from-cluster without a profile: %v", err)
	}
	if err := s.Validate(ctx, Options{Paths: []string{ordersFixture}, KnownKeysFile: "/nonexistent/known.txt"}); err == nil {
		t.Error("a missing known keys file must fail")
	}
	bad := filepath.Join(t.TempDir(), "bad.txt")
	if err := os.WriteFile(bad, []byte("two words\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(ctx, Options{Paths: []string{ordersFixture}, KnownKeysFile: bad}); err == nil {
		t.Error("a malformed known keys file must fail")
	}
}

// A profile without a ConfigMap in envFrom has nothing to read: a note, and
// the cluster is not asked.
func TestKnownKeysFromClusterWithoutConfigMapRefs(t *testing.T) {
	api := newFakeConfigMapAPI(t, nil)
	opts := Options{Paths: []string{ordersFixture}, KnownKeysFromCluster: true, SynthProfile: writeProfile(t, "envFrom:\n  - secretRef: only-a-secret\n")}
	s, _, err := extractKnown(t, opts, api.client())
	if err != nil {
		t.Fatal(err)
	}
	if len(api.seen()) != 0 {
		t.Errorf("requests = %v", api.seen())
	}
	if !strings.Contains(strings.Join(s.Notes(), "\n"), "--known-keys-from-cluster") {
		t.Errorf("notes = %v", s.Notes())
	}
}

func TestKnownKeysNamespaceFallsBackToKubeconfigThenDefault(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "none"))
	if ns := knownKeysNamespace(Options{Namespace: "team"}); ns != "team" {
		t.Errorf("namespace = %q", ns)
	}
	if ns := knownKeysNamespace(Options{}); ns != "default" {
		t.Errorf("namespace = %q, want default", ns)
	}
	kc := filepath.Join(t.TempDir(), "kubeconfig")
	cfg := `apiVersion: v1
kind: Config
current-context: a
clusters: [{name: c, cluster: {server: "https://x"}}]
users: [{name: u, user: {token: t}}]
contexts:
  - {name: a, context: {cluster: c, user: u, namespace: from-a}}
  - {name: b, context: {cluster: c, user: u, namespace: from-b}}
`
	if err := os.WriteFile(kc, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if ns := knownKeysNamespace(Options{KubeConfig: kc}); ns != "from-a" {
		t.Errorf("namespace = %q, want the current context's", ns)
	}
	if ns := knownKeysNamespace(Options{KubeConfig: kc, KubeContext: "b"}); ns != "from-b" {
		t.Errorf("namespace = %q, want context b's", ns)
	}
}

// Only ConfigMaps are read, and only by a real name: a path in the name,
// an answer of another kind or a redirect must not end at a Secret.
func TestConfigMapKeysReadsOnlyConfigMaps(t *testing.T) {
	cms := platformConfigMaps()
	cms["team/looks-like-secret"] = map[string]interface{}{
		"kind": "Secret",
		"data": map[string]string{"TOKEN": secretSentinel},
	}
	api := newFakeConfigMapAPI(t, cms)
	keys, notes, err := configMapKeys(context.Background(), api.client(), "team",
		[]string{"common/../../secrets/db-secret", "..", "Upper_Case", "looks-like-secret", "acme-dev-system-config"})
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || len(keys["acme-dev-system-config"]) == 0 {
		t.Errorf("keys = %v, want the one real ConfigMap", keys)
	}
	want := []string{
		"GET /api/v1/namespaces/team/configmaps/acme-dev-system-config",
		"GET /api/v1/namespaces/team/configmaps/looks-like-secret",
	}
	if strings.Join(api.seen(), "|") != strings.Join(want, "|") {
		t.Errorf("requests = %v, want %v", api.seen(), want)
	}
	text := strings.Join(notes, "\n")
	for _, w := range []string{"common/../../secrets/db-secret", "Upper_Case", "is not a valid ConfigMap name", `has kind "Secret", not ConfigMap`} {
		if !strings.Contains(text, w) {
			t.Errorf("notes lack %q:\n%s", w, text)
		}
	}
	if strings.Contains(text, secretSentinel) {
		t.Error("a value reached the notes")
	}
}

func TestConfigMapKeysDoesNotFollowRedirects(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if strings.Contains(r.URL.Path, "/secrets/") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"kind": "ConfigMap", "data": map[string]string{"TOKEN": secretSentinel}})
			return
		}
		http.Redirect(w, r, "/api/v1/namespaces/team/secrets/db-secret", http.StatusFound)
	}))
	defer srv.Close()
	_, _, err := configMapKeys(context.Background(), newClusterClientFromHTTP(srv.Client(), srv.URL), "team", []string{"cm"})
	if err == nil {
		t.Error("a redirect must fail the request")
	}
	mu.Lock()
	defer mu.Unlock()
	for _, p := range paths {
		if strings.Contains(p, "secrets") {
			t.Errorf("the redirect was followed: %v", paths)
		}
	}
}
