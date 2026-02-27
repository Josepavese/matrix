# Layer / Middleware / Stack Architecture

This document defines a strict, project-agnostic architecture for cross-platform and system-level components.

## Conceptual Flow

```mermaid
graph TD
    User[User / API / CLI] --> Logic[Logic Layer<br/>(Agnostic Business Logic)]
    Logic -- "Semantic Operation" --> Middleware[Middleware/Abstraction<br/>(Contract + Router)]
    Middleware -- "Select Provider Strategy" --> Stack{Provider Layer}
    Stack -->|Linux| LinuxProvider[Linux Adapter]
    Stack -->|Windows| WinProvider[Windows Adapter]
    Stack -->|macOS| MacProvider[Darwin Adapter]
    
    LinuxProvider --> OS[Operating System]
    WinProvider --> OS
    MacProvider --> OS
```

## Layers Defined

### 1. Logic Layer
- **Responsibility**: Execute use-cases, business invariants, and orchestration.
- **Knowledge**: Knows **what** must happen.
- **Must not know**: OS, drivers, shell commands, platform SDK specifics.
- **Imports**: Domain types + middleware contracts only.

### 2. Middleware (Abstraction) Layer
- **Responsibility**: Define contracts, select providers, normalize errors, enforce fallback policy.
- **Knowledge**: Knows available adapters and runtime capability checks.
- **Action**: Route semantic call to selected provider.
- **Example**: `OpenExternalURL(url)` delegates to platform-specific provider.

### 3. Stack (Provider) Layer
- **Responsibility**: Execute concrete platform operations.
- **Knowledge**: Knows deeply about one platform or one technology boundary.
- **Isolation**: Keep provider-specific code in dedicated modules/files (build tags or adapter packages).
- **Example**: filesystem, process execution, registry, kernel or device APIs.

## Responsibility Matrix

| Concern | Logic | Middleware | Provider |
|---|---|---|---|
| Business rules | Owner | No | No |
| Platform routing | No | Owner | No |
| Fallback policy | No | Owner | No |
| Syscalls/SDK/device access | No | No | Owner |
| Error normalization | Consumer only | Owner | Source |
| Feature capability checks | Optional consumer | Owner | Source |
| Platform retries/timeouts | No | Optional orchestration | Owner |

## Guidelines

1. **Dependency direction**: `Logic -> Middleware -> Provider` only.
2. **Interface ownership**: Middleware defines interfaces; providers implement them.
3. **No leakage**: Never expose provider-specific errors/types to logic directly.
4. **Single routing point**: Keep provider selection in one module.
5. **Deterministic contracts**: Document sync/async behavior and retries per operation.
6. **Capability-based design**: Expose optional features through explicit capability checks.

## Anti-Patterns

- Scattered `if platform == ...` checks in business modules.
- Business modules importing syscall/process/platform packages.
- Providers importing domain/business packages.
- Middleware containing business logic unrelated to routing/normalization.
- Silent fallback without logging or metrics.
