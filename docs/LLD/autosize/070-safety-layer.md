# LLD-070: Safety layer

## Purpose

Apply spec §9 mechanisms to raw recommendations from 060: max **10% decrease** per cycle (90% floor), unlimited increase, cooldown vs `lastApplied`, event overrides (OOM, throttling, restart spike), memory trend guard (`slopePositive`), and final **`rationale`** strings. Output is the **safe** recommendation list and a boolean `shouldPatch` / override flags for 080/090.

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| §9 | Change limits, cooldown, overrides table, trend guard, rationale |
| §5 | Throttling signal for override (>50%) |
| §6 | `slopePositive` on memory aggregate |
| §16 | Explainable rationale on every recommendation |

## Scope and non-goals

**In scope:** Pure function `Apply` in [`internal/safety`](../../../internal/safety) (same responsibilities as `ApplySafety` below).

**Out of scope:** Kubelet fetch (030), sketch updates (040), Kubernetes PATCH (090).

## Dependencies

- **Upstream:** [010-workloadprofile-api.md](./010-workloadprofile-api.md), [050-pod-signals.md](./050-pod-signals.md), [060-recommendation-engine.md](./060-recommendation-engine.md)
- **Downstream:** [080-observe-reconcile.md](./080-observe-reconcile.md), [090-actuation.md](./090-actuation.md)

## Data model / API surface

```go
type Result struct {
    Recommendations []Recommendation
    ShouldPatch     bool
    SkipMemory      map[string]bool // slope guard
}

func Apply(
    profile *autosizev1.WorkloadProfile,
    base []Recommendation,
    current map[string]corev1.ResourceRequirements,
    sig *podsignals.Snapshot,
    now time.Time,
    blockDownsize bool,
) Result
```

- **Throttle ratios** live on `sig.ThrottleRatios` (kubelet-derived in reconcile).
- **`blockDownsize`:** `true` when `status.downsizePauseCyclesRemaining > 0` or when any container’s restart delta indicates a spike this reconcile (computed in reconcile; see 050). When `true`, decrease clamps become **hold at current template** instead of the 90% floor; rationale includes `safety: pause_downsize`.

**State in status:** `downsizePauseCyclesRemaining int32` on `WorkloadProfile` — reconciler sets it to `4` after a spike (delta > 3 with baseline) and decrements each reconcile; persisted for crash safety ([`restart_pause.go`](../../../internal/controller/restart_pause.go)).

## Algorithms and invariants

### Max decrease 10%

For each quantity (request/limit): if `new < current`, require `new >= current * 0.9` (floating compare in millis then round). If violated, **clamp** `new` to `ceil(current * 0.9)` and append a rationale fragment such as `; safety: decrease_step <axis> <before>-><after> (floor 90% of current <current>)` so operators can see why effective resources differ from sketch-only percentiles.

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
| Restart spike | delta > 3 | set pause downsizing **4 reconcile cycles** |

**Ordering:** Trend guard → OOM override → throttle override → decrease clamps (90% floor or pause hold). Overrides apply **before** the clamp pass so memory/CPU **increases** from OOM/throttle still apply; only downward moves are limited by clamp or pause.

### Trend guard

- If `slopePositive` for container memory: **omit memory from actuation** (Pod resize leaves memory unchanged for that container — 090); **do not** apply decrease clamps to memory for that container. Engine values may still appear in `status.metricsRecommendations` / `status.recommendations` with rationale `trend_guard: memory slope positive` (spec §9).

**Status vs live resources:** `status.recommendations` memory for that container can differ from the currently applied Pod memory (for example engine-proposed downsize while trend guard freezes memory) — intentional traceability, not “strip from status.”

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

- **Table tests:** cooldown boundary ±1s; 10% clamp from 1000m → proposed 600m → expect 900m.
- **Overrides:** OOM sets mem limit; throttle ratio 0.51 triggers CPU bump.
- **Restart spike:** sets pause flag; two reconciles decrement.
- **Slope:** memory omitted from PATCH (`SkipMemory`); template memory unchanged.

## Rollout / migration

- Tunables (lookback window, throttle threshold) — start fixed per spec; later ConfigMap (120).

## Open questions

- (Resolved) Pause cycles are persisted as **`status.downsizePauseCyclesRemaining`** (see 010).
