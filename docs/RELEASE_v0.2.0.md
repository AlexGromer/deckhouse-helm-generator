# Release v0.2.0 — Deckhouse Helm Generator

## Summary

v0.2.0 adds 9 new Kubernetes resource processors, bringing the total to 18 supported resource types. Full RBAC support, batch workloads (CronJob/Job), autoscaling (HPA/PDB), and network security (NetworkPolicy) are now handled automatically.

## What's New

### New Processors (9)
- **Autoscaling**: HorizontalPodAutoscaler (v2), PodDisruptionBudget
- **Batch**: CronJob, Job (with Helm hook support)
- **RBAC**: Role, ClusterRole, RoleBinding, ClusterRoleBinding
- **Security**: NetworkPolicy

### Testing Infrastructure
- Integration test framework with full pipeline testing
- E2E test framework with Helm lint/template/install validation
- Performance benchmark suite (10/100/1000 resource sets)
- CI pipeline with coverage gate (80% threshold)

### Bug Fixes
- Fixed YAML numeric type handling (float64 vs int64) via `nestedInt64` helper
- Fixed Helm hook annotation embedding in Job templates
- Improved mock API server stability in E2E tests

## All 18 Supported Resource Types

| Category | Resources |
|----------|-----------|
| Workloads | Deployment, StatefulSet, DaemonSet, CronJob, Job |
| Networking | Service, Ingress, NetworkPolicy |
| Configuration | ConfigMap, Secret, PersistentVolumeClaim |
| Autoscaling | HorizontalPodAutoscaler, PodDisruptionBudget |
| RBAC | ServiceAccount, Role, ClusterRole, RoleBinding, ClusterRoleBinding |

## Installation

### Binary
```bash
# Linux AMD64
curl -LO https://github.com/deckhouse/deckhouse-helm-generator/releases/download/v0.2.0/dhg-linux-amd64
chmod +x dhg-linux-amd64
sudo mv dhg-linux-amd64 /usr/local/bin/dhg
```

### Docker
```bash
docker pull ghcr.io/deckhouse/deckhouse-helm-generator:v0.2.0
docker run --rm -v $(pwd):/data ghcr.io/deckhouse/deckhouse-helm-generator:v0.2.0 generate -f /data/manifests -o /data/chart
```

### From Source
```bash
git clone https://github.com/deckhouse/deckhouse-helm-generator.git
cd deckhouse-helm-generator
git checkout v0.2.0
make build
```

## Performance

| Resources | Pipeline Time |
|-----------|--------------|
| 50 | ~15ms |
| 500 | ~191ms |
| 5000 | ~3.4s |

## Upgrade Notes

No breaking changes from v0.1.0. Existing workflows continue to work without modification. New resource types are processed automatically when present in input manifests.
