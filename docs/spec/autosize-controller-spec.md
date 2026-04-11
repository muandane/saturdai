# Kubernetes Deterministic Autosizing Controller — Final Spec

## 1. Objective

A Kubernetes controller that dynamically adjusts CPU and memory requests/limits for workloads using deterministic, fully explainable algorithms. No Prometheus. No ML. No external dependencies.

**Comparable to CAST AI behavior — without the black box or SaaS dependency.**

Key differences from existing tools:

- **Goldilocks**: recommend-only, VPA-dependent, no actuation
- **CAST AI**: closed-source, ML-based, cloud-dependent
- **This**: self-contained actuation, no VPA, no Prometheus, deterministic + auditable

---

## 2. Scope

### Included

- Deployments, StatefulSets
- Pods (indirect via controllers)
- Native Kubernetes metrics APIs (kubelet `/stats/summary`, not Metrics Server for history)
- Mutating admission webhook (cold-start defaults)

### Excluded

- Horizontal Pod Autoscaling
- Prometheus / external observability
- ML or probabilistic models
- Direct Pod mutation
- DRA (Phase 2+)

---

## 3. Architecture

```
┌─────────────────────────────────────────────┐
│              Kubebuilder Controller          │
│                                             │
│  ┌─────────────┐    ┌────────────────────┐  │
│  │   Reconciler │    │  Admission Webhook  │  │
│  │  (main loop) │    │  (cold-start inject)│  │
│  └──────┬──────┘    └────────────────────┘  │
│         │                                   │
│  ┌──────▼──────────────────────────────┐    │
│  │           Metrics Collector          │    │
│  │  kubelet /stats/summary (per node)   │    │
│  │  Pod status (OOMKill, restarts)      │    │
│  └──────┬──────────────────────────────┘    │
│         │                                   │
│  ┌──────▼──────────────────────────────┐    │
│  │         Stats Engine                 │    │
│  │  EMA (short + long)                  │    │
│  │  DDSketch (percentiles)              │    │
│  └──────┬──────────────────────────────┘    │
│         │                                   │
│  ┌──────▼──────────────────────────────┐    │
│  │     WorkloadProfile CRD (etcd)       │    │
│  │  Aggregates only — no raw samples    │    │
│  └─────────────────────────────────────┘    │
└─────────────────────────────────────────────┘
```

---

## 4. Custom Resource Definition

**Canonical API group (this repository):** `autosize.saturdai.auto/v1`. Earlier drafts used the placeholder group `autosize.io`; all copy-paste YAML and RBAC must use the group from [`PROJECT`](../../PROJECT) / generated CRDs.

```yaml
apiVersion: autosize.saturdai.auto/v1
kind: WorkloadProfile
metadata:
  name: my-app
  namespace: production
spec:
  targetRef:
    kind: Deployment       # Deployment | StatefulSet
    name: my-app
  mode: balanced           # cost | balanced | resilience | burst
  containers:
    - name: app            # optional per-container overrides
      minCPU: "50m"
      maxCPU: "4000m"
      minMemory: "64Mi"
      maxMemory: "8Gi"
  cooldownMinutes: 15      # default: 15
status:
  containers:
    - name: app
      stats:
        cpu:
          emaShort: 0.0
          emaLong: 0.0
          sketch: ""       # base64 DDSketch
          lastUpdated: ""
        memory:
          emaShort: 0.0
          emaLong: 0.0
          sketch: ""
          lastUpdated: ""
          slopePositive: false   # monotonic increase detection
        lastOOMKill: null
        restartCount: 0
  metricsRecommendations:
    - containerName: app
      cpuRequest: "180m"
      cpuLimit: "750m"
      memoryRequest: "240Mi"
      memoryLimit: "480Mi"
      rationale: "balanced: P70/P95 cpu & mem, mode=balanced"
  recommendations:
    - containerName: app
      cpuRequest: "200m"
      cpuLimit: "800m"
      memoryRequest: "256Mi"
      memoryLimit: "512Mi"
      rationale: "balanced: P70/P95, no OOM events; safety: decrease_step …"
  lastApplied: ""
  lastEvaluated: ""
```

---

## 5. Metrics Collection

### Source: kubelet `/stats/summary`

**Do not rely on `metrics.k8s.io` (Metrics Server) for history** — it provides only instantaneous snapshots with no throttling data and no retention.

Access per node:

```
GET https://<node-ip>:10250/stats/summary
```

Requires `nodes/stats` RBAC permission.

### Signals Per Container

| Signal | Source | Notes |
|---|---|---|
| CPU usage (millicores) | kubelet summary | `usageNanoCores / 1e6` |
| Memory usage (bytes) | kubelet summary | `workingSetBytes` |
| CPU throttling | kubelet summary | `throttledUsageNanoCores` — not always populated |
| OOMKilled | Pod `containerStatuses[].lastState.terminated.reason` | Watch for `"OOMKilled"` |
| Restart count | Pod `containerStatuses[].restartCount` | Delta between reconciles |

