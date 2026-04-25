#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

CLUSTER_NAME="${CLUSTER_NAME:-prom-replay-e2e}"
NAMESPACE="${NAMESPACE:-prom-replay}"
RELEASE_NAME="${RELEASE_NAME:-pr}"
CHART_DIR="${ROOT_DIR}/charts/prom-replay"
REPLAY_MANAGER_DIR="${ROOT_DIR}/replay-manager"
REPLAY_MANAGER_IMAGE="prom-replay/replay-manager:e2e"

REPLAY_MANAGER_PORT="${REPLAY_MANAGER_PORT:-18080}"
VM_PORT="${VM_PORT:-18428}"

SKIP_SETUP="${SKIP_SETUP:-false}"
SKIP_TEARDOWN="${SKIP_TEARDOWN:-false}"

cleanup_portforward() {
    if [ -f /tmp/prom-replay-e2e-pf.pid ]; then
        while read -r pid; do
            kill "$pid" 2>/dev/null || true
        done < /tmp/prom-replay-e2e-pf.pid
        rm -f /tmp/prom-replay-e2e-pf.pid
    fi
}

teardown() {
    cleanup_portforward
    if [ "$SKIP_TEARDOWN" = "false" ]; then
        echo "==> Tearing down kind cluster"
        kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
    fi
}

if [ "$SKIP_SETUP" = "false" ]; then
    echo "==> Creating kind cluster: $CLUSTER_NAME"
    kind create cluster --name "$CLUSTER_NAME" --wait 60s 2>&1 || true
    kubectl cluster-info --context "kind-${CLUSTER_NAME}"

    echo "==> Building replay manager image"
    docker build -t "$REPLAY_MANAGER_IMAGE" "$REPLAY_MANAGER_DIR"

    echo "==> Loading image into kind"
    kind load docker-image "$REPLAY_MANAGER_IMAGE" --name "$CLUSTER_NAME"

    echo "==> Updating Helm dependencies"
    helm dependency update "$CHART_DIR"

    echo "==> Installing Helm chart"
    kubectl create namespace "$NAMESPACE" --context "kind-${CLUSTER_NAME}" 2>/dev/null || true
    helm upgrade --install "$RELEASE_NAME" "$CHART_DIR" \
        --namespace "$NAMESPACE" \
        --kube-context "kind-${CLUSTER_NAME}" \
        --set replayManager.image.repository=prom-replay/replay-manager \
        --set replayManager.image.tag=e2e \
        --set replayManager.image.pullPolicy=Never \
        --set minio.persistence.size=1Gi \
        --wait \
        --timeout 300s

    echo "==> Waiting for pods"
    kubectl wait --for=condition=ready pod --all \
        -n "$NAMESPACE" \
        --context "kind-${CLUSTER_NAME}" \
        --timeout=120s
fi

echo "==> Setting up port-forwarding"
cleanup_portforward

kubectl port-forward "svc/${RELEASE_NAME}-prom-replay-replay-manager" "${REPLAY_MANAGER_PORT}:8080" \
    -n "$NAMESPACE" \
    --context "kind-${CLUSTER_NAME}" &
echo $! >> /tmp/prom-replay-e2e-pf.pid

kubectl port-forward "svc/${RELEASE_NAME}-victoria-metrics-single-server" "${VM_PORT}:8428" \
    -n "$NAMESPACE" \
    --context "kind-${CLUSTER_NAME}" &
echo $! >> /tmp/prom-replay-e2e-pf.pid

echo "==> Waiting for port-forwards to be ready"
for port in "$REPLAY_MANAGER_PORT" "$VM_PORT"; do
    for i in $(seq 1 30); do
        if curl -sf "http://localhost:${port}" >/dev/null 2>&1 || curl -sf "http://localhost:${port}/healthz" >/dev/null 2>&1; then
            break
        fi
        if [ "$i" -eq 30 ]; then
            echo "ERROR: port-forward on $port not ready after 30s"
            teardown
            exit 1
        fi
        sleep 1
    done
done

echo "==> Running BATS tests"
export REPLAY_MANAGER_PORT VM_PORT CLUSTER_NAME NAMESPACE RELEASE_NAME

if REPLAY_MANAGER_PORT="$REPLAY_MANAGER_PORT" VM_PORT="$VM_PORT" \
    bats "$SCRIPT_DIR/lifecycle.bats"; then
    echo "==> All tests passed"
    test_result=0
else
    echo "==> Some tests failed"
    test_result=1
fi

teardown
exit $test_result
