package generator

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// Sizing of the namespace governance objects (ResourceQuota, LimitRange)
// from the workloads of a service group.
//
// Containers are read from the processed values (Values["containers"] and
// Values["initContainers"], written by the workload processors for
// Deployments, StatefulSets, DaemonSets, Jobs and CronJobs), so the quota
// matches what the chart renders with its default values.

// quotaResourceNames are the compute resources the quota and LimitRange cover.
var quotaResourceNames = []string{"cpu", "memory"}

// resourceAmounts maps a resource name (cpu, memory) to a quantity.
type resourceAmounts map[string]resource.Quantity

// add adds every quantity of other to a.
func (a resourceAmounts) add(other resourceAmounts) {
	for name, q := range other {
		// DeepCopy: a Quantity copied by value may share its inf.Dec with
		// the original, and Add mutates it in place.
		sum := a[name].DeepCopy()
		sum.Add(q)
		a[name] = sum
	}
}

// max raises every quantity of a to the matching quantity of other.
func (a resourceAmounts) max(other resourceAmounts) {
	for name, q := range other {
		if cur, ok := a[name]; !ok || q.Cmp(cur) > 0 {
			a[name] = q.DeepCopy()
		}
	}
}

// containerSpec holds the explicit requests and limits of one container.
type containerSpec struct {
	requests resourceAmounts
	limits   resourceAmounts
	// sidecar is an init container with restartPolicy: Always (Kubernetes
	// 1.29+ native sidecar): it keeps running next to the app containers.
	sidecar bool
}

// quotaWorkload is a pod-creating workload of a group.
type quotaWorkload struct {
	kind, name     string
	containers     []containerSpec
	initContainers []containerSpec
	pods           int64  // peak number of pods counted against the quota
	podsNote       string // how pods was derived
}

// workloadQuotaKinds are the kinds whose pods count against a quota.
var workloadQuotaKinds = map[string]bool{
	"Deployment": true, "StatefulSet": true, "DaemonSet": true, "Job": true, "CronJob": true,
}

// groupWorkloads returns the pod-creating workloads of a group, sorted by
// kind and name so the output does not depend on input order.
func groupWorkloads(group *ServiceGroup) []quotaWorkload {
	if group == nil {
		return nil
	}
	hpaMax := hpaMaxReplicas(group)
	var out []quotaWorkload
	for _, r := range group.Resources {
		kind, name := resourceKindName(r)
		if !workloadQuotaKinds[kind] || r.Values == nil {
			continue
		}
		w := quotaWorkload{
			kind:           kind,
			name:           name,
			containers:     containerSpecs(r.Values["containers"]),
			initContainers: containerSpecs(r.Values["initContainers"]),
		}
		if len(w.containers) == 0 {
			continue
		}
		w.pods, w.podsNote = peakPods(kind, r.Values, hpaMax[kind+"/"+name])
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].kind != out[j].kind {
			return out[i].kind < out[j].kind
		}
		return out[i].name < out[j].name
	})
	return out
}

func resourceKindName(r *types.ProcessedResource) (kind, name string) {
	if r == nil || r.Original == nil {
		return "", ""
	}
	kind = r.Original.GVK.Kind
	if r.Original.Object != nil {
		if kind == "" {
			kind = r.Original.Object.GetKind()
		}
		name = r.Original.Object.GetName()
	}
	return kind, name
}

// hpaMaxReplicas returns spec.maxReplicas of the HorizontalPodAutoscalers of
// the group, keyed by "<target kind>/<target name>". Only HPAs in the same
// group are seen; an HPA grouped elsewhere leaves the workload at its
// replicas. KEDA ScaledObjects are not considered (maxReplicaCount defaults
// to 100 and is usually set far above the steady state).
func hpaMaxReplicas(group *ServiceGroup) map[string]int64 {
	out := map[string]int64{}
	for _, r := range group.Resources {
		if kind, _ := resourceKindName(r); kind != "HorizontalPodAutoscaler" || r.Values == nil {
			continue
		}
		ref, _ := r.Values["scaleTargetRef"].(map[string]interface{})
		kind, _ := ref["kind"].(string)
		name, _ := ref["name"].(string)
		if n, ok := intValue(r.Values["maxReplicas"]); ok && kind != "" && name != "" {
			out[kind+"/"+name] = n
		}
	}
	return out
}

