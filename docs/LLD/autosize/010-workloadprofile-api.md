# LLD-010: WorkloadProfile API

## Purpose

Define the `WorkloadProfile` CRD (`autosize.saturdai.auto/v1`): field semantics, validation, defaulting, status subresource shape, and etcd size constraints so downstream LLDs share one stable API contract. Grounded in spec §4 and §6.

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| §4 | `targetRef` (Deployment \| StatefulSet), `mode`, optional per-container min/max, `cooldownMinutes`, status with containers stats, recommendations, `lastApplied` / `lastEvaluated`, `downsizePauseCyclesRemaining` (restart-spike pause) |
| §6 | Status holds aggregates only (EMA, base64 sketch, OOM/restart fields); no raw samples; estimated size budget |
| §16 | Deterministic, explainable via `recommendations[].rationale`; no raw sample storage |

## Scope and non-goals

**In scope:** API types, kubebuilder markers (`+kubebuilder:validation`, defaulting webhook or CRD defaults), status-only fields for stats and recommendations, printer columns optional.

**Out of scope:** Reconcile logic (080), kubelet client (030), recommendation math (060), safety rules (070), RBAC manifests (100).

## Dependencies

- **Upstream:** [000-doc-conventions.md](./000-doc-conventions.md), [autosize-controller-spec.md](../../spec/autosize-controller-spec.md)
- **Downstream:** 020, 040, 060, 070, 080, 090, 100, 110

## Data model / API surface

- **Group/version:** `autosize.saturdai.auto/v1`, kind `WorkloadProfile` (see repo [`PROJECT`](../../../PROJECT)).
- **Spec fields:**
  - `targetRef`: `kind` enum `Deployment|StatefulSet`, `name` string (required).
  - `mode`: enum `cost|balanced|resilience|burst`; default `balanced`.
  - `containers[]`: optional; `name` matches pod container name; `minCPU`, `maxCPU`, `minMemory`, `maxMemory` as `resource.Quantity` or string with validation.
  - `cooldownMinutes`: optional int32; default 15.
  - `collectionIntervalSeconds`: optional; default 30 (spec §5) — if omitted from spec YAML, add here for implementability.
- **Status fields:** mirror spec §4 YAML; Go structs align with §6 `ContainerStats` / `MetricAggregate`; `recommendations[]` includes `containerName`, four quantities, `rationale` string; `downsizePauseCyclesRemaining` counts reconcile cycles where downsizing is suppressed after a restart spike (070).
- **Subresources:** `status: {}` enabled; no scale subresource.

**Validation (minimum):**

- `targetRef.name` non-empty; `kind` only allowed values.
- Min ≤ max for each resource pair when both set.
- `cooldownMinutes` ≥ 0; `collectionIntervalSeconds` in bounded range (e.g. 10–300).

## Algorithms and invariants

- Status object must remain **well below** etcd 1Mi practical limit; target **< 100Ki** per profile with documented max containers (e.g. 20) enforced by validation webhook if needed.
- `lastUpdated` fields use `metav1.Time` (RFC3339 in JSON).

## Failure modes and behavior

- Invalid spec: set `WorkloadProfile` condition `Accepted=False` with reason (validation); reconciler skips actuation (020/080).
- Status update conflict: standard optimistic retry; no special API fields.

## Security / RBAC

- API server enforces auth; controller service account needs `workloadprofiles` + `workloadprofiles/status` (full matrix in 100).

## Observability

- Optional: `kubectl describe` shows conditions; no metrics in this LLD.

## Test plan

- **Unit:** CRD schema defaulting/validation via envtest or `api/v1` conversion tests.
- **Acceptance:** Applying invalid `WorkloadProfile` is rejected; valid minimal object persists; status patch allowed with subresource.
- **Regression:** Golden YAML fixtures for sample WorkloadProfile.

## Rollout / migration

- v1alpha1 only if needed for rapid iteration; prefer `v1` with strict validation. Bump with conversion webhook if fields rename.

## Open questions

- Whether `collectionIntervalSeconds` belongs in spec (recommended) vs controller ConfigMap only — **recommend spec field** for per-workload tuning per §5.
