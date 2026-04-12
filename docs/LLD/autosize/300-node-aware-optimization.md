# LLD-300: Node-aware optimization

## Purpose

Phase 4 (spec §15): persist **bounded per-node DDSketches** for each template container, **merge** them for percentile queries where appropriate, and expose **read-only bin-packing hints** in `WorkloadProfile.status` for external schedulers and automation—without DRA (LLD-200).

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| §15 Phase 4 | Node-aware optimization |
| §6 | DDSketch mergeable; etcd-bounded status |
| §16 | DRA remains excluded from core scope |

## Scope and non-goals

**In scope:**

- Per-node sketch ingest (one sample per node per reconcile: mean usage of that container’s pods on that node).
- Merge via `(*ddsketch.DDSketch).MergeWith` on decoded sketches (`github.com/DataDog/sketches-go/ddsketch` v1.4.8+).
- Global CPU/memory EMA, CUSUM, Holt–Winters, quadrant sketches, and feedback continue to use the **workload-level mean** sample (same time series as pre–Phase 4) for continuity.
- Recommendations: quantiles from **merged per-node sketches** when at least one node entry exists; else global sketch (§6 / §8).
- Status: `containers[].stats.nodeSketches` (max **32** entries), `status.binPacking` hints (heterogeneity score + node count).

**Out of scope:**

- DRA / ResourceClaims / device classes (LLD-200).
- Mutating scheduler configuration or core scheduler plugins.
- Actuation that patches pod templates for hints (hints are observe-only unless a future flag explicitly adds it).

## Data model / API surface

**Types:** [`api/v1/workloadprofile_types.go`](../../api/v1/workloadprofile_types.go)

| Field | Meaning |
|-------|---------|
| `ContainerResourceStats.NodeSketches` | `[]NodeSketchEntry`, max 32. Each entry: `nodeName`, `lastSeen`, `cpuSketch`, `memSketch` (base64 DDSketch protobuf per metric). |
| `WorkloadProfileStatus.BinPacking` | Optional `BinPackingHints`: `heteroScore` (0–1), `nodeCount`, `observedAt`. |

**Eviction:** When a node no longer hosts any pod of the workload, its entry is removed. When exceeding 32 nodes (rare), drop the entry with oldest `lastSeen` before inserting a new node.

## Algorithms and invariants

### Per-node sample

From kubelet summaries keyed by `pod.Spec.NodeName`, for each template container name: average CPU (millicores) and memory (bytes) across pods on **that node** only. One pair per node per reconcile.

### Ingest

- **Global** CPU/memory sketches: `Add` the **workload mean** (mean of per-node values, or equivalently current `collectUsageForContainer` aggregate).
- **Per-node** sketches: for each node with pods, `Add` that node’s mean CPU/memory to that node’s pair of sketches.
- **Quadrant** sketches: still updated from the **workload mean** (same as global series) for time-of-day buckets.
- **CUSUM:** compares new sample to pre-update `emaLong` using **workload mean** only. On shift: clear global + quadrant + **all per-node** sketches for that resource path.

### Merge for recommendations

1. Decode each non-empty `cpuSketch` in `nodeSketches` for the container; `MergeWith` into a copy of the first non-empty (or fresh default sketch). Same for memory.
2. If merged sketch is non-empty (`GetCount() > 0`), use it for `recommend.Input` CPU/memory sketches (quadrant override logic unchanged: prefers quadrant when samples sufficient).
3. If no node entries, use global `stats.cpu.sketch` / `stats.memory.sketch` as today.

### Bin-packing hints (`status.binPacking`)

- `nodeCount`: distinct nodes with scheduled pods for this workload.
- `heteroScore`: if `nodeCount < 2`, `0`. Else: per-node **current** CPU millicore sample (same as ingest input), compute `(max - min) / max(mean, 1e-6)`, clamp to `[0, 1]`.
- `observedAt`: reconcile timestamp.

Deterministic given the same kubelet snapshots.

## Failure modes and behavior

- Corrupt per-node base64: treat as empty sketch for that metric; entry may be dropped on next successful update.
- Missing kubelet summary for a node: skip ingest for that node; existing entry ages until pruned when no pods remain on that node.

## Security / RBAC

Unchanged from LLD-030: `nodes/proxy` for kubelet stats. No additional API objects.

## Observability

Optional: log line when node sketch list is pruned or capped (debug).

## Test plan

- Unit: `collectUsagePerNode` multi-node fixtures; `MergeSketches` / merge from base64; eviction order.
- Controller tests: node sketch population and merged recommend path.

## Rollout / migration

New status fields are optional; existing `WorkloadProfile` objects gain empty `nodeSketches` and optional `binPacking` on next reconcile.

## Open questions (deferred)

- Finer heterogeneity (memory, CPU throttling per node).
- Integration with external scheduler plugins beyond JSON status consumption.

## Bin-packing spike (MVP decision)

**Chosen:** Read-only **`status.binPacking`** for operators and external automation. No core scheduler API dependency; no DRA. Pod-template annotation actuation remains out of scope unless explicitly added later behind a feature flag.