// peakPods returns the largest number of pods of a workload that exist at
// the same time, which is what quota admission has to accept:
//
//   - Deployment: replicas (spec.maxReplicas of a targeting HPA, if any)
//     plus maxSurge of a RollingUpdate (default 25%, rounded up), since the
//     surge pods are created before old ones are removed; Recreate has no
//     surge.
//   - StatefulSet: replicas (or HPA maxReplicas); pods are replaced one at a
//     time, without surge.
//   - DaemonSet: one pod per node, and the node count is not known from the
//     manifests, so one pod is counted. The quota must be raised by hand for
//     multi-node clusters; the template says so in a comment.
//   - Job: parallelism (default 1), at most completions when set.
//   - CronJob: the same for one Job. With concurrencyPolicy Allow (the
//     default) runs may overlap; overlapping runs are not counted.
func peakPods(kind string, values map[string]interface{}, hpaMax int64) (int64, string) {
	switch kind {
	case "DaemonSet":
		return 1, "1 pod per node, counted as one node"
	case "Job":
		return jobPods(values)
	case "CronJob":
		jt, _ := values["jobTemplate"].(map[string]interface{})
		pods, note := jobPods(jt)
		if policy, _ := values["concurrencyPolicy"].(string); policy == "" || policy == "Allow" {
			note += ", overlapping runs not counted"
		}
		return pods, note
	}

	replicas, ok := intValue(values["replicas"])
	if !ok {
		replicas = 1 // Kubernetes default
	}
	note := fmt.Sprintf("replicas %d", replicas)
	if hpaMax > 0 {
		replicas = hpaMax
		note = fmt.Sprintf("HPA maxReplicas %d", hpaMax)
	}
	if kind == "Deployment" {
		if surge := deploymentSurge(values, replicas); surge > 0 {
			return replicas + surge, fmt.Sprintf("%s + maxSurge %d", note, surge)
		}
	}
	return replicas, note
}

// jobPods returns the number of pods a Job runs in parallel.
func jobPods(values map[string]interface{}) (int64, string) {
	pods, ok := intValue(values["parallelism"])
	if !ok {
		pods = 1 // Kubernetes default
	}
	note := fmt.Sprintf("parallelism %d", pods)
	if completions, ok := intValue(values["completions"]); ok && completions < pods {
		pods = completions
		note = fmt.Sprintf("completions %d", completions)
	}
	return pods, note
}

// deploymentSurge returns the number of extra pods a RollingUpdate creates.
func deploymentSurge(values map[string]interface{}, replicas int64) int64 {
	strategy, _ := values["strategy"].(map[string]interface{})
	if t, _ := strategy["type"].(string); t == "Recreate" {
		return 0
	}
	surge := intstr.FromString("25%") // Kubernetes default
	if ru, ok := strategy["rollingUpdate"].(map[string]interface{}); ok {
		switch v := ru["maxSurge"].(type) {
		case string:
			surge = intstr.Parse(v)
		case int, int32, int64, float64:
			n, _ := intValue(v)
			surge = intstr.FromInt32(int32(n))
		}
	}
	n, err := intstr.GetScaledValueFromIntOrPercent(&surge, int(replicas), true)
	if err != nil || n < 0 {
		return 0
	}
	return int64(n)
}

// containerSpecs reads the containers stored in values (a slice of maps).
func containerSpecs(raw interface{}) []containerSpec {
	var list []map[string]interface{}
	switch v := raw.(type) {
	case []map[string]interface{}:
		list = v
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				list = append(list, m)
			}
		}
	}
	out := make([]containerSpec, 0, len(list))
	for _, c := range list {
		res, _ := c["resources"].(map[string]interface{})
		policy, _ := c["restartPolicy"].(string)
		out = append(out, containerSpec{
			requests: parseAmounts(res["requests"]),
			limits:   parseAmounts(res["limits"]),
			sidecar:  policy == "Always",
		})
	}
	return out
}

// parseAmounts parses the cpu and memory entries of a requests/limits map.
// Unparsable values are ignored (treated as not set).
func parseAmounts(raw interface{}) resourceAmounts {
	m, _ := raw.(map[string]interface{})
	out := resourceAmounts{}
	for _, name := range quotaResourceNames {
		var s string
		switch v := m[name].(type) {
		case string:
			s = v
		case int:
			s = strconv.Itoa(v)
		case int32:
			s = strconv.FormatInt(int64(v), 10)
		case int64:
			s = strconv.FormatInt(v, 10)
		case float64:
			s = strconv.FormatFloat(v, 'f', -1, 64)
		default:
			continue
		}
		if q, err := resource.ParseQuantity(strings.TrimSpace(s)); err == nil {
			out[name] = q
		}
	}
	return out
}

// intValue converts a numeric value from values (int64 from processors,
// float64 after a JSON/YAML round trip) to int64.
func intValue(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	}
	return 0, false
}

// limitDefaults are the per-container defaults of the group's LimitRange.
type limitDefaults struct {
	request resourceAmounts // defaultRequest
	limit   resourceAmounts // default
}

// Fallbacks used when no container of the group declares a value. They are
// not derived from the input; they keep the LimitRange valid and are meant to
// be reviewed (namespace.limitRange.enabled turns the object off).
var limitRangeFallback = map[string][2]string{
	"cpu":    {"100m", "500m"},   // defaultRequest, default
	"memory": {"128Mi", "512Mi"}, // defaultRequest, default
}

