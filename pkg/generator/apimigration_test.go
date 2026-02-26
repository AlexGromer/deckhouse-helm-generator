package generator

import (
	"testing"
)

// ============================================================
// MigrateAPIVersion Tests
// ============================================================

func TestMigrateAPIVersion_DeprecatedExtensionsIngress_MigratesCorrectly(t *testing.T) {
	newAPI, newKind, migrated := MigrateAPIVersion("extensions/v1beta1", "Ingress")

	if !migrated {
		t.Fatal("expected migration to be available for extensions/v1beta1 Ingress")
	}
	if newAPI != "networking.k8s.io/v1" {
		t.Errorf("expected newAPI 'networking.k8s.io/v1', got %q", newAPI)
	}
	if newKind != "Ingress" {
		t.Errorf("expected newKind 'Ingress', got %q", newKind)
	}
}

func TestMigrateAPIVersion_DeprecatedNetworkingIngress_MigratesCorrectly(t *testing.T) {
	newAPI, newKind, migrated := MigrateAPIVersion("networking.k8s.io/v1beta1", "Ingress")

	if !migrated {
		t.Fatal("expected migration to be available for networking.k8s.io/v1beta1 Ingress")
	}
	if newAPI != "networking.k8s.io/v1" {
		t.Errorf("expected newAPI 'networking.k8s.io/v1', got %q", newAPI)
	}
	if newKind != "Ingress" {
		t.Errorf("expected newKind 'Ingress', got %q", newKind)
	}
}

func TestMigrateAPIVersion_CurrentAPI_NoMigration(t *testing.T) {
	newAPI, newKind, migrated := MigrateAPIVersion("networking.k8s.io/v1", "Ingress")

	if migrated {
		t.Fatal("expected no migration for current API networking.k8s.io/v1 Ingress")
	}
	// Should return original values unchanged
	if newAPI != "networking.k8s.io/v1" {
		t.Errorf("expected original API returned, got %q", newAPI)
	}
	if newKind != "Ingress" {
		t.Errorf("expected original kind returned, got %q", newKind)
	}
}

func TestMigrateAPIVersion_UnknownAPI_NoMigration(t *testing.T) {
	newAPI, newKind, migrated := MigrateAPIVersion("apps/v1", "Deployment")

	if migrated {
		t.Fatal("expected no migration for unknown API apps/v1 Deployment")
	}
	if newAPI != "apps/v1" {
		t.Errorf("expected original API returned, got %q", newAPI)
	}
	if newKind != "Deployment" {
		t.Errorf("expected original kind returned, got %q", newKind)
	}
}

func TestMigrateAPIVersion_RemovedAPINoReplacement_ReturnsFalse(t *testing.T) {
	newAPI, newKind, migrated := MigrateAPIVersion("policy/v1beta1", "PodSecurityPolicy")

	// PodSecurityPolicy was removed with no direct replacement
	if migrated {
		t.Fatal("expected no migration (false) for removed PodSecurityPolicy")
	}
	if newAPI != "" {
		t.Errorf("expected empty newAPI for removed API, got %q", newAPI)
	}
	if newKind != "" {
		t.Errorf("expected empty newKind for removed API, got %q", newKind)
	}
}

func TestMigrateAPIVersion_BatchCronJob_MigratesCorrectly(t *testing.T) {
	newAPI, newKind, migrated := MigrateAPIVersion("batch/v1beta1", "CronJob")

	if !migrated {
		t.Fatal("expected migration to be available for batch/v1beta1 CronJob")
	}
	if newAPI != "batch/v1" {
		t.Errorf("expected newAPI 'batch/v1', got %q", newAPI)
	}
	if newKind != "CronJob" {
		t.Errorf("expected newKind 'CronJob', got %q", newKind)
	}
}

func TestMigrateAPIVersion_AutoscalingV2Beta2HPA_MigratesCorrectly(t *testing.T) {
	newAPI, newKind, migrated := MigrateAPIVersion("autoscaling/v2beta2", "HorizontalPodAutoscaler")

	if !migrated {
		t.Fatal("expected migration to be available for autoscaling/v2beta2 HPA")
	}
	if newAPI != "autoscaling/v2" {
		t.Errorf("expected newAPI 'autoscaling/v2', got %q", newAPI)
	}
	if newKind != "HorizontalPodAutoscaler" {
		t.Errorf("expected newKind 'HorizontalPodAutoscaler', got %q", newKind)
	}
}

