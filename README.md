# prom-replay

Archive and replay Prometheus metrics from benchmark runs. View historical results in Grafana with full PromQL support and compare runs side-by-side.

## Architecture

A Helm chart deploying four components:

- **VictoriaMetrics** (single-node) — scrapes live metrics and stores historical runs
- **MinIO** (single-node) — S3-compatible archive for run exports
- **Replay Manager** — Go REST API managing the run lifecycle
- **Grafana** — dashboards with `run_id` template variable for filtering and comparison

See [docs/solution-overview.md](docs/solution-overview.md) for full design details.

## Prerequisites

- Kubernetes cluster (or [kind](https://kind.sigs.k8s.io/) for local dev)
- Helm 3
- kubectl

## Install

Install the Helm chart:

```bash
helm install prom-replay charts/prom-replay --namespace prom-replay --create-namespace
```

## Access

```bash
# Replay Manager (includes UI and Grafana proxy)
kubectl port-forward svc/prom-replay-prom-replay-replay-manager 8080:8080 -n prom-replay
```

Open `http://localhost:8080/ui` for the run management UI. Grafana dashboards are proxied at `/grafana/`.

## Usage

### 1. Export a run

After a benchmark completes, export the metrics by time range:

```bash
curl -X POST http://localhost:8080/runs \
  -H 'Content-Type: application/json' \
  -d '{
    "start": "2026-04-25T00:00:00Z",
    "end": "2026-04-25T01:00:00Z",
    "labels": {"benchmark": "sysbench", "config": "16-threads"}
  }'
```

Returns a `run_id` (e.g., `20260425T010530Z`).

### 2. List runs

```bash
curl http://localhost:8080/runs
```

Returns all archived runs with metadata, size, and whether each is currently loaded in VictoriaMetrics.

### 3. Load a run

```bash
curl -X POST http://localhost:8080/runs/20260425T010530Z/load
```

Imports the run into VictoriaMetrics with a `run_id` label. The data becomes queryable in Grafana.

### 4. View in Grafana

In the run management UI, click a run to select it, then click a dashboard link to view it in Grafana with the correct time range. Select multiple `run_id` values in the dashboard dropdown to compare runs side-by-side.

### 5. Unload a run

```bash
curl -X DELETE http://localhost:8080/runs/20260425T010530Z/load
```

Removes the run from VictoriaMetrics queries. The archive in MinIO is untouched.

### 6. Delete a run

```bash
curl -X DELETE http://localhost:8080/runs/20260425T010530Z
```

Deletes the run from MinIO entirely (and unloads from VictoriaMetrics if loaded).

## Integration with benchmark scripts

Add the export call at the end of your benchmark script:

```bash
START_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# ... run benchmark ...

END_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

curl -X POST http://replay-manager:8080/runs \
  -H 'Content-Type: application/json' \
  -d "{\"start\":\"${START_TIME}\",\"end\":\"${END_TIME}\",\"labels\":{\"benchmark\":\"my-bench\"}}"
```

## Configuration

Key values in `values.yaml`:

| Value | Default | Description |
|---|---|---|
| `victoria-metrics-single.server.retentionPeriod` | `30d` | How long loaded runs stay in VM before disk cleanup |
| `minio.persistence.size` | `10Gi` | Storage for archived run exports |
| `dashboards.maxSizeBytes` | `1048576` | Max dashboard JSON size (ConfigMap limit validation) |
| `grafana.image.repository` | `grafana/grafana` | Grafana image |

## Development

### Build the replay manager

```bash
make build
```

### Run e2e tests

Requires: kind, bats, docker, helm, kubectl, jq

```bash
make e2e
```

This creates a kind cluster, builds and loads the replay manager image, installs the Helm chart, runs the full lifecycle test suite (15 tests), and tears down.

To keep the cluster alive for debugging:

```bash
make e2e-setup
```

To tear down manually:

```bash
make e2e-teardown
```
