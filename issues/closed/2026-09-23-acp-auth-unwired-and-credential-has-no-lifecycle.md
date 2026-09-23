# ACP auth exists as a capability but is not wired, and the credential has no lifecycle

Date observed: 2026-09-23

> **Note from an external agent.** Written by an agent working in the
> `halfpocket` repository, as the agreed reporting channel for problems found
> outside its own repository. **Nothing else in MATRIX has been touched**: no
> commit, no branch, no working-tree change. This file is deliberately left
> untracked so the MATRIX team can triage it and decide.

## Environment

- MATRIX `0.1.35`, commit `3c8158b`
- Linux workstation, read-only inspection
- Agents involved: `opencode` (ACP, installed locally), `codex-acp` (adapter not
  present at the expected path, so its `initialize` could not be observed)
- Live ACP registry index observed on the same day: 56,607 B, 41 agents

## Context: how we want to use MATRIX

We drive MATRIX from an orchestrator (the Half Pocket Hub) that provisions agents
on a host. The flow we are building toward is:

```
a2a / acp  ->  auth  ->  MATRIX vault (persistent)  ->  later sessions need no auth
```

Meaning, concretely:

1. An agent is installed and addressed over ACP or A2A.
2. Authentication happens **once**, through the protocol's own mechanism.
3. The resulting credential is **persisted in the MATRIX vault**, not in an
   env file and not in a caller's own store.
4. **Subsequent sessions reuse it.** No re-authentication, no interactive step,
   no operator present.

Point 4 is the whole point: MATRIX is the layer that makes agent auth a
one-time event for the machine, so that a session started tomorrow inherits what
was authorised today.

We are **not** asking MATRIX to become a credential broker for shared corporate
accounts, nor to hand secrets to other processes. That job already exists
elsewhere and we are not proposing to move it. What we are asking for is that
MATRIX's own auth path be real, complete and persistable for the agents it
installs.

## Symptom

### 1. The protocol path is implemented and has no consumers

`internal/providers/agents/acp_adapter.go:76` reads `authMethods` from the
agent's `initialize` response. `acp_features.go:87-103` exposes them, filtering
to types `""` and `"agent"`. `:105-113` can invoke `authenticate(methodID)` and
`:117-123` `logout`. The port is on `router_protocol_controls.go:10,18,26` and
`middleware/protocol_optional.go:64-72`.

**Nothing calls it.** An exhaustive search across the repository finds only the
interface, its implementation, and assertions in tests — no caller. It is a
socket with nothing plugged in.

### 2. The path that actually runs does not follow the protocol

The onboarding wizard is the only code that really authenticates an agent.
`auth_handler.go:52-60` registers `codex` and `opencode` and a fallback. The
fallback described as "generic ACP" **does not read `initialize`**: it returns a
hardcoded `api_key` / `env_var` method (`acp_auth_handler.go:19-29`).

The three auth method types and the `_meta.terminal-auth` / `_meta.api-key`
handling do exist (`:31-42,108-179`) — but they only ever react to an
`AuthMethod` supplied by the caller, and the only producers in the repository are
tests (`wizard_test.go:408,440`).

The doc comment at `acp_auth_handler.go:11-14` states that it supports "three auth
method types discovered from the agent's initialize response". **It does not.**
The comment is ahead of the code.

### 3. The credential has no lifecycle, so persistence means nothing

Credentials end up as `KEY=VALUE` in the agent's env override, stored in the
MATRIX vault (`wizard_steps.go:254-263` → `wizard.go:275-310` →
`agentcfg/store.go:15,51,108`), and are consumed by the agent process of the same
user.

There is **no refresh, no expiry, no lease, no rotation and no re-validation**.
For an API key that is survivable. For anything OAuth-shaped it defeats the goal
above: the value is persisted but goes stale, and the next session that needs a
valid token has no path back to health except asking a human again.

### 4. Two writers, one file, and neither declares it

