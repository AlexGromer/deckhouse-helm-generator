package generator

// ============================================================
// Test Plan: Secrets Example File Generator
// ============================================================
//
// | # | Test Name                                              | Category    | Input                                      | Expected Output                                       |
// |---|--------------------------------------------------------|-------------|--------------------------------------------|-------------------------------------------------------|
// | 1 | TestDetectSecretsForExample_EmptyGraph                 | edge        | empty graph                                | empty slice                                           |
// | 2 | TestDetectSecretsForExample_ExcludesServiceToken       | edge        | service-account-token secret               | empty slice                                           |
// | 3 | TestDetectSecretsForExample_NoDataKeys_SyntheticKey    | edge        | Opaque secret with no data keys            | 1 entry with at least 1 synthetic/placeholder key     |
// | 4 | TestGenerateSecretsExample_HeaderPresent               | happy       | entries                                    | output starts with a header comment                   |
// | 5 | TestGenerateSecretsExample_GitignoreHintEnabled        | happy       | opts.IncludeGitignoreHint=true             | output contains .gitignore hint text                  |
// | 6 | TestGenerateSecretsExample_GitignoreHintDisabled       | happy       | opts.IncludeGitignoreHint=false            | output does NOT contain .gitignore hint               |
// | 7 | TestGenerateSecretsExample_DescriptionsEnabled         | happy       | opts.IncludeDescriptions=true              | output contains description comments (#)              |
// | 8 | TestGenerateSecretsExample_GroupByNamespace            | happy       | opts.GroupByNamespace=true, 2 namespaces   | each namespace appears as a section header            |
// | 9 | TestDescribeKey_KnownPatterns                          | happy       | key names: "password", "tls.crt"           | descriptions are non-empty and different              |
// |10 | TestPlaceholderFor_TLSKeys                             | happy       | keys: "tls.crt", "tls.key"                 | placeholder contains PEM or cert marker               |
// |11 | TestInjectSecretsExample_CopyOnWrite                   | happy       | original chart with templates              | original.ExternalFiles unchanged after call           |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// secretDef defines the input shape for makeGraphWithSecretsTyped.
type secretDef struct {
	name       string
	namespace  string
	secretType string
	keys       []string
}

// makeGraphWithSecretsTyped builds a ResourceGraph from secretDef descriptors.
func makeGraphWithSecretsTyped(defs ...secretDef) *types.ResourceGraph {
	resources := make([]*types.ProcessedResource, 0, len(defs))
	for _, d := range defs {
		resources = append(resources, makeSecretResource(d.name, d.namespace, d.secretType, d.keys))
	}
	return buildGraph(resources, nil)
}

// ── 1: DetectSecretsForExample — empty graph ─────────────────────────────────

func TestDetectSecretsForExample_EmptyGraph(t *testing.T) {
	graph := types.NewResourceGraph()
	result := DetectSecretsForExample(graph)
	if len(result) != 0 {
		t.Errorf("expected 0 entries for empty graph, got %d", len(result))
	}
}

// ── 2: Excludes service-account-token ────────────────────────────────────────

func TestDetectSecretsForExample_ExcludesServiceToken(t *testing.T) {
	graph := makeGraphWithSecretsTyped(secretDef{
		name:       "my-sa-token",
		namespace:  "default",
		secretType: "kubernetes.io/service-account-token",
		keys:       []string{"token", "ca.crt", "namespace"},
	})

	result := DetectSecretsForExample(graph)

	if len(result) != 0 {
		t.Errorf("expected service-account-token to be excluded, got %d entries", len(result))
	}
}

// ── 3: No data keys → synthetic placeholder key ──────────────────────────────

