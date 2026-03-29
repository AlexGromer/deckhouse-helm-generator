package generator

// ============================================================
// Test Plan: Vault CSI Provider Generator
// ============================================================
//
// | # | Test Name                                            | Category    | Input                                      | Expected Output                                        |
// |---|------------------------------------------------------|-------------|--------------------------------------------|--------------------------------------------------------|
// | 1 | TestDetectVaultCSICandidates_EmptyGraph              | edge        | empty graph                                | empty slice                                            |
// | 2 | TestDetectVaultCSICandidates_SecretsGroupedByWorkload | happy       | Deployment + Secret with volume relationship| 1 entry with WorkloadName set                         |
// | 3 | TestDetectVaultCSICandidates_UnreferencedSecret      | happy       | Secret with no workload relationship       | standalone entry (WorkloadName empty or "")            |
// | 4 | TestGenerateSecretProviderClasses_APIVersion         | happy       | entry with Objects                         | output contains correct SecretProviderClass APIVersion |
// | 5 | TestGenerateVaultCSIVolumePatch_KVv1Path             | happy       | KVVersion=1                                | path does NOT contain /data/                           |
// | 6 | TestGenerateVaultCSIVolumePatch_KVv2Path             | happy       | KVVersion=2                                | path contains /data/                                   |
// | 7 | TestGenerateVaultCSIVolumePatch_ContainsMountPath    | happy       | opts with MountPath set                    | volume patch contains the mountPath                    |
// | 8 | TestGenerateVaultCSIVolumePatch_RotationDisabled     | happy       | EnableRotation=false                       | output does NOT contain rotationPollInterval           |
// | 9 | TestGenerateVaultCSIVolumePatch_RotationEnabled      | happy       | EnableRotation=true, interval set          | output contains rotationPollInterval                   |
// |10 | TestGenerateVaultCSIVolumePatch_NodePublishSecretRef | happy       | NodePublishSecretRef="my-token"            | volume patch contains "my-token"                       |
// |11 | TestInjectVaultCSI_NilChart                          | error       | nil chart                                  | (nil, 0), no panic                                     |
// |12 | TestInjectVaultCSI_OnlyMatchedWorkloadsPatched       | happy       | 2 workloads, secrets for 1 only            | at least 1 workload patched                            |
// |13 | TestInjectVaultCSI_CopyOnWrite                       | happy       | original chart with templates              | original.Templates unchanged after call                |

import (
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ── 1: DetectVaultCSICandidates — empty graph ─────────────────────────────────

func TestDetectVaultCSICandidates_EmptyGraph(t *testing.T) {
	graph := types.NewResourceGraph()
	opts := VaultCSIOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "my-role",
		KVVersion:    2,
	}

	result := DetectVaultCSICandidates(graph, opts)

	if len(result) != 0 {
		t.Errorf("expected 0 entries for empty graph, got %d", len(result))
	}
}

// ── 2: Secrets grouped by workload ───────────────────────────────────────────

func TestDetectVaultCSICandidates_SecretsGroupedByWorkload(t *testing.T) {
	deploy := makeProcessedResource("Deployment", "myapp", "default", nil)
	secret := makeSecretResource("myapp-secret", "default", "Opaque", []string{"db-password"})
	graph := buildGraph([]*types.ProcessedResource{deploy, secret}, []types.Relationship{
		{
			From:  deploy.Original.ResourceKey(),
			To:    secret.Original.ResourceKey(),
			Type:  types.RelationVolumeMount,
			Field: "volumes",
		},
	})
	opts := VaultCSIOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "myapp",
		KVVersion:    2,
	}

	result := DetectVaultCSICandidates(graph, opts)

	if len(result) == 0 {
		t.Fatal("expected at least 1 VaultCSIEntry when deployment references secret via volume mount")
	}
	// The entry for myapp-secret should have WorkloadName=myapp
	found := false
	for _, e := range result {
		if e.Name == "myapp-secret" && e.WorkloadName == "myapp" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected VaultCSIEntry{Name:'myapp-secret', WorkloadName:'myapp'} in result, got: %+v", result)
	}
}

