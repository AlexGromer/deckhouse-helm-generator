# User Guide — DHG (Deckhouse Helm Generator)

> **Type:** Tutorial + Reference
> **Audience:** developers and platform engineers using DHG to generate Helm charts
> **Last updated:** 2026-03-30
> **Related:** [DEVELOPER.md](DEVELOPER.md), [ADR.md](ADR.md)

## Overview

DHG is a CLI tool that generates production-ready Helm charts from Kubernetes YAML manifests. It extracts your resources, detects relationships between them, groups them into services, and writes a complete chart — including `values.yaml`, templates, `_helpers.tpl`, and optional extras like environment overlays, security policies, and Deckhouse module scaffolding.

---

## 1. Installation

### Homebrew (macOS and Linux)

```bash
brew install AlexGromer/tap/dhg
```

### Binary release (Linux)

```bash
VERSION=$(curl -s https://api.github.com/repos/AlexGromer/deckhouse-helm-generator/releases/latest \
  | grep tag_name | cut -d '"' -f4)

# Linux AMD64
curl -LO "https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/${VERSION}/dhg_${VERSION#v}_linux_amd64.tar.gz"
tar xzf "dhg_${VERSION#v}_linux_amd64.tar.gz"
sudo mv dhg /usr/local/bin/

# Linux ARM64
curl -LO "https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/${VERSION}/dhg_${VERSION#v}_linux_arm64.tar.gz"
tar xzf "dhg_${VERSION#v}_linux_arm64.tar.gz"
sudo mv dhg /usr/local/bin/
```

### Binary release (macOS)

```bash
VERSION=$(curl -s https://api.github.com/repos/AlexGromer/deckhouse-helm-generator/releases/latest \
  | grep tag_name | cut -d '"' -f4)

# macOS ARM64 (Apple Silicon)
curl -LO "https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/${VERSION}/dhg_${VERSION#v}_darwin_arm64.tar.gz"
tar xzf "dhg_${VERSION#v}_darwin_arm64.tar.gz"
sudo mv dhg /usr/local/bin/

# macOS AMD64 (Intel)
curl -LO "https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/${VERSION}/dhg_${VERSION#v}_darwin_amd64.tar.gz"
tar xzf "dhg_${VERSION#v}_darwin_amd64.tar.gz"
sudo mv dhg /usr/local/bin/
```

### go install

```bash
go install github.com/AlexGromer/deckhouse-helm-generator/cmd/dhg@latest
```

### Docker

```bash
docker pull ghcr.io/alexgromer/dhg:latest

# Run against files in the current directory
docker run --rm -v $(pwd):/work \
  ghcr.io/alexgromer/dhg:latest \
  generate -f /work/manifests -o /work/chart --chart-name myapp
```

### Build from source

```bash
git clone https://github.com/AlexGromer/deckhouse-helm-generator.git
cd deckhouse-helm-generator
make build
sudo cp bin/dhg /usr/local/bin/
```

### Verify

```bash
dhg version
# dhg version v0.7.3 (built: ...)
```

---

## 2. Quick Start

**Time to complete:** ~5 minutes

**Prerequisites:** `dhg` installed, a directory of Kubernetes YAML manifests

### Step 1: Prepare your manifests

Place your Kubernetes YAML files in a directory:

```
manifests/
  deployment.yaml
  service.yaml
  ingress.yaml
  configmap.yaml
```

### Step 2: Generate the chart

```bash
dhg generate -f ./manifests -o ./my-chart --chart-name myapp
```

Expected output:

```
[1/5] Extracting resources from source...
[2/5] Processing resources...
[3/5] Analyzing relationships...
[4/5] Generating Helm chart...
[5/5] Writing charts to disk...

Successfully generated 1 chart(s) in ./my-chart

To install the chart, run:
  helm install my-release ./my-chart/myapp
```

### Step 3: Inspect the result

```
my-chart/
└── myapp/
    ├── Chart.yaml
    ├── values.yaml
    ├── README.md
    └── templates/
        ├── _helpers.tpl
        ├── myapp-deployment.yaml
        ├── myapp-service.yaml
        ├── myapp-ingress.yaml
        └── myapp-configmap.yaml
```

