# Linting and Static Enforcement

Use linting not only for style, but to enforce architecture boundaries.

## Baseline Rule

- Treat warnings as failures in CI for architecture-related checks.
- Keep a small allowlist for temporary migrations and expire it with date + owner.

## Recommended Tooling by Language

| Language | Linter / Static Analyzer | Notes |
| :--- | :--- | :--- |
| **Python** | `Ruff` (+ `mypy` where typing exists) | Ban forbidden imports in logic modules. |
| **JS/TS** | `Biome` or `ESLint` (+ `tsc`) | Use import-boundary plugins/rules. |
| **Go** | `golangci-lint` | Add `depguard` for forbidden imports by package path. |
| **Rust** | `clippy` | Enforce module boundaries via crate layout and lint groups. |
| **Java/Kotlin** | `SpotBugs` / `Detekt` | Use architectural package rules. |
| **C#/.NET** | Roslyn analyzers | Add dependency and layering analyzers. |
| **C/C++** | `clang-tidy` | Combine with directory-level ownership conventions. |

## Architecture-Specific Checks

For Platform Abstraction Layer projects, enforce:

1. Logic layer must not import platform/system APIs.
2. Logic layer must not branch on runtime platform.
3. Provider layer must not import domain/business modules.
4. Middleware is the only layer allowed to resolve provider selection.
5. No direct calls from Logic to Provider packages.

## Practical Patterns

### 1) Forbidden Imports by Path

- Define forbidden lists per directory.
- Example policy:
  - `logic/**` forbids `os`, `syscall`, shell/process packages.
  - `provider/**` forbids `domain/**` imports.

### 2) Runtime Branch Guard

- Reject `if os == ...` checks in logic modules.
- Allow platform checks only in middleware/provider selection files.

### 3) Contract Stability Guard

- Add CI diff check on interface/contract packages.
- Require test updates when contracts change.

## CI Gate Order (Suggested)

1. Format check
2. Lint/static checks
3. Architecture boundary checks
4. Unit tests by layer
5. Contract/provider integration tests

Fail fast at step 2 or 3 when layering rules are violated.

## Reporting Template for Violations

When a violation is found, report:

- File and line.
- Violated rule (e.g., “Logic imported platform API”).
- Why it breaks PAL design.
- Required fix path (move to middleware/provider, add contract).
