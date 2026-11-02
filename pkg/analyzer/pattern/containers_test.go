package pattern

import "testing"

func TestContainersOf_MapFormFollowsOrderAndFillsName(t *testing.T) {
	values := map[string]interface{}{
		"containers": map[string]interface{}{
			"b": map[string]interface{}{"image": "x"},
			"a": map[string]interface{}{"image": "y"},
		},
		"containerOrder": []interface{}{"b", "a"},
	}
	list, ok := containersOf(values, "containers")
	if !ok || len(list) != 2 || list[0]["name"] != "b" || list[1]["name"] != "a" {
		t.Fatalf("got %v ok=%v, want [b a]", list, ok)
	}
	if _, polluted := values["containers"].(map[string]interface{})["b"].(map[string]interface{})["name"]; polluted {
		t.Error("containersOf must not modify the input values")
	}
}

func TestContainersOf_SortedFallbackAndLegacyList(t *testing.T) {
	values := map[string]interface{}{
		"initContainers": map[string]interface{}{"z": map[string]interface{}{}, "m": map[string]interface{}{}},
	}
	list, ok := containersOf(values, "initContainers")
	if !ok || len(list) != 2 || list[0]["name"] != "m" || list[1]["name"] != "z" {
		t.Fatalf("got %v ok=%v, want sorted [m z]", list, ok)
	}
	legacy := map[string]interface{}{"containers": []map[string]interface{}{{"name": "only"}}}
	if l, ok := containersOf(legacy, "containers"); !ok || len(l) != 1 {
		t.Fatalf("legacy list form: got %v ok=%v", l, ok)
	}
	if _, ok := containersOf(map[string]interface{}{}, "containers"); ok {
		t.Error("absent field must give ok=false")
	}
}
