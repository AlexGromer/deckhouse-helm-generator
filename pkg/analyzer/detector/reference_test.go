package detector

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// makeProcessedResourceExtra creates a ProcessedResource where arbitrary top-level
// fields (beyond "spec") can be injected via extraTopLevel.
// This is necessary for resources like RoleBinding whose roleRef and subjects fields
// live at the object root rather than under spec.
func makeProcessedResourceExtra(apiVersion, kind, name, namespace string, topLevel map[string]interface{}) *types.ProcessedResource {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
		},
	}

	for k, v := range topLevel {
		obj.Object[k] = v
	}

	gv, _ := schema.ParseGroupVersion(apiVersion)
	gvk := gv.WithKind(kind)

	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    gvk,
		},
	}
}

// TestNewNameReferenceDetector verifies the constructor, Name(), and Priority().
func TestNewNameReferenceDetector(t *testing.T) {
	d := NewNameReferenceDetector()

	if d == nil {
		t.Fatal("NewNameReferenceDetector() returned nil")
	}

	if got := d.Name(); got != "name_reference" {
		t.Errorf("Name() = %q, want %q", got, "name_reference")
	}

	if got := d.Priority(); got != 90 {
		t.Errorf("Priority() = %d, want 90", got)
	}
}

// TestReferenceDetector_IngressToService verifies that an Ingress with a backend
// service name produces a name_reference relationship when the Service exists.
func TestReferenceDetector_IngressToService(t *testing.T) {
	const (
		ns          = "default"
		serviceName = "my-service"
	)

	ingress := makeProcessedResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", ns,
		nil, nil,
		map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"host": "example.com",
					"http": map[string]interface{}{
						"paths": []interface{}{
							map[string]interface{}{
								"path":     "/",
								"pathType": "Prefix",
								"backend": map[string]interface{}{
									"service": map[string]interface{}{
										"name": serviceName,
										"port": map[string]interface{}{
											"number": int64(80),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	)

	svc := makeProcessedResource("v1", "Service", serviceName, ns, nil, nil, nil)

	allResources := buildAllResources(ingress, svc)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	svcKey := svc.Original.ResourceKey()
	found := false
	for _, rel := range rels {
		if rel.Type == types.RelationNameReference &&
			rel.To == svcKey &&
			rel.Field == "spec.rules[].http.paths[].backend.service.name" &&
			rel.Details["serviceName"] == serviceName {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected name_reference relationship to Service %q, got: %v", serviceName, rels)
	}
}

// TestReferenceDetector_IngressTLSSecret verifies that an Ingress with a TLS secretName
// produces a name_reference relationship when the Secret exists.
func TestReferenceDetector_IngressTLSSecret(t *testing.T) {
	const (
		ns         = "default"
		secretName = "tls-secret"
	)

	ingress := makeProcessedResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", ns,
		nil, nil,
		map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"host": "example.com",
					"http": map[string]interface{}{
						"paths": []interface{}{
							map[string]interface{}{
								"path":     "/",
								"pathType": "Prefix",
								"backend": map[string]interface{}{
									"service": map[string]interface{}{
										"name": "nonexistent-service",
										"port": map[string]interface{}{
											"number": int64(80),
										},
									},
								},
							},
						},
					},
				},
			},
			"tls": []interface{}{
				map[string]interface{}{
					"hosts":      []interface{}{"example.com"},
					"secretName": secretName,
				},
			},
		},
	)

	secret := makeProcessedResource("v1", "Secret", secretName, ns, nil, nil, nil)

	allResources := buildAllResources(ingress, secret)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	secretKey := secret.Original.ResourceKey()
	found := false
	for _, rel := range rels {
		if rel.Type == types.RelationNameReference &&
			rel.To == secretKey &&
			rel.Field == "spec.tls[].secretName" &&
			rel.Details["secretName"] == secretName {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected name_reference relationship to Secret %q, got: %v", secretName, rels)
	}
}

// TestReferenceDetector_IngressMissingService verifies that no relationship is created
// when the referenced Service does not exist in allResources.
func TestReferenceDetector_IngressMissingService(t *testing.T) {
	const (
		ns          = "default"
		serviceName = "does-not-exist"
	)

	ingress := makeProcessedResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", ns,
		nil, nil,
		map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"host": "example.com",
					"http": map[string]interface{}{
						"paths": []interface{}{
							map[string]interface{}{
								"path":     "/",
								"pathType": "Prefix",
								"backend": map[string]interface{}{
									"service": map[string]interface{}{
										"name": serviceName,
										"port": map[string]interface{}{
											"number": int64(80),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	)

	// allResources contains only the Ingress itself — the referenced Service is absent.
	allResources := buildAllResources(ingress)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationNameReference {
			t.Errorf("expected no name_reference relationship for missing Service, but got: %v", rel)
		}
	}
}

// TestReferenceDetector_StatefulSetToService verifies that a StatefulSet with
// spec.serviceName produces a name_reference relationship when the Service exists.
func TestReferenceDetector_StatefulSetToService(t *testing.T) {
	const (
		ns          = "default"
		serviceName = "headless-svc"
	)

	sts := makeProcessedResource(
		"apps/v1", "StatefulSet", "my-sts", ns,
		nil, nil,
		map[string]interface{}{
			"serviceName": serviceName,
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": "my-sts",
				},
			},
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app": "my-sts",
					},
				},
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "main",
							"image": "nginx:latest",
						},
					},
				},
			},
		},
	)

	svc := makeProcessedResource("v1", "Service", serviceName, ns, nil, nil, nil)

	allResources := buildAllResources(sts, svc)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), sts, allResources)

	svcKey := svc.Original.ResourceKey()
	found := false
	for _, rel := range rels {
		if rel.Type == types.RelationNameReference &&
			rel.To == svcKey &&
			rel.Field == "spec.serviceName" &&
			rel.Details["serviceName"] == serviceName {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected name_reference relationship to Service %q, got: %v", serviceName, rels)
	}
}

