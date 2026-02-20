package detector

import (
	"context"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// ============================================================
// Constructor / metadata tests
// ============================================================

func TestNewVolumeMountDetector(t *testing.T) {
	d := NewVolumeMountDetector()

	if d == nil {
		t.Fatal("NewVolumeMountDetector() returned nil")
	}

	if d.Name() != "volume_mount" {
		t.Errorf("Name() = %q, want %q", d.Name(), "volume_mount")
	}

	if d.Priority() != 80 {
		t.Errorf("Priority() = %d, want 80", d.Priority())
	}
}

// ============================================================
// Volume-based relationship tests
// ============================================================

func TestVolumeDetector_ConfigMapVolume(t *testing.T) {
	// Deployment that mounts a ConfigMap as a volume; ConfigMap is in allResources.
	deployment := makeProcessedResource("apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{
						map[string]interface{}{
							"name": "config-vol",
							"configMap": map[string]interface{}{
								"name": "app-config",
							},
						},
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
						},
					},
				},
			},
		})

	cmKey := resourceKey("v1", "ConfigMap", "default", "app-config")
	cm := makeProcessedResource("v1", "ConfigMap", "app-config", "default", nil, nil, nil)

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
		cmKey:                             cm,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), deployment, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(rels))
	}

	rel := rels[0]
	if rel.Type != types.RelationVolumeMount {
		t.Errorf("Type = %q, want %q", rel.Type, types.RelationVolumeMount)
	}
	if rel.To != cmKey {
		t.Errorf("To = %v, want %v", rel.To, cmKey)
	}
	if rel.Field != "spec.template.spec.volumes[].configMap" {
		t.Errorf("Field = %q, want %q", rel.Field, "spec.template.spec.volumes[].configMap")
	}
	if rel.Details["configMapName"] != "app-config" {
		t.Errorf("Details[configMapName] = %q, want %q", rel.Details["configMapName"], "app-config")
	}
	if rel.Details["volumeName"] != "config-vol" {
		t.Errorf("Details[volumeName] = %q, want %q", rel.Details["volumeName"], "config-vol")
	}
}

func TestVolumeDetector_SecretVolume(t *testing.T) {
	// Deployment that mounts a Secret as a volume; Secret is in allResources.
	deployment := makeProcessedResource("apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{
						map[string]interface{}{
							"name": "secret-vol",
							"secret": map[string]interface{}{
								"secretName": "tls-secret",
							},
						},
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
						},
					},
				},
			},
		})

	secretKey := resourceKey("v1", "Secret", "default", "tls-secret")
	secret := makeProcessedResource("v1", "Secret", "tls-secret", "default", nil, nil, nil)

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
		secretKey:                         secret,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), deployment, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(rels))
	}

	rel := rels[0]
	if rel.Type != types.RelationVolumeMount {
		t.Errorf("Type = %q, want %q", rel.Type, types.RelationVolumeMount)
	}
	if rel.To != secretKey {
		t.Errorf("To = %v, want %v", rel.To, secretKey)
	}
	if rel.Field != "spec.template.spec.volumes[].secret" {
		t.Errorf("Field = %q, want %q", rel.Field, "spec.template.spec.volumes[].secret")
	}
	if rel.Details["secretName"] != "tls-secret" {
		t.Errorf("Details[secretName] = %q, want %q", rel.Details["secretName"], "tls-secret")
	}
	if rel.Details["volumeName"] != "secret-vol" {
		t.Errorf("Details[volumeName] = %q, want %q", rel.Details["volumeName"], "secret-vol")
	}
}