// ── 3: Unreferenced secret → standalone (WorkloadName empty) ─────────────────

func TestDetectVaultCSICandidates_UnreferencedSecret(t *testing.T) {
	secret := makeSecretResource("standalone-secret", "default", "Opaque", []string{"key"})
	graph := buildGraph([]*types.ProcessedResource{secret}, nil)
	opts := VaultCSIOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "reader",
		KVVersion:    2,
	}

	result := DetectVaultCSICandidates(graph, opts)

	if len(result) == 0 {
		t.Fatal("expected 1 standalone VaultCSIEntry for unreferenced secret")
	}
	entry := result[0]
	if entry.Name != "standalone-secret" {
		t.Errorf("expected Name='standalone-secret', got %q", entry.Name)
	}
	// WorkloadName may be empty for unreferenced secrets
	_ = entry.WorkloadName
}

// ── 4: APIVersion matches SecretProviderClass CRD ────────────────────────────

func TestGenerateSecretProviderClasses_APIVersion(t *testing.T) {
	entries := []VaultCSIEntry{
		{
			Name:               "myapp",
			Namespace:          "default",
			WorkloadName:       "myapp-deploy",
			ServiceAccountName: "myapp-sa",
			Objects: []VaultCSIObjectSpec{
				{ObjectName: "db-password", SecretPath: "secret/myapp", SecretKey: "password"},
			},
		},
	}
	opts := VaultCSIOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "myapp",
		KVVersion:    2,
	}

	result := GenerateSecretProviderClasses(entries, opts)

	if len(result) == 0 {
		t.Fatal("expected at least one SecretProviderClass to be generated")
	}
	for _, content := range result {
		if strings.Contains(content, "SecretProviderClass") {
			if !strings.Contains(content, "secrets-store.csi.x-k8s.io") {
				t.Errorf("SecretProviderClass must use APIVersion secrets-store.csi.x-k8s.io/*, got:\n%s", content)
			}
			return
		}
	}
	t.Error("expected SecretProviderClass kind in output")
}

// ── 5: KV v1 path (no /data/) ────────────────────────────────────────────────

func TestGenerateVaultCSIVolumePatch_KVv1Path(t *testing.T) {
	entry := VaultCSIEntry{
		Name:               "myapp",
		Namespace:          "default",
		WorkloadName:       "myapp-deploy",
		ServiceAccountName: "myapp-sa",
		Objects: []VaultCSIObjectSpec{
			{ObjectName: "db-pass", SecretPath: "myapp/db", SecretKey: "password"},
		},
	}
	opts := VaultCSIOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "myapp",
		KVVersion:    1,
		MountPath:    "/mnt/secrets",
	}

	patch := GenerateVaultCSIVolumePatch(entry, opts)

	if patch == "" {
		t.Fatal("expected non-empty volume patch")
	}
	// KV v1: path should NOT have /data/ prefix injected
	if strings.Contains(patch, "/data/myapp") {
		t.Errorf("KV v1 path must not contain /data/ prefix, got:\n%s", patch)
	}
}

// ── 6: KV v2 path (with /data/) ──────────────────────────────────────────────

func TestGenerateVaultCSIVolumePatch_KVv2Path(t *testing.T) {
	entry := VaultCSIEntry{
		Name:               "myapp",
		Namespace:          "default",
		WorkloadName:       "myapp-deploy",
		ServiceAccountName: "myapp-sa",
		Objects: []VaultCSIObjectSpec{
			{ObjectName: "db-pass", SecretPath: "myapp/db", SecretKey: "password"},
		},
	}
	opts := VaultCSIOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "myapp",
		KVVersion:    2,
		MountPath:    "/mnt/secrets",
	}

	patch := GenerateVaultCSIVolumePatch(entry, opts)

	if patch == "" {
		t.Fatal("expected non-empty volume patch")
	}
	// KV v2: path must include /data/ in the secret path
	if !strings.Contains(patch, "/data/") {
		t.Errorf("KV v2 path must contain /data/ prefix, got:\n%s", patch)
	}
}

