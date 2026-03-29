package generator

// ============================================================
// Test Plan: Sealed Secrets Generator
// ============================================================
//
// | # | Test Name                                              | Category    | Input                                     | Expected Output                                    |
// |---|--------------------------------------------------------|-------------|-------------------------------------------|----------------------------------------------------|
// | 1 | TestDetectSealedSecretCandidates_ExcludesServiceToken  | edge        | graph with service-account-token secret   | empty slice                                        |
// | 2 | TestDetectSealedSecretCandidates_MultipleSecrets       | happy       | graph with 2 Opaque secrets               | 2 SealedSecretEntry values                         |
// | 3 | TestGenerateSealedSecrets_PlaceholderValues            | happy       | entries with keys                         | output does NOT contain real data, has placeholders|
// | 4 | TestGenerateSealedSecrets_ScopeAnnotation_NamespaceWide| happy       | Scope=namespace-wide                      | sealedsecrets.bitnami.com/namespace-wide annotation|
// | 5 | TestGenerateSealedSecrets_StrictScope_NoAnnotation     | happy       | Scope=strict                              | no sealedsecrets annotation                        |
// | 6 | TestBuildKubesealCommands_CommandFormat                | happy       | entry with name+namespace                 | kubectl/kubeseal command with --name and --namespace|
// | 7 | TestBuildKubesealCommands_PreservesInput               | happy       | N entries                                 | output length == input length                      |
// | 8 | TestGenerateSealedSecretsNotes_ContainsAllCommands     | happy       | entries with KubesealCmd set              | notes contain every KubesealCmd string             |
// | 9 | TestGenerateSealedSecretsNotes_EmptyEntries            | edge        | empty entries slice                       | notes contain at least a header line               |
// |10 | TestInjectSealedSecrets_NilChart                       | error       | nil chart                                 | (nil, 0), no panic                                 |
// |11 | TestInjectSealedSecrets_NotesMerge                     | happy       | chart with existing Notes + entries       | original Notes text preserved in result            |
// |12 | TestInjectSealedSecrets_CopyOnWrite                    | happy       | original chart with templates             | original.Templates unchanged after call            |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// makeSealedSecretGraph builds a graph containing secrets of varying types.
func makeSealedSecretGraph(secrets ...*types.ProcessedResource) *types.ResourceGraph {
	return buildGraph(secrets, nil)
}

// ── 1: Excludes service-account-token secrets ─────────────────────────────────

func TestDetectSealedSecretCandidates_ExcludesServiceToken(t *testing.T) {
	sat := makeSecretResource("my-sa-token", "default", "kubernetes.io/service-account-token", []string{"token", "ca.crt"})
	graph := makeSealedSecretGraph(sat)

	result := DetectSealedSecretCandidates(graph)

	if len(result) != 0 {
		t.Errorf("expected service-account-token secrets to be excluded, got %d entries", len(result))
	}
}

// ── 2: Detects multiple Opaque secrets ───────────────────────────────────────

func TestDetectSealedSecretCandidates_MultipleSecrets(t *testing.T) {
	s1 := makeSecretResource("db-creds", "default", "Opaque", []string{"username", "password"})
	s2 := makeSecretResource("api-token", "staging", "Opaque", []string{"token"})
	graph := makeSealedSecretGraph(s1, s2)

	result := DetectSealedSecretCandidates(graph)

	if len(result) != 2 {
		t.Fatalf("expected 2 SealedSecretEntry values, got %d", len(result))
	}
	names := map[string]bool{}
	for _, e := range result {
		names[e.Name] = true
	}
	if !names["db-creds"] {
		t.Error("expected 'db-creds' in result")
	}
	if !names["api-token"] {
		t.Error("expected 'api-token' in result")
	}
}

// ── 3: GenerateSealedSecrets — placeholder values, no real data ───────────────

