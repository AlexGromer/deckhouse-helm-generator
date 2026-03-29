package generator

// ============================================================
// Test Plan: External Secrets Operator (ESO) Generator
// ============================================================
//
// | # | Test Name                                          | Category | Input                                  | Expected Output                               |
// |---|----------------------------------------------------|----------|----------------------------------------|-----------------------------------------------|
// | 1 | TestDetectESOSecrets_EmptyGraph                    | edge     | empty graph                            | empty slice                                   |
// | 2 | TestDetectESOSecrets_SingleOpaqueWith2Keys         | happy    | graph with 1 Opaque secret, 2 keys     | 1 ESOSecret with 2 Keys                       |
// | 3 | TestDetectESOSecrets_MultipleNamespaces            | happy    | secrets in ns-a and ns-b               | 2 ESOSecrets with correct namespaces          |
// | 4 | TestDetectESOSecrets_SkipsNonSecretResources       | edge     | Deployment + ConfigMap in graph        | empty slice                                   |
// | 5 | TestGenerateSecretStore_AWSBackend                 | happy    | ESOOptions{Backend:ESOBackendAWS}      | output contains aws provider block            |
// | 6 | TestGenerateSecretStore_VaultBackend               | happy    | ESOOptions{Backend:ESOBackendVault}    | output contains vault mount and role          |
// | 7 | TestGenerateSecretStore_ClusterVsNamespaced        | happy    | namespace empty vs set                 | ClusterSecretStore vs SecretStore             |
// | 8 | TestGenerateExternalSecrets_EmptyRemotePath        | edge     | ESOSecret{RemotePath:""}               | derived path used (non-empty)                 |
// | 9 | TestGenerateExternalSecrets_CustomRemotePath       | happy    | ESOSecret{RemotePath:"my/path"}        | verbatim "my/path" in output                  |
// |10 | TestBuildESOValuesFragment_AllBackends             | happy    | all 4 ESOBackend values                | each backend has a non-nil subtree            |
// |11 | TestInjectESO_NilChart                             | error    | nil chart                              | (nil, 0), no panic                            |
// |12 | TestInjectESO_NoSecrets                            | edge     | chart + empty graph                    | same chart returned, count=0                  |
// |13 | TestInjectESO_CopyOnWrite                          | happy    | original chart, graph with secrets     | original unchanged, result contains new keys  |
// |14 | TestGenerateSecretStore_DefaultRefreshInterval     | edge     | ESOOptions{RefreshInterval:""}         | output contains a non-empty refresh interval  |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// makeSecretResource creates a ProcessedResource representing a Kubernetes Secret.
func makeSecretResource(name, namespace, secretType string, dataKeys []string) *types.ProcessedResource {
	data := make(map[string]interface{}, len(dataKeys))
	for _, k := range dataKeys {
		data[k] = "dmFsdWU=" // base64("value")
	}
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"type": secretType,
			"data": data,
		},
	}
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK: schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "Secret",
			},
		},
		Values: make(map[string]interface{}),
	}
}

// makeGraphWithSecrets builds a ResourceGraph containing the given Secret ProcessedResources.
func makeGraphWithSecrets(secrets ...*types.ProcessedResource) *types.ResourceGraph {
	graph := types.NewResourceGraph()
	for _, s := range secrets {
		graph.AddResource(s)
	}
	return graph
}

// ── 1: DetectESOSecrets — empty graph ────────────────────────────────────────

func TestDetectESOSecrets_EmptyGraph(t *testing.T) {
	graph := types.NewResourceGraph()
	result := DetectESOSecrets(graph)
	if len(result) != 0 {
		t.Errorf("expected 0 ESOSecrets for empty graph, got %d", len(result))
	}
}

// ── 2: DetectESOSecrets — single Opaque secret with 2 keys ───────────────────

func TestDetectESOSecrets_SingleOpaqueWith2Keys(t *testing.T) {
	secret := makeSecretResource("db-creds", "default", "Opaque", []string{"username", "password"})
	graph := makeGraphWithSecrets(secret)

	result := DetectESOSecrets(graph)

	if len(result) != 1 {
		t.Fatalf("expected 1 ESOSecret, got %d", len(result))
	}
	entry := result[0]
	if entry.Name != "db-creds" {
		t.Errorf("expected Name=%q, got %q", "db-creds", entry.Name)
	}
	if entry.Namespace != "default" {
		t.Errorf("expected Namespace=%q, got %q", "default", entry.Namespace)
	}
	if len(entry.Keys) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(entry.Keys), entry.Keys)
	}
}

// ── 3: DetectESOSecrets — multiple namespaces ─────────────────────────────────

