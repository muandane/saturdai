# saturdai

Kubernetes operator that recommends CPU and memory requests and limits for Deployments and StatefulSets, using kubelet stats (via the apiserver node proxy) and storing bounded aggregates in custom resource status. No Prometheus dependency and no external ML service—rules and rationales are deterministic and auditable.

**Why it exists:** explainable recommendations (with human-readable rationales), safety caps and cooldowns, observe-only by default, optional mutating webhook for defaults on new pods, and flexible targeting from one workload up through namespace- and cluster-scoped policies.

Built with [Kubebuilder](https://book.kubebuilder.io/) and controller-runtime.

## Requirements

- Go — see [`go.mod`](go.mod)
- Docker — for image builds
- `kubectl` and a cluster
- [cert-manager](https://cert-manager.io/docs/installation/) — default webhook TLS install

## Install and run

1. Build and run tests (optional):

   ```sh
   make build
   make test
   ```

2. Install CRDs:

   ```sh
   make install
   ```

3. Build, push, and deploy the manager:

   ```sh
   make docker-build docker-push IMG=<registry>/saturdai:tag
   make deploy IMG=<registry>/saturdai:tag
   ```

   The webhook uses `failurePolicy: Ignore` so temporary webhook downtime does not block pod creation.

4. Apply samples (edit namespaces and labels first):

   ```sh
   kubectl apply -k config/samples/
   ```

5. Inspect objects in API group **`autosize.saturdai.auto/v1`** (`WorkloadProfile`, `NamespaceProfile`, `ClusterProfile`). Recommendations and metrics appear on **`WorkloadProfile.status`**.

Optional global defaults for pods with no matching profile: [`config/samples/autosize_global_defaults_configmap.yaml`](config/samples/autosize_global_defaults_configmap.yaml). Align manager flags with [`config/manager/manager.yaml`](config/manager/manager.yaml).

## How it works

1. Resolve the target workload, list its pods, fetch **per-node kubelet `/stats/summary`** through the node proxy, and update rolling aggregates (EMA, DDSketch percentiles) in status.
2. Run the recommendation engine in one of four modes (e.g. cost, balanced, resilience, burst). Apply a safety layer: decrease floors, cooldown vs last apply, OOM and trend guards, restart-spike pause.
3. Persist auxiliary learned state in a ConfigMap per profile (`mlstate-<name>`), owner-referenced to the `WorkloadProfile`.
4. **Actuation** (in-place resize via the `pods/resize` subresource) stays **off** unless the manager sets **`AUTOSIZE_ACTUATION=true`**.
5. Export actuation counters on the controller metrics endpoint, for example:
   - `autosize_actuation_total{result=success|noop|error}`
   - `autosize_actuation_pod_resize_reason_total{reason=...}`

Deeper behavior, CRD field semantics, and design notes live under [`docs/`](docs/).

## Custom resources

| Kind | Scope | Use |
| --- | --- | --- |
| **WorkloadProfile** | Namespace | One Deployment or StatefulSet by name. Holds metrics, recommendations, and mlstate link. |
| **NamespaceProfile** | Namespace | Select workloads in that namespace; operator creates child **WorkloadProfile** objects. |
| **ClusterProfile** | Cluster | Namespace + workload selection cluster-wide; children are **WorkloadProfile** resources. |

If two policies claim the same workload, that workload is not doubly managed—conflicts surface on parent status.

## Samples

| File | Purpose |
| --- | --- |
| [`autosize_v1_workloadprofile.yaml`](config/samples/autosize_v1_workloadprofile.yaml) | Single workload |
| [`autosize_v1_namespaceprofile.yaml`](config/samples/autosize_v1_namespaceprofile.yaml) | Namespace selection |
| [`autosize_v1_clusterprofile.yaml`](config/samples/autosize_v1_clusterprofile.yaml) | Cluster selection |
| [`sample-deployment.yaml`](config/samples/sample-deployment.yaml) | Example workload to attach |
| [`autosize_global_defaults_configmap.yaml`](config/samples/autosize_global_defaults_configmap.yaml) | Webhook defaults |

## Configuration

**Actuation** — allow in-place pod resize:

```yaml
env:
  - name: AUTOSIZE_ACTUATION
    value: "true"
```

Roll out gradually (staging before production).

**Installer bundle:**

```sh
make build-installer IMG=<registry>/saturdai:tag
```

Output under [`dist/`](dist/). Helm assets may live under `dist/chart/` depending on your release process.

## Uninstall

```sh
kubectl delete -k config/samples/
make uninstall
make undeploy
```

## Development

After API or kubebuilder marker changes:

```sh
make manifests generate
```

See `make help`. Run `make test` before submitting changes.

## Contributing

Follow standard Kubebuilder workflows ([book](https://book.kubebuilder.io/introduction.html)). Run `make test`.

## License

Apache License 2.0. See <https://www.apache.org/licenses/LICENSE-2.0>.

Copyright 2026.
