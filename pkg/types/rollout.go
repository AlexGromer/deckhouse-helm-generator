package types

import (
	"fmt"
	"strconv"
	"strings"
)

// defaultMaxUnavailable is what Kubernetes applies to a RollingUpdate
// Deployment that leaves maxUnavailable unset.
const defaultMaxUnavailable = "25%"

// RolloutStopsBeforeReady reports why a rollout of a Deployment with the
// given replicas and spec.strategy may stop the old pod before a new one is
// Ready, which means downtime. It returns "" when the rollout keeps a ready
// pod, and for a workload without replicas. Whether `helm --wait` also
// returns before the new pod is Ready is a narrower question, answered by
// RolloutHelmWaitReturnsEarly.
//
// Recreate always stops the old pods first. For RollingUpdate the rollout is
// unsafe when maxUnavailable, resolved against replicas, is at least the
// replica count; a percent rounds down, as Kubernetes rounds it.
func RolloutStopsBeforeReady(replicas int64, strategy map[string]interface{}) string {
	if replicas <= 0 {
		return ""
	}
	if t, _ := strategy["type"].(string); t == "Recreate" {
		return "strategy is Recreate"
	}
	ru, _ := strategy["rollingUpdate"].(map[string]interface{})
	raw := ru["maxUnavailable"]
	resolved, ok := resolveIntOrPercent(raw, replicas)
	if !ok {
		raw = defaultMaxUnavailable
		resolved, _ = resolveIntOrPercent(raw, replicas)
	}
	if resolved < replicas {
		return ""
	}
	return fmt.Sprintf("maxUnavailable %v resolves to %d of %d replicas", raw, resolved, replicas)
}

// RolloutHelmWaitReturnsEarly reports whether `helm --wait` may return before
// the new pod of a Deployment is Ready. Helm counts the Deployment ready at
// replicas minus maxUnavailable, and that count is 0 for every strategy but
// RollingUpdate, so a Recreate Deployment has downtime but --wait waits for its
// new pods. Only a RollingUpdate whose maxUnavailable is at least the replica
// count returns early.
func RolloutHelmWaitReturnsEarly(replicas int64, strategy map[string]interface{}) bool {
	if t, _ := strategy["type"].(string); t == "Recreate" {
		return false
	}
	return RolloutStopsBeforeReady(replicas, strategy) != ""
}

// resolveIntOrPercent resolves a Kubernetes IntOrString against total (a
// percent rounds down). ok is false for a value that is neither a number nor
// a percent.
func resolveIntOrPercent(v interface{}, total int64) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case string:
		s := strings.TrimSpace(n)
		if pct, found := strings.CutSuffix(s, "%"); found {
			p, err := strconv.ParseInt(strings.TrimSpace(pct), 10, 64)
			if err != nil {
				return 0, false
			}
			return total * p / 100, true
		}
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, false
		}
		return i, true
	}
	return 0, false
}
