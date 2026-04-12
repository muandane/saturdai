# saturdai

Kubernetes operator for **deterministic, explainable** CPU/memory right-sizing on **Deployments** and **StatefulSets**: kubelet `/stats/summary` (no Prometheus), EMA + DDSketch aggregates, four recommendation modes, safety rules, optional workload PATCH, and a mutating webhook for cold-start defaults.

---

## Start here (5 minutes)

| Step | What to do |
|------|------------|
| 1 | Read **[Controller spec](docs/spec/autosize-controller-spec.md)** §2–4 for scope and API shape. |
| 2 | Skim **[Implementation status](docs/implementation-status.md)** for what is shipped vs planned. |
| 3 | Clone, `make build`, `make test` (see [Build and test](#build-and-test-local)). |
| 4 | Install CRDs: `make install`. Deploy controller: `make deploy IMG=...` (see [Deploy](#deploy-on-a-cluster)). |
| 5 | Apply samples: `kubectl apply -k config/samples/` — then inspect CRs (see [Samples](#samples-yaml)). |

**API group:** `autosize.saturdai.auto/v1`

---

## What this project is

- **Observe path:** For each reconciled workload, the controller lists pods, fetches kubelet stats per node, updates bounded aggregates and recommendations in **`WorkloadProfile.status`**, and persists heavier learned state in **`ConfigMap mlstate-<name>`** (same namespace).
- **Actuation path:** Optional; off by default. Set **`AUTOSIZE_ACTUATION=true`** on the manager to PATCH pod template resources.
- **Admission:** Mutating webhook on Pod create can inject recommendations or global defaults (`failurePolicy: Ignore` by default install).

Deep dive: [LLD index](docs/LLD/autosize/README.md) · [Kubebuilder layout](AGENTS.md) (this repo follows upstream Kubebuilder conventions).

---

## Custom resources (3 kinds)

| Kind | Scope | Use when |
|------|--------|----------|
| **WorkloadProfile** | Namespaced | One **named** `Deployment` or `StatefulSet` (`spec.targetRef`). This is where metrics, recommendations, and `mlstate-*` attach. |
| **NamespaceProfile** | Namespaced | Select many workloads **in the same namespace** via `spec.workloadSelector` (labels and/or `selectAll`). Creates **child** `WorkloadProfile` objects (fan-out). |
| **ClusterProfile** | Cluster | Select workloads across namespaces: `spec.namespaceSelector` + `spec.workloadSelector`. Creates **child** `WorkloadProfile` objects in each namespace. |

**Fan-out:** Parent profiles do not run the full kubelet loop themselves; children carry `spec.targetRef` so the existing **`WorkloadProfile`** reconciler and pod webhook keep a single code path. Overlap with another profile managing the same workload is **denied** (see `SelectorConflict` in parent status). Details: [LLD-085](docs/LLD/autosize/085-bulk-target-selection.md), spec §4.

---

## Documentation map

| Doc | Purpose |
|-----|---------|
| [docs/spec/autosize-controller-spec.md](docs/spec/autosize-controller-spec.md) | Normative behavior, API examples, reconcile pseudocode |
| [docs/implementation-status.md](docs/implementation-status.md) | Feature checklist vs spec / LLDs |
| [docs/LLD/autosize/README.md](docs/LLD/autosize/README.md) | Design index and dependency graph |
| [Kubebuilder CONTRIBUTING](https://github.com/kubernetes-sigs/kubebuilder/blob/master/CONTRIBUTING.md) | Upstream contribution norms (this repo may add its own `CONTRIBUTING.md`) |
| [AGENTS.md](AGENTS.md) | Kubebuilder-oriented agent notes for this repository |

---

## Samples (`config/samples/`)

Applied together via `kubectl apply -k config/samples/` ([kustomization](config/samples/kustomization.yaml)).

| File | Purpose |
|------|---------|
| [autosize_v1_workloadprofile.yaml](config/samples/autosize_v1_workloadprofile.yaml) | Single workload by name (`targetRef`) |
| [autosize_v1_namespaceprofile.yaml](config/samples/autosize_v1_namespaceprofile.yaml) | Namespace-scoped label / select-all / kinds examples |
| [autosize_v1_clusterprofile.yaml](config/samples/autosize_v1_clusterprofile.yaml) | Cluster-wide namespace + workload selection |
| [sample-deployment.yaml](config/samples/sample-deployment.yaml) | Example workload to point a profile at |
| [autosize_global_defaults_configmap.yaml](config/samples/autosize_global_defaults_configmap.yaml) | Fallback defaults for the webhook |

Edit namespaces and labels to match your cluster before relying on them in production.

---

## Prerequisites

- Go (version in [`go.mod`](go.mod))
- Docker (for image build)
- `kubectl` matching your cluster
- A Kubernetes cluster
- [cert-manager](https://cert-manager.io/docs/installation/) (default `config/default` bundle uses it for webhook TLS)

---

## Build and test (local)

```sh
make build
make test
```

After API or marker changes:

```sh
make manifests generate
```

Formatting / lint: `make lint-fix` or `make lint` (see `Makefile`).

---

## Deploy on a cluster

**Build and push the manager image:**

```sh
make docker-build docker-push IMG=<registry>/saturdai:tag
```

**Install CRDs:**

```sh
make install
```

**Deploy the controller:**

```sh
make deploy IMG=<registry>/saturdai:tag
```

The default kustomize bundle enables the **Pod mutating admission webhook** and a **cert-manager** `Certificate` for webhook TLS. Install cert-manager first. The webhook uses `failurePolicy: Ignore` so scheduling continues if the webhook is unavailable.

**Optional — global defaults ConfigMap** (webhook fallback when no profile applies):

```sh
kubectl apply -f config/samples/autosize_global_defaults_configmap.yaml
```

Wire the manager with `--defaults-configmap-namespace` and `--defaults-configmap-name` (see [`config/manager/manager.yaml`](config/manager/manager.yaml)).

**Apply samples:**

```sh
kubectl apply -k config/samples/
```

---

## Actuation (off by default)

The controller **does not** PATCH workloads unless you enable it:

```yaml
env:
  - name: AUTOSIZE_ACTUATION
    value: "true"
```

Run observe-only first; enable in non-production before production.

---

## Uninstall

```sh
kubectl delete -k config/samples/
make uninstall
make undeploy
```

---

## Project distribution

**Installer bundle:**

```sh
make build-installer IMG=<registry>/saturdai:tag
```

Output under `dist/` (see `make help`).

**Helm (optional):** `kubebuilder edit --plugins=helm/v2-alpha`

---

## Contributing

See [Kubebuilder book](https://book.kubebuilder.io/introduction.html) and upstream [contributing](https://github.com/kubernetes-sigs/kubebuilder/blob/master/CONTRIBUTING.md). Run `make help` for targets; follow `make test` / `make lint` before opening a PR when available in your environment.

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
