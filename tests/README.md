# Matrix - Testing Strategy

Matrix strictly enforces a hybrid two-tiered testing methodology to ensure OS-level stability while maintaining cross-layer (PAL) validation.

## Directory Structure

- `/tests/integration/`: For integration tests like TCP JSON-RPC bindings or FUSE mount verification across multiple internal packages.
- `/tests/e2e/`: For CLI integration tests spanning from the Cobra input all the way down to the output (powered by `testscript`).
- `/tests/mocks/`: Auto-generated mocks (via tools like GoMock) simulating the `middleware` PAL layer to completely isolate the `logic` layer during unit tests.
- `/tests/fixtures/`: Contains generated `bbolt` database files, sample YAML configs, and other static data required for testing edge cases uniformly.

## Guidelines

1. **Unit Tests (`*_test.go`)** must remain adjacent to their source files (e.g. `/internal/providers/bolt/bolt_test.go`).
2. Never leak Mock objects into the `internal/` or `pkg/` trees.
