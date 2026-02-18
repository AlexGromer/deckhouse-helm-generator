package generator

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
)

// allLibraryTemplates collects all template content from the library chart into one string
// to search across all template files regardless of how they are split.
func allLibraryTemplates(libChart *types.GeneratedChart) string {
	var sb strings.Builder
	sb.WriteString(libChart.Helpers)
	for _, content := range libChart.Templates {
		sb.WriteString(content)
	}
	return sb.String()
}

// workloadKinds are the K8s workload types that must reuse shared blocks.
var workloadKinds = []string{"deployment", "statefulset", "daemonset", "job", "cronjob"}

func libChartForDRY(t *testing.T) *types.GeneratedChart {
	t.Helper()
	graph := buildGraph(nil, nil)
	gen := NewLibraryGenerator()
	charts, err := gen.Generate(context.Background(), graph, Options{ChartVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	lib := findLibraryChart(charts)
	if lib == nil {
		t.Fatal("library chart not found")
	}
	return lib
}

// ============================================================
// Subtask 1: Shared resources block
// ============================================================

func TestDRYTemplates_SharedBlock_Resources(t *testing.T) {
	lib := libChartForDRY(t)
	all := allLibraryTemplates(lib)

	// Define must exist
	if !strings.Contains(all, `define "library.resources"`) {
		t.Error("library.resources define not found in library templates")
	}

	// All workload templates must call it
	for _, kind := range []string{"deployment", "statefulset", "daemonset", "job", "cronjob"} {
		key := fmt.Sprintf("templates/_%s.tpl", kind)
		tmpl, ok := lib.Templates[key]
		if !ok {
			t.Errorf("template file %s not found", key)
			continue
		}
		if !strings.Contains(tmpl, `include "library.resources"`) {
			t.Errorf("%s template does not include library.resources", kind)
		}
	}
}

// ============================================================
// Subtask 2: Shared probes block
// ============================================================

func TestDRYTemplates_SharedBlock_Probes(t *testing.T) {
	lib := libChartForDRY(t)
	all := allLibraryTemplates(lib)

	// Define must exist
	if !strings.Contains(all, `define "library.probes"`) {
		t.Error("library.probes define not found in library templates")
	}

	// Deployment and StatefulSet must include probes
	for _, kind := range []string{"deployment", "statefulset"} {
		key := fmt.Sprintf("templates/_%s.tpl", kind)
		tmpl, ok := lib.Templates[key]
		if !ok {
			t.Errorf("template file %s not found", key)
			continue
		}
		if !strings.Contains(tmpl, `include "library.probes"`) {
			t.Errorf("%s template does not include library.probes", kind)
		}
	}
}

// ============================================================
// Subtask 3: Shared securityContext block
// ============================================================

func TestDRYTemplates_SharedBlock_SecurityContext(t *testing.T) {
	lib := libChartForDRY(t)
	all := allLibraryTemplates(lib)

	if !strings.Contains(all, `define "library.securityContext"`) {
		t.Error("library.securityContext define not found")
	}
	if !strings.Contains(all, `define "library.containerSecurityContext"`) {
		t.Error("library.containerSecurityContext define not found")
	}
}

// ============================================================
// Subtask 4: Shared env block
// ============================================================

func TestDRYTemplates_SharedBlock_Env(t *testing.T) {
	lib := libChartForDRY(t)
	all := allLibraryTemplates(lib)

	if !strings.Contains(all, `define "library.env"`) {
		t.Error("library.env define not found")
	}

	// All workload types must include env
	for _, kind := range workloadKinds {
		key := fmt.Sprintf("templates/_%s.tpl", kind)
		tmpl, ok := lib.Templates[key]
		if !ok {
			t.Errorf("template file %s not found", key)
			continue
		}
		if !strings.Contains(tmpl, `include "library.env"`) {
			t.Errorf("%s template does not include library.env", kind)
		}
	}
}

// ============================================================
// Subtask 5: Shared volumeMounts block
// ============================================================

func TestDRYTemplates_SharedBlock_VolumeMounts(t *testing.T) {
	lib := libChartForDRY(t)
	all := allLibraryTemplates(lib)

	if !strings.Contains(all, `define "library.volumeMounts"`) {
		t.Error("library.volumeMounts define not found")
	}
	if !strings.Contains(all, `define "library.volumes"`) {
		t.Error("library.volumes define not found")
	}
}

// ============================================================
// Subtask 6: Shared labels and annotations blocks
// ============================================================

func TestDRYTemplates_SharedBlock_Labels(t *testing.T) {
	lib := libChartForDRY(t)
	all := allLibraryTemplates(lib)

	if !strings.Contains(all, `define "library.labels"`) {
		t.Error("library.labels define not found")
	}
	if !strings.Contains(all, `define "library.annotations"`) {
		t.Error("library.annotations define not found")
	}
}
