.PHONY: build test lint clean install deps fmt vet bench ci

BINARY_NAME=dhg
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Build the binary
build:
	$(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/dhg

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/dhg
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/dhg
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-amd64 ./cmd/dhg
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-arm64 ./cmd/dhg
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME)-windows-amd64.exe ./cmd/dhg

# Run tests
test:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

# Run tests with coverage report
test-coverage: test
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Run linter
lint:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

# Format code
fmt:
	$(GOFMT) ./...

# Vet code
vet:
	$(GOVET) ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Install binary
install: build
	cp bin/$(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME)

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Run the application
run: build
	./bin/$(BINARY_NAME)

# Generate mocks (if needed)
generate:
	$(GOCMD) generate ./...

# E2E test: generate chart and validate with helm
e2e: build
	./bin/$(BINARY_NAME) generate -f testdata/simple -o /tmp/dhg-test-chart --chart-name test-app
	helm lint /tmp/dhg-test-chart
	helm template test-release /tmp/dhg-test-chart
	@echo "E2E test passed!"

# Run benchmarks
bench:
	$(GOTEST) ./tests/integration/ -bench=BenchmarkPipeline -benchmem -benchtime=3x -run=^$ -timeout=300s

# Development helper: build and run with sample data
dev: build
	./bin/$(BINARY_NAME) generate -f testdata/simple -o /tmp/dhg-dev-chart --chart-name dev-app --verbose

# Run full CI pipeline locally
ci: deps vet lint test build
	@echo "CI pipeline passed!"

.DEFAULT_GOAL := build
