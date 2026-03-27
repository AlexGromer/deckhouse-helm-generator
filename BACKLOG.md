# BACKLOG — deckhouse-helm-generator

## Active (max 10)

- [ ] HS-1: Shell injection in airgap.go GenerateMirrorScript (P1) @security
- [ ] HS-2: Makefile target injection in monorepo.go sanitizeChartName (P1) @security
- [ ] HS-3: YAML injection in kustomize.go resource names (P1) @security
- [ ] HS-4: CI workflow injection in monorepo.go (P1) @security
- [ ] CR-1: namespace-resources mutates Templates directly (P1) @correctness
- [ ] CR-2: InjectDependencies shares Templates map reference (P1) @correctness
- [ ] CR-3: Spot provider silently defaults to AWS (P1) @correctness
- [ ] HC-1: Double-injection cloud+ingress on same Ingress (P1) @correctness
- [ ] HC-3: Nil pointer dereference in extractContainers (P1) @correctness
- [ ] HC-7: injectTolerationsIntoTemplate appends to doc root (P1) @correctness

## Queue
<!-- auto-promote top 10 to Active when slots free -->

### Phase 1 — Incomplete
- [ ] 1.3.2: Helm test templates auto-scaffold (P2) @generator
- [ ] 1.3.3: Chart hooks generation — pre-upgrade, post-install, pre-delete (P2) @generator
- [ ] 1.3.6: NOTES.txt dynamic generation (P2) @generator
- [ ] 1.3.4: Configurable template style --template-style (P3) @generator
- [ ] 1.3.7: Values.yaml design patterns + --values-flat (P3) @generator
- [ ] 1.3.9: Advanced _helpers.tpl named templates (P3) @generator
- [ ] 1.4.4: Progress bar for large directories (P3) @cli
- [ ] 1.5.3: Sidecar detection — envoy, fluent-bit, vault-agent (P3) @analyzer
- [ ] 1.5.5: Topology Spread Constraints templating (P3) @analyzer

### Phase 2 — Security & Compliance (2.6.x)
- [ ] 2.6.1: PSS auto-migration — securityContext auto-fix (P2) @security
- [ ] 2.6.2: NetworkPolicy default-deny + allow patterns (P2) @security
- [ ] 2.6.3: RBAC least-privilege auto-generation (P2) @security
- [ ] 2.6.4: Resource limits auto-configuration by workload type (P2) @security
- [ ] 2.6.5: External Secrets migration — Secret to ExternalSecret (P2) @security
- [ ] 2.6.8: Ingress security — TLS auto-add, cert-manager integration (P2) @security
- [ ] 2.6.6: Image security — pullPolicy, private registry, tag validation (P3) @security
- [ ] 2.6.7: Audit logging — K8s Audit Policy generation (P3) @security
- [ ] 2.6.9: Admission control — Kyverno/OPA policy generation (P3) @security
- [ ] 2.6.10: Supply chain — SBOM + cosign signing CI templates (P3) @security
- [ ] 2.4.7: Kustomize-hybrid --post-renderer-mode for Flux CD (P3) @generator

### Phase 3 — Deckhouse (partially done)
- [ ] 3.1.7: Cloud-specific InstanceClass processors (P3) @processor
- [ ] 3.3.3: Deckhouse version compatibility validation (P3) @analyzer
- [ ] 3.5.7: Gateway API — GRPCRoute/TLSRoute processors (P3) @processor
- [ ] 3.5.10: Flagger Canary processor (P3) @processor

### Phase 4 — Source Expansion
- [ ] 4.1: Cluster Extractor — live cluster via client-go (P2) @extractor — 6 subtasks
- [ ] 4.2: GitOps Extractor — git repo via go-git (P2) @extractor — 5 subtasks
- [ ] 4.3: Multi-Source Merge — dedup, conflict resolution (P3) @extractor — 3 subtasks

### Phase 5 — Advanced Analysis
- [ ] 5.1: Auto-Fix Engine — securityContext, resources, probes, PDB (P2) @generator — 7 subtasks
- [ ] 5.2: Generic CRD Support — schema extraction, crd/ dir (P2) @processor — 3 subtasks
- [ ] 5.3: Dependency Analysis — DOT graph, circular detection (P3) @analyzer — 3 subtasks
- [ ] 5.4: Migration Support — drift detection, migration plan (P2) @cli — 3 subtasks
- [ ] 5.5: Smart Analysis — cost estimation, right-sizing, compliance (P3) @analyzer — 9 subtasks
- [ ] 5.6: Advanced Templating — post-renderer, operator scaffold (P3) @generator — 2 subtasks
- [ ] 5.7: Secret Management — ESO, Sealed Secrets, Vault, SOPS (P2) @generator — 7 subtasks
- [ ] 5.8: Service Mesh Integration (P3) @generator — from Pass 7

## Deferred
<!-- tasks postponed by user decision -->

_No deferred tasks._

## Completed Archive
<!-- keep last 50, older delete -->

- [x] Phase 2 Code Review + Release v0.7.0 (P1) — 2026-03-27 ✓ 2026-03-27
- [x] Merge PR #24 (Phase 2 Tier 3) (P1) — 2026-02-27 ✓ 2026-02-27
- [x] Phase 2 Architecture Generation (Tiers 1+2 PR #23) (P1) ✓ 2026-02-27
- [x] Phase 2 Tier 3 — monorepo, spot, kustomize, autodeps (P1) ✓ 2026-02-27
- [x] Phase 2 Tier 2 — featureflags, cloudannotations, ingressdetect (P1) ✓ 2026-02-26
- [x] Phase 2 Tier 1 — airgap, namespace, networkpolicy, multitenant (P1) ✓ 2026-02-26
- [x] CHANGELOG v0.5.0/v0.6.0, golangci config, lint fixes (P2) ✓ 2026-02-26
- [x] Boost coverage to 86% (P1) ✓ 2026-02-26
- [x] Phase 1 completion — processors, detectors, generator, CLI (P1) ✓ 2026-02-26
