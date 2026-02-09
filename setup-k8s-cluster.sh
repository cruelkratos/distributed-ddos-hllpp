#!/bin/bash
# Script to set up a local Kubernetes cluster using kind

set -e

echo "=== Setting up local Kubernetes cluster ==="
echo ""

# Check if kind is installed
if ! command -v kind &> /dev/null; then
    echo "kind is not installed. Installing..."
    curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.26.0/kind-linux-amd64
    chmod +x ./kind
    sudo mv ./kind /usr/local/bin/kind
    echo "✓ kind installed"
else
    echo "✓ kind found: $(kind --version)"
fi

# Check if cluster already exists
if kind get clusters | grep -q "^ddos$"; then
    echo ""
    echo "Cluster 'ddos' already exists. Options:"
    echo "  1. Delete and recreate: kind delete cluster --name ddos"
    echo "  2. Use existing: kubectl config use-context kind-ddos"
    echo ""
    read -p "Delete and recreate? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        kind delete cluster --name ddos
    else
        kubectl config use-context kind-ddos
        echo "✓ Using existing cluster"
        exit 0
    fi
fi

# Create cluster
echo ""
echo "Creating Kubernetes cluster 'ddos'..."
kind create cluster --name ddos

# Set context
kubectl config use-context kind-ddos

echo ""
echo "=== Cluster Setup Complete ==="
echo ""
echo "Cluster info:"
kubectl cluster-info
echo ""
echo "Nodes:"
kubectl get nodes
echo ""
echo "You can now deploy your DDoS detector:"
echo "  kubectl apply -f k8s/daemonset-agent.yaml"
echo "  kubectl apply -f k8s/deployment-aggregator.yaml"
