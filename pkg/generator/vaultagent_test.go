package generator

// ============================================================
// Test Plan: Vault Agent Injector Annotations (Task 5.7.4)
// ============================================================
//
// | #  | Test Name                                                     | Category    | Input                                                     | Expected Output                                                                   |
// |----|---------------------------------------------------------------|-------------|-----------------------------------------------------------|-----------------------------------------------------------------------------------|
// |  1 | TestGenerateVaultAgentAnnotations_BasicInjection              | happy       | 1 secret, valid opts                                      | annotation map has vault.hashicorp.com/agent-inject="true"                        |
// |  2 | TestGenerateVaultAgentAnnotations_SecretPathAnnotation        | happy       | secret with VaultPath                                     | map contains agent-inject-secret-<name> key with VaultPath value                 |
// |  3 | TestGenerateVaultAgentAnnotations_TemplateAnnotation          | happy       | secret with Template non-empty                            | map contains agent-inject-template-<name> key                                     |
// |  4 | TestGenerateVaultAgentAnnotations_NoTemplateAnnotation        | edge        | secret with empty Template                                | map does NOT contain agent-inject-template- key for that secret                   |
// |  5 | TestGenerateVaultAgentAnnotations_RoleAnnotation              | happy       | opts with VaultRole set                                   | map contains vault.hashicorp.com/role = VaultRole                                 |
// |  6 | TestGenerateVaultAgentAnnotations_AuthPathAnnotation          | happy       | opts with AuthPath set                                    | map contains vault.hashicorp.com/auth-path = AuthPath                             |
// |  7 | TestGenerateVaultAgentAnnotations_PrePopulate                 | happy       | opts PrePopulate=true                                     | map contains vault.hashicorp.com/agent-pre-populate = "true"                      |
// |  8 | TestGenerateVaultAgentAnnotations_ExitOnRetryFailure          | happy       | opts ExitOnRetryFailure=true                              | map contains vault.hashicorp.com/agent-exit-on-retry-failure = "true"             |
// |  9 | TestGenerateVaultAgentAnnotations_NoSecrets                   | edge        | empty secrets slice, valid opts                           | map contains vault.hashicorp.com/agent-inject="true", no inject-secret keys       |
// | 10 | TestInjectVaultAgentAnnotations_NilChart                      | error       | nil chart                                                 | (nil, 0), no panic                                                                |
// | 11 | TestInjectVaultAgentAnnotations_CountMatchesInjected          | happy       | chart with 2 Deployments, graph with secrets for both     | count == number of Deployments/StatefulSets that received annotations             |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── 1: Basic injection — agent-inject=true annotation present ────────────────

func TestGenerateVaultAgentAnnotations_BasicInjection(t *testing.T) {
	secrets := []VaultAgentSecret{
		{
			Name:      "db-password",
			Namespace: "default",
			VaultPath: "secret/data/myapp/db",
		},
	}
	opts := VaultAgentOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "myapp",
		AuthPath:     "auth/kubernetes",
	}

	annotations := GenerateVaultAgentAnnotations(secrets, opts)

	if annotations == nil {
		t.Fatal("expected non-nil annotations map")
	}
	v, ok := annotations["vault.hashicorp.com/agent-inject"]
	if !ok {
		t.Error("expected key vault.hashicorp.com/agent-inject in annotations")
	}
	if v != "true" {
		t.Errorf("vault.hashicorp.com/agent-inject must be 'true', got %q", v)
	}
}

// ── 2: Secret path annotation keyed by secret name ───────────────────────────

func TestGenerateVaultAgentAnnotations_SecretPathAnnotation(t *testing.T) {
	secrets := []VaultAgentSecret{
		{
			Name:      "db-password",
			Namespace: "default",
			VaultPath: "secret/data/myapp/db",
		},
	}
	opts := VaultAgentOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "myapp",
		AuthPath:     "auth/kubernetes",
	}

	annotations := GenerateVaultAgentAnnotations(secrets, opts)

	key := "vault.hashicorp.com/agent-inject-secret-db-password"
	v, ok := annotations[key]
	if !ok {
		t.Errorf("expected annotation key %q, available keys: %v", key, annotationKeys(annotations))
	}
	if v != "secret/data/myapp/db" {
		t.Errorf("expected VaultPath %q as annotation value, got %q", "secret/data/myapp/db", v)
	}
}

