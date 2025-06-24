# Discobox Production Makefile
# Following the implementation guide for a production-grade reverse proxy

# Variables
BINARY_NAME=discobox
MAIN_PATH=./cmd/discobox
BUILD_DIR=./build
DB_DIR=./data
DIST_DIR=./dist
DOCKER_IMAGE=discobox
DOCKER_TAG?=latest

# Version information
VERSION?=$(shell git describe --tags --always --dirty)
COMMIT=$(shell git rev-parse --short HEAD)
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GORUN=$(GOCMD) run
GOGEN=$(GOCMD) generate
GOFMT=gofmt
GOLINT=golangci-lint
GOVET=$(GOCMD) vet

# Test parameters
TEST_TIMEOUT=10m
COVERAGE_DIR=./coverage
COVERAGE_FILE=$(COVERAGE_DIR)/coverage.out

.PHONY: all build test clean docker help

# Default target
all: clean lint test build

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for multiple platforms
build-all: clean
	@echo "Building for multiple platforms..."
	@mkdir -p $(DIST_DIR)
	# Linux AMD64
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	# Linux ARM64
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PATH)
	# Darwin AMD64
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	# Darwin ARM64 (M1)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	# Windows AMD64
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)
	@echo "Multi-platform build complete"

# Run the application
run: gen build
	@echo "Running $(BINARY_NAME)..."
	$(BUILD_DIR)/$(BINARY_NAME) -config configs/discobox.yml

gen:
	@echo "Generating/building svelte ui for $(BINARY_NAME)..."
	$(GOGEN) pkg/ui/discobox/ui.go

# Development mode with hot reload
dev: gen clean
	@echo "Running in development mode with hot reload..."
	$(GORUN) $(MAIN_PATH)
#	@which air > /dev/null || go install github.com/air-verse/air@latest
# 	air -c .air.toml

# Run tests
test:
	@echo "Running tests..."
	@mkdir -p $(COVERAGE_DIR)
	$(GOTEST) -v -race -timeout $(TEST_TIMEOUT) -coverprofile=$(COVERAGE_FILE) ./...
	@echo "Tests complete"

# Run unit tests only
test-unit:
	@echo "Running unit tests..."
	$(GOTEST) -v -short -race ./...

# Run integration tests
test-integration:
	@echo "Running integration tests..."
	$(GOTEST) -v -race -run Integration ./test/...

# Run end-to-end tests
test-e2e:
	@echo "Running end-to-end tests..."
	$(GOTEST) -v -race ./test/e2e/...

# Run load tests
test-load:
	@echo "Running load tests..."
	@which k6 > /dev/null || (echo "Please install k6: https://k6.io/docs/getting-started/installation/" && exit 1)
	k6 run test/load/script.js

# Generate test coverage report
coverage: test
	@echo "Generating coverage report..."
	$(GOCMD) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_DIR)/coverage.html
	@echo "Coverage report: $(COVERAGE_DIR)/coverage.html"

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

# Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -w -s .
	@echo "Code formatting complete"

# Lint code
lint:
	@echo "Running linters..."
	@which $(GOLINT) > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GOLINT) run --timeout 5m
	@echo "Linting complete"

# Vet code
vet:
	@echo "Running go vet..."
	$(GOVET) ./...
	@echo "Vet complete"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR) $(DIST_DIR) $(COVERAGE_DIR) $(DB_DIR)
	@rm -f cpu.prof mem.prof
	@echo "Clean complete"

# Dependency management
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "Dependencies updated"

# Verify dependencies
deps-verify:
	@echo "Verifying dependencies..."
	$(GOMOD) verify
	@echo "Dependencies verified"

# Update dependencies
deps-update:
	@echo "Updating dependencies..."
	$(GOGET) -u ./...
	$(GOMOD) tidy
	@echo "Dependencies updated"

# Generate code (if needed for mocks, etc.)
generate:
	@echo "Generating code..."
	$(GOCMD) generate ./...
	@echo "Code generation complete"

# Docker targets
docker-build:
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-f deployments/docker/Dockerfile .
	@echo "Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

docker-push:
	@echo "Pushing Docker image..."
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	@echo "Docker image pushed: $(DOCKER_IMAGE):$(DOCKER_TAG)"

docker-run:
	@echo "Running Docker container..."
	docker run -d \
		--name $(BINARY_NAME) \
		-p 8080:8080 \
		-p 8081:8081 \
		-v $(PWD)/configs:/etc/discobox \
		-v $(PWD)/data:/var/lib/discobox \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

docker-stop:
	@echo "Stopping Docker container..."
	docker stop $(BINARY_NAME) || true
	docker rm $(BINARY_NAME) || true

# Kubernetes deployment
k8s-deploy:
	@echo "Deploying to Kubernetes..."
	kubectl apply -f deployments/k8s/
	@echo "Kubernetes deployment complete"

k8s-delete:
	@echo "Removing from Kubernetes..."
	kubectl delete -f deployments/k8s/
	@echo "Kubernetes resources deleted"

