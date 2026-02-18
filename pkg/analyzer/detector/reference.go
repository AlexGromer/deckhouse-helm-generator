package detector

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// NameReferenceDetector detects relationships based on direct name references.
// Example: Ingress referencing Service by name.
type NameReferenceDetector struct {
	priority int
}

// NewNameReferenceDetector creates a new name reference detector.
func NewNameReferenceDetector() *NameReferenceDetector {
	return &NameReferenceDetector{
		priority: 90,
	}
}

// Name returns the detector name.
func (d *NameReferenceDetector) Name() string {
	return "name_reference"
}

// Priority returns the detector priority.
func (d *NameReferenceDetector) Priority() int {
	return d.priority
}

// Detect detects name reference relationships.
func (d *NameReferenceDetector) Detect(ctx context.Context, resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	kind := obj.GetKind()

	switch kind {
	case "Ingress":
		relationships = append(relationships, d.detectIngressToService(resource, allResources)...)
	case "StatefulSet":
		relationships = append(relationships, d.detectStatefulSetToService(resource, allResources)...)
	case "RoleBinding":
		relationships = append(relationships, d.detectRoleBindingReferences(resource, allResources)...)
	case "ClusterRoleBinding":
		relationships = append(relationships, d.detectClusterRoleBindingReferences(resource, allResources)...)
	case "PersistentVolumeClaim":
		relationships = append(relationships, d.detectPVCToStorageClass(resource, allResources)...)
	}

	// Common: ServiceAccount references
	relationships = append(relationships, d.detectServiceAccountReferences(resource, allResources)...)

	// Common: ImagePullSecrets references
	relationships = append(relationships, d.detectImagePullSecretReferences(resource, allResources)...)

	return relationships
}

// detectIngressToService detects Ingress -> Service relationships.
func (d *NameReferenceDetector) detectIngressToService(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	namespace := obj.GetNamespace()

	// Parse Ingress rules
	rules, found, _ := unstructured.NestedSlice(obj.Object, "spec", "rules")
	if !found {
		return relationships
	}

	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		http, found, _ := unstructured.NestedMap(ruleMap, "http")
		if !found {
			continue
		}

		paths, found, _ := unstructured.NestedSlice(http, "paths")
		if !found {
			continue
		}

		for _, path := range paths {
			pathMap, ok := path.(map[string]interface{})
			if !ok {
				continue
			}

			serviceName, found, _ := unstructured.NestedString(pathMap, "backend", "service", "name")
			if !found || serviceName == "" {
				continue
			}

			// Look for the service
			targetKey := types.ResourceKey{
				GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Service"},
				Namespace: namespace,
				Name:      serviceName,
			}

			if _, exists := allResources[targetKey]; exists {
				relationships = append(relationships, types.Relationship{
					From: resource.Original.ResourceKey(),
					To:   targetKey,
					Type: types.RelationNameReference,
					Field: "spec.rules[].http.paths[].backend.service.name",
					Details: map[string]string{
						"serviceName": serviceName,
					},
				})
			}
		}
	}

	// Check TLS secrets
	tls, found, _ := unstructured.NestedSlice(obj.Object, "spec", "tls")
	if found {
		for _, tlsEntry := range tls {
			tlsMap, ok := tlsEntry.(map[string]interface{})
			if !ok {
				continue
			}

			secretName, found, _ := unstructured.NestedString(tlsMap, "secretName")
			if !found || secretName == "" {
				continue
			}

			targetKey := types.ResourceKey{
				GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Secret"},
				Namespace: namespace,
				Name:      secretName,
			}

			if _, exists := allResources[targetKey]; exists {
				relationships = append(relationships, types.Relationship{
					From: resource.Original.ResourceKey(),
					To:   targetKey,
					Type: types.RelationNameReference,
					Field: "spec.tls[].secretName",
					Details: map[string]string{
						"secretName": secretName,
					},
				})
			}
		}
	}

	return relationships
}