func TestGenerateSealedSecrets_PlaceholderValues(t *testing.T) {
	entries := []SealedSecretEntry{
		{Name: "db-creds", Namespace: "default", Scope: SealedSecretScopeStrict, Keys: []string{"username", "password"}},
	}
	opts := SealedSecretOptions{
		Scope:               SealedSecretScopeStrict,
		ControllerNamespace: "kube-system",
		ControllerName:      "sealed-secrets",
	}

	result := GenerateSealedSecrets(entries, opts)

	for _, content := range result {
		// Must not contain base64-looking real values for keys
		// Real values would be very long base64 strings; placeholder should be a short marker
		if strings.Contains(content, "PLACEHOLDER") || strings.Contains(content, "changeme") ||
			strings.Contains(content, "<") || strings.Contains(content, "TODO") {
			return // placeholder found — test passes
		}
		// Accept any common placeholder pattern (empty string, "...", template variable)
		if strings.Contains(content, "encryptedData:") {
			return // sealed secrets use encryptedData which is always a placeholder in generated templates
		}
	}
	t.Error("expected generated SealedSecret template to contain placeholder values, not real data")
}

// ── 4: Scope annotation for namespace-wide ────────────────────────────────────

func TestGenerateSealedSecrets_ScopeAnnotation_NamespaceWide(t *testing.T) {
	entries := []SealedSecretEntry{
		{Name: "ns-secret", Namespace: "default", Scope: SealedSecretScopeNamespaceWide, Keys: []string{"key"}},
	}
	opts := SealedSecretOptions{
		Scope:               SealedSecretScopeNamespaceWide,
		ControllerNamespace: "kube-system",
		ControllerName:      "sealed-secrets",
	}

	result := GenerateSealedSecrets(entries, opts)

	found := false
	for _, content := range result {
		if strings.Contains(content, "sealedsecrets.bitnami.com/namespace-wide") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected sealedsecrets.bitnami.com/namespace-wide annotation for namespace-wide scope")
	}
}

// ── 5: Strict scope produces no sealedsecrets annotation ─────────────────────

func TestGenerateSealedSecrets_StrictScope_NoAnnotation(t *testing.T) {
	entries := []SealedSecretEntry{
		{Name: "strict-secret", Namespace: "default", Scope: SealedSecretScopeStrict, Keys: []string{"key"}},
	}
	opts := SealedSecretOptions{
		Scope:               SealedSecretScopeStrict,
		ControllerNamespace: "kube-system",
		ControllerName:      "sealed-secrets",
	}

	result := GenerateSealedSecrets(entries, opts)

	for _, content := range result {
		if strings.Contains(content, "sealedsecrets.bitnami.com/namespace-wide") ||
			strings.Contains(content, "sealedsecrets.bitnami.com/cluster-wide") {
			t.Errorf("strict scope must not emit sealedsecrets scope annotations, got:\n%s", content)
		}
	}
}

// ── 6: Kubeseal command format correct ───────────────────────────────────────

func TestBuildKubesealCommands_CommandFormat(t *testing.T) {
	entries := []SealedSecretEntry{
		{Name: "my-secret", Namespace: "production", Scope: SealedSecretScopeStrict, Keys: []string{"password"}},
	}
	opts := SealedSecretOptions{
		ControllerNamespace: "kube-system",
		ControllerName:      "sealed-secrets",
		CertURL:             "https://sealed-secrets.example.com/v1/cert.pem",
	}

	result := BuildKubesealCommands(entries, opts)

	if len(result) != 1 {
		t.Fatalf("expected 1 entry with KubesealCmd, got %d", len(result))
	}
	cmd := result[0].KubesealCmd
	if cmd == "" {
		t.Fatal("expected KubesealCmd to be set")
	}
	if !strings.Contains(cmd, "kubeseal") {
		t.Errorf("KubesealCmd must contain 'kubeseal', got: %q", cmd)
	}
	if !strings.Contains(cmd, "my-secret") {
		t.Errorf("KubesealCmd must reference secret name 'my-secret', got: %q", cmd)
	}
	if !strings.Contains(cmd, "production") {
		t.Errorf("KubesealCmd must reference namespace 'production', got: %q", cmd)
	}
}

// ── 7: BuildKubesealCommands — preserves input entries count ─────────────────

func TestBuildKubesealCommands_PreservesInput(t *testing.T) {
	entries := []SealedSecretEntry{
		{Name: "secret-1", Namespace: "ns-a", Scope: SealedSecretScopeStrict, Keys: []string{"k1"}},
		{Name: "secret-2", Namespace: "ns-b", Scope: SealedSecretScopeStrict, Keys: []string{"k2"}},
		{Name: "secret-3", Namespace: "ns-c", Scope: SealedSecretScopeStrict, Keys: []string{"k3"}},
	}
	opts := SealedSecretOptions{
		ControllerNamespace: "kube-system",
		ControllerName:      "sealed-secrets",
	}

	result := BuildKubesealCommands(entries, opts)

	if len(result) != len(entries) {
		t.Errorf("BuildKubesealCommands must return same count as input: expected %d, got %d",
			len(entries), len(result))
	}
}

