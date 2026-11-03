package pattern

import (
	"strings"
	"testing"
)

func haPractice(practices []BestPractice, id string) *BestPractice {
	for i := range practices {
		if practices[i].ID == id {
			return &practices[i]
		}
	}
	return nil
}

func TestHighAvailabilityChecker_RolloutStopsBeforeReady(t *testing.T) {
	tests := []struct {
		name     string
		replicas int64
		strategy map[string]interface{}
		want     bool
	}{
		{"surge 0, unavailable 1, one replica", 1, map[string]interface{}{
			"type": "RollingUpdate", "rollingUpdate": map[string]interface{}{"maxSurge": int64(0), "maxUnavailable": int64(1)}}, true},
		{"recreate", 2, map[string]interface{}{"type": "Recreate"}, true},
		{"percent resolved against replicas", 2, map[string]interface{}{
			"rollingUpdate": map[string]interface{}{"maxUnavailable": "100%"}}, true},
		{"surge 1, unavailable 0", 1, map[string]interface{}{
			"rollingUpdate": map[string]interface{}{"maxSurge": int64(1), "maxUnavailable": int64(0)}}, false},
		{"default strategy", 1, nil, false},
		{"three replicas, unavailable 1", 3, map[string]interface{}{
			"rollingUpdate": map[string]interface{}{"maxSurge": int64(0), "maxUnavailable": int64(1)}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := makeGraph()
			pr := addWorkloadWithContainers(g, "Deployment", "app", "app", nil)
			pr.Values["replicas"] = tt.replicas
			if tt.strategy != nil {
				pr.Values["strategy"] = tt.strategy
			}
			addGroup(g, "app", pr)

			p := haPractice(NewHighAvailabilityChecker().Check(g), "BP-HA-004")
			if (p != nil) != tt.want {
				t.Fatalf("BP-HA-004 reported = %v, want %v", p != nil, tt.want)
			}
			if p == nil {
				return
			}
			if p.Compliant || len(p.AffectedResources) != 1 {
				t.Errorf("practice = %+v", p)
			}
			joined := strings.Join(p.Recommendations, "\n")
			for _, want := range []string{"maxSurge: 1", "maxUnavailable: 0"} {
				if !strings.Contains(joined, want) {
					t.Errorf("recommendations lack %q:\n%s", want, joined)
				}
			}
			if p.AutoFixable {
				t.Error("the rollout strategy is not changed automatically")
			}
		})
	}
}

// The finding names helm --wait only for the workloads where it returns early.
func TestHighAvailabilityChecker_RolloutWaitText(t *testing.T) {
	build := func(replicas int64, strategy map[string]interface{}) *BestPractice {
		g := makeGraph()
		pr := addWorkloadWithContainers(g, "Deployment", "app", "app", nil)
		pr.Values["replicas"] = replicas
		pr.Values["strategy"] = strategy
		addGroup(g, "app", pr)
		return haPractice(NewHighAvailabilityChecker().Check(g), "BP-HA-004")
	}
	recreate := build(2, map[string]interface{}{"type": "Recreate"})
	if recreate == nil {
		t.Fatal("no BP-HA-004 for Recreate")
	}
	if strings.Contains(recreate.Description, "helm --wait") {
		t.Errorf("Recreate: helm --wait waits for the new pods, the text must not say otherwise: %s", recreate.Description)
	}
	rolling := build(1, map[string]interface{}{"rollingUpdate": map[string]interface{}{"maxSurge": int64(0), "maxUnavailable": int64(1)}})
	if rolling == nil {
		t.Fatal("no BP-HA-004 for maxUnavailable >= replicas")
	}
	if !strings.Contains(rolling.Description, "helm --wait does not wait for readiness") {
		t.Errorf("RollingUpdate with maxUnavailable >= replicas: %s", rolling.Description)
	}
}
