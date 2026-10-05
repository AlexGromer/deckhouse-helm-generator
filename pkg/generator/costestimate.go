package generator

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/deckhouse/deckhouse-helm-generator/pkg/types"
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

// cloudPrices contains approximate on-demand list prices per provider.
// AWS: ~$0.048/vCPU-hour, ~$0.006/GiB-hour (us-east-1), gp3 $0.08/GiB-month.
// GCP: ~$0.044/vCPU-hour, ~$0.006/GiB-hour (us-central1), pd-balanced $0.10/GiB-month.
// Azure: ~$0.052/vCPU-hour, ~$0.007/GiB-hour (eastus), Standard SSD ~$0.095/GiB-month.
var cloudPrices = map[CloudProvider]providerPricing{
	CloudProviderAWS: {
		CPUPerMillicorePerHour: 0.048 / 1000.0,
		MemPerMiBPerHour:       0.006 / 1024.0,
		StoragePerGiBPerMonth:  0.08,
	},
	CloudProviderGCP: {
		CPUPerMillicorePerHour: 0.044 / 1000.0,
		MemPerMiBPerHour:       0.006 / 1024.0,
		StoragePerGiBPerMonth:  0.10,
	},
	CloudProviderAzure: {
		CPUPerMillicorePerHour: 0.052 / 1000.0,
		MemPerMiBPerHour:       0.007 / 1024.0,
		StoragePerGiBPerMonth:  0.095,
	},
}

// defaultProviderRegions maps each provider to the region its prices are for.
var defaultProviderRegions = map[CloudProvider]string{
	CloudProviderAWS:   "us-east-1",
	CloudProviderGCP:   "us-central1",
	CloudProviderAzure: "eastus",
}

// CostEstimateOptions configures a cost estimation run.
type CostEstimateOptions struct {
	Provider CloudProvider
	// Region is informational: prices are list prices of the provider's
	// default region. Empty means that default region.
	Region CostRegion
	Unit   CostUnit
	// IncludeStorage adds PersistentVolumeClaims and StatefulSet
	// volumeClaimTemplates (one volume per replica).
	IncludeStorage bool
}

// WorkloadCostEstimate holds per-workload cost breakdown.
type WorkloadCostEstimate struct {
	Name      string
	Namespace string
	Kind      string
	Replicas  int
	// CPUMillis and MemoryMiB are the requests of one replica (all containers).
	CPUMillis  int64
	MemoryMiB  int64
	CPUCost    float64
	MemoryCost float64
	TotalCost  float64
	Warnings   []string
}

// StorageCostEstimate holds the cost of one persistent volume claim (or of
// all claims of one StatefulSet volumeClaimTemplate).
type StorageCostEstimate struct {
	Name      string
	Namespace string
	// Owner is "StatefulSet/<name>" for volumeClaimTemplates, empty for PVCs.
	Owner string
	// SizeGiB is the total requested size (all replicas).
	SizeGiB float64
	Cost    float64
}

// CostEstimateReport holds the full cost estimation result.
type CostEstimateReport struct {
	Provider         CloudProvider
	Region           CostRegion
	Unit             CostUnit
	Workloads        []WorkloadCostEstimate
	Storage          []StorageCostEstimate
	TotalStorageCost float64
	GrandTotal       float64
}

// parseResourceQuantity parses a Kubernetes quantity string.
// If isCPU is true, returns millicores; otherwise returns MiB.
func parseResourceQuantity(s string, isCPU bool) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty quantity string")
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return 0, fmt.Errorf("invalid quantity %q: %w", s, err)
	}
	if isCPU {
		return q.MilliValue(), nil
	}
	return q.Value() / (1024 * 1024), nil
}

// extractReplicasFromResource reads the replica/parallelism count from a workload object.
func extractReplicasFromResource(obj *unstructured.Unstructured) int {
	switch obj.GetKind() {
	case "Job":
		if v := nestedCount(obj.Object, "spec", "parallelism"); v > 0 {
			return v
		}
		return 1
	case "CronJob", "DaemonSet":
		// A DaemonSet runs one pod per node; the node count is unknown.
		return 1
	default:
		if v := nestedCount(obj.Object, "spec", "replicas"); v > 0 {
			return v
		}
		return 1
	}
}

