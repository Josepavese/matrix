# Platform Abstraction Refactor Playbook

Use this playbook when converting existing code to `Logic -> Middleware -> Provider`.

## 1. Scope and Inventory

1. List all modules in the target feature.
2. Mark platform-coupled calls:
   - filesystem
   - process execution
   - sockets/network primitives
   - environment/runtime inspection
   - hardware/device APIs
3. Group call sites by capability domain.

Deliverable:
- A migration map: `call_site -> capability -> target contract`.

## 2. Define Contracts First

For each capability domain:

1. Create one middleware interface.
2. Prefer semantic method names.
3. Define error semantics (typed errors or canonical error codes).
4. Keep method signatures stable and testable.

Example (semantic):
- `CredentialStore.Save(token)`
- `Clock.Now()`
- `ProcessRunner.Run(spec)`

## 3. Implement Providers

1. Add provider implementations per platform/technology.
2. Keep provider modules focused and minimal.
3. Convert low-level errors to middleware-level errors where possible.
4. Add provider integration tests for critical paths.

## 4. Add Middleware Router

1. Register providers in one place.
2. Select provider via environment/capability policy.
3. Implement explicit fallback behavior.
4. Emit structured logs with chosen provider + fallback reason.

## 5. Replace Legacy Calls in Logic

1. Replace direct platform calls with middleware contracts.
2. Remove platform branches from business modules.
3. Keep orchestration and business decisions unchanged.

## 6. Test and Enforce

1. Logic tests with mocks only.
2. Middleware tests for routing/fallback.
3. Provider tests per platform.
4. Contract tests shared across providers.
5. Add lint rules to prevent regressions.

## 7. Migration Checklist

- [ ] No platform imports in logic modules.
- [ ] No runtime platform branching in logic modules.
- [ ] Providers do not import domain/business modules.
- [ ] Middleware is single source of provider selection.
- [ ] Errors normalized before they reach logic.
- [ ] Layered test suite present and passing.
- [ ] CI boundary checks enabled.

## 8. Pull Request Template (Recommended)

Include in every PAL-related PR:

1. Layer boundary changes.
2. New/changed contracts.
3. Provider changes by platform.
4. Tests added/updated by layer.
5. Lint boundary checks status.
6. Known risks and rollback notes.
