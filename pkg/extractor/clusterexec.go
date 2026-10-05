package extractor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// execConfig is the exec section of a kubeconfig user: a credential plugin
// implementing the client.authentication.k8s.io ExecCredential protocol.
type execConfig struct {
	APIVersion         string       `json:"apiVersion"`
	Command            string       `json:"command"`
	Args               []string     `json:"args,omitempty"`
	Env                []execEnvVar `json:"env,omitempty"`
	InstallHint        string       `json:"installHint,omitempty"`
	ProvideClusterInfo bool         `json:"provideClusterInfo,omitempty"`
	InteractiveMode    string       `json:"interactiveMode,omitempty"`
}

type execEnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// execCredential is both the request passed to the plugin in
// KUBERNETES_EXEC_INFO and the response it prints on stdout.
type execCredential struct {
	APIVersion string                `json:"apiVersion"`
	Kind       string                `json:"kind"`
	Spec       execCredentialSpec    `json:"spec"`
	Status     *execCredentialStatus `json:"status,omitempty"`
}

type execCredentialSpec struct {
	Interactive bool                `json:"interactive"`
	Cluster     *execCredentialInfo `json:"cluster,omitempty"`
}

type execCredentialInfo struct {
	Server                   string `json:"server"`
	CertificateAuthorityData string `json:"certificate-authority-data,omitempty"`
	InsecureSkipTLSVerify    bool   `json:"insecure-skip-tls-verify,omitempty"`
}

type execCredentialStatus struct {
	ExpirationTimestamp   *time.Time `json:"expirationTimestamp,omitempty"`
	Token                 string     `json:"token,omitempty"`
	ClientCertificateData string     `json:"clientCertificateData,omitempty"`
	ClientKeyData         string     `json:"clientKeyData,omitempty"`
}

var supportedExecAPIVersions = map[string]bool{
	"client.authentication.k8s.io/v1":      true,
	"client.authentication.k8s.io/v1beta1": true,
}

// runExecPlugin runs a credential plugin as kubectl does and returns the
// credentials it prints. A command containing a path separator is resolved
// relative to the kubeconfig's directory; a bare name is looked up in PATH.
// The plugin may prompt (e.g. open a browser for an OIDC login) when the
// interactive mode allows it and stdin is a terminal.
func runExecPlugin(cfg *execConfig, kubeconfigDir string, cluster kubeconfigClusterDetail) (*execCredentialStatus, error) {
	if !supportedExecAPIVersions[cfg.APIVersion] {
		return nil, fmt.Errorf("unsupported apiVersion %q (want client.authentication.k8s.io/v1 or v1beta1)", cfg.APIVersion)
	}
	if cfg.Command == "" {
		return nil, fmt.Errorf("exec command is empty")
	}

	command := cfg.Command
	if strings.ContainsRune(command, filepath.Separator) && !filepath.IsAbs(command) {
		command = filepath.Join(kubeconfigDir, command)
	}
	path, err := exec.LookPath(command)
	if err != nil {
		if cfg.InstallHint != "" {
			return nil, fmt.Errorf("%s not found: %s", cfg.Command, cfg.InstallHint)
		}
		return nil, fmt.Errorf("%s not found: %w", cfg.Command, err)
	}

	interactive := cfg.InteractiveMode != "Never" && stdinIsTerminal()
	if cfg.InteractiveMode == "Always" && !interactive {
		return nil, fmt.Errorf("plugin requires an interactive terminal (interactiveMode: Always)")
	}
	request := execCredential{
		APIVersion: cfg.APIVersion,
		Kind:       "ExecCredential",
		Spec:       execCredentialSpec{Interactive: interactive},
	}
	if cfg.ProvideClusterInfo {
		request.Spec.Cluster = &execCredentialInfo{
			Server:                   cluster.Server,
			CertificateAuthorityData: cluster.CertificateAuthorityData,
			InsecureSkipTLSVerify:    cluster.InsecureSkipTLSVerify,
		}
	}
	info, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(path, cfg.Args...) //nolint:gosec // the command comes from the user's kubeconfig
	cmd.Env = append(os.Environ(), "KUBERNETES_EXEC_INFO="+string(info))
	for _, e := range cfg.Env {
		cmd.Env = append(cmd.Env, e.Name+"="+e.Value)
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if interactive {
		cmd.Stdin = os.Stdin
	}
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s failed: %w", cfg.Command, err)
	}

	var response execCredential
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return nil, fmt.Errorf("cannot parse ExecCredential printed by %s: %w", cfg.Command, err)
	}
	if response.Kind != "ExecCredential" || response.APIVersion != cfg.APIVersion {
		return nil, fmt.Errorf("%s printed %s %s, want ExecCredential %s", cfg.Command, response.APIVersion, response.Kind, cfg.APIVersion)
	}
	st := response.Status
	if st == nil || (st.Token == "" && st.ClientCertificateData == "") {
		return nil, fmt.Errorf("%s returned no token or client certificate", cfg.Command)
	}
	if (st.ClientCertificateData == "") != (st.ClientKeyData == "") {
		return nil, fmt.Errorf("%s returned a client certificate without its key", cfg.Command)
	}
	return st, nil
}

// stdinIsTerminal reports whether stdin can answer a prompt: a character
// device other than the null device (CI runners and `< /dev/null` give
// the latter, which is a character device too).
func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	if null, err := os.Stat(os.DevNull); err == nil && os.SameFile(fi, null) {
		return false
	}
	return true
}
