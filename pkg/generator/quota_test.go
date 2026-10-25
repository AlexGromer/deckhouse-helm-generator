package generator

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// qRes builds a container resources map: qRes("100m", "128Mi", "500m", "512Mi").
// An empty string leaves the entry out.
func qRes(cpuReq, memReq, cpuLim, memLim string) map[string]interface{} {
	out := map[string]interface{}{}
	add := func(section, name, v string) {
		if v == "" {
			return
		}
		m, _ := out[section].(map[string]interface{})
		if m == nil {
			m = map[string]interface{}{}
			out[section] = m
		}
		m[name] = v
	}
	add("requests", "cpu", cpuReq)
	add("requests", "memory", memReq)
	add("limits", "cpu", cpuLim)
	add("limits", "memory", memLim)
	return out
}

// qContainer returns a container value as written by the workload processors.
func qContainer(name string, resources map[string]interface{}) map[string]interface{} {
	c := map[string]interface{}{"name": name, "image": map[string]interface{}{"repository": "app"}}
	if resources != nil {
		c["resources"] = resources
	}
	return c
}

func qWorkload(kind, name string, values map[string]interface{}, containers ...map[string]interface{}) *types.ProcessedResource {
	if values == nil {
		values = map[string]interface{}{}
	}
	values["containers"] = containers
	return makeResourceWithValues(kind, name, "default", nil, values)
}

// quotaHard returns the spec.hard entries of a rendered quota template.
func quotaHard(t *testing.T, tmpl string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, l := range strings.Split(tmpl, "\n") {
		l = strings.TrimSpace(l)
		for _, key := range []string{"requests.cpu", "requests.memory", "limits.cpu", "limits.memory"} {
			if strings.HasPrefix(l, key+": ") {
				out[key] = strings.Trim(strings.TrimPrefix(l, key+": "), `"`)
			}
		}
	}
	if len(out) != 4 {
		t.Fatalf("quota template has %d hard entries, want 4:\n%s", len(out), tmpl)
	}
	return out
}

