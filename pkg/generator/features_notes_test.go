package generator

import (
	"reflect"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

func TestFeatureContextNoteNilSink(t *testing.T) {
	fc := FeatureContext{} // fc.Notes is nil
	// Should not panic
	fc.Note("x")
}

func TestFeatureNotesDeduplicatedAndOrder(t *testing.T) {
	var notes []string
	fc := FeatureContext{Notes: &notes}

	// Should ignore empty
	fc.Note("")
	fc.Note("   ")

	fc.Note(" b ") // spaces should be trimmed
	fc.Note("a")
	fc.Note("b") // duplicate
	fc.Note("  c ")
	fc.Note("a") // duplicate

	want := []string{"b", "a", "c"}
	if !reflect.DeepEqual(notes, want) {
		t.Errorf("expected notes %q, got %q", want, notes)
	}
}

func TestApplyFeaturesWithNotes(t *testing.T) {
	withTestFeature(t, Feature{
		Name:        "note-feature",
		Description: "test notes",
		Params:      map[string]string{},
		Apply: func(chart *types.GeneratedChart, fc FeatureContext) (*types.GeneratedChart, error) {
			fc.Note("same note")
			return chart, nil
		},
	})

	app1 := &types.GeneratedChart{Name: "app1", ChartYAML: "apiVersion: v2\nname: app1\ntype: application\n", Templates: map[string]string{}}
	app2 := &types.GeneratedChart{Name: "app2", ChartYAML: "apiVersion: v2\nname: app2\ntype: application\n", Templates: map[string]string{}}

	charts, notes, err := ApplyFeaturesWithNotes([]*types.GeneratedChart{app1, app2}, []string{"note-feature"}, nil, types.NewResourceGraph())
	if err != nil {
		t.Fatalf("ApplyFeaturesWithNotes failed: %v", err)
	}

	if len(charts) != 2 {
		t.Errorf("expected 2 charts, got %d", len(charts))
	}

	wantNotes := []string{"same note"}
	if !reflect.DeepEqual(notes, wantNotes) {
		t.Errorf("expected notes %q, got %q", wantNotes, notes)
	}
}