func TestMigrateAPIVersion_AutoscalingV2Beta1HPA_MigratesCorrectly(t *testing.T) {
	newAPI, _, migrated := MigrateAPIVersion("autoscaling/v2beta1", "HorizontalPodAutoscaler")

	if !migrated {
		t.Fatal("expected migration to be available for autoscaling/v2beta1 HPA")
	}
	if newAPI != "autoscaling/v2" {
		t.Errorf("expected newAPI 'autoscaling/v2', got %q", newAPI)
	}
}

func TestMigrateAPIVersion_RBACv1beta1ClusterRole_MigratesCorrectly(t *testing.T) {
	newAPI, newKind, migrated := MigrateAPIVersion("rbac.authorization.k8s.io/v1beta1", "ClusterRole")

	if !migrated {
		t.Fatal("expected migration for rbac.authorization.k8s.io/v1beta1 ClusterRole")
	}
	if newAPI != "rbac.authorization.k8s.io/v1" {
		t.Errorf("expected 'rbac.authorization.k8s.io/v1', got %q", newAPI)
	}
	if newKind != "ClusterRole" {
		t.Errorf("expected 'ClusterRole', got %q", newKind)
	}
}

func TestMigrateAPIVersion_RBACv1beta1ClusterRoleBinding_MigratesCorrectly(t *testing.T) {
	newAPI, newKind, migrated := MigrateAPIVersion("rbac.authorization.k8s.io/v1beta1", "ClusterRoleBinding")

	if !migrated {
		t.Fatal("expected migration for rbac.authorization.k8s.io/v1beta1 ClusterRoleBinding")
	}
	if newAPI != "rbac.authorization.k8s.io/v1" {
		t.Errorf("expected 'rbac.authorization.k8s.io/v1', got %q", newAPI)
	}
	if newKind != "ClusterRoleBinding" {
		t.Errorf("expected 'ClusterRoleBinding', got %q", newKind)
	}
}

func TestMigrateAPIVersion_RBACv1beta1Role_MigratesCorrectly(t *testing.T) {
	newAPI, newKind, migrated := MigrateAPIVersion("rbac.authorization.k8s.io/v1beta1", "Role")

	if !migrated {
		t.Fatal("expected migration for rbac.authorization.k8s.io/v1beta1 Role")
	}
	if newAPI != "rbac.authorization.k8s.io/v1" {
		t.Errorf("expected 'rbac.authorization.k8s.io/v1', got %q", newAPI)
	}
	if newKind != "Role" {
		t.Errorf("expected 'Role', got %q", newKind)
	}
}

func TestMigrateAPIVersion_RBACv1beta1RoleBinding_MigratesCorrectly(t *testing.T) {
	newAPI, newKind, migrated := MigrateAPIVersion("rbac.authorization.k8s.io/v1beta1", "RoleBinding")

	if !migrated {
		t.Fatal("expected migration for rbac.authorization.k8s.io/v1beta1 RoleBinding")
	}
	if newAPI != "rbac.authorization.k8s.io/v1" {
		t.Errorf("expected 'rbac.authorization.k8s.io/v1', got %q", newAPI)
	}
	if newKind != "RoleBinding" {
		t.Errorf("expected 'RoleBinding', got %q", newKind)
	}
}

func TestMigrateAPIVersion_PolicyPDB_MigratesCorrectly(t *testing.T) {
	newAPI, newKind, migrated := MigrateAPIVersion("policy/v1beta1", "PodDisruptionBudget")

	if !migrated {
		t.Fatal("expected migration for policy/v1beta1 PodDisruptionBudget")
	}
	if newAPI != "policy/v1" {
		t.Errorf("expected 'policy/v1', got %q", newAPI)
	}
	if newKind != "PodDisruptionBudget" {
		t.Errorf("expected 'PodDisruptionBudget', got %q", newKind)
	}
}

func TestMigrateAPIVersion_ExtensionsNetworkPolicy_MigratesCorrectly(t *testing.T) {
	newAPI, newKind, migrated := MigrateAPIVersion("extensions/v1beta1", "NetworkPolicy")

	if !migrated {
		t.Fatal("expected migration for extensions/v1beta1 NetworkPolicy")
	}
	if newAPI != "networking.k8s.io/v1" {
		t.Errorf("expected 'networking.k8s.io/v1', got %q", newAPI)
	}
	if newKind != "NetworkPolicy" {
		t.Errorf("expected 'NetworkPolicy', got %q", newKind)
	}
}

// ============================================================
// GetMigrationInfo Tests
// ============================================================