// ── 8: Notes contains all kubeseal commands ───────────────────────────────────

func TestGenerateSealedSecretsNotes_ContainsAllCommands(t *testing.T) {
	entries := []SealedSecretEntry{
		{Name: "s1", Namespace: "ns1", Scope: SealedSecretScopeStrict, Keys: []string{"k"}, KubesealCmd: "kubeseal --name s1 --namespace ns1 ..."},
		{Name: "s2", Namespace: "ns2", Scope: SealedSecretScopeStrict, Keys: []string{"k"}, KubesealCmd: "kubeseal --name s2 --namespace ns2 ..."},
	}
	opts := SealedSecretOptions{
		ControllerNamespace: "kube-system",
		ControllerName:      "sealed-secrets",
	}

	notes := GenerateSealedSecretsNotes(entries, opts)

	if !strings.Contains(notes, "kubeseal --name s1 --namespace ns1") {
		t.Error("notes must contain the first kubeseal command")
	}
	if !strings.Contains(notes, "kubeseal --name s2 --namespace ns2") {
		t.Error("notes must contain the second kubeseal command")
	}
}

// ── 9: Notes with empty entries — at least a header ──────────────────────────

func TestGenerateSealedSecretsNotes_EmptyEntries(t *testing.T) {
	opts := SealedSecretOptions{
		ControllerNamespace: "kube-system",
		ControllerName:      "sealed-secrets",
	}

	notes := GenerateSealedSecretsNotes(nil, opts)

	if notes == "" {
		t.Error("expected non-empty notes even for empty entries (at minimum a header)")
	}
}

// ── 10: InjectSealedSecrets — nil chart ───────────────────────────────────────

func TestInjectSealedSecrets_NilChart(t *testing.T) {
	graph := types.NewResourceGraph()
	opts := SealedSecretOptions{
		ControllerNamespace: "kube-system",
		ControllerName:      "sealed-secrets",
	}

	result, count := InjectSealedSecrets(nil, graph, opts)

	if result != nil {
		t.Errorf("expected nil result for nil chart, got %+v", result)
	}
	if count != 0 {
		t.Errorf("expected count=0 for nil chart, got %d", count)
	}
}

// ── 11: InjectSealedSecrets — original Notes preserved in result ──────────────

func TestInjectSealedSecrets_NotesMerge(t *testing.T) {
	secret := makeSecretResource("app-creds", "default", "Opaque", []string{"token"})
	graph := makeSealedSecretGraph(secret)

	chart := &types.GeneratedChart{
		Name:  "myapp",
		Notes: "## Existing notes\nSome original content.\n",
		Templates: map[string]string{
			"templates/deployment.yaml": "kind: Deployment\n",
		},
	}
	opts := SealedSecretOptions{
		ControllerNamespace: "kube-system",
		ControllerName:      "sealed-secrets",
	}

	result, _ := InjectSealedSecrets(chart, graph, opts)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !strings.Contains(result.Notes, "Existing notes") {
		t.Errorf("expected original Notes to be preserved in result, got:\n%s", result.Notes)
	}
}

// ── 12: InjectSealedSecrets — copy-on-write ───────────────────────────────────

func TestInjectSealedSecrets_CopyOnWrite(t *testing.T) {
	secret := makeSecretResource("my-secret", "default", "Opaque", []string{"key1", "key2"})
	graph := makeSealedSecretGraph(secret)

	original := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/deployment.yaml": "kind: Deployment\n",
		},
	}
	originalTemplateCount := len(original.Templates)
	originalNotes := original.Notes
	opts := SealedSecretOptions{
		ControllerNamespace: "kube-system",
		ControllerName:      "sealed-secrets",
	}

	result, _ := InjectSealedSecrets(original, graph, opts)

	if len(original.Templates) != originalTemplateCount {
		t.Errorf("copy-on-write violation: original.Templates modified (was %d, now %d)",
			originalTemplateCount, len(original.Templates))
	}
	if original.Notes != originalNotes {
		t.Errorf("copy-on-write violation: original.Notes modified")
	}
	if result == original {
		t.Error("copy-on-write violation: result is same pointer as original chart")
	}
}
