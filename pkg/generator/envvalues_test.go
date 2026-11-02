package generator

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func parseEnvYAML(t *testing.T, data []byte) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid YAML in env values: %v\ncontent:\n%s", err, string(data))
	}
	return out
}

// toInt converts an interface{} numeric value to int.
// sigs.k8s.io/yaml.Unmarshal returns float64 for integers (via JSON round-trip).
func toInt(v interface{}) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}

func mapKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ============================================================
// Workload detection
// ============================================================

// ============================================================
// Section A: DetectWorkloadType
// ============================================================

// TestWorkload_DetectWeb_HasIngress — group with Deployment + Service + Ingress → WorkloadWeb
func TestWorkload_DetectWeb_HasIngress(t *testing.T) {
	group := makeGroupForEnv("webapp",
		makeResourceForEnv("Deployment", "webapp"),
		makeResourceForEnv("Service", "webapp"),
		makeResourceForEnv("Ingress", "webapp"),
	)

	got := DetectWorkloadType(group)
	if got != WorkloadWeb {
		t.Errorf("DetectWorkloadType with Ingress: got %q, want %q", got, WorkloadWeb)
	}
}

// TestWorkload_DetectWeb_Port8080 — group with Deployment exposing port 8080 → WorkloadWeb
func TestWorkload_DetectWeb_Port8080(t *testing.T) {
	group := makeGroupForEnv("webapp",
		makeResourceWithPorts("Deployment", "webapp", []int{8080}),
	)

	got := DetectWorkloadType(group)
	if got != WorkloadWeb {
		t.Errorf("DetectWorkloadType with port 8080: got %q, want %q", got, WorkloadWeb)
	}
}

// TestWorkload_DetectWorker_NoService — Deployment without any Service → WorkloadWorker
func TestWorkload_DetectWorker_NoService(t *testing.T) {
	group := makeGroupForEnv("worker",
		makeResourceForEnv("Deployment", "worker"),
	)

	got := DetectWorkloadType(group)
	if got != WorkloadWorker {
		t.Errorf("DetectWorkloadType Deployment-only: got %q, want %q", got, WorkloadWorker)
	}
}

// TestWorkload_DetectWorker_AMQPEnvVar — Deployment with AMQP_URL env var → WorkloadWorker
func TestWorkload_DetectWorker_AMQPEnvVar(t *testing.T) {
	group := makeGroupForEnv("queue-consumer",
		makeResourceWithEnvVars("Deployment", "queue-consumer", map[string]string{
			"AMQP_URL": "amqp://rabbitmq:5672",
		}),
	)

	got := DetectWorkloadType(group)
	if got != WorkloadWorker {
		t.Errorf("DetectWorkloadType with AMQP_URL env var: got %q, want %q", got, WorkloadWorker)
	}
}

// TestWorkload_DetectDatabase_StatefulSetPVC — StatefulSet + PVC → WorkloadDatabase
func TestWorkload_DetectDatabase_StatefulSetPVC(t *testing.T) {
	group := makeGroupForEnv("postgres",
		makeResourceForEnv("StatefulSet", "postgres"),
		makeResourceForEnv("PersistentVolumeClaim", "postgres-data"),
	)

	got := DetectWorkloadType(group)
	if got != WorkloadDatabase {
		t.Errorf("DetectWorkloadType StatefulSet+PVC: got %q, want %q", got, WorkloadDatabase)
	}
}

// TestWorkload_DetectDatabase_PostgresImage — Deployment with image postgres:15 → WorkloadDatabase
func TestWorkload_DetectDatabase_PostgresImage(t *testing.T) {
	group := makeGroupForEnv("db",
		makeResourceWithImage("Deployment", "db", "postgres:15"),
	)

	got := DetectWorkloadType(group)
	if got != WorkloadDatabase {
		t.Errorf("DetectWorkloadType with postgres image: got %q, want %q", got, WorkloadDatabase)
	}
}