// nestedCount reads an integer field. Manifests decoded from YAML hold
// numbers as float64, objects built in code as int64 or int.
func nestedCount(obj map[string]interface{}, fields ...string) int {
	v, found, err := unstructured.NestedFieldNoCopy(obj, fields...)
	if err != nil || !found {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return int(n)
	case int:
		return n
	case int32:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

// extractContainersFromObj returns the containers slice for the workload kind.
func extractContainersFromObj(obj *unstructured.Unstructured) ([]interface{}, bool) {
	var path []string
	switch obj.GetKind() {
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
		Kind:      r.Original.GVK.Kind,
		Replicas:  extractReplicasFromResource(obj),
	}

	containers, ok := extractContainersFromObj(obj)
	if !ok || len(containers) == 0 {
		est.Warnings = append(est.Warnings, "no containers found; using default resource requests")
		containers = []interface{}{map[string]interface{}{}}
	}

	for _, cRaw := range containers {
		c, ok := cRaw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := c["name"].(string)
		if name == "" {
			name = "container"
		}
		requests := containerQuantities(c, "requests")
		cpuStr := requests["cpu"]
		memStr := requests["memory"]

		cpuMillis := int64(defaultCPUMillicores)
		if cpuStr == "" {
			est.Warnings = append(est.Warnings, fmt.Sprintf("%s: no CPU request; assuming %dm", name, defaultCPUMillicores))
		} else if v, err := parseResourceQuantity(cpuStr, true); err != nil {
			est.Warnings = append(est.Warnings, fmt.Sprintf("%s: could not parse CPU request %q; assuming %dm", name, cpuStr, defaultCPUMillicores))
		} else {
			cpuMillis = v
		}

		memMiB := int64(defaultMemoryMiB)
		if memStr == "" {
			est.Warnings = append(est.Warnings, fmt.Sprintf("%s: no memory request; assuming %dMi", name, defaultMemoryMiB))
		} else if v, err := parseResourceQuantity(memStr, false); err != nil {
			est.Warnings = append(est.Warnings, fmt.Sprintf("%s: could not parse memory request %q; assuming %dMi", name, memStr, defaultMemoryMiB))
		} else {
			memMiB = v
		}

		est.CPUMillis += cpuMillis
		est.MemoryMiB += memMiB
	}

	multiplier := 1.0
	if unit == CostUnitMonthly {
		multiplier = hoursPerMonth
	}
	est.CPUCost = float64(est.CPUMillis) * pricing.CPUPerMillicorePerHour * float64(est.Replicas) * multiplier
	est.MemoryCost = float64(est.MemoryMiB) * pricing.MemPerMiBPerHour * float64(est.Replicas) * multiplier
	est.TotalCost = est.CPUCost + est.MemoryCost
	return est
}

// claimSizeGiB returns spec.resources.requests.storage of a PVC spec in GiB.
func claimSizeGiB(spec map[string]interface{}) float64 {
	storage, _, _ := unstructured.NestedString(spec, "resources", "requests", "storage")
	if storage == "" {
		return 0
	}
	q, err := resource.ParseQuantity(storage)
	if err != nil {
		return 0
	}
	return float64(q.Value()) / (1024 * 1024 * 1024)
}

// opsSortedResources returns the graph's resources ordered by key, so reports
// are deterministic.
func opsSortedResources(graph *types.ResourceGraph) []*types.ProcessedResource {
	out := make([]*types.ProcessedResource, 0, len(graph.Resources))
	for _, r := range graph.Resources {
		if r != nil && r.Original != nil && r.Original.Object != nil {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Original.ResourceKey().String() < out[j].Original.ResourceKey().String()
	})
	return out
}

// GenerateCostEstimate computes cost estimates for all workloads in the resource graph.
func GenerateCostEstimate(graph *types.ResourceGraph, opts CostEstimateOptions) *CostEstimateReport {
	region := opts.Region
	if region == "" {
		if def, ok := defaultProviderRegions[opts.Provider]; ok {
			region = def
		} else {
			region = "us-east-1"
		}
	}
	pricing, ok := cloudPrices[opts.Provider]
	if !ok {
		pricing = cloudPrices[CloudProviderAWS]
	}
	unit := opts.Unit
	if unit == "" {
		unit = CostUnitMonthly
	}

	report := &CostEstimateReport{
		Provider:  opts.Provider,
		Region:    region,
		Unit:      unit,
		Workloads: []WorkloadCostEstimate{},
	}
	if graph == nil {
		return report
	}

	storageCost := func(sizeGiB float64) float64 {
		cost := sizeGiB * pricing.StoragePerGiBPerMonth
		if unit == CostUnitHourly {
			cost /= hoursPerMonth
		}
		return cost
	}

	for _, r := range opsSortedResources(graph) {
		kind := r.Original.GVK.Kind
		obj := r.Original.Object
		switch {
		case isWorkloadKind(kind):
			est := estimateWorkloadCost(r, pricing, unit)
			report.Workloads = append(report.Workloads, est)
			report.GrandTotal += est.TotalCost
			if opts.IncludeStorage && kind == "StatefulSet" {
				for _, vct := range volumeClaimTemplates(r) {
					spec, _ := vct["spec"].(map[string]interface{})
					size := claimSizeGiB(spec) * float64(est.Replicas)
					if size <= 0 {
						continue
					}
					name, _, _ := unstructured.NestedString(vct, "metadata", "name")
					s := StorageCostEstimate{
						Name:      name,
						Namespace: obj.GetNamespace(),
						Owner:     "StatefulSet/" + obj.GetName(),
						SizeGiB:   size,
						Cost:      storageCost(size),
					}
					report.Storage = append(report.Storage, s)
					report.TotalStorageCost += s.Cost
					report.GrandTotal += s.Cost
				}
			}
		case opts.IncludeStorage && kind == "PersistentVolumeClaim":
			spec, _ := obj.Object["spec"].(map[string]interface{})
			size := claimSizeGiB(spec)
			if size <= 0 {
				continue
			}
			s := StorageCostEstimate{
				Name:      obj.GetName(),
				Namespace: obj.GetNamespace(),
				SizeGiB:   size,
				Cost:      storageCost(size),
			}
			report.Storage = append(report.Storage, s)
			report.TotalStorageCost += s.Cost
			report.GrandTotal += s.Cost
		}
	}
	return report
}

// Markdown renders the cost estimate as a Markdown section.
func (r *CostEstimateReport) Markdown() string {
	var b strings.Builder
	b.WriteString("## Estimated cost\n\n")
	if r == nil || (len(r.Workloads) == 0 && len(r.Storage) == 0) {
		b.WriteString("No workloads or volumes.\n")
		return b.String()
	}
	per := "month"
	if r.Unit == CostUnitHourly {
		per = "hour"
	}
	fmt.Fprintf(&b, "On-demand list prices of %s (%s), USD per %s, based on resource **requests**. "+
		"Containers without requests are counted with %dm CPU / %dMi memory. "+
		"DaemonSets are counted as one pod; multiply by your node count.\n\n",
		strings.ToUpper(string(r.Provider)), r.Region, per, defaultCPUMillicores, defaultMemoryMiB)

	if len(r.Workloads) > 0 {
		b.WriteString("| Workload | Replicas | CPU / replica | Memory / replica | CPU cost | Memory cost | Total |\n")
		b.WriteString("|---|---:|---:|---:|---:|---:|---:|\n")
		for _, w := range r.Workloads {
			fmt.Fprintf(&b, "| %s/%s | %d | %dm | %dMi | $%.2f | $%.2f | $%.2f |\n",
				w.Kind, w.Name, w.Replicas, w.CPUMillis, w.MemoryMiB, w.CPUCost, w.MemoryCost, w.TotalCost)
		}
		b.WriteString("\n")
	}
	if len(r.Storage) > 0 {
		b.WriteString("| Volume | Size | Cost |\n")
		b.WriteString("|---|---:|---:|\n")
		for _, s := range r.Storage {
			name := "PersistentVolumeClaim/" + s.Name
			if s.Owner != "" {
				name = s.Owner + " volumeClaimTemplate " + s.Name
			}
			fmt.Fprintf(&b, "| %s | %.1f GiB | $%.2f |\n", name, s.SizeGiB, s.Cost)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "**Total: $%.2f per %s**", r.GrandTotal, per)
	if r.TotalStorageCost > 0 {
		fmt.Fprintf(&b, " (storage $%.2f)", r.TotalStorageCost)
	}
	b.WriteString("\n")

	var warnings []string
	for _, w := range r.Workloads {
		for _, msg := range w.Warnings {
			warnings = append(warnings, fmt.Sprintf("- %s/%s: %s", w.Kind, w.Name, msg))
		}
	}
	if len(warnings) > 0 {
		b.WriteString("\nAssumptions:\n\n")
		b.WriteString(strings.Join(warnings, "\n"))
		b.WriteString("\n")
	}
	return b.String()
}
