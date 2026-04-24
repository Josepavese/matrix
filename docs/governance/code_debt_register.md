# Code Debt Register

This register tracks allowed governance debt. It is not permission to grow debt.

Baseline date: 2026-04-24.

Baseline command:

```bash
go run ./scripts/code_governance.go --config code-governance.toml
```

Current allowed baseline:

- Total quality warnings: 17
- Maximum branch points in one function: 16
- Hard budget failures: 0

Latest reduction:

- 2026-04-24: extracted session action rendering into `internal/logic/sessionview`.
- 2026-04-24: split session mirror removal into smaller state mutation helpers.
- Result: warning count reduced from 19 to 17; maximum branch points reduced from 20 to 16.

Ratchet:

- `code-governance.toml` fails when total quality warnings exceed the baseline.
- `code-governance.toml` fails when maximum branch complexity exceeds the baseline.
- Lowering either number is encouraged after refactors.
- Raising either number is a governance change and requires documented maintainer approval.

Primary debt cluster:

- `internal/logic/session`: session lifecycle, cleanup, routing, workspace reads, handoff, and intent handling.
- `internal/logic/onboarding`: wizard step handlers.
- `internal/logic/workspace`: timeline event recording.

Reduction strategy:

- Extract pure helpers from high-branch functions.
- Keep protocol and channel behavior unchanged.
- Prefer lower branch count without moving complexity into equally large replacement functions.
- After each reduction, lower the warning budget baseline.
