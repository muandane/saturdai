# LLD-060: Recommendation engine

## Purpose

Compute `status.recommendations[]` (CPU/memory request and limit quantities) from `WorkloadProfile` spec `mode`, per-container min/max clamps, DDSketch quantiles, EMA-based prediction (spec §7), and `burst`-mode peak rules — **before** safety mutations (070). Deterministic and fully explainable per spec §8 and §16.

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| §7 | `prediction = EMA_long + k * (EMA_short - EMA_long)`; k per mode |
| §8 | cost/balanced/resilience/burst percentile formulas |
| §4 | Output shape: `cpuRequest`, `cpuLimit`, `memoryRequest`, `memoryLimit`, `rationale` seed |
| §16 | No ML; same inputs → same outputs |

## Scope and non-goals

**In scope:** Query P40/P50/P70/P90/P95/P99 from sketches; apply mode table; apply min/max from spec; merge prediction into burst CPU limit rule where spec says `max(P99, peak_observed)`.

**Out of scope:** 30% decrease cap, cooldown, OOM overrides, slope guard (070), PATCH (090).

## Dependencies

- **Upstream:** [010-workloadprofile-api.md](./010-workloadprofile-api.md), [040-aggregate-engine.md](./040-aggregate-engine.md)
- **Downstream:** [070-safety-layer.md](./070-safety-layer.md)

## Data model / API surface

```go
type ProfileMode string

const (
    ModeCost        ProfileMode = "cost"
    ModeBalanced    ProfileMode = "balanced"
    ModeResilience  ProfileMode = "resilience"
    ModeBurst       ProfileMode = "burst"
)

// RecommendInput carries read-only view of status + spec for one container.
type RecommendInput struct {
    Mode           ProfileMode
    MinCPU, MaxCPU, MinMem, MaxMem resource.Quantity
    CPUSketch      *ddsketch.DDSketch // decoded from status
    MemSketch      *ddsketch.DDSketch
    CPUEMALong, CPUEMAShort     float64
    MemEMALong, MemEMAShort     float64
}

type Recommendation struct {
    ContainerName     string
    CPURequest, CPULimit       resource.Quantity
    MemRequest, MemLimit       resource.Quantity
    RationaleParts    []string // joined by 070
}

func ComputeRecommendation(in RecommendInput) (Recommendation, error)
```

**Peak observed (burst):** Use max of sketch samples is not directly exposed — **define** `peak_observed` as **max of EMA_short and P99** from memory sketch for memory limit; for CPU limit use `max(P99_cpu, CPUEMA_short)` unless better signal exists from sketch `getMaxValue()` if API allows. Document exact choice in code comments; must be deterministic.

## Algorithms and invariants

### Percentiles

- From decoded DDSketch: `getValueAtQuantile(q)` where q ∈ (0,1).
- Map spec: P50 → 0.5, P70 → 0.7, etc.

### Mode table (reference spec §8 — do not drift)

| Mode | cpuRequest | cpuLimit | memRequest | memLimit | k |
|------|------------|----------|------------|----------|---|
| cost | P50 | P90 | P50 | P90 | 0.5 |
| balanced | P70 | P95 | P70 | P95 | 1.0 |
| resilience | P90 | P99×1.1 | P90 | P99×1.2 | 1.5 |
| burst | P40 | max(P99, peak_cpu) | P40 | peak_mem | 2.0 |

**Resilience:** Apply multipliers after quantile fetch using exact floating math then round to `Quantity` with documented rounding (ceil for limits).

### Prediction usage

- Spec §7 ties prediction to burst behavior implicitly via `k`; **clarify implementation:** use prediction to **adjust** limit upward from baseline quantile when short EMA > long EMA:

`cpuLimit = max(quantileLimit, predictionCpu)` where `predictionCpu = cpuEMALong + k*(cpuEMAShort - cpuEMALong)` in millicores, then convert to Quantity.

- Same optional adjustment for memory **unless** slopePositive (handled in 070 by skipping memory recommendation entirely).

### Clamping

- After computation: clamp to `[min*, max*]` per resource.

### Rationale seed

- Build structured parts, e.g. `balanced`, `P70_cpu=230m`, `P95_cpu=890m` — **070** appends safety/override phrases.

## Failure modes and behavior

| Case | Behavior |
|------|----------|
| Empty sketch / insufficient data | Omit recommendation or emit **no-op** rec equal to current template resources (090 defines read path); set rationale `insufficient_data` |
| Decode sketch fails | Same as insufficient_data; do not panic |

## Security / RBAC

- None (pure function over in-memory inputs).

## Observability

- Debug log with mode and quantile values (no PII).

## Test plan

- **Unit:** Golden files per mode with fixed sketch contents (construct sketch, add known values, freeze expected quantiles).
- **Unit:** k and prediction monotonicity: higher short EMA raises limit.
- **Unit:** Clamp min/max boundaries.
- **Acceptance:** Table tests for all four modes + edge burst peak rule.

## Rollout / migration

- Mode enum changes require API + this LLD + spec alignment.

## Open questions

- Exact definition of `peak_observed` for burst — **lock in implementation** with spec amendment if needed.
- Whether prediction applies to **requests** or **limits only** — spec emphasis on limits; **recommend limits + requests from percentiles only**, prediction merges into limit side only.
