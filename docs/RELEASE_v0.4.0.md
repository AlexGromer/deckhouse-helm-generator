# Release v0.4.0 — Deckhouse Integration + Modern K8s

**Date**: 2026-02-19
**Tag**: v0.4.0

## Highlights

- **7 Deckhouse CRD processors**: ModuleConfig, IngressNginxController, ClusterAuthorizationRule, NodeGroup, DexAuthenticator, User, Group
- **Deckhouse Module Scaffold** (`--deckhouse-module`): helm_lib dependency, OpenAPI schemas
- **Monitoring Stack**: ServiceMonitor, PodMonitor, PrometheusRule, GrafanaDashboard
- **Gateway API**: HTTPRoute + Gateway with dependency tracking
- **KEDA**: ScaledObject (scale-to-zero detection) + TriggerAuthentication
- **cert-manager**: Certificate + ClusterIssuer
- **Argo Rollouts**: canary/blueGreen strategy support
- **ExternalDNS detection** on Services and Ingresses
- **TopologySpreadConstraints** extraction from Deployments

## Stats

| Metric | Value |
|--------|-------|
| Total processors | 36 (was 18 in v0.3.0) |
| New processors | 18 |
| New unit tests | 97 |
| New integration tests | 7 |
| Coverage (processor/k8s) | 89.2% |
| Coverage (detector) | 92.5% |

## Benchmarks

| Scenario | Time | Memory | Allocs |
|----------|------|--------|--------|
| 10 resources | 16.2ms | 7.2MB | 36.6K |
| 100 resources | 198.3ms | 80.4MB | 378K |
| 1000 resources | 3.72s | 1.12GB | 5.57M |

## Breaking Changes

None. Fully backward-compatible with v0.3.0.

## Docker

```bash
docker pull dhg:v0.4.0
docker run --rm dhg:v0.4.0 generate -f ./manifests -o ./chart --chart-name myapp
```

## Implementation Velocity

| Phase | Solo Est | Actual | Velocity |
|-------|----------|--------|----------|
| Deckhouse Core (1.1-1.5) | 55.0h | 0.27h | 203.7x |
| Monitoring + Modern K8s (2.1-2.5) | 55.0h | 0.44h | 125.0x |
| Integration + Release (3.1-3.3) | 19.5h | ~0.2h | ~100x |
| **Total** | **129.5h** | **~0.9h** | **~144x** |
