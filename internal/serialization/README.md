# Protocol Buffer Code Generation Guide

**Status: internal and not wired to the tracker.** This package lives under `internal/` so that it is not part of the module's public API. Nothing in the tracker reads or writes these messages yet, and the schema predates two changes to the data structure, so revise it before using it:

- Buckets are placed with SipHash-2-4 under a 128-bit key. The schema still describes `MURMURHASH_32` with a 32-bit seed.
- A bucket's last-update time is a signed count of milliseconds since its structure was built, not an unsigned wall-clock timestamp.

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