// TestWorkload_DetectBatch_CronJob — group containing only a CronJob → WorkloadBatch
func TestWorkload_DetectBatch_CronJob(t *testing.T) {
	group := makeGroupForEnv("nightly-report",
		makeResourceForEnv("CronJob", "nightly-report"),
	)

	got := DetectWorkloadType(group)
	if got != WorkloadBatch {
		t.Errorf("DetectWorkloadType CronJob-only: got %q, want %q", got, WorkloadBatch)
	}
}

// TestWorkload_DetectCache_RedisImage — Deployment with image redis:7 (no PVC) → WorkloadCache
func TestWorkload_DetectCache_RedisImage(t *testing.T) {
	group := makeGroupForEnv("cache",
		makeResourceWithImage("Deployment", "cache", "redis:7"),
	)

	got := DetectWorkloadType(group)
	if got != WorkloadCache {
		t.Errorf("DetectWorkloadType with redis image (no PVC): got %q, want %q", got, WorkloadCache)
	}
}

// ============================================================
// Workload Test Helpers
// ============================================================

// makeResourceForEnv creates a minimal ProcessedResource with a given Kind and Name.
// Uses gvkForKind from grouping_test.go for proper GVK resolution.
func makeResourceForEnv(kind, name string) *types.ProcessedResource {
	obj := &unstructured.Unstructured{}
	obj.SetKind(kind)
	obj.SetName(name)
	gvk := gvkForKind(kind)
	obj.SetAPIVersion(gvk.GroupVersion().String())
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    gvk,
		},
	}
}

// makeResourceWithEnvVars creates a ProcessedResource whose spec embeds env vars in
// the first container, to exercise worker detection via AMQP_*/KAFKA_* vars.
func makeResourceWithEnvVars(kind, name string, envVars map[string]string) *types.ProcessedResource {
	obj := &unstructured.Unstructured{}
	obj.SetKind(kind)
	obj.SetName(name)
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind}
	obj.SetAPIVersion(gvk.GroupVersion().String())

	envList := make([]interface{}, 0, len(envVars))
	for k, v := range envVars {
		envList = append(envList, map[string]interface{}{"name": k, "value": v})
	}
	obj.Object["spec"] = map[string]interface{}{
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name": "main",
						"env":  envList,
					},
				},
			},
		},
	}
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    gvk,
		},
	}
}

// makeResourceWithImage creates a ProcessedResource whose first container uses the
// supplied image string, to exercise database/cache detection by image name.
func makeResourceWithImage(kind, name, image string) *types.ProcessedResource {
	obj := &unstructured.Unstructured{}
	obj.SetKind(kind)
	obj.SetName(name)
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind}
	obj.SetAPIVersion(gvk.GroupVersion().String())

	obj.Object["spec"] = map[string]interface{}{
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"image": image,
					},
				},
			},
		},
	}
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    gvk,
		},
	}
}

// makeResourceWithPorts creates a ProcessedResource whose first container exposes the
// given containerPorts, to exercise web detection via port numbers.
func makeResourceWithPorts(kind, name string, ports []int) *types.ProcessedResource {
	obj := &unstructured.Unstructured{}
	obj.SetKind(kind)
	obj.SetName(name)
	gvk := gvkForKind(kind)
	obj.SetAPIVersion(gvk.GroupVersion().String())

	portList := make([]interface{}, 0, len(ports))
	for _, p := range ports {
		portList = append(portList, map[string]interface{}{
			"containerPort": int64(p),
		})
	}
	obj.Object["spec"] = map[string]interface{}{
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "main",
						"ports": portList,
					},
				},
			},
		},
	}
	return &types.ProcessedResource{
		Original: &types.ExtractedResource{
			Object: obj,
			GVK:    gvk,
		},
	}
}

// makeGroupForEnv creates a ServiceGroup from variadic ProcessedResources.
// Named distinctly from makeGroup (namespace_test.go) which has a different signature.
func makeGroupForEnv(name string, resources ...*types.ProcessedResource) *ServiceGroup {
	return &ServiceGroup{
		Name:      name,
		Resources: resources,
		Namespace: "default",
		Strategy:  GroupByLabel,
	}
}

