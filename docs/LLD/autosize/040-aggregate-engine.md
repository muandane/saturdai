# LLD-040: Aggregate engine (EMA + DDSketch)

## Purpose

Define the **pure** online update path from one observation cycle into `WorkloadProfile.status.containers[].stats`: dual EMAs per metric, DDSketch ingest and serialization, memory-only slope flag updates, and cold-start behavior. This is the statistical **storage** layer from spec §6; it does not decide requests/limits (060).

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| §6 | EMA short α=0.2, long α=0.05; DDSketch 1% relative accuracy; proto → base64; `MetricAggregate` shape; no raw samples |
| §7 | Slope detection: counter on memory `EMA_short` increases; threshold → `slopePositive` |
| §14 | `github.com/DataDog/sketches-go/ddsketch`, protobuf marshal |

## Scope and non-goals

**In scope:** Merge incoming samples into existing status fields; deserialize sketch, `Add`, re-serialize; EMA recurrence; slope counter state (may live in status or derived each tick — see algorithms).

**Out of scope:** Percentile **interpretation** for limits (060), safety caps (070), kubelet I/O (030), Pod OOM timestamps (050).

## Dependencies

- **Upstream:** [010-workloadprofile-api.md](./010-workloadprofile-api.md)
- **Downstream:** [060-recommendation-engine.md](./060-recommendation-engine.md), [070-safety-layer.md](./070-safety-layer.md), [080-observe-reconcile.md](./080-observe-reconcile.md)

## Data model / API surface

**Package suggestion:** `internal/stats` or `pkg/aggregate`.

**Public functions:**

```go
// UpdateContainerStats merges samples into profile.Status for one container.
// cpu/memory samples in millicores / bytes; now is observation time.
func UpdateContainerStats(
    status *autosizev1.WorkloadProfileStatus,
    containerName string,
    cpuSampleMillicores float64,
    memSampleBytes float64,
    now metav1.Time,
    prevMemoryEMAShort float64, // for slope: pass previous before update
) error
```

**Internal:**

- `func updateEMA(prev float64, sample float64, alpha float64) float64`
- `func mergeSketch(encoded string, sample float64) (string, error)` — decode or new sketch, Add, encode
- `func updateSlopeCounter(prevEMAShort, newEMAShort float64, counter int, threshold int) (newCounter int, slopePositive bool)`

**Constants:**

- `alphaShort = 0.2`, `alphaLong = 0.05`
- `ddsketchRelativeAccuracy = 0.01`
- `slopeThresholdCycles = N` — **must match spec §7**; spec says “exceeds threshold” without N: **implement default N=5** and make configurable in 010 or controller flags (document in Open questions if spec amended).

## Algorithms and invariants

### EMA

`EMA_t = α * sample_t + (1-α) * EMA_(t-1)`.

**Cold start:** If prior EMA is unset (0 and never updated), **bootstrap** `EMA_short = EMA_long = sample` on first non-nil sample to avoid long ramp from zero (document in rationale; matches spec §12 cold start).

### DDSketch

- `sketch, err := ddsketch.NewDefaultDDSketch(0.01)`
- If `encoded != ""`, unmarshal proto → sketch; else new sketch.
- `sketch.Add(sampleValue)` for CPU and memory separately.
- Marshal: `sketch.ToProto().Marshal()` → base64.StdEncoding.

**Invariant:** Sketch string must round-trip; reject corrupt base64 with metric error and **preserve** previous sketch on decode failure (do not wipe history).

### Slope (memory)

- Maintain `slopeStreak` in status **or** recompute from last N values — **recommend** storing `memorySlopeStreak int32` in `MetricAggregate` extension field if not in spec YAML (requires 010 amendment) **or** store only `slopePositive` bool and internal streak in annotation — **prefer** minimal: keep counter in `status.containers[].stats.memory` as optional `slopeStreak` field added in 010.

**Logic:** if `newEMAShort > prevEMAShort` then `streak++` else `streak=0`. If `streak >= N` then `slopePositive=true`. Spec: block downsizing when true (070).

### LastUpdated

Set `cpu.lastUpdated` and `memory.lastUpdated` to `now` when respective sample applied.

## Failure modes and behavior

| Case | Behavior |
|------|----------|
| Decode sketch fails | Log warning; retain old sketch; still update EMA if desired or skip metric |
| NaN/negative sample | Reject sample for that metric |
| Missing container in status | Append new `ContainerStats` row matching template order |

## Security / RBAC

- None beyond API write permissions for status (100).

## Observability

- Counter: `autosize_sketch_decode_errors_total`
- Debug log on first bootstrap per container

## Test plan

- **Unit (table-driven):**
  - EMA: known α, 3-step hand-computed values.
  - Sketch: add values [1,2,3…], query P50/P95 via library, stable within accuracy bounds.
  - Round-trip: empty → add → encode → decode → add again.
  - Slope: monotonic increase triggers `slopePositive` after N; reset on dip.
- **Acceptance:** 10 containers × 2 metrics encoded size < budget from 010.
- **Property:** Idempotent apply of same sample twice is wrong — engine runs once per reconcile; document single-writer assumption.

## Rollout / migration

- Changing α or sketch accuracy invalidates historical comparability; bump `aggregateVersion` in status if needed (optional field).

## Open questions

- Spec §7 does not fix **N** for slope cycles — **propose N=5** default and add to spec in follow-up PR.
- Whether `slopeStreak` needs a first-class API field — if yes, extend 010 before implementation.
