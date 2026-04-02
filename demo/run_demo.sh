#!/usr/bin/env bash
# run_demo.sh – Deploy the full DDoS detection demo on kind and display access info
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-ddos-demo}"
CONTEXT="kind-${CLUSTER_NAME}"

echo "==> Using kubectl context: ${CONTEXT}"
kubectl config use-context "${CONTEXT}" 2>/dev/null || true

echo ""
echo "==> Step 1: Deploying aggregator..."
kubectl apply -f k8s/deployment-aggregator.yaml

echo "==> Step 2: Deploying monitoring stack..."
kubectl apply -f k8s/monitoring/prometheus.yaml
kubectl apply -f k8s/monitoring/grafana.yaml

echo "==> Step 3: Waiting for aggregator to be ready..."
kubectl rollout status deployment/ddos-aggregator --timeout=60s

echo "==> Step 4: Deploying IoT simulation agents..."
kubectl apply -f k8s/iot-simulation/deployment-iot-agent.yaml

echo "==> Step 5: Waiting for IoT agents to be ready..."
kubectl rollout status deployment/iot-agent --timeout=60s

echo "==> Step 6: Launching attacker job..."
kubectl apply -f k8s/iot-simulation/job-attacker.yaml

echo ""
echo "============================================"
echo "  DDoS Detection Demo is running!"
echo "============================================"
echo ""
echo "  Prometheus: http://localhost:30090"
echo "  Grafana:    http://localhost:30030  (admin/admin)"
echo ""
echo "  The attacker job will inject traffic for ~120s."
echo "  Attack traffic starts at t=30s (mixed profile)."
echo ""
echo "  Watch aggregator logs:"
echo "    kubectl logs -f deployment/ddos-aggregator"
echo ""
echo "  Watch agent logs:"
echo "    kubectl logs -f deployment/iot-agent"
echo ""
echo "  Watch attacker:"
echo "    kubectl logs -f job/iot-attacker"
echo ""
echo "  Cleanup:"
echo "    make kind-clean"
echo "============================================"