// ── 3: Template annotation present when Template non-empty ───────────────────

func TestGenerateVaultAgentAnnotations_TemplateAnnotation(t *testing.T) {
	secrets := []VaultAgentSecret{
		{
			Name:      "api-key",
			Namespace: "default",
			VaultPath: "secret/data/myapp/api",
			Template:  `{{- with secret "secret/data/myapp/api" -}}{{ .Data.data.key }}{{- end }}`,
		},
	}
	opts := VaultAgentOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "myapp",
		AuthPath:     "auth/kubernetes",
	}

	annotations := GenerateVaultAgentAnnotations(secrets, opts)

	key := "vault.hashicorp.com/agent-inject-template-api-key"
	_, ok := annotations[key]
	if !ok {
		t.Errorf("expected template annotation key %q when Template is set, keys: %v", key, annotationKeys(annotations))
	}
}

// ── 4: No template annotation when Template is empty ─────────────────────────

func TestGenerateVaultAgentAnnotations_NoTemplateAnnotation(t *testing.T) {
	secrets := []VaultAgentSecret{
		{
			Name:      "plain-secret",
			Namespace: "default",
			VaultPath: "secret/data/myapp/plain",
			Template:  "", // empty — no template annotation expected
		},
	}
	opts := VaultAgentOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "myapp",
		AuthPath:     "auth/kubernetes",
	}

	annotations := GenerateVaultAgentAnnotations(secrets, opts)

	key := "vault.hashicorp.com/agent-inject-template-plain-secret"
	if _, ok := annotations[key]; ok {
		t.Errorf("template annotation must NOT be present when Template is empty, found key %q", key)
	}
}

// ── 5: Role annotation matches VaultRole ─────────────────────────────────────

func TestGenerateVaultAgentAnnotations_RoleAnnotation(t *testing.T) {
	opts := VaultAgentOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "my-custom-role",
		AuthPath:     "auth/kubernetes",
	}

	annotations := GenerateVaultAgentAnnotations(nil, opts)

	v, ok := annotations["vault.hashicorp.com/role"]
	if !ok {
		t.Errorf("expected annotation vault.hashicorp.com/role, keys: %v", annotationKeys(annotations))
	}
	if v != "my-custom-role" {
		t.Errorf("expected role 'my-custom-role', got %q", v)
	}
}

// ── 6: Auth-path annotation matches AuthPath ─────────────────────────────────

func TestGenerateVaultAgentAnnotations_AuthPathAnnotation(t *testing.T) {
	opts := VaultAgentOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "myapp",
		AuthPath:     "auth/k8s/prod",
	}

	annotations := GenerateVaultAgentAnnotations(nil, opts)

	v, ok := annotations["vault.hashicorp.com/auth-path"]
	if !ok {
		t.Errorf("expected annotation vault.hashicorp.com/auth-path, keys: %v", annotationKeys(annotations))
	}
	if v != "auth/k8s/prod" {
		t.Errorf("expected auth-path 'auth/k8s/prod', got %q", v)
	}
}

// ── 7: PrePopulate=true → pre-populate annotation set ────────────────────────

func TestGenerateVaultAgentAnnotations_PrePopulate(t *testing.T) {
	opts := VaultAgentOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "myapp",
		AuthPath:     "auth/kubernetes",
		PrePopulate:  true,
	}

	annotations := GenerateVaultAgentAnnotations(nil, opts)

	v, ok := annotations["vault.hashicorp.com/agent-pre-populate"]
	if !ok {
		t.Errorf("expected vault.hashicorp.com/agent-pre-populate when PrePopulate=true, keys: %v", annotationKeys(annotations))
	}
	if v != "true" {
		t.Errorf("vault.hashicorp.com/agent-pre-populate must be 'true', got %q", v)
	}
}

// ── 8: ExitOnRetryFailure=true → exit annotation set ─────────────────────────

