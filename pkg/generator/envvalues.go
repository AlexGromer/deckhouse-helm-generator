package generator

import (
	"sort"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

// GenerateEnvValues creates environment-specific values override files from base values.
// It returns a map of filename → YAML bytes for values-dev.yaml, values-staging.yaml,
// and values-prod.yaml. Each file contains only the overrides relevant to that environment
// (override-only principle — no full copy of base values).
//
// The overrides are keyed on the values paths the chart's templates actually read
// (services.<svc>.deployment.replicas, services.<svc>.hpa.minReplicas, ...), found by
// walking baseValues; settings the base has no slot for are not emitted, because
// `helm template -f values-prod.yaml` would silently ignore them.
func GenerateEnvValues(baseValues map[string]interface{}) map[string][]byte {
	return renderEnvFiles(baseValues, genericEnvProfile, map[string]string{
		"dev":     "# Dev environment overrides — relaxed settings for local development\n",
		"staging": "# Staging environment overrides — mirrors prod at reduced scale\n",
		"prod":    "# Production environment overrides — hardened settings\n",
	})
}

// envProfile is the set of per-environment settings. A nil field is left as in the base values.
type envProfile struct {
	replicas        *int // deployment / statefulSet replicas
	backoffLimit    *int // job / cronJob backoffLimit
	hpaMin          *int // hpa minReplicas
	hpaMax          *int // hpa maxReplicas
	pdbMinAvailable *int // pdb minAvailable (numeric only)
	antiAffinity    bool // preferred pod anti-affinity across nodes
}

func ip(n int) *int { return &n }

// genericEnvProfile is the workload-agnostic profile used when the workload type is unknown.
// Dev: single replica. Staging: 2 replicas, PDB minAvailable=1.
// Prod: >=3 replicas, PDB minAvailable=2, anti-affinity.
func genericEnvProfile(env string) envProfile {
	switch env {
	case "dev":
		return envProfile{replicas: ip(1)}
	case "staging":
		return envProfile{replicas: ip(2), pdbMinAvailable: ip(1)}
	default:
		return envProfile{replicas: ip(3), pdbMinAvailable: ip(2), antiAffinity: true}
	}
}

// stagingEnvOverlay is the staging overrides over baseValues, for environments
// that have no profile of their own (custom environment names).
func stagingEnvOverlay(baseValues map[string]interface{}) map[string]interface{} {
	return buildEnvOverlay(findEnvSlots(baseValues), genericEnvProfile("staging"), "staging")
}

// envEnvironments is the fixed set of generated environments (file values-<env>.yaml).
var envEnvironments = []string{"dev", "staging", "prod"}

// renderEnvFiles builds values-<env>.yaml for every environment from the profile
// chosen by profileFor, anchored on the value slots found in baseValues.
func renderEnvFiles(baseValues map[string]interface{}, profileFor func(env string) envProfile, headers map[string]string) map[string][]byte {
	slots := findEnvSlots(baseValues)
	result := make(map[string][]byte, len(envEnvironments))
	for _, env := range envEnvironments {
		overlay := buildEnvOverlay(slots, profileFor(env), env)
		data, _ := yaml.Marshal(overlay)
		result["values-"+env+".yaml"] = append([]byte(headers[env]), data...)
	}
	return result
}

// Value-block kinds that carry overridable settings. The keys are the ones the
// processors write below services.<svc>. (or at the top level / under the service
// name in separate and umbrella layouts).
const (
	slotDeployment  = "deployment"
	slotStatefulSet = "statefulSet"
	slotJob         = "job"
	slotCronJob     = "cronJob"
	slotHPA         = "hpa"
	slotPDB         = "pdb"
)

// envMaxDepth bounds the search for value blocks: services.<svc>.<kind> is 3 levels,
// an umbrella parent adds one more.
const envMaxDepth = 5

// envSlot is a value block of the base values found at path (path ends with kind).
type envSlot struct {
	path []string
	kind string
	node map[string]interface{}
}

// findEnvSlots walks baseValues and returns the value blocks of the workload-related
// kinds in a deterministic order. A block is only taken when it looks like what the
// processors write (e.g. a "deployment" block with replicas or containers).
func findEnvSlots(base map[string]interface{}) []envSlot {
	var slots []envSlot
	var walk func(node map[string]interface{}, path []string)
	walk = func(node map[string]interface{}, path []string) {
		keys := make([]string, 0, len(node))
		for k := range node {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child, ok := node[k].(map[string]interface{})
			if !ok || k == "global" {
				continue
			}
			childPath := append(append([]string{}, path...), k)
			if looksLikeSlot(k, child) {
				slots = append(slots, envSlot{path: childPath, kind: k, node: child})
				continue
			}
			if len(childPath) < envMaxDepth {
				walk(child, childPath)
			}
		}
	}
	walk(base, nil)
	return slots
}

// looksLikeSlot reports whether the map under key is a value block of that kind.
func looksLikeSlot(key string, node map[string]interface{}) bool {
	has := func(names ...string) bool {
		for _, n := range names {
			if _, ok := node[n]; ok {
				return true
			}
		}
		return false
	}
	switch key {
	case slotDeployment, slotStatefulSet:
		return has("replicas", "containers")
	case slotJob:
		return has("containers", "backoffLimit")
	case slotCronJob:
		return has("schedule", "jobTemplate")
	case slotHPA:
		return has("scaleTargetRef", "minReplicas", "maxReplicas")
	case slotPDB:
		return has("minAvailable", "maxUnavailable")
	}
	return false
}

// buildEnvOverlay turns a profile into an override map over the found slots.
// Prod never lowers what the base already has (replicas, HPA bounds): it only raises.
func buildEnvOverlay(slots []envSlot, p envProfile, env string) map[string]interface{} {
	overlay := map[string]interface{}{}
	prod := env == "prod"
	// raise returns want, or the base value when prod must not go below it.
	raise := func(node map[string]interface{}, key string, want int) int {
		if base, ok := asInt(node[key]); ok && prod && base > want {
			return base
		}
		return want
	}
	for _, s := range slots {
		set := func(key string, val interface{}) { setPath(overlay, append(append([]string{}, s.path...), key), val) }
		switch s.kind {
		case slotDeployment, slotStatefulSet:
			if p.replicas != nil {
				set("replicas", raise(s.node, "replicas", *p.replicas))
			}
			if p.antiAffinity && s.node["affinity"] == nil {
				if aff := antiAffinityFor(s.node); aff != nil {
					set("affinity", aff)
				}
			}
		case slotJob:
			if p.backoffLimit != nil {
				set("backoffLimit", *p.backoffLimit)
			}
		case slotCronJob:
			if p.backoffLimit != nil {
				setPath(overlay, append(append([]string{}, s.path...), "jobTemplate", "backoffLimit"), *p.backoffLimit)
			}
		case slotHPA:
			if p.hpaMin != nil {
				set("minReplicas", raise(s.node, "minReplicas", *p.hpaMin))
			}
			if p.hpaMax != nil {
				set("maxReplicas", raise(s.node, "maxReplicas", *p.hpaMax))
			}
		case slotPDB:
			// Only a numeric minAvailable is overridden: a percentage or maxUnavailable
			// in the base is left alone (setting both would make the PDB invalid).
			if _, ok := asInt(s.node["minAvailable"]); ok && p.pdbMinAvailable != nil && s.node["maxUnavailable"] == nil {
				set("minAvailable", *p.pdbMinAvailable)
			}
		}
	}
	return overlay
}

// antiAffinityFor returns a preferred pod anti-affinity (spread over nodes) for the
// pods selected by node.selector.matchLabels, or nil when the block has no such selector.
func antiAffinityFor(node map[string]interface{}) map[string]interface{} {
	selector, _ := node["selector"].(map[string]interface{})
	labels, _ := selector["matchLabels"].(map[string]interface{})
	if len(labels) == 0 {
		return nil
	}
	matchLabels := make(map[string]interface{}, len(labels))
	for k, v := range labels {
		matchLabels[k] = v
	}
	return map[string]interface{}{
		"podAntiAffinity": map[string]interface{}{
			"preferredDuringSchedulingIgnoredDuringExecution": []interface{}{
				map[string]interface{}{
					"weight": 100,
					"podAffinityTerm": map[string]interface{}{
						"labelSelector": map[string]interface{}{"matchLabels": matchLabels},
						"topologyKey":   "kubernetes.io/hostname",
					},
				},
			},
		},
	}
}

// setPath sets m[path[0]][path[1]]... = val, creating the intermediate maps.
func setPath(m map[string]interface{}, path []string, val interface{}) {
	for _, k := range path[:len(path)-1] {
		next, ok := m[k].(map[string]interface{})
		if !ok {
			next = map[string]interface{}{}
			m[k] = next
		}
		m = next
	}
	m[path[len(path)-1]] = val
}

// asInt reads a YAML/JSON number (int, int64 or float64 without fraction) as int.
func asInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		if n == float64(int(n)) {
			return int(n), true
		}
	}
	return 0, false
}