func TestVolumeDetector_PVCVolume(t *testing.T) {
	// Deployment that mounts a PVC; PVC is in allResources.
	deployment := makeProcessedResource("apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{
						map[string]interface{}{
							"name": "data-vol",
							"persistentVolumeClaim": map[string]interface{}{
								"claimName": "my-pvc",
							},
						},
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
						},
					},
				},
			},
		})

	pvcKey := resourceKey("v1", "PersistentVolumeClaim", "default", "my-pvc")
	pvc := makeProcessedResource("v1", "PersistentVolumeClaim", "my-pvc", "default", nil, nil, nil)

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
		pvcKey:                            pvc,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), deployment, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(rels))
	}

	rel := rels[0]
	if rel.Type != types.RelationPVC {
		t.Errorf("Type = %q, want %q", rel.Type, types.RelationPVC)
	}
	if rel.To != pvcKey {
		t.Errorf("To = %v, want %v", rel.To, pvcKey)
	}
	if rel.Field != "spec.template.spec.volumes[].persistentVolumeClaim" {
		t.Errorf("Field = %q, want %q", rel.Field, "spec.template.spec.volumes[].persistentVolumeClaim")
	}
	if rel.Details["pvcName"] != "my-pvc" {
		t.Errorf("Details[pvcName] = %q, want %q", rel.Details["pvcName"], "my-pvc")
	}
	if rel.Details["volumeName"] != "data-vol" {
		t.Errorf("Details[volumeName] = %q, want %q", rel.Details["volumeName"], "data-vol")
	}
}

func TestVolumeDetector_EmptyDirVolume(t *testing.T) {
	// Deployment with only an emptyDir volume -> no relationships expected.
	deployment := makeProcessedResource("apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{
						map[string]interface{}{
							"name":     "cache",
							"emptyDir": map[string]interface{}{},
						},
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
						},
					},
				},
			},
		})

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), deployment, allResources)

	if len(rels) != 0 {
		t.Errorf("expected 0 relationships for emptyDir volume, got %d", len(rels))
	}
}

func TestVolumeDetector_ProjectedVolume(t *testing.T) {
	// Deployment with a projected volume containing a configMap source; ConfigMap is in allResources.
	deployment := makeProcessedResource("apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{
						map[string]interface{}{
							"name": "projected-vol",
							"projected": map[string]interface{}{
								"sources": []interface{}{
									map[string]interface{}{
										"configMap": map[string]interface{}{
											"name": "proj-config",
										},
									},
								},
							},
						},
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
						},
					},
				},
			},
		})

	cmKey := resourceKey("v1", "ConfigMap", "default", "proj-config")
	cm := makeProcessedResource("v1", "ConfigMap", "proj-config", "default", nil, nil, nil)

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
		cmKey:                             cm,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), deployment, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship for projected configMap, got %d", len(rels))
	}

	rel := rels[0]
	if rel.Type != types.RelationVolumeMount {
		t.Errorf("Type = %q, want %q", rel.Type, types.RelationVolumeMount)
	}
	if rel.To != cmKey {
		t.Errorf("To = %v, want %v", rel.To, cmKey)
	}
	if rel.Field != "spec.template.spec.volumes[].projected.sources[].configMap" {
		t.Errorf("Field = %q, want %q", rel.Field, "spec.template.spec.volumes[].projected.sources[].configMap")
	}
	if rel.Details["configMapName"] != "proj-config" {
		t.Errorf("Details[configMapName] = %q, want %q", rel.Details["configMapName"], "proj-config")
	}
	if rel.Details["volumeName"] != "projected-vol" {
		t.Errorf("Details[volumeName] = %q, want %q", rel.Details["volumeName"], "projected-vol")
	}
}

// ============================================================
// envFrom-based relationship tests
// ============================================================

func TestVolumeDetector_EnvFromConfigMap(t *testing.T) {
	// Deployment with envFrom configMapRef; ConfigMap is in allResources.
	// An empty volumes slice is required so that Detect() does not early-return
	// before reaching the envFrom detection logic.
	deployment := makeProcessedResource("apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
							"envFrom": []interface{}{
								map[string]interface{}{
									"configMapRef": map[string]interface{}{
										"name": "env-config",
									},
								},
							},
						},
					},
				},
			},
		})

	cmKey := resourceKey("v1", "ConfigMap", "default", "env-config")
	cm := makeProcessedResource("v1", "ConfigMap", "env-config", "default", nil, nil, nil)

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
		cmKey:                             cm,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), deployment, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship for envFrom configMapRef, got %d", len(rels))
	}

	rel := rels[0]
	if rel.Type != types.RelationEnvFrom {
		t.Errorf("Type = %q, want %q", rel.Type, types.RelationEnvFrom)
	}
	if rel.To != cmKey {
		t.Errorf("To = %v, want %v", rel.To, cmKey)
	}
	if rel.Field != "spec.template.spec.containers[].envFrom[].configMapRef" {
		t.Errorf("Field = %q, want %q", rel.Field, "spec.template.spec.containers[].envFrom[].configMapRef")
	}
	if rel.Details["configMapName"] != "env-config" {
		t.Errorf("Details[configMapName] = %q, want %q", rel.Details["configMapName"], "env-config")
	}
}