### Step 4: Install with Helm

```bash
helm install my-release ./my-chart/myapp
# or with custom values
helm install my-release ./my-chart/myapp --set myapp.deployment.replicas=3
```

---

## 3. CLI Reference

### Global commands

| Command | Description |
|---------|-------------|
| `dhg generate` | Generate a Helm chart from Kubernetes resources |
| `dhg analyze` | Analyze resources and print architecture recommendations |
| `dhg validate` | Validate Helm chart structure and template syntax |
| `dhg diff` | Show differences between two chart directories |
| `dhg fix` | Auto-fix manifests with security best practices |
| `dhg migrate` | Detect drift and generate migration plan |
| `dhg version` | Print version information |

---

### `dhg generate`

Generate a Helm chart from Kubernetes resource files.

```
dhg generate [flags]
```

**Required flags:**

| Flag | Description |
|------|-------------|
| `-f, --file strings` | Path(s) to YAML files or directories |
| `--chart-name string` | Name of the chart |

**Core flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `-o, --output string` | `./chart` | Output directory |
| `--chart-version string` | `0.1.0` | Helm chart version |
| `--app-version string` | `1.0.0` | Application version |
| `--mode string` | `universal` | Output mode: `universal`, `separate`, `library`, `umbrella` |
| `-r, --recursive` | `true` | Recursively scan directories |
| `-v, --verbose` | `false` | Verbose output |
| `--dry-run` | `false` | Print chart to stdout, do not write to disk |

**Filtering flags:**

| Flag | Description |
|------|-------------|
| `-n, --namespace string` | Filter resources by namespace |
| `--namespaces strings` | Filter by multiple namespaces |
| `-l, --selector string` | Label selector filter (e.g., `app=myapp`) |
| `--include-kinds strings` | Include only these resource kinds |
| `--exclude-kinds strings` | Exclude these resource kinds |

**Output flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--include-tests` | `false` | Generate helm-unittest test templates |
| `--include-readme` | `true` | Generate README.md |
| `--include-schema` | `false` | Generate `values.schema.json` |
| `--template-style string` | `standard` | Template output style: `standard` or `helm` |
| `--values-flat` | `false` | Add dot-notation path comments to values.yaml for `--set` reference |

**Environment and infrastructure flags:**

| Flag | Description |
|------|-------------|
| `--env-values` | Generate `values-dev.yaml`, `values-staging.yaml`, `values-prod.yaml` |
| `--namespace-resources` | Generate ResourceQuota, LimitRange, NetworkPolicy |
| `--feature-flags` | Inject feature flag guards (monitoring, ingress, autoscaling, security, storage, rbac) |
| `--cloud-provider string` | Cloud provider for Service annotations: `aws`, `gcp`, `azure` |
| `--cloud-internal` | Use internal load balancer (default: internet-facing) |
| `--detect-ingress` | Auto-detect ingress controller; inject controller-specific annotations |
| `--airgap-registry string` | Generate air-gap artifacts targeting this registry |
| `--auto-deps` | Auto-detect infrastructure dependencies (PostgreSQL, Redis, etc.) |

**Topology flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--monorepo` | `false` | Generate monorepo layout (Makefile, .helmignore, ct.yaml) |
| `--kustomize` | `false` | Generate Kustomize layout with base and dev/staging/prod overlays |
| `--post-renderer` | `false` | Generate Kustomize overlays compatible with Flux CD `postBuild` |
| `--multi-tenant` | `false` | Generate multi-tenant overlay with per-tenant isolation |
| `--tenant-count int` | `2` | Number of tenant examples to scaffold |
| `--spot` | `false` | Inject spot/preemptible instance tolerations and PDB |
| `--spot-grace-period int` | `15` | Grace period (seconds) for spot instance preStop hook |

**Deckhouse flags:**

| Flag | Description |
|------|-------------|
| `--deckhouse-module` | Generate Deckhouse module scaffold (helm_lib, openapi/, images/, hooks/) |
| `--hooks` | Generate Helm lifecycle hook Job templates (pre-upgrade, post-install, pre-delete) |

> Note: `--monorepo` and `--kustomize` are mutually exclusive.

---

### `dhg analyze`