// T2-LR-5: Redis + PVC disambiguation test

func TestDetectWorkloadType_RedisWithPVC_IsNotCache(t *testing.T) {
	redis := makeResourceWithImage("StatefulSet", "redis", "redis:7")
	pvc := makeResourceForEnv("PersistentVolumeClaim", "redis-data")
	group := makeGroupForEnv("redis", redis, pvc)
	wt := DetectWorkloadType(group)
	if wt == WorkloadCache {
		t.Errorf("redis + PVC should NOT be Cache, got %s (expected Database)", wt)
	}
}

// HC-3: Nil pointer dereference tests

func TestExtractContainers_NilResource_ReturnsNil(t *testing.T) {
	result := extractContainers(nil)
	if result != nil {
		t.Errorf("expected nil for nil resource, got %v", result)
	}
}

func TestExtractContainers_NilOriginal_ReturnsNil(t *testing.T) {
	res := &types.ProcessedResource{Original: nil}
	result := extractContainers(res)
	if result != nil {
		t.Errorf("expected nil for nil Original, got %v", result)
	}
}

func TestDetectWorkloadType_NilResourceInGroup_NoPanic(t *testing.T) {
	group := &ServiceGroup{
		Name:      "test",
		Resources: []*types.ProcessedResource{nil, nil},
		Namespace: "default",
		Strategy:  GroupByLabel,
	}
	// Should not panic
	wt := DetectWorkloadType(group)
	if wt == "" {
		t.Error("expected non-empty workload type")
	}
}

// ============================================================
// Overlays keyed on the real values paths of the generated chart
// (F-S3-1/S4-F7: top-level replicaCount/logLevel/... were read by no template)
// ============================================================

// chartValues mimics the values.yaml of a universal chart: one service "web" with a
// Deployment, an HPA and a PDB, one StatefulSet service, a Job and a CronJob.
const chartValues = `
global:
  imagePullSecrets: []
services:
  web:
    enabled: true
    deployment:
      replicas: 1
      selector:
        matchLabels:
          app.kubernetes.io/name: web
      containers:
      - name: web
        image: {repository: nginx, tag: "1.27"}
        resources: {limits: {cpu: "1"}}
    hpa:
      scaleTargetRef: {kind: Deployment, name: web}
      minReplicas: 2
      maxReplicas: 4
    pdb:
      minAvailable: 1
  db:
    enabled: true
    statefulSet:
      replicas: 1
      selector:
        matchLabels:
          app.kubernetes.io/name: db
      containers:
      - name: db
        image: {repository: postgres}
  migrate:
    enabled: true
    job:
      backoffLimit: 6
      containers:
      - name: migrate
        image: {repository: migrate}
  nightly:
    enabled: true
    cronJob:
      schedule: "0 3 * * *"
      jobTemplate:
        backoffLimit: 6
`

func chartBase(t *testing.T) map[string]interface{} {
	t.Helper()
	return parseEnvYAML(t, []byte(chartValues))
}

// lookup follows a dotted path in a decoded YAML document.
func lookup(doc map[string]interface{}, path string) (interface{}, bool) {
	var cur interface{} = doc
	for _, k := range strings.Split(path, ".") {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		if cur, ok = m[k]; !ok {
			return nil, false
		}
	}
	return cur, true
}

func mustInt(t *testing.T, doc map[string]interface{}, path string) int {
	t.Helper()
	v, ok := lookup(doc, path)
	if !ok {
		t.Fatalf("overlay has no %s; got %v", path, doc)
	}
	return toInt(v)
}

func hasPath(doc map[string]interface{}, path string) bool {
	_, ok := lookup(doc, path)
	return ok
}