// ============================================================
// env[].valueFrom-based relationship tests
// ============================================================

func TestVolumeDetector_EnvValueFromSecret(t *testing.T) {
	// Deployment with env[].valueFrom.secretKeyRef; Secret is in allResources.
	// An empty volumes slice is required so that Detect() does not early-return
	// before reaching the env valueFrom detection logic.
	deployment := makeProcessedResource("apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
							"env": []interface{}{
								map[string]interface{}{
									"name": "DB_PASSWORD",
									"valueFrom": map[string]interface{}{
										"secretKeyRef": map[string]interface{}{
											"name": "db-credentials",
											"key":  "password",
										},
									},
								},
							},
						},
					},
				},
			},
		})

	secretKey := resourceKey("v1", "Secret", "default", "db-credentials")
	secret := makeProcessedResource("v1", "Secret", "db-credentials", "default", nil, nil, nil)

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
		secretKey:                         secret,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), deployment, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship for env valueFrom secretKeyRef, got %d", len(rels))
	}

	rel := rels[0]
	if rel.Type != types.RelationEnvValueFrom {
		t.Errorf("Type = %q, want %q", rel.Type, types.RelationEnvValueFrom)
	}
	if rel.To != secretKey {
		t.Errorf("To = %v, want %v", rel.To, secretKey)
	}
	if rel.Field != "spec.template.spec.containers[].env[].valueFrom.secretKeyRef" {
		t.Errorf("Field = %q, want %q", rel.Field, "spec.template.spec.containers[].env[].valueFrom.secretKeyRef")
	}
	if rel.Details["secretName"] != "db-credentials" {
		t.Errorf("Details[secretName] = %q, want %q", rel.Details["secretName"], "db-credentials")
	}
	if rel.Details["envVarName"] != "DB_PASSWORD" {
		t.Errorf("Details[envVarName] = %q, want %q", rel.Details["envVarName"], "DB_PASSWORD")
	}
}

func TestVolumeDetector_EnvFromSecretRef(t *testing.T) {
	// Deployment with envFrom secretRef; Secret is in allResources.
	deployment := makeProcessedResource("apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
							"envFrom": []interface{}{
								map[string]interface{}{
									"secretRef": map[string]interface{}{
										"name": "app-secrets",
									},
								},
							},
						},
					},
				},
			},
		})

	secretKey := resourceKey("v1", "Secret", "default", "app-secrets")
	secret := makeProcessedResource("v1", "Secret", "app-secrets", "default", nil, nil, nil)

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
		secretKey:                         secret,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), deployment, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship for envFrom secretRef, got %d", len(rels))
	}
	if rels[0].Type != types.RelationEnvFrom {
		t.Errorf("Type = %q, want %q", rels[0].Type, types.RelationEnvFrom)
	}
	if rels[0].Details["secretName"] != "app-secrets" {
		t.Errorf("Details[secretName] = %q, want %q", rels[0].Details["secretName"], "app-secrets")
	}
}

func TestVolumeDetector_EnvValueFromConfigMapKeyRef(t *testing.T) {
	// Deployment with env[].valueFrom.configMapKeyRef; ConfigMap is in allResources.
	deployment := makeProcessedResource("apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
							"env": []interface{}{
								map[string]interface{}{
									"name": "APP_CONFIG",
									"valueFrom": map[string]interface{}{
										"configMapKeyRef": map[string]interface{}{
											"name": "app-config",
											"key":  "config.yaml",
										},
									},
								},
							},
						},
					},
				},
			},
		})

	cmKey := resourceKey("v1", "ConfigMap", "default", "app-config")
	cm := makeProcessedResource("v1", "ConfigMap", "app-config", "default", nil, nil, nil)

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
		cmKey:                             cm,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), deployment, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship for env configMapKeyRef, got %d", len(rels))
	}
	if rels[0].Type != types.RelationEnvValueFrom {
		t.Errorf("Type = %q, want %q", rels[0].Type, types.RelationEnvValueFrom)
	}
	if rels[0].Details["configMapName"] != "app-config" {
		t.Errorf("Details[configMapName] = %q, want %q", rels[0].Details["configMapName"], "app-config")
	}
	if rels[0].Details["envVarName"] != "APP_CONFIG" {
		t.Errorf("Details[envVarName] = %q, want %q", rels[0].Details["envVarName"], "APP_CONFIG")
	}
}

