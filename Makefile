.PHONY: build test lint clean run

# Variables
BINARY_NAME=github-renovate-updater
GO=go
GOFMT=gofmt
GOLINT=golangci-lint

# Build the application
build:
	$(GO) build -o $(BINARY_NAME) ./cmd/github-renovate-updater

# Run tests
test:
	$(GO) test -v ./...

# Run tests with coverage
test-coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out

# Run linter
lint:
	$(GOLINT) run

# Format code
fmt:
	$(GOFMT) -w .

# Clean build artifacts
clean:
	$(GO) clean
	rm -f $(BINARY_NAME)
	rm -f coverage.out

# Run the application
run:
	$(GO) run ./cmd/github-renovate-updater

# Install dependencies
deps:
	$(GO) mod download

# Install development tools
install-tools:
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Default target
all: deps lint test build 
