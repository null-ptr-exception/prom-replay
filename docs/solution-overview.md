# prom-replay: Prometheus Snapshot Replay System

## Problem

Performance benchmarks generate rich Prometheus metrics during a run, but once
the benchmark ends and infrastructure is torn down, the metrics are gone. Teams
need to:

- Archive metrics from each benchmark run for later analysis
- Replay historical runs in Grafana with full PromQL support
- Compare runs side-by-side
- Share results without requiring the original infrastructure

Existing approaches either lose metrics (tear down Prometheus with the cluster)
or build custom "poor man's Grafana" visualizations in static HTML with JSON
metric dumps and charting libraries. These self-contained HTML reports are easy
to compare side-by-side but can't match a real dashboard tool's flexibility —
every new metric or visualization requires code changes, and you lose PromQL's
aggregation, rate calculations, and histogram support.

## Solution

A Helm chart deploying four components:

```
┌──────────────────────────────────────────────────────┐
│                    prom-replay                       │
│                                                      │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────┐ │
│  │ VictoriaMetrics│  │    MinIO      │  │  Grafana  │ │
│  │ (single-node) │  │  (single-node) │  │           │ │
│  │               │  │               │  │  DS: vm   │ │
│  │ - scrapes     │  │  - S3-compat  │  │           │ │
│  │ - remote write│  │  - run archive│  │ Dashboards│ │
│  │ - all runs    │  │  - meta.json  │  │ Run Mgmt  │ │
│  └──────┬────────┘  └───────┬───────┘  └─────┬─────┘ │
│         │                   │                │       │
│         │       ┌───────────┴───────────┐    │       │
│         │       │    Replay Manager     │    │       │
│         │       │    (REST API)         │    │       │
│         │       │                       │    │       │
│         │       │  POST   /runs         │    │       │
│         │       │  GET    /runs         │    │       │
│         │       │  POST   /runs/:id/load│    │       │
│         │       │  DELETE /runs/:id/load│    │       │
│         │       │  DELETE /runs/:id     │    │       │
│         │       └───────────────────────┘    │       │
│         │                                    │       │
│         ▼                                    │       │
│     [scrape targets / remote write]                  │
└──────────────────────────────────────────────────────┘
```

### Components

#### 1. VictoriaMetrics (single-node)

A single VictoriaMetrics instance handles both live and historical data.
During benchmarks it scrapes targets and/or receives remote write (e.g., from
k6). Historical runs are imported back with a `run_id` label, so all runs
coexist in one TSDB and can be queried simultaneously for side-by-side
comparison.

VictoriaMetrics implements MetricsQL, a superset of PromQL — all standard
PromQL queries work as-is. Grafana connects to it as a Prometheus datasource.

Configuration:
- Scrape configs provided via values.yaml
- `--retentionPeriod=30d` (default, configurable via values.yaml)

Note: VM's delete API (`DELETE /api/v1/admin/tsdb/delete_series`) marks series
for deletion, but actual disk reclaim is deferred and governed by retention
settings. Unloading a run removes it from query results immediately, but disk
space is reclaimed asynchronously.

#### 2. MinIO (single-node)

S3-compatible object storage for archiving run exports. Each run is stored as:

```
runs/<run_id>/meta.json
runs/<run_id>/data.jsonl
```

`meta.json` contains:
- `run_id` — unique identifier (e.g., timestamp-based)
- `start` — benchmark start time
- `end` — benchmark end time
- `created_at` — when the export was taken
- `labels` — optional user-provided metadata (benchmark name, parameters, etc.)

Object size is available from S3 object info, so it is not duplicated in
metadata.

#### 3. Replay Manager

A lightweight Go REST API that manages the run lifecycle:

| Endpoint | Action |
|---|---|
| `POST /runs` | Export from VM by time range, upload to MinIO |
| `GET /runs` | List runs from MinIO, cross-reference with VM for loaded status |
| `POST /runs/:id/load` | Download from MinIO, import into VM with `run_id` label |
| `DELETE /runs/:id/load` | Delete series with that `run_id` from VM |
| `DELETE /runs/:id` | Delete from MinIO (and unload from VM if loaded) |

