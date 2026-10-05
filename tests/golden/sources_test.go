package golden

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// apiResource describes one listable resource served by the fake API server.
type apiResource struct {
	group, version, kind, plural string
	namespaced                   bool
}

// fakeCluster serves discovery and list endpoints of the Kubernetes API for a
// fixed set of objects, the way a real API server returns them (list items
// without apiVersion/kind, server-populated metadata and status).
func fakeCluster(t *testing.T, objects []map[string]interface{}) *httptest.Server {
	t.Helper()
	resources := map[string]apiResource{}
	items := map[string][]interface{}{}
	for _, obj := range objects {
		apiVersion, _ := obj["apiVersion"].(string)
		kind, _ := obj["kind"].(string)
		group, version := "", apiVersion
		if i := strings.Index(apiVersion, "/"); i >= 0 {
			group, version = apiVersion[:i], apiVersion[i+1:]
		}
		plural := strings.ToLower(kind) + "s"
		if strings.HasSuffix(kind, "s") {
			plural = strings.ToLower(kind) + "es"
		}
		if strings.HasSuffix(kind, "y") {
			plural = strings.ToLower(strings.TrimSuffix(kind, "y")) + "ies"
		}
		meta, _ := obj["metadata"].(map[string]interface{})
		_, namespaced := meta["namespace"]
		resources[group+"/"+version+"/"+plural] = apiResource{group, version, kind, plural, namespaced || kind != "ClusterRole" && kind != "ClusterRoleBinding" && kind != "Namespace"}
		item := map[string]interface{}{}
		for k, v := range obj {
			if k != "apiVersion" && k != "kind" {
				item[k] = v
			}
		}
		items[group+"/"+version+"/"+plural] = append(items[group+"/"+version+"/"+plural], item)
	}

	resourceList := func(group, version string) map[string]interface{} {
		var list []interface{}
		for _, r := range resources {
			if r.group == group && r.version == version {
				list = append(list, map[string]interface{}{
					"name": r.plural, "kind": r.kind, "namespaced": r.namespaced,
					"verbs": []string{"get", "list", "watch"},
				})
			}
		}
		gv := version
		if group != "" {
			gv = group + "/" + version
		}
		return map[string]interface{}{"kind": "APIResourceList", "groupVersion": gv, "resources": list}
	}

	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case r.URL.Path == "/api/v1":
			write(w, resourceList("", "v1"))
		case r.URL.Path == "/apis":
			groups := map[string]bool{}
			var list []interface{}
			for _, res := range resources {
				if res.group != "" && !groups[res.group] {
					groups[res.group] = true
					gv := res.group + "/" + res.version
					list = append(list, map[string]interface{}{
						"name":             res.group,
						"versions":         []interface{}{map[string]interface{}{"groupVersion": gv, "version": res.version}},
						"preferredVersion": map[string]interface{}{"groupVersion": gv, "version": res.version},
					})
				}
			}
			write(w, map[string]interface{}{"kind": "APIGroupList", "groups": list})
		case parts[0] == "apis" && len(parts) == 3:
			write(w, resourceList(parts[1], parts[2]))
		case parts[0] == "api" && len(parts) == 3:
			write(w, map[string]interface{}{"items": items["/v1/"+parts[2]]})
		case parts[0] == "apis" && len(parts) == 4:
			write(w, map[string]interface{}{"items": items[parts[1]+"/"+parts[2]+"/"+parts[3]]})
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// liveObject makes an input manifest look like it was read from a cluster.
func liveObject(obj map[string]interface{}, namespace string) map[string]interface{} {
	meta, _ := obj["metadata"].(map[string]interface{})
	if meta == nil {
		meta = map[string]interface{}{}
		obj["metadata"] = meta
	}
	if obj["kind"] != "ClusterRole" && obj["kind"] != "ClusterRoleBinding" {
		meta["namespace"] = namespace
	}
	meta["uid"] = "0f1e2d3c"
	meta["resourceVersion"] = "12345"
	meta["creationTimestamp"] = "2026-01-01T00:00:00Z"
	meta["managedFields"] = []interface{}{map[string]interface{}{"manager": "kubectl"}}
	meta["annotations"] = map[string]interface{}{"kubectl.kubernetes.io/last-applied-configuration": "{}"}
	obj["status"] = map[string]interface{}{"observedGeneration": 1}
	if obj["kind"] == "Service" {
		if spec, ok := obj["spec"].(map[string]interface{}); ok {
			spec["clusterIP"] = "10.96.12.34"
			spec["clusterIPs"] = []interface{}{"10.96.12.34"}
		}
	}
	return obj
}

func readManifests(t *testing.T, dir string) []map[string]interface{} {
	t.Helper()
	var out []map[string]interface{}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, doc := range strings.Split(string(data), "\n---") {
			var obj map[string]interface{}
			if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
				t.Fatalf("%s: %v", p, err)
			}
			if obj != nil {
				out = append(out, obj)
			}
		}
	}
	return out
}

