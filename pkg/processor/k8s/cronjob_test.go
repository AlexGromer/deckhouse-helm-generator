package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Task 4.4: CronJob Processor Tests (TDD)
// ============================================================

func makeCronJobObj(name, namespace string, labels map[string]interface{}, spec map[string]interface{}) *unstructured.Unstructured {
	metadata := map[string]interface{}{
		"name":      name,
		"namespace": namespace,
	}
	if labels != nil {
		metadata["labels"] = labels
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "CronJob",
			"metadata":   metadata,
			"spec":       spec,
		},
	}
}

func makeBasicCronJobSpec(schedule string, containerName, image string) map[string]interface{} {
	return map[string]interface{}{
		"schedule": schedule,
		"jobTemplate": map[string]interface{}{
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  containerName,
								"image": image,
							},
						},
						"restartPolicy": "OnFailure",
					},
				},
			},
		},
	}
}

// ============================================================
// Subtask 1: Extract schedule
// ============================================================

func TestProcessCronJob_ExtractsSchedule(t *testing.T) {
	p := NewCronJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicCronJobSpec("0 2 * * *", "backup", "backup:1.0")
	obj := makeCronJobObj("backup-job", "default",
		map[string]interface{}{"app": "backup"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)
	testutil.AssertEqual(t, "0 2 * * *", result.Values["schedule"])
}

// ============================================================
// Subtask 2: Extract timezone
// ============================================================

func TestProcessCronJob_ExtractsTimezone(t *testing.T) {
	p := NewCronJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicCronJobSpec("0 9 * * 1-5", "report", "report:1.0")
	spec["timeZone"] = "America/New_York"

	obj := makeCronJobObj("report-job", "default",
		map[string]interface{}{"app": "report"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "America/New_York", result.Values["timeZone"])
}

// ============================================================
// Subtask 3: Extract concurrencyPolicy
// ============================================================

func TestProcessCronJob_ExtractsConcurrencyPolicy(t *testing.T) {
	p := NewCronJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicCronJobSpec("*/5 * * * *", "sync", "sync:1.0")
	spec["concurrencyPolicy"] = "Forbid"

	obj := makeCronJobObj("sync-job", "default",
		map[string]interface{}{"app": "sync"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "Forbid", result.Values["concurrencyPolicy"])
}

// ============================================================
// Subtask 4: Extract suspend flag
// ============================================================

func TestProcessCronJob_ExtractsSuspend(t *testing.T) {
	p := NewCronJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicCronJobSpec("0 0 * * *", "disabled", "disabled:1.0")
	spec["suspend"] = true

	obj := makeCronJobObj("disabled-job", "default",
		map[string]interface{}{"app": "disabled"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Values["suspend"])
}

// ============================================================
// Subtask 5: Extract successfulJobsHistoryLimit
// ============================================================

func TestProcessCronJob_ExtractsSuccessHistoryLimit(t *testing.T) {
	p := NewCronJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicCronJobSpec("0 3 * * *", "cleaner", "cleaner:1.0")
	spec["successfulJobsHistoryLimit"] = int64(3)

	obj := makeCronJobObj("cleaner-job", "default",
		map[string]interface{}{"app": "cleaner"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, int64(3), result.Values["successfulJobsHistoryLimit"])
}

// ============================================================
// Subtask 6: Extract failedJobsHistoryLimit
// ============================================================

func TestProcessCronJob_ExtractsFailedHistoryLimit(t *testing.T) {
	p := NewCronJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicCronJobSpec("0 4 * * *", "migrator", "migrator:1.0")
	spec["failedJobsHistoryLimit"] = int64(1)

	obj := makeCronJobObj("migrator-job", "default",
		map[string]interface{}{"app": "migrator"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, int64(1), result.Values["failedJobsHistoryLimit"])
}

// ============================================================
// Subtask 7: Extract startingDeadlineSeconds
// ============================================================

func TestProcessCronJob_ExtractsStartingDeadline(t *testing.T) {
	p := NewCronJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicCronJobSpec("*/10 * * * *", "urgent", "urgent:1.0")
	spec["startingDeadlineSeconds"] = int64(300)

	obj := makeCronJobObj("urgent-job", "default",
		map[string]interface{}{"app": "urgent"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, int64(300), result.Values["startingDeadlineSeconds"])
}

// ============================================================
// Subtask 8: Extract jobTemplate spec
// ============================================================

func TestProcessCronJob_ExtractsJobTemplateSpec(t *testing.T) {
	p := NewCronJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicCronJobSpec("0 1 * * *", "processor", "processor:2.0")
	jobSpec := spec["jobTemplate"].(map[string]interface{})["spec"].(map[string]interface{})
	jobSpec["completions"] = int64(5)
	jobSpec["parallelism"] = int64(2)
	jobSpec["backoffLimit"] = int64(3)
	jobSpec["activeDeadlineSeconds"] = int64(600)

	obj := makeCronJobObj("processor-job", "default",
		map[string]interface{}{"app": "processor"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	jobTemplate, ok := result.Values["jobTemplate"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected jobTemplate in values")
	}
	testutil.AssertEqual(t, int64(5), jobTemplate["completions"])
	testutil.AssertEqual(t, int64(2), jobTemplate["parallelism"])
	testutil.AssertEqual(t, int64(3), jobTemplate["backoffLimit"])
	testutil.AssertEqual(t, int64(600), jobTemplate["activeDeadlineSeconds"])
}

// ============================================================
// Subtask 9: Extract pod template from jobTemplate
// ============================================================

func TestProcessCronJob_ExtractsPodTemplate(t *testing.T) {
	p := NewCronJobProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"schedule": "0 0 * * *",
		"jobTemplate": map[string]interface{}{
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "worker",
								"image": "worker:3.0",
								"resources": map[string]interface{}{
									"limits": map[string]interface{}{
										"cpu":    "500m",
										"memory": "256Mi",
									},
								},
							},
						},
						"restartPolicy": "Never",
					},
				},
			},
		},
	}
	obj := makeCronJobObj("worker-cronjob", "default",
		map[string]interface{}{"app": "worker"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	containers, ok := result.Values["containers"].([]map[string]interface{})
	if !ok {
		t.Fatalf("Expected containers to be []map[string]interface{}, got %T", result.Values["containers"])
	}
	testutil.AssertEqual(t, 1, len(containers))
	testutil.AssertEqual(t, "worker", containers[0]["name"])

	restartPolicy, ok := result.Values["restartPolicy"].(string)
	if !ok {
		t.Fatal("Expected restartPolicy in values")
	}
	testutil.AssertEqual(t, "Never", restartPolicy)
}

// ============================================================
// Edge cases and metadata
// ============================================================

func TestProcessCronJob_EdgeCases(t *testing.T) {
	p := NewCronJobProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilCronJob", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil CronJob")
		}
	})

	t.Run("MinimalCronJob", func(t *testing.T) {
		spec := makeBasicCronJobSpec("* * * * *", "minimal", "minimal:1.0")
		obj := makeCronJobObj("minimal-cron", "default",
			map[string]interface{}{"app": "minimal"}, spec)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)
		testutil.AssertEqual(t, "* * * * *", result.Values["schedule"])
	})
}

