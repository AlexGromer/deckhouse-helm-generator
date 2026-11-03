package k8s

import (
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/testutil"
)

func deploymentWithStrategy(replicas int64, strategy map[string]interface{}) map[string]interface{} {
	spec := makeBasicSpec(replicas, "app", "app:1.0")
	if strategy != nil {
		spec["strategy"] = strategy
	}
	return spec
}

func TestProcessDeployment_RolloutNote(t *testing.T) {
	tests := []struct {
		name     string
		replicas int64
		strategy map[string]interface{}
		wantNote bool
	}{
		{"single replica, surge 0, unavailable 1", 1, map[string]interface{}{
			"type": "RollingUpdate", "rollingUpdate": map[string]interface{}{"maxSurge": int64(0), "maxUnavailable": int64(1)}}, true},
		{"single replica, percent 100%", 1, map[string]interface{}{
			"rollingUpdate": map[string]interface{}{"maxUnavailable": "100%"}}, true},
		{"recreate", 3, map[string]interface{}{"type": "Recreate"}, true},
		{"single replica, surge 1, unavailable 0", 1, map[string]interface{}{
			"type": "RollingUpdate", "rollingUpdate": map[string]interface{}{"maxSurge": int64(1), "maxUnavailable": int64(0)}}, false},
		{"default strategy", 1, nil, false},
		{"three replicas, unavailable 1", 3, map[string]interface{}{
			"rollingUpdate": map[string]interface{}{"maxSurge": int64(0), "maxUnavailable": int64(1)}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := makeDeploymentObj("web", "prod", nil, deploymentWithStrategy(tt.replicas, tt.strategy))
			result, err := NewDeploymentProcessor().Process(newTestProcessorContext(), obj)
			testutil.AssertNoError(t, err)
			if !tt.wantNote {
				testutil.AssertEqual(t, 0, len(result.Notes))
				return
			}
			if len(result.Notes) != 1 {
				t.Fatalf("notes = %q, want one", result.Notes)
			}
			note := result.Notes[0]
			// helm --wait waits for the new pods of a Recreate Deployment; only a
			// RollingUpdate with maxUnavailable >= replicas lets it return early.
			if recreate := tt.strategy["type"] == "Recreate"; recreate {
				if strings.Contains(note, "helm --wait") {
					t.Errorf("a Recreate note says nothing about helm --wait: %s", note)
				}
			} else {
				testutil.AssertContains(t, note, "helm --wait does not wait for readiness")
			}
			for _, want := range []string{
				"Deployment web",
				"before a new one is Ready",
				"services.web.deployment.strategy.rollingUpdate.maxSurge=1",
				"services.web.deployment.strategy.rollingUpdate.maxUnavailable=0",
			} {
				testutil.AssertContains(t, note, want)
			}
		})
	}
}

// The strategy of the input stays as it is: the note only suggests values.
func TestProcessDeployment_RolloutNoteKeepsStrategy(t *testing.T) {
	strategy := map[string]interface{}{"type": "Recreate"}
	obj := makeDeploymentObj("web", "prod", nil, deploymentWithStrategy(1, strategy))
	result, err := NewDeploymentProcessor().Process(newTestProcessorContext(), obj)
	testutil.AssertNoError(t, err)
	got, _ := result.Values["strategy"].(map[string]interface{})
	testutil.AssertEqual(t, "Recreate", got["type"])
	// A Recreate strategy needs the type switched together with the knobs.
	if !strings.Contains(result.Notes[0], "services.web.deployment.strategy.type=RollingUpdate") {
		t.Errorf("a Recreate note must suggest strategy.type=RollingUpdate: %s", result.Notes[0])
	}
}
