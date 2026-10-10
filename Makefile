.PHONY: build test lint run clean docker-build docker-up docker-down

APP_NAME=gateway
BUILD_DIR=bin
MAIN_PKG=./cmd/gateway

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PKG)

test:
	go test -race -cover ./...

lint:
	golangci-lint run ./...

run:
	go run $(MAIN_PKG) --config config/default.yaml

clean:
	rm -rf $(BUILD_DIR)

docker-build:
	docker build -t go-api-gateway:latest -f deploy/docker/Dockerfile .

docker-up:
	docker-compose -f deploy/compose/docker-compose.yml up -d

docker-down:
	docker-compose -f deploy/compose/docker-compose.yml down
