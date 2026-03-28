package generator

import "strings"

// GenerateSupplyChainCI generates CI pipeline files for supply chain security.
// Supported platforms: "github", "gitlab".
// Returns a map of filepath → file content.
func GenerateSupplyChainCI(platform string) map[string]string {
	switch platform {
	case "github":
		return generateGitHubSupplyChain()
	case "gitlab":
		return generateGitLabSupplyChain()
	default:
		return map[string]string{}
	}
}

func generateGitHubSupplyChain() map[string]string {
	var b strings.Builder

	b.WriteString("name: Supply Chain Security\n")
	b.WriteString("on:\n")
	b.WriteString("  push:\n")
	b.WriteString("    branches: [main]\n")
	b.WriteString("  pull_request:\n")
	b.WriteString("    branches: [main]\n")
	b.WriteString("\n")
	b.WriteString("permissions:\n")
	b.WriteString("  contents: read\n")
	b.WriteString("  packages: write\n")
	b.WriteString("  id-token: write\n")
	b.WriteString("\n")
	b.WriteString("jobs:\n")
	b.WriteString("  sbom-and-sign:\n")
	b.WriteString("    runs-on: ubuntu-latest\n")
	b.WriteString("    steps:\n")
	b.WriteString("      - uses: actions/checkout@v4\n")
	b.WriteString("\n")
	b.WriteString("      - name: Build container image\n")
	b.WriteString("        run: docker build -t ${{ github.repository }}:${{ github.sha }} .\n")
	b.WriteString("\n")
	b.WriteString("      - name: Generate SBOM with Syft\n")
	b.WriteString("        uses: anchore/sbom-action@v0\n")
	b.WriteString("        with:\n")
	b.WriteString("          image: ${{ github.repository }}:${{ github.sha }}\n")
	b.WriteString("          format: spdx-json\n")
	b.WriteString("          output-file: sbom.spdx.json\n")
	b.WriteString("\n")
	b.WriteString("      - name: Install Cosign\n")
	b.WriteString("        uses: sigstore/cosign-installer@v3\n")
	b.WriteString("\n")
	b.WriteString("      - name: Sign container image with Cosign\n")
	b.WriteString("        run: cosign sign --yes ${{ github.repository }}:${{ github.sha }}\n")
	b.WriteString("\n")
	b.WriteString("      - name: Attach SBOM to image\n")
	b.WriteString("        run: cosign attach sbom --sbom sbom.spdx.json ${{ github.repository }}:${{ github.sha }}\n")
	b.WriteString("\n")
	b.WriteString("      - name: Upload SBOM artifact\n")
	b.WriteString("        uses: actions/upload-artifact@v4\n")
	b.WriteString("        with:\n")
	b.WriteString("          name: sbom\n")
	b.WriteString("          path: sbom.spdx.json\n")

	return map[string]string{
		".github/workflows/supply-chain.yml": b.String(),
	}
}

func generateGitLabSupplyChain() map[string]string {
	var b strings.Builder

	b.WriteString("# Supply Chain Security - GitLab CI snippet\n")
	b.WriteString("# Add this to your .gitlab-ci.yml\n")
	b.WriteString("\n")
	b.WriteString("stages:\n")
	b.WriteString("  - build\n")
	b.WriteString("  - sbom\n")
	b.WriteString("  - sign\n")
	b.WriteString("\n")
	b.WriteString("build-image:\n")
	b.WriteString("  stage: build\n")
	b.WriteString("  image: docker:latest\n")
	b.WriteString("  services:\n")
	b.WriteString("    - docker:dind\n")
	b.WriteString("  script:\n")
	b.WriteString("    - docker build -t $CI_REGISTRY_IMAGE:$CI_COMMIT_SHA .\n")
	b.WriteString("    - docker push $CI_REGISTRY_IMAGE:$CI_COMMIT_SHA\n")
	b.WriteString("\n")
	b.WriteString("generate-sbom:\n")
	b.WriteString("  stage: sbom\n")
	b.WriteString("  image: anchore/syft:latest\n")
	b.WriteString("  script:\n")
	b.WriteString("    - syft $CI_REGISTRY_IMAGE:$CI_COMMIT_SHA -o spdx-json=sbom.spdx.json\n")
	b.WriteString("  artifacts:\n")
	b.WriteString("    paths:\n")
	b.WriteString("      - sbom.spdx.json\n")
	b.WriteString("\n")
	b.WriteString("sign-image:\n")
	b.WriteString("  stage: sign\n")
	b.WriteString("  image: bitnami/cosign:latest\n")
	b.WriteString("  script:\n")
	b.WriteString("    - cosign sign --yes $CI_REGISTRY_IMAGE:$CI_COMMIT_SHA\n")
	b.WriteString("    - cosign attach sbom --sbom sbom.spdx.json $CI_REGISTRY_IMAGE:$CI_COMMIT_SHA\n")

	return map[string]string{
		".gitlab-ci.yml": b.String(),
	}
}
