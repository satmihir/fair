# AGENTS.md

This file is a guide for AI agents working on the FAIR repository. It outlines the project structure, development workflows, and coding standards that must be followed.

## Project Overview

FAIR is a Go library for fairness in resource-constrained environments. It uses a Stochastic Fair BLUE algorithm and multi-level Bloom Filters to distribute resources evenly.

**Key Goal**: Ensure fairness without over-allocation or starvation.

## Directory Structure & Architecture

The codebase is organized as follows:

- **`pkg/`**: Contains the core library code.
    - **`tracker/`**: The main entry point and logic for the fairness tracker.
    - **`config/`**: Configuration structures and defaults.
    - **`data/`**: Underlying data structures (e.g., Bloom Filters).
    - **`serialization/`**: Protobuf definitions and generated code.
    - **`request/`**: Request and response models.
    - **`logger/`**: Logging interface and default implementations.
    - **`integration/`**: Integration tests.
- **`designs/`**: Design documents and templates.
- **`mutations/`**: Mutation testing resources, including diffs and drivers.
- **`tasks.go`**: A Go script for running maintenance tasks (like linting).
- **`Makefile`**: Standard build and test commands.

## Development Workflow

You must use the following commands to build, test, and verify your work.

### Build and Test

- **Generate Protobufs**: `make proto`
    - Run this if you modify any `.proto` files in `pkg/serialization`.
- **Build**: `make build`
    - Compiles the project.
- **Test**: `make test`
    - Runs unit tests (`go test -v ./...`) and linting (`golangci-lint run`).
    - **Always run this before submitting changes.**
- **Clean**: `make clean`
    - Removes build artifacts.

### Linting & Static Analysis

Linting is strictly enforced in the CI pipeline. You must ensure all checks pass locally.

- **Command**: `golangci-lint run ./...` (or via `make test`)
- **Configuration**: Rules are defined in `.golangci.yml`.

**Key Enforced Rules**:
- **`revive`**: Enforces Go best practices (variable naming, error handling, etc.).
- **`misspell`**: Enforces US English spelling (e.g., "color" instead of "colour").
- **`gci`**: Enforces import order (Standard > Default > Local).
- **`errcheck`**: Ensures all errors are handled.

## Coding Standards

- **Style**: Follow standard Go formatting (`go fmt`).
- **Imports**: Group imports correctly (Standard Library, Third Party, Local).
- **Context**: Pass `context.Context` as the first argument to functions where applicable.
- **Errors**:
    - Use `fmt.Errorf` instead of `errors.New(fmt.Sprintf(...))`.
    - Error variables should be prefixed with `err`.
    - Handle all errors explicitly.

For more details on contributing, refer to [CONTRIBUTING.md](./CONTRIBUTING.md).

## Common Tasks for Agents

1.  **Modifying Logic**:
    - If you modify `pkg/tracker` or `pkg/data`, ensure you add/update tests in the corresponding `_test.go` files.
    - Run `make test` to verify no regressions.

2.  **Updating Protobufs**:
    - Edit `pkg/serialization/v1.proto`.
    - Run `make proto` to regenerate code.
    - Commit the generated files.

3.  **Refactoring**:
    - Ensure `golangci-lint` passes after refactoring.
    - Watch out for `revive` warnings regarding variable naming and error handling.

## Maintenance

This file should be updated whenever:
- The directory structure changes.
- New tools or linter rules are added.
- The build or test commands change.

Please keep this guide up-to-date to ensure it remains a single source of truth for agents.