The auth method that `opencode` itself publishes at `initialize`, observed from a
real handshake, is:

```json
[{"id":"opencode-login","name":"Login with opencode",
  "description":"Run `opencode auth login` in the terminal"}]
```

That is a **personal** login, and it writes the same file that an orchestrator
provisioning a corporate account also writes. The two surfaces do not duplicate
each other — they **contend for one file**, with no ownership declared anywhere
and no detection when one overwrites the other.

### 5. The registry publishes no authentication information

Measured on the live index: the keys are `authors`, `description`,
`distribution`, `icon`, `id`, `license`, `license_url`, `name`, `repository`,
`version`, `website`. There are **zero** occurrences of `token`, `apikey`,
`api_key`, `credential`, `oauth`, `login` or `secret`; the 41 occurrences of
`auth` are all inside `authors`.

This is not a defect — the correct source is the agent's `initialize` response,
which does carry it. It is recorded here because it means **the protocol path is
the only source**, and therefore that path being unwired (1 and 2) leaves no
alternative.

For completeness, on the same index: 19 of 41 agents publish a linux-x86_64
binary, and **only 10 of those 19 publish a `sha256`** (`antigravity-acp`,
`cortex-code`, `corust-agent`, `crow-cli`, `cursor`, `devin`, `junie`,
`stakpak`, `vtcode` do not).

## Impact

- The goal `auth once, reuse in later sessions` is **not reachable today** for
  any agent whose credential is not a static API key.
- An orchestrator cannot tell, from MATRIX, whether an agent is authenticated or
  whether auth will be demanded at the next session.
- The undeclared sharing of the credential file means a personal interactive
  login can silently invalidate a provisioned one, and vice versa.
- Because the port has no consumers, any integration written against the
  documented behaviour will find it does not run.

## Expected behavior

We are asking for the flow we described in **Context** to become real:

1. **The wizard reads `authMethods` from `initialize`**, so the path that runs and
   the protocol agree. The generic fallback should stop returning a hardcoded
   method when the agent has advertised its own.
2. **The advertised method is what gets executed** — `env_var`, `terminal`, or the
   agent-specific `_meta` flow — instead of a type the agent never claimed.
3. **A persisted credential has a lifecycle.** At minimum: know whether it is
   still valid; refresh it when the agent offers a refresh path; and expose that
   state so a caller can ask "does this agent need auth?" before starting a
   session rather than discovering it mid-run.
4. **One owner per credential file, declared.** If a personal login and a
   provisioned credential share a path, that must be stated, and MATRIX should be
   able to say which one it is currently holding.
5. **The auth state readable from outside**, so an orchestrator can act on it
   without reimplementing it. `matrix agent doctor` already has the shape of a
   surface that answers questions like this.

We are **not** asking for: shared-account brokerage, per-UID authorization,
leases, or handing credentials to worker processes. Those exist elsewhere in our
stack and are out of scope for this note. The boundary between "MATRIX
authenticates its agent and remembers it" and "an orchestrator brokers a shared
corporate account" is currently undocumented, and we would like it documented
more than we would like it moved.

## Suggested regression test

- **Protocol parity**: for an agent whose `initialize` advertises `authMethods`,
  assert that the wizard executes **the advertised method** and not a hardcoded
  one. A test that feeds a synthetic `AuthMethod` (as `wizard_test.go:408,440`
  does) cannot catch the defect, because it supplies the very input the real path
  is failing to read.
- **No dead port**: assert that the auth features exposed by the adapter have at
  least one consumer, or are not exposed. A capability nobody can invoke is worse
  than an absent one, because it is documented.
- **Persistence actually persists**: authenticate an OAuth-shaped agent once,
  then start a second session and assert no auth step is required. This is the
  test that would have caught the missing lifecycle.
- **File ownership**: if two flows can write the same credential path, assert that
  the second one detects and reports the conflict instead of overwriting.

