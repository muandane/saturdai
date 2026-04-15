# LLD-090: Actuation (Pod resize subresource)

## Purpose

Apply safe recommendations to **running Pods** via the Pod `resize` subresource (`pods/resize`). This avoids parent workload template mutations and rollout-triggered restarts. Update `status.lastApplied` only when at least one Pod resize succeeds.

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| §11 | In-place Pod resource resize via `pods/resize` |
| §9 | `lastApplied` for cooldown |
| §16 | No direct mutation of parent workload templates for actuation |

## Scope and non-goals

**In scope:** Build per-Pod desired resources from recommendations; call Pod resize subresource; idempotent no-op when already equal.

**Out of scope:** Webhook injection (110), HPA interaction (excluded by spec §2).

## Dependencies

- **Upstream:** [020-target-resolution.md](./020-target-resolution.md), [070-safety-layer.md](./070-safety-layer.md), [080-observe-reconcile.md](./080-observe-reconcile.md)
- **Downstream:** [110-admission-webhook.md](./110-admission-webhook.md) (expects stable resource shape)

## Data model / API surface

```go
type Result struct {
    Resized int
    Noop    int
    Failed  int
}

func Apply(ctx context.Context, c client.Client, pods []corev1.Pod, recs []Recommendation, skipMemory map[string]bool) (Result, error)
```

**Matching:** Map `recs` by `containerName` to `pod.spec.containers[].name`.

**Quantities:** Serialize as canonical k8s quantity strings matching API expectations.

## Algorithms and invariants

1. If `ShouldPatch` from 070 is false → skip actuation; still update status recommendations.
2. For each selected Pod, compute desired container resources from recommendation map.
3. If desired equals current Pod resources → count as noop.
4. If changed, call `SubResource(\"resize\").Update(...)` for that Pod.
5. Aggregate result: `{resized, noop, failed}`.
6. Set `status.lastApplied` only when `resized > 0`.

## Failure modes and behavior

| Failure | Behavior |
|---------|----------|
| Pod resize rejected/deferred | Record `ActuationApplied=False` with reason `PartialFailure`; requeue with bounded backoff |
| Invalid quantity | Should not happen if 060/070 validated — return error, set condition |
| No pod resource diffs | `ActuationApplied=True` with reason `Noop`; do not bump `lastApplied` |

## Security / RBAC

- `patch`/`update` on `pods/resize`.
- `get`/`list`/`watch` on `pods`.

## Observability

- Counter: `autosize_actuation_total{result=success|noop|error}`
- Condition: `ActuationApplied` with `Applied|Noop|PartialFailure`

## Test plan

- **Unit:** Pod resource patching by container name, skip-memory behavior, noop detection, partial-failure aggregation.
- **Integration:** resize path updates Pod resources without parent template mutation.
- **Acceptance:** No parent rollout is triggered by actuation.
- **e2e (kind):** Pod resources change in-place and restart count remains stable.

## Rollout / migration

- Enable `AUTOSIZE_ACTUATION=true` after 080 soak.
- Consider **canary** annotation on workload to opt-in first.

## Open questions

- Opt-in label on workload vs profile-only — **recommend** profile presence implies opt-in; document blast radius.