// detectStatefulSetToService detects StatefulSet -> Service relationship (serviceName field).
func (d *NameReferenceDetector) detectStatefulSetToService(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	namespace := obj.GetNamespace()

	serviceName, found, _ := unstructured.NestedString(obj.Object, "spec", "serviceName")
	if !found || serviceName == "" {
		return relationships
	}

	targetKey := types.ResourceKey{
		GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Service"},
		Namespace: namespace,
		Name:      serviceName,
	}

	if _, exists := allResources[targetKey]; exists {
		relationships = append(relationships, types.Relationship{
			From: resource.Original.ResourceKey(),
			To:   targetKey,
			Type: types.RelationNameReference,
			Field: "spec.serviceName",
			Details: map[string]string{
				"serviceName": serviceName,
			},
		})
	}

	return relationships
}

// detectRoleBindingReferences detects RoleBinding -> Role and ServiceAccount relationships.
func (d *NameReferenceDetector) detectRoleBindingReferences(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	namespace := obj.GetNamespace()

	// RoleRef
	roleRefKind, _, _ := unstructured.NestedString(obj.Object, "roleRef", "kind")
	roleRefName, _, _ := unstructured.NestedString(obj.Object, "roleRef", "name")

	if roleRefName != "" {
		var targetGVK schema.GroupVersionKind
		if roleRefKind == "Role" {
			targetGVK = schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"}
		} else if roleRefKind == "ClusterRole" {
			targetGVK = schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"}
		}

		if targetGVK.Kind != "" {
			targetKey := types.ResourceKey{
				GVK:       targetGVK,
				Namespace: namespace,
				Name:      roleRefName,
			}
			if targetGVK.Kind == "ClusterRole" {
				targetKey.Namespace = ""
			}

			if _, exists := allResources[targetKey]; exists {
				relationships = append(relationships, types.Relationship{
					From: resource.Original.ResourceKey(),
					To:   targetKey,
					Type: types.RelationRoleBinding,
					Field: "roleRef",
					Details: map[string]string{
						"roleKind": roleRefKind,
						"roleName": roleRefName,
					},
				})
			}
		}
	}

	// Subjects (ServiceAccounts)
	subjects, found, _ := unstructured.NestedSlice(obj.Object, "subjects")
	if found {
		for _, subject := range subjects {
			subjectMap, ok := subject.(map[string]interface{})
			if !ok {
				continue
			}

			kind, _ := subjectMap["kind"].(string)
			name, _ := subjectMap["name"].(string)
			subjectNS, _ := subjectMap["namespace"].(string)

			if kind == "ServiceAccount" && name != "" {
				if subjectNS == "" {
					subjectNS = namespace
				}

				targetKey := types.ResourceKey{
					GVK:       schema.GroupVersionKind{Version: "v1", Kind: "ServiceAccount"},
					Namespace: subjectNS,
					Name:      name,
				}

				if _, exists := allResources[targetKey]; exists {
					relationships = append(relationships, types.Relationship{
						From: resource.Original.ResourceKey(),
						To:   targetKey,
						Type: types.RelationRoleBinding,
						Field: "subjects[]",
						Details: map[string]string{
							"subjectKind": kind,
							"subjectName": name,
						},
					})
				}
			}
		}
	}

	return relationships
}

// detectClusterRoleBindingReferences detects ClusterRoleBinding references.
func (d *NameReferenceDetector) detectClusterRoleBindingReferences(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	// Similar to RoleBinding but for cluster-scoped resources
	return d.detectRoleBindingReferences(resource, allResources)
}

