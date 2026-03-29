package generator

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// CostRegion is a cloud region identifier string.
type CostRegion = string

// CostUnit controls whether costs are reported hourly or monthly.
type CostUnit string

const (
	CostUnitHourly  CostUnit = "hourly"
	CostUnitMonthly CostUnit = "monthly"
)

// hoursPerMonth is the standard billing constant (730 hours).
const hoursPerMonth = 730.0

// defaultCPUMillicores is applied when no CPU request is found.
const defaultCPUMillicores = 100

// defaultMemoryMiB is applied when no memory request is found.
const defaultMemoryMiB = 128

// providerPricing holds per-millicore and per-MiB per-hour rates.
type providerPricing struct {
	CPUPerMillicorePerHour float64 // USD per millicore-hour
	MemPerMiBPerHour       float64 // USD per MiB-hour
	StoragePerGiBPerMonth  float64 // USD per GiB-month
}

// cloudPrices contains realistic approximate pricing for each provider.
// AWS: ~$0.048/vCPU-hour, ~$0.006/GB-hour for on-demand (us-east-1).
// GCP: ~$0.044/vCPU-hour, ~$0.006/GB-hour (us-central1).
// Azure: ~$0.052/vCPU-hour, ~$0.007/GB-hour (eastus).
var cloudPrices = map[CloudProvider]providerPricing{
	CloudProviderAWS: {
		CPUPerMillicorePerHour: 0.048 / 1000.0,
		MemPerMiBPerHour:       0.006 / 1024.0,
		StoragePerGiBPerMonth:  0.10,
	},
	CloudProviderGCP: {
		CPUPerMillicorePerHour: 0.044 / 1000.0,
		MemPerMiBPerHour:       0.006 / 1024.0,
		StoragePerGiBPerMonth:  0.08,
	},
	CloudProviderAzure: {
		CPUPerMillicorePerHour: 0.052 / 1000.0,
		MemPerMiBPerHour:       0.007 / 1024.0,
		StoragePerGiBPerMonth:  0.095,
	},
}

// defaultProviderRegions maps each provider to its default region.
var defaultProviderRegions = map[CloudProvider]string{
	CloudProviderAWS:   "us-east-1",
	CloudProviderGCP:   "us-central1",
	CloudProviderAzure: "eastus",
}

// CostEstimateOptions configures a cost estimation run.
type CostEstimateOptions struct {
	Provider       CloudProvider
	Region         CostRegion
	Unit           CostUnit
	IncludeStorage bool
}

// WorkloadCostEstimate holds per-workload cost breakdown.
type WorkloadCostEstimate struct {
	Name       string
	Namespace  string
	Kind       string
	Replicas   int
	CPUCost    float64
	MemoryCost float64
	TotalCost  float64
	Warnings   []string
}

// CostEstimateReport holds the full cost estimation result.
type CostEstimateReport struct {
	Provider         CloudProvider
	Region           CostRegion
	Unit             CostUnit
	Workloads        []WorkloadCostEstimate
	TotalStorageCost float64
	GrandTotal       float64
}

// parseResourceQuantity parses a Kubernetes quantity string.
// If isCPU is true, returns millicores; otherwise returns MiB.
// Supports: "500m", "2" (CPU cores), "256Mi", "1Gi", "512M", "1G".
func parseResourceQuantity(s string, isCPU bool) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty quantity string")
	}

	if isCPU {
		// CPU: suffix "m" = millicores, no suffix = cores
		if strings.HasSuffix(s, "m") {
			val, err := strconv.ParseFloat(strings.TrimSuffix(s, "m"), 64)
			if err != nil {
				return 0, fmt.Errorf("invalid CPU quantity %q: %w", s, err)
			}
			return int64(val), nil
		}
		// Whole cores
		val, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid CPU quantity %q: %w", s, err)
		}
		return int64(val * 1000), nil
	}

	// Memory: parse suffixes Ki, Mi, Gi, K, M, G
	suffixes := []struct {
		suffix string
		factor float64
	}{
		{"Ki", 1.0 / 1024.0}, // KiB → MiB: divide by 1024
		{"Mi", 1.0},
		{"Gi", 1024.0},
		{"K", 1.0 / 1000.0}, // KB → MiB (approx)
		{"M", 1.0},           // MB ≈ MiB (close enough for cost)
		{"G", 1000.0},        // GB → MiB
	}

	for _, sf := range suffixes {
		if strings.HasSuffix(s, sf.suffix) {
			numStr := strings.TrimSuffix(s, sf.suffix)
			val, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid memory quantity %q: %w", s, err)
			}
			return int64(val * sf.factor), nil
		}
	}

	// Plain bytes
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory quantity %q: %w", s, err)
	}
	return int64(val / (1024 * 1024)), nil
}

