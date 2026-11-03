package types

import "testing"

func TestRolloutStopsBeforeReady(t *testing.T) {
	rolling := func(maxSurge, maxUnavailable interface{}) map[string]interface{} {
		ru := map[string]interface{}{}
		if maxSurge != nil {
			ru["maxSurge"] = maxSurge
		}
		if maxUnavailable != nil {
			ru["maxUnavailable"] = maxUnavailable
		}
		return map[string]interface{}{"type": "RollingUpdate", "rollingUpdate": ru}
	}
	tests := []struct {
		name     string
		replicas int64
		strategy map[string]interface{}
		want     bool
	}{
		{"no strategy, one replica: default 25% rounds to 0", 1, nil, false},
		{"one replica, surge 0, unavailable 1", 1, rolling(int64(0), int64(1)), true},
		{"one replica, surge 1, unavailable 0", 1, rolling(int64(1), int64(0)), false},
		{"unavailable as float64 (decoded JSON)", 1, rolling(float64(0), float64(1)), true},
		{"unavailable as int", 2, rolling(0, 2), true},
		{"two replicas, unavailable 1", 2, rolling(int64(0), int64(1)), false},
		{"percent 100% of 3", 3, rolling(nil, "100%"), true},
		{"percent 50% of 2 is 1, below replicas", 2, rolling(nil, "50%"), false},
		{"percent 50% of 1 rounds down to 0", 1, rolling(nil, "50%"), false},
		{"percent 100% of 1", 1, rolling("0%", "100%"), true},
		{"numeric string", 1, rolling(nil, "1"), true},
		{"only surge set, default unavailable", 1, rolling(int64(1), nil), false},
		{"recreate", 3, map[string]interface{}{"type": "Recreate"}, true},
		{"recreate, one replica", 1, map[string]interface{}{"type": "Recreate"}, true},
		{"recreate, no pods", 0, map[string]interface{}{"type": "Recreate"}, false},
		{"zero replicas", 0, rolling(int64(0), int64(1)), false},
		{"garbage unavailable falls back to default", 1, rolling(nil, "lots"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RolloutStopsBeforeReady(tt.replicas, tt.strategy)
			if (got != "") != tt.want {
				t.Fatalf("RolloutStopsBeforeReady(%d, %v) = %q, want reason: %v", tt.replicas, tt.strategy, got, tt.want)
			}
		})
	}
}

func TestRolloutStopsBeforeReadyReasonNamesTheCause(t *testing.T) {
	if got := RolloutStopsBeforeReady(1, map[string]interface{}{"type": "Recreate"}); got != "strategy is Recreate" {
		t.Errorf("reason = %q", got)
	}
	got := RolloutStopsBeforeReady(2, map[string]interface{}{"rollingUpdate": map[string]interface{}{"maxUnavailable": "100%"}})
	if got != "maxUnavailable 100% resolves to 2 of 2 replicas" {
		t.Errorf("reason = %q", got)
	}
}

// helm --wait counts a Deployment ready at replicas minus maxUnavailable, and
// the unavailable count is 0 for every strategy but RollingUpdate: only a
// RollingUpdate with maxUnavailable >= replicas lets --wait return before the
// new pod is Ready. Recreate has downtime, but --wait waits for the new pods.
func TestRolloutHelmWaitReturnsEarly(t *testing.T) {
	rolling := func(maxUnavailable interface{}) map[string]interface{} {
		return map[string]interface{}{"type": "RollingUpdate", "rollingUpdate": map[string]interface{}{"maxUnavailable": maxUnavailable}}
	}
	tests := []struct {
		name     string
		replicas int64
		strategy map[string]interface{}
		want     bool
	}{
		{"recreate", 3, map[string]interface{}{"type": "Recreate"}, false},
		{"recreate, one replica", 1, map[string]interface{}{"type": "Recreate"}, false},
		{"one replica, unavailable 1", 1, rolling(int64(1)), true},
		{"percent 100%", 2, rolling("100%"), true},
		{"unset type is RollingUpdate", 1, map[string]interface{}{"rollingUpdate": map[string]interface{}{"maxUnavailable": int64(1)}}, true},
		{"three replicas, unavailable 1", 3, rolling(int64(1)), false},
		{"default strategy", 1, nil, false},
		{"no pods", 0, rolling(int64(1)), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RolloutHelmWaitReturnsEarly(tt.replicas, tt.strategy); got != tt.want {
				t.Fatalf("RolloutHelmWaitReturnsEarly(%d, %v) = %v, want %v", tt.replicas, tt.strategy, got, tt.want)
			}
		})
	}
}
