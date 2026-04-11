# saturdai

Kubernetes operator that implements a **deterministic, explainable** CPU/memory right-sizing loop for Deployments and StatefulSets: kubelet metrics (no Prometheus), EMA + DDSketch aggregates in `WorkloadProfile` status, mode-based recommendations, safety rules, and optional actuation via workload PATCH.

**Docs:** [Controller spec](docs/spec/autosize-controller-spec.md) · [LLD index](docs/LLD/autosize/README.md) · [Implementation status](docs/implementation-status.md)

**API group:** `autosize.saturdai.auto/v1` · kind: `WorkloadProfile`

## Description

The reconciler resolves `spec.targetRef` to a workload, lists pods, pulls kubelet `/stats/summary` through the API server node proxy, updates per-container aggregates and recommendations in status, and optionally patches the workload when **`AUTOSIZE_ACTUATION`** is set to `true` or `1` (default is **observe-only**). Pod **restart counts** (max per container across replicas) are stored in status; when the delta since the last reconcile exceeds a threshold after a baseline exists, the safety layer **pauses downsizing** for two cycles (`status.downsizePauseCyclesRemaining`). See [LLD-050](docs/LLD/autosize/050-pod-signals.md) / [LLD-070](docs/LLD/autosize/070-safety-layer.md).

## Getting Started

### Prerequisites

- Go (see `go.mod`)
- Docker (for image build)
- `kubectl` aligned with your cluster
- A Kubernetes cluster
- [cert-manager](https://cert-manager.io/docs/installation/) (required for mutating webhook TLS in the default `config/default` bundle)

### Build and test (local)

```sh
make build
make test
```

After API changes: `make manifests generate`

### Deploy on a cluster

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

The default kustomize bundle enables the **Pod mutating admission webhook** and a **cert-manager** `Certificate` for webhook TLS. Ensure cert-manager is installed first; the webhook uses `failurePolicy: Ignore` so scheduling still works if the webhook is unavailable.

**Apply global defaults (optional, used as fallback when no `WorkloadProfile` recommendation applies):**

```sh
kubectl apply -f config/samples/autosize_global_defaults_configmap.yaml
```

The manager is configured with `--defaults-configmap-namespace=saturdai-system` and `--defaults-configmap-name=autosize-global-defaults` (see [`config/manager/manager.yaml`](config/manager/manager.yaml)).

**Apply a sample `WorkloadProfile`:**

```sh
kubectl apply -k config/samples/
```

Samples use `apiVersion: autosize.saturdai.auto/v1`.

### Actuation

By default the controller **does not** PATCH workloads. To enable actuation:

```sh
# In the manager Deployment, set:
env:
  - name: AUTOSIZE_ACTUATION
    value: "true"
```

Soak with observe-only first; then enable in non-production.

### Uninstall

```sh
kubectl delete -k config/samples/
make uninstall
make undeploy
```

## Project distribution

### Installer bundle

```sh
make build-installer IMG=<registry>/saturdai:tag
```

Produces consolidated YAML under `dist/` (see Makefile help).

### Helm (optional)

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

## Contributing

See [Kubebuilder book](https://book.kubebuilder.io/introduction.html). Run `make help` for targets.

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
