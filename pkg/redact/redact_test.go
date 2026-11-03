package redact

import (
	"fmt"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/AlexGromer/deckhouse-helm-generator/pkg/types"
)

const fixture = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: shop
data:
  APP_TOKENSERVICE_CLIENTSECRET: real-client-secret
  APP_TOKENSERVICE_CLIENTID: shop-client
  APP_TOKENSERVICE_GRANTTYPE: client_credentials
  APP_TOKENSERVICE_SERVICE: tokens
  ACCESS_TOKEN_VALIDITY_SECONDS: "300"
  TOKEN_URL: https://auth.example.com/token
  KEYCLOAK_URL: https://auth.example.com
  USE_TOKEN_FOR_FEIGN: "true"
  QOAUTH_VALIDATE_TOKENS: "false"
  secretName: not-a-credential
  existingSecret: db-credentials
  EMPTY_PASSWORD: ""
binaryData:
  DB_PASSWORD: c2VjcmV0
  LOGO: aGVsbG8=
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: shop
spec:
  template:
    spec:
      initContainers:
      - name: init
        image: busybox
        env:
        - name: INIT_API_KEY
          value: init-key
      containers:
      - name: app
        image: shop:1
        envFrom:
        - secretRef:
            name: db-credentials
        - configMapRef:
            name: app-config
        env:
        - name: KC_DB_PASSWORD
          value: hunter2
        - name: KC_DB_PASSWORD_FROM_SECRET
          valueFrom:
            secretKeyRef:
              name: db-credentials
              key: password
        - name: DB_SECRET_NAME
          value: db-credentials
        - name: KC_DB_URL
          value: jdbc:postgresql://db/kc
        - name: ACCESS_TOKEN_VALIDITY_SECONDS
          value: "300"
        - name: JAVA_OPTS_APPEND
          value: "-Xmx512m -Djavax.net.ssl.trustStorePassword=changeit -Djavax.net.ssl.trustStore=/etc/ssl/ts.jks -Dapp.keyStorePassword:s3cr3t -Dapp.name=shop"
      volumes:
      - name: creds
        secret:
          secretName: db-credentials