Analyze resources for architecture patterns, best practices, and service grouping recommendations.

```
dhg analyze -f ./manifests [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-f, --file strings` | required | Path(s) to YAML files or directories |
| `--output-format string` | `text` | Output format: `text`, `json`, `markdown` |
| `-o, --output string` | stdout | Output file |
| `--summary` | `false` | Show only the summary section |
| `--color` | `true` | Enable colored output |
| `-r, --recursive` | `true` | Recursively scan directories |
| `-n, --namespace string` | | Filter by namespace |

**Example:**

```bash
# Print recommendations to stdout
dhg analyze -f ./manifests

# Export as Markdown report
dhg analyze -f ./manifests --output-format markdown -o analysis.md
```

---

### `dhg validate`

Validate an existing Helm chart for structural issues and template syntax errors.

```
dhg validate -f ./chart/myapp [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-f, --file strings` | `.` | Path(s) to chart directories |
| `-v, --verbose` | `false` | Verbose output |

Checks performed:
- `Chart.yaml` presence and required fields (`apiVersion`, `name`, `version`)
- `values.yaml` presence and valid YAML
- Template syntax: balanced `{{ }}` delimiters

**Example:**

```bash
dhg validate -f ./chart/myapp -v
```

Expected output on a valid chart:

```
Validating chart at: ./chart/myapp
  OK: Chart.yaml found (142 bytes)
  OK: values.yaml valid (520 bytes)
  OK: deployment.yaml (12 template expressions)
  Templates: 5 files checked

Validation complete: 0 error(s), 0 warning(s)
```

---

### `dhg diff`

Show differences between two chart directories.

```
dhg diff <dir1> <dir2> [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--color` | `true` | Enable colored output |

**Example:**

```bash
# Compare chart before and after regeneration
dhg diff ./chart-v1 ./chart-v2
```

Output shows:
- Files present in one directory but not the other
- Line-by-line diffs for changed files

---

### `dhg fix`

Auto-fix Kubernetes manifests by injecting security best practices.

```
dhg fix -f ./manifests -o ./fixed [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-f, --file strings` | required | Path(s) to YAML files or directories |
| `-o, --output string` | `./fixed` | Output directory for fixed manifests |
| `--chart-name string` | `fixed-chart` | Name of the output chart |
| `--workload-type string` | `web` | Resource profile: `web`, `worker`, `database`, `batch`, `cache` |
| `-r, --recursive` | `true` | Recursively scan |
| `-v, --verbose` | `false` | Verbose output |

Fixes applied:
- `SecurityContext` (`runAsNonRoot`, `readOnlyRootFilesystem`, `allowPrivilegeEscalation: false`)
- Resource requests and limits (sized by `--workload-type`)
- Liveness, readiness, and startup probes
- PodDisruptionBudgets
- PSS Restricted compliance labels
- Graceful shutdown `preStop` hooks

**Example:**

```bash
dhg fix -f ./manifests -o ./fixed --workload-type web -v
```

---

### `dhg migrate`

Compare an existing chart against manifests and produce a drift report and migration plan.

```
dhg migrate --from ./existing-chart -f ./manifests --chart-name myapp [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--from string` | required | Path to the existing chart directory |
| `-f, --source strings` | required | Path(s) to source manifest files |
| `--chart-name string` | required | Chart name |
| `--chart-version string` | `0.1.0` | Chart version for the generated comparison chart |
| `--mode string` | `universal` | Output mode |
| `-v, --verbose` | `false` | Verbose output |

**Example:**

```bash
dhg migrate --from ./chart/myapp -f ./manifests --chart-name myapp -v
```

Output:
- Drift summary (added templates, removed templates, changed values)
- Step-by-step migration plan
- `_migrate.tpl` values migration template (when values keys have changed)

---

## 4. Output Modes

DHG supports four output modes, selected with `--mode`.

### universal (default)

All resources go into a single Helm chart. Best for simple applications or when you want one `helm install` command.

```bash
dhg generate -f ./manifests -o ./chart --chart-name myapp --mode universal
```

```
chart/
└── myapp/
    ├── Chart.yaml
    ├── values.yaml
    └── templates/
        ├── frontend-deployment.yaml
        ├── backend-deployment.yaml
        └── ...
```

