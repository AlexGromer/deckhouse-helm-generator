package generator

// APIMigration represents a migration from deprecated to current API version.
type APIMigration struct {
	// OldAPIVersion is the deprecated API version (e.g., "extensions/v1beta1")
	OldAPIVersion string
	// OldKind is the resource kind (may change during migration)
	OldKind string
	// NewAPIVersion is the replacement API version
	NewAPIVersion string
	// NewKind is the replacement kind (usually same as OldKind)
	NewKind string
	// DeprecatedIn is the Kubernetes version where this API was deprecated
	DeprecatedIn string
	// RemovedIn is the Kubernetes version where this API was removed
	RemovedIn string
	// Notes contains migration-specific notes
	Notes string
}

// apiMigrations is the migration table for known deprecated APIs.
var apiMigrations = []APIMigration{
	{
		OldAPIVersion: "extensions/v1beta1", OldKind: "Ingress",
		NewAPIVersion: "networking.k8s.io/v1", NewKind: "Ingress",
		DeprecatedIn: "1.14", RemovedIn: "1.22",
		Notes: "spec.rules[].http.paths[].pathType is now required",
	},
	{
		OldAPIVersion: "networking.k8s.io/v1beta1", OldKind: "Ingress",
		NewAPIVersion: "networking.k8s.io/v1", NewKind: "Ingress",
		DeprecatedIn: "1.19", RemovedIn: "1.22",
		Notes: "spec.rules[].http.paths[].pathType is now required",
	},
	{
		OldAPIVersion: "extensions/v1beta1", OldKind: "NetworkPolicy",
		NewAPIVersion: "networking.k8s.io/v1", NewKind: "NetworkPolicy",
		DeprecatedIn: "1.9", RemovedIn: "1.16",
	},
	{
		OldAPIVersion: "policy/v1beta1", OldKind: "PodDisruptionBudget",
		NewAPIVersion: "policy/v1", NewKind: "PodDisruptionBudget",
		DeprecatedIn: "1.21", RemovedIn: "1.25",
	},
	{
		OldAPIVersion: "policy/v1beta1", OldKind: "PodSecurityPolicy",
		NewAPIVersion: "", NewKind: "", // Removed, no direct replacement
		DeprecatedIn: "1.21", RemovedIn: "1.25",
		Notes: "Use Pod Security Standards (PSS) instead",
	},
	{
		OldAPIVersion: "rbac.authorization.k8s.io/v1beta1", OldKind: "ClusterRole",
		NewAPIVersion: "rbac.authorization.k8s.io/v1", NewKind: "ClusterRole",
		DeprecatedIn: "1.17", RemovedIn: "1.22",
	},
	{
		OldAPIVersion: "rbac.authorization.k8s.io/v1beta1", OldKind: "ClusterRoleBinding",
		NewAPIVersion: "rbac.authorization.k8s.io/v1", NewKind: "ClusterRoleBinding",
		DeprecatedIn: "1.17", RemovedIn: "1.22",
	},
	{
		OldAPIVersion: "rbac.authorization.k8s.io/v1beta1", OldKind: "Role",
		NewAPIVersion: "rbac.authorization.k8s.io/v1", NewKind: "Role",
		DeprecatedIn: "1.17", RemovedIn: "1.22",
	},
	{
		OldAPIVersion: "rbac.authorization.k8s.io/v1beta1", OldKind: "RoleBinding",
		NewAPIVersion: "rbac.authorization.k8s.io/v1", NewKind: "RoleBinding",
		DeprecatedIn: "1.17", RemovedIn: "1.22",
	},
	{
		OldAPIVersion: "autoscaling/v2beta1", OldKind: "HorizontalPodAutoscaler",
		NewAPIVersion: "autoscaling/v2", NewKind: "HorizontalPodAutoscaler",
		DeprecatedIn: "1.23", RemovedIn: "1.26",
	},
	{
		OldAPIVersion: "autoscaling/v2beta2", OldKind: "HorizontalPodAutoscaler",
		NewAPIVersion: "autoscaling/v2", NewKind: "HorizontalPodAutoscaler",
		DeprecatedIn: "1.23", RemovedIn: "1.26",
	},
	{
		OldAPIVersion: "batch/v1beta1", OldKind: "CronJob",
		NewAPIVersion: "batch/v1", NewKind: "CronJob",
		DeprecatedIn: "1.21", RemovedIn: "1.25",
	},
}

// MigrateAPIVersion checks if the given apiVersion+kind is deprecated and returns
// the new API version and kind if migration is available.
// Returns (newAPI, newKind, migrated bool).
func MigrateAPIVersion(apiVersion, kind string) (string, string, bool) {
	for _, m := range apiMigrations {
		if m.OldAPIVersion == apiVersion && m.OldKind == kind {
			if m.NewAPIVersion == "" {
				// Removed API with no replacement
				return "", "", false
			}
			return m.NewAPIVersion, m.NewKind, true
		}
	}
	return apiVersion, kind, false
}

// GetMigrationInfo returns full migration details for a deprecated API.
// Returns nil if no migration exists.
func GetMigrationInfo(apiVersion, kind string) *APIMigration {
	for _, m := range apiMigrations {
		if m.OldAPIVersion == apiVersion && m.OldKind == kind {
			return &m
		}
	}
	return nil
}

// ListDeprecatedAPIs returns all known deprecated API migrations.
func ListDeprecatedAPIs() []APIMigration {
	result := make([]APIMigration, len(apiMigrations))
	copy(result, apiMigrations)
	return result
}