### Collection Interval

- Default: every 30s
- Configurable per profile

---

## 6. State Storage: CRD-Only, No Raw Samples

**Problem**: 24h of 30s samples = 2880 samples/container. Storing raw samples hits etcd's ~1MB object limit.

**Solution**: Online algorithms — store only fixed-size aggregates in CRD status.

### EMA State (trivial, ~16 bytes per metric)

```go
type EMAState struct {
    Short float64 `json:"short"` // α = 0.2 — reacts fast
    Long  float64 `json:"long"`  // α = 0.05 — smooths noise
}
```

Update formula:

```
EMA_t = α * sample_t + (1 - α) * EMA_(t-1)
```

### Percentile State: DDSketch

Library: `github.com/DataDog/sketches-go/ddsketch`

- Bounded memory (~1KB per metric at 1% relative accuracy)
- Any percentile on demand (P40, P50, P70, P90, P95, P99)
- Serializes to protobuf → base64 in CRD status
- Mergeable (useful later for node-level aggregation)

```go
import "github.com/DataDog/sketches-go/ddsketch"

sketch, _ := ddsketch.NewDefaultDDSketch(0.01) // 1% relative accuracy
sketch.Add(sampleValue)

// Serialize for CRD storage
bytes, _ := sketch.ToProto().Marshal()
encoded := base64.StdEncoding.EncodeToString(bytes)
```

### Full CRD State Per Container

```go
type ContainerStats struct {
    CPU    MetricAggregate `json:"cpu"`
    Memory MetricAggregate `json:"memory"`
    LastOOMKill  *metav1.Time `json:"lastOOMKill,omitempty"`
    RestartCount int32        `json:"restartCount"`
}

type MetricAggregate struct {
    EMAShort    float64      `json:"emaShort"`
    EMALong     float64      `json:"emaLong"`
    Sketch      string       `json:"sketch"` // base64 DDSketch proto
    SlopePos    bool         `json:"slopePositive,omitempty"` // memory only
    LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`
}
```

**Estimated CRD size**: 10 containers × 2 metrics × ~1KB = ~20KB. Well within etcd limits.

---

## 7. Statistical Model

### Prediction

```
baseline   = EMA_long
burst      = EMA_short - EMA_long
prediction = baseline + k * burst
```

`k` is tuned per profile (see Section 8).

### Slope Detection (memory only)

Track whether memory EMA is monotonically increasing across the last N reconcile cycles. Implemented as a simple counter: if `EMA_short_t > EMA_short_(t-1)` increment, else reset. If counter exceeds threshold → `slopePositive = true` → block downsizing.

---

## 8. Profiles

All values derived from DDSketch percentile queries.

### cost

```
cpuRequest  = P50
cpuLimit    = P90
memRequest  = P50
memLimit    = P90
k           = 0.5
```

### balanced (default)

```
cpuRequest  = P70
cpuLimit    = P95
memRequest  = P70
memLimit    = P95
k           = 1.0
```

### resilience

```
cpuRequest  = P90
cpuLimit    = P99 * 1.1
memRequest  = P90
memLimit    = P99 * 1.2
k           = 1.5
```

### burst

```
cpuRequest  = P40
cpuLimit    = max(P99, peak_observed)
memRequest  = P40
memLimit    = peak_observed
k           = 2.0
```

---

## 9. Safety Mechanisms

### Change Limits

- Max decrease per cycle: **30%**
- Max increase: unrestricted (fast recovery from under-provisioning)
- Hard floor from `spec.containers[].min*` values

### Cooldown

- Default: 15 minutes between patch applications
- Tracked via `status.lastApplied`

### Event Overrides (immediate, bypass cooldown)

| Event | Action |
|---|---|
| OOMKilled | `memLimit *= 1.5`, mark applied |
| High CPU throttling (>50%) | `cpuLimit += 25%` |
| Restart spike (delta > 3) | Pause downsizing for 2 cycles |

### Trend Guard

- `slopePositive == true` → skip memory recommendation entirely

### Rationale Field

Every recommendation in `status.recommendations[].rationale` must be a human-readable string explaining the decision. **`status.recommendations`** holds values **after** safety rules (including the 30% decrease floor). Sketch-only outputs are mirrored on **`status.metricsRecommendations`** (pre-safety) when present so operators can compare engine output to effective values.

```
"balanced: P70=230m cpu, P95=890m cpu, no OOM in 24h, cooldown satisfied"
"override: OOMKill detected 3m ago, memory x1.5 applied"
"; safety: decrease_step cpu_request 98m->70m (floor 70% of current 100m)"
```

---

## 10. Reconciliation Loop

Pseudocode; production code lives under `internal/controller/` and imports `github.com/muandane/saturdai/api/v1` as `autosizev1`.

```go
func (r *WorkloadProfileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    profile := &autosizev1.WorkloadProfile{}
    if err := r.Get(ctx, req.NamespacedName, profile); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    target, err := r.resolveTarget(ctx, profile)
    if err != nil {
        return ctrl.Result{}, err
    }

    metrics, err := r.collectKubeletMetrics(ctx, target)
    if err != nil {
        return ctrl.Result{RequeueAfter: 30 * time.Second}, err
    }

    r.updateAggregates(profile, metrics)    // EMA + DDSketch update
    recs := r.computeRecommendations(profile) // percentile queries + profile

    if r.safeToApply(profile, recs) {
        if err := r.patchTarget(ctx, target, recs); err != nil {
            return ctrl.Result{}, err
        }
        profile.Status.LastApplied = &metav1.Time{Time: time.Now()}
    }

    profile.Status.Recommendations = recs
    profile.Status.LastEvaluated = &metav1.Time{Time: time.Now()}

    if err := r.Status().Update(ctx, profile); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}
