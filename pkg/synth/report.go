package synth

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Report renders SYNTHESIS.md: where the manifests came from, which objects
// were built and what has to be completed by hand.
func Report(sources []string, objects []*unstructured.Unstructured, notes []string) string {
	var b strings.Builder
	b.WriteString("# Synthesized manifests\n\n")
	b.WriteString("dhg built the Kubernetes objects of this chart from the inputs below rather than reading them from manifests. ")
	b.WriteString("Only facts found in the inputs were used; review the notes before deploying.\n\n")

	b.WriteString("## Inputs\n\n")
	for _, s := range sources {
		fmt.Fprintf(&b, "- %s\n", s)
	}

	b.WriteString("\n## Objects\n\n| Kind | Name |\n|---|---|\n")
	rows := make([]string, 0, len(objects))
	for _, o := range objects {
		rows = append(rows, fmt.Sprintf("| %s | %s |", o.GetKind(), o.GetName()))
	}
	sort.Strings(rows)
	for _, r := range rows {
		b.WriteString(r + "\n")
	}

	b.WriteString("\n## To complete by hand\n\n")
	b.WriteString("- Resource requests and limits: not derived, set them in values.yaml for each workload.\n")
	b.WriteString("- Ingress, TLS and external exposure: not derived.\n")
	if len(notes) > 0 {
		b.WriteString("\n## Notes\n\n")
		for _, n := range notes {
			fmt.Fprintf(&b, "- %s\n", n)
		}
	}
	return b.String()
}