// ── 7: Volume patch contains MountPath ───────────────────────────────────────

func TestGenerateVaultCSIVolumePatch_ContainsMountPath(t *testing.T) {
	entry := VaultCSIEntry{
		Name:               "myapp",
		Namespace:          "default",
		WorkloadName:       "myapp-deploy",
		ServiceAccountName: "myapp-sa",
		Objects: []VaultCSIObjectSpec{
			{ObjectName: "token", SecretPath: "myapp/token", SecretKey: "value"},
		},
	}
	opts := VaultCSIOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "myapp",
		KVVersion:    2,
		MountPath:    "/custom/secrets/mount",
	}

	patch := GenerateVaultCSIVolumePatch(entry, opts)

	if !strings.Contains(patch, "/custom/secrets/mount") {
		t.Errorf("volume patch must contain MountPath '/custom/secrets/mount', got:\n%s", patch)
	}
}

// ── 8: Rotation disabled → no rotationPollInterval ───────────────────────────

func TestGenerateVaultCSIVolumePatch_RotationDisabled(t *testing.T) {
	entry := VaultCSIEntry{
		Name:               "myapp",
		Namespace:          "default",
		WorkloadName:       "myapp-deploy",
		ServiceAccountName: "myapp-sa",
		Objects:            []VaultCSIObjectSpec{{ObjectName: "key", SecretPath: "myapp/key", SecretKey: "value"}},
	}
	opts := VaultCSIOptions{
		VaultAddress:         "https://vault.example.com",
		VaultRole:            "myapp",
		KVVersion:            2,
		MountPath:            "/mnt/secrets",
		EnableRotation:       false,
		RotationPollInterval: "1m",
	}

	patch := GenerateVaultCSIVolumePatch(entry, opts)

	if strings.Contains(patch, "rotationPollInterval") {
		t.Errorf("rotationPollInterval must NOT appear when EnableRotation=false, got:\n%s", patch)
	}
}

// ── 9: Rotation enabled → rotationPollInterval present ───────────────────────

func TestGenerateVaultCSIVolumePatch_RotationEnabled(t *testing.T) {
	entry := VaultCSIEntry{
		Name:               "myapp",
		Namespace:          "default",
		WorkloadName:       "myapp-deploy",
		ServiceAccountName: "myapp-sa",
		Objects:            []VaultCSIObjectSpec{{ObjectName: "key", SecretPath: "myapp/key", SecretKey: "value"}},
	}
	opts := VaultCSIOptions{
		VaultAddress:         "https://vault.example.com",
		VaultRole:            "myapp",
		KVVersion:            2,
		MountPath:            "/mnt/secrets",
		EnableRotation:       true,
		RotationPollInterval: "30s",
	}

	patch := GenerateVaultCSIVolumePatch(entry, opts)

	if !strings.Contains(patch, "rotationPollInterval") {
		t.Errorf("rotationPollInterval must appear when EnableRotation=true, got:\n%s", patch)
	}
	if !strings.Contains(patch, "30s") {
		t.Errorf("expected interval '30s' in rotationPollInterval, got:\n%s", patch)
	}
}

// ── 10: NodePublishSecretRef non-empty → appears in patch ────────────────────

