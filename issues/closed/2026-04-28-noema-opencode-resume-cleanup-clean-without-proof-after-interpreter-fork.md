# Noema OpenCode active resume cleanup fails without remote/process proof after Matrix fork interpreter

Status: closed
Reporter: Noema evaluation platform
Date: 2026-04-28

## Summary

Noema verified the new ACP live-attach policy and switched OpenCode `live_attach` to blocked. The valid OpenCode intervention path is now `active_learned_resume`.

A fresh Noema diagnostic run with embeddings-first routing reached the `active_learned_resume` path, used the Matrix fork interpreter, and then failed before resume completion because Matrix returned cleanup evidence with no remote or process proof:

`cleanup_clean_without_remote_or_process_proof`

This blocks Noema from treating the interrupt/resume path as safe in current Matrix.

## Environment

- Matrix version: `matrix 0.1.16-snapshot`
- Matrix commit: `fcd679a5225f0a4cfa459709cdd55092b44730e1`
- Matrix build: `2026-04-27T22:04:42Z`
- Agent: `opencode`
- Protocol: ACP
- Noema run mode: `active_learned_resume`
- Semantic provider: Ollama `embeddinggemma:latest`

## Noema Evidence

Artifact:

`/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-resume-embedding-diagnostic-after-matrix-fix-v1`

Command:

```bash
NOEMA_SEMANTIC_PROVIDER=ollama NOEMA_SEMANTIC_OLLAMA_MODEL=embeddinggemma:latest \
go run ./cmd/noema-eval run plan \
  --config-dir ./configs \
  --batch-plan ./artifacts/phase91-resume-embedding-diagnostic-after-matrix-fix-plan.json \
  --agents opencode \
  --arms active_learned_resume \
  --active-seed-records ./artifacts/phase91-experience-production-midlong-v5/runs/phase84-evidence-hardening-midrun-phase84-validation-backed-parser-warmup-001-opencode-active-cold-seed-7/execution-record.json \
  --matrix \
  --matrix-base-url http://127.0.0.1:9091 \
  --active-interpreter matrix_fork \
  --require-llm-guidance \
  --output-dir ./artifacts/phase91-resume-embedding-diagnostic-after-matrix-fix-v1
```

Noema-side key facts:

- Seed record loaded: `patterns=1`
- Current run semantic evidence was projected in the same space: `ollama:embeddinggemma:latest`
- Matrix fork interpreter capability reported `supported`, `stability=draft`
- Matrix fork interpreter sessions produced artifacts before the failure
- Run failed with: `active resume initial cleanup not clean: cleanup_clean_without_remote_or_process_proof`

Relevant record notes:

```text
active_sidecar_resume_initial_matrix_cleanup_proof=true
active_sidecar_resume_initial_matrix_cleanup_clean=false
active_sidecar_resume_initial_matrix_cleanup_strong=false
active_sidecar_resume_initial_matrix_cleanup_local_forgotten=true
active_sidecar_resume_initial_matrix_cleanup_remote_deleted=false
active_sidecar_resume_initial_matrix_cleanup_remote_closed=false
active_sidecar_resume_initial_matrix_cleanup_remote_canceled=false
active_sidecar_resume_initial_matrix_cleanup_process_reaped=false
active_sidecar_resume_initial_matrix_cleanup_process_retained=false
active_sidecar_resume_initial_matrix_cleanup_process_retention_allowed=false
active_sidecar_resume_initial_matrix_cleanup_strength=failed
active_sidecar_resume_initial_matrix_cleanup_process_retention_reason=no matching cached agent client
active_sidecar_resume_initial_matrix_cleanup_failure_code=cleanup_clean_without_remote_or_process_proof
```

Matrix runtime log excerpt:

```text
matrix session cleanup returned typed failure
failure_code=cleanup_clean_without_remote_or_process_proof
cleanup_strength=failed
clean=false
strong_cleanup=false
local_forgotten=true
remote_deleted=false
remote_closed=false
remote_canceled=false
process_reaped=false
process_retained=false
remote_session_id=ses_22efa06f3ffetBgqkKCIWmuD92
warnings=["remote_lifecycle_skipped_no_reusable_cached_agent_client"]

matrix async run cancelled
error="[agent_preflight_failed] agent provider preflight failed agent=opencode protocol=acp phase=session/prompt: ACP prompt failed: context canceled"
cleanup_clean=false
strong_cleanup=false
cleanup_strength=failed
failure_code=cleanup_clean_without_remote_or_process_proof
```

## Expected Contract

For Noema `active_learned_resume`, Matrix cleanup must prove at least one safe lifecycle outcome before Noema resumes:

- remote session canceled, closed, or deleted; or
- local agent process reaped; or
- explicit retained-process proof with safe retention reason.

If Matrix cannot prove any of those, returning `clean=false` with `cleanup_clean_without_remote_or_process_proof` is correct, but then current OpenCode `interrupt_resume` capability is not usable by Noema.

## Request

Please clarify and fix the OpenCode ACP cleanup path for interrupted/resumed runs after Matrix fork interpreter activity:

1. Ensure cleanup can locate the relevant cached agent client/session after fork interpreter turns.
2. If the agent process was killed, return `process_reaped=true` or equivalent proof.
3. If remote cancel happened, return `remote_canceled=true`.
4. If no proof is possible, keep returning typed failure and mark OpenCode `interrupt_resume` unsupported/blocked for this lifecycle.

Noema will keep failing closed until Matrix returns strong cleanup or explicit safe retained-process proof.

## Matrix Maintainer Response - 2026-04-28

Accepted and fixed.

The issue was a real lifecycle-evidence race, not a Noema false positive.
Runtime logs showed the relevant OpenCode ACP stdio process had already exited
with `signal: killed` and Matrix keepalive evicted the dead workspace client
before strict cleanup called `ReapAgentClient`. Cleanup then had only local
forget evidence and correctly failed closed with
`cleanup_clean_without_remote_or_process_proof`.

Matrix now preserves a short-lived, session-bound process tombstone when
keepalive evicts a dead local stdio ACP workspace client. Strict cleanup uses a
new remote-session-bound reap path, so `process_reaped=true` is returned only
when the tombstone or live cached client is known to match the target
`remote_session_id`. Mismatched sessions do not consume the proof and do not
close unrelated current clients.

Changed implementation:

- `middleware.AgentSessionClientReaper` adds strict `agent + workspace +
  remote_session_id` process proof.
- Router dead-client eviction records a bounded tombstone keyed by cached
  workspace client and tracked remote sessions.
- ACP clients expose tracked loaded remote sessions for proof binding.
- Session cleanup prefers the remote-session-bound reap path before falling
  back to the older workspace-only reaper.

Validation:

- `go test ./internal/providers/agents -run 'TestRouter_ReapAgentSessionClient|TestRouter_CheckAndReconnect' -count=1`
- `go test ./internal/logic/session -run 'TestSessionManager_EphemeralParentCleanupCascadesToForkChild|TestSessionManager_ForkInputCanCleanupChildAndRestoreParent' -count=1`
- `go test ./internal/providers/agents ./internal/logic/session ./internal/providers/runapi -count=1`
- `go test ./... -count=1`
- `go run ./scripts/code_governance.go`

Documentation updated:

- `docs/matrix_agent_communication_run_trace.md`
- `docs/matrix_protocol_neutral_runtime.md`
- `docs/matrix_live_context_interrupt_policy.md`
- `docs/governance/code_debt_register.md`

The OpenCode `active_learned_resume` path should no longer fail closed in this
specific race when the process was actually killed and keepalive observed it
before cleanup.
