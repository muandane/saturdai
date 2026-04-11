# WorkloadProfile dashboard

The manager can serve a read-only HTML dashboard backed by cluster data (`WorkloadProfile` + ML state ConfigMaps).

## Enable

Binary flags (default: off):

- `--dashboard-enabled` — start the HTTP server
- `--dashboard-bind-address` — listen address (default `:8090`)

Example local run:

```bash
go run ./cmd/main.go --dashboard-enabled=true --dashboard-bind-address=127.0.0.1:8090
```

## URLs

- UI: `http://<addr>/dashboard/` (open `index.html` or the directory root)
- API: `GET http://<addr>/api/dashboard/v1/profiles` — JSON for all namespaces

## Kubernetes

An optional JSON6902 patch is provided at [config/default/manager_dashboard_patch.yaml](../config/default/manager_dashboard_patch.yaml). Add it to `config/default/kustomization.yaml` under `patches` (with `target: kind: Deployment`) alongside your metrics and webhook patches so args and ports accumulate correctly.

Access is usually via port-forward (no TLS on the dashboard port):

```bash
kubectl -n saturdai-system port-forward deploy/saturdai-controller-manager 8090:8090
```

Then open `http://127.0.0.1:8090/dashboard/`.

## Security

The dashboard uses **plain HTTP** and the controller’s **ServiceAccount** (same RBAC as reconciliation). Do not expose port `8090` on a public Service without an authenticated reverse proxy or network policy. CPU throttle ratio is not persisted on `WorkloadProfile` status; the UI shows “not on status” for that field.

## Source files

Static assets live under `internal/dashboard/static/` and are embedded with `go:embed`.
