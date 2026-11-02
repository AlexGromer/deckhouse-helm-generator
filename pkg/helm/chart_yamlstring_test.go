package helm

import "testing"

func TestYamlString(t *testing.T) {
	for _, v := range []string{"0.1.0", "^1.2.0", "~1.2.0", "1.x"} {
		if got := yamlString(v); got != v {
			t.Errorf("yamlString(%q) = %q, want it plain", v, got)
		}
	}
	for _, v := range []string{">=1.0.0 <2.0.0", "*", "1.20", "true", ""} {
		if got := yamlString(v); got != `"`+v+`"` {
			t.Errorf("yamlString(%q) = %q, want it quoted", v, got)
		}
	}
}
