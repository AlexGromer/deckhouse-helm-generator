package generator

// ============================================================
// Test Plan: SOPS + Helm Secrets Integration (Task 5.7.6)
// ============================================================
//
// | #  | Test Name                                              | Category    | Input                                                      | Expected Output                                                              |
// |----|--------------------------------------------------------|-------------|------------------------------------------------------------|------------------------------------------------------------------------------|
// |  1 | TestGenerateSOPSConfig_AgeKey                          | happy       | KeyType="age", KeyID="age1..."                             | .sops.yaml contains age: recipients entry with the given KeyID               |
// |  2 | TestGenerateSOPSConfig_PGPKey                          | happy       | KeyType="pgp", KeyID="ABCD1234"                            | .sops.yaml contains pgp: key_groups entry with the fingerprint               |
// |  3 | TestGenerateSOPSConfig_AWSKMS                          | happy       | KeyType="aws-kms", KeyID="arn:aws:kms:..."                 | .sops.yaml contains kms: arn entry                                           |
// |  4 | TestGenerateSOPSConfig_GCPKMS                          | happy       | KeyType="gcp-kms", KeyID="projects/..."                    | .sops.yaml contains gcp_kms: resource_id entry                               |
// |  5 | TestGenerateSOPSConfig_EncryptedFilesPattern           | happy       | EncryptedFiles=["secrets.yaml","values-prod.yaml"]         | .sops.yaml has creation_rules entries for each file                          |
// |  6 | TestGenerateSOPSConfig_EmptyOpts                       | edge        | all-zero SOPSOptions                                       | returns non-empty .sops.yaml map (at least a valid YAML skeleton)            |
// |  7 | TestGenerateHelmSecretsWrapper_ContainsPlugin          | happy       | Provider="helm-secrets", KeyType="age"                     | wrapper string references helm-secrets or vals plugin call                   |
// |  8 | TestGenerateHelmSecretsWrapper_SOPSProvider            | happy       | Provider="sops"                                            | wrapper string references sops binary invocation                             |
// |  9 | TestInjectSOPS_AddsExternalFiles                       | happy       | chart with templates, valid opts                           | returned chart has .sops.yaml in ExternalFiles                               |
// | 10 | TestInjectSOPS_NilChart                                | error       | nil chart                                                  | (nil, 0), no panic                                                           |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── 1: Age key → recipients in .sops.yaml ─────────────────────────────────────

func TestGenerateSOPSConfig_AgeKey(t *testing.T) {
	opts := SOPSOptions{
		KeyType:  "age",
		KeyID:    "age1qyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpzq5x7d9",
		Provider: "sops",
	}

	result := GenerateSOPSConfig(opts)

	if result == nil {
		t.Fatal("expected non-nil map from GenerateSOPSConfig")
	}
	sopsYAML, ok := result[".sops.yaml"]
	if !ok {
		t.Fatalf("expected .sops.yaml key in result, got keys: %v", sopsConfigKeys(result))
	}
	if !strings.Contains(sopsYAML, "age") {
		t.Errorf(".sops.yaml must contain 'age' key type, got: %s", sopsYAML)
	}
	if !strings.Contains(sopsYAML, opts.KeyID) {
		t.Errorf(".sops.yaml must contain the KeyID %q, got: %s", opts.KeyID, sopsYAML)
	}
}

// ── 2: PGP key → fingerprint in .sops.yaml ────────────────────────────────────

func TestGenerateSOPSConfig_PGPKey(t *testing.T) {
	opts := SOPSOptions{
		KeyType:  "pgp",
		KeyID:    "ABCD1234EF567890ABCD1234EF567890ABCD1234",
		Provider: "sops",
	}

	result := GenerateSOPSConfig(opts)

	sopsYAML, ok := result[".sops.yaml"]
	if !ok {
		t.Fatalf("expected .sops.yaml key, got: %v", sopsConfigKeys(result))
	}
	if !strings.Contains(sopsYAML, "pgp") {
		t.Errorf(".sops.yaml must contain 'pgp' for pgp key type, got: %s", sopsYAML)
	}
	if !strings.Contains(sopsYAML, opts.KeyID) {
		t.Errorf(".sops.yaml must contain PGP fingerprint %q, got: %s", opts.KeyID, sopsYAML)
	}
}