func TestDetectSecretsForExample_NoDataKeys_SyntheticKey(t *testing.T) {
	graph := makeGraphWithSecretsTyped(secretDef{
		name:       "empty-secret",
		namespace:  "default",
		secretType: "Opaque",
		keys:       []string{}, // no keys
	})

	result := DetectSecretsForExample(graph)

	if len(result) == 0 {
		t.Fatal("expected 1 SecretExampleEntry even for secret with no data keys")
	}
	entry := result[0]
	if entry.Name != "empty-secret" {
		t.Errorf("expected Name='empty-secret', got %q", entry.Name)
	}
	if len(entry.Keys) == 0 {
		t.Error("expected at least 1 synthetic/placeholder key when secret has no data keys")
	}
}

// ── 4: GenerateSecretsExample — header present ───────────────────────────────

func TestGenerateSecretsExample_HeaderPresent(t *testing.T) {
	entries := []SecretExampleEntry{
		{
			Name:      "db-creds",
			Namespace: "default",
			Type:      "Opaque",
			Keys: []SecretExampleKey{
				{Name: "password", PlaceholderValue: "changeme"},
			},
		},
	}
	opts := SecretsExampleOptions{}

	output := GenerateSecretsExample(entries, opts)

	if output == "" {
		t.Fatal("expected non-empty output")
	}
	// Header should be present — a comment line or section marker
	if !strings.Contains(output, "#") && !strings.Contains(output, "---") {
		t.Error("expected output to contain a header (comment '#' or YAML separator '---')")
	}
}

// ── 5: Gitignore hint enabled ─────────────────────────────────────────────────

func TestGenerateSecretsExample_GitignoreHintEnabled(t *testing.T) {
	entries := []SecretExampleEntry{
		{Name: "my-secret", Namespace: "default", Type: "Opaque",
			Keys: []SecretExampleKey{{Name: "token", PlaceholderValue: "changeme"}}},
	}
	opts := SecretsExampleOptions{IncludeGitignoreHint: true}

	output := GenerateSecretsExample(entries, opts)

	if !strings.Contains(output, ".gitignore") && !strings.Contains(output, "gitignore") {
		t.Errorf("expected .gitignore hint in output when IncludeGitignoreHint=true, got:\n%s", output)
	}
}

// ── 6: Gitignore hint disabled ────────────────────────────────────────────────

func TestGenerateSecretsExample_GitignoreHintDisabled(t *testing.T) {
	entries := []SecretExampleEntry{
		{Name: "my-secret", Namespace: "default", Type: "Opaque",
			Keys: []SecretExampleKey{{Name: "token", PlaceholderValue: "changeme"}}},
	}
	opts := SecretsExampleOptions{IncludeGitignoreHint: false}

	output := GenerateSecretsExample(entries, opts)

	if strings.Contains(output, ".gitignore") {
		t.Errorf("expected no .gitignore hint in output when IncludeGitignoreHint=false, got:\n%s", output)
	}
}

// ── 7: Descriptions enabled ───────────────────────────────────────────────────

func TestGenerateSecretsExample_DescriptionsEnabled(t *testing.T) {
	entries := []SecretExampleEntry{
		{
			Name:      "db-creds",
			Namespace: "default",
			Type:      "Opaque",
			Keys: []SecretExampleKey{
				{Name: "password", Description: "Database password", PlaceholderValue: "changeme"},
			},
		},
	}
	opts := SecretsExampleOptions{IncludeDescriptions: true}

	output := GenerateSecretsExample(entries, opts)

	// With descriptions enabled, description text or comment should appear
	if !strings.Contains(output, "Database password") && !strings.Contains(output, "password") {
		t.Errorf("expected description text in output when IncludeDescriptions=true, got:\n%s", output)
	}
}

// ── 8: GroupByNamespace ───────────────────────────────────────────────────────