func assertHard(t *testing.T, got map[string]string, want map[string]string) {
	t.Helper()
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestQuota_SumsWorkloadsAndReplicas(t *testing.T) {
	group := makeGroup("app", "default", []*types.ProcessedResource{
		qWorkload("StatefulSet", "db", map[string]interface{}{"replicas": int64(3)},
			qContainer("db", qRes("250m", "256Mi", "1", "1Gi"))),
		qWorkload("Deployment", "web", map[string]interface{}{
			"replicas": int64(2),
			"strategy": map[string]interface{}{"type": "Recreate"},
		}, qContainer("web", qRes("100m", "128Mi", "500m", "512Mi"))),
	})
	hard := quotaHard(t, GenerateResourceQuotaTemplate("app", group))
	// 3 x 250m + 2 x 100m = 950m; 3 x 256Mi + 2 x 128Mi = 1Gi
	assertHard(t, hard, map[string]string{
		"requests.cpu": "950m", "requests.memory": "1Gi",
		"limits.cpu": "4", "limits.memory": "4Gi",
	})
}

func TestQuota_MultipleContainersMixedUnits(t *testing.T) {
	group := makeGroup("app", "default", []*types.ProcessedResource{
		qWorkload("StatefulSet", "web", nil,
			qContainer("app", qRes("500m", "512Mi", "1", "1Gi")),
			qContainer("proxy", qRes("1", "1Gi", "500m", "512Mi"))),
	})
	hard := quotaHard(t, GenerateResourceQuotaTemplate("app", group))
	assertHard(t, hard, map[string]string{
		"requests.cpu": "1500m", "requests.memory": "1536Mi",
		"limits.cpu": "1500m", "limits.memory": "1536Mi",
	})
}

func TestQuota_NumericQuantities(t *testing.T) {
	group := makeGroup("app", "default", []*types.ProcessedResource{
		qWorkload("StatefulSet", "web", nil, qContainer("app", map[string]interface{}{
			"requests": map[string]interface{}{"cpu": 0.5, "memory": int64(268435456)},
			"limits":   map[string]interface{}{"cpu": int64(2), "memory": "1Gi"},
		})),
	})
	hard := quotaHard(t, GenerateResourceQuotaTemplate("app", group))
	assertHard(t, hard, map[string]string{
		"requests.cpu": "500m", "requests.memory": "268435456",
		"limits.cpu": "2", "limits.memory": "1Gi",
	})
}

func TestQuota_InitContainersMaxRule(t *testing.T) {
	values := map[string]interface{}{
		"initContainers": []map[string]interface{}{
			// Larger than the app containers for cpu, smaller for memory.
			qContainer("migrate", qRes("2", "64Mi", "2", "64Mi")),
			qContainer("warmup", qRes("100m", "1Gi", "100m", "1Gi")),
		},
	}
	group := makeGroup("app", "default", []*types.ProcessedResource{
		qWorkload("StatefulSet", "web", values,
			qContainer("a", qRes("500m", "256Mi", "500m", "256Mi")),
			qContainer("b", qRes("500m", "256Mi", "500m", "256Mi"))),
	})
	hard := quotaHard(t, GenerateResourceQuotaTemplate("app", group))
	// cpu: max(2, 1) = 2; memory: max(1Gi, 512Mi) = 1Gi
	assertHard(t, hard, map[string]string{
		"requests.cpu": "2", "requests.memory": "1Gi",
		"limits.cpu": "2", "limits.memory": "1Gi",
	})
}

func TestQuota_NativeSidecarAddsToApp(t *testing.T) {
	sidecar := qContainer("mesh", qRes("100m", "64Mi", "100m", "64Mi"))
	sidecar["restartPolicy"] = "Always"
	values := map[string]interface{}{
		"initContainers": []interface{}{
			sidecar,
			qContainer("migrate", qRes("1", "64Mi", "1", "64Mi")),
		},
	}
	group := makeGroup("app", "default", []*types.ProcessedResource{
		qWorkload("StatefulSet", "web", values, qContainer("app", qRes("200m", "128Mi", "200m", "128Mi"))),
	})
	hard := quotaHard(t, GenerateResourceQuotaTemplate("app", group))
	// app + sidecar = 300m/192Mi; migrate runs next to the sidecar: 1100m/128Mi.
	assertHard(t, hard, map[string]string{
		"requests.cpu": "1100m", "requests.memory": "192Mi",
	})
}

func TestQuota_PeakPods(t *testing.T) {
	cases := []struct {
		name   string
		kind   string
		values map[string]interface{}
		hpa    int64
		want   int64
	}{
		{"deployment default surge", "Deployment", map[string]interface{}{"replicas": int64(4)}, 0, 5},
		{"deployment surge rounds up", "Deployment", map[string]interface{}{"replicas": int64(1)}, 0, 2},
		{"deployment int surge", "Deployment", map[string]interface{}{
			"replicas": int64(4),
			"strategy": map[string]interface{}{"rollingUpdate": map[string]interface{}{"maxSurge": int64(0)}},
		}, 0, 4},
		{"deployment percent surge", "Deployment", map[string]interface{}{
			"replicas": int64(10),
			"strategy": map[string]interface{}{"rollingUpdate": map[string]interface{}{"maxSurge": "50%"}},
		}, 0, 15},
		{"deployment recreate", "Deployment", map[string]interface{}{
			"replicas": int64(3), "strategy": map[string]interface{}{"type": "Recreate"},
		}, 0, 3},
		{"deployment hpa", "Deployment", map[string]interface{}{"replicas": int64(2)}, 8, 10},
		{"statefulset default", "StatefulSet", map[string]interface{}{}, 0, 1},
		{"statefulset float replicas", "StatefulSet", map[string]interface{}{"replicas": float64(3)}, 0, 3},
		{"daemonset", "DaemonSet", map[string]interface{}{}, 0, 1},
		{"job default", "Job", map[string]interface{}{}, 0, 1},
		{"job parallelism", "Job", map[string]interface{}{"parallelism": int64(4)}, 0, 4},
		{"job completions cap", "Job", map[string]interface{}{"parallelism": int64(4), "completions": int64(2)}, 0, 2},
		{"cronjob", "CronJob", map[string]interface{}{
			"jobTemplate": map[string]interface{}{"parallelism": int64(3)},
		}, 0, 3},
		{"cronjob forbid", "CronJob", map[string]interface{}{"concurrencyPolicy": "Forbid"}, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, note := peakPods(tc.kind, tc.values, tc.hpa)
			if got != tc.want {
				t.Errorf("peakPods = %d (%s), want %d", got, note, tc.want)
			}
			if note == "" {
				t.Error("empty note")
			}
		})
	}
}