func TestVolumeDetector_PodKind(t *testing.T) {
	// Pod has volumes at spec.volumes (not spec.template.spec.volumes)
	pod := makeProcessedResource("v1", "Pod", "my-pod", "default",
		nil, nil,
		map[string]interface{}{
			"volumes": []interface{}{
				map[string]interface{}{
					"name": "config-vol",
					"configMap": map[string]interface{}{
						"name": "pod-config",
					},
				},
			},
			"containers": []interface{}{
				map[string]interface{}{
					"name":  "app",
					"image": "app:latest",
				},
			},
		})

	cmKey := resourceKey("v1", "ConfigMap", "default", "pod-config")
	cm := makeProcessedResource("v1", "ConfigMap", "pod-config", "default", nil, nil, nil)

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		pod.Original.ResourceKey(): pod,
		cmKey:                      cm,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), pod, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship for Pod volume, got %d", len(rels))
	}
	if rels[0].Type != types.RelationVolumeMount {
		t.Errorf("Type = %q, want %q", rels[0].Type, types.RelationVolumeMount)
	}
}

func TestVolumeDetector_CronJobKind(t *testing.T) {
	// CronJob has volumes at spec.jobTemplate.spec.template.spec.volumes
	cronJob := makeProcessedResource("batch/v1", "CronJob", "my-cronjob", "default",
		nil, nil,
		map[string]interface{}{
			"jobTemplate": map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"volumes": []interface{}{
								map[string]interface{}{
									"name": "config-vol",
									"configMap": map[string]interface{}{
										"name": "cron-config",
									},
								},
							},
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
		})

	cmKey := resourceKey("v1", "ConfigMap", "default", "cron-config")
	cm := makeProcessedResource("v1", "ConfigMap", "cron-config", "default", nil, nil, nil)

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		cronJob.Original.ResourceKey(): cronJob,
		cmKey:                          cm,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), cronJob, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship for CronJob volume, got %d", len(rels))
	}
	if rels[0].Type != types.RelationVolumeMount {
		t.Errorf("Type = %q, want %q", rels[0].Type, types.RelationVolumeMount)
	}
}

func TestVolumeDetector_ProjectedSecretVolume(t *testing.T) {
	// Deployment with projected volume containing a secret source
	deployment := makeProcessedResource("apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{
						map[string]interface{}{
							"name": "projected-vol",
							"projected": map[string]interface{}{
								"sources": []interface{}{
									map[string]interface{}{
										"secret": map[string]interface{}{
											"name": "proj-secret",
										},
									},
								},
							},
						},
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
						},
					},
				},
			},
		})

	secretKey := resourceKey("v1", "Secret", "default", "proj-secret")
	secret := makeProcessedResource("v1", "Secret", "proj-secret", "default", nil, nil, nil)

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
		secretKey:                         secret,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), deployment, allResources)

	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship for projected secret, got %d", len(rels))
	}
	if rels[0].Type != types.RelationVolumeMount {
		t.Errorf("Type = %q, want %q", rels[0].Type, types.RelationVolumeMount)
	}
	if rels[0].Field != "spec.template.spec.volumes[].projected.sources[].secret" {
		t.Errorf("Field = %q, want projected secret field", rels[0].Field)
	}
}

// ============================================================
// Non-workload / guard tests
// ============================================================

func TestVolumeDetector_NonWorkload(t *testing.T) {
	// ConfigMap kind is not a workload; Detect must return no relationships.
	cm := makeProcessedResource("v1", "ConfigMap", "some-config", "default",
		nil, nil,
		map[string]interface{}{
			"data": map[string]interface{}{
				"key": "value",
			},
		})

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		cm.Original.ResourceKey(): cm,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), cm, allResources)

	if len(rels) != 0 {
		t.Errorf("expected 0 relationships for non-workload ConfigMap, got %d", len(rels))
	}
}

