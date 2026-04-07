#!/usr/bin/env bash
# run_docker_demo.sh – Run the full demo using docker-compose (no K8s required)
set -euo pipefail

echo "==> Building images with docker-compose..."
docker-compose build

echo "==> Starting the stack..."
docker-compose up -d

echo ""
echo "============================================"
echo "  DDoS Detection Demo (Docker Compose)"
echo "============================================"
echo ""
echo "  Prometheus: http://localhost:9090"
echo "  Grafana:    http://localhost:3000  (admin/admin)"
echo ""
echo "  IoT Node 1 metrics: http://localhost:9001/metrics"
echo "  IoT Node 2 metrics: http://localhost:9002/metrics"
echo "  IoT Node 3 metrics: http://localhost:9003/metrics (mixed: attack at t=30s)"
echo "  Aggregator metrics:  http://localhost:9091/metrics"
echo ""
echo "  Follow logs:"
echo "    docker-compose logs -f"
echo ""
echo "  Stop:"
echo "    docker-compose down"
echo "============================================"
