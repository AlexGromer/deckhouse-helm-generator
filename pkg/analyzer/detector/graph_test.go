package detector

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// TestComplexGraphConstruction verifies that a complex application with 5 resources
// produces correct relationships when all detectors are run. This validates
// the relationship graph construction and that topological ordering is possible
// (no cycles in detected relationships for a well-structured app).
func TestComplexGraphConstruction(t *testing.T) {
	const ns = "production"

	// ── 5 Resources ─────────────────────────────────────────────────
	// 1. Deployment (web-app) — references ConfigMap, Secret (envFrom), ServiceAccount
	// 2. Service — selects Deployment by label
	// 3. ConfigMap — standalone
	// 4. Secret — standalone
	// 5. Ingress — references Service, has cert-manager annotation

	deployment := makeGraphResource("apps/v1", "Deployment", "web-app", ns,
		map[string]string{"app": "web-app"},
		nil,
		map[string]interface{}{
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{"app": "web-app"},
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{"app": "web-app"},
					},
					"spec": map[string]interface{}{
						"serviceAccountName": "web-app-sa",
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "web",
								"image": "nginx:latest",
								"envFrom": []interface{}{
									map[string]interface{}{
										"configMapRef": map[string]interface{}{
											"name": "web-config",
										},
									},
								},
								"env": []interface{}{
									map[string]interface{}{
										"name": "DB_PASSWORD",
										"valueFrom": map[string]interface{}{
											"secretKeyRef": map[string]interface{}{
												"name": "web-secret",
												"key":  "password",
											},
										},
									},
								},
								"volumeMounts": []interface{}{
									map[string]interface{}{
										"name":      "config-vol",
										"mountPath": "/etc/config",
									},
								},
							},
						},
						"volumes": []interface{}{
							map[string]interface{}{
								"name": "config-vol",
								"configMap": map[string]interface{}{
									"name": "web-config",
								},
							},
						},
					},
				},
			},
		},
	)

	service := makeGraphResource("v1", "Service", "web-svc", ns,
		map[string]string{"app": "web-app"},
		nil,
		map[string]interface{}{
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{"app": "web-app"},
				"ports": []interface{}{
					map[string]interface{}{
						"port":       int64(80),
						"targetPort": int64(8080),
					},
				},
			},
		},
	)

	configMap := makeGraphResource("v1", "ConfigMap", "web-config", ns,
		map[string]string{"app": "web-app"},
		nil,
		map[string]interface{}{
			"data": map[string]interface{}{
				"APP_ENV": "production",
			},
		},
	)

	secret := makeGraphResource("v1", "Secret", "web-secret", ns,
		map[string]string{"app": "web-app"},
		nil,
		map[string]interface{}{
			"type": "Opaque",
			"data": map[string]interface{}{
				"password": "c2VjcmV0",
			},
		},
	)

	ingress := makeGraphResource("networking.k8s.io/v1", "Ingress", "web-ingress", ns,
		map[string]string{"app": "web-app"},
		map[string]string{
			"cert-manager.io/cluster-issuer": "letsencrypt-prod",
		},
		map[string]interface{}{
			"spec": map[string]interface{}{
				"ingressClassName": "nginx",
				"tls": []interface{}{
					map[string]interface{}{
						"hosts":      []interface{}{"web.example.com"},
						"secretName": "web-tls",
					},
				},
				"rules": []interface{}{
					map[string]interface{}{
						"host": "web.example.com",
						"http": map[string]interface{}{
							"paths": []interface{}{
								map[string]interface{}{
									"path":     "/",
									"pathType": "Prefix",
									"backend": map[string]interface{}{
										"service": map[string]interface{}{
											"name": "web-svc",
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

	// ── Build allResources map ──────────────────────────────────────
	resources := []*types.ProcessedResource{deployment, service, configMap, secret, ingress}
	allResources := make(map[types.ResourceKey]*types.ProcessedResource)
	for _, r := range resources {
		allResources[r.Original.ResourceKey()] = r
	}

	// ── Run all detectors ───────────────────────────────────────────
	detectors := []interface {
		Detect(context.Context, *types.ProcessedResource, map[types.ResourceKey]*types.ProcessedResource) []types.Relationship
	}{
		NewLabelSelectorDetector(),
		NewNameReferenceDetector(),
		NewVolumeMountDetector(),
		NewAnnotationDetector(),
	}

	graph := types.NewResourceGraph()
	for _, r := range resources {
		graph.AddResource(r)
	}

	ctx := context.Background()
	for _, r := range resources {
		for _, d := range detectors {
			rels := d.Detect(ctx, r, allResources)
			for _, rel := range rels {
				graph.AddRelationship(rel)
			}
		}
	}

	// ── Verify graph construction ───────────────────────────────────

	// 5 resources in graph
	if len(graph.Resources) != 5 {
		t.Errorf("Expected 5 resources in graph, got %d", len(graph.Resources))
	}

	// At least 5 relationships expected:
	// 1. Service → Deployment (label_selector)
	// 2. Ingress → Service (name_reference)
	// 3. Deployment → ConfigMap (volume_mount or env_from)
	// 4. Deployment → Secret (env_value_from)
	// 5. Ingress cert-manager annotation (annotation)
	if len(graph.Relationships) < 5 {
		t.Errorf("Expected at least 5 relationships, got %d", len(graph.Relationships))
		for i, rel := range graph.Relationships {
			t.Logf("  rel[%d]: %s → %s (type=%s, field=%s)", i,
				rel.From.String(), rel.To.String(), rel.Type, rel.Field)
		}
	}

	// Verify specific relationship types are present
	relTypes := make(map[types.RelationshipType]int)
	for _, rel := range graph.Relationships {
		relTypes[rel.Type]++
	}

	expectedTypes := []types.RelationshipType{
		types.RelationLabelSelector,
		types.RelationNameReference,
	}
	for _, et := range expectedTypes {
		if relTypes[et] == 0 {
			t.Errorf("Expected at least 1 relationship of type %q, found 0", et)
		}
	}

	// Verify the graph has env-related relationships (envFrom or env_value_from or volume_mount)
	envRelCount := relTypes[types.RelationEnvFrom] + relTypes[types.RelationEnvValueFrom] + relTypes[types.RelationVolumeMount]
	if envRelCount == 0 {
		t.Error("Expected at least 1 env/volume relationship from Deployment")
	}

	// Verify topological sort is possible: check there are no self-referencing relationships
	for _, rel := range graph.Relationships {
		if rel.From == rel.To {
			t.Errorf("Self-referencing relationship detected: %s → %s", rel.From.String(), rel.To.String())
		}
	}

	// Verify that Ingress→Service→Deployment forms a valid chain
	ingressKey := ingress.Original.ResourceKey()
	serviceKey := service.Original.ResourceKey()
	deploymentKey := deployment.Original.ResourceKey()

	ingressToSvc := false
	svcToDeploy := false
	for _, rel := range graph.Relationships {
		if rel.From == ingressKey && rel.To == serviceKey {
			ingressToSvc = true
		}
		if rel.From == serviceKey && rel.To == deploymentKey {
			svcToDeploy = true
		}
	}
	if !ingressToSvc {
		t.Error("Expected Ingress → Service relationship in graph")
	}
	if !svcToDeploy {
		t.Error("Expected Service → Deployment relationship in graph")
	}

	t.Logf("Graph: %d resources, %d relationships", len(graph.Resources), len(graph.Relationships))
	for i, rel := range graph.Relationships {
		t.Logf("  [%d] %s → %s (type=%s)", i, rel.From.String(), rel.To.String(), rel.Type)
	}
}

// makeGraphResource creates a ProcessedResource for graph tests with labels, annotations, and extra fields.
func makeGraphResource(apiVersion, kind, name, namespace string, labels, annotations map[string]string, extra map[string]interface{}) *types.ProcessedResource {
	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}
	if labels != nil {
		labelsIface := make(map[string]interface{})
		for k, v := range labels {
			labelsIface[k] = v
		}
		metadata["labels"] = labelsIface
	}
	if annotations != nil {
		annIface := make(map[string]interface{})
		for k, v := range annotations {
			annIface[k] = v
		}
		metadata["annotations"] = annIface
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata":   metadata,
		},
	}

	for k, v := range extra {
		obj.Object[k] = v
	}

	gv, _ := schema.ParseGroupVersion(apiVersion)
	gvk := gv.WithKind(kind)

	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    gvk,
		},
		ServiceName: name,
	}
}
