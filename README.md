# saturdai

A Kubernetes operator that **recommends and optionally applies** CPU and memory requests/limits for **Deployments** and **StatefulSets**. It uses **kubelet stats** (via the API server node proxy), keeps **bounded aggregates** in custom resource status, and applies **deterministic rules** you can audit—no Prometheus dependency, no external ML service, and no opaque scoring.

The controller is built with **Kubebuilder** and **controller-runtime**, following the same patterns as many production Kubernetes operators.

---

## Why use it

- **Explainable output** — Recommendations include human-readable rationales; safety rules cap risky changes.
- **Observe first** — By default the operator **does not** change workloads; you can run it until you trust the numbers, then enable patching.
- **Cold start covered** — An optional mutating webhook can inject defaults or last-known recommendations on new pods.
- **Flexible targeting** — Point at a single workload, everything matching labels in a namespace, or a cluster-wide policy—see [Custom resources](#custom-resources) below.

---

## Quick start

1. **Clone and verify the build**

   ```sh
   make build
   make test
   ```

2. **Install CRDs** (from repo root, with a working kubeconfig):

   ```sh
   make install
   ```

3. **Build and deploy the manager** (set your registry and tag):

   ```sh
   make docker-build docker-push IMG=<registry>/saturdai:tag
   make deploy IMG=<registry>/saturdai:tag
   ```

   The default install expects **[cert-manager](https://cert-manager.io/docs/installation/)** for webhook TLS. The webhook is configured with `failurePolicy: Ignore` so pod creation is not blocked if the webhook is temporarily unavailable.

4. **Apply the sample bundle** (edit namespaces/labels to match your cluster first):

   ```sh
   kubectl apply -k config/samples/
   ```

5. **Inspect resources** — Look for `WorkloadProfile` and related objects in the API group `autosize.saturdai.auto/v1`. Sample manifests live under [`config/samples/`](config/samples/).

For global default resources used when no profile matches a pod, apply [`config/samples/autosize_global_defaults_configmap.yaml`](config/samples/autosize_global_defaults_configmap.yaml) and align the manager flags with [`config/manager/manager.yaml`](config/manager/manager.yaml).

---

## How it works (short)

1. For each managed workload, the controller finds pods, reads **per-node kubelet summary** stats, and updates **rolling aggregates** (EMA, DDSketch-backed percentiles) in status.
2. A **recommendation engine** applies one of four modes (for example cost vs resilience). A **safety layer** enforces floors, cooldowns, and guards around restarts and memory trends.
3. **Learned state** that does not belong in etcd-heavy status is stored in a **ConfigMap** per profile (`mlstate-<name>`), owner-referenced for cleanup.
4. **Actuation** (in-place Pod resize via `pods/resize`) is **disabled unless** you set `AUTOSIZE_ACTUATION=true` on the manager Deployment—use observe-only until you are ready.

---

## Custom resources

| Kind | Scope | When to use it |
|------|--------|----------------|
| **WorkloadProfile** | Namespace | One **named** `Deployment` or `StatefulSet` in that namespace. Holds metrics, recommendations, and links to `mlstate-*`. |
| **NamespaceProfile** | Namespace | Match many workloads **in that namespace** (labels or explicit select-all). The operator creates **child** `WorkloadProfile` objects so each workload still has a single profile to reconcile. |
| **ClusterProfile** | Cluster | Same idea across namespaces: namespace labels + workload selection. Again, children are normal `WorkloadProfile` resources. |

If two policies would claim the same workload, the overlapping case is **not** applied for that workload; parent objects surface that in status so you can fix the configuration.

---

## Samples

| File | What it shows |
|------|----------------|
| [`autosize_v1_workloadprofile.yaml`](config/samples/autosize_v1_workloadprofile.yaml) | Single workload by name |
| [`autosize_v1_namespaceprofile.yaml`](config/samples/autosize_v1_namespaceprofile.yaml) | Namespace-scoped selection examples |
| [`autosize_v1_clusterprofile.yaml`](config/samples/autosize_v1_clusterprofile.yaml) | Cluster-wide selection examples |
| [`sample-deployment.yaml`](config/samples/sample-deployment.yaml) | Example workload to attach a profile to |
| [`autosize_global_defaults_configmap.yaml`](config/samples/autosize_global_defaults_configmap.yaml) | Webhook fallback defaults |

---

## Prerequisites

- Go (see [`go.mod`](go.mod))
- Docker (for image builds)
- `kubectl` and a reachable cluster
- cert-manager (for the default webhook install)

---

## Development

After changing API types or kubebuilder markers:

```sh
make manifests generate
```

Other useful targets: `make help`, `make lint`, `make lint-fix` (if configured in your environment).

---

## Actuation

To allow the controller to resize running Pods in place:

```yaml
env:
  - name: AUTOSIZE_ACTUATION
    value: "true"
```

Roll this out gradually (staging before production).

---

## Uninstall

```sh
kubectl delete -k config/samples/
make uninstall
make undeploy
```

---

## Packaging

**Installer YAML:**

```sh
make build-installer IMG=<registry>/saturdai:tag
```

Artifacts under `dist/`. Optional Helm: `kubebuilder edit --plugins=helm/v2-alpha`.

---

## More documentation

Extra reference material (behavioral notes, design write-ups) lives under [`docs/`](docs/) for when you need depth beyond this page.

---

## Contributing

This project uses standard **Kubebuilder** workflows. See the [Kubebuilder book](https://book.kubebuilder.io/introduction.html). Run tests before submitting changes; `make test` runs the suite defined in the `Makefile`.

---

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