// TestClusterSourcePassesHelm generates a chart from a (fake) live cluster and
// checks that server-side noise is gone and the chart installs cleanly.
func TestClusterSourcePassesHelm(t *testing.T) {
	helm := requireHelm(t)
	input := filepath.Join(repoRoot(), "examples", "05-full-stack")
	manifests := readManifests(t, input)

	var objects []map[string]interface{}
	for _, m := range manifests {
		objects = append(objects, liveObject(m, "shop"))
	}
	noise := []string{
		"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: kube-root-ca.crt\n  namespace: shop\ndata:\n  ca.crt: x\n",
		"apiVersion: v1\nkind: ServiceAccount\nmetadata:\n  name: default\n  namespace: shop\n",
		"apiVersion: v1\nkind: Event\nmetadata:\n  name: e1\n  namespace: shop\nreason: Scheduled\n",
		"apiVersion: v1\nkind: Pod\nmetadata:\n  name: api-1\n  namespace: shop\n  ownerReferences:\n  - {apiVersion: apps/v1, kind: ReplicaSet, name: api, uid: u}\nspec:\n  containers:\n  - {name: c, image: x}\n",
		"apiVersion: apps/v1\nkind: ReplicaSet\nmetadata:\n  name: api-5d8\n  namespace: shop\n  ownerReferences:\n  - {apiVersion: apps/v1, kind: Deployment, name: api, uid: u}\nspec:\n  selector: {matchLabels: {a: b}}\n  template: {metadata: {labels: {a: b}}, spec: {containers: [{name: c, image: x}]}}\n",
		"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: coredns\n  namespace: kube-system\ndata:\n  Corefile: x\n",
		"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: deckhouse\n  namespace: d8-system\ndata:\n  x: y\n",
	}
	for _, n := range noise {
		var obj map[string]interface{}
		if err := yaml.Unmarshal([]byte(n), &obj); err != nil {
			t.Fatal(err)
		}
		objects = append(objects, obj)
	}
	srv := fakeCluster(t, objects)

	kubeconfig := filepath.Join(t.TempDir(), "kubeconfig")
	kc := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: fake
  cluster:
    server: %s
contexts:
- name: fake
  context: {cluster: fake, user: fake}
current-context: fake
users:
- name: fake
  user: {token: test}
`, srv.URL)
	if err := os.WriteFile(kubeconfig, []byte(kc), 0o600); err != nil {
		t.Fatal(err)
	}

	out := t.TempDir()
	runOK(t, dhgBin, "generate", "-s", "cluster", "--kubeconfig", kubeconfig, "-o", out, "--chart-name", "app")
	rendered, _ := checkCharts(t, helm, out)
	if rendered != len(manifests) {
		t.Errorf("rendered %d objects, want exactly the %d application objects (cluster noise must be skipped)", rendered, len(manifests))
	}

	chartDir := filepath.Join(out, "app")
	var all strings.Builder
	_ = filepath.Walk(chartDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			data, _ := os.ReadFile(path)
			all.Write(data)
		}
		return nil
	})
	for _, leaked := range []string{"10.96.12.34", "resourceVersion", "managedFields", "last-applied-configuration", "kube-root-ca", "coredns", "0f1e2d3c"} {
		if strings.Contains(all.String(), leaked) {
			t.Errorf("generated chart contains server-side data %q", leaked)
		}
	}
}

// TestGitOpsSourcePassesHelm generates a chart from a Git repository.
func TestGitOpsSourcePassesHelm(t *testing.T) {
	helm := requireHelm(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	src := filepath.Join(repoRoot(), "examples", "05-full-stack")
	if err := os.MkdirAll(filepath.Join(repo, "apps", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	paths, _ := filepath.Glob(filepath.Join(src, "*.yaml"))
	for _, p := range paths {
		data, _ := os.ReadFile(p)
		if err := os.WriteFile(filepath.Join(repo, "apps", "api", filepath.Base(p)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"}, {"add", "."},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, b)
		}
	}

	out := t.TempDir()
	runOK(t, dhgBin, "generate", "-s", "gitops", "--git-repo", "file://"+repo, "--git-branch", "main", "--git-path", "apps/api", "-o", out, "--chart-name", "app")
	rendered, _ := checkCharts(t, helm, out)
	if want := countInputObjects(t, src); rendered < want {
		t.Errorf("rendered %d objects, want at least %d", rendered, want)
	}
}