# Development setup
setup:
	@echo "Setting up development environment..."
	# Install tools
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/air-verse/air@latest
	go install github.com/swaggo/swag/cmd/swag@latest
	# Install dependencies
	$(MAKE) deps
	# Create directories
	mkdir -p $(BUILD_DIR) $(DIST_DIR) $(COVERAGE_DIR) data logs
	# Copy example configs
	cp -n configs/discobox.yaml.example configs/discobox.yaml || true
	@echo "Development environment ready"

# Generate API documentation
docs:
	@echo "Generating API documentation..."
	@which swag > /dev/null || go install github.com/swaggo/swag/cmd/swag@latest
	swag init -g $(MAIN_PATH)/main.go -o docs/api
	@echo "API documentation generated"

# Security scan
security:
	@echo "Running security scan..."
	@which gosec > /dev/null || go install github.com/securego/gosec/v2/cmd/gosec@latest
	gosec -fmt=json -out=security-report.json ./... || true
	@echo "Security scan complete: security-report.json"

# Performance profiling
profile:
	@echo "Running with profiling enabled..."
	$(BUILD_DIR)/$(BINARY_NAME) -cpuprofile=cpu.prof -memprofile=mem.prof

# Analyze CPU profile
profile-cpu:
	@echo "Analyzing CPU profile..."
	$(GOCMD) tool pprof -http=:8081 cpu.prof

# Analyze memory profile
profile-mem:
	@echo "Analyzing memory profile..."
	$(GOCMD) tool pprof -http=:8082 mem.prof

# Release targets
release: clean lint test build-all
	@echo "Creating release $(VERSION)..."
	@mkdir -p releases/$(VERSION)
	# Copy binaries
	cp -r $(DIST_DIR)/* releases/$(VERSION)/
	# Copy configs
	cp -r configs releases/$(VERSION)/
	# Copy documentation
	cp README.md LICENSE releases/$(VERSION)/
	# Create archives
	cd releases/$(VERSION) && tar -czf ../$(BINARY_NAME)-$(VERSION)-linux-amd64.tar.gz $(BINARY_NAME)-linux-amd64 configs README.md LICENSE
	cd releases/$(VERSION) && tar -czf ../$(BINARY_NAME)-$(VERSION)-darwin-amd64.tar.gz $(BINARY_NAME)-darwin-amd64 configs README.md LICENSE
	cd releases/$(VERSION) && zip -r ../$(BINARY_NAME)-$(VERSION)-windows-amd64.zip $(BINARY_NAME)-windows-amd64.exe configs README.md LICENSE
	@echo "Release $(VERSION) created"

# Install locally
install: build
	@echo "Installing $(BINARY_NAME)..."
	@sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	@sudo mkdir -p /etc/discobox
	@sudo cp configs/discobox.yaml /etc/discobox/
	@echo "Installation complete"

# Uninstall
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	@sudo rm -f /usr/local/bin/$(BINARY_NAME)
	@sudo rm -rf /etc/discobox
	@echo "Uninstallation complete"

# Validate configuration
validate:
	@echo "Validating configuration..."
	$(BUILD_DIR)/$(BINARY_NAME) -validate -config configs/discobox.yaml

# Generate self-signed certificates for testing
certs:
	@echo "Generating self-signed certificates..."
	@mkdir -p certs
	openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
		-keyout certs/key.pem -out certs/cert.pem \
		-subj "/C=US/ST=State/L=City/O=Discobox/CN=localhost"
	@echo "Certificates generated in certs/"

# Help target
help:
	@echo "Discobox Makefile"
	@echo "=================="
	@echo ""
	@echo "Build targets:"
	@echo "  make build          - Build for current platform"
	@echo "  make build-all      - Build for all platforms"
	@echo "  make run            - Build and run"
	@echo "  make dev            - Run with hot reload"
	@echo ""
	@echo "Test targets:"
	@echo "  make test           - Run all tests with coverage"
	@echo "  make test-unit      - Run unit tests only"
	@echo "  make test-integration - Run integration tests"
	@echo "  make test-e2e       - Run end-to-end tests"
	@echo "  make test-load      - Run load tests"
	@echo "  make coverage       - Generate coverage report"
	@echo "  make bench          - Run benchmarks"
	@echo ""
	@echo "Code quality:"
	@echo "  make fmt            - Format code"
	@echo "  make lint           - Run linters"
	@echo "  make vet            - Run go vet"
	@echo "  make security       - Run security scan"
	@echo ""
	@echo "Docker targets:"
	@echo "  make docker-build   - Build Docker image"
	@echo "  make docker-push    - Push Docker image"
	@echo "  make docker-run     - Run Docker container"
	@echo "  make docker-stop    - Stop Docker container"
	@echo ""
	@echo "Other targets:"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make deps           - Download dependencies"
	@echo "  make setup          - Setup development environment"
	@echo "  make docs           - Generate API documentation"
	@echo "  make release        - Create release artifacts"
	@echo "  make help           - Show this help"

# Set default goal
.DEFAULT_GOAL := help
