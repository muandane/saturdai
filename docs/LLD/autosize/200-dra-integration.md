# LLD-200: DRA integration (stub)

## Purpose

Placeholder for Dynamic Resource Allocation integration per spec §15 Phase 3: ResourceClass tiers, controller-managed ResourceClaims, late binding. **Not part of MVP.** Expand when target Kubernetes versions and DRA API stability are confirmed.

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| §15 Phase 3 | DRA integration bullets |
| §2 | DRA excluded from core scope (Phase 2+) |

## Scope and non-goals

**In scope (future):** Feature-gated integration that does not break non-DRA clusters.

**Out of scope (now):** Concrete APIs, CRD changes, RBAC, tests — all TBD below.

## Dependencies

- **Upstream:** [090-actuation.md](./090-actuation.md), [100-packaging-rbac.md](./100-packaging-rbac.md)
- **Downstream:** TBD design doc

## Data model / API surface

TBD — likely new fields on `WorkloadProfile` or a separate CRD for claims.

## Algorithms and invariants

TBD — must remain deterministic; no ML (spec §16).

## Failure modes and behavior

TBD

## Security / RBAC

TBD — additional rules for `resource.k8s.io` API group.

## Observability

TBD

## Test plan

TBD — kind with DRA feature gates when available.

## Rollout / migration

TBD — feature gate default off.

## Open questions

- Minimum Kubernetes version for DRA in production
- Interaction between CPU/mem patch and device claims
- Whether autosize owns claims or only suggests
