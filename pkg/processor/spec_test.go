package processor

import (
	"strings"
	"testing"
)

// The overlay's behaviour under Helm is covered by tests/golden (fixture
// custom-resources); these tests pin the template it produces.

func TestSpecOverlay_RendersWholeSpecWithOverrides(t *testing.T) {
	got := SpecOverlay("$svc.moduleConfig", "enabled", "settings")
	want := `{{- $dhgValues := $svc.moduleConfig }}
{{- $dhgSpec := deepCopy ($dhgValues.spec | default dict) }}
{{- range $k := list "enabled" "settings" }}
{{- if hasKey $dhgValues $k }}{{- $_ := set $dhgSpec $k (index $dhgValues $k) }}{{- end }}
{{- end }}
spec:
  {{- toYaml $dhgSpec | nindent 2 }}
`
	if got != want {
		t.Errorf("SpecOverlay:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(got, "mergeOverwrite") {
		t.Error("mergeOverwrite drops zero-valued overrides such as enabled: false")
	}
}

func TestSpecOverlayVars_NoKeysNoRange(t *testing.T) {
	got := SpecOverlayVars(".")
	if strings.Contains(got, "range") || strings.Contains(got, "spec:") {
		t.Errorf("without keys only the spec copy is built:\n%s", got)
	}
	if !strings.Contains(got, "{{- $dhgValues := . }}") {
		t.Errorf("dot expression not used:\n%s", got)
	}
}

func TestResourceNameSuffix(t *testing.T) {
	for in, want := range map[string]string{"webApp": "web-app", "api": "api", "myHTTPServer": "my-h-t-t-p-server", "": ""} {
		if got := ResourceNameSuffix(in); got != want {
			t.Errorf("ResourceNameSuffix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeServiceName(t *testing.T) {
	r := &Result{ServiceName: "web-app", ValuesPath: "services.web-app.deployment"}
	normalizeServiceName(r)
	if r.ServiceName != "webApp" || r.ValuesPath != "services.webApp.deployment" {
		t.Errorf("got %s %s", r.ServiceName, r.ValuesPath)
	}
	same := &Result{ServiceName: "web", ValuesPath: "services.web.deployment"}
	normalizeServiceName(same)
	if same.ServiceName != "web" || same.ValuesPath != "services.web.deployment" {
		t.Errorf("already sanitized name changed: %+v", same)
	}
}
