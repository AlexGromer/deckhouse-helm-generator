# Release v0.3.0 — Complete Output Modes

**Date**: 2026-02-18
**Type**: Minor release (new features, backwards-compatible)

---

## Summary

v0.3.0 completes the **Output Modes** feature set, bringing DHG from 1 mode to 4 modes:

| Mode | Status |
|------|--------|
| `universal` | ✅ Existing (v0.1.0) |
| `separate` | ✅ **NEW** |
| `library` | ✅ **NEW** |
| `umbrella` | ✅ **NEW** |
| `--env-values` flag | ✅ **NEW** |

---

## What's New

### 1. Separate Mode (`--mode separate`)

Generates an independent Helm chart per detected service group.

```bash
dhg generate -f ./manifests -o ./charts --chart-name myapp --mode separate
```

- Each chart has its own `Chart.yaml`, `values.yaml`, `templates/`
- Inter-chart dependency declarations (via `Chart.yaml dependencies`)
- Full pipeline: grouping → per-group chart generation → dependency detection

### 2. Library Mode (`--mode library`)

Generates a shared library chart with DRY named templates + thin wrapper charts per service.

```bash
dhg generate -f ./manifests -o ./charts --chart-name myapp --mode library
```

DRY shared sub-templates (defined once in library chart, used by all wrappers):
- `library.resources` — CPU/memory requests and limits
- `library.probes` — liveness and readiness probes
- `library.env` — environment variables
- `library.volumeMounts` / `library.volumes` — volume configuration
- `library.labels` / `library.annotations` — metadata
- `library.securityContext` / `library.containerSecurityContext` — security settings

### 3. Umbrella Mode (`--mode umbrella`)

Generates a parent umbrella chart with all services as conditionally-togglable subcharts.

```bash
dhg generate -f ./manifests -o ./charts --chart-name myapp --mode umbrella
# Deploy without database:
helm upgrade --install myapp ./charts/myapp --set database.enabled=false
```

Features:
- Automatic `condition: <name>.enabled` in parent `Chart.yaml`
- Default `enabled: true` per subchart in parent `values.yaml`
- Global values section for shared configuration (`imageRegistry`, etc.)
- Cascading values: parent values override subchart values

### 4. Environment-Specific Values (`--env-values`)

Generates `values-dev.yaml`, `values-staging.yaml`, `values-prod.yaml` alongside the main chart.

```bash
dhg generate -f ./manifests -o ./chart --chart-name myapp --env-values
helm upgrade --install myapp ./chart/myapp -f ./chart/myapp/values-prod.yaml
```

Environment profiles:

| Setting | Dev | Staging | Prod |
|---------|-----|---------|------|
| `replicaCount` | 1 | 2 | 3 |
| `logLevel` | debug | info | warn |
| PDB | disabled | enabled, min=1 | enabled, min=2 |
| Resource limits | none | none | cpu: 500m, memory: 512Mi |
| Anti-affinity | none | none | pod anti-affinity |

---

## Installation

```bash
# From source
git clone https://github.com/deckhouse/deckhouse-helm-generator.git
cd deckhouse-helm-generator
git checkout v0.3.0
make build
sudo cp bin/dhg /usr/local/bin/

# Docker
docker run --rm -v $(pwd):/workspace ghcr.io/deckhouse/dhg:v0.3.0 \
  generate -f /workspace/manifests -o /workspace/charts --chart-name myapp
```

---

## Performance

Benchmarks on Intel Core i5-6300U @ 2.40GHz:

| Resources | Time | Memory |
|-----------|------|--------|
| 10 | 16.96ms | 7.2 MB |
| 100 | 217.69ms | 80.4 MB |
| 1000 | 4.17s | 1.1 GB |

All within release criteria of <10s for 100 resources. ✅

---

## Upgrade Notes

v0.3.0 is **backwards-compatible** with v0.2.0:

- Default mode remains `universal` — no changes required for existing users
- New `Options.EnvValues bool` field defaults to `false` — no behavior change
- All existing tests pass unchanged

---

## Test Coverage

| Package | Coverage |
|---------|----------|
| `pkg/generator` (new code) | 71.3% |
| `pkg/generator/envvalues.go` | 100% |
| `pkg/generator/umbrella.go` | 87.5% |
| `pkg/generator/library.go` | 85.7% |
| `pkg/generator/separate.go` | 75% |
| `pkg/processor/k8s` | 89.2% |
| `pkg/analyzer/detector` | 92.6% |

---

## Contributors

- DHG Team + Claude Code (AI-assisted development)
