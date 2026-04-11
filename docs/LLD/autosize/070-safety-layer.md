# LLD-070: Safety layer

## Purpose

Apply spec §9 mechanisms to raw recommendations from 060: max **30% decrease** per cycle, unlimited increase, cooldown vs `lastApplied`, event overrides (OOM, throttling, restart spike), memory trend guard (`slopePositive`), and final **`rationale`** strings. Output is the **safe** recommendation list and a boolean `shouldPatch` / override flags for 080/090.

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| §9 | Change limits, cooldown, overrides table, trend guard, rationale |
| §5 | Throttling signal for override (>50%) |
| §6 | `slopePositive` on memory aggregate |
| §16 | Explainable rationale on every recommendation |

## Scope and non-goals

**In scope:** Pure functions `ApplySafety(profile, recs, podSignals, metrics, now) → SafeResult`.

**Out of scope:** Kubelet fetch (030), sketch updates (040), Kubernetes PATCH (090).

## Dependencies

- **Upstream:** [010-workloadprofile-api.md](./010-workloadprofile-api.md), [050-pod-signals.md](./050-pod-signals.md), [060-recommendation-engine.md](./060-recommendation-engine.md)
- **Downstream:** [080-observe-reconcile.md](./080-observe-reconcile.md), [090-actuation.md](./090-actuation.md)

## Data model / API surface

```go
type SafeResult struct {
    Recommendations []Recommendation // mutated copies
    ShouldPatch     bool
    BypassCooldown  bool // true when override applied
    PauseDownsizeCyclesRemaining int // restart spike
    SkipMemoryForContainer map[string]bool // slope guard
}

func ApplySafety(
    profile *autosizev1.WorkloadProfile,
    base []Recommendation,
    currentResources map[string]corev1.ResourceRequirements, // from template
    podSignals PodSignalSnapshot,
    throttlingRatio map[string]float64, // container -> throttled/cpu usage
    now time.Time,
) SafeResult
```

**State in status (optional):** `pauseDownsizeCycles` counter in annotation or status field — **recommend** small status extension `downsizePauseUntilCycle int32` or use controller-only memory with **annotation** on profile for simplicity in MVP.

## Algorithms and invariants

### Max decrease 30%

For each quantity (request/limit): if `new < current`, require `new >= current * 0.7` (floating compare in millis then round). If violated, **clamp** `new` to `ceil(current * 0.7)` and append a rationale fragment such as `; safety: decrease_step <axis> <before>-><after> (floor 70% of current <current>)` so operators can see why effective resources differ from sketch-only percentiles.

**Status fields:** `status.metricsRecommendations` reflects the engine output before this clamp; `status.recommendations` reflects the clamped/effective list used for PATCH and webhook injection.

**Increases:** no upper clamp beyond spec max*.

### Cooldown

- If `now - lastApplied < cooldownMinutes` and **not** override → `ShouldPatch=false` for non-override path; still update recommendations in status (spec §10 shows always writing recommendations).

**Clarify:** Spec shows patch only when `safeToApply`; recommendations always updated — **OK**.

### Overrides (immediate, bypass cooldown)

| Event | Detection | Action |
|-------|-----------|--------|
| OOMKilled | `lastOOMKill` within lookback (e.g. 10m) | `memLimit *= 1.5`, mark applied, rationale `override: OOMKill` |
| High CPU throttle | `throttledUsage / usage > 0.5` when usage > epsilon | `cpuLimit *= 1.25` |
| Restart spike | delta > 3 | set pause downsizing **2 reconcile cycles** |

**Ordering:** Compute base recs → apply overrides → apply 30% clamp to **non-override** changes? **Rule:** Overrides apply **after** base recs; clamp may still apply to prevent absurd values — document: OOM ×1.5 is **not** subject to 30% decrease cap (it's increase).

### Trend guard

- If `slopePositive` for container memory: **skip memory** recommendation (keep previous template memory or omit patch fields for memory — 090 merges). Rationale: `trend_guard: memory slope positive`.

### Rationale

- Join parts: `mode`, key percentiles, override tags, cooldown skip reason.
- Example: `"balanced: P70=230m cpu, P95=890m cpu, no OOM in window, cooldown satisfied"`.

## Failure modes and behavior

| Case | Behavior |
|------|----------|
| Missing `lastApplied` | Cooldown satisfied |
| Clock skew | Use controller clock; document trust model |

## Security / RBAC

- None.

## Observability

- Counter: `autosize_patch_blocked_total{reason=cooldown|throttle|...}`

## Test plan

- **Table tests:** cooldown boundary ±1s; 30% clamp from 1000m → proposed 600m → expect 700m.
- **Overrides:** OOM sets mem limit; throttle ratio 0.51 triggers CPU bump.
- **Restart spike:** sets pause flag; two reconciles decrement.
- **Slope:** memory fields stripped from patch intent.

## Rollout / migration

- Tunables (lookback window, throttle threshold) — start fixed per spec; later ConfigMap (120).

## Open questions

- Persist **pause downsize cycles** in status vs ephemeral — **recommend status field** `downsizePauseCyclesRemaining int32` added via 010 for crash safety.
