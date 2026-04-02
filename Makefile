.PHONY: all build proto test clean \
       docker-build docker-demo docker-down \
       kind-setup kind-demo kind-clean

# ── Build ─────────────────────────────────────────────
all: build

build:
	go build -o bin/agent       ./cmd/agent/
	go build -o bin/aggregator  ./cmd/aggregator/
	CGO_ENABLED=0 go build -o bin/iot-sim ./cmd/iot-sim/

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
