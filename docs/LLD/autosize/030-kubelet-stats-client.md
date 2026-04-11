# LLD-030: Kubelet stats client

## Purpose

Define how the controller fetches container-level CPU/memory (and throttling when present) from each node’s kubelet `GET /stats/summary`, maps stats to pods belonging to a target workload, and handles timeouts and partial failures. Implements spec §5 and supports §10 `collectKubeletMetrics`.

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| §5 | Source `/stats/summary`; signals: CPU millicores, memory working set, throttling; interval default 30s; no Metrics Server for history |
| §13 | RBAC: `nodes` + `nodes/proxy` (see [spec §13](../../spec/autosize-controller-spec.md#13-rbac-requirements)) |
| §12 | No kubelet access → requeue, surface CRD condition |

## Scope and non-goals

**In scope:** Fetching summary via API server **node proxy** (`GET /api/v1/nodes/{name}/proxy/stats/summary`) with bounded timeout, parsing summary JSON, extracting per-container stats for pods on that node. Direct TLS to kubelet is an alternative but not what the current implementation uses.

**Out of scope:** Persisting aggregates (040), reconcile orchestration (080), Pod OOM from API (050).

## Dependencies

- **Upstream:** [020-target-resolution.md](./020-target-resolution.md) (pods + nodes for target)
- **Downstream:** [080-observe-reconcile.md](./080-observe-reconcile.md)

## Data model / API surface

- **Input:** List of `corev1.Pod` (scheduled), possibly grouped by `pod.Spec.NodeName`.
- **Output:** `CollectedMetrics` struct: per `(namespace, podUID, containerName)` → `CPUMillicores`, `MemoryWorkingSetBytes`, `ThrottledNanoCores` (optional), `ObservedAt time.Time`.
- **Client interface:**

```go
type SummaryClient interface {
    FetchNodeSummary(ctx context.Context, nodeName string) (*stats.Summary, error)
}
```

Use `k8s.io/kubelet/pkg/apis/stats/v1alpha1` types or equivalent client-go path; align with cluster version.

**Node address:** Prefer `Node.Status.Addresses` (InternalIP); respect dual-stack policy.

## Algorithms and invariants

- **CPU millicores:** `usageNanoCores / 1e6` per spec table.
- **Memory:** `workingSetBytes` only (not RSS vs cache debate — spec picks working set).
- **Fan-out:** One summary fetch per distinct node hosting target pods; concurrent with bounded parallelism (configurable, e.g. 5).
- **Stale pod:** If pod not in summary, omit sample for that cycle (do not zero-out sketches).

## Failure modes and behavior

| Failure | Behavior |
|---------|----------|
| Single node timeout | Log; continue other nodes; partial metrics OK if any pod covered |
| All nodes fail | Return error; 080 sets `MetricsAvailable=False`; `RequeueAfter` backoff |
| TLS/cert errors | Same as network; surface reason in condition |

## Security / RBAC

- `nodes/stats` **get** (subject accesses kubelet proxy API).
- May require elevated permissions in hardening guides (100).

## Observability

- Histogram: `autosize_kubelet_request_duration_seconds` by node result.
- Log sample count per reconcile: pods expected vs pods with data.

## Test plan

- **Integration:** httptest server returning fixture `summary.json`; client parses expected millicores/memory.
- **Acceptance:** Missing `throttledUsageNanoCores` does not error.
- **Soak:** Bounded goroutines under 50+ node fan-out (scale test optional).

## Rollout / migration

- Insecure TLS skip for local dev only; production uses cluster CA (document in 100).

## Open questions

- Whether to use kubelet API via **API server proxy** (`/api/v1/nodes/{name}/proxy/stats/summary`) vs direct node IP — **prefer proxy** for uniform auth if supported by platform; document tradeoffs in implementation.