// TestReferenceDetector_DeploymentToServiceAccount verifies that a Deployment with
// spec.template.spec.serviceAccountName produces a service_account relationship
// when the ServiceAccount exists.
func TestReferenceDetector_DeploymentToServiceAccount(t *testing.T) {
	const (
		ns     = "default"
		saName = "my-sa"
	)

	deploy := makeProcessedResource(
		"apps/v1", "Deployment", "my-deploy", ns,
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"serviceAccountName": saName,
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "main",
							"image": "nginx:latest",
						},
					},
				},
			},
		},
	)

	sa := makeProcessedResource("v1", "ServiceAccount", saName, ns, nil, nil, nil)

	allResources := buildAllResources(deploy, sa)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), deploy, allResources)

	saKey := sa.Original.ResourceKey()
	found := false
	for _, rel := range rels {
		if rel.Type == types.RelationServiceAccount &&
			rel.To == saKey &&
			rel.Details["serviceAccountName"] == saName {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected service_account relationship to ServiceAccount %q, got: %v", saName, rels)
	}
}

// TestReferenceDetector_NonWorkload verifies that a non-workload resource (ConfigMap)
// does not produce any service_account or image_pull_secret relationships.
func TestReferenceDetector_NonWorkload(t *testing.T) {
	const ns = "default"

	cm := makeProcessedResource(
		"v1", "ConfigMap", "my-config", ns,
		nil, nil,
		nil,
	)

	// Add a ServiceAccount to allResources to ensure it is not accidentally referenced.
	sa := makeProcessedResource("v1", "ServiceAccount", "some-sa", ns, nil, nil, nil)

	allResources := buildAllResources(cm, sa)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), cm, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationServiceAccount {
			t.Errorf("expected no service_account relationship for ConfigMap, but got: %v", rel)
		}
		if rel.Type == types.RelationImagePullSecret {
			t.Errorf("expected no image_pull_secret relationship for ConfigMap, but got: %v", rel)
		}
	}
}

// TestReferenceDetector_ImagePullSecrets verifies that a Deployment with imagePullSecrets
// produces image_pull_secret relationships for each existing Secret.
func TestReferenceDetector_ImagePullSecrets(t *testing.T) {
	const (
		ns          = "default"
		secretName1 = "registry-secret"
		secretName2 = "another-registry-secret"
	)

	deploy := makeProcessedResource(
		"apps/v1", "Deployment", "my-deploy", ns,
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"imagePullSecrets": []interface{}{
						map[string]interface{}{"name": secretName1},
						map[string]interface{}{"name": secretName2},
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "main",
							"image": "private-registry/app:latest",
						},
					},
				},
			},
		},
	)

	secret1 := makeProcessedResource("v1", "Secret", secretName1, ns, nil, nil, nil)
	secret2 := makeProcessedResource("v1", "Secret", secretName2, ns, nil, nil, nil)

	allResources := buildAllResources(deploy, secret1, secret2)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), deploy, allResources)

	secret1Key := secret1.Original.ResourceKey()
	secret2Key := secret2.Original.ResourceKey()

	foundSecret1 := false
	foundSecret2 := false

	for _, rel := range rels {
		if rel.Type == types.RelationImagePullSecret {
			switch rel.To {
			case secret1Key:
				if rel.Details["secretName"] == secretName1 {
					foundSecret1 = true
				}
			case secret2Key:
				if rel.Details["secretName"] == secretName2 {
					foundSecret2 = true
				}
			}
		}
	}

	if !foundSecret1 {
		t.Errorf("expected image_pull_secret relationship to Secret %q, got: %v", secretName1, rels)
	}
	if !foundSecret2 {
		t.Errorf("expected image_pull_secret relationship to Secret %q, got: %v", secretName2, rels)
	}
}