// ============================================================
// Workload-Aware Environment Profiles
// ============================================================

// WorkloadType represents the detected workload category for a service group.
type WorkloadType string

const (
	WorkloadWeb      WorkloadType = "web"
	WorkloadWorker   WorkloadType = "worker"
	WorkloadDatabase WorkloadType = "database"
	WorkloadBatch    WorkloadType = "batch"
	WorkloadCache    WorkloadType = "cache"
)

// extractContainers returns the list of container maps from a ProcessedResource
// whose spec follows the Pod template pattern (Deployment, StatefulSet, DaemonSet, Job, CronJob).
func extractContainers(resource *types.ProcessedResource) []map[string]interface{} {
	if resource == nil || resource.Original == nil {
		return nil
	}
	obj := resource.Original.Object.Object
	spec, ok := obj["spec"].(map[string]interface{})
	if !ok {
		return nil
	}
	// For Deployment/StatefulSet/DaemonSet: spec.template.spec.containers
	template, ok := spec["template"].(map[string]interface{})
	if !ok {
		return nil
	}
	tplSpec, ok := template["spec"].(map[string]interface{})
	if !ok {
		return nil
	}
	containers, ok := tplSpec["containers"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(containers))
	for _, c := range containers {
		if cm, ok := c.(map[string]interface{}); ok {
			result = append(result, cm)
		}
	}
	return result
}

