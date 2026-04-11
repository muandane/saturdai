# LLD-400: Time-based patterns

## Purpose

Spec §15 Phase 5: hour-of-day bucketing, automatic burst/off-peak profile switching. This document describes what is **implemented today** vs the **future** Phase 5 stub.

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| §15 Phase 5 | Time-based patterns (full design TBD) |
| §2 Extensions | Quadrant sketches, Holt–Winters (deterministic, in-cluster) |

## Implemented subset (current)

### UTC quadrant sketches (6-hour buckets)

Per CPU/memory aggregate, status may hold **`quadrantSketches`**: up to **four** base64-encoded DDSketches aligned to **UTC** wall-clock windows:

| Index | UTC hours |
|-------|-----------|
| 0 | 00:00–06:00 |
| 1 | 06:00–12:00 |
| 2 | 12:00–18:00 |
| 3 | 18:00–24:00 |

The reconciler selects the active bucket with **`UTC_hour / 6`** (integer division). Recommendation input prefers non-empty quadrant sketches for quantiles when present (see [040-aggregate-engine.md](./040-aggregate-engine.md), [060-recommendation-engine.md](./060-recommendation-engine.md)).

**Etcd budget:** four sketches per metric per container — far smaller than a hypothetical 24-slot-per-metric design.

### Holt–Winters seasonal component

**Holt–Winters** (additive / multiplicative helpers in `internal/aggregate/holtwinters.go`) feeds **forecast** values into the recommendation engine (`Input.ForecastCPU` / `ForecastMem`) together with mlstate persistence. This is **hourly seasonality** and changepoint context — **not** the Phase 5 “24 sketch slots + auto mode switch” product feature.

## Future Phase 5 (stub)

- **24 sketch slots** per metric per container (or alternative ring buffer) with timezone/DST per profile
- **Automatic** burst ↔ off-peak **mode** switching with cooldown policy
- Larger status footprint — must stay within etcd object limits (spec §6)

## Dependencies

- **Upstream:** [040-aggregate-engine.md](./040-aggregate-engine.md), [060-recommendation-engine.md](./060-recommendation-engine.md), [070-safety-layer.md](./070-safety-layer.md), [010-workloadprofile-api.md](./010-workloadprofile-api.md), mlstate / reconcile ingest
- **Downstream:** TBD (full Phase 5)

## Algorithms and invariants

- Quadrant bucket selection is **deterministic** given UTC time (`internal/controller` clock injectable for tests).
- HW parameters and CUSUM thresholds live in code defaults / mlstate — no external tuning service.

## Failure modes and behavior

| Case | Behavior |
|------|----------|
| Clock skew | Trust controller node clock; document trust model for multi-cluster |
| Missed reconciles | Quadrant index jumps to current window; sketches merge online |

## Security / RBAC

None beyond existing controller RBAC (ConfigMaps for mlstate).

## Observability

Log quadrant index at debug when updating sketches (optional).

## Test plan

- Unit: bucket index `hour/6` across day boundary
- Integration: profile status shows non-empty quadrant sketch after N reconciles in a bucket

## Rollout / migration

Quadrant fields are optional on status — backward compatible.

## Open questions (Phase 5 full)

- Timezone per cluster vs per profile
- Interaction between auto mode switch and [070-safety-layer.md](./070-safety-layer.md) cooldown
