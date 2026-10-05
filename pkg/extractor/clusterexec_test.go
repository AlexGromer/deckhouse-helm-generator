package extractor

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeExecKubeconfig writes a kubeconfig whose user runs plugin (a shell
// script body) as a credential plugin, next to the kubeconfig.
func writeExecKubeconfig(t *testing.T, server, plugin, user string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.sh"), []byte("#!/bin/sh\n"+plugin), 0o755); err != nil {
		t.Fatal(err)
	}
	kubeconfig := `apiVersion: v1
kind: Config
current-context: ctx
clusters:
  - name: c
    cluster:
      server: ` + server + `
contexts:
  - name: ctx
    context: {cluster: c, user: u}
users:
  - name: u
    user:
` + user
	path := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(path, []byte(kubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const execUser = `      exec:
        apiVersion: client.authentication.k8s.io/v1
        command: ./plugin.sh
        args: ["get-token"]
        env:
          - {name: PLUGIN_GREETING, value: hello}
        provideClusterInfo: true
        interactiveMode: Never
`

func TestClusterClient_ExecPluginToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"kind":"APIVersions"}`))
	}))
	defer srv.Close()

	infoFile := filepath.Join(t.TempDir(), "info.json")
	plugin := `echo "$KUBERNETES_EXEC_INFO" > ` + infoFile + `
[ "$1" = get-token ] && [ "$PLUGIN_GREETING" = hello ] || exit 3
echo '{"apiVersion":"client.authentication.k8s.io/v1","kind":"ExecCredential","status":{"token":"oidc-id-token","expirationTimestamp":"2030-01-01T00:00:00Z"}}'
`
	cc, err := newClusterClient(writeExecKubeconfig(t, srv.URL, plugin, execUser), "")
	if err != nil {
		t.Fatalf("newClusterClient: %v", err)
	}
	if _, err := cc.doGet(context.Background(), "/api"); err != nil {
		t.Fatalf("request: %v", err)
	}
	if gotAuth != "Bearer oidc-id-token" {
		t.Errorf("Authorization = %q, want the plugin's token", gotAuth)
	}

	data, err := os.ReadFile(infoFile)
	if err != nil {
		t.Fatalf("plugin did not run: %v", err)
	}
	var info execCredential
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("KUBERNETES_EXEC_INFO is not JSON: %v (%s)", err, data)
	}
	if info.Kind != "ExecCredential" || info.APIVersion != "client.authentication.k8s.io/v1" ||
		info.Spec.Interactive || info.Spec.Cluster == nil || info.Spec.Cluster.Server != srv.URL {
		t.Errorf("unexpected KUBERNETES_EXEC_INFO: %s", data)
	}
}

func TestClusterClient_ExecPluginErrors(t *testing.T) {
	tests := []struct {
		name, plugin, user, want string
	}{
		{"plugin fails", "exit 1", execUser, "plugin.sh failed"},
		{"not JSON", "echo nope", execUser, "cannot parse ExecCredential"},
		{"wrong apiVersion", `echo '{"apiVersion":"client.authentication.k8s.io/v1beta1","kind":"ExecCredential","status":{"token":"t"}}'`, execUser, "want ExecCredential client.authentication.k8s.io/v1"},
		{"no credentials", `echo '{"apiVersion":"client.authentication.k8s.io/v1","kind":"ExecCredential","status":{}}'`, execUser, "no token or client certificate"},
		{"missing command", "", "      exec:\n        apiVersion: client.authentication.k8s.io/v1\n        command: kubectl-no-such-plugin\n        installHint: install kubelogin\n", "install kubelogin"},
		{"unsupported version", "", "      exec:\n        apiVersion: client.authentication.k8s.io/v1alpha1\n        command: ./plugin.sh\n", "unsupported apiVersion"},
		{"auth-provider", "", "      auth-provider:\n        name: oidc\n", "auth-provider"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newClusterClient(writeExecKubeconfig(t, "https://127.0.0.1:1", tt.plugin, tt.user), "")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestClusterClient_TokenFile(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cc, err := newClusterClient(writeExecKubeconfig(t, srv.URL, "", "      tokenFile: "+tokenFile+"\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cc.doGet(context.Background(), "/api"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer file-token" {
		t.Errorf("Authorization = %q", gotAuth)
	}
}

// selfSignedPEM returns a PEM certificate and key for a test client.
func selfSignedPEM(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "dhg"}, NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
}

func TestClusterClient_ExecPluginClientCertificate(t *testing.T) {
	certPEM, keyPEM := selfSignedPEM(t)
	status, _ := json.Marshal(map[string]interface{}{
		"apiVersion": "client.authentication.k8s.io/v1beta1",
		"kind":       "ExecCredential",
		"status":     map[string]string{"clientCertificateData": certPEM, "clientKeyData": keyPEM},
	})
	user := "      exec:\n        apiVersion: client.authentication.k8s.io/v1beta1\n        command: ./plugin.sh\n        interactiveMode: IfAvailable\n"
	cc, err := newClusterClient(writeExecKubeconfig(t, "https://127.0.0.1:1", "cat <<'JSON'\n"+string(status)+"\nJSON\n", user), "")
	if err != nil {
		t.Fatalf("newClusterClient: %v", err)
	}
	transport, _ := cc.httpClient.Transport.(*http.Transport)
	if transport == nil || len(transport.TLSClientConfig.Certificates) != 1 {
		t.Fatal("client certificate from the plugin is not used")
	}
	if cc.headers.Get("Authorization") != "" {
		t.Error("no bearer token expected with certificate credentials")
	}
}

func TestClusterClient_ExecPluginMoreErrors(t *testing.T) {
	certPEM, _ := selfSignedPEM(t)
	certOnly, _ := json.Marshal(map[string]interface{}{
		"apiVersion": "client.authentication.k8s.io/v1",
		"kind":       "ExecCredential",
		"status":     map[string]string{"clientCertificateData": certPEM},
	})
	tests := []struct {
		name, plugin, user, want string
	}{
		{"certificate without key", "cat <<'JSON'\n" + string(certOnly) + "\nJSON\n", execUser, "without its key"},
		{"empty command", "", "      exec:\n        apiVersion: client.authentication.k8s.io/v1\n        command: \"\"\n", "command is empty"},
		{"missing command without hint", "", "      exec:\n        apiVersion: client.authentication.k8s.io/v1\n        command: kubectl-no-such-plugin\n", "not found"},
		// go test does not run with a terminal on stdin.
		{"interactive required", "", "      exec:\n        apiVersion: client.authentication.k8s.io/v1\n        command: ./plugin.sh\n        interactiveMode: Always\n", "interactive terminal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newClusterClient(writeExecKubeconfig(t, "https://127.0.0.1:1", tt.plugin, tt.user), "")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}
