# Noema post-task Matrix judge stalls after successful run cleanup

## Status

Closed.

## Reporter

Noema experience-layer evaluation.

## Summary

After Matrix fixed the retained-process cleanup inconsistency, the original run cleanup now looks coherent and strong. However, the same Noema non-interference reproduction still does not reach `execution-record.json` because the post-task Matrix judge session stalls after receiving the judge prompt.

This is a separate blocker from `2026-05-03-noema-opencode-run-cancelled-retained-process-no-record.md`.

## Reproduction Context

Matrix binary:

```text
matrix 0.1.16-snapshot
commit fcd679a5225f0a4cfa459709cdd55092b44730e1
built 2026-05-03T18:59:24Z
```

Matrix daemon:

```text
started Sun May 3 21:00:43 2026
```

Noema command:

```bash
env NOEMA_SEMANTIC_PROVIDER=ollama \
  NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest \
  NOEMA_OUTCOME_CRITIC_PROVIDER=auto \
  NOEMA_OUTCOME_CRITIC_MATRIX_URL=http://127.0.0.1:9091 \
  go run ./cmd/noema-eval run plan \
  --batch-plan examples/live/phase91-experience-normal-budget-fast-seed7-plan.json \
  --output-dir artifacts/phase91-experience-normal-budget-fast-seed7-v2 \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --parallelism 1 \
  --max-runs 1
```

Run directory:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-experience-normal-budget-fast-seed7-v2/runs/phase91-experience-normal-budget-fast-seed7-phase91-long-incident-runbook-001-opencode-active-cold-seed-7
```

## Positive Fix Evidence

The original run wrote `capture.json`. Final cleanup now reports:

```json
{
  "clean": true,
  "cleanup_strength": "strong",
  "strong_cleanup": true,
  "process_reaped": true,
  "process_retained": false,
  "related_sessions": null
}
```

So the retained-process cleanup inconsistency appears fixed for this reproduction.

## Remaining Failure

Noema still does not write:

```text
execution-record.json
```

Observed files after more than 17 minutes:

```text
capture.json
```

Processes still alive:

```text
noema-eval run plan
opencode acp
opencode acp
```

Matrix log shows the post-task judge started and then no terminal progress:

```text
21:10:08 routing channel input channel=noema-outcome-critic-c374f813471291c0 requested_agent=opencode input_len=8303
21:10:09 conversation client initialized cwd=.../workspace
21:10:40 session update received session=ses_210c07be7ffeS5Nq5xN09V1cHi update_type=user_message_chunk text_len=8303
```

No further Matrix log events appeared for that judge session during the observation window.

## Expected

The Matrix judge run should either:

- complete normally and let Noema write `outcome-critic-response.json` plus `execution-record.json`;
- fail with structured terminal status quickly enough that Noema can write a failed execution record;
- expose a bounded timeout/cleanup path for stuck ACP judge sessions.

## Actual

The judge session stays alive and Noema remains blocked waiting for Matrix/provider completion. This keeps the end-to-end Noema proof lane non-acceptable even though the previous cleanup inconsistency is fixed.

## Requested Fix

- Add terminal timeout/failure semantics for post-task judge Matrix runs or expose enough run status for clients to fail closed.
- Ensure stalled ACP sessions are cleaned or reported with explicit cleanup proof.
- Preserve the successful cleanup behavior from the original run.

## Maintainer Analysis

Accepted.

The retained-process cleanup bug is fixed for the primary run: the observed
`capture.json` now shows `clean=true`, `strong_cleanup=true`, and
`process_reaped=true`.

The remaining failure is a different class. The post-task judge used a normal
Matrix run against OpenCode ACP, the provider accepted the prompt, then emitted
no agent/tool progress for several minutes. Matrix eventually saw the HTTP/run
context cancel and returned a generic provider/context failure. That is not an
infinite Matrix daemon leak: at inspection time there were no surviving
`noema-eval run plan` or `opencode acp` processes, and Matrix readiness was
still `ready`. The product gap was that callers had no protocol-neutral idle
progress fuse distinct from the explicitly discouraged default hard turn
timeout.

## Resolution

Matrix now supports explicit run activity timeouts:

```json
{
  "activity_timeout_seconds": 300
}
```

This is an opt-in idle-progress watchdog, not a default absolute timeout. When
set, Matrix cancels the run with `stop_reason=activity_timeout` if no agent or
tool activity is observed through the run notifier for the configured duration.
The normal detached cleanup path still runs, so stalled ACP sessions can fail
closed with terminal run state and cleanup evidence.

No default timeout was added. Long autonomous agent turns remain valid unless
the caller opts into either `emergency_kill_seconds` or
`activity_timeout_seconds`.

Implementation:

- added `internal/logic/runactivity` watchdog;
- added `/v1/runs` request field `activity_timeout_seconds`;
- added runapi handling for sync, stream, and async activity timeout reporting;
- added runapi coverage for explicit activity timeout cancellation;
- updated timeout/recovery and API docs.

Validation:

- `go test ./internal/providers/runapi ./internal/providers/matrixapi ./internal/logic/sessioncleanup -count=1`
- `go run ./scripts/governance_check --manifest governance/manifest.toml`
- `go run ./scripts/code_governance.go --config code-governance.toml`
- `go test ./...`
