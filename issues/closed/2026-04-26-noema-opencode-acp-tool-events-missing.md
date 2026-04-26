# Noema experience run: OpenCode ACP edits workspace but Matrix trace has no tool events

## Summary

Noema Phase 91 non-interference experience testing found a Matrix/OpenCode observability gap: the agent modified a writable Go workspace and reported running `go test ./...`, but the Matrix run trace exposed only agent message events and no provider-neutral tool/edit/shell events.

This blocks Noema's experience layer from learning or applying live structural priors. The run can be judged successful after completion, but the active sidecar sees no structural pressure such as read/edit/execute/test-failure/test-success.

## Evidence

Artifact:

`/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-non-interference-coding-pressure-cold-warm-v1`

Batch command:

```sh
NOEMA_OUTCOME_CRITIC_PROVIDER=auto \
NOEMA_SEMANTIC_PROVIDER=ollama \
NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest \
NOEMA_SEMANTIC_PROFILE=embeddinggemma_local_text_300m \
go run ./cmd/noema-eval run plan \
  --config-dir ./configs \
  --batch-plan ./examples/live/phase91-experience-coding-pressure-cold-warm-plan.json \
  --agents opencode \
  --arms active_cold,active_learned \
  --matrix \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --output-dir ./artifacts/phase91-non-interference-coding-pressure-cold-warm-v1
```

Runs:

- `phase91-experience-coding-pressure-cold-warm-phase91-go-checkout-summary-001-opencode-active-cold-seed-7`
- `phase91-experience-coding-pressure-cold-warm-phase91-go-checkout-summary-001-opencode-active-learned-seed-7`

Both runs:

- `status=succeeded`
- `proof_mode=non_interference`
- `validation=emergent.observation`
- `outcome_critic_provider=noema.outcomecritic.matrix_judge`
- `outcome_critic_intent_satisfied=yes`
- `outcome_critic_risk=low`
- `matrix_cleanup.cleanup_strength=strong`

The isolated workspaces prove real file mutation and passing tests:

```sh
cd /home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-non-interference-coding-pressure-cold-warm-v1/runs/phase91-experience-coding-pressure-cold-warm-phase91-go-checkout-summary-001-opencode-active-cold-seed-7/workspace
go test ./...
# ok checkoutsummary

cd /home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-non-interference-coding-pressure-cold-warm-v1/runs/phase91-experience-coding-pressure-cold-warm-phase91-go-checkout-summary-001-opencode-active-learned-seed-7/workspace
go test ./...
# ok checkoutsummary
```

However the Matrix traces contain no tool events:

```text
active_cold matrix-trace.json
40 agent.message.delta
1  agent.message.final
1  agent.prompt.sent
1  routing.decision
1  run.completed
1  run.started
1  session.cleanup
1  session.created
1  session.policy.applied

active_learned matrix-trace.json
18 agent.message.delta
1  agent.message.final
1  agent.prompt.sent
1  routing.decision
1  run.completed
1  run.started
1  session.cleanup
1  session.created
1  session.policy.applied
```

Agent final messages claim the agent ran the expected command, and the workspace content confirms the code fix. Matrix still does not expose any `tool.call.requested`, `tool.result.received`, edit, shell, or test execution events for these runs.

## Expected Matrix behavior

For ACP/OpenCode runs where the workspace is changed or shell commands are executed, Matrix should emit provider-neutral structural events such as:

- `tool.call.requested` with `tool_kind=edit|execute|read`
- `tool.result.received` with status/result evidence
- safe path/artifact refs where available
- command/test execution status when the provider exposes it

Raw provider payloads do not need to be leaked. Noema needs stable structural event evidence, not raw text.

## Impact on Noema

Noema correctly loaded `1` learned pattern in the warm run, but generated `0` suggestions because the active timeline contained no structural pressure beyond agent deltas/final output.

This makes the current run valid as post-task judge evidence, but not valid active-sidecar efficacy evidence. Without tool events, Noema cannot:

- learn reliable action sequences from traces;
- detect repeated execution/edit/test pressure;
- trigger Matrix fork/interpreter guidance at the right time;
- prove live agent consumption of experience guidance.

## Request

Please verify whether Matrix is dropping ACP/OpenCode tool-call updates for this route, or whether the provider is mutating the workspace through a path that Matrix does not currently project into the public run trace.

The acceptance condition for Noema is: a normal Matrix/OpenCode coding task that edits a file and runs tests must expose provider-neutral tool/edit/execute events in `/v1/runs/{run_id}/trace` or `/events` without requiring Noema to parse agent prose.

---

## Matrix maintainer response

Accepted and implemented.

Root cause confirmed: real OpenCode ACP emits `tool_call` / `tool_call_update`
updates with empty text content. Matrix previously forwarded tool updates only
when `content.text` was non-empty, so the public run trace lost structural tool
pressure even though ACP updates arrived.

Changes made:

- metadata-only ACP tool updates are now forwarded when `title`, `toolCallId`,
  `kind`, `status`, `rawInput`, locations, or other metadata exist;
- `/v1/runs/{run_id}/trace` and `/events` now receive neutral
  `tool.call.requested` / `tool.result.received` events for those updates;
- ACP client-side tool requests handled by Matrix (`fs/read_text_file`,
  `fs/write_text_file`, `terminal/create`) are also projected as neutral
  read/edit/execute events;
- raw file content is not copied into tool metadata; `fs/write_text_file`
  redacts the `content` field from stored raw input;
- docs updated in `docs/matrix_agent_communication_run_trace.md`,
  `docs/matrix_protocol_neutral_runtime.md`, and
  `docs/wiki/API-Reference.md`;
- optional real smoke added for OpenCode ACP:
  `TestSmoke_OpenCodeWS_ProjectsToolEvents`.

Evidence:

- `go test ./...` passed;
- `go run ./scripts/code_governance.go --config code-governance.toml` passed;
- real OpenCode ACP stdio smoke passed with actual empty-content `tool_call`
  events observed, a real file edit, `go test ./...`, and trace-level
  structural tool assertions.

Note: OpenCode ACP WebSocket on the installed `opencode 1.4.1` rejected the
tested WebSocket upgrade paths (`/` and `/acp`) with `unexpected EOF`. The real
provider validation therefore used OpenCode ACP stdio, which is the configured
Matrix default for OpenCode in this repo.
