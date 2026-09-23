# Agent authentication in Matrix: what it does, and where its boundary is

This document records the boundary the Half Pocket team asked for: what Matrix
does when it authenticates an agent, what it deliberately does not do, and who
owns a credential once one exists.

## The flow Matrix implements

```
agent installed  ->  method advertised by the agent  ->  authenticate(methodID)
                 ->  credential stored in the vault  ->  later sessions reuse it
```

1. **The agent is asked first.** When onboarding needs to authenticate an agent,
   the generic ACP handler asks the agent — through the same protocol control the
   runtime uses — which methods it publishes in its `initialize` response. The
   method shown to the operator is the one the agent advertised.
2. **The advertised method is what runs.** For a method the agent published,
   Matrix performs the protocol `authenticate(methodID)` once the interactive part
   is done. A rejection is reported as a failure; Matrix does not treat "the user
   finished the terminal step" as a completed authorization.
3. **The credential lands in the vault.** An `env_var` method returns environment
   variables, and those are stored in the agent's override in the Matrix vault
   (`agent.meta.<id>` / entry override), which is where later sessions read them.
4. **The fallback is for agents that publish nothing.** When an agent advertises
   no method, or cannot be reached, the handler offers the generic API-key method.
   It is never offered *instead of* a method the agent advertised.

## Where the boundary is

Matrix authenticates **its own agents**, for the machine it runs on, and remembers
what it authorized. It is not a credential broker:

- It does not hold credentials for a shared account on behalf of several users.
- It does not lease, pin or expire credentials, and it does not rotate them.
- It does not hand credentials to worker processes other than the agent process it
  launches for the same user.
- It does not arbitrate a credential file that the agent itself owns and writes.
  When an agent's own login flow (for example `opencode auth login`) writes a file
  in its own configuration directory, that file is the agent's. Matrix asks the
  agent to authenticate and records that it did; it does not claim to own the
  resulting file.

An orchestrator that provisions agents on a host can therefore rely on Matrix for
"authenticate this agent once, on this machine, and remember it", and must keep
its own store for anything shared, leased or multi-tenant.

## What has no lifecycle yet

A stored credential is not re-validated. Matrix records that an authentication
happened and with which method; it does not know whether the credential is still
valid, and it has no refresh path. For a static API key that is survivable. For
anything OAuth-shaped it means the value can go stale and the next session has no
path back to health except asking a human again.

This is a known limitation, not an oversight in the wiring: refreshing, expiring
or re-validating a credential requires the agent to publish a refresh path, and
the registry index publishes no authentication information at all — the agent's
`initialize` response is the only source. It is tracked in the issue
`issues/closed/2026-09-23-acp-auth-unwired-and-credential-has-no-lifecycle.md`.

## Two writers, one file

For agents whose auth method is their own interactive login (the common shape for
ACP agents), the credential file is written by the agent, not by Matrix. A
personal login and a provisioned login for the same agent therefore contend for
the same file, and neither Matrix nor the agent declares ownership. Matrix's
record says which method it invoked and when; it cannot say which writer produced
the file's current contents. Detecting that conflict would require the agent to
report it, and no agent in the registry does.

## Reading the state

- `matrix agent info <id> --source=local` reports the installed version and the
  artifact verification recorded at install time.
- `matrix agent doctor` reports, per agent, the same evidence in machine-readable
  form.
- The ACP capability report (`authenticate`) is true only because the path above
  consumes it: the wizard is the consumer that makes the advertised capability
  reachable.
