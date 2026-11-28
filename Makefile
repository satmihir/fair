# Makefile for Fair Project

.PHONY: proto build test clean

# Generate Protocol Buffer code
proto:
	@echo "Generating Protocol Buffer code..."
	@cd internal/serialization && protoc --go_out=. --go_opt=paths=source_relative v1.proto
	@echo "✅ Proto generation completed"

# Build the project
build: proto
	@echo "Building..."
	@go build ./...
	@echo "✅ Build completed"

# Run tests
test: proto
	@echo "Running tests..."
	@go test -v ./...
	@echo "✅ Tests completed"
	@golangci-lint run ./...
	@echo "✅ Linting completed"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@go clean ./...
	@rm -f fair coverage.out
	@echo "✅ Clean completed"

# Default target
all: proto build test
