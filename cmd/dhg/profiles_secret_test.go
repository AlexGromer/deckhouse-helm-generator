package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// A profile that adds a credential makes the synthesis build a Secret the
// base chart does not have; its values overlay must not point envFrom at it.
func TestProfileValuesDoNotRefDanglingSecret(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"pom.xml": `<project><artifactId>shop</artifactId><version>1.0.0</version><dependencies>
<dependency><groupId>org.springframework.boot</groupId><artifactId>spring-boot-starter-web</artifactId></dependency>
</dependencies></project>`,
		"Dockerfile": "FROM eclipse-temurin:21-jre\nEXPOSE 8080\n",
		"src/main/resources/application.yml": "spring:\n  application:\n    name: shop\n  datasource:\n    username: shop\n" +
			"---\nspring:\n  config:\n    activate:\n      on-profile: prod\n  datasource:\n    password: ${DB_PASSWORD}\n",
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := t.TempDir()
	if _, err := executeCmd(t, "generate", "-s", "source", "-f", dir, "--image", "shop:1", "-o", out, "--chart-name", "shop", "--mode", "universal"); err != nil {
		t.Fatal(err)
	}
	overlay, err := os.ReadFile(filepath.Join(out, "shop", "values-profile-prod.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(overlay), "secretRef") {
		t.Errorf("overlay references a Secret the base chart does not define:\n%s", overlay)
	}
	report, _ := os.ReadFile(filepath.Join(out, "SYNTHESIS.md"))
	if !strings.Contains(string(report), "shop-env") || !strings.Contains(string(report), "prod") || !strings.Contains(string(report), "create") {
		t.Errorf("SYNTHESIS.md does not name the Secret to create:\n%s", report)
	}
}

// The other ways to reference a Secret (env secretKeyRef, a secret volume,
// imagePullSecrets) are dropped too, and make the note appear.
func TestDropMissingSecretsOtherReferences(t *testing.T) {
	overlay := map[string]interface{}{
		"services": map[string]interface{}{
			"web": map[string]interface{}{
				"imagePullSecrets": []interface{}{
					map[string]interface{}{"name": "pull-prof"},
					map[string]interface{}{"name": "pull-base"},
				},
				"volumes": []interface{}{
					map[string]interface{}{"name": "creds", "secret": map[string]interface{}{"secretName": "vol-prof"}},
					map[string]interface{}{"name": "keep", "secret": map[string]interface{}{"secretName": "vol-base"}},
				},
				"containers": []interface{}{
					map[string]interface{}{
						"name": "app",
						"env": []interface{}{
							map[string]interface{}{"name": "A", "valueFrom": map[string]interface{}{"secretKeyRef": map[string]interface{}{"name": "env-prof", "key": "k"}}},
							map[string]interface{}{"name": "B", "valueFrom": map[string]interface{}{"secretKeyRef": map[string]interface{}{"name": "env-base", "key": "k"}}},
							map[string]interface{}{"name": "C", "value": "plain"},
						},
						"volumeMounts": []interface{}{
							map[string]interface{}{"name": "creds", "mountPath": "/creds"},
							map[string]interface{}{"name": "keep", "mountPath": "/keep"},
						},
					},
				},
			},
		},
	}
	missing := map[string]bool{"pull-prof": true, "vol-prof": true, "env-prof": true}
	got := dropMissingSecrets(overlay, map[string]interface{}{}, missing)
	if strings.Join(got, ",") != "env-prof,pull-prof,vol-prof" {
		t.Errorf("found = %v, want env-prof, pull-prof, vol-prof", got)
	}
	out, err := yaml.Marshal(overlay)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, gone := range []string{"pull-prof", "vol-prof", "env-prof", "creds"} {
		if strings.Contains(text, gone) {
			t.Errorf("overlay still mentions %q:\n%s", gone, text)
		}
	}
	for _, kept := range []string{"pull-base", "vol-base", "env-base", "name: C", "/keep"} {
		if !strings.Contains(text, kept) {
			t.Errorf("overlay lost %q:\n%s", kept, text)
		}
	}
}
