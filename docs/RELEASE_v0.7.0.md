# Release v0.7.0 — Phase 1 Completion + Phase 2 Architecture Generation

**Date**: 2026-03-27
**Tag**: v0.7.0

## Highlights

### Phase 1 (v0.7.0 base)
- 4 new K8s processors: VPA, PriorityClass, LimitRange, ResourceQuota
- 8 new pattern detectors/checkers: Job, Operator, InitContainer, QoS, StatefulSet, DaemonSet, GracefulShutdown, PodSecurityStandards
- Checksum annotations for ConfigMap/Secret change detection
- Deprecated API migration (12 entries)
- `dhg validate` and `dhg diff` CLI commands

### Phase 2 — Architecture Generators (12 new generators, 3 tiers)

**Tier 1 — Infrastructure:**
- Air-gapped deployment support (`--airgap-registry`)
- Namespace governance: ResourceQuota, LimitRange, NetworkPolicy (`--namespace-resources`)
- Auto-NetworkPolicy from service relationship analysis
- Multi-tenant overlay with per-tenant isolation (`--multi-tenant`)

**Tier 2 — Detection & Annotation:**
- Feature flags with 6-category conditional guards (`--feature-flags`)
- Cloud annotations for AWS/GCP/Azure load balancers (`--cloud-provider`)
- Ingress controller detection: nginx, traefik, haproxy, istio (`--detect-ingress`)
- Workload-aware environment profiles (web/worker/database/batch/cache)

**Tier 3 — Advanced Orchestration:**
- Monorepo layout with Makefile and chart-testing config (`--monorepo`)
- Spot/preemptible instance support with graceful shutdown (`--spot`)
- Kustomize overlay generation: base + dev/staging/prod (`--kustomize`)
- Auto dependency detection for 7 infra services (`--auto-deps`)

## Stats

| Metric | Value |
|--------|-------|
| New generators | 12 |
| New CLI flags | 12 |
| New tests (Phase 2) | 161 (50 + 59 + 52) |
| Implementation LOC | 2,772 |
| Test LOC | 4,916 |
| Test:impl ratio | 1.78:1 |
| Total packages green | 14/14 |
| Coverage | 86%+ |
| Dependencies added | 0 |

## New CLI Flags

| Flag | Type | Description |
|------|------|-------------|
| `--airgap-registry` | string | Target registry URL for air-gapped environments |
| `--namespace-resources` | bool | Generate ResourceQuota/LimitRange/NetworkPolicy |
| `--multi-tenant` | bool | Generate multi-tenant overlay |
| `--feature-flags` | bool | Inject feature-flag conditional guards |
| `--cloud-provider` | string | Cloud provider (aws/gcp/azure) for annotations |
| `--cloud-internal` | bool | Use internal load balancer annotations |
| `--detect-ingress` | bool | Auto-detect ingress controller type |
| `--monorepo` | bool | Generate monorepo layout structure |
| `--spot` | bool | Enable spot/preemptible instance config |
| `--spot-grace-period` | int | Graceful shutdown period in seconds (default: 15) |
| `--kustomize` | bool | Generate Kustomize overlays |
| `--auto-deps` | bool | Auto-detect infrastructure dependencies |

## Known Issues

See [#29](https://github.com/AlexGromer/deckhouse-helm-generator/issues/29) for Phase 2 code review findings (71 items, tracked for v0.7.1).

## Installation

```bash
# Homebrew
brew install AlexGromer/tap/dhg

# Go install
go install github.com/AlexGromer/deckhouse-helm-generator/cmd/dhg@v0.7.0

# Docker
docker pull ghcr.io/alexgromer/dhg:0.7.0

# Binary (Linux amd64)
curl -sL https://github.com/AlexGromer/deckhouse-helm-generator/releases/download/v0.7.0/dhg_Linux_x86_64.tar.gz | tar xz
```
