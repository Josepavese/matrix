# Required ACP session mode: deterministic tools-free inference for one role

This document records the neutral contract a consumer uses when one role must run
inference **without native tools announced to the model**, how Matrix enforces and
verifies it, and what remains the provider's own guarantee. It is the Matrix side
of `issues/closed/2026-10-25-external-vicky-json-planning-tool-permission-denied.md`: a
planning turn that must produce JSON from typed data was interrupted by a denied
native tool call, and a suggestion in the prompt cannot deterministically prevent
one.

The contract is a session mode, not a prompt instruction and not a permission
policy. Matrix selects the provider's own mode, reads back the provider's own
statement that the mode is in effect, and refuses the session when it cannot.

## Public contract

One environment variable on an **agent endpoint**:

```
MATRIX_ACP_SESSION_MODE=<mode id>
```

| Property | Rule |
| --- | --- |
| Scope | The agent whose endpoint declares it. Matrix launches the provider per run/session, so the declaration is role-scoped by construction. It is never inferred from the daemon's environment, the agent's name, or the prompt. |
| Protocol | ACP endpoints only. Declaring it on a non-ACP endpoint fails the launch. |
| Cardinality | Exactly one entry per endpoint. A duplicate declaration is refused, not resolved to "last wins". |
| Value | `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`. The value is the provider's exact mode identifier, never a prefix, a display name, or a normalized guess. |
| Conflict | A launch that also declares the Codex launch-policy contract (`MATRIX_CODEX_LAUNCH_POLICY_CONTRACT`) is refused: one launch has one owner of the session mode. |
| Enforcement | `required`. There is no silent fallback to another mode and no downgrade to "build". |
| Failure | Before the prompt. An unavailable, unconfirmed, mismatched or rejected mode fails the session; the error names the mode. |

The declaration is validated when the endpoint is resolved, before any provider
process is spawned. Resolution records the contract in run evidence (see below)
with `verified: false`, because a launch declaration is not provider proof.

## What Matrix does on every session

For every ACP session Matrix creates (`session/new`) or reattaches
(`session/resume`, `session/load`), the adapter:

1. reads the mode contract from the resolved endpoint;
2. selects the surface the provider actually exposes:
   - a config option whose category or id is `mode` (ACP `session/set_config_option`), or
   - the session-mode surface (`session/set_mode`, the ACP v1 shape);
3. applies the exact mode when the session does not already report it;
4. **verifies** the application from a provider statement:
   - config option: the current value in the `session/set_config_option` response (or, when the session already reports the mode, the session state itself);
   - session modes: the `current_mode_update` the provider reports after `session/set_mode`, within a bounded window, because the `session/set_mode` acknowledgement carries no state;
5. fails closed, before the prompt, when the mode is not advertised by the
   surface, the apply call fails, the provider's response states another mode, or
   no confirmation arrives.

Applying on **resume** is not redundant. OpenCode 1.18.31 was observed to reset
the session mode to `build` on `session/resume` even after a successful
`session/set_config_option` on the same session (see probe evidence below). This
is provider-observed behaviour, not a generic ACP guarantee; Matrix therefore
re-applies and re-verifies the required mode on every new and resumed session
rather than trusting session state it did not just read.

### Fail-closed cases

| Situation | Result |
| --- | --- |
| Contract value malformed, empty or duplicated | Launch rejected during endpoint resolution |
| Contract declared on a non-ACP endpoint | Launch rejected |
| Neutral contract and Codex launch policy both declared | Launch rejected as a conflict |
| Provider session exposes no such mode | Session fails, error names the mode |
| Provider is asked to set the mode and returns an error | Session fails, error names the mode and carries the provider error |
| Provider returns state that reports another mode | Session fails, error names the required and reported modes |
| Session-mode surface returns no `current_mode_update` | Session fails as unconfirmed (the acknowledgement alone is not evidence) |
| Prompt | Never sent when the mode step failed |

## Trace evidence

Two different facts are recorded, and they are deliberately not mixed:

1. **Launch contract** — the `routing.decision` event carries
   `protocol_meta.agent_launch_policy.session_mode`:
   `configured` (the mode id), `enforcement: "required"`,
   `application_mechanism: "acp-session-mode"`,
   `verification_status: "pending_provider_application"`, `verified: false`.
   A launch declaration is an input, never a confirmation.
2. **Provider confirmation** — the adapter records `session_mode` in the ACP
   result metadata only from a provider statement it actually read:
   `configured_mode`, `effective_mode`, `config_id` (for a config option),
   `evidence_source` (`session_state`, `session/set_config_option` or
   `session/set_mode+current_mode_update`), `verification:
   "provider_confirmed"`, `enforcement: "required"`. On the turn path this
   metadata travels with the turn result and the session manager merges it into
   the session's remote metadata. The materialize-without-turn path returns it to
   the caller; the session fork-materialize flow currently persists only
   title/status from that metadata, so a consumer that forks a session reads the
   evidence from the materialize result, not from the persisted session record.

