#!/usr/bin/env bash
#
# Deploy smoke test: builds the container image, loads it into a kind cluster,
# applies the manifests in deploy/, and asserts that the exporter
#   1. rolls out and stays healthy,
#   2. serves its /-/healthy, /-/ready and /metrics endpoints, and
#   3. actually observes and emits cluster events (via the stdout sink).
#
# It validates packaging, RBAC and runtime wiring that the in-process Go tests
# deliberately bypass. Requires: docker, kind, kubectl.
#
# Usage:
#   KIND_CLUSTER=kee-e2e test/e2e/smoke.sh
#
set -euo pipefail

IMAGE="${IMAGE:-ghcr.io/mustafaakin/kubernetes-event-exporter:latest}"
KIND_CLUSTER="${KIND_CLUSTER:-kee-e2e}"
NAMESPACE="monitoring"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

log() { echo "[smoke] $*" >&2; }

PF_PID=""
cleanup() {
  [[ -n "${PF_PID}" ]] && kill "${PF_PID}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

cd "${REPO_ROOT}"

log "Building image ${IMAGE}"
docker build -t "${IMAGE}" .

log "Loading image into kind cluster ${KIND_CLUSTER}"
kind load docker-image "${IMAGE}" --name "${KIND_CLUSTER}"

log "Applying manifests"
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f deploy/

log "Waiting for rollout"
kubectl -n "${NAMESPACE}" rollout status deploy/event-exporter --timeout=120s

POD="$(kubectl -n "${NAMESPACE}" get pod -l app=event-exporter -o jsonpath='{.items[0].metadata.name}')"
log "Exporter pod: ${POD}"

log "Port-forwarding metrics endpoint"
kubectl -n "${NAMESPACE}" port-forward "pod/${POD}" 2112:2112 >/dev/null 2>&1 &
PF_PID=$!
# Wait for the port-forward to become usable.
for _ in $(seq 1 20); do
  if curl -fsS "http://127.0.0.1:2112/-/healthy" >/dev/null 2>&1; then break; fi
  sleep 1
done

log "Checking /-/healthy, /-/ready and /metrics"
curl -fsS "http://127.0.0.1:2112/-/healthy" >/dev/null
curl -fsS "http://127.0.0.1:2112/-/ready" >/dev/null
if ! curl -fsS "http://127.0.0.1:2112/metrics" | grep -q "event_exporter_build_info"; then
  log "FAIL: expected metric event_exporter_build_info not found"
  exit 1
fi

log "Generating a real event (a pod with an invalid image)"
PROBE_NS="smoke-probe"
kubectl create namespace "${PROBE_NS}" --dry-run=client -o yaml | kubectl apply -f -
kubectl -n "${PROBE_NS}" delete pod smoke-failer --ignore-not-found
kubectl -n "${PROBE_NS}" run smoke-failer \
  --image=registry.invalid/does-not-exist:nope \
  --restart=Never >/dev/null

log "Asserting the exporter dumped events to stdout"
found=0
for _ in $(seq 1 40); do
  if kubectl -n "${NAMESPACE}" logs "${POD}" --tail=-1 2>/dev/null | grep -q '"reason"'; then
    found=1
    break
  fi
  sleep 3
done

kubectl -n "${PROBE_NS}" delete pod smoke-failer --ignore-not-found >/dev/null 2>&1 || true
kubectl delete namespace "${PROBE_NS}" --ignore-not-found >/dev/null 2>&1 || true

if [[ "${found}" -ne 1 ]]; then
  log "FAIL: exporter did not emit any event to stdout"
  kubectl -n "${NAMESPACE}" logs "${POD}" --tail=50 >&2 || true
  exit 1
fi

log "PASS: deploy smoke test succeeded"
