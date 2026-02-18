package integration

import (
	"os"
	"strings"
	"testing"
)

// ============================================================
// Task 4.6 Subtask 4: Pipeline with CronJob
// ============================================================

func TestPipeline_CronJobWithConfigMap(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("configmap.yaml", `apiVersion: v1
kind: ConfigMap
metadata:
  name: backup-config
  namespace: default
  labels:
    app.kubernetes.io/name: backup
data:
  backup.conf: |
    retention: 30
    compression: gzip
`)

	h.WriteInputFile("cronjob.yaml", `apiVersion: batch/v1
kind: CronJob
metadata:
  name: nightly-backup
  namespace: default
  labels:
    app.kubernetes.io/name: backup
spec:
  schedule: "0 2 * * *"
  concurrencyPolicy: Forbid
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 1
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: backup
              image: backup-tool:1.0
              command: ["/backup.sh"]
              volumeMounts:
                - name: config
                  mountPath: /etc/backup
          volumes:
            - name: config
              configMap:
                name: backup-config
          restartPolicy: OnFailure
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "backup-app",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	if len(output.Resources) < 2 {
		t.Fatalf("Expected at least 2 resources (CronJob + ConfigMap), got %d", len(output.Resources))
	}

	foundCronJob := false
	for _, res := range output.Resources {
		if res.Original.Object.GetKind() == "CronJob" {
			foundCronJob = true

			if res.TemplateContent == "" {
				t.Error("Expected non-empty CronJob template")
			}
			if !strings.Contains(res.TemplateContent, "CronJob") {
				t.Error("Expected CronJob kind in template")
			}
			if !strings.Contains(res.TemplateContent, "schedule") {
				t.Error("Expected schedule in CronJob template")
			}

			// Values should contain CronJob config
			if res.Values["schedule"] == nil {
				t.Error("Expected schedule in CronJob values")
			}
			if res.Values["concurrencyPolicy"] == nil {
				t.Error("Expected concurrencyPolicy in CronJob values")
			}

			break
		}
	}
	if !foundCronJob {
		t.Error("CronJob resource not found in pipeline output")
	}
}

// ============================================================
// Task 4.6 Subtask 5: Pipeline with Job
// ============================================================

func TestPipeline_JobWithSecret(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("secret.yaml", `apiVersion: v1
kind: Secret
metadata:
  name: db-credentials
  namespace: default
  labels:
    app.kubernetes.io/name: migration
type: Opaque
data:
  DB_HOST: cG9zdGdyZXM=
  DB_PASSWORD: c2VjcmV0MTIz
`)

	h.WriteInputFile("job.yaml", `apiVersion: batch/v1
kind: Job
metadata:
  name: db-migration
  namespace: default
  labels:
    app.kubernetes.io/name: migration
spec:
  completions: 1
  backoffLimit: 3
  ttlSecondsAfterFinished: 3600
  template:
    spec:
      containers:
        - name: migrate
          image: migration-tool:2.0
          command: ["./migrate", "up"]
          env:
            - name: DB_HOST
              valueFrom:
                secretKeyRef:
                  name: db-credentials
                  key: DB_HOST
            - name: DB_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: db-credentials
                  key: DB_PASSWORD
      restartPolicy: Never
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "migration-app",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	if len(output.Resources) < 2 {
		t.Fatalf("Expected at least 2 resources (Job + Secret), got %d", len(output.Resources))
	}

	foundJob := false
	for _, res := range output.Resources {
		if res.Original.Object.GetKind() == "Job" {
			foundJob = true

			if res.TemplateContent == "" {
				t.Error("Expected non-empty Job template")
			}
			if !strings.Contains(res.TemplateContent, "Job") {
				t.Error("Expected Job kind in template")
			}
			if !strings.Contains(res.TemplateContent, "restartPolicy") {
				t.Error("Expected restartPolicy in Job template")
			}

			// Values should contain Job config
			if res.Values["completions"] == nil {
				t.Error("Expected completions in Job values")
			}
			if res.Values["backoffLimit"] == nil {
				t.Error("Expected backoffLimit in Job values")
			}
			if res.Values["ttl"] == nil {
				t.Error("Expected ttl in Job values")
			}

			break
		}
	}
	if !foundJob {
		t.Error("Job resource not found in pipeline output")
	}
}

// ============================================================
// Batch scenario: CronJob + Job together
// ============================================================

func TestPipeline_BatchWorkloads(t *testing.T) {
	h := NewTestHarness(t)
	h.Setup()
	t.Cleanup(h.Cleanup)

	h.WriteInputFile("cronjob.yaml", `apiVersion: batch/v1
kind: CronJob
metadata:
  name: hourly-sync
  namespace: default
  labels:
    app.kubernetes.io/name: sync
spec:
  schedule: "0 * * * *"
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: sync
              image: sync-tool:1.0
          restartPolicy: OnFailure
`)

	h.WriteInputFile("job.yaml", `apiVersion: batch/v1
kind: Job
metadata:
  name: initial-setup
  namespace: default
  labels:
    app.kubernetes.io/name: setup
spec:
  completions: 1
  template:
    spec:
      containers:
        - name: setup
          image: setup-tool:1.0
      restartPolicy: Never
`)

	output, err := ExecutePipeline(h.InputDir, PipelineOptions{
		ChartName: "batch-app",
	})
	if err != nil {
		t.Fatalf("Pipeline failed: %v", err)
	}
	defer os.RemoveAll(output.OutputDir)

	if len(output.Resources) < 2 {
		t.Fatalf("Expected at least 2 resources, got %d", len(output.Resources))
	}

	kinds := make(map[string]bool)
	for _, res := range output.Resources {
		kinds[res.Original.Object.GetKind()] = true
	}

	if !kinds["CronJob"] {
		t.Error("Expected CronJob in pipeline output")
	}
	if !kinds["Job"] {
		t.Error("Expected Job in pipeline output")
	}
}
