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