// extractReplicasFromResource reads the replica/parallelism count from a workload object.
func extractReplicasFromResource(obj *unstructured.Unstructured) int {
	kind := obj.GetKind()
	switch kind {
	case "Job":
		v, found, err := unstructured.NestedInt64(obj.Object, "spec", "parallelism")
		if err == nil && found && v > 0 {
			return int(v)
		}
		return 1
	case "CronJob":
		return 1
	default:
		v, found, err := unstructured.NestedInt64(obj.Object, "spec", "replicas")
		if err == nil && found && v > 0 {
			return int(v)
		}
		return 1
	}
}

// extractContainersFromObj returns the containers slice for the workload kind.
func extractContainersFromObj(obj *unstructured.Unstructured) ([]interface{}, bool) {
	kind := obj.GetKind()
	var path []string
	switch kind {
	case "CronJob":
		path = []string{"spec", "jobTemplate", "spec", "template", "spec", "containers"}
	default:
		path = []string{"spec", "template", "spec", "containers"}
	}
	containers, found, err := unstructured.NestedSlice(obj.Object, path...)
	if err != nil || !found {
		return nil, false
	}
	return containers, true
}

// isWorkloadKind returns true for Kubernetes workload kinds.
func isWorkloadKind(kind string) bool {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob":
		return true
	}
	return false
}

// estimateWorkloadCost computes a WorkloadCostEstimate for a single workload resource.
func estimateWorkloadCost(r *types.ProcessedResource, pricing providerPricing, unit CostUnit) WorkloadCostEstimate {
	obj := r.Original.Object
	est := WorkloadCostEstimate{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Kind:      obj.GetKind(),
		Replicas:  extractReplicasFromResource(obj),
	}

	containers, ok := extractContainersFromObj(obj)
	if !ok || len(containers) == 0 {
		est.Warnings = append(est.Warnings, "no containers found; using default resource requests")
		containers = []interface{}{map[string]interface{}{}}
	}

	var totalCPUMillis int64
	var totalMemMiB int64

	for _, cRaw := range containers {
		c, ok := cRaw.(map[string]interface{})
		if !ok {
			continue
		}

		requests, _, _ := unstructured.NestedStringMap(c, "resources", "requests")
		cpuStr := requests["cpu"]
		memStr := requests["memory"]

		// Check if there are any requests at all.
		if cpuStr == "" && memStr == "" {
			est.Warnings = append(est.Warnings, "container has no resource requests; using default values")
		}

		// Parse CPU.
		cpuMillis := int64(defaultCPUMillicores)
		if cpuStr != "" {
			v, err := parseResourceQuantity(cpuStr, true)
			if err != nil {
				est.Warnings = append(est.Warnings, fmt.Sprintf("could not parse CPU request %q: %v; using default", cpuStr, err))
			} else {
				cpuMillis = v
			}
		}

		// Parse memory.
		memMiB := int64(defaultMemoryMiB)
		if memStr != "" {
			v, err := parseResourceQuantity(memStr, false)
			if err != nil {
				est.Warnings = append(est.Warnings, fmt.Sprintf("could not parse memory request %q: %v; using default", memStr, err))
			} else {
				memMiB = v
			}
		}

		totalCPUMillis += cpuMillis
		totalMemMiB += memMiB
	}

	// Compute per-replica hourly costs
	cpuCostPerReplicaHourly := float64(totalCPUMillis) * pricing.CPUPerMillicorePerHour
	memCostPerReplicaHourly := float64(totalMemMiB) * pricing.MemPerMiBPerHour

	multiplier := 1.0
	if unit == CostUnitMonthly {
		multiplier = hoursPerMonth
	}

	est.CPUCost = cpuCostPerReplicaHourly * float64(est.Replicas) * multiplier
	est.MemoryCost = memCostPerReplicaHourly * float64(est.Replicas) * multiplier
	est.TotalCost = est.CPUCost + est.MemoryCost

	return est
}

