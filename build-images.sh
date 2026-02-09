#!/bin/bash
# Build script for DDoS detector Docker images
# This script handles network issues by building binaries locally first

set -e

echo "=== Building DDoS Detector Images ==="
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "ERROR: Go is not installed."
    echo ""
    echo "Please install Go first:"
    echo "  Option 1 (recommended): sudo snap install go --classic"
    echo "  Option 2: sudo apt install golang-go"
    echo "  Option 3: Download from https://go.dev/dl/ and extract to ~/go"
    echo ""
    echo "After installing Go, run this script again."
    exit 1
fi

echo "✓ Go found: $(go version)"
echo ""

# Set build variables
AGENT_IMAGE="docker.io/gaud4/ddos-agent:latest"
AGGREGATOR_IMAGE="docker.io/gaud4/ddos-aggregator:latest"

# Build agent binary (CGO required for gopacket/pcap)
echo "Building agent binary..."
echo "Note: This requires libpcap-dev. Install with: sudo apt-get install libpcap-dev"
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o agent ./cmd/agent
if [ $? -ne 0 ]; then
    echo "ERROR: Failed to build agent binary"
    exit 1
fi
echo "✓ Agent binary built"

# Copy libpcap library files for Docker image (required!)
echo "Preparing libpcap libraries..."
if find /usr/lib/x86_64-linux-gnu -name "libpcap.so*" -exec cp {} . \; 2>/dev/null; then
    echo "✓ Copied libpcap from /usr/lib/x86_64-linux-gnu"
elif find /lib/x86_64-linux-gnu -name "libpcap.so*" -exec cp {} . \; 2>/dev/null; then
    echo "✓ Copied libpcap from /lib/x86_64-linux-gnu"
else
    echo "ERROR: Could not find libpcap.so* libraries!"
    echo "Please install libpcap-dev: sudo apt-get install libpcap-dev"
    echo "Then find the libraries: find /usr -name 'libpcap.so*'"
    exit 1
fi

# Copy libdbus libraries (required for agent)
# Try to copy from host, but if not found, Docker will install via apt
echo "Preparing libdbus libraries..."
if find /usr/lib/x86_64-linux-gnu -name "libdbus-1.so*" -exec cp {} . \; 2>/dev/null | head -1 > /dev/null; then
    echo "✓ Copied libdbus from /usr/lib/x86_64-linux-gnu"
elif find /lib/x86_64-linux-gnu -name "libdbus-1.so*" -exec cp {} . \; 2>/dev/null | head -1 > /dev/null; then
    echo "✓ Copied libdbus from /lib/x86_64-linux-gnu"
else
    echo "Note: libdbus-1.so* not found locally"
    echo "  Docker will install libdbus-1-3 via apt (requires network)"
    echo "  If network fails, install locally first: sudo apt-get install libdbus-1-3"
    # Create a dummy file so COPY doesn't fail (Docker will install via apt anyway)
    touch libdbus-1.so.dummy
fi

# Build agent Docker image
echo "Building agent Docker image..."
docker build -f Dockerfile.agent.local -t "$AGENT_IMAGE" .
if [ $? -ne 0 ]; then
    echo "ERROR: Failed to build agent Docker image"
    rm -f agent libpcap.so* 2>/dev/null || true
    exit 1
fi
echo "✓ Agent image built: $AGENT_IMAGE"
rm -f agent libpcap.so* libdbus-1.so* 2>/dev/null || true

# Build aggregator binary
echo ""
echo "Building aggregator binary..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o aggregator .
if [ $? -ne 0 ]; then
    echo "ERROR: Failed to build aggregator binary"
    exit 1
fi
echo "✓ Aggregator binary built"

# Build aggregator Docker image
echo "Building aggregator Docker image..."
docker build -f Dockerfile.aggregator.local -t "$AGGREGATOR_IMAGE" .
if [ $? -ne 0 ]; then
    echo "ERROR: Failed to build aggregator Docker image"
    rm -f aggregator
    exit 1
fi
echo "✓ Aggregator image built: $AGGREGATOR_IMAGE"
rm -f aggregator

echo ""
echo "=== Build Complete ==="
echo ""
echo "Images built:"
echo "  - $AGENT_IMAGE"
echo "  - $AGGREGATOR_IMAGE"
echo ""
echo "Next steps:"
echo "  1. Push images: docker push $AGENT_IMAGE && docker push $AGGREGATOR_IMAGE"
echo "  2. Deploy: kubectl apply -f k8s/daemonset-agent.yaml && kubectl apply -f k8s/deployment-aggregator.yaml"
