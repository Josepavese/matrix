# Noema diagnostic: Codex ACP fails on `gpt-5.5` while Codex CLI succeeds

Date: 2026-04-24

## Summary

During a Noema wiki-memory market-proof canary, Matrix launched Codex through
`codex-acp` and the run failed before task work started:

```text
stream disconnected before completion: The model `gpt-5.5` does not exist or you do not have access to it.
```

This does not match the local Codex CLI state. The same machine, user, config,
and even the same failed evaluation workspace can run `codex exec -m gpt-5.5`
successfully.

Noema did not modify Matrix source. This issue records installed-runtime
behavior observed through Matrix PAL logs and Noema artifacts.

## Why This Matters To Noema

Noema needs Codex and OpenCode as independent real-agent lanes for the
wiki-memory proof batch. OpenCode currently passes the canary, but Codex via
Matrix fails at provider startup/model access, before Noema can measure the
memory layer.

This should be treated as a provider capability/preflight mismatch, not as a
task failure and not as evidence against the memory system.

## Local Codex Evidence

Local config:

```text
~/.codex/config.toml
model = "gpt-5.5"
model_reasoning_effort = "xhigh"
```

Auth status:

```bash
codex login status
```

Observed:

```text
Logged in using ChatGPT
```

Direct Codex CLI probe from the same workspace that Matrix failed:

```bash
cd /home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase88-wikimemory-market-proof-agent-canary/runs/phase88-wikimemory-market-proof-agent-plan-phase88-memory-slug-warmup-001-codex-off-seed-7/workspace
timeout 90s codex exec -m gpt-5.5 "Respond with exactly OK"
```

Observed:

```text
OpenAI Codex v0.124.0 (research preview)
workdir: /home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase88-wikimemory-market-proof-agent-canary/runs/phase88-wikimemory-market-proof-agent-plan-phase88-memory-slug-warmup-001-codex-off-seed-7/workspace
model: gpt-5.5
provider: openai
approval: never
sandbox: danger-full-access
reasoning effort: xhigh
...
OK
```

So the user/account/Codex CLI path can access `gpt-5.5`.

## Matrix / Codex ACP Evidence

Installed Matrix resolved Codex to:

```text
/home/jose/.nvm/versions/node/v22.12.0/bin/codex-acp
```

Installed package versions:

```text
@openai/codex@0.124.0
@zed-industries/codex-acp@0.11.1
```

`codex-acp --help` confirms it reads `~/.codex/config.toml` and supports config
overrides:

```text
Override a configuration value that would otherwise be loaded from `~/.codex/config.toml`.
Examples: - `-c model="o3"`
```

Matrix runtime log:

```text
/home/jose/.local/share/matrix/logs/matrix-runtime.jsonl
```

Relevant log sequence:

```text
2026-04-24T10:40:03.597+02:00 agent stderr
agent="/home/jose/.nvm/versions/node/v22.12.0/bin/codex-acp"
stderr="... Reconnecting... 2/5 ... stream disconnected before completion: The model `gpt-5.5` does not exist or you do not have access to it."

2026-04-24T10:40:36.709+02:00 agent stderr
agent="/home/jose/.nvm/versions/node/v22.12.0/bin/codex-acp"
stderr="... Unhandled error during turn: stream disconnected before completion: The model `gpt-5.5` does not exist or you do not have access to it. Some(Other)"

2026-04-24T10:40:36.909+02:00 matrix run bridge failed
error="ACP prompt failed: RPC error -32603: Internal error (map[codex_error_info:other message:stream disconnected before completion: The model `gpt-5.5` does not exist or you do not have access to it.])"
```