// leafPaths returns the dotted paths of every scalar/list in doc; "affinity" is
// treated as a leaf (the subtree is a new block the base does not have).
func leafPaths(doc map[string]interface{}, prefix string) []string {
	var out []string
	for k, v := range doc {
		p := k
		if prefix != "" {
			p = prefix + "." + k
		}
		if m, ok := v.(map[string]interface{}); ok && k != "affinity" {
			out = append(out, leafPaths(m, p)...)
			continue
		}
		out = append(out, p)
	}
	return out
}

// TestEnvValues_OverlayKeysExistInBase is the regression for the dead overlays:
// every key an overlay sets must be a key (or a new "affinity" block of an existing
// block) of the base values - otherwise no template consumes it.
func TestEnvValues_OverlayKeysExistInBase(t *testing.T) {
	base := chartBase(t)
	gens := map[string]map[string][]byte{
		"generic": GenerateEnvValues(base),
		"web":     GenerateEnvValuesForWorkload(base, WorkloadWeb),
		"worker":  GenerateEnvValuesForWorkload(base, WorkloadWorker),
		"db":      GenerateEnvValuesForWorkload(base, WorkloadDatabase),
		"batch":   GenerateEnvValuesForWorkload(base, WorkloadBatch),
		"cache":   GenerateEnvValuesForWorkload(base, WorkloadCache),
	}
	for name, files := range gens {
		for file, data := range files {
			overlay := parseEnvYAML(t, data)
			for _, p := range leafPaths(overlay, "") {
				if hasPath(base, p) {
					continue
				}
				// A new key is only allowed for the affinity of an existing block.
				i := strings.LastIndex(p, ".")
				if i < 0 || !strings.HasSuffix(p, ".affinity") || !hasPath(base, p[:i]) {
					t.Errorf("%s/%s: overlay key %s does not exist in the base values", name, file, p)
				}
			}
		}
	}
}

// TestEnvValues_NoLegacyTopLevelKeys: replicaCount, logLevel, resources,
// podDisruptionBudget and autoscaling at the top level are consumed by no template.
func TestEnvValues_NoLegacyTopLevelKeys(t *testing.T) {
	for file, data := range GenerateEnvValues(chartBase(t)) {
		overlay := parseEnvYAML(t, data)
		for _, k := range []string{"replicaCount", "logLevel", "resources", "podDisruptionBudget", "autoscaling", "affinity", "backoffLimit"} {
			if _, ok := overlay[k]; ok {
				t.Errorf("%s: top-level key %q is read by no template", file, k)
			}
		}
	}
}

// TestEnvValues_ReplicasAtRealPath: the overlays change what helm renders, i.e. the
// replicas of the Deployment and the StatefulSet.
func TestEnvValues_ReplicasAtRealPath(t *testing.T) {
	files := GenerateEnvValues(chartBase(t))
	want := map[string]int{"values-dev.yaml": 1, "values-staging.yaml": 2, "values-prod.yaml": 3}
	for file, n := range want {
		overlay := parseEnvYAML(t, files[file])
		if got := mustInt(t, overlay, "services.web.deployment.replicas"); got != n {
			t.Errorf("%s: deployment replicas %d, want %d", file, got, n)
		}
		if got := mustInt(t, overlay, "services.db.statefulSet.replicas"); got != n {
			t.Errorf("%s: statefulSet replicas %d, want %d", file, got, n)
		}
	}
}

// TestEnvValues_ProdNeverLowersReplicas: prod is "at least", dev/staging are absolute.
func TestEnvValues_ProdNeverLowersReplicas(t *testing.T) {
	base := parseEnvYAML(t, []byte("services:\n  web:\n    deployment:\n      replicas: 5\n"))
	files := GenerateEnvValues(base)
	if got := mustInt(t, parseEnvYAML(t, files["values-prod.yaml"]), "services.web.deployment.replicas"); got != 5 {
		t.Errorf("prod replicas: got %d, want 5 (base is higher than the profile)", got)
	}
	if got := mustInt(t, parseEnvYAML(t, files["values-dev.yaml"]), "services.web.deployment.replicas"); got != 1 {
		t.Errorf("dev replicas: got %d, want 1", got)
	}
	if got := mustInt(t, parseEnvYAML(t, files["values-staging.yaml"]), "services.web.deployment.replicas"); got != 2 {
		t.Errorf("staging replicas: got %d, want 2", got)
	}
}

