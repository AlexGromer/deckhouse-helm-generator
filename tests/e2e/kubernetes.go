package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// KubernetesClient provides Kubernetes cluster interaction for E2E tests.
// It supports both real clusters and dry-run validation without a cluster.
type KubernetesClient struct {
	// KubectlPath is the path to the kubectl binary.
	KubectlPath string

	// KubeConfig is the path to the kubeconfig file.
	KubeConfig string

	// DryRunMode specifies whether to use "client" or "server" dry-run.
	DryRunMode string
}

// NewKubernetesClient creates a KubernetesClient, optionally connecting to a cluster.
// If no kubeconfig is provided, dry-run=client is used (no cluster needed).
func NewKubernetesClient(kubeconfig string) (*KubernetesClient, error) {
	kubectlPath, err := exec.LookPath("kubectl")
	if err != nil {
		return nil, fmt.Errorf("kubectl not found: %w", err)
	}

	dryRunMode := "client"
	if kubeconfig != "" {
		// Verify connection
		cmd := exec.Command(kubectlPath, "--kubeconfig", kubeconfig, "cluster-info")
		if err := cmd.Run(); err == nil {
			dryRunMode = "server"
		}
	}

	return &KubernetesClient{
		KubectlPath: kubectlPath,
		KubeConfig:  kubeconfig,
		DryRunMode:  dryRunMode,
	}, nil
}

// HasCluster returns true if a real Kubernetes cluster is available.
func (k *KubernetesClient) HasCluster() bool {
	return k.DryRunMode == "server" && k.KubeConfig != ""
}

// ValidateManifest applies a YAML manifest with dry-run to validate it.
func (k *KubernetesClient) ValidateManifest(manifest string) *KubeResult {
	args := []string{"apply", "--dry-run=" + k.DryRunMode, "-f", "-"}

	if k.KubeConfig != "" {
		args = append([]string{"--kubeconfig", k.KubeConfig}, args...)
	}

	cmd := exec.Command(k.KubectlPath, args...)
	cmd.Stdin = strings.NewReader(manifest)

	output, err := cmd.CombinedOutput()
	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		exitCode = -1
	}

	return &KubeResult{
		Output:   string(output),
		ExitCode: exitCode,
		Err:      err,
	}
}

// ValidateFile applies a YAML file with dry-run to validate it.
func (k *KubernetesClient) ValidateFile(path string) *KubeResult {
	args := []string{"apply", "--dry-run=" + k.DryRunMode, "-f", path}

	if k.KubeConfig != "" {
		args = append([]string{"--kubeconfig", k.KubeConfig}, args...)
	}

	cmd := exec.Command(k.KubectlPath, args...)
	output, err := cmd.CombinedOutput()
	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		exitCode = -1
	}

	return &KubeResult{
		Output:   string(output),
		ExitCode: exitCode,
		Err:      err,
	}
}

// KubeResult contains the result of a kubectl command execution.
type KubeResult struct {
	// Output is the combined stdout+stderr.
	Output string

	// ExitCode is the process exit code.
	ExitCode int

	// Err is the Go error.
	Err error
}

// Success returns true if the command succeeded.
func (r *KubeResult) Success() bool {
	return r.ExitCode == 0 && r.Err == nil
}

// HasKubectl checks if kubectl is available.
func HasKubectl() bool {
	_, err := exec.LookPath("kubectl")
	return err == nil
}

// RequireKubectl skips the test if kubectl is not available.
func RequireKubectl(t *testing.T) *KubernetesClient {
	t.Helper()
	if !HasKubectl() {
		t.Skip("kubectl not found, skipping K8s validation test")
	}
	client, err := NewKubernetesClient("")
	if err != nil {
		t.Skipf("Cannot create K8s client: %v", err)
	}
	return client
}

// MockKubeConfig creates a temporary kubeconfig pointing to a non-existent cluster.
// This allows helm install --dry-run=client to work without a real cluster.
func MockKubeConfig(t *testing.T, dir string) string {
	t.Helper()
	kubeconfig := `apiVersion: v1
kind: Config
clusters:
  - cluster:
      server: https://127.0.0.1:6443
      insecure-skip-tls-verify: true
    name: mock-cluster
contexts:
  - context:
      cluster: mock-cluster
      user: mock-user
    name: mock-context
current-context: mock-context
users:
  - name: mock-user
    user:
      token: mock-token
`
	path := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(path, []byte(kubeconfig), 0600); err != nil {
		t.Fatalf("Failed to write mock kubeconfig: %v", err)
	}
	return path
}
