# LLD-400: Time-based patterns

## Purpose

Engineering contract for **shipped** time-aware behavior: **UTC quadrant DDSketches** (four 6-hour buckets per CPU/memory aggregate) and **Holt–Winters** hourly forecasts that feed recommendation inputs. This document also lists **Phase 5 backlog** items that are **not** implemented (24 separate DDSketch slots per hour-of-day, automatic burst/off-peak profile switching).

## Spec traceability

| Spec § | Requirement (summary) | Status |
|--------|------------------------|--------|
| §2 Extensions | Per-container UTC quadrant sketches; Holt–Winters in `mlstate` (deterministic, in-cluster) | **Shipped** |
| §6 | Learned pipeline; quadrant vs global sketch for quantiles | **Shipped** |
| §15 Phase 5 | Hour-of-day bucketing (24 sketch slots per container); automatic burst/off-peak profile switching | **Deferred** |

## Disambiguation: “24” in two places

| Mechanism | What “24” means | Notes |
|-----------|-----------------|-------|
| **Holt–Winters** | `HWState.Season [24]float64` — **hourly seasonal indices** (multipliers), not DDSketch storage | [`holtwinters.go`](../../../../internal/aggregate/holtwinters.go) |
| **Phase 5 (deferred)** | Up to **24 separate DDSketch** time slots per metric per container (spec roadmap) | Not implemented |

Shipped **quadrant** sketches are **four** base64 DDSketches per metric (6-hour UTC windows), not 24.

## Shipped behavior

### UTC quadrant sketches (6-hour UTC buckets)

Per CPU/memory aggregate, status may hold **`quadrantSketches`**: up to **four** base64-encoded DDSketches aligned to **UTC** wall-clock windows:

| Index | UTC hours |
|-------|-----------|
| 0 | 00:00–06:00 |
| 1 | 06:00–12:00 |
| 2 | 12:00–18:00 |
| 3 | 18:00–24:00 |

The reconciler selects the active bucket with **`UTC_hour / 6`** (integer division), implemented as **`utcQuadrantIndex`** in [`internal/controller/reconcile_ml_helpers.go`](../../../../internal/controller/reconcile_ml_helpers.go) and used from [`reconcile_ingest.go`](../../../../internal/controller/reconcile_ingest.go) (ingest) and [`reconcile.go`](../../../../internal/controller/reconcile.go) (recommendation input).

**Quantile source:** [`internal/recommend/recommend.go`](../../../../internal/recommend/recommend.go) uses the active quadrant sketch for CPU/memory quantiles when that sketch has at least **`minQuadrantSketchCount`** (30) samples; otherwise the **global** sketch is used. See [`sketch_test.go`](../../../../internal/recommend/sketch_test.go).

**CUSUM changepoint** on a resource clears the global sketch and **resets all quadrant buckets** via **`clearQuadrantSketches`** ([`reconcile_ml_helpers.go`](../../../../internal/controller/reconcile_ml_helpers.go)) so aggregation restarts clean.

**Etcd budget:** four sketches per metric per container — smaller than a hypothetical 24-sketch-per-metric Phase 5 design.

### Holt–Winters forecasts

- **State:** `HWState` — level, trend, **`Season [24]float64`** (hourly season), smoothing params, sample count — [`internal/aggregate/holtwinters.go`](../../../../internal/aggregate/holtwinters.go).
- **Update:** `Update(x, hour)` with **UTC hour** 0–23; returns forecast for the **next** hour.
- **Outputs:** `Input.ForecastCPU` / `Input.ForecastMem` (millicores / bytes); **burst** / **resilience** paths use forecasts when **> 0** after warmup.
- **Persistence:** per-container CPU/memory HW in **`internal/mlstate`** (`ContainerHW`).

This is **hourly seasonality for headroom/forecasts**, not Phase 5 profile switching.

## Phase 5 backlog (not shipped)

Explicitly **out of scope** for the current codebase:

- **24 sketch slots** per metric per container (or alternative ring buffer) with timezone/DST per profile
- **Automatic** burst ↔ off-peak **mode** switching with cooldown policy
- Larger **`status` footprint** — any future design must stay within etcd object limits (spec §6)

## Dependencies

- **Upstream:** [040-aggregate-engine.md](./040-aggregate-engine.md), [060-recommendation-engine.md](./060-recommendation-engine.md), [070-safety-layer.md](./070-safety-layer.md), [010-workloadprofile-api.md](./010-workloadprofile-api.md), `internal/mlstate`, reconcile ingest
- **Downstream:** Phase 5 items above (future); no downstream consumer until those features are specified

## Algorithms and invariants

- Quadrant bucket selection is **deterministic** given UTC time (reconciler **`Clock`** injectable for tests in `internal/controller`).
- HW parameters and CUSUM thresholds live in code defaults / mlstate — no external tuning service.

## Failure modes and behavior

| Case | Behavior |
|------|----------|
| Clock skew | Trust controller node clock; multi-cluster trust model TBD if needed |
| Missed reconciles | Quadrant index jumps to current window; sketches merge online |

## Security / RBAC

None beyond existing controller RBAC (ConfigMaps for mlstate).

## Observability

Quadrant sketch updates may log at debug in [`reconcile_ingest.go`](../../../../internal/controller/reconcile_ingest.go) (optional).

## Test plan

Existing coverage:

- **Quadrant vs global:** [`internal/recommend/sketch_test.go`](../../../../internal/recommend/sketch_test.go) — `TestEffectiveCPUSketch_prefersQuadrantWhenEnoughSamples` (and memory analogue).
- **Holt–Winters:** [`internal/aggregate/holtwinters_test.go`](../../../../internal/aggregate/holtwinters_test.go) — warmup, season updates, finite samples.
- **UTC quadrant index:** [`internal/controller/reconcile_ml_helpers_test.go`](../../../../internal/controller/reconcile_ml_helpers_test.go) — `utcQuadrantIndex` table tests (day boundary).

## Rollout / migration

`quadrantSketches` fields are optional on status — backward compatible.

## Open questions (Phase 5 full)

- Timezone per cluster vs per profile
- Interaction between automatic mode switch and [070-safety-layer.md](./070-safety-layer.md) cooldown
