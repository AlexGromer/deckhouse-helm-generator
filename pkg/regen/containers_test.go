package regen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/generator"
)

// Values written before containers became a map by name (lists), the user's
// edit on top of them, and what the new generator writes.
const (
	valuesListBase = `services:
  web:
    deployment:
      replicas: 1
      containers:
        - name: app
          image:
            repository: reg/app
            tag: v1
        - name: proxy
          image:
            repository: reg/proxy
            tag: v1
      initContainers:
        - name: migrate
          image:
            repository: reg/app
            tag: v1
`
	valuesListOurs = `services:
  web:
    deployment:
      replicas: 3
      containers:
        - name: app
          image:
            repository: reg/app
            tag: v2-custom
        - name: proxy
          image:
            repository: reg/proxy
            tag: v1
      initContainers:
        - name: migrate
          image:
            repository: reg/app
            tag: v1
`
	valuesMapTheirs = `services:
  web:
    deployment:
      replicas: 1
      containers:
        app:
          image:
            repository: reg/app
            tag: v1
        proxy:
          image:
            repository: reg/proxy
            tag: v1
      containerOrder:
        - app
        - proxy
      initContainers:
        migrate:
          image:
            repository: reg/app
            tag: v1
      initContainerOrder:
        - migrate
`
)

func TestMigrateContainerLists_ConvertsListsToMaps(t *testing.T) {
	b, o := migrateContainerLists([]byte(valuesListBase), []byte(valuesListOurs), []byte(valuesMapTheirs))

	var got map[string]interface{}
	if err := yaml.Unmarshal(o, &got); err != nil {
		t.Fatalf("migrated ours is not YAML: %v\n%s", err, o)
	}
	dep := got["services"].(map[string]interface{})["web"].(map[string]interface{})["deployment"].(map[string]interface{})
	containers, ok := dep["containers"].(map[string]interface{})
	if !ok {
		t.Fatalf("containers = %T, want a map:\n%s", dep["containers"], o)
	}
	app := containers["app"].(map[string]interface{})
	if tag := app["image"].(map[string]interface{})["tag"]; tag != "v2-custom" {
		t.Errorf("the user's tag is lost: %v", tag)
	}
	if _, has := app["name"]; has {
		t.Errorf("containers.app keeps name: the key is the name now")
	}
	order := dep["containerOrder"].([]interface{})
	if len(order) != 2 || order[0] != "app" || order[1] != "proxy" {
		t.Errorf("containerOrder = %v, want [app proxy]", order)
	}
	if _, ok := dep["initContainers"].(map[string]interface{})["migrate"]; !ok {
		t.Errorf("initContainers.migrate missing:\n%s", o)
	}
	if io := dep["initContainerOrder"].([]interface{}); len(io) != 1 || io[0] != "migrate" {
		t.Errorf("initContainerOrder = %v", io)
	}
	if dep["replicas"] != 3 {
		t.Errorf("replicas = %v, want the user's 3", dep["replicas"])
	}
	if !strings.Contains(string(b), "containerOrder") {
		t.Errorf("base is not converted:\n%s", b)
	}
}

func TestMigrateContainerLists_LeavesOtherInputAlone(t *testing.T) {
	// Already maps: byte for byte the same.
	b, o := migrateContainerLists([]byte(valuesMapTheirs), []byte(valuesMapTheirs), []byte(valuesMapTheirs))
	if string(b) != valuesMapTheirs || string(o) != valuesMapTheirs {
		t.Errorf("map-form values were rewritten:\n%s", o)
	}
	// Theirs has no containers map: a containers list elsewhere is not touched.
	theirs := "custom:\n  containers:\n    - name: x\n"
	_, o = migrateContainerLists([]byte(theirs), []byte(theirs), []byte("custom: {}\n"))
	if string(o) != theirs {
		t.Errorf("a list outside the generated containers was rewritten:\n%s", o)
	}
	// Not YAML: unchanged.
	bad := "a: [\n"
	_, o = migrateContainerLists([]byte(bad), []byte(bad), []byte(valuesMapTheirs))
	if string(o) != bad {
		t.Errorf("invalid YAML was rewritten:\n%s", o)
	}
}