func TestQuota_HPAInGroupUsesMaxReplicas(t *testing.T) {
	hpa := makeResourceWithValues("HorizontalPodAutoscaler", "web", "default", nil, map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{"apiVersion": "apps/v1", "kind": "StatefulSet", "name": "web"},
		"minReplicas":    int64(2),
		"maxReplicas":    int64(6),
	})
	group := makeGroup("app", "default", []*types.ProcessedResource{
		qWorkload("StatefulSet", "web", map[string]interface{}{"replicas": int64(2)},
			qContainer("app", qRes("100m", "100Mi", "200m", "200Mi"))),
		hpa,
	})
	tmpl := GenerateResourceQuotaTemplate("app", group)
	assertHard(t, quotaHard(t, tmpl), map[string]string{
		"requests.cpu": "600m", "requests.memory": "600Mi",
		"limits.cpu": "1200m", "limits.memory": "1200Mi",
	})
	if !strings.Contains(tmpl, "HPA maxReplicas 6") {
		t.Errorf("template does not explain the HPA multiplier:\n%s", tmpl)
	}
}

func TestQuota_MissingValuesUseLimitRangeDefaults(t *testing.T) {
	group := makeGroup("app", "default", []*types.ProcessedResource{
		qWorkload("StatefulSet", "a", nil, qContainer("app", qRes("300m", "256Mi", "1", "1Gi"))),
		// No resources at all: gets the LimitRange defaults (the group's maxima).
		qWorkload("StatefulSet", "b", nil, qContainer("app", nil)),
		// Limit only: the API server defaults the request to the limit.
		qWorkload("StatefulSet", "c", nil, qContainer("app", qRes("", "", "200m", "128Mi"))),
	})
	hard := quotaHard(t, GenerateResourceQuotaTemplate("app", group))
	assertHard(t, hard, map[string]string{
		"requests.cpu": "800m", "requests.memory": "640Mi",
		"limits.cpu": "2200m", "limits.memory": "2176Mi",
	})
}

func TestQuota_NoWorkloadUsesPlaceholder(t *testing.T) {
	cm := makeResourceWithValues("ConfigMap", "cfg", "default", nil, map[string]interface{}{"data": map[string]interface{}{"a": "b"}})
	tmpl := GenerateResourceQuotaTemplate("app", makeGroup("cfg", "default", []*types.ProcessedResource{cm}))
	assertHard(t, quotaHard(t, tmpl), map[string]string{
		"requests.cpu": "1", "requests.memory": "1Gi", "limits.cpu": "2", "limits.memory": "2Gi",
	})
	if !strings.Contains(tmpl, "placeholder") {
		t.Errorf("placeholder quota is not marked as such:\n%s", tmpl)
	}
}

func TestQuota_DaemonSetNote(t *testing.T) {
	group := makeGroup("agent", "default", []*types.ProcessedResource{
		qWorkload("DaemonSet", "agent", nil, qContainer("agent", qRes("50m", "64Mi", "100m", "128Mi"))),
	})
	tmpl := GenerateResourceQuotaTemplate("app", group)
	if !strings.Contains(tmpl, "number of nodes") {
		t.Errorf("DaemonSet quota does not explain the node count:\n%s", tmpl)
	}
	assertHard(t, quotaHard(t, tmpl), map[string]string{"requests.cpu": "50m", "limits.memory": "128Mi"})
}

