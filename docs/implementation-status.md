# Autosize controller — implementation status

**Authoritative requirements:** [spec/autosize-controller-spec.md](./spec/autosize-controller-spec.md)  
**LLD index:** [LLD/autosize/README.md](./LLD/autosize/README.md)  
**GitHub:** [muandane/saturdai](https://github.com/muandane/saturdai) — tracking issues use `[#NN](https://github.com/muandane/saturdai/issues/NN)` in **Issue** columns below.

Last reviewed: 2026-04-11 — includes learned-state pipeline (ConfigMap `mlstate-*`, CUSUM, feedback bias, quadrant sketches, Holt–Winters forecasts).

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
| Pod signals: OOM from pod status | §5, 050 | Done | Merged pod `lastState` OOM `finishedAt` (max per container) in `status.containers[].stats.lastOOMKill`; safety uses same snapshot |
| Pod signals: restart count & delta | §5, §9, 050 | Done | Max restart per container in `status.containers[].stats.restartCount`; delta vs prior after `lastEvaluated` baseline (`internal/controller/reconcile.go`, `internal/podsignals`) |
| Four modes + percentile tables | §8, 060 | Done | Strategy `Engine` + `biasedEngine` in `internal/recommend` (`engine.go`, `strategies.go`, `biased.go`, `bias.go`); `Compute` delegates to `New(..., NoopBias{})` |
| Prediction `EMA_long + k * (EMA_short - EMA_long)` | §6–7, 060 | Done | `mergeLimitsWithEMAPrediction` in `internal/recommend` (limits only, after quantiles/forecast/peak); rationales include `k` and `cpu_pred`/`mem_pred`; tests in `recommend_test.go` |
| Safety: max 30% decrease | §9, 070 | Done | Implemented as ≥70% of current (`internal/safety`) |
| Safety: cooldown vs `lastApplied` | §9, 070 | Done | |
| Safety: OOM override mem×1.5, bypass cooldown | §9, 070 | Done | Lookback ~10m in code |
| Safety: high CPU throttle override | §9, 070 | Done | >50% throttled/usage |
| Safety: restart spike → pause downsizing 2 cycles | §9, 070 | Done | `delta > 3` after baseline → `status.downsizePauseCyclesRemaining`; safety holds decreases (`internal/safety`); counter decremented each reconcile (`restart_pause.go`) |
| Trend guard: `slopePositive` blocks memory downsize | §6, §9, 070 | Done | `status.containers[].stats.memory.slopeStreak` + prior `EMA_short` from last reconcile; `N=5` in `internal/aggregate/slope.go`; tests in `slope_test.go` |
| `metricsRecommendations` vs `recommendations` | §4, §9, 060–070 | Done | Pre/post safety |
| Reconcile loop, status update | §10, 080 | Done | `internal/controller/reconcile.go` |
| Actuation: PATCH workload template | §11, 090 | Done | `internal/actuate`; gated by `AUTOSIZE_ACTUATION=true` |
| RBAC / packaging baseline | 100 | Done | `config/rbac`, samples |

## Learned state & heuristics (post-MVP)

| Item | Location | Status | Notes |
|------|----------|--------|--------|
| Template-method sketch + EMA update | `internal/aggregate/updater.go` | Done | Shared CPU/mem path; corrupt status sketch falls back to empty sketch |
| Injectable `Clock` on reconciler | `internal/controller/workloadprofile_controller.go`, `reconcile.go` | Done | `cmd/main.go` sets `time.Now`; tests can fix time |
| ML state ConfigMap repository | `internal/mlstate` | Done | Name `mlstate-<profile.Name>`; JSON key `state`; owner ref to `WorkloadProfile`; corrupt JSON → fresh state |
| RBAC: ConfigMaps for ML state | `config/rbac/role.yaml` | Done | Kubebuilder marker on controller |
| CUSUM changepoint on CPU/memory | `internal/changepoint`, `reconcile_ingest.go` | Done | Compared to **pre-sample** `EMA_long`; threshold defaults in `cusum.go`; shift clears global sketch string |
| Feedback EWMA + `LiveBias` | `internal/recommend/feedback.go` | Done | `RecordUsage` vs prior **`status.recommendations`** (post-safety); min samples before bias |
| UTC quadrant DDSketches (6h buckets) | `api/v1` `QuadrantSketches`, `reconcile_ingest.go` | Done | Slice max 4; `recommend.Input` prefers non-empty quadrant sketch for quantiles |
| Holt–Winters + forecasts | `internal/aggregate/holtwinters.go`, `mlstate` HW map | Done | `Input.ForecastCPU` / `ForecastMem`; burst/resilience use when > 0 |
| `ShiftDetector` / handlers | `internal/changepoint/handler.go` | Done | Optional; reconciler notifies on shift (extensible) |

## Phase 2 — Admission

| Item | LLD | Status | Notes |
|------|-----|--------|--------|
| Mutating webhook: inject from profile / defaults | 110 | Done | `internal/webhook` |
| Global defaults ConfigMap | 120 | Done | `internal/defaults`, manager flags in `cmd/main.go` |

## Future phases (stub LLDs)

| Item | LLD | Status | Notes | Issue |
|------|-----|--------|-------|-------|
| DRA integration | 200 | N/A — stub | | — |
| Node-aware sketches / bin-packing | 300 | N/A — stub | | — |
| Time-of-day / hour buckets | 400 | Partial | Quadrant sketches (6h UTC) + HW hourly season in code; LLD 400 still stub-level | — |

## Explicit spec exclusions (unchanged)

| Item | Spec § |
|------|--------|
| HPA, Prometheus, ML, direct Pod mutation | §2, §16 |
| DRA in core scope | §2, §15 |

## How to update this doc

1. After merging a feature PR, set the row to **Done** or **Partial** and one line in **Notes**.
2. If behavior intentionally diverges from spec, add a **Notes** bullet and consider opening a spec/LLD alignment issue.
3. For rows with status **Partial** or **Not started**, add a link to the tracking GitHub issue in the **Issue** column (`[#NN](https://github.com/muandane/saturdai/issues/NN)`), or **—** until an issue exists.
