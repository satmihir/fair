# Makefile for Fair Project

.PHONY: proto build test clean

# Regenerate Protocol Buffer code. Run this only for an intentional schema
# change, with protoc 3.20.3 and protoc-gen-go v1.36.10: CI compares the
# checked-in file byte for byte, and other versions produce different output.
# build and test use the checked-in file and do not depend on this target.
proto:
	@echo "Generating Protocol Buffer code..."
	@cd internal/serialization && protoc --go_out=. --go_opt=paths=source_relative v1.proto
	@echo "✅ Proto generation completed"

# Build the project
build:
	@echo "Building..."
	@go build ./...
	@echo "✅ Build completed"

# Run tests
test:
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
all: build test
