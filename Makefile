.PHONY: help test coverage test-verbose test-specific lint fmt build clean examples

# Use Go 1.25.0
GO := ~/go/bin/go1.25.0

help:
	@echo "Flagmint Go SDK - Development Tasks"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  test              Run all tests"
	@echo "  test-verbose      Run all tests with verbose output"
	@echo "  test-transport    Run transport package tests"
	@echo "  test-cache        Run cache package tests"
	@echo "  test-evaluate     Run evaluate package tests"
	@echo "  coverage          Generate coverage report"
	@echo "  coverage-html     Generate and open HTML coverage report"
	@echo "  lint              Run go vet linter"
	@echo "  fmt               Format code with gofmt"
	@echo "  build             Build the SDK"
	@echo "  examples          Run example programs"
	@echo "  clean             Remove build artifacts and coverage files"
	@echo ""

# Test targets
test:
	@echo "Running tests..."
	@$(GO) test ./...

test-verbose:
	@echo "Running tests with verbose output..."
	@$(GO) test -v ./...

test-transport:
	@echo "Running transport tests..."
	@$(GO) test -v ./transport

test-cache:
	@echo "Running cache tests..."
	@$(GO) test -v ./cache

test-evaluate:
	@echo "Running evaluate tests..."
	@$(GO) test -v ./evaluate

# Coverage targets
coverage:
	@echo "Generating coverage report..."
	@$(GO) test -coverprofile=coverage.out ./...
	@$(GO) tool cover -func=coverage.out
	@echo ""
	@echo "Run 'make coverage-html' to view detailed report"

coverage-html: coverage
	@echo "Opening coverage report in browser..."
	@$(GO) tool cover -html=coverage.out -o coverage.html
	@open coverage.html

# Code quality targets
lint:
	@echo "Running go vet..."
	@$(GO) vet ./...

fmt:
	@echo "Formatting code..."
	@$(GO) fmt ./...
	@gofmt -s -w .

# Build target
build:
	@echo "Building SDK..."
	@$(GO) build ./...

# Examples
examples:
	@echo "Available examples:"
	@echo "  make example-basic      - Run basic example"
	@echo "  make example-local-eval - Run local evaluation example"
	@echo "  make example-debug      - Run debug example (shows initialization behavior)"

example-basic:
	@echo "Running basic example..."
	@$(GO) run ./examples/basic/main.go

example-local-eval:
	@echo "Running local evaluation example..."
	@$(GO) run ./examples/local-eval/main.go

example-debug:
	@echo "Running debug example..."
	@$(GO) run ./examples/debug/main.go

# Cleanup
clean:
	@echo "Cleaning up..."
	@$(GO) clean
	@rm -f coverage.out coverage.html
	@find . -type f -name '*.test' -delete
	@echo "Done!"

# Default target
.DEFAULT_GOAL := help
