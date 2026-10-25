package generator

import (
	"strings"
)

// pssWorkloadKinds lists Kubernetes workload kinds that contain pod templates.
var pssWorkloadKinds = []string{"Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob"}

// isWorkloadTemplate checks if the template content represents a workload kind.
func isWorkloadTemplate(content string) bool {
	for _, kind := range pssWorkloadKinds {
		if strings.Contains(content, "kind: "+kind) {
			return true
		}
	}
	return false
}

// leadingSpaces returns the whitespace prefix of a string.
func leadingSpaces(s string) string {
	trimmed := strings.TrimLeft(s, " \t")
	return s[:len(s)-len(trimmed)]
}
