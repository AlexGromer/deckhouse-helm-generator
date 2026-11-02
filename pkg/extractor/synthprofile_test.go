package extractor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const ordersFixture = "../synth/testdata/orders"

const extractorProfile = `
vars: {prefix: acme-dev-}
envFrom:
  - configMapRef: "{{ .Vars.prefix }}system-config"
podLabels: {app: "{{ .Name }}"}
defaults:
  resources: {requests: {cpu: 500m, memory: 1Gi}}
  probes:
    readiness: {tcp: true, periodSeconds: 30}
jobs:
  - when: liquibase
    name: "database-{{ .Name }}"
    manifest:
      spec:
        template:
          spec:
            restartPolicy: Never
            containers:
              - {name: migrate, image: "registry.example.com/db/{{ .Name }}:1"}
`

func writeProfile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSourceExtractorAppliesTheSynthProfile(t *testing.T) {
	opts := Options{Paths: []string{ordersFixture}, Images: []string{"registry.example.com/orders:1"}, SynthProfile: writeProfile(t, extractorProfile)}
	s := NewSourceExtractor()
	if err := s.Validate(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	rs, err := drain(s.Extract(context.Background(), opts))
	if err != nil {
		t.Fatal(err)
	}
	got := kinds(rs)
	for _, want := range []string{"Deployment/orders", "Job/database-orders", "ConfigMap/orders-env", "Secret/orders-env"} {
		if !strings.Contains(got, want) {
			t.Errorf("resources lack %s: %s", want, got)
		}
	}
	for _, r := range rs {
		if r.Object.GetKind() != "Deployment" {
			continue
		}
		envFrom, _, _ := unstructured.NestedSlice(r.Object.Object, "spec", "template", "spec", "containers")
		first := envFrom[0].(map[string]interface{})["envFrom"].([]interface{})[0].(map[string]interface{})
		if first["configMapRef"].(map[string]interface{})["name"] != "acme-dev-system-config" {
			t.Errorf("envFrom = %v", envFrom[0])
		}
		if lbl := r.Object.GetLabels(); lbl["app"] != "" {
			t.Logf("deployment labels %v", lbl)
		}
	}
	if !strings.Contains(strings.Join(s.Inputs(), "\n"), "profile.yaml") {
		t.Errorf("inputs = %v: the profile must be listed", s.Inputs())
	}
	// Variants (Spring profiles) get the platform profile too.
	for name, variant := range s.Variants() {
		if !strings.Contains(kinds(variant), "Job/database-orders") {
			t.Errorf("variant %s lacks the profile's Job: %s", name, kinds(variant))
		}
	}
}

func TestSynthProfileErrorsAreReported(t *testing.T) {
	s := NewSourceExtractor()
	for name, path := range map[string]string{
		"missing":     filepath.Join(t.TempDir(), "none.yaml"),
		"unknown key": writeProfile(t, "initContainer: []\n"),
	} {
		opts := Options{Paths: []string{ordersFixture}, SynthProfile: path}
		if err := s.Validate(context.Background(), opts); err == nil {
			t.Errorf("%s: Validate must fail", name)
		}
		if _, err := drain(NewSourceExtractor().Extract(context.Background(), opts)); err == nil {
			t.Errorf("%s: Extract must fail", name)
		}
	}
	// A template that needs a variable the profile lacks fails the extraction.
	opts := Options{Paths: []string{ordersFixture}, SynthProfile: writeProfile(t, "podLabels: {a: \"{{ .Vars.nope }}\"}\n")}
	if _, err := drain(NewSourceExtractor().Extract(context.Background(), opts)); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("err = %v", err)
	}
}

func TestComposeAndImageExtractorsApplyTheSynthProfile(t *testing.T) {
	compose := filepath.Join(t.TempDir(), "docker-compose.yml")
	if err := os.WriteFile(compose, []byte("services:\n  api:\n    image: api:1\n    ports: ['8080:8080']\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := Options{Paths: []string{compose}, SynthProfile: writeProfile(t, "vars: {prefix: acme-}\nenvFrom:\n  - configMapRef: \"{{ .Vars.prefix }}cfg\"\n")}
	rs, err := drain(NewComposeExtractor().Extract(context.Background(), opts))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rs {
		if r.Object.GetKind() == "Deployment" {
			cs, _, _ := unstructured.NestedSlice(r.Object.Object, "spec", "template", "spec", "containers")
			ef, _ := cs[0].(map[string]interface{})["envFrom"].([]interface{})
			if len(ef) != 1 || ef[0].(map[string]interface{})["configMapRef"].(map[string]interface{})["name"] != "acme-cfg" {
				t.Errorf("envFrom = %v", ef)
			}
		}
	}
	if _, err := drain(NewComposeExtractor().Extract(context.Background(), Options{Paths: []string{compose}, SynthProfile: "/nonexistent.yaml"})); err == nil {
		t.Error("a missing profile must fail")
	}
}

func TestNoProfileNoChange(t *testing.T) {
	rs, err := drain(NewSourceExtractor().Extract(context.Background(), Options{Paths: []string{ordersFixture}}))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rs {
		if r.Object.GetKind() == "Job" {
			t.Errorf("a Job without a profile: %s", r.Object.GetName())
		}
	}
	_ = types.SourceCode
}