### separate

A separate chart per detected service. Best for microservices that are deployed independently.

```bash
dhg generate -f ./manifests -o ./charts --chart-name myapp --mode separate
```

```
charts/
├── frontend/
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
└── backend/
    ├── Chart.yaml
    ├── values.yaml
    └── templates/
```

### library

A single library chart with shared templates, plus a thin wrapper chart per service. Best for organizations that enforce DRY templates across many services.

```bash
dhg generate -f ./manifests -o ./charts --chart-name myapp --mode library
```

### umbrella

A parent chart that lists each service as a subchart dependency. Best for deploying all services together while keeping their configurations separate.

```bash
dhg generate -f ./manifests -o ./charts --chart-name myapp --mode umbrella
```

```
charts/
└── myapp/             # parent (umbrella) chart
    ├── Chart.yaml     # lists frontend + backend as dependencies
    ├── values.yaml    # global values
    └── charts/
        ├── frontend/
        └── backend/
```

---

## 5. Environment Overlays (`--env-values`)

Generate environment-specific `values-*.yaml` files alongside the main `values.yaml`:

```bash
dhg generate -f ./manifests -o ./chart --chart-name myapp --env-values
```

DHG detects the workload type (web, worker, database, etc.) and generates profiles accordingly:

```
chart/myapp/
├── values.yaml            # base values
├── values-dev.yaml        # reduced replicas, relaxed resource limits
├── values-staging.yaml    # production-like, lower scale
└── values-prod.yaml       # full replicas, tight resource limits, HPA enabled
```

Install with an environment overlay:

```bash
helm install myapp ./chart/myapp -f ./chart/myapp/values-prod.yaml
```

---

## 6. Security Features (`--security-mode` and friends)

### Pod Security Standards

DHG can inject PSS labels onto namespaces and pods during generation:

```bash
# Enforce PSS baseline on all resources
dhg generate -f ./manifests -o ./chart --chart-name myapp
# PSS labels are generated by the pss.go post-processor; enable via --deckhouse-module or in Phase 2.5 defaults
```

Use `dhg fix` to retrofit existing manifests:

```bash
dhg fix -f ./manifests -o ./fixed --workload-type web
```

### Resource limits

The `dhg fix` command injects CPU and memory requests/limits sized to the workload type:

| `--workload-type` | CPU request | Memory request | Use case |
|-------------------|-------------|----------------|----------|
| `web` | 100m | 128Mi | Stateless HTTP services |
| `worker` | 500m | 256Mi | Background job processors |
| `database` | 1000m | 512Mi | Stateful data stores |
| `batch` | 200m | 128Mi | One-shot Job workloads |
| `cache` | 100m | 256Mi | In-memory caches (Redis, Memcached) |

### RBAC scaffolding

```bash
# RBAC templates are generated automatically when ServiceAccount resources are present
# For explicit RBAC generation, the Phase 2.5 rbac.go post-processor runs during generate
dhg generate -f ./manifests -o ./chart --chart-name myapp
```

### Air-gapped environments

```bash
dhg generate -f ./manifests -o ./chart --chart-name myapp \
  --airgap-registry registry.internal.example.com/mirror
```

This generates alongside the chart:
- `images.txt` — list of all container images referenced in templates
- `mirror-images.sh` — script to pull and push images to your registry
- `values-airgap.yaml` — values override pointing all images to the mirror registry

---

## 7. Secret Strategies (`--secret-strategy`)

> Note: `--secret-strategy` is introduced in Phase 5.7. Verify it is available in your installed version with `dhg generate --help`.

DHG supports four mutually exclusive secret management strategies (ADR-042):

| Strategy | Flag value | Provider |
|----------|-----------|---------|
| External Secrets Operator | `eso` | ESO `ExternalSecret` CRs |
| Sealed Secrets | `sealed` | Bitnami `SealedSecret` CRs |
| Vault CSI Provider | `vault-csi` | `SecretProviderClass` CRs |
| SOPS / Helm Secrets | `sops` | Encrypted values files |

```bash
# Generate chart with ESO-managed secrets
dhg generate -f ./manifests -o ./chart --chart-name myapp --secret-strategy eso

# Generate chart with Sealed Secrets
dhg generate -f ./manifests -o ./chart --chart-name myapp --secret-strategy sealed
```

