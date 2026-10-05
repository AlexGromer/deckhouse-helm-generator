package processor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func writePlugin(t *testing.T, body string) string {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	path := filepath.Join(t.TempDir(), "plugin.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func widget() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("example.com/v1")
	obj.SetKind("Widget")
	obj.SetName("w1")
	return obj
}

var widgetGVK = schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"}

func TestPluginProcessor_HandlesResource(t *testing.T) {
	plugin := writePlugin(t, `input=$(cat)
case "$input" in *'"name":"w1"'*) ;; *) echo "unexpected input: $input" >&2; exit 1;; esac
printf '%s' '{"serviceName":"w1","templatePath":"templates/w1.yaml","templateContent":"kind: Widget\n","valuesPath":"services.w1.widget","values":{"size":3}}'
`)
	r := NewRegistry()
	r.Register(NewPluginProcessor(plugin, 5*time.Second, widgetGVK))

	res, err := r.Process(Context{ChartName: "app"}, widget())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Processed || res.TemplatePath != "templates/w1.yaml" || res.Values["size"] != float64(3) {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestPluginProcessor_EmptyOutputFallsBack(t *testing.T) {
	plugin := writePlugin(t, "cat >/dev/null\nprintf '{}'\n")
	r := NewRegistry()
	r.Register(NewPluginProcessor(plugin, 5*time.Second, widgetGVK))

	res, err := r.Process(Context{ChartName: "app"}, widget())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Processed || !strings.Contains(res.TemplateContent, "kind: Widget") {
		t.Errorf("expected the generic fallback to handle the resource, got %+v", res)
	}
}

func TestPluginProcessor_Errors(t *testing.T) {
	for name, body := range map[string]string{
		"exit":    "cat >/dev/null\necho boom >&2\nexit 3\n",
		"garbage": "cat >/dev/null\necho not-json\n",
		"partial": "cat >/dev/null\nprintf '{\"templateContent\":\"x\"}'\n",
	} {
		plugin := writePlugin(t, body)
		_, err := NewPluginProcessor(plugin, 5*time.Second, widgetGVK).Process(Context{}, widget())
		if err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	slow := writePlugin(t, "sleep 5\n")
	if _, err := NewPluginProcessor(slow, 100*time.Millisecond, widgetGVK).Process(Context{}, widget()); err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected timeout, got %v", err)
	}
}

func TestParsePluginSpec(t *testing.T) {
	path, gvks, err := ParsePluginSpec("example.com/v1/Widget,v1/ConfigMap=./bin/p")
	if err != nil {
		t.Fatal(err)
	}
	if path != "./bin/p" || len(gvks) != 2 || gvks[0] != widgetGVK || gvks[1] != (schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}) {
		t.Errorf("unexpected: %s %v", path, gvks)
	}
	for _, bad := range []string{"Widget=./p", "example.com/v1/Widget", "=./p", "example.com/v1/=./p"} {
		if _, _, err := ParsePluginSpec(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}
