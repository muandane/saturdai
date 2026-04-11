# LLD-050: Pod signals (OOM and restarts)

## Purpose

Define how the controller reads `Pod` API state for containers belonging to the target workload: detect `OOMKilled`, track `restartCount` deltas between reconciles, and expose fields used by the safety layer (070) for overrides. Implements spec §5 table (OOM/restart rows) and §9 event overrides inputs.

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| §5 | OOM from `lastState.terminated.reason`; restart count delta between reconciles |
| §9 | OOM → mem limit ×1.5; restart spike delta > 3 → pause downsizing 2 cycles |
| §12 | Memory leak / OOM handling ties to `slopePositive` (040) and overrides |

## Scope and non-goals

**In scope:** List/watch pods by workload selector; per-container last OOM timestamp; restart count snapshot storage in status or ephemeral reconcile state.

**Out of scope:** Applying overrides (070), kubelet metrics (030).

## Dependencies

- **Upstream:** [020-target-resolution.md](./020-target-resolution.md)
- **Downstream:** [070-safety-layer.md](./070-safety-layer.md), [080-observe-reconcile.md](./080-observe-reconcile.md)

## Data model / API surface

- **Inputs:** Pods matching target selector, previous `restartCount` from `status.containers[].restartCount` (spec §4) or dedicated map.
- **Outputs:**
  - `lastOOMKill *metav1.Time` per container — update when termination reason `OOMKilled` observed with recent timestamp.
  - `restartCount int32` — **snapshot** of max or sum policy: **recommend max** across pods for container name (multiple replicas).

**Restart delta:** `delta = currentMaxRestart - status.restartCount`; persist `currentMaxRestart` into status each cycle.

**Function sketch:**

```go
type PodSignalSnapshot struct {
    LastOOMKill    map[string]*metav1.Time // containerName -> time
    RestartCount   map[string]int32
}

func CollectPodSignals(pods []corev1.Pod, prevRestart map[string]int32) (PodSignalSnapshot, map[string]int32 /* delta by container */)
```

## Algorithms and invariants

- Consider only pods **owned** by target (ownerReferences match deployment/statefulset UID) if selector is broad.
- OOM time: use `lastState.terminated.finishedAt` when reason is `OOMKilled`.
- If multiple pods OOM same container name, use **most recent** `finishedAt`.

## Failure modes and behavior

| Case | Behavior |
|------|----------|
| Pod list RBAC denied | Error to 080; condition |
| Pod terminating | Still read status for terminal OOM |
| Missing container status | Skip that container |

## Security / RBAC

- `pods` get, list, watch in namespace (100).

## Observability

- Counter: `autosize_oom_detected_total{container=...}` (low cardinality: aggregate or omit label)
- Log at info when OOM timestamp updates

## Test plan

- **Unit:** Pod fixtures with `lastState.terminated` OOM vs `Running`.
- **Unit:** Restart delta: prev 2, now 6 → delta 4.
- **Integration:** envtest pod with simulated status (may require status subresource patch in test).

## Rollout / migration

- Field additions to status already in 010; no migration if using existing `lastOOMKill` and `restartCount`.

## Open questions

- Whether restart spike compares **per-pod** delta or **aggregated** — spec implies delta on a single counter; **recommend per-container max across pods** for simplicity.