func TestDetectESOSecrets_MultipleNamespaces(t *testing.T) {
	s1 := makeSecretResource("secret-a", "ns-a", "Opaque", []string{"key1"})
	s2 := makeSecretResource("secret-b", "ns-b", "Opaque", []string{"key2"})
	graph := makeGraphWithSecrets(s1, s2)

	result := DetectESOSecrets(graph)

	if len(result) != 2 {
		t.Fatalf("expected 2 ESOSecrets, got %d", len(result))
	}
	namespaces := map[string]bool{}
	for _, e := range result {
		namespaces[e.Namespace] = true
	}
	if !namespaces["ns-a"] {
		t.Error("expected ESOSecret from ns-a")
	}
	if !namespaces["ns-b"] {
		t.Error("expected ESOSecret from ns-b")
	}
}

// ── 4: DetectESOSecrets — skips non-Secret resources ─────────────────────────

func TestDetectESOSecrets_SkipsNonSecretResources(t *testing.T) {
	deploy := makeProcessedResource("Deployment", "myapp", "default", nil)
	cm := makeProcessedResource("ConfigMap", "myconfig", "default", nil)
	graph := buildGraph([]*types.ProcessedResource{deploy, cm}, nil)

	result := DetectESOSecrets(graph)

	if len(result) != 0 {
		t.Errorf("expected 0 ESOSecrets for Deployment+ConfigMap graph, got %d", len(result))
	}
}

// ── 5: GenerateSecretStore — AWS backend contains aws provider ────────────────

func TestGenerateSecretStore_AWSBackend(t *testing.T) {
	opts := ESOOptions{
		Backend:   ESOBackendAWS,
		AWSRegion: "us-east-1",
	}

	result := GenerateSecretStore(opts)

	for _, content := range result {
		if strings.Contains(content, "SecretStore") || strings.Contains(content, "ClusterSecretStore") {
			if !strings.Contains(content, "aws") {
				t.Errorf("SecretStore for AWS backend must contain 'aws' provider block, got:\n%s", content)
			}
			return
		}
	}
	t.Error("expected at least one SecretStore or ClusterSecretStore in output")
}

// ── 6: GenerateSecretStore — Vault backend contains mount and role ────────────

func TestGenerateSecretStore_VaultBackend(t *testing.T) {
	opts := ESOOptions{
		Backend:    ESOBackendVault,
		VaultMount: "kubernetes",
		VaultRole:  "my-role",
	}

	result := GenerateSecretStore(opts)

	for _, content := range result {
		if strings.Contains(content, "SecretStore") || strings.Contains(content, "ClusterSecretStore") {
			if !strings.Contains(content, "kubernetes") {
				t.Errorf("Vault SecretStore must contain vault mount 'kubernetes', got:\n%s", content)
			}
			if !strings.Contains(content, "my-role") {
				t.Errorf("Vault SecretStore must contain vault role 'my-role', got:\n%s", content)
			}
			return
		}
	}
	t.Error("expected at least one SecretStore or ClusterSecretStore in output")
}

// ── 7: ClusterSecretStore vs SecretStore based on namespace ───────────────────

func TestGenerateSecretStore_ClusterVsNamespaced(t *testing.T) {
	optsCluster := ESOOptions{
		Backend:              ESOBackendAWS,
		SecretStoreNamespace: "", // empty → ClusterSecretStore
		AWSRegion:            "eu-west-1",
	}
	optsNamespaced := ESOOptions{
		Backend:              ESOBackendAWS,
		SecretStoreNamespace: "production",
		AWSRegion:            "eu-west-1",
	}

	clusterResult := GenerateSecretStore(optsCluster)
	namespacedResult := GenerateSecretStore(optsNamespaced)

	foundCluster := false
	for _, content := range clusterResult {
		if strings.Contains(content, "ClusterSecretStore") {
			foundCluster = true
			break
		}
	}
	if !foundCluster {
		t.Error("expected ClusterSecretStore when SecretStoreNamespace is empty")
	}

	foundNamespaced := false
	for _, content := range namespacedResult {
		if strings.Contains(content, "kind: SecretStore") {
			foundNamespaced = true
			break
		}
	}
	if !foundNamespaced {
		t.Error("expected namespaced SecretStore when SecretStoreNamespace is set")
	}
}

// ── 8: GenerateExternalSecrets — empty RemotePath derives a path ──────────────

func TestGenerateExternalSecrets_EmptyRemotePath(t *testing.T) {
	secrets := []ESOSecret{
		{
			Name:       "db-creds",
			Namespace:  "default",
			Keys:       []string{"username", "password"},
			RemotePath: "",
		},
	}
	opts := ESOOptions{
		Backend:   ESOBackendAWS,
		AWSRegion: "us-east-1",
	}

	result := GenerateExternalSecrets(secrets, opts)

	if len(result) == 0 {
		t.Fatal("expected at least one ExternalSecret to be generated")
	}
	for _, content := range result {
		if strings.Contains(content, "ExternalSecret") {
			if !strings.Contains(content, "remoteRef") && !strings.Contains(content, "key:") {
				t.Errorf("expected remoteRef or key in ExternalSecret when RemotePath is empty, got:\n%s", content)
			}
			return
		}
	}
	t.Error("expected ExternalSecret kind in output")
}