// TestReferenceDetector_RoleBindingReferences verifies that a RoleBinding referencing
// a Role and a ServiceAccount subject produces the expected role_binding relationships.
func TestReferenceDetector_RoleBindingReferences(t *testing.T) {
	const (
		ns       = "default"
		roleName = "my-role"
		saName   = "my-sa"
	)

	// RoleBinding has top-level "roleRef" and "subjects" fields, not under "spec".
	rb := makeProcessedResourceExtra(
		"rbac.authorization.k8s.io/v1", "RoleBinding", "my-rolebinding", ns,
		map[string]interface{}{
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     roleName,
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      saName,
					"namespace": ns,
				},
			},
		},
	)

	role := makeProcessedResourceExtra(
		"rbac.authorization.k8s.io/v1", "Role", roleName, ns, nil,
	)

	sa := makeProcessedResource("v1", "ServiceAccount", saName, ns, nil, nil, nil)

	allResources := buildAllResources(rb, role, sa)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), rb, allResources)

	roleKey := role.Original.ResourceKey()
	saKey := sa.Original.ResourceKey()

	foundRoleRef := false
	foundSubject := false

	for _, rel := range rels {
		if rel.Type == types.RelationRoleBinding {
			switch rel.To {
			case roleKey:
				if rel.Field == "roleRef" &&
					rel.Details["roleKind"] == "Role" &&
					rel.Details["roleName"] == roleName {
					foundRoleRef = true
				}
			case saKey:
				if rel.Field == "subjects[]" &&
					rel.Details["subjectKind"] == "ServiceAccount" &&
					rel.Details["subjectName"] == saName {
					foundSubject = true
				}
			}
		}
	}

	if !foundRoleRef {
		t.Errorf("expected role_binding relationship to Role %q via roleRef, got: %v", roleName, rels)
	}
	if !foundSubject {
		t.Errorf("expected role_binding relationship to ServiceAccount %q via subjects[], got: %v", saName, rels)
	}
}

// TestReferenceDetector_RoleBindingMissingTargets verifies that a RoleBinding produces
// no relationships when neither the referenced Role nor the ServiceAccount subject exist.
func TestReferenceDetector_RoleBindingMissingTargets(t *testing.T) {
	const (
		ns       = "default"
		roleName = "missing-role"
		saName   = "missing-sa"
	)

	rb := makeProcessedResourceExtra(
		"rbac.authorization.k8s.io/v1", "RoleBinding", "my-rolebinding", ns,
		map[string]interface{}{
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     roleName,
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      saName,
					"namespace": ns,
				},
			},
		},
	)

	// allResources contains only the RoleBinding — targets don't exist.
	allResources := buildAllResources(rb)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), rb, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationRoleBinding {
			t.Errorf("expected no role_binding relationships for missing targets, but got: %v", rel)
		}
	}
}

// TestReferenceDetector_ClusterRoleBindingToClusterRole verifies that a ClusterRoleBinding
// referencing a ClusterRole produces a role_binding relationship (cluster-scoped, empty namespace).
func TestReferenceDetector_ClusterRoleBindingToClusterRole(t *testing.T) {
	const (
		clusterRoleName = "my-cluster-role"
		saName          = "my-sa"
		saNamespace     = "default"
	)

	crb := makeProcessedResourceExtra(
		"rbac.authorization.k8s.io/v1", "ClusterRoleBinding", "my-crb", "",
		map[string]interface{}{
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "ClusterRole",
				"name":     clusterRoleName,
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      saName,
					"namespace": saNamespace,
				},
			},
		},
	)

	// ClusterRole is cluster-scoped (no namespace).
	clusterRole := makeProcessedResourceExtra(
		"rbac.authorization.k8s.io/v1", "ClusterRole", clusterRoleName, "", nil,
	)

	sa := makeProcessedResource("v1", "ServiceAccount", saName, saNamespace, nil, nil, nil)

	allResources := buildAllResources(crb, clusterRole, sa)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), crb, allResources)

	clusterRoleKey := clusterRole.Original.ResourceKey()
	saKey := sa.Original.ResourceKey()

	foundClusterRole := false
	foundSA := false

	for _, rel := range rels {
		if rel.Type == types.RelationRoleBinding {
			switch rel.To {
			case clusterRoleKey:
				if rel.Field == "roleRef" &&
					rel.Details["roleKind"] == "ClusterRole" &&
					rel.Details["roleName"] == clusterRoleName {
					foundClusterRole = true
				}
			case saKey:
				if rel.Field == "subjects[]" &&
					rel.Details["subjectKind"] == "ServiceAccount" &&
					rel.Details["subjectName"] == saName {
					foundSA = true
				}
			}
		}
	}

	if !foundClusterRole {
		t.Errorf("expected role_binding relationship to ClusterRole %q, got: %v", clusterRoleName, rels)
	}
	if !foundSA {
		t.Errorf("expected role_binding relationship to ServiceAccount %q via subjects[], got: %v", saName, rels)
	}
}