func TestVolumeDetector_MissingTarget(t *testing.T) {
	// Deployment references a ConfigMap by volume, but that ConfigMap is NOT in allResources.
	// No relationship should be created.
	deployment := makeProcessedResource("apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{
						map[string]interface{}{
							"name": "config-vol",
							"configMap": map[string]interface{}{
								"name": "missing-config",
							},
						},
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
						},
					},
				},
			},
		})

	// Only the deployment itself is in allResources; ConfigMap is absent.
	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), deployment, allResources)

	if len(rels) != 0 {
		t.Errorf("expected 0 relationships when target ConfigMap is missing from allResources, got %d", len(rels))
	}
}

// ============================================================
// CronJob / Pod paths for envFrom and env valueFrom
// ============================================================

// TestVolumeDetector_CronJobEnvFrom verifies that a CronJob with envFrom configMapRef in
// the pod template spec produces an env_from relationship.
func TestVolumeDetector_CronJobEnvFrom(t *testing.T) {
	cronJob := makeProcessedResource("batch/v1", "CronJob", "my-cronjob", "default",
		nil, nil,
		map[string]interface{}{
			"jobTemplate": map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"volumes": []interface{}{},
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "job",
									"image": "job:latest",
									"envFrom": []interface{}{
										map[string]interface{}{
											"configMapRef": map[string]interface{}{
												"name": "cron-env-config",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		})

	cmKey := resourceKey("v1", "ConfigMap", "default", "cron-env-config")
	cm := makeProcessedResource("v1", "ConfigMap", "cron-env-config", "default", nil, nil, nil)

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		cronJob.Original.ResourceKey(): cronJob,
		cmKey:                          cm,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), cronJob, allResources)

	found := false
	for _, rel := range rels {
		if rel.Type == types.RelationEnvFrom && rel.To == cmKey {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected env_from relationship from CronJob to ConfigMap, got: %v", rels)
	}
}

// TestVolumeDetector_PodEnvFrom verifies that a Pod with envFrom secretRef produces
// an env_from relationship.
func TestVolumeDetector_PodEnvFrom(t *testing.T) {
	pod := makeProcessedResource("v1", "Pod", "my-pod", "default",
		nil, nil,
		map[string]interface{}{
			"volumes": []interface{}{},
			"containers": []interface{}{
				map[string]interface{}{
					"name":  "app",
					"image": "app:latest",
					"envFrom": []interface{}{
						map[string]interface{}{
							"secretRef": map[string]interface{}{
								"name": "pod-secrets",
							},
						},
					},
				},
			},
		})

	secretKey := resourceKey("v1", "Secret", "default", "pod-secrets")
	secret := makeProcessedResource("v1", "Secret", "pod-secrets", "default", nil, nil, nil)

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		pod.Original.ResourceKey(): pod,
		secretKey:                  secret,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), pod, allResources)

	found := false
	for _, rel := range rels {
		if rel.Type == types.RelationEnvFrom && rel.To == secretKey {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected env_from relationship from Pod to Secret, got: %v", rels)
	}
}

// TestVolumeDetector_CronJobEnvValueFrom verifies that a CronJob with
// env[].valueFrom.configMapKeyRef produces an env_value_from relationship.
func TestVolumeDetector_CronJobEnvValueFrom(t *testing.T) {
	cronJob := makeProcessedResource("batch/v1", "CronJob", "my-cronjob", "default",
		nil, nil,
		map[string]interface{}{
			"jobTemplate": map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"volumes": []interface{}{},
							"containers": []interface{}{
								map[string]interface{}{
									"name":  "job",
									"image": "job:latest",
									"env": []interface{}{
										map[string]interface{}{
											"name": "DB_HOST",
											"valueFrom": map[string]interface{}{
												"configMapKeyRef": map[string]interface{}{
													"name": "db-config",
													"key":  "host",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		})

	cmKey := resourceKey("v1", "ConfigMap", "default", "db-config")
	cm := makeProcessedResource("v1", "ConfigMap", "db-config", "default", nil, nil, nil)

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		cronJob.Original.ResourceKey(): cronJob,
		cmKey:                          cm,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), cronJob, allResources)

	found := false
	for _, rel := range rels {
		if rel.Type == types.RelationEnvValueFrom && rel.To == cmKey {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected env_value_from relationship from CronJob to ConfigMap, got: %v", rels)
	}
}

// TestVolumeDetector_PodEnvValueFrom verifies that a Pod with
// env[].valueFrom.secretKeyRef produces an env_value_from relationship.
func TestVolumeDetector_PodEnvValueFrom(t *testing.T) {
	pod := makeProcessedResource("v1", "Pod", "my-pod", "default",
		nil, nil,
		map[string]interface{}{
			"volumes": []interface{}{},
			"containers": []interface{}{
				map[string]interface{}{
					"name":  "app",
					"image": "app:latest",
					"env": []interface{}{
						map[string]interface{}{
							"name": "API_KEY",
							"valueFrom": map[string]interface{}{
								"secretKeyRef": map[string]interface{}{
									"name": "api-secret",
									"key":  "key",
								},
							},
						},
					},
				},
			},
		})

	secretKey := resourceKey("v1", "Secret", "default", "api-secret")
	secret := makeProcessedResource("v1", "Secret", "api-secret", "default", nil, nil, nil)

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		pod.Original.ResourceKey(): pod,
		secretKey:                  secret,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), pod, allResources)

	found := false
	for _, rel := range rels {
		if rel.Type == types.RelationEnvValueFrom && rel.To == secretKey {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected env_value_from relationship from Pod to Secret, got: %v", rels)
	}
}

// TestVolumeDetector_EnvFromMalformedContainers verifies that malformed container
// entries (non-map container, no envFrom field, non-slice envFrom, non-map envFrom
// source) are handled gracefully without panicking and produce no relationships.
func TestVolumeDetector_EnvFromMalformedContainers(t *testing.T) {
	deployment := makeProcessedResource("apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{},
					"containers": []interface{}{
						"not-a-map-container",
						map[string]interface{}{
							"name":  "no-envfrom",
							"image": "app:latest",
							// no envFrom field
						},
						map[string]interface{}{
							"name":    "bad-envfrom-type",
							"image":   "app:latest",
							"envFrom": "not-a-slice",
						},
						map[string]interface{}{
							"name":  "bad-envfrom-entry",
							"image": "app:latest",
							"envFrom": []interface{}{
								"not-a-map-entry",
							},
						},
					},
				},
			},
		})

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), deployment, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationEnvFrom {
			t.Errorf("expected no env_from relationships for malformed containers, got: %v", rel)
		}
	}
}