func TestGetMigrationInfo_ReturnsFullDetails(t *testing.T) {
	info := GetMigrationInfo("extensions/v1beta1", "Ingress")

	if info == nil {
		t.Fatal("expected migration info, got nil")
	}
	if info.OldAPIVersion != "extensions/v1beta1" {
		t.Errorf("expected OldAPIVersion 'extensions/v1beta1', got %q", info.OldAPIVersion)
	}
	if info.OldKind != "Ingress" {
		t.Errorf("expected OldKind 'Ingress', got %q", info.OldKind)
	}
	if info.NewAPIVersion != "networking.k8s.io/v1" {
		t.Errorf("expected NewAPIVersion 'networking.k8s.io/v1', got %q", info.NewAPIVersion)
	}
	if info.NewKind != "Ingress" {
		t.Errorf("expected NewKind 'Ingress', got %q", info.NewKind)
	}
	if info.DeprecatedIn == "" {
		t.Error("expected DeprecatedIn to be populated")
	}
	if info.RemovedIn == "" {
		t.Error("expected RemovedIn to be populated")
	}
	if info.Notes == "" {
		t.Error("expected Notes to be populated for Ingress migration")
	}
}

func TestGetMigrationInfo_ReturnsNilForUnknown(t *testing.T) {
	info := GetMigrationInfo("apps/v1", "Deployment")

	if info != nil {
		t.Errorf("expected nil for unknown API, got %+v", info)
	}
}

func TestGetMigrationInfo_ReturnsNilForCurrentAPI(t *testing.T) {
	info := GetMigrationInfo("networking.k8s.io/v1", "Ingress")

	if info != nil {
		t.Errorf("expected nil for current API, got %+v", info)
	}
}

func TestGetMigrationInfo_PodSecurityPolicy_HasNotes(t *testing.T) {
	info := GetMigrationInfo("policy/v1beta1", "PodSecurityPolicy")

	if info == nil {
		t.Fatal("expected migration info for PodSecurityPolicy, got nil")
	}
	if info.NewAPIVersion != "" {
		t.Errorf("expected empty NewAPIVersion for removed PSP, got %q", info.NewAPIVersion)
	}
	if info.Notes == "" {
		t.Error("expected Notes to contain PSS migration guidance")
	}
}

func TestGetMigrationInfo_CronJob_DeprecationVersionsPopulated(t *testing.T) {
	info := GetMigrationInfo("batch/v1beta1", "CronJob")

	if info == nil {
		t.Fatal("expected migration info for CronJob, got nil")
	}
	if info.DeprecatedIn != "1.21" {
		t.Errorf("expected DeprecatedIn '1.21', got %q", info.DeprecatedIn)
	}
	if info.RemovedIn != "1.25" {
		t.Errorf("expected RemovedIn '1.25', got %q", info.RemovedIn)
	}
}

// ============================================================
// ListDeprecatedAPIs Tests
// ============================================================

func TestListDeprecatedAPIs_ReturnsAllEntries(t *testing.T) {
	list := ListDeprecatedAPIs()

	// We have 12 entries in the migration table
	if len(list) != 12 {
		t.Errorf("expected 12 deprecated API entries, got %d", len(list))
	}
}

func TestListDeprecatedAPIs_ReturnsCopy(t *testing.T) {
	list1 := ListDeprecatedAPIs()
	list2 := ListDeprecatedAPIs()

	// Modifying one should not affect the other
	list1[0].OldAPIVersion = "modified"

	if list2[0].OldAPIVersion == "modified" {
		t.Error("ListDeprecatedAPIs should return an independent copy")
	}
}

func TestListDeprecatedAPIs_ContainsAllExpectedAPIs(t *testing.T) {
	list := ListDeprecatedAPIs()

	type apiKey struct {
		api  string
		kind string
	}
	expected := []apiKey{
		{"extensions/v1beta1", "Ingress"},
		{"networking.k8s.io/v1beta1", "Ingress"},
		{"extensions/v1beta1", "NetworkPolicy"},
		{"policy/v1beta1", "PodDisruptionBudget"},
		{"policy/v1beta1", "PodSecurityPolicy"},
		{"rbac.authorization.k8s.io/v1beta1", "ClusterRole"},
		{"rbac.authorization.k8s.io/v1beta1", "ClusterRoleBinding"},
		{"rbac.authorization.k8s.io/v1beta1", "Role"},
		{"rbac.authorization.k8s.io/v1beta1", "RoleBinding"},
		{"autoscaling/v2beta1", "HorizontalPodAutoscaler"},
		{"autoscaling/v2beta2", "HorizontalPodAutoscaler"},
		{"batch/v1beta1", "CronJob"},
	}

	found := make(map[apiKey]bool)
	for _, m := range list {
		found[apiKey{m.OldAPIVersion, m.OldKind}] = true
	}

	for _, exp := range expected {
		if !found[exp] {
			t.Errorf("expected API %q / %q not found in list", exp.api, exp.kind)
		}
	}
}
