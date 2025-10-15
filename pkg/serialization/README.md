# Protocol Buffer Code Generation Guide

This package provides serialization and deserialization APIs for the FairStruct Protocol Buffer message. The Go code (`v1.pb.go`) is generated from the Protocol Buffer definition (`v1.proto`).

## Creating and Managing pb.go Files

### Quick Start (Recommended)

Use the project Makefile from the root directory:

```bash
# Generate Protocol Buffer code
make proto

# Or build everything (includes proto generation)
make all
```

### Manual Setup (Optional)

If you prefer to generate proto files manually:

#### Step 1: Install Prerequisites

```bash
# Install Protocol Buffer compiler
# See installation guide: https://protobuf.dev/downloads/
# 
# Quick install options:
# - macOS: brew install protobuf
# - Ubuntu/Debian: apt install protobuf-compiler
# - Windows: Download from releases or use chocolatey: choco install protoc

# Install the Go Protocol Buffer plugin
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# Verify installation
protoc --version
which protoc-gen-go
```

#### Step 2: Generate Go Code from Proto Files

```bash
protoc --go_out=. --go_opt=paths=source_relative v1.proto
```