// TestVolumeDetector_EnvValueFromMalformedContainers verifies that malformed env[]
// entries (non-map env entry, no valueFrom field, non-map valueFrom) are handled
// gracefully without panicking and produce no env_value_from relationships.
func TestVolumeDetector_EnvValueFromMalformedContainers(t *testing.T) {
	deployment := makeProcessedResource("apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
							"env": []interface{}{
								"not-a-map-env-entry",
								map[string]interface{}{
									"name": "NO_VALUE_FROM",
									// no valueFrom key
								},
								map[string]interface{}{
									"name":      "BAD_VALUE_FROM",
									"valueFrom": "not-a-map",
								},
							},
						},
					},
				},
			},
		})

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), deployment, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationEnvValueFrom {
			t.Errorf("expected no env_value_from relationships for malformed env entries, got: %v", rel)
		}
	}
}

// TestVolumeDetector_MalformedVolumeEntries verifies that non-map volume entries in the
// volumes slice are skipped without panicking.
func TestVolumeDetector_MalformedVolumeEntries(t *testing.T) {
	deployment := makeProcessedResource("apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{
						"not-a-map-volume",
						map[string]interface{}{
							"name":     "empty-vol",
							"emptyDir": map[string]interface{}{},
						},
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
						},
					},
				},
			},
		})

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), deployment, allResources)

	if len(rels) != 0 {
		t.Errorf("expected 0 relationships for malformed volume entries, got %d", len(rels))
	}
}

// ============================================================
// Additional edge case coverage tests
// ============================================================

// TestVolumeDetector_JobKindNoVolumes verifies that a Job workload with no volumes
// field returns early from Detect without panicking (covers the !found early return
// in the volumes lookup for the else-branch kind path).
func TestVolumeDetector_JobKindNoVolumes(t *testing.T) {
	job := makeProcessedResource("batch/v1", "Job", "my-job", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					// no "volumes" key — triggers !found early return
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "worker",
							"image": "worker:latest",
						},
					},
				},
			},
		})

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		job.Original.ResourceKey(): job,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), job, allResources)

	if len(rels) != 0 {
		t.Errorf("expected 0 relationships for Job with no volumes, got %d", len(rels))
	}
}