// groupLimitDefaults chooses the LimitRange defaults of a group: for each
// resource, the largest value declared by any container (init containers
// included) of the group's workloads.
//
// The defaults apply only to containers that declare nothing, i.e. whose
// needs are unknown. The maximum is a value the input actually uses, does not
// depend on the order of the input (unlike "the first workload") or on ties
// (unlike "the most common value"), and errs on the side of a container being
// given too much rather than being throttled or OOM-killed. When nothing is
// declared the fallbacks above are used. The default limit is raised to the
// default request when it is lower: the API server rejects a LimitRange whose
// defaultRequest exceeds its default.
func groupLimitDefaults(workloads []quotaWorkload) limitDefaults {
	d := limitDefaults{request: resourceAmounts{}, limit: resourceAmounts{}}
	for _, w := range workloads {
		for _, c := range append(append([]containerSpec(nil), w.containers...), w.initContainers...) {
			d.request.max(c.requests)
			d.limit.max(c.limits)
		}
	}
	for _, name := range quotaResourceNames {
		if _, ok := d.request[name]; !ok {
			d.request[name] = resource.MustParse(limitRangeFallback[name][0])
		}
		if _, ok := d.limit[name]; !ok {
			d.limit[name] = resource.MustParse(limitRangeFallback[name][1])
		}
		if req, lim := d.request[name], d.limit[name]; lim.Cmp(req) < 0 {
			d.limit[name] = req.DeepCopy()
		}
	}
	return d
}

// effective returns the requests and limits a container runs with after
// admission: an unset request defaults to the container's limit (API server
// defaulting), then to the LimitRange defaultRequest; an unset limit defaults
// to the LimitRange default. Containers without values therefore count
// against the quota with the LimitRange defaults the same chart installs.
func (c containerSpec) effective(d limitDefaults) (requests, limits resourceAmounts) {
	requests, limits = resourceAmounts{}, resourceAmounts{}
	for _, name := range quotaResourceNames {
		switch {
		case hasAmount(c.requests, name):
			requests[name] = c.requests[name]
		case hasAmount(c.limits, name):
			requests[name] = c.limits[name]
		default:
			requests[name] = d.request[name]
		}
		if hasAmount(c.limits, name) {
			limits[name] = c.limits[name]
		} else {
			limits[name] = d.limit[name]
		}
	}
	return requests, limits
}

func hasAmount(a resourceAmounts, name string) bool {
	_, ok := a[name]
	return ok
}

// podAmounts returns the effective requests and limits of one pod, computed
// the way the scheduler and quota admission do (Kubernetes
// resourcehelper.PodRequests/PodLimits): the app containers are summed;
// init containers run one at a time before them, so a regular init
// container counts on its own (plus the sidecars started before it), and
// the pod needs the maximum of that and the app sum. Native sidecars
// (init containers with restartPolicy: Always) keep running and are added
// to the app sum.
func podAmounts(w quotaWorkload, d limitDefaults) (requests, limits resourceAmounts) {
	requests, limits = resourceAmounts{}, resourceAmounts{}
	for _, c := range w.containers {
		req, lim := c.effective(d)
		requests.add(req)
		limits.add(lim)
	}
	initReq, initLim := resourceAmounts{}, resourceAmounts{}
	sidecarReq, sidecarLim := resourceAmounts{}, resourceAmounts{}
	for _, c := range w.initContainers {
		req, lim := c.effective(d)
		if c.sidecar {
			requests.add(req)
			limits.add(lim)
			sidecarReq.add(req)
			sidecarLim.add(lim)
			initReq.max(sidecarReq)
			initLim.max(sidecarLim)
			continue
		}
		req.add(sidecarReq)
		lim.add(sidecarLim)
		initReq.max(req)
		initLim.max(lim)
	}
	requests.max(initReq)
	limits.max(initLim)
	return requests, limits
}

// quotaTotals is the result of sizing a group's ResourceQuota.
type quotaTotals struct {
	requests, limits resourceAmounts
	// notes explain the computation, one line per workload.
	notes []string
}

// groupQuotaTotals sums, over all workloads of the group, the effective pod
// requests and limits times the peak number of pods. It returns false when
// the group has no workload with containers.
func groupQuotaTotals(group *ServiceGroup) (quotaTotals, bool) {
	workloads := groupWorkloads(group)
	if len(workloads) == 0 {
		return quotaTotals{}, false
	}
	defaults := groupLimitDefaults(workloads)
	t := quotaTotals{requests: resourceAmounts{}, limits: resourceAmounts{}}
	for _, w := range workloads {
		req, lim := podAmounts(w, defaults)
		t.notes = append(t.notes, fmt.Sprintf("%s %s: %d pod(s) (%s) x requests cpu %s, memory %s; limits cpu %s, memory %s",
			w.kind, w.name, w.pods, w.podsNote,
			qString(req["cpu"]), qString(req["memory"]), qString(lim["cpu"]), qString(lim["memory"])))
		for _, amounts := range []struct{ pod, total resourceAmounts }{{req, t.requests}, {lim, t.limits}} {
			for name, q := range amounts.pod {
				q = q.DeepCopy()
				q.Mul(w.pods)
				amounts.total.add(resourceAmounts{name: q})
			}
		}
	}
	return t, true
}

// qString formats a quantity canonically (e.g. 1500m, 1536Mi).
func qString(q resource.Quantity) string { return q.String() }