// TestEnvValues_OverlayChangesEffectiveValues: base merged with the overlay differs
// from the base at a path the Deployment template reads.
func TestEnvValues_OverlayChangesEffectiveValues(t *testing.T) {
	base := chartBase(t)
	overlay := parseEnvYAML(t, GenerateEnvValues(base)["values-prod.yaml"])
	merged := deepMergeValues(base, overlay)
	got, _ := lookup(merged, "services.web.deployment.replicas")
	if toInt(got) != 3 {
		t.Errorf("merged prod replicas: got %v, want 3", got)
	}
	// The container list (image, resources) is not part of the overlay: Helm replaces
	// lists wholesale, so an overlay would freeze the image tag of the base.
	if _, ok := lookup(overlay, "services.web.deployment.containers"); ok {
		t.Error("overlay must not copy the containers list")
	}
}

// deepMergeValues merges override over base the way Helm does (maps merge, the rest replaces).
func deepMergeValues(base, override map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(base))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		if ov, ok := v.(map[string]interface{}); ok {
			if bv, ok := out[k].(map[string]interface{}); ok {
				out[k] = deepMergeValues(bv, ov)
				continue
			}
		}
		out[k] = v
	}
	return out
}

func TestEnvValues_HPAAndPDB(t *testing.T) {
	files := GenerateEnvValuesForWorkload(chartBase(t), WorkloadWeb)

	prod := parseEnvYAML(t, files["values-prod.yaml"])
	if got := mustInt(t, prod, "services.web.hpa.minReplicas"); got != 3 {
		t.Errorf("web prod HPA min: got %d, want 3", got)
	}
	if got := mustInt(t, prod, "services.web.hpa.maxReplicas"); got != 10 {
		t.Errorf("web prod HPA max: got %d, want 10", got)
	}
	if got := mustInt(t, prod, "services.web.pdb.minAvailable"); got != 2 {
		t.Errorf("web prod PDB minAvailable: got %d, want 2", got)
	}

	dev := parseEnvYAML(t, files["values-dev.yaml"])
	if mustInt(t, dev, "services.web.hpa.minReplicas") != 1 || mustInt(t, dev, "services.web.hpa.maxReplicas") != 1 {
		t.Errorf("web dev HPA must be pinned to one replica, got %v", dev)
	}
	if hasPath(dev, "services.web.pdb") {
		t.Errorf("web dev must not touch the PDB, got %v", dev)
	}
}

func TestEnvValues_ProdKeepsHigherHPABounds(t *testing.T) {
	base := parseEnvYAML(t, []byte("services:\n  web:\n    hpa:\n      minReplicas: 5\n      maxReplicas: 40\n"))
	prod := parseEnvYAML(t, GenerateEnvValuesForWorkload(base, WorkloadWeb)["values-prod.yaml"])
	if got := mustInt(t, prod, "services.web.hpa.minReplicas"); got != 5 {
		t.Errorf("prod HPA min: got %d, want 5", got)
	}
	if got := mustInt(t, prod, "services.web.hpa.maxReplicas"); got != 40 {
		t.Errorf("prod HPA max: got %d, want 40", got)
	}
}

// TestEnvValues_PDBOnlyNumericMinAvailable: a PDB with maxUnavailable or a percentage
// minAvailable would become invalid (both set) or lose its meaning.
func TestEnvValues_PDBOnlyNumericMinAvailable(t *testing.T) {
	for name, pdb := range map[string]string{
		"maxUnavailable":      "maxUnavailable: 1",
		"percentMinAvailable": "minAvailable: 50%",
		"both":                "minAvailable: 1\n      maxUnavailable: 1",
	} {
		base := parseEnvYAML(t, []byte("services:\n  web:\n    pdb:\n      "+pdb+"\n"))
		for file, data := range GenerateEnvValues(base) {
			if parsed := parseEnvYAML(t, data); hasPath(parsed, "services.web.pdb") {
				t.Errorf("%s/%s: PDB must be left alone, got %v", name, file, parsed)
			}
		}
	}
}