// TestReferenceDetector_IngressBothServiceAndTLS verifies that an Ingress produces both
// a service name_reference and a TLS secret name_reference in the same Detect call.
func TestReferenceDetector_IngressBothServiceAndTLS(t *testing.T) {
	const (
		ns          = "default"
		serviceName = "web-service"
		secretName  = "tls-cert"
	)

	ingress := makeProcessedResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", ns,
		nil, nil,
		map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"host": "example.com",
					"http": map[string]interface{}{
						"paths": []interface{}{
							map[string]interface{}{
								"path":     "/",
								"pathType": "Prefix",
								"backend": map[string]interface{}{
									"service": map[string]interface{}{
										"name": serviceName,
										"port": map[string]interface{}{
											"number": int64(443),
										},
									},
								},
							},
						},
					},
				},
			},
			"tls": []interface{}{
				map[string]interface{}{
					"hosts":      []interface{}{"example.com"},
					"secretName": secretName,
				},
			},
		},
	)

	svc := makeProcessedResource("v1", "Service", serviceName, ns, nil, nil, nil)
	secret := makeProcessedResource("v1", "Secret", secretName, ns, nil, nil, nil)

	allResources := buildAllResources(ingress, svc, secret)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	svcKey := svc.Original.ResourceKey()
	secretKey := secret.Original.ResourceKey()

	foundService := false
	foundSecret := false

	for _, rel := range rels {
		if rel.Type == types.RelationNameReference {
			switch rel.To {
			case svcKey:
				foundService = true
			case secretKey:
				foundSecret = true
			}
		}
	}

	if !foundService {
		t.Errorf("expected name_reference relationship to Service %q, got: %v", serviceName, rels)
	}
	if !foundSecret {
		t.Errorf("expected name_reference relationship to Secret %q, got: %v", secretName, rels)
	}
}

// TestReferenceDetector_StatefulSetMissingService verifies that no relationship is created
// when the StatefulSet references a Service that does not exist.
func TestReferenceDetector_StatefulSetMissingService(t *testing.T) {
	const (
		ns          = "default"
		serviceName = "nonexistent-headless"
	)

	sts := makeProcessedResource(
		"apps/v1", "StatefulSet", "my-sts", ns,
		nil, nil,
		map[string]interface{}{
			"serviceName": serviceName,
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "main",
							"image": "nginx:latest",
						},
					},
				},
			},
		},
	)

	// allResources contains only the StatefulSet — the Service is absent.
	allResources := buildAllResources(sts)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), sts, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationNameReference {
			t.Errorf("expected no name_reference for missing Service, but got: %v", rel)
		}
	}
}

// TestReferenceDetector_DeploymentNoServiceAccount verifies that a Deployment without
// serviceAccountName set produces no service_account relationship.
func TestReferenceDetector_DeploymentNoServiceAccount(t *testing.T) {
	const ns = "default"

	deploy := makeProcessedResource(
		"apps/v1", "Deployment", "my-deploy", ns,
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "main",
							"image": "nginx:latest",
						},
					},
				},
			},
		},
	)

	allResources := buildAllResources(deploy)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), deploy, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationServiceAccount {
			t.Errorf("expected no service_account relationship for Deployment without SA, but got: %v", rel)
		}
	}
}

// TestReferenceDetector_IngressMalformedRules verifies that malformed Ingress rules
// (missing http, no paths, invalid rule type, empty secretName in TLS) don't panic
// and produce no relationships.
func TestReferenceDetector_IngressMalformedRules(t *testing.T) {
	const ns = "default"

	// Ingress with: non-map rule, rule without http, rule with http but no paths,
	// path without backend, TLS entry without secretName, TLS with empty secretName
	ingress := makeProcessedResourceExtra(
		"networking.k8s.io/v1", "Ingress", "malformed-ing", ns,
		map[string]interface{}{
			"spec": map[string]interface{}{
				"rules": []interface{}{
					"not-a-map",
					map[string]interface{}{
						"host": "no-http.example.com",
					},
					map[string]interface{}{
						"host": "no-paths.example.com",
						"http": map[string]interface{}{},
					},
					map[string]interface{}{
						"host": "bad-path.example.com",
						"http": map[string]interface{}{
							"paths": []interface{}{
								"not-a-map-path",
								map[string]interface{}{
									"path": "/no-backend",
								},
							},
						},
					},
				},
				"tls": []interface{}{
					"not-a-map-tls",
					map[string]interface{}{
						"hosts": []interface{}{"no-secret.example.com"},
					},
					map[string]interface{}{
						"secretName": "",
						"hosts":      []interface{}{"empty-secret.example.com"},
					},
				},
			},
		},
	)

	allResources := buildAllResources(ingress)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationNameReference {
			t.Errorf("expected no relationships from malformed Ingress, but got: %v", rel)
		}
	}
}