// GenerateCostEstimate computes cost estimates for all workloads in the resource graph.
func GenerateCostEstimate(graph *types.ResourceGraph, opts CostEstimateOptions) *CostEstimateReport {
	// Apply default region if empty.
	region := opts.Region
	if region == "" {
		if def, ok := defaultProviderRegions[opts.Provider]; ok {
			region = def
		} else {
			region = "us-east-1"
		}
	}

	// Resolve pricing.
	pricing, ok := cloudPrices[opts.Provider]
	if !ok {
		pricing = cloudPrices[CloudProviderAWS]
	}

	report := &CostEstimateReport{
		Provider:  opts.Provider,
		Region:    region,
		Unit:      opts.Unit,
		Workloads: []WorkloadCostEstimate{},
	}

	if graph == nil {
		return report
	}

	// Process workloads.
	for _, r := range graph.Resources {
		kind := r.Original.GVK.Kind
		if !isWorkloadKind(kind) {
			continue
		}
		est := estimateWorkloadCost(r, pricing, opts.Unit)
		report.Workloads = append(report.Workloads, est)
		report.GrandTotal += est.TotalCost
	}

	// Process storage if requested.
	if opts.IncludeStorage {
		for _, r := range graph.Resources {
			if r.Original.GVK.Kind != "PersistentVolumeClaim" {
				continue
			}
			obj := r.Original.Object
			// Try annotation first.
			annotations := obj.GetAnnotations()
			storageGiStr := annotations["dhg.deckhouse.io/storage-gi"]
			var storageGi float64
			if storageGiStr != "" {
				v, err := strconv.ParseFloat(storageGiStr, 64)
				if err == nil {
					storageGi = v
				}
			}
			// Fallback to spec.resources.requests.storage.
			if storageGi == 0 {
				storageSt, _, _ := unstructured.NestedString(obj.Object, "spec", "resources", "requests", "storage")
				if storageSt != "" {
					mib, err := parseResourceQuantity(storageSt, false)
					if err == nil {
						storageGi = float64(mib) / 1024.0
					}
				}
			}
			if storageGi > 0 {
				cost := storageGi * pricing.StoragePerGiBPerMonth
				if opts.Unit == CostUnitHourly {
					cost = cost / hoursPerMonth
				}
				report.TotalStorageCost += cost
				report.GrandTotal += cost
			}
		}
	}

	return report
}

// InjectCostNotes injects a cost estimate section into the chart's NOTES.txt.
// Returns a copy of the chart with updated Notes and a boolean indicating whether
// injection occurred (false if notes were already present).
func InjectCostNotes(chart *types.GeneratedChart, report *CostEstimateReport) (*types.GeneratedChart, bool) {
	if chart == nil {
		return nil, false
	}
	if report == nil {
		result := copyChartTemplates(chart)
		return result, false
	}

	const marker = "Cost Estimate"
	if strings.Contains(chart.Notes, marker) {
		result := copyChartTemplates(chart)
		return result, false
	}

	result := copyChartTemplates(chart)

	var sb strings.Builder
	if result.Notes != "" {
		sb.WriteString(result.Notes)
		if !strings.HasSuffix(result.Notes, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Cost Estimate\n")
	sb.WriteString(fmt.Sprintf("Provider: %s | Region: %s | Unit: %s\n\n", report.Provider, report.Region, report.Unit))

	for _, w := range report.Workloads {
		sb.WriteString(fmt.Sprintf("- %s/%s (%s, %d replica(s)): CPU=$%.4f, Mem=$%.4f, Total=$%.4f\n",
			w.Namespace, w.Name, w.Kind, w.Replicas, w.CPUCost, w.MemoryCost, w.TotalCost))
		for _, warn := range w.Warnings {
			sb.WriteString(fmt.Sprintf("  WARNING: %s\n", warn))
		}
	}

	if report.TotalStorageCost > 0 {
		sb.WriteString(fmt.Sprintf("\nStorage: $%.4f\n", report.TotalStorageCost))
	}
	sb.WriteString(fmt.Sprintf("\nGrand Total: $%.4f\n", report.GrandTotal))

	result.Notes = sb.String()
	return result, true
}
