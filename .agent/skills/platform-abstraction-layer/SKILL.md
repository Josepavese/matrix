---
name: platform-abstraction-layer
description: Enforce a strict Platform Abstraction Layer architecture (Logic -> Middleware -> Provider) for any code that touches OS, drivers, filesystem, process execution, networking primitives, or hardware. Use when implementing, refactoring, or reviewing cross-platform/system-level components and when isolating responsibilities between business logic and platform-specific adapters.
---

# Platform Abstraction Layer

Apply this skill to keep code **agnostic**, **modular**, and **portable** across platforms and runtime environments.

## Core Principle

Keep business logic unaware of platform details.

- Do not branch on OS in business code.
- Do not call syscalls, shell commands, device APIs, or platform SDKs from logic code.
- Express intent as semantic operations and delegate execution to abstractions.

## Mandatory Layer Model

All platform interactions must flow through exactly three layers:

1. **Logic Layer (What)**
   - Own use-cases, orchestration, invariants, and business decisions.
   - Consume interfaces only.
   - Forbidden: OS checks, platform imports, direct hardware/process/filesystem calls.

2. **Middleware/Abstraction Layer (How to Adapt)**
   - Define interfaces (ports/contracts).
   - Resolve and route providers (runtime selection, feature flags, fallback strategy).
   - Normalize error model and return semantic errors to logic layer.

3. **Provider/Stack Layer (How it Works)**
   - Implement interfaces using concrete platform APIs.
   - Scope each provider to one platform/technology boundary.
   - Keep translation details local to provider.

## Responsibilities by Concern

For each concern, assign one owner layer:

- **Business rules and invariants** -> Logic.
- **Provider selection and fallback policy** -> Middleware.
- **Protocol/SDK/syscall/device details** -> Provider.
- **Cross-platform contract shape** -> Middleware.
- **Data validation at domain boundaries** -> Logic + Middleware.
- **Platform-specific retries/timeouts/flags** -> Provider (exposed as semantic options via Middleware).

## Dependency Direction (Strict)

Dependencies are one-way:

`Logic -> Middleware (interfaces/router) -> Provider`

Never allow reverse imports. Providers must not import domain/business packages.

## Contract Design Rules

Define interfaces around **intent**, not implementation details.

- Good: `ConfigStore.Save(config)`, `PlatformClock.Now()`, `ProcessRunner.Run(commandSpec)`.
- Bad: `RunBash(command)`, `ReadRegistryKey(path)` in domain-facing interfaces.

When defining contracts:

1. Use semantic names and domain-level types.
2. Expose deterministic behavior and explicit error contracts.
3. Keep sync/async behavior documented and consistent.
4. Add capability discovery when not all providers support a feature.

## Runtime Selection Rules

In Middleware:

1. Build provider registry at startup.
2. Select provider via environment/runtime/capability checks in one place.
3. Apply fallback policy explicitly.
4. Emit structured logs for selected provider and fallback events.

## Refactor Workflow (Existing Code)

Use this sequence when migrating legacy code to layered architecture:

1. Identify all platform-coupled call sites in business modules.
2. Group call sites by capability domain (filesystem, process, network, hardware, clock, env).
3. Define middleware contracts per capability domain.
4. Implement provider adapters per platform/technology.
5. Replace direct calls in logic with contract usage.
6. Centralize provider selection in one router/factory.
7. Normalize provider errors into semantic errors.
8. Add tests by layer (logic, middleware, provider, contract).
9. Enforce with linters/static checks and block regressions.

## Deliverables for Any Change

Every significant PAL change should include:

1. Updated contracts or provider routing notes.
2. Tests proving routing + behavior + error normalization.
3. Review evidence that no platform logic leaked into business modules.
4. Lint/static-analysis pass output.

## Testing Strategy by Layer

1. **Logic tests**: mock interfaces, assert business behavior only.
2. **Middleware tests**: verify routing, fallback, error normalization.
3. **Provider tests**: integration/contract tests per platform.
4. **Contract tests**: shared suite each provider must pass.

## Common Violations (Reject in Review)

- `runtime.GOOS`, `syscall`, shell invocations, or driver calls in Logic.
- `if platform == ...` spread across business modules.
- Leaking provider-specific errors into Logic.
- One provider implementing multiple unrelated concerns with hidden state.
- Direct imports from Provider to Logic or domain package.

## Minimal Example

```go
// Logic
type ConfigService struct{ fs FileSystem }
func (s ConfigService) Save(path string, data []byte) error {
    return s.fs.Write(path, data)
}

// Middleware contract
type FileSystem interface {
    Write(path string, data []byte) error
}

// Provider (Linux)
type LinuxFS struct{}
func (LinuxFS) Write(path string, data []byte) error {
    return os.WriteFile(path, data, 0o644)
}
```

## Review Checklist

Before finalizing any change:

1. Verify Logic has no platform branches/imports.
2. Verify Middleware is the only routing point.
3. Verify Provider scope is single-platform/single-technology.
4. Verify errors are normalized before returning to Logic.
5. Verify tests cover contract + routing + provider behavior.
6. Verify linting/static analysis passes (see references).

## References

- `references/architecture.md` for diagrams and expanded layer responsibility matrix.
- `references/linters.md` for strict linting baseline.
- `references/refactor-playbook.md` for step-by-step migration patterns and checklists.