// TestReferenceDetector_ImagePullSecretMissingSecret verifies that no image_pull_secret
// relationship is created when the referenced Secret does not exist in allResources.
func TestReferenceDetector_ImagePullSecretMissingSecret(t *testing.T) {
	const (
		ns         = "default"
		secretName = "missing-registry-secret"
	)

	deploy := makeProcessedResource(
		"apps/v1", "Deployment", "my-deploy", ns,
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"imagePullSecrets": []interface{}{
						map[string]interface{}{"name": secretName},
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "main",
							"image": "private-registry/app:latest",
						},
					},
				},
			},
		},
	)

	// allResources contains only the Deployment — the Secret is absent.
	allResources := buildAllResources(deploy)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), deploy, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationImagePullSecret {
			t.Errorf("expected no image_pull_secret relationship for missing Secret, but got: %v", rel)
		}
	}
}

// TestReferenceDetector_CircularDependencies verifies that circular references
// (A→B, B→A) don't cause infinite loops or panics. Detectors should return
// relationships normally without crashing.
func TestReferenceDetector_CircularDependencies(t *testing.T) {
	// Ingress → Service (via backend)
	ingress := makeProcessedResourceExtra(
		"networking.k8s.io/v1", "Ingress", "my-ingress", "default",
		map[string]interface{}{
			"spec": map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{
						"host": "example.com",
						"http": map[string]interface{}{
							"paths": []interface{}{
								map[string]interface{}{
									"path": "/",
									"backend": map[string]interface{}{
										"service": map[string]interface{}{
											"name": "my-svc",
											"port": map[string]interface{}{"number": int64(80)},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	)

	// Service exists (would normally reference back to workloads via label selector)
	svc := makeProcessedResourceExtra(
		"v1", "Service", "my-svc", "default",
		map[string]interface{}{
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{"app": "my-app"},
				"ports": []interface{}{
					map[string]interface{}{"port": int64(80)},
				},
			},
		},
	)

	allResources := buildAllResources(ingress, svc)

	d := NewNameReferenceDetector()

	// Process both resources — should not panic or hang
	relsIngress := d.Detect(context.Background(), ingress, allResources)
	relsSvc := d.Detect(context.Background(), svc, allResources)

	// Ingress should find at least 1 relationship to Service
	foundIngressToSvc := false
	for _, rel := range relsIngress {
		if rel.Type == types.RelationNameReference && rel.To.Name == "my-svc" {
			foundIngressToSvc = true
		}
	}
	if !foundIngressToSvc {
		t.Error("expected Ingress→Service relationship")
	}

	// Service detector (NameReference) should not crash — it just won't find
	// references since Service is not in its kind switch
	_ = relsSvc
}

// TestReferenceDetector_PVCToStorageClass verifies that a PVC referencing a StorageClass
// via spec.storageClassName produces a storage_class relationship.
func TestReferenceDetector_PVCToStorageClass(t *testing.T) {
	pvc := makeProcessedResourceExtra(
		"v1", "PersistentVolumeClaim", "my-pvc", "default",
		map[string]interface{}{
			"spec": map[string]interface{}{
				"storageClassName": "fast-ssd",
				"accessModes":      []interface{}{"ReadWriteOnce"},
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"storage": "10Gi",
					},
				},
			},
		},
	)

	sc := makeProcessedResourceExtra(
		"storage.k8s.io/v1", "StorageClass", "fast-ssd", "",
		map[string]interface{}{
			"provisioner": "kubernetes.io/gce-pd",
		},
	)

	allResources := buildAllResources(pvc, sc)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), pvc, allResources)

	found := false
	for _, rel := range rels {
		if rel.Type == types.RelationStorageClass && rel.To.Name == "fast-ssd" {
			found = true
			if rel.Field != "spec.storageClassName" {
				t.Errorf("expected field 'spec.storageClassName', got %q", rel.Field)
			}
			if rel.Details["storageClassName"] != "fast-ssd" {
				t.Errorf("expected detail storageClassName 'fast-ssd', got %q", rel.Details["storageClassName"])
			}
		}
	}
	if !found {
		t.Error("expected storage_class relationship from PVC to StorageClass, but none found")
	}
}

// TestReferenceDetector_PVCMissingStorageClass verifies no relationship when StorageClass is absent.
func TestReferenceDetector_PVCMissingStorageClass(t *testing.T) {
	pvc := makeProcessedResourceExtra(
		"v1", "PersistentVolumeClaim", "my-pvc", "default",
		map[string]interface{}{
			"spec": map[string]interface{}{
				"storageClassName": "nonexistent",
				"accessModes":      []interface{}{"ReadWriteOnce"},
			},
		},
	)

	allResources := buildAllResources(pvc)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), pvc, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationStorageClass {
			t.Errorf("expected no storage_class relationship for missing StorageClass, got: %v", rel)
		}
	}
}