func TestNewCronJobProcessor(t *testing.T) {
	p := NewCronJobProcessor()
	testutil.AssertEqual(t, "cronjob", p.Name())
	testutil.AssertEqual(t, 100, p.Priority())

	gvks := p.Supports()
	testutil.AssertEqual(t, 1, len(gvks))
	testutil.AssertEqual(t, schema.GroupVersionKind{
		Group: "batch", Version: "v1", Kind: "CronJob",
	}, gvks[0])
}

func TestProcessCronJob_ResultMetadata(t *testing.T) {
	p := NewCronJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicCronJobSpec("0 * * * *", "hourly", "hourly:1.0")
	obj := makeCronJobObj("hourly-cron", "default",
		map[string]interface{}{"app": "hourly"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "hourly", result.ServiceName)
	testutil.AssertEqual(t, "templates/hourly-cronjob.yaml", result.TemplatePath)
	testutil.AssertEqual(t, "services.hourly.cronJob", result.ValuesPath)
	testutil.AssertContains(t, result.TemplateContent, "kind: CronJob")
}

func TestProcessCronJob_GeneratesTemplate(t *testing.T) {
	p := NewCronJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicCronJobSpec("0 2 * * *", "tmpl", "tmpl:1.0")
	spec["concurrencyPolicy"] = "Forbid"
	obj := makeCronJobObj("tmpl-cron", "default",
		map[string]interface{}{"app": "tmpl"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tmpl := result.TemplateContent
	testutil.AssertContains(t, tmpl, "batch/v1")
	testutil.AssertContains(t, tmpl, "CronJob")
	testutil.AssertContains(t, tmpl, "schedule")
	testutil.AssertContains(t, tmpl, "concurrencyPolicy")
	testutil.AssertContains(t, tmpl, "jobTemplate")
	if !strings.Contains(tmpl, "test-chart") {
		t.Error("Expected template to reference chart name")
	}
}
