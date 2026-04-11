# LLD-120: Global defaults ConfigMap

## Purpose

Provide **cluster- or namespace-scoped** conservative default CPU/memory requests and limits used by the admission webhook (110) when no `WorkloadProfile` exists or profile has no recommendations yet. Implements spec §11 “configurable global fallback” and Phase 2 roadmap.

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| §11 | Fallback global defaults configmap |
| §15 | Phase 2 — Admission Webhook + fallback |

## Scope and non-goals

**In scope:** ConfigMap schema, loader with reload interval, precedence: profile recommendations > defaults; validation of quantities.

**Out of scope:** Per-namespace profiles (already WorkloadProfile), controller reconcile defaults (could mirror — optional).

## Dependencies

- **Upstream:** [010-workloadprofile-api.md](./010-workloadprofile-api.md)
- **Downstream:** [110-admission-webhook.md](./110-admission-webhook.md)

## Data model / API surface

**Suggested ConfigMap:** `autosize-system/global-defaults`

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: autosize-global-defaults
  namespace: autosize-system
data:
  cpuRequest: "100m"
  cpuLimit: "500m"
  memoryRequest: "128Mi"
  memoryLimit: "512Mi"
```

**Optional per-container keys** or JSON blob for multi-arch — keep MVP flat keys.

**Controller/webhook flags:**

- `--defaults-configmap-namespace`
- `--defaults-configmap-name`
- `--defaults-reload-interval` (e.g. 1m)

## Algorithms and invariants

- Invalid quantity in ConfigMap → log error; **fail closed** to last good snapshot or deny inject (choose: **last good**).
- Hot reload must be **atomic** (read all keys then swap pointer).

## Failure modes and behavior

| Failure | Behavior |
|---------|----------|
| CM missing | No default inject; webhook no-op |
| Partial keys | Validate all required present |

## Security / RBAC

- Webhook SA: `get` on ConfigMap in system namespace.

## Observability

- Gauge: `autosize_defaults_loaded{valid=true|false}`

## Test plan

- **Unit:** Parse valid/invalid quantities.
- **Integration:** Change ConfigMap; webhook sees new defaults after reload.

## Rollout / migration

- Ship default CM with chart.

## Open questions

- Namespace-scoped overrides via second CM naming pattern — defer post-MVP.