func TestEnvValues_ProdAntiAffinity(t *testing.T) {
	prod := parseEnvYAML(t, GenerateEnvValues(chartBase(t))["values-prod.yaml"])
	for _, p := range []string{"services.web.deployment.affinity", "services.db.statefulSet.affinity"} {
		aff, ok := lookup(prod, p)
		if !ok {
			t.Fatalf("prod overlay has no %s", p)
		}
		if _, ok := lookup(aff.(map[string]interface{}), "podAntiAffinity"); !ok {
			t.Errorf("%s must contain podAntiAffinity, got %v", p, aff)
		}
	}
	// The selector comes from the block itself, not from a made-up label.
	sel, _ := lookup(prod, "services.web.deployment.affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution")
	term := sel.([]interface{})[0].(map[string]interface{})["podAffinityTerm"].(map[string]interface{})
	labels := term["labelSelector"].(map[string]interface{})["matchLabels"].(map[string]interface{})
	if labels["app.kubernetes.io/name"] != "web" {
		t.Errorf("anti-affinity selector: got %v, want app.kubernetes.io/name=web", labels)
	}
	for _, env := range []string{"values-dev.yaml", "values-staging.yaml"} {
		if hasPath(parseEnvYAML(t, GenerateEnvValues(chartBase(t))[env]), "services.web.deployment.affinity") {
			t.Errorf("%s must not set affinity", env)
		}
	}
}

func TestEnvValues_ProdAffinityKeepsExistingAndNeedsSelector(t *testing.T) {
	withAffinity := parseEnvYAML(t, []byte("services:\n  web:\n    deployment:\n      replicas: 1\n      selector: {matchLabels: {a: b}}\n      affinity: {nodeAffinity: {}}\n"))
	if hasPath(parseEnvYAML(t, GenerateEnvValues(withAffinity)["values-prod.yaml"]), "services.web.deployment.affinity") {
		t.Error("an affinity of the base must not be merged with a generated one")
	}
	noSelector := parseEnvYAML(t, []byte("services:\n  web:\n    deployment:\n      replicas: 1\n"))
	if hasPath(parseEnvYAML(t, GenerateEnvValues(noSelector)["values-prod.yaml"]), "services.web.deployment.affinity") {
		t.Error("without a selector there is nothing to spread; affinity must not be generated")
	}
}

func TestEnvValues_BatchBackoffLimit(t *testing.T) {
	files := GenerateEnvValuesForWorkload(chartBase(t), WorkloadBatch)
	want := map[string]int{"values-dev.yaml": 1, "values-staging.yaml": 2, "values-prod.yaml": 3}
	for file, n := range want {
		overlay := parseEnvYAML(t, files[file])
		if got := mustInt(t, overlay, "services.migrate.job.backoffLimit"); got != n {
			t.Errorf("%s: job backoffLimit %d, want %d", file, got, n)
		}
		if got := mustInt(t, overlay, "services.nightly.cronJob.jobTemplate.backoffLimit"); got != n {
			t.Errorf("%s: cronJob backoffLimit %d, want %d", file, got, n)
		}
		// A batch profile does not scale Deployments.
		if hasPath(overlay, "services.web.deployment.replicas") {
			t.Errorf("%s: batch profile must not set deployment replicas", file)
		}
	}
}

