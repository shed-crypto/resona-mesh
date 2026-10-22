# ResonaMesh Build & Test Automation

BINARY_NAME=resonamesh.exe
VERSION=0.1.0-alpha

.PHONY: all build build-ui test test-acceptance benchmark clean docker-up docker-down

all: build-ui build

build-ui:
	cd web && npm run build

build:
	go build -ldflags="-s -w -X main.AppVersion=$(VERSION) -X main.AppName=ResonaMesh" -o $(BINARY_NAME) ./cmd/resonamesh

test:
	go test ./...

test-acceptance:
	go test -v -count=1 ./tests/integration

benchmark:
	go run ./cmd/benchmark
	go run ./experiments/analysis/analyze.go

docker-up:
	docker compose -f deploy/compose/docker-compose.yml up --build -d

docker-down:
	docker compose -f deploy/compose/docker-compose.yml down -v

clean:
	@if exist $(BINARY_NAME) del $(BINARY_NAME)
	@if exist bench.exe del bench.exe