// DetectWorkloadType infers the workload category from the resources in a ServiceGroup.
// Detection priority: Batch > Database+PVC > Database(image) > Cache > Web > Worker > default(Web).
func DetectWorkloadType(group *ServiceGroup) WorkloadType {
	var (
		hasService     bool
		hasIngress     bool
		hasStatefulSet bool
		hasPVC         bool
		hasJob         bool
		hasCronJob     bool
		hasDeployment  bool
	)

	// Flags for image/env-based heuristics.
	hasCacheImage := false
	hasDBImage := false
	hasWebPort := false
	hasAMQPKafkaEnv := false

	for _, res := range group.Resources {
		if res == nil || res.Original == nil {
			continue
		}
		kind := res.Original.GVK.Kind

		switch kind {
		case "Service":
			hasService = true
		case "Ingress", "HTTPRoute":
			hasIngress = true
		case "StatefulSet":
			hasStatefulSet = true
		case "PersistentVolumeClaim":
			hasPVC = true
		case "Job":
			hasJob = true
		case "CronJob":
			hasCronJob = true
		case "Deployment":
			hasDeployment = true
		}

		// Inspect containers for image names, ports, and env vars.
		for _, container := range extractContainers(res) {
			// Image-based detection.
			image, _ := container["image"].(string)
			imageLower := strings.ToLower(image)
			if strings.Contains(imageLower, "postgres") ||
				strings.Contains(imageLower, "mysql") ||
				strings.Contains(imageLower, "mongo") {
				hasDBImage = true
			}
			if strings.Contains(imageLower, "redis") ||
				strings.Contains(imageLower, "memcached") {
				hasCacheImage = true
			}

			// Port-based detection.
			if ports, ok := container["ports"].([]interface{}); ok {
				for _, p := range ports {
					if pm, ok := p.(map[string]interface{}); ok {
						var portNum int64
						switch v := pm["containerPort"].(type) {
						case int64:
							portNum = v
						case float64:
							portNum = int64(v)
						case int:
							portNum = int64(v)
						}
						if portNum == 80 || portNum == 443 || portNum == 8080 {
							hasWebPort = true
						}
					}
				}
			}

			// Env-var-based detection.
			if envList, ok := container["env"].([]interface{}); ok {
				for _, e := range envList {
					if em, ok := e.(map[string]interface{}); ok {
						name, _ := em["name"].(string)
						if strings.HasPrefix(name, "AMQP_") ||
							strings.HasPrefix(name, "KAFKA_") {
							hasAMQPKafkaEnv = true
						}
					}
				}
			}
		}
	}

	// Priority 1: Batch — any Job or CronJob in the group.
	if hasJob || hasCronJob {
		return WorkloadBatch
	}

	// Priority 2: Database — StatefulSet + PVC, or DB-specific image (with PVC).
	if (hasStatefulSet && hasPVC) || (hasDBImage && hasPVC) {
		return WorkloadDatabase
	}

	// Priority 3: Database by image alone (no PVC required for image-based DB detection
	// when there is no competing cache signal).
	if hasDBImage && !hasCacheImage {
		return WorkloadDatabase
	}

	// Priority 4: Cache — cache image (redis/memcached) without PVC.
	if hasCacheImage && !hasPVC {
		return WorkloadCache
	}

	// Priority 5: Web — has Ingress/HTTPRoute, or has Service + web port, or any web port.
	if hasIngress || hasWebPort {
		return WorkloadWeb
	}
	if hasService {
		return WorkloadWeb
	}

	// Priority 6: Worker — Deployment without Service, or AMQP/Kafka env vars.
	if hasDeployment || hasAMQPKafkaEnv {
		return WorkloadWorker
	}

	// Default to Web — safest profile (replicas, HPA, standard limits).
	return WorkloadWeb
}

