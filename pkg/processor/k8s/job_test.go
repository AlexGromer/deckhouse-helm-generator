package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/testutil"
)

// ============================================================
// Task 4.5: Job Processor Tests (TDD)
// ============================================================

func makeJobObj(name, namespace string, labels map[string]interface{}, spec map[string]interface{}) *unstructured.Unstructured {
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
			"kind":       "Job",
			"metadata":   metadata,
			"spec":       spec,
		},
	}
}

func makeBasicJobSpec(containerName, image string) map[string]interface{} {
	return map[string]interface{}{
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  containerName,
						"image": image,
					},
				},
				"restartPolicy": "Never",
			},
		},
	}
}

// ============================================================
// Subtask 1: Extract completions
// ============================================================

func TestProcessJob_ExtractsCompletions(t *testing.T) {
	p := NewJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicJobSpec("worker", "worker:1.0")
	spec["completions"] = int64(5)

	obj := makeJobObj("batch-job", "default",
		map[string]interface{}{"app": "batch"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Processed)
	testutil.AssertEqual(t, int64(5), result.Values["completions"])
}

// ============================================================
// Subtask 2: Extract parallelism
// ============================================================

func TestProcessJob_ExtractsParallelism(t *testing.T) {
	p := NewJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicJobSpec("parallel", "parallel:1.0")
	spec["parallelism"] = int64(3)

	obj := makeJobObj("parallel-job", "default",
		map[string]interface{}{"app": "parallel"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, int64(3), result.Values["parallelism"])
}

// ============================================================
// Subtask 3: Extract backoffLimit
// ============================================================

func TestProcessJob_ExtractsBackoffLimit(t *testing.T) {
	p := NewJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicJobSpec("retry", "retry:1.0")
	spec["backoffLimit"] = int64(4)

	obj := makeJobObj("retry-job", "default",
		map[string]interface{}{"app": "retry"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, int64(4), result.Values["backoffLimit"])
}

// ============================================================
// Subtask 4: Extract activeDeadlineSeconds
// ============================================================

func TestProcessJob_ExtractsActiveDeadlineSeconds(t *testing.T) {
	p := NewJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicJobSpec("timed", "timed:1.0")
	spec["activeDeadlineSeconds"] = int64(600)

	obj := makeJobObj("timed-job", "default",
		map[string]interface{}{"app": "timed"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, int64(600), result.Values["activeDeadlineSeconds"])
}

// ============================================================
// Subtask 5: Extract ttlSecondsAfterFinished → ttl
// ============================================================

func TestProcessJob_ExtractsTTL(t *testing.T) {
	p := NewJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicJobSpec("cleanup", "cleanup:1.0")
	spec["ttlSecondsAfterFinished"] = int64(3600)

	obj := makeJobObj("cleanup-job", "default",
		map[string]interface{}{"app": "cleanup"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, int64(3600), result.Values["ttl"])
}

// ============================================================
// Subtask 6: Extract completionMode
// ============================================================

func TestProcessJob_ExtractsCompletionMode(t *testing.T) {
	p := NewJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicJobSpec("indexed", "indexed:1.0")
	spec["completionMode"] = "Indexed"
	spec["completions"] = int64(10)

	obj := makeJobObj("indexed-job", "default",
		map[string]interface{}{"app": "indexed"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "Indexed", result.Values["completionMode"])
}

// ============================================================
// Subtask 7: Extract suspend flag
// ============================================================

func TestProcessJob_ExtractsSuspend(t *testing.T) {
	p := NewJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicJobSpec("suspended", "suspended:1.0")
	spec["suspend"] = true

	obj := makeJobObj("suspended-job", "default",
		map[string]interface{}{"app": "suspended"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, true, result.Values["suspend"])
}

// ============================================================
// Subtask 8: Extract pod template
// ============================================================

func TestProcessJob_ExtractsPodTemplate(t *testing.T) {
	p := NewJobProcessor()
	ctx := newTestProcessorContext()

	spec := map[string]interface{}{
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "processor",
						"image": "processor:2.0",
						"resources": map[string]interface{}{
							"limits": map[string]interface{}{
								"cpu":    "1",
								"memory": "512Mi",
							},
						},
						"command": []interface{}{"/bin/sh", "-c", "process"},
						"args":    []interface{}{"--verbose"},
					},
				},
				"restartPolicy": "Never",
			},
		},
	}
	obj := makeJobObj("processor-job", "default",
		map[string]interface{}{"app": "processor"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	containers, ok := result.Values["containers"].([]map[string]interface{})
	if !ok {
		t.Fatalf("Expected containers to be []map[string]interface{}, got %T", result.Values["containers"])
	}
	testutil.AssertEqual(t, 1, len(containers))
	testutil.AssertEqual(t, "processor", containers[0]["name"])

	img, ok := containers[0]["image"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected image to be a map")
	}
	testutil.AssertEqual(t, "processor", img["repository"])
	testutil.AssertEqual(t, "2.0", img["tag"])

	if containers[0]["resources"] == nil {
		t.Error("Expected resources to be extracted")
	}
	if containers[0]["command"] == nil {
		t.Error("Expected command to be extracted")
	}
	if containers[0]["args"] == nil {
		t.Error("Expected args to be extracted")
	}

	restartPolicy, ok := result.Values["restartPolicy"].(string)
	if !ok {
		t.Fatal("Expected restartPolicy in values")
	}
	testutil.AssertEqual(t, "Never", restartPolicy)
}

// ============================================================
// Subtask 9: Edge cases
// ============================================================

func TestProcessJob_EdgeCases(t *testing.T) {
	p := NewJobProcessor()
	ctx := newTestProcessorContext()

	t.Run("NilJob", func(t *testing.T) {
		_, err := p.Process(ctx, nil)
		if err == nil {
			t.Error("Expected error for nil Job")
		}
	})

	t.Run("MinimalJob", func(t *testing.T) {
		spec := makeBasicJobSpec("minimal", "minimal:1.0")
		obj := makeJobObj("minimal-job", "default",
			map[string]interface{}{"app": "minimal"}, spec)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)
		// Minimal job: no completions, parallelism, etc. — just containers
		if _, exists := result.Values["completions"]; exists {
			t.Error("Minimal job should not have completions")
		}
		if _, exists := result.Values["parallelism"]; exists {
			t.Error("Minimal job should not have parallelism")
		}
	})

	t.Run("JobWithoutCompletions", func(t *testing.T) {
		// Job without completions runs until one pod succeeds
		spec := makeBasicJobSpec("single", "single:1.0")
		spec["backoffLimit"] = int64(3)

		obj := makeJobObj("single-run", "default",
			map[string]interface{}{"app": "single"}, spec)

		result, err := p.Process(ctx, obj)
		testutil.AssertNoError(t, err)
		testutil.AssertEqual(t, true, result.Processed)
		testutil.AssertEqual(t, int64(3), result.Values["backoffLimit"])
	})
}

// ============================================================
// Constructor and metadata tests
// ============================================================

func TestNewJobProcessor(t *testing.T) {
	p := NewJobProcessor()
	testutil.AssertEqual(t, "job", p.Name())
	testutil.AssertEqual(t, 100, p.Priority())

	gvks := p.Supports()
	testutil.AssertEqual(t, 1, len(gvks))
	testutil.AssertEqual(t, schema.GroupVersionKind{
		Group: "batch", Version: "v1", Kind: "Job",
	}, gvks[0])
}

func TestProcessJob_ResultMetadata(t *testing.T) {
	p := NewJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicJobSpec("migrate", "migrate:1.0")
	obj := makeJobObj("migrate-job", "default",
		map[string]interface{}{"app": "migrate"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "migrate", result.ServiceName)
	testutil.AssertEqual(t, "templates/migrate-job.yaml", result.TemplatePath)
	testutil.AssertEqual(t, "services.migrate.job", result.ValuesPath)
	testutil.AssertContains(t, result.TemplateContent, "kind: Job")
}

func TestProcessJob_GeneratesTemplate(t *testing.T) {
	p := NewJobProcessor()
	ctx := newTestProcessorContext()

	spec := makeBasicJobSpec("tmpl", "tmpl:1.0")
	spec["completions"] = int64(3)
	spec["backoffLimit"] = int64(2)
	obj := makeJobObj("tmpl-job", "default",
		map[string]interface{}{"app": "tmpl"}, spec)

	result, err := p.Process(ctx, obj)
	testutil.AssertNoError(t, err)

	tmpl := result.TemplateContent
	testutil.AssertContains(t, tmpl, "batch/v1")
	testutil.AssertContains(t, tmpl, "Job")
	testutil.AssertContains(t, tmpl, "completions")
	testutil.AssertContains(t, tmpl, "backoffLimit")
	testutil.AssertContains(t, tmpl, "restartPolicy")
	if !strings.Contains(tmpl, "test-chart") {
		t.Error("Expected template to reference chart name")
	}
}