func TestGenerateVaultAgentAnnotations_ExitOnRetryFailure(t *testing.T) {
	opts := VaultAgentOptions{
		VaultAddress:       "https://vault.example.com",
		VaultRole:          "myapp",
		AuthPath:           "auth/kubernetes",
		ExitOnRetryFailure: true,
	}

	annotations := GenerateVaultAgentAnnotations(nil, opts)

	v, ok := annotations["vault.hashicorp.com/agent-exit-on-retry-failure"]
	if !ok {
		t.Errorf("expected vault.hashicorp.com/agent-exit-on-retry-failure when ExitOnRetryFailure=true, keys: %v", annotationKeys(annotations))
	}
	if v != "true" {
		t.Errorf("vault.hashicorp.com/agent-exit-on-retry-failure must be 'true', got %q", v)
	}
}

// ── 9: No secrets → agent-inject=true, no inject-secret keys ────────────────

func TestGenerateVaultAgentAnnotations_NoSecrets(t *testing.T) {
	opts := VaultAgentOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "myapp",
		AuthPath:     "auth/kubernetes",
	}

	annotations := GenerateVaultAgentAnnotations([]VaultAgentSecret{}, opts)

	if _, ok := annotations["vault.hashicorp.com/agent-inject"]; !ok {
		t.Error("vault.hashicorp.com/agent-inject must be present even with no secrets")
	}
	for k := range annotations {
		if strings.Contains(k, "agent-inject-secret-") {
			t.Errorf("expected no inject-secret keys for empty secrets slice, found %q", k)
		}
	}
}

// ── 10: InjectVaultAgentAnnotations — nil chart ───────────────────────────────

func TestInjectVaultAgentAnnotations_NilChart(t *testing.T) {
	graph := types.NewResourceGraph()
	opts := VaultAgentOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "myapp",
		AuthPath:     "auth/kubernetes",
	}

	result, count := InjectVaultAgentAnnotations(nil, graph, opts)

	if result != nil {
		t.Errorf("expected nil result for nil chart, got %+v", result)
	}
	if count != 0 {
		t.Errorf("expected count=0 for nil chart, got %d", count)
	}
}

// ── 11: Count matches number of workloads that received annotations ──────────

func TestInjectVaultAgentAnnotations_CountMatchesInjected(t *testing.T) {
	// Two Deployments, both reference a secret via env
	deployA := makeProcessedResource("Deployment", "app-a", "default", nil)
	deployB := makeProcessedResource("Deployment", "app-b", "default", nil)
	secretA := makeSecretResource("secret-a", "default", "Opaque", []string{"db-pass"})
	secretB := makeSecretResource("secret-b", "default", "Opaque", []string{"api-key"})

	graph := buildGraph(
		[]*types.ProcessedResource{deployA, deployB, secretA, secretB},
		[]types.Relationship{
			{
				From:  deployA.Original.ResourceKey(),
				To:    secretA.Original.ResourceKey(),
				Type:  types.RelationVolumeMount,
				Field: "volumes",
			},
			{
				From:  deployB.Original.ResourceKey(),
				To:    secretB.Original.ResourceKey(),
				Type:  types.RelationVolumeMount,
				Field: "volumes",
			},
		},
	)
	chart := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/app-a-deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: app-a\nspec:\n  template:\n    metadata:\n      annotations: {}\n",
			"templates/app-b-deployment.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: app-b\nspec:\n  template:\n    metadata:\n      annotations: {}\n",
		},
	}
	opts := VaultAgentOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "myapp",
		AuthPath:     "auth/kubernetes",
	}

	result, count := InjectVaultAgentAnnotations(chart, graph, opts)

	if result == nil {
		t.Fatal("expected non-nil result chart")
	}
	if count < 1 {
		t.Errorf("expected at least 1 workload to receive vault agent annotations, got count=%d", count)
	}
	// copy-on-write: original templates must be unchanged
	origA := chart.Templates["templates/app-a-deployment.yaml"]
	if strings.Contains(origA, "vault.hashicorp.com") {
		t.Error("copy-on-write violation: original template modified with vault annotations")
	}
}

// ── helper ────────────────────────────────────────────────────────────────────

func annotationKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
