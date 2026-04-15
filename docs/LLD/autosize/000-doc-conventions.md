# Autosize LLD — Documentation conventions

This directory holds **Low-Level Design (LLD)** documents for the deterministic autosizing controller. The authoritative requirements live in [autosize-controller-spec.md](../../spec/autosize-controller-spec.md).

## Hierarchy

| Layer | Path | Role |
|-------|------|------|
| Spec | `docs/spec/autosize-controller-spec.md` | What and why (requirements, roadmap) |
| LLD | `docs/LLD/autosize/*.md` | Engineering contract per subsystem |
| Code | `api/`, `internal/`, etc. | Implementation |

If an LLD contradicts the spec, **update the spec first**, then the LLD.

## Canonical API identity (this repository)

| Item | Value |
|------|--------|
| API group | `autosize.saturdai.auto` |
| Version | `v1` |
| Kind | `WorkloadProfile` |
| Resource (URL) | `workloadprofiles` |
| Go types | [`api/v1`](../../../api/v1) |
| CRD manifest | [`config/crd/bases`](../../../config/crd/bases) |
| Sample | [`config/samples/autosize_v1_workloadprofile.yaml`](../../../config/samples/autosize_v1_workloadprofile.yaml) |

The high-level product name in prose may say *autosize*; the **Kubernetes API group** is always as in [`PROJECT`](../../../PROJECT).

## File naming

- `NNN-short-name.md` — three-digit prefix is **merge order** (dependencies flow downward in [README.md](./README.md)).
- Insert new docs with a free slot (e.g. `085-...`) without renumbering the whole tree when possible.

## Status values (README index)

| Status | Meaning |
|--------|---------|
| `draft` | Structure present; not reviewed |
| `reviewed` | Traceability + test plan approved; open questions empty or deferred with spike ID |
| `implemented` | Code merged; README points to packages |

## Mandatory sections (every LLD)

Use these **exact** H2 headings so reviews stay mechanical:

1. **Purpose**
2. **Spec traceability**
3. **Scope and non-goals**
4. **Dependencies**
5. **Data model / API surface**
6. **Algorithms and invariants**
7. **Failure modes and behavior**
8. **Security / RBAC**
9. **Observability**
10. **Test plan**
11. **Rollout / migration**
12. **Open questions**

Optional: short mermaid sequence diagram when control flow is non-obvious.

## Copy-paste template

```markdown
# LLD-NNN: Title

## Purpose

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| | |

## Scope and non-goals

## Dependencies

## Data model / API surface

## Algorithms and invariants

## Failure modes and behavior

## Security / RBAC

## Observability

## Test plan

## Rollout / migration

## Open questions
```

## Glossary

| Term | Definition |
|------|------------|
| **WorkloadProfile** | CRD binding a `targetRef` (Deployment or StatefulSet) to autosize policy and status aggregates. |
| **Mode** | One of `cost`, `balanced`, `resilience`, `burst` — selects percentile targets and prediction gain `k`. |
| **EMA** | Exponential moving average; short α=0.2, long α=0.05 per spec. |
| **DDSketch** | DataDog sketch — bounded-memory quantile structure; serialized as base64 protobuf in status. |
| **Observe-only reconcile** | Reconciler path that updates `WorkloadProfile` status but does not PATCH the workload (LLD-080 milestone). |
| **Actuation** | In-place Pod resource resize via `pods/resize` (LLD-090). |
| **Cold start** | No prior aggregates or recommendations; webhook / defaults apply until data exists. |

## Review checklist (before marking `reviewed`)

- [ ] Every in-scope requirement maps to a spec § in **Spec traceability**.
- [ ] Every **must** in this LLD has a **Test plan** bullet (unit / integration / e2e).
- **Open questions** is empty, or each item has an owner and spike issue ID.
- [ ] No contradiction with [autosize-controller-spec.md](../../spec/autosize-controller-spec.md) §16 constraints.

## Implementation PR convention

Reference the LLD path in the PR description, for example:

`LLD: docs/LLD/autosize/040-aggregate-engine.md`

Update [README.md](./README.md) status to `implemented` when the slice is done.

## Tracking issues

The [README](./README.md) **Issue** column holds the GitHub (or other) issue/epic ID for scheduling. Replace `—` when filed.
