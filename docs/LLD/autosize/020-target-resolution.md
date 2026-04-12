# LLD-020: Target resolution

## Purpose

Specify how a `WorkloadProfile` resolves to a live `Deployment` or `StatefulSet`, how missing or mismatched targets are surfaced, and when reconciliation should no-op or clean up. Implements the `resolveTarget` step in spec §10.

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| §4 | `targetRef.kind` and `name` in same namespace as profile |
| §10 | `resolveTarget` before metrics collection |
| §12 | Edge cases: target gone, controller restart (state in CRD) |
| §16 | No direct Pod mutation — resolution targets parent workload only |

## Scope and non-goals

**In scope:** Get/List workload by ref, owner namespace rules, conditions on `WorkloadProfile`, selector/pod template identity for metrics correlation.

**Out of scope:** Kubelet calls (030), patching (090), webhook (110).

## Dependencies

- **Upstream:** [010-workloadprofile-api.md](./010-workloadprofile-api.md)
- **Related (future):** [085-bulk-target-selection.md](./085-bulk-target-selection.md) — extends resolution to selector-based and cluster-wide targets
- **Downstream:** 030, 080, 090

## Data model / API surface

- **Inputs:** `WorkloadProfile` object, client for `apps/v1` Deployments and StatefulSets.
- **Outputs:** Resolved object (typed), pod list selector from `.spec.selector`, template container names for alignment with `spec.containers[]` overrides.
- **Conditions (recommended):**
  - `TargetResolved` (True/False): reason `NotFound`, `KindNotSupported`, `Ambiguous`.
  - `PodsReady` optional for gating metrics quality (future).

**Functions (conceptual):**

- `ResolveTarget(ctx, profile) (*appsv1.Deployment | *appsv1.StatefulSet, error)`
- `SelectorForTarget(target) labels.Selector`
- `TemplateContainerNames(podTemplateSpec) []string` — must match metrics and recommendation keys.

## Algorithms and invariants

- Namespace: always `profile.Namespace`; `targetRef` has no cross-namespace field.
- Unsupported kind: fail fast with clear error; do not patch.
- If target deleted: set condition, stop actuation; optionally remove finalizer policy (product decision) — **default:** keep profile, surface `TargetResolved=False`.

## Failure modes and behavior

| Condition | Behavior |
|-----------|----------|
| 404 Not Found | Condition false; requeue with backoff; no kubelet traffic |
| RBAC denied | Error; requeue; condition with `Forbidden` |
| Selector missing/invalid | Treat as permanent failure for metrics mapping |

## Security / RBAC

- `get`, `list`, `watch` on `deployments`, `statefulsets` in profile namespace (100).

## Observability

- Structured log: `workloadProfile`, `targetKind`, `targetName`, `resolvedUID`.
- Metric counter: `autosize_target_resolve_total{result=success|not_found|error}`.

## Test plan

- **Integration (envtest):** Profile + Deployment in namespace → resolve succeeds; delete Deployment → condition updates.
- **Unit:** Kind switch, nil selector edge cases.
- **Acceptance:** StatefulSet path mirrors Deployment.

## Rollout / migration

- Adding new target kinds requires API change (010) + this LLD update.

## Open questions

- Whether to auto-create or require pre-existing workload — **spec assumes existing workload**.
