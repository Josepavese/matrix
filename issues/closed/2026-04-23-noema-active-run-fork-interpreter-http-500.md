# Noema Phase 87: active-session fork interpreter returns HTTP 500 instead of typed capability/block evidence

## Context

Noema Phase 87 wires its experience-interpreter provider seam to Matrix `action=fork` as an opt-in diagnostic lane.

The target product flow is:

`active Matrix run -> Noema detects structural pressure -> Noema asks Matrix to fork the current parent session -> fork child renders a one-turn JSON guidance artifact -> Matrix restores/preserves parent -> Noema verifies artifact -> Noema delivers concise guidance to the active run`

Noema calls Matrix with the parent-safe contract already discussed:

- `action=fork`
- `target=<active logical parent session id>`
- `make_active=false`
- `restore_parent=true`
- `ephemeral=true`
- `cleanup_policy=delete_remote_or_cancel_and_forget_local`
- `input=<one-turn interpreter prompt>`

Matrix currently reports OpenCode fork capability as supported/draft:

- `matrix_fork_interpreter_supported=true`
- `matrix_fork_interpreter_status=supported`
- `matrix_fork_interpreter_stability=draft`
- `matrix_fork_interpreter_source=zed_acp_rfd_session_fork`

## Observed Failure

Real Noema evaluation run:

- repo: `/home/jose/hpdev/Libraries/noema`
- artifact dir: `/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase87-active-interpreter-matrix-fork-opencode-short-v1`
- plan: `programs/evaluation-platform/examples/live/phase72-active-sidecar-short-plan.json`
- command:

```bash
cd /home/jose/hpdev/Libraries/noema/programs/evaluation-platform
go run ./cmd/noema-eval run plan \
  --config-dir ./configs \
  --batch-plan ./examples/live/phase72-active-sidecar-short-plan.json \
  --agents opencode \
  --arms active_cold,active_learned \
  --matrix \
  --active-interpreter matrix_fork \
  --output-dir ./artifacts/phase87-active-interpreter-matrix-fork-opencode-short-v1
```

The active learned run passed validation and cleanup, but Matrix fork interpreter failed:

- Noema run id: `phase72-active-sidecar-short-phase72-active-short-slug-001-opencode-active-learned-seed-7`
- Matrix run id: `run-21b5762f-3f4a-459e-baaa-5f0d51bd0de3`
- logical parent session id: `40420a48-47bf-4b84-b2b1-4f5c932f9e73`
- remote parent session id in trace: `ses_24641ef0affeSXbNkwNTAOqdXm`
- cleanup: `clean=true`, `strong_cleanup=true`, `remote_canceled=true`, `process_reaped=true`
- validation: passed
- fork attempts: `2`
- fork accepted: `0`
- fork rejected: `2`
- fallback count: `2`
- reject reason:

```text
matrix fork interpreter action failed: matrix http status=500: Internal Server Error
```

Relevant execution record:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase87-active-interpreter-matrix-fork-opencode-short-v1/batch-execution.json
```

Relevant Matrix trace:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase87-active-interpreter-matrix-fork-opencode-short-v1/runs/phase72-active-sidecar-short-phase72-active-short-slug-001-opencode-active-learned-seed-7/matrix-trace.json
```

## Expected Behavior

Matrix should not return HTTP `500` for this path.

One of these contracts is needed:

1. Support active-parent fork artifact turns:
   - fork may run while parent session is active
   - child stays inactive
   - parent remains/restores active
   - child artifact is returned
   - child cleanup proof is returned if a child is created
   - no prompt/history replay is used

2. If active-parent fork is not safe, return typed blocked evidence:
   - HTTP `409` or `422`, not `500`
   - machine-readable code such as `active_parent_fork_unsupported`, `parent_session_busy`, or `active_session_fork_requires_idle_parent`
   - include parent preservation status and cleanup proof if anything was created

3. Expose capability truth before Noema calls the path:
   - e.g. `fork.active_parent_safe=true|false`
   - or `fork.requires_idle_parent=true|false`
   - ideally also `fork.artifact_turn=true|false`

## Why This Matters

Noema can use fork interpretation only if Matrix can make the capability truth explicit.

Without typed active-parent capability or blocked evidence, Noema can only treat `fork_interpreter` as diagnostic and must fall back to deterministic interpretation.

The current behavior is safe enough because Noema falls back and preserves validation/cleanup, but it prevents controlled efficacy evidence for the Matrix fork interpreter lane.

## Noema-Side Handling

Noema has kept the lane diagnostic:

- deterministic provider remains default
- `matrix_fork` is opt-in only
- fork errors fall back to deterministic guidance
- records preserve fork attempts/rejections/fallbacks
- market/product claims remain blocked until this path is clean

---

## Matrix Maintainer Response

Accepted.

The issue is aligned with Matrix's product contract: provider/session lifecycle
errors must be explicit typed evidence, not generic HTTP `500`, and channel or
supervisor callers must be able to inspect capability truth before choosing an
automation lane.

Root cause found in Matrix runtime logs:

```text
matrix session action failed: ACP prompt failed: client context cancelled
```

The failing path was not `session/fork` itself. Matrix created the fork child,
then the active parent run finished and its ephemeral cleanup reaped the shared
OpenCode ACP process while the child artifact turn was still in flight. The
child prompt then failed with a canceled ACP client context, and Matrix
propagated that as an untyped Go error, which the HTTP handler converted to
`500`.

Implemented Matrix-side changes:

- Ephemeral parent cleanup now retains the shared provider process when a fork
  child still references the same `agent_id + workspace_path`.
- Retention is explicit in cleanup proof through `process_retained=true`,
  `process_retention_allowed=true`, and
  `process_retention_reason=other local sessions still reference agent client`.
- Fork child artifact turn failures now return typed JSON with
  `error.code=fork_child_turn_failed` instead of HTTP `500`.
- Fork child cleanup failures now return typed JSON with
  `error.code=fork_child_cleanup_failed` instead of HTTP `500`.
- If a child was created, Matrix attempts detached bounded cleanup and includes
  any available `fork.cleanup` proof in the typed response.
- Fork capability descriptors now expose:
  `active_parent_safe`, `requires_idle_parent`, and `artifact_turn`.

Expected Noema behavior after this fix:

- If OpenCode ACP advertises fork support, Noema can treat
  `capabilities.session.fork.active_parent_safe=true` and
  `artifact_turn=true` as permission to try the active fork interpreter lane.
- If the artifact turn still fails, Noema receives machine-readable typed
  evidence instead of an opaque `500` and can keep deterministic fallback.
- If the fork child succeeds, Matrix returns `fork.artifact.content`, preserves
  or restores the parent active session, and includes child cleanup proof.

Verification added in Matrix:

- unit test: fork child turn failure returns typed error plus cleanup proof
- unit test: ephemeral parent cleanup does not reap process while fork child
  exists
- HTTP test: `fork_child_turn_failed` maps to `502 Bad Gateway` with typed body
- documentation updated in API/wiki/runtime docs