// ── 3: AWS KMS → KMS ARN in .sops.yaml ───────────────────────────────────────

func TestGenerateSOPSConfig_AWSKMS(t *testing.T) {
	opts := SOPSOptions{
		KeyType:  "aws-kms",
		KeyID:    "arn:aws:kms:us-east-1:123456789012:key/mrk-abc123",
		Provider: "sops",
	}

	result := GenerateSOPSConfig(opts)

	sopsYAML, ok := result[".sops.yaml"]
	if !ok {
		t.Fatalf("expected .sops.yaml key, got: %v", sopsConfigKeys(result))
	}
	if !strings.Contains(sopsYAML, "kms") {
		t.Errorf(".sops.yaml must contain 'kms' for aws-kms key type, got: %s", sopsYAML)
	}
	if !strings.Contains(sopsYAML, opts.KeyID) {
		t.Errorf(".sops.yaml must contain KMS ARN %q, got: %s", opts.KeyID, sopsYAML)
	}
}

// ── 4: GCP KMS → resource_id in .sops.yaml ───────────────────────────────────

func TestGenerateSOPSConfig_GCPKMS(t *testing.T) {
	opts := SOPSOptions{
		KeyType:  "gcp-kms",
		KeyID:    "projects/my-project/locations/global/keyRings/my-ring/cryptoKeys/my-key",
		Provider: "sops",
	}

	result := GenerateSOPSConfig(opts)

	sopsYAML, ok := result[".sops.yaml"]
	if !ok {
		t.Fatalf("expected .sops.yaml key, got: %v", sopsConfigKeys(result))
	}
	if !strings.Contains(sopsYAML, "gcp_kms") && !strings.Contains(sopsYAML, "gcp-kms") && !strings.Contains(sopsYAML, "gcp") {
		t.Errorf(".sops.yaml must contain gcp_kms section for gcp-kms key type, got: %s", sopsYAML)
	}
	if !strings.Contains(sopsYAML, opts.KeyID) {
		t.Errorf(".sops.yaml must contain GCP key resource %q, got: %s", opts.KeyID, sopsYAML)
	}
}

// ── 5: EncryptedFiles → creation_rules entries ────────────────────────────────

func TestGenerateSOPSConfig_EncryptedFilesPattern(t *testing.T) {
	opts := SOPSOptions{
		KeyType:        "age",
		KeyID:          "age1qyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpzq5x7d9",
		EncryptedFiles: []string{"secrets.yaml", "values-prod.yaml"},
		Provider:       "sops",
	}

	result := GenerateSOPSConfig(opts)

	sopsYAML, ok := result[".sops.yaml"]
	if !ok {
		t.Fatalf("expected .sops.yaml key, got: %v", sopsConfigKeys(result))
	}
	if !strings.Contains(sopsYAML, "creation_rules") {
		t.Errorf(".sops.yaml must contain creation_rules for encrypted files, got: %s", sopsYAML)
	}
	for _, f := range opts.EncryptedFiles {
		if !strings.Contains(sopsYAML, f) {
			t.Errorf("expected file %q in creation_rules, not found in .sops.yaml:\n%s", f, sopsYAML)
		}
	}
}

// ── 6: Empty opts → valid YAML skeleton returned ─────────────────────────────

func TestGenerateSOPSConfig_EmptyOpts(t *testing.T) {
	opts := SOPSOptions{}

	// Must not panic
	result := GenerateSOPSConfig(opts)

	if result == nil {
		t.Fatal("expected non-nil result even for empty SOPSOptions")
	}
	// The returned map must contain at least .sops.yaml
	if _, ok := result[".sops.yaml"]; !ok {
		t.Errorf("expected .sops.yaml key even for empty opts, got keys: %v", sopsConfigKeys(result))
	}
}

