package golden

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// redactInput keeps credentials in a ConfigMap and in container env next to
// look-alike settings and references that must survive.
const redactInput = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
data:
  APP_TOKENSERVICE_CLIENTSECRET: cm-client-secret-value
  APP_TOKENSERVICE_CLIENTID: shop-client
  ACCESS_TOKEN_VALIDITY_SECONDS: "300"
  TOKEN_URL: https://auth.example.com/token
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  labels:
    app.kubernetes.io/name: web
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
        - name: web
          image: web:1.0
          envFrom:
            - configMapRef:
                name: app-config
          env:
            - name: KC_DB_PASSWORD
              value: env-db-password-value
            - name: KEYCLOAK_ADMIN_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: admin-credentials
                  key: password
            - name: JAVA_OPTS_APPEND
              value: "-Xmx256m -Djavax.net.ssl.trustStorePassword=jvm-store-password-value -Dapp.name=web"
---
apiVersion: v1
kind: Service
metadata:
  name: web
  labels:
    app.kubernetes.io/name: web
spec:
  selector:
    app: web
  ports:
    - port: 80
`

var redactedSecrets = []string{"cm-client-secret-value", "env-db-password-value", "jvm-store-password-value"}

// dirText is the text of every file under dir.
func dirText(t *testing.T, dir string) string {
	t.Helper()
	var sb strings.Builder
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sb.WriteString(string(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

func TestRedactCredentials(t *testing.T) {
	helm := requireHelm(t)
	for _, tc := range []struct {
		name   string
		args   []string
		config string
		keeps  bool
	}{
		{name: "default"},
		{name: "flag-false", args: []string{"--redact-credentials=false"}, keeps: true},
		{name: "config-file-false", config: "redact-credentials: false\n", keeps: true},
		{name: "config-file-true-flag-wins", config: "redact-credentials: false\n", args: []string{"--redact-credentials=true"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			in := filepath.Join(dir, "in")
			if err := os.MkdirAll(in, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(in, "app.yaml"), []byte(redactInput), 0o644); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, "out")
			args := []string{"generate", "-f", in, "-o", out, "--chart-name", "app"}
			if tc.config != "" {
				cfg := filepath.Join(dir, "dhg.yaml")
				if err := os.WriteFile(cfg, []byte(tc.config), 0o644); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--config", cfg)
			}
			args = append(args, tc.args...)
			log := runOK(t, dhgBin, args...)

			text := dirText(t, out)
			for _, secret := range redactedSecrets {
				if has := strings.Contains(text, secret); has != tc.keeps {
					t.Errorf("%q in the chart = %v, want %v", secret, has, tc.keeps)
				}
			}
			if hasNote := strings.Contains(log, "credential values replaced"); hasNote == tc.keeps {
				t.Errorf("redaction note present = %v, want %v:\n%s", hasNote, !tc.keeps, log)
			}
			if !tc.keeps {
				for _, want := range []string{
					"<SET-ME>", "-Djavax.net.ssl.trustStorePassword=<SET-ME>", "-Xmx256m", "-Dapp.name=web",
					"shop-client", "ACCESS_TOKEN_VALIDITY_SECONDS", "https://auth.example.com/token",
					"admin-credentials", "secretKeyRef",
				} {
					if !strings.Contains(text, want) {
						t.Errorf("chart lacks %q", want)
					}
				}
				for _, want := range []string{"ConfigMap/app-config data.APP_TOKENSERVICE_CLIENTSECRET", "Deployment/web spec.template.spec.containers[web].env[KC_DB_PASSWORD]"} {
					if !strings.Contains(log, want) {
						t.Errorf("note lacks %q:\n%s", want, log)
					}
				}
			}

			// The chart is still accepted by Helm.
			if _, complete := checkCharts(t, helm, out); !complete {
				t.Error("chart was not rendered")
			}
			rendered, err := run(helm, "template", "golden", filepath.Join(out, "app"))
			if err != nil {
				t.Fatalf("helm template: %v\n%s", err, rendered)
			}
			if !tc.keeps && !strings.Contains(rendered, "<SET-ME>") {
				t.Errorf("rendered manifests lack the placeholder:\n%s", rendered)
			}
		})
	}
}