// TestReferenceDetector_PVCEmptyStorageClassName verifies that a PVC with an empty
// storageClassName string (field present but blank) produces no relationship.
func TestReferenceDetector_PVCEmptyStorageClassName(t *testing.T) {
	pvc := makeProcessedResourceExtra(
		"v1", "PersistentVolumeClaim", "my-pvc", "default",
		map[string]interface{}{
			"spec": map[string]interface{}{
				"storageClassName": "",
				"accessModes":      []interface{}{"ReadWriteOnce"},
			},
		},
	)

	allResources := buildAllResources(pvc)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), pvc, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationStorageClass {
			t.Errorf("expected no storage_class relationship for empty storageClassName, got: %v", rel)
		}
	}
}

// TestReferenceDetector_StatefulSetEmptyServiceName verifies that a StatefulSet with
// spec.serviceName set to an empty string produces no relationship (the "" guard).
func TestReferenceDetector_StatefulSetEmptyServiceName(t *testing.T) {
	const ns = "default"

	sts := makeProcessedResource(
		"apps/v1", "StatefulSet", "my-sts", ns,
		nil, nil,
		map[string]interface{}{
			"serviceName": "",
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "main",
							"image": "nginx:latest",
						},
					},
				},
			},
		},
	)

	svc := makeProcessedResource("v1", "Service", "some-svc", ns, nil, nil, nil)
	allResources := buildAllResources(sts, svc)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), sts, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationNameReference {
			t.Errorf("expected no name_reference for empty serviceName, but got: %v", rel)
		}
	}
}

// TestReferenceDetector_PodServiceAccount verifies that a Pod with
// spec.serviceAccountName produces a service_account relationship.
func TestReferenceDetector_PodServiceAccount(t *testing.T) {
	const (
		ns     = "default"
		saName = "pod-sa"
	)

	pod := makeProcessedResource(
		"v1", "Pod", "my-pod", ns,
		nil, nil,
		map[string]interface{}{
			"serviceAccountName": saName,
			"containers": []interface{}{
				map[string]interface{}{
					"name":  "main",
					"image": "nginx:latest",
				},
			},
		},
	)

	sa := makeProcessedResource("v1", "ServiceAccount", saName, ns, nil, nil, nil)
	allResources := buildAllResources(pod, sa)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), pod, allResources)

	saKey := sa.Original.ResourceKey()
	found := false
	for _, rel := range rels {
		if rel.Type == types.RelationServiceAccount && rel.To == saKey {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected service_account relationship from Pod to ServiceAccount %q, got: %v", saName, rels)
	}
}

// TestReferenceDetector_CronJobServiceAccount verifies that a CronJob with
// spec.jobTemplate.spec.template.spec.serviceAccountName produces a
// service_account relationship.
func TestReferenceDetector_CronJobServiceAccount(t *testing.T) {
	const (
		ns     = "default"
		saName = "cron-sa"
	)

	cronJob := makeProcessedResource(
		"batch/v1", "CronJob", "my-cronjob", ns,
		nil, nil,
		map[string]interface{}{
			"jobTemplate": map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"serviceAccountName": saName,
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "job",
									"image": "job:latest",
								},
							},
						},
					},
				},
			},
		},
	)

	sa := makeProcessedResource("v1", "ServiceAccount", saName, ns, nil, nil, nil)
	allResources := buildAllResources(cronJob, sa)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), cronJob, allResources)

	saKey := sa.Original.ResourceKey()
	found := false
	for _, rel := range rels {
		if rel.Type == types.RelationServiceAccount && rel.To == saKey {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected service_account relationship from CronJob to ServiceAccount %q, got: %v", saName, rels)
	}
}

// TestReferenceDetector_PodImagePullSecrets verifies that a Pod with imagePullSecrets
// at spec.imagePullSecrets produces image_pull_secret relationships.
func TestReferenceDetector_PodImagePullSecrets(t *testing.T) {
	const (
		ns         = "default"
		secretName = "pod-registry-secret"
	)

	pod := makeProcessedResource(
		"v1", "Pod", "my-pod", ns,
		nil, nil,
		map[string]interface{}{
			"imagePullSecrets": []interface{}{
				map[string]interface{}{"name": secretName},
			},
			"containers": []interface{}{
				map[string]interface{}{
					"name":  "main",
					"image": "private/app:latest",
				},
			},
		},
	)

	secret := makeProcessedResource("v1", "Secret", secretName, ns, nil, nil, nil)
	allResources := buildAllResources(pod, secret)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), pod, allResources)

	secretKey := secret.Original.ResourceKey()
	found := false
	for _, rel := range rels {
		if rel.Type == types.RelationImagePullSecret && rel.To == secretKey {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected image_pull_secret relationship from Pod to Secret %q, got: %v", secretName, rels)
	}
}