```

---

## 11. Mutation Strategy

### Apply via

- `PATCH /apis/apps/v1/namespaces/{ns}/deployments/{name}` — container resources in pod template
- Triggers rolling update automatically

### Never

- Mutate Pods directly — controllers own Pods

### Admission Webhook (cold-start)

- Mutating webhook on Pod create
- If `WorkloadProfile` exists for the owning controller → inject last known recommendation
- If no profile exists → inject conservative defaults (configurable global fallback)

```go
// Webhook injects into pod.spec.containers[].resources
// before the pod is scheduled — eliminates the gap between
// profile creation and first reconcile cycle
```

---

## 12. Edge Cases

| Case | Handling |
|---|---|
| Cold start (no data) | Webhook injects defaults; EMA bootstraps from first sample |
| Noisy workloads | Use `resilience` profile (P95+) |
| CronJobs | Use `burst` profile; detect idle periods via near-zero EMA |
| Memory leak | `slopePositive` detection blocks downsizing |
| Controller restart | Full state in CRD status — no in-memory loss |
| No kubelet access | Requeue with backoff, surface condition on CRD |

---

## 13. RBAC Requirements

Minimum logical rules below. The **implemented** ClusterRole is generated from `+kubebuilder:rbac` markers — see [`config/rbac/role.yaml`](../../config/rbac/role.yaml).

Kubelet metrics are read via the API server **node proxy** (`GET /api/v1/nodes/{name}/proxy/stats/summary`), which requires `nodes` **get/list/watch** and `nodes/proxy` **get** (not only `nodes/stats`).

```yaml
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["nodes/proxy"]
  verbs: ["get"]
- apiGroups: ["apps"]
  resources: ["deployments", "statefulsets"]
  verbs: ["get", "list", "watch", "patch"]
- apiGroups: ["autosize.saturdai.auto"]
  resources: ["workloadprofiles", "workloadprofiles/status"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
```

---

## 14. Key Dependencies

| Dependency | Purpose |
|---|---|
| `sigs.k8s.io/controller-runtime` | Controller scaffolding (via Kubebuilder) |
| `github.com/DataDog/sketches-go/ddsketch` | Online percentile computation |
| `k8s.io/client-go` | Kubelet stats API access |
| `google.golang.org/protobuf` | DDSketch serialization |

No Prometheus. No VPA. No external store.

---

## 15. Roadmap

### Phase 1 — Core (MVP)

- WorkloadProfile CRD + controller
- Kubelet metrics collection
- EMA + DDSketch stats engine
- Profile-driven recommendations (all 4 modes)
- Safety mechanisms
- Deployment/StatefulSet patching

### Phase 2 — Admission Webhook

- Cold-start default injection
- Fallback global defaults configmap

### Phase 3 — DRA Integration

- ResourceClass tiers
- Controller-managed ResourceClaims
- Late binding for better bin-packing
- *(DRA API status: beta in k8s 1.30+, production readiness TBD)*

### Phase 4 — Node-Aware Optimization

- Aggregate DDSketches across nodes
- Bin-packing hints for scheduler

### Phase 5 — Time-Based Patterns

- Hour-of-day bucketing (24 sketch slots per container)
- Automatic burst/off-peak profile switching

**Low-level designs:** Per-subsystem engineering contracts (traceability, APIs, test plans) live under [`docs/LLD/autosize/`](../LLD/autosize/README.md), indexed by phase and dependency order.

---

## 16. Constraints (Non-Negotiable)

- No Prometheus dependency
- No ML or probabilistic models
- All decisions deterministic and explainable via `status.recommendations[].rationale`
- Must not destabilize workloads (safety mechanisms always active)
- No raw sample storage — online algorithms only
- No direct Pod mutation
