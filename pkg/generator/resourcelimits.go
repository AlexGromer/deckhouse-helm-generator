package generator

import (
	"fmt"
	"strings"
)

// ResourceProfile defines CPU and memory requests/limits for a workload type.
type ResourceProfile struct {
	CPURequest    string
	CPULimit      string
	MemoryRequest string
	MemoryLimit   string
}

// resourceProfiles maps workload types to their default resource profiles.
var resourceProfiles = map[WorkloadType]ResourceProfile{
	WorkloadWeb:      {CPURequest: "100m", CPULimit: "500m", MemoryRequest: "128Mi", MemoryLimit: "512Mi"},
	WorkloadWorker:   {CPURequest: "250m", CPULimit: "1", MemoryRequest: "256Mi", MemoryLimit: "1Gi"},
	WorkloadDatabase: {CPURequest: "500m", CPULimit: "2", MemoryRequest: "512Mi", MemoryLimit: "4Gi"},
	WorkloadBatch:    {CPURequest: "100m", CPULimit: "500m", MemoryRequest: "128Mi", MemoryLimit: "512Mi"},
	WorkloadCache:    {CPURequest: "100m", CPULimit: "250m", MemoryRequest: "64Mi", MemoryLimit: "256Mi"},
}

// injectResources adds a resources block after the image: line in a template.
func injectResources(content string, profile ResourceProfile) string {
	lines := strings.Split(content, "\n")
	var result []string
	injected := false

	for _, line := range lines {
		result = append(result, line)
		if !injected && strings.Contains(line, "image:") {
			indent := leadingSpaces(line)
			block := fmt.Sprintf(
				"%sresources:\n"+
					"%s  requests:\n"+
					"%s    cpu: %q\n"+
					"%s    memory: %q\n"+
					"%s  limits:\n"+
					"%s    cpu: %q\n"+
					"%s    memory: %q",
				indent,
				indent, indent, profile.CPURequest,
				indent, profile.MemoryRequest,
				indent, indent, profile.CPULimit,
				indent, profile.MemoryLimit,
			)
			result = append(result, block)
			injected = true
		}
	}

	return strings.Join(result, "\n")
}
