# BACKLOG — deckhouse-helm-generator

## Active (max 10)

- [ ] M-2: GenerateSpotPDB exported but never called from pipeline (P2) @quality
- [ ] M-3: Tenant count hardcoded to 2, no --tenant-count flag (P2) @quality
- [x] M-4: --cloud-provider validation — ALREADY DONE in CR-3 (v0.7.2) ✓
- [ ] M-5: networkpolicy.go hardcoded namespace literal instead of Release.Namespace (P2) @correctness
- [ ] M-6: buildCrossNamespaceIndex non-deterministic order (P2) @quality
- [ ] M-7: Template key collision between namespace.go and networkpolicy.go NP generators (P2) @correctness
- [ ] M-8: Traefik multi-feature: last annotation wins on same key (P2) @correctness
- [x] M-9: kind: Service substring — ALREADY DONE in HC-10 (v0.7.2) ✓
- [ ] M-10: mergeFeatureValues swallows YAML parse error (P2) @quality
- [ ] 1.3.2: Helm test templates auto-scaffold (P2) @generator

## Queue
<!-- auto-promote top 10 to Active when slots free -->

### Issue #29 — Remaining findings (M-11..M-22)
- [ ] M-11: envvalues.go priority comment says 6 levels, logic has 7 branches (P3) @quality
- [ ] M-13: extractPorts only handles int64, misses int (P3) @correctness
- [ ] M-14: appName unsanitized in PDB YAML (P3) @security
- [ ] M-15: Nil chart input causes panic in GenerateKustomizeLayout (P3) @correctness
- [ ] M-16: monorepo Charts slice stored by reference, not copied (P3) @quality
- [ ] M-17: spot_test.go test name/comment misleading about default GracePeriod (P3) @quality
- [ ] M-18: GroupResources called twice in main.go, error discarded (P3) @quality
- [ ] M-19: multitenant NP allows DNS UDP only, not TCP (P3) @correctness
- [ ] M-20: GenerateAirgapValues ignores refs parameter (P3) @quality
- [ ] M-21: --monorepo + --kustomize no conflict guard (P3) @quality
- [ ] M-22: 4 independent workload container-scanning implementations — DRY violation (P3) @quality

### Phase 1 — Incomplete
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

- [x] v0.7.2 P2 fixes — 10 findings (HC-2..HC-11, M-1, M-12) + deps bump (P2) — 2026-03-27 ✓ 2026-03-27
- [x] v0.7.1 P1 fixes — 10 findings (4 security + 6 correctness) (P1) — 2026-03-27 ✓ 2026-03-27
- [x] Phase 2 Code Review + Release v0.7.0 (P1) — 2026-03-27 ✓ 2026-03-27
- [x] Merge PR #24 (Phase 2 Tier 3) (P1) — 2026-02-27 ✓ 2026-02-27
- [x] Phase 2 Architecture Generation (Tiers 1+2 PR #23) (P1) ✓ 2026-02-27
- [x] Phase 2 Tier 3 — monorepo, spot, kustomize, autodeps (P1) ✓ 2026-02-27
- [x] Phase 2 Tier 2 — featureflags, cloudannotations, ingressdetect (P1) ✓ 2026-02-26
- [x] Phase 2 Tier 1 — airgap, namespace, networkpolicy, multitenant (P1) ✓ 2026-02-26
- [x] CHANGELOG v0.5.0/v0.6.0, golangci config, lint fixes (P2) ✓ 2026-02-26
- [x] Boost coverage to 86% (P1) ✓ 2026-02-26
- [x] Phase 1 completion — processors, detectors, generator, CLI (P1) ✓ 2026-02-26
