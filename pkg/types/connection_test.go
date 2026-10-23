package types

import "testing"

func TestIsConnection(t *testing.T) {
	if !RelationServiceCall.IsConnection() {
		t.Error("RelationServiceCall should be a connection")
	}
}
func TestConnectionMethods(t *testing.T) {
	_ = Confidence(0).AtLeast(Confidence(1))
	ct := ConnectionTarget{}
	_ = ct.String()
	topo := &Topology{}
	topo.PBCOf("")
	topo.Unassigned()
}