// ── 7: Helm-secrets provider → wrapper references helm-secrets ───────────────

func TestGenerateHelmSecretsWrapper_ContainsPlugin(t *testing.T) {
	opts := SOPSOptions{
		KeyType:  "age",
		KeyID:    "age1qyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpzq5x7d9",
		Provider: "helm-secrets",
	}

	wrapper := GenerateHelmSecretsWrapper(opts)

	if wrapper == "" {
		t.Fatal("expected non-empty helm-secrets wrapper script")
	}
	// Must reference helm-secrets plugin or vals driver
	if !strings.Contains(wrapper, "helm-secrets") && !strings.Contains(wrapper, "helm secrets") {
		t.Errorf("helm-secrets wrapper must reference 'helm-secrets' or 'helm secrets', got: %s", wrapper)
	}
}

// ── 8: SOPS provider → wrapper references sops binary ────────────────────────

func TestGenerateHelmSecretsWrapper_SOPSProvider(t *testing.T) {
	opts := SOPSOptions{
		KeyType:  "age",
		KeyID:    "age1qyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpzq5x7d9",
		Provider: "sops",
	}

	wrapper := GenerateHelmSecretsWrapper(opts)

	if wrapper == "" {
		t.Fatal("expected non-empty sops wrapper script")
	}
	if !strings.Contains(wrapper, "sops") {
		t.Errorf("sops provider wrapper must reference 'sops', got: %s", wrapper)
	}
}

// ── 9: InjectSOPS adds .sops.yaml to ExternalFiles ───────────────────────────

func TestInjectSOPS_AddsExternalFiles(t *testing.T) {
	chart := &types.GeneratedChart{
		Name:          "test-chart",
		Templates:     map[string]string{},
		ExternalFiles: []types.ExternalFileInfo{},
	}
	opts := SOPSOptions{
		KeyType:        "age",
		KeyID:          "age1qyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpzq5x7d9",
		EncryptedFiles: []string{"secrets.yaml"},
		Provider:       "sops",
	}

	result, count := InjectSOPS(chart, opts)

	if result == nil {
		t.Fatal("expected non-nil result chart from InjectSOPS")
	}
	if count == 0 {
		t.Error("expected count > 0 — at least .sops.yaml should be added")
	}
	// Verify .sops.yaml appears in ExternalFiles
	found := false
	for _, ef := range result.ExternalFiles {
		if ef.Path == ".sops.yaml" || strings.HasSuffix(ef.Path, ".sops.yaml") {
			found = true
			if ef.Content == "" {
				t.Error(".sops.yaml ExternalFile must have non-empty Content")
			}
			break
		}
	}
	if !found {
		t.Errorf("expected .sops.yaml in ExternalFiles after InjectSOPS, got: %v", sopsExternalFilePaths(result.ExternalFiles))
	}
	// copy-on-write: original chart must be unchanged
	if len(chart.ExternalFiles) != 0 {
		t.Error("copy-on-write violation: original chart ExternalFiles modified")
	}
	if result == chart {
		t.Error("copy-on-write violation: result is same pointer as original chart")
	}
}

// ── 10: InjectSOPS — nil chart → (nil, 0), no panic ──────────────────────────

func TestInjectSOPS_NilChart(t *testing.T) {
	opts := SOPSOptions{
		KeyType:  "age",
		KeyID:    "age1qyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpzq5x7d9",
		Provider: "sops",
	}

	result, count := InjectSOPS(nil, opts)

	if result != nil {
		t.Errorf("expected nil result for nil chart, got %+v", result)
	}
	if count != 0 {
		t.Errorf("expected count=0 for nil chart, got %d", count)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// sopsConfigKeys returns the keys of a map[string]string (local to sops tests).
func sopsConfigKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func sopsExternalFilePaths(files []types.ExternalFileInfo) []string {
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	return paths
}
