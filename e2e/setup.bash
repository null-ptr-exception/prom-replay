#!/usr/bin/env bash
# Shared helpers for e2e tests

CLUSTER_NAME="${CLUSTER_NAME:-prom-replay-e2e}"
NAMESPACE="${NAMESPACE:-prom-replay}"
RELEASE_NAME="${RELEASE_NAME:-pr}"
CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../charts/prom-replay" && pwd)"
REPLAY_MANAGER_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../replay-manager" && pwd)"
REPLAY_MANAGER_IMAGE="prom-replay/replay-manager:e2e"

wait_for_pod_ready() {
    local label="$1"
    local timeout="${2:-120}"
    kubectl wait --for=condition=ready pod \
        -l "$label" \
        -n "$NAMESPACE" \
        --timeout="${timeout}s" \
        --context "kind-${CLUSTER_NAME}"
}

replay_manager_url() {
    echo "http://localhost:${REPLAY_MANAGER_PORT:-18080}"
}

vm_url() {
    echo "http://localhost:${VM_PORT:-18428}"
}

api_post() {
    local path="$1"
    shift
    curl -sf -X POST "$(replay_manager_url)${path}" \
        -H 'Content-Type: application/json' \
        "$@"
}

api_get() {
    local path="$1"
    curl -sf "$(replay_manager_url)${path}"
}

api_delete() {
    local path="$1"
    curl -sf -X DELETE "$(replay_manager_url)${path}"
}

inject_test_metrics() {
    local vm="$(vm_url)"
    local now
    now=$(date +%s)
    local past=$((now - 300))

    local data=""
    for i in $(seq "$past" 5 "$now"); do
        data+='{"metric":{"__name__":"e2e_test_gauge","job":"e2e","instance":"test:9090"},"values":['$((RANDOM % 100))'],"timestamps":['${i}'000]}'$'\n'
    done

    curl -sf -X POST "${vm}/api/v1/import" \
        -H 'Content-Type: application/x-ndjson' \
        --data-binary "$data"
}
