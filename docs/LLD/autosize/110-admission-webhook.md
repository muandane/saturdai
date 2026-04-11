# LLD-110: Admission webhook (cold-start injection)

## Purpose

Mutating admission webhook on **Pod create** that injects `resources` into `pod.spec.containers[]` when the pod belongs to a workload covered by a `WorkloadProfile`: prefer **last known recommendations** from profile status; otherwise fall back to global defaults (120). Implements spec §11 “Admission Webhook (cold-start)” and supports §12 cold start.

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| §11 | Mutating webhook on Pod create; inject from profile or conservative defaults |
| §2 | Webhook in scope; no direct pod mutation by controller |
| §12 | Cold start: webhook injects defaults; EMA bootstraps later |

## Scope and non-goals

**In scope:** Webhook server TLS, `MutatingWebhookConfiguration`, object selector or namespace selector, request decoding, patch JSON for resources only on matching containers, timeout budget (< 10s default).

**Out of scope:** Controller reconcile (080), cert-manager vs cert-rotation implementation detail (document choice in 100).

## Dependencies

- **Upstream:** [010-workloadprofile-api.md](./010-workloadprofile-api.md), [090-actuation.md](./090-actuation.md) (stable shape of recommendations), [120-global-defaults-configmap.md](./120-global-defaults-configmap.md)
- **Downstream:** Operations runbooks

## Data model / API surface

- **Input:** `AdmissionReview` with `Pod`.
- **Pod → workload:** Resolve via **ownerReferences** chain to ReplicaSet → Deployment or StatefulSet (handle Job/CronJob as **no inject** unless extended).
- **Lookup:** `WorkloadProfile` in same namespace where `targetRef` matches owning Deployment/StatefulSet name/kind.
- **Patch:** JSON patch add/replace `resources.requests` and `resources.limits` **only if** policy allows override (e.g. empty requests).

**Override policy (must document):**

- If user set non-zero requests/limits → **do not overwrite** (conservative) **or** opt-in annotation `autosize.io/inject: force` — pick one; **recommend** do-not-overwrite if any resource field set on container.

## Algorithms and invariants

- Idempotent: same webhook call twice should not happen; replay safe.
- Failure policy: `Ignore` vs `Fail` — **recommend Fail** in prod for guaranteed inject when profile exists; **Ignore** for soft rollout — product choice per environment.

## Failure modes and behavior

| Failure | Behavior |
|---------|----------|
| Profile not found | Apply 120 defaults if enabled; else no-op |
| API timeout listing profile | Timeout → Fail or Ignore per policy |
| Container name mismatch | Skip unknown names |

## Security / RBAC

- Webhook service account needs **get/list** `workloadprofiles` in target namespaces.
- TLS cert trust bundle in `MutatingWebhookConfiguration`.

## Observability

- Log webhook latency; metric `autosize_webhook_inject_total{result}`.

## Test plan

- **Integration:** envtest with webhook test harness (controller-runtime webhook package).
- **Acceptance:** Pod created with injected resources matches status recommendations.

## Rollout / migration

- Roll out with `failurePolicy=Ignore` first, then tighten.

## Open questions

- Exact annotation for force-inject and opt-out (`autosize.io/webhook: disabled`).
