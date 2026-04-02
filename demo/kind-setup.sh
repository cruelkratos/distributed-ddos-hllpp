#!/usr/bin/env bash
# kind-setup.sh – Create a kind cluster and load images for the DDoS detection demo
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-ddos-demo}"
echo "==> Creating kind cluster: ${CLUSTER_NAME}"

# Create cluster if it doesn't exist
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
  echo "    Cluster '${CLUSTER_NAME}' already exists, skipping creation."
else
  kind create cluster --name "${CLUSTER_NAME}" --wait 60s
fi

echo "==> Building Docker images..."
docker build -t gaud4/ddos-agent:latest     -f Dockerfile.agent .
docker build -t gaud4/ddos-aggregator:latest -f Dockerfile.aggregator .
docker build -t gaud4/ddos-iot-sim:latest    -f Dockerfile.iot-sim .

echo "==> Loading images into kind cluster..."
kind load docker-image gaud4/ddos-agent:latest       --name "${CLUSTER_NAME}"
kind load docker-image gaud4/ddos-aggregator:latest   --name "${CLUSTER_NAME}"
kind load docker-image gaud4/ddos-iot-sim:latest      --name "${CLUSTER_NAME}"

echo "==> Done! Cluster '${CLUSTER_NAME}' is ready."
echo "    Run 'demo/run_demo.sh' to deploy the stack."
