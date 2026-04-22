# Noema request: expose provider-neutral structural tool kinds for active sidecar traces

## Context

Noema Phase 77 now compiles active-sidecar learned suggestions from typed Matrix structure only:

- event kind
- event status
- provider-neutral tool kind
- run/agent/family/task scope
- validation truth

Noema deliberately does **not** parse prompt prose, command strings, file names, locale-specific words, or provider-specific text to infer meaning.

During the Phase 77 Codex holdout rerun, the integration worked end-to-end:

- artifact: `/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase77-active-sidecar-structural-codex-holdout`
- status: `succeeded`
- duration: `297.951s`
- active sidecar: `patterns_available=1`, `suggestions_generated=2`, `suggestions_resumed=2`
- validation: passed
- cleanup: clean

The learned suggestion content was correctly structural, but most useful tool events were classified as `tool_kind=other`.

Example observed Matrix trace shape:

```json
{
  "kind": "tool.call.requested",
  "status": "pending",
  "tool_kind": "other",
  "tool_name": "run_go_test_./..."
}
```

Noema could infer that this is probably an execution/validation action from the `tool_name`, but doing so would violate the project rule: no lexical, command-name, natural-language, or provider-string heuristics.

## Request

Please expose a richer provider-neutral structural tool taxonomy in Matrix events, derived from adapter/protocol semantics rather than string parsing in downstream consumers.

Desired event contract:

```json
{
  "kind": "tool.call.requested",
  "status": "pending",
  "tool_kind": "execute",
  "tool_semantic_kind": "validation",
  "tool_effect": "read_write_or_execute",
  "tool_subject_kind": "repository",
  "tool_name": "..."
}
```

The exact field names are up to Matrix. Noema only needs stable structural fields that do not require inspecting `tool_name`.

## Suggested taxonomy

Minimum useful set:

- `read`: file/list/search/read-only repository inspection
- `write`: file modification or patch application
- `execute`: shell/process execution
- `validate`: tests, build, lint, validation scripts, benchmark checks
- `vcs`: git status/diff/log/branch operations
- `network`: network/API/browser/web operations
- `agent`: provider/session/control operations
- `unknown`: explicitly unknown, not a catch-all for known executable operations

Optional orthogonal fields:

- `tool_effect`: `read_only`, `write`, `execute`, `control`, `unknown`
- `tool_subject_kind`: `filesystem`, `repository`, `process`, `network`, `agent_session`, `unknown`
- `tool_confidence`: numeric or enum if classification is adapter-derived but imperfect
- `classification_source`: `adapter`, `protocol`, `provider_metadata`, `fallback`

## Why this matters

Noema active-sidecar suggestions now include structural fields like:

```text
validated_routine: tool_call(other) -> tool_result(other) -> repeated_tool_use(other) -> run_completed
validated_pressure: repeated_tool_use(other)x27 -> repeated_tool_use(read)x7
current_pressure: repeated_tool_use(other)
```

This is truthful, but too weak for high-quality learned guidance.

If Matrix emits `validate`, `execute`, `read`, `write`, and `vcs` structurally, Noema can produce stronger learned routines such as:

```text
validated_routine: read -> execute -> write -> validate -> run_completed
validated_pressure: repeated_tool_use(validate)
current_pressure: repeated_tool_use(validate)
```

That remains meaning-only and protocol-derived, without Noema parsing natural language or command strings.

## Acceptance criteria

- Matrix events expose stable structural tool classification richer than `other`.
- Classification is produced by Matrix adapters/protocol metadata, not by downstream Noema prompt or command-string parsing.
- Existing `tool_kind` remains backward compatible or a new field is added beside it.
- Noema can consume the taxonomy without provider-specific branches for Codex/OpenCode/Gemini/Claude.

## Matrix maintainer response

Status: accepted and implemented.

Matrix now emits structural tool taxonomy fields on run trace tool events:

- `tool_kind`
- `tool_semantic_kind`
- `tool_effect`
- `tool_subject_kind`
- `tool_classification_source`
- `tool_classification_confidence`

`tool_kind` is aligned with the official Zed ACP enum: `read`, `edit`,
`delete`, `move`, `search`, `execute`, `think`, `fetch`, `switch_mode`,
`other`. Provider/protocol metadata wins over heuristics. Heuristic fallback is
still available for weak providers, but it is explicitly marked
`tool_classification_source=heuristic_fallback` and
`tool_classification_confidence=low`, so Noema can reject or downrank it without
provider-specific parsing.

Implementation points:

- `pkg/zedacp` now parses ACP `kind`, `status`, `toolCallId`, `rawInput`, and
  `locations` from `session/update`.
- Matrix ACP observer forwards those native fields as protocol metadata.
- `frontendevents.NormalizeTool` produces the provider-neutral structural
  fields.
- `runtrace.Event` stores the fields as first-class trace data.
- Redacted trace projection removes all tool structural fields.

Validation:

- Unit coverage added for protocol-kind precedence, semantic/effect/subject
  propagation, and low-confidence fallback.
- `go test ./...` passes.
- Deploy preflight, lint, tests, GoReleaser snapshot, and local PAL install pass.