func TestBuildPlan_MergesOldListValuesIntoMapValues(t *testing.T) {
	dir := t.TempDir()
	base := generator.NewOutputSet()
	theirs := generator.NewOutputSet()
	if err := base.Add("app/values.yaml", []byte(valuesListBase), 0644); err != nil {
		t.Fatal(err)
	}
	if err := theirs.Add("app/values.yaml", []byte(valuesMapTheirs), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app/values.yaml"), []byte(valuesListOurs), 0644); err != nil {
		t.Fatal(err)
	}

	plan, err := BuildPlan(dir, base, theirs, PlanOptions{})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Files) != 1 {
		t.Fatalf("plan has %d files", len(plan.Files))
	}
	fp := plan.Files[0]
	if fp.Action != ActionMerged || fp.Conflicts != 0 {
		t.Fatalf("action = %s, conflicts = %d, want merged without conflicts:\n%s", fp.Action, fp.Conflicts, fp.Content)
	}
	var got map[string]interface{}
	if err := yaml.Unmarshal(fp.Content, &got); err != nil {
		t.Fatalf("merged values are not YAML: %v\n%s", err, fp.Content)
	}
	dep := got["services"].(map[string]interface{})["web"].(map[string]interface{})["deployment"].(map[string]interface{})
	app := dep["containers"].(map[string]interface{})["app"].(map[string]interface{})
	if tag := app["image"].(map[string]interface{})["tag"]; tag != "v2-custom" {
		t.Errorf("merged tag = %v, want the user's v2-custom:\n%s", tag, fp.Content)
	}
	if dep["replicas"] != 3 {
		t.Errorf("merged replicas = %v, want the user's 3", dep["replicas"])
	}
	if _, ok := dep["containerOrder"]; !ok {
		t.Errorf("containerOrder missing:\n%s", fp.Content)
	}
}

// The conversion rewrites the converted entries only: comments, blank lines
// and the style of every other sequence of the user's file stay as they are.
func TestMigrateContainerLists_KeepsTheRestOfTheFileByteForByte(t *testing.T) {
	ours := `# edited by hand
services:
  web:
    deployment:
      replicas: 3

      # the containers of the pod
      containers:
      - name: app
        image:
          repository: reg/app
          tag: v2-custom
      - name: proxy
        image:
          repository: reg/proxy

      nodeSelector:
        disk:
        - ssd
        - nvme
      podLabels:
        a: b
`
	_, got := migrateContainerLists([]byte(ours), []byte(ours), []byte(valuesMapTheirs))
	want := `# edited by hand
services:
  web:
    deployment:
      replicas: 3

      # the containers of the pod
      containers:
        app:
          image:
            repository: reg/app
            tag: v2-custom
        proxy:
          image:
            repository: reg/proxy
      containerOrder:
      - app
      - proxy

      nodeSelector:
        disk:
        - ssd
        - nvme
      podLabels:
        a: b
`
	if string(got) != want {
		t.Errorf("converted file differs from the expected one:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// Lists the splice cannot place (flow style) are converted by re-encoding.
func TestMigrateContainerLists_FlowListFallsBackToReencoding(t *testing.T) {
	ours := "services:\n  web:\n    deployment:\n      containers: [{name: app, image: {repository: reg/app}}]\n"
	_, got := migrateContainerLists([]byte(ours), []byte(ours), []byte(valuesMapTheirs))
	var parsed map[string]interface{}
	if err := yaml.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("not YAML: %v\n%s", err, got)
	}
	dep := parsed["services"].(map[string]interface{})["web"].(map[string]interface{})["deployment"].(map[string]interface{})
	if _, ok := dep["containers"].(map[string]interface{})["app"]; !ok {
		t.Errorf("containers.app missing:\n%s", got)
	}
	if order, _ := dep["containerOrder"].([]interface{}); len(order) != 1 || order[0] != "app" {
		t.Errorf("containerOrder = %v", dep["containerOrder"])
	}
}