The existing **model attestation** (`model.selection`, `configured_model`,
`effective_model`, `provider_confirmed`) is not touched: a session mode is not a
model, and recording one never makes a claim about the other.

## OpenCode 1.18.31 recipe

OpenCode turns a custom primary agent into an ACP session mode option. Give the
agent endpoint this declaration (role-scoped, not global):

```
OPENCODE_CONFIG_CONTENT={"agent":{"json-planning":{
  "mode":"primary",
  "description":"JSON planning without tools",
  "tools":{"bash":false,"edit":false,"read":false,"write":false,"glob":false,
           "grep":false,"patch":false,"webfetch":false,"todowrite":false,
           "todoread":false,"task":false,"skill":false,"question":false,"list":false}}}}
MATRIX_ACP_SESSION_MODE=json-planning
```

Every key in `agent.<id>.tools` must be `false`, and the id in
`MATRIX_ACP_SESSION_MODE` must be exactly the agent id. Matrix selects that mode
on every session it creates or resumes and records the provider's confirmation.
The provider's own tool set is what removes the tools from its model request;
Matrix verifies the mode, not the request.

### Probe evidence (workstation, OpenCode 1.18.31, ACP v1)

- `initialize` negotiates `protocolVersion: 1`; the provider advertises
  `sessionCapabilities.resume/list/close/fork`.
- `session/new` returns
  `configOptions: [{id:"model", category:"model"}, {id:"mode", category:"mode",
  currentValue:"build", options:[build, "json-planning", plan]}]`: the custom
  agent is a real mode option with the description Matrix declared, and it is not
  a Matrix-local concept.
- `session/set_config_option {configId:"mode", value:"json-planning"}` returns
  `configOptions` with `currentValue:"json-planning"`: the provider's own
  confirmation, which is the evidence the adapter records.
- `session/resume` for the same session returns `currentValue:"build"` again:
  OpenCode does not persist the mode across resume, which is why Matrix
  re-applies and re-verifies it on resume.
- `opencode debug agent json-planning --tool bash --params '{"command":"echo hi"}'`
  answers `Tool bash is disabled for agent json-planning`: the tools-free agent
  exists independently of Matrix, and the mode maps to it.
- Matrix-side opt-in test: `tests/integration/acp_session_mode_real_test.go`
  (`MATRIX_SMOKE_TEST=1 MATRIX_OPENCODE_ACP_STDIO=1 go test ./tests/integration
  -run TestOpenCodeRequiredSessionModeAppliedAndVerifiedForToolsFreeAgent`) runs
  both the tools-free assertion and the real `session/new` +
  `session/set_config_option` + verification flow with no model call, and the
  negative case where an unexposed mode fails the session naming the mode.

The exact captured payloads are probe artifacts, not repository fixtures; the
integration test recollects them through Matrix on every opt-in run.

An optional offline capture (a local OpenAI-compatible endpoint receiving the
provider's model request) observed the request shape behind the mode: with the
same synthetic prompt text, the `build` request carried ten tool definitions and
`tool_choice: "auto"` (bash, edit, glob, grep, read, ...), while the
`json-planning` request carried no `tools` key at all. The capture endpoint
answered in Chat Completions form while OpenCode expected the Responses API, so
that probe turn itself ended in the provider's API error after the request was
received; the decisive observation is the captured request body, not a completed
answer. This is the provider-owned half of the guarantee: the mode decides which
tools OpenCode announces, and Matrix verifies the mode.

## Guarantees and limits

Matrix guarantees, for an agent that declares `MATRIX_ACP_SESSION_MODE`:

- the declared mode is applied to every session it creates or resumes, or the
  session fails before a prompt;
- the application is confirmed by a provider statement, and the confirmation is
  recorded with its source;
- the failure is explicit and names the mode, so a caller cannot mistake an
  unavailable deployment for a successful tools-free turn;
- no prompt suggestion, tool list override or local environment variable is used
  to guess the role's tools.

Matrix does not guarantee, and the consumer must not assume:

- **the provider's request shape.** The provider decides which tools it announces
  to the model and which options it exposes. Matrix verifies the mode the provider
  exposes; it cannot inspect a model request, so "no tools were announced" is the
  provider's guarantee, established here by OpenCode's own tools-disabled agent.
- **arbitrary mode ids.** A mode id is provider-owned. OpenCode derives it from
  the custom agent's id; another provider may expose no mode surface at all, in
  which case a required mode fails closed rather than being approximated.
- **session-mode surface confirmation.** For ACP v1 `session/set_mode` the
  protocol acknowledgement carries no state, so the required contract waits for a
  `current_mode_update`; a provider that never sends one is reported unconfirmed.
- **persistence across provider restarts or resumes.** The mode is re-applied and
  re-verified each time; nothing is assumed to have survived.
- **model/credential independence.** The mode controls tools, not which model
  answers; model selection keeps its own contract and its own attestation.
