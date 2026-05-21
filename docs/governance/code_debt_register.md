# Code Debt Register

This register tracks allowed governance debt. It is not permission to grow debt.

Baseline date: 2026-04-24.

Baseline command:

```bash
go run ./scripts/code_governance.go --config code-governance.toml
```

Current allowed baseline:

- Total quality warnings: 0 observed; enforcement ceiling is 1 because the checker treats 0 as disabled
- Maximum branch points in one function: 10
- Hard budget failures: 0

Latest budget realignment:

- 2026-04-27: complexity-warning cleanup split branch-heavy session and
  workspace timeline code into smaller helpers. Quality warnings dropped to 0
  and maximum branch points dropped to 10. Package LOC overrides increased only
  to account for the helper split; this is a temporary maintainability
  realignment, not permission for feature growth.
- 2026-04-27: run-level ephemeral cleanup gained related-session accounting so
  a run cannot report isolated strong cleanup while a touched parent/fork
  session remains alive. The implementation was split across focused run API
  cleanup files; package LOC overrides increased for `internal/providers/runapi`
  and `internal/middleware`, with no new function/file hard failures.
- 2026-05-04: fork subtree cleanup was tightened so parent cleanup reconciles
  child proofs after parent process reap, and run cleanup no longer double-cleans
  fork children already covered by `fork_children`. This removes false retained
  related-session failures without weakening fail-closed semantics. Package LOC
  overrides were raised only for `internal/logic/session`,
  `internal/providers/runapi`, and the ACP router tombstone path in
  `internal/providers/agents`; quality warnings remain 0 and maximum branch
  points stay within budget.
- 2026-05-08: ACP prompt-cancellation recovery added router-side eviction of
  poisoned workspace clients plus remote-session tombstone proof. The
  `internal/providers/agents` package LOC override was raised to the measured
  baseline because this is production cleanup/recovery logic, not optional
  compatibility code. Follow-up refactor target: split ACP client lifecycle,
  prompt execution, and cleanup proof ownership into smaller subpackages once
  the provider surface stabilizes.
- 2026-05-08: ACP cleanup proof now separates target remote-session cleanup
  from unrelated same-workspace provider ownership. A cleanup with strong target
  proof records other local owners as non-retained
  `shared_agent_client_owner` evidence instead of returning unsafe
  `process_retained=true`; workspace-only reaps also cannot consume explicit
  remote-session tombstones.
- 2026-05-08: `/v1/runs` cancellation cleanup now binds ephemeral policy routes
  to the prepared logical session and follows a late-selected remote session if
  provider startup races with cancel. This prevents stale cleanup proofs with an
  empty `remote_session_id` from poisoning the next same-provider request.
- 2026-05-04: supervisor-facing cleanup now fails standalone retained
  fork-child cleanup with `run_related_session_retained` instead of exposing
  `clean=true cleanup_strength=retained` to HTTP/run consumers. Retained cleanup
  remains an internal/shared-session state, not production-safe proof.
- 2026-05-04: forced cleanup of run-owned ephemeral fork children can now
  remediate parent-client retention from child provider proof while preserving
  the parent owner for active-resume restore. Parent-owner subtree cleanup is a
  fallback for missing child proof, and final run cleanup remains responsible
  for closing or reaping the shared workspace client.
- 2026-05-04: standalone fork-child cleanup now treats a verified live parent
  owner as non-retained when the child mirror is forgotten and provider
  lifecycle is quiesced. For stdio ACP this includes the explicit
  `remote_lifecycle_skipped_no_reusable_cached_agent_client` proof, avoiding
  fresh cleanup-only provider spawns while still failing closed for orphan
  parent ownership. The `internal/logic/session` LOC budget was raised only for
  this tested product surface; branch complexity and quality warnings remain
  inside governance limits.
- 2026-05-04: run cleanup related-session ownership now treats same-agent
  sessions created inside the run channel as cleanup-owned even when a
  fork/resume path has not flagged them ephemeral yet. Reconcile evidence is
  scoped to the run agent/workspace and retained in-scope clients carry
  logical/remote/workspace ownership details; unrelated retained clients no
  longer make a completed run fail with misleading proof.