func TestQuota_InvalidQuantityIgnored(t *testing.T) {
	group := makeGroup("app", "default", []*types.ProcessedResource{
		qWorkload("StatefulSet", "web", nil, qContainer("app", qRes("lots", "128Mi", "1", "256Mi"))),
	})
	// The unparsable cpu request falls back to the cpu limit.
	assertHard(t, quotaHard(t, GenerateResourceQuotaTemplate("app", group)), map[string]string{"requests.cpu": "1"})
}

func TestQuota_OrderIndependent(t *testing.T) {
	a := qWorkload("StatefulSet", "a", nil, qContainer("app", qRes("100m", "1Gi", "", "")))
	b := qWorkload("StatefulSet", "b", nil, qContainer("app", qRes("1", "100Mi", "", "")))
	one := GenerateResourceQuotaTemplate("app", makeGroup("g", "default", []*types.ProcessedResource{a, b}))
	two := GenerateResourceQuotaTemplate("app", makeGroup("g", "default", []*types.ProcessedResource{b, a}))
	if one != two {
		t.Errorf("quota depends on input order:\n%s\n---\n%s", one, two)
	}
}

func TestLimitRange_UsesMaximumAndStaysValid(t *testing.T) {
	group := makeGroup("app", "default", []*types.ProcessedResource{
		qWorkload("StatefulSet", "a", nil, qContainer("app", qRes("100m", "128Mi", "500m", "512Mi"))),
		// Request above every declared limit: the default limit is raised to it.
		qWorkload("StatefulSet", "b", nil, qContainer("app", qRes("2", "64Mi", "", ""))),
	})
	tmpl := GenerateLimitRangeTemplate("app", group)
	want := "      default:\n        cpu: \"2\"\n        memory: \"512Mi\"\n      defaultRequest:\n        cpu: \"2\"\n        memory: \"128Mi\"\n"
	if !strings.Contains(tmpl, want) {
		t.Errorf("LimitRange defaults:\n%s\nwant to contain:\n%s", tmpl, want)
	}
}

func TestLimitRange_FallbackWithoutValues(t *testing.T) {
	group := makeGroup("app", "default", []*types.ProcessedResource{
		qWorkload("Deployment", "a", nil, qContainer("app", nil)),
	})
	tmpl := GenerateLimitRangeTemplate("app", group)
	want := "      default:\n        cpu: \"500m\"\n        memory: \"512Mi\"\n      defaultRequest:\n        cpu: \"100m\"\n        memory: \"128Mi\"\n"
	if !strings.Contains(tmpl, want) {
		t.Errorf("LimitRange fallback:\n%s\nwant to contain:\n%s", tmpl, want)
	}
}

func TestContainerSpecs_AcceptsInterfaceSlices(t *testing.T) {
	specs := containerSpecs([]interface{}{qContainer("a", qRes("1", "", "", "")), "not-a-map"})
	if len(specs) != 1 || !hasAmount(specs[0].requests, "cpu") {
		t.Errorf("containerSpecs = %+v", specs)
	}
	if len(containerSpecs("garbage")) != 0 || len(containerSpecs(nil)) != 0 {
		t.Error("containerSpecs of a non-slice must be empty")
	}
}

func TestGroupWorkloads_SkipsNilAndContainerless(t *testing.T) {
	if groupWorkloads(nil) != nil {
		t.Error("nil group must have no workloads")
	}
	noContainers := makeResourceWithValues("Deployment", "x", "default", nil, map[string]interface{}{})
	if w := groupWorkloads(makeGroup("g", "default", []*types.ProcessedResource{noContainers, nil})); len(w) != 0 {
		t.Errorf("groupWorkloads = %+v, want none", w)
	}
}
