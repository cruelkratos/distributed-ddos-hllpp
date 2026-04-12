.PHONY: all build proto test clean \
       docker-build docker-demo docker-down \
       kind-setup kind-demo kind-clean

# ── Build ─────────────────────────────────────────────
all: build

build:
	go build -o bin/agent       ./cmd/agent/
	go build -o bin/aggregator  ./cmd/aggregator/
	CGO_ENABLED=0 go build -o bin/iot-sim ./cmd/iot-sim/
	go build -o bin/serial-bridge ./cmd/serial-bridge/

# Cross-compile for Raspberry Pi 3 (ARMv7). Simulation mode only (no pcap/CGO).
build-arm:
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o bin/agent-arm       ./cmd/agent/
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o bin/aggregator-arm  ./cmd/aggregator/
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o bin/iot-sim-arm     ./cmd/iot-sim/
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o bin/serial-bridge-arm ./cmd/serial-bridge/

# Build for Pi natively ON the Pi (with libpcap support for real packet capture).
build-pi-native:
	go build -o bin/agent       ./cmd/agent/
	go build -o bin/aggregator  ./cmd/aggregator/
	go build -o bin/serial-bridge ./cmd/serial-bridge/

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
	       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
	       server.proto
	@echo "Proto files regenerated."

test:
	go test -v -count=1 ./ddos/detector/... ./ddos/mitigation/... ./ddos/window/...

test-cover:
	go test -coverprofile=coverage.out ./ddos/detector/... ./ddos/mitigation/... ./ddos/window/...
	go tool cover -func=coverage.out

clean:
	rm -rf bin/ coverage.out

# ── Docker Compose demo ──────────────────────────────
docker-build:
	docker-compose build

docker-demo:
	bash demo/run_docker_demo.sh

docker-down:
	docker-compose down -v

# ── Kind (K8s) demo ──────────────────────────────────
kind-setup:
	bash demo/kind-setup.sh

kind-demo: kind-setup
	bash demo/run_demo.sh

kind-clean:
	kind delete cluster --name "$${CLUSTER_NAME:-ddos-demo}"
