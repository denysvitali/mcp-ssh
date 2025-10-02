.PHONY: build test lint fmt vet clean install run help coverage test-race

BINARY_NAME=mcp-ssh
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

# Default target
.DEFAULT_GOAL := help

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME) version $(VERSION)..."
	go build $(LDFLAGS) -o $(BINARY_NAME)

## test: Run tests
test:
	@echo "Running tests..."
	go test -v ./...

## test-race: Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	go test -v -race ./...

## coverage: Generate test coverage report
coverage:
	@echo "Generating coverage report..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## lint: Run linter
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed. Install from https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run

## fmt: Format code
fmt:
	@echo "Formatting code..."
	goimports -w . || gofmt -s -w .

## vet: Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

## clean: Remove build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-*
	rm -f coverage.out coverage.html
	rm -f *.log

## install: Install binary to GOPATH/bin
install: build
	@echo "Installing to $(GOPATH)/bin/..."
	cp $(BINARY_NAME) $(GOPATH)/bin/

## run: Build and run with default settings
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BINARY_NAME) --allowed-hosts "localhost" --log-level debug

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod verify

## tidy: Tidy go.mod
tidy:
	@echo "Tidying go.mod..."
	go mod tidy

## build-all: Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY_NAME)-linux-arm64
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY_NAME)-darwin-arm64
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-windows-amd64.exe

## check: Run all checks (fmt, vet, lint, test)
check: fmt vet lint test
	@echo "All checks passed!"

## release-dry-run: Test release process without publishing
release-dry-run:
	@echo "Running GoReleaser in dry-run mode..."
	@which goreleaser > /dev/null || (echo "goreleaser not installed. Install from https://goreleaser.com/install/" && exit 1)
	goreleaser release --snapshot --clean --skip=publish

## release-snapshot: Build snapshot release locally
release-snapshot:
	@echo "Building snapshot release..."
	@which goreleaser > /dev/null || (echo "goreleaser not installed. Install from https://goreleaser.com/install/" && exit 1)
	goreleaser release --snapshot --clean

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'