**Export flow (POST /runs):**
1. Caller provides `start`, `end`, and optional `labels`
2. Replay manager calls VM export API (`GET /api/v1/export`) for the time range
3. Uploads exported data as `data.jsonl` to MinIO
4. Writes `meta.json` alongside it
5. Returns `run_id`

**Load flow (POST /runs/:id/load):**
1. Check if `run_id` is already loaded (query VM for existing label)
2. Download `data.jsonl` from MinIO
3. Import into VM via import API with `run_id` extra label injected
4. Return success

**Loaded status detection:**
The replay manager queries `GET /api/v1/label/run_id/values` on VM to determine
which runs are currently loaded. VM is the source of truth for runtime state;
S3/MinIO is purely the archive. No loaded/unloaded flags are stored in
`meta.json`.

**Idempotency:**
VM import is not idempotent — re-importing the same data duplicates samples.
The replay manager checks whether a `run_id` is already loaded before importing
and skips re-import.

#### 4. Grafana

Pre-configured with a single VictoriaMetrics datasource.

**Metric dashboards** use a `run_id` template variable (populated from VM's
`run_id` label values) to filter which run(s) to display. Selecting multiple
`run_id` values enables side-by-side comparison on the same dashboard.

**Run management UI** is served by the Replay Manager at `/ui`, providing:
- Table listing available runs with load/unload/delete actions
- Dashboard links per run that open Grafana with the correct time range
- Grafana is reverse-proxied at `/grafana/`, so only one port-forward is needed

**Dashboard provisioning:**
Dashboards are stored as JSON files in the Helm chart and deployed as
ConfigMaps. Grafana's sidecar provisioner picks them up automatically.

The Helm chart validates each dashboard file's size against a configurable
limit (`dashboards.maxSizeBytes`, default `1048576` / 1 MB) and fails at
`helm install`/`helm upgrade` time if any dashboard exceeds it. This catches
the ConfigMap size constraint early rather than failing silently at runtime.

## Data Flow

### During a benchmark run

```
Benchmark (k6/sysbench)
  → remote write to VictoriaMetrics
  → Grafana shows real-time metrics

Scrape targets (YB tserver, master, node-exporter)
  → VictoriaMetrics scrapes at configured interval
```

### After a benchmark run

```
Test script calls: POST /runs  (start=T0, end=T1, labels={...})
  → Replay Manager exports from VM for time range [T0, T1]
  → Uploads data.jsonl + meta.json to MinIO
  → Returns run_id
  → Infrastructure can now be torn down
```

### Viewing a historical run

```
Developer opens Replay Manager UI at /ui
  → Sees list of archived runs
  → Clicks "Load" on a run
  → POST /runs/:id/load
  → Replay Manager imports data into VM with run_id label
  → Clicks a dashboard link for the run
  → Opens Grafana (proxied at /grafana/) with correct time range
  → Selects run_id from dropdown → sees historical metrics
```

### Comparing runs side-by-side

```
Developer loads two runs (run_id=A and run_id=B)
  → Selects both in the run_id template variable
  → Dashboard panels show both runs overlaid or in split view
```

## Sizing

Based on a real vm-virsh benchmark (3 YB tservers, 3-hour Prometheus uptime):

| Metric | Value |
|---|---|
| Series count | ~50,000 |
| Total samples | 68 million |
| Raw TSDB on disk | 126 MB |
| **Compressed export** | **~19 MB** |
| Benchmark duration | 3 minutes |

A typical benchmark export is 10-50 MB compressed. MinIO storage cost is
negligible even for hundreds of runs.

## Deployment

### Kubernetes (Helm chart)

Primary deployment target. All four components run as pods.

```bash
helm install prom-replay ./charts/prom-replay
```

MinIO is included in the chart as built-in storage — no external S3
configuration required for the default setup.

### VM / Docker Compose (future)

For environments without Kubernetes, a docker-compose.yml provides the
same stack.

## Non-Goals

- Not a general-purpose metrics HA/federation solution
- Not a long-term metrics store (use Thanos/Cortex for that)
- No multi-tenant access control
- No automatic snapshot scheduling (the test script decides when to export)
