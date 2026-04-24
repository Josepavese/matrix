# Code Debt Register

This register tracks allowed governance debt. It is not permission to grow debt.

Baseline date: 2026-04-24.

Baseline command:

```bash
go run ./scripts/code_governance.go --config code-governance.toml
```

Current allowed baseline:

- Total quality warnings: 12
- Maximum branch points in one function: 13
- Hard budget failures: 0

Latest reduction:

- 2026-04-24: split session cancel target resolution out of the typed cancel handler without changing remote/local cancel semantics.
- Result: warning count reduced from 13 to 12; maximum branch points remains 13.
- 2026-04-24: split wizard agent selection and auth method handling into focused helpers.
- Result: warning count reduced from 16 to 13; maximum branch points reduced from 14 to 13.
- 2026-04-24: typed provider readiness failures were extracted into `internal/logic/providerfailure`, keeping run API and middleware budgets clean.
- 2026-04-24: split remote-session import into mirror lookup, metadata construction, and persistence helpers.
- Result: warning count reduced from 17 to 16; maximum branch points reduced from 16 to 14.
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
- `internal/logic/workspace`: timeline event recording.

Reduction strategy:

- Extract pure helpers from high-branch functions.
- Keep protocol and channel behavior unchanged.
- Prefer lower branch count without moving complexity into equally large replacement functions.
- After each reduction, lower the warning budget baseline.
