# Issue: Codex ACP must support trusted terminal / max-permissions mode for local workspaces

Opened: 2026-06-23  
Reporter: Half Pocket Desk integration  
Status: closed

## Summary

Half Pocket Desk routes Roberto's AI-first dashboard through local MATRIX and
Codex. For this use case, MATRIX must run Codex with trusted terminal access on
the explicitly selected local workspace, so agentic reads and CLI calls do not
fail through Codex sandboxing or require manual permission escalation.

Current behavior is inconsistent:

1. With `agent.trust_mode=false`, MATRIX receives ACP
   `session/request_permission` and auto-denies it.
2. After setting `agent.trust_mode=true`, MATRIX no longer denies ACP
   permission requests, but Codex still first attempts sandboxed `exec_command`
   calls that fail with:

```text
bwrap: loopback: Failed RTM_NEWADDR: Operation not permitted
```

3. Codex then tries escalated execution (`sandbox_permissions=require_escalated`)
   but that path is not useful in this non-interactive MATRIX flow.
4. In the same run, a later ACP/unified terminal execution can succeed, proving
   the workspace and command are valid, but the user-facing run still contains
   noisy failed sandbox attempts.

## Expected Behavior

For trusted local workspaces, MATRIX should be able to launch Codex ACP in a
max-permissions mode equivalent to direct Codex CLI usage with:

- no Codex sandbox for model-generated shell commands;
- no interactive approval requirement inside HTTP/MATRIX runs;
- terminal/file tools allowed when `agent.trust_mode=true`;
- clear audit events showing the run used trusted terminal mode;
- no user-visible fallback text such as "ripeto fuori sandbox" for ordinary
  read-only CLI operations.

This should be controlled by explicit MATRIX configuration, not by Half Pocket
prompt workarounds.

## Evidence

Local MATRIX runtime:

```text
matrix.service active
WorkingDirectory=%h/.local/share/matrix
MATRIX HTTP: 127.0.0.1:9091
agent: codex via codex-acp
workspace_path: /home/jose/halfpocket
```

Before mitigation:

```text
run_id: run-79a4908d-9a12-4474-a116-fcb2e8d97a53
permission_requested: 1
permission_denied: 1
permission_approved: 0
bwrap failures: 1
```

Representative event:

```text
kind: permission.requested
summary: Permission requested for /home/jose/halfpocket/AGENTS.md
operation: awk
options: allow_once, allow_always, reject_once

kind: permission.resolved
decision: denied
```

Local mitigation applied:

```bash
systemctl --user stop matrix.service
matrix config set agent.trust_mode true
systemctl --user start matrix.service
```

After mitigation, non-mutating diagnostic run:

```text
run_id: run-fa4cae09-5833-40c3-85c0-4aa782cf8bba
prompt: read AGENTS.md first line and execute pwd in /home/jose/halfpocket
status: completed
final output:
  pwd: `/home/jose/halfpocket`
  AGENTS.md prima riga: `# Half Pocket - Istruzioni agenti`
```

But the run still recorded:

```text
permission_requested: 0
permission_denied: 0
bwrap failures: 10
unified_exec_success: 1
```

Representative failed tool output:

```text
raw_input:
  cmd: cat AGENTS.md
  workdir: /home/jose/halfpocket

raw_output:
  bwrap: loopback: Failed RTM_NEWADDR: Operation not permitted
```

Representative successful later execution:

```text
raw_input:
  command: ["/bin/bash", "-lc", "pwd; sed -n '1p' /home/jose/halfpocket/AGENTS.md"]
  cwd: /home/jose/halfpocket
  source: unified_exec_startup

raw_output:
  exit_code: 0
  stdout:
    /home/jose/halfpocket
    # Half Pocket - Istruzioni agenti
```

## Likely Root Cause

MATRIX `agent.trust_mode=true` controls MATRIX's ACP request handler:

```text
internal/providers/agents/default_handler.go
internal/providers/agents/permission_handler.go
cmd/matrix/run.go
```

However, Codex ACP itself appears to keep its own sandbox/approval policy for
model-generated `exec_command` calls. `codex-acp --help` supports `-c
key=value` overrides, while direct `codex --help` exposes:

```text
--sandbox danger-full-access
--dangerously-bypass-approvals-and-sandbox
--ask-for-approval never
```

MATRIX likely needs a first-class Codex ACP configuration surface to pass the
equivalent runtime policy into `codex-acp`.

## Related Operational Issue

While `matrix.service` is running, local `matrix config get/list` can fail with:

```text
vault error: [ERR_VAULT_OPEN] Failed to open bbolt database: timeout (op: bolt.NewProvider)
```

This forced a stop/set/start sequence for `agent.trust_mode=true`. If this is
expected because the daemon owns the vault, the operator path should be
documented or routed through a daemon API. If not expected, it is a lock/contention
bug.

## Acceptance Criteria

- A local operator can configure Codex ACP max-permissions/trusted-terminal mode
  through MATRIX config or agent override.
- `POST /v1/runs` against a trusted workspace can execute a read-only command
  like `pwd; sed -n '1p' AGENTS.md` without `bwrap` failures.
- `agent.trust_mode=true` behavior is documented as approving MATRIX ACP
  permission requests, while Codex sandbox configuration is separately explicit.
- Run trace exposes enough evidence to know whether a run used trusted terminal
  mode.
- `matrix config get/set/list` either works while daemon is active or documents
  the supported operator path.

## Maintainer Response

Resolved on 2026-06-23.

Matrix now supports appended agent launch arguments through the SSOT override
layer:

```bash
matrix agent args set codex -- -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"'
```

This keeps `agent.trust_mode=true` scoped to Matrix ACP permission handling and
keeps Codex sandbox/approval policy as explicit provider launch config.

Run traces now add launch-policy evidence on `routing.decision` when recognized
non-secret command arguments are present:

```json
{
  "agent_launch_policy": {
    "source": "agent_args",
    "sandbox_mode": "danger-full-access",
    "approval_policy": "never",
    "trusted_terminal": true
  }
}
```

Docs were updated to cover `matrix agent args`, Codex ACP trusted terminal mode,
the trust boundary split, trace evidence, and the supported stop/config/start
operator path when the daemon owns the bbolt vault lock.

Verification:

```bash
go test ./internal/logic/agentcfg ./internal/logic/agentmgr ./internal/providers/runapi ./cmd/matrix
```