`

func load(t *testing.T) []*types.ExtractedResource {
	t.Helper()
	var out []*types.ExtractedResource
	for _, doc := range strings.Split(fixture, "\n---\n") {
		obj := map[string]interface{}{}
		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
			t.Fatal(err)
		}
		u := &unstructured.Unstructured{Object: obj}
		out = append(out, &types.ExtractedResource{Object: u, GVK: u.GroupVersionKind(), Source: types.SourceCluster})
	}
	return out
}

func get(t *testing.T, r *types.ExtractedResource, path ...string) interface{} {
	t.Helper()
	v, found, err := unstructured.NestedFieldNoCopy(r.Object.Object, path...)
	if err != nil || !found {
		t.Fatalf("%s: %v found=%v", strings.Join(path, "."), err, found)
	}
	return v
}

func TestConfigMapAndEnvAreRedacted(t *testing.T) {
	res := load(t)
	cm, dep := res[0], res[1]
	items := Credentials(res)

	for key, want := range map[string]string{
		"APP_TOKENSERVICE_CLIENTSECRET": Placeholder,
		"APP_TOKENSERVICE_CLIENTID":     "shop-client",
		"APP_TOKENSERVICE_GRANTTYPE":    "client_credentials",
		"APP_TOKENSERVICE_SERVICE":      "tokens",
		"ACCESS_TOKEN_VALIDITY_SECONDS": "300",
		"TOKEN_URL":                     "https://auth.example.com/token",
		"KEYCLOAK_URL":                  "https://auth.example.com",
		"USE_TOKEN_FOR_FEIGN":           "true",
		"QOAUTH_VALIDATE_TOKENS":        "false",
		"secretName":                    "not-a-credential",
		"existingSecret":                "db-credentials",
		"EMPTY_PASSWORD":                "",
	} {
		if got := get(t, cm, "data", key); got != want {
			t.Errorf("data.%s = %v, want %v", key, got, want)
		}
	}
	if got := get(t, cm, "binaryData", "DB_PASSWORD"); got != Placeholder {
		t.Errorf("binaryData.DB_PASSWORD = %v", got)
	}
	if got := get(t, cm, "binaryData", "LOGO"); got != "aGVsbG8=" {
		t.Errorf("binaryData.LOGO = %v", got)
	}

	containers := get(t, dep, "spec", "template", "spec", "containers").([]interface{})
	env := containers[0].(map[string]interface{})["env"].([]interface{})
	byName := map[string]map[string]interface{}{}
	for _, e := range env {
		m := e.(map[string]interface{})
		byName[m["name"].(string)] = m
	}
	if byName["KC_DB_PASSWORD"]["value"] != Placeholder {
		t.Errorf("KC_DB_PASSWORD = %v", byName["KC_DB_PASSWORD"]["value"])
	}
	if _, ok := byName["KC_DB_PASSWORD_FROM_SECRET"]["valueFrom"].(map[string]interface{})["secretKeyRef"]; !ok {
		t.Error("valueFrom.secretKeyRef must stay")
	}
	if _, has := byName["KC_DB_PASSWORD_FROM_SECRET"]["value"]; has {
		t.Error("an env entry with valueFrom must not get a value")
	}
	if byName["KC_DB_URL"]["value"] != "jdbc:postgresql://db/kc" || byName["ACCESS_TOKEN_VALIDITY_SECONDS"]["value"] != "300" {
		t.Errorf("non-secret env changed: %v %v", byName["KC_DB_URL"], byName["ACCESS_TOKEN_VALIDITY_SECONDS"])
	}
	wantOpts := "-Xmx512m -Djavax.net.ssl.trustStorePassword=<SET-ME> -Djavax.net.ssl.trustStore=/etc/ssl/ts.jks -Dapp.keyStorePassword:<SET-ME> -Dapp.name=shop"
	if got := byName["JAVA_OPTS_APPEND"]["value"]; got != wantOpts {
		t.Errorf("JAVA_OPTS_APPEND = %v\nwant %v", got, wantOpts)
	}

	init := get(t, dep, "spec", "template", "spec", "initContainers").([]interface{})[0].(map[string]interface{})
	if init["env"].([]interface{})[0].(map[string]interface{})["value"] != Placeholder {
		t.Errorf("init container env not redacted: %v", init["env"])
	}

	// References are intact.
	c := containers[0].(map[string]interface{})
	envFrom := c["envFrom"].([]interface{})
	if envFrom[0].(map[string]interface{})["secretRef"].(map[string]interface{})["name"] != "db-credentials" ||
		envFrom[1].(map[string]interface{})["configMapRef"].(map[string]interface{})["name"] != "app-config" {
		t.Errorf("envFrom changed: %v", envFrom)
	}
	vol := get(t, dep, "spec", "template", "spec", "volumes").([]interface{})[0].(map[string]interface{})
	if vol["secret"].(map[string]interface{})["secretName"] != "db-credentials" {
		t.Errorf("volume secretName changed: %v", vol)
	}
	if byName["KC_DB_PASSWORD_FROM_SECRET"]["valueFrom"].(map[string]interface{})["secretKeyRef"].(map[string]interface{})["name"] != "db-credentials" {
		t.Error("secretKeyRef.name changed")
	}

	want := []string{
		"ConfigMap/app-config binaryData.DB_PASSWORD",
		"ConfigMap/app-config data.APP_TOKENSERVICE_CLIENTSECRET",
		"Deployment/web spec.template.spec.containers[app].env[JAVA_OPTS_APPEND]",
		"Deployment/web spec.template.spec.containers[app].env[KC_DB_PASSWORD]",
		"Deployment/web spec.template.spec.initContainers[init].env[INIT_API_KEY]",
	}
	if fmt.Sprint(items) != fmt.Sprint(want) {
		t.Errorf("items =\n%s\nwant\n%s", strings.Join(items, "\n"), strings.Join(want, "\n"))
	}
}

func TestSecretsAreNotTouched(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Secret",
		"metadata": map[string]interface{}{"name": "s"},
		"data":     map[string]interface{}{"password": "c2VjcmV0", "DB_PASSWORD": "c2VjcmV0"},
	}}
	items := Credentials([]*types.ExtractedResource{{Object: u, GVK: u.GroupVersionKind()}})
	if len(items) != 0 || u.Object["data"].(map[string]interface{})["DB_PASSWORD"] != "c2VjcmV0" {
		t.Errorf("Secret must keep its own strategy: %v %v", items, u.Object["data"])
	}
}

func TestPodTemplateOwners(t *testing.T) {
	for _, tc := range []struct{ kind, path string }{
		{"CronJob", "spec.jobTemplate.spec.template.spec"},
		{"Pod", "spec"},
		{"StatefulSet", "spec.template.spec"},
	} {
		spec := map[string]interface{}{"containers": []interface{}{map[string]interface{}{
			"name": "c", "env": []interface{}{map[string]interface{}{"name": "DB_PASSWORD", "value": "x"}},
		}}}
		obj := map[string]interface{}{"apiVersion": "v1", "kind": tc.kind, "metadata": map[string]interface{}{"name": "o"}}
		cur := obj
		parts := strings.Split(tc.path, ".")
		for _, p := range parts[:len(parts)-1] {
			next := map[string]interface{}{}
			cur[p] = next
			cur = next
		}
		cur[parts[len(parts)-1]] = spec
		u := &unstructured.Unstructured{Object: obj}
		items := Credentials([]*types.ExtractedResource{{Object: u, GVK: u.GroupVersionKind()}})
		want := fmt.Sprintf("%s/o %s.containers[c].env[DB_PASSWORD]", tc.kind, tc.path)
		if len(items) != 1 || items[0] != want {
			t.Errorf("%s: items = %v, want %s", tc.kind, items, want)
		}
	}
}

func TestIdempotentAndNote(t *testing.T) {
	res := load(t)
	first := Credentials(res)
	if again := Credentials(res); len(again) != 0 {
		t.Errorf("a second pass redacts nothing more, got %v", again)
	}
	if len(first) == 0 {
		t.Fatal("nothing redacted")
	}

	short := Note(first, false)
	if !strings.Contains(short, "5 credential") || !strings.Contains(short, "--set") || !strings.Contains(short, "Secret") {
		t.Errorf("note = %q", short)
	}

	var many []string
	for i := 0; i < 25; i++ {
		many = append(many, fmt.Sprintf("ConfigMap/c%02d data.A_PASSWORD", i))
	}
	n := Note(many, false)
	if !strings.Contains(n, "ConfigMap/c19 data.A_PASSWORD") || strings.Contains(n, "ConfigMap/c20 ") || !strings.Contains(n, "+5 more") {
		t.Errorf("note = %q", n)
	}
	all := Note(many, true)
	if !strings.Contains(all, "ConfigMap/c24 data.A_PASSWORD") || strings.Contains(all, "more") {
		t.Errorf("verbose note = %q", all)
	}
	if Note(nil, false) != "" {
		t.Error("no note without redactions")
	}
}
