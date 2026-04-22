# Noema Phase84: Codex ACP processes retained after parallel batch completion

Date: 2026-04-22

## Context

Noema executed a Matrix-backed Phase84 long-run batch:

- Noema artifact: `/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase84-evidence-hardening-longrun-v4`
- Plan: `phase84-evidence-hardening-longrun-plan.json`
- Agents: `codex`, `opencode`
- Arms: `active_cold`, `active_learned_resume`
- Seeds: `7`, `11`, `19`
- Parallelism: `4`
- Matrix service: PAL runtime, HTTP at `127.0.0.1:9091`

The batch completed and wrote `36/36` execution records plus `batch-execution.json`.

## Observation

After Noema's `noema-eval run plan` process exited, two Codex ACP processes were still alive:

```text
node .../bin/codex-acp
.../codex-acp-linux-x64/bin/codex-acp
node .../bin/codex-acp
.../codex-acp-linux-x64/bin/codex-acp
```

The corresponding Noema evidence report now surfaces this correctly:

```text
Cleanup proof runs: 36/36
Clean cleanup runs: 36/36
Strong cleanup proof runs: 34/36
Process-retained cleanup runs: 2
```

The retained records are:

```text
phase84-evidence-hardening-longrun-phase84-validation-backed-parser-warmup-001-codex-active-cold-seed-7
phase84-evidence-hardening-longrun-phase84-validation-backed-parser-warmup-001-codex-active-learned-resume-seed-11
```

Both reported:

```text
clean=true
local_forgotten=true
process_retained=true
process_retention_reason="other local sessions still reference agent client"
```

## Why Noema Cares

For Noema market evidence, `clean=true` is not enough to claim strong isolation.

Noema can accept `process_retained=true` as a weaker cleanup state and now records it as a claim limiter, but it needs Matrix to make the lifecycle unambiguous:

- if process retention is expected while sibling sessions are active, the retained process should be reaped when the last referencing session closes
- if process retention is a daemon-level optimization, Matrix should expose that as explicit lifecycle semantics separate from session cleanup
- if processes remain after all runs in a batch are done, Matrix should expose a way to reconcile or reap retained agent clients safely

## Desired Outcome

Please evaluate whether this is expected Matrix behavior.

If expected:

- document `process_retained=true` semantics for parallel ACP sessions
- expose whether the retained process is still allowed to carry agent-side conversation state
- expose a way for callers to request strict batch isolation or post-batch reap

If not expected:

- ensure retained ACP processes are reaped after the final local session referencing the agent client is cleaned up
- preserve cleanup proof fields so Noema can distinguish strong isolation from weaker retention

## Noema Current Mitigation

Noema has already been hardened to avoid overclaiming:

- evidence reports count `proof_runs`, `clean_runs`, `strong_proof_runs`, and `process_retained_runs`
- process-retained cleanup adds a market-claim limit
- official evidence does not treat retained-process cleanup as strong isolation proof

## Matrix maintainer response

Status: accepted and implemented.

Matrix now treats process retention as explicit weak cleanup evidence, not as
strong isolation proof.

Semantics:

- `process_retained=true` means the provider client was intentionally kept
  because another local Matrix session still references the same
  `agent_id + workspace_path`.
- For non-ephemeral shared sessions this can remain `clean=true`, but Matrix
  reports `cleanup_strength=retained` and
  `weak_cleanup_reason=process_retained`.
- For ephemeral interrupt/resume runs, local-only or retained-only cleanup is
  not enough; strong proof is required.
- Strong proof remains one of `remote_deleted`, `remote_closed`,
  `remote_canceled`, or `process_reaped`.

Matrix also exposes a post-batch reconcile operation:

```json
{
  "channel_id": "batch-or-supervisor",
  "action": "reconcile"
}
```

`reconcile` closes cached provider clients that no longer have any Matrix vault
session reference and returns `reconcile.reaped` / `reconcile.retained`.

Validation:

- Unit test coverage added for retained cleanup semantics and client reconcile.
- Real OpenCode fork cleanup showed retained process while parent still existed,
  then strong cleanup after parent close.
- Real `reconcile` was executed against the installed daemon and returned typed
  retained/reaped evidence.
