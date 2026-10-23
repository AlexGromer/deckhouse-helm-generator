package types

import "testing"

func TestGraphInit(t *testing.T) {
	g := NewResourceGraph()
	if g == nil {
		t.Error("expected non-nil")
	}
}