## Open decisions we have already taken (so they are not re-litigated)

- **We are not moving our credential broker into MATRIX.** We verified an earlier
  hypothesis that our broker duplicated MATRIX's ACP auth; it is refuted. MATRIX
  has no representation of a shared account (no peer credentials, no lease, no
  pin, no expiry), and its `runtimebroker` is a vault *storage* RPC, not a
  credential broker.
- **We are not asking MATRIX's vault to hold a corporate credential.** Our own
  cache remains the authoritative copy, and the agent-side projection is
  write-only and discarded. A corporate value in the agent env override would be
  a second writer, not a copy.
- **The note is deliberately untracked.** Triage, reword, or discard it freely.

## Resolution (2026-09-23, Matrix workstream)

The first two symptoms were defects and are fixed; the rest is now documented
rather than implied, which is what this note asked for more than a change.

### Fixed: the path that runs now follows the protocol

- `onboarding.AgentAuthController` is a narrow port for the protocol side: the
  methods the agent publishes in `initialize`, and the `authenticate(methodID)`
  call that executes one. It is wired to the same `agents.Router` the runtime
  uses, immediately after that router is configured (`cmd/matrix/run.go`).
- `acpAuthHandler.Methods` now asks the agent. The hardcoded `api_key` method is
  offered **only** when the agent publishes no method, or when the protocol cannot
  be reached; it is never offered in place of a method the agent advertised.
- `acpAuthHandler.Authenticate` performs the protocol `authenticate(methodID)`
  once the interactive step is done, and reports a rejection as a failure instead
  of returning a successful-looking empty result. Completion of a terminal step is
  no longer treated as authorization.
- The handler's doc comment, which claimed behaviour the code did not have, now
  describes what the code does.
- The generic handler is bound to the agent it serves (`authHandlerRegistry.get`),
  because the methods it offers belong to that agent.

Both defects the note identified are covered by tests in
`internal/logic/onboarding/acp_auth_wiring_test.go`, written so that they fail
without the fix: with the protocol call removed, "the protocol authenticate was
not performed for the advertised method" and "a rejected authenticate must
surface as an error" both fail.

Scope note, stated plainly: `opencode` and `codex` keep their dedicated handlers
(OpenRouter login, Codex device auth) and do not take this path. The note's
example of opencode's advertised `opencode-login` is therefore still not the
method the wizard offers for opencode; the generic path is what the note's
expected behaviour is written about ("the generic fallback should stop returning
a hardcoded method"), and that is what changed.

### Documented: the boundary, ownership and the missing lifecycle

`docs/matrix_agent_auth.md` now records all of it:

- the flow Matrix implements, end to end;
- the boundary — Matrix authenticates its own agents for this machine and
  remembers them; it is not a credential broker, does not lease or expire, and
  does not arbitrate a credential file the agent itself writes;
- that a stored credential has **no lifecycle**: no re-validation, no refresh, no
  expiry, because the registry index publishes no authentication information and
  the agent's `initialize` is the only source;
- the two-writers-one-file contention: for agent-owned login files, Matrix records
  which method it invoked and when, and cannot say which writer produced the
  file's current contents.

### Not done, deliberately

- **Refresh / lease / expiry.** Requires a refresh path the agents do not
  currently publish. Adding a lease concept without one would be a claim, not a
  capability.
- **Validity probing.** The only honest test of a credential is a real session;
  a cheaper probe would report a green that means nothing.
- **Ownership arbitration between a personal and a provisioned login.** No agent
  in the registry reports which writer owns its credential file, so Matrix cannot
  detect the conflict without inventing a protocol.

### Evidence

`go test ./...` passes, `golangci-lint run ./...` reports 0 issues, `gofmt -l` is
clean, and the governance gate passes with the two package budgets raised and
dated for this work (`internal/logic/onboarding` 1400→1415, `cmd/matrix`
3231→3235).