- 2026-05-05: Noema-like final resume cleanup coverage added scoped
  `agent_id + workspace_path` reconcile, richer retained-client proof, stdio
  cleanup-only spawn prevention, shared-client remote-session tombstone
  preservation, and ACP prompt/load observer drain for real providers that emit
  trailing `session/update` chunks after `session/prompt` returns. Package LOC
  overrides were raised only to the measured baseline for
  `internal/providers/agents`, `internal/providers/runapi`, and `pkg/zedacp`;
  quality warnings remain 0 and maximum branch points remain within budget.
- 2026-05-08: ACP provider clients were moved to router-owned lifetime instead
  of per-request lifetime, preventing `/v1/runs` cancellation from killing a
  cached stdio ACP process before cleanup/resume proof can complete. Remote
  lifecycle lookup now evicts and tombstones a dead exact workspace client so a
  later strict cleanup can consume matching remote-session process proof. The
  `internal/providers/agents` LOC override was raised to the measured baseline;
  quality warnings remain 0 and maximum branch points remain within budget.
- 2026-04-27: async fork jobs added pollable live-sidecar orchestration state, channel command parity, and a stricter fork capability contract.
- 2026-04-27: ACP prompt concurrency guard added one-active-prompt-per-remote-session enforcement and live-attach rejection for active ACP turns. Package/file LOC overrides increased only for `internal/providers/agents` and `internal/middleware`; quality warnings remain 0 and maximum branch points remain 10.
- 2026-04-28: local stdio ACP cleanup gained session-bound process tombstones
  for dead clients evicted by keepalive before cleanup. The tombstone is
  short-lived and remote-session-matched, so it strengthens cleanup evidence
  without allowing unrelated client/process proof to satisfy an ephemeral run.
- 2026-05-04: `pkg/zedacp` budget covers Zed ACP schema v0.12.2
  compliance tracking: `additionalDirectories`, prompt message ids,
  `session/list` filters/cursors, stable `session/resume`, stable
  `session/close`, stable `session/set_config_option`, and explicit
  documentation that ACP has no `side` primitive. This keeps the ACP package as
  a standalone protocol facade and avoids pushing protocol fields into Matrix
  runtime code.
- 2026-05-21: ACP drift review updated the tracked schema baseline to
  `agent-client-protocol` v0.13.2. `additionalDirectories` now matches the
  current unstable schema (`new`/`load`/`resume`/`fork` plus `SessionInfo`,
  not `session/list`) and remains capability-gated in Matrix runtime.
- 2026-05-04: ACP provider budgets were ratcheted to the real stable-surface
  cost after implementation: stable `session/resume`, stable `session/close`,
  stable `session/set_config_option`, structured updates, official terminal
  lifecycle, and three-provider real smoke coverage. Complexity remains clean:
  quality warnings are 0 and maximum branch points stay within budget.
- Result: package LOC overrides increased only for documented feature surfaces; hard failures remain 0, total quality warnings stay at 0, and maximum branch points stay at 10.

Latest reduction:

- 2026-04-27: split workspace routing, workspace binding, remote cleanup,
  fork preparation, intent workspace resolution, workspace read resolution,
  timeline event recording, and cleanup finalization into smaller helpers.
- Result: warning count reduced from 10 to 0; maximum branch points reduced
  from 13 to 10.
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

Budget notes:

- 2026-04-27: `internal/logic/session` budget was raised to cover async fork
  jobs plus fork-subtree cleanup accounting. This is product surface, not legacy
  compatibility; the cleanup finalization logic was split into helpers to avoid
  increasing function-level branch debt.

Reduction strategy:

- Extract pure helpers from high-branch functions.
- Keep protocol and channel behavior unchanged.
- Prefer lower branch count without moving complexity into equally large replacement functions.
- After each reduction, lower the warning budget baseline.