Noema artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase88-wikimemory-market-proof-agent-canary/batch-execution.json
```

Relevant record:

```json
{
  "run_id": "phase88-wikimemory-market-proof-agent-plan-phase88-memory-slug-warmup-001-codex-off-seed-7",
  "agent_id": "codex",
  "arm_id": "off",
  "status": "failed",
  "error": "matrix http status=500: {\"run_id\":\"run-b14bc1d3-9722-4551-9241-19b22d94cac9\",\"status\":\"failed\",\"error\":\"ACP prompt failed: RPC error -32603: Internal error (map[codex_error_info:other message:stream disconnected before completion: The model `gpt-5.5` does not exist or you do not have access to it.])\"}"
}
```

Cleanup proof in the same failed run was strong and clean:

```json
{
  "clean": true,
  "strong_cleanup": true,
  "cleanup_strength": "strong",
  "remote_close_attempted": true,
  "remote_closed": true,
  "process_reap_attempted": true,
  "process_reaped": true,
  "local_forgotten": true
}
```

## Current Interpretation

The evidence points to a Matrix/Codex-ACP provider boundary mismatch, not a
general Codex access problem.

Possible causes to investigate:

- `codex-acp` uses a different Codex API/auth path than `codex exec`.
- `codex-acp` is reading the same config but hitting a model availability path
  that does not accept `gpt-5.5`.
- Matrix's Codex provider discovery/setup can mark Codex usable even when the
  ACP adapter cannot actually complete a one-turn prompt with the configured
  model.
- The error is returned as generic HTTP 500 / ACP internal error instead of a
  typed provider capability failure.

## Expected Contract

For Matrix as Noema's local PAL agent router:

- If `codex-acp` cannot run the configured Codex model, Matrix should expose a
  typed failure such as `provider_model_unavailable`,
  `provider_auth_mismatch`, or `agent_preflight_failed`.
- Matrix health/setup should distinguish `codex` CLI availability from
  `codex-acp` prompt capability.
- The failure should include non-secret diagnostic facts: resolved command,
  adapter version if available, requested model, auth method class if safely
  knowable, and whether the failure occurred during initialize, session/new, or
  session/prompt.
- Noema should be able to preflight the Codex ACP lane before launching a batch,
  rather than burning a real evaluation run and receiving generic HTTP 500.
- Cleanup must stay strict; the current strong cleanup proof is good and should
  be preserved.

## Requested Behavior

Please add or expose a Codex ACP capability/preflight path that proves the
selected Matrix Codex runtime can complete a minimal prompt with the configured
model.

Desired outcomes:

- `codex exec -m gpt-5.5` and Matrix `codex-acp` agree on model availability, or
  Matrix reports a clear typed mismatch.
- If this is a known `@zed-industries/codex-acp` limitation, Matrix should
  surface that limitation in agent capabilities/setup instead of allowing a
  normal run to start.
- If users need a Matrix-side model override for Codex ACP, the supported
  configuration path should be documented and observable.

## Acceptance Criteria

- Running a minimal Matrix HTTP `/v1/runs` request against `agent_id=codex` with
  the configured `gpt-5.5` either succeeds or fails with typed provider
  capability evidence instead of generic HTTP 500.
- The failure mode is machine-readable enough for Noema to mark Codex memory
  proof as blocked/preflight-failed rather than task-failed.
- No secret values are logged.
- Existing cleanup evidence remains strong and explicit.

Until this is resolved or classified, Noema should avoid treating Codex Matrix
failures on this lane as memory-layer evidence.

## Matrix Maintainer Response

Accepted and implemented.

Matrix now treats this class of failure as a provider boundary/preflight problem,
not as a generic task failure.

Implemented behavior:

- ACP prompt/setup failures are classified at the provider boundary.
- Model availability/access errors such as `The model ... does not exist or you
  do not have access to it` become `provider_model_unavailable`.
- Auth/access failures can be surfaced as `provider_auth_mismatch`.
- Generic adapter readiness failures can be surfaced as `agent_preflight_failed`.
- Sync and stream `/v1/runs` error responses now include machine-readable
  `code` and safe `details`.
- Provider failures emit a `provider.preflight.failed` run event with safe
  diagnostics: `agent_id`, `protocol`, `phase`, `requested_model`, `adapter`,
  `transport`, command/address when available, and no secret values.
- HTTP status for model/auth provider readiness failures is `424 Failed
  Dependency`, so external evaluators can classify the lane as blocked instead
  of task-failed.

Recommended Noema preflight:

Use the canonical Matrix `/v1/runs` surface before a batch, with a minimal prompt
and `session_policy=new_ephemeral_delete_after_run`. This exercises the same
runtime path as production traffic and still produces cleanup evidence.

Example expected blocked response:

```json
{
  "status": "failed",
  "code": "provider_model_unavailable",
  "details": {
    "agent_id": "codex",
    "protocol": "acp",
    "phase": "session/prompt",
    "requested_model": "gpt-5.5",
    "adapter": "codex-acp",
    "transport": "stdio"
  }
}
```

Verification:

- Added provider failure classifier tests.
- Added `/v1/runs` typed provider failure test.
- Full `scripts/deploy_preflight.sh` passed.

Follow-up evidence after archive:

- `npm view @zed-industries/codex-acp version` reports `0.11.1`.
- Reinstalling with `npm install -g @zed-industries/codex-acp@latest` keeps
  `@zed-industries/codex-acp@0.11.1`, so the local adapter was already current.
- A real Matrix `/v1/runs` preflight against `agent_id=codex` still fails with
  `The model 'gpt-5.5' does not exist or you do not have access to it`.
- The same machine and workspace still succeed with
  `codex exec -m gpt-5.5 "Respond with exactly OK."`.
- The observed failure remains an ACP adapter/provider-boundary mismatch, not a
  general Codex login failure.

Closed: 2026-04-24.