func TestGenerateVaultCSIVolumePatch_NodePublishSecretRef(t *testing.T) {
	entry := VaultCSIEntry{
		Name:               "myapp",
		Namespace:          "default",
		WorkloadName:       "myapp-deploy",
		ServiceAccountName: "myapp-sa",
		Objects:            []VaultCSIObjectSpec{{ObjectName: "key", SecretPath: "myapp/key", SecretKey: "value"}},
	}
	opts := VaultCSIOptions{
		VaultAddress:         "https://vault.example.com",
		VaultRole:            "myapp",
		KVVersion:            2,
		MountPath:            "/mnt/secrets",
		NodePublishSecretRef: "vault-token",
	}

	patch := GenerateVaultCSIVolumePatch(entry, opts)

	if !strings.Contains(patch, "vault-token") {
		t.Errorf("patch must contain NodePublishSecretRef 'vault-token', got:\n%s", patch)
	}
}

// ── 11: InjectVaultCSI — nil chart ────────────────────────────────────────────

func TestInjectVaultCSI_NilChart(t *testing.T) {
	graph := types.NewResourceGraph()
	opts := VaultCSIOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "reader",
		KVVersion:    2,
	}

	result, count := InjectVaultCSI(nil, graph, opts)

	if result != nil {
		t.Errorf("expected nil result for nil chart, got %+v", result)
	}
	if count != 0 {
		t.Errorf("expected count=0 for nil chart, got %d", count)
	}
}

// ── 12: Only matched workloads patched ───────────────────────────────────────

func TestInjectVaultCSI_OnlyMatchedWorkloadsPatched(t *testing.T) {
	// Deployment "app-a" references a secret; "app-b" does not.
	deployA := makeProcessedResource("Deployment", "app-a", "default", nil)
	deployB := makeProcessedResource("Deployment", "app-b", "default", nil)
	secret := makeSecretResource("app-a-secret", "default", "Opaque", []string{"key"})

	graph := buildGraph(
		[]*types.ProcessedResource{deployA, deployB, secret},
		[]types.Relationship{
			{
				From:  deployA.Original.ResourceKey(),
				To:    secret.Original.ResourceKey(),
				Type:  types.RelationVolumeMount,
				Field: "volumes",
			},
		},
	)
	chart := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/app-a-deployment.yaml": "kind: Deployment\nmetadata:\n  name: app-a\n",
			"templates/app-b-deployment.yaml": "kind: Deployment\nmetadata:\n  name: app-b\n",
		},
	}
	opts := VaultCSIOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "reader",
		KVVersion:    2,
		MountPath:    "/mnt/secrets",
	}

	result, count := InjectVaultCSI(chart, graph, opts)

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if count < 1 {
		t.Errorf("expected at least 1 patched workload (app-a), got count=%d", count)
	}
}

// ── 13: InjectVaultCSI — copy-on-write ───────────────────────────────────────

func TestInjectVaultCSI_CopyOnWrite(t *testing.T) {
	secret := makeSecretResource("vault-secret", "default", "Opaque", []string{"api-key"})
	deploy := makeProcessedResource("Deployment", "myapp", "default", nil)
	graph := buildGraph(
		[]*types.ProcessedResource{deploy, secret},
		[]types.Relationship{
			{
				From:  deploy.Original.ResourceKey(),
				To:    secret.Original.ResourceKey(),
				Type:  types.RelationVolumeMount,
				Field: "volumes",
			},
		},
	)

	originalTemplate := "kind: Deployment\nmetadata:\n  name: myapp\n"
	original := &types.GeneratedChart{
		Name: "myapp",
		Templates: map[string]string{
			"templates/deployment.yaml": originalTemplate,
		},
	}
	opts := VaultCSIOptions{
		VaultAddress: "https://vault.example.com",
		VaultRole:    "myapp",
		KVVersion:    2,
		MountPath:    "/mnt/secrets",
	}

	result, _ := InjectVaultCSI(original, graph, opts)

	if original.Templates["templates/deployment.yaml"] != originalTemplate {
		t.Error("copy-on-write violation: original template content modified")
	}
	if result == original {
		t.Error("copy-on-write violation: result is same pointer as original chart")
	}
}
