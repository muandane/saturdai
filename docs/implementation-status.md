# Autosize controller — implementation status

**Authoritative requirements:** [spec/autosize-controller-spec.md](./spec/autosize-controller-spec.md)  
**LLD index:** [LLD/autosize/README.md](./LLD/autosize/README.md)

Last reviewed: 2026-04-11 (update when scope changes).

## Legend

| Status | Meaning |
|--------|---------|
| Done | Implemented and wired in production code paths |
| Partial | Exists but differs from spec/LLD or missing edge behavior |
| Not started | No meaningful implementation |
| N/A | Explicitly out of scope (future phase / stub LLD) |

## Phase 1 — Core MVP (spec §15)

| Item | Spec § / LLD | Status | Notes |
|------|----------------|--------|--------|
| WorkloadProfile CRD & API shapes | §4, 010 | Done | `api/v1`, `config/crd/bases` |
| Target: Deployment / StatefulSet | 020 | Done | `internal/target` |
| Kubelet stats via node proxy | §5, 030 | Done | `internal/kubelet` — not direct kubelet |
| EMA short/long (α 0.2 / 0.05) | §6, 040 | Done | `internal/aggregate/ema.go` |
| DDSketch in status (base64) | §6, 040 | Done | `internal/aggregate` |
| Pod signals: OOM from pod status | §5, 050 | Partial | OOM used in safety via in-memory snapshot; `status.containers[].lastOOMKill` not populated from pods |
| Pod signals: restart count & delta | §5, §9, 050 | Not started | `RestartCount` collected in snapshot but not persisted; no delta vs prior reconcile |
| Four modes + percentile tables | §8, 060 | Done | `internal/recommend/recommend.go` |
| Prediction `EMA_long + k * (EMA_short - EMA_long)` | §6–7, 060 | Not started | Percentiles only; `k` unused |
| Safety: max 30% decrease | §9, 070 | Done | Implemented as ≥70% of current (`internal/safety`) |
| Safety: cooldown vs `lastApplied` | §9, 070 | Done | |
| Safety: OOM override mem×1.5, bypass cooldown | §9, 070 | Done | Lookback ~10m in code |
| Safety: high CPU throttle override | §9, 070 | Done | >50% throttled/usage |
| Safety: restart spike → pause downsizing 2 cycles | §9, 070 | Not started | |
| Trend guard: `slopePositive` blocks memory downsize | §6, §9, 070 | Partial | Uses `EMA_short > EMA_long * 1.01`, not spec counter-over-N |
| `metricsRecommendations` vs `recommendations` | §4, §9, 060–070 | Done | Pre/post safety |
| Reconcile loop, status update | §10, 080 | Done | `internal/controller/reconcile.go` |
| Actuation: PATCH workload template | §11, 090 | Done | `internal/actuate`; gated by `AUTOSIZE_ACTUATION=true` |
| RBAC / packaging baseline | 100 | Done | `config/rbac`, samples |

## Phase 2 — Admission

| Item | LLD | Status | Notes |
|------|-----|--------|--------|
| Mutating webhook: inject from profile / defaults | 110 | Done | `internal/webhook` |
| Global defaults ConfigMap | 120 | Done | `internal/defaults`, manager flags in `cmd/main.go` |

## Future phases (stub LLDs)

| Item | LLD | Status |
|------|-----|--------|
| DRA integration | 200 | N/A — stub |
| Node-aware sketches / bin-packing | 300 | N/A — stub |
| Time-of-day / hour buckets | 400 | N/A — stub |

## Explicit spec exclusions (unchanged)

| Item | Spec § |
|------|--------|
| HPA, Prometheus, ML, direct Pod mutation | §2, §16 |
| DRA in core scope | §2, §15 |

## How to update this doc

1. After merging a feature PR, set the row to **Done** or **Partial** and one line in **Notes**.
2. If behavior intentionally diverges from spec, add a **Notes** bullet and consider opening a spec/LLD alignment issue.
