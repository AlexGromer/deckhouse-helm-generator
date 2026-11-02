package k8s

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// containersMapObj is a Deployment whose containers and init containers are
// not in alphabetical order, so a render that sorts them is caught.
func containersMapObj() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "web", "namespace": "ns"},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{"spec": map[string]interface{}{
				"initContainers": []interface{}{
					map[string]interface{}{"name": "wait-db", "image": "busybox:1"},
					map[string]interface{}{"name": "migrate", "image": "app:v1"},
				},
				"containers": []interface{}{
					map[string]interface{}{"name": "proxy", "image": "envoy:v1"},
					map[string]interface{}{"name": "app", "image": "reg:5000/app:v1"},
				},
			}},
		},
	}}
}

// orderedContainers returns the containers stored under key ("containers" or
// "initContainers") in the order of their order list, each with its name
// set again, so a test can address them by position.
func orderedContainers(values map[string]interface{}, key string) ([]map[string]interface{}, bool) {
	byName, ok := values[key].(map[string]interface{})
	if !ok {
		return nil, false
	}
	order, _ := values[containerOrderKey(key)].([]interface{})
	out := make([]map[string]interface{}, 0, len(order))
	for _, n := range order {
		name, _ := n.(string)
		c, ok := byName[name].(map[string]interface{})
		if !ok {
			return nil, false
		}
		withName := map[string]interface{}{"name": name}
		for k, v := range c {
			withName[k] = v
		}
		out = append(out, withName)
	}
	return out, true
}

func TestExtractPodTemplateValues_ContainersAreMaps(t *testing.T) {
	values := map[string]interface{}{}
	extractPodTemplateValues(containersMapObj(), values, "spec", "template")

	containers, ok := values["containers"].(map[string]interface{})
	if !ok {
		t.Fatalf("containers is %T, want map[string]interface{}", values["containers"])
	}
	app, ok := containers["app"].(map[string]interface{})
	if !ok {
		t.Fatalf("containers[app] is %T, want a map", containers["app"])
	}
	img, _ := app["image"].(map[string]interface{})
	if img["repository"] != "reg:5000/app" || img["tag"] != "v1" {
		t.Errorf("containers[app].image = %v", img)
	}
	if _, has := app["name"]; has {
		t.Errorf("containers[app] repeats its name %v: the map key is the name", app["name"])
	}
	if got, want := values["containerOrder"], []interface{}{"proxy", "app"}; !reflect.DeepEqual(got, want) {
		t.Errorf("containerOrder = %#v, want %#v", got, want)
	}

	inits, ok := values["initContainers"].(map[string]interface{})
	if !ok {
		t.Fatalf("initContainers is %T, want map[string]interface{}", values["initContainers"])
	}
	if _, ok := inits["migrate"]; !ok {
		t.Errorf("initContainers lacks migrate: %v", inits)
	}
	if got, want := values["initContainerOrder"], []interface{}{"wait-db", "migrate"}; !reflect.DeepEqual(got, want) {
		t.Errorf("initContainerOrder = %#v, want %#v", got, want)
	}
}

func TestPodTemplate_RendersContainersByOrder(t *testing.T) {
	tmpl := podTemplate("app", 2, "", "Deployment/web")
	for _, want := range []string{
		`.containerOrder`,
		`.initContainerOrder`,
		`$dhgName`,
	} {
		if !strings.Contains(tmpl, want) {
			t.Errorf("podTemplate lacks %q:\n%s", want, tmpl)
		}
	}
	if strings.Contains(tmpl, `range .containers }}`) {
		t.Errorf("podTemplate still ranges over the containers value:\n%s", tmpl)
	}
	// Anchor of the text patchers (anti-affinity).
	found := false
	for _, line := range strings.Split(tmpl, "\n") {
		if strings.TrimSpace(line) == "containers:" {
			found = true
		}
	}
	if !found {
		t.Error(`podTemplate has no literal "containers:" line`)
	}
}

// renderedContainerImages returns the images of the rendered Deployment's
// init containers and containers, in rendered order.
func renderedContainerImages(t *testing.T, rendered string) (inits, mains []string) {
	t.Helper()
	section := ""
	re := regexp.MustCompile(`^\s*image:\s*"?([^"\s]+)"?\s*$`)
	for _, line := range strings.Split(rendered, "\n") {
		switch strings.TrimSpace(line) {
		case "initContainers:":
			section = "init"
			continue
		case "containers:":
			section = "main"
			continue
		}
		if m := re.FindStringSubmatch(line); m != nil {
			switch section {
			case "init":
				inits = append(inits, m[1])
			case "main":
				mains = append(mains, m[1])
			}
		}
	}
	return inits, mains
}

func TestHelmTemplateContainersMap(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		if os.Getenv("DHG_REQUIRE_HELM") != "" {
			t.Fatalf("helm not found: %v", err)
		}
		t.Skip("helm not installed")
	}
	dir := t.TempDir()
	input := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  selector:
    matchLabels: {app: web}
  template:
    metadata:
      labels: {app: web}
    spec:
      initContainers:
      - name: wait-db
        image: busybox:1
      - name: migrate
        image: app:v1
      containers:
      - name: proxy
        image: envoy:v1
      - name: app
        image: reg:5000/app:v1
`
	inputDir := filepath.Join(dir, "input")
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "m.yaml"), []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	gen := exec.Command("go", "run", "../../../cmd/dhg", "generate", "-f", inputDir, "--chart-name", "app", "--mode", "universal", "-o", outDir)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("dhg generate failed: %v\n%s", err, out)
	}
	helmPath := "helm"
	if p := os.Getenv("DHG_HELM_PATH"); p != "" {
		helmPath = p
	}
	chart := filepath.Join(outDir, "app")
	template := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(helmPath, append([]string{"template", "r", chart}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("helm template %v failed: %v\n%s", args, err, out)
		}
		return string(out)
	}

	// The default render keeps the input order.
	inits, mains := renderedContainerImages(t, template())
	if want := []string{"busybox:1", "app:v1"}; !reflect.DeepEqual(inits, want) {
		t.Errorf("init container images = %v, want %v", inits, want)
	}
	if want := []string{"envoy:v1", "reg:5000/app:v1"}; !reflect.DeepEqual(mains, want) {
		t.Errorf("container images = %v, want %v", mains, want)
	}

	// A --set on one container reaches only that container.
	inits, mains = renderedContainerImages(t, template(
		"--set", "services.web.deployment.containers.app.image.tag=v2",
		"--set", "services.web.deployment.initContainers.migrate.image.tag=v3",
	))
	if want := []string{"busybox:1", "app:v3"}; !reflect.DeepEqual(inits, want) {
		t.Errorf("init container images with --set = %v, want %v", inits, want)
	}
	if want := []string{"envoy:v1", "reg:5000/app:v2"}; !reflect.DeepEqual(mains, want) {
		t.Errorf("container images with --set = %v, want %v", mains, want)
	}
}