// detectServiceAccountReferences detects ServiceAccount references in workloads.
func (d *NameReferenceDetector) detectServiceAccountReferences(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	kind := obj.GetKind()
	namespace := obj.GetNamespace()

	// Check if this is a workload resource
	workloadKinds := map[string]bool{
		"Deployment":  true,
		"StatefulSet": true,
		"DaemonSet":   true,
		"Job":         true,
		"CronJob":     true,
		"Pod":         true,
	}

	if !workloadKinds[kind] {
		return relationships
	}

	var saName string
	var found bool

	if kind == "CronJob" {
		saName, found, _ = unstructured.NestedString(obj.Object, "spec", "jobTemplate", "spec", "template", "spec", "serviceAccountName")
	} else if kind == "Pod" {
		saName, found, _ = unstructured.NestedString(obj.Object, "spec", "serviceAccountName")
	} else {
		saName, found, _ = unstructured.NestedString(obj.Object, "spec", "template", "spec", "serviceAccountName")
	}

	if !found || saName == "" {
		return relationships
	}

	targetKey := types.ResourceKey{
		GVK:       schema.GroupVersionKind{Version: "v1", Kind: "ServiceAccount"},
		Namespace: namespace,
		Name:      saName,
	}

	if _, exists := allResources[targetKey]; exists {
		relationships = append(relationships, types.Relationship{
			From: resource.Original.ResourceKey(),
			To:   targetKey,
			Type: types.RelationServiceAccount,
			Field: "spec.template.spec.serviceAccountName",
			Details: map[string]string{
				"serviceAccountName": saName,
			},
		})
	}

	return relationships
}

// detectImagePullSecretReferences detects imagePullSecrets references.
func (d *NameReferenceDetector) detectImagePullSecretReferences(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object
	kind := obj.GetKind()
	namespace := obj.GetNamespace()

	workloadKinds := map[string]bool{
		"Deployment":  true,
		"StatefulSet": true,
		"DaemonSet":   true,
		"Job":         true,
		"CronJob":     true,
		"Pod":         true,
	}

	if !workloadKinds[kind] {
		return relationships
	}

	var secrets []interface{}
	var found bool

	if kind == "CronJob" {
		secrets, found, _ = unstructured.NestedSlice(obj.Object, "spec", "jobTemplate", "spec", "template", "spec", "imagePullSecrets")
	} else if kind == "Pod" {
		secrets, found, _ = unstructured.NestedSlice(obj.Object, "spec", "imagePullSecrets")
	} else {
		secrets, found, _ = unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "imagePullSecrets")
	}

	if !found {
		return relationships
	}

	for _, secret := range secrets {
		secretMap, ok := secret.(map[string]interface{})
		if !ok {
			continue
		}

		secretName, ok := secretMap["name"].(string)
		if !ok || secretName == "" {
			continue
		}

		targetKey := types.ResourceKey{
			GVK:       schema.GroupVersionKind{Version: "v1", Kind: "Secret"},
			Namespace: namespace,
			Name:      secretName,
		}

		if _, exists := allResources[targetKey]; exists {
			relationships = append(relationships, types.Relationship{
				From: resource.Original.ResourceKey(),
				To:   targetKey,
				Type: types.RelationImagePullSecret,
				Field: "spec.template.spec.imagePullSecrets",
				Details: map[string]string{
					"secretName": secretName,
				},
			})
		}
	}

	return relationships
}

// detectPVCToStorageClass detects PVC -> StorageClass relationships via spec.storageClassName.
func (d *NameReferenceDetector) detectPVCToStorageClass(resource *types.ProcessedResource, allResources map[types.ResourceKey]*types.ProcessedResource) []types.Relationship {
	var relationships []types.Relationship

	obj := resource.Original.Object

	storageClassName, found, _ := unstructured.NestedString(obj.Object, "spec", "storageClassName")
	if !found || storageClassName == "" {
		return relationships
	}

	targetKey := types.ResourceKey{
		GVK:  schema.GroupVersionKind{Group: "storage.k8s.io", Version: "v1", Kind: "StorageClass"},
		Name: storageClassName,
	}

	if _, exists := allResources[targetKey]; exists {
		relationships = append(relationships, types.Relationship{
			From:  resource.Original.ResourceKey(),
			To:    targetKey,
			Type:  types.RelationStorageClass,
			Field: "spec.storageClassName",
			Details: map[string]string{
				"storageClassName": storageClassName,
			},
		})
	}

	return relationships
}