// TestReferenceDetector_CronJobImagePullSecrets verifies that a CronJob with
// imagePullSecrets in the pod template produces image_pull_secret relationships.
func TestReferenceDetector_CronJobImagePullSecrets(t *testing.T) {
	const (
		ns         = "default"
		secretName = "cron-registry-secret"
	)

	cronJob := makeProcessedResource(
		"batch/v1", "CronJob", "my-cronjob", ns,
		nil, nil,
		map[string]interface{}{
			"jobTemplate": map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"imagePullSecrets": []interface{}{
								map[string]interface{}{"name": secretName},
							},
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "job",
									"image": "private/job:latest",
								},
							},
						},
					},
				},
			},
		},
	)

	secret := makeProcessedResource("v1", "Secret", secretName, ns, nil, nil, nil)
	allResources := buildAllResources(cronJob, secret)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), cronJob, allResources)

	secretKey := secret.Original.ResourceKey()
	found := false
	for _, rel := range rels {
		if rel.Type == types.RelationImagePullSecret && rel.To == secretKey {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected image_pull_secret relationship from CronJob to Secret %q, got: %v", secretName, rels)
	}
}

// TestReferenceDetector_ImagePullSecretsMalformed verifies that malformed entries in
// imagePullSecrets (non-map entries, missing name field, empty name) are skipped without
// panicking and produce no relationships.
func TestReferenceDetector_ImagePullSecretsMalformed(t *testing.T) {
	const ns = "default"

	deploy := makeProcessedResource(
		"apps/v1", "Deployment", "my-deploy", ns,
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"imagePullSecrets": []interface{}{
						"not-a-map",
						map[string]interface{}{"name": ""},
						map[string]interface{}{"notname": "something"},
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "main",
							"image": "nginx:latest",
						},
					},
				},
			},
		},
	)

	allResources := buildAllResources(deploy)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), deploy, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationImagePullSecret {
			t.Errorf("expected no image_pull_secret relationships for malformed entries, got: %v", rel)
		}
	}
}

// TestReferenceDetector_RoleBindingNoRoleRefKind verifies that a RoleBinding whose
// roleRef.kind is not "Role" or "ClusterRole" produces no role_binding to a role
// (but may still produce subject relationships if subjects exist).
func TestReferenceDetector_RoleBindingNoRoleRefKind(t *testing.T) {
	const ns = "default"

	rb := makeProcessedResourceExtra(
		"rbac.authorization.k8s.io/v1", "RoleBinding", "my-rb", ns,
		map[string]interface{}{
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "UnknownKind",
				"name":     "some-role",
			},
		},
	)

	allResources := buildAllResources(rb)
	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), rb, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationRoleBinding && rel.Field == "roleRef" {
			t.Errorf("expected no roleRef relationship for unknown roleRef kind, got: %v", rel)
		}
	}
}

// TestReferenceDetector_RoleBindingSubjectWithoutNamespace verifies that a subject
// without an explicit namespace defaults to the RoleBinding's namespace.
func TestReferenceDetector_RoleBindingSubjectWithoutNamespace(t *testing.T) {
	const (
		ns     = "mynamespace"
		saName = "my-sa"
	)

	rb := makeProcessedResourceExtra(
		"rbac.authorization.k8s.io/v1", "RoleBinding", "my-rb", ns,
		map[string]interface{}{
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     "some-role",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind": "ServiceAccount",
					"name": saName,
					// no "namespace" key — should default to RoleBinding's namespace
				},
			},
		},
	)

	sa := makeProcessedResource("v1", "ServiceAccount", saName, ns, nil, nil, nil)
	allResources := buildAllResources(rb, sa)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), rb, allResources)

	saKey := sa.Original.ResourceKey()
	found := false
	for _, rel := range rels {
		if rel.Type == types.RelationRoleBinding && rel.To == saKey {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected role_binding relationship to SA %q using RoleBinding namespace, got: %v", saName, rels)
	}
}