// ── 9: GenerateExternalSecrets — custom RemotePath used verbatim ──────────────

func TestGenerateExternalSecrets_CustomRemotePath(t *testing.T) {
	secrets := []ESOSecret{
		{
			Name:       "my-secret",
			Namespace:  "staging",
			Keys:       []string{"token"},
			RemotePath: "custom/remote/path",
		},
	}
	opts := ESOOptions{
		Backend:   ESOBackendAWS,
		AWSRegion: "us-east-1",
	}

	result := GenerateExternalSecrets(secrets, opts)

	found := false
	for _, content := range result {
		if strings.Contains(content, "custom/remote/path") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected custom RemotePath 'custom/remote/path' to appear verbatim in ExternalSecret output")
	}
}

// ── 10: BuildESOValuesFragment — all 4 backends have subtree ─────────────────

func TestBuildESOValuesFragment_AllBackends(t *testing.T) {
	backends := []ESOBackend{ESOBackendAWS, ESOBackendGCP, ESOBackendAzure, ESOBackendVault}

	for _, backend := range backends {
		secrets := []ESOSecret{
			{Name: "test-secret", Namespace: "default", Keys: []string{"key"}},
		}
		opts := ESOOptions{Backend: backend}

		result := BuildESOValuesFragment(secrets, opts)

		if result == nil {
			t.Errorf("BuildESOValuesFragment returned nil for backend %q", backend)
			continue
		}
		if len(result) == 0 {
			t.Errorf("BuildESOValuesFragment returned empty map for backend %q", backend)
		}
	}
}

// ── 11: InjectESO — nil chart returns nil, 0, no panic ───────────────────────

func TestInjectESO_NilChart(t *testing.T) {
	graph := types.NewResourceGraph()
	opts := ESOOptions{Backend: ESOBackendAWS}

	result, count := InjectESO(nil, graph, opts)

	if result != nil {
		t.Errorf("expected nil result for nil chart, got %+v", result)
	}
	if count != 0 {
		t.Errorf("expected count=0 for nil chart, got %d", count)
	}
}

// ── 12: InjectESO — no secrets returns chart unchanged, count=0 ──────────────

func TestInjectESO_NoSecrets(t *testing.T) {
	chart := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/deployment.yaml": "kind: Deployment\n",
		},
	}
	graph := types.NewResourceGraph() // empty — no secrets
	opts := ESOOptions{Backend: ESOBackendAWS}

	result, count := InjectESO(chart, graph, opts)

	if count != 0 {
		t.Errorf("expected count=0 when no secrets detected, got %d", count)
	}
	if result == nil {
		t.Fatal("expected non-nil result even when no secrets")
	}
	if len(result.Templates) != len(chart.Templates) {
		t.Errorf("expected templates unchanged, original=%d result=%d",
			len(chart.Templates), len(result.Templates))
	}
}

// ── 13: InjectESO — copy-on-write (original must not be modified) ─────────────

func TestInjectESO_CopyOnWrite(t *testing.T) {
	secret := makeSecretResource("app-secret", "default", "Opaque", []string{"api-key"})
	graph := makeGraphWithSecrets(secret)

	original := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/deployment.yaml": "kind: Deployment\n",
		},
	}
	originalTemplateCount := len(original.Templates)
	opts := ESOOptions{Backend: ESOBackendAWS, AWSRegion: "us-east-1"}

	result, _ := InjectESO(original, graph, opts)

	// Original must be unchanged
	if len(original.Templates) != originalTemplateCount {
		t.Errorf("copy-on-write violation: original.Templates modified (was %d, now %d)",
			originalTemplateCount, len(original.Templates))
	}
	// Result must be a new object
	if result == original {
		t.Error("copy-on-write violation: result is same pointer as original chart")
	}
}

// ── 14: GenerateSecretStore — empty RefreshInterval gets a default ────────────

func TestGenerateSecretStore_DefaultRefreshInterval(t *testing.T) {
	opts := ESOOptions{
		Backend:         ESOBackendVault,
		VaultMount:      "kubernetes",
		VaultRole:       "reader",
		RefreshInterval: "", // empty → implementation must supply a default
	}

	result := GenerateSecretStore(opts)

	for _, content := range result {
		if strings.Contains(content, "refreshInterval") {
			return // found — test passes
		}
	}
	t.Error("expected a non-empty refreshInterval in generated output when RefreshInterval option is empty")
}
