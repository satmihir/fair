# Protocol Buffer Code Generation Guide

This package provides serialization and deserialization APIs for the FairStruct Protocol Buffer message. The Go code (`v1.pb.go`) is automatically generated from the Protocol Buffer definition (`v1.proto`).

## Creating and Managing pb.go Files

### Step 1: Install Prerequisites

Before generating Protocol Buffer code, ensure you have the necessary tools:

```bash
# Install Protocol Buffer compiler
# See installation guide: https://protobuf.dev/downloads/
# 
# Quick install options:
# - macOS: brew install protobuf
# - Ubuntu/Debian: apt install protobuf-compiler
# - Windows: Download from releases or use chocolatey: choco install protoc
# - Or download pre-compiled binaries from GitHub releases

# Install the Go Protocol Buffer plugin
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# Verify installation
protoc --version
which protoc-gen-go
```

### Step 2: Generate Go Code from Proto Files

```
protoc --go_out=. --go_opt=paths=source_relative v1.proto
```