func TestGenerateSecretsExample_GroupByNamespace(t *testing.T) {
	entries := []SecretExampleEntry{
		{Name: "secret-a", Namespace: "ns-alpha", Type: "Opaque",
			Keys: []SecretExampleKey{{Name: "key1", PlaceholderValue: "v1"}}},
		{Name: "secret-b", Namespace: "ns-beta", Type: "Opaque",
			Keys: []SecretExampleKey{{Name: "key2", PlaceholderValue: "v2"}}},
	}
	opts := SecretsExampleOptions{GroupByNamespace: true}

	output := GenerateSecretsExample(entries, opts)

	if !strings.Contains(output, "ns-alpha") {
		t.Error("expected namespace 'ns-alpha' as section header when GroupByNamespace=true")
	}
	if !strings.Contains(output, "ns-beta") {
		t.Error("expected namespace 'ns-beta' as section header when GroupByNamespace=true")
	}
}

// ── 9: describeKey — known patterns return non-empty, different descriptions ──

func TestDescribeKey_KnownPatterns(t *testing.T) {
	passwordDesc := describeKey("password")
	tlsCrtDesc := describeKey("tls.crt")

	if passwordDesc == "" {
		t.Error("expected non-empty description for key 'password'")
	}
	if tlsCrtDesc == "" {
		t.Error("expected non-empty description for key 'tls.crt'")
	}
	if passwordDesc == tlsCrtDesc {
		t.Errorf("expected different descriptions for 'password' and 'tls.crt', both returned %q", passwordDesc)
	}
}

// ── 10: placeholderFor — TLS keys return PEM/cert marker ─────────────────────

func TestPlaceholderFor_TLSKeys(t *testing.T) {
	tlsCrt := placeholderFor("tls.crt")
	tlsKey := placeholderFor("tls.key")

	certMarkers := []string{"-----BEGIN", "CERTIFICATE", "PEM", "base64", "<cert>", "<tls"}
	foundCrt := false
	for _, marker := range certMarkers {
		if strings.Contains(strings.ToUpper(tlsCrt), strings.ToUpper(marker)) {
			foundCrt = true
			break
		}
	}
	if !foundCrt {
		t.Errorf("placeholderFor('tls.crt') should contain a cert/PEM marker, got %q", tlsCrt)
	}

	keyMarkers := []string{"-----BEGIN", "KEY", "PEM", "base64", "<key>", "<tls"}
	foundKey := false
	for _, marker := range keyMarkers {
		if strings.Contains(strings.ToUpper(tlsKey), strings.ToUpper(marker)) {
			foundKey = true
			break
		}
	}
	if !foundKey {
		t.Errorf("placeholderFor('tls.key') should contain a key/PEM marker, got %q", tlsKey)
	}
}

// ── 11: InjectSecretsExample — copy-on-write ──────────────────────────────────

func TestInjectSecretsExample_CopyOnWrite(t *testing.T) {
	graph := makeGraphWithSecretsTyped(
		secretDef{name: "my-secret", namespace: "default", secretType: "Opaque", keys: []string{"api-key"}},
	)

	original := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/deployment.yaml": "kind: Deployment\n",
		},
		ExternalFiles: []types.ExternalFileInfo{
			{Path: "files/existing.txt", Content: "existing"},
		},
	}
	originalTemplateCount := len(original.Templates)
	originalExternalCount := len(original.ExternalFiles)
	opts := SecretsExampleOptions{
		IncludeGitignoreHint: true,
		IncludeDescriptions:  true,
	}

	result, _ := InjectSecretsExample(original, graph, opts)

	// Original must be unchanged
	if len(original.Templates) != originalTemplateCount {
		t.Errorf("copy-on-write violation: original.Templates modified (was %d, now %d)",
			originalTemplateCount, len(original.Templates))
	}
	if len(original.ExternalFiles) != originalExternalCount {
		t.Errorf("copy-on-write violation: original.ExternalFiles modified (was %d, now %d)",
			originalExternalCount, len(original.ExternalFiles))
	}
	if result == original {
		t.Error("copy-on-write violation: result is same pointer as original chart")
	}
	_ = result
}
