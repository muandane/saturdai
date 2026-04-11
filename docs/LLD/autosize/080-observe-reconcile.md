# LLD-080: Observe reconcile (no actuation)

## Purpose

Wire the **read-only** reconciliation path: load profile, resolve target (020), list pods, collect kubelet metrics (030), merge aggregates (040), collect pod signals (050), compute recommendations (060), apply safety (070), **update `WorkloadProfile` status only** — no workload PATCH. This milestone validates data plane correctness before actuation (090). Implements spec §10 with `patchTarget` disabled or feature-flagged off.

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| §10 | Full loop except optional patch; `RequeueAfter` 30s; status update |
| §12 | Controller restart — state in CRD |
| §5 | Collection interval default 30s |

## Scope and non-goals

**In scope:** Controller-runtime `Reconcile`, owner references, rate limiting, feature flag `ACTUATION_ENABLED=false`, conditions (`MetricsAvailable`, `TargetResolved`, `ProfileReady`).

**Out of scope:** Deployment PATCH (090), webhook (110).

## Dependencies

- **Upstream:** 020, 030, 040, 050, 060, 070
- **Downstream:** [090-actuation.md](./090-actuation.md)

## Data model / API surface

- **Reconciler struct** holds clients: `client.Client`, `SummaryClient`, scheme.
- **Requeue:** `RequeueAfter: profile.Spec.CollectionInterval` or 30s default.
- **Feature flag:** env `AUTOSIZE_ACTUATION=false` (default false until 090 complete).

**Pseudo flow:**

1. Get WorkloadProfile
2. Resolve target — fail → status condition + requeue
3. List pods for selector
4. `collectKubeletMetrics` — partial OK
5. For each template container: `UpdateContainerStats` with samples
6. Pod signal snapshot (`MergePodStatus` per pod) → OOM times + max `restartCount` per container; reconcile persists restarts and computes spike deltas vs prior status (050); updates `downsizePauseCyclesRemaining` (070)
7. Decode sketches, `ComputeRecommendation` per container
8. `Apply` (safety) with `blockDownsize` from pause counter / spike; `ShouldPatch` ignored for patch but use for metrics/logging
9. `Status().Update` recommendations, `lastEvaluated`, aggregates

## Algorithms and invariants

- **Single writer** per profile: leader-elected manager (100).
- **Ordering:** aggregates updated before recommendations read sketches that include **this cycle’s** samples (same reconcile).
- **Idempotency:** Same observed metrics twice → status converges (sketch additive).

## Failure modes and behavior

| Failure | Behavior |
|---------|----------|
| Kubelet partial | Update what we have; condition `MetricsAvailable=Partial` |
| Status conflict | Retry with backoff |
| Target missing | Clear recommendations optional; keep last stats or mark stale — **recommend** keep last with condition |

## Security / RBAC

- As 100.

## Observability

- Reconcile duration histogram per phase label.
- Log errors with profile NS/name.

## Test plan

- **Integration (envtest):** Deployment + pods + fake summary server → profile status shows non-zero EMA after N loops.
- **Acceptance:** With actuation off, Deployment template resources unchanged across 10 reconciles.
- **Unit:** Reconcile returns `RequeueAfter` within bounds.

## Rollout / migration

- Ship 080 to production with actuation off for soak.

## Open questions

- Whether to expose **dry-run** field on CRD — optional `spec.dryRun: true` instead of global flag.
