# Commit b3f27f2 Decomposition

Purpose: make the pushed broad commit reviewable and auditable without rewriting
the already-published `main` history.

Commit:

- `b3f27f2 chore: sync ACP protocol and session cleanup`

Logical slices:

1. ACP protocol drift sync
   - `pkg/zedacp/*`
   - `internal/providers/agents/acp_*`
   - `docs/matrix_zed_acp_protocol_tracking.md`
   - related ACP compliance docs

2. Protocol-neutral request shape
   - `internal/middleware/agent.go`
   - `internal/middleware/protocol.go`
   - `internal/providers/a2aclient/adapter.go`
   - `internal/providers/agents/router.go`

3. Session cleanup and fork cleanup proof
   - `internal/logic/session/manager_cleanup*`
   - `internal/logic/session/manager_fork*`
   - `internal/logic/sessioncleanup/*`
   - `tests/integration/opencode_run_owned_fork_cleanup_test.go`

4. Provider client failure eviction and tombstone evidence
   - `internal/providers/agents/provider_failure*`
   - `internal/providers/agents/router_failure_eviction.go`
   - `internal/providers/agents/router_tombstones.go`
   - related router recovery tests

5. Run API and notifier evidence
   - `internal/providers/runapi/*`
   - `internal/logic/runnotifier/*`
   - `internal/logic/runreconcile/*`
   - `docs/matrix_agent_communication_run_trace.md`

6. Issue evidence archive
   - `issues/closed/2026-05-*`

Validation baseline after the broad commit:

- `go test ./...`

Follow-up rule:

- Do not add new unrelated work to this broad commit by history rewrite.
- Make every correction as a focused follow-up commit with a narrower subject.
- If a future release needs selective rollback, revert by logical slice where
  possible and use this document as the review map.

