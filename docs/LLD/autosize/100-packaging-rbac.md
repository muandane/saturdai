# LLD-100: Packaging and RBAC

## Purpose

Deliver deployable operator artifacts: Deployment/Helm/Kustomize, ServiceAccount, **complete RBAC** from spec §13, leader election, metrics endpoint, and cross-cutting **observability conventions** (conditions vocabulary, log keys). This LLD is the integration umbrella for running 080/090 in-cluster.

## Spec traceability

| Spec § | Requirement (summary) |
|--------|------------------------|
| §13 | Full rules: pods, nodes/stats, apps workloads, workloadprofiles |
| §14 | controller-runtime, client-go, ddsketch deps |
| §3 | Architecture: reconciler + metrics + CRD |

## Scope and non-goals

**In scope:** ClusterRole/Binding, namespace-scoped vs cluster-scoped choices, kube-rbac-proxy optional, webhook TLS job (stub for 110), Prometheus **scrape optional** without Prometheus dependency (exposes metrics handler only).

**Out of scope:** Webhook rules detail (110), DRA (200).

## Dependencies

- **Upstream:** [010-workloadprofile-api.md](./010-workloadprofile-api.md) (CRD install), all runtime LLDs
- **Downstream:** CI, release

## Data model / API surface

**RBAC (exact verbs from spec §13):**

| API group | Resources | Verbs |
|-----------|-----------|-------|
| `""` | `pods` | get, list, watch |
| `""` | `nodes` | get, list, watch |
| `""` | `nodes/proxy` | get |
| `apps` | `deployments`, `statefulsets` | get, list, watch, patch |
| `autosize.saturdai.auto` | `workloadprofiles`, `workloadprofiles/status` | get, list, watch, create, update, patch |

**Additional (typical):**

- `coordination.k8s.io` `leases` for leader election: get, create, update, patch
- `""` `events` record if used: create, patch
- Kubelet `/stats/summary` is accessed through **nodes/proxy** (see spec §13 and [`config/rbac/role.yaml`](../../../config/rbac/role.yaml)).

**Controller flags:**

- `--metrics-bind-address`
- `--health-probe-bind-address`
- `--leader-elect`
- Actuation is gated by env **`AUTOSIZE_ACTUATION`** (`true` / `1` enables workload PATCH); there is no separate `--actuation-enabled` flag in the current binary.

## Algorithms and invariants

- Single replica with leader elect **or** multiple with leader elect; document HPA not for controller.
- Image non-root, read-only root FS where possible.

## Failure modes and behavior

- RBAC misconfig → OAuth forbidden in logs; readiness fails if cannot list CRD (optional startup check).

## Security / RBAC

- Principle of least privilege review before GA.
- Node-scoped access (`nodes`, `nodes/proxy`) is cluster-scoped — document cluster-wide requirement for the controller ServiceAccount.

## Observability

**Standard conditions (WorkloadProfile):**

| Type | True meaning |
|------|----------------|
| `TargetResolved` | Workload exists |
| `MetricsAvailable` | At least one sample this cycle or within 2× interval |
| `ActuationReady` | Actuation enabled and last patch succeeded |

**Log keys:** `controller`, `workloadProfile`, `namespace`, `name`, `reconcileID` (use controller-runtime values).

**Metrics:** reconcile total, errors, queue depth if available.

## Test plan

- **Manifests:** kubeconform or `kubectl apply --dry-run=server` in CI.
- **Integration:** envtest with aggregated fake rules matching Role.
- **Acceptance:** Fresh cluster: install CRD + RBAC + controller; create profile; no Forbidden in logs.

## Rollout / migration

- Helm chart version bumps with app version.
- OLM bundle future optional.

## Open questions

- None blocking; spec §13 now documents `nodes` and `nodes/proxy` alongside the CRD group.