// TestReferenceDetector_IngressEmptyServiceName verifies that an Ingress path whose
// backend service name is present in the object but set to an empty string ("")
// produces no relationship. This covers the `serviceName == ""` branch of the
// `!found || serviceName == ""` guard in detectIngressToService.
func TestReferenceDetector_IngressEmptyServiceName(t *testing.T) {
	const ns = "default"

	ingress := makeProcessedResource(
		"networking.k8s.io/v1", "Ingress", "my-ingress", ns,
		nil, nil,
		map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"host": "example.com",
					"http": map[string]interface{}{
						"paths": []interface{}{
							map[string]interface{}{
								"path":     "/",
								"pathType": "Prefix",
								"backend": map[string]interface{}{
									"service": map[string]interface{}{
										// name field is present but empty string
										"name": "",
										"port": map[string]interface{}{
											"number": int64(80),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	)

	// Add a Service so the lookup would succeed if the guard were bypassed.
	svc := makeProcessedResource("v1", "Service", "any-service", ns, nil, nil, nil)
	allResources := buildAllResources(ingress, svc)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationNameReference {
			t.Errorf("expected no name_reference for empty backend service name, but got: %v", rel)
		}
	}
}

// TestReferenceDetector_RoleBindingNonServiceAccountSubjects verifies that subjects
// of kind "User" or "Group" — which are not ServiceAccounts — are skipped and produce
// no role_binding relationships. Also covers a ServiceAccount subject with an empty
// name (the `name != ""` guard).
func TestReferenceDetector_RoleBindingNonServiceAccountSubjects(t *testing.T) {
	const (
		ns     = "default"
		saName = "real-sa"
	)

	rb := makeProcessedResourceExtra(
		"rbac.authorization.k8s.io/v1", "RoleBinding", "my-rb", ns,
		map[string]interface{}{
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     "my-role",
			},
			"subjects": []interface{}{
				// kind "User" — not a ServiceAccount, must be skipped
				map[string]interface{}{
					"kind": "User",
					"name": "alice",
				},
				// kind "Group" — not a ServiceAccount, must be skipped
				map[string]interface{}{
					"kind": "Group",
					"name": "developers",
				},
				// ServiceAccount with empty name — the `name != ""` guard must skip it
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      "",
					"namespace": ns,
				},
				// A valid ServiceAccount to confirm the loop continues past the skipped entries
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      saName,
					"namespace": ns,
				},
			},
		},
	)

	sa := makeProcessedResource("v1", "ServiceAccount", saName, ns, nil, nil, nil)
	allResources := buildAllResources(rb, sa)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), rb, allResources)

	saKey := sa.Original.ResourceKey()
	foundSA := false
	for _, rel := range rels {
		if rel.Type == types.RelationRoleBinding {
			// There must not be a relationship pointing to "alice" or "developers"
			if rel.To.Name == "alice" || rel.To.Name == "developers" {
				t.Errorf("unexpected relationship to User/Group subject: %v", rel)
			}
			// The valid ServiceAccount should be found
			if rel.To == saKey {
				foundSA = true
			}
		}
	}

	if !foundSA {
		t.Errorf("expected role_binding relationship to ServiceAccount %q to still be found, got: %v", saName, rels)
	}
}

// TestReferenceDetector_IngressNoRules verifies that an Ingress with no spec.rules
// field at all returns early from detectIngressToService without panicking. This
// covers the `if !found { return relationships }` guard at line 73 of reference.go.
func TestReferenceDetector_IngressNoRules(t *testing.T) {
	const ns = "default"

	// Ingress object has no spec.rules key at all.
	ingress := makeProcessedResourceExtra(
		"networking.k8s.io/v1", "Ingress", "no-rules-ingress", ns,
		map[string]interface{}{
			"spec": map[string]interface{}{
				// no "rules" key — triggers !found early return in detectIngressToService
				"tls": []interface{}{},
			},
		},
	)

	allResources := buildAllResources(ingress)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), ingress, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationNameReference {
			t.Errorf("expected no name_reference for Ingress with no rules, but got: %v", rel)
		}
	}
}

// TestReferenceDetector_RoleBindingSubjectsNonMapEntry verifies that a non-map entry
// in the subjects slice (e.g., a string literal) is skipped without panicking. This
// covers the `subjectMap, ok := subject.(map[string]interface{}); if !ok { continue }`
// guard at line 243-245 of reference.go.
func TestReferenceDetector_RoleBindingSubjectsNonMapEntry(t *testing.T) {
	const (
		ns     = "default"
		saName = "real-sa"
	)

	rb := makeProcessedResourceExtra(
		"rbac.authorization.k8s.io/v1", "RoleBinding", "my-rb", ns,
		map[string]interface{}{
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     "my-role",
			},
			"subjects": []interface{}{
				// non-map entry — triggers the !ok continue branch
				"not-a-map-subject",
				// valid ServiceAccount that follows the bad entry
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      saName,
					"namespace": ns,
				},
			},
		},
	)

	sa := makeProcessedResource("v1", "ServiceAccount", saName, ns, nil, nil, nil)
	allResources := buildAllResources(rb, sa)

	d := NewNameReferenceDetector()
	rels := d.Detect(context.Background(), rb, allResources)

	// The non-map entry should be skipped; the valid SA should still produce a relationship.
	saKey := sa.Original.ResourceKey()
	found := false
	for _, rel := range rels {
		if rel.Type == types.RelationRoleBinding && rel.To == saKey {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected role_binding relationship to ServiceAccount %q after skipping non-map subject, got: %v", saName, rels)
	}
}