// TestVolumeDetector_DaemonSetNoVolumes verifies that a DaemonSet with no volumes
// field returns early from Detect (covers !found path for DaemonSet/StatefulSet/Job kind).
func TestVolumeDetector_DaemonSetNoVolumes(t *testing.T) {
	ds := makeProcessedResource("apps/v1", "DaemonSet", "my-ds", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					// no "volumes" key
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "agent",
							"image": "agent:latest",
						},
					},
				},
			},
		})

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		ds.Original.ResourceKey(): ds,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), ds, allResources)

	if len(rels) != 0 {
		t.Errorf("expected 0 relationships for DaemonSet with no volumes, got %d", len(rels))
	}
}

// TestVolumeDetector_DeploymentVolumesButNoContainers verifies that a Deployment with
// a volumes field (found=true) but no containers key in spec.template.spec causes
// detectEnvFromReferences and detectEnvValueFromReferences to return early via their
// own !found guard.
func TestVolumeDetector_DeploymentVolumesButNoContainers(t *testing.T) {
	deployment := makeProcessedResource("apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{
						map[string]interface{}{
							"name":     "data",
							"emptyDir": map[string]interface{}{},
						},
					},
					// no "containers" key — triggers !found in detectEnvFromReferences
					// and detectEnvValueFromReferences
				},
			},
		})

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), deployment, allResources)

	for _, rel := range rels {
		if rel.Type == types.RelationEnvFrom || rel.Type == types.RelationEnvValueFrom {
			t.Errorf("expected no env relationships when no containers key present, got: %v", rel)
		}
	}
}

// TestVolumeDetector_ProjectedVolumeNonMapSource verifies that a projected volume whose
// "sources" field is a valid []interface{} but contains a non-map element (e.g., a
// string) causes the inner `sourceMap, ok := source.(map[string]interface{})` type
// assertion to fail, triggering the `if !ok { continue }` guard at line 157-159 of
// volume.go. The non-map source must be skipped gracefully.
func TestVolumeDetector_ProjectedVolumeNonMapSource(t *testing.T) {
	deployment := makeProcessedResource("apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{
						map[string]interface{}{
							"name": "projected-vol",
							"projected": map[string]interface{}{
								// sources IS a []interface{} (outer assertion passes),
								// but contains a non-map entry first — triggers !ok continue.
								// The second entry is a valid map but has no configMap/secret
								// so no relationship is produced.
								"sources": []interface{}{
									"not-a-map-source",
									map[string]interface{}{
										"serviceAccountToken": map[string]interface{}{
											"expirationSeconds": int64(3600),
										},
									},
								},
							},
						},
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
						},
					},
				},
			},
		})

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), deployment, allResources)

	if len(rels) != 0 {
		t.Errorf("expected 0 relationships for projected volume with non-map source entry, got %d: %v", len(rels), rels)
	}
}

// TestVolumeDetector_ProjectedVolumeNonSliceSources verifies that a projected volume
// whose "sources" field is not a []interface{} (e.g., is a string) is skipped without
// panicking. This covers the outer `if sources, ok := projectedMap["sources"].([]interface{}); ok`
// assertion failing.
func TestVolumeDetector_ProjectedVolumeNonSliceSources(t *testing.T) {
	deployment := makeProcessedResource("apps/v1", "Deployment", "my-deploy", "default",
		nil, nil,
		map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{
						map[string]interface{}{
							"name": "projected-vol",
							"projected": map[string]interface{}{
								// "sources" is a string, not a []interface{} —
								// the outer type assertion `.([]interface{})` fails gracefully.
								"sources": "not-a-slice",
							},
						},
					},
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "app",
							"image": "app:latest",
						},
					},
				},
			},
		})

	allResources := map[types.ResourceKey]*types.ProcessedResource{
		deployment.Original.ResourceKey(): deployment,
	}

	d := NewVolumeMountDetector()
	rels := d.Detect(context.Background(), deployment, allResources)

	if len(rels) != 0 {
		t.Errorf("expected 0 relationships for projected volume with non-slice sources, got %d: %v", len(rels), rels)
	}
}