When a secret strategy is specified, DHG replaces plain `Secret` template resources with the corresponding provider CRs, and adds an annotation-based `Reloader` integration to restart pods when secrets rotate.

---

## 8. Deckhouse Module Generation (`--deckhouse-module`)

Generate a Deckhouse-compatible module scaffold instead of a plain Helm chart:

```bash
dhg generate -f ./manifests -o ./module --chart-name my-module --deckhouse-module
```

This adds:

```
module/my-module/
├── Chart.yaml          # with helm_lib dependency
├── values.yaml
├── openapi/
│   └── config-values.yaml   # OpenAPI schema for ModuleConfig validation
├── images/
│   └── .gitkeep             # placeholder for image build contexts
├── hooks/
│   └── .gitkeep             # placeholder for shell hooks
└── templates/
    ├── _helpers.tpl
    └── ...
```

The `openapi/config-values.yaml` schema is generated from the `values.yaml` structure and is compatible with Deckhouse's `ModuleConfig` CRD validation.

---

## 9. Examples

### Basic: single-service application

```bash
dhg generate -f ./manifests/nginx -o ./chart --chart-name nginx-app
helm lint ./chart/nginx-app
helm install nginx ./chart/nginx-app
```

### Multi-service application with separate charts

```bash
dhg generate -f ./manifests -o ./charts --chart-name shop --mode separate -v
# Inspect detected services
ls ./charts/
# frontend  backend  postgres  redis

# Deploy all services
for dir in ./charts/*/; do
  name=$(basename "$dir")
  helm install "$name" "$dir"
done
```

### With security and environment overlays

```bash
dhg generate -f ./manifests \
  -o ./chart \
  --chart-name myapp \
  --env-values \
  --namespace-resources \
  --feature-flags \
  --include-schema \
  --include-tests

# Install for production
helm install myapp ./chart/myapp \
  -f ./chart/myapp/values-prod.yaml \
  --namespace production
```

### With cloud provider annotations (AWS)

```bash
dhg generate -f ./manifests \
  -o ./chart \
  --chart-name myapp \
  --cloud-provider aws \
  --detect-ingress

# Services get AWS NLB annotations; Ingress gets nginx/alb controller annotations
```

### Spot instance workload

```bash
dhg generate -f ./manifests \
  -o ./chart \
  --chart-name batch-jobs \
  --spot \
  --spot-grace-period 30 \
  --cloud-provider gcp
```

### Deckhouse module with secrets

```bash
dhg generate -f ./manifests \
  -o ./module \
  --chart-name my-deckhouse-module \
  --deckhouse-module \
  --secret-strategy eso \
  --env-values
```

### Analyze before generating

```bash
# First understand what you have
dhg analyze -f ./manifests --output-format markdown -o analysis.md
cat analysis.md

# Then generate based on recommendations
dhg generate -f ./manifests -o ./chart --chart-name myapp
```

### Fix then generate

```bash
# Fix security issues in manifests first
dhg fix -f ./manifests -o ./fixed --workload-type web -v

# Generate from the fixed manifests
dhg generate -f ./fixed -o ./chart --chart-name myapp

# Validate the result
dhg validate -f ./chart/myapp -v
```

---

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `no resources extracted` | `-f` path does not exist or contains no YAML | Verify path: `ls ./manifests/*.yaml` |
| `invalid mode: umbrella` | Typo in `--mode` value | Must be one of: `universal`, `separate`, `library`, `umbrella` |
| `--monorepo and --kustomize are mutually exclusive` | Both flags specified | Use one or the other |
| `unknown cloud provider: "eks"` | `--cloud-provider` value not recognized | Must be `aws`, `gcp`, or `azure` |
| Templates have unbalanced `{{ }}` | Manually edited template with syntax error | Run `dhg validate -f ./chart/myapp` to identify the file |
| `no extractor available for source type: cluster` | `--source cluster` used before Phase 4 is complete | Use `--source file` (default) |
| Docker permission error | `$(pwd)` resolves incorrectly on Windows | Use absolute paths: `-v /c/Users/you/project:/work` |
