package generator

import (
	"strings"
	"testing"
)

func TestSupplyChain_GitHubWorkflow(t *testing.T) {
	files := GenerateSupplyChainCI("github")

	content, ok := files[".github/workflows/supply-chain.yml"]
	if !ok {
		t.Fatal("missing .github/workflows/supply-chain.yml")
	}

	checks := map[string]string{
		"workflow name":    "name: Supply Chain Security",
		"syft SBOM action": "anchore/sbom-action",
		"cosign installer": "sigstore/cosign-installer",
		"cosign sign":      "cosign sign",
		"SBOM format":      "spdx-json",
		"cosign attach":    "cosign attach sbom",
		"upload artifact":  "actions/upload-artifact",
	}
	for desc, substr := range checks {
		if !strings.Contains(content, substr) {
			t.Errorf("GitHub workflow missing %s (expected substring: %q)", desc, substr)
		}
	}
}

func TestSupplyChain_GitHubPermissions(t *testing.T) {
	files := GenerateSupplyChainCI("github")
	content := files[".github/workflows/supply-chain.yml"]

	// id-token: write is required for keyless cosign signing
	if !strings.Contains(content, "id-token: write") {
		t.Error("GitHub workflow must request id-token: write for cosign keyless signing")
	}
}

func TestSupplyChain_GitLabCI(t *testing.T) {
	files := GenerateSupplyChainCI("gitlab")

	content, ok := files[".gitlab-ci.yml"]
	if !ok {
		t.Fatal("missing .gitlab-ci.yml")
	}

	checks := map[string]string{
		"syft":        "syft",
		"cosign sign": "cosign sign",
		"SBOM stage":  "stage: sbom",
		"sign stage":  "stage: sign",
		"spdx-json":   "spdx-json",
		"cosign attach": "cosign attach sbom",
	}
	for desc, substr := range checks {
		if !strings.Contains(content, substr) {
			t.Errorf("GitLab CI missing %s (expected substring: %q)", desc, substr)
		}
	}
}

func TestSupplyChain_UnknownPlatform(t *testing.T) {
	files := GenerateSupplyChainCI("unknown")
	if len(files) != 0 {
		t.Error("unknown platform should return empty map")
	}
}