// GenerateEnvValuesForWorkload generates environment-specific values files with
// workload-aware profiles. Returns a map with keys "values-dev.yaml",
// "values-staging.yaml", and "values-prod.yaml".
// The profile of workloadType is applied to the value blocks found in baseValues
// (see GenerateEnvValues); with no baseValues the files are empty overrides.
func GenerateEnvValuesForWorkload(baseValues map[string]interface{}, workloadType WorkloadType) map[string][]byte {
	profileFor := func(env string) envProfile { return buildWorkloadProfile(workloadType, env) }
	wt := string(workloadType)
	return renderEnvFiles(baseValues, profileFor, map[string]string{
		"dev":     "# Dev environment overrides (" + wt + " workload)\n",
		"staging": "# Staging environment overrides (" + wt + " workload)\n",
		"prod":    "# Production environment overrides (" + wt + " workload)\n",
	})
}

// buildWorkloadProfile returns the profile for a given workload type and environment.
func buildWorkloadProfile(workloadType WorkloadType, env string) envProfile {
	switch workloadType {
	case WorkloadWorker:
		return buildWorkerProfile(env)
	case WorkloadDatabase:
		return buildDatabaseProfile(env)
	case WorkloadBatch:
		return buildBatchProfile(env)
	case WorkloadCache:
		return buildCacheProfile(env)
	default:
		return buildWebProfile(env)
	}
}

// Web: HPA pinned to one replica in dev (an HPA cannot be switched off by values),
// 3..10 replicas with PDB minAvailable=2 in prod.
func buildWebProfile(env string) envProfile {
	switch env {
	case "dev":
		return envProfile{replicas: ip(1), hpaMin: ip(1), hpaMax: ip(1)}
	case "staging":
		return envProfile{replicas: ip(2)}
	default:
		return envProfile{replicas: ip(3), hpaMin: ip(3), hpaMax: ip(10), pdbMinAvailable: ip(2), antiAffinity: true}
	}
}

func buildWorkerProfile(env string) envProfile {
	switch env {
	case "dev":
		return envProfile{replicas: ip(1)}
	case "staging":
		return envProfile{replicas: ip(2)}
	default:
		return envProfile{replicas: ip(3), pdbMinAvailable: ip(1), antiAffinity: true}
	}
}

func buildDatabaseProfile(env string) envProfile {
	switch env {
	case "dev":
		return envProfile{replicas: ip(1)}
	case "staging":
		return envProfile{replicas: ip(2)}
	default:
		return envProfile{replicas: ip(3), pdbMinAvailable: ip(2), antiAffinity: true}
	}
}

func buildBatchProfile(env string) envProfile {
	switch env {
	case "dev":
		return envProfile{backoffLimit: ip(1)}
	case "staging":
		return envProfile{backoffLimit: ip(2)}
	default:
		return envProfile{backoffLimit: ip(3)}
	}
}

func buildCacheProfile(env string) envProfile {
	switch env {
	case "dev":
		return envProfile{replicas: ip(1)}
	case "staging":
		return envProfile{replicas: ip(2)}
	default:
		return envProfile{replicas: ip(2), pdbMinAvailable: ip(1), antiAffinity: true}
	}
}
