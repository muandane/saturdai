# LLD-090: Actuation (workload PATCH)

## Purpose

Apply safe recommendations to the **Deployment** or **StatefulSet** pod template `resources` via strategic merge PATCH (or JSON patch), triggering rolling updates per spec §11. Never mutate Pods directly. Update `status.lastApplied` when patch succeeds and safety (070) allows. Integrates with the loop in spec §10.

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| §11 | PATCH apps/v1 deployment/statefulset; container resources in pod template; no direct pod mutation |
| §9 | `lastApplied` for cooldown |
| §16 | No pod mutation |

## Scope and non-goals

**In scope:** Build patch from diff between current template resources and desired recommendations; container name matching; idempotent no-op when already equal.

**Out of scope:** Webhook injection (110), HPA interaction (excluded by spec §2).

## Dependencies

- **Upstream:** [020-target-resolution.md](./020-target-resolution.md), [070-safety-layer.md](./070-safety-layer.md), [080-observe-reconcile.md](./080-observe-reconcile.md)
- **Downstream:** [110-admission-webhook.md](./110-admission-webhook.md) (expects stable resource shape)

## Data model / API surface

```go
func BuildResourcePatch(
    target *unstructured.Unstructured,
    recs []Recommendation,
) (*Patch, error)

func ApplyActuation(ctx context.Context, c client.Client, target client.Object, patch *Patch) error
```

**Matching:** Map `recs` by `containerName` to `spec.template.spec.containers[].name`.

**Quantities:** Serialize as canonical k8s quantity strings matching API expectations.

## Algorithms and invariants

1. Read live object **resourceVersion**.
2. If `ShouldPatch` from 070 is false → skip PATCH; still update status recommendations in 080 order.
3. If desired equals current for all autosized containers → skip PATCH; **do not** bump `lastApplied` (no application occurred).
4. On success: set `status.lastApplied = now` **after** API returns success.
5. **Ordering with status update:** Option A: patch then status in same reconcile; Option B: patch then immediate status — **prefer** patch first, then status update in one flow; if status update fails, next loop may retry patch (idempotent).

### StatefulSet specifics

- Same patch path; note **ordered rollout** behavior; document pod management policy if parallel (user responsibility).

## Failure modes and behavior

| Failure | Behavior |
|---------|----------|
| Conflict 409 | Requeue; refresh from API |
| Invalid quantity | Should not happen if 060/070 validated — return error, set condition |
| Admission webhook rejects pod template | Surface event; backoff |

## Security / RBAC

- `patch` on `deployments`, `statefulsets` (100).

## Observability

- Counter: `autosize_actuation_total{result=success|noop|error}`
- Log patch diff summary (reduced verbosity)

## Test plan

- **Integration:** envtest patch Deployment; assert new `resourceVersion` and template CPU request.
- **Unit:** BuildResourcePatch only touches named containers; preserves others.
- **Acceptance:** No-op when recommendations match live template.
- **e2e (kind):** Rolling update completes; pods get new limits.

## Rollout / migration

- Enable `AUTOSIZE_ACTUATION=true` after 080 soak.
- Consider **canary** annotation on workload to opt-in first.

## Open questions

- Opt-in label on workload vs profile-only — **recommend** profile presence implies opt-in; document blast radius.