func TestEnvValues_WorkloadProfiles(t *testing.T) {
	cases := []struct {
		wt       WorkloadType
		replicas map[string]int
		pdb      int // prod minAvailable, 0 = untouched
	}{
		{WorkloadWeb, map[string]int{"dev": 1, "staging": 2, "prod": 3}, 2},
		{WorkloadWorker, map[string]int{"dev": 1, "staging": 2, "prod": 3}, 1},
		{WorkloadDatabase, map[string]int{"dev": 1, "staging": 2, "prod": 3}, 2},
		{WorkloadCache, map[string]int{"dev": 1, "staging": 2, "prod": 2}, 1},
	}
	for _, c := range cases {
		files := GenerateEnvValuesForWorkload(chartBase(t), c.wt)
		for env, n := range c.replicas {
			overlay := parseEnvYAML(t, files["values-"+env+".yaml"])
			if got := mustInt(t, overlay, "services.web.deployment.replicas"); got != n {
				t.Errorf("%s %s: replicas %d, want %d", c.wt, env, got, n)
			}
		}
		prod := parseEnvYAML(t, files["values-prod.yaml"])
		if got := mustInt(t, prod, "services.web.pdb.minAvailable"); got != c.pdb {
			t.Errorf("%s prod: PDB minAvailable %d, want %d", c.wt, got, c.pdb)
		}
		if c.wt != WorkloadWeb && hasPath(prod, "services.web.hpa") {
			t.Errorf("%s prod: HPA must be left alone (only web scales by HPA)", c.wt)
		}
	}
}

// TestEnvValues_SeparateAndUmbrellaLayouts: the value blocks are found where the
// layouts put them - at the top level, under a service key, under a subchart.
func TestEnvValues_SeparateAndUmbrellaLayouts(t *testing.T) {
	for path, doc := range map[string]string{
		"deployment.replicas":                       "deployment:\n  replicas: 1\n",
		"api.deployment.replicas":                   "api:\n  deployment:\n    replicas: 1\n",
		"frontend.services.web.deployment.replicas": "frontend:\n  services:\n    web:\n      deployment:\n        replicas: 1\n",
	} {
		prod := parseEnvYAML(t, GenerateEnvValues(parseEnvYAML(t, []byte(doc)))["values-prod.yaml"])
		if got := mustInt(t, prod, path); got != 3 {
			t.Errorf("%s: prod replicas %d, want 3", path, got)
		}
	}
}

// TestEnvValues_NoSlots: nothing to override (nil base, or no workload blocks) still
// yields three valid, empty overlays.
func TestEnvValues_NoSlots(t *testing.T) {
	for name, base := range map[string]map[string]interface{}{
		"nil":      nil,
		"empty":    {},
		"no-slots": {"global": map[string]interface{}{"imageRegistry": ""}, "services": map[string]interface{}{"cm": map[string]interface{}{"configMaps": map[string]interface{}{"x": map[string]interface{}{"data": map[string]interface{}{"job": "x"}}}}}},
	} {
		files := GenerateEnvValues(base)
		if len(files) != 3 {
			t.Fatalf("%s: expected 3 files, got %v", name, mapKeys(files))
		}
		for file, data := range files {
			if overlay := parseEnvYAML(t, data); len(overlay) != 0 {
				t.Errorf("%s/%s: expected an empty overlay, got %v", name, file, overlay)
			}
		}
	}
}

func TestEnvValues_Deterministic(t *testing.T) {
	a := GenerateEnvValues(chartBase(t))
	b := GenerateEnvValues(chartBase(t))
	for file := range a {
		if string(a[file]) != string(b[file]) {
			t.Errorf("%s differs between runs", file)
		}
	}
}

func TestEnvValues_FileNaming(t *testing.T) {
	files := GenerateEnvValues(chartBase(t))
	for _, name := range []string{"values-dev.yaml", "values-staging.yaml", "values-prod.yaml"} {
		if _, ok := files[name]; !ok {
			t.Errorf("expected file %q in output, not found. Keys: %v", name, mapKeys(files))
		}
	}
	if len(files) != 3 {
		t.Errorf("expected exactly 3 env value files, got %d: %v", len(files), mapKeys(files))
	}
}

func TestEnvValues_CLIFlag_Disabled(t *testing.T) {
	// When EnvValues flag is disabled, no env files generated
	// This is tested via Options.EnvValues field
	opts := Options{}
	if opts.EnvValues {
		t.Error("EnvValues option should default to false (disabled)")
	}
}
