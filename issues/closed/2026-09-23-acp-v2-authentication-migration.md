# ACP v2 authentication: Matrix speaks the previous generation of the protocol

Date observed: 2026-09-23
Status: closed — the migration is complete

## What is true today

Matrix's ACP client (`pkg/zedacp`) negotiates `ProtocolVersion: 1`
(`internal/providers/agents/acp_adapter.go:56`) and implements the v1
authentication surface:

- methods `authenticate` and `logout`
  (`pkg/zedacp/client.go:221`, `pkg/zedacp/client.go:394`);
- `authMethods` entries carrying `id`, `name`, `description`, `type` (optional)
  and `_meta` (`pkg/zedacp/types.go:132`);
- the adapter filters advertised methods to types `""` and `"agent"`
  (`internal/providers/agents/acp_features.go:87-103`);
- authentication failures are recognised by substring matching on the error text
  (`internal/providers/agents/provider_failure.go:61`, `strings.Contains(lower,
  "auth")`), not by a structured error.

## What the current specification defines

From the ACP v2 authentication page (https://agentclientprotocol.com/protocol/v2/authentication.md),
verified 2026-09-23:

- the methods are `auth/login` and `auth/logout`, addressed by `methodId`;
- every authentication method **must** carry a `type` discriminator; the standard
  type is `agent`, and custom types **must** begin with `_`;
- `authMethods` being non-empty obliges the agent to implement both `auth/login`
  and `auth/logout`; an empty or absent list means clients **must not** call them;
- the `terminal` type is not a text prompt: the client launches the configured
  agent program in an interactive terminal with the method's `args` and `env`,
  waits for exit, then **reconnects and re-initializes** the agent and retries the
  operation that required authentication. A client must **not** send `auth/login`
  for a terminal method, and must advertise `capabilities.auth.terminal` during
  initialization to be allowed to receive one;
- authentication-gated requests answer with an `auth_required` error, and after
  `auth/logout` active sessions may terminate or start answering `auth_required`.

## The gap, precisely

| Concern | Spec v2 | Matrix today |
| --- | --- | --- |
| Method names | `auth/login`, `auth/logout` | `authenticate`, `logout` |
| Identifier | `methodId` | `id` |
| Method type | mandatory, with `terminal` and `_*` customs | optional, filtered to `""`/`agent` |
| Terminal flow | relaunch agent, reconnect, reinitialize | print a command, wait for "done" |
| Client capability | `capabilities.auth.terminal` | not advertised |
| Gated requests | structured `auth_required` | substring heuristics |

## Why this is not a one-line rename

1. **Negotiation**: installed agents answer v1 today. The client has to keep
   speaking v1 to them and v2 to agents that declare it, which means the version
   has to be carried through the adapter, the feature report and the wizard rather
   than assumed.
2. **Terminal semantics**: the v2 flow is a process re-launch plus reconnect and
   reinitialize, driven by the launch configuration of the connection. Matrix's
   wizard currently treats it as a prompt, which is exactly the behaviour the spec
   forbids for `terminal`.
3. **Structured errors**: `auth_required` should replace the substring heuristic,
   and the runtime should retry the gated operation after a successful login
   instead of surfacing a provider failure.
4. **Capability report**: `authenticate` is advertised per the v1 shape
   (`internal/providers/agents/acp_protocol_capabilities.go:8`); the operation
   names and the client capability set both change.

## Done: the negotiated surface

The first slice is implemented. `pkg/zedacp` now speaks both generations and
chooses by what the agent agrees to:

- `Initialize` asks for the highest supported version (`MaxSupportedProtocolVersion
  = 2`) unless a caller pins one, retries once at v1 when an agent refuses the
  newer number, and records the agreed version. An agent that declares nothing is
  treated as v1, so an installed agent that omits the field is never spoken to in
  v2.
- `Authenticate` and `Logout` choose their wire method from the agreed version:
  `authenticate`/`logout` for v1, `auth/login`/`auth/logout` for v2.
- `AuthMethod` carries `methodId` alongside `id`, and `Identifier()` reads
  whichever the agent supplied; the adapter uses it, so an agent speaking either
  version is understood.
- `zedacp.IsAuthenticationRequired` recognises v2's structured `auth_required`
  signal in the error data, with a bounded walk because that data comes from the
  agent process; the provider classifier consults it before its text heuristics.

Deliberate non-goal in this slice: Matrix does **not** advertise
`capabilities.auth.terminal` and does not offer `terminal` methods, because it does
not implement the relaunch flow. Advertising it would be a claim, not a capability.

Evidence: `pkg/zedacp/auth_negotiation_test.go` covers both generations, the
never-exceed-requested rule, the undeclared-version default, the narrow version
rejection trigger, the structured signal in five shapes plus false positives, and a
5000-deep payload against the depth bound.

## Resolution (2026-09-23): the terminal flow and retry-after-login shipped

Commit `58a302a` completes the migration:

- **Terminal methods**, opt-in and v2-only: `capabilities.auth.terminal` is
  advertised only when the operator enables `agent.terminal_auth_enabled` and a
  process backend exists, and the same predicate decides whether the method can be
  run, so Matrix cannot advertise a flow it would refuse. Running one launches the
  configured program with the method's `args` and `env` (overriding same-named base
  variables), treats a non-zero or signalled exit, a run error or a cancellation as
  failure, never sends `auth/login`, and on success rebuilds the connection from the
  same endpoint and reinitializes before the gated operation is retried.
- **Structured `auth_required`**: one login and exactly one retry, with a second
  failure surfaced and the original signal still wrapped; without a runnable method
  the gate stays visible.
- **Everything runnable is offered**: terminal methods on v2 connections, custom
  underscore-prefixed types reported generically without invented semantics.
- **Two pre-existing gaps had to be fixed for any of it to be reachable**: `doCall`
  flattened RPC errors, so the structured `auth_required` data could never survive a
  real error, and the adapter rejected any initialize response whose raw
  `protocolVersion` was not 1, which refused every v2 agent and even v1 agents that
  omit the field.
- **The initialize request now uses the parameter names of the generation it
  requests**: v2 renamed `clientCapabilities`/`clientInfo`, so a spec-strict v2 agent
  previously read no advertisement at all.

### What remains true and is not a defect to fix

- Matrix does not implement a TTY surface, so a terminal login that requires an
  interactive terminal cannot complete through the daemon; the method runs through
  the process provider with piped stdio and is bounded by the caller's context.
- No v2 agent exists on this workstation, so the flow is verified against the
  specification and test doubles, not against a real peer.

## Original scope note: what this issue covered

## What is already done, so it is not re-done

- The advertised methods are read from the agent instead of being hardcoded, and
  completing an advertised method performs the protocol authenticate call
  (commit `4249610`).
- The methods an agent advertises and revoking them are reachable at runtime over
  `GET /v1/agent-auth?agent=<id>` and `POST /v1/agent-auth/logout?agent=<id>`
  (commit `625e033`).
